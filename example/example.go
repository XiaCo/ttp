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
	listen("127.0.0.1:55555")
}

func TestPush() {
	go Push()
	listen("127.0.0.1:55555")
}

func main() {
	//TestPull()
	TestPush()
}

func Pull() {
	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:55555")
	localAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56321")
	conn, _ := net.ListenUDP("udp", localAddr)
	over := make(chan struct{})
	tt := ttp.NewTTP(conn, remoteAddr, over)
	tt.Pull("/Users/xia/Downloads/GoLand201913.zip", "./temp.zip", 5000)
}

func Push() {
	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:55555")
	localAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56321")
	conn, _ := net.ListenUDP("udp", localAddr)
	over := make(chan struct{})
	tt := ttp.NewTTP(conn, remoteAddr, over)
	tt.Push("/Users/xia/Downloads/GoLand201913.zip", 5000)
}
