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
