package hestia

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// queueStates 是文件队列的状态机（方案报告 5.1）。Atlas 建齐四个目录但只写 pending/；
// processing/ done/ failed/ 由消费者（M3 的 warp-hestia）流转。建齐是为了让消费者第一次
// 来就看到完整形状，不必自己建。
var queueStates = []string{"pending", "processing", "done", "failed"}

// queueWorkDirs 是消费者的工作区，**不是生命周期状态**（2026-09-18 追加）。
//
// 🔴 **drafts/ 为什么存在**：消费者原先把成稿 `.note.md` 写在 `processing/`，收尾时
// `mv $Q/processing/* $Q/done/` 的通配符会把它一起带进 `done/`。SKILL.md 明写 `rm -f`、
// 又加了显式警告，**下一轮照样发生**（2026-09-18 两轮实撞）⇒ **注释拦不住，改结构**：
// 成稿写 drafts/，任何对 processing/ 的通配符都碰不到它。
//
// ⚠️ **刻意不进 queueStates**：它不是状态，混进去会出现在 `hestia_queue_items` 里，
// 而「草稿数」不是队列健康度的一部分。QueueHealthOf 也因此不扫它。
var queueWorkDirs = []string{"drafts"}

// EnsureQueueDirs 建齐四个状态目录与消费者工作区，幂等（M2a 的 TASK-004）。
func EnsureQueueDirs(dir string) error {
	for _, s := range slices.Concat(queueStates, queueWorkDirs) {
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
