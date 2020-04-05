package ttp

import (
	"log"
	"net"
	"sync"
)

type Listener struct {
	conn     *net.UDPConn
	TTPMap   *sync.Map //map[string]*TTP
	TTPQueue chan *TTP
	over     chan struct{}
}

func (l *Listener) Close() error {
	close(l.over)
	close(l.TTPQueue)
	return l.conn.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func (l *Listener) Accept() (net.Conn, error) {
	return l.AcceptTTP()
}

func (l *Listener) closeTTP(remoteAddr string) {
	l.TTPMap.Delete(remoteAddr)
}

func (l *Listener) AcceptTTP() (*TTP, error) {
	select {
	case ttp := <-l.TTPQueue:
		return ttp, nil
	}
}

func (l *Listener) handle(remoteAddr *net.UDPAddr, buf []byte) {
	var tt *TTP
	if sess, exist := l.TTPMap.Load(remoteAddr.String()); exist {
		tt = sess.(*TTP)
	} else {
		log.Printf("任务注册，访问地址: %s\n", remoteAddr.String())
		over := make(chan struct{})
		tt = NewTTP(l.conn, remoteAddr, over)
		go func() {
			select {
			case <-tt.Done():
				l.closeTTP(remoteAddr.String())
				log.Printf("任务结束，删除ttp: %s\n", remoteAddr.String())
			}
		}()
		l.TTPMap.Store(remoteAddr.String(), tt)
		l.TTPQueue <- tt
	}
	// todo pool
	b := make([]byte, len(buf))
	copy(b, buf)
	tt.Read(b)
}

func (l *Listener) recv() {
	buf := make([]byte, 1024*4)
	for {
		select {
		case <-l.over:
			return
		default:
			n, udpRemoteAddr, err := l.conn.ReadFromUDP(buf)
			if err != nil {
				if _, ok := err.(*net.OpError); ok {
					return
				}
				panic(err)
			}
			l.handle(udpRemoteAddr, buf[:n])
		}
	}
}

func NewListener(conn *net.UDPConn) *Listener {
	l := &Listener{
		conn:     conn,
		TTPMap:   new(sync.Map),
		TTPQueue: make(chan *TTP, 64),
		over:     make(chan struct{}),
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
	udpConn, listenErr := net.ListenUDP("udp4", udpAddr)
	if listenErr != nil {
		return nil, listenErr
	}
	return NewListener(udpConn), nil
}
