package ttp

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SplitFileSize uint32 = 1024 * 1 // 将文件切割成小块，每块的大小  why? 暂测16k无法接受到

	RequestPullFlag  = 1 << iota // 请求文件
	RequestPushFlag              // 请求推送文件
	SupplyFlag                   // 请求补充
	CloseFlag                    // 关闭
	ReplyNumbersFlag             // 回复客户端文件编号
	ReplyConfirmFlag             // 编号包收到确认
	FileDataFlag                 // 文件数据
)

var (
	protobufTypeErr = errors.New("parameter cannot be converted to proto.Message")
	fullError       = errors.New("queue is full")
	savePathError   = errors.New("invalid save path")
)

type TTP struct {
	TTPInterpreter
	readConnMsg  TTPMessage
	writeConnMsg TTPMessage
	conn         *net.UDPConn
	remoteAddr   *net.UDPAddr
	readQueue    chan []byte
	over         chan struct{}
	overMu       *sync.Mutex

	// initial make
	writeReady chan struct{} // 准备写入，握手完毕
	readReady  chan struct{} // 握手完毕的信号，否则触发重试行为
	speedCal   SpeedCalculator

	initialized        uint32        // 0 or 1
	writeBuffer        []byte        // 读取文件的buffer
	writeTimeout       *time.Timer   // 当待发区一定时间没有编号时，认为任务完毕
	readSemaphore      chan struct{} // 每读取一个包，发送一个信号
	nonRecvNumbersLock *sync.Mutex

	// lazy make
	file           *os.File            // 指向需要发送的文件或本地保存的文件
	nonRecvNumbers map[uint32]struct{} // 用以判断 1.数据包是否已经写入过了 2.数据包剩余量
	rto            int64               // time.Duration

	sendNumbersBuffer chan uint32 // 存待发文件编号
	sendSpeed         uint32
	sendSleep         func() // 控制发送速度
	lastSupplyCount   int    // 接收端上一次请求补充的包的数量
}

func NewTTP(conn *net.UDPConn, remoteAddr *net.UDPAddr, over chan struct{}) *TTP {
	t := &TTP{
		TTPInterpreter: new(ProtobufInterpreter),
		readConnMsg:    new(UDPFilePackage),
		writeConnMsg:   new(UDPFilePackage),
		conn:           conn,
		remoteAddr:     remoteAddr,
		readQueue:      make(chan []byte, 1024),
		over:           over,
		overMu:         new(sync.Mutex),

		writeReady:         make(chan struct{}),
		readReady:          make(chan struct{}),
		speedCal:           NewSpeedCalculator(time.Second),
		initialized:        0,
		writeBuffer:        make([]byte, SplitFileSize),
		writeTimeout:       time.NewTimer(time.Second * 10),
		readSemaphore:      make(chan struct{}, 1024),
		nonRecvNumbersLock: new(sync.Mutex),
	}
	go t.recvAndHandle()
	return t
}

func (ttp *TTP) Read(b []byte) (n int, err error) {
	// read to readQueue
	select {
	case ttp.readQueue <- b:
		return len(b), nil
	default:
		return 0, fullError
	}
}

func (ttp *TTP) flushMsgToRemote() (n int, err error) {
	// send the writeConnMsg to the udp buffer
	buf, err := ttp.Marshal(ttp.writeConnMsg)
	if err != nil {
		return 0, err
	}
	return ttp.conn.WriteToUDP(buf, ttp.remoteAddr)
}

func (ttp *TTP) Write(b []byte) (n int, err error) {
	// nothing to do
	return 0, nil
}

func (ttp *TTP) Close() error {
	ttp.overMu.Lock()
	defer ttp.overMu.Unlock()
	select {
	case <-ttp.over:
		return nil
	default:
		close(ttp.over)
		close(ttp.readQueue)
		return ttp.file.Close()
	}
}

func (ttp *TTP) Done() <-chan struct{} {
	return ttp.over
}

func (ttp *TTP) LocalAddr() net.Addr {
	return ttp.conn.LocalAddr()
}

func (ttp *TTP) RemoteAddr() net.Addr {
	return ttp.remoteAddr
}

func (ttp *TTP) SetDeadline(t time.Time) error {
	return nil
}

func (ttp *TTP) SetReadDeadline(t time.Time) error {
	return nil
}

