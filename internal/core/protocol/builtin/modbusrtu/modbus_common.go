// Package modbusrtu 实现 Modbus RTU 串口协议。
//
// 设计要点：
//   - 站号在连接参数(slave_id)中指定，点位只配 address+length+parseType
//   - 地址用 Modbus 标准前缀式(00001/10001/30001/40001)自动识别功能码
//   - 32/64 位类型通过 ABCD/CDAB/BADC/DCBA 后缀区分字节序
//   - 同功能码且地址相邻/重叠的点位合并为一次 Modbus 通信（planReadBatch）
package modbusrtu

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	modbus "github.com/thinkgos/gomodbus/v2"

	"edge5/internal/core/protocol"
)

// modbusAddress 解析后的 Modbus 地址
type modbusAddress struct {
	code  byte   // 1=线圈(FC1/5/15), 2=离散输入(FC2), 3=输入寄存器(FC4), 4=保持寄存器(FC3/6/16)
	addr  uint16 // 0-based 寄存器/线圈地址
	isBit bool   // 线圈/离散输入为 true
}

// parseAddress 按前缀识别功能码。5 位以上标准地址(00001/10001/30001/40001)
// 按首字符识别功能码，地址 1-based 转 0-based；纯数字（不足5位）默认保持寄存器。
func parseAddress(address string) (modbusAddress, error) {
	s := strings.TrimSpace(address)
	if s == "" {
		return modbusAddress{}, fmt.Errorf("地址为空")
	}

	// 5 位以上且首字符为功能码标识
	if len(s) >= 5 {
		first := s[0]
		rest := s[1:]
		if strings.IndexByte("0134", first) >= 0 {
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 {
				return modbusAddress{}, fmt.Errorf("地址无效: %s", address)
			}
			addr := uint16(n - 1)
			switch first {
			case '0':
				return modbusAddress{code: 1, addr: addr, isBit: true}, nil
			case '1':
				return modbusAddress{code: 2, addr: addr, isBit: true}, nil
			case '3':
				return modbusAddress{code: 3, addr: addr, isBit: false}, nil
			case '4':
				return modbusAddress{code: 4, addr: addr, isBit: false}, nil
			}
		}
	}

	// 纯数字 → 默认保持寄存器
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return modbusAddress{}, fmt.Errorf("地址无效: %s", address)
	}
	return modbusAddress{code: 4, addr: uint16(n - 1), isBit: false}, nil
}

// dataTypeInfo 数据类型解析结果
type dataTypeInfo struct {
	base     string // bool/byte/int16/uint16/int32/uint32/int64/uint64/float32/float64/string
	order    string // abcd/cdab/badc/dcba（仅多寄存器类型生效）
	regCount uint16 // 占用寄存器数（string 动态计算）
	isString bool
}

// parseDataType 从数据类型名解析基础类型与字节序，默认字序 cdab（Modbus 最常见）
func parseDataType(dt string) (dataTypeInfo, error) {
	lower := strings.ToLower(strings.TrimSpace(dt))
	info := dataTypeInfo{order: "cdab"}
	base := lower
	for _, o := range []string{"abcd", "cdab", "badc", "dcba"} {
		if strings.HasSuffix(lower, "_"+o) {
			info.order = o
			base = strings.TrimSuffix(lower, "_"+o)
			break
		}
	}
	info.base = base
	switch base {
	case "bool":
		info.regCount = 1
		return info, nil
	case "byte":
		info.regCount = 1
		return info, nil
	case "int16", "short":
		info.base = "int16"
		info.regCount = 1
		return info, nil
	case "uint16", "ushort":
		info.base = "uint16"
		info.regCount = 1
		return info, nil
	case "int32", "int":
		info.base = "int32"
		info.regCount = 2
		return info, nil
	case "uint32", "uint":
		info.base = "uint32"
		info.regCount = 2
		return info, nil
	case "float32", "float":
		info.base = "float32"
		info.regCount = 2
		return info, nil
	case "int64", "long":
		info.base = "int64"
		info.regCount = 4
		return info, nil
	case "uint64", "ulong":
		info.base = "uint64"
		info.regCount = 4
		return info, nil
	case "float64", "double":
		info.base = "float64"
		info.regCount = 4
		return info, nil
	case "string":
		info.isString = true
		return info, nil
	}
	return info, fmt.Errorf("不支持的数据类型: %s", dt)
}

