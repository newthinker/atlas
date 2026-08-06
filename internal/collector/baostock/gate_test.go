package baostock

import (
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
// functional[1]     缓存 key 覆盖影响结果的参数，两组参数互不命中
//                       → TestCacheKeyCoversParams（a→b→a 重放，陷阱 16）
//                       + TestIntervalDoesNotSplitSlot（interval **不进 key** 的理由，见其注释）
// functional[2]     构造函数取 policy.Default()
//                       → TestNewUsesDefaultGate（陷阱 12）
// functional[3]     主题**域段**与内置表一致（通配登记下只有域段可测）
//                       → TestTopicDomainMatchesBuiltinTable
// boundary[0]       不被节流
//                       → TestNotThrottled（直接观测 + 对照组，陷阱 8）
// boundary[1]       缓存命中返回独立切片
//                       → TestFetchHistoryReturnsIndependentSlice（§T2）
// boundary[2]       既有测试互不串味
//                       → 由本文件 TestMain 装零策略闸门承接（陷阱 13）
// error_handling[0] 错误不写缓存
//                       → TestErrorIsNotCached（决定性断言是 HTTP 次数）
// error_handling[0] policy 错误不外泄
//                       → TestPolicyErrorsDoNotLeak
//
// ⚠ 本包**没有「HTTP 200 但响应体表示失败」的形态**：FetchDaily 对空数组返回
//   空切片 + nil error，业务失败只经 HTTP 状态码表达。故陷阱 14 那条「校验必须
//   留在 fn 内部」在此无对应风险面，不硬造测试（一条永远绿的测试比一条注释更糟，
//   它占着「已守护」的名分）。错误不写缓存这一面仍由 TestErrorIsNotCached 守护。

// TestMain 把进程默认闸门换成零策略。
//
// ⚠ 必须：collector_test.go 的 TestCollectorFetchHistory（成功）与「桥不可达应报错」
// 用例都走 FetchHistory 且共用 `600519.SH`；两个 Client 实例不同但 **gate 同为
// policy.Default()**，缓存是共享的 ⇒ 先跑的成功响应会让后跑的错误用例命中缓存而
// 静默失效（契约陷阱 13）。
func TestMain(m *testing.M) {
	tbl := policy.NewTable()
	tbl.Set(topicDaily, policy.Policy{})
	policy.SetDefault(policy.New(tbl, nil))
	os.Exit(m.Run())
}

// cachingGate 用**内置表**而非手搓策略，让测试跑生产查表路径。
func cachingGate() *policy.Gate { return policy.New(policy.NewTable(), nil) }

const dailyBody = `[{"date":"2023-11-14","close":1800.5}]`

func countingBridge(t *testing.T, status int, body string) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return hits }
}

func TestFetchHistoryCachesViaGate(t *testing.T) {
	srv, hits := countingBridge(t, http.StatusOK, dailyBody)
	c := New(srv.URL)
	c.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 3; i++ {
		if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := hits(); n != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", n)
	}
}

// TestCacheKeyCoversParams 用 a → b → a 重放验证键。
//
// 正确实现下总请求数为 2（a 发一次、b 发一次、重放的 a 命中缓存）；若键不区分该
// 参数，b 会直接**命中** a 的槽、三次挤在一起 ⇒ 只发 1 次，且 b 拿到 a 的数据。
func TestCacheKeyCoversParams(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	cases := []struct {
		name    string
		symbolB string
		endB    time.Time
	}{
		{"symbol", "000001.SZ", end},
		{"区间", "600519.SH", end.Add(48 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := countingBridge(t, http.StatusOK, dailyBody)
			c := New(srv.URL)
			c.gate = cachingGate()

			if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.FetchHistory(tc.symbolB, start, tc.endB, "1d"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
				t.Fatal(err)
			}
			if n := hits(); n != 2 {
				t.Errorf("缓存键未区分 %s: HTTP 请求 %d 次, want 2"+
					"（为 1 说明第二组参数命中了第一组的槽——生产上会拿到别的标的/区间的数据）",
					tc.name, n)
			}
		})
	}
}

