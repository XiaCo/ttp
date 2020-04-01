package ttp

import (
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ttpUnit uint32 = 1472
)

var (
	bytePool sync.Pool
)

func init() {
	bytePool.New = func() interface{} {
		return make([]byte, ttpUnit)
	}
}

func SplitFile(size int64) uint32 {
	// 分割文件成小块编号
	max := uint32(math.Ceil(float64(size) / float64(SplitFileSize)))
	return max
}

func SavePathIsValid(p string) bool {
	_, statErr := os.Stat(p)
	if statErr != nil {
		return !os.IsExist(statErr)
	}
	return false
}

func SleepAfterSendPackage(n uint32, sendSpeed uint32) func() {
	// n: 多少次作为一批发送的数据
	// sendSpeed: 需要控制的速率，单位 kb/s
	count := uint32(0)
	sleepTime := time.Duration(int64(n) * 1000000000 / int64(sendSpeed))
	return func() {
		count++
		if count == n {
			time.Sleep(sleepTime)
			count = 0
		}
	}
}

type speedCalculator struct {
	flow  uint32
	speed uint32
	t     *time.Ticker
	over  chan struct{}
}

func (s *speedCalculator) GetSpeed() uint32 {
	return atomic.LoadUint32(&s.speed)
}

func (s *speedCalculator) AddFlow(n uint32) {
	atomic.AddUint32(&s.flow, n)
}

func (s *speedCalculator) Close() {
	s.t.Stop()
	s.over <- struct{}{}
}

func NewSpeedCalculator(t time.Duration) *speedCalculator {
	delay := time.NewTicker(t)
	s := speedCalculator{0, 0, delay, make(chan struct{})}
	go func() {
		for {
			select {
			case <-delay.C:
				f := atomic.LoadUint32(&s.flow)
				atomic.StoreUint32(&s.speed, f/uint32(t.Seconds()))
				atomic.StoreUint32(&s.flow, 0)
			case <-s.over:
				return
			}
		}
	}()
	return &s
}
