package eastmoney

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
//                       → TestFetchHistoryCachesViaGate（断言请求次数，陷阱 2）
// functional[1]     缓存 key 覆盖 symbol/区间/interval，两组参数互不命中
//                       → TestCacheKeyCoversAllParams（a→b→a 重放，陷阱 16）
// functional[2]     构造函数取 policy.Default()
//                       → TestNewUsesDefaultGate（陷阱 12）
// functional[3]     主题常量与内置表一致
//                       → TestTopicMatchesBuiltinTable（陷阱 15）
// boundary[0]       不被节流（本任务只补缓存）
//                       → TestNotThrottled（直接观测耗时 + 对照组，陷阱 8）
// boundary[1]       缓存命中返回独立切片
//                       → TestFetchHistoryReturnsIndependentSlice（§T2）
// boundary[2]       既有测试互不串味
//                       → 由本文件的 TestMain 装零策略闸门承接（陷阱 13）
// error_handling[0] 错误不写缓存，校验留在 fn 内部
//                       → TestErrorResponseIsNotCached（决定性断言是 HTTP 次数，陷阱 14）

// TestMain 把进程默认闸门换成零策略。
//
// ⚠ 这不是为了跑得快，是**必须的**：既有 21 个用例里，成功路径（:209）与「应报错」
// 路径（:239 / :261）共用 `600519.SH` + 同一区间 ⇒ 接入缓存后它们是同一个缓存槽，
// 先跑的成功响应会让后跑的错误用例命中缓存而静默失效（契约陷阱 13）。
// 需要真闸门的用例自己给 e.gate 赋值。
func TestMain(m *testing.M) {
	policy.SetDefault(policy.New(zeroPolicyTable(), nil))
	os.Exit(m.Run())
}

// zeroPolicyTable 返回一张把 eastmoney 主题登记为零策略的表（不缓存、不节流）。
func zeroPolicyTable() *policy.Table {
	tbl := policy.NewTable()
	tbl.Set(topicHistory, policy.Policy{})
	return tbl
}

// cachingGate 返回带 TTL 的闸门；用**内置表**而非手搓策略，这样测试跑的是生产查表路径。
func cachingGate() *policy.Gate {
	return policy.New(policy.NewTable(), nil)
}

// klineBody 是 FetchHistory 可解析的最小合法响应。
const klineBody = `{"data":{"code":"600519","name":"x","klines":["2023-11-14,1800.00,1810.00,1820.00,1790.00,12345,1000000,1.5"]}}`

// emptyKlineBody 是「HTTP 200 但业务失败」的响应：data 存在但 klines 为空。
const emptyKlineBody = `{"data":{"code":"600519","name":"x","klines":[]}}`

func countingServer(t *testing.T, body string) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

func newWithHistoryURL(t *testing.T, url string) *Eastmoney {
	t.Helper()
	e := New()
	e.historyURL = url
	return e
}

func TestFetchHistoryCachesViaGate(t *testing.T) {
	srv, hits := countingServer(t, klineBody)
	e := newWithHistoryURL(t, srv.URL)
	e.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 3; i++ {
		if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// 断言请求次数而非返回值：Fetch 在类型断言失败时会「当作未命中重取」，
	// 返回值仍然正确，断言返回值的写法永远绿（契约陷阱 2）。
	if n := hits(); n != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", n)
	}
}

// TestCacheKeyCoversAllParams 用 **a → b → a** 重放序列验证缓存键。
//
// 只用一组参数的话，键里丢掉 symbol / 区间 / interval 中任何一个都照样全绿，
// 而生产上会拿到**别的标的**的数据——静默错数据比缓存没生效危险得多（陷阱 16）。
// 正确实现下总请求数为 2（a 发一次、b 发一次、重放的 a 命中缓存）；若键不区分该
// 参数，b 会直接**命中** a 的槽、三次挤在一起 ⇒ 只发 1 次，且 b 拿到的是 a 的数据。
func TestCacheKeyCoversAllParams(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	otherEnd := end.Add(48 * time.Hour)

	cases := []struct {
		name             string
		symbolB, intervB string
		endB             time.Time
	}{
		{"symbol", "000001.SZ", "1d", end},
		{"interval", "600519.SH", "1wk", end},
		{"区间", "600519.SH", "1d", otherEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := countingServer(t, klineBody)
			e := newWithHistoryURL(t, srv.URL)
			e.gate = cachingGate()

			if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
			if _, err := e.FetchHistory(tc.symbolB, start, tc.endB, tc.intervB); err != nil {
				t.Fatal(err)
			}
			if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}

			if n := hits(); n != 2 {
				t.Errorf("缓存键未区分 %s: HTTP 请求 %d 次, want 2"+
					"（为 1 说明三次调用挤在同一个缓存槽里——第二组参数直接命中了第一组的结果，"+
					"生产上就是拿到别的标的/区间/周期的数据）",
					tc.name, n)
			}
		})
	}
}

