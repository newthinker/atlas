# Collector 策略闸门与路由表 实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用新包 `internal/collector/policy` 的 Gate 中介层统一承接各 collector 的限流／缓存／合并／配额，并把散落的符号路由谓词收敛为一张具体度优先的路由表。

**Architecture:** Gate **不包装 collector**，而是由各 collector 在发 HTTP 请求处调用 `policy.Fetch[T]` / `Gate.Do`，执行链为「TTL 缓存 → singleflight 合并 → 配额预判 → 限流域节流 → 执行 fn」。配额通过 flock + JSON 账本跨进程持久化，以在 `atlas prism refresh` 这种短命进程形态下生效。旧的 `CachedCollector` 装饰器与 `maybeCache` 一并删除，缓存下沉到 HTTP 调用处，顺带修复 lixinger 从未进入缓存路径的既有缺陷。

**Tech Stack:** Go 1.24.4、`golang.org/x/sync/singleflight`（`go.mod:18` 已有）、`syscall.Flock`、`time/tzdata`（标准库嵌入时区）。

**设计文档：** `docs/superpowers/specs/2026-08-06-collector-policy-gate-design.md`

## Global Constraints

- **零新增第三方依赖**：只用标准库 + `golang.org/x/sync`（`go.mod:18` 已存在 `v0.16.0`）。
- **`internal/prism/refresh.go` 零改动**（设计 §5、验收标准 1）。配额错误必须在 collector 内部映射为 `tushare.ErrRateLimited`，`policy` 包的错误不得外泄到 `prism` 层。
- **`internal/collector/policy` 不得 import `internal/collector`**（避免循环，设计 §3.1）。
- **公开路由 API 签名不变**：`SelectForSymbol` / `SelectExternalForSymbol` / `MarketForSymbol` / `KnownIndexMarket`（设计 §6.1）。6 个外部调用点零改动。
- **限流数值全部从现有常量平移，不做调整**：yahoo 500ms、tushare 200ms、twelvedata 8s（设计 §4.2）。
- **未登记的主题 = 零策略**（不缓存、不限流、不计配额），`eastmoney` / `akshare` / `crypto` / `fred` / `edgar` / `baostock` 行为零变更（设计 §4.1）。
- **配额账本异常一律 fail-open**：放行 + 告警，绝不阻断降级链（设计 §4.4）。
- 每个任务结束时必须 `go build ./... && go vet ./...` 通过再提交。
- 提交信息用中文，格式 `<type>(collector): <描述>`，与仓库既有风格一致。

---

## 设计文档之外的实现决定（偏离与补充）

执行时按本节口径实现；这些是设计文档未定死、但实现必须确定的点。

1. **`Gate.Wait(topic)` 第三个入口**（设计 §3.2 只列了两个）。理由：yahoo 的重试循环今天每次 attempt 都调 `throttle()`，若只在 `Fetch` 入口节流一次，重试请求会绕过闸门且 `lastReq` 失准。`Wait` 只施加节流，不碰缓存／配额／合并。约定：`Fetch` 在调用 `fn` **前**节流一次，`fn` 内部的重试循环只在 `attempt > 0` 时再调 `Wait`，避免首次双倍等待。
2. **新增 `yahoo.quote` 主题**（设计 §4.2 表未列）。理由：`FetchQuote` 今天走同一个 `do()` 闸门，不登记就会丢掉 500ms 节流。策略 = `{Domain: yahoo, MinInterval: 500ms, TTL: 0, Coalesce: true}`，TTL 0 兑现设计 §4.2「FetchQuote 类主题 TTL = 0」。
3. **`Table.Lookup` 支持 `<域>.*` 通配兜底**。理由：lixinger 有 8 个 endpoint 且形如 `cn/company/fundamental/non_financial`，逐条登记不可维护。查表顺序：精确匹配 → `<主题名第一段>.*` → 未登记。
4. **`QuotaStore.Take` 签名带 `Quota` 结构**而非设计 §3.4 的 `(topic, limit, window, now)`：`Quota.Loc`（自然边界对齐时区）必须传进去，拆成四个参数会漏掉它。
5. **缓存命中返回同一底层数组**。`Fetch` 不复制返回值（无法对任意 `T` 通用地深拷贝）。因此**返回切片的 collector 必须在 `Fetch` 之后自行 `slices.Clone`**，替代旧 `CachedCollector.cloneOHLCV` 的保护。各任务已在步骤中写明。
6. **配额账本路径**：`data/collector-quota.json`（与 `storage.signals.path` 的 `data/signals.db` 同风格），可经 `collector.quota.path` 覆盖。仓库 config 无 `data_dir` 字段，不为此新增。
7. **进程内单例注入方式**：`policy` 包持有懒初始化的全局默认 Gate（`policy.Default()`），启动时由 `policy.SetDefault(g)` 替换。各 collector 在**构造函数里**取 `policy.Default()` 存入私有字段，测试通过同包内赋值该字段注入测试 Gate。要求 `SetDefault` 必须在构造 collector 之前调用（serve / prism 两条路径均满足，任务 5 会核实）。
8. **`syscall.Flock` 仅在 unix 可用**。本仓库只在 darwin/linux 运行（launchd 部署），文件加锁不加 build tag；若将来需要 Windows 支持再补 `_unix.go` / `_other.go` 拆分。

---

## File Structure

**新建**

| 文件 | 职责 |
|---|---|
| `internal/collector/policy/policy.go` | `Policy` / `Quota` / `Table` 结构，内置策略表与查表 |
| `internal/collector/policy/gate.go` | `Gate`：准入控制 + TTL 缓存，`Fetch[T]` / `Do` / `Wait` |
| `internal/collector/policy/quota.go` | `QuotaStore` 接口、`ErrQuotaExceeded`、`MemStore` |
| `internal/collector/policy/quota_file.go` | `FileStore`：flock + 原子写的跨进程账本 |
| `internal/collector/policy/default.go` | 进程内单例 `Default()` / `SetDefault()` |
| `internal/collector/route.go` | `Route` 结构、glob 匹配、具体度排序、内置路由表 |
| `cmd/atlas/policy.go` | 从 config 构建 Gate 并 `SetDefault` |

**修改**

| 文件 | 改动 |
|---|---|
| `internal/collector/yahoo/yahoo.go` | 删 `mu`/`lastReq`/`minInterval`/`throttle()`，改走 Gate |
| `internal/collector/yahoo/eps.go` | `do` 调用点带上主题名 |
| `internal/collector/twelvedata/client.go` | 删 `mu`/`lastReq`/`minInterval`/`throttle()`，改走 Gate |
| `internal/collector/tushare/client.go` | 删 `mu`/`lastReq`，`call()` 走 Gate，配额错误映射 `ErrRateLimited` |
| `internal/collector/lixinger/lixinger.go` `client.go` | `request()` 走 Gate（仅 TTL） |
| `internal/collector/selector.go` | 五个谓词改为查 `route.go` 的表 |
| `internal/config/config.go` | `CollectorGlobalConfig` 增 `Quota` / `Topics` |
| `cmd/atlas/serve.go` | 删 `maybeCache`；配置装载后 `SetDefault` |
| `cmd/atlas/collectors.go` | 去掉 `maybeCache` 包装 |
| `cmd/atlas/export_ohlcv.go` | `loadConfigOrDefaults` 后 `SetDefault` |

**删除**

| 文件 | 原因 |
|---|---|
| `internal/collector/cache.go` | `CachedCollector` 被 Gate 取代（设计 §3.5） |
| `internal/collector/cache_test.go` | 随之删除，能力已由 `policy` 包测试覆盖 |
| `internal/collector/yahoo/throttle_test.go` 的节流用例 | 迁移到 Gate（重试用例保留） |

---

## 任务索引

| # | 任务 | 交付物 |
|---|---|---|
| 1 | policy 包：策略表与查表 | `policy.go` + 测试 |
| 2 | Gate：节流 / 合并 / TTL / 超时 | `gate.go` + 测试 |
| 3 | QuotaStore 接口 + 内存实现 + Gate 接线 | `quota.go` + 测试 |
| 4 | QuotaStore 文件实现（跨进程） | `quota_file.go` + 测试 |
| 5 | 全局单例 + config 接线 | `default.go`、`cmd/atlas/policy.go`、config |
| 6 | yahoo 接入 Gate | yahoo 改造 + 测试迁移 |
| 7 | twelvedata 接入 Gate | twelvedata 改造 + 测试迁移 |
| 8 | tushare 接入 Gate + 配额 + 错误映射 | tushare 改造 + 降级链兼容测试 |
| 9 | lixinger 接入 Gate（仅 TTL） | lixinger 改造 + 测试 |
| 10 | 删除 `CachedCollector` 与 `maybeCache` | 装配层改造 |
| 11 | 路由表重写 | 黄金值测试 + `route.go` |
| 12 | 防回潮 AST 测试 + prism 集成回归 | 两条兜底测试 |

任务 1→5 严格顺序；6/7/8/9 相互独立可并行；10 依赖 6-9 全部完成；11 与前十项完全独立，可任意时点插入；12 最后。

---

### Task 1: policy 包 —— 策略表与查表

**Files:**
- Create: `internal/collector/policy/policy.go`
- Test: `internal/collector/policy/policy_test.go`

**Interfaces:**
- Consumes: 无（本任务是包根）
- Produces:
  - `type Quota struct { Limit int; Window time.Duration; Loc *time.Location }`
  - `type Policy struct { Domain string; TTL, MinInterval time.Duration; Coalesce bool; Quota *Quota; Timeout time.Duration }`
  - `type Table struct { ... }`
  - `func NewTable() *Table`
  - `func (t *Table) Set(topic string, p Policy)`
  - `func (t *Table) Lookup(topic string) (Policy, bool)`
  - `func (t *Table) ApplyTTL(ttl time.Duration)`
  - `func (t *Table) DisableTTL()`
  - `func (t *Table) Topics() []string`

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/policy/policy_test.go`：

```go
package policy

import (
	"testing"
	"time"
)

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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/policy/ -run 'TestLookup|TestDailyBasic|TestOtherTushare|TestLixinger|TestSet|TestApplyTTL|TestDisableTTL' -v`
Expected: 编译失败 —— `undefined: NewTable`（`policy.go` 尚不存在）

- [ ] **Step 3: 写实现**

创建 `internal/collector/policy/policy.go`：

```go
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
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/collector/policy/ -v`
Expected: PASS（8 个测试全绿）

- [ ] **Step 5: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/policy/policy.go internal/collector/policy/policy_test.go
git commit -m "feat(collector): policy 包策略表与查表（内置主题 + 域缺省 + 通配兜底）"
```

---

### Task 2: Gate —— 节流 / 合并 / TTL / 超时

**Files:**
- Create: `internal/collector/policy/gate.go`
- Test: `internal/collector/policy/gate_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Table` / `Policy` / `Table.Lookup`
- Produces:
  - `type Gate struct { ... }`
  - `type Option func(*Gate)`
  - `func WithWarn(f func(msg string, err error)) Option`
  - `func New(t *Table, opts ...Option) *Gate` —— 本任务的签名。`QuotaStore` 参数由 Task 3 加入，届时变为 `New(t *Table, q QuotaStore, opts ...Option)`
  - `func Fetch[T any](g *Gate, topic, key string, fn func() (T, error)) (T, error)`
  - `func (g *Gate) Do(topic, key string, fn func() error) error`
  - `func (g *Gate) Wait(topic string)`
  - `var ErrTimeout = errors.New("policy: fn timeout")`

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/policy/gate_test.go`：

```go
package policy

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// newTestGate 返回一个只含 topics 的 Gate（不含内置表），避免测试受生产
// 数值（8s 等）拖慢。
func newTestGate(t *testing.T, topics map[string]Policy) *Gate {
	t.Helper()
	tbl := &Table{policies: make(map[string]Policy)}
	for topic, p := range topics {
		tbl.Set(topic, p)
	}
	return New(tbl)
}

func TestGateThrottlesSameDomain(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {MinInterval: 80 * time.Millisecond},
		"a.y": {MinInterval: 80 * time.Millisecond},
	})
	var calls []time.Time
	record := func() (int, error) { calls = append(calls, time.Now()); return 0, nil }

	if _, err := Fetch(g, "a.x", "k", record); err != nil {
		t.Fatal(err)
	}
	// 同域不同主题也必须共享闸门（设计 §3.3）
	if _, err := Fetch(g, "a.y", "k", record); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if gap := calls[1].Sub(calls[0]); gap < 60*time.Millisecond {
		t.Errorf("同域相邻请求间隔 = %v, want >= 60ms（留计时容差）", gap)
	}
}

func TestGateThrottleIsPerDomain(t *testing.T) {
	g := newTestGate(t, map[string]Policy{
		"a.x": {MinInterval: 300 * time.Millisecond},
		"b.x": {MinInterval: 300 * time.Millisecond},
	})
	noop := func() (int, error) { return 0, nil }
	if _, err := Fetch(g, "a.x", "k", noop); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := Fetch(g, "b.x", "k", noop); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("跨域不应互相阻塞, 第二次耗时 %v", elapsed)
	}
}

func TestGateCoalescesConcurrentSameKey(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Coalesce: true}})
	var mu sync.Mutex
	fnCalls := 0
	fn := func() (string, error) {
		mu.Lock()
		fnCalls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond) // 撑开在途窗口
		return "value", nil
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = Fetch(g, "a.x", "same", fn)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if fnCalls != 1 {
		t.Errorf("fn 被调用 %d 次, want 1（singleflight 合并）", fnCalls)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil || results[i] != "value" {
			t.Fatalf("caller %d: got (%q, %v), want (\"value\", nil)", i, results[i], errs[i])
		}
	}
}

func TestGateCoalesceIsPerKey(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Coalesce: true}})
	var mu sync.Mutex
	fnCalls := 0
	fn := func() (int, error) {
		mu.Lock()
		fnCalls++
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		return 1, nil
	}
	var wg sync.WaitGroup
	wg.Add(2)
	for _, key := range []string{"k1", "k2"} {
		go func(key string) { defer wg.Done(); _, _ = Fetch(g, "a.x", key, fn) }(key)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if fnCalls != 2 {
		t.Errorf("不同 key 不得合并: fn 调用 %d 次, want 2", fnCalls)
	}
}

func TestGateTTLHitSkipsFn(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 42, nil }

	for i := 0; i < 3; i++ {
		v, err := Fetch(g, "a.x", "k", fn)
		if err != nil || v != 42 {
			t.Fatalf("call %d: got (%d, %v)", i, v, err)
		}
	}
	if fnCalls != 1 {
		t.Errorf("TTL 内 fn 调用 %d 次, want 1", fnCalls)
	}
}

func TestGateTTLExpires(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: 30 * time.Millisecond}})
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return fnCalls, nil }

	if v, _ := Fetch(g, "a.x", "k", fn); v != 1 {
		t.Fatalf("first: got %d", v)
	}
	time.Sleep(50 * time.Millisecond)
	if v, _ := Fetch(g, "a.x", "k", fn); v != 2 {
		t.Errorf("TTL 过期后应重新调用 fn, got %d", v)
	}
}

func TestGateDoesNotCacheErrors(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	wantErr := errors.New("boom")
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 0, wantErr }

	for i := 0; i < 2; i++ {
		if _, err := Fetch(g, "a.x", "k", fn); !errors.Is(err, wantErr) {
			t.Fatalf("call %d: err = %v, want %v", i, err, wantErr)
		}
	}
	if fnCalls != 2 {
		t.Errorf("错误不得写缓存: fn 调用 %d 次, want 2", fnCalls)
	}
}

func TestGateTimeout(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {Timeout: 30 * time.Millisecond}})
	_, err := Fetch(g, "a.x", "k", func() (int, error) {
		time.Sleep(500 * time.Millisecond)
		return 1, nil
	})
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

