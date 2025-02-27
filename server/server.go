package main

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"strings"
	"sync"
)

// User 用户信息结构
type User struct {
	Name string
	Conn net.Conn
}

type userListStruct struct {
	users     map[string]*User
	userMutex sync.Mutex
}

var userList *userListStruct

// 聊天消息管道
var messageChan = make(chan string)

// 初始化日志记录器
var log = logrus.New()

func newUserListStruct() *userListStruct {
	return &userListStruct{
		users:     make(map[string]*User),
		userMutex: sync.Mutex{},
	}
}

func (uls *userListStruct) userAdd(name string, conn net.Conn) {
	uls.userMutex.Lock()
	defer uls.userMutex.Unlock()
	uls.users[name] = &User{
		Name: name,
		Conn: conn,
	}
}

func (uls *userListStruct) isExistsName(name string) bool {
	uls.userMutex.Lock()
	defer uls.userMutex.Unlock()
	if _, exists := uls.users[name]; exists {
		return exists
	}
	return false
}

func (uls *userListStruct) userDelete(name string) {
	uls.userMutex.Lock()
	defer uls.userMutex.Unlock()
	delete(uls.users, name)
}

func (uls *userListStruct) getUsers() map[string]*User {
	return uls.users
}

func logInit() {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	file, err := os.OpenFile("./server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("打开日志文件失败！其原因是:", err)
		return
	}
	defer file.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, file))
}

func main() {

	userList = newUserListStruct()

	logInit()

	// 监听 8082 端口
	listener, err2 := net.Listen("tcp", ":8082")
	if err2 != nil {
		log.Fatal("监听失败！其原因是:", err2)
		return
	}
	defer listener.Close()
	log.Info("已成功连接8082端口！")

	// 创建一个协程用于接收客户端连接请求
	go handleConnections(listener)

	for message := range messageChan {
		// 将消息广播到所有连接的客户端
		for _, user := range userList.getUsers() {
			fmt.Fprintf(user.Conn, "%s\n", message)
		}
	}

}

// 处理客户端连接请求
func handleConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error("接收连接请求失败！其原因是:", err)
			continue
		}
		go handleClient(conn)
	}
}

// 处理客户端
func handleClient(conn net.Conn) {
	reader := bufio.NewReader(conn)
	var name string

	for {
		nameInput, err := reader.ReadString('\n')
		if err != nil {
			log.Error("读取客户端姓名失败！其原因是:", err)
			conn.Close()
			return
		}
		name = strings.TrimSpace(nameInput)

		if userList.isExistsName(name) {
			fmt.Fprintf(conn, "ERROR: 用户名重复!!!\n") // 修改提示信息前缀
			continue
		} else {
			userList.userAdd(name, conn)
		}
		break
	}

	// 记录用户加入信息
	log.Infof("欢迎%s加入了聊天室。", name)
	messageChan <- fmt.Sprintf("欢迎%s加入了聊天室。", name) // 广播加入信息

	receiveMessages(conn, name)
}

// 接收客户端消息
func receiveMessages(conn net.Conn, name string) {
	defer conn.Close()
	for {
		message, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			log.Warnf("读取%s的信息失败！其原因是: %v", name, err)
			userList.userDelete(name)
			log.Infof("%s离开了聊天室。", name)
			messageChan <- fmt.Sprintf("%s离开了聊天室。", name)
			return
		}
		message = strings.TrimSpace(message)

		if message == "exit" { // 处理退出信号
			log.Infof("%s离开了聊天室。", name)
			messageChan <- fmt.Sprintf("%s离开了聊天室。", name)
			userList.userDelete(name)
			return
		}

		log.Infof("%s: %s", name, message)

		for _, user := range userList.getUsers() {
			if user.Name != name {
				fmt.Fprintf(user.Conn, "%s: %s\n", name, message)
			}
		}
	}
}
