package crypto

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-016)
// functional[0] "FetchHistory 经 Gate 走 TTL 缓存，同参数只发一次请求"
//               → TestFetchHistoryCachedByGate（断**调用次数**而非返回值，陷阱 2）
// functional[1] "缓存 key 覆盖 symbol/区间/interval，至少两组参数不互相命中"
//               → TestCacheKeyCoversAllParams（a→b→a 重放，陷阱 16）
// functional[2] "构造函数取 policy.Default() 存入私有 gate，并有独立测试"
//               → TestConstructorsSnapshotDefaultGate（陷阱 12）
// functional[3] "主题名的**域段**与内置表一致"（Leader 裁定：通配登记下只验域段）
//               → TestTopicMatchesBuiltinTable（陷阱 15）
// boundary[0]   "不被节流"（否定断言，直接观测 + 对照组，陷阱 8/9）
//               → TestFetchHistoryNotThrottled
// boundary[1]   "缓存命中时返回独立切片"（[]core.OHLCV 全值类型 ⇒ slices.Clone 足够）
//               → TestFetchHistoryReturnsIndependentSlice
// error_handling "错误不写缓存（判据是请求次数）/ policy 错误不外泄"
//               → TestErrorIsNotCached / TestPolicyErrorDoesNotLeak

// TestMain 把默认闸门换成零策略。
//
// 不是可选的保险（陷阱 13）：接入缓存后，共用同一 topic+key 的既有用例会落进
// 同一个缓存槽，先跑的成功响应会让后跑的「应报错」用例静默失效。本包目前只有
// 一处 FetchHistory 调用，风险小，但这条约束要随缓存一起落地——下一个往本包
// 加用例的人不会知道有这回事。
func TestMain(m *testing.M) {
	policy.SetDefault(policy.New(zeroCryptoTable(), nil))
	os.Exit(m.Run())
}

// zeroCryptoTable 把 crypto 主题登记为零策略（不缓存、不节流、不计配额）。
func zeroCryptoTable() *policy.Table {
	tbl := policy.NewTable()
	tbl.Set("crypto.*", policy.Policy{Domain: "crypto"})
	return tbl
}

// builtinGate 用**真实内置表**构造闸门。
//
// 不手搓 Policy：手搓会把主题名换成测试自己造的那份，于是「FetchHistory 拼出的
// topic 是否真能命中内置 crypto.* 条目」这一层完全没被验（陷阱 15）。用内置表后，
// 若有人删掉内置条目或把 TTL 归零，这些测试会一起转红——那正是要守的东西。
func builtinGate() *policy.Gate { return policy.New(policy.NewTable(), nil) }

// countingProvider 记录 FetchHistory 被调用的次数与参数。
//
// 既有的 mockProvider 不计次，而本任务几乎所有判据都是**请求次数**：
// policy.Fetch 的类型断言失败会「当作未命中重取」，返回值仍然正确，
// 断言返回值的写法永远绿（陷阱 2）。
type countingProvider struct {
	name    string
	history []core.OHLCV
	err     error

	mu    sync.Mutex
	calls int
	args  []string
}

func (p *countingProvider) Name() string { return p.name }

func (p *countingProvider) FetchQuote(symbol string) (*core.Quote, error) {
	return &core.Quote{Symbol: symbol}, nil
}

func (p *countingProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	p.mu.Lock()
	p.calls++
	p.args = append(p.args, fmt.Sprintf("%s|%s|%s|%s",
		symbol, start.Format(time.RFC3339), end.Format(time.RFC3339), interval))
	p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	return p.history, nil
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func sampleBars() []core.OHLCV {
	return []core.OHLCV{
		{Symbol: "BTCUSDT", Interval: "1d", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 10,
			Time: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)},
		{Symbol: "BTCUSDT", Interval: "1d", Open: 1.5, High: 3, Low: 1, Close: 2.5, Volume: 20,
			Time: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)},
	}
}