func TestGateTimeoutDoesNotCache(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute, Timeout: 20 * time.Millisecond}})
	slow := func() (int, error) { time.Sleep(200 * time.Millisecond); return 1, nil }
	if _, err := Fetch(g, "a.x", "k", slow); !errors.Is(err, ErrTimeout) {
		t.Fatalf("first: err = %v, want ErrTimeout", err)
	}
	fast := func() (int, error) { return 7, nil }
	if v, err := Fetch(g, "a.x", "k", fast); err != nil || v != 7 {
		t.Errorf("超时不得写缓存: got (%d, %v), want (7, nil)", v, err)
	}
}

func TestGateUnregisteredTopicPassesThrough(t *testing.T) {
	g := newTestGate(t, nil)
	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "eastmoney.kline", "k", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("未登记主题应直通不缓存: fn 调用 %d 次, want 3", fnCalls)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("未登记主题不应被节流, 耗时 %v", elapsed)
	}
}

func TestNilGateIsTransparent(t *testing.T) {
	var g *Gate
	v, err := Fetch(g, "a.x", "k", func() (int, error) { return 9, nil })
	if err != nil || v != 9 {
		t.Errorf("nil Gate 应直通: got (%d, %v)", v, err)
	}
	g.Wait("a.x") // 不得 panic
	if err := g.Do("a.x", "k", func() error { return nil }); err != nil {
		t.Errorf("nil Gate Do: %v", err)
	}
}

func TestDoForcesNoCache(t *testing.T) {
	// 同一主题带 TTL，但 Do 必须每次都执行 fn（设计 §3.2：Do 强制 TTL=0）
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Minute}})
	fnCalls := 0
	for i := 0; i < 3; i++ {
		if err := g.Do("a.x", "k", func() error { fnCalls++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("Do 调用 %d 次 fn, want 3", fnCalls)
	}
}

func TestWaitThrottlesWithoutFn(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {MinInterval: 80 * time.Millisecond}})
	g.Wait("a.x")
	start := time.Now()
	g.Wait("a.x")
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("Wait 应施加节流, 第二次耗时 %v", elapsed)
	}
}

func TestGateEvictsOldestEntry(t *testing.T) {
	g := newTestGate(t, map[string]Policy{"a.x": {TTL: time.Hour}})
	for i := 0; i < maxCacheEntries+10; i++ {
		key := time.Duration(i).String()
		if _, err := Fetch(g, "a.x", key, func() (int, error) { return i, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if n := g.entryCount(); n > maxCacheEntries {
		t.Errorf("缓存条目 %d 超过上限 %d", n, maxCacheEntries)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/policy/ -run TestGate -v`
Expected: 编译失败 —— `undefined: Gate` / `undefined: New` / `undefined: Fetch`

- [ ] **Step 3: 写实现**

创建 `internal/collector/policy/gate.go`：

```go
package policy

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// maxCacheEntries 限制 TTL 缓存条目数，超出时淘汰最旧的一条
// （沿用被本包取代的 CachedCollector 的界，量级相同）。
const maxCacheEntries = 512

// ErrTimeout 表示 fn 自身超过了主题策略的 Timeout。
var ErrTimeout = errors.New("policy: fn timeout")

// Gate 是唯一对外入口：按主题策略施加 TTL 缓存、在途合并、配额预判与节流。
//
// 执行链（设计 §3.2）：
//
//	① 查 TTL 缓存（命中即返回）
//	② singleflight 合并同 key 在途请求
//	③ 配额预判（超额 → ErrQuotaExceeded，不发请求）
//	④ 节流等待到限流域的最小间隔
//	⑤ 执行 fn（各 collector 自己的重试/退避层留在 fn 内部）
//
// 零值不可用，用 New 构造。nil *Gate 是透明的（直接执行 fn），
// 让未接线的调用点安全降级。
type Gate struct {
	table *Table
	warn  func(msg string, err error)

	domainsMu sync.Mutex
	domains   map[string]*domainState

	cacheMu sync.Mutex
	entries map[string]cacheEntry
	seq     uint64

	sf singleflight.Group
}

// domainState 是一个限流域的闸门状态。刻意在持锁状态下 sleep：并发调用方被
// 依次串行放行，天然形成均匀节奏（与被取代的 yahoo.throttle 行为一致）。
type domainState struct {
	mu      sync.Mutex
	lastReq time.Time
}

type cacheEntry struct {
	val      any
	storedAt time.Time
	seq      uint64
}

// Option 配置 Gate。
type Option func(*Gate)

// WithWarn 注入告警回调。配额账本异常时 fail-open 放行并经此上报。
func WithWarn(f func(msg string, err error)) Option {
	return func(g *Gate) { g.warn = f }
}

// New 构造 Gate。table 为 nil 时用内置表。
func New(t *Table, opts ...Option) *Gate {
	if t == nil {
		t = NewTable()
	}
	g := &Gate{
		table:   t,
		warn:    func(string, error) {},
		domains: make(map[string]*domainState),
		entries: make(map[string]cacheEntry),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Fetch 是带 TTL 缓存的准入控制。
//
// Go 不支持泛型方法，故这是包级函数而非 Gate 的方法（设计 §3.2）。
//
// **返回值所有权**：缓存命中时多个调用方拿到的是同一个值。T 为切片/映射时
// 调用方必须自行 clone 后再交给上层，否则会共享底层数组。
func Fetch[T any](g *Gate, topic, key string, fn func() (T, error)) (T, error) {
	return fetch(g, topic, key, false, fn)
}

// Do 是无返回值的准入控制，等价于 Fetch 且**强制 TTL=0**：没有结果可缓存时，
// 缓存一个空值会静默吞掉后续调用。
func (g *Gate) Do(topic, key string, fn func() error) error {
	_, err := fetch(g, topic, key, true, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// Wait 只施加限流域节流，不碰缓存/合并/配额。
//
// 供 fn **内部的重试循环**复用同一个闸门：Fetch 已在调用 fn 前节流一次，
// 故重试循环只应在 attempt > 0 时调用本方法，否则首次请求会等两倍间隔。
func (g *Gate) Wait(topic string) {
	if g == nil {
		return
	}
	p, ok := g.table.Lookup(topic)
	if !ok {
		return
	}
	g.throttle(p)
}

func fetch[T any](g *Gate, topic, key string, noCache bool, fn func() (T, error)) (T, error) {
	var zero T
	if g == nil {
		return fn()
	}
	p, ok := g.table.Lookup(topic)
	if !ok {
		return fn() // 未登记 = 零策略，直通（设计 §4.1）
	}
	ttl := p.TTL
	if noCache {
		ttl = 0
	}
	ck := topic + "|" + key

	if v, hit := g.load(ck, ttl); hit {
		if tv, ok := v.(T); ok {
			return tv, nil
		}
		// 同 key 被不同 T 复用是编程错误；当作未命中重新取，不 panic。
	}

	call := func() (any, error) {
		// 合并窗口内可能已有人写好缓存，二次检查省掉一次请求。
		if v, hit := g.load(ck, ttl); hit {
			if tv, ok := v.(T); ok {
				return tv, nil
			}
		}
		if err := g.takeQuota(topic, p); err != nil {
			return zero, err
		}
		g.throttle(p)
		v, err := runWithTimeout(p.Timeout, fn)
		if err != nil {
			return zero, err // 失败不写缓存、不延长 TTL
		}
		if ttl > 0 {
			g.store(ck, v)
		}
		return v, nil
	}

	var (
		v   any
		err error
	)
	if p.Coalesce {
		v, err, _ = g.sf.Do(ck, call)
	} else {
		v, err = call()
	}
	if err != nil {
		return zero, err
	}
	tv, _ := v.(T)
	return tv, nil
}

// throttle 阻塞到该限流域距上次请求满 MinInterval 为止。
func (g *Gate) throttle(p Policy) {
	if p.MinInterval <= 0 {
		return
	}
	d := g.domainState(p.Domain)
	d.mu.Lock()
	defer d.mu.Unlock()
	if wait := p.MinInterval - time.Since(d.lastReq); wait > 0 {
		time.Sleep(wait)
	}
	d.lastReq = time.Now()
}

func (g *Gate) domainState(domain string) *domainState {
	g.domainsMu.Lock()
	defer g.domainsMu.Unlock()
	d, ok := g.domains[domain]
	if !ok {
		d = &domainState{}
		g.domains[domain] = d
	}
	return d
}

func (g *Gate) load(ck string, ttl time.Duration) (any, bool) {
	if ttl <= 0 {
		return nil, false
	}
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	e, ok := g.entries[ck]
	if !ok || time.Since(e.storedAt) >= ttl {
		return nil, false
	}
	return e.val, true
}

func (g *Gate) store(ck string, v any) {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	if _, exists := g.entries[ck]; !exists && len(g.entries) >= maxCacheEntries {
		g.evictOldest()
	}
	g.seq++
	g.entries[ck] = cacheEntry{val: v, storedAt: time.Now(), seq: g.seq}
}

// evictOldest 删除 seq 最小的条目。调用方须持 cacheMu。
// 每个条目 seq 均非零，故 oldestSeq==0 表示表为空。
func (g *Gate) evictOldest() {
	var oldestKey string
	var oldestSeq uint64
	for k, e := range g.entries {
		if oldestSeq == 0 || e.seq < oldestSeq {
			oldestKey, oldestSeq = k, e.seq
		}
	}
	if oldestSeq != 0 {
		delete(g.entries, oldestKey)
	}
}

// entryCount 是测试辅助。
func (g *Gate) entryCount() int {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	return len(g.entries)
}

// runWithTimeout 让 fn 在超时时间内完成，超时返回 ErrTimeout。
// 超时后 fn 的 goroutine 继续跑完（HTTP client 自带 Timeout 兜底），
// 结果写入带缓冲的 channel 后被丢弃，不泄漏 goroutine。
func runWithTimeout[T any](d time.Duration, fn func() (T, error)) (T, error) {
	if d <= 0 {
		return fn()
	}
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-timer.C:
		var zero T
		return zero, ErrTimeout
	}
}

// takeQuota 在 Task 3 接入 QuotaStore 前先放行一切。
func (g *Gate) takeQuota(topic string, p Policy) error { return nil }
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/collector/policy/ -race -v`
Expected: PASS（含 `-race`，coalesce 与 throttle 用例都有并发）

- [ ] **Step 5: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/policy/gate.go internal/collector/policy/gate_test.go
git commit -m "feat(collector): policy Gate 节流/合并/TTL/超时（配额桩位待接）"
```

---

### Task 3: QuotaStore 接口 + 内存实现 + Gate 接线

**Files:**
- Create: `internal/collector/policy/quota.go`
- Create: `internal/collector/policy/quota_test.go`
- Modify: `internal/collector/policy/gate.go`（`New` 加 `QuotaStore` 参数、`takeQuota` 实现）
- Modify: `internal/collector/policy/gate_test.go`（`newTestGate` 跟随 `New` 签名）

**Interfaces:**
- Consumes: Task 1 的 `Quota`；Task 2 的 `Gate` / `Option` / `WithWarn`
- Produces:
  - `var ErrQuotaExceeded = errors.New("policy: quota exceeded")`
  - `type QuotaStore interface { Take(topic string, q Quota, now time.Time) (bool, error) }`
  - `func NewMemStore() *MemStore`
  - `func (m *MemStore) Take(topic string, q Quota, now time.Time) (bool, error)`
  - `func (m *MemStore) Count(topic string) int`（测试辅助）
  - `func windowStart(now time.Time, q Quota) time.Time`
  - `func New(t *Table, q QuotaStore, opts ...Option) *Gate`（签名变更）

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/policy/quota_test.go`：

```go
package policy

import (
	"errors"
	"testing"
	"time"
)

func TestWindowStartAlignsNaturalDay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	q := Quota{Limit: 5, Window: 24 * time.Hour, Loc: loc}

	now := time.Date(2026, 8, 6, 23, 59, 59, 0, loc)
	want := time.Date(2026, 8, 6, 0, 0, 0, 0, loc)
	if got := windowStart(now, q); !got.Equal(want) {
		t.Errorf("windowStart = %v, want %v", got, want)
	}

	// 同一自然日内任意时刻应落到同一个窗口起点
	early := time.Date(2026, 8, 6, 0, 0, 1, 0, loc)
	if !windowStart(early, q).Equal(windowStart(now, q)) {
		t.Error("同一自然日的两个时刻窗口起点应相同")
	}
	// 跨日必须换窗口
	next := time.Date(2026, 8, 7, 0, 0, 1, 0, loc)
	if windowStart(next, q).Equal(windowStart(now, q)) {
		t.Error("跨自然日应换窗口")
	}
}

func TestWindowStartSubDayUsesTruncate(t *testing.T) {
	q := Quota{Limit: 1, Window: time.Minute}
	now := time.Date(2026, 8, 6, 10, 30, 45, 0, time.UTC)
	want := time.Date(2026, 8, 6, 10, 30, 0, 0, time.UTC)
	if got := windowStart(now, q); !got.Equal(want) {
		t.Errorf("windowStart = %v, want %v", got, want)
	}
}

func TestMemStoreTakeUpToLimit(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 3, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		ok, err := m.Take("t", q, now)
		if err != nil || !ok {
			t.Fatalf("第 %d 次 Take: (%v, %v), want (true, nil)", i, ok, err)
		}
	}
	ok, err := m.Take("t", q, now)
	if err != nil || ok {
		t.Fatalf("第 4 次 Take: (%v, %v), want (false, nil)", ok, err)
	}
	if got := m.Count("t"); got != 3 {
		t.Errorf("被拒的请求不得计数: Count = %d, want 3", got)
	}
}

func TestMemStoreResetsOnNewWindow(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	day1 := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)

	if ok, _ := m.Take("t", q, day1); !ok {
		t.Fatal("day1 首次应放行")
	}
	if ok, _ := m.Take("t", q, day1); ok {
		t.Fatal("day1 第二次应被拒")
	}
	if ok, _ := m.Take("t", q, day2); !ok {
		t.Error("窗口翻篇后计数应归零")
	}
}

func TestMemStoreIsolatesTopics(t *testing.T) {
	m := NewMemStore()
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()
	if ok, _ := m.Take("a", q, now); !ok {
		t.Fatal("a 首次应放行")
	}
	if ok, _ := m.Take("b", q, now); !ok {
		t.Error("不同主题账本互不影响")
	}
}

func TestGateBlocksBeforeSendingRequest(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 2, Window: 24 * time.Hour, Loc: time.UTC}})
	store := NewMemStore()
	g := New(tbl, store)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }

	for i := 1; i <= 2; i++ {
		if _, err := Fetch(g, "a.x", "k", fn); err != nil {
			t.Fatalf("第 %d 次: %v", i, err)
		}
	}
	_, err := Fetch(g, "a.x", "k", fn)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
	if fnCalls != 2 {
		t.Errorf("超额请求必须在发出前被拦下: fn 调用 %d 次, want 2", fnCalls)
	}
}

func TestGateCountsFailedRequests(t *testing.T) {
	// 请求已发出，服务端已计数 —— 本地账本也必须计数（设计 §4.4）
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC}})
	store := NewMemStore()
	g := New(tbl, store)

	boom := errors.New("boom")
	if _, err := Fetch(g, "a.x", "k", func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got := store.Count("a.x"); got != 1 {
		t.Errorf("失败请求应计数: Count = %d, want 1", got)
	}
}