// TestNewUsesDefaultGate 覆盖 functional[2]（陷阱 12）。
//
// 本包其余用例各自给 e.gate 赋值，而 nil *Gate 是透明直通的 ⇒ 构造函数漏掉
// `gate: policy.Default()` 时它们全都照常绿，生产路径缓存整体静默 no-op。
// 判据用指针相等，不用行为推断。
func TestNewUsesDefaultGate(t *testing.T) {
	orig := policy.Default()
	t.Cleanup(func() { policy.SetDefault(orig) })

	marker := cachingGate()
	policy.SetDefault(marker)

	if e := New(); e.gate != marker {
		t.Error("New 必须把 policy.Default() 存进 gate 字段；" +
			"为 nil 时 Gate 透明直通，缓存整体静默失效")
	}
}

// TestTopicMatchesBuiltinTable 覆盖 functional[3]（陷阱 15）。
//
// 测试里若用同一个常量既登记又查询，常量写错也自洽；生产走**内置表**则 Lookup
// 落空、Gate 直通。所以要拿常量去查真正的 NewTable()。
func TestTopicMatchesBuiltinTable(t *testing.T) {
	p, ok := policy.NewTable().Lookup(topicHistory)
	if !ok {
		t.Fatalf("主题常量 %q 在内置表中查不到——生产路径会退化为无策略直通", topicHistory)
	}
	if p.TTL <= 0 {
		t.Errorf("%s 必须有 TTL（本任务修复的正是缓存丢失）, got %v", topicHistory, p.TTL)
	}
	if p.MinInterval != 0 || p.Quota != nil {
		t.Errorf("%s 只应有缓存，不得有限流/配额: MinInterval=%v Quota=%v",
			topicHistory, p.MinInterval, p.Quota)
	}
}

// TestNotThrottled 覆盖 boundary[0]：本任务只补缓存，不新增节流。
//
// 否定断言直接观测耗时，并带一个 MinInterval=200ms 的**对照组**证明这套计时确实
// 能测出节流——否则「耗时很短」既可能是没节流，也可能是计时方式根本测不到（陷阱 8）。
func TestNotThrottled(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	measure := func(t *testing.T, tbl *policy.Table) time.Duration {
		t.Helper()
		srv, _ := countingServer(t, klineBody)
		e := newWithHistoryURL(t, srv.URL)
		e.gate = policy.New(tbl, nil)
		t0 := time.Now()
		for i := 0; i < 3; i++ {
			// 每次换区间避开缓存，确保 3 次都真的发请求
			if _, err := e.FetchHistory("600519.SH", start, end.Add(time.Duration(i)*time.Hour), "1d"); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(t0)
	}

	// 对照组：显式给 200ms 间隔，证明这套计时能测出节流
	throttled := policy.NewTable()
	throttled.Set(topicHistory, policy.Policy{MinInterval: 200 * time.Millisecond})
	if d := measure(t, throttled); d < 300*time.Millisecond {
		t.Fatalf("对照组耗时 %v，说明这套计时测不出节流，下面的断言无意义", d)
	}

	// 实际组：内置表，应当没有任何节流
	if d := measure(t, policy.NewTable()); d > 150*time.Millisecond {
		t.Errorf("内置策略下 3 次请求耗时 %v——eastmoney 不应被节流（本任务只补缓存）", d)
	}
}

func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv, _ := countingServer(t, klineBody)
	e := newWithHistoryURL(t, srv.URL)
	e.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	first, err := e.FetchHistory("600519.SH", start, end, "1d")
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bars, %v)", len(first), err)
	}
	first[0].Close = -999 // 调用方原地改写

	second, err := e.FetchHistory("600519.SH", start, end, "1d")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Close == -999 {
		t.Error("缓存命中必须返回独立副本，否则调用方能污染缓存")
	}
}

// TestErrorResponseIsNotCached 覆盖 error_handling[0]（陷阱 14）。
//
// eastmoney 的失败是「HTTP 200 + data.klines 为空」，校验在 fetchStockHistory 内部。
// 若把它挪到 Fetch 外面，fn 会返回成功值 ⇒ 错误响应被缓存 ⇒ 一次瞬时故障变成整个
// TTL 期的持续故障。
//
// **两个断言守的是两件事**：「返回 error」证明失败被识别；「两次调用各发一次 HTTP」
// 证明它没被缓存——一个实现完全可以正确返回 error 却仍然把它写进缓存。
func TestErrorResponseIsNotCached(t *testing.T) {
	srv, hits := countingServer(t, emptyKlineBody)
	e := newWithHistoryURL(t, srv.URL)
	e.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 2; i++ {
		if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err == nil {
			t.Errorf("第 %d 次: 空 klines 必须识别为错误, got nil", i)
		}
	}
	if n := hits(); n != 2 {
		t.Errorf("错误不得写缓存，两次调用应各发一次 HTTP, got %d"+
			"（为 1 说明错误响应被当成功值缓存了）", n)
	}
}
