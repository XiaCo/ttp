package ttp

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SplitFileSize uint32 = 1024 * 1 // 将文件切割成小块，每块的大小  todo 暂测16k无法接受到

	RequestPullFlag  = 1 // 请求文件
	RequestPushFlag  = 2 // 请求推送文件
	SupplyFlag       = 3 // 请求补充
	CloseFlag        = 4 // 关闭
	ReplyNumbersFlag = 5 // 回复客户端文件编号
	ReplyConfirmFlag = 6 // 编号包收到确认
	FileDataFlag     = 7 // 文件数据
)

type TTP struct {
	tid   TTPId
	tSess *TTPSession

	readConnMsg  *UDPFilePackage
	writeConnMsg *UDPFilePackage
	writeBuffer  []byte
	readQueue    chan []byte

	file               *os.File // 指向需要发送的文件或本地保存的文件
	sendNumbersBuffer  chan uint32
	nonRecvNumbers     map[uint32]struct{} // 用以判断 1.数据包是否已经写入过了 2.数据包剩余量
	nonRecvNumbersLock *sync.Mutex

	sendSpeed     uint32
	sendSleep     func() // 控制发送速度
	readReady     chan struct{}
	writeReady    chan struct{}
	rto           time.Duration
	readTimeout   *time.Timer
	writeTimeout  *time.Timer
	readSemaphore chan struct{} // 每读取一个包，发送一个信号
	speedCal      SpeedCalculator

	init uint32 // 0 or 1
	over uint32 // 0 or 1
}

func NewTTP(ts *TTPSession) *TTP {
	t := &TTP{
		tid:                GetuuidByte(),
		tSess:              ts,
		readConnMsg:        new(UDPFilePackage),
		writeConnMsg:       new(UDPFilePackage),
		readQueue:          make(chan []byte, 1024),
		nonRecvNumbersLock: new(sync.Mutex),
		readReady:          make(chan struct{}),
		writeReady:         make(chan struct{}),
		readTimeout:        nil,
		writeTimeout:       time.NewTimer(time.Second * 10),
		readSemaphore:      make(chan struct{}, 1024),

		speedCal: NewSpeedCalculator(time.Second),
	}
	go t.recvAndHandle()
	return t
}

func (ttp *TTP) Read(buf []byte) (int, error) {
	select {
	case ttp.readQueue <- buf:
		return len(buf), nil
	default:
		return 0, errors.New("readQueue is full")
	}
}

func (ttp *TTP) writeMsgToSession() {
	// send writeMsg to TTPSession
	// todo 待优化
	buf, err := proto.Marshal(ttp.writeConnMsg)
	fmt.Printf("msg: %v\n", buf)
	if err != nil {
		fmt.Println(err)
	}
	b := writePool.Get().([]byte)
	b = append(b, ttp.tid[:]...)
	b = append(b, buf...)
	fmt.Printf("b: %v\n", b)
	ttp.tSess.Write(b)
}

func (ttp *TTP) setNumbersAndSendReplyNumber() {
	// 设置编号并发送文件编号包
	var numbersLength uint32
	if atomic.LoadUint32(&ttp.init) != 1 {
		goto sendNumbers // 已经初始化过的话，直接发送编号包
	}
	{ // 初始化下载任务信息
		path := ttp.readConnMsg.GetPath()
		f, openErr := os.Open(path)
		if openErr != nil {
			log.Fatalln(openErr)
		} else {
			ttp.file = f
		}
		stat, statErr := ttp.file.Stat() // 初始化请求信息
		if statErr != nil {
			log.Fatalln(statErr)
		}

		numbersLength := SplitFile(stat.Size())
		fmt.Printf("包数：%d\n", numbersLength)
		ttp.sendNumbersBuffer = make(chan uint32, numbersLength)
		for i := uint32(0); i < numbersLength; i++ {
			ttp.sendNumbersBuffer <- i
		}
	}
sendNumbers:
	{ // 给客户端发送编号回复包
		ttp.writeConnMsg.Ack = ReplyNumbersFlag
		ttp.writeConnMsg.Path = ttp.readConnMsg.Path
		ttp.writeConnMsg.Number = []uint32{numbersLength}
		ttp.writeMsgToSession()
	}
}

