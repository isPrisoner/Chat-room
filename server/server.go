package main

import (
	"Chat-room/proto"
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// User 用户信息结构
type User struct {
	Name string
	Conn net.Conn
}

// 用户列表结构体
type userListStruct struct {
	users     map[string]*User
	userMutex sync.Mutex
}

var userList *userListStruct

// 聊天消息管道
var messageChan = make(chan string)

// 初始化日志记录器
var log = logrus.New()

// 保护日志文件对象
var logFile *os.File

// 创建一个redis客户端实例
var rdb *redis.Client

// 为redis提供一个传入请求的顶级上下文
var ctx = context.Background()

func newUserListStruct() *userListStruct {
	return &userListStruct{
		users:     make(map[string]*User),
		userMutex: sync.Mutex{},
	}
}

func (uls *userListStruct) getConnByName(name string) (net.Conn, error) {
	if user, exists := uls.users[name]; exists {
		return user.Conn, nil
	}
	return nil, errors.New("未查询到用户名！！！")
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

// 日志配置
func logInit() {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	file, err := os.OpenFile("./server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("打开日志文件失败！其原因是:", err)
		return
	}

	log.SetOutput(io.MultiWriter(os.Stdout, file))
}

// 程序退出时显示关闭日志文件
func closeLogFile() {
	if logFile != nil {
		err := logFile.Close()
		if err != nil {
			log.Error("关闭日志文件失败！其原因是：", err)
		}
	}
}

// redis配置
func redisInit() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "182.42.110.229:6379",
		Password: "xiaomu@303",
		DB:       0,
	})
}

// 更新用户活跃度，每次加1
func updateUserActivity(name string) {
	_, err := rdb.ZIncrBy(ctx, "活跃度排行榜", 1, name).Result()
	if err != nil {
		log.Error("更新用户活跃度失败！其原因是：", err)
	}
}

// 查看排行榜前十名的用户
func getTopUsers() []string {
	users, err := rdb.ZRevRange(ctx, "活跃度排行榜", 0, 9).Result()
	if err != nil {
		log.Error("获取排行榜前十名用户失败！其原因是：", err)
		return nil
	}
	return users
}

// 查看所有用户的排名
func getAllUsers() []string {
	users, err := rdb.ZRevRange(ctx, "活跃度排行榜", 0, -1).Result()
	if err != nil {
		log.Error("获取排行榜所有用户失败！其原因是：", err)
		return nil
	}
	return users
}

// 查看某个用户的排名
func getUser(name string) int64 {
	rank, err := rdb.ZRevRank(ctx, "活跃度排行榜", name).Result()
	if err != nil {
		log.Error("获取用户排名失败！其原因是：", err)
		return -1
	}
	return rank + 1
}

// 删除某个用户的排名
func deleteUser(name string) {
	_, err := rdb.ZRem(ctx, "活跃度排行榜", name).Result()
	if err != nil {
		log.Error("删除用户排名失败！其原因是：", err)
	}
}

// 删除数据库所有数据
func deleteAll() {
	_, err := rdb.ZRemRangeByRank(ctx, "活跃度排行榜", 0, -1).Result()
	if err != nil {
		log.Error("删除所有数据失败！其原因是：", err)
	}
}

func main() {

	redisInit()
	userList = newUserListStruct()
	logInit()

	// 监听 8081 端口
	listener, err2 := net.Listen("tcp", ":8081")
	if err2 != nil {
		log.Fatal("监听失败！其原因是:", err2)
		return
	}
	defer listener.Close()
	deleteAll()
	log.Info("已成功连接8081端口！")

	// 程序结束时，关闭日志文件
	defer closeLogFile()

	// 创建一个协程用于接收客户端连接请求
	go handleConnections(listener)

	// 启动命令交互协程
	go handleServerCommands()

	for message := range messageChan {
		// 将消息广播到所有连接的客户端
		for _, user := range userList.getUsers() {
			date, err := proto.Encode(message)
			if err != nil {
				fmt.Println("编码失败！其原因是：", err)
				return
			}
			user.Conn.Write(date)
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
		nameInput, err := proto.Decode(reader)
		//nameInput, err := reader.ReadString('\n')
		if err != nil {
			log.Error("读取客户端姓名失败！其原因是:", err)
			conn.Close()
			return
		}
		name = strings.TrimSpace(nameInput)

		if userList.isExistsName(name) {
			date, err := proto.Encode("ERROR: 用户名重复!!!\n")
			if err != nil {
				fmt.Println("编码失败！其原因是：", err)
				return
			}
			conn.Write(date)
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
		message, err := proto.Decode(bufio.NewReader(conn))
		if err != nil {
			log.Warnf("读取%s的信息失败！其原因是: %v", name, err)
			return
		}
		message = strings.TrimSpace(message)

		if message == "exit" { // 处理退出信号
			log.Infof("%s离开了聊天室。", name)
			messageChan <- fmt.Sprintf("%s离开了聊天室。", name)
			userList.userDelete(name)
			deleteUser(name)
			return
		}
		if message == "all" { // 处理查看信号
			conn, _ := userList.getConnByName(name)
			var result string
			result = "排行榜所有用户：\n"
			allUsers := getAllUsers()
			for i, user := range allUsers {
				result = result + strconv.Itoa(i+1) + ". " + user + "\n"
			}
			date, err := proto.Encode(result)
			if err != nil {
				fmt.Println("编码失败！其原因是：", err)
				return
			}
			conn.Write(date)
		}

		// 每次接收到消息后，更新活跃度
		updateUserActivity(name)

		log.Infof("%s: %s", name, message)

		message = name + ":" + message
		for _, user := range userList.getUsers() {
			if user.Name != name {
				date, err := proto.Encode(message)
				if err != nil {
					fmt.Println("编码失败！其原因是：", err)
					return
				}
				user.Conn.Write(date)
			}
		}
	}
}

// 服务端控制排行榜
func handleServerCommands() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("欢迎使用Strong服务端，可使用以下命令：")
	fmt.Println("(查看排名前十：top10;查看某人排名:rank <用户名>;" +
		"查看所有人的排行榜:all;删除某人排名:del <用户名>)")
	for {
		fmt.Print("请输入命令: ")
		cmd, err := reader.ReadString('\n')
		if err != nil {
			log.Error("读取命令失败！其原因是:", err)
			return
		}
		cmd = strings.TrimSpace(cmd)

		switch {
		case cmd == "top10":
			topUsers := getTopUsers()
			fmt.Println("排名前十的用户：")
			for i, user := range topUsers {
				fmt.Printf("%d. %s\n", i+1, user)
			}
		case cmd == "all":
			allUsers := getAllUsers()
			fmt.Println("排行榜所有用户：")
			for i, user := range allUsers {
				fmt.Printf("%d. %s\n", i+1, user)
			}
		case strings.HasPrefix(cmd, "rank "):
			name := strings.TrimSpace(strings.TrimPrefix(cmd, "rank "))
			rank := getUser(name)
			if rank == -1 {
				fmt.Printf("未找到用户%s的排名。\n", name)
			} else {
				fmt.Printf("%s的排名是：%d\n", name, rank)
			}
		case strings.HasPrefix(cmd, "del "):
			name := strings.TrimSpace(strings.TrimPrefix(cmd, "del "))
			deleteUser(name)
			fmt.Printf("%s的排名数据已删除!!!\n", name)
		default:
			fmt.Println("无效命令!!!")
		}
	}
}
