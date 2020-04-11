package main

import (
	"fmt"
	"github.com/XiaCo/ttp"
	"log"
	"net"
	"time"
)

func init() {
	log.SetFlags(log.Llongfile | log.LstdFlags)
}

func listen(addr string) {
	l, err := ttp.Listen(addr)
	if err != nil {
		panic(err)
	}
	for {
		c, acceptErr := l.AcceptTTP()
		fmt.Println(c.RemoteAddr())
		if acceptErr != nil {
			fmt.Println(acceptErr)
		}
	}
}

func TestPull() {
	go Pull()
	listen("0.0.0.0:56789")
}

func TestPush() {
	go Push()
	listen("0.0.0.0:56789")
}

func main() {
	//TestPull()
	//TestPush()
	listen("0.0.0.0:56789")
}

func Pull() {
	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56789")
	localAddr, _ := net.ResolveUDPAddr("udp4", "0.0.0.0:56321")
	conn, _ := net.ListenUDP("udp", localAddr)
	over := make(chan struct{})
	tt := ttp.NewTTP(conn, remoteAddr, over)
	tt.Pull("/root/Python-3.7.3.tar", "./temp.tar", 5000)
}

func Push() {
	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56789")
	localAddr, _ := net.ResolveUDPAddr("udp4", "0.0.0.0:56321")
	conn, _ := net.ListenUDP("udp", localAddr)
	over := make(chan struct{})
	tt := ttp.NewTTP(conn, remoteAddr, over)
	tt.Push("./temp.tar", "", 5000)
}
