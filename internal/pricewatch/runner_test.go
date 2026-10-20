package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-008)
// functional[0]     at 前后、当天已发、睡眠错过、at 为空、Asia/Shanghai 本地日期 → TestDueHeartbeat
// functional[1]     真实时间 Tick=10ms、30ms/90ms 两资产 → 前者 ≥ 2×后者     → TestRunner_Schedules（另 TestRunner_ScheduleNoDrift 用可控时钟）
// functional[2]     两次 tick 都过 09:00 只发 1 条心跳、kv 为当天、内容来自 Snapshot/Sent24h → TestRunner_HeartbeatOncePerDay
// functional[3]     RunOnce 每资产一次且出错不阻断；Run 立即跑首轮       → TestRunner_RunOnce、TestRunner_FirstRoundImmediate
// boundary[0]       心跳发送失败不写 heartbeat_date，下一 tick 重试成功才写 → TestRunner_HeartbeatRetryAfterSendFailure
// error_handling[0] ctx 取消 1 秒内返回 nil，之后不再调用 CheckAsset       → TestRunner_StopsOnCancel
// 使用约束（TASK-007）同一资产不得并发 CheckAsset                        → TestRunner_SerialPerAsset

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDueHeartbeat(t *testing.T) {
	sh, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	d := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name string
		now  time.Time
		at   string
		last string
		want bool
	}{
		{"未到 at", d(20, 8, 59), "09:00", "", false},
		{"恰到 at", d(20, 9, 0), "09:00", "", true},
		{"已过 at 且昨天发过", d(20, 15, 0), "09:00", "2026-10-19", true},
		{"当天已发", d(20, 9, 30), "09:00", "2026-10-20", false},
		{"睡眠错过次日补发", d(21, 10, 30), "09:00", "2026-10-20", true},
		{"at 为空", d(20, 23, 0), "", "", false},
		{"at 非法", d(20, 23, 0), "9点", "", false},
		// 2026-10-20 01:30 上海 = 2026-10-19 17:30 UTC：本地 at=01:00 已过，本地日期 10-20
		{"上海本地日期已过 at（UTC 仍是前一天）", time.Date(2026, 10, 20, 1, 30, 0, 0, sh), "01:00", "2026-10-19", true},
		{"上海本地当天已发（UTC 日期不同）", time.Date(2026, 10, 20, 1, 30, 0, 0, sh), "01:00", "2026-10-20", false},
		// 2026-10-20 07:30 上海 = 2026-10-19 23:30 UTC：按 UTC 已过 08:00 且日期 ≠ last，按本地未到 08:00
		{"上海本地未到 at（UTC 已过）", time.Date(2026, 10, 20, 7, 30, 0, 0, sh), "08:00", "2026-10-18", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dueHeartbeat(c.now, c.at, c.last); got != c.want {
				t.Fatalf("dueHeartbeat(%s, %q, %q) = %v, want %v", c.now, c.at, c.last, got, c.want)
			}
		})
	}
}

// fakeChecker 记录各资产的调用次数与同一资产的并发峰值。
type fakeChecker struct {
	delay time.Duration
	errOn string

	mu       sync.Mutex
	calls    map[string]int
	inflight map[string]int
	peak     map[string]int
	total    atomic.Int64
}

func newFakeChecker(delay time.Duration) *fakeChecker {
	return &fakeChecker{delay: delay, calls: map[string]int{}, inflight: map[string]int{}, peak: map[string]int{}}
}

func (f *fakeChecker) check(_ context.Context, a Asset) error {
	f.total.Add(1)
	f.mu.Lock()
	f.calls[a.Symbol]++
	f.inflight[a.Symbol]++
	if f.inflight[a.Symbol] > f.peak[a.Symbol] {
		f.peak[a.Symbol] = f.inflight[a.Symbol]
	}
	f.mu.Unlock()

	time.Sleep(f.delay)

	f.mu.Lock()
	f.inflight[a.Symbol]--
	f.mu.Unlock()
	if a.Symbol == f.errOn {
		return errors.New("boom " + a.Symbol)
	}
	return nil
}

func (f *fakeChecker) count(sym string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[sym]
}

func (f *fakeChecker) peakOf(sym string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak[sym]
}

func newFakeRunner(fc *fakeChecker, tick time.Duration, now func() time.Time, assets ...Asset) *Runner {
	return &Runner{Engine: &Engine{Now: now}, Assets: assets, Tick: tick, check: fc.check}
}

// runFor 跑 Run 约 d 后取消，返回 Run 的返回值。
func runFor(t *testing.T, r *Runner, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return r.Run(ctx)
}

