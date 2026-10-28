// manifest.go —— 记录「这次解包依据的是哪几个归档、它们当时的大小与时间」。
//
// 为什么要它：判断「要不要重解」的正确依据是**源归档有没有变**，而不是产物文件。
// 只比产物大小会被「补丁改了内容但大小没变」骗过；给每个产物存哈希又要在重解时
// 把所有文件读一遍（把增量省下的 IO 全花回去）。归档只有几个，stat 一下就知道。
package unpack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const manifestName = ".ucgr-source.json"

// manifest 写入产物目录根，记录参与解包的归档指纹。
type manifest struct {
	Version  int           `json:"version"`
	Written  string        `json:"written"`
	Archives []archiveMark `json:"archives"`
	// Videos 解出来的影片文件名（相对产物根）。有了它，「已含几个影片」
	// 只需 stat 这几个文件，不必扫整个产物目录（1.6 万个文件要 200+ ms）。
	Videos []string `json:"videos,omitempty"`
	// Files/Bytes 这次解出来的文件数与字节数（用于预览显示体积）。
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

// archiveMark 一个归档的指纹（名称 + 大小 + 修改时间）。
type archiveMark struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime_ns"` // 纳秒：同一秒内改写也要能发现
}

// markArchives 现算一组归档的指纹（按文件名排序，保证可比）。
func markArchives(archives []string) []archiveMark {
	out := make([]archiveMark, 0, len(archives))
	for _, a := range archives {
		st, err := os.Stat(a)
		if err != nil {
			continue
		}
		out = append(out, archiveMark{
			Name:  filepath.Base(a),
			Size:  st.Size(),
			Mtime: st.ModTime().UnixNano(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sameMarks 两组指纹是否一致。
func sameMarks(a, b []archiveMark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// readManifest 读产物目录里的清单；没有/损坏都返回 nil（当作「第一次解」）。
func readManifest(dest string) *manifest {
	data, err := os.ReadFile(filepath.Join(dest, manifestName))
	if err != nil {
		return nil
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil || m.Version != 1 {
		return nil
	}
	return &m
}

// ExistingVideos 产物目录里现有多少影片。
//
// 优先用清单里记的文件名（只需 stat 几个文件）；清单缺失（旧版本解的）就退回扫目录。
func ExistingVideos(dest string) int {
	if m := readManifest(dest); m != nil && len(m.Videos) > 0 {
		n := 0
		for _, name := range m.Videos {
			if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err == nil {
				n++
			}
		}
		return n
	}
	return CountVideos(dest)
}

// writeManifest 落盘清单（失败不影响解包结果，只让下次退化成「归档变了→重解」）。
func writeManifest(dest string, archives []string, videos []string, files int, bytes int64) error {
	m := manifest{
		Version:  1,
		Written:  time.Now().Format(time.RFC3339),
		Archives: markArchives(archives),
		Videos:   videos,
		Files:    files,
		Bytes:    bytes,
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, manifestName), data, 0o644)
}
