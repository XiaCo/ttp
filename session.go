package ttp

import (
	"net"
	"sync"
	"time"
)

const mtu uint32 = 1500

var (
	bytePool sync.Pool
)

func init() {
	bytePool.New = func() interface{} {
		return make([]byte, 0, mtu)
	}
}

type (
	TTPSession struct {
		conn       *net.UDPConn
		remoteAddr *net.UDPAddr
		ttpMap     sync.Map //map[[14]byte]*TTP
	}
)

func (s *TTPSession) Write([]byte) (n int, err error) {

}

func (s *TTPSession) Read([]byte) (n int, err error) {

}

func (s *TTPSession) Close() error {

}

func (s *TTPSession) LocalAddr() net.Addr {

}

func (s *TTPSession) RemoteAddr() net.Addr {

}

func (s *TTPSession) SetDeadline(t time.Time) error {

}

func (s *TTPSession) SetReadDeadline(t time.Time) error {

}
func (s *TTPSession) SetWriteDeadline(t time.Time) error {

}

func NewTTPSession(conn *net.UDPConn) *TTPSession {
	return &TTPSession{}
}