// newGatedCollector 造一个接到指定闸门上的 collector。
//
// ⚠ 顺序（契约陷阱 4）：构造函数**在构造时快照** policy.Default()，所以这里直接
// 赋 gate 字段，而不是 SetDefault 之后再 New —— 后者会污染其它测试的全局单例。
func newGatedCollector(p Provider, g *policy.Gate) *CryptoCollector {
	c := NewWithProviders([]Provider{p}, "USDT")
	c.gate = g
	return c
}

func testRange() (time.Time, time.Time) {
	return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
}

// functional[0]：同参数重复调用只发一次请求。
func TestFetchHistoryCachedByGate(t *testing.T) {
	p := &countingProvider{name: "stub", history: sampleBars()}
	c := newGatedCollector(p, builtinGate())
	start, end := testRange()

	for i := 0; i < 3; i++ {
		data, err := c.FetchHistory("BTC", start, end, "1d")
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
		if len(data) != 2 {
			t.Fatalf("第 %d 次返回 %d 根，want 2", i+1, len(data))
		}
	}
	if got := p.count(); got != 1 {
		t.Errorf("TTL 内应只向 provider 取一次，实得 %d 次", got)
	}
}

// functional[1]：缓存键必须覆盖 symbol / interval / 时间区间三个维度。
//
// 判据是 **a → b → a 重放**，断言请求次数 == 2，而不是「a → b 断言 == 2」：
// 后者对「根本没缓存」是假绿（不缓存时 a、b 也恰好 2 次，与正确实现无法区分）。
// 第三次重放 a 才把两种缺陷同时排除——键里丢了该维度（第 2 次误命中 ⇒ 总数 1）、
// 以及压根没缓存（总数 3）。
func TestCacheKeyCoversAllParams(t *testing.T) {
	start, end := testRange()
	otherStart, otherEnd := start.AddDate(0, 0, -10), end.AddDate(0, 0, -10)

	cases := []struct {
		dim  string
		a, b func(c *CryptoCollector) ([]core.OHLCV, error)
	}{
		{
			dim: "symbol",
			a:   func(c *CryptoCollector) ([]core.OHLCV, error) { return c.FetchHistory("BTC", start, end, "1d") },
			b:   func(c *CryptoCollector) ([]core.OHLCV, error) { return c.FetchHistory("ETH", start, end, "1d") },
		},
		{
			dim: "interval",
			a:   func(c *CryptoCollector) ([]core.OHLCV, error) { return c.FetchHistory("BTC", start, end, "1d") },
			b:   func(c *CryptoCollector) ([]core.OHLCV, error) { return c.FetchHistory("BTC", start, end, "1h") },
		},
		{
			dim: "区间",
			a:   func(c *CryptoCollector) ([]core.OHLCV, error) { return c.FetchHistory("BTC", start, end, "1d") },
			b: func(c *CryptoCollector) ([]core.OHLCV, error) {
				return c.FetchHistory("BTC", otherStart, otherEnd, "1d")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.dim, func(t *testing.T) {
			p := &countingProvider{name: "stub", history: sampleBars()}
			c := newGatedCollector(p, builtinGate())

			if _, err := tc.a(c); err != nil {
				t.Fatalf("a: %v", err)
			}
			if _, err := tc.b(c); err != nil {
				t.Fatalf("b: %v", err)
			}
			if _, err := tc.a(c); err != nil {
				t.Fatalf("a 重放: %v", err)
			}

			switch got := p.count(); got {
			case 2:
				// 正确：a 未命中取一次、b 未命中取一次、a 重放命中缓存
			case 1:
				t.Errorf("缓存键里丢了 %s 维度：b 误命中了 a 的条目，会静默返回错标的/错区间的数据（args=%v）",
					tc.dim, p.args)
			case 3:
				t.Errorf("完全没有缓存：三次调用发了三次请求（args=%v）", p.args)
			default:
				t.Errorf("请求次数 %d 不在预期内（args=%v）", got, p.args)
			}
		})
	}
}

// 缓存键的**聚合度**：以墙钟为 end 的两次相邻调用必须落进同一个缓存槽。
//
// 这是 functional[1] 的另一半。原 criteria 只写了「区分度」（不同参数不得互相命中），
// 我也只测了那一半，于是 `UnixNano()` 入键这个缺陷完整通过了全部测试——
// **单测里用固定 start/end 调两次是全绿的，只有生产路径（app.go 传 end = time.Now()）
// 才失效，且失效方式是「缓存命中率恒为零」这种不报错、不出错数据的静默无效。**
// ⇒ 只约束一个方向的判据，会放过反方向的失效。
//
// 两条断言缺一不可：
//   - 相邻时间聚合：去掉 Truncate 后转红
//   - **分钟粒度不得放粗**：把 Truncate 改成小时/天也能让上一条通过，
//     但那会让相隔几分钟的不同查询串槽、静默返回错区间数据
//
// 时间基准取「当前分钟的中点」而非直接 time.Now()：与生产同形（end 源自墙钟），
// 但确定性地避开分钟边界，不引入概率性抖动。
func TestCacheKeyAggregatesNearbyTimes(t *testing.T) {
	start, _ := testRange()
	base := time.Now().Truncate(time.Minute).Add(30 * time.Second)

	t.Run("相邻时间落进同一槽", func(t *testing.T) {
		p := &countingProvider{name: "stub", history: sampleBars()}
		c := newGatedCollector(p, builtinGate())
		for i, end := range []time.Time{base, base.Add(50 * time.Millisecond), base.Add(900 * time.Millisecond)} {
			if _, err := c.FetchHistory("BTC", start, end, "1d"); err != nil {
				t.Fatalf("第 %d 次: %v", i+1, err)
			}
		}
		if got := p.count(); got != 1 {
			t.Errorf("以墙钟为 end 的相邻调用未命中同一缓存槽：向 provider 取了 %d 次，want 1。"+
				"生产路径 app.go 传的 end 是 time.Now()，键若按原始精度构造则命中率恒为零（args=%v）",
				got, p.args)
		}
	})

	t.Run("分钟粒度不得放粗", func(t *testing.T) {
		p := &countingProvider{name: "stub", history: sampleBars()}
		c := newGatedCollector(p, builtinGate())
		for i, end := range []time.Time{base, base.Add(time.Minute)} {
			if _, err := c.FetchHistory("BTC", start, end, "1d"); err != nil {
				t.Fatalf("第 %d 次: %v", i+1, err)
			}
		}
		if got := p.count(); got != 2 {
			t.Errorf("相隔一分钟的两次查询落进了同一槽：向 provider 取了 %d 次，want 2。"+
				"截断粒度被放粗到分钟以上会让不同区间的查询静默返回同一份数据（args=%v）",
				got, p.args)
		}
	})
}

// functional[2]：两个构造函数都必须快照 policy.Default()。
//
// 这条不是形式要求（陷阱 12）：nil *Gate 是**透明的**（policy.Fetch 直接执行 fn），
// 而其余测试都各自注入了闸门 —— 漏掉构造函数里那一行时，全套测试照常绿，
// 而生产路径上缓存整体静默 no-op。本包有 New() 与 NewWithProviders() 两个入口，
// 两个都要验：只验一个时，另一个漏写同样是全绿。
func TestConstructorsSnapshotDefaultGate(t *testing.T) {
	sentinel := policy.New(zeroCryptoTable(), nil)
	prev := policy.Default()
	policy.SetDefault(sentinel)
	defer policy.SetDefault(prev)

	if got := New().gate; got != sentinel {
		t.Errorf("New() 未把 policy.Default() 存进 gate 字段（got %p, want %p）——"+
			"nil Gate 是透明的，漏掉这行时生产路径缓存整体静默失效", got, sentinel)
	}
	if got := NewWithProviders([]Provider{&countingProvider{name: "x"}}, "USDT").gate; got != sentinel {
		t.Errorf("NewWithProviders() 未把 policy.Default() 存进 gate 字段（got %p, want %p）",
			got, sentinel)
	}
}

// functional[3]：主题名的**域段**必须与内置表登记的域一致。
//
// Leader 裁定：三家均为通配登记（<域>.*），Lookup 的通配回退让任何 crypto.xxx
// 都能命中 ⇒ 接口段写错无害，只有**域段**写错才会落空（Lookup 未登记 ⇒ Gate 直通、
// 缓存彻底失效且不报任何错）。故只验域段。
//
// 用真正的 policy.NewTable() 查，不用测试自己造的表——用同一常量登记又查询是自洽的，
// 写错也发现不了（陷阱 15）。
func TestTopicMatchesBuiltinTable(t *testing.T) {
	tbl := policy.NewTable()

	p, ok := tbl.Lookup(topicHistory)
	if !ok {
		t.Fatalf("主题 %q 在内置表里查不到 —— 域段写错时 Gate 会静默直通、缓存彻底失效", topicHistory)
	}
	if p.TTL <= 0 {
		t.Errorf("内置 crypto 策略必须带 TTL（本任务的全部意义是恢复被删的 TTL 缓存），got %v", p.TTL)
	}

	// 阳性对照：证明这条查询确实能区分对错，而不是「什么都能命中」。
	if _, ok := tbl.Lookup("cryptoo.history"); ok {
		t.Error("域段写错的主题竟然也命中了 —— 上面那条断言无法区分对错，是空转的绿")
	}
}

// boundary[0]：不被节流。本任务只补缓存，不新增任何本包今天没有的限流行为。
//
// 否定断言只能直接观测那条路径本身（陷阱 8）：「有没有等」唯一的观测量是墙钟耗时，
// 查一眼策略表验的是 TASK-015 的表、验不到本包的请求路径。
//
// 两个细节：
//   - 探针用「内置策略 + 只把 TTL 归零」，不靠逐次换参数来避开缓存。后者会让这条
//     断言暗中依赖 functional[1]：缓存键一旦坏掉，三次调用塌成一个 key、后两次命中
//     缓存跳过节流，本测试随之变哑（TASK-010 实测踩过）。
//   - 对照组的 MinInterval 取正值而非 0，确保它真的进入 throttle 的持锁临界区
//     （陷阱 9：用「完全不触发该路径」的配置做对照，测的是「不进入时会怎样」）。
//     它是每次运行都执行的常驻自检 —— 没有它，被测组的「快」是空转的绿。
func TestFetchHistoryNotThrottled(t *testing.T) {
	const rounds = 3
	start, end := testRange()

	elapsed := func(g *policy.Gate) time.Duration {
		p := &countingProvider{name: "stub", history: sampleBars()}
		c := newGatedCollector(p, g)
		t0 := time.Now()
		for i := 0; i < rounds; i++ {
			if _, err := c.FetchHistory("BTC", start, end, "1d"); err != nil {
				t.Fatalf("第 %d 次: %v", i+1, err)
			}
		}
		d := time.Since(t0)
		if p.count() != rounds {
			t.Fatalf("探针失效：%d 次调用只到达 provider %d 次（TTL 未归零，后续调用命中缓存跳过了节流）",
				rounds, p.count())
		}
		return d
	}

	// 被测组：内置 crypto 策略，只把 TTL 归零。
	probe := policy.NewTable()
	bp, _ := probe.Lookup(topicHistory)
	bp.TTL = 0
	probe.Set("crypto.*", bp)

	// 对照组：同样零 TTL，但显式加 MinInterval —— 证明这套观测手段确实看得见节流。
	ctrl := policy.NewTable()
	cp, _ := ctrl.Lookup(topicHistory)
	cp.TTL = 0
	cp.MinInterval = 200 * time.Millisecond
	ctrl.Set("crypto.*", cp)

	got := elapsed(policy.New(probe, nil))
	want := elapsed(policy.New(ctrl, nil))

	if want < 300*time.Millisecond {
		t.Fatalf("对照组只用了 %v —— 观测手段本身看不见节流，被测组的「快」不构成证据", want)
	}
	if got >= 200*time.Millisecond {
		t.Errorf("crypto 被节流了：%d 次调用耗时 %v（对照组 %v）。本任务只补缓存，不得新增限流",
			rounds, got, want)
	}
}

// boundary[1]：缓存命中时返回独立切片，调用方改写不污染缓存。
//
// 调**三次**而不是两次：第 1 次走未命中路径、第 2/3 次走命中路径。只验两次会漏掉
// 「未命中时复制、命中时直接返回缓存原件」这种实现——那种实现下第 2 次拿到的是
// 干净的原件，内容断言通过，要到第 3 次才暴露（TASK-010 实测教训）。
//
// []core.OHLCV 的元素是 flat value type（types.go:68 的约束注释），
// 故 slices.Clone 的浅元素拷贝等于深拷贝；元素若将来加了 map/slice/指针字段，
// 那里的 Clone 会静默退化，types.go 已在定义处写明这条约束。
func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	p := &countingProvider{name: "stub", history: sampleBars()}
	c := newGatedCollector(p, builtinGate())
	start, end := testRange()

	for i := 1; i <= 3; i++ {
		data, err := c.FetchHistory("BTC", start, end, "1d")
		if err != nil {
			t.Fatalf("第 %d 次: %v", i, err)
		}
		if len(data) != 2 {
			t.Fatalf("第 %d 次返回 %d 根，want 2", i, len(data))
		}
		if data[0].Close != 1.5 {
			t.Fatalf("第 %d 次拿到被前一次调用改脏的数据：Close=%v，want 1.5", i, data[0].Close)
		}
		// 改脏自己这一份，下一次必须不受影响。
		data[0].Close = -999
	}
	if got := p.count(); got != 1 {
		t.Errorf("本测试前提是三次都走缓存，实际向 provider 取了 %d 次", got)
	}
}

