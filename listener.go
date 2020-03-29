package ttp

import (
	"net"
	"sync"
	"sync/atomic"
)

type Listener struct {
	conn         *net.UDPConn
	sessionMap   *sync.Map //map[string]*TTPSession
	sessionQueue chan *TTPSession
	over         uint32 // 0 or 1
}

func (l *Listener) Close() error {
	atomic.StoreUint32(&l.over, 1)
	close(l.sessionQueue)
	return l.conn.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func (l *Listener) Accept() (net.Conn, error) {
	return l.AcceptTTPSession()
}

func (l *Listener) closeSession(remoteAddr string) {
	l.sessionMap.Delete(remoteAddr)
}

func (l *Listener) AcceptTTPSession() (*TTPSession, error) {
	select {
	case c := <-l.sessionQueue:
		return c, nil
	}
}

func (l *Listener) handle(remoteAddr *net.UDPAddr, buf []byte) {
	var ts *TTPSession
	if sess, exist := l.sessionMap.Load(remoteAddr.String()); exist {
		ts = sess.(*TTPSession)
	} else {
		ts = NewTTPSession(l, remoteAddr)
		l.sessionMap.Store(remoteAddr.String(), ts)
		l.sessionQueue <- ts
	}
	ts.Read(buf)
}

func (l *Listener) recv() {
	buf := make([]byte, ttpUnit)
	for {
		if atomic.LoadUint32(&l.over) == 0 {
			n, udpRemoteAddr, err := l.conn.ReadFromUDP(buf)
			if err != nil {
				if _, ok := err.(*net.OpError); ok {
					return
				}
				panic(err)
			}
			l.handle(udpRemoteAddr, buf[:n])
		} else {
			return
		}
	}
}

func NewListener(conn *net.UDPConn) *Listener {
	l := &Listener{
		conn:         conn,
		sessionMap:   new(sync.Map),
		sessionQueue: make(chan *TTPSession, 64),
	}
	go l.recv()
	return l
}

func Listen(bindAddr string) (*Listener, error) {
	udpAddr, resolveErr := net.ResolveUDPAddr("udp4", bindAddr)
	if resolveErr != nil {
		return nil, resolveErr
	}
	//监听端口
	udpConn, listenErr := net.ListenUDP("udp", udpAddr)
	if listenErr != nil {
		return nil, listenErr
	}
	return NewListener(udpConn), nil
}
