package policy

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[0] 同域串行 + 同域跨主题共享闸门
//                     → TestGateThrottlesSameDomain
// functional[0] **跨域必须并发验证** —— 三个测试分工，各自能捕获的变异形态不同：
//   TestGateThrottleIsPerDomain            方案原版串行检查，**任何全局锁形态都测不出**，保留仅作基础回归
//   TestGateThrottleIsPerDomainConcurrent  双域对称，捕获「所有域共享同一个 lastReq」
//   TestGateThrottleDoesNotHoldGlobalLock  a 域长等待 + b 域不节流，捕获「每域独立 lastReq 但 throttle 额外持全局锁」
//   ⚠ 后两者互补、缺一不可：双域对称版测不出后一种形态（throttle 等的是绝对时刻，
//     两域同时预热时 B 的等待期与 A 的并行流逝了）。详见两函数各自的注释。
// functional[1] 20 并发同 key 只触发 1 次 fn 且全部拿到正确返回值
//                     → TestGateCoalescesConcurrentSameKey
// functional[1] 不同 key 不合并
//                     → TestGateCoalesceIsPerKey
// functional[1] **缓存 key 含 topic**（断言 fn 调用次数，不能断言返回值）
//                     → TestCacheKeyIncludesTopic
// functional[1] **singleflight key 含 topic**（断言每个 waiter 拿到非零值）
//                     → TestCoalesceKeyIncludesTopic
// functional[2] TTL 内只调一次 / 过期后重调
//                     → TestGateTTLHitSkipsFn / TestGateTTLExpires
// functional[2] **缓存命中不等待 MinInterval**（执行链 ① 在 ④ 之前）
//                     → TestCacheHitSkipsThrottle
// functional[3] Do 强制 TTL=0 每次执行 / Wait 只节流
//                     → TestDoForcesNoCache / TestWaitThrottlesWithoutFn / TestWaitDoesNotCache
// boundary[0]   未登记主题直通，3 次 Fetch 触发 3 次 fn 且 <50ms
//                     → TestGateUnregisteredTopicPassesThrough
// boundary[1]   nil *Gate 完全透明
//                     → TestNilGateIsTransparent
// boundary[2]   超过 maxCacheEntries 淘汰最旧
//                     → TestGateEvictsOldestEntry
// error_handling[0] fn 出错不写缓存 / 超时返回 ErrTimeout 且不写缓存
//                     → TestGateDoesNotCacheErrors / TestGateTimeout / TestGateTimeoutDoesNotCache
// error_handling[0] **Coalesce 失败时全部 waiter 共享同一错误**（+ 失败后在途表须已清理）
//                     → TestCoalesceSharesErrorWithAllWaiters
// non_functional[0] -race 全绿（由 CI 命令保证）；**runWithTimeout 不泄漏 goroutine**
//                     → TestRunWithTimeoutDoesNotLeakGoroutine
// non_functional[0] **Timeout=0 语义为不限时**（不得实现成立即超时）
//                     → TestZeroTimeoutMeansNoLimit

// newTestGate 返回一个只含 topics 的 Gate（不含内置表），避免测试受生产
// 数值（8s 等）拖慢。
func newTestGate(t *testing.T, topics map[string]Policy) *Gate {
	t.Helper()
	tbl := &Table{policies: make(map[string]Policy)}
	for topic, p := range topics {
		tbl.Set(topic, p)
	}
	return New(tbl, nil) // 不接配额账本：配额相关用例在 quota_test.go
}

func TestGateThrottlesSameDomain(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {MinInterval: 80 * time.Millisecond},
		"a.y": {MinInterval: 80 * time.Millisecond},
	})
	var calls []time.Time
	record := func() (int, error) { calls = append(calls, time.Now()); return 0, nil }

	if _, err := Fetch(g, "a.x", "k", record); err != nil {
		t.Fatal(err)
	}
	// 同域不同主题也必须共享闸门（设计 §3.3）
	if _, err := Fetch(g, "a.y", "k", record); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if gap := calls[1].Sub(calls[0]); gap < 60*time.Millisecond {
		t.Errorf("同域相邻请求间隔 = %v, want >= 60ms（留计时容差）", gap)
	}
}