func TestGateFailsOpenOnStoreError(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}})

	storeErr := errors.New("ledger corrupted")
	var warned error
	g := New(tbl, brokenStore{err: storeErr}, WithWarn(func(_ string, err error) { warned = err }))

	fnCalls := 0
	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "a.x", "k", func() (int, error) { fnCalls++; return 1, nil }); err != nil {
			t.Fatalf("账本异常必须 fail-open, got err = %v", err)
		}
	}
	if fnCalls != 3 {
		t.Errorf("fail-open 应放行全部请求: fn 调用 %d 次, want 3", fnCalls)
	}
	if !errors.Is(warned, storeErr) {
		t.Errorf("fail-open 必须告警: warned = %v, want %v", warned, storeErr)
	}
}

func TestGateWithoutQuotaStoreAllows(t *testing.T) {
	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("a.x", Policy{Quota: &Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}})
	g := New(tbl, nil) // 未接线账本

	for i := 0; i < 3; i++ {
		if _, err := Fetch(g, "a.x", "k", func() (int, error) { return 1, nil }); err != nil {
			t.Fatalf("无 QuotaStore 时应放行, got %v", err)
		}
	}
}

// brokenStore 永远报错，用于验证 fail-open。
type brokenStore struct{ err error }

func (b brokenStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	return false, b.err
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/policy/ -run 'TestWindowStart|TestMemStore|TestGateBlocks|TestGateCounts|TestGateFailsOpen|TestGateWithout' -v`
Expected: 编译失败 —— `undefined: NewMemStore` / `undefined: windowStart` / `too many arguments in call to New`

- [ ] **Step 3: 写 quota.go**

创建 `internal/collector/policy/quota.go`：

```go
package policy

import (
	"errors"
	"sync"
	"time"
)

// ErrQuotaExceeded 表示本地配额预判判定该主题在当前窗口已用尽。
//
// 语义与 tushare.ErrRateLimited 一致：**临时性**，窗口过后自愈。各 collector
// 负责在自己的包内把它映射成本包既有的哨兵错误，policy 的错误不外泄到
// prism 层（设计 §5.1）。
var ErrQuotaExceeded = errors.New("policy: quota exceeded")

// QuotaStore 是配额账本。实现必须并发安全。
//
// Take 返回 (true, nil) 表示放行并已计数；(false, nil) 表示当前窗口已用尽
// （**不计数** —— 请求没发出去）；err != nil 表示账本本身异常，调用方
// 必须 fail-open（放行 + 告警），不因账本损坏阻断降级链（设计 §4.4）。
type QuotaStore interface {
	Take(topic string, q Quota, now time.Time) (bool, error)
}

// windowStart 返回 now 所属窗口的起点。
//
// Window >= 24h 走自然日对齐（tushare 的「5 次/天」是自然日口径，不是滑动
// 24 小时）；更短的窗口按 UTC 截断——分钟级窗口没有时区含义。
func windowStart(now time.Time, q Quota) time.Time {
	if q.Window >= 24*time.Hour {
		loc := q.Loc
		if loc == nil {
			loc = time.UTC
		}
		t := now.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	}
	return now.UTC().Truncate(q.Window)
}

// ledgerEntry 是单个主题的账本条目。JSON tag 供 FileStore 复用。
type ledgerEntry struct {
	WindowStart time.Time `json:"window_start"`
	Count       int       `json:"count"`
}

// take 是窗口判定 + 计数的纯逻辑，被内存与文件两个实现共用。
// 返回 (放行?, 更新后的条目)。
func take(e ledgerEntry, q Quota, now time.Time) (bool, ledgerEntry) {
	ws := windowStart(now, q)
	if !e.WindowStart.Equal(ws) {
		e = ledgerEntry{WindowStart: ws, Count: 0}
	}
	if q.Limit > 0 && e.Count >= q.Limit {
		return false, e // 拦下的请求不计数
	}
	e.Count++
	return true, e
}

// MemStore 是进程内配额账本。**在 launchd 短命进程形态下无效**
// （每次启动归零，设计 §1.5），仅供测试与不需要跨进程语义的场景。
type MemStore struct {
	mu      sync.Mutex
	ledgers map[string]ledgerEntry
}

func NewMemStore() *MemStore {
	return &MemStore{ledgers: make(map[string]ledgerEntry)}
}

func (m *MemStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ok, e := take(m.ledgers[topic], q, now)
	m.ledgers[topic] = e
	return ok, nil
}

// Count 返回当前窗口已用次数（测试辅助）。
func (m *MemStore) Count(topic string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ledgers[topic].Count
}
```

- [ ] **Step 4: 改 gate.go 接线**

在 `internal/collector/policy/gate.go` 中：

`Gate` 结构体加字段（放在 `warn` 之后）：

```go
	quota QuotaStore
```

`New` 换成：

```go
// New 构造 Gate。table 为 nil 时用内置表；q 为 nil 时不做配额预判。
func New(t *Table, q QuotaStore, opts ...Option) *Gate {
	if t == nil {
		t = NewTable()
	}
	g := &Gate{
		table:   t,
		quota:   q,
		warn:    func(string, error) {},
		domains: make(map[string]*domainState),
		entries: make(map[string]cacheEntry),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}
```

`takeQuota` 桩位换成实现：

```go
// takeQuota 做配额预判。账本异常时 fail-open：放行并告警，绝不因账本损坏
// 阻断降级链（设计 §4.4）。
func (g *Gate) takeQuota(topic string, p Policy) error {
	if p.Quota == nil || g.quota == nil {
		return nil
	}
	ok, err := g.quota.Take(topic, *p.Quota, time.Now())
	if err != nil {
		g.warn("collector quota ledger unavailable, failing open: topic="+topic, err)
		return nil
	}
	if !ok {
		return ErrQuotaExceeded
	}
	return nil
}
```

- [ ] **Step 5: 改 gate_test.go 的 helper 跟随新签名**

在 `internal/collector/policy/gate_test.go` 中把

```go
	return New(tbl)
```

改为

```go
	return New(tbl, nil)
```

- [ ] **Step 6: 跑全包测试确认通过**

Run: `go test ./internal/collector/policy/ -race -v`
Expected: PASS（Task 1/2/3 全部用例）

- [ ] **Step 7: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/policy/
git commit -m "feat(collector): QuotaStore 接口 + 内存实现，Gate 配额预判 fail-open"
```

---

### Task 4: QuotaStore 文件实现（跨进程）

**Files:**
- Create: `internal/collector/policy/quota_file.go`
- Test: `internal/collector/policy/quota_file_test.go`

**Interfaces:**
- Consumes: Task 3 的 `QuotaStore` / `ledgerEntry` / `take` / `Quota`
- Produces:
  - `func NewFileStore(path string) *FileStore`
  - `func (f *FileStore) Take(topic string, q Quota, now time.Time) (bool, error)`

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/policy/quota_file_test.go`：

```go
package policy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func quotaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "collector-quota.json")
}

// TestFileStoreQuotaSurvivesProcessRestart 是配额设计的立身之本（设计 §7.3、
// 验收标准 3）：两个独立 Gate 实例指向同一账本文件，模拟 launchd 的两次启动。
func TestFileStoreQuotaSurvivesProcessRestart(t *testing.T) {
	path := quotaPath(t)

	newGate := func() *Gate {
		tbl := &Table{policies: make(map[string]Policy)}
		tbl.Set("tushare.daily_basic", Policy{
			Quota: &Quota{Limit: 5, Window: 24 * time.Hour, Loc: time.UTC},
		})
		return New(tbl, NewFileStore(path))
	}

	// 第一次「启动」：用掉全部 5 次
	first := newGate()
	for i := 1; i <= 5; i++ {
		if _, err := Fetch(first, "tushare.daily_basic", "600519.SH", func() (int, error) { return i, nil }); err != nil {
			t.Fatalf("第一个实例第 %d 次: %v", i, err)
		}
	}

	// 第二次「启动」：全新 Gate、全新内存，首次 Take 就该被拒
	second := newGate()
	fnCalls := 0
	_, err := Fetch(second, "tushare.daily_basic", "600519.SH", func() (int, error) {
		fnCalls++
		return 0, nil
	})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("跨进程配额未生效: err = %v, want ErrQuotaExceeded", err)
	}
	if fnCalls != 0 {
		t.Errorf("超额请求不得发出: fn 调用 %d 次, want 0", fnCalls)
	}
}

func TestFileStoreResetsOnWindowRollover(t *testing.T) {
	path := quotaPath(t)
	loc := time.UTC
	q := Quota{Limit: 2, Window: 24 * time.Hour, Loc: loc}

	// 手工写一份「昨天已用满」的账本
	yesterday := time.Date(2026, 8, 5, 0, 0, 0, 0, loc)
	writeTestLedger(t, path, map[string]ledgerEntry{
		"t": {WindowStart: yesterday, Count: 2},
	})

	s := NewFileStore(path)
	today := time.Date(2026, 8, 6, 9, 0, 0, 0, loc)
	ok, err := s.Take("t", q, today)
	if err != nil || !ok {
		t.Fatalf("窗口翻篇后应放行: (%v, %v)", ok, err)
	}

	got := readTestLedger(t, path)
	if got["t"].Count != 1 {
		t.Errorf("翻篇后计数应归零再计一: Count = %d, want 1", got["t"].Count)
	}
	if !got["t"].WindowStart.Equal(time.Date(2026, 8, 6, 0, 0, 0, 0, loc)) {
		t.Errorf("window_start 未推进: %v", got["t"].WindowStart)
	}
}

func TestFileStoreFailsOpenOnCorruptLedger(t *testing.T) {
	path := quotaPath(t)
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}

	ok, err := s.Take("t", q, time.Now())
	if !ok {
		t.Error("账本损坏必须 fail-open 放行（设计 §4.4）")
	}
	if err == nil {
		t.Error("账本损坏必须同时报错以便告警")
	}
}

func TestFileStoreMissingFileStartsEmpty(t *testing.T) {
	s := NewFileStore(quotaPath(t)) // 文件不存在
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	ok, err := s.Take("t", q, time.Now())
	if err != nil || !ok {
		t.Fatalf("账本首次创建应放行: (%v, %v)", ok, err)
	}
}

func TestFileStoreCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "collector-quota.json")
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	if ok, err := s.Take("t", q, time.Now()); err != nil || !ok {
		t.Fatalf("应自动建目录: (%v, %v)", ok, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("账本文件未落盘: %v", err)
	}
}

func TestFileStoreRejectedTakeDoesNotIncrement(t *testing.T) {
	path := quotaPath(t)
	s := NewFileStore(path)
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	if ok, _ := s.Take("t", q, now); !ok {
		t.Fatal("首次应放行")
	}
	for i := 0; i < 3; i++ {
		if ok, _ := s.Take("t", q, now); ok {
			t.Fatal("超额应被拒")
		}
	}
	if got := readTestLedger(t, path)["t"].Count; got != 1 {
		t.Errorf("被拒的请求不得计数: Count = %d, want 1", got)
	}
}

