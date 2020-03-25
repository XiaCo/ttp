package main

import (
	"github.com/XiaCo/ttp"
	"log"
	"net"
)

func main() {
	l, err := ttp.Listen("0.0.0.0:55555")
	if err != nil {
		panic(err)
	}
	for {
		net.ListenTCP()
		c, acceptErr := l.Accept()
		if acceptErr != nil {
			log.Println(acceptErr)
		}

	}
}
