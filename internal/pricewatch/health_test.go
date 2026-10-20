package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-011)
// functional[0]     After=30m：29m 无事件、30m 返回 Down、确认后 60m 不重复 → TestHealth_DownAfterThreshold
// functional[1]     Down 后 OK 返回 Up、确认后再 OK 无事件；短暂失败无事件 → TestHealth_UpAfterAckedDown、TestHealth_TransientFailureSilent
// functional[2]     不同键互不影响；Fail→OK→Fail 重新计时             → TestHealth_KeysIndependent、TestHealth_RestartTimingAfterOK
// non_functional[0] 并发 Fail/OK/Ack 在 -race 下无 data race           → TestHealth_ConcurrentSafe、TestHealth_ConcurrentFailOKAck
// QA R1-1（AD-30）  只在 Ack 后推进；未 Ack 每次重复返回；未确认 Down 就恢复 ⇒ 无事件 →
//                   TestHealth_UnackedDownRepeats、TestHealth_UnackedUpRepeats、TestHealth_RecoverBeforeDownAcked、TestHealth_AckEdgeCases

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

// failAck / okAck 模拟「发送成功后确认」的调用方。
func failAck(h *HealthTracker, key string, now time.Time) HealthEvent {
	ev := h.Fail(key, now)
	h.Ack(ev)
	return ev
}

func okAck(h *HealthTracker, key string, now time.Time) HealthEvent {
	ev := h.OK(key, now)
	h.Ack(ev)
	return ev
}

func TestHealth_DownAfterThreshold(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	noHealthEvent(t, "T0", failAck(h, k, healthAt(0)))
	noHealthEvent(t, "T0+29m", failAck(h, k, healthAt(29)))
	wantHealthEvent(t, "T0+30m", failAck(h, k, healthAt(30)), k, HealthDown)
	noHealthEvent(t, "已确认后 T0+31m", failAck(h, k, healthAt(31)))
	noHealthEvent(t, "已确认后 T0+60m", failAck(h, k, healthAt(60)))
}

func TestHealth_DownJustBeforeThreshold(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	h.Fail("k", healthT0)
	noHealthEvent(t, "T0+30m-1ns", h.Fail("k", healthT0.Add(30*time.Minute-time.Nanosecond)))
}

func TestHealth_UnackedDownRepeats(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	h.Fail(k, healthAt(0))
	wantHealthEvent(t, "30m 首次", h.Fail(k, healthAt(30)), k, HealthDown)
	wantHealthEvent(t, "未确认 → 35m 重复", h.Fail(k, healthAt(35)), k, HealthDown)
	wantHealthEvent(t, "未确认 → 90m 重复", h.Fail(k, healthAt(90)), k, HealthDown)
	h.Ack(HealthEvent{Key: k, Kind: HealthDown})
	noHealthEvent(t, "确认后 95m", h.Fail(k, healthAt(95)))
}

func TestHealth_UpAfterAckedDown(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "gap:BTC/week_drop"
	h.Fail(k, healthAt(0))
	wantHealthEvent(t, "Down", failAck(h, k, healthAt(30)), k, HealthDown)
	wantHealthEvent(t, "OK", okAck(h, k, healthAt(35)), k, HealthUp)
	noHealthEvent(t, "已确认恢复后再 OK", okAck(h, k, healthAt(40)))
}

func TestHealth_UnackedUpRepeats(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "sync:BTC/1d"
	h.Fail(k, healthAt(0))
	failAck(h, k, healthAt(30))
	wantHealthEvent(t, "首次 OK", h.OK(k, healthAt(35)), k, HealthUp)
	wantHealthEvent(t, "未确认 → 再 OK 重复", h.OK(k, healthAt(40)), k, HealthUp)
	// Up 未确认期间又失败：用户看到的仍是「异常」，不应再发 Down
	noHealthEvent(t, "Up 未确认时 Fail", h.Fail(k, healthAt(45)))
	wantHealthEvent(t, "仍返回 Up", h.OK(k, healthAt(50)), k, HealthUp)
	h.Ack(HealthEvent{Key: k, Kind: HealthUp})
	noHealthEvent(t, "确认恢复后 OK", h.OK(k, healthAt(55)))
	// 确认恢复后重新从零计时
	noHealthEvent(t, "重新失败", h.Fail(k, healthAt(60)))
	noHealthEvent(t, "重新失败 29m", h.Fail(k, healthAt(89)))
	wantHealthEvent(t, "重新失败 30m", h.Fail(k, healthAt(90)), k, HealthDown)
}

// QA R1-1 场景：第 30 分钟发送失败（不 Ack），之后恢复 ⇒ 不能只有「已恢复」而没有「异常」。
func TestHealth_RecoverBeforeDownAcked(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	h.Fail(k, healthAt(0))
	wantHealthEvent(t, "30m Down（发送失败，不确认）", h.Fail(k, healthAt(30)), k, HealthDown)
	noHealthEvent(t, "未确认 Down 就恢复 ⇒ 无事件", h.OK(k, healthAt(31)))
	// 计时已清除：再次失败从头计时
	noHealthEvent(t, "再失败", h.Fail(k, healthAt(32)))
	noHealthEvent(t, "再失败 29m", h.Fail(k, healthAt(61)))
	wantHealthEvent(t, "再失败 30m", h.Fail(k, healthAt(62)), k, HealthDown)
}

