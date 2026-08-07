package tushare

// Context Checkpoint: done_criteria → test mapping（TASK-020）
//
// functional[0]  缓存键区分度（key 维度）：每个影响结果的参数各自独立验证
//                    → TestCacheSlotDistinguishesEachParam（4 个子用例）
// functional[0]  区分度（topic 维度）：不同 api 不得共用缓存槽
//                    → TestCacheSlotDistinguishesTopic
// error[1]       业务错误不写缓存（校验须留在被缓存的 fn 内）
//                    → TestBusinessErrorIsNotCached
//
// **为什么既有的 TestCallKeyDistinguishesParams 挡不住**（test-agent-16 变异实测）：
// 它是 `callKey` 的**纯函数**断言（`callKey(A) != callKey(B)`），不走 Fetch 路径。
// 把**调用点** `callKey(params, fields)` 整个换成固定串时：纯函数断言不受影响
// （改的是调用点、`callKey` 本身没动），而 TestCallKeyIsOrderIndependent 的聚合度
// 断言在固定键下仍然 hits()==1 ⇒ 两条都通过、变异存活。
// ⇒ **缺的不是「callKey 会区分」，是「缓存槽真的用了 callKey 的输出」。**
// 本文件的断言一律落在**行为侧**（HTTP 请求次数），不再断言纯函数返回值。
//
// ⚠ 本包 key 的实际构造为**现读**，不照搬别包（契约教训 23）：
// `callKey` = 按键名字典序拼 `k=v&` + `fields=<fields>`；日期经
// `dateParams` 用 `Format("20060102")` ⇒ **日粒度**。故日期维度的对照必须相差
// **≥ 1 天** —— 照搬别包的毫秒/秒级偏移会落进同一个键，失效方向是**假的
// 「变异无效」**（它伪装成「这里本来就没问题」），比假绿更隐蔽。

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// ttlTable 给指定主题登记**同一份**带 TTL 的策略。
//
// 刻意不用内置表：内置表里四个 tushare 主题**策略并不等价**（policy.go:75-82，
// `daily_basic` 比其余三个多一个 Quota{5,24h,SH}）。topic 维度的对照若踩在
// 策略不等价的两个主题上，配额会成为混淆因子——本文件要隔离的是**缓存槽**，
// 不是策略解析。
func ttlTable(topics ...string) *policy.Table {
	tbl := zeroTable()
	for _, tp := range topics {
		tbl.Set(tp, policy.Policy{Domain: "tushare", TTL: time.Minute, Coalesce: true})
	}
	return tbl
}

const keyGuardBody = `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","close"],"items":[["600519.SH","20260805",1680.5]]}}`

// scriptedServer 按调用次序返回 bodies[i]（用尽后重复最后一条），并计数。
// 既有 countingServer 只能返回固定 body，错误路径用例需要「先错后对」。
func scriptedServer(t *testing.T, bodies ...string) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := n
		n++
		mu.Unlock()
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		_, _ = w.Write([]byte(bodies[i]))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// baseParams 是对照组的基准参数。日期取相差一个月的两个**自然日**字面量，
// 不用 time.Now() 偏移——本包 key 是日粒度，用相对时间会让「相差多少」
// 依赖运行时刻。
func baseParams() map[string]string {
	return map[string]string{
		"ts_code":    "600519.SH",
		"start_date": "20260101",
		"end_date":   "20260131",
	}
}

const baseFields = "ts_code,trade_date,close"

// TestCacheSlotDistinguishesEachParam 守护 functional[0] 的 key 维度。
//
// **每个影响结果的参数各自独立验证**（DoD 明确要求，参照 crypto 的做法）：
// 一条测试凑合覆盖多个维度时，任一维度漏进 key 都会被其余维度的差异掩盖，
// 失败信息也指不出是哪个参数。
//
// 判据是 **HTTP 请求次数**而非返回值（契约陷阱 2：Fetch 的类型断言失败会静默
// 降级成「重取一次」，返回值照样正确）。
//
// 序列取 base → variant → base 的**重放**形态：它把两种缺陷同时排除 ——
// 该参数不进 key（第 2 次误命中 ⇒ 总数 1）与 根本没缓存（总数 3）。
// 只发 base、variant 两次并断言 2 的写法对后者是假绿。
func TestCacheSlotDistinguishesEachParam(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string) // 改一个参数，其余不动
		fields  string
		explain string
	}{
		{"ts_code", func(p map[string]string) { p["ts_code"] = "000001.SZ" }, baseFields,
			"不同标的共用缓存槽 ⇒ 拿到别的标的的收盘价（静默错数据，比缓存没生效危险得多）"},
		{"start_date", func(p map[string]string) { p["start_date"] = "20260201" }, baseFields,
			"不同起始日共用缓存槽 ⇒ 拿到错误区间的数据"},
		{"end_date", func(p map[string]string) { p["end_date"] = "20260228" }, baseFields,
			"不同结束日共用缓存槽 ⇒ 拿不到最新一根收盘价"},
		{"fields", nil, "ts_code,trade_date,pe_ttm,pb,ps_ttm",
			"不同字段集共用缓存槽 ⇒ 拿到缺列的行，下游按列名取值会静默拿到零值"},
	}
	if len(cases) == 0 {
		t.Fatal("对照集合为空，本测试空转") // 下界，防空真
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := scriptedServer(t, keyGuardBody)
			c := NewWithBaseURL("tok", srv.URL)
			c.gate = policy.New(ttlTable("tushare.daily"), nil)

			variant := baseParams()
			if tc.mutate != nil {
				tc.mutate(variant)
			}
			seq := []struct {
				params map[string]string
				fields string
			}{
				{baseParams(), baseFields},
				{variant, tc.fields},
				{baseParams(), baseFields},
			}
			for i, s := range seq {
				if _, err := c.call("daily", s.params, s.fields); err != nil {
					t.Fatalf("第 %d 次调用: %v", i, err)
				}
			}
			if got := hits(); got != 2 {
				t.Errorf("HTTP 请求 %d 次, want 2 —— %s", got, tc.explain)
			}
		})
	}
}

