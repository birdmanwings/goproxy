// client程序，存放于客户端
// author: bdwms
// time: 2019.11.30
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// 定义远程服务器地址，远程端口，本地端口
var host = flag.String("host", "127.0.0.1", "Please input server's ip")
var remotePort = flag.String("remotePort", "2333", "server port")
var localPort = flag.String("localPort", "8080", "local port")

// 与 browser 相关的数据
// er 用于 server 和 client 之间通信关闭的 channel 信息
// writ 用与 server 或者 client 本身读写数据关闭时的 channel 信息，这里两者都是利用 channel 作为 goroutine 之间的通信
type browser struct {
	conn net.Conn
	er   chan bool
	writ chan bool
	recv chan []byte
	send chan []byte
}

// 与 server 相关的数据
type server struct {
	conn net.Conn
	er   chan bool
	writ chan bool
	recv chan []byte
	send chan []byte
}

// read 函数读取 browser 过来的数据
func (b browser) read() {
	for {
		// 声明 recv 切片用于缓冲数据
		recv := make([]byte, 10240)
		n, err := b.conn.Read(recv)
		if err == io.EOF {
			fmt.Println("Read finished")
		} else if err != nil {
			b.writ <- true
			b.er <- true
			fmt.Println("Read browser fail", err)
			break
		}
		b.recv <- recv[:n]
	}
}

// write 函数发送数据给 browser
func (b browser) write() {
	for {
		send := make([]byte, 10240)
		// 当 writ chan 有数据时说明 browser 停止接受数据
		select {
		case send = <-b.send:
			_, _ = b.conn.Write(send)
		case <-b.writ:
			fmt.Println("Write process close")
			// 这里存疑，break跳不出循环啊,感觉改成return
			break
		}
	}
}

// 读取 server 过来的数据，心跳检测一般是由客户端发起的
func (s *server) read() {
	// 利用 isHeart 是否发送过心跳包以及是否有流量交互两个条件来判断是否需要心跳检查来保活
	isHeart := false
	// 20秒发送一次心跳包
	_ = s.conn.SetReadDeadline(time.Now().Add(time.Second * 20))
	for {
		recv := make([]byte, 10240)
		n, err := s.conn.Read(recv)
		if err == io.EOF {
			fmt.Println("Read finished")
		} else if err != nil {
			// 超时并且没有发送过心跳包
			if strings.Contains(err.Error(), "timeout") && !isHeart {
				fmt.Println("Send the heartbreak packet")
				_, _ = s.conn.Write([]byte("cc"))
				_ = s.conn.SetReadDeadline(time.Now().Add(time.Second * 20))
				isHeart = true
				continue
			}
			// 已经尝试发送过一次心跳包但是仍然超时没有得到回应
			s.recv <- []byte("0")
			s.er <- true
			s.writ <- true
			fmt.Println("No heartbreak packet received,close the tcp connection", err)
			break
		}
		// 成功收到心跳包，刷新超时时间回20秒
		if recv[0] == 's' && recv[1] == 's' {
			fmt.Println("Receive the heartbreak packet")
			_ = s.conn.SetReadDeadline(time.Now().Add(time.Second * 20))
			isHeart = false
			continue
		}
		s.recv <- recv[:n]
	}
}

// 发送数据给 server
func (s server) write() {
	for {
		send := make([]byte, 10240)
		select {
		case send = <-s.send:
			_, _ = s.conn.Write(send)
		case <-s.writ:
			fmt.Println("Write process close")
			break
		}
	}
}

// 显示错误信息并退出
func logExit(err error) {
	if err != nil {
		fmt.Printf("Error,then exit: %v\n", err)
		// 退出当前 goroutine (但是 defer 语句会照常执行)
		runtime.Goexit()
	}
}

// tcp 链接端口
func dialTCP(hostPort string) net.Conn {
	conn, err := net.Dial("tcp", hostPort)
	logExit(err)
	return conn
}

// 两个 socket 衔接相关处理
func handle(server *server, next chan bool) {
	serverRecv := make([]byte, 10240)
	// 阻塞等待 server 传来数据后再链接 browser
	fmt.Println("Wait server message")
	serverRecv = <-server.recv
	// 链接后，下一个 tcp 连上服务器
	next <- true
	// fmt.Println("Start a new tcp connection,message is:", string(serverRecv))
	var browse *browser
	// 链接本地80端口
	serverConn := dialTCP("127.0.0.1:" + *localPort)
	recv := make(chan []byte)
	send := make(chan []byte)
	er := make(chan bool, 1)
	writ := make(chan bool)
	browse = &browser{serverConn, er, writ, recv, send}
	// 开启两个 goroutines 进行 browser 的数据读写
	go browse.read()
	go browse.write()
	// 从服务器中读取的数据
	browse.send <- serverRecv

	for {
		serverRecv := make([]byte, 10240)
		browserRecv := make([]byte, 10240)
		// 利用 select 进行 chan 内选择
		select {
		case serverRecv = <-server.recv:
			if serverRecv[0] != '0' {
				browse.send <- serverRecv
			}
		case browserRecv = <-browse.recv:
			server.send <- browserRecv
		case <-server.er:
			fmt.Println("Server close,then close browse")
			_ = server.conn.Close()
			_ = browse.conn.Close()
			runtime.Goexit()
		case <-browse.er:
			fmt.Println("Browse close,then close server")
			_ = server.conn.Close()
			_ = browse.conn.Close()
			runtime.Goexit()
		}
	}
}

func main() {
	// 设置最大 CPU 核数
	runtime.GOMAXPROCS(runtime.NumCPU())
	// 解析参数，并检查参数是否为3个
	flag.Parse()
	if flag.NFlag() != 3 {
		flag.PrintDefaults()
		os.Exit(1)
	}
	// 将字符串类型转换为 int 型
	local, _ := strconv.Atoi(*localPort)
	remote, _ := strconv.Atoi(*remotePort)
	if !(local >= 0 && local < 65536) {
		fmt.Println("Port Error")
		os.Exit(1)
	}
	if !(remote >= 0 && remote < 65536) {
		fmt.Println("Port Error")
		os.Exit(1)
	}
	// 拼接出地址
	target := net.JoinHostPort(*host, *remotePort)
	for {
		serverConn := dialTCP(target)

		recv := make(chan []byte)
		send := make(chan []byte)
		er := make(chan bool, 1)
		writ := make(chan bool)
		next := make(chan bool)
		server := &server{serverConn, er, writ, recv, send}

		// 开启两个 goroutine 对服务端的数据读写，第三个 goroutine 用来服务端与用户本地浏览器之前的数据交互
		go server.read()
		go server.write()
		go handle(server, next)
		// 利用 next 这个 channel 对链接的进行控制，在断开链接前， handle 函数中的 next 一直被阻塞，主程序一直被阻塞在这里，不会开启新的 TCP 链接
		<-next
	}
}