func TestFileStoreIsolatesTopics(t *testing.T) {
	s := NewFileStore(quotaPath(t))
	q := Quota{Limit: 1, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()
	if ok, _ := s.Take("a", q, now); !ok {
		t.Fatal("a 首次应放行")
	}
	if ok, _ := s.Take("b", q, now); !ok {
		t.Error("不同主题账本互不影响")
	}
}

func TestFileStoreConcurrentTakesRespectLimit(t *testing.T) {
	path := quotaPath(t)
	s := NewFileStore(path)
	q := Quota{Limit: 10, Window: 24 * time.Hour, Loc: time.UTC}
	now := time.Now()

	const n = 50
	var mu sync.Mutex
	granted := 0
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if ok, err := s.Take("t", q, now); err == nil && ok {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if granted != 10 {
		t.Errorf("并发下放行 %d 次, want 10", granted)
	}
	if got := readTestLedger(t, path)["t"].Count; got != 10 {
		t.Errorf("账本计数 = %d, want 10", got)
	}
}

func writeTestLedger(t *testing.T, path string, l map[string]ledgerEntry) {
	t.Helper()
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestLedger(t *testing.T, path string) map[string]ledgerEntry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var l map[string]ledgerEntry
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("账本不是合法 JSON: %v (%s)", err, raw)
	}
	return l
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/policy/ -run TestFileStore -v`
Expected: 编译失败 —— `undefined: NewFileStore`

- [ ] **Step 3: 写实现**

创建 `internal/collector/policy/quota_file.go`：

```go
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// FileStore 是跨进程的 JSON 配额账本。
//
// 存在的理由：atlas 的 `prism refresh` 形态是 launchd 拉起的**短命进程**
// （设计 §1.5），内存计数每次启动归零，配额根本不会生效。
//
// 账本形如：
//
//	{"tushare.daily_basic": {"window_start": "2026-08-06T00:00:00+08:00", "count": 3}}
//
// 「读 → 判窗口 → 计数 → 原子写」全程持 flock 排他锁，故多进程并发安全。
// 用独立的 <path>.lock 加锁而非账本文件本身：账本靠 rename 原子替换，
// 对被替换掉的 inode 加锁没有意义。
//
// 平台：flock 是 unix 系统调用。本仓库只在 darwin/linux 运行，故不加
// build tag；将来若需 Windows 支持再拆 _unix.go / _other.go。
type FileStore struct {
	path string
	mu   sync.Mutex // 同进程内串行；跨进程由 flock 负责
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// Take 实现 QuotaStore。任何 I/O 或解析失败都返回 (true, err)：调用方据此
// fail-open 放行并告警——账本损坏绝不能阻断降级链（设计 §4.4）。
func (f *FileStore) Take(topic string, q Quota, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	unlock, err := f.lock()
	if err != nil {
		return true, err
	}
	defer unlock()

	// 解析失败时 readLedger 返回空账本 + err：账本就此自愈重建，
	// 同时把 err 带出去让 Gate 告警。
	ledgers, readErr := f.read()
	ok, entry := take(ledgers[topic], q, now)
	if !ok {
		// 被拦下的请求没发出去，不计数也就无需写盘。
		return false, readErr
	}
	ledgers[topic] = entry
	if err := f.write(ledgers); err != nil {
		return true, err // 已放行，但账本没记上——必须告警
	}
	return true, readErr
}

// lock 建目录、开锁文件并取排他 flock，返回释放函数。
func (f *FileStore) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return nil, fmt.Errorf("policy: quota dir: %w", err)
	}
	lf, err := os.OpenFile(f.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("policy: quota lock file: %w", err)
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, fmt.Errorf("policy: quota flock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		_ = lf.Close()
	}, nil
}

// read 返回账本。文件不存在是正常的首次运行，不算错误；内容损坏则返回
// 空账本 + 错误（fail-open + 自愈重建）。
func (f *FileStore) read() (map[string]ledgerEntry, error) {
	raw, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return make(map[string]ledgerEntry), nil
	}
	if err != nil {
		return make(map[string]ledgerEntry), fmt.Errorf("policy: read quota ledger: %w", err)
	}
	var l map[string]ledgerEntry
	if err := json.Unmarshal(raw, &l); err != nil || l == nil {
		return make(map[string]ledgerEntry), fmt.Errorf("policy: quota ledger corrupted at %s: %w", f.path, err)
	}
	return l, nil
}

// write 以 temp + rename 原子替换账本，避免崩溃留下半截文件。
func (f *FileStore) write(l map[string]ledgerEntry) error {
	raw, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("policy: encode quota ledger: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".collector-quota-*.json")
	if err != nil {
		return fmt.Errorf("policy: temp quota ledger: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("policy: write quota ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("policy: close quota ledger: %w", err)
	}
	if err := os.Rename(tmpName, f.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("policy: rename quota ledger: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/collector/policy/ -race -v`
Expected: PASS（含 `TestFileStoreConcurrentTakesRespectLimit`）

- [ ] **Step 5: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/policy/quota_file.go internal/collector/policy/quota_file_test.go
git commit -m "feat(collector): 跨进程配额账本（flock + 原子写 + 自然日窗口 + fail-open）"
```

---

### Task 5: 全局单例 + config 覆盖 + cmd 接线

**Files:**
- Create: `internal/collector/policy/default.go`
- Create: `internal/collector/policy/default_test.go`
- Create: `cmd/atlas/policy.go`
- Create: `cmd/atlas/policy_test.go`
- Modify: `internal/collector/policy/policy.go`（加 `Override`）
- Modify: `internal/collector/policy/policy_test.go`（加 Override 用例）
- Modify: `internal/config/config.go:59-68`（`CollectorGlobalConfig` 扩字段）、`Load` 与 `Defaults` 的默认值
- Modify: `cmd/atlas/serve.go:76`（配置校验后 `initPolicyGate`）
- Modify: `cmd/atlas/export_ohlcv.go:283-292`（`loadConfigOrDefaults` 内 `initPolicyGate`）

**Interfaces:**
- Consumes: Task 1–4 的 `Table` / `Gate` / `New` / `NewFileStore` / `WithWarn`
- Produces:
  - `func policy.Default() *Gate`
  - `func policy.SetDefault(g *Gate)`
  - `type policy.Override struct { TTL, MinInterval, Timeout, QuotaWindow *time.Duration; Coalesce *bool; QuotaLimit *int }`
  - `func (t *Table) Override(topic string, o Override)`
  - `type config.QuotaConfig struct { Path string }`
  - `type config.TopicConfig struct { TTL, MinInterval, Timeout, QuotaWindow *time.Duration; Coalesce *bool; QuotaLimit *int }`
  - `func initPolicyGate(cfg *config.Config, log *zap.Logger)`（cmd/atlas 包内）

- [ ] **Step 1: 写 policy 侧失败测试**

创建 `internal/collector/policy/default_test.go`：

```go
package policy

import (
	"testing"
	"time"
)

func TestDefaultIsLazyAndUsesBuiltinTable(t *testing.T) {
	g := Default()
	if g == nil {
		t.Fatal("Default() 不得返回 nil —— 未接线的调用点也要能拿到内置策略")
	}
	if p, ok := g.table.Lookup("yahoo.chart"); !ok || p.MinInterval != 500*time.Millisecond {
		t.Errorf("Default 应使用内置表, got (%+v, %v)", p, ok)
	}
	if g2 := Default(); g2 != g {
		t.Error("Default() 必须返回同一实例")
	}
}

func TestSetDefaultReplaces(t *testing.T) {
	原 := Default()
	t.Cleanup(func() { SetDefault(原) })

	tbl := &Table{policies: make(map[string]Policy)}
	tbl.Set("x.y", Policy{MinInterval: time.Second})
	custom := New(tbl, nil)
	SetDefault(custom)

	if Default() != custom {
		t.Error("SetDefault 未生效")
	}
}
```

在 `internal/collector/policy/policy_test.go` 末尾追加：

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/policy/ -run 'TestDefault|TestSetDefault|TestOverride' -v`
Expected: 编译失败 —— `undefined: Default` / `undefined: Override`

- [ ] **Step 3: 写 default.go 与 Override**

创建 `internal/collector/policy/default.go`：

```go
package policy

import "sync"

// 进程内单例。各 collector 在**构造函数**里取 Default() 存入私有字段，因此
// SetDefault 必须在构造任何 collector 之前调用（cmd/atlas 的两条装配路径
// 都在配置装载后立即调用，见 cmd/atlas/policy.go）。
//
// 未调用 SetDefault 时 Default() 懒构造一个「内置表 + 无配额账本」的 Gate：
// 限流与合并仍生效，只是拿不到 config 覆盖与跨进程配额。这让 broker 等
// 边缘 CLI 路径无需接线也能安全运行。
var (
	defaultMu   sync.RWMutex
	defaultGate *Gate
)

// Default 返回进程内默认 Gate，永不为 nil。
func Default() *Gate {
	defaultMu.RLock()
	g := defaultGate
	defaultMu.RUnlock()
	if g != nil {
		return g
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultGate == nil {
		defaultGate = New(NewTable(), nil)
	}
	return defaultGate
}

// SetDefault 替换进程内默认 Gate。
func SetDefault(g *Gate) {
	defaultMu.Lock()
	defaultGate = g
	defaultMu.Unlock()
}
```

在 `internal/collector/policy/policy.go` 末尾追加：

```go
// Override 是 config 对单个主题的**字段级**覆盖：nil 字段保持内置值。
// 用指针而非零值判定，因为 0 是合法取值（TTL: 0 表示显式关掉该主题的缓存）。
type Override struct {
	TTL         *time.Duration
	MinInterval *time.Duration
	Timeout     *time.Duration
	Coalesce    *bool
	QuotaLimit  *int
	QuotaWindow *time.Duration
}

// Override 把 o 应用到 topic 上。主题未登记时从零策略起步（config 可以
// 为 eastmoney 这类当前无策略的 collector 新增策略，设计 §4.1）。
func (t *Table) Override(topic string, o Override) {
	p, _ := t.Lookup(topic)
	if o.TTL != nil {
		p.TTL = *o.TTL
	}
	if o.MinInterval != nil {
		p.MinInterval = *o.MinInterval
	}
	if o.Timeout != nil {
		p.Timeout = *o.Timeout
	}
	if o.Coalesce != nil {
		p.Coalesce = *o.Coalesce
	}
	if o.QuotaLimit != nil || o.QuotaWindow != nil {
		q := Quota{Window: 24 * time.Hour, Loc: shanghai()}
		if p.Quota != nil {
			q = *p.Quota // 复制而非共享：内置表的 *Quota 可能被多主题引用
		}
		if o.QuotaLimit != nil {
			q.Limit = *o.QuotaLimit
		}
		if o.QuotaWindow != nil {
			q.Window = *o.QuotaWindow
		}
		p.Quota = &q
	}
	// Lookup 命中通配主题时 topic 本身尚未登记，Set 会把它固化为精确条目。
	p.Domain = ""
	t.Set(topic, p)
}
```

注意：`Override` 里把 `p.Domain` 清空再 `Set`，让 `Set` 重新按 topic 推导域。否则从 `lixinger.*` 通配继承来的 Domain 会被原样复制（本例恰好相同，但对 `Domain` 显式设为别的域的主题会串味）。

- [ ] **Step 4: 跑 policy 包测试确认通过**

Run: `go test ./internal/collector/policy/ -race -v`
Expected: PASS

- [ ] **Step 5: 扩 config**

在 `internal/config/config.go` 把 `CollectorGlobalConfig` / `CacheConfig` 那一段（约 `:57-68`）替换为：

```go
// CollectorGlobalConfig holds collector-wide settings (distinct from the
// per-collector Collectors map). Top-level yaml key: "collector".
type CollectorGlobalConfig struct {
	Cache CacheConfig `mapstructure:"cache"`
	Quota QuotaConfig `mapstructure:"quota"`
	// Topics 按主题名字段级覆盖内置策略表（internal/collector/policy）。
	// 键形如 "tushare.daily_basic" / "yahoo.chart"。
	Topics map[string]TopicConfig `mapstructure:"topics"`
}

// CacheConfig holds OHLCV collector cache settings. Enabled=false 会把**所有**
// 主题的 TTL 强制归零（限流与配额不受影响）。
type CacheConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	TTL     time.Duration `mapstructure:"ttl"`
}

// QuotaConfig points at the cross-process collector quota ledger.
type QuotaConfig struct {
	Path string `mapstructure:"path"`
}

// TopicConfig 是单个策略主题的字段级覆盖。指针字段：未写的键保持内置值，
// 写成 0 则是「显式关掉」（如 ttl: 0 关闭该主题缓存）。
type TopicConfig struct {
	TTL         *time.Duration `mapstructure:"ttl"`
	MinInterval *time.Duration `mapstructure:"min_interval"`
	Timeout     *time.Duration `mapstructure:"timeout"`
	Coalesce    *bool          `mapstructure:"coalesce"`
	QuotaLimit  *int           `mapstructure:"quota_limit"`
	QuotaWindow *time.Duration `mapstructure:"quota_window"`
}
```

在 `Load()` 里紧挨 `if cfg.Collector.Cache.TTL <= 0 { ... }` 之后追加：

```go
	if cfg.Collector.Quota.Path == "" {
		cfg.Collector.Quota.Path = defaultQuotaPath
	}
```

在 `Defaults()` 的 `Collector: CollectorGlobalConfig{...}` 块里，`Cache` 之后追加：

```go
			Quota: QuotaConfig{Path: defaultQuotaPath},
```

在 `CacheConfig` 定义上方加常量：

```go
// defaultQuotaPath 与 storage.signals.path 同风格，落在仓库的 data/ 下。
const defaultQuotaPath = "data/collector-quota.json"
```

- [ ] **Step 6: 写 cmd 侧失败测试**

创建 `cmd/atlas/policy_test.go`：

```go
package main

import (
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/config"
)

func restoreGate(t *testing.T) {
	t.Helper()
	原 := policy.Default()
	t.Cleanup(func() { policy.SetDefault(原) })
}

func TestInitPolicyGateAppliesCacheTTL(t *testing.T) {
	restoreGate(t)
	cfg := config.Defaults()
	cfg.Collector.Cache.Enabled = true
	cfg.Collector.Cache.TTL = 42 * time.Second
	cfg.Collector.Quota.Path = t.TempDir() + "/quota.json"

	initPolicyGate(cfg, nil)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	for i := 0; i < 2; i++ {
		if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "AAPL", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 1 {
		t.Errorf("cache.enabled=true 时同 key 应命中缓存: fn 调用 %d 次, want 1", fnCalls)
	}
}

func TestInitPolicyGateDisabledCacheStillThrottles(t *testing.T) {
	restoreGate(t)
	cfg := config.Defaults()
	cfg.Collector.Cache.Enabled = false
	cfg.Collector.Quota.Path = t.TempDir() + "/quota.json"
	// 把 yahoo 闸门调小，避免用例真的等 500ms
	iv := 60 * time.Millisecond
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"yahoo.chart": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)

	fnCalls := 0
	fn := func() (int, error) { fnCalls++; return 1, nil }
	start := time.Now()
	for i := 0; i < 2; i++ {
		if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "AAPL", fn); err != nil {
			t.Fatal(err)
		}
	}
	if fnCalls != 2 {
		t.Errorf("cache.enabled=false 时不得缓存: fn 调用 %d 次, want 2", fnCalls)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("限流不应随缓存开关关闭, 两次耗时 %v", elapsed)
	}
}

func TestInitPolicyGateAppliesTopicQuotaOverride(t *testing.T) {
	restoreGate(t)
	cfg := config.Defaults()
	cfg.Collector.Quota.Path = t.TempDir() + "/quota.json"
	limit := 1
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"tushare.daily_basic": {QuotaLimit: &limit, MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)

	fn := func() (int, error) { return 1, nil }
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k", fn); err != nil {
		t.Fatalf("首次应放行: %v", err)
	}
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k2", fn); err == nil {
		t.Error("配额上限被覆盖为 1，第二次应被拒")
	}
}

func TestInitPolicyGateUsesConfiguredQuotaPath(t *testing.T) {
	restoreGate(t)
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Collector.Quota.Path = dir + "/nested/quota.json"
	iv := time.Duration(0)
	cfg.Collector.Topics = map[string]config.TopicConfig{
		"tushare.daily_basic": {MinInterval: &iv},
	}

	initPolicyGate(cfg, nil)
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "k", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/nested/quota.json"); err != nil {
		t.Errorf("账本未写到配置路径: %v", err)
	}
}
```

（`TestInitPolicyGateUsesConfiguredQuotaPath` 需要 `import "os"`。）

- [ ] **Step 7: 跑测试确认失败**

Run: `go test ./cmd/atlas/ -run TestInitPolicyGate -v`
Expected: 编译失败 —— `undefined: initPolicyGate`

- [ ] **Step 8: 写 cmd/atlas/policy.go**

```go
package main

import (
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// initPolicyGate 从配置构建 collector 策略闸门并装成进程内单例。
//
// 必须在**构造任何 collector 之前**调用：各 collector 在构造函数里取
// policy.Default() 存入私有字段（设计 §3.4）。log 可为 nil（离线 CLI 路径）。
func initPolicyGate(cfg *config.Config, log *zap.Logger) {
	tbl := policy.NewTable()
	if cfg.Collector.Cache.Enabled {
		tbl.ApplyTTL(cfg.Collector.Cache.TTL)
	} else {
		// 等价于今天 maybeCache 直接返回原 collector（设计 §4.2）。
		tbl.DisableTTL()
	}
	for topic, tc := range cfg.Collector.Topics {
		tbl.Override(topic, policy.Override{
			TTL:         tc.TTL,
			MinInterval: tc.MinInterval,
			Timeout:     tc.Timeout,
			Coalesce:    tc.Coalesce,
			QuotaLimit:  tc.QuotaLimit,
			QuotaWindow: tc.QuotaWindow,
		})
	}

	path := cfg.Collector.Quota.Path
	if path == "" {
		path = "data/collector-quota.json"
	}

	warn := func(string, error) {}
	if log != nil {
		warn = func(msg string, err error) { log.Warn(msg, zap.Error(err)) }
	}
	policy.SetDefault(policy.New(tbl, policy.NewFileStore(path), policy.WithWarn(warn)))
}
```

- [ ] **Step 9: 接进两条装配路径**

在 `cmd/atlas/serve.go`，把配置校验之后那段：

```go
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
```

改为：

```go
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// 必须早于 buildCollectors：collector 在构造函数里取 policy.Default()。
	initPolicyGate(cfg, log)
```

在 `cmd/atlas/export_ohlcv.go` 把 `loadConfigOrDefaults` 改为：

```go
func loadConfigOrDefaults() (*config.Config, error) {
	var cfg *config.Config
	if cfgFile != "" {
		loaded, err := config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		cfg = loaded
	} else {
		cfg = config.Defaults()
	}
	// 覆盖 prism refresh / crisis / export 等离线路径：collector 构造前装好闸门。
	initPolicyGate(cfg, nil)
	return cfg, nil
}
```

- [ ] **Step 10: 跑测试确认通过**

Run: `go test ./cmd/atlas/ ./internal/config/ ./internal/collector/... -race`
Expected: PASS

- [ ] **Step 11: 核实接线顺序**

Run: `grep -n "initPolicyGate\|buildCollectors\|lixinger.New\|yahoo.New\|tushare.New" cmd/atlas/serve.go cmd/atlas/prism.go cmd/atlas/export_ohlcv.go`
Expected: 每个文件里 `initPolicyGate`（或调用它的 `loadConfigOrDefaults`）的行号都**小于**该文件里所有 collector 构造行号。若不成立，把 `initPolicyGate` 上移。

- [ ] **Step 12: 提交**

```bash
go build ./... && go vet ./...
git add internal/collector/policy/ internal/config/config.go cmd/atlas/policy.go cmd/atlas/policy_test.go cmd/atlas/serve.go cmd/atlas/export_ohlcv.go
git commit -m "feat(collector): 策略闸门进程内单例 + config 主题覆盖 + 装配接线"
```

---

### Task 6: yahoo 接入 Gate

**Files:**
- Modify: `internal/collector/yahoo/yahoo.go`
- Modify: `internal/collector/yahoo/eps.go:57`
- Modify: `internal/collector/yahoo/throttle_test.go`
- Test: `internal/collector/yahoo/gate_test.go`（新建，含 `TestMain`）

**Interfaces:**
- Consumes: `policy.Default()` / `policy.Fetch` / `Gate.Wait` / `policy.New` / `policy.NewTable`
- Produces:
  - `func (y *Yahoo) do(topic string, req *http.Request) (*http.Response, error)`（签名变更，包内）
  - 主题常量 `topicChart = "yahoo.chart"` / `topicEPS = "yahoo.eps"` / `topicQuote = "yahoo.quote"`
  - `Yahoo.gate *policy.Gate` 字段（包内测试可赋值注入）

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/yahoo/gate_test.go`：

```go
package yahoo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// TestMain 把进程默认闸门换成「零间隔、零 TTL」，否则本包每个用例都会被
// 生产策略的 500ms 节流拖慢，且 TTL 会吞掉重复请求让计数断言失真。
// 单个用例需要真闸门时自己给 y.gate 赋值（见 gateWith）。
func TestMain(m *testing.M) {
	policy.SetDefault(gateWith(policy.Policy{}))
	os.Exit(m.Run())
}

// gateWith 用同一份策略登记三个 yahoo 主题（同域，共享闸门）。
func gateWith(p policy.Policy) *policy.Gate {
	tbl := policy.NewTable()
	for _, topic := range []string{"yahoo.chart", "yahoo.eps", "yahoo.quote"} {
		q := p
		q.Domain = "yahoo"
		tbl.Set(topic, q)
	}
	return policy.New(tbl, nil)
}

func TestFetchHistoryCachesViaGate(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for i := 0; i < 3; i++ {
		if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", hits)
	}
}

func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	first, err := y.FetchHistory("AAPL", start, end, "1d")
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bars, %v)", len(first), err)
	}
	first[0].Close = -999 // 调用方原地改写

	second, err := y.FetchHistory("AAPL", start, end, "1d")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Close == -999 {
		t.Error("缓存命中必须返回独立副本，否则调用方能污染缓存")
	}
}

func TestFetchQuoteIsNotCached(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	// 生产 yahoo.quote 主题 TTL=0；这里显式登记同一策略验证实时语义
	tbl := policy.NewTable()
	tbl.Set("yahoo.quote", policy.Policy{Domain: "yahoo", TTL: 0, Coalesce: true})
	tbl.Set("yahoo.chart", policy.Policy{Domain: "yahoo"})
	y.gate = policy.New(tbl, nil)

	for i := 0; i < 3; i++ {
		if _, err := y.FetchQuote("AAPL"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 3 {
		t.Errorf("FetchQuote 必须保持实时不缓存, HTTP 请求 %d 次, want 3", hits)
	}
}

func TestChartAndEPSShareThrottleDomain(t *testing.T) {
	var mu sync.Mutex
	var arrivals []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
		if r.URL.Query().Get("type") == "trailingDilutedEPS" {
			_, _ = w.Write([]byte(`{"timeseries":{"result":[{"trailingDilutedEPS":[{"asOfDate":"2023-11-14","reportedValue":{"raw":6.42}}]}]}}`))
			return
		}
		_, _ = w.Write([]byte(throttleChartBody))
	}))
	t.Cleanup(srv.Close)

	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{MinInterval: 80 * time.Millisecond})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	if _, err := y.FetchHistory("AAPL", start, end, "1d"); err != nil {
		t.Fatal(err)
	}
	if _, err := y.FetchEPSHistory("AAPL", start, end); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(arrivals) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(arrivals))
	}
	if gap := arrivals[1].Sub(arrivals[0]); gap < 60*time.Millisecond {
		t.Errorf("chart 与 eps 同域应共享闸门, 间隔 %v", gap)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/yahoo/ -run 'TestFetchHistoryCaches|TestFetchQuoteIsNotCached|TestChartAndEPS' -v`
Expected: 编译失败 —— `y.gate undefined (type *Yahoo has no field or method gate)`

- [ ] **Step 3: 改 yahoo.go**

导入区加：

```go
	"slices"

	"github.com/newthinker/atlas/internal/collector/policy"
```

把节流常量段（`minRequestInterval` 那条）改为——**删掉 `minRequestInterval`**，其余保留，并加主题常量：

```go
// 突发限流参数(设计 §9 M2.1):日总量安全 ≠ 突发安全——2026-07-24 v1.4.0
// 首跑 20 家背靠背拉 10Y 行情即触发 429,且 engine fallback 同依赖 yahoo。
// 取包内常量不进 config(无按环境调参需求)。
//
// 相邻请求最小间隔已迁到 policy 策略表的 yahoo 限流域(500ms 不变),
// 三个主题同域共享同一个闸门。
const (
	retryBackoffBase  = time.Second      // 指数退避基数:1s→2s→4s
	maxRetryAttempts  = 3                // 单请求最多重试次数
	retryBudgetPerRun = 20               // 实例级重试预算(极端持续限流下快速失败)
	maxRetryAfterWait = 60 * time.Second // Retry-After 上界:服务端给极大值时不阻塞调度窗口
)

// 策略主题名。chart/eps/quote 同属 yahoo 限流域,TTL 各自独立。
const (
	topicChart = "yahoo.chart"
	topicEPS   = "yahoo.eps"
	topicQuote = "yahoo.quote"
)
```

结构体改为（删 `lastReq` / `minInterval`，加 `gate`；`mu` 保留给 `retryBudget`）：

```go
type Yahoo struct {
	client     *http.Client
	config     collector.Config
	baseURL    string
	epsBaseURL string

	// gate 提供限流/缓存/合并/配额;退避与重试预算仍是本包自己的事(设计 §3.2)。
	gate *policy.Gate

	// 退避状态(见 do);字段可在测试中覆盖以缩短用例耗时。
	mu            sync.Mutex
	backoffBase   time.Duration
	maxRetries    int
	retryBudget   int
	maxRetryAfter time.Duration
}
```

`NewWithBaseURLs` 的字面量：删 `minInterval`，加 `gate: policy.Default()`：

```go
func NewWithBaseURLs(chartURL, epsURL string) *Yahoo {
	return &Yahoo{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:       chartURL,
		epsBaseURL:    epsURL,
		gate:          policy.Default(),
		backoffBase:   retryBackoffBase,
		maxRetries:    maxRetryAttempts,
		retryBudget:   retryBudgetPerRun,
		maxRetryAfter: maxRetryAfterWait,
	}
}
```

删除整个 `throttle()` 方法，并把 `do` 改为：

```go
// do sends req through the retry/backoff loop. Only idempotent GETs with nil
// body go through here (newRequest 只构造这种请求),所以同一 *http.Request
// 可安全重发。重试耗尽或预算用尽时返回最后一个响应,交由调用点既有的
// unexpected status 路径报错——错误语义零变更。
//
// 节流由 Gate 负责:首发请求已在 policy.Fetch 进入 fn 前节流过一次,故这里
// 只对**重试**(attempt > 0)补一次 Wait,避免首发等两倍间隔。
func (y *Yahoo) do(topic string, req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			y.gate.Wait(topic)
		}
		resp, err := y.client.Do(req)
		if err != nil {
			return nil, err // 网络层错误不重试,与既有行为一致
		}
		if !retryableStatus(resp.StatusCode) || attempt >= y.maxRetries || !y.takeRetryToken() {
			return resp, nil
		}
		wait := retryAfterWait(resp, y.backoffBase<<attempt, y.maxRetryAfter)
		_, _ = io.Copy(io.Discard, resp.Body) // 排干并关闭,允许连接复用
		_ = resp.Body.Close()
		time.Sleep(wait)
	}
}
```

`FetchQuote` 改为外壳 + 内核两层：

```go
// FetchQuote fetches real-time quote. TTL=0(实时语义),但仍走 Gate 以共享
// yahoo 限流闸门并合并同 symbol 的在途请求。
func (y *Yahoo) FetchQuote(symbol string) (*core.Quote, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	q, err := policy.Fetch(y.gate, topicQuote, symbol, func() (*core.Quote, error) {
		return y.fetchQuote(symbol)
	})
	if err != nil {
		return nil, err
	}
	// 合并时多个调用方共享同一指针,返回副本避免相互污染。
	out := *q
	return &out, nil
}