// pointPlan 单个点位的采集计划
type pointPlan struct {
	pt       protocol.Point
	addr     modbusAddress
	info     dataTypeInfo
	regCount uint16 // 实际占用寄存器/线圈数（string 按 length 计算）
	index    int    // 原始点位在 req.Points 中的下标
}

// readSegment 合并后的读取段
type readSegment struct {
	code   byte
	start  uint16 // 0-based 起始地址
	count  uint16 // 段总寄存器/线圈数
	isBit  bool
	points []pointPlan
}

// 功能码单次读取上限
func maxQuantity(code byte) uint16 {
	if code == 1 || code == 2 {
		return 2000 // 线圈/离散输入
	}
	return 125 // 输入/保持寄存器
}

// planReadBatch 分组合并算法：解析点位→按功能码分组→组内排序→合并相邻/重叠地址为段
func planReadBatch(points []protocol.Point) ([]readSegment, error) {
	// 1. 解析每个点位
	plans := make([]pointPlan, 0, len(points))
	for i, pt := range points {
		addr, err := parseAddress(pt.Resource)
		if err != nil {
			return nil, fmt.Errorf("点位 %s: %w", pt.Name, err)
		}
		info, err := parseDataType(pt.DataType)
		if err != nil {
			return nil, fmt.Errorf("点位 %s: %w", pt.Name, err)
		}
		rc := info.regCount
		if info.isString {
			length := pt.Count
			if length <= 0 {
				length = 1
			}
			rc = uint16((length + 1) / 2)
			if rc < 1 {
				rc = 1
			}
		}
		plans = append(plans, pointPlan{pt: pt, addr: addr, info: info, regCount: rc, index: i})
	}

	// 2. 按功能码分组
	groups := make(map[byte][]pointPlan)
	for _, p := range plans {
		groups[p.addr.code] = append(groups[p.addr.code], p)
	}

	// 3+4. 组内排序并合并
	var segments []readSegment
	for code, g := range groups {
		sort.Slice(g, func(i, j int) bool { return g[i].addr.addr < g[j].addr.addr })

		maxQ := maxQuantity(code)
		var cur *readSegment
		for _, p := range g {
			pEnd := p.addr.addr + p.regCount // 不含
			if cur == nil {
				// 开新段
				seg := readSegment{code: code, start: p.addr.addr, isBit: p.addr.isBit, points: []pointPlan{p}}
				seg.count = pEnd - seg.start
				if seg.count > maxQ {
					seg.count = maxQ // 单点已超上限，截断（极端情况）
				}
				segments = append(segments, seg)
				cur = &segments[len(segments)-1]
				continue
			}
			curEnd := cur.start + cur.count
			// 相邻或重叠且不超上限则并入
			if p.addr.addr <= curEnd && pEnd-cur.start <= maxQ {
				cur.points = append(cur.points, p)
				if pEnd > curEnd {
					cur.count = pEnd - cur.start
				}
			} else {
				// 开新段
				seg := readSegment{code: code, start: p.addr.addr, isBit: p.addr.isBit, points: []pointPlan{p}}
				seg.count = pEnd - seg.start
				if seg.count > maxQ {
					seg.count = maxQ
				}
				segments = append(segments, seg)
				cur = &segments[len(segments)-1]
			}
		}
	}
	return segments, nil
}

// readSegmentData 按 code 调用一次 Modbus 读取，返回线路字节流
// 位段：[]byte 位打包（每字节8位）；寄存器段：[]uint16 → 转 [hi,lo,hi,lo...] 字节流
func readSegmentData(client modbus.Client, slaveID byte, seg readSegment) ([]byte, error) {
	switch seg.code {
	case 1:
		return client.ReadCoils(slaveID, seg.start, seg.count)
	case 2:
		return client.ReadDiscreteInputs(slaveID, seg.start, seg.count)
	case 3:
		regs, err := client.ReadInputRegisters(slaveID, seg.start, seg.count)
		if err != nil {
			return nil, err
		}
		return regsToBytes(regs), nil
	case 4:
		regs, err := client.ReadHoldingRegisters(slaveID, seg.start, seg.count)
		if err != nil {
			return nil, err
		}
		return regsToBytes(regs), nil
	}
	return nil, fmt.Errorf("未知功能码: %d", seg.code)
}

