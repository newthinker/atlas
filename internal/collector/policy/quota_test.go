package policy

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[0]     windowStart：Window>=24h 按 Loc 自然日对齐 / Window<24h 按 UTC 截断
//                       → TestWindowStartAlignsNaturalDay / TestWindowStartSubDayUsesTruncate
//                       + TestWindowStartNilLocUsesUTC（Loc 未配置时的兜底，方案实现有该分支）
// functional[1]     MemStore.Take 放行到 Limit / 窗口翻篇归零 / 主题隔离
//                       → TestMemStoreTakeUpToLimit / TestMemStoreResetsOnNewWindow / TestMemStoreIsolatesTopics
// functional[2]     配额用尽返 ErrQuotaExceeded 且 fn 一次都不被调用
//                       → TestGateBlocksBeforeSendingRequest
// functional[3]     New 签名变更，TASK-002 全部用例保持全绿
//                       → 由 gate_test.go 全绿保证（newTestGate 已跟随改为 New(tbl, nil)）
// boundary[0]       被拒的请求不计数 / QuotaStore 为 nil 时一律放行
//                       → TestMemStoreTakeUpToLimit（Count 仍为 3）/ TestGateWithoutQuotaStoreAllows
// boundary[1]       fn 已执行但返回错误的请求必须计数
//                       → TestGateCountsFailedRequests
// boundary[2]①      缓存命中不消耗配额 —— TTL 命中时 Take 不被调用
//                       → TestCacheHitDoesNotConsumeQuota
// boundary[2]②      Coalesce 合并的 N 个并发同 key 请求只消耗 1 次配额
//                       → TestCoalescedRequestsConsumeOneQuota
// boundary[2]③      未登记主题不得触达配额记账（约束 C6）
//                       → TestUnregisteredTopicDoesNotTouchQuota
// error_handling[0] 分句1 账本异常时放行全部请求
//                       → TestGateFailsOpenOnStoreError（fnCalls == 3）
// error_handling[0] 分句2 经 WithWarn 上报该 err
//                       → TestGateFailsOpenOnStoreError（errors.Is(warned, storeErr)）
// error_handling[0] 未注入 WithWarn 时 fail-open 不得 panic
//                       → TestFailOpenWithoutWarnDoesNotPanic
//
// ⚠ boundary[2] 三条都是**否定断言**（「Take 不被调用」「只调用 1 次」），必须用
//   可观测的假账本 countingStore 直接数调用次数。**不能看 MemStore.Count**：
//   计数值不变既可能是「没调用」，也可能是「调用了但被拒且不计数」（boundary[0]
//   恰好规定被拒不计数），两条路径在计数值上无法区分。

// countingStore 是可观测的假账本：直接记录 Take 的调用次数与主题。
// 用于 boundary[2] 那三条否定断言 —— 见上方注释说明为何不能用 MemStore.Count。
type countingStore struct {
	mu     sync.Mutex
	calls  int
	topics []string
	allow  bool
}

func (c *countingStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.topics = append(c.topics, topic)
	return c.allow, nil
}

func (c *countingStore) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// brokenStore 永远报错，用于验证 fail-open。
type brokenStore struct{ err error }

func (b brokenStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	return false, b.err
}

func TestWindowStartAlignsNaturalDay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	q := Quota{Limit: 5, Window: 24 * time.Hour, Loc: loc}

	now := time.Date(2026, 8, 6, 23, 59, 59, 0, loc)
	want := time.Date(2026, 8, 6, 0, 0, 0, 0, loc)
	if got := windowStart(now, q); !got.Equal(want) {
		t.Errorf("windowStart = %v, want %v", got, want)
	}

	// 同一自然日内任意时刻应落到同一个窗口起点
	early := time.Date(2026, 8, 6, 0, 0, 1, 0, loc)
	if !windowStart(early, q).Equal(windowStart(now, q)) {
		t.Error("同一自然日的两个时刻窗口起点应相同")
	}
	// 跨日必须换窗口
	next := time.Date(2026, 8, 7, 0, 0, 1, 0, loc)
	if windowStart(next, q).Equal(windowStart(now, q)) {
		t.Error("跨自然日应换窗口")
	}
}