func (y *Yahoo) fetchQuote(symbol string) (*core.Quote, error) {
	yahooSymbol := y.toYahooSymbol(symbol)
	reqURL := fmt.Sprintf("%s/%s?interval=1d&range=1d", y.baseURL, url.PathEscape(yahooSymbol))

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.do(topicQuote, req)
	// ...（以下原样保留 FetchQuote 原有函数体自 `if err != nil { return nil, fmt.Errorf("fetching quote: %w", err) }` 起的全部内容）
}
```

`FetchHistory` 同样拆两层：

```go
// FetchHistory fetches historical OHLCV data. 缓存键把 start/end 截断到分钟,
// 让上层「以 time.Now() 为 end」的抖动仍能命中同一缓存槽(沿用被取代的
// CachedCollector.cacheKey 的口径)。
func (y *Yahoo) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d|%d|%s",
		symbol, start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix(), interval)
	data, err := policy.Fetch(y.gate, topicChart, key, func() ([]core.OHLCV, error) {
		return y.fetchHistory(symbol, start, end, interval)
	})
	if err != nil {
		return nil, err
	}
	// Gate 不复制返回值:缓存命中时多个调用方共享同一底层数组,故在此 clone。
	return slices.Clone(data), nil
}

func (y *Yahoo) fetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	yahooSymbol := y.toYahooSymbol(symbol)
	// ...（原 FetchHistory 自 `yahooInterval := ...` 起的全部内容，其中
	//      `y.do(req)` 改为 `y.do(topicChart, req)`）
}
```

- [ ] **Step 4: 改 eps.go**

`FetchEPSHistory` 同样拆两层：把原函数体自 `reqURL := ...` 起移入私有 `fetchEPSHistory`，其中 `y.do(req)` 改为 `y.do(topicEPS, req)`；外壳：

```go
func (y *Yahoo) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	if strings.HasPrefix(symbol, "^") {
		return nil, fmt.Errorf("eps history unavailable for index symbol %s", symbol)
	}
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d|%d", symbol, start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix())
	pts, err := policy.Fetch(y.gate, topicEPS, key, func() ([]core.EPSPoint, error) {
		return y.fetchEPSHistory(symbol, start, end)
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(pts), nil
}
```

`eps.go` 导入区加 `"slices"` 与 `"github.com/newthinker/atlas/internal/collector/policy"`。

- [ ] **Step 5: 迁移 throttle_test.go**

在 `newRetryServer` 里把

```go
	y := NewWithBaseURL(srv.URL)
	y.minInterval = 0
	y.backoffBase = time.Millisecond
```

改为

```go
	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{}) // 零间隔零 TTL:重试计数断言不能被节流/缓存干扰
	y.backoffBase = time.Millisecond
