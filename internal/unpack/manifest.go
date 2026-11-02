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
	"strings"
	"time"
)

const manifestName = ".ucgr-source.json"

// manifest 写入产物目录根，记录参与解包的归档指纹。
type manifest struct {
	Version  int           `json:"version"`
	Written  string        `json:"written"`
	Archives []archiveMark `json:"archives"`
	// Decrypt/Key 上次解包用的方案与密钥。只看归档大小/mtime 不够：
	// 换了 XOR 方案后旧产物虽然大小一致，内容却不可复用。
	Decrypt string `json:"decrypt,omitempty"`
	Key     int    `json:"key,omitempty"`
	// Videos 解出来的影片文件名（相对产物根）。有了它，「已含几个影片」
	// 只需 stat 这几个文件，不必扫整个产物目录（1.6 万个文件要 200+ ms）。
	Videos []string `json:"videos,omitempty"`
	// Files/Bytes 产物总文件数与总字节数（含复用的文件，用于预览显示体积）。
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
	if err := json.Unmarshal(data, &m); err != nil || m.Version != 2 {
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
			full := filepath.Join(dest, filepath.FromSlash(name))
			rel, rerr := filepath.Rel(dest, full)
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue // 清单被改坏时不越出产物目录
			}
			if st, err := os.Stat(full); err == nil && !st.IsDir() && st.Size() > 0 {
				n++ // 空文件/截断文件不算「已提取」
			}
		}
		return n
	}
	return CountVideos(dest)
}

// writeManifest 落盘清单（失败不影响解包结果，只让下次退化成「归档变了→重解」）。
func writeManifest(dest string, archives []string, videos []string, files int, bytes int64, opts Options) error {
	m := manifest{
		Version:  2,
		Written:  time.Now().Format(time.RFC3339),
		Archives: markArchives(archives),
		Decrypt:  string(opts.Decrypt),
		Key:      int(opts.Key),
		Videos:   videos,
		Files:    files,
		Bytes:    bytes,
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	// 先写临时文件再改名：进程中途退出不会留下半截 JSON（下次会退化成全量重解）。
	path := filepath.Join(dest, manifestName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
