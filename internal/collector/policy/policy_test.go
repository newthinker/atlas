package policy

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "内置主题齐全且数值平移(yahoo 500ms/tushare 200ms/twelvedata 8s, Coalesce=true)"
//                                                          → TestLookupBuiltinTopics（8 个具体主题）
//                                                          + TestLixingerWildcardTTLOnly（第 9 个登记主题 lixinger.* 的 Coalesce）
// functional[1]     "daily_basic 带 Quota{5,24h,Asia/Shanghai}；其余 tushare 主题 Quota 为 nil"
//                                                          → TestDailyBasicQuota / TestOtherTushareTopicsHaveNoQuota
// functional[2]     "三段查表：精确 → <域>.* 通配 → 未登记；lixinger 只补 TTL"
//                                                          → TestLixingerWildcardTTLOnly（含精确遮蔽通配的次序断言）
// functional[3]     "Set 缺省 Domain 取主题名第一段，显式 Domain 保留"
//                                                          → TestSetOverridesAndDefaultsDomain
// boundary[0]       "ApplyTTL 只提升 TTL>0 的主题且 ttl<=0 为 no-op；DisableTTL 归零 TTL 不动限流"
//                                                          → TestApplyTTLOnlyLiftsCachingTopics / TestApplyTTLNonPositiveIsNoop / TestDisableTTLKeepsThrottle
// boundary[1]       "六个未登记主题 Lookup 一律 ok=false"    → TestLookupUnregisteredTopicIsZeroPolicy
// error_handling[0] 分句1 "LoadLocation 失败时退回 time.UTC 而非 panic"
//                                                          → TestLoadLocFallsBackToUTCWithoutPanic
// error_handling[0] 分句2 "tzdata 经 _ \"time/tzdata\" 嵌入，不依赖部署机装 tzdata"
//                                                          → TestShanghaiLoadsEmbeddedTZData
//   ⚠ 两条互补而非重叠：分句 2 验的是「加载成功」，钉不住分句 1 的失败分支
//     （返工前只有后者，变异 M10 把 return time.UTC 换成 panic(err) 后套件仍全绿）。
// non_functional[0] "policy 包不得 import internal/collector（约束 C3）"
//                                                          → TestNoImportOfCollectorRoot

func TestLookupBuiltinTopics(t *testing.T) {
	tbl := NewTable()

	tests := []struct {
		topic       string
		wantDomain  string
		wantMinIntv time.Duration
	}{
		{"yahoo.chart", "yahoo", 500 * time.Millisecond},
		{"yahoo.eps", "yahoo", 500 * time.Millisecond},
		{"yahoo.quote", "yahoo", 500 * time.Millisecond},
		{"tushare.daily", "tushare", 200 * time.Millisecond},
		{"tushare.index_daily", "tushare", 200 * time.Millisecond},
		{"tushare.hk_daily", "tushare", 200 * time.Millisecond},
		{"tushare.daily_basic", "tushare", 200 * time.Millisecond},
		{"twelvedata.time_series", "twelvedata", 8 * time.Second},
	}
	for _, tt := range tests {
		p, ok := tbl.Lookup(tt.topic)
		if !ok {
			t.Fatalf("%s: 应为内置主题", tt.topic)
		}
		if p.Domain != tt.wantDomain {
			t.Errorf("%s: Domain = %q, want %q", tt.topic, p.Domain, tt.wantDomain)
		}
		if p.MinInterval != tt.wantMinIntv {
			t.Errorf("%s: MinInterval = %v, want %v", tt.topic, p.MinInterval, tt.wantMinIntv)
		}
		if !p.Coalesce {
			t.Errorf("%s: 登记主题默认应开启 Coalesce", tt.topic)
		}
	}
}

func TestLookupUnregisteredTopicIsZeroPolicy(t *testing.T) {
	tbl := NewTable()
	for _, topic := range []string{"eastmoney.kline", "akshare.valuation", "crypto.ticker", "fred.series", "edgar.facts", "baostock.daily"} {
		if _, ok := tbl.Lookup(topic); ok {
			t.Errorf("%s: 未登记主题不应命中策略表（设计 §4.1）", topic)
		}
	}
}

