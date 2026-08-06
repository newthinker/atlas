package tushare

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// Context Checkpoint: done_criteria → test mapping (TASK-009)
// functional[0]  "配额耗尽 → errors.Is(err, ErrRateLimited)，ErrQuotaExceeded 不外泄"
//                → TestQuotaExceededMapsToErrRateLimited
// functional[1]  "policy 错误不出现在对外错误链上；绝不映射成永久性 ErrNoPermission"
//                → TestPolicyErrorDoesNotLeakToCallers
// functional[2]  "call 经 Gate 走 TTL 缓存，同 api+params+fields 只发一次请求"
//                → TestCallCachedByGate
// functional[3]  "callKey 区分 params 与 fields，且必须按键名排序"
//                → TestCallKeyDistinguishesParams
// boundary[0]    "只有 daily_basic 受配额，其余三个 api 不受影响"
//                → TestOtherApisAreNotQuotaLimited + TestNonQuotaApisNeverConsultQuotaStore
// boundary[1]    "缓存返回值所有权显式处理，禁止 slices.Clone 充数"
//                → TestCachedRowsNotMutatedByConsumers
// error_handling[0] "既有用例一条不删一条不改全通过" → client_test.go 全部（仅 TestThrottleMinInterval
//                   按 DoD 明文跟随 Gate 改造为 TestThrottleViaGate）

// TestMain 把默认闸门换成零间隔零 TTL：生产策略的 200ms 会拖慢整包用例，
// 5 次/天的配额更会让用例互相干扰。
//
// 顺序要求（design-spec 陷阱 4）：NewWithBaseURL 在**构造时快照** policy.Default()，
// 所以替换必须发生在任何 client 构造之前。TestMain 早于全部测试，满足该前提。
func TestMain(m *testing.M) {
	policy.SetDefault(policy.New(zeroTable(), nil))
	os.Exit(m.Run())
}

// zeroTable 把四个 tushare 主题都登记为零策略（不节流、不缓存、不计配额）。
func zeroTable() *policy.Table {
	tbl := policy.NewTable()
	for _, api := range []string{"daily", "index_daily", "hk_daily", "daily_basic"} {
		tbl.Set("tushare."+api, policy.Policy{Domain: "tushare"})
	}
	return tbl
}

const gateDailyBasicBody = `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],"items":[["600519.SH","20260805",25.1,8.2,11.3]]}}`

func countingServer(t *testing.T, body string) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// 验收标准 4 + 设计 §5.1：配额预判产生的错误必须满足
// errors.Is(err, ErrRateLimited)，这是「refresh.go 一行不改」承诺的锚点。
func TestQuotaExceededMapsToErrRateLimited(t *testing.T) {
	srv, hits := countingServer(t, gateDailyBasicBody)

	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{
		Domain: "tushare",
		Quota:  &policy.Quota{Limit: 2, Window: 24 * time.Hour, Loc: time.UTC},
	})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, policy.NewMemStore())

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	for i := 1; i <= 2; i++ {
		if _, err := c.FetchDailyBasic("600519.SH", start, end); err != nil {
			t.Fatalf("第 %d 次应成功: %v", i, err)
		}
	}

	_, err := c.FetchDailyBasic("000001.SZ", start, end)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("配额耗尽必须映射为 ErrRateLimited（降级链据此分叉）, got %v", err)
	}
	if errors.Is(err, ErrNoPermission) {
		t.Error("配额是临时性错误，不得映射成永久性的 ErrNoPermission")
	}
	if hits() != 2 {
		t.Errorf("撞墙前拦截：HTTP 请求 %d 次, want 2", hits())
	}
}

// 注意 Limit 取 1 而非方案原文的 0：policy 包里 `Limit <= 0` 的语义是**不设上限**
// （quota.go:51-57，TASK-003 的决策），方案原文的 `Limit: 0` 根本不会触发配额拒绝，
// 该测试会拿到 err == nil 而非 ErrRateLimited。改为 Limit:1 并先用掉这一次，
// 第二次才真正走到配额预判——这样测的才是「映射发生了」而不是「什么都没发生」。
func TestPolicyErrorDoesNotLeakToCallers(t *testing.T) {
	srv, _ := countingServer(t, gateDailyBasicBody)
	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{
		Domain: "tushare",
		Quota:  &policy.Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC},
	})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, policy.NewMemStore())

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	if _, err := c.FetchDailyBasic("600519.SH", start, end); err != nil {
		t.Fatalf("第一次应成功（配额未耗尽）: %v", err)
	}

	_, err := c.FetchDailyBasic("000001.SZ", start, end)
	if errors.Is(err, policy.ErrQuotaExceeded) {
		t.Error("policy 包错误不得外泄到调用方（设计 §5.1），必须在本包内映射")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}

func TestOtherApisAreNotQuotaLimited(t *testing.T) {
	srv, hits := countingServer(t, `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","close"],"items":[["600519.SH","20260805",1680.5]]}}`)
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(zeroTable(), policy.NewMemStore())

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	for i := 0; i < 8; i++ {
		if _, err := c.FetchDaily("600519.SH", start, end); err != nil {
			t.Fatalf("daily 无实测配额，不得设限: 第 %d 次 %v", i, err)
		}
	}
	if hits() != 8 {
		t.Errorf("HTTP 请求 %d 次, want 8", hits())
	}
}