// regsToBytes 把 []uint16 还原为 Modbus 线路原始字节流 [hi0,lo0,hi1,lo1...]
func regsToBytes(results []uint16) []byte {
	b := make([]byte, len(results)*2)
	for i, r := range results {
		b[i*2] = byte(r >> 8)
		b[i*2+1] = byte(r & 0xFF)
	}
	return b
}

// reorderBytes 按字节序重排字节流（输入为线路顺序 [W0hi,W0lo,W1hi,W1lo,...]）
//
//	abcd: 原序（大端）
//	dcba: 完全反转（小端）
//	badc: 每字内字节交换
//	cdab: 字(2字节一组)顺序反转
func reorderBytes(b []byte, order string) []byte {
	n := len(b)
	out := make([]byte, n)
	switch order {
	case "abcd":
		copy(out, b)
	case "dcba":
		for i := 0; i < n; i++ {
			out[i] = b[n-1-i]
		}
	case "badc":
		for i := 0; i < n; i += 2 {
			out[i] = b[i+1]
			out[i+1] = b[i]
		}
	case "cdab":
		words := n / 2
		for w := 0; w < words; w++ {
			src := (words - 1 - w) * 2
			out[w*2] = b[src]
			out[w*2+1] = b[src+1]
		}
	default:
		copy(out, b)
	}
	return out
}

// decodePoint 从段结果字节流按偏移解析单个点位值
// segData: 位段为位打包字节，寄存器段为 [hi,lo,hi,lo...] 字节流
func decodePoint(p pointPlan, segData []byte, segStart uint16, isBitSeg bool) (interface{}, error) {
	// 位访问（bool / byte 在线圈地址）
	if isBitSeg || p.addr.isBit {
		bitOffset := int(p.addr.addr) - int(segStart)
		if bitOffset < 0 || bitOffset/8 >= len(segData) {
			return nil, fmt.Errorf("位偏移越界")
		}
		bit := (segData[bitOffset/8] >> uint(bitOffset%8)) & 0x01
		if p.info.base == "byte" {
			return byte(bit), nil
		}
		return bit == 1, nil
	}

	// 寄存器访问
	regOffset := int(p.addr.addr) - int(segStart)
	if regOffset < 0 {
		return nil, fmt.Errorf("寄存器偏移越界")
	}
	start := regOffset * 2
	rc := int(p.regCount)
	if start+rc*2 > len(segData) {
		return nil, fmt.Errorf("寄存器偏移越界")
	}
	regs := segData[start : start+rc*2] // 线路字节

	switch p.info.base {
	case "byte":
		return regs[1], nil // 低字节（与 delta readByte results[0]&0xFF 一致）
	case "int16":
		return int16(binary.BigEndian.Uint16(regs)), nil
	case "uint16":
		return binary.BigEndian.Uint16(regs), nil
	case "int32":
		b := reorderBytes(regs, p.info.order)
		return int32(binary.BigEndian.Uint32(b)), nil
	case "uint32":
		b := reorderBytes(regs, p.info.order)
		return binary.BigEndian.Uint32(b), nil
	case "float32":
		b := reorderBytes(regs, p.info.order)
		return math.Float32frombits(binary.BigEndian.Uint32(b)), nil
	case "int64":
		b := reorderBytes(regs, p.info.order)
		return int64(binary.BigEndian.Uint64(b)), nil
	case "uint64":
		b := reorderBytes(regs, p.info.order)
		return binary.BigEndian.Uint64(b), nil
	case "float64":
		b := reorderBytes(regs, p.info.order)
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	case "string":
		length := p.pt.Count
		if length <= 0 {
			length = 1
		}
		out := make([]byte, len(regs))
		copy(out, regs)
		if len(out) > length {
			out = out[:length]
		}
		return strings.TrimRight(string(out), "\x00"), nil
	}
	return nil, fmt.Errorf("不支持的数据类型: %s", p.info.base)
}

