// show.go —— 从 CommonEvent.dat 直接构建单场景事件（CLI / desktop 复用）。
//
// entryID 是 CommonEvent 的 intId 十进制字符串（library 事件库的 Label）；
// 为空时取第一个含 Message 的剧情 CE。
package wolf

import (
	"fmt"
	"strconv"

	"ucgr/internal/ir"
)

// BuildWolfEventStage 读 CommonEvent.dat，按 CE 编号构建场景事件。
func BuildWolfEventStage(datPath, entryID string) (*ir.Event, error) {
	ce, err := ParseCommonEventFile(datPath)
	if err != nil {
		return nil, err
	}
	if id, e := strconv.Atoi(entryID); e == nil {
		target := ce.find(id)
		if target == nil {
			return nil, fmt.Errorf("wolf: CommonEvent 里没有编号 %d", id)
		}
		ev, err := SceneEventOf(*target)
		if err != nil {
			return nil, fmt.Errorf("CE%d %s: %w", target.ID, target.Name, err)
		}
		return ev, nil
	}
	// 非数字：按名字匹配（首个）
	for i := range ce.Events {
		if ce.Events[i].Name == entryID {
			ev, err := SceneEventOf(ce.Events[i])
			if err != nil {
				return nil, fmt.Errorf("CE%d %s: %w", ce.Events[i].ID, ce.Events[i].Name, err)
			}
			return ev, nil
		}
	}
	return nil, fmt.Errorf("wolf: CommonEvent 里没有 %q", entryID)
}
