package runner

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/google/gopacket/pcap"
	"github.com/phayes/freeport"
	_ "github.com/projectdiscovery/fdmax/autofdmax"
	"github.com/projectdiscovery/hmap/store/hybrid"
	ratelimit "golang.org/x/time/rate"
	"io"
	"ksubdomain/core"
	"ksubdomain/gologger"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

type runner struct {
	ether        core.EthTable
	hm           *hybrid.HybridMap
	options      *Options
	limit        *ratelimit.Limiter
	handle       *pcap.Handle
	successIndex uint64
	sentIndex    uint64
	recvIndex    uint64
	faildIndex   uint64
	sender       chan core.StatusTable
	recver       chan core.RecvResult
	freeport     int
	dnsid        uint16 // dnsid 用于接收的确定ID
	maxRetry     int    // 最大重试次数
	timeout      int64
	lock         sync.RWMutex
}

func New(options *Options) (*runner, error) {
	version := pcap.Version()
	r := new(runner)
	r.options = options
	gologger.Infof(version + "\n")
	if options.ListNetwork {
		core.GetIpv4Devices()
		os.Exit(0)
	}
	var ether core.EthTable
	if options.NetworkId == -1 {
		ether = core.AutoGetDevices()
	} else {
		ether = core.GetDevices(options.NetworkId)
	}
	r.ether = ether
	if options.Test {
		TestSpeed(ether)
		os.Exit(0)
	}

	hm, err := hybrid.New(hybrid.DefaultDiskOptions)
	if err != nil {
		return nil, err
	}
	r.hm = hm

	// get targets
	var f io.Reader
	if options.Stdin {
		if options.Verify {
			f = os.Stdin
		} else {
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Split(bufio.ScanLines)
			for scanner.Scan() {
				options.Domain = append(options.Domain, scanner.Text())
			}
		}
	}
	if len(options.Domain) > 0 {
		if options.Verify {
			f = strings.NewReader(strings.Join(options.Domain, "\n"))
		} else if options.FileName == "" {
			gologger.Infof("加载内置字典\n")
			f = strings.NewReader(core.GetSubdomainData())
		}
	} else {
		f2, err := os.Open(options.FileName)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("打开文件:%s 错误:%s", options.FileName, err.Error()))
		}
		defer f2.Close()
		f = f2
	}
	if options.Verify && options.FileName != "" {
		f2, err := os.Open(options.FileName)
		if err != nil {
			gologger.Fatalf("\n", options.FileName, err.Error())
			return nil, errors.New("打开文件:%s 出现错误:%s")
		}
		defer f2.Close()
		f = f2
	}
	if options.SkipWildCard && len(options.Domain) > 0 {
		var tmpDomains []string
		gologger.Infof("检测泛解析\n")
		for _, domain := range options.Domain {
			if !core.IsWildCard(domain) {
				tmpDomains = append(tmpDomains, domain)
			} else {
				gologger.Warningf("域名:%s 存在泛解析记录,已跳过\n", domain)
			}
		}
		options.Domain = tmpDomains
	}
	if len(options.Domain) > 0 {
		gologger.Infof("检测域名:%s\n", options.Domain)
	}
	gologger.Infof("设置rate:%dpps\n", options.Rate)
	gologger.Infof("DNS:%s\n", options.Resolvers)

	r.pcapInit()
	r.limit = ratelimit.NewLimiter(ratelimit.Every(time.Duration(time.Second.Nanoseconds()/options.Rate)), int(options.Rate))
	r.sender = make(chan core.StatusTable, r.options.Rate)
	r.recver = make(chan core.RecvResult)
	tmpFreeport, err := freeport.GetFreePort()
	if err != nil {
		return nil, err
	}
	r.freeport = tmpFreeport
	gologger.Infof("获取FreePort:%d\n", tmpFreeport)
	r.dnsid = 0x2021 // set dnsid 65500
	r.maxRetry = r.options.Retry
	r.timeout = int64(r.options.TimeOut)
	r.lock = sync.RWMutex{}

	go r.loadTargets(f)
	return r, nil
}
func (r *runner) ChoseDns() string {
	dns := r.options.Resolvers
	return dns[rand.Intn(len(dns))]
}
func (r *runner) pcapInit() {
	var (
		snapshot_len int32 = 1024
		//promiscuous  bool  = false
		err     error
		timeout time.Duration = -1 * time.Second
	)
	r.handle, err = pcap.OpenLive(r.ether.Device, snapshot_len, false, timeout)
	if err != nil {
		gologger.Fatalf("pcap初始化失败:%s\n", err.Error())
		return
	}
}
func (r *runner) loadTargets(f io.Reader) {
	reader := bufio.NewReader(f)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			break
		}
		msg := string(line)
		if r.options.Verify {
			// send msg
			r.sender <- core.StatusTable{
				Domain:      msg,
				Dns:         r.ChoseDns(),
				Time:        0,
				Retry:       0,
				DomainLevel: 0,
			}
		} else {
			for _, tmpDomain := range r.options.Domain {
				newDomain := msg + "." + tmpDomain
				// send newdomain
				r.sender <- core.StatusTable{
					Domain:      newDomain,
					Dns:         r.ChoseDns(),
					Time:        0,
					Retry:       0,
					DomainLevel: 0,
				}
			}
		}
	}
}
func (r *runner) PrintStatus() {
	gologger.Printf("\rSuccess:%d Sent:%d Recved:%d Faild:%d", r.successIndex, r.sentIndex, r.recvIndex, r.faildIndex)
}
func (r *runner) RunEnumeration() {
	go r.recv()         // 启动接收线程
	go r.handleResult() // 处理结果，打印输出
	go r.sendCycle()    // 发送线程
	go r.retry()        // 遍历hm，依次重试
	// 主循环
	for {
		time.Sleep(time.Millisecond * 500)
		r.PrintStatus()
		if r.hm.Size() == 0 {
			break
		}
	}
	gologger.Printf("\n")
	for i := 5; i >= 0; i-- {
		gologger.Printf("检测完毕，等待%ds\n", i)
		time.Sleep(time.Second)
	}

	if r.options.FilterWildCard {
		r.FilterWildCard()
	}
	if r.options.OutputCSV {
		gologger.Printf("\n")
		OutputExcel(r.options.Output)
	}
}
func (r *runner) handleResult() {
	var isWrite bool = false
	var err error
	var windowWith int

	if r.options.Silent {
		windowWith = 0
	} else {
		windowWith = core.GetWindowWith()
	}

	if r.options.Output != "" {
		isWrite = true
	}
	var foutput *os.File
	if isWrite {
		foutput, err = os.OpenFile(r.options.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			gologger.Errorf("写入结果文件失败：%s\n", err.Error())
		}
	}
	for {
		result := <-r.recver
		var content []string
		content = append(content, result.Subdomain)
		for _, v := range result.Answers {
			content = append(content, v.String())
		}
		msg := strings.Join(content, " => ")

		fontlenth := windowWith - len(msg) - 1
		if !r.options.Silent {
			if windowWith > 0 && fontlenth > 0 {
				gologger.Silentf("\r%s% *s\n", msg, fontlenth, "")
			} else {
				gologger.Silentf("\r%s\n", msg)
			}
		} else {
			gologger.Silentf("%s\n", msg)
		}
		if isWrite {
			w := bufio.NewWriter(foutput)
			_, err = w.WriteString(content[0] + "\n")
			if err != nil {
				gologger.Errorf("写入结果文件失败.Err:%s\n", err.Error())
			}
			_ = w.Flush()
		}
	}
}
func (r *runner) Close() {
	r.handle.Close()
	r.hm.Close()
}
