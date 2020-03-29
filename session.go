package ttp

import (
	"errors"
	"net"
	"sync"
	"time"
)

type (
	TTPSession struct {
		listener   *Listener
		conn       *net.UDPConn
		remoteAddr *net.UDPAddr
		ttpMap     *sync.Map //map[TTPId]*TTP
		readQueue  chan []byte
		writeQueue chan []byte
	}

	TTPId [8]byte
)

func (s *TTPSession) preprocessAndSend() {
	for msg := range s.writeQueue {
		s.conn.WriteToUDP(msg, s.remoteAddr)
	}
}

func (s *TTPSession) recvAndHandle() {
	for msg := range s.readQueue {
		data, id := s.split(msg)
		if t, exist := s.ttpMap.Load(id); exist {
			t.(*TTP).Read(data)
		} else {
			ttp := NewTTP(s)
			s.ttpMap.Store(id, ttp)
		}
	}
}

func (s *TTPSession) closeTTP(id TTPId) {
	s.ttpMap.Delete(id)
}

func (s *TTPSession) split(b []byte) ([]byte, TTPId) {
	var id TTPId
	sl := id[:]
	copy(sl, b[:8])
	return b[8:], id
}

func (s *TTPSession) Write(buf []byte) (n int, err error) {
	// send buf to writeQueue
	select {
	case s.writeQueue <- buf:
		return len(buf), nil
	default:
		return 0, errors.New("writeQueue is full")
	}
}

func (s *TTPSession) Read(buf []byte) (n int, err error) {
	// send buf to readQueue
	b := bytePool.Get().([]byte)
	copy(b, buf)
	select {
	case s.readQueue <- b[:len(buf)]:
		return len(b), nil
	default:
		return 0, errors.New("readQueue is full")
	}
}

func (s *TTPSession) Pull(remoteFilePath string, localSavePath string, speedKBS uint32) error {
	ttp := NewTTP(s)
	return ttp.Pull(remoteFilePath, localSavePath, speedKBS)
}

func (s *TTPSession) Close() error {
	s.listener.closeSession(s.remoteAddr.String())
	close(s.readQueue)
	close(s.writeQueue)
	return nil
}

func (s *TTPSession) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *TTPSession) RemoteAddr() net.Addr {
	return s.remoteAddr
}

func (s *TTPSession) SetDeadline(t time.Time) error {
	return nil
}

func (s *TTPSession) SetReadDeadline(t time.Time) error {
	return nil
}
func (s *TTPSession) SetWriteDeadline(t time.Time) error {
	return nil
}

func NewTTPSession(ls *Listener, remoteAddr *net.UDPAddr) *TTPSession {
	ts := &TTPSession{
		ls,
		ls.conn,
		remoteAddr,
		new(sync.Map),
		make(chan []byte, 1024),
		make(chan []byte, 1024),
	}
	go ts.preprocessAndSend()
	go ts.recvAndHandle()
	return ts
}