// TestGateThrottleIsPerDomain 是方案原版的串行检查。**它不构成守护**：
// b 域的 domainState 是新建的、lastReq 为零值，无论闸门是按域隔离还是全局
// 一把锁，这里都会立即返回。真正的守护是下面的并发版。保留它只为基础回归。
func TestGateThrottleIsPerDomain(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {MinInterval: 300 * time.Millisecond},
		"b.x": {MinInterval: 300 * time.Millisecond},
	})
	noop := func() (int, error) { return 0, nil }
	if _, err := Fetch(g, "a.x", "k", noop); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := Fetch(g, "b.x", "k", noop); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("跨域不应互相阻塞, 第二次耗时 %v", elapsed)
	}
}

// TestGateThrottleIsPerDomainConcurrent 覆盖 functional[0] 的「跨域必须并发验证」。
//
// **双域对称设计**：两域都预热过，因此两域的下一次请求都必须各等 ~300ms。
// 用 barrier 同时放行两个 goroutine，测总耗时：
//
//	按域隔离 → 两者并行等待，总耗时 ~300ms
//	全局一把锁 → 串行，总耗时 ~600ms
//
// 它不依赖「谁先进入 throttle」——throttle 的 sleep 发生在持锁临界区内，
// 从包外没有任何可观测信号，「等它进入 sleep」只能靠猜，猜错就退化成顺序调用
// 而断言照样绿。本设计里无论调度顺序如何，全局锁都必然串行、按域隔离都必然
// 并行；把测试误写成顺序调用也必然 600ms 而转红。
func TestGateThrottleIsPerDomainConcurrent(t *testing.T) {
	const minInterval = 300 * time.Millisecond
	g := newTestGate(t, map[string]Policy{
		"a.x": {MinInterval: minInterval},
		"b.x": {MinInterval: minInterval},
	})
	noop := func() (int, error) { return 0, nil }

	// 预热两个域：此后两域的下一次请求都要各等满 minInterval。
	for _, topic := range []string{"a.x", "b.x"} {
		if _, err := Fetch(g, topic, "warm", noop); err != nil {
			t.Fatal(err)
		}
	}

	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(2)
	done.Add(2)
	for _, topic := range []string{"a.x", "b.x"} {
		go func(topic string) {
			defer done.Done()
			ready.Done()
			<-start // barrier：排除 goroutine 启动开销，不靠 sleep 猜时序
			if _, err := Fetch(g, topic, "k", noop); err != nil {
				t.Errorf("%s: %v", topic, err)
			}
		}(topic)
	}
	ready.Wait()
	t0 := time.Now()
	close(start)
	done.Wait()

	if elapsed := time.Since(t0); elapsed > 450*time.Millisecond {
		t.Errorf("跨域请求被串行化，总耗时 %v；按域隔离应 ~%v，全局锁则 ~%v",
			elapsed, minInterval, 2*minInterval)
	}
}