// TestIntervalDoesNotSplitSlot 说明并守护一个**有意的决定**：interval 不进缓存键。
//
// FetchHistory 只接受 "" 与 "1d"（其余值在进 Gate 之前就被拒），二者语义相同、
// 取到的是同一份日线 ⇒ interval **不影响结果**，放进键只会让两个等价调用占两个槽。
// DoD functional[1] 要求「键覆盖全部**影响结果**的参数」，本包的 interval 不在其列。
//
// 这条同时是那个决定的守护：若将来桥支持了别的粒度，本测试会因为两次调用返回不同
// 数据而失去意义——那时必须把 interval 加进键，并把这条测试改成区分槽的断言。
func TestIntervalDoesNotSplitSlot(t *testing.T) {
	srv, hits := countingBridge(t, http.StatusOK, dailyBody)
	c := New(srv.URL)
	c.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	if _, err := c.FetchHistory("600519.SH", start, end, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
		t.Fatal(err)
	}
	if n := hits(); n != 1 {
		t.Errorf(`"" 与 "1d" 语义等价，应命中同一缓存槽: HTTP 请求 %d 次, want 1`, n)
	}

	// 其余粒度必须在进 Gate 之前就被拒——否则它们会各占一个缓存槽却取到日线数据
	if _, err := c.FetchHistory("600519.SH", start, end, "1wk"); err == nil {
		t.Error(`interval "1wk" 应被拒（桥只有日线）`)
	}
}

// TestNewUsesDefaultGate 覆盖 functional[2]（陷阱 12）。
func TestNewUsesDefaultGate(t *testing.T) {
	orig := policy.Default()
	t.Cleanup(func() { policy.SetDefault(orig) })

	marker := cachingGate()
	policy.SetDefault(marker)

	if c := New("http://x"); c.gate != marker {
		t.Error("New 必须把 policy.Default() 存进 gate 字段；" +
			"为 nil 时 Gate 透明直通，缓存整体静默失效")
	}
}

// TestTopicDomainMatchesBuiltinTable 覆盖 functional[3]。
//
// ⚠ 三家均为**通配登记**（`<域>.*`），Lookup 的通配回退让 `baostock.任意后缀` 都能
// 命中同一条策略 ⇒ **接口段写错测不出东西**（QA 实证）。真正可测的是**域段**：
// 写错域名才会落空、Gate 退化为直通。
func TestTopicDomainMatchesBuiltinTable(t *testing.T) {
	domain, _, ok := strings.Cut(topicDaily, ".")
	if !ok || domain == "" {
		t.Fatalf("主题常量 %q 应为 <域>.<接口> 形式", topicDaily)
	}

	p, hit := policy.NewTable().Lookup(topicDaily)
	if !hit {
		t.Fatalf("主题常量 %q 在内置表中查不到——生产路径会退化为无策略直通", topicDaily)
	}
	if p.Domain != domain {
		t.Errorf("Lookup 返回的 Domain = %q, want %q（域段写错会让限流域串味）", p.Domain, domain)
	}
	if p.TTL <= 0 {
		t.Errorf("%s 必须有 TTL（本任务修复的正是缓存丢失）, got %v", topicDaily, p.TTL)
	}
	if p.MinInterval != 0 || p.Quota != nil {
		t.Errorf("%s 只应有缓存，不得有限流/配额: MinInterval=%v Quota=%v",
			topicDaily, p.MinInterval, p.Quota)
	}
}

