package hestia

import (
	"fmt"
	"os"
	"path/filepath"
)

// queueStates 是文件队列的状态机（方案报告 5.1）。Atlas 建齐四个目录但只写 pending/；
// processing/ done/ failed/ 由消费者（M3 的 warp-hestia）流转。建齐是为了让消费者第一次
// 来就看到完整形状，不必自己建。
var queueStates = []string{"pending", "processing", "done", "failed"}

// EnsureQueueDirs 建齐四个子目录，幂等（M2a 的 TASK-004）。
func EnsureQueueDirs(dir string) error {
	for _, s := range queueStates {
		sub := filepath.Join(dir, s)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return fmt.Errorf("contract queue dir %s: %w", sub, err)
		}
	}
	return nil
}

// WriteContract 把契约写进 <dir>/pending/<period>-<period_type>.json，返回路径。
//
// 同名覆盖：修订到达时旧契约还没被消费，最新的赢；已进 done/ 的不动（那是消费者的）。
// 原子写（writeAtomic，与快照同一手法）：消费者读到的是旧的完整文件或新的完整文件，
// 没有半个。
func WriteContract(dir string, c Contract) (string, error) {
	b, err := c.JSON()
	if err != nil {
		return "", fmt.Errorf("contract %s: %w", c.FileName(), err)
	}
	pending := filepath.Join(dir, "pending")
	if err := os.MkdirAll(pending, 0o755); err != nil {
		return "", fmt.Errorf("contract queue dir %s: %w", dir, err)
	}
	path := filepath.Join(pending, c.FileName())
	if err := writeAtomic(path, b); err != nil {
		return "", fmt.Errorf("contract %s: %w", c.FileName(), err)
	}
	return path, nil
}