func TestDailyBasicQuota(t *testing.T) {
	p, ok := NewTable().Lookup("tushare.daily_basic")
	if !ok {
		t.Fatal("tushare.daily_basic 应为内置主题")
	}
	if p.Quota == nil {
		t.Fatal("tushare.daily_basic 必须带日配额（ea5ac30 实测 5 次/天）")
	}
	if p.Quota.Limit != 5 {
		t.Errorf("Limit = %d, want 5", p.Quota.Limit)
	}
	if p.Quota.Window != 24*time.Hour {
		t.Errorf("Window = %v, want 24h", p.Quota.Window)
	}
	// 断言具体值而非「不是 UTC」：DoD 写明了 Asia/Shanghai，排除法只在候选集
	// 二元时才等价。Loc 错成别的非 UTC 时区（如 Asia/Tokyo）会让配额在错误时刻
	// 重置自然日窗口——tushare 的 5 次/天是北京时间口径。
	if p.Quota.Loc == nil || p.Quota.Loc.String() != "Asia/Shanghai" {
		t.Errorf("Loc = %v, want Asia/Shanghai", p.Quota.Loc)
	}
}

func TestOtherTushareTopicsHaveNoQuota(t *testing.T) {
	tbl := NewTable()
	for _, topic := range []string{"tushare.daily", "tushare.index_daily", "tushare.hk_daily"} {
		p, _ := tbl.Lookup(topic)
		if p.Quota != nil {
			t.Errorf("%s: 只有 daily_basic 有实测配额，其余不得凭空设限", topic)
		}
	}
}

func TestLixingerWildcardTTLOnly(t *testing.T) {
	tbl := NewTable()
	p, ok := tbl.Lookup("lixinger.cn/company/fundamental/non_financial")
	if !ok {
		t.Fatal("lixinger 端点应命中 lixinger.* 通配主题")
	}
	if p.TTL <= 0 {
		t.Error("lixinger 主题必须有 TTL（修复 §1.3 缺陷）")
	}
	if p.MinInterval != 0 || p.Quota != nil {
		t.Errorf("lixinger 只补缓存，不得新增限流/配额: MinInterval=%v Quota=%v", p.MinInterval, p.Quota)
	}
	if p.Domain != "lixinger" {
		t.Errorf("Domain = %q, want lixinger", p.Domain)
	}
	// lixinger.* 是第 9 个登记主题，TestLookupBuiltinTopics 只覆盖了另外 8 个。
	// functional[0] 要求「登记主题 Coalesce 默认 true」，这里补上最后一个。
	if !p.Coalesce {
		t.Error("lixinger.* 也是登记主题，Coalesce 应为 true")
	}

	// 精确条目必须遮蔽同域通配条目（functional[2] 指定的查表次序）。
	// 今天没有任何域同时存在精确与通配条目，故次序对调时行为等价、测不出来；
	// 但 config 覆盖一旦给 lixinger 追加精确主题，次序错误会静默生效错策略。
	tbl.Set("lixinger.exact", Policy{TTL: time.Minute, MinInterval: 3 * time.Second})
	if p, _ := tbl.Lookup("lixinger.exact"); p.MinInterval != 3*time.Second {
		t.Errorf("精确匹配须优先于 <域>.* 通配, got %+v", p)
	}
}

func TestSetOverridesAndDefaultsDomain(t *testing.T) {
	tbl := NewTable()
	tbl.Set("yahoo.chart", Policy{MinInterval: time.Second, TTL: time.Minute})
	p, _ := tbl.Lookup("yahoo.chart")
	if p.MinInterval != time.Second || p.TTL != time.Minute {
		t.Errorf("Set 未生效: %+v", p)
	}
	if p.Domain != "yahoo" {
		t.Errorf("Domain 应缺省取主题名第一段, got %q", p.Domain)
	}

	tbl.Set("custom.x", Policy{Domain: "shared", MinInterval: time.Second})
	if p, _ := tbl.Lookup("custom.x"); p.Domain != "shared" {
		t.Errorf("显式 Domain 应被保留, got %q", p.Domain)
	}
}