func (ttp *TTP) writeToFile() error {
	num := ttp.readConnMsg.Start / SplitFileSize
	if _, exist := ttp.nonRecvNumbers[num]; !exist {
		return nil
	}
	_, seekErr := ttp.file.WriteAt(ttp.readConnMsg.Data, int64(ttp.readConnMsg.Start)) // 写入文件
	if seekErr != nil {
		return seekErr
	} else {
		ttp.nonRecvNumbersLock.Lock()
		delete(ttp.nonRecvNumbers, num)
		ttp.nonRecvNumbersLock.Unlock()
		ttp.readSemaphore <- struct{}{}
		ttp.speedCal.AddFlow(uint32(SplitFileSize / 1024))
		if len(ttp.nonRecvNumbers) == 0 {
			ttp.close()
		}
	}
	return nil
}

func (ttp *TTP) handleMsg() {
	switch ttp.readConnMsg.Ack {
	case RequestPullFlag:
		ttp.sendSpeed = ttp.readConnMsg.Speed
		ttp.sendSleep = SleepAfterSendPackage(250, ttp.sendSpeed)
		ttp.setNumbersAndSendReplyNumber()
	case SupplyFlag:
		for _, number := range ttp.readConnMsg.Number {
			ttp.sendNumbersBuffer <- number
		}
	case ReplyConfirmFlag:
		ttp.waitToSendFile()
	case CloseFlag:
		ttp.close()
	case ReplyNumbersFlag:
		fmt.Printf("the total package of file: %d\n", ttp.readConnMsg.Number[0])
		ttp.nonRecvNumbers = make(map[uint32]struct{}, ttp.readConnMsg.Number[0])
		ttp.rto = time.Duration(time.Now().UnixNano()) - ttp.rto // 算出发出请求到接收到编号包的时间
		for i := uint32(0); i < ttp.readConnMsg.Number[0]; i++ {
			ttp.nonRecvNumbers[i] = struct{}{}
		}
		ttp.readReady <- struct{}{}
		ttp.sendReplyConfirm()
	case FileDataFlag:
		_ = ttp.writeToFile()
	}
}

func (ttp *TTP) recvAndHandle() error {
	// read from readQueue and handle it forever
	for data := range ttp.readQueue {
		unmarshalErr := proto.Unmarshal(data, ttp.readConnMsg)
		fmt.Printf("data: %v\n", data)
		if unmarshalErr != nil {
			fmt.Printf("unmarshal : %s\n", unmarshalErr)
			ttp.tSess.closeTTP(ttp.tid)
			return unmarshalErr
		}
		bytePool.Put(data)
		ttp.handleMsg()
	}
	return nil
}

func (ttp *TTP) sendFileSegment() error {
	// 取一个待发编号，取到文件对应数据，并发送
	select {
	case fileNumber := <-ttp.sendNumbersBuffer:
		offset, seekErr := ttp.file.Seek(int64(SplitFileSize*fileNumber), 0)
		if seekErr != nil {
			return seekErr
		}
		n, readErr := ttp.file.Read(ttp.writeBuffer)
		if readErr != nil {
			return readErr
		}

		ttp.writeConnMsg.Ack = FileDataFlag
		ttp.writeConnMsg.Start = uint32(offset)
		ttp.writeConnMsg.Data = ttp.writeBuffer[:n]
		ttp.writeMsgToSession()
		ttp.sendSleep()
	case <-ttp.writeTimeout.C: // 一定时间后，都没有收到补充请求包，待写区一直为空
		ttp.close()
	}
	return nil
}

func (ttp *TTP) waitToSendFile() {
	<-ttp.writeReady
	for atomic.LoadUint32(&ttp.over) == 0 {
		err := ttp.sendFileSegment()
		if err != nil {
			fmt.Println(err)
		}
		ttp.writeTimeout.Reset(time.Second * 10)
	}
}

func (ttp *TTP) sendFileRequest(remoteFilePath string, speed uint32) {
	// 发送请求文件信息
	ttp.writeConnMsg.Ack = RequestPullFlag
	ttp.writeConnMsg.Path = remoteFilePath
	ttp.writeConnMsg.Speed = speed
	ttp.writeMsgToSession()
	ttp.rto = time.Duration(time.Now().UnixNano()) // 注册起始时间
}

func (ttp *TTP) sendReplyConfirm() {
	ttp.writeConnMsg.Ack = ReplyConfirmFlag
	ttp.writeMsgToSession()
}

