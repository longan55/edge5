package mcprotocol

import (
	"fmt"

	"edge5/internal/core/protocol"
	"edge5/internal/core/protocol/builtin"

	"github.com/longan55/go-mcprotocol/mcp"
)

// mcConnParams MC 协议统一的连接参数（TCP/UDP 共用）。
// 两种传输层使用完全相同的 3E 帧报文，仅传输方式不同，
// 因此连接参数解析逻辑可共用。
type mcConnParams struct {
	ip             string
	port           int
	networkNumber  string
	pcNum          string
	unitIONum      string
	unitStationNum string
}

// parseConnParams 从协议参数 Metadata 中解析 MC 连接参数并填充默认值。
// TCP 与 UDP 两个协议共用此解析逻辑，避免在 Connect/IsConnected/ReadBatch/
// WriteBatch 中重复书写参数提取与默认值处理。
func parseConnParams(params protocol.Metadata) (mcConnParams, error) {
	p := mcConnParams{
		ip:             builtin.GetString(params, "ip"),
		port:           builtin.GetInt(params, "port"),
		networkNumber:  builtin.GetString(params, "networkNumber"),
		pcNum:          builtin.GetString(params, "pcNum"),
		unitIONum:      builtin.GetString(params, "unitIONum"),
		unitStationNum: builtin.GetString(params, "unitStationNum"),
	}

	if p.ip == "" {
		return p, fmt.Errorf("缺少 IP 地址参数")
	}
	if p.port <= 0 {
		p.port = 6000
	}
	if p.networkNumber == "" {
		p.networkNumber = "00"
	}
	if p.pcNum == "" {
		p.pcNum = "FF"
	}
	if p.unitIONum == "" {
		p.unitIONum = "FF03"
	}
	if p.unitStationNum == "" {
		p.unitStationNum = "00"
	}
	return p, nil
}

// probeRead 通过发送读请求验证设备可达性与连接参数正确性。
//
// 读取 D 寄存器 1 个点，校验响应结束码为 0000（正常完成）。
// TCP 与 UDP 两个协议共用此探测逻辑：只要 client 实现 mcp.Client 接口即可，
// 与传输层无关。
//
// 注意：折返测试（自回路测试，命令 1906）在 Q 系列/UDP 模式下会被设备拒绝
// （返回错误码 C059），不能用于连接验证，故改用实际的读请求探测。
// 读命令（0104）是 PLC 必须支持的核心功能，校验结束码 0000 可同时确认
// 设备可达、3E 帧协议正确、网络号/站号等连接参数无误。
func probeRead(client mcp.Client) error {
	// 长连接版 client（TCP 3E Alive）的 Read/Write 复用 c.conn，
	// 需要先 Connect 建立连接才能使用；UDP 版的 Connect/Close 为空操作，
	// 故此处统一调用对 UDP 无影响。
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.Read("D", 0, 1)
	if err != nil {
		return err
	}

	// 3E 帧响应头至少 11 字节：子头(2)+网络号(1)+PC号(1)+IO号(2)+站号(1)+数据长度(2)+结束码(2)
	if len(resp) < 11 {
		return fmt.Errorf("响应长度不足: %d 字节 [%X]", len(resp), resp)
	}

	endCode := fmt.Sprintf("%X", resp[9:11])
	if endCode != "0000" {
		return fmt.Errorf("设备返回错误码: %s", endCode)
	}
	return nil
}