// TestGateThrottleDoesNotHoldGlobalLock 覆盖 functional[0] 的另一半：throttle
// 等待期间不得持有**跨域**的锁（twelvedata 的 8s 等待会卡死进程内所有 collector）。
//
// 它与上面的双域对称版互补，两个都需要 —— 双域对称版**测不出这一形态**：
// throttle 等的是绝对时刻 lastReq+MinInterval，不是「轮到我时再等 MinInterval」。
// 两域几乎同时预热时，A 持全局锁等到 T+300ms 释放，B 拿到锁时它要等的绝对时刻
// 也已经过了，于是立即返回，总耗时与按域隔离无异。**要暴露全局锁，必须让 B 要等的
// 时刻显著晚于 A 的完成时刻**：故这里 a 域长等待、b 域只等一个极小间隔。
//
// ⚠ b 域的 MinInterval **必须 > 0**：throttle 开头有 `if p.MinInterval <= 0 { return }`
// 的快速路径，配成 0 则 b 域根本不进入持锁临界区，于是只能捕获「全局锁加在该
// early-return **之前**」这一种形态，加在**之后**（只保护真正节流的路径，而
// 「先做快速路径检查再拿锁」是很自然的写法）的形态会静默存活。
//
// ⚠ 局限（如实声明）：本测试靠一个短 sleep 让 A 先进入 throttle，是**概率性**的
// —— throttle 的等待发生在持锁临界区内，包外没有任何可观测信号，无法确定性地
// 知道 A 已持锁。但猜错的后果是**假绿而非假红**（B 抢先拿到锁则测不出问题），
// 故用多轮重复降低漏检概率。
func TestGateThrottleDoesNotHoldGlobalLock(t *testing.T) {
	const rounds = 3
	for round := 0; round < rounds; round++ {
		g := newTestGate(t, map[string]Policy{
			"a.x": {MinInterval: 500 * time.Millisecond},
			// b 域必须 MinInterval > 0：throttle 开头有 `if p.MinInterval <= 0 { return }`
			// 的快速路径，配成 0 的话 b 域**根本不进入持锁临界区**，任何加在该
			// early-return 之后的全局锁对它都不可见。取极小正值：既进入临界区，
			// 正确实现下又几乎立即返回，不影响下面的 150ms 阈值。
			"b.x": {MinInterval: time.Millisecond},
		})
		noop := func() (int, error) { return 0, nil }

		// 预热 a 域：其下一次请求要等满 500ms。
		if _, err := Fetch(g, "a.x", "warm", noop); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = Fetch(g, "a.x", "k", noop) }()

		time.Sleep(20 * time.Millisecond) // 让 A 进入 throttle（概率性，见函数注释）

		start := time.Now()
		if _, err := Fetch(g, "b.x", "k", noop); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		wg.Wait()

		if elapsed > 150*time.Millisecond {
			t.Fatalf("第 %d 轮: a 域节流期间 b 域被阻塞 %v —— throttle 持有了跨域的全局锁",
				round, elapsed)
		}
	}
}

func TestGateCoalescesConcurrentSameKey(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Coalesce: true}})
	var mu sync.Mutex
	fnCalls := 0
	fn := func() (string, error) {
		mu.Lock()
		fnCalls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond) // 撑开在途窗口
		return "value", nil
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = Fetch(g, "a.x", "same", fn)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if fnCalls != 1 {
		t.Errorf("fn 被调用 %d 次, want 1（singleflight 合并）", fnCalls)
	}
	// 「全部拿到正确返回值」是量词断言。results 是定长切片、索引由测试控制，
	// 故遍历必然执行 n 次，无空真风险；若改用 append 收集则必须先断言 len。
	for i := 0; i < n; i++ {
		if errs[i] != nil || results[i] != "value" {
			t.Fatalf("caller %d: got (%q, %v), want (\"value\", nil)", i, results[i], errs[i])
		}
	}
}

func TestGateCoalesceIsPerKey(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Coalesce: true}})
	var mu sync.Mutex
	fnCalls := 0
	fn := func() (int, error) {
		mu.Lock()
		fnCalls++
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		return 1, nil
	}
	var wg sync.WaitGroup
	wg.Add(2)
	for _, key := range []string{"k1", "k2"} {
		go func(key string) { defer wg.Done(); _, _ = Fetch(g, "a.x", key, fn) }(key)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if fnCalls != 2 {
		t.Errorf("不同 key 不得合并: fn 调用 %d 次, want 2", fnCalls)
	}
}

// TestCacheKeyIncludesTopic 覆盖 functional[1] 的「缓存 key 必须包含 topic」。
//
// ⚠ **只能断言 fn 调用次数，不能断言返回值**：缓存命中处的类型断言失败会
// 「当作未命中重新取」，所以 key 不含 topic 时返回值**仍然正确**，bug 自愈成
// 「多调一次 fn」——断言返回值的写法永远绿。同理也不能断言 `strings.Contains(ck, topic)`，
// 那在 `ck = topic` 这种同样错误的实现下也绿。
func TestCacheKeyIncludesTopic(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {TTL: time.Minute},
		"a.y": {TTL: time.Minute},
	})
	xCalls, yCalls := 0, 0
	fnX := func() (int, error) { xCalls++; return 1, nil }
	fnY := func() (string, error) { yCalls++; return "eps", nil }

	if _, err := Fetch(g, "a.x", "AAPL", fnX); err != nil {
		t.Fatal(err)
	}
	// 同一个 key、不同主题、不同 T：ck 不含 topic 时这一步会覆盖上一步的条目。
	if _, err := Fetch(g, "a.y", "AAPL", fnY); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(g, "a.x", "AAPL", fnX); err != nil {
		t.Fatal(err)
	}

	if xCalls != 1 {
		t.Errorf("a.x 的 fn 调用 %d 次, want 1（a.y 的同 key 请求不得覆盖 a.x 的缓存条目）", xCalls)
	}
	if yCalls != 1 {
		t.Errorf("a.y 的 fn 调用 %d 次, want 1", yCalls)
	}
}

