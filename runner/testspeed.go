package runner

import (
	"ksubdomain/core"
)

func TestSpeed(ether core.EthTable) {
	//sendog := core.SendDog{}
	//ether.DstMac = net.HardwareAddr{0x5c, 0xc9, 0x09, 0x33, 0x34, 0x80} // 指定一个错误的dstmac地址，包会经过本机网卡，但是发不出去
	//sendog.Init(ether, []string{"8.8.8.8"}, 404, false)
	//defer sendog.Close()
	//var index int64 = 0
	//start := time.Now().UnixNano() / 1e6
	//flag := int64(15) // 15s
	//var now int64
	//for {
	//	sendog.Send("seebug.org", "8.8.8.8", 1234, 1)
	//	index++
	//	now = time.Now().UnixNano() / 1e6
	//	tickTime := (now - start) / 1000
	//	if tickTime >= flag {
	//		break
	//	}
	//	if (now-start)%1000 == 0 && now-start >= 900 {
	//		tickIndex := index / tickTime
	//		gologger.Printf("\r %ds 总发送:%d Packet 平均每秒速度:%dpps", tickTime, index, tickIndex)
	//	}
	//}
	//now = time.Now().UnixNano() / 1e6
	//tickTime := (now - start) / 1000
	//tickIndex := index / tickTime
	//gologger.Printf("\r %ds 总发送:%d Packet 平均每秒速度:%dpps\n", tickTime, index, tickIndex)
}
