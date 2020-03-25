// Code generated by protoc-gen-go. DO NOT EDIT.
// source: udpfile.proto

package ttp

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type UDPFilePackage struct {
	Ack                  uint32   `protobuf:"varint,1,opt,name=ack,proto3" json:"ack,omitempty"`
	Path                 string   `protobuf:"bytes,2,opt,name=path,proto3" json:"path,omitempty"`
	Number               []uint32 `protobuf:"varint,3,rep,packed,name=number,proto3" json:"number,omitempty"`
	Start                uint32   `protobuf:"varint,4,opt,name=start,proto3" json:"start,omitempty"`
	Speed                uint32   `protobuf:"varint,5,opt,name=speed,proto3" json:"speed,omitempty"`
	Data                 []byte   `protobuf:"bytes,6,opt,name=data,proto3" json:"data,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UDPFilePackage) Reset()         { *m = UDPFilePackage{} }
func (m *UDPFilePackage) String() string { return proto.CompactTextString(m) }
func (*UDPFilePackage) ProtoMessage()    {}
func (*UDPFilePackage) Descriptor() ([]byte, []int) {
	return fileDescriptor_46504f52a9d6c315, []int{0}
}

func (m *UDPFilePackage) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UDPFilePackage.Unmarshal(m, b)
}
func (m *UDPFilePackage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UDPFilePackage.Marshal(b, m, deterministic)
}
func (m *UDPFilePackage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UDPFilePackage.Merge(m, src)
}
func (m *UDPFilePackage) XXX_Size() int {
	return xxx_messageInfo_UDPFilePackage.Size(m)
}
func (m *UDPFilePackage) XXX_DiscardUnknown() {
	xxx_messageInfo_UDPFilePackage.DiscardUnknown(m)
}

var xxx_messageInfo_UDPFilePackage proto.InternalMessageInfo

func (m *UDPFilePackage) GetAck() uint32 {
	if m != nil {
		return m.Ack
	}
	return 0
}

func (m *UDPFilePackage) GetPath() string {
	if m != nil {
		return m.Path
	}
	return ""
}

func (m *UDPFilePackage) GetNumber() []uint32 {
	if m != nil {
		return m.Number
	}
	return nil
}

func (m *UDPFilePackage) GetStart() uint32 {
	if m != nil {
		return m.Start
	}
	return 0
}

func (m *UDPFilePackage) GetSpeed() uint32 {
	if m != nil {
		return m.Speed
	}
	return 0
}

func (m *UDPFilePackage) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

func init() {
	proto.RegisterType((*UDPFilePackage)(nil), "UDPFilePackage")
}

func init() {
	proto.RegisterFile("udpfile.proto", fileDescriptor_46504f52a9d6c315)
}

var fileDescriptor_46504f52a9d6c315 = []byte{
	// 156 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2d, 0x4d, 0x29, 0x48,
	0xcb, 0xcc, 0x49, 0xd5, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x57, 0xea, 0x63, 0xe4, 0xe2, 0x0b, 0x75,
	0x09, 0x70, 0xcb, 0xcc, 0x49, 0x0d, 0x48, 0x4c, 0xce, 0x4e, 0x4c, 0x4f, 0x15, 0x12, 0xe0, 0x62,
	0x4e, 0x4c, 0xce, 0x96, 0x60, 0x54, 0x60, 0xd4, 0xe0, 0x0d, 0x02, 0x31, 0x85, 0x84, 0xb8, 0x58,
	0x0a, 0x12, 0x4b, 0x32, 0x24, 0x98, 0x14, 0x18, 0x35, 0x38, 0x83, 0xc0, 0x6c, 0x21, 0x31, 0x2e,
	0xb6, 0xbc, 0xd2, 0xdc, 0xa4, 0xd4, 0x22, 0x09, 0x66, 0x05, 0x66, 0x0d, 0xde, 0x20, 0x28, 0x4f,
	0x48, 0x84, 0x8b, 0xb5, 0xb8, 0x24, 0xb1, 0xa8, 0x44, 0x82, 0x05, 0xac, 0x1f, 0xc2, 0x01, 0x8b,
	0x16, 0xa4, 0xa6, 0xa6, 0x48, 0xb0, 0x42, 0x45, 0x41, 0x1c, 0x90, 0xb9, 0x29, 0x89, 0x25, 0x89,
	0x12, 0x6c, 0x0a, 0x8c, 0x1a, 0x3c, 0x41, 0x60, 0x76, 0x12, 0x1b, 0xd8, 0x5d, 0xc6, 0x80, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xb4, 0xb3, 0xf4, 0xa3, 0xa8, 0x00, 0x00, 0x00,
}
