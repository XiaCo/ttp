package main

import (
	"fmt"
	"github.com/XiaCo/ttp"
	"net"
	"time"
)

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

func main() {
	go Client("0.0.0.0:55556")
	listen("0.0.0.0:55555")
}

func Client(addr string) {
	l, err := ttp.Listen(addr)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			c, acceptErr := l.AcceptTTP()
			fmt.Println(c.RemoteAddr())
			if acceptErr != nil {
				fmt.Println(acceptErr)
			}
		}
	}()

	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:55555")
	localAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56321")
	c, _ := net.DialUDP("udp4", localAddr, remoteAddr)
	over := make(chan struct{})
	tt := ttp.NewTTP(c, remoteAddr, over)
	pullErr := tt.Pull("/Users/xia/java_error_in_goland.hprof", "./temp", 1000)
	if pullErr != nil {
		panic(err)
	}
}
