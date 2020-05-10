package ttp

import (
	"math"
	"os"
	"time"
)

//const (
//	ttpUnit uint32 = 1472
//)
//
//var (
//	bytePool sync.Pool
//)
//
//func init() {
//	bytePool.New = func() interface{} {
//		return make([]byte, ttpUnit)
//	}
//}

func SplitFile(size int64) uint32 {
	// 分割文件成小块编号
	return uint32(math.Ceil(float64(size) / float64(SplitFileSize)))
}

func SavePathIsValid(p string) bool {
	_, statErr := os.Stat(p)
	if statErr != nil {
		return !os.IsExist(statErr)
	}
	return false
}

func SleepAfterSendPackage(n uint32, sendSpeed uint32) func() {
	// n: 多少次作为一批发送的数据
	// sendSpeed: 需要控制的速率，单位 kb/s
	count := uint32(0)
	sleepTime := time.Duration(int64(n) * 1000000000 / int64(sendSpeed))
	return func() {
		count++
		if count == n {
			time.Sleep(sleepTime)
			count = 0
		}
	}
}