func TestApplyTTLOnlyLiftsCachingTopics(t *testing.T) {
	tbl := NewTable()
	tbl.ApplyTTL(90 * time.Second)

	if p, _ := tbl.Lookup("yahoo.chart"); p.TTL != 90*time.Second {
		t.Errorf("yahoo.chart TTL = %v, want 90s", p.TTL)
	}
	if p, _ := tbl.Lookup("yahoo.quote"); p.TTL != 0 {
		t.Errorf("yahoo.quote 是实时主题，TTL 必须保持 0, got %v", p.TTL)
	}
}

// TestApplyTTLNonPositiveIsNoop 覆盖 boundary[0] 的「ttl<=0 时为 no-op」：
// 未配置 TTL 不应把内置兜底值抹掉。
func TestApplyTTLNonPositiveIsNoop(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		tbl := NewTable()
		before, _ := tbl.Lookup("yahoo.chart")
		tbl.ApplyTTL(ttl)
		after, _ := tbl.Lookup("yahoo.chart")
		if after.TTL != before.TTL {
			t.Errorf("ApplyTTL(%v) 应为 no-op: TTL %v → %v", ttl, before.TTL, after.TTL)
		}
	}
}

func TestDisableTTLKeepsThrottle(t *testing.T) {
	tbl := NewTable()
	tbl.DisableTTL()

	// 三层断言，每层守一个**不同的被测对象**，不可合并也不可省略。
	//
	// ① 对 Topics() 做**集合等值**。此前用的「基数 + 逐元素 Lookup 命中」有三条
	//    实测逃逸：返 9 个 lixinger.* 域内假名（Lookup 是三段查表，任何
	//    lixinger.<任意> 都经通配段命中，该谓词**宽于**「已登记主题」）、返同一
	//    主题重复 9 次、以及前者与「DisableTTL 只清 lixinger.*」组合后全绿逃逸。
	//    集合等值不给任何逼近的余地。want 的 9 个主题名从实现直读
	//    （grep 't\.Set("' policy.go），不凭记忆写。
	want := []string{
		"yahoo.chart", "yahoo.eps", "yahoo.quote",
		"tushare.daily", "tushare.index_daily", "tushare.hk_daily", "tushare.daily_basic",
		"twelvedata.time_series", "lixinger.*",
	}
	got := tbl.Topics()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("Topics() = %v, want %v", got, want)
	}

	for _, topic := range got {
		// ② 守 Lookup() 本身。**不能因为有了 ① 就删掉这条**：若 Lookup 恒返
		//    ok=false，① 仍通过（Topics() 是对的）、③ 也仍通过（p 是零值、TTL
		//    恰好为 0），漏检。强弱只在同一守护对象内可比。
		p, ok := tbl.Lookup(topic)
		if !ok {
			t.Errorf("%s: Topics() 返回的主题必须能被 Lookup 命中", topic)
			continue
		}
		// ③ 守 DisableTTL()。
		if p.TTL != 0 {
			t.Errorf("%s: cache.enabled=false 时所有 TTL 须归零, got %v", topic, p.TTL)
		}
	}
	if p, _ := tbl.Lookup("yahoo.chart"); p.MinInterval != 500*time.Millisecond {
		t.Errorf("限流不受缓存开关影响, got %v", p.MinInterval)
	}
}

// TestLoadLocFallsBackToUTCWithoutPanic 覆盖 error_handling[0] 的分句 1：
// 时区加载失败时退回 time.UTC 而非 panic——配额账本的时区退化不值得让进程起不来。
//
// 该分支在 shanghai() 里不可达（时区名是写死的字面量且 tzdata 已嵌入），
// 故把时区名抽成 loadLoc 的参数后才能直接喂一个必然失败的名字。
func TestLoadLocFallsBackToUTCWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("时区加载失败不得 panic: %v", r)
		}
	}()

	if got := loadLoc("Not/AZone"); got != time.UTC {
		t.Errorf("加载失败应退回 time.UTC, got %v", got)
	}
}

