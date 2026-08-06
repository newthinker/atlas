package lixinger

// Context Checkpoint: done_criteria → test mapping
// functional[0] "request() 经 Gate 进入缓存路径，同 endpoint+payload 重复调用
//                只发一次 HTTP 请求（验收标准 6 —— §1.3 缺陷修复）"
//                                              → TestLixingerRequestIsCached
// functional[1] "缓存 key 包含请求 payload，不同 payload 不共享缓存"
//                                              → TestLixingerCacheKeyIncludesPayload
// functional[2] "不同 endpoint 使用互相独立的缓存 key"
//                                              → TestLixingerEndpointsAreSeparateKeys
// functional[3] "Lixinger 持有 gate *policy.Gate 字段，构造函数取 policy.Default()，
//                包内测试可赋值注入"           → TestLixingerConstructorSnapshotsDefaultGate
//                                                （注入侧由上列各测试的 l.gate = … 覆盖）
// boundary[0]   "lixinger 不被节流：连续请求无最小间隔等待"
//                                              → TestLixingerNotThrottled
// boundary[1]   "缓存命中时返回独立的字节切片，调用方修改不污染缓存"
//                                              → TestLixingerReturnsIndependentBytes
// error[0]      "错误不写缓存"                 → TestLixingerErrorIsNotCached
//               "既有测试一条不删不改且全部通过" → 由 TestMain 的零 TTL 默认闸门保证

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// TestMain 把默认闸门换成零策略：内置 lixinger.* 主题带 5m 缓存，而本包既有的
// 错误路径用例大量复用同一个 endpoint "x" + 空 payload（即同一个缓存槽），
// 带 TTL 的默认闸门会让它们互相串味。
func TestMain(m *testing.M) {
	policy.SetDefault(policy.New(gateTable(policy.Policy{}), nil))
	os.Exit(m.Run())
}

// gateTable 构造只含 lixinger.* 通配主题的表，用于需要精确控制策略的用例。
func gateTable(p policy.Policy) *policy.Table {
	tbl := policy.NewTable()
	p.Domain = "lixinger"
	tbl.Set("lixinger.*", p)
	return tbl
}

// builtinGate 用**内置表**构造闸门。缓存类用例刻意不用手搓策略：内置
// lixinger.* 恰是 {TTL: 5m, Coalesce: true}，用它才能连带守护「request() 拼出的
// 主题名确实能命中内置通配条目」——主题名拼错（如漏掉 "lixinger." 前缀）时
// Lookup 落到未登记直通，缓存静默失效。
func builtinGate() *policy.Gate { return policy.New(policy.NewTable(), nil) }

const (
	gateFundamentalBody = `{"code":1,"message":"ok","data":[]}`
	gateEndpoint        = "cn/company/fundamental/non_financial"
)

// scriptedLixServer 按调用次序返回 bodies[i]（用尽后重复最后一条），
// 并返回累计 HTTP 请求次数。
func scriptedLixServer(t *testing.T, bodies ...string) (*httptest.Server, func() int) {
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

func countingLixServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	return scriptedLixServer(t, gateFundamentalBody)
}

// 验收标准 6：lixinger 必须能进入缓存路径。它从未被 RegisterCollector 注册，
// 只以「eastmoney 的内部 fallback」和「Valuation/Fundamental source」两种身份
// 存在，两条路径都绕过任何缓存层；Gate 放进 request() 后两条同时被覆盖。
func TestLixingerRequestIsCached(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = builtinGate()

	payload := map[string]any{"token": "key", "stockCodes": []string{"600519"}}
	for i := 0; i < 3; i++ {
		if _, err := l.request(gateEndpoint, payload); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// 断言的是 HTTP 请求次数而非返回值：Fetch 在类型断言失败时会「当作未命中
	// 重新取」，返回值仍然正确，缺陷自愈成「多发一次请求」（契约陷阱 2）。
	if got := hits(); got != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", got)
	}
}

