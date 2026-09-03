package hestia

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// snapshotKind 说明这次落盘做了什么。三态而非布尔：「没写」有两种意思——
// 字节相同不必写，与字节不同但不许覆盖——运维要分得开。
type snapshotKind string

const (
	snapshotWritten   snapshotKind = "written"   // 首次落盘
	snapshotUnchanged snapshotKind = "unchanged" // 同 id 同字节，跳过
	snapshotDiverged  snapshotKind = "diverged"  // 同 id 不同字节，另存
)

type snapshotResult struct {
	Path string
	Kind snapshotKind
}

// saveSnapshot 把 ingest 抓到的原文落盘（M1d 的 TASK-002）。
//
// 文件名 <dir>/<articleID>.html，与回填语料 articles/<id>.html 同一命名规则。
//
// 幂等规则（spec §3.2）：
//   - 不存在 ⇒ 写入
//   - 已存在且字节相同 ⇒ 跳过，不改 mtime
//   - 已存在且字节不同 ⇒ **不覆盖**，另存 <articleID>.<UTC 时间戳>.html
//
// 第三种是央行改稿，两版都要留；它不是错误，调用方只需把它说出来。
//
// 写盘经临时文件 + rename：进程在写一半时被杀，不会留下一个「看起来完整」的半篇快照。
// 用 bytes.Equal 而不是 sha256 比对：两者判等语义相同，前者少一处能算错的地方。
func saveSnapshot(dir, articleID string, raw []byte, now time.Time) (snapshotResult, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return snapshotResult{}, fmt.Errorf("snapshot dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, articleID+".html")

	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := writeAtomic(path, raw); err != nil {
			return snapshotResult{}, err
		}
		return snapshotResult{Path: path, Kind: snapshotWritten}, nil
	case err != nil:
		return snapshotResult{}, fmt.Errorf("snapshot read %s: %w", path, err)
	case bytes.Equal(existing, raw):
		return snapshotResult{Path: path, Kind: snapshotUnchanged}, nil
	default:
		alt := filepath.Join(dir, articleID+"."+now.UTC().Format("20060102T150405Z")+".html")
		if err := writeAtomic(alt, raw); err != nil {
			return snapshotResult{}, err
		}
		return snapshotResult{Path: alt, Kind: snapshotDiverged}, nil
	}
}

// writeAtomic 先写 <path>.tmp 再 rename。同目录 rename 在 POSIX 上是原子的。
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("snapshot write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("snapshot rename %s: %w", path, err)
	}
	return nil
}
