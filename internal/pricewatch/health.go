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
//
// Down/Up 状态只在调用方 Ack 后推进（AD-30）：发送失败不 Ack，下一次
// Fail/OK 会再次返回同一事件，从而自动重发。
type HealthTracker struct {
	After time.Duration // 连续失败满这么久发 Down

	mu sync.Mutex
	m  map[string]*healthState
}

type healthState struct {
	since time.Time // 本次连续失败的起点
	down  bool      // Down 是否已被确认送达
}

// Fail 记录 key 在 now 失败。连续失败满 After 且 Down 尚未确认时，每次都返回 Down。
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
	return HealthEvent{Key: key, Kind: HealthDown}
}

// OK 记录 key 恢复正常。Down 已确认时每次都返回 Up（Ack 后才回到正常）；
// 否则（短暂失败、Down 未送达、本就正常）清除失败起点，无事件。
func (h *HealthTracker) OK(key string, _ time.Time) HealthEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.m[key]
	if !ok {
		return HealthEvent{}
	}
	if !s.down {
		delete(h.m, key)
		return HealthEvent{}
	}
	return HealthEvent{Key: key, Kind: HealthUp}
}

// Ack 确认 ev 已送达：Down → 该键不再返回 Down；Up → 该键回到正常（下次失败重新计时）。
// HealthNone、未知键、对未确认 Down 的键 Ack Up 均无效果。
func (h *HealthTracker) Ack(ev HealthEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.m[ev.Key]
	if !ok {
		return
	}
	switch ev.Kind {
	case HealthDown:
		s.down = true
	case HealthUp:
		if s.down {
			delete(h.m, ev.Key)
		}
	}
}
