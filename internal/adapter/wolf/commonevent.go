// commonevent.go —— Wolf CommonEvent.dat 解析。
//
// 文件布局（UTF-8 变体，塔纳托斯实测 version=0x90）：
//
//	u8 indicator(0x00) + magic 9 字节（"W\x00\x00OL\x55FC\x00"，idx5=0x55
//	为 UTF-8 标记）+ u8 version（0x93/0xCC = v3.5 需 LZ4 解压，本作 0x90
//	直读）+ u32 事件数 + 事件数组 + u8 terminator(≥0x89)
//
// 每个 CommonEvent：
//
//	u8 0x8E + u32 intId + u32 unknown1 + 7B unknown2 + string name
//	+ u32 命令数 + 命令数组 + string unknown11 + string description
//	+ u8 0x8F + 4 组计数数组（string / byte / string串 / int串）
//	+ 0x1D 字节 unknown7 + 定长 100 个 string + u8 0x91 + string
//	+ [u8 0x92 + string + u32 + u8 0x92]（可选尾段）
package wolf

import "fmt"

// CommonEvent 一个公共事件（含名字、描述与指令流）。
type CommonEvent struct {
	ID          int
	Name        string
	Description string
	Commands    []command
}

// commonEvents 全部公共事件 + 版本信息。
type commonEvents struct {
	Version int
	Events  []CommonEvent
}

// magicCommonEvent CommonEvent.dat 的 9 字节魔数（idx5 = UTF-8 标记位）。
var magicCommonEvent = [9]byte{0x57, 0x00, 0x00, 0x4F, 0x4C, 0x00, 0x46, 0x43, 0x00}

// parseCommonEvents 解析 CommonEvent.dat 全量。
func parseCommonEvents(data []byte) (*commonEvents, error) {
	r := &coder{buf: data}
	if r.readByte() != 0x00 {
		return nil, fmt.Errorf("wolf: CommonEvent.dat indicator 非 0x00")
	}
	magic := r.readBytes(9)
	if !r.ok() {
		return nil, r.err
	}
	for i, b := range magic {
		want := magicCommonEvent[i]
		if i == 5 && (b == 0x55 || b == want) {
			continue // idx5 是 UTF-8 标记位，两种取值都接受
		}
		if b != want {
			return nil, fmt.Errorf("wolf: CommonEvent.dat 魔数不符（% X）", magic)
		}
	}
	version := int(r.readByte())
	if version == 0x93 || version == 0xCC {
		return nil, fmt.Errorf("wolf: v3.5 LZ4 容器暂不支持（version %#x）", version)
	}
	out := &commonEvents{Version: version}
	cnt := int(r.readUint())
	if cnt < 0 || cnt > len(data) {
		return nil, fmt.Errorf("wolf: 事件数 %d 可疑（文件 %d 字节）", cnt, len(data))
	}
	for i := 0; i < cnt && r.ok(); i++ {
		ev, err := parseCommonEvent(r, i)
		if err != nil {
			return nil, err
		}
		out.Events = append(out.Events, ev)
	}
	if !r.ok() {
		return nil, r.err
	}
	if term := r.readByte(); term < 0x89 {
		return nil, fmt.Errorf("wolf: CommonEvent 尾标记 %#x < 0x89", term)
	}
	if r.i != len(r.buf) {
		return nil, fmt.Errorf("wolf: CommonEvent.dat 尾部有多余数据（%d/%d）", r.i, len(r.buf))
	}
	return out, nil
}

func parseCommonEvent(r *coder, idx int) (CommonEvent, error) {
	var ev CommonEvent
	if r.readByte() != 0x8E {
		return ev, fmt.Errorf("wolf: 事件 %d 头标记非 0x8E（偏移 %d）", idx, r.i)
	}
	ev.ID = int(r.readUint())
	r.readInt() // unknown1
	r.readBytes(7)
	ev.Name = r.readString()
	ev.Commands = parseCommands(r)
	r.readString() // unknown11
	ev.Description = r.readString()
	if r.readByte() != 0x8F {
		return ev, fmt.Errorf("wolf: 事件 %d 数据标记非 0x8F（CE%d %s）", idx, ev.ID, ev.Name)
	}
	for i, n := 0, int(r.readUint()); i < n && r.ok(); i++ {
		r.readString()
	}
	for i, n := 0, int(r.readUint()); i < n && r.ok(); i++ {
		r.readByte()
	}
	for i, n := 0, int(r.readUint()); i < n && r.ok(); i++ {
		for j, m := 0, int(r.readUint()); j < m && r.ok(); j++ {
			r.readString()
		}
	}
	for i, n := 0, int(r.readUint()); i < n && r.ok(); i++ {
		for j, m := 0, int(r.readUint()); j < m && r.ok(); j++ {
			r.readInt()
		}
	}
	r.readBytes(0x1D)
	for i := 0; i < 100 && r.ok(); i++ { // unknown8 定长 100 字符串
		r.readString()
	}
	if r.readByte() != 0x91 {
		return ev, fmt.Errorf("wolf: 事件 %d 尾标记非 0x91（CE%d）", idx, ev.ID)
	}
	r.readString()
	if ind := r.readByte(); ind == 0x92 {
		r.readString()
		r.readInt()
		if r.readByte() != 0x92 {
			return ev, fmt.Errorf("wolf: 事件 %d 可选尾段标记非 0x92（CE%d）", idx, ev.ID)
		}
	} else if ind != 0x91 {
		return ev, fmt.Errorf("wolf: 事件 %d 尾标记非 0x91/0x92（CE%d）", idx, ev.ID)
	}
	if !r.ok() {
		return ev, r.err
	}
	return ev, nil
}

// find 按编号查事件（-1 = 没有）。
func (c *commonEvents) find(id int) *CommonEvent {
	for i := range c.Events {
		if c.Events[i].ID == id {
			return &c.Events[i]
		}
	}
	return nil
}
