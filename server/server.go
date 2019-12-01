// 部署在 server 端，即有公网 ip 的服务器上
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
	"time"
)

var localPort = flag.String("localPort", "9001", "用户访问服务器端口")
var remotePort = flag.String("remotePort", "2333", "与客户端通讯端口")

// client 相关数据
// disHeart 表示没有收到心跳包
type client struct {
	conn     net.Conn
	er       chan bool
	disHeart chan bool
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
		// 40秒没有数据（心跳包或者数据）传输则断开链接
		_ = c.conn.SetReadDeadline(time.Now().Add(time.Second * 40))
		recv := make([]byte, 10240)
		n, err := c.conn.Read(recv)
		if err == io.EOF {
			fmt.Println("Client Read finished")
		} else if err != nil {
			c.disHeart <- true
			c.er <- true
			c.writ <- true
			fmt.Println("Message has not been transmitted for a long time,or the client has been closed,accept a new tcp connection")
		}
		// 检测如果收到的是心跳包 cc 那么返回一个 ss
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
		if err == io.EOF {
			fmt.Println("User Read finished")
		}
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
func accept(c net.Listener) net.Conn {
	ClientConn, err := c.Accept()
	logExit(err)
	return ClientConn
}

// 在另一个进程监听端口
func goAccept(u net.Listener, UserConn chan net.Conn) {
	TmpConn, err := u.Accept()
	logExit(err)
	UserConn <- TmpConn
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
		fmt.Println("Port error")
		os.Exit(1)
	}
	if !(remote >= 0 && remote < 65536) {
		fmt.Println("Port error")
		os.Exit(1)
	}

	// 监听端口
	c, err := net.Listen("tcp", ":"+*remotePort)
	log(err)
	u, err := net.Listen("tcp", ":"+*localPort)
	log(err)

EXIT:

	// 与 user 建立连接
	UserConn := make(chan net.Conn)
	// 监听 user ，这里因为 UserConn 被阻塞在等待与 Client 建立一条 TCP 连接后才会释放
	go goAccept(u, UserConn)
	fmt.Println("User has prepared")
	clientConn := accept(c)
	fmt.Println("client has prepared", clientConn.LocalAddr().String())

	recv := make(chan []byte)
	send := make(chan []byte)
	disHeart := make(chan bool, 1)
	// 1个位置是为了防止两个读取线程一个退出后另一个永远卡住
	er := make(chan bool, 1)
	writ := make(chan bool)
	client := &client{clientConn, er, disHeart, writ, recv, send}

	go client.read()
	go client.write()

	for {
		select {
		case <-client.disHeart:
			goto EXIT
		case userConn := <-UserConn:
			recv = make(chan []byte)
			send = make(chan []byte)
			// 1个位置是为了防止两个读取线程一个退出后另一个永远卡住，即 user 或者 client 有一个 er 为 true 时
			// 就是断开它断开连接了，这是要关闭这个 TCP 连接， 但是 read goroutine 可能不止一个,每关闭一个 read goroutine 就要
			// 关闭一个 TCP 连接，那么多个 read goroutine 不加锁的情况下可能就会造成 handle goroutine死锁
			er = make(chan bool, 1)
			writ = make(chan bool)
			user := &user{userConn, er, writ, recv, send}
			go user.read()
			go user.write()
			// 当两个 socket 都创立后进入 handle 处理
			go handle(client, user)
			goto EXIT
		}
	}

}