func (ttp *TTP) SetWriteDeadline(t time.Time) error {
	return nil
}

func (ttp *TTP) setNumbersAndSendReplyNumber() {
	// 设置编号并发送文件编号包
	var numbersLength uint32
	if swapped := atomic.CompareAndSwapUint32(&ttp.initialized, 0, 1); swapped {
		// 初始化下载任务信息
		atomic.StoreUint32(&ttp.initialized, 1)
		path := ttp.readConnMsg.GetPath()
		f, openErr := os.Open(path)
		if openErr != nil {
			panic(openErr) // todo 错误捕捉
		} else {
			ttp.file = f
		}
		stat, statErr := ttp.file.Stat() // 初始化请求信息
		if statErr != nil {
			panic(openErr) // todo
		}
		numbersLength = SplitFile(stat.Size())
		log.Printf("发送包数：%d\n", numbersLength)
		{ // lazy init
			ttp.sendNumbersBuffer = make(chan uint32, numbersLength)
		}
		for i := uint32(0); i < numbersLength; i++ {
			ttp.sendNumbersBuffer <- i
		}
	}
	// 给客户端发送编号回复包
	ttp.writeConnMsg.SetAck(ReplyNumbersFlag)
	ttp.writeConnMsg.SetPath(ttp.readConnMsg.GetPath())
	ttp.writeConnMsg.SetNumbers([]uint32{numbersLength})
	ttp.flushMsgToRemote()
}

func (ttp *TTP) writeToFile() error {
	// write readConnMsg file data to local file
	num := ttp.readConnMsg.GetStart() / SplitFileSize
	if _, exist := ttp.nonRecvNumbers[num]; !exist {
		return nil
	}
	_, seekErr := ttp.file.WriteAt(ttp.readConnMsg.GetData(), int64(ttp.readConnMsg.GetStart())) // 写入文件
	if seekErr != nil {
		return seekErr
	} else {
		ttp.nonRecvNumbersLock.Lock()
		delete(ttp.nonRecvNumbers, num)
		ttp.nonRecvNumbersLock.Unlock()
		ttp.readSemaphore <- struct{}{}
		ttp.speedCal.AddFlow(SplitFileSize)
		if len(ttp.nonRecvNumbers) == 0 {
			ttp.Close()
		}
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

		ttp.writeConnMsg.SetAck(FileDataFlag)
		ttp.writeConnMsg.SetStart(uint32(offset))
		ttp.writeConnMsg.SetData(ttp.writeBuffer[:n])
		_, err := ttp.flushMsgToRemote()
		if err != nil {
			panic(err)
		}
		ttp.sendSleep()
		ttp.writeTimeout.Reset(time.Second * 10)
	case <-ttp.writeTimeout.C: // 一定时间后，都没有收到补充请求包，待写区一直为空
		log.Println("待发区为空，timeout")
		ttp.Close()
	}
	return nil
}