// encodePoint 将写入值编码为寄存器序列（保持寄存器）或位字节（线圈）
func encodePoint(p pointPlan, value interface{}) ([]uint16, []byte, error) {
	// 位类型
	if p.addr.isBit {
		on, err := toBool(value)
		if err != nil {
			return nil, nil, err
		}
		if on {
			return nil, []byte{0x01}, nil
		}
		return nil, []byte{0x00}, nil
	}

	switch p.info.base {
	case "byte":
		v, err := toUint64(value)
		if err != nil {
			return nil, nil, err
		}
		return []uint16{uint16(byte(v))}, nil, nil
	case "int16":
		v, err := toInt64(value)
		if err != nil {
			return nil, nil, err
		}
		return []uint16{uint16(int16(v))}, nil, nil
	case "uint16":
		v, err := toUint64(value)
		if err != nil {
			return nil, nil, err
		}
		return []uint16{uint16(v)}, nil, nil
	case "int32":
		v, err := toInt64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs32(uint32(int32(v)), p.info.order), nil, nil
	case "uint32":
		v, err := toUint64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs32(uint32(v), p.info.order), nil, nil
	case "float32":
		v, err := toFloat64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs32(math.Float32bits(float32(v)), p.info.order), nil, nil
	case "int64":
		v, err := toInt64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs64(uint64(int64(v)), p.info.order), nil, nil
	case "uint64":
		v, err := toUint64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs64(v, p.info.order), nil, nil
	case "float64":
		v, err := toFloat64(value)
		if err != nil {
			return nil, nil, err
		}
		return encodeRegs64(math.Float64bits(v), p.info.order), nil, nil
	case "string":
		s, err := toString(value)
		if err != nil {
			return nil, nil, err
		}
		data := []byte(s)
		if len(data)%2 != 0 {
			data = append(data, 0)
		}
		regs := make([]uint16, len(data)/2)
		for i := 0; i < len(data); i += 2 {
			regs[i/2] = uint16(data[i])<<8 | uint16(data[i+1])
		}
		return regs, nil, nil
	}
	return nil, nil, fmt.Errorf("不支持的数据类型: %s", p.info.base)
}

// encodeRegs32 把 32 位值按字节序编码为 2 个寄存器
func encodeRegs32(bits uint32, order string) []uint16 {
	abcd := make([]byte, 4)
	binary.BigEndian.PutUint32(abcd, bits)
	wire := reorderBytes(abcd, order)
	return []uint16{binary.BigEndian.Uint16(wire[0:2]), binary.BigEndian.Uint16(wire[2:4])}
}

// encodeRegs64 把 64 位值按字节序编码为 4 个寄存器
func encodeRegs64(bits uint64, order string) []uint16 {
	abcd := make([]byte, 8)
	binary.BigEndian.PutUint64(abcd, bits)
	wire := reorderBytes(abcd, order)
	return []uint16{
		binary.BigEndian.Uint16(wire[0:2]),
		binary.BigEndian.Uint16(wire[2:4]),
		binary.BigEndian.Uint16(wire[4:6]),
		binary.BigEndian.Uint16(wire[6:8]),
	}
}

// writePlan 写入段
type writeSegment struct {
	code  byte
	start uint16
	isBit bool
	regs  []uint16 // 寄存器段：按段偏移填好的连续寄存器（无空隙）
	bits  []byte   // 线圈段：位打包
}