func TestRunner_Schedules(t *testing.T) {
	fc := newFakeChecker(0)
	r := newFakeRunner(fc, 10*time.Millisecond, time.Now,
		Asset{Symbol: "FAST", Interval: 30 * time.Millisecond},
		Asset{Symbol: "SLOW", Interval: 90 * time.Millisecond})
	if err := runFor(t, r, 200*time.Millisecond); err != nil {
		t.Fatalf("Run: %v", err)
	}
	fast, slow := fc.count("FAST"), fc.count("SLOW")
	if slow < 1 || fast < 2*slow {
		t.Fatalf("want FAST ≥ 2×SLOW (SLOW ≥ 1), got FAST=%d SLOW=%d", fast, slow)
	}
}

// 可控时钟：tick 落在 10ms 网格上，奇数 tick 晚 5µs（调度抖动）。到期按「上次到期 + Interval」推进，
// 抖动不累积；若按「检测时刻 + Interval」推进，晚到的那次会把下一次推到网格之后而漏掉一个 tick。
func TestRunner_ScheduleNoDrift(t *testing.T) {
	var now time.Time
	fc := newFakeChecker(0)
	r := newFakeRunner(fc, time.Hour, func() time.Time { return now },
		Asset{Symbol: "FAST", Interval: 30 * time.Millisecond},
		Asset{Symbol: "SLOW", Interval: 90 * time.Millisecond})
	t0 := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for k := 0; k < 20; k++ { // t = 0, 10, …, 190ms
		now = t0.Add(time.Duration(k) * 10 * time.Millisecond)
		if k%2 == 1 {
			now = now.Add(5 * time.Microsecond)
		}
		r.step(context.Background())
	}
	if fast, slow := fc.count("FAST"), fc.count("SLOW"); fast != 7 || slow != 3 {
		t.Fatalf("want FAST=7 (0,30,…,180) SLOW=3 (0,90,180), got FAST=%d SLOW=%d", fast, slow)
	}
}

func TestRunner_FirstRoundImmediate(t *testing.T) {
	fc := newFakeChecker(0)
	r := newFakeRunner(fc, time.Hour, time.Now, Asset{Symbol: "BTC", Interval: time.Hour})
	if err := runFor(t, r, 100*time.Millisecond); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := fc.count("BTC"); got != 1 {
		t.Fatalf("first round must run immediately exactly once, got %d", got)
	}
}

func TestRunner_RunOnce(t *testing.T) {
	fc := newFakeChecker(0)
	fc.errOn = "ETH"
	r := newFakeRunner(fc, time.Hour, time.Now,
		Asset{Symbol: "BTC", Interval: time.Hour}, Asset{Symbol: "ETH", Interval: time.Hour}, Asset{Symbol: "SOL", Interval: time.Hour})
	err := r.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ETH") {
		t.Fatalf("want error mentioning ETH, got %v", err)
	}
	for _, sym := range []string{"BTC", "ETH", "SOL"} {
		if got := fc.count(sym); got != 1 {
			t.Fatalf("%s: want exactly 1 call, got %d", sym, got)
		}
	}
	fc.errOn = ""
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce without failures: %v", err)
	}
}

// 同一资产一轮未结束时不得再起下一轮（TASK-007：同一资产不得并发 CheckAsset）。
// 每次检测耗时 50ms，远长于 Tick（5ms）与 Interval（10ms）⇒ 每次检测期间都有多个到期点经过；
// 若实现每 tick 另起 goroutine 而不等上一轮，峰值必然 >1。
func TestRunner_SerialPerAsset(t *testing.T) {
	fc := newFakeChecker(50 * time.Millisecond)
	r := newFakeRunner(fc, 5*time.Millisecond, time.Now,
		Asset{Symbol: "BTC", Interval: 10 * time.Millisecond}, Asset{Symbol: "ETH", Interval: 10 * time.Millisecond})
	if err := runFor(t, r, 400*time.Millisecond); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, sym := range []string{"BTC", "ETH"} {
		if n := fc.count(sym); n < 3 {
			t.Fatalf("%s: want ≥3 calls to exercise overlap, got %d", sym, n)
		}
		if p := fc.peakOf(sym); p != 1 {
			t.Fatalf("%s: concurrent CheckAsset peak = %d, want 1", sym, p)
		}
	}
}