// TestCoalesceKeyIncludesTopic 覆盖 functional[1] 的「singleflight key 必须包含 topic」。
//
// 两组并发打在同一个 key、不同主题、不同 T 上。sf key 不含 topic 时两组会被
// 合并成一次调用：其中一个 fn 根本不执行（调用次数为 0），且它那一组的 waiter
// 会因 `tv, _ := v.(T)` 断言失败而**静默拿到零值 + nil error**。故两个方向都断言。
func TestCoalesceKeyIncludesTopic(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {Coalesce: true},
		"a.y": {Coalesce: true},
	})
	var mu sync.Mutex
	xCalls, yCalls := 0, 0
	fnX := func() (int, error) {
		mu.Lock()
		xCalls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return 42, nil
	}
	fnY := func() (string, error) {
		mu.Lock()
		yCalls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return "eps", nil
	}

	const n = 10
	xs := make([]int, n)
	ys := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(2 * n)
	for i := 0; i < n; i++ {
		go func(i int) { defer wg.Done(); xs[i], _ = Fetch(g, "a.x", "same", fnX) }(i)
		go func(i int) { defer wg.Done(); ys[i], _ = Fetch(g, "a.y", "same", fnY) }(i)
	}
	wg.Wait()

	mu.Lock()
	if xCalls != 1 || yCalls != 1 {
		t.Errorf("两个主题应各自合并一次: xCalls = %d, yCalls = %d, want 1/1（0 表示该组被跨主题合并掉了）",
			xCalls, yCalls)
	}
	mu.Unlock()

	for i := 0; i < n; i++ {
		if xs[i] != 42 {
			t.Errorf("a.x waiter %d 拿到 %d, want 42（零值 = sf key 跨主题串味后的静默失败）", i, xs[i])
		}
		if ys[i] != "eps" {
			t.Errorf("a.y waiter %d 拿到 %q, want \"eps\"（空串 = 同上）", i, ys[i])
		}
	}
}

func TestGateTTLHitSkipsFn(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 42, nil }

	for i := 0; i < 3; i++ {
		v, err := Fetch(g, "a.x", "k", fn)
		if err != nil || v != 42 {
			t.Fatalf("call %d: got (%d, %v)", i, v, err)
		}
	}
	if fnCalls != 1 {
		t.Errorf("TTL 内 fn 调用 %d 次, want 1", fnCalls)
	}
}

func TestGateTTLExpires(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: 30 * time.Millisecond}})
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return fnCalls, nil }

	if v, _ := Fetch(g, "a.x", "k", fn); v != 1 {
		t.Fatalf("first: got %d", v)
	}
	time.Sleep(50 * time.Millisecond)
	if v, _ := Fetch(g, "a.x", "k", fn); v != 2 {
		t.Errorf("TTL 过期后应重新调用 fn, got %d", v)
	}
}

// TestCacheHitSkipsThrottle 覆盖 functional[2] 的「缓存命中必须不等待 MinInterval」。
// 执行链 ①查缓存 必须在 ④节流 之前，否则 twelvedata 缓存命中还要白等 8s，缓存等于没加。
func TestCacheHitSkipsThrottle(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {TTL: time.Minute, MinInterval: 500 * time.Millisecond},
	})
	fn := func() (int, error) { return 7, nil }

	if _, err := Fetch(g, "a.x", "k", fn); err != nil { // 填充缓存，并把 lastReq 置为 now
		t.Fatal(err)
	}
	start := time.Now()
	v, err := Fetch(g, "a.x", "k", fn)
	elapsed := time.Since(start)

	if err != nil || v != 7 {
		t.Fatalf("got (%d, %v), want (7, nil)", v, err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("缓存命中仍等了节流 %v（查缓存必须先于节流）", elapsed)
	}
}

