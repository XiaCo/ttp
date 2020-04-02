package ttp

import "github.com/golang/protobuf/proto"

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
		Marshal(v interface{}) ([]byte, error)
		Unmarshal(buf []byte, v interface{}) error
	}

	SpeedCalculator interface {
		GetSpeed() uint32 // 获取当前速度
		AddFlow(uint32)   // 增加流量
		Close()           // 关闭
	}
)

type ProtobufInterpreter int

func (p ProtobufInterpreter) Marshal(v interface{}) ([]byte, error) {
	if msg, ok := v.(proto.Message); ok {
		return proto.Marshal(msg)
	} else {
		return nil, protobufTypeErr
	}
}

func (p ProtobufInterpreter) Unmarshal(buf []byte, v interface{}) error {
	if msg, ok := v.(proto.Message); ok {
		return proto.Unmarshal(buf, msg)
	} else {
		return protobufTypeErr
	}
}