// recordingQuotaStore 记录 Take 的每次调用，用于**直接观测**「配额路径是否被走到」。
type recordingQuotaStore struct {
	mu     sync.Mutex
	topics []string
}

func (r *recordingQuotaStore) Take(topic string, q policy.Quota, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics = append(r.topics, topic)
	return true, nil
}

func (r *recordingQuotaStore) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.topics...)
}

// boundary[0] 的**否定断言**部分（design-spec 陷阱 8）。
//
// 为什么不能只靠 TestOtherApisAreNotQuotaLimited 的 hits()==8：那是**正向**观测，
// 证明了「8 次请求都发出去了」，但证不出「没走配额路径」——一个把三个 api 也
// 记进账本、只是恰好没到上限的实现，同样会让 8 次全部通过。计数值无法区分
// 「没调用」与「调用了但放行」。⇒ 直接注入可观测的假账本，断言 Take 从未被调用。
func TestNonQuotaApisNeverConsultQuotaStore(t *testing.T) {
	srv, _ := countingServer(t, `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","close"],"items":[["600519.SH","20260805",1680.5]]}}`)
	rec := &recordingQuotaStore{}
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(zeroTable(), rec)

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	for _, fetch := range []func(string, time.Time, time.Time) ([]PricePoint, error){
		c.FetchDaily, c.FetchIndexDaily, c.FetchHKDaily,
	} {
		if _, err := fetch("600519.SH", start, end); err != nil {
			t.Fatalf("不受配额限制的 api 不应失败: %v", err)
		}
	}
	if got := rec.calls(); len(got) != 0 {
		t.Errorf("daily/index_daily/hk_daily 不得触达配额账本, Take 被调用于 %v", got)
	}

	// 对照：daily_basic 配了 Quota，同一个账本必须被调用——否则上面的 0 次
	// 可能只是因为整条配额链根本没接上，那样这个测试就成了永远绿的摆设。
	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{
		Domain: "tushare",
		Quota:  &policy.Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC},
	})
	basicSrv, _ := countingServer(t, gateDailyBasicBody)
	c2 := NewWithBaseURL("tok", basicSrv.URL)
	c2.gate = policy.New(tbl, rec)
	if _, err := c2.FetchDailyBasic("600519.SH", start, end); err != nil {
		t.Fatalf("daily_basic 应成功: %v", err)
	}
	if got := rec.calls(); len(got) != 1 || got[0] != "tushare.daily_basic" {
		t.Errorf("daily_basic 必须触达配额账本一次, got %v", got)
	}
}

func TestCallCachedByGate(t *testing.T) {
	srv, hits := countingServer(t, gateDailyBasicBody)
	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{Domain: "tushare", TTL: time.Minute, Coalesce: true})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, nil)

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.FetchDailyBasic("600519.SH", start, end); err != nil {
			t.Fatal(err)
		}
	}
	if hits() != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", hits())
	}
}

// boundary[1]（反审 A7）：缓存返回值的所有权 —— 解法 (b) 逐行 map 深拷贝。
//
// `policy.Fetch` 命中缓存时**不复制返回值**，多个调用方拿到同一份 []row，而
// `row.values` 是 map，共享的是同一个 map header。`call` 在返回前 cloneRows 深拷贝，
// 使每个调用方拿到独立的一份。
//
// 判据是「改返回值 → 再取 → 缓存未被污染」，直接观测**隔离本身**这个性质，
// 而不是去观测调用方代码有没有写操作。
//
// 关键在于它能把 `slices.Clone` 与真深拷贝区分开（DoD 明文禁止前者）：
// `slices.Clone` 复制 row 结构体但 values 仍指向同一块底层数据，第 1 段的 map
// 写入会穿透到缓存里，第 2 次取值即转红。已用变异实测确认。
//
// **不要退回「用测试钉死消费者只读」那个方案**（首轮曾采用，验证后撤销）：
// 行为测试可测的是「写**导致返回值可观测地变化**」，不可测的是「写本身」，
// 两者的差集永远测不到。实测漏检的一类是**归一化写回**——消费者写
// `if math.IsNaN(r.values["pe_ttm"]) { r.values["pe_ttm"] = 0 }` 时全套用例全绿，
// 而 NaN 恰恰是本包自己造的（client.go 里非数值列与缺列一律填 NaN），
// 消费者做 NaN→0 归一化是完全合理的需求。差集还包括幂等写与条件不触发的写。
// (b) 没有这个差集：调用方拿到的本就是自己的副本，写什么都到不了缓存。
func TestCachedRowsAreIsolatedFromCallers(t *testing.T) {
	body := `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],"items":[["600519.SH","20260805",25.1,8.2,11.3],["600519.SH","20260804",24.9,8.1,11.2]]}}`
	srv, hits := countingServer(t, body)
	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{Domain: "tushare", TTL: time.Minute, Coalesce: true})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, nil)

	params := dateParams("600519.SH", time.Now().AddDate(0, 0, -5), time.Now())
	const fields = "ts_code,trade_date,pe_ttm,pb,ps_ttm"

	r1, err := c.call("daily_basic", params, fields)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1) != 2 {
		t.Fatalf("len(r1) = %d, want 2", len(r1))
	}

	// 调用方改自己拿到的那份：map 值、新增键、以及结构体字段本身。
	r1[0].values["pe_ttm"] = -999
	r1[0].values["injected"] = 1
	r1[1].date = time.Time{}

	r2, err := c.call("daily_basic", params, fields)
	if err != nil {
		t.Fatal(err)
	}
	if hits() != 1 {
		t.Fatalf("第二次必须命中缓存才构成本测试的前提, HTTP 请求 %d 次", hits())
	}

	if got := r2[0].values["pe_ttm"]; got != 24.9 { // 行按日期升序，r[0] 是 20260804
		t.Errorf("缓存被调用方的 map 写入污染: pe_ttm = %v, want 24.9（slices.Clone 会走到这里）", got)
	}
	if _, ok := r2[0].values["injected"]; ok {
		t.Error("调用方新增的键穿透到了缓存条目，说明 values 仍是共享的同一个 map")
	}
	if r2[1].date.IsZero() {
		t.Error("调用方对 row 结构体字段的写入穿透到了缓存条目")
	}

	// 反向：两次取到的必须是不同的 map 实例，否则上面的断言只是碰巧成立。
	r2[0].values["pe_ttm"] = -1
	if r1[0].values["pe_ttm"] == r2[0].values["pe_ttm"] {
		t.Error("两次调用返回了同一个 map 实例，未发生深拷贝")
	}
}

