// Package policy 提供 collector 的准入策略中介层：把散落在各 client 内部的
// 限流闸门、TTL 缓存、在途请求合并与跨进程配额收敛到一处。
//
// 本包**刻意不依赖** internal/collector（避免循环导入）：Gate 不包装 Collector
// 接口，而是由各 collector 在发 HTTP 请求处直接调用，扩展接口因此不会被遮蔽
// （设计 §3.1 / §3.2）。
package policy

import (
	"strings"
	"time"

	// 嵌入 IANA 时区库：配额的自然日边界依赖 Asia/Shanghai，
	// 不能指望部署机（尤其容器）装了 tzdata。
	_ "time/tzdata"
)

// builtinTTL 是内置表里「OHLCV 类主题」的兜底 TTL。运行期会被
// config 的 collector.cache.ttl 经 ApplyTTL 覆盖（设计 §4.2）。
const builtinTTL = 5 * time.Minute

// Quota 描述一个主题在固定窗口内的调用上限。
//
// Window >= 24h 时按 Loc 的自然日对齐（今天 00:00 起算），否则按 UTC 截断到
// Window 的整数倍——tushare 的 5 次/天是自然日口径，分钟级窗口无时区含义。
type Quota struct {
	Limit  int
	Window time.Duration
	Loc    *time.Location
}

// Policy 是单个主题的策略。零值 = 零策略（不缓存、不限流、不计配额）。
type Policy struct {
	// Domain 是限流域：多个主题可共享同一个服务端闸门
	// （yahoo.chart 与 yahoo.eps 同域）。缺省取主题名第一段（设计 §3.3）。
	Domain      string
	TTL         time.Duration // 0 = 不缓存
	MinInterval time.Duration // 0 = 不节流
	Coalesce    bool          // 合并同 key 的在途请求
	Quota       *Quota        // nil = 不计配额
	Timeout     time.Duration // 0 = 不设超时，作用于 fn 本身
}

// Table 是主题 → Policy 的查找表，构造后只读（config 覆盖发生在构造阶段）。
type Table struct {
	policies map[string]Policy
}

// shanghai 返回配额自然日对齐用的时区。tzdata 已嵌入，加载失败只可能是
// 时区名写错，此时退回 UTC 而不是 panic——配额账本不值得让进程起不来。
func shanghai() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return loc
}

// NewTable 返回装好内置策略的表（设计 §4.2，数值全部从现有常量平移）。
func NewTable() *Table {
	t := &Table{policies: make(map[string]Policy)}

	yahooPolicy := Policy{MinInterval: 500 * time.Millisecond, TTL: builtinTTL, Coalesce: true}
	t.Set("yahoo.chart", yahooPolicy)
	t.Set("yahoo.eps", yahooPolicy)
	// FetchQuote 保持「实时不缓存」语义，但共享同一个 500ms 闸门。
	t.Set("yahoo.quote", Policy{MinInterval: 500 * time.Millisecond, TTL: 0, Coalesce: true})

	tusharePolicy := Policy{MinInterval: 200 * time.Millisecond, TTL: builtinTTL, Coalesce: true}
	t.Set("tushare.daily", tusharePolicy)
	t.Set("tushare.index_daily", tusharePolicy)
	t.Set("tushare.hk_daily", tusharePolicy)
	// daily_basic 的 5 次/天是 ea5ac30 实测值；其余接口未实测，不凭空设限。
	dailyBasic := tusharePolicy
	dailyBasic.Quota = &Quota{Limit: 5, Window: 24 * time.Hour, Loc: shanghai()}
	t.Set("tushare.daily_basic", dailyBasic)

	t.Set("twelvedata.time_series", Policy{MinInterval: 8 * time.Second, TTL: builtinTTL, Coalesce: true})

	// lixinger 是 §1.3 要修复的对象：它今天完全拿不到缓存。只补 TTL，
	// 不新增任何它今天没有的限流行为。端点形如 cn/company/fundamental/
	// non_financial，共 8 个且会增长，故用通配主题登记。
	t.Set("lixinger.*", Policy{TTL: builtinTTL, Coalesce: true})

	return t
}

// Set 登记或覆盖一个主题。Domain 为空时缺省取主题名第一段。
func (t *Table) Set(topic string, p Policy) {
	if p.Domain == "" {
		p.Domain = domainOf(topic)
	}
	t.policies[topic] = p
}

// Lookup 查表：精确匹配 → `<域>.*` 通配 → 未登记（零策略）。
func (t *Table) Lookup(topic string) (Policy, bool) {
	if p, ok := t.policies[topic]; ok {
		return p, true
	}
	if p, ok := t.policies[domainOf(topic)+".*"]; ok {
		return p, true
	}
	return Policy{}, false
}

// Topics 返回所有已登记的主题名（顺序不定，供测试与诊断用）。
func (t *Table) Topics() []string {
	out := make([]string, 0, len(t.policies))
	for topic := range t.policies {
		out = append(out, topic)
	}
	return out
}

// ApplyTTL 把 config 的缓存 TTL 施加到所有**本来就缓存**的主题上。
// TTL 已为 0 的主题（如 yahoo.quote）保持不缓存的语义，不被提升。
func (t *Table) ApplyTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	for topic, p := range t.policies {
		if p.TTL > 0 {
			p.TTL = ttl
			t.policies[topic] = p
		}
	}
}

// DisableTTL 对应 config 的 collector.cache.enabled=false：所有主题 TTL 归零，
// 等价于今天 maybeCache 直接返回原 collector。限流与配额不受影响（设计 §4.2）。
func (t *Table) DisableTTL() {
	for topic, p := range t.policies {
		p.TTL = 0
		t.policies[topic] = p
	}
}

// domainOf 取主题名第一段作为限流域。无 '.' 时整个主题名即是域。
func domainOf(topic string) string {
	if i := strings.Index(topic, "."); i >= 0 {
		return topic[:i]
	}
	return topic
}