func TestGateDoesNotCacheErrors(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	wantErr := errors.New("boom")
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 0, wantErr }

	for i := 0; i < 2; i++ {
		if _, err := Fetch(g, "a.x", "k", fn); !errors.Is(err, wantErr) {
			t.Fatalf("call %d: err = %v, want %v", i, err, wantErr)
		}
	}
	if fnCalls != 2 {
		t.Errorf("错误不得写缓存: fn 调用 %d 次, want 2", fnCalls)
	}
}

// TestCoalesceSharesErrorWithAllWaiters 覆盖 error_handling[0] 的
// 「Coalesce 路径下 fn 失败时全部等待者共享同一错误」（设计 §5.2）。
//
// 若对 waiter 返回零值 + nil error，20 个并发调用方会拿到「空数据但无错误」，
// 上层降级链不触发、静默写入空结果。
//
// 末尾额外验「失败后在途表已清理」：漏清会让后续调用命中已失败的旧条目而不
// 重新执行 fn。singleflight 自身会在 fn 返回后删 key，但实现若为配额/诊断
// 另加了在途记录，就可能漏清。
func TestCoalesceSharesErrorWithAllWaiters(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Coalesce: true}})
	wantErr := errors.New("boom")
	var mu sync.Mutex
	calls := 0
	fn := func() (int, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return 0, wantErr
	}

	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) { defer wg.Done(); _, errs[i] = Fetch(g, "a.x", "same", fn) }(i)
	}
	wg.Wait()

	mu.Lock()
	if calls != 1 {
		t.Errorf("fn 调用 %d 次, want 1", calls)
	}
	mu.Unlock()

	for i := 0; i < n; i++ {
		if !errors.Is(errs[i], wantErr) {
			t.Errorf("waiter %d 拿到 err = %v, want %v（零值+nil error 会让上层降级链不触发）",
				i, errs[i], wantErr)
		}
	}

	if _, err := Fetch(g, "a.x", "same", fn); !errors.Is(err, wantErr) {
		t.Errorf("失败后重试: err = %v, want %v", err, wantErr)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("失败后再次调用必须重新执行 fn: calls = %d, want 2（在途表漏清会复用已失败的条目）", calls)
	}
}

func TestGateTimeout(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Timeout: 30 * time.Millisecond}})
	_, err := Fetch(g, "a.x", "k", func() (int, error) {
		time.Sleep(500 * time.Millisecond)
		return 1, nil
	})
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

func TestGateTimeoutDoesNotCache(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute, Timeout: 20 * time.Millisecond}})
	slow := func() (int, error) { time.Sleep(200 * time.Millisecond); return 1, nil }
	if _, err := Fetch(g, "a.x", "k", slow); !errors.Is(err, ErrTimeout) {
		t.Fatalf("first: err = %v, want ErrTimeout", err)
	}
	fast := func() (int, error) { return 7, nil }
	if v, err := Fetch(g, "a.x", "k", fast); err != nil || v != 7 {
		t.Errorf("超时不得写缓存: got (%d, %v), want (7, nil)", v, err)
	}
}

// TestZeroTimeoutMeansNoLimit 覆盖 non_functional[0] 的「Timeout=0 语义为不限时」。
// 内置表所有主题的 Timeout 均为 0，实现成「立即超时」会让全部主题一起失效。
func TestZeroTimeoutMeansNoLimit(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Timeout: 0}})
	v, err := Fetch(g, "a.x", "k", func() (int, error) {
		time.Sleep(100 * time.Millisecond)
		return 42, nil
	})
	if err != nil || v != 42 {
		t.Errorf("Timeout=0 应不限时: got (%d, %v), want (42, nil)", v, err)
	}
}