```

其余每处独立构造 `NewWithBaseURL` 并设 `y.minInterval = 0` 的地方（`TestDoHonorsRetryAfterHeader`、`TestDoRetryAfterCapped`、`TestFetchEPSHistoryRetries429`、`TestDoRetryAfterInvalidFallsBack`、`TestDoNetworkErrorNoRetry`），把 `y.minInterval = 0` 一律替换为 `y.gate = gateWith(policy.Policy{})`。

把 `TestDoThrottlesConsecutiveRequests` 改写为经 Gate 的等价断言（**保留而非删除**——它是「yahoo 真的走了 Gate」的端到端锚点）：

```go
func TestDoThrottlesConsecutiveRequests(t *testing.T) {
	y, arrivals := newRetryServer(t) // 全 200
	y.gate = gateWith(policy.Policy{MinInterval: 80 * time.Millisecond})
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if err := fetchOnce(t, y); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	ts := arrivals()
	if len(ts) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(ts))
	}
	if gap := ts[1].Sub(ts[0]); gap < 60*time.Millisecond { // 留计时容差
		t.Errorf("expected >=60ms gap between requests, got %v", gap)
	}
}
```

注意：`fetchOnce` 两次用同样的参数，因此 `gateWith(policy.Policy{...})` 的 TTL 必须是 0，否则第二次会命中缓存不发请求。

`throttle_test.go` 导入区加 `"github.com/newthinker/atlas/internal/collector/policy"`。

- [ ] **Step 6: 跑 yahoo 全包测试**

Run: `go test ./internal/collector/yahoo/ -race -v`
Expected: PASS。若某用例超时/变慢，检查它是否落在 `TestMain` 设的零间隔默认闸门之外（即自己 `y.gate = ` 覆盖成了带间隔的）。

- [ ] **Step 7: 核实 lastReq 已消失**

Run: `grep -n "lastReq\|minInterval\|func (y \*Yahoo) throttle" internal/collector/yahoo/*.go`
Expected: 无输出

- [ ] **Step 8: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/yahoo/
git commit -m "feat(collector): yahoo 接入 policy Gate（chart/eps/quote 同域共享闸门）"
```

---

### Task 7: twelvedata 接入 Gate

**Files:**
- Modify: `internal/collector/twelvedata/client.go`
- Modify: `internal/collector/twelvedata/client_test.go`
- Test: `internal/collector/twelvedata/gate_test.go`（新建，含 `TestMain`）

**Interfaces:**
- Consumes: `policy.Default()` / `policy.Fetch` / `policy.New` / `policy.NewTable`
- Produces: `Client.gate *policy.Gate` 字段；主题常量 `topicTimeSeries = "twelvedata.time_series"`

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/twelvedata/gate_test.go`：

```go
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

// TestMain 把默认闸门换成零间隔零 TTL：生产策略是 8s/次，会让整包用例挂死。
func TestMain(m *testing.M) {
	policy.SetDefault(gateWith(policy.Policy{}))
	os.Exit(m.Run())
}

func gateWith(p policy.Policy) *policy.Gate {
	tbl := policy.NewTable()
	p.Domain = "twelvedata"
	tbl.Set(topicTimeSeries, p)
	return policy.New(tbl, nil)
}

const gateSeriesBody = `{"status":"ok","values":[{"datetime":"2026-08-03","close":"101.5"}]}`

func newCountingServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(gateSeriesBody))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

func TestFetchHistoryThrottledByGate(t *testing.T) {
	srv, _ := newCountingServer(t)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{MinInterval: 80 * time.Millisecond})

	start := time.Now()
	if _, err := c.FetchHistory("NVDA", time.Now().AddDate(0, 0, -5), time.Now()); err != nil {
		t.Fatal(err)
	}
	// 换个 symbol 以免命中缓存（本用例 TTL=0，此处只为语义清晰）
	if _, err := c.FetchHistory("AAPL", time.Now().AddDate(0, 0, -5), time.Now()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("连续两次调用须被节流至少一个 MinInterval, 耗时 %v", elapsed)
	}
}

func TestFetchHistoryCachedByGate(t *testing.T) {
	srv, hits := newCountingServer(t)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.FetchHistory("NVDA", start, end); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if hits() != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", hits())
	}
}

func TestFetchHistoryReturnsIndependentSlice(t *testing.T) {
	srv, _ := newCountingServer(t)
	c := NewWithBaseURL("k", srv.URL)
	c.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Now().AddDate(0, 0, -5), time.Now()
	first, err := c.FetchHistory("NVDA", start, end)
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bars, %v)", len(first), err)
	}
	first[0].Close = -1

	second, err := c.FetchHistory("NVDA", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Close == -1 {
		t.Error("缓存命中必须返回独立副本")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/twelvedata/ -run TestFetchHistory -v`
Expected: 编译失败 —— `c.gate undefined` / `undefined: topicTimeSeries`

- [ ] **Step 3: 改 client.go**

导入区加 `"slices"` 与 `"github.com/newthinker/atlas/internal/collector/policy"`；删掉不再用到的 `"sync"`。

常量段改为：

```go
const (
	defaultBaseURL = "https://api.twelvedata.com"
	// outputSize 是单次请求的最大返回条数(TD 上限 5000),覆盖 10Y 日线。
	outputSize = "5000"
	// topicTimeSeries 的 8s 最小间隔(免费层 8 req/min)登记在 policy 策略表里。
	topicTimeSeries = "twelvedata.time_series"
)
```

结构体改为：

```go
type Client struct {
	apiKey  string
	baseURL string
	hc      *http.Client

	gate *policy.Gate
}
```

`NewWithBaseURL` 改为：

```go
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 30 * time.Second},
		gate:    policy.Default(),
	}
}
```

删除整个 `throttle()` 方法。

`FetchHistory` 拆两层——把原函数体自 `q := url.Values{...}` 起（即删掉开头的 `c.throttle()`）移入私有 `fetchHistory`，外壳：

```go
func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	key := fmt.Sprintf("%s|%s|%s", symbol, start.Format("2006-01-02"), end.Format("2006-01-02"))
	out, err := policy.Fetch(c.gate, topicTimeSeries, key, func() ([]core.OHLCV, error) {
		return c.fetchHistory(symbol, start, end)
	})
	if err != nil {
		return nil, err
	}
	// Gate 不复制返回值,缓存命中时多个调用方共享同一底层数组。
	return slices.Clone(out), nil
}
```

`FetchHistory` 原有的长篇注释（TD end_date 排他等）随函数体一起移到 `fetchHistory` 上。

- [ ] **Step 4: 改既有测试**

在 `internal/collector/twelvedata/client_test.go`：

- 把所有 `c.minInterval = 0`（`:46`、`:140`、`:152`）整行删除 —— `TestMain` 已把默认闸门设为零间隔。
- 删除 `:176` 的 `assert.Equal(t, 8*time.Second, New("k1").minInterval, ...)` —— 该断言已由 `policy` 包的 `TestLookupBuiltinTopics` 覆盖（`twelvedata.time_series` → 8s）。
- 把 `:178-185` 的节流用例（`c.minInterval = 50 * time.Millisecond` + 耗时断言）整个删除 —— 已由本任务的 `TestFetchHistoryThrottledByGate` 取代。删除后若其外层测试函数变空，一并删除该函数。

Run: `grep -n "minInterval" internal/collector/twelvedata/*.go`
Expected: 无输出

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/collector/twelvedata/ -race -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/twelvedata/
git commit -m "feat(collector): twelvedata 接入 policy Gate（8s 闸门迁入策略表）"
```

---

### Task 8: tushare 接入 Gate + 配额 + 错误映射

**Files:**
- Modify: `internal/collector/tushare/client.go`
- Modify: `internal/collector/tushare/client_test.go:155-167`（`TestThrottleMinInterval`）
- Test: `internal/collector/tushare/gate_test.go`（新建，含 `TestMain`）

**Interfaces:**
- Consumes: `policy.Default()` / `policy.Fetch` / `policy.ErrQuotaExceeded` / `policy.NewMemStore`
- Produces: `Client.gate *policy.Gate` 字段；`callKey(params, fields) string`（包内）

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/tushare/gate_test.go`：

```go
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

// TestMain 把默认闸门换成零间隔零 TTL：生产策略的 200ms 会拖慢整包用例，
// 5 次/天的配额更会让用例互相干扰。
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

func TestPolicyErrorDoesNotLeakToCallers(t *testing.T) {
	srv, _ := countingServer(t, gateDailyBasicBody)
	tbl := zeroTable()
	tbl.Set("tushare.daily_basic", policy.Policy{
		Domain: "tushare",
		Quota:  &policy.Quota{Limit: 0, Window: 24 * time.Hour, Loc: time.UTC},
	})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, policy.NewMemStore())

	_, err := c.FetchDailyBasic("600519.SH", time.Now().AddDate(0, 0, -5), time.Now())
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

func TestCallKeyDistinguishesParams(t *testing.T) {
	a := callKey(map[string]string{"ts_code": "600519.SH", "start_date": "20260101"}, "close")
	b := callKey(map[string]string{"ts_code": "000001.SZ", "start_date": "20260101"}, "close")
	if a == b {
		t.Error("不同 ts_code 必须产生不同缓存键")
	}
	// map 遍历顺序随机，键必须稳定
	for i := 0; i < 20; i++ {
		if callKey(map[string]string{"ts_code": "600519.SH", "start_date": "20260101"}, "close") != a {
			t.Fatal("缓存键必须与 map 遍历顺序无关")
		}
	}
	if callKey(map[string]string{"ts_code": "600519.SH"}, "close") == callKey(map[string]string{"ts_code": "600519.SH"}, "pe_ttm") {
		t.Error("fields 不同必须产生不同缓存键")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/tushare/ -run 'TestQuota|TestPolicyError|TestOtherApis|TestCallKey|TestCallCached' -v`
Expected: 编译失败 —— `c.gate undefined` / `undefined: callKey`

- [ ] **Step 3: 改 client.go**

导入区加 `"github.com/newthinker/atlas/internal/collector/policy"`；删掉 `"sync"`。

删除 `const minInterval = 200 * time.Millisecond` 那一行，改为主题前缀常量：

```go
// topicPrefix + api_name 构成策略主题（如 tushare.daily_basic）。
// 200ms 基础档限频兜底与 daily_basic 的 5 次/天配额均登记在 policy 策略表里。
const topicPrefix = "tushare."
```

结构体改为：

```go
type Client struct {
	token, baseURL string
	hc             *http.Client
	gate           *policy.Gate
}
```

`NewWithBaseURL` 改为：

```go
func NewWithBaseURL(token, baseURL string) *Client {
	return &Client{
		token:   token,
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 60 * time.Second},
		gate:    policy.Default(),
	}
}
```

把原 `call` 拆两层：函数体自 `body, _ := json.Marshal(...)` 起（即删掉开头的 `c.mu.Lock() ... c.mu.Unlock()` 节流块）移入私有 `callHTTP`，新的 `call` 为：

```go
// call 经策略闸门发一次 api 并返回按 fields 列名索引的行（按日期升序）。
//
// 返回的 []row 在缓存命中时由多个调用方共享；本包所有消费者（FetchDailyBasic /
// fetchClose）都只读 row 并立即转成新的 ValuationPoint/PricePoint 切片，
// 故不额外复制。
func (c *Client) call(apiName string, params map[string]string, fields string) ([]row, error) {
	rows, err := policy.Fetch(c.gate, topicPrefix+apiName, callKey(params, fields), func() ([]row, error) {
		return c.callHTTP(apiName, params, fields)
	})
	if err != nil {
		// 本地配额预判与服务端限频语义一致：临时性、窗口过后自愈。映射成本包
		// 既有哨兵错误，让 prism 的降级链一行不改（设计 §5.1）——policy 包的
		// 错误绝不外泄到 prism 层。行为上只是从「撞墙后降级」提前为「撞墙前降级」。
		if errors.Is(err, policy.ErrQuotaExceeded) {
			return nil, fmt.Errorf("%w: %s (本地配额预判，未发出请求)", ErrRateLimited, apiName)
		}
		return nil, err
	}
	return rows, nil
}

// callKey 是缓存/合并键。map 遍历顺序随机，必须按键名排序后拼接，
// 否则同一次查询会散落到多个缓存槽。
func callKey(params map[string]string, fields string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
		b.WriteByte('&')
	}
	b.WriteString("fields=")
	b.WriteString(fields)
	return b.String()
}
```

（`sort` 与 `strings` 已在导入区。）

- [ ] **Step 4: 改既有节流测试**

把 `internal/collector/tushare/client_test.go:155-167` 的 `TestThrottleMinInterval` 整体替换为：

```go
// 节流已迁到 policy 策略表；这里断言 tushare 确实**经过**了闸门
// （200ms 的生产取值由 policy 包的 TestLookupBuiltinTopics 守住）。
func TestThrottleViaGate(t *testing.T) {
	var got map[string]any
	resp := `{"code":0,"data":{"fields":["ts_code","trade_date","close"],"items":[]},"msg":""}`
	srv := tsServer(t, resp, &got)
	defer srv.Close()

	tbl := zeroTable()
	tbl.Set("tushare.daily", policy.Policy{Domain: "tushare", MinInterval: 80 * time.Millisecond})
	c := NewWithBaseURL("tok", srv.URL)
	c.gate = policy.New(tbl, nil)
	start, end := yesterdayToNow()

	t0 := time.Now()
	_, err := c.FetchDaily("600519.SH", start, end)
	require.NoError(t, err)
	_, err = c.FetchDaily("000001.SZ", start, end) // 换标的，避免命中缓存键
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(t0), 60*time.Millisecond, "连续两次请求须被闸门拉开间隔")
}
```

`client_test.go` 导入区加 `"github.com/newthinker/atlas/internal/collector/policy"`。

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/collector/tushare/ -race -v`
Expected: PASS

Run: `grep -n "lastReq\|minInterval" internal/collector/tushare/*.go`
Expected: 无输出

- [ ] **Step 6: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/tushare/
git commit -m "feat(collector): tushare 接入 Gate，daily_basic 日配额撞墙前降级"
```

---

### Task 9: lixinger 接入 Gate（仅 TTL）

**Files:**
- Modify: `internal/collector/lixinger/lixinger.go`（结构体 + 构造函数）
- Modify: `internal/collector/lixinger/client.go`（`request`）
- Test: `internal/collector/lixinger/gate_test.go`（新建，含 `TestMain`）

**Interfaces:**
- Consumes: `policy.Default()` / `policy.Fetch`
- Produces: `Lixinger.gate *policy.Gate` 字段

这是设计 §1.3 缺陷的正面修复点：lixinger 从未被 `RegisterCollector` 注册，只以「eastmoney 的内部 fallback」和「Valuation/Fundamental source」两种身份存在，两条路径都绕过任何缓存层。把 Gate 放在 `request()` 里，两条路径同时被覆盖——**不需要**改注册方式。

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/lixinger/gate_test.go`：

```go
package lixinger

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// TestMain 把默认闸门换成零 TTL：内置 lixinger.* 主题带 5m 缓存，
// 会让本包既有的「按次计数」用例失真。
func TestMain(m *testing.M) {
	policy.SetDefault(policy.New(gateTable(policy.Policy{}), nil))
	os.Exit(m.Run())
}

func gateTable(p policy.Policy) *policy.Table {
	tbl := policy.NewTable()
	p.Domain = "lixinger"
	tbl.Set("lixinger.*", p)
	return tbl
}

const gateFundamentalBody = `{"code":1,"message":"ok","data":[]}`

func countingLixServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(gateFundamentalBody))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// 验收标准 6：lixinger 必须能进入缓存路径。
func TestLixingerRequestIsCached(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = policy.New(gateTable(policy.Policy{TTL: time.Minute, Coalesce: true}), nil)

	payload := map[string]any{"token": "key", "stockCodes": []string{"600519"}}
	for i := 0; i < 3; i++ {
		if _, err := l.request("cn/company/fundamental/non_financial", payload); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if hits() != 1 {
		t.Errorf("TTL 内应只发 1 次 HTTP 请求, got %d", hits())
	}
}

func TestLixingerCacheKeyIncludesPayload(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = policy.New(gateTable(policy.Policy{TTL: time.Minute, Coalesce: true}), nil)

	if _, err := l.request("cn/company/fundamental/non_financial", map[string]any{"stockCodes": []string{"600519"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.request("cn/company/fundamental/non_financial", map[string]any{"stockCodes": []string{"000001"}}); err != nil {
		t.Fatal(err)
	}
	if hits() != 2 {
		t.Errorf("不同 payload 不得共用缓存槽, HTTP 请求 %d 次, want 2", hits())
	}
}

func TestLixingerEndpointsAreSeparateKeys(t *testing.T) {
	srv, hits := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = policy.New(gateTable(policy.Policy{TTL: time.Minute, Coalesce: true}), nil)

	payload := map[string]any{"x": 1}
	if _, err := l.request("cn/fund/net-value", payload); err != nil {
		t.Fatal(err)
	}
	if _, err := l.request("cn/fund/profile", payload); err != nil {
		t.Fatal(err)
	}
	if hits() != 2 {
		t.Errorf("不同 endpoint 不得共用缓存槽, HTTP 请求 %d 次, want 2", hits())
	}
}

func TestLixingerNotThrottled(t *testing.T) {
	// 设计 §4.1 例外：只补它从未有过的缓存，不新增任何它今天没有的限流行为。
	p, ok := policy.NewTable().Lookup("lixinger.cn/company/fundamental/non_financial")
	if !ok {
		t.Fatal("lixinger 主题应登记")
	}
	if p.MinInterval != 0 || p.Quota != nil {
		t.Errorf("lixinger 不得被加上限流/配额: %+v", p)
	}
}

func TestLixingerReturnsIndependentBytes(t *testing.T) {
	srv, _ := countingLixServer(t)
	l := NewWithBaseURL("key", srv.URL)
	l.gate = policy.New(gateTable(policy.Policy{TTL: time.Minute, Coalesce: true}), nil)

	payload := map[string]any{"x": 1}
	first, err := l.request("cn/fund/profile", payload)
	if err != nil || len(first) == 0 {
		t.Fatalf("first: (%d bytes, %v)", len(first), err)
	}
	first[0] = 'X'

	second, err := l.request("cn/fund/profile", payload)
	if err != nil {
		t.Fatal(err)
	}
	if second[0] == 'X' {
		t.Error("缓存命中必须返回独立副本")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/collector/lixinger/ -run TestLixinger -v`
Expected: 编译失败 —— `l.gate undefined`

- [ ] **Step 3: 改 lixinger.go**

导入区加 `"github.com/newthinker/atlas/internal/collector/policy"`。

结构体加字段：

```go
type Lixinger struct {
	apiKey      string
	baseURL     string
	client      *http.Client
	retry       bool            // 429/5xx 退避重试开关
	retryDelays []time.Duration // 退避调度；测试可置零加速

	// gate 只给 lixinger 补 TTL 缓存（设计 §4.1 例外）：它从未走过任何
	// 缓存层，而 CachedCollector 装饰器对它根本装不上（会遮蔽
	// FundamentalCollector 扩展方法）。不新增限流与配额。
	gate *policy.Gate
}
```

`newWithBaseURL` 的字面量加 `gate: policy.Default(),`。

- [ ] **Step 4: 改 client.go 的 request**

导入区加 `"bytes"`（已有）与 `"github.com/newthinker/atlas/internal/collector/policy"`。

把 `request` 拆两层：原函数体自 `url := fmt.Sprintf(...)` 起移入私有 `requestHTTP(endpoint string, body []byte) ([]byte, error)`（内部沿用 `l.doOnce(url, body)` 的重试循环，不改），新的 `request` 为：

```go
// request POSTs payload as JSON to baseURL/endpoint and returns the raw body
// after validating the Lixinger envelope (code==1). 退避重试策略留在 fn 内部
// （requestHTTP），Gate 只负责 TTL 缓存与在途合并。
func (l *Lixinger) request(endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// 缓存键含完整请求体：同一 endpoint 不同标的/字段必须落到不同槽。
	raw, err := policy.Fetch(l.gate, "lixinger."+endpoint, string(body), func() ([]byte, error) {
		return l.requestHTTP(endpoint, body)
	})
	if err != nil {
		return nil, err
	}
	// Gate 不复制返回值；调用方会把它交给 json.Unmarshal，返回副本更安全。
	return bytes.Clone(raw), nil
}
```

`requestHTTP` 签名与函数体：

```go
func (l *Lixinger) requestHTTP(endpoint string, body []byte) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", l.baseURL, endpoint)

	maxAttempts := 1
	if l.retry {
		maxAttempts = len(l.retryDelays) + 1
	}
	// ...（以下原样保留 request 原有的 for 循环与返回，直至函数结尾）
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/collector/lixinger/ -race -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
go build ./... && go vet ./internal/collector/...
git add internal/collector/lixinger/
git commit -m "fix(collector): lixinger 经 Gate 获得 TTL 缓存（修复其从未进入缓存路径）"
```

---

### Task 10: 删除 `CachedCollector` 与 `maybeCache`

**Files:**
- Delete: `internal/collector/cache.go`
- Delete: `internal/collector/cache_test.go`
- Modify: `cmd/atlas/serve.go:434-449`（删 `maybeCache`）
- Modify: `cmd/atlas/collectors.go:26-95`（去掉包装）
- Modify: `cmd/atlas/serve_test.go:337-430`（删 `TestMaybeCache_*` 与随之无用的桩类型）

**Interfaces:**
- Consumes: Task 6-9 —— 五个 collector 已各自在 HTTP 调用处接入 Gate，缓存能力不依赖装饰器
- Produces: 无新符号；`collector.NewCached` / `collector.CachedCollector` / `maybeCache` 全部消失

- [ ] **Step 1: 删除装饰器**

```bash
git rm internal/collector/cache.go internal/collector/cache_test.go
```

- [ ] **Step 2: 删 maybeCache**

删除 `cmd/atlas/serve.go` 里 `maybeCache` 函数及其上方注释（`// maybeCache wraps c in an OHLCV TTL CachedCollector...` 到函数结尾）。若 `serve.go` 因此不再用到 `collector` 或 `time` 包，一并删掉对应 import。

- [ ] **Step 3: 改 collectors.go**

把 `buildCollectors` 开头的缓存变量块：

```go
	// OHLCV cache settings applied when registering collectors below.
	cacheEnabled := cfg.Collector.Cache.Enabled
	cacheTTL := cfg.Collector.Cache.TTL
	if cacheEnabled {
		log.Info("OHLCV collector cache enabled", zap.Duration("ttl", cacheTTL))
	}
```

替换为：

```go
	// 缓存不再由装配层的装饰器提供：TTL 已随限流一起下沉到各 collector 的
	// HTTP 调用处（internal/collector/policy）。装配层只负责注册。
	if cfg.Collector.Cache.Enabled {
		log.Info("collector policy cache enabled", zap.Duration("ttl", cfg.Collector.Cache.TTL))
	}
```

把五处 `maybeCache(x, cacheEnabled, cacheTTL)` 换成裸的 `x`：

- `:38` → `application.RegisterCollector(yahooCollector)`
- `:61` → `application.RegisterCollector(eastmoneyCollector)`
- `:74` → `application.RegisterCollector(cryptoCollector)`
- `:83` → `application.RegisterCollector(tushare.New(collectorCfg.APIKey))`
- `:92` → `application.RegisterCollector(baostock.New(prismCfg.BaostockBaseURL))`

**不**为 lixinger 新增 `RegisterCollector`：Task 9 已让它在 `request()` 层拿到缓存，两条既有调用路径（eastmoney fallback、Valuation/Fundamental source）都被覆盖；改注册方式会改变路由行为，超出本次范围。

- [ ] **Step 4: 删既有测试**

删除 `cmd/atlas/serve_test.go` 中：

- 文件头注释里提到 `TestMaybeCache_*` 的 done_criteria 映射行（`:4` 起那几行）
- `TestMaybeCache_EnabledWrapsPlain`、`TestMaybeCache_DisabledNoWrap`、`TestMaybeCache_FundamentalNotWrapped`、`TestMaybeCache_SelectorRoutingUnchanged` 四个测试函数
- `plainCollector` 与 `fundamentalCollectorStub` 两个桩类型 —— **前提是文件内没有其他用处**

Run: `grep -n "plainCollector\|fundamentalCollectorStub\|maybeCache\|CachedCollector\|NewCached" cmd/atlas/*.go internal/**/*.go`
Expected: 无输出。若 `plainCollector` 仍被别的测试使用，保留该类型，只删四个 `TestMaybeCache_*`。

- [ ] **Step 5: 跑全量测试**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add -A cmd/atlas internal/collector
git commit -m "refactor(collector): 删除 CachedCollector 与 maybeCache，缓存下沉到 HTTP 调用处"
```

