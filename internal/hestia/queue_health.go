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

// queueReadDir 是 os.ReadDir 的包内接缝：测试用它注入「ReadDir 看得见、Info() 已消失」
// 的条目（那个竞态在真实文件系统上不可稳定复现）。生产路径恒为 os.ReadDir。
var queueReadDir = os.ReadDir

// ByState 按状态给出件数，是 collector 的**单一口径**（QA W-2）：消费侧不要再各自
// 维护一份状态名单——hestia 侧加了状态而消费侧漏跟时，`go test ./...` 会全绿，
// 而新状态的件数静默消失。
//
// 🔴 **未接线的状态故意不出现在返回值里**，好让「键集合 == queueStates」那条断言变红
// （TestQueueHealthByStateCoversAllStates）。若改成 default 也填 0，加第五个状态时守卫恒绿，
// 这个方法就白设了。
func (h QueueHealth) ByState() map[string]int {
	by := make(map[string]int, len(queueStates))
	for _, state := range queueStates {
		switch state {
		case "pending":
			by[state] = h.PendingCount
		case "processing":
			by[state] = h.ProcessingCount
		case "done":
			by[state] = h.DoneCount
		case "failed":
			by[state] = h.FailedCount
		}
	}
	return by
}

// QueueHealthOf 扫契约队列的四个子目录，返回件数与最旧文件的修改时间。
//
// **纯文件系统读**：不开库、不接收 *Store。队列是文件系统状态，健康度就该从文件系统读。
//
// 🔴 **目录读不到就报错，不退化成零件数。** `deploy.sh` 的 rsync --delete 删掉整棵队列
// 那次（2026-09-15）暴露的正是这件事：「正常空」与「被删空」在件数上完全同形，
// 于是一期契约消失了三天没有任何信号。退化会把这个失效模式原样保留下来。
//
// 不算队列项的条目：**一切非常规文件**（子目录、符号链接、FIFO、socket、设备）、
// 以 `.` 开头的文件（.DS_Store）、以 `.tmp` 结尾的文件（writeAtomic 的中间态）。
// 判据与 scripts/ops/hestia-warp-trigger.sh 的 `find -type f` 一致，否则两边对「空」的
// 认定会分叉。
//
// 🔴 **symlink 要显式排除，靠 IsDir() 挡不住**（QA W-1 实证）：os.ReadDir 的
// DirEntry.Type() 来自 lstat，指向目录的 symlink 其 IsDir() 也是 false ⇒ 不判
// Type().IsRegular() 就会把它计成一件。代价不是多一个数：指标说「有 N 件」而触发器说
// 「queue empty」，24h 后 hestia_queue_stuck 亮起、文案却指向「触发器没跑」，值班人被引偏。
// 守卫：TestQueueHealthIgnoresIrregularFiles（含 symlink 三例 + 同一夹具跑触发脚本比对）。
func QueueHealthOf(dir string) (QueueHealth, error) {
	var h QueueHealth
	for _, state := range queueStates {
		entries, err := queueReadDir(filepath.Join(dir, state))
		if err != nil {
			return QueueHealth{}, fmt.Errorf("hestia queue health: %s: %w", state, err)
		}
		// 只有 pending/processing 要最旧年龄；done/failed 只数个数，省掉每轮两次 lstat。
		needOldest := state == "pending" || state == "processing"
		var n int
		var oldest time.Time
		for _, e := range entries {
			if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".tmp") {
				continue
			}
			if !needOldest {
				n++
				continue
			}
			info, err := e.Info()
			if err != nil {
				// 🔴 条目在 ReadDir 与 Info() 之间消失**不是故障**（QA W-6）：ReadDir 先返回
				// 条目名、Info() 才 lstat，契约从 pending/ rename 到 processing/ 正好落在这两步
				// 之间就会走到这里。把它当致命错误的代价是整轮塌掉——queue_up=0、队列指标整组
				// 缺失，stuck/failed 的 for 计时跟着清零，真告警最多晚 10 分钟。
				// 其它 Info 错误仍响亮失败。
				if os.IsNotExist(err) {
					continue
				}
				return QueueHealth{}, fmt.Errorf("hestia queue health: %s/%s: %w", state, e.Name(), err)
			}
			n++
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