// TestShanghaiLoadsEmbeddedTZData 覆盖 error_handling[0] 的分句 2：tzdata 已嵌入，
// 即便部署机没装 tzdata（模拟为把 ZONEINFO 指向不存在的路径）也应拿到
// Asia/Shanghai；万一加载失败也只能退回 UTC，绝不 panic。
func TestShanghaiLoadsEmbeddedTZData(t *testing.T) {
	t.Setenv("ZONEINFO", "/nonexistent/zoneinfo.zip")

	loc := shanghai()
	if loc == nil {
		t.Fatal("shanghai() 不得返回 nil")
	}
	if loc.String() != "Asia/Shanghai" {
		t.Errorf("嵌入 tzdata 后应拿到 Asia/Shanghai, got %q", loc.String())
	}
}

// TestNoImportOfCollectorRoot 覆盖 non_functional[0]（约束 C3）：policy 包
// 不得依赖 internal/collector——Gate 由各 collector 反向调用，包一旦回指就是循环导入。
func TestNoImportOfCollectorRoot(t *testing.T) {
	const (
		selfPkg       = "github.com/newthinker/atlas/internal/collector/policy"
		collectorRoot = "github.com/newthinker/atlas/internal/collector"
	)

	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		// 只有「PATH 里根本没有 go」才允许跳过（如 go test -c 后脱离工具链单独执行）。
		// 其余错误（依赖解析失败等）本应转红——一律 Skip 会让 C3 守卫静默失效，
		// 那与 M10「守卫看起来在、实际不设防」是同一类问题。
		if errors.Is(err, exec.ErrNotFound) {
			t.Skipf("PATH 中没有 go，无法执行 C3 依赖约束检查: %v", err)
		}
		t.Fatalf("go list -deps 失败，C3 约束无法验证: %v", err)
	}

	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dep = strings.TrimSpace(dep)
		// 前缀匹配的陷阱：policy 包自身也以 collectorRoot 为前缀，须先排除。
		if dep == "" || dep == selfPkg {
			continue
		}
		if dep == collectorRoot || strings.HasPrefix(dep, collectorRoot+"/") {
			t.Errorf("policy 不得依赖 collector 包（约束 C3 循环导入）: %s", dep)
		}
	}
}

// ——— Table.Override（TASK-005）———
//
// functional[1] 只应用显式设置的字段 → TestOverrideAppliesOnlySetFields
//                                     + TestOverrideExplicitZeroValues（指针语义的关键）
// functional[2] 只覆盖 QuotaLimit 时保留 Window/Loc → TestOverrideQuotaLimitKeepsWindowAndLoc
// boundary[0]   可为无 Quota 的主题新增配额     → TestOverrideCanAddQuotaToTopicWithout
// boundary[1]   Override 未登记主题会将其登记   → TestOverrideRegistersUnknownTopic
// （自补）Quota 必须复制而非就地改             → TestOverrideCopiesQuotaNotShares
// （自补）Domain 须按主题名重推，不从通配条目继承 → TestOverrideRecomputesDomain

func TestOverrideAppliesOnlySetFields(t *testing.T) {
	tbl := NewTable()
	ttl := 30 * time.Second
	tbl.Override("yahoo.chart", Override{TTL: &ttl})

	p, _ := tbl.Lookup("yahoo.chart")
	if p.TTL != ttl {
		t.Errorf("TTL = %v, want %v", p.TTL, ttl)
	}
	if p.MinInterval != 500*time.Millisecond {
		t.Errorf("未设置的字段应保持内置值, MinInterval = %v", p.MinInterval)
	}
	if !p.Coalesce {
		t.Error("未设置的 Coalesce 应保持内置 true")
	}
}

// TestOverrideExplicitZeroValues 是**指针语义的立身之本**：Override 全用指针字段，
// 就是为了区分「未设置」与「显式设为零值」。用零值判定的实现（if o.TTL != 0）会让
// 这条红——而 TTL: 0 是合法取值，表示显式关掉该主题的缓存。
func TestOverrideExplicitZeroValues(t *testing.T) {
	tbl := NewTable()
	zeroTTL := time.Duration(0)
	no := false
	tbl.Override("yahoo.chart", Override{TTL: &zeroTTL, Coalesce: &no})

	p, _ := tbl.Lookup("yahoo.chart")
	if p.TTL != 0 {
		t.Errorf("显式 TTL=0 应关掉该主题的缓存, got %v", p.TTL)
	}
	if p.Coalesce {
		t.Error("显式 Coalesce=false 应关掉合并")
	}
	if p.MinInterval != 500*time.Millisecond {
		t.Errorf("未设置的字段仍应保持内置值, MinInterval = %v", p.MinInterval)
	}
}

