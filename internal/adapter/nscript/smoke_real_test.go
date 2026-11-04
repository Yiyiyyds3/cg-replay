package nscript

import (
	"fmt"
	"os"
	"testing"
	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// 冒烟：用真实归档验证索引解析与解压（跳过条件：语料不存在）。
func TestSmokeRealNsa(t *testing.T) {
	cases := []string{
		`E:/Personal_entertainment/game/zfsgalgame/寒蝉鸣泣之时·解题篇（萌化版）/arc.nsa`,
		`E:/Personal_entertainment/game/zfsgalgame/海猫鸣泣之时 出题篇（萌化版）/arc.nsa`,
	}
	for _, path := range cases {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("语料不存在: %s", path)
		}
		a, err := ParseNsa(path)
		if err != nil {
			t.Fatalf("解析 %s: %v", path, err)
		}
		t.Logf("%s: %d 条目", path, len(a.Entries))

		// 各压缩类型解一个，验证头字节。
		done := map[uint8]bool{}
		for _, k := range a.order {
			e := a.Entries[k]
			if done[e.Comp] || e.Length < 200 {
				continue
			}
			done[e.Comp] = true
			out, err := a.ReadEntry(e)
			if err != nil {
				t.Errorf("读 %s (comp=%d): %v", e.Name, e.Comp, err)
				continue
			}
			t.Logf("comp=%d %s -> %d 字节 head=%x", e.Comp, e.Name, len(out), head(out, 8))
		}
	}
}

func head(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// 冒烟：真实 nscript.dat 解码 + 解析。
func TestSmokeRealScript(t *testing.T) {
	cases := []string{
		`E:/Personal_entertainment/game/zfsgalgame/寒蝉鸣泣之时·解题篇（萌化版）/nscript.dat`,
		`E:/Personal_entertainment/game/zfsgalgame/海猫鸣泣之时 出题篇（萌化版）/nscript.dat`,
	}
	for _, path := range cases {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("语料不存在: %s", path)
		}
		data, err := ReadScriptData(path)
		if err != nil {
			t.Fatalf("读 %s: %v", path, err)
		}
		s, err := ParseScript(string(data), path)
		if err != nil {
			t.Fatalf("解析 %s: %v", path, err)
		}
		cmds := map[string]int{}
		texts := 0
		for _, l := range s.Lines {
			switch l.Kind {
			case LineCmd:
				cmds[l.Cmd]++
			case LineText:
				texts++
			}
		}
		t.Logf("%s: %d 行，台词 %d，stralias %d", path, len(s.Lines), texts, len(s.StrAlias))
		// 打印前 10 个高频命令
		type kv struct {
			k string
			v int
		}
		var kvs []kv
		for k, v := range cmds {
			kvs = append(kvs, kv{k, v})
		}
		for i := 0; i < len(kvs) && i < 10; i++ {
			for j := i + 1; j < len(kvs); j++ {
				if kvs[j].v > kvs[i].v {
					kvs[i], kvs[j] = kvs[j], kvs[i]
				}
			}
		}
		for _, p := range kvs {
			fmt.Printf("  %-10s %d\n", p.k, p.v)
		}

		// 全量构建 IR：数事件、数图层资产。
		a := &Adapter{}
		obj, _ := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: path})
		irDoc, err := a.BuildIR(obj, &adapter.BuildCtx{GameID: path})
		if err != nil {
			t.Errorf("BuildIR %s: %v", path, err)
			continue
		}
		texts, bgs, cg := 0, 0, 0
		for _, ev := range irDoc.Events {
			texts += len(ev.Texts)
			for _, l := range ev.Layers {
				switch l.Kind {
				case ir.LayerBackground:
					bgs++
				case ir.LayerCG:
					cg++
				}
			}
		}
		t.Logf("IR: %d 事件、%d 条台词、bg 层 %d、CG 层 %d", len(irDoc.Events), texts, bgs, cg)
	}
}
