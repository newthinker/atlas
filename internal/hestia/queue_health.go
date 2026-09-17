package hestia

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QueueHealth 是契约队列的一次快照。年龄为零值表示该目录为空。
type QueueHealth struct {
	PendingCount, ProcessingCount, DoneCount, FailedCount int
	OldestPending, OldestProcessing                       time.Time
}

// QueueHealthOf 扫契约队列的四个子目录，返回件数与最旧文件的修改时间。
//
// **纯文件系统读**：不开库、不接收 *Store。队列是文件系统状态，健康度就该从文件系统读。
//
// 🔴 **目录读不到就报错，不退化成零件数。** `deploy.sh` 的 rsync --delete 删掉整棵队列
// 那次（2026-09-15）暴露的正是这件事：「正常空」与「被删空」在件数上完全同形，
// 于是一期契约消失了三天没有任何信号。退化会把这个失效模式原样保留下来。
//
// 不算队列项的条目：子目录、以 `.` 开头的文件（.DS_Store）、以 `.tmp` 结尾的文件
// （writeAtomic 的中间态）。判据与触发脚本一致，否则两边对「空」的认定会分叉。
func QueueHealthOf(dir string) (QueueHealth, error) {
	var h QueueHealth
	for _, state := range queueStates {
		entries, err := os.ReadDir(filepath.Join(dir, state))
		if err != nil {
			return QueueHealth{}, fmt.Errorf("hestia queue health: %s: %w", state, err)
		}
		var n int
		var oldest time.Time
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".tmp") {
				continue
			}
			n++
			info, err := e.Info()
			if err != nil {
				return QueueHealth{}, fmt.Errorf("hestia queue health: %s/%s: %w", state, e.Name(), err)
			}
			if oldest.IsZero() || info.ModTime().Before(oldest) {
				oldest = info.ModTime()
			}
		}
		switch state {
		case "pending":
			h.PendingCount, h.OldestPending = n, oldest
		case "processing":
			h.ProcessingCount, h.OldestProcessing = n, oldest
		case "done":
			h.DoneCount = n
		case "failed":
			h.FailedCount = n
		default:
			// queueStates 加了第五个状态而这里没跟上：响亮失败，别静默丢掉那个目录的件数。
			return QueueHealth{}, fmt.Errorf("hestia queue health: %s: unknown queue state", state)
		}
	}
	return h, nil
}