// TestNotThrottled 覆盖 boundary[0]，带对照组证明这套计时确实能测出节流（陷阱 8）。
func TestNotThrottled(t *testing.T) {
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	measure := func(t *testing.T, tbl *policy.Table) time.Duration {
		t.Helper()
		srv, _ := countingBridge(t, http.StatusOK, dailyBody)
		c := New(srv.URL)
		c.gate = policy.New(tbl, nil)
		t0 := time.Now()
		for i := 0; i < 3; i++ {
			// 每次换区间避开缓存，确保 3 次都真的发请求
			if _, err := c.FetchHistory("600519.SH", start, end.Add(time.Duration(i)*time.Hour), "1d"); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(t0)
	}

	throttled := policy.NewTable()
	throttled.Set(topicDaily, policy.Policy{MinInterval: 200 * time.Millisecond})
	if d := measure(t, throttled); d < 300*time.Millisecond {
		t.Fatalf("对照组耗时 %v，说明这套计时测不出节流，下面的断言无意义", d)
	}

	if d := measure(t, policy.NewTable()); d > 150*time.Millisecond {
		t.Errorf("内置策略下 3 次请求耗时 %v——baostock 不应被节流（本任务只补缓存）", d)
	}
}

func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv, _ := countingBridge(t, http.StatusOK, dailyBody)
	c := New(srv.URL)
	c.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	first, err := c.FetchHistory("600519.SH", start, end, "1d")
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bars, %v)", len(first), err)
	}
	first[0].Close = -999

	second, err := c.FetchHistory("600519.SH", start, end, "1d")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Close == -999 {
		t.Error("缓存命中必须返回独立副本，否则调用方能污染缓存")
	}
}

// TestErrorIsNotCached：桥返回 5xx 时两次调用应各发一次请求。
func TestErrorIsNotCached(t *testing.T) {
	srv, hits := countingBridge(t, http.StatusInternalServerError, "bridge down")
	c := New(srv.URL)
	c.gate = cachingGate()

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 2; i++ {
		if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err == nil {
			t.Errorf("第 %d 次: 桥 5xx 必须报错, got nil", i)
		}
	}
	if n := hits(); n != 2 {
		t.Errorf("错误不得写缓存，两次调用应各发一次 HTTP, got %d", n)
	}
}

// TestPolicyErrorsDoNotLeak 覆盖 error_handling[0] 的后半：policy 包错误不得出现在
// 调用方可见的错误链上。
//
// 判据是 `errors.Is` **不成立**——只检查错误消息文本会被「%w 包一层前缀」骗过：
// 那种写法消息变了但链还在，上层 errors.Is(err, policy.ErrQuotaExceeded) 照样为真。
//
// 配额与超时都是**临时性**错误，映射后的消息也必须体现这一点，不得暗示永久失败
// （否则运维会照着去改配置，而问题在于等窗口）。
func TestPolicyErrorsDoNotLeak(t *testing.T) {
	srv, _ := countingBridge(t, http.StatusOK, dailyBody)
	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)

	t.Run("配额耗尽", func(t *testing.T) {
		tbl := policy.NewTable()
		tbl.Set(topicDaily, policy.Policy{
			Quota: &policy.Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC},
		})
		c := New(srv.URL)
		c.gate = policy.New(tbl, policy.NewMemStore())

		if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
			t.Fatalf("首次应放行: %v", err)
		}
		_, err := c.FetchHistory("600519.SH", start, end.Add(time.Hour), "1d")
		if err == nil {
			t.Fatal("配额耗尽后应报错")
		}
		if errors.Is(err, policy.ErrQuotaExceeded) {
			t.Errorf("policy.ErrQuotaExceeded 泄漏到调用方错误链: %v", err)
		}
		if !strings.Contains(err.Error(), "baostock") {
			t.Errorf("映射后的错误应带本包前缀, got %v", err)
		}
	})

	t.Run("超时", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(dailyBody))
		}))
		t.Cleanup(slow.Close)

		tbl := policy.NewTable()
		tbl.Set(topicDaily, policy.Policy{Timeout: 20 * time.Millisecond})
		c := New(slow.URL)
		c.gate = policy.New(tbl, nil)

		_, err := c.FetchHistory("600519.SH", start, end, "1d")
		if err == nil {
			t.Fatal("超时应报错")
		}
		if errors.Is(err, policy.ErrTimeout) {
			t.Errorf("policy.ErrTimeout 泄漏到调用方错误链: %v", err)
		}
		if !strings.Contains(err.Error(), "baostock") {
			t.Errorf("映射后的错误应带本包前缀, got %v", err)
		}
	})
}