// TestCacheSlotDistinguishesTopic 守护 functional[0] 的 topic 维度。
//
// 两次调用的 params 与 fields **完全相同**，只有 apiName 不同 —— 这样才把
// topic 单独隔离出来。若缓存槽不按 topic 分，第二次会命中第一次的槽，
// 拿到**另一个接口**的数据。
//
// 两个主题取 daily 与 index_daily：它们在内置表里策略等价（同一份
// tusharePolicy），不像 daily_basic 还带 Quota —— 避免配额成为混淆因子。
func TestCacheSlotDistinguishesTopic(t *testing.T) {
	srv, hits := scriptedServer(t, keyGuardBody)
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(ttlTable("tushare.daily", "tushare.index_daily"), nil)

	for i, api := range []string{"daily", "index_daily", "daily"} {
		if _, err := c.call(api, baseParams(), baseFields); err != nil {
			t.Fatalf("第 %d 次调用(%s): %v", i, api, err)
		}
	}
	if got := hits(); got != 2 {
		t.Errorf("HTTP 请求 %d 次, want 2 —— 不同 api 共用缓存槽 ⇒ 拿到另一个接口的数据", got)
	}
}

// TestBusinessErrorIsNotCached 守护 error_handling[1]（本 Sprint 八家横表里
// tushare 是唯一缺这条的）。
//
// **失效形态比「错误持续一个 TTL」更坏**：若业务错误被当成功值写进缓存，
// 第二次调用会**静默返回成功且无数据** —— 上层会当作「该标的今天没数据」
// 并可能落库。错误变成了成功。
//
// 实现上的对应约束：40203 等信封校验必须留在 `callHTTP`（即被缓存的 fn）
// 内部，让 Fetch 看到的是 error 而非成功值（Gate 不缓存错误）。把校验挪到
// Fetch 之外，本测试即转红。
func TestBusinessErrorIsNotCached(t *testing.T) {
	const errBody = `{"code":40203,"msg":"抱歉，您访问该接口的频率超限"}`
	srv, hits := scriptedServer(t, errBody, errBody)
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(ttlTable("tushare.daily"), nil)

	var errs [2]error
	for i := 0; i < 2; i++ {
		_, errs[i] = c.call("daily", baseParams(), baseFields)
	}

	// **先判被测行为，再判本轮是否有效**（契约陷阱 10）。
	// 先前把「每次调用都应报错」写在前面，结果注入「错误被当成功缓存」的变异时，
	// 失败信息是「本轮未构成检验」—— 缺陷伪装成了测试构造失败，方向指反。
	// 请求次数才是「错误有没有进缓存槽」的直接观测量。
	if got := hits(); got != 2 {
		t.Errorf("HTTP 请求 %d 次, want 2 —— 业务错误被写进了缓存槽，第二次调用不再发请求", got)
	}
	for i, err := range errs {
		if err == nil {
			t.Errorf("第 %d 次调用静默返回了成功 —— 业务错误被当成功值缓存，"+
				"上层会把它当作「该标的今天没数据」并可能落库", i)
		}
	}

	// 有效性判定放在行为断言**之后**（契约陷阱 10：反过来会让缺陷伪装成
	// 「测试构造失败」）。没有这一格，「2 次请求」在 TTL 根本没生效时
	// 也恒成立 —— 那时本测试与「缓存压根没开」无法区分。
	srvOK, hitsOK := scriptedServer(t, keyGuardBody)
	cOK := NewWithBaseURL("tok", srvOK.URL)
	cOK.gate = policy.New(ttlTable("tushare.daily"), nil)
	for i := 0; i < 2; i++ {
		if _, err := cOK.call("daily", baseParams(), baseFields); err != nil {
			t.Fatalf("对照组第 %d 次调用: %v", i, err)
		}
	}
	if got := hitsOK(); got != 1 {
		t.Fatalf("对照组 HTTP 请求 %d 次, want 1 —— TTL 未生效，"+
			"上面那条「错误不写缓存」的断言不构成检验", got)
	}
}
