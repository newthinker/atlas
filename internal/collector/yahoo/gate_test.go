package yahoo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// Context Checkpoint: done_criteria → test mapping
//
// functional[0]     FetchHistory 经 Gate 走 TTL 缓存，同参数只发一次 HTTP
//                       → TestFetchHistoryCachesViaGate
// functional[1]     FetchQuote 用 yahoo.quote 主题、TTL=0 不缓存
//                       → TestFetchQuoteIsNotCached
// functional[1]     ……**但仍受 500ms 节流**（后半句，方案未覆盖）
//                       → TestFetchQuoteStillThrottled
// functional[2]     chart 与 eps 同域共享闸门
//                       → TestChartAndEPSShareThrottleDomain
// functional[3]     重试循环只在 attempt>0 调 Wait，**首次请求不重复等待**（方案未覆盖）
//                       → TestFirstRequestDoesNotWaitTwice
//                       （重试预算/退避等既有行为由 throttle_test.go 原有用例守护）
// boundary[0]       缓存命中返回独立切片
//                       → TestFetchHistoryReturnsIndependentSlice
//
// ⚠ 「TTL=0 不缓存」与「仍受节流」是 functional[1] 的两个分句，证据方向不同：
//   前者证明每次都真的发了 HTTP，后者证明这些请求之间仍有间隔。只验前者时，
//   把 quote 主题排除出限流域也照样绿。

// TestMain 把进程默认闸门换成「零间隔、零 TTL」，否则本包每个用例都会被
// 生产策略的 500ms 节流拖慢，且 TTL 会吞掉重复请求让计数断言失真。
// 单个用例需要真闸门时自己给 y.gate 赋值（见 gateWith）。
func TestMain(m *testing.M) {
	policy.SetDefault(gateWith(policy.Policy{}))
	os.Exit(m.Run())
}

// gateWith 用同一份策略登记三个 yahoo 主题（同域，共享闸门）。
func gateWith(p policy.Policy) *policy.Gate {
	tbl := policy.NewTable()
	for _, topic := range []string{topicChart, topicEPS, topicQuote} {
		q := p
		q.Domain = "yahoo"
		tbl.Set(topic, q)
	}
	return policy.New(tbl, nil)
}

// countingServer 返回一个记录命中次数与到达时刻的 chart 服务端。
func countingServer(t *testing.T) (*httptest.Server, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var arrivals []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Time(nil), arrivals...)
	}
}

// TestNewUsesDefaultGate 守护装配：构造函数必须取 policy.Default()。
//
// ⚠ 这条缺了会形成与 A1 同型的盲区：本包**其余每个用例都自己给 y.gate 赋值**，
// 所以 New 里漏掉 `gate: policy.Default()` 时它们全都照常绿——而生产路径拿到的
// 是 nil gate，policy.Gate 对 nil 是透明直通的，于是限流/缓存/配额**全部静默失效**。
// 判据用指针相等：Default() 返回的就是那个单例，不需要通过行为间接推断。
func TestNewUsesDefaultGate(t *testing.T) {
	orig := policy.Default()
	t.Cleanup(func() { policy.SetDefault(orig) })

	marker := gateWith(policy.Policy{TTL: time.Minute})
	policy.SetDefault(marker)

	if y := New(); y.gate != marker {
		t.Error("New 必须把 policy.Default() 存进 gate 字段；" +
			"为 nil 时 Gate 透明直通，config 装配的策略全部静默失效")
	}
	if y := NewWithBaseURL("http://x"); y.gate != marker {
		t.Error("NewWithBaseURL 同样必须取 policy.Default()")
	}
}

func TestFetchHistoryCachesViaGate(t *testing.T) {
	srv, arrivals := countingServer(t)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 3; i++ {
		if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := len(arrivals()); n != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", n)
	}
}

func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv, _ := countingServer(t)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	first, err := y.FetchHistory("AAPL", start, end, "1d")
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bars, %v)", len(first), err)
	}
	first[0].Close = -999 // 调用方原地改写

	second, err := y.FetchHistory("AAPL", start, end, "1d")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Close == -999 {
		t.Error("缓存命中必须返回独立副本，否则调用方能污染缓存")
	}
}

