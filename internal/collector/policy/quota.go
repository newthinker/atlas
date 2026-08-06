package policy

import (
	"errors"
	"sync"
	"time"
)

// ErrQuotaExceeded 表示本地配额预判判定该主题在当前窗口已用尽。
//
// 语义与 tushare.ErrRateLimited 一致：**临时性**，窗口过后自愈。各 collector
// 负责在自己的包内把它映射成本包既有的哨兵错误，policy 的错误不外泄到
// prism 层（设计 §5.1）。
var ErrQuotaExceeded = errors.New("policy: quota exceeded")

// QuotaStore 是配额账本。实现必须并发安全。
//
// Take 返回 (true, nil) 表示放行并已计数；(false, nil) 表示当前窗口已用尽
// （**不计数** —— 请求没发出去）；err != nil 表示账本本身异常，调用方
// 必须 fail-open（放行 + 告警），不因账本损坏阻断降级链（设计 §4.4）。
type QuotaStore interface {
	Take(topic string, q Quota, now time.Time) (bool, error)
}

// windowStart 返回 now 所属窗口的起点。
//
// Window >= 24h 走自然日对齐（tushare 的「5 次/天」是自然日口径，不是滑动
// 24 小时）；更短的窗口按 UTC 截断——分钟级窗口没有时区含义。
// Loc 为 nil 时按 UTC 自然日对齐：Quota 由调用方构造，缺时区不该 panic。
func windowStart(now time.Time, q Quota) time.Time {
	if q.Window >= 24*time.Hour {
		loc := q.Loc
		if loc == nil {
			loc = time.UTC
		}
		t := now.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	}
	return now.UTC().Truncate(q.Window)
}

// ledgerEntry 是单个主题的账本条目。JSON tag 供 FileStore 复用。
type ledgerEntry struct {
	WindowStart time.Time `json:"window_start"`
	Count       int       `json:"count"`
}

// take 是窗口判定 + 计数的纯逻辑，被内存与文件两个实现共用。
// 返回 (放行?, 更新后的条目)。
//
// Limit <= 0 表示不设上限（`q.Limit > 0` 是计数上限判定的前置条件）。
func take(e ledgerEntry, q Quota, now time.Time) (bool, ledgerEntry) {
	ws := windowStart(now, q)
	if !e.WindowStart.Equal(ws) {
		e = ledgerEntry{WindowStart: ws, Count: 0}
	}
	if q.Limit > 0 && e.Count >= q.Limit {
		return false, e // 拦下的请求不计数
	}
	e.Count++
	return true, e
}

// MemStore 是进程内配额账本。**在 launchd 短命进程形态下无效**
// （每次启动归零，设计 §1.5），仅供测试与不需要跨进程语义的场景。
type MemStore struct {
	mu      sync.Mutex
	ledgers map[string]ledgerEntry
}

func NewMemStore() *MemStore {
	return &MemStore{ledgers: make(map[string]ledgerEntry)}
}

func (m *MemStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ok, e := take(m.ledgers[topic], q, now)
	m.ledgers[topic] = e
	return ok, nil
}

// Count 返回当前窗口已用次数（测试辅助）。
func (m *MemStore) Count(topic string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ledgers[topic].Count
}