// planWriteBatch 写入合并：连续无空隙的保持寄存器合并为 FC16，连续线圈合并为 FC15，否则单点写
// 调用方按段内 len(regs)/len(bits) 判断单点(FC6/FC5)或多点(FC16/FC15)
func planWriteBatch(items []protocol.BatchWriteItem) ([]writeSegment, error) {
	type writeItem struct {
		plan pointPlan
		item protocol.BatchWriteItem
		regs []uint16
		bits []byte
	}
	var regItems []writeItem
	var bitItems []writeItem

	for i, item := range items {
		addr, err := parseAddress(item.Point.Resource)
		if err != nil {
			return nil, fmt.Errorf("点位 %s: %w", item.Point.Name, err)
		}
		info, err := parseDataType(item.Point.DataType)
		if err != nil {
			return nil, fmt.Errorf("点位 %s: %w", item.Point.Name, err)
		}
		rc := info.regCount
		if info.isString {
			length := item.Point.Count
			if length <= 0 {
				length = 1
			}
			rc = uint16((length + 1) / 2)
			if rc < 1 {
				rc = 1
			}
		}
		p := pointPlan{pt: item.Point, addr: addr, info: info, regCount: rc, index: i}
		regs, bits, err := encodePoint(p, item.Value)
		if err != nil {
			return nil, fmt.Errorf("点位 %s: %w", item.Point.Name, err)
		}
		wi := writeItem{plan: p, item: item, regs: regs, bits: bits}
		if addr.isBit {
			bitItems = append(bitItems, wi)
		} else {
			regItems = append(regItems, wi)
		}
	}

	var segments []writeSegment

	// 寄存器：按地址排序，合并连续无空隙段
	sort.Slice(regItems, func(i, j int) bool { return regItems[i].plan.addr.addr < regItems[j].plan.addr.addr })
	i := 0
	for i < len(regItems) {
		start := regItems[i].plan.addr.addr
		end := start + regItems[i].plan.regCount
		buf := make([]uint16, 0, end-start)
		buf = append(buf, regItems[i].regs...)
		j := i + 1
		for j < len(regItems) {
			nextAddr := regItems[j].plan.addr.addr
			if nextAddr != end {
				break // 有空隙，停止合并
			}
			buf = append(buf, regItems[j].regs...)
			end = nextAddr + regItems[j].plan.regCount
			j++
		}
		segments = append(segments, writeSegment{code: 4, start: start, isBit: false, regs: buf})
		i = j
	}

	// 线圈：按地址排序，合并连续无空隙段
	sort.Slice(bitItems, func(i, j int) bool { return bitItems[i].plan.addr.addr < bitItems[j].plan.addr.addr })
	i = 0
	for i < len(bitItems) {
		start := bitItems[i].plan.addr.addr
		end := start + 1
		buf := make([]byte, 0, 1)
		buf = append(buf, bitItems[i].bits...)
		j := i + 1
		for j < len(bitItems) {
			nextAddr := bitItems[j].plan.addr.addr
			if nextAddr != end {
				break
			}
			buf = append(buf, bitItems[j].bits...)
			end = nextAddr + 1
			j++
		}
		segments = append(segments, writeSegment{code: 1, start: start, isBit: true, bits: buf})
		i = j
	}

	return segments, nil
}

// ---------------------------------------------------------------------------
// 值类型转换辅助（参考 delta_common.go）
// ---------------------------------------------------------------------------

func toBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case float64:
		return val != 0, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case string:
		switch strings.ToUpper(strings.TrimSpace(val)) {
		case "1", "TRUE", "ON":
			return true, nil
		case "0", "FALSE", "OFF", "":
			return false, nil
		}
		return false, fmt.Errorf("无效的布尔值: %s", val)
	}
	return false, fmt.Errorf("不支持的布尔值类型: %T", v)
}

func toInt64(v interface{}) (int64, error) {
	switch val := v.(type) {
	case float64:
		return int64(val), nil
	case int:
		return int64(val), nil
	case int64:
		return val, nil
	case int32:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case uint:
		return int64(val), nil
	case uint64:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint16:
		return int64(val), nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的整数: %v", val)
		}
		return n, nil
	}
	return 0, fmt.Errorf("不支持的类型转换为 int64: %T", v)
}

func toUint64(v interface{}) (uint64, error) {
	switch val := v.(type) {
	case float64:
		return uint64(val), nil
	case int:
		return uint64(val), nil
	case int64:
		return uint64(val), nil
	case uint:
		return uint64(val), nil
	case uint64:
		return val, nil
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的无符号整数: %v", val)
		}
		return n, nil
	}
	return 0, fmt.Errorf("不支持的类型转换为 uint64: %T", v)
}

func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0, fmt.Errorf("无效的浮点数: %v", val)
		}
		return f, nil
	}
	return 0, fmt.Errorf("不支持的类型转换为 float64: %T", v)
}

func toString(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	}
	return "", fmt.Errorf("不支持的类型转换为 string: %T", v)
}