---

### Task 11: 路由表重写

本任务与 Task 1-10 完全独立（不碰 policy 包），可任意时点插入。

**Files:**
- Create: `internal/collector/route.go`
- Test: `internal/collector/route_golden_test.go`（新建）
- Modify: `internal/collector/selector.go`

**Interfaces:**
- Consumes: `core.Market`、既有 `IsAShareIndex`、`Registry`
- Produces:
  - `type Route struct { Pattern string; Collector string; Market core.Market }`
  - `func lookupRoute(upper string) (Route, bool)`（包内）
  - `func matchPattern(pattern, s string) bool`（包内）
  - `func specificity(pattern string) int`（包内）
  - 四个公开函数签名不变

- [ ] **Step 1: 写黄金值测试（对着**旧**实现）**

创建 `internal/collector/route_golden_test.go`：

```go
package collector

import (
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

// 路由黄金值回归（设计 §7.1）：穷举符号形态，逐一钉死四个公开函数的返回值。
// 本文件**先对旧实现跑绿**，再重写内部实现，必须仍然全绿。
//
// 用例覆盖 A 股、中证跨市场指数、港股、美股、已登记/未登记指数、商品、
// 两种加密后缀、加密前缀裸符号、空串与畸形符号。
func TestRouteGoldenValues(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")

	tests := []struct {
		symbol        string
		wantCollector string
		wantMarket    core.Market
		wantKnownIdx  bool
	}{
		{"600519.SH", "eastmoney", core.MarketCNA, false},
		{"000001.SZ", "eastmoney", core.MarketCNA, false},
		{"930713.CSI", "eastmoney", core.MarketCNA, false}, // 表成员判定，非后缀
		{"930604.CSI", "eastmoney", core.MarketCNA, false},
		{"0700.HK", "yahoo", core.MarketHK, false},
		{"AAPL", "yahoo", core.MarketUS, false},
		{"^GSPC", "yahoo", core.MarketUS, true},
		{"^IXIC", "yahoo", core.MarketUS, true},
		{"^DJI", "yahoo", core.MarketUS, true},
		{"^HSI", "yahoo", core.MarketHK, true},
		{"^HSCE", "yahoo", core.MarketHK, true},
		{"^N225", "yahoo", core.MarketUS, false}, // 未登记指数：兜底 US 且 unknown
		{"CL=F", "yahoo", core.MarketUS, false},
		{"GC=F", "yahoo", core.MarketUS, false},
		{"BTC-USD", "crypto", core.MarketCrypto, false},
		{"ETH-USD", "crypto", core.MarketCrypto, false},
		{"ETHUSDT", "crypto", core.MarketCrypto, false},
		{"BTCUSDT", "crypto", core.MarketCrypto, false},
		{"BTC", "crypto", core.MarketCrypto, false},
		{"SOL", "crypto", core.MarketCrypto, false},
		{"MATIC", "crypto", core.MarketCrypto, false},
		{"", "yahoo", core.MarketUS, false},
		{"!!!", "yahoo", core.MarketUS, false},
		{"...", "yahoo", core.MarketUS, false},
		{"600519", "yahoo", core.MarketUS, false}, // 无后缀：今天不认作 A 股
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got := SelectExternalForSymbol(reg, tt.symbol)
			if got == nil {
				t.Fatalf("SelectExternalForSymbol = nil, want %q", tt.wantCollector)
			}
			if got.Name() != tt.wantCollector {
				t.Errorf("SelectExternalForSymbol = %q, want %q", got.Name(), tt.wantCollector)
			}
			if m := MarketForSymbol(tt.symbol); m != tt.wantMarket {
				t.Errorf("MarketForSymbol = %q, want %q", m, tt.wantMarket)
			}
			m, known := KnownIndexMarket(tt.symbol)
			if known != tt.wantKnownIdx {
				t.Errorf("KnownIndexMarket known = %v, want %v", known, tt.wantKnownIdx)
			}
			if known && m != tt.wantMarket {
				t.Errorf("KnownIndexMarket market = %q, want %q", m, tt.wantMarket)
			}
		})
	}
}

// 大小写不敏感：四个函数都先 ToUpper。
func TestRouteCaseInsensitive(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	for _, pair := range [][2]string{{"600519.sh", "600519.SH"}, {"btc-usd", "BTC-USD"}, {"^gspc", "^GSPC"}} {
		lower, upper := pair[0], pair[1]
		if MarketForSymbol(lower) != MarketForSymbol(upper) {
			t.Errorf("%s vs %s: MarketForSymbol 不一致", lower, upper)
		}
		if SelectExternalForSymbol(reg, lower).Name() != SelectExternalForSymbol(reg, upper).Name() {
			t.Errorf("%s vs %s: SelectExternalForSymbol 不一致", lower, upper)
		}
	}
}

// SelectForSymbol 的 qlib Covers() 前置规则（设计 §6.4 第 1 条）优先级最高。
func TestQlibCoversTakesPrecedenceOverTable(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	reg.Register(&coveringQlib{fakeCollector{name: "qlib"}})

	if got := SelectForSymbol(reg, "600519.SH"); got.Name() != "qlib" {
		t.Errorf("qlib 覆盖时应优先于路由表, got %q", got.Name())
	}
	// SelectExternalForSymbol 必须绕开 qlib
	if got := SelectExternalForSymbol(reg, "600519.SH"); got.Name() != "eastmoney" {
		t.Errorf("SelectExternalForSymbol 不得选中 qlib, got %q", got.Name())
	}
}

// 首选 collector 未注册时的回退链：yahoo → 任一非 qlib collector。
func TestRouteFallsBackWhenCollectorMissing(t *testing.T) {
	reg := newRegistryWith("yahoo") // 没有 eastmoney / crypto
	if got := SelectExternalForSymbol(reg, "600519.SH"); got.Name() != "yahoo" {
		t.Errorf("eastmoney 缺席应回退 yahoo, got %q", got.Name())
	}
	if got := SelectExternalForSymbol(reg, "BTC-USD"); got.Name() != "yahoo" {
		t.Errorf("crypto 缺席应回退 yahoo, got %q", got.Name())
	}

	only := newRegistryWith("eastmoney")
	if got := SelectExternalForSymbol(only, "AAPL"); got.Name() != "eastmoney" {
		t.Errorf("yahoo 也缺席时回退任一非 qlib collector, got %q", got.Name())
	}

	empty := NewRegistry()
	if got := SelectExternalForSymbol(empty, "AAPL"); got != nil {
		t.Errorf("空 registry 应返回 nil, got %q", got.Name())
	}
	if got := SelectExternalForSymbol(nil, "AAPL"); got != nil {
		t.Error("nil registry 应返回 nil")
	}
}

// coveringQlib 是 Covers() 恒真的 qlib 桩。
type coveringQlib struct{ fakeCollector }

func (c *coveringQlib) Covers(symbol string) bool { return true }
```

- [ ] **Step 2: 对旧实现跑绿**

Run: `go test ./internal/collector/ -run 'TestRoute|TestQlibCovers' -v`
Expected: PASS —— 这是重写前的基线。**如果这一步不绿，先修测试期望值使之匹配旧行为，不要改实现。**

- [ ] **Step 3: 提交基线**

```bash
git add internal/collector/route_golden_test.go
git commit -m "test(collector): 路由黄金值回归基线（对旧实现跑绿）"
```

- [ ] **Step 4: 写 route.go**

创建 `internal/collector/route.go`：

```go
package collector

import (
	"sort"
	"strings"

	"github.com/newthinker/atlas/internal/core"
)

// Route 把一类符号形态绑定到一个 collector 与一个市场。
//
// Pattern 是简化 glob：只认一个 '*'，位于开头（后缀匹配）、结尾（前缀匹配）
// 或独占全串（兜底）。不用 path.Match：'.' 与 '=' 在符号里是普通字符，
// 而 path.Match 的 '*' 不跨 '/'，语义对不上还更慢。
type Route struct {
	Pattern   string
	Collector string
	Market    core.Market
}

// routes 是内置路由表（设计 §6.3）。**具体度优先，而非注册顺序**：查表前按
// 非通配字符数降序稳定排序，故 config 追加规则时不存在顺序陷阱——'^HSI'
// 永远胜过 '^*'，与它写在文件哪一行无关。
//
// 具体度相同时按本切片的书写顺序决胜（稳定排序）。唯一真正相撞的形态是
// '*.HK' 与加密前缀（同为 3，如 "BTC.HK"）：旧实现在这里是**自相矛盾**的
// ——SelectExternalForSymbol 判它 crypto（isCryptoSymbol 前缀命中），
// MarketForSymbol 判它 HK（'.HK' 分支排在 crypto 之前）。统一到一张表后
// 必须二选一，取 '.HK' 优先（后缀是显式的交易所标识，比裸前缀更强的信号），
// 故 '*.HK' 写在加密前缀之前。
var routes = sortRoutes([]Route{
	// A 股（isAShareSymbol）
	{"*.SH", "eastmoney", core.MarketCNA},
	{"*.SZ", "eastmoney", core.MarketCNA},
	// 港股（MarketForSymbol 的 .HK 分支）—— 见上方与加密前缀的决胜说明
	{"*.HK", "yahoo", core.MarketHK},
	// 已登记指数（indexMarkets）
	{"^GSPC", "yahoo", core.MarketUS},
	{"^IXIC", "yahoo", core.MarketUS},
	{"^DJI", "yahoo", core.MarketUS},
	{"^HSI", "yahoo", core.MarketHK},
	{"^HSCE", "yahoo", core.MarketHK},
	// 未登记指数兜底（isIndexSymbol）——命中它即 KnownIndexMarket 的 unknown
	{"^*", "yahoo", core.MarketUS},
	// 商品（isCommoditySymbol）
	{"*=F", "yahoo", core.MarketUS},
	// 加密后缀（isCryptoSymbol 的后缀分支）
	{"*-USD", "crypto", core.MarketCrypto},
	{"*USDT", "crypto", core.MarketCrypto},
	// 加密前缀（原 cryptoTickers 白名单，逐条平移；加币种改这里或走 config）
	{"BTC*", "crypto", core.MarketCrypto},
	{"ETH*", "crypto", core.MarketCrypto},
	{"SOL*", "crypto", core.MarketCrypto},
	{"XRP*", "crypto", core.MarketCrypto},
	{"DOGE*", "crypto", core.MarketCrypto},
	{"ADA*", "crypto", core.MarketCrypto},
	{"DOT*", "crypto", core.MarketCrypto},
	{"AVAX*", "crypto", core.MarketCrypto},
	{"MATIC*", "crypto", core.MarketCrypto},
	{"LINK*", "crypto", core.MarketCrypto},
	{"UNI*", "crypto", core.MarketCrypto},
	{"ATOM*", "crypto", core.MarketCrypto},
	{"LTC*", "crypto", core.MarketCrypto},
	// 默认兜底：美股
	{"*", "yahoo", core.MarketUS},
})

// sortRoutes 按具体度降序稳定排序。稳定性是语义的一部分：同具体度时
// 书写顺序决胜（见 routes 上的 '*.HK' 说明）。
func sortRoutes(in []Route) []Route {
	out := append([]Route(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		return specificity(out[i].Pattern) > specificity(out[j].Pattern)
	})
	return out
}

// specificity 是 pattern 中的非通配字符数。
func specificity(pattern string) int {
	return len(pattern) - strings.Count(pattern, "*")
}

// matchPattern 匹配简化 glob。s 必须已大写。
func matchPattern(pattern, s string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*"):
		return strings.HasSuffix(s, pattern[1:])
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	default:
		return pattern == s
	}
}

// lookupRoute 返回第一个命中的路由。表末尾有 "*" 兜底，故实际总能命中；
// 返回 ok 是为了在表被改坏时让调用方保守回退而非 panic。
func lookupRoute(upper string) (Route, bool) {
	for _, r := range routes {
		if matchPattern(r.Pattern, upper) {
			return r, true
		}
	}
	return Route{}, false
}
```