func TestFetchQuoteIsNotCached(t *testing.T) {
	srv, arrivals := countingServer(t)

	y := NewWithBaseURL(srv.URL)
	// 生产 yahoo.quote 主题 TTL=0；这里显式登记同一策略验证实时语义
	tbl := policy.NewTable()
	tbl.Set(topicQuote, policy.Policy{Domain: "yahoo", TTL: 0, Coalesce: true})
	tbl.Set(topicChart, policy.Policy{Domain: "yahoo"})
	y.gate = policy.New(tbl, nil)

	for i := 0; i < 3; i++ {
		if _, err := y.FetchQuote("AAPL"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := len(arrivals()); n != 3 {
		t.Errorf("FetchQuote 必须保持实时不缓存, HTTP 请求 %d 次, want 3", n)
	}
}

// TestFetchQuoteStillThrottled 覆盖 functional[1] 的后半句：quote 虽 TTL=0
// 不缓存，但**仍受同一个 yahoo 闸门节流**。
//
// 只验「不缓存」是不够的：把 quote 主题从限流域里摘掉（或干脆不登记）时，
// TestFetchQuoteIsNotCached 照样绿——3 次请求全发出去了，只是没有间隔。
func TestFetchQuoteStillThrottled(t *testing.T) {
	const iv = 80 * time.Millisecond
	srv, arrivals := countingServer(t)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{MinInterval: iv}) // TTL 为 0

	for i := 0; i < 2; i++ {
		if _, err := y.FetchQuote("AAPL"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	got := arrivals()
	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got))
	}
	if gap := got[1].Sub(got[0]); gap < 60*time.Millisecond {
		t.Errorf("quote 不缓存但仍应受 yahoo 闸门节流, 间隔 %v", gap)
	}
}

// TestEachMethodUsesItsOwnTopic 守护「三个方法各自用对主题」。
//
// 其余用例都用 gateWith 给三个主题登记**同一份策略**，于是把 FetchQuote 或
// FetchEPSHistory 的主题写成 topicChart 也照样绿。这里反过来给 chart 单独配 TTL、
// 另两个不配，用「是否被缓存」区分主题：误用 chart 主题的方法会被缓存住。
//
// 后果不是纸面的：生产内置表里 chart 的 TTL 是 5 分钟，FetchQuote 若误用 chart
// 主题就会被缓存 5 分钟，「实时报价」直接失效。
func TestEachMethodUsesItsOwnTopic(t *testing.T) {
	var mu sync.Mutex
	var chartHits, epsHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.URL.Query().Get("type") == "trailingDilutedEPS" {
			epsHits++
		} else {
			chartHits++
		}
		mu.Unlock()
		if r.URL.Query().Get("type") == "trailingDilutedEPS" {
			_, _ = w.Write([]byte(`{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`))
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	tbl := policy.NewTable()
	tbl.Set(topicChart, policy.Policy{Domain: "yahoo", TTL: time.Minute}) // 只有 chart 缓存
	tbl.Set(topicEPS, policy.Policy{Domain: "yahoo"})                     // TTL=0
	tbl.Set(topicQuote, policy.Policy{Domain: "yahoo"})                   // TTL=0
	y := NewWithBaseURL(srv.URL)
	y.gate = policy.New(tbl, nil)

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 2; i++ {
		if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
			t.Fatal(err)
		}
		if _, err := y.FetchEPSHistory("AAPL", start, end); err != nil {
			t.Fatal(err)
		}
		if _, err := y.FetchQuote("AAPL"); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	// chart 有 TTL → 2 次调用只发 1 次；quote 无 TTL → 每次都发，与 chart 共用
	// 同一个 handler 计数，故 chartHits = 1(history) + 2(quote) = 3
	if chartHits != 3 {
		t.Errorf("chart+quote 命中 %d 次, want 3（history 被缓存 1 次 + quote 不缓存 2 次）；"+
			"少于 3 说明 FetchQuote 误用了带 TTL 的 chart 主题", chartHits)
	}
	if epsHits != 2 {
		t.Errorf("eps 命中 %d 次, want 2（eps 主题 TTL=0 不缓存）；"+
			"为 1 说明 FetchEPSHistory 误用了带 TTL 的 chart 主题", epsHits)
	}
}

func TestChartAndEPSShareThrottleDomain(t *testing.T) {
	var mu sync.Mutex
	var arrivals []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
		if r.URL.Query().Get("type") == "trailingDilutedEPS" {
			_, _ = w.Write([]byte(`{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`))
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{MinInterval: 80 * time.Millisecond})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
		t.Fatal(err)
	}
	if _, err := y.FetchEPSHistory("AAPL", start, end); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(arrivals) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(arrivals))
	}
	if gap := arrivals[1].Sub(arrivals[0]); gap < 60*time.Millisecond {
		t.Errorf("chart 与 eps 同域应共享闸门, 间隔 %v", gap)
	}
}