func (ttp *TTP) waitToSendFile() {
	for {
		select {
		case <-ttp.over:
			return
		default:
			err := ttp.sendFileSegment()
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func (ttp *TTP) sendReplyConfirm() {
	ttp.writeConnMsg.SetAck(ReplyConfirmFlag)
	_, err := ttp.flushMsgToRemote()
	if err != nil {
		panic(err)
	}
}

func (ttp *TTP) handleMsg() {
	switch ttp.readConnMsg.GetAck() {
	case RequestPullFlag:
		{ // lazy init
			ttp.sendSpeed = ttp.readConnMsg.GetSpeed()
			log.Printf("speed limit:%d\n", ttp.sendSpeed)
			ttp.sendSleep = SleepAfterSendPackage(200, ttp.sendSpeed)
		}
		ttp.setNumbersAndSendReplyNumber()
	case RequestPushFlag:
		go ttp.PrintDownloadProgress()
		{
			ttp.nonRecvNumbers = make(map[uint32]struct{}, ttp.readConnMsg.GetNumbers()[0])
			for i := uint32(0); i < ttp.readConnMsg.GetNumbers()[0]; i++ {
				ttp.nonRecvNumbers[i] = struct{}{}
			}
			atomic.StoreInt64(&ttp.rto, time.Now().UnixNano())
			fileName := ttp.readConnMsg.GetPath()
			if SavePathIsValid(fileName) {
				ttp.file, _ = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0666)
			} else {
				ttp.Close()
				return
			}
		}
		ttp.sendReplyConfirm()
	case SupplyFlag:
		for _, number := range ttp.readConnMsg.GetNumbers() {
			ttp.sendNumbersBuffer <- number
		}
		log.Printf("请求地址%s, 剩余:%d\n", ttp.remoteAddr.String(), len(ttp.sendNumbersBuffer))
	case ReplyConfirmFlag:
		close(ttp.writeReady)
		go ttp.waitToSendFile()
	case CloseFlag:
		ttp.Close()
	case ReplyNumbersFlag:
		log.Printf("the total package of file: %d\n", ttp.readConnMsg.GetNumbers()[0])
		{ // lazy init
			atomic.AddInt64(&ttp.rto, -time.Now().UnixNano()) // 算出接收到编号包到发出请求的时间，是个负数
			ttp.nonRecvNumbers = make(map[uint32]struct{}, ttp.readConnMsg.GetNumbers()[0])
			for i := uint32(0); i < ttp.readConnMsg.GetNumbers()[0]; i++ {
				ttp.nonRecvNumbers[i] = struct{}{}
			}
		}
		ttp.readReady <- struct{}{}
		ttp.sendReplyConfirm()
	case FileDataFlag:
		if atomic.LoadUint32(&ttp.initialized) == 0 {
			if swapped := atomic.CompareAndSwapUint32(&ttp.initialized, 0, 1); swapped {
				atomic.AddInt64(&ttp.rto, -time.Now().UnixNano())
				go ttp.readSemaphoreTimeout()
			}
		}
		err := ttp.writeToFile()
		if err != nil {
			log.Println(err)
		}
	}
}

func (ttp *TTP) recvAndHandle() {
	// read from readQueue and handle it forever
	for data := range ttp.readQueue {
		unmarshalErr := ttp.Unmarshal(data, ttp.readConnMsg)
		if unmarshalErr != nil {
			log.Printf("unmarshal : %s\n", unmarshalErr)
			ttp.Close()
		} else {
			// todo sync.pool ?
			ttp.handleMsg()
		}
	}
}

func (ttp *TTP) readFromConn() {
	buf := make([]byte, 1024*4)
	for {
		select {
		case <-ttp.over:
			return
		default:
			n, readErr := ttp.conn.Read(buf)
			if readErr != nil {
				log.Println(readErr)
			} else {
				b := make([]byte, n)
				copy(b, buf[:n])
				_, err := ttp.Read(b)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}
}

func (ttp *TTP) sendFileRequest(remoteFilePath string, speed uint32) {
	// 发送请求文件信息
	ttp.writeConnMsg.SetAck(RequestPullFlag)
	ttp.writeConnMsg.SetPath(remoteFilePath)
	ttp.writeConnMsg.SetSpeed(speed)
	ttp.flushMsgToRemote()
}

func (ttp *TTP) sendNonRecvNumbers() {
	// 发送未接收成功的包的编号

	// copying data that has not been received
	ttp.nonRecvNumbersLock.Lock()
	nonRecvCount := len(ttp.nonRecvNumbers)
	if nonRecvCount == ttp.lastSupplyCount { // 防止网络不好时，rtt过大，发送冗余的补充包
		ttp.nonRecvNumbersLock.Unlock()
		return
	} else {
		ttp.lastSupplyCount = nonRecvCount
	}
	copyNumbers := make(map[uint32]struct{}, nonRecvCount)
	for n := range ttp.nonRecvNumbers {
		copyNumbers[n] = struct{}{}
	}
	ttp.nonRecvNumbersLock.Unlock()

	// packet transmission
	numbers := [500]uint32{}
	s := numbers[:0]
	for fileNumber := range copyNumbers {
		if len(s) != 500 {
			s = append(s, fileNumber)
		} else {
			ttp.writeConnMsg.SetAck(SupplyFlag)
			ttp.writeConnMsg.SetNumbers(s)
			ttp.flushMsgToRemote()
			time.Sleep(time.Millisecond) // todo constant?
			s = numbers[:0]
		}
	}
	if len(s) != 0 { // 发送最后一个分组
		ttp.writeConnMsg.SetAck(SupplyFlag)
		ttp.writeConnMsg.SetNumbers(s)
		ttp.flushMsgToRemote()
	}
}

func (ttp *TTP) sendOver() {
	// 发送关闭包
	ttp.writeConnMsg.SetAck(CloseFlag)
	ttp.flushMsgToRemote()
}

func (ttp *TTP) readSemaphoreTimeout() {
	// 控制udp超时，在规定时间内未读到服务端的包
	rtt := time.Duration(-atomic.LoadInt64(&ttp.rto) * 2)
	readTimeout := time.NewTimer(rtt)
	defer readTimeout.Stop()
	for {
		select {
		case <-readTimeout.C:
			ttp.sendNonRecvNumbers()
		case <-ttp.readSemaphore:
		case <-ttp.over:
			return
		}
		readTimeout.Reset(rtt)
	}
}

func (ttp *TTP) pullRetryUntilReady(remoteFilePath string, speedKBS uint32) { // 发送请求信息直到收到服务器回应或者超时
	timeout := time.After(time.Second * 10)
	requestRetry := time.NewTicker(time.Second * 2)
	defer requestRetry.Stop()
	for i := 0; i < 5; i++ {
		select {
		case <-ttp.readReady:
			log.Println("Ready to receive data")
			return
		case <-requestRetry.C:
			ttp.sendFileRequest(remoteFilePath, speedKBS)
			atomic.StoreInt64(&ttp.rto, time.Now().UnixNano()) // 注册起始时间
		case <-timeout:
			ttp.Close()
			return
		}
	}
}

func (ttp *TTP) sendPushRequest(name string) {
	ttp.writeConnMsg.SetAck(RequestPushFlag)
	ttp.writeConnMsg.SetNumbers([]uint32{uint32(len(ttp.sendNumbersBuffer))})
	ttp.writeConnMsg.SetPath(name)
	ttp.flushMsgToRemote()
}

func (ttp *TTP) pushRetryUntilReady(fileName string, numbersLength uint32) {
	timeout := time.After(time.Second * 10)
	requestRetry := time.NewTicker(time.Second * 2)
	defer requestRetry.Stop()
	for i := 0; i < 5; i++ {
		select {
		case <-ttp.writeReady:
			log.Println("Ready to push data")
			return
		case <-requestRetry.C:
			ttp.sendPushRequest(fileName)
		case <-timeout:
			ttp.Close()
			return
		}
	}
}

func (ttp *TTP) Pull(remoteFilePath string, saveFilePath string, speedKBS uint16) error {
	// 向服务端请求数据，并接收数据
	if !SavePathIsValid(saveFilePath) {
		return savePathError
	}
	{
		ttp.file, _ = os.OpenFile(saveFilePath, os.O_CREATE|os.O_WRONLY, 0666)
		ttp.nonRecvNumbersLock = new(sync.Mutex)
	}
	go ttp.readFromConn()
	ttp.pullRetryUntilReady(remoteFilePath, uint32(speedKBS))
	ttp.readSemaphoreTimeout()
	return nil
}

func (ttp *TTP) Push(localFilePath string, remoteSavePath string, speedKBS uint32) error {
	fInfo, statErr := os.Stat(localFilePath)
	if statErr != nil {
		return statErr
	}
	numbersLength := SplitFile(fInfo.Size())
	{
		ttp.file, _ = os.Open(localFilePath)
		ttp.sendSpeed = speedKBS
		ttp.sendSleep = SleepAfterSendPackage(200, ttp.sendSpeed)
		ttp.sendNumbersBuffer = make(chan uint32, numbersLength)
		for i := uint32(0); i < numbersLength; i++ {
			ttp.sendNumbersBuffer <- i
		}
	}
	if remoteSavePath == "" {
		remoteSavePath = fInfo.Name()
	}
	go ttp.pushRetryUntilReady(remoteSavePath, numbersLength)
	ttp.readFromConn()
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

func (ttp *TTP) PrintDownloadProgress() {
	delay := time.NewTicker(time.Second)
	defer delay.Stop()
	clear := "\r                                                                          "
	for {
		select {
		case <-delay.C:
			fmt.Print(clear)
			fmt.Printf("\rcurrent speed: %d kb/s\t\twill download: %d kb", ttp.GetSpeed()/1024, ttp.GetProgress())
		case <-ttp.over:
			fmt.Print(clear)
			fmt.Printf("\rfile was downloaded, save path: %s\n", ttp.file.Name())
			return
		}
	}
}
