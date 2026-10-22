// bgi.go —— BGI / Ethornell（ARC + .bs 脚本）解包。
//
// .bs 是编译过的脚本，适配器已能解析并提取引用（`(*Script).ExtractRefs`）。
// ARC 有两种（PackFile / BURIKO），适配器的 ParseArchive 都能列条目；其中
// BSE 1.00/1.01 图像是**加密**的（适配器会标 Encrypted + KeyHint）——
// 解包层不解密，如实报告并跳过。
//
// 事件切分：按 .bs 脚本头里的**标签**（`Script.Labels`）分段；闭包按整份
// 脚本的引用算（与 Artemis 同口径，宁多勿缺）。
package unpack

import (
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/bgi"
)

func init() {
	a := &bgi.Adapter{}
	registerEngine(engineDeps{
		ID:           "bgi",
		ArchiveExts:  []string{".arc"},
		ParseArchive: a.ParseArchive,
		IsScript: func(p string) bool {
			return strings.EqualFold(filepath.Ext(p), ".bs")
		},
		ParseScript: func(data []byte, name string) (scriptInfo, error) {
			obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
			if err != nil {
				return scriptInfo{}, err
			}
			sc, ok := obj.(*bgi.Script)
			if !ok {
				return scriptInfo{}, errTypeMismatch
			}
			labels := make([]string, 0, len(sc.Labels))
			for _, l := range sc.Labels {
				if l.Name != "" {
					labels = append(labels, l.Name)
				}
			}
			sort.Strings(labels)
			refs := dedupSorted(sc.ExtractRefs())
			return scriptInfo{
				Labels:  labels,
				Refs:    refs,
				AllRefs: refs,
			}, nil
		},
		ScriptExt: ".bs",
		ScriptDir: "scenario",
		ImageDir:  "image",
	})
}