func TestOverrideQuotaLimitKeepsWindowAndLoc(t *testing.T) {
	tbl := NewTable()
	before, _ := tbl.Lookup("tushare.daily_basic")
	limit := 20
	tbl.Override("tushare.daily_basic", Override{QuotaLimit: &limit})

	p, _ := tbl.Lookup("tushare.daily_basic")
	if p.Quota == nil || p.Quota.Limit != 20 {
		t.Fatalf("Quota = %+v, want Limit 20", p.Quota)
	}
	if p.Quota.Window != before.Quota.Window || p.Quota.Loc != before.Quota.Loc {
		t.Errorf("只改 limit 时 Window/Loc 应保持: %+v", p.Quota)
	}
}

func TestOverrideCanAddQuotaToTopicWithout(t *testing.T) {
	tbl := NewTable()
	limit, window := 100, time.Minute
	tbl.Override("yahoo.chart", Override{QuotaLimit: &limit, QuotaWindow: &window})

	p, _ := tbl.Lookup("yahoo.chart")
	if p.Quota == nil || p.Quota.Limit != 100 || p.Quota.Window != time.Minute {
		t.Fatalf("Quota = %+v", p.Quota)
	}
	if p.Quota.Loc == nil {
		t.Error("新建 Quota 必须带时区（自然日边界对齐）")
	}
}

func TestOverrideRegistersUnknownTopic(t *testing.T) {
	tbl := NewTable()
	iv := 3 * time.Second
	tbl.Override("eastmoney.kline", Override{MinInterval: &iv})

	p, ok := tbl.Lookup("eastmoney.kline")
	if !ok {
		t.Fatal("config 覆盖应能登记新主题")
	}
	if p.MinInterval != iv || p.Domain != "eastmoney" {
		t.Errorf("p = %+v", p)
	}
}

// TestOverrideCopiesQuotaNotShares 守护「复制而非共享」：Policy.Quota 是 *Quota，
// 多个主题可能引用同一个。就地改会连带改掉别的主题的配额。
func TestOverrideCopiesQuotaNotShares(t *testing.T) {
	tbl := NewTable()
	shared := &Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC}
	tbl.Set("a.x", Policy{Quota: shared})
	tbl.Set("a.y", Policy{Quota: shared})

	limit := 99
	tbl.Override("a.x", Override{QuotaLimit: &limit})

	px, _ := tbl.Lookup("a.x")
	py, _ := tbl.Lookup("a.y")
	if px.Quota.Limit != 99 {
		t.Errorf("a.x 的 Limit = %d, want 99", px.Quota.Limit)
	}
	if py.Quota.Limit != 5 {
		t.Errorf("a.y 的 Limit = %d, want 5（Override 须复制 Quota，不得就地改共享实例）", py.Quota.Limit)
	}
	if shared.Limit != 5 {
		t.Errorf("原 *Quota 被就地修改了: Limit = %d, want 5", shared.Limit)
	}
}

// TestOverrideRecomputesDomain 守护「Domain 按主题名重推」：Lookup 命中通配条目时
// 返回的是通配条目的 Domain，直接 Set 回去会让新主题继承错误的限流域。
func TestOverrideRecomputesDomain(t *testing.T) {
	tbl := NewTable()
	// 通配条目显式指定了一个与主题名不同的域
	tbl.Set("shared.*", Policy{Domain: "custom-domain", TTL: time.Minute})

	iv := time.Second
	tbl.Override("shared.endpoint", Override{MinInterval: &iv})

	p, _ := tbl.Lookup("shared.endpoint")
	if p.Domain != "shared" {
		t.Errorf("Domain = %q, want \"shared\"（从通配条目继承 Domain 会让限流域串味）", p.Domain)
	}
}
