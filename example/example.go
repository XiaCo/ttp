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

func main() {
	go Client()
	listen("127.0.0.1:55555")
}

func Client() {
	time.Sleep(time.Second)
	remoteAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:55555")
	localAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:56321")
	conn, _ := net.ListenUDP("udp", localAddr)
	//c, _ := net.DialUDP("udp4", localAddr, remoteAddr)  dialUDP is unconnected
	over := make(chan struct{})
	tt := ttp.NewDrivingTTP(conn, remoteAddr, over)
	tt.Pull("D:\\NLP\\nltk_data.zip", "./temp.zip", 50000)
}
