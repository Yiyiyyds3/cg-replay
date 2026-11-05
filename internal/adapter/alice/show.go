// show.go —— 从 .ain 文件直接构建单场景事件（CLI play / desktop 复用）。
//
// 兰斯10 的演出提取在 events.go（SceneEventOf）：场景函数 → 立绘/CG 展示 +
// 台词序列。这里只是包一层文件读取与舞台尺寸。
package alice

import (
	"fmt"
	"os"

	"ucgr/internal/ir"
)

// ParseAINFile 读入并解析 .ain 文件（library / CLI 共用）。
func ParseAINFile(ainPath string) (*AIN, error) {
	data, err := os.ReadFile(ainPath)
	if err != nil {
		return nil, err
	}
	ain, err := ParseAIN(data)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", ainPath, err)
	}
	return ain, nil
}

// EncodeGBK 把 UTF-8 字符串编码成 GBK 字节（跨包查 AFA 条目名用）：
// 事件里的资产名是 ain 解码后的 UTF-8，而 AFA 条目名是 GBK 字节，
// 回放池按名查条目前要转回去。
func EncodeGBK(s string) ([]byte, error) { return gbkEncode(s) }

// DecodeGBKString 把 GBK 字节解成 UTF-8（DCF 底图名等原始字节域转换）。
// 解不动时原样返回（与 AIN zstr 的兜底语义一致）。
func DecodeGBKString(b string) string {
	if b == "" {
		return ""
	}
	return decodeGBK([]byte(b))
}

// BuildAINEventStage 读 .ain 文件，按函数名构建场景事件。
// funcName 为空时取第一个场景函数（供「缺省播第一个」）。
// 舞台几何由事件自带（MessageBoxOps 按 1920×1080）或播放器按素材推断，
// 这里不再收舞台参数（收了也用不上，徒增误配面）。
func BuildAINEventStage(ainPath, funcName string) (*ir.Event, error) {
	data, err := os.ReadFile(ainPath)
	if err != nil {
		return nil, err
	}
	ain, err := ParseAIN(data)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", ainPath, err)
	}
	idx := -1
	if funcName != "" {
		idx = ain.FindFunc(funcName)
		if idx < 0 {
			return nil, fmt.Errorf("ain 里没有函数 %q", funcName)
		}
	} else {
		scenes := ain.SceneFuncs()
		if len(scenes) == 0 {
			return nil, fmt.Errorf("ain 里没有场景函数")
		}
		idx = scenes[0]
	}
	return ain.SceneEventOf(idx)
}
