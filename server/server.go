// 部署在 server 端，即有公网 ip 的服务器上
// author: bdwms
// time: 2019.11.30
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"time"
)

var localPort = flag.String("localPort", "9001", "用户访问服务器端口")
var remotePort = flag.String("remotePort", "2333", "与客户端通讯端口")

// client 相关数据
type client struct {
	conn     net.Conn
	er       chan bool
	isHeart  chan bool
	disHeart bool
	writ     chan bool
	recv     chan []byte
	send     chan []byte
}

// user 相关的数据
type user struct {
	conn net.Conn
	er   chan bool
	writ chan bool
	recv chan []byte
	send chan []byte
}

// 读取从 client 中的数据
func (c *client) read() {
	for {
		// 40秒没有数据传输则断开链接
		_ = c.conn.SetReadDeadline(time.Now().Add(time.Second * 40))
		recv := make([]byte, 10240)
		n, err := c.conn.Read(recv)

		if err != nil {
			c.isHeart <- true
			c.er <- true
			c.writ <- true
			fmt.Println("Message has not been transmitted for a long time,or the client has been closed,accept a new tcp connection")
		}
		// 检测如果收到心跳包 cc 那么返回一个 ss
		if recv[0] == 'c' && recv[1] == 'c' {
			_, _ = c.conn.Write([]byte("ss"))
			continue
		}
		c.recv <- recv[:n]
	}
}

// 把数据发送给 client
func (c client) write() {
	for {
		send := make([]byte, 10240)
		select {
		case send = <-c.send:
			_, _ = c.conn.Write(send)
		case <-c.writ:
			fmt.Println("Write process close")
			break
		}
	}
}

// 读取 user 的数据
func (u user) read() {
	_ = u.conn.SetReadDeadline(time.Now().Add(time.Millisecond * 800))
	for {
		recv := make([]byte, 10240)
		n, err := u.conn.Read(recv)
		_ = u.conn.SetReadDeadline(time.Time{})
		if err != nil {
			u.er <- true
			u.writ <- true
			fmt.Println("Read user message fail", err)
			break
		}
		u.recv <- recv[:n]
	}
}

// 发送数据给 user
func (u user) write() {
	for {
		send := make([]byte, 10240)
		select {
		case send = <-u.send:
			_, _ = u.conn.Write(send)
		case <-u.writ:
			fmt.Println("Write user process close")
			break

		}
	}
}

// 显示错误并退出
func logExit(err error) {
	if err != nil {
		fmt.Printf("Exit %v\n", err)
		runtime.Goexit()
	}
}

// 显示错误
func log(err error) {
	if err != nil {
		fmt.Printf("Exit %v\n", err)
	}
}

// 监听端口
func accept(con net.Listener) net.Conn {
	CorU, err := con.Accept()
	logExit(err)
	return CorU
}

// 在另一个进程监听端口
func goAccept(con net.Listener, Uconn chan net.Conn) {
	CorU, err := con.Accept()
	logExit(err)
	Uconn <- CorU
}

// 处理两个 socket 之间的衔接
func handle(client *client, user *user) {
	for {
		clientRecv := make([]byte, 10240)
		userRecv := make([]byte, 10240)
		select {
		case clientRecv = <-client.recv:
			user.send <- clientRecv
		case userRecv = <-user.recv:
			client.send <- userRecv
		case <-user.er:
			_ = client.conn.Close()
			_ = user.conn.Close()
			runtime.Goexit()
		case <-client.er:
			_ = client.conn.Close()
			_ = user.conn.Close()
			runtime.Goexit()
		}
	}
}

func main() {
	// 设置最大 CPU 核数
	runtime.GOMAXPROCS(runtime.NumCPU())
	// 解析参数，并检查参数是否为2个
	flag.Parse()
	if flag.NFlag() != 2 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 将字符串类型转换为 int 型
	local, _ := strconv.Atoi(*localPort)
	remote, _ := strconv.Atoi(*remotePort)
	if !(local >= 0 && local < 65536) {
		fmt.Println("端口设置错误")
		os.Exit(1)
	}
	if !(remote >= 0 && remote < 65536) {
		fmt.Println("端口设置错误")
		os.Exit(1)
	}

	// 监听端口
	c, err := net.Listen("tcp", ":"+*remotePort)
	log(err)
	u, err := net.Listen("tcp", ":"+*localPort)
	log(err)
}
