package ttp

import (
	"net"
	"sync"
)

type Listener struct {
	conn         *net.UDPConn
	sessionMap   *sync.Map //map[string]*TTPSession
	sessionQueue chan *TTPSession
}

func (l *Listener) Close() error {
	return nil
}

func (l *Listener) Addr() net.Addr {
	return nil
}

func (l *Listener) Accept() (net.Conn, error) {
	return l.acceptTTPSession()
}

func (l *Listener) acceptTTPSession() (*TTPSession, error) {
	go l.recv()
	select {
	case c := <-l.sessionQueue:
		return c, nil
	}
}

func (l *Listener) handle(remoteAddr *net.UDPAddr, buf []byte, uuid []byte) {
	var ttp *TTP
	copyuuid := [14]byte{}
	copy(copyuuid[:], uuid)
	ts := NewTTPSession(l.conn)
	if sess, loaded := l.sessionMap.LoadOrStore(remoteAddr.String(), ts); loaded {
		ttp = sess
	}
	b := bytePool.Get().([]byte)
	copy(b, buf)
	ttp.PutReadQueue(b)
}

func (l *Listener) recv() {
	buf := make([]byte, 1024*32)
	for {
		n, udpRemoteAddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		l.handle(udpRemoteAddr, buf[:n-14], buf[n-14:n])
	}
}

func NewListener(conn *net.UDPConn) *Listener {
	l := &Listener{
		conn:       conn,
		sessionMap: new(sync.Map),
	}
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