func TestRunner_StopsOnCancel(t *testing.T) {
	fc := newFakeChecker(20 * time.Millisecond) // 不看 ctx 的慢检测
	r := newFakeRunner(fc, 5*time.Millisecond, time.Now, Asset{Symbol: "BTC", Interval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: want nil, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s after cancel")
	}
	before := fc.total.Load()
	if before == 0 {
		t.Fatal("expected some checks before cancel")
	}
	time.Sleep(60 * time.Millisecond)
	if after := fc.total.Load(); after != before {
		t.Fatalf("CheckAsset called after Run returned: %d → %d", before, after)
	}
}

func TestRunner_CancelledBeforeStart(t *testing.T) {
	fc := newFakeChecker(0)
	r := newFakeRunner(fc, time.Hour, time.Now, Asset{Symbol: "BTC", Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if fc.total.Load() != 0 {
		t.Fatal("no check after ctx already cancelled")
	}
}

// heartbeatFixture：真实 Engine，先跑一轮触发一条提醒，使 Snapshot 有价格、Sent24h == 1。
func heartbeatFixture(t *testing.T) (*engFixture, *Runner) {
	t.Helper()
	f := newEngFixture(t, engDayDrop)
	f.okx.setPrice(90) // 自 24h 高点 100 跌 10% ⇒ 触发 day_drop
	f.at(0)
	f.mustRun(t)
	if n := f.e.Sent24h(); n != 1 {
		t.Fatalf("setup: want 1 alert sent, got %d", n)
	}
	// 不带资产：只看心跳，避免检测再发提醒改变 Sent24h
	return f, &Runner{Engine: f.e, HeartbeatAt: "09:00", Tick: time.Hour}
}

const heartbeatPrefix = "💓"

func TestRunner_HeartbeatOncePerDay(t *testing.T) {
	f, r := heartbeatFixture(t)
	ctx := context.Background()

	f.at(50 * time.Minute) // 08:50 UTC，未到 09:00
	r.step(ctx)
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 0 {
		t.Fatalf("heartbeat before 09:00: %d", n)
	}

	now := f.at(90 * time.Minute) // 09:30
	want := FormatHeartbeat(now, f.e.Snapshot(), f.e.Sent24h())
	r.step(ctx)
	f.at(5 * time.Hour) // 13:00，同一天第二次 tick
	r.step(ctx)

	sent := f.snd.sent()
	if n := engCount(sent, heartbeatPrefix); n != 1 {
		t.Fatalf("want exactly 1 heartbeat, got %d: %q", n, sent)
	}
	var got string
	for _, s := range sent {
		if strings.HasPrefix(s, heartbeatPrefix) {
			got = s
		}
	}
	if got != want {
		t.Fatalf("heartbeat text not from Snapshot/Sent24h:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(got, "90") {
		t.Fatalf("heartbeat should carry the snapshot price 90: %q", got)
	}
	if v, _ := f.st.GetKV("heartbeat_date"); v != "2026-10-20" {
		t.Fatalf("heartbeat_date = %q, want 2026-10-20", v)
	}

	f.at(engDay + 90*time.Minute) // 次日 09:30
	r.step(ctx)
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 2 {
		t.Fatalf("next day: want 2 heartbeats total, got %d", n)
	}
	if v, _ := f.st.GetKV("heartbeat_date"); v != "2026-10-21" {
		t.Fatalf("heartbeat_date = %q, want 2026-10-21", v)
	}
}

func TestRunner_HeartbeatRetryAfterSendFailure(t *testing.T) {
	f, r := heartbeatFixture(t)
	ctx := context.Background()
	f.at(90 * time.Minute)

	f.snd.setErr(errors.New("telegram down"))
	r.step(ctx)
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 1 {
		t.Fatalf("want 1 attempted heartbeat, got %d", n)
	}
	if v, _ := f.st.GetKV("heartbeat_date"); v != "" {
		t.Fatalf("heartbeat_date written despite send failure: %q", v)
	}

	f.snd.setErr(nil)
	f.at(100 * time.Minute)
	r.step(ctx)
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 2 {
		t.Fatalf("want retry on next tick, attempts=%d", n)
	}
	if v, _ := f.st.GetKV("heartbeat_date"); v != "2026-10-20" {
		t.Fatalf("heartbeat_date = %q after successful retry", v)
	}
	f.at(110 * time.Minute)
	r.step(ctx)
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 2 {
		t.Fatalf("no more heartbeat after success, attempts=%d", n)
	}
}

// 心跳日期按本地日期写：上海 10-21 01:30 = UTC 10-20 17:30。
func TestRunner_HeartbeatDateIsLocal(t *testing.T) {
	sh, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	f, r := heartbeatFixture(t)
	r.HeartbeatAt = "01:00"
	f.e.Now = func() time.Time { return time.Date(2026, 10, 21, 1, 30, 0, 0, sh) }
	r.step(context.Background())
	if n := engCount(f.snd.sent(), heartbeatPrefix); n != 1 {
		t.Fatalf("want 1 heartbeat, got %d", n)
	}
	if v, _ := f.st.GetKV("heartbeat_date"); v != "2026-10-21" {
		t.Fatalf("heartbeat_date = %q, want local date 2026-10-21", v)
	}
}
