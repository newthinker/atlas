package twelvedata

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// Context Checkpoint: done_criteria → test mapping (TASK-008 twelvedata 接入 Gate):
//   functional[0]     "FetchHistory 经 Gate 受 8s 最小间隔节流,相邻请求间隔符合策略"
//                                                              → TestFetchHistoryThrottledByGate
//   functional[0] 补  "8s 数值 + 主题名与内置策略表对得上"      → TestTopicMatchesBuiltinPolicy
//   functional[1]     "经 Gate 走 TTL 缓存,同参数重复调用只发一次 HTTP"
//                                                              → TestFetchHistoryCachedByGate
//   functional[1] 补  "缓存键覆盖 symbol/start/end,换参数必须另发"
//                                                              → TestFetchHistoryCacheKeyCoversAllParams
//   functional[2]     "Client 持有 gate *policy.Gate,构造函数取 policy.Default(),包内可注入"
//                                                              → TestNewSnapshotsDefaultGate
//   boundary[0]       "缓存命中时返回独立切片,调用方修改不污染缓存"
//                                                              → TestFetchHistoryReturnsIndependentSlice
//   error_handling[0] 分句 2 "错误不写缓存"                     → TestFetchHistoryErrorNotCached
//   error_handling[0] 分句 1 "既有错误路径行为不变"             → client_test.go 既有用例(断言体未改)
//   non_functional[0] "包源码中不再有 lastReq / minInterval / throttle 私有节流状态"
//                     (verify_by: review;本文件里那个 80ms 的 const 刻意命名为
//                      gateInterval 而非 minInterval,免得 TASK-013 的 AST 断言误伤)
//                                                              → 由 TASK-013 的 AST 断言统一守护
//   non_functional[1] "-race 全绿 + 覆盖率不低于 92%"           → CI/交付报告

// testDefaultGate 是 TestMain 装上的零策略闸门。凡是临时 SetDefault 的用例,
// t.Cleanup 必须恢复到**它**而不是 nil —— SetDefault(nil) 后 Default() 会懒构造
// 内置表闸门(twelvedata.time_series = 8s),后续用例每次 FetchHistory 白等 8s。
var testDefaultGate *policy.Gate

// TestMain 把默认闸门换成零间隔零 TTL:生产策略是 8s/次,会让整包用例挂死。
func TestMain(m *testing.M) {
	testDefaultGate = gateWith(policy.Policy{})
	policy.SetDefault(testDefaultGate)
	os.Exit(m.Run())
}

func gateWith(p policy.Policy) *policy.Gate {
	tbl := policy.NewTable()
	p.Domain = "twelvedata"
	tbl.Set(topicTimeSeries, p)
	return policy.New(tbl, nil)
}

const gateSeriesBody = `{"status":"ok","values":[{"datetime":"2026-08-03","close":"101.5"}]}`

// probeServer 记录每次请求的**到达时刻**。用到达时刻而非「调用方总耗时」是因为
// functional[0] 说的是「相邻请求间隔符合策略」—— 那是服务端可直接观测的量,
// 而总耗时还掺着解析/建连开销,是间接现象(陷阱 11 附:别用间接现象推断)。
func probeServer(t *testing.T, body string) (*httptest.Server, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []time.Time { mu.Lock(); defer mu.Unlock(); return append([]time.Time(nil), at...) }
}

func recentRange() (time.Time, time.Time) {
	return time.Now().AddDate(0, 0, -5), time.Now()
}