// TestRunWithTimeoutDoesNotLeakGoroutine 覆盖 non_functional[0] 的
// 「超时后 fn 的 goroutine 结果写入带缓冲 channel 不泄漏」。
// 结果 channel 若无缓冲，超时后没人再读，那个 goroutine 会永久阻塞在发送处。
func TestRunWithTimeoutDoesNotLeakGoroutine(t *testing.T) {
	fnReturned := make(chan struct{})
	base := runtime.NumGoroutine()

	_, err := runWithTimeout(20*time.Millisecond, func() (int, error) {
		defer close(fnReturned)
		time.Sleep(100 * time.Millisecond)
		return 1, nil
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}

	<-fnReturned // fn 本体已返回；其结果写入带缓冲 channel 后 goroutine 应立即退出
	for i := 0; i < 100; i++ {
		if runtime.NumGoroutine() <= base {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("超时后 fn 的 goroutine 未退出: NumGoroutine = %d, base = %d",
		runtime.NumGoroutine(), base)
}

func TestGateUnregisteredTopicPassesThrough(t *testing.T) {
	g := newTestGate(t, nil)
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "eastmoney.kline", "k", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("未登记主题应直通不缓存: fn 调用 %d 次, want 3", fnCalls)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("未登记主题不应被节流, 耗时 %v", elapsed)
	}
}

func TestNilGateIsTransparent(t *testing.T) {
	var g *Gate
	v, err := Fetch(g, "a.x", "k", func() (int, error) { return 9, nil })
	if err != nil || v != 9 {
		t.Errorf("nil Gate 应直通: got (%d, %v)", v, err)
	}
	g.Wait("a.x") // 不得 panic
	if err := g.Do("a.x", "k", func() error { return nil }); err != nil {
		t.Errorf("nil Gate Do: %v", err)
	}
}

func TestDoForcesNoCache(t *testing.T) {
	// 同一主题带 TTL，但 Do 必须每次都执行 fn（设计 §3.2：Do 强制 TTL=0）
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	fnCalls := 0
	for i := 0; i < 3; i++ {
		if err := g.Do("a.x", "k", func() error { fnCalls++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("Do 调用 %d 次 fn, want 3", fnCalls)
	}
}

func TestWaitThrottlesWithoutFn(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {MinInterval: 80 * time.Millisecond}})
	g.Wait("a.x")
	start := time.Now()
	g.Wait("a.x")
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("Wait 应施加节流, 第二次耗时 %v", elapsed)
	}

	// 未登记主题：不节流、不 panic（与 Fetch 的直通语义一致）。
	start = time.Now()
	g.Wait("eastmoney.kline")
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("未登记主题不应被 Wait 节流, 耗时 %v", elapsed)
	}
}

// TestWithWarnIsApplied 守护 Option 的注入契约：TASK-003 的配额 fail-open 要靠
// 这个回调上报账本异常，若 Option 没被应用，告警会静默丢失而一切测试照常绿。
// 顺带覆盖 New(nil) 用内置表的分支。
func TestWithWarnIsApplied(t *testing.T) {
	var gotMsg string
	var gotErr error
	wantErr := errors.New("ledger broken")

	g := New(nil, nil, WithWarn(func(msg string, err error) { gotMsg, gotErr = msg, err }))
	if g.table == nil {
		t.Fatal("New(nil) 应使用内置表")
	}
	if _, ok := g.table.Lookup("yahoo.chart"); !ok {
		t.Error("New(nil) 的表里应有内置主题")
	}

	g.warn("quota ledger unavailable", wantErr)
	if gotMsg != "quota ledger unavailable" || !errors.Is(gotErr, wantErr) {
		t.Errorf("WithWarn 注入的回调未被调用: msg = %q, err = %v", gotMsg, gotErr)
	}
}

// TestWaitDoesNotCache 覆盖 functional[3] 的「Wait 只施加节流，不碰缓存/合并/配额」
// 那一半——方案原有的 TestWaitThrottlesWithoutFn 只验了节流。
func TestWaitDoesNotCache(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute, Coalesce: true}})
	g.Wait("a.x")

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	if v, err := Fetch(g, "a.x", "k", fn); err != nil || v != 1 {
		t.Fatalf("got (%d, %v), want (1, nil)", v, err)
	}
	if fnCalls != 1 {
		t.Errorf("Wait 不得往缓存里塞东西: fn 调用 %d 次, want 1", fnCalls)
	}
	if n := g.entryCount(); n != 1 {
		t.Errorf("缓存条目数 = %d, want 1（Wait 自身不应产生条目）", n)
	}
}

func TestGateEvictsOldestEntry(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Hour}})
	for i := 0; i < maxCacheEntries+10; i++ {
		key := time.Duration(i).String()
		if _, err := Fetch(g, "a.x", key, func() (int, error) { return i, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if n := g.entryCount(); n > maxCacheEntries {
		t.Errorf("缓存条目 %d 超过上限 %d", n, maxCacheEntries)
	}
}
