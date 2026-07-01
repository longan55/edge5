package mcprotocol

import (
	"fmt"
	"net"
	"testing"
)

func TestConnect(t *testing.T) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", "192.168.1.202", 6000))
	if err != nil {
		t.Errorf("解析 TCP 地址失败: %v", err)
		return
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		t.Errorf("连接 MC 客户端失败: %v", err)
	}
	defer conn.Close()
}
