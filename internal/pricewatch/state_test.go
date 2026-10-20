package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[2] 计划 10 例 + 恰满间隔 + 用尽后跌深一档，每行断言 next 与 kind → TestDecide
// boundary[1]   不修改入参；零值 LastSentAt 视为已满间隔                    → TestDecide（case 3）、TestDecide_DoesNotMutatePrev

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	p := NotifyPolicy{RepeatInterval: time.Hour, MaxPerTier: 3}
	T0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	var zero time.Time

	cases := []struct {
		name     string
		prev     RuleState
		tier     int
		now      time.Time
		wantKind SendKind
		wantNext RuleState
	}{
		{"1 未满足且未提醒过", RuleState{0, 0, zero}, 0, T0, SendNone, RuleState{0, 0, zero}},
		{"2 不满足清零保留 LastSentAt", RuleState{2, 1, T0}, 0, T0.Add(time.Hour), SendNone, RuleState{0, 0, T0}},
		{"3 从未发过立即提醒", RuleState{0, 0, zero}, 1, T0, SendNew, RuleState{1, 1, T0}},
		{"4 第1档防抖", RuleState{0, 0, T0}, 1, T0.Add(30 * time.Minute), SendNone, RuleState{0, 0, T0}},
		{"5 第1档已满间隔", RuleState{0, 0, T0}, 1, T0.Add(time.Hour), SendNew, RuleState{1, 1, T0.Add(time.Hour)}},
		{"6 第2档不受防抖", RuleState{0, 0, T0}, 2, T0.Add(time.Minute), SendNew, RuleState{2, 1, T0.Add(time.Minute)}},
		{"7 跌进更深一档", RuleState{1, 1, T0}, 3, T0.Add(time.Minute), SendNew, RuleState{3, 1, T0.Add(time.Minute)}},
		{"8 同档未满间隔", RuleState{2, 1, T0}, 2, T0.Add(59 * time.Minute), SendNone, RuleState{2, 1, T0}},
		{"9 回落仍在档内重复提醒", RuleState{2, 1, T0}, 1, T0.Add(time.Hour), SendRepeat, RuleState{2, 2, T0.Add(time.Hour)}},
		{"10 已达 max_per_tier", RuleState{2, 3, T0}, 2, T0.Add(5 * time.Hour), SendNone, RuleState{2, 3, T0}},
		{"恰满间隔重复", RuleState{2, 1, T0}, 2, T0.Add(time.Hour), SendRepeat, RuleState{2, 2, T0.Add(time.Hour)}},
		{"次数用尽后跌深一档", RuleState{2, 3, T0}, 3, T0.Add(time.Minute), SendNew, RuleState{3, 1, T0.Add(time.Minute)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next, kind := Decide(c.prev, c.tier, c.now, p)
			if kind != c.wantKind {
				t.Errorf("kind = %v, want %v", kind, c.wantKind)
			}
			if next.Tier != c.wantNext.Tier || next.SentInTier != c.wantNext.SentInTier || !next.LastSentAt.Equal(c.wantNext.LastSentAt) {
				t.Errorf("next = %+v, want %+v", next, c.wantNext)
			}
		})
	}
}

func TestDecide_DoesNotMutatePrev(t *testing.T) {
	p := NotifyPolicy{RepeatInterval: time.Hour, MaxPerTier: 3}
	T0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	prev := RuleState{Tier: 2, SentInTier: 1, LastSentAt: T0}
	snapshot := prev
	for _, tier := range []int{0, 1, 2, 3} {
		Decide(prev, tier, T0.Add(2*time.Hour), p)
		if prev != snapshot {
			t.Fatalf("tier %d: prev mutated to %+v", tier, prev)
		}
	}
}
