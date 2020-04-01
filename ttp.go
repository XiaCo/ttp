package ttp

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SplitFileSize uint32 = 1024 * 1 // 将文件切割成小块，每块的大小  todo 暂测16k无法接受到

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

type TTPMessage interface {
	GetAck() uint32
	SetAck(uint32)
	GetPath() string
	SetPath(string)
	GetNumbers() []uint32
	SetNumbers([]uint32)
	GetStart() uint32
	SetStart(uint32)
	GetSpeed() uint32
	SetSpeed(uint32)
	GetData() []byte
	SetData([]byte)

	Reset()
}

type TTPInterpreter interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(buf []byte, v interface{}) error
}

type ProtobufInterpreter int

func (p ProtobufInterpreter) Marshal(v interface{}) ([]byte, error) {
	if msg, ok := v.(proto.Message); ok {
		return proto.Marshal(msg)
	} else {
		return nil, protobufTypeErr
	}
}

func (p ProtobufInterpreter) Unmarshal(buf []byte, v interface{}) error {
	if msg, ok := v.(proto.Message); ok {
		return proto.Unmarshal(buf, msg)
	} else {
		return protobufTypeErr
	}
}

type TTP struct {
	TTPInterpreter
	readConnMsg  TTPMessage
	writeConnMsg TTPMessage
	speedCal     SpeedCalculator
	conn         *net.UDPConn
	remoteAddr   *net.UDPAddr

	writeBuffer []byte // 读取文件的buffer
	readQueue   chan []byte

	file               *os.File // 指向需要发送的文件或本地保存的文件
	sendNumbersBuffer  chan uint32
	nonRecvNumbers     map[uint32]struct{} // 用以判断 1.数据包是否已经写入过了 2.数据包剩余量
	nonRecvNumbersLock *sync.Mutex
	sendSpeed          uint32
	sendSleep          func() // 控制发送速度
	rto                int64  // time.Duration
	//readTimeout        *time.Timer
	writeTimeout  *time.Timer
	readSemaphore chan struct{} // 每读取一个包，发送一个信号

	readReady chan struct{} // 握手完毕的信号
	//writeReady  chan struct{} // 同上
	initialized uint32 // 0 or 1
	over        chan struct{}
	overMu      *sync.Mutex
}

func NewTTP(conn *net.UDPConn, remoteAddr *net.UDPAddr, c chan struct{}) *TTP {
	t := &TTP{
		TTPInterpreter: new(ProtobufInterpreter),
		readConnMsg:    new(UDPFilePackage),
		writeConnMsg:   new(UDPFilePackage),
		speedCal:       NewSpeedCalculator(time.Second),
		conn:           conn,
		remoteAddr:     remoteAddr,

		readQueue:          make(chan []byte, 1024),
		nonRecvNumbersLock: new(sync.Mutex),
		readReady:          make(chan struct{}),
		//writeReady:         make(chan struct{}),
		//readTimeout:        nil,
		writeTimeout:  time.NewTimer(time.Second * 10),
		readSemaphore: make(chan struct{}, 1024),
		over:          c,
		overMu:        new(sync.Mutex),
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
	buf, err := ttp.Marshal(ttp.writeConnMsg)
	if err != nil {
		return 0, err
	}
	return ttp.conn.WriteToUDP(buf, ttp.remoteAddr)
}

func (ttp *TTP) Write(b []byte) (n int, err error) {
	// do nothing
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
		close(ttp.sendNumbersBuffer)
		close(ttp.readReady)
		//close(ttp.writeReady)
		close(ttp.readSemaphore)
		ttp.speedCal.Close()
		fileCloseErr := ttp.file.Close()
		if fileCloseErr != nil {
			return fileCloseErr
		}
		return nil
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
	if atomic.LoadUint32(&ttp.initialized) != 1 {
		goto sendNumbers // 已经初始化过的话，直接发送编号包
	}
	{ // 初始化下载任务信息
		path := ttp.readConnMsg.GetPath()
		f, openErr := os.Open(path)
		if openErr != nil {
			log.Println(openErr)
		} else {
			ttp.file = f
		}
		stat, statErr := ttp.file.Stat() // 初始化请求信息
		if statErr != nil {
			log.Println(statErr)
		}

		numbersLength := SplitFile(stat.Size())
		log.Printf("包数：%d\n", numbersLength)
		ttp.sendNumbersBuffer = make(chan uint32, numbersLength)
		for i := uint32(0); i < numbersLength; i++ {
			ttp.sendNumbersBuffer <- i
		}
		atomic.StoreUint32(&ttp.initialized, 1)
	}
sendNumbers:
	{ // 给客户端发送编号回复包
		ttp.writeConnMsg.SetAck(ReplyNumbersFlag)
		ttp.writeConnMsg.SetPath(ttp.readConnMsg.GetPath())
		ttp.writeConnMsg.SetNumbers([]uint32{numbersLength})
		_, err := ttp.flushMsgToRemote()
		if err != nil {
			log.Println(err)
		}
	}
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
		ttp.flushMsgToRemote()
		ttp.sendSleep()
	case <-ttp.writeTimeout.C: // 一定时间后，都没有收到补充请求包，待写区一直为空
		ttp.Close()
	}
	return nil
}