func TestHealth_AckEdgeCases(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	h.Ack(HealthEvent{})                                 // 无事件：no-op
	h.Ack(HealthEvent{Key: "price:X", Kind: HealthDown}) // 未知键：no-op，不凭空造出 Down 状态
	noHealthEvent(t, "未知键被 Ack Down 后 OK", h.OK("price:X", healthAt(0)))
	noHealthEvent(t, "未知键 Fail 从头计时", h.Fail("price:X", healthAt(1)))
	noHealthEvent(t, "29m", h.Fail("price:X", healthAt(30)))
	h.Ack(HealthEvent{Key: "price:X", Kind: HealthUp}) // 未确认 Down 的键被 Ack Up：不应清掉计时
	wantHealthEvent(t, "计时未被清除", h.Fail("price:X", healthAt(31)), "price:X", HealthDown)
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
	wantHealthEvent(t, "BTC 到时", failAck(h, "price:BTC", healthAt(30)), "price:BTC", HealthDown)
	noHealthEvent(t, "ETH 未到时", h.Fail("price:ETH", healthAt(30)))
	noHealthEvent(t, "ETH 短暂失败恢复", h.OK("price:ETH", healthAt(31)))
	noHealthEvent(t, "BTC 已确认不重复", h.Fail("price:BTC", healthAt(32)))
	wantHealthEvent(t, "BTC 恢复", okAck(h, "price:BTC", healthAt(33)), "price:BTC", HealthUp)
}

func TestHealth_RestartTimingAfterOK(t *testing.T) {
	h := &HealthTracker{After: 30 * time.Minute}
	const k = "price:BTC"
	h.Fail(k, healthAt(0))
	h.OK(k, healthAt(20))
	noHealthEvent(t, "第二次 Fail", h.Fail(k, healthAt(25)))
	noHealthEvent(t, "距首次 Fail 已 30m 但距第二次仅 5m", h.Fail(k, healthAt(30)))
	noHealthEvent(t, "距第二次 29m", h.Fail(k, healthAt(54)))
	wantHealthEvent(t, "距第二次 30m", failAck(h, k, healthAt(55)), k, HealthDown)

	// Down→Up 之后再失败，同样从新起点计时、可再次 Down
	wantHealthEvent(t, "Up", okAck(h, k, healthAt(60)), k, HealthUp)
	noHealthEvent(t, "再失败", h.Fail(k, healthAt(61)))
	noHealthEvent(t, "再失败 29m", h.Fail(k, healthAt(90)))
	wantHealthEvent(t, "再失败 30m", h.Fail(k, healthAt(91)), k, HealthDown)
}

func TestHealth_ZeroValueUsable(t *testing.T) {
	var h HealthTracker // After=0：首次失败即告警
	wantHealthEvent(t, "Fail", failAck(&h, "k", healthT0), "k", HealthDown)
	wantHealthEvent(t, "OK", okAck(&h, "k", healthT0), "k", HealthUp)
	var h2 HealthTracker
	h2.Ack(HealthEvent{Key: "k", Kind: HealthDown}) // 零值上 Ack 不 panic
}

// 多个 goroutine 对共享键并发 Fail 并确认：每个键至少 1 次 Down；全部结束后该键处于已确认 Down。
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
				if ev := failAck(h, k, healthAt(m)); ev.Kind == HealthDown {
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
		if downs[k] < 1 {
			t.Fatalf("%s 没有任何 Down", k)
		}
		noHealthEvent(t, k+" 已确认 Down 后 Fail", h.Fail(k, healthAt(61)))
		wantHealthEvent(t, k+" 恢复", okAck(h, k, healthAt(62)), k, HealthUp)
	}
}

// Fail、OK、Ack 对同一键（及不同键）并发交替：去掉任一方法的锁时 -race 必报 DATA RACE。
// 用 start 屏障让所有 goroutine 同时开跑，保证读写确有重叠。
// 结束后状态须自洽：先 OK 并确认（若仍为 Down 则返回 Up），之后 Fail 立即重新给出 Down。
func TestHealth_ConcurrentFailOKAck(t *testing.T) {
	var h HealthTracker // After=0：每次失败即可产生 Down
	var wg sync.WaitGroup
	var mu sync.Mutex
	start := make(chan struct{})
	ackedDown := map[string]int{}
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			k := fmt.Sprintf("price:K%d", g%2) // 每个键 4 个 goroutine 共享
			<-start
			for i := 0; i < 300; i++ {
				var ev HealthEvent
				if (i+g)%2 == 0 {
					ev = h.Fail(k, healthAt(i))
				} else {
					ev = h.OK(k, healthAt(i))
				}
				if i%3 != 0 { // 部分事件不确认，模拟发送失败
					h.Ack(ev)
					if ev.Kind == HealthDown {
						mu.Lock()
						ackedDown[ev.Key]++
						mu.Unlock()
					}
				}
			}
		}(g)
	}
	close(start)
	wg.Wait()
	for i := 0; i < 2; i++ {
		k := fmt.Sprintf("price:K%d", i)
		if ackedDown[k] == 0 {
			t.Fatalf("%s: 没有任何被确认的 Down", k)
		}
		okAck(&h, k, healthAt(300))
		noHealthEvent(t, k+" 恢复后再 OK", h.OK(k, healthAt(301)))
		wantHealthEvent(t, k+" 恢复后 Fail", h.Fail(k, healthAt(302)), k, HealthDown)
	}
}
