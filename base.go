package ttp

type SpeedCalculator interface {
	GetSpeed() uint32 // 获取当前速度
	AddFlow(uint32)   // 增加流量
	Close()           // 关闭
}
