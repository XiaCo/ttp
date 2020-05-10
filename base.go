package ttp

import (
	"github.com/golang/protobuf/proto"
	"sync/atomic"
	"time"
)

type (
	TTPMessage interface {
		GetAck() uint32
		SetAck(uint32)
		GetPath() string
		SetPath(string)
		GetNumbers() []uint32
		SetNumbers([]uint32)
		GetStart() uint32
		SetStart(uint32)
		GetSpeed() uint32
		SetSpeed(uint32)
		GetData() []byte
		SetData([]byte)

		Reset()
	}

	TTPInterpreter interface {
		Marshal(TTPMessage) ([]byte, error)
		Unmarshal([]byte, TTPMessage) error
	}

	SpeedCalculator interface {
		GetSpeed() uint32 // 获取当前速度
		AddFlow(uint32)   // 增加流量
		Close()           // 关闭
	}
)

type ProtobufInterpreter int

func (p ProtobufInterpreter) Marshal(v TTPMessage) ([]byte, error) {
	if msg, ok := v.(proto.Message); ok {
		return proto.Marshal(msg)
	} else {
		return nil, protobufTypeErr
	}
}

func (p ProtobufInterpreter) Unmarshal(buf []byte, v TTPMessage) error {
	if msg, ok := v.(proto.Message); ok {
		return proto.Unmarshal(buf, msg)
	} else {
		return protobufTypeErr
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
	close(s.over)
}

func NewSpeedCalculator(t time.Duration) *speedCalculator {
	delay := time.NewTicker(t)
	s := speedCalculator{0, 0, delay, make(chan struct{})}
	go func() { // update speed every t
		for {
			select {
			case <-delay.C:
				f := atomic.LoadUint32(&s.flow)
				atomic.StoreUint32(&s.speed, f/uint32(t.Seconds()))
				atomic.AddUint32(&s.flow, -f)
			case <-s.over:
				return
			}
		}
	}()
	return &s
}
