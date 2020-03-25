package ttp

import (
	"fmt"
	"net"
	"testing"
)

func TestInt64ToBytes(t *testing.T) {
	if buf := Int64ToBytes(256); buf != [8]byte{0, 0, 0, 0, 0, 0, 1, 0} {
		t.Fatal(buf)
	}
}

func TestBytesToInt64(t *testing.T) {
	if n := BytesToInt64([8]byte{0, 0, 0, 0, 0, 0, 1, 0}); n != 256 {
		t.Fatal(n)
	}
}

func TestGetuuidByte(t *testing.T) {
	fmt.Println(GetuuidByte(net.UDPAddr{net.IP{222, 222, 222, 222}, 65535, ""}))
}