// TestFirstRequestDoesNotWaitTwice 覆盖 functional[3]：重试循环只在 attempt>0
// 时调 Gate.Wait。
//
// policy.Fetch 在进入 fn 之前已经节流过一次；若 do() 在 attempt==0 也 Wait，
// 首次请求会等**两倍**间隔。判据是单次请求的耗时：正确实现 ~1×iv，
// 无条件 Wait 则 ~2×iv。
func TestFirstRequestDoesNotWaitTwice(t *testing.T) {
	const iv = 100 * time.Millisecond
	srv, _ := countingServer(t)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{MinInterval: iv}) // 不设 TTL，每次都真发请求

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	// 预热：把 yahoo 域的 lastReq 置为「刚刚」，让下一次请求必须等满 iv
	if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now()
	if _, err := y.FetchHistory("MSFT", start, end, "1d"); err != nil { // 换 key 绕开缓存
		t.Fatal(err)
	}
	elapsed := time.Since(t0)

	if elapsed > iv+50*time.Millisecond {
		t.Errorf("首次请求等了 %v（约 %v 的两倍）—— Gate 已在进入 fn 前节流一次，"+
			"do() 不得在 attempt==0 时再 Wait", elapsed, iv)
	}
}

// ——— TASK-007 review_fix：两条缺口的守护 ———

// errorEnvelopeBody 是「HTTP 200 但业务失败」的响应：**result 非空**且 error 非空。
//
// ⚠ result 必须非空。fetchHistory 的校验顺序是 `Chart.Error != nil` 在
// `len(Chart.Result) == 0` **之前**；若这里给空 result，短路掉 Error 校验的变异会被
// 后一条兜底分支拦下、照样报错，测试就测不出东西了（「变异被另一个更早的分支拦截」）。
const errorEnvelopeBody = `{"chart":{"result":[{"meta":{"symbol":"AAPL","regularMarketPrice":150,"chartPreviousClose":149,"regularMarketTime":1700000000},"timestamp":[1700000000],"indicators":{"quote":[{"open":[149],"high":[151],"low":[148],"close":[150],"volume":[1000]}]}}],"error":{"code":"Not Found","description":"No data found for symbol"}}}`

// TestErrorEnvelopeIsNotCached 覆盖 error_handling[0] 的「错误不写缓存」在 yahoo 侧
// 的那一半。
//
// **两个断言守的是两件事，缺一不可**：
//   - 「返回 error」证明 200-but-error 被识别成了失败；
//   - 「两次调用各发一次 HTTP」证明它**没有被缓存** —— 一个实现完全可以正确返回
//     error 却仍然把它写进缓存，那时前一条照样绿。
//
// policy 层的 TestGateDoesNotCacheErrors 只保证「fn 返回 error 时 Gate 不缓存」，
// 保证不了「yahoo 把 200-but-error 识别成 error」。**每一层都测了自己那部分，
// 层与层之间的契约没人测** —— 这条就是补那个缝。
func TestErrorEnvelopeIsNotCached(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK) // 注意：HTTP 层是成功的
		_, _ = w.Write([]byte(errorEnvelopeBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true}) // 有 TTL 才谈得上「被缓存」

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 2; i++ {
		if _, err := y.FetchHistory("AAPL", start, end, "1d"); err == nil {
			t.Errorf("第 %d 次: 200-but-error 信封必须识别为错误, got nil", i)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("错误不得写缓存，两次调用应各发一次 HTTP, got %d"+
			"（为 1 说明错误响应被当成功值缓存了——一次瞬时故障会变成整个 TTL 期的持续故障）", hits)
	}
}

// TestFetchEPSHistoryReturnsIndependentSlice 覆盖 boundary[0] 在 EPS 侧的那一半。
// DoD 只点名了 FetchHistory，但 FetchEPSHistory 同样返回缓存里的切片。
func TestFetchEPSHistoryReturnsIndependentSlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	first, err := y.FetchEPSHistory("AAPL", start, end)
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d points, %v)", len(first), err)
	}
	first[0].EPS = -999 // 调用方原地改写

	second, err := y.FetchEPSHistory("AAPL", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].EPS == -999 {
		t.Error("缓存命中必须返回独立副本，否则调用方能污染缓存")
	}
}