// functional[0]:节流由 Gate 承接。两次请求用**不同 symbol**,即便本用例 TTL=0
// 也不会有人误以为第二次是缓存命中。
func TestFetchHistoryThrottledByGate(t *testing.T) {
	const gateInterval = 80 * time.Millisecond
	srv, arrivals := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{MinInterval: gateInterval})

	start, end := recentRange()
	for _, sym := range []string{"NVDA", "AAPL"} {
		if _, err := c.FetchHistory(sym, start, end); err != nil {
			t.Fatalf("FetchHistory(%s): %v", sym, err)
		}
	}

	at := arrivals()
	if len(at) != 2 {
		t.Fatalf("服务端收到 %d 次请求, want 2", len(at))
	}
	// time.Sleep 保证不短于给定时长;留 10ms 余量只为吸收计时器粒度,
	// 未接闸门时该间隔是亚毫秒级,判据依然锋利。
	if gap := at[1].Sub(at[0]); gap < gateInterval-10*time.Millisecond {
		t.Errorf("相邻请求间隔 %v, want >= %v", gap, gateInterval-10*time.Millisecond)
	}
}

// functional[1]:TTL 内同参数只发一次 HTTP。**判据必须是请求次数而非返回值**——
// fetch 的类型断言失败是静默降级成「重取一次」,返回值照样正确(契约陷阱 2)。
func TestFetchHistoryCachedByGate(t *testing.T) {
	srv, arrivals := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := recentRange()
	for i := 0; i < 3; i++ {
		pts, err := c.FetchHistory("NVDA", start, end)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		// singleflight 返回处的断言失败会**静默返回零值 + nil error**(契约陷阱 2),
		// 只断言 err==nil 漏得掉;逐次钉住实际取值。
		if len(pts) != 1 || pts[0].Close != 101.5 {
			t.Fatalf("call %d: 返回 %d 条 %v, want 1 条 close=101.5", i, len(pts), pts)
		}
	}
	if n := len(arrivals()); n != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", n)
	}
}

// functional[1] 的另一半:缓存键必须覆盖 symbol / start / end 三个参数。
//
// 为什么单独一条:上面那个「同参数只发一次」的用例**测不出键少了参数**——
// 键里丢掉 symbol 时,它自己只用一个 symbol,照样 1 次请求全绿;而生产上
// AAPL 会拿到 NVDA 的收盘价,是静默的**错数据**,比缓存没生效危险得多。
// 判据取请求次数(契约陷阱 2:类型断言失败是静默的,断言返回值永远绿)。
func TestFetchHistoryCacheKeyCoversAllParams(t *testing.T) {
	srv, arrivals := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := recentRange()
	for _, tc := range []struct {
		name       string
		sym        string
		start, end time.Time
		wantHits   int
	}{
		{"首次填充缓存", "NVDA", start, end, 1},
		{"换 symbol 必须另发一次", "AAPL", start, end, 2},
		{"换 start 必须另发一次", "NVDA", start.AddDate(0, 0, -1), end, 3},
		{"换 end 必须另发一次", "NVDA", start, end.AddDate(0, 0, -1), 4},
		// 最后回到首个组合:键若被做成「每次都不同」(如掺了时间戳),这里会涨到 5。
		{"回到首个组合应命中缓存", "NVDA", start, end, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.FetchHistory(tc.sym, tc.start, tc.end); err != nil {
				t.Fatal(err)
			}
			if n := len(arrivals()); n != tc.wantHits {
				t.Errorf("累计 HTTP 请求数 = %d, want %d", n, tc.wantHits)
			}
		})
	}
}

// functional[0] 的另一半:主题常量必须与 policy 内置表对得上。
//
// 本文件其余用例都用 topicTimeSeries 自己登记测试策略,所以**常量拼错它们全绿**;
// 而生产上 Lookup 落空 = 该主题完全不节流(Gate 对未登记主题直通),直接撞
// TD 免费层 8 req/min。这条是那个缺口唯一的守护者,也是「8s」这个数值在本包
// 侧的唯一锚点(数值平移不调整)。
func TestTopicMatchesBuiltinPolicy(t *testing.T) {
	p, ok := policy.NewTable().Lookup(topicTimeSeries)
	if !ok {
		t.Fatalf("主题 %q 未登记在 policy 内置表里(该主题将完全不节流)", topicTimeSeries)
	}
	if p.MinInterval != 8*time.Second {
		t.Errorf("%s 的 MinInterval = %v, want 8s(免费层 8 req/min,数值平移不调整)",
			topicTimeSeries, p.MinInterval)
	}
}