func TestLixingerCacheKeyIncludesPayload(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = builtinGate()

	a := map[string]any{"stockCodes": []string{"600519"}}
	b := map[string]any{"stockCodes": []string{"000001"}}
	// a → b → a 的重放序列是关键：它把两种缺陷同时排除 ——
	// key 不含 payload（第 2 次误命中 → 总数 1）与 根本没缓存（总数 3）。
	// 只发 a、b 两次、断言 2 的写法对后者是假绿。
	for i, p := range []map[string]any{a, b, a} {
		if _, err := l.request(gateEndpoint, p); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := hits(); got != 2 {
		t.Errorf("HTTP 请求 %d 次, want 2（不同 payload 不得共用缓存槽，同 payload 须命中）", got)
	}
}

func TestLixingerEndpointsAreSeparateKeys(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = builtinGate()

	payload := map[string]any{"x": 1}
	// 同上：e1 → e2 → e1，排除「endpoint 不进 key」与「压根没缓存」两种缺陷。
	for i, endpoint := range []string{"cn/fund/net-value", "cn/fund/profile", "cn/fund/net-value"} {
		if _, err := l.request(endpoint, payload); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := hits(); got != 2 {
		t.Errorf("HTTP 请求 %d 次, want 2（不同 endpoint 不得共用缓存槽，同 endpoint 须命中）", got)
	}
}

// TestLixingerConstructorSnapshotsDefaultGate 守护 functional[3]：构造函数必须
// 取 policy.Default()，而不是各自新建或留 nil（nil *Gate 是透明的，缓存会静默
// 失效而所有既有测试照常绿）。用指针相等断言，任何别的 *Gate 都转红。
func TestLixingerConstructorSnapshotsDefaultGate(t *testing.T) {
	prev := policy.Default()
	t.Cleanup(func() { policy.SetDefault(prev) })

	want := policy.New(gateTable(policy.Policy{TTL: time.Minute, Coalesce: true}), nil)
	// 陷阱 4：gate 在构造时**快照** Default() 的当时值，故 SetDefault 必须先于构造。
	policy.SetDefault(want)

	if got := NewWithBaseURL("k", "http://example.invalid").gate; got != want {
		t.Errorf("NewWithBaseURL().gate = %p, want %p（构造函数须取 policy.Default()）", got, want)
	}
	if got := New("k").gate; got != want {
		t.Errorf("New().gate = %p, want %p（构造函数须取 policy.Default()）", got, want)
	}
}

const (
	throttleProbeCalls = 3
	// throttleProbeInterval 取 200ms（本仓库现存最小的 MinInterval，tushare）。
	// 对照组 3 次调用应等 2×200ms，被测组是 3 次本地 httptest 往返（~毫秒级），
	// 两侧相差一个量级，同一阈值即可分开。
	throttleProbeInterval = 200 * time.Millisecond
)

// TestLixingerNotThrottled 守护 boundary[0]：只补它从未有过的缓存，不新增任何
// 它今天没有的限流行为（设计 §4.1 例外）。
//
// 这是**否定断言**（契约陷阱 8）：「没有等待」不能靠间接现象推断，唯一有效的
// 观测量就是墙钟耗时，因此必须真的发请求计时，而不是只查一眼策略表。
func TestLixingerNotThrottled(t *testing.T) {
	// ① 策略表侧：等值断言。写成「!= 某个错值」的排除式会漏掉别的非零值（陷阱 6）。
	p, ok := policy.NewTable().Lookup("lixinger." + gateEndpoint)
	if !ok {
		t.Fatal("lixinger 主题应登记")
	}
	if p.MinInterval != 0 {
		t.Errorf("MinInterval = %v, want 0（只补缓存，不新增限流）", p.MinInterval)
	}
	if p.Quota != nil {
		t.Errorf("Quota = %+v, want nil（只补缓存，不新增配额）", *p.Quota)
	}

	// ② 请求路径侧：直接观测「有没有等」。
	//
	// 探针策略 = 内置 lixinger 策略 + TTL 归零。归零是为了让每次调用都必然走到
	// 执行链 ④ 节流（缓存命中会跳过节流，陷阱 9：对照组必须真的进入被测路径）。
	// 刻意**不**靠「逐次换 payload」来避开缓存——那会让本测试暗中依赖
	// functional[1]，那条 DoD 一坏，这里就只会报「本轮不构成检验」而不是本该
	// 报的结论（实测：L2 变异下确实如此）。
	elapsed := func(minInterval time.Duration) time.Duration {
		t.Helper()
		p, _ := policy.NewTable().Lookup("lixinger." + gateEndpoint)
		p.TTL = 0
		if minInterval > 0 {
			p.MinInterval = minInterval // 仅对照组：显式装上节流
		}
		srv, _ := countingLixServer(t)
		l := NewWithBaseURL("key", srv.URL)
		l.gate = policy.New(gateTable(p), nil)
		start := time.Now()
		for i := 0; i < throttleProbeCalls; i++ {
			if _, err := l.request(gateEndpoint, map[string]any{"x": 1}); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		return time.Since(start)
	}

	// 先判被测行为，再判本轮是否构成检验（陷阱 10：反过来会让缺陷伪装成
	// 「测试构造失败」）。被测组的 MinInterval 原样取自内置表。
	got := elapsed(0)
	throttled := elapsed(throttleProbeInterval)

	if got >= throttleProbeInterval {
		t.Errorf("内置策略下 %d 次连续请求耗时 %v（阈值 %v），说明 lixinger 被节流了",
			throttleProbeCalls, got, throttleProbeInterval)
	}
	// 对照组：证明这套观测手段确实看得见节流，否则被测组的「快」是空转的绿。
	if throttled < throttleProbeInterval {
		t.Errorf("对照组（MinInterval=%v）耗时 %v < %v：观测手段看不见节流，本测试不构成检验",
			throttleProbeInterval, throttled, throttleProbeInterval)
	}
}

// TestLixingerReturnsIndependentBytes 守护 boundary[1]。Gate 命中缓存时**不复制**
// 返回值（policy.Fetch 的契约），防线只能落在这里（教训 6：注释贴在受害者一侧，
// 防线必须在违反者一侧）。
func TestLixingerReturnsIndependentBytes(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = builtinGate()

	payload := map[string]any{"x": 1}
	// 三次而非两次：第 1 次走未命中路径，第 2、3 次走命中路径。每次都改脏自己
	// 拿到的切片。只验两次会漏掉「未命中时复制、命中时直接返回缓存」这种实现
	// ——那种实现下第 2 次拿到的是干净的缓存原件（内容断言通过），
	// 而它被改脏后第 3 次才暴露。
	for i := 0; i < 3; i++ {
		got, err := l.request(gateEndpoint, payload)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if string(got) != gateFundamentalBody {
			t.Fatalf("call %d: body = %q, want %q（上一次调用方的改写污染了缓存）",
				i, got, gateFundamentalBody)
		}
		got[0] = 'X' // 调用方改写自己拿到的副本，不得影响缓存
	}
	// 有效性判定放在行为断言之后：若一次都没命中缓存，上面的循环恒真、本测试空转。
	if got := hits(); got != 1 {
		t.Fatalf("HTTP 请求 %d 次, want 1 —— 未命中缓存则本测试根本没走到命中路径", got)
	}
}

// TestLixingerErrorIsNotCached 守护 error_handling[0] 的「错误不写缓存」。
//
// 它同时钉住一个很自然的实现走偏：把信封校验（parseEnvelope）放到 Fetch **外面**
// ——那样 HTTP 200 + code:0 的业务错误会被当作成功 body 写进缓存，
// 后续同 key 调用永远拿到那个错误，且再也不发请求。
func TestLixingerErrorIsNotCached(t *testing.T) {
	srv, hits := scriptedLixServer(t,
		`{"code":0,"error":{"message":"transient"}}`,
		gateFundamentalBody,
	)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = builtinGate()

	payload := map[string]any{"stockCodes": []string{"600519"}}
	if _, err := l.request(gateEndpoint, payload); err == nil {
		t.Fatal("首次调用应返回信封错误")
	}
	raw, err := l.request(gateEndpoint, payload)
	if err != nil {
		t.Fatalf("错误不得写缓存，重试应重新发请求并成功, got: %v", err)
	}
	if string(raw) != gateFundamentalBody {
		t.Errorf("body = %q, want %q", raw, gateFundamentalBody)
	}
	if got := hits(); got != 2 {
		t.Errorf("HTTP 请求 %d 次, want 2（失败的那次不得占用缓存槽）", got)
	}
}