// ——— TASK-007 review_fix：缓存键时间精度的双向守护（两处 key 各一条）———
//
// 本包有**两处**独立的缓存键：yahoo.go:312 的 FetchHistory 与 eps.go:49 的
// FetchEPSHistory，各自需要独立实证 —— 一处修好不代表另一处也修好。
//
// **(a) 聚合度** —— 相邻时间必须落进同一槽。生产路径 `app.go:451` 的
// `end := time.Now()` 经 :462 传进来；键若保留秒级精度 ⇒ 每次调用键都不同 ⇒
// **命中率恒为零**。这是「静默的错」：不报错、不出错数据、返回值完全正确，
// 只有观测 HTTP 请求数才看得见。本包原有测试对它完全无感 —— 它们**全用固定时刻**
// （`time.Unix(1600000000,0)` 7 处），而固定时刻在 Truncate 前后相等。
//
// **(b) 粒度不得放粗** —— 只写 (a) 挡不住把 `Truncate(time.Minute)` 改成
// `Truncate(time.Hour)`：相邻时刻仍落在同一小时，(a) 照样通过。而放粗会让相隔
// 几分钟的不同区间串槽、静默返回错区间数据（「吵闹的错」）。
//
// ⚠ **偏移必须跨秒**：本包两处 key 都用 `.Unix()`（秒级）。照搬别包的毫秒级偏移
// （如 `base+50ms/+900ms`）会让去掉 Truncate 的变异**测不出来** —— `.Unix()` 把
// 亚秒差异归为同一秒，(a) 照样绿。我在 eastmoney 上踩过这一次。
// 基准取当前分钟的中点而非字面 `time.Now()`，避免跨分钟边界的偶发假红。

func TestFetchHistoryCacheKeyAggregatesNearbyTimes(t *testing.T) {
	start := time.Unix(1600000000, 0)
	base := time.Now().Truncate(time.Minute).Add(20 * time.Second)

	t.Run("相邻时间落进同一槽", func(t *testing.T) {
		srv, arrivals := countingServer(t)
		y := NewWithBaseURL(srv.URL)
		y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

		for _, end := range []time.Time{base, base.Add(3 * time.Second), base.Add(15 * time.Second)} {
			if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(arrivals()); n != 1 {
			t.Errorf("同一分钟内相隔数秒的三次调用应命中同一缓存槽: HTTP 请求 %d 次, want 1"+
				"（大于 1 说明键保留了秒级精度——生产以 time.Now() 为 end，命中率会恒为零）", n)
		}
	})

	t.Run("分钟粒度不得放粗", func(t *testing.T) {
		srv, arrivals := countingServer(t)
		y := NewWithBaseURL(srv.URL)
		y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

		for _, end := range []time.Time{base, base.Add(time.Minute)} {
			if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(arrivals()); n != 2 {
			t.Errorf("相隔 1 分钟的两次调用是不同区间，必须分槽: HTTP 请求 %d 次, want 2"+
				"（为 1 说明截断粒度被放粗到小时/天——不同区间会串槽，静默返回错区间数据）", n)
		}
	})
}

const epsCountingBody = `{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`

// epsCountingServer 是 countingServer 的 EPS 侧对应物：记录命中次数的 EPS 服务端。
// 聚合度与区分度两组用例共用。
func epsCountingServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(epsCountingBody))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

func TestFetchEPSHistoryCacheKeyAggregatesNearbyTimes(t *testing.T) {
	start := time.Unix(1600000000, 0)
	base := time.Now().Truncate(time.Minute).Add(20 * time.Second)

	t.Run("相邻时间落进同一槽", func(t *testing.T) {
		srv, hits := epsCountingServer(t)
		y := NewWithBaseURL(srv.URL)
		y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

		for _, end := range []time.Time{base, base.Add(3 * time.Second), base.Add(15 * time.Second)} {
			if _, err := y.FetchEPSHistory("AAPL", start, end); err != nil {
				t.Fatal(err)
			}
		}
		if n := hits(); n != 1 {
			t.Errorf("EPS 键同样须聚合到分钟: HTTP 请求 %d 次, want 1", n)
		}
	})

	t.Run("分钟粒度不得放粗", func(t *testing.T) {
		srv, hits := epsCountingServer(t)
		y := NewWithBaseURL(srv.URL)
		y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

		for _, end := range []time.Time{base, base.Add(time.Minute)} {
			if _, err := y.FetchEPSHistory("AAPL", start, end); err != nil {
				t.Fatal(err)
			}
		}
		if n := hits(); n != 2 {
			t.Errorf("EPS 键相隔 1 分钟须分槽: HTTP 请求 %d 次, want 2", n)
		}
	})
}

// ---------------------------------------------------------------------------
// fix_items[F2]：缓存键的**区分度** —— 键必须含全部影响结果的参数（契约陷阱 16）。
//
// 上面两组守护的是键的**聚合度/粒度**（相邻时刻要落同一槽、分钟粒度不得放粗），
// 方向是「该合的要合」；本组守护相反方向「该分的要分」。两个方向缺一不可 ——
// 实测把 yahoo.go 键里的 symbol 换成固定串后**全包测试仍然全绿**：AAPL 会拿到
// MSFT 的行情，静默错数据，而 yahoo 是美股/A 股主源。
//
// 判据统一为 **a → b → a 重放，期望 2 次 HTTP**，一条断言同时排除两种缺陷：
//   - 键漏掉该维度 ⇒ b 误命中 a 的槽 ⇒ 总数 1（本组要抓的）
//   - 压根没缓存   ⇒ 总数 3（只发 a、b 两次并断言 2 的写法对后者是假绿）
//
// 每个参数维度**各占独立一格**，不用一条测试凑合覆盖：变异只打掉键里的某一个
// 参数时，必须恰好只有对应那格转红，否则无从定位是哪个维度失守。
//
// ⚠ 时间维度的变体取**整分钟**偏移：键把 start/end 截断到分钟，亚分钟差异穿不过
// 截断，会得到假的「变异无效」（同上面那条跨秒偏移的坑，只是换了个精度单位）。
// ---------------------------------------------------------------------------

type histArgs struct {
	symbol   string
	start    time.Time
	end      time.Time
	interval string
}

func TestFetchHistoryCacheKeyDistinguishesParams(t *testing.T) {
	base := histArgs{"AAPL", time.Unix(1600000000, 0), time.Unix(1700086400, 0), "1d"}

	cases := []struct {
		name   string
		vary   func(*histArgs)
		impact string
	}{
		{"symbol", func(a *histArgs) { a.symbol = "MSFT" }, "不同标的共用一槽，会返回别的标的的行情"},
		// start 这一格顺带堵上聚合度那组的残留缺口：它的「粒度不得放粗」只变动
		// end，start 的 Truncate 被放粗到小时原先无人能抓。故此处取整分钟偏移，
		// 而不是随便找个不同的时刻。
		{"start", func(a *histArgs) { a.start = a.start.Add(time.Minute) }, "不同起点共用一槽，会返回别的区间"},
		{"end", func(a *histArgs) { a.end = a.end.Add(time.Minute) }, "不同终点共用一槽，会返回别的区间"},
		// interval 变体取 "1h" 而非随手写个 "1wk"：toYahooInterval 是 switch +
		// default 兜底成 "1d"，拿未知值当变体等于要求键区分两个**上游 URL 完全
		// 相同**的请求 —— 那样断言即使绿，守的也是一个并不存在的区分。
		{"interval", func(a *histArgs) { a.interval = "1h" }, "小时线与日线共用一槽，会返回错周期的数据"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, arrivals := countingServer(t)
			y := NewWithBaseURL(srv.URL)
			y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

			other := base
			tc.vary(&other)
			for _, a := range []histArgs{base, other, base} {
				if _, err := y.FetchHistory(a.symbol, a.start, a.end, a.interval); err != nil {
					t.Fatal(err)
				}
			}
			if n := len(arrivals()); n != 2 {
				t.Errorf("缓存键未区分 %s: HTTP 请求 %d 次, want 2"+
					"（为 1 说明该参数没进键 —— %s；为 3 说明缓存压根没生效）",
					tc.name, n, tc.impact)
			}
		})
	}
}