func TestWindowStartSubDayUsesTruncate(t *testing.T) {
	q := Quota{Limit: 1, Window: time.Minute}
	now := time.Date(2026, 8, 6, 10, 30, 45, 0, time.UTC)
	want := time.Date(2026, 8, 6, 10, 30, 0, 0, time.UTC)
	if got := windowStart(now, q); !got.Equal(want) {
		t.Errorf("windowStart = %v, want %v", got, want)
	}
}

// TestWindowStartNilLocUsesUTC 覆盖 functional[0] 中 Loc 未配置的兜底路径：
// 自然日对齐需要一个时区，Quota 由调用方构造、Loc 可能为 nil，此时按 UTC 自然日
// 对齐而不是 panic。
func TestWindowStartNilLocUsesUTC(t *testing.T) {
	q := Quota{Limit: 1, Window: 24 * time.Hour} // Loc 为 nil
	now := time.Date(2026, 8, 6, 23, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

	got := windowStart(now, q)
	if !got.Equal(want) {
		t.Errorf("Loc 为 nil 时应按 UTC 自然日对齐: got %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("窗口起点的时区 = %v, want UTC", got.Location())
	}
}

func TestMemStoreTakeUpToLimit(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 3, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		ok, err := m.Take("t", q, now)
		if err != nil || !ok {
			t.Fatalf("第 %d 次 Take: (%v, %v), want (true, nil)", i, ok, err)
		}
	}
	ok, err := m.Take("t", q, now)
	if err != nil || ok {
		t.Fatalf("第 4 次 Take: (%v, %v), want (false, nil)", ok, err)
	}
	if got := m.Count("t"); got != 3 {
		t.Errorf("被拒的请求不得计数: Count = %d, want 3", got)
	}
}

func TestMemStoreResetsOnNewWindow(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	day1 := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)

	if ok, _ := m.Take("t", q, day1); !ok {
		t.Fatal("day1 首次应放行")
	}
	if ok, _ := m.Take("t", q, day1); ok {
		t.Fatal("day1 第二次应被拒")
	}
	if ok, _ := m.Take("t", q, day2); !ok {
		t.Error("窗口翻篇后计数应归零")
	}
}

func TestMemStoreIsolatesTopics(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()
	if ok, _ := m.Take("a", q, now); !ok {
		t.Fatal("a 首次应放行")
	}
	if ok, _ := m.Take("b", q, now); !ok {
		t.Error("不同主题账本互不影响")
	}
}

func TestGateBlocksBeforeSendingRequest(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 2, Window: 24 * time.Hour, Loc: time.UTC}})
	store := NewMemStore()
	g := New(tbl, store)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }

	for i := 1; i <= 2; i++ {
		if _, err := Fetch(g, "a.x", "k", fn); err != nil {
			t.Fatalf("第 %d 次: %v", i, err)
		}
	}
	_, err := Fetch(g, "a.x", "k", fn)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
	if fnCalls != 2 {
		t.Errorf("超额请求必须在发出前被拦下: fn 调用 %d 次, want 2", fnCalls)
	}
}

func TestGateCountsFailedRequests(t *testing.T) {
	// 请求已发出，服务端已计数 —— 本地账本也必须计数（设计 §4.4）
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC}})
	store := NewMemStore()
	g := New(tbl, store)

	boom := errors.New("boom")
	if _, err := Fetch(g, "a.x", "k", func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got := store.Count("a.x"); got != 1 {
		t.Errorf("失败请求应计数: Count = %d, want 1", got)
	}
}

// TestCacheHitDoesNotConsumeQuota 覆盖 boundary[2]①：TTL 命中时 Take 不被调用。
//
// 否则 daily_basic 的 5 次/天会被缓存命中吃掉——逻辑上只发 2 次 HTTP 却报配额
// 耗尽，与设计 §4.4「未发出的请求不计数」直接冲突。
func TestCacheHitDoesNotConsumeQuota(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{
		TTL:   time.Minute,
		Quota: &Quota{Limit: 10, Window: 24 * time.Hour, Loc: time.UTC},
	})
	store := &countingStore{allow: true}
	g := New(tbl, store)

	fn := func() (int, error) { return 7, nil }
	for i := 0; i < 3; i++ {
		if v, err := Fetch(g, "a.x", "k", fn); err != nil || v != 7 {
			t.Fatalf("第 %d 次: got (%d, %v)", i, v, err)
		}
	}

	if n := store.Calls(); n != 1 {
		t.Errorf("3 次 Fetch 只有首次真正发请求，Take 应只被调用 1 次, got %d", n)
	}
}