- [ ] **Step 5: 重写 selector.go**

把 `internal/collector/selector.go` 整体替换为：

```go
package collector

import (
	"strings"

	"github.com/newthinker/atlas/internal/core"
)

// KnownIndexMarket reports whether a ^-prefixed symbol is in the phase-1
// index list and its market. The app assembly layer warns on unknown ones.
//
// 「已登记」的口径 = 命中的路由不含通配符：'^HSI' 有精确条目故 known，
// '^N225' 落到 '^*' 兜底故 unknown（设计 §6.5，与旧的 indexMarkets 表等价）。
func KnownIndexMarket(symbol string) (core.Market, bool) {
	var zero core.Market
	r, ok := lookupRoute(strings.ToUpper(symbol))
	if !ok || strings.Contains(r.Pattern, "*") {
		return zero, false
	}
	return r.Market, true
}

// warehouseCoverer is implemented by the qlib warehouse collector.
// Using an interface here avoids a direct import of the qlib package.
type warehouseCoverer interface{ Covers(symbol string) bool }

// SelectForSymbol picks the most appropriate registered collector for a symbol.
//
// Routing rules (in priority order):
//  1. qlib warehouse collector covers the symbol → return qlib
//  2. 其余一律查路由表（route.go），具体度优先
//
// If the preferred collector is not registered it falls back to any available
// collector, returning nil only when the registry is empty.
func SelectForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}
	if c, ok := reg.Get("qlib"); ok {
		if cov, ok2 := c.(warehouseCoverer); ok2 && cov.Covers(symbol) {
			return c
		}
	}
	return SelectExternalForSymbol(reg, symbol)
}

// SelectExternalForSymbol routes to an external (non-qlib) collector.
// It applies the same market-based routing as SelectForSymbol but explicitly
// skips the qlib collector to prevent tail-fill delegation loops.
func SelectExternalForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}

	name := routeCollector(symbol)
	if c, ok := reg.Get(name); ok {
		return c
	}

	// Default to Yahoo for US/HK stocks.
	if c, ok := reg.Get("yahoo"); ok {
		return c
	}

	// Fallback: return any available external collector, skipping qlib to
	// prevent infinite delegation when qlib is the only registered collector.
	for _, c := range reg.GetAll() {
		if c.Name() == "qlib" {
			continue
		}
		return c
	}
	return nil
}

// MarketForSymbol infers the trading market from a symbol's pattern.
func MarketForSymbol(symbol string) core.Market {
	// 表前置规则（设计 §6.4 第 2 条）：AShareIndexSecIDs 覆盖 930713.CSI 这类
	// 不带 .SH/.SZ 后缀的中证跨市场指数。键集离散，无法通配。
	if IsAShareIndex(symbol) {
		return core.MarketCNA
	}
	if r, ok := lookupRoute(strings.ToUpper(symbol)); ok {
		return r.Market
	}
	return core.MarketUS
}

// routeCollector 返回符号应走的 collector 名（同样先过 IsAShareIndex 前置规则）。
func routeCollector(symbol string) string {
	if IsAShareIndex(symbol) {
		return "eastmoney"
	}
	if r, ok := lookupRoute(strings.ToUpper(symbol)); ok {
		return r.Collector
	}
	return "yahoo"
}
```

- [ ] **Step 6: 跑黄金值测试确认仍然全绿**

Run: `go test ./internal/collector/ -run 'TestRoute|TestQlibCovers' -v`
Expected: PASS —— 与 Step 2 的基线逐条一致

- [ ] **Step 7: 补一条统一后的行为差异测试**

在 `route_golden_test.go` 末尾追加：

```go
// TestHKSuffixBeatsCryptoPrefix 记录一处**刻意的行为统一**：旧实现里
// "BTC.HK" 的路由与市场自相矛盾（SelectExternalForSymbol → crypto，
// MarketForSymbol → HK）。统一到一张表后取 '.HK' 优先，两者一致。
// 该形态在 watchlist 中不存在，此测试只为把决定钉死，防止后来者反复横跳。
func TestHKSuffixBeatsCryptoPrefix(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	if got := SelectExternalForSymbol(reg, "BTC.HK"); got.Name() != "yahoo" {
		t.Errorf("SelectExternalForSymbol = %q, want yahoo", got.Name())
	}
	if m := MarketForSymbol("BTC.HK"); m != core.MarketHK {
		t.Errorf("MarketForSymbol = %q, want %q", m, core.MarketHK)
	}
}
```

- [ ] **Step 8: 跑 collector 与调用方全量测试**

Run: `go test ./internal/collector/... ./internal/app/... ./internal/api/... -race`
Expected: PASS。`selector_test.go` 的既有用例必须原样通过 —— 若不通过，说明重写改了行为，先修 `route.go` 而不是改测试。

Run: `grep -rn "isAShareSymbol\|isCryptoSymbol\|isIndexSymbol\|isCommoditySymbol\|cryptoTickers\|indexMarkets" internal/collector/`
Expected: 无输出（五个谓词与两张旧表已被路由表取代）

- [ ] **Step 9: 提交**

```bash
go build ./... && go vet ./...
git add internal/collector/route.go internal/collector/selector.go internal/collector/route_golden_test.go
git commit -m "refactor(collector): 符号路由收敛为具体度优先的路由表"
```

---

### Task 12: 防回潮 AST 测试 + prism refresh 集成回归

**Files:**
- Test: `internal/collector/nothrottle_test.go`（新建）
- Test: `internal/prism/quota_degrade_test.go`（新建）

**Interfaces:**
- Consumes: Task 6-9 的 collector 改造；`prism.Refresh` 与 `refresh_test.go` 里既有的 `newFakeStore` / `akCfg` / `aStockInst` / `fakeAkshare` / `fakeLix` / `fakeUS` / `fakeEdgar` 桩
- Produces: 无生产代码

- [ ] **Step 1: 写防回潮测试**

创建 `internal/collector/nothrottle_test.go`：

```go
package collector

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoPrivateThrottleState 扫描 internal/collector 树下所有 collector 源码，
// 断言不再有私有节流状态字段（lastReq）——限流一律走 policy Gate。
//
// Go 没法机制化强制「必须走 Gate」，这是最接近的替代：它只能挡住「照抄旧
// collector 重写一套 mu+lastReq+sleep」这一种回潮方式，不宜高估其强度
// （设计 §7.4）。policy 包自身持有唯一合法的 lastReq，故跳过。
func TestNoPrivateThrottleState(t *testing.T) {
	const forbidden = "lastReq"

	var offenders []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// policy 包是闸门本体，lastReq 在那里是实现而非回潮。
			if d.Name() == "policy" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, fld := range st.Fields.List {
				for _, name := range fld.Names {
					if name.Name == forbidden {
						offenders = append(offenders,
							path+":"+strconv.Itoa(fset.Position(name.Pos()).Line))
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("发现私有节流状态字段 %q（限流须走 internal/collector/policy 的 Gate）:\n  %s",
			forbidden, strings.Join(offenders, "\n  "))
	}
}
```

- [ ] **Step 2: 跑测试确认通过**

Run: `go test ./internal/collector/ -run TestNoPrivateThrottleState -v`
Expected: PASS（Task 6-9 已清干净）。若失败，输出会直接指出哪个文件哪一行漏改。

- [ ] **Step 3: 写 prism 集成回归测试**

创建 `internal/prism/quota_degrade_test.go`：

```go
package prism

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/collector/tushare"
)

// 设计 §7.6：把 tushare.daily_basic 打到配额上限后，降级链行为与「撞墙后降级」
// 一致（Degraded 文案报限频、标的判 Failed），且**不再真的发出那次注定失败的
// HTTP 请求**。账本预置为已用满，等价于「今天前面几次已经用光」。
func TestRefreshTushareLocalQuotaBlocksBeforeHTTP(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","pe_ttm","pb","ps_ttm"],"items":[]}}`))
	}))
	defer srv.Close()

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(loc)
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	ledger := filepath.Join(t.TempDir(), "collector-quota.json")
	require.NoError(t, os.WriteFile(ledger, []byte(fmt.Sprintf(
		`{"tushare.daily_basic":{"window_start":%q,"count":5}}`,
		windowStart.Format(time.RFC3339))), 0o644))

	// 用生产内置表（daily_basic = 5 次/自然日），只把节流间隔归零加速用例。
	tbl := policy.NewTable()
	builtin, _ := tbl.Lookup("tushare.daily_basic")
	builtin.MinInterval = 0
	builtin.TTL = 0
	tbl.Set("tushare.daily_basic", builtin)

	原 := policy.Default()
	defer policy.SetDefault(原)
	policy.SetDefault(policy.New(tbl, policy.NewFileStore(ledger)))

	// 真实 client（非 stub）：这条链路必须端到端成立才算数。
	ts := tushare.NewWithBaseURL("tok", srv.URL)

	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	refreshAt := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, refreshAt)

	mu.Lock()
	got := hits
	mu.Unlock()
	assert.Zero(t, got, "配额已满时不得真的发出 HTTP 请求（撞墙前拦截）")

	require.NotEmpty(t, rep.Degraded, "应产出限频降级提示")
	assert.Contains(t, rep.Degraded[0], "限频", "文案须与撞墙后降级一致，运维口径不变")
	assert.Contains(t, rep.Degraded[0], "600519.SH")
	require.Len(t, rep.Failed, 1, "兜底未成功，标的仍判失败")
	assert.Equal(t, 0, rep.Refreshed)
}

// 验收标准 1 的锚点：refresh.go 靠 errors.Is(err, tushare.ErrRateLimited) 分叉，
// 本地配额预判必须满足同一判定，否则降级链会掉进「未识别错误」分支。
func TestQuotaErrorSatisfiesRateLimitedAssertion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("配额已满时不应发出请求")
	}))
	defer srv.Close()

	tbl := policy.NewTable()
	p, _ := tbl.Lookup("tushare.daily_basic")
	p.MinInterval, p.TTL = 0, 0
	p.Quota = &policy.Quota{Limit: 0, Window: 24 * time.Hour, Loc: time.UTC}
	tbl.Set("tushare.daily_basic", p)

	原 := policy.Default()
	defer policy.SetDefault(原)
	policy.SetDefault(policy.New(tbl, policy.NewMemStore()))

	c := tushare.NewWithBaseURL("tok", srv.URL)
	_, err := c.FetchDailyBasic("600519.SH", time.Now().AddDate(0, 0, -5), time.Now())
	assert.True(t, errors.Is(err, tushare.ErrRateLimited),
		"err = %v，必须满足 errors.Is(err, tushare.ErrRateLimited)", err)
}
```

**若 `Refresh(...)` 的实参个数或桩类型名与 `internal/prism/refresh_test.go` 里 `TestRefreshTusharePermissionNotRetried` 用的不一致**，以那个既有测试为准照抄装配方式（本测试刻意与它同构，只把 `ts` 从 `&fakeTushare{...}` 换成真实 client）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/prism/ -run 'TestRefreshTushareLocalQuota|TestQuotaErrorSatisfies' -v`
Expected: PASS

- [ ] **Step 5: 全量验收**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: PASS

逐条核对设计 §8 验收标准：

```bash
# 1. refresh.go 零改动
git diff --stat master -- internal/prism/refresh.go   # 期望：无输出

# 2. 路由黄金值全绿（新旧实现均已跑过，见 Task 11 Step 2 / Step 6）
go test ./internal/collector/ -run TestRouteGoldenValues -v

# 3. 两个 Gate 实例共享文件时配额跨「进程」生效
go test ./internal/collector/policy/ -run TestFileStoreQuotaSurvivesProcessRestart -v

# 4. tushare 配额耗尽满足 errors.Is(err, ErrRateLimited)
go test ./internal/collector/tushare/ -run TestQuotaExceededMapsToErrRateLimited -v
go test ./internal/prism/ -run TestQuotaErrorSatisfiesRateLimitedAssertion -v

# 5. 各 collector 源码中不再存在 lastReq
go test ./internal/collector/ -run TestNoPrivateThrottleState -v

# 6. lixinger 能进入缓存路径
go test ./internal/collector/lixinger/ -run TestLixingerRequestIsCached -v
```

- [ ] **Step 6: 代码简化**

按 `~/.claude/CLAUDE.md` 的提交前规范，用 Task tool 调用 code-simplifier：

```
subagent_type: "code-simplifier:code-simplifier"
prompt: "请检查并简化本次改动的代码文件：internal/collector/policy/、internal/collector/route.go、internal/collector/selector.go、internal/collector/{yahoo,twelvedata,tushare,lixinger}/、cmd/atlas/policy.go、cmd/atlas/collectors.go"
```

简化结果一并纳入提交。

- [ ] **Step 7: 提交**

```bash
go test ./... -race
git add internal/collector/nothrottle_test.go internal/prism/quota_degrade_test.go
git commit -m "test(collector): 防回潮 AST 断言 + prism 配额撞墙前降级集成回归"
```

---

## 自查记录（写完方案后对着设计文档核对）

**设计条款 → 任务覆盖**

| 设计条款 | 覆盖任务 |
|---|---|
| §3.1 包结构（4 个文件） | Task 1/2/3/4（`quota.go` 合并了接口与内存实现，`quota_mem.go` 不单列） |
| §3.2 两个入口 + 执行链 ①–⑤ | Task 2（Gate）+ Task 3（③ 配额） |
| §3.3 限流域 / 主题两个维度 | Task 1（`Domain` 缺省）+ Task 2（`domainState`）+ Task 6（chart/eps 同域测试） |
| §3.4 组件职责 | Task 1（Policy 表）/ Task 2（Gate）/ Task 3-4（QuotaStore） |
| §3.5 删除 CachedCollector | Task 10 |
| §4.1 未登记 = 零策略；lixinger 例外 | Task 1（`TestLookupUnregisteredTopicIsZeroPolicy`、`TestLixingerWildcardTTLOnly`）+ Task 9 |
| §4.2 内置默认表 + cache.enabled 语义 | Task 1（`ApplyTTL`/`DisableTTL`）+ Task 5（config 接线） |
| §4.3 结构定义 + config 覆盖 | Task 1 + Task 5（`Override`） |
| §4.4 QuotaStore 文件实现 + 计数时机 | Task 4 + Task 3（`TestGateBlocksBeforeSendingRequest`、`TestGateCountsFailedRequests`） |
| §5.1 错误映射 | Task 8 |
| §5.2 各错误路径（5 行） | Task 2（fn 错误 / Timeout / coalesce）+ Task 3（配额拦截 / fail-open） |
| §6.1-6.5 路由表 | Task 11 |
| §7.1 路由黄金值 | Task 11 Step 1-2、6 |
| §7.2 Gate 单元测试 | Task 2 |
| §7.3 QuotaStore 跨进程 | Task 4 |
| §7.4 防回潮断言 | Task 12 |
| §7.5 降级链兼容性 | Task 8 + Task 12 |
| §7.6 prism 集成回归 | Task 12 |
| §7.7 需改写的既有测试 | Task 10（cache_test）、Task 6（yahoo throttle_test）、Task 7/8（td/tushare 节流用例） |
| §8 验收标准 1-6 | Task 12 Step 5 逐条核对 |

**已知偏离**（均在「设计文档之外的实现决定」一节详述并说明理由）：`Gate.Wait` 第三入口、新增 `yahoo.quote` 主题、`<域>.*` 通配兜底、`QuotaStore.Take` 传 `Quota` 结构、缓存返回值由调用方 clone、`quota_mem.go` 并入 `quota.go`、`BTC.HK` 行为统一。

**未覆盖的设计条款**：无。§9「不在本次范围」的四项（数据新鲜度元数据、meta 层 request_id、给 `Collector` 接口加 `context.Context`、为 eastmoney/akshare 等配限流）本方案均未触碰。
