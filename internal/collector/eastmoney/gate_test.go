package eastmoney

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// TestPolicyErrorsDoNotLeak 覆盖 error_handling[1]：policy 包错误不得出现在调用方
// 可见的错误链上（QA 前瞻：三家新接入若只包 Fetch 而不处理，config 可达的泄漏点
// 会从 1 处变 4 处）。
//
// ⚠ **判据是 `errors.Is` 不成立，不是检查错误消息文本** —— 只查文本会被
// `fmt.Errorf("eastmoney: ...: %w", err)` 这种写法骗过：消息变了、链还在，上层
// `errors.Is(err, policy.ErrQuotaExceeded)` 照样为真（TASK-017 的变异 B8 实证）。
//
// ⚠ **ErrTimeout 不需要配额就能触发**：任何主题配上 Timeout 即可，而
// `collector.topics.*.timeout` 是 TASK-005/006 已落地的可配置项（Override.Timeout），
// **今天就能被配出来** —— 这不是「不可达所以测不出」。
//
// 映射后的消息必须体现**临时性**：配额与超时都是窗口过后/下次调用即可自愈的错误，
// 不得暗示永久失败，更不得映射成「无此标的」那类 —— 那会让调用方停止重试。
func TestPolicyErrorsDoNotLeak(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	t.Run("配额耗尽", func(t *testing.T) {
		srv, _ := countingServer(t, klineBody)
		tbl := policy.NewTable()
		tbl.Set(topicHistory, policy.Policy{
			Quota: &policy.Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC},
		})
		e := newWithHistoryURL(t, srv.URL)
		e.gate = policy.New(tbl, policy.NewMemStore())

		if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
			t.Fatalf("首次应放行: %v", err)
		}
		_, err := e.FetchHistory("600519.SH", start, end.Add(time.Hour), "1d")
		if err == nil {
			t.Fatal("配额耗尽后应报错")
		}
		if errors.Is(err, policy.ErrQuotaExceeded) {
			t.Errorf("policy.ErrQuotaExceeded 泄漏到调用方错误链: %v", err)
		}
		if !strings.Contains(err.Error(), "eastmoney") {
			t.Errorf("映射后的错误应带本包前缀, got %v", err)
		}
		if !strings.Contains(err.Error(), "临时") {
			t.Errorf("配额是临时性错误，消息须体现（否则调用方会停止重试）, got %v", err)
		}
	})

	t.Run("超时", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(klineBody))
		}))
		t.Cleanup(slow.Close)

		tbl := policy.NewTable()
		tbl.Set(topicHistory, policy.Policy{Timeout: 20 * time.Millisecond})
		e := newWithHistoryURL(t, slow.URL)
		e.gate = policy.New(tbl, nil)

		_, err := e.FetchHistory("600519.SH", start, end, "1d")
		if err == nil {
			t.Fatal("超时应报错")
		}
		if errors.Is(err, policy.ErrTimeout) {
			t.Errorf("policy.ErrTimeout 泄漏到调用方错误链: %v", err)
		}
		if !strings.Contains(err.Error(), "eastmoney") {
			t.Errorf("映射后的错误应带本包前缀, got %v", err)
		}
		if !strings.Contains(err.Error(), "临时") {
			t.Errorf("超时是临时性错误，消息须体现, got %v", err)
		}
	})

	t.Run("非policy错误原样透传", func(t *testing.T) {
		// 桥返回 5xx：错误应保留原始信息，不被映射层吞掉
		srv, _ := countingServer(t, emptyKlineBody)
		e := newWithHistoryURL(t, srv.URL)
		e.gate = cachingGate()

		_, err := e.FetchHistory("600519.SH", start, end, "1d")
		if err == nil {
			t.Fatal("空 klines 应报错")
		}
		if !strings.Contains(err.Error(), "no history") {
			t.Errorf("非 policy 错误应原样透传，保留原始错误链, got %v", err)
		}
	})
}

