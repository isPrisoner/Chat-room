package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	// 连接到服务端
	conn, err := net.Dial("tcp", "localhost:8082")
	if err != nil {
		fmt.Println("连接服务端失败！其原因是:", err)
		return
	}
	defer func() {
		conn.Close()
	}()

	// 获取用户输入的网名
	reader := bufio.NewReader(os.Stdin)

	// 将网名发送给服务端
	for {
		fmt.Print("请输出您的姓名: ")
		name, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("读取用户姓名失败！其原因是:", err)
			return
		}
		name = strings.TrimSpace(name)

		if name == "" {
			fmt.Println("用户名不能为空!!!")
			continue
		}

		// 将网名发送给服务端
		fmt.Fprintf(conn, "%s\n", name)

		// 检查服务端的回复
		response, err2 := bufio.NewReader(conn).ReadString('\n')
		if err2 != nil {
			fmt.Println("读取服务端消息失败！其原因是:", err2)
			return
		}
		if strings.HasPrefix(response, "ERROR: ") {
			fmt.Println(response) // 打印错误消息
			continue              // 继续循环，重新输入姓名
		}
		break // 正常情况下跳出循环
	}

	fmt.Println("欢迎您加入聊天室。")

	// 创建一个协程接收服务端发送的消息
	go ReceiveMessages(conn)

	// 循环读取用户输入并发送消息
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("读取用户消息失败！其原因是:", err)
			return
		}
		message = strings.TrimSpace(message)
		if message != "" {
			if message == "exit" {
				fmt.Fprintf(conn, "exit\n") // 发送退出信号

				break
			}
			fmt.Fprintf(conn, "%s\n", message)
		}
	}
}

// ReceiveMessages 接收服务端发送的消息
func ReceiveMessages(conn net.Conn) {
	for {
		// 尝试读取服务端消息
		message, err2 := bufio.NewReader(conn).ReadString('\n')
		if err2 != nil {
			if strings.Contains(err2.Error(), "use of closed network connection") {
				fmt.Println("连接已关闭，停止接收消息")
				return
			}
			fmt.Println("读取服务端消息失败！其原因是:", err2)
			return
		}
		message = strings.TrimSpace(message)

		// 打印消息
		fmt.Println(message)
	}
}
