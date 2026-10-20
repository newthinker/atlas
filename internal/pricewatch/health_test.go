package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-011)
// functional[0]     After=30m：29m 无事件、30m 恰 1 次 Down、60m 不重复 → TestHealth_DownOnceAfterThreshold
// functional[1]     Down 后 OK 恰 1 次 Up、再 OK 无事件；短暂失败无事件 → TestHealth_UpOnceAfterDown、TestHealth_TransientFailureSilent
// functional[2]     不同键互不影响；Fail→OK→Fail 重新计时             → TestHealth_KeysIndependent、TestHealth_RestartTimingAfterOK
// non_functional[0] 并发调用 Fail/OK 在 -race 下无 data race          → TestHealth_ConcurrentSafe、TestHealth_ConcurrentFailOK

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var healthT0 = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

func healthAt(m int) time.Time { return healthT0.Add(time.Duration(m) * time.Minute) }

func noHealthEvent(t *testing.T, step string, ev HealthEvent) {
	t.Helper()
	if ev.Kind != HealthNone {
		t.Fatalf("%s: 期望无事件，得到 %+v", step, ev)
	}
}

func wantHealthEvent(t *testing.T, step string, ev HealthEvent, key string, kind HealthKind) {
	t.Helper()
	if ev != (HealthEvent{Key: key, Kind: kind}) {
		t.Fatalf("%s: 期望 {%s %v}，得到 %+v", step, key, kind, ev)
	}
}

func TestHealth_DownOnceAfterThreshold(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	noHealthEvent(t, "T0", h.Fail(k, healthAt(0)))
	noHealthEvent(t, "T0+29m", h.Fail(k, healthAt(29)))
	wantHealthEvent(t, "T0+30m", h.Fail(k, healthAt(30)), k, HealthDown)
	noHealthEvent(t, "T0+31m", h.Fail(k, healthAt(31)))
	noHealthEvent(t, "T0+60m", h.Fail(k, healthAt(60)))
}

func TestHealth_DownJustBeforeThreshold(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	h.Fail("k", healthT0)
	noHealthEvent(t, "T0+30m-1ns", h.Fail("k", healthT0.Add(30*time.Minute-time.Nanosecond)))
}

func TestHealth_UpOnceAfterDown(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "gap:BTC/week_drop"
	h.Fail(k, healthAt(0))
	wantHealthEvent(t, "Down", h.Fail(k, healthAt(30)), k, HealthDown)
	wantHealthEvent(t, "OK", h.OK(k, healthAt(35)), k, HealthUp)
	noHealthEvent(t, "再 OK", h.OK(k, healthAt(40)))
}

func TestHealth_TransientFailureSilent(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "sync:BTC/1d"
	noHealthEvent(t, "Fail", h.Fail(k, healthAt(0)))
	noHealthEvent(t, "Fail", h.Fail(k, healthAt(29)))
	noHealthEvent(t, "OK", h.OK(k, healthAt(29)))
	noHealthEvent(t, "从未失败的键 OK", h.OK("price:ETH", healthAt(29)))
}

func TestHealth_KeysIndependent(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	h.Fail("price:BTC", healthAt(0))
	noHealthEvent(t, "ETH 首次失败", h.Fail("price:ETH", healthAt(20)))
	wantHealthEvent(t, "BTC 到时", h.Fail("price:BTC", healthAt(30)), "price:BTC", HealthDown)
	noHealthEvent(t, "ETH 未到时", h.Fail("price:ETH", healthAt(30)))
	noHealthEvent(t, "ETH 短暂失败恢复", h.OK("price:ETH", healthAt(31)))
	noHealthEvent(t, "BTC 已告警不重复", h.Fail("price:BTC", healthAt(32)))
	wantHealthEvent(t, "BTC 恢复", h.OK("price:BTC", healthAt(33)), "price:BTC", HealthUp)
}

func TestHealth_RestartTimingAfterOK(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	h.Fail(k, healthAt(0))
	h.OK(k, healthAt(20))
	noHealthEvent(t, "第二次 Fail", h.Fail(k, healthAt(25)))
	noHealthEvent(t, "距首次 Fail 已 30m 但距第二次仅 5m", h.Fail(k, healthAt(30)))
	noHealthEvent(t, "距第二次 29m", h.Fail(k, healthAt(54)))
	wantHealthEvent(t, "距第二次 30m", h.Fail(k, healthAt(55)), k, HealthDown)

	// Down→Up 之后再失败，同样从新起点计时、可再次 Down
	wantHealthEvent(t, "Up", h.OK(k, healthAt(60)), k, HealthUp)
	noHealthEvent(t, "再失败", h.Fail(k, healthAt(61)))
	noHealthEvent(t, "再失败 29m", h.Fail(k, healthAt(90)))
	wantHealthEvent(t, "再失败 30m", h.Fail(k, healthAt(91)), k, HealthDown)
}

func TestHealth_ZeroValueUsable(t *testing.T) {
	var h HealthTracker // After=0：首次失败即告警
	wantHealthEvent(t, "Fail", h.Fail("k", healthT0), "k", HealthDown)
	wantHealthEvent(t, "OK", h.OK("k", healthT0), "k", HealthUp)
}

func TestHealth_ConcurrentSafe(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	var wg sync.WaitGroup
	var mu sync.Mutex
	downs := map[string]int{}
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			k := fmt.Sprintf("price:K%d", g%4) // 两个 goroutine 共享一个键
			for m := 0; m <= 60; m++ {
				if ev := h.Fail(k, healthAt(m)); ev.Kind == HealthDown {
					mu.Lock()
					downs[ev.Key]++
					mu.Unlock()
				}
			}
		}(g)
	}
	wg.Wait()
	for i := 0; i < 4; i++ {
		k := fmt.Sprintf("price:K%d", i)
		if downs[k] != 1 {
			t.Fatalf("%s 的 Down 次数 = %d，期望恰 1", k, downs[k])
		}
		wantHealthEvent(t, k+" 恢复", h.OK(k, healthAt(61)), k, HealthUp)
	}
}

// Fail 与 OK 对同一键（及不同键）并发交替调用：OK 去锁时 -race 必报 DATA RACE。
// 每个键的事件序列在锁下串行，因此 Down/Up 必然交替且以 Down 开头：ups ≤ downs ≤ ups+1。
func TestHealth_ConcurrentFailOK(t *testing.T) {
	var h HealthTracker // After=0：每次失败起点即告警，使 Down/Up 频繁出现
	var wg sync.WaitGroup
	var mu sync.Mutex
	downs, ups := map[string]int{}, map[string]int{}
	record := func(ev HealthEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case HealthDown:
			downs[ev.Key]++
		case HealthUp:
			ups[ev.Key]++
		}
	}
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			k := fmt.Sprintf("price:K%d", g%2) // 每个键 4 个 goroutine 共享
			for i := 0; i < 200; i++ {
				if (i+g)%2 == 0 {
					record(h.Fail(k, healthAt(i)))
				} else {
					record(h.OK(k, healthAt(i)))
				}
			}
		}(g)
	}
	wg.Wait()
	for i := 0; i < 2; i++ {
		k := fmt.Sprintf("price:K%d", i)
		if downs[k] == 0 || ups[k] > downs[k] || downs[k] > ups[k]+1 {
			t.Fatalf("%s: downs=%d ups=%d，违反交替不变量", k, downs[k], ups[k])
		}
	}
}