// error_handling：错误不得写进缓存。
//
// 决定性判据是**请求次数**而不是返回的 error：错误被缓存时第二次同样返回 error，
// 断言 error 的写法两种实现都绿。一次瞬时故障若被缓存，会变成整个 TTL 的持续故障。
func TestErrorIsNotCached(t *testing.T) {
	p := &countingProvider{name: "stub", err: errors.New("upstream down")}
	c := newGatedCollector(p, builtinGate())
	start, end := testRange()

	for i := 1; i <= 2; i++ {
		if _, err := c.FetchHistory("BTC", start, end, "1d"); err == nil {
			t.Fatalf("第 %d 次应失败", i)
		}
	}
	if got := p.count(); got != 2 {
		t.Errorf("失败被写进了缓存：两次调用只向 provider 取了 %d 次，"+
			"一次瞬时故障会变成整个 TTL 的持续故障", got)
	}
}

// error_handling：policy 包的错误不得出现在调用方可见的错误链上。
//
// crypto 今天没有 Quota/Timeout，但 config 可达（TopicConfig 能给任何主题加配额），
// 泄漏点数量随接入家数增长。这里显式配一个 Quota 打满来触发。
func TestPolicyErrorDoesNotLeak(t *testing.T) {
	tbl := policy.NewTable()
	p, _ := tbl.Lookup(topicHistory)
	p.TTL = 0
	p.Quota = &policy.Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	tbl.Set("crypto.*", p)

	prov := &countingProvider{name: "stub", history: sampleBars()}
	c := newGatedCollector(prov, policy.New(tbl, policy.NewMemStore()))
	start, end := testRange()

	if _, err := c.FetchHistory("BTC", start, end, "1d"); err != nil {
		t.Fatalf("首次调用应成功（配额未耗尽）: %v", err)
	}
	_, err := c.FetchHistory("ETH", start, end, "1d")
	if err == nil {
		t.Fatal("配额耗尽后应返回错误")
	}
	if errors.Is(err, policy.ErrQuotaExceeded) {
		t.Errorf("policy 包错误外泄到了调用方可见的错误链上: %v", err)
	}
	if prov.count() != 1 {
		t.Errorf("配额耗尽的那次不得到达 provider，实得 %d 次", prov.count())
	}
}