func (ttp *TTP) waitToSendFile() {
	// todo <-ttp.writeReady 需要?
	for {
		select {
		case <-ttp.over:
			return
		default:
			err := ttp.sendFileSegment()
			if err != nil {
				fmt.Println(err)
			}
			ttp.writeTimeout.Reset(time.Second * 10)
		}
	}
}

func (ttp *TTP) sendReplyConfirm() {
	ttp.writeConnMsg.SetAck(ReplyConfirmFlag)
	ttp.flushMsgToRemote()
}

func (ttp *TTP) handleMsg() {
	switch ttp.readConnMsg.GetAck() {
	case RequestPullFlag:
		ttp.sendSpeed = ttp.readConnMsg.GetSpeed()
		ttp.sendSleep = SleepAfterSendPackage(250, ttp.sendSpeed)
		ttp.setNumbersAndSendReplyNumber()
	case SupplyFlag:
		for _, number := range ttp.readConnMsg.GetNumbers() {
			ttp.sendNumbersBuffer <- number
		}
	case ReplyConfirmFlag:
		ttp.waitToSendFile()
	case CloseFlag:
		ttp.Close()
	case ReplyNumbersFlag:
		log.Printf("the total package of file: %d\n", ttp.readConnMsg.GetNumbers()[0])
		ttp.nonRecvNumbers = make(map[uint32]struct{}, ttp.readConnMsg.GetNumbers()[0])
		atomic.AddInt64(&ttp.rto, -time.Now().UnixNano()) // 算出接收到编号包到发出请求的时间，是个负数
		for i := uint32(0); i < ttp.readConnMsg.GetNumbers()[0]; i++ {
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
		unmarshalErr := ttp.Unmarshal(data, ttp.readConnMsg)
		if unmarshalErr != nil {
			log.Printf("unmarshal : %s\n", unmarshalErr)
			// todo listener.closeTTP(ttp.tid)
			return unmarshalErr
		}
		// todo sync.pool ?
		ttp.handleMsg()
	}
	return nil
}

func (ttp *TTP) sendFileRequest(remoteFilePath string, speed uint32) {
	// 发送请求文件信息
	ttp.writeConnMsg.SetAck(RequestPullFlag)
	ttp.writeConnMsg.SetPath(remoteFilePath)
	ttp.writeConnMsg.SetSpeed(speed)
	ttp.flushMsgToRemote()
	atomic.StoreInt64(&ttp.rto, time.Now().UnixNano()) // 注册起始时间
}

func (ttp *TTP) sendNonRecvNumbers() {
	// 发送未接收成功的包的编号

	// copying data that has not been received
	ttp.nonRecvNumbersLock.Lock()
	copyNumbers := make(map[uint32]struct{}, len(ttp.nonRecvNumbers))
	for n := range ttp.nonRecvNumbers {
		copyNumbers[n] = struct{}{}
	}
	ttp.nonRecvNumbersLock.Unlock()

	// packet transmission
	numbers := [1000]uint32{}
	s := numbers[:0]
	for fileNumber := range copyNumbers {
		if len(s) != 1000 {
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

func (ttp *TTP) pullRetryUntilReady(remoteFilePath string, speedKBS uint32) { // 发送请求信息直到收到服务器回应或者超时
	timeout := time.After(time.Second * 10)
	requestRetry := time.NewTicker(time.Second * 2)
	defer requestRetry.Stop()
retryLoop:
	for i := 0; i < 5; i++ {
		select {
		case <-ttp.readReady:
			log.Println("Ready to receive data")
			break retryLoop
		case <-requestRetry.C:
			ttp.sendFileRequest(remoteFilePath, speedKBS)
		case <-timeout:
			ttp.Close()
		}
	}
}

func (ttp *TTP) Pull(remoteFilePath string, saveFilePath string, speedKBS uint32) error {
	// 向服务端请求数据，并接收数据

	// 初始化文件
	if !SavePathIsValid(saveFilePath) {
		return savePathError
	}
	ttp.file, _ = os.OpenFile(saveFilePath, os.O_CREATE|os.O_WRONLY, 0666)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go ttp.PrintDownloadProgress(&wg)

	ttp.pullRetryUntilReady(remoteFilePath, speedKBS)

	{ // 控制udp超时，在规定时间内未读到服务端的包
		rtt := time.Duration(-atomic.LoadInt64(&ttp.rto) * 2)
		readTimeout := time.NewTimer(rtt)
		for {
			select {
			case <-readTimeout.C:
				ttp.sendNonRecvNumbers()
			case <-ttp.readSemaphore:
			case <-ttp.over:
				wg.Wait()
				return nil
			}
			readTimeout.Reset(rtt)
		}
	}
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
	for {
		select {
		case <-delay.C:
			speed := ttp.GetSpeed()
			willDownload := ttp.GetProgress()
			fmt.Print(clear)
			fmt.Printf("\rcurrent speed: %d kb/s\t\twill download: %d kb", speed/1024, willDownload)
		case <-ttp.over:
			fmt.Print(clear)
			fmt.Printf("\rfile was downloaded, save path: %s\n", ttp.file.Name())
			wg.Done()
			return
		}
	}
}