// TestCoalescedRequestsConsumeOneQuota 覆盖 boundary[2]②：合并的 N 个并发同 key
// 请求只消耗 1 次配额（配额预判必须在 singleflight 内侧）。
//
// 否则 errgroup 并发下 3 个 waiter 一次吃掉 3 次配额，5 次/天一轮见底、降级链
// 无谓触发，而 HTTP 只发了 1 次。
func TestCoalescedRequestsConsumeOneQuota(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{
		Coalesce: true,
		Quota:    &Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC},
	})
	store := &countingStore{allow: true}
	g := New(tbl, store)

	const n = 20
	fn := func() (int, error) { time.Sleep(50 * time.Millisecond); return 1, nil }
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) { defer wg.Done(); _, errs[i] = Fetch(g, "a.x", "same", fn) }(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("waiter %d: %v", i, errs[i])
		}
	}
	if got := store.Calls(); got != 1 {
		t.Errorf("%d 个合并请求只应消耗 1 次配额: Take 调用 %d 次, want 1", n, got)
	}
}

// TestUnregisteredTopicDoesNotTouchQuota 覆盖 boundary[2]③（约束 C6）。
//
// 这条针对的是 fetch 里 `if !ok { return fn() }` 这个直通短路：删掉它，在
// takeQuota 还是空桩时是等价变异（零值 Policy 让后续步骤全部短路），但接入
// 真实 QuotaStore 后未登记主题会被计入配额，而
// TestGateUnregisteredTopicPassesThrough 测不出（它只断言 fn 次数与耗时）。
func TestUnregisteredTopicDoesNotTouchQuota(t *testing.T) {
	store := &countingStore{allow: true}
	g := New(&Table{policies: make(map[string]Policy)}, store) // 空表：任何主题都未登记

	fn := func() (int, error) { return 1, nil }
	for _, topic := range []string{"eastmoney.kline", "akshare.valuation", "crypto.ticker"} {
		if _, err := Fetch(g, topic, "k", fn); err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
	}

	if n := store.Calls(); n != 0 {
		t.Errorf("未登记主题不得触达配额记账（约束 C6）: Take 调用 %d 次 %v, want 0",
			n, store.topics)
	}
}

func TestGateFailsOpenOnStoreError(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}})

	storeErr := errors.New("ledger corrupted")
	var warned error
	g := New(tbl, brokenStore{err: storeErr}, WithWarn(func(_ string, err error) { warned = err }))

	fnCalls := 0
	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "a.x", "k", func() (int, error) { fnCalls++; return 1, nil }); err != nil {
			t.Fatalf("账本异常必须 fail-open, got err = %v", err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("fail-open 应放行全部请求: fn 调用 %d 次, want 3", fnCalls)
	}
	if !errors.Is(warned, storeErr) {
		t.Errorf("fail-open 必须告警: warned = %v, want %v", warned, storeErr)
	}
}

// TestFailOpenWithoutWarnDoesNotPanic 覆盖 error_handling[0] 的末句：未注入
// WithWarn 时 fail-open 路径不得 panic（New 须给 warn 兜底空实现）。
//
// 这是「不可达分支」型断言：正常接线都会注入 WithWarn，只有未接线的调用点会
// 走到兜底。若 New 不给 warn 兜底，这里会 nil 函数调用 panic。
func TestFailOpenWithoutWarnDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("未注入 WithWarn 时 fail-open 不得 panic: %v", r)
		}
	}()

	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}})
	g := New(tbl, brokenStore{err: errors.New("ledger corrupted")}) // 不注入 WithWarn

	v, err := Fetch(g, "a.x", "k", func() (int, error) { return 42, nil })
	if err != nil || v != 42 {
		t.Errorf("fail-open 应放行: got (%d, %v), want (42, nil)", v, err)
	}
}

func TestGateWithoutQuotaStoreAllows(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}})
	g := New(tbl, nil) // 未接线账本

	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "a.x", "k", func() (int, error) { return 1, nil }); err != nil {
			t.Fatalf("无 QuotaStore 时应放行, got %v", err)
		}
	}
}
