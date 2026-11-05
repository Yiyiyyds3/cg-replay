// map.go —— Wolf 地图文件（MapData/*.mps）解析。
//
// 布局（WolfTL Map.hpp；塔纳托斯实测 version=0x64，未压缩）：
//
//	20 字节 magic（10×00 + "WOLFM" + 5×00）+ u32 version
//	+ u8 unknown2 + string unknown3 + u32 tileset + u32 width + u32 height
//	+ u32 事件数 +（version ≥ 0x67：u32 unknown4 + u32 layerCnt）
//	+ tiles（UTF-8 模式：i32 -1 表示无；否则 width*height*layer*4）
//	+ 循环（u8 0x6F + 地图事件）+ u8 0x66
//
// 地图事件：magic "39 30 00 00" + u32 id + string name + u32 x + u32 y
// + u32 page 数 + "00 00 00 00" + 循环（u8 0x79 + Page）+ u8 0x70
//
// Page：u32 unknown1 + string graphicName + 4×u8 + 37B conditions
// + 4B movement + 2×u8 + 路线点数组 + u32 命令数 + 命令数组
// + u32 features + 3×u8 +（features > 3：u8）+ u8 0x7A
//
// version ≥ 0x65 走 LZ4 容器（Seek(25) + u32 decSize + u32 encSize），
// ≥ 0x67 命令带 v3.5 附加数据；塔纳托斯全部 0x64，两者都不触发。
package wolf

import "fmt"

const (
	mapEventIndicator = 0x6F
	mapTerminator     = 0x66
	pageIndicator     = 0x79
	pageTerminator    = 0x7A
)

// MapEvent 地图事件（含全部 Page）。
type MapEvent struct {
	ID    int
	Name  string
	Pages []MapPage
}

// MapPage 一个事件页（graphicName + 指令流）。
type MapPage struct {
	GraphicName string
	Commands    []command
}

// parseMap 解析单个 .mps（仅事件部分；version ≥ 0x65 的 LZ4 容器不支持）。
func parseMap(data []byte) ([]MapEvent, error) {
	r := &coder{buf: data}
	if len(data) < 25 {
		return nil, fmt.Errorf("wolf: .mps 过短（%d 字节）", len(data))
	}
	ver := int(r.readUint())
	if ver >= 0x65 {
		return nil, fmt.Errorf("wolf: .mps version %#x 走 LZ4 容器，暂不支持", ver)
	}
	r.readByte()
	r.readString()
	r.readInt() // tileset
	w, h := int(r.readUint()), int(r.readUint())
	ecnt := int(r.readUint())
	if w <= 0 || h <= 0 || w > 1<<16 || h > 1<<16 {
		return nil, fmt.Errorf("wolf: .mps 尺寸非法 %dx%d", w, h)
	}
	// UTF-8 模式的 tiles 标记：i32 -1 = 无 tiles，否则 w*h*3*4 字节
	if v := r.readInt(); v != -1 {
		r.i -= 4
		r.readBytes(w * h * 3 * 4)
	}
	var evs []MapEvent
	ind := r.readByte()
	for ind == mapEventIndicator && r.ok() {
		ev, err := parseMapEvent(r)
		if err != nil {
			return nil, err
		}
		evs = append(evs, ev)
		ind = r.readByte()
	}
	if !r.ok() {
		return nil, r.err
	}
	if ind != mapTerminator {
		return nil, fmt.Errorf("wolf: .mps 事件终止符 %#x 非 0x66", ind)
	}
	if r.i != len(r.buf) {
		return nil, fmt.Errorf("wolf: .mps 尾部多余数据（%d/%d）", r.i, len(r.buf))
	}
	if ecnt != len(evs) {
		return nil, fmt.Errorf("wolf: .mps 事件数不符（声明 %d 实得 %d）", ecnt, len(evs))
	}
	return evs, nil
}

func parseMapEvent(r *coder) (MapEvent, error) {
	var ev MapEvent
	if m := r.readBytes(4); len(m) != 4 || m[0] != 0x39 || m[1] != 0x30 || m[2] != 0x00 || m[3] != 0x00 {
		return ev, fmt.Errorf("wolf: 地图事件魔数不符（% X）", m)
	}
	ev.ID = int(r.readUint())
	ev.Name = r.readString()
	r.readInt() // x
	r.readInt() // y
	pcnt := int(r.readUint())
	if m := r.readBytes(4); len(m) != 4 || m[0] != 0x00 || m[1] != 0x00 || m[2] != 0x00 || m[3] != 0x00 {
		return ev, fmt.Errorf("wolf: 地图事件第二魔数不符（CE%d %s）", ev.ID, ev.Name)
	}
	for i := 0; i < pcnt && r.ok(); i++ {
		p, err := parsePage(r)
		if err != nil {
			return ev, err
		}
		ev.Pages = append(ev.Pages, p)
	}
	if !r.ok() {
		return ev, r.err
	}
	if r.readByte() != 0x70 {
		return ev, fmt.Errorf("wolf: 地图事件终止符非 0x70（%s）", ev.Name)
	}
	return ev, nil
}

func parsePage(r *coder) (MapPage, error) {
	var p MapPage
	r.readInt() // unknown1
	p.GraphicName = r.readString()
	r.readBytes(4) // direction / frame / opacity / renderMode
	r.readBytes(37)
	r.readBytes(4)
	r.readByte() // flags
	r.readByte() // routeFlags
	for i, n := 0, int(r.readUint()); i < n && r.ok(); i++ {
		r.readByte()
		ac := int(r.readByte())
		for j := 0; j < ac; j++ {
			r.readInt()
		}
		if m := r.readBytes(2); len(m) != 2 || m[0] != 0x01 || m[1] != 0x00 {
			return p, fmt.Errorf("wolf: 路线终止符非法（偏移 %d）", r.i)
		}
	}
	p.Commands = parseCommands(r)
	feats := r.readUint()
	r.readBytes(3) // shadow / collisionW / collisionH
	if feats > 3 {
		r.readByte() // pageTransfer
	}
	if !r.ok() {
		return p, r.err
	}
	if r.readByte() != pageTerminator {
		return p, fmt.Errorf("wolf: Page 终止符非 0x7A（偏移 %d）", r.i)
	}
	return p, nil
}
