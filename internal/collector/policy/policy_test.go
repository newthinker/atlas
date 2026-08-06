package policy

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "内置主题齐全且数值平移(yahoo 500ms/tushare 200ms/twelvedata 8s, Coalesce=true)"
//                                                          → TestLookupBuiltinTopics
// functional[1]     "daily_basic 带 Quota{5,24h,Asia/Shanghai}；其余 tushare 主题 Quota 为 nil"
//                                                          → TestDailyBasicQuota / TestOtherTushareTopicsHaveNoQuota
// functional[2]     "三段查表：精确 → <域>.* 通配 → 未登记；lixinger 只补 TTL"
//                                                          → TestLixingerWildcardTTLOnly
// functional[3]     "Set 缺省 Domain 取主题名第一段，显式 Domain 保留"
//                                                          → TestSetOverridesAndDefaultsDomain
// boundary[0]       "ApplyTTL 只提升 TTL>0 的主题且 ttl<=0 为 no-op；DisableTTL 归零 TTL 不动限流"
//                                                          → TestApplyTTLOnlyLiftsCachingTopics / TestApplyTTLNonPositiveIsNoop / TestDisableTTLKeepsThrottle
// boundary[1]       "六个未登记主题 Lookup 一律 ok=false"    → TestLookupUnregisteredTopicIsZeroPolicy
// error_handling[0] "shanghai() 加载失败退回 UTC 而非 panic；tzdata 已嵌入"
//                                                          → TestShanghaiLoadsEmbeddedTZData
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
	if p.Quota.Loc == nil || p.Quota.Loc.String() == "UTC" {
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
	for _, topic := range tbl.Topics() {
		p, _ := tbl.Lookup(topic)
		if p.TTL != 0 {
			t.Errorf("%s: cache.enabled=false 时所有 TTL 须归零, got %v", topic, p.TTL)
		}
	}
	if p, _ := tbl.Lookup("yahoo.chart"); p.MinInterval != 500*time.Millisecond {
		t.Errorf("限流不受缓存开关影响, got %v", p.MinInterval)
	}
}

// TestShanghaiLoadsEmbeddedTZData 覆盖 error_handling[0]：tzdata 已嵌入，
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
		t.Skipf("go list 不可用，跳过依赖约束检查: %v", err)
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