// TestCacheKeyAggregatesNearbyTimes 覆盖 functional[4]：缓存键的时间精度**双向约束**。
//
// **(a) 聚合度** —— 相邻时间必须落进同一槽。生产路径 `app.go:451` 的 `end := time.Now()`
// 经 :462 传进 FetchHistory；键若保留秒/纳秒精度，每次调用键都不同 ⇒ **命中率恒为零**。
// 这是「静默的错」：不报错、不出错数据、返回值完全正确，只有生产路径才暴露。而本包
// 原有 8 个测试对它完全无感，因为它们**全用固定时刻**，固定时刻在 Truncate 前后相等。
//
// **(b) 粒度不得放粗** —— 只写 (a) 挡不住把 `Truncate(time.Minute)` 改成 `Truncate(time.Hour)`：
// `base` 与 `base+15s` 仍落在同一小时，(a) 照样通过。而放粗会让相隔几分钟的不同区间
// 串槽、静默返回错数据（「吵闹的错」）。
//
// ⚠ 写法两处：①**不字面用两次 `time.Now()`** —— 跨分钟边界会偶发假红，取当前分钟的中点
// 作基准；②**偏移必须跨秒**（本包 key 用 `.Unix()`，秒级）—— 首版照搬了别包的 50ms/900ms
// 偏移，去掉 Truncate 后 `.Unix()` 仍把它们归为同一秒，(a) 照样绿、变异测不出。
// **照搬别包的测试形态时要检查它与本包 key 的精度单位是否匹配。**
//
// ⚠ 自我记录：`FetchHistory` 的注释里我本来就写了「让上层以 time.Now() 为 end 的抖动
// 仍能命中同一槽」——**我知道这个行为、还写下来了，却没有为它写测试**。这正是契约
// 教训 5 的形态：注释描述的究竟是「当前巧合」还是「被守护的契约」，写注释的人自己
// 最容易分不清。
func TestCacheKeyAggregatesNearbyTimes(t *testing.T) {
	start := time.Unix(1600000000, 0)
	base := time.Now().Truncate(time.Minute).Add(20 * time.Second) // 当前分钟的中点

	t.Run("相邻时间落进同一槽", func(t *testing.T) {
		srv, hits := countingServer(t, klineBody)
		e := newWithHistoryURL(t, srv.URL)
		e.gate = cachingGate()

		for _, end := range []time.Time{base, base.Add(3 * time.Second), base.Add(15 * time.Second)} {
			if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
		}
		if n := hits(); n != 1 {
			t.Errorf("同一分钟内相隔数秒的三次调用应命中同一缓存槽: HTTP 请求 %d 次, want 1"+
				"（大于 1 说明键保留了秒/纳秒精度——生产路径以 time.Now() 为 end，命中率会恒为零）", n)
		}
	})

	t.Run("分钟粒度不得放粗", func(t *testing.T) {
		srv, hits := countingServer(t, klineBody)
		e := newWithHistoryURL(t, srv.URL)
		e.gate = cachingGate()

		for _, end := range []time.Time{base, base.Add(time.Minute)} {
			if _, err := e.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
		}
		if n := hits(); n != 2 {
			t.Errorf("相隔 1 分钟的两次调用是不同区间，必须分槽: HTTP 请求 %d 次, want 2"+
				"（为 1 说明截断粒度被放粗到小时/天——不同区间的查询会串槽，静默返回错区间数据）", n)
		}
	})
}

// TestNonPolicyErrorChainPreserved 守护「非 policy 错误的**链**保留」。
//
// 上面 TestPolicyErrorsDoNotLeak 的「非policy错误原样透传」那一格挡不住这一面：
// 它用 emptyKlineBody，错误是 `no history for symbol: ...`——**纯 fmt.Errorf，
// 本来就没有链**，判据又是文本包含。于是对抗变异（调用点多包一层 `%v`、映射函数
// 一个字不动）下它照常绿。
//
// 对抗变异形态（test-agent-17 在 TASK-018 上造出来的）：
//
//	return nil, mapPolicyError(symbol, err)
//	  → return nil, fmt.Errorf("eastmoney: %v", mapPolicyError(symbol, err))
//
// `%v` 只切链、不改文本，所以「文本里还有没有原判别词」这类判据一律看不见它。
// ⇒ 无哨兵错误的包必须**显式对某个上游可判定错误写 errors.As/errors.Is**，
// 否则「链保留」这个属性没有任何东西钉住。
//
// 判据取 `*json.SyntaxError` 而非 `io.ErrUnexpectedEOF`：两者都对但**互斥**，由 body
// 形态唯一决定——`{not json` 这类非法字符 body 走 SyntaxError，`{"data":` 这类截断
// body 才走 ErrUnexpectedEOF。已现读实测本包 `json.NewDecoder(...).Decode` 对该 body
// 的实际返回类型，不照搬别包（test-agent-17 在 lixinger 照搬 yahoo 的判据，基线就红）。
func TestNonPolicyErrorChainPreserved(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	srv, _ := countingServer(t, `{not json`)
	e := newWithHistoryURL(t, srv.URL)
	e.gate = cachingGate()

	_, err := e.FetchHistory("600519.SH", start, end, "1d")
	if err == nil {
		t.Fatal("畸形 JSON 必须返回错误 —— 本轮未构成检验")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("链被切断: 上游解码错误必须能被 errors.As 穿透到 *json.SyntaxError\n"+
			"  got: %v (%T)\n"+
			"  调用点若用 %%v 多包一层（而非 %%w），文本一字不变、映射函数也没改，"+
			"但上层再也无法按类型判别上游错误", err, err)
	}
}