func TestCallKeyDistinguishesParams(t *testing.T) {
	a := callKey(map[string]string{"ts_code": "600519.SH", "start_date": "20260101"}, "close")
	b := callKey(map[string]string{"ts_code": "000001.SZ", "start_date": "20260101"}, "close")
	if a == b {
		t.Error("不同 ts_code 必须产生不同缓存键")
	}
	if callKey(map[string]string{"ts_code": "600519.SH"}, "close") == callKey(map[string]string{"ts_code": "600519.SH"}, "pe_ttm") {
		t.Error("fields 不同必须产生不同缓存键")
	}
}

// 排序守护单独立一条:它防的是**静默失效**——不排序时功能全对、只是同一次查询
// 散落到多个缓存槽、命中率归零,没有任何断言会因「结果不对」而红。
//
// 首轮交付时这条守护是概率性的:只断言「重复算 N 次结果相同」,而 Go 的 map
// 遍历顺序是每次 range 随机,2 个键时未排序实现有 1/2 概率碰巧一致
// (验证者实测捕获率 90~95%,我当时误报成「连跑 5 次稳定红」)。现改为两条确定性判据:
//
//  1. **对着排好序的字面量断言**,而不是断言「两次算的一样」。判据不再依赖随机性,
//     只要某次遍历吐出非字典序就必红;配合 4 个键 × 200 次,未排序实现要连续 200 次
//     都恰好撞中字典序才能逃过,概率 (1/24)^200。
//  2. **端到端断言散落多槽的后果本身**:同一组 params 用不同插入顺序构造两个 map,
//     经完整 Fetch 路径只应发出 1 次 HTTP 请求。
func TestCallKeyIsOrderIndependent(t *testing.T) {
	const want = "end_date=20260131&start_date=20260101&ts_code=600519.SH&fields=close"
	for i := 0; i < 200; i++ {
		params := map[string]string{
			"ts_code":    "600519.SH",
			"start_date": "20260101",
			"end_date":   "20260131",
		}
		if got := callKey(params, "close"); got != want {
			t.Fatalf("第 %d 次:缓存键必须按键名字典序拼接\n got = %q\nwant = %q", i, got, want)
		}
	}

	// 后果侧:插入顺序不同的两个等价 params 必须命中同一个缓存槽。
	srv, hits := countingServer(t, `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","close"],"items":[["600519.SH","20260805",1680.5]]}}`)
	tbl := zeroTable()
	tbl.Set("tushare.daily", policy.Policy{Domain: "tushare", TTL: time.Minute, Coalesce: true})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, nil)

	p1 := map[string]string{}
	for _, kv := range [][2]string{{"ts_code", "600519.SH"}, {"start_date", "20260101"}, {"end_date", "20260131"}} {
		p1[kv[0]] = kv[1]
	}
	p2 := map[string]string{}
	for _, kv := range [][2]string{{"end_date", "20260131"}, {"start_date", "20260101"}, {"ts_code", "600519.SH"}} {
		p2[kv[0]] = kv[1]
	}
	if _, err := c.call("daily", p1, "ts_code,trade_date,close"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.call("daily", p2, "ts_code,trade_date,close"); err != nil {
		t.Fatal(err)
	}
	if hits() != 1 {
		t.Errorf("等价 params 散落到了多个缓存槽:HTTP 请求 %d 次, want 1", hits())
	}
}