func (ttp *TTP) sendNonRecvNumbers() {
	// 发送未接收成功的包的编号

	// copy old numbers
	ttp.nonRecvNumbersLock.Lock()
	copyNumbers := make(map[uint32]struct{}, len(ttp.nonRecvNumbers))
	for n := range ttp.nonRecvNumbers {
		copyNumbers[n] = struct{}{}
	}
	ttp.nonRecvNumbersLock.Unlock()

	numbers := [1000]uint32{}
	s := numbers[:0]
	for fileNumber := range copyNumbers {
		if len(s) != 1000 {
			s = append(s, fileNumber)
		} else { // 分组并发送
			ttp.writeConnMsg.Ack = SupplyFlag
			ttp.writeConnMsg.Number = s
			ttp.writeMsgToSession()
			time.Sleep(time.Millisecond)
			s = numbers[:0]
		}
	}
	if len(s) != 0 { // 发送最后一个分组
		ttp.writeConnMsg.Ack = SupplyFlag
		ttp.writeConnMsg.Number = s
		ttp.writeMsgToSession()
	}
}

func (ttp *TTP) sendOver() {
	// 发送关闭包
	ttp.writeConnMsg.Ack = CloseFlag
	ttp.writeMsgToSession()
}

func (ttp *TTP) close() {
	if atomic.LoadUint32(&ttp.over) == 1 {
		return
	}
	_ = ttp.file.Close()
	close(ttp.readQueue)
	close(ttp.sendNumbersBuffer)
	close(ttp.readReady)
	close(ttp.writeReady)
	close(ttp.readSemaphore)
	ttp.speedCal.Close()
}

func (ttp *TTP) pullReqRetry(remoteFilePath string, speedKBS uint32) { // 发送请求信息直到收到服务器回应或者超时
	timeout := time.After(time.Second * 10)
	requestRetry := time.NewTicker(time.Second * 2)
	defer requestRetry.Stop()
loop:
	for i := 0; i < 5; i++ {
		select {
		case <-ttp.readReady:
			fmt.Println("Ready to receive data")
			break loop
		case <-requestRetry.C:
			ttp.sendFileRequest(remoteFilePath, speedKBS)
		case <-timeout:
			ttp.close()
		}
	}
}

func (ttp *TTP) Pull(remoteFilePath string, saveFilePath string, speedKBS uint32) error {
	// 向服务端请求数据，并接收数据
	ttp.file, _ = os.OpenFile(saveFilePath, os.O_CREATE|os.O_WRONLY, 0666)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go ttp.recvAndHandle()
	ttp.pullReqRetry(remoteFilePath, speedKBS)

	go ttp.PrintDownloadProgress(&wg)

	{ // 控制udp超时，在规定时间内未读到服务端的包
		rtt := ttp.rto * 2
		ttp.readTimeout = time.NewTimer(rtt)
		for atomic.LoadUint32(&ttp.over) == 0 {
			select {
			case <-ttp.readTimeout.C:
				ttp.sendNonRecvNumbers()
			case <-ttp.readSemaphore:
			}
			ttp.readTimeout.Reset(rtt)
		}
	}
	wg.Wait()
	return nil
}

func (ttp *TTP) Push(remoteFilePath string, localFilePath string, speedKBS uint32) error {
	return nil
}

func (ttp *TTP) GetSpeed() uint32 {
	return ttp.speedCal.GetSpeed()
}

func (ttp *TTP) GetProgress() uint32 {
	ttp.nonRecvNumbersLock.Lock()
	n := len(ttp.nonRecvNumbers)
	ttp.nonRecvNumbersLock.Unlock()
	return uint32(n)
}

func (ttp *TTP) PrintDownloadProgress(wg *sync.WaitGroup) {
	delay := time.NewTicker(time.Second)
	defer delay.Stop()
	clear := "\r                                                                       "
	for atomic.LoadUint32(&ttp.over) == 0 {
		select {
		case <-delay.C:
			speed := ttp.GetSpeed()
			willDownload := ttp.GetProgress()
			fmt.Print(clear)
			fmt.Printf("\rcurrent speed: %d kb/s\t\twill download: %d kb", speed, willDownload)
		}
	}
	fmt.Print(clear)
	fmt.Printf("\rfile was downloaded, save path: %s\n", ttp.file.Name())
	wg.Done()
}