// boundary[0]:返回值必须与缓存里的底层数组无关。
//
// 这条同时守住两侧:改 first 若能污染缓存,说明**第一次**返回的就不是副本
// (Gate 存进缓存的正是 fn 的返回值);second 若与 first 共享底层数组,说明
// **命中缓存**那次没复制。任一侧失守本用例都转红。
//
// 深拷贝手段用 slices.Clone 的前提:core.OHLCV 是 flat value type(字段全为
// string/float64/int64/time.Time,无 map/slice/指针),浅元素复制即深复制——
// 这是 cache.go:127-128 原有的限定,契约 §T2 迁移时曾丢失过一次。元素类型
// 若将来加入引用型字段,此处必须改逐元素深拷贝。
func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv, _ := probeServer(t, gateSeriesBody)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := recentRange()
	first, err := c.FetchHistory("NVDA", start, end)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first 返回 %d 条, want 1", len(first))
	}
	first[0].Close = -1

	second, err := c.FetchHistory("NVDA", start, end)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second 返回 %d 条, want 1", len(second))
	}
	// 等值断言而非「!= -1」的排除断言:排除断言只挡住恰好枚举到的那个错值(陷阱 6①)。
	if second[0].Close != 101.5 {
		t.Errorf("缓存被调用方改动污染: second[0].Close = %v, want 101.5", second[0].Close)
	}
	// 底层数组必须相异。即便上面的值断言因将来改动而失守,这条仍钉住所有权语义。
	if len(first) > 0 && len(second) > 0 && &first[0] == &second[0] {
		t.Error("两次返回共享同一底层数组,缓存命中未复制")
	}
}

// error_handling[0] 分句 2:错误不写缓存。
//
// 复合句 DoD 的第二个分句必须独立取证(教训 12):判据取「**两次都真的发了 HTTP**」,
// 而不是「第二次仍返回 error」—— 后者在「把 error 也缓存下来」的实现下同样成立,
// 骗得过。
func TestFetchHistoryErrorNotCached(t *testing.T) {
	srv, arrivals := probeServer(t, `{"code":429,"message":"limit","status":"error"}`)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := recentRange()
	for i := 0; i < 2; i++ {
		// 刻意用 Errorf 而非 Fatalf:「错误被缓存」的实现里第二次会拿到缓存的零值 +
		// nil error,若在此中止就永远走不到下面那条请求次数断言,读者只会看到
		// 「want error got nil」而看不见「只发了 1 次 HTTP」(陷阱 10:先判被测行为,
		// 别让先到的判定替缺陷把话说完)。
		if _, err := c.FetchHistory("NVDA", start, end); err == nil {
			t.Errorf("call %d: want error, got nil", i)
		}
	}
	if n := len(arrivals()); n != 2 {
		t.Errorf("失败不得写缓存,两次调用应各发一次 HTTP, got %d", n)
	}
}

// functional[2]:构造函数在**构造时快照** policy.Default()(契约陷阱 4)。
// 这也正是本文件其余用例得以「包内直接赋值 c.gate」注入测试闸门的前提。
func TestNewSnapshotsDefaultGate(t *testing.T) {
	want := gateWith(policy.Policy{})
	policy.SetDefault(want)
	t.Cleanup(func() { policy.SetDefault(testDefaultGate) })

	if got := New("k1").gate; got != want {
		t.Errorf("New().gate = %p, want 构造时的 policy.Default() = %p", got, want)
	}
	if got := NewWithBaseURL("k1", "http://127.0.0.1:1").gate; got != want {
		t.Errorf("NewWithBaseURL().gate = %p, want 构造时的 policy.Default() = %p", got, want)
	}
}
