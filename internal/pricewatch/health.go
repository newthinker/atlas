package pricewatch

import (
	"sync"
	"time"
)

// HealthKind 表示一次 Fail/OK 调用后需要发送的健康消息种类。
type HealthKind int

const (
	HealthNone HealthKind = iota // 无需发送
	HealthDown                   // 连续失败已满 After：「监控异常」
	HealthUp                     // 告警过的键恢复：「已恢复」
)

// HealthEvent 是需要发送的健康消息；Kind==HealthNone 表示无事件。
// 调用方用 FormatHealth(ev.Kind == HealthDown, …) 生成文本。
type HealthEvent struct {
	Key  string
	Kind HealthKind
}

// HealthTracker 追踪各键的连续失败（spec §9、AD-25）。键形如
// "price:BTC"、"gap:BTC/week_drop"、"sync:BTC/1d"。状态只放内存，
// 重启清空（AD-15）。零值可用；可被多个 goroutine 并发调用。
type HealthTracker struct {
	After time.Duration // 连续失败满这么久发 Down

	mu sync.Mutex
	m  map[string]*healthState
}

type healthState struct {
	since time.Time // 本次连续失败的起点
	down  bool      // 是否已发过 Down
}

// Fail 记录 key 在 now 失败。连续失败满 After 时返回一次 Down，之后不重复。
func (h *HealthTracker) Fail(key string, now time.Time) HealthEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = map[string]*healthState{}
	}
	s, ok := h.m[key]
	if !ok {
		s = &healthState{since: now}
		h.m[key] = s
	}
	if s.down || now.Sub(s.since) < h.After {
		return HealthEvent{}
	}
	s.down = true
	return HealthEvent{Key: key, Kind: HealthDown}
}

// OK 记录 key 恢复正常并清除失败起点。已发过 Down 时返回一次 Up；
// 短暂失败（未发 Down）或本就正常时无事件。
func (h *HealthTracker) OK(key string, _ time.Time) HealthEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.m[key]
	if !ok {
		return HealthEvent{}
	}
	delete(h.m, key)
	if !s.down {
		return HealthEvent{}
	}
	return HealthEvent{Key: key, Kind: HealthUp}
}