func TestFetchEPSHistoryCacheKeyDistinguishesParams(t *testing.T) {
	type epsArgs struct {
		symbol string
		start  time.Time
		end    time.Time
	}
	base := epsArgs{"AAPL", time.Unix(1600000000, 0), time.Unix(1700086400, 0)}

	cases := []struct {
		name   string
		vary   func(*epsArgs)
		impact string
	}{
		{"symbol", func(a *epsArgs) { a.symbol = "MSFT" }, "不同标的共用一槽，会返回别家的 EPS"},
		{"start", func(a *epsArgs) { a.start = a.start.Add(time.Minute) }, "不同起点共用一槽，会返回别的区间"},
		{"end", func(a *epsArgs) { a.end = a.end.Add(time.Minute) }, "不同终点共用一槽，会返回别的区间"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := epsCountingServer(t)
			y := NewWithBaseURL(srv.URL)
			y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

			other := base
			tc.vary(&other)
			for _, a := range []epsArgs{base, other, base} {
				if _, err := y.FetchEPSHistory(a.symbol, a.start, a.end); err != nil {
					t.Fatal(err)
				}
			}
			if n := hits(); n != 2 {
				t.Errorf("EPS 缓存键未区分 %s: HTTP 请求 %d 次, want 2"+
					"（为 1 说明该参数没进键 —— %s；为 3 说明缓存压根没生效）",
					tc.name, n, tc.impact)
			}
		})
	}
}
