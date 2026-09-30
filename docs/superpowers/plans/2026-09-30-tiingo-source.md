# Tiingo 美股备用数据源 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Tiingo 接为美股日线价格备源：prism 美股价格链 `Yahoo → Tiingo → Twelve Data`，`atlas serve` 注册为最后一个外部兜底 collector。

**Architecture:** 新包 `internal/collector/tiingo`（symbols / client / collector 三个文件）；policy 内置表登记 `tiingo.daily`（小时配额 40）；`collector.Registry` 改为按注册顺序返回；prism 的单个 `td` 参数改为有序 `[]PriceHop`；serve 装配在 tushare/baostock 之后注册 tiingo。

**Tech Stack:** Go 1.24、`net/http`、`httptest`、testify、既有 `internal/collector/policy` 闸门。

**Spec:** `docs/superpowers/specs/2026-09-30-tiingo-source-design.md`

## Global Constraints

- 一期只做美股（含 ETF）；A 股 / 港股 / 指数 / 期货 / 加密一律由白名单拒绝，**不发 HTTP、不占配额**。
- 端点：`GET https://api.tiingo.com/tiingo/daily/<ticker>/prices?startDate=YYYY-MM-DD`，**不带 `endDate`**。
- 鉴权只走请求头 `Authorization: Token <key>`；token 不进 URL、不进日志、不进仓库。
- HTTP 客户端强制直连：`Transport` 为 `http.DefaultTransport` 的 Clone 且 `Proxy = nil`。
- 价格口径 = yahoo chart `quote.close`：拆股调整、不含分红。`t` 日 O/H/L/C ÷ `t` 之后（不含 `t`）所有行 `splitFactor` 之积；Volume × 同一积并四舍五入。折算在截取 `[start, end]` 之前对全量行做。
- policy 主题 `tiingo.daily`：`Policy{TTL: builtinTTL, Coalesce: true, Quota: &Quota{Limit: 40, Window: time.Hour}}`。
- 所有 error 出口经 `wrapErr`：前缀 `tiingo: `、token 替换为 `<redacted>`、`%v` 断链。
- 缺 key 或未启用：prism 与 serve 均不接入，行为与现状逐字一致。
- 改动现有符号前做 gitnexus impact：MCP 不可用时用 `npx -y gitnexus@latest impact <sym> --direction upstream --repo atlas`；结果为 `UNKNOWN` 时必须用 `grep` 补查调用方并记录。
- 提交前：① 调用 `code-simplifier:code-simplifier` 子代理简化本任务改动文件；② `npx -y gitnexus@latest detect-changes --scope staged --repo atlas`。提交信息 `feat(tiingo): …` / `refactor(collector): …`，结尾 `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`。
- 分支：`feature/tiingo-source`（已存在，spec 在其上）。
- 集成测试用 `//go:build integration`，token 取环境变量 `ATLAS_TIINGO_TOKEN`，缺失则 `t.Skip`。

## Review Focus

1. **结束日之后的拆股**：`end` 早于某次拆股时，`end` 之前的价格仍须按该拆股折算（否则与 yahoo 差 N 倍）—— Task 4 `TestFetchHistoryEndBeforeLaterSplitStillAdjusted`。
2. **进程带 `https_proxy` 时仍直连**：用 localhost httptest 测不出（Go 对 loopback 本就不走代理），须结构断言 —— Task 4 `TestClientTransportIgnoresProxy`。
3. **错误文本泄露 token**：传输层错误、HTTP 错误、decode 错误都不能带出 key —— Task 4 `TestFetchHistoryStatusErrorsRedacted`、`TestFetchHistoryTransportErrorRedacted`。
4. **非美股代码消耗配额**：serve 兜底循环会把 A 股 / 港股 / 指数交给 tiingo —— Task 3 `TestSupported` + Task 5 `TestCollectorRejectsUnsupportedWithoutHTTP`。
5. **prism 无备用跳时的错误串回归**：`hops` 为空或只有 twelvedata 时错误与降级文案须与现状逐字一致 —— Task 6 `TestFetchClosesNoHopsKeepsPrefix`、`TestFetchClosesSingleHopMatchesLegacyFormat`。

---

## File Structure

| 文件 | 动作 | 职责 |
| --- | --- | --- |
| `internal/collector/registry.go` | Modify | 增加 `order []string`，`GetAll` 按注册顺序 |
| `internal/collector/registry_test.go` | Modify | 顺序与同名覆盖测试 |
| `internal/collector/policy/policy.go` | Modify | 登记 `tiingo.daily` |
| `internal/collector/policy/policy_test.go` | Modify | Topics 集合加 `tiingo.daily`；配额断言 |
| `internal/collector/tiingo/symbols.go` | Create | `Supported`、`toTicker` |
| `internal/collector/tiingo/client.go` | Create | 直连 HTTP、折算、截取、错误映射 |
| `internal/collector/tiingo/collector.go` | Create | `collector.Collector` 实现 |
| `internal/collector/tiingo/*_test.go` + `testdata/nvda_split_sample.json` | Create | 单测与样本 |
| `internal/collector/tiingo/client_integration_test.go` | Create | 真实 API 冒烟 |
| `internal/prism/refresh.go` | Modify | `PriceHop`、`fetchCloses` 多跳、`Refresh` 签名 |
| `internal/prism/refresh_test.go` | Modify | 两处 `td` 实参改为 hops；新增多跳测试 |
| `cmd/atlas/prism.go` / `prism_test.go` | Modify | `usPriceHops` 组装 |
| `cmd/atlas/collectors.go` / `collectors_test.go` | Modify | 注册 tiingo |
| `cmd/atlas/gate_wiring_test.go` | Modify | `collectorCtors` 加 `tiingo.New` |
| `configs/config.example.yaml` | Modify | `collectors.tiingo` 示例 |

---

### Task 1: Registry 按注册顺序返回

**Files:**
- Modify: `internal/collector/registry.go`
- Test: `internal/collector/registry_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `(*Registry).GetAll() []Collector` 语义变为「按首次注册顺序」；`Register` 同名覆盖保持原位置。签名不变。

- [ ] **Step 1: impact 分析**

Run: `npx -y gitnexus@latest impact "GetAll" --direction upstream --repo atlas`
然后（因同名方法多、索引可能落后，结果多为 `UNKNOWN`）补查：
Run: `grep -rnE '\.GetAll\(\)' --include='*.go' internal cmd | grep -v _test`
Expected: collector Registry 的调用方为 `internal/app/app.go`（orderedCollectors、统计计数、GetCollectors）与 `internal/collector/selector.go:66`；把结论记进提交说明。

- [ ] **Step 2: 写失败测试**（追加到 `internal/collector/registry_test.go`）

```go
func TestRegistry_GetAllKeepsRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	names := []string{"yahoo", "eastmoney", "crypto", "tushare", "baostock", "tiingo", "qlib"}
	for _, n := range names {
		r.Register(&mockCollector{name: n})
	}
	for i := 0; i < 100; i++ { // map 迭代顺序随机：多次调用才能暴露
		all := r.GetAll()
		got := make([]string, len(all))
		for j, c := range all {
			got[j] = c.Name()
		}
		if !reflect.DeepEqual(got, names) {
			t.Fatalf("GetAll 第 %d 次 = %v, want %v", i, got, names)
		}
	}
}

func TestRegistry_ReRegisterKeepsPosition(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockCollector{name: "a"})
	r.Register(&mockCollector{name: "b"})
	replacement := &mockCollector{name: "a"}
	r.Register(replacement)

	all := r.GetAll()
	if len(all) != 2 || all[0] != replacement || all[1].Name() != "b" {
		t.Fatalf("同名重注册应覆盖且保持原位置, got %v", all)
	}
}
```

在文件 import 中加入 `"reflect"`。

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/collector/ -run 'TestRegistry_' -count=1 -v`
Expected: `TestRegistry_GetAllKeepsRegistrationOrder` FAIL（顺序随机）。

- [ ] **Step 4: 实现** —— `internal/collector/registry.go` 的 `Registry`、`NewRegistry`、`Register`、`GetAll` 改为：

```go
// Registry manages collector plugins
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
	// order 记录首次注册顺序。GetAll 必须确定性地按它返回：app.orderedCollectors
	// 把「首选之外」的 collector 当作有序兜底链逐个尝试，map 迭代顺序是随机的。
	order []string
}

// NewRegistry creates a new collector registry
func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a collector to the registry. 同名重注册覆盖实例但保持原位置。
func (r *Registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := c.Name()
	if _, ok := r.collectors[name]; !ok {
		r.order = append(r.order, name)
	}
	r.collectors[name] = c
}
```

```go
// GetAll returns all registered collectors in registration order.
func (r *Registry) GetAll() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Collector, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.collectors[name])
	}
	return result
}
```

- [ ] **Step 5: 运行确认通过，并回归依赖方**

Run: `go test ./internal/collector/ ./internal/app/ ./cmd/atlas/ -count=1`
Expected: 三个包 `ok`。

- [ ] **Step 6: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/registry.go internal/collector/registry_test.go
git commit -m "refactor(collector): Registry.GetAll 按注册顺序返回，兜底链顺序确定"
```

---

### Task 2: policy 内置表登记 `tiingo.daily`

**Files:**
- Modify: `internal/collector/policy/policy.go`（`NewTable` 内 `twelvedata.time_series` 那行之后）
- Test: `internal/collector/policy/policy_test.go`

**Interfaces:**
- Consumes: 无
- Produces: 主题 `"tiingo.daily"`，Domain `"tiingo"`，`Quota{Limit: 40, Window: time.Hour}`。

- [ ] **Step 1: 写失败测试**

在 `policy_test.go` 的 Topics 集合等值测试 `want` 切片中，`"twelvedata.time_series", "lixinger.*",` 之后加入 `"tiingo.daily",`，并同步更新该处注释里的主题计数说明（如有「9 个主题」字样改为实际数）。

追加新测试：

```go
func TestBuiltinTiingoDailyQuota(t *testing.T) {
	p, ok := NewTable().Lookup("tiingo.daily")
	if !ok {
		t.Fatal("tiingo.daily 应为内置主题")
	}
	if p.Domain != "tiingo" {
		t.Errorf("Domain = %q, want tiingo", p.Domain)
	}
	if p.Quota == nil || p.Quota.Limit != 40 || p.Quota.Window != time.Hour {
		t.Errorf("Quota = %+v, want 40/1h（免费档 50/h、1000/d：40×24=960 兼顾日上限）", p.Quota)
	}
	if p.MinInterval != 0 {
		t.Errorf("MinInterval = %v, want 0（只靠配额，不节流）", p.MinInterval)
	}
	if !p.Coalesce || p.TTL != builtinTTL {
		t.Errorf("Coalesce/TTL = %v/%v, want true/%v", p.Coalesce, p.TTL, builtinTTL)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/policy/ -count=1`
Expected: FAIL（Topics 集合不等；`tiingo.daily 应为内置主题`）。

- [ ] **Step 3: 实现** —— 在 `t.Set("twelvedata.time_series", …)` 之后加入：

```go
	// tiingo 免费档 50 次/时、1000 次/日：只设小时配额 40（40×24=960 < 1000 兼顾日上限），
	// 超额即失败（设计 docs/superpowers/specs/2026-09-30-tiingo-source-design.md §3.4）。
	// 不在这里写长 TTL：ApplyTTL 会用全局 collector.cache.ttl 覆盖它；需要长 TTL 请用
	// collector.topics."tiingo.daily".ttl 覆盖。
	t.Set("tiingo.daily", Policy{TTL: builtinTTL, Coalesce: true,
		Quota: &Quota{Limit: 40, Window: time.Hour}})
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/policy/ -count=1`
Expected: `ok`。

- [ ] **Step 5: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/policy/policy.go internal/collector/policy/policy_test.go
git commit -m "feat(tiingo): policy 内置表登记 tiingo.daily（小时配额 40）"
```

---

### Task 3: 代码白名单 `symbols.go`

**Files:**
- Create: `internal/collector/tiingo/symbols.go`
- Test: `internal/collector/tiingo/symbols_test.go`

**Interfaces:**
- Consumes: `collector.MarketForSymbol(symbol string) core.Market`（`internal/collector/selector.go`）
- Produces: `func Supported(symbol string) bool`；未导出 `func toTicker(symbol string) string`

- [ ] **Step 1: 写失败测试**

```go
package tiingo

import "testing"

func TestSupported(t *testing.T) {
	cases := map[string]bool{
		"AAPL": true, "SPY": true, "BRK-B": true, "BRK.B": true, "QQQ": true,
		"600036.SH": false, "000001.SZ": false, "930713.CSI": false, // A 股 / 中证指数
		"0700.HK": false, "03968.HK": false, // 港股
		"^GSPC": false, "^HSI": false, // 指数
		"GC=F": false, // 期货
		"BTC-USD": false, "BTCUSDT": false, "ETH": false, // 加密
		"": false, "aapl": false, "TOOLONGTICKER1": false,
	}
	for sym, want := range cases {
		if got := Supported(sym); got != want {
			t.Errorf("Supported(%q) = %v, want %v", sym, got, want)
		}
	}
}

func TestToTicker(t *testing.T) {
	if got := toTicker("BRK.B"); got != "BRK-B" {
		t.Errorf("toTicker(BRK.B) = %q, want BRK-B", got)
	}
	if got := toTicker("AAPL"); got != "AAPL" {
		t.Errorf("toTicker(AAPL) = %q", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/tiingo/ -run 'TestSupported|TestToTicker' -count=1`
Expected: 编译失败 `undefined: Supported`。

- [ ] **Step 3: 实现**

```go
package tiingo

import (
	"regexp"
	"strings"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// reTicker 是美股/ETF 代码形态：大写字母数字开头，可含 '.'/'-'（份额类），≤10 字符。
// '^'（指数）与 '='（期货）不在字符集内，天然被拒。
var reTicker = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.\-]{0,9}$`)

// Supported 报告 symbol 是否属于一期覆盖范围（美股/ETF，设计 §3.2）。
// 其余一律拒绝且不发请求：serve 兜底循环会把任意标的交给最后一跳，
// 不拦截就会用 A 股/港股/指数白白消耗小时配额。
func Supported(symbol string) bool {
	return reTicker.MatchString(symbol) && collector.MarketForSymbol(symbol) == core.MarketUS
}

// toTicker 把 atlas 代码转为 Tiingo ticker：份额类分隔符 '.' 改 '-'（BRK.B → BRK-B）。
func toTicker(symbol string) string { return strings.ReplaceAll(symbol, ".", "-") }
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/tiingo/ -count=1 -v`
Expected: PASS。若某个加密形态（如 `ETH`）未按预期落到 crypto 路由，以 `internal/collector/route.go` 实际路由为准修正**测试期望**并在测试注释写明依据，不改路由表。

- [ ] **Step 5: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/tiingo/symbols.go internal/collector/tiingo/symbols_test.go
git commit -m "feat(tiingo): 美股代码白名单"
```

---

### Task 4: HTTP 客户端 `client.go`（直连、折算、错误映射）

**Files:**
- Create: `internal/collector/tiingo/client.go`
- Create: `internal/collector/tiingo/testdata/nvda_split_sample.json`
- Test: `internal/collector/tiingo/client_test.go`、`internal/collector/tiingo/main_test.go`

**Interfaces:**
- Consumes: `policy.Default()`、`policy.Fetch[T]`、`policy.ErrTimeout`、`policy.ErrQuotaExceeded`、`core.OHLCV`；Task 3 `toTicker`
- Produces:
  - `type Client struct{ apiKey, baseURL string; hc *http.Client; gate *policy.Gate }`
  - `func New(apiKey string) *Client`
  - `func NewWithBaseURL(apiKey, baseURL string) *Client`
  - `func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error)`（满足 `prism.TwelvedataClient`）

- [ ] **Step 1: 写样本** `testdata/nvda_split_sample.json`（收盘价为 2026-09-30 实测 Tiingo 原始值，NVDA 2024-06-10 拆股 10:1；O/H/L/Volume 为构造值：拆股前 H=C+10、L=C−10、Vol=1000，拆股后 H=C+1、L=C−1、Vol=10000）

```json
[
  {"date": "2024-06-03T00:00:00.000Z", "open": 1150.0, "high": 1160.0, "low": 1140.0, "close": 1150.0, "volume": 1000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-04T00:00:00.000Z", "open": 1164.37, "high": 1174.37, "low": 1154.37, "close": 1164.37, "volume": 1000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-05T00:00:00.000Z", "open": 1224.4, "high": 1234.4, "low": 1214.4, "close": 1224.4, "volume": 1000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-06T00:00:00.000Z", "open": 1209.98, "high": 1219.98, "low": 1199.98, "close": 1209.98, "volume": 1000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-07T00:00:00.000Z", "open": 1208.88, "high": 1218.88, "low": 1198.88, "close": 1208.88, "volume": 1000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-10T00:00:00.000Z", "open": 121.79, "high": 122.79, "low": 120.79, "close": 121.79, "volume": 10000, "divCash": 0.0, "splitFactor": 10.0},
  {"date": "2024-06-11T00:00:00.000Z", "open": 120.91, "high": 121.91, "low": 119.91, "close": 120.91, "volume": 10000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-12T00:00:00.000Z", "open": 125.2, "high": 126.2, "low": 124.2, "close": 125.2, "volume": 10000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-13T00:00:00.000Z", "open": 129.61, "high": 130.61, "low": 128.61, "close": 129.61, "volume": 10000, "divCash": 0.0, "splitFactor": 1.0},
  {"date": "2024-06-14T00:00:00.000Z", "open": 131.88, "high": 132.88, "low": 130.88, "close": 131.88, "volume": 10000, "divCash": 0.0, "splitFactor": 1.0}
]
```

- [ ] **Step 2: 写测试闸门** `main_test.go`（生产配额 40/h 会让整包用例互相挤占；TTL 缓存会让同 key 用例串味）

```go
package tiingo

import (
	"os"
	"testing"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// testDefaultGate：零策略闸门（不缓存、不节流、不计配额）。临时 SetDefault 的用例
// 必须在 t.Cleanup 里恢复到它，而不是 nil——nil 会懒构造带 40/h 配额的内置表闸门。
var testDefaultGate *policy.Gate

func TestMain(m *testing.M) {
	testDefaultGate = gateWith(policy.Policy{}, nil)
	policy.SetDefault(testDefaultGate)
	os.Exit(m.Run())
}

func gateWith(p policy.Policy, q policy.QuotaStore) *policy.Gate {
	tbl := policy.NewTable()
	p.Domain = "tiingo"
	tbl.Set(topicDaily, p)
	return policy.New(tbl, q)
}
```

- [ ] **Step 3: 写失败测试** `client_test.go`

```go
package tiingo

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/policy"
)

const testKey = "secret-key-123"

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// reqLog 记录 handler 看到的最后一次请求。handler 跑在服务端 goroutine，
// 经网络的同步对 race detector 不可见，故必须加锁（否则 -race 误报）。
type reqLog struct {
	mu   sync.Mutex
	url  url.URL
	auth string
	n    int
}

func (l *reqLog) last() (url.URL, string, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url, l.auth, l.n
}

// serveFixture 返回一个回放 NVDA 样本的服务器及其请求记录。
func serveFixture(t *testing.T) (*httptest.Server, *reqLog) {
	t.Helper()
	body, err := os.ReadFile("testdata/nvda_split_sample.json")
	require.NoError(t, err)
	log := &reqLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.mu.Lock()
		log.url, log.auth, log.n = *r.URL, r.Header.Get("Authorization"), log.n+1
		log.mu.Unlock()
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

func TestFetchHistoryRequestShape(t *testing.T) {
	srv, log := serveFixture(t)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)

	u, auth, _ := log.last()
	assert.Equal(t, "/tiingo/daily/NVDA/prices", u.Path)
	assert.Equal(t, "2024-06-03", u.Query().Get("startDate"))
	assert.False(t, u.Query().Has("endDate"), "不带 endDate：end 之后的拆股必须可见")
	assert.NotContains(t, u.RawQuery, testKey, "token 不进 URL")
	assert.Equal(t, "Token "+testKey, auth)
}

func TestFetchHistoryTickerMapping(t *testing.T) {
	srv, log := serveFixture(t)
	_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("BRK.B", day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)
	u, _, _ := log.last()
	assert.Equal(t, "/tiingo/daily/BRK-B/prices", u.Path)
}

func TestFetchHistorySplitNormalizedAndClipped(t *testing.T) {
	srv, _ := serveFixture(t)
	bars, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-05"), day("2024-06-11"))
	require.NoError(t, err)
	require.Len(t, bars, 5, "闭区间 [06-05, 06-11] 共 5 个交易日")

	assert.Equal(t, day("2024-06-05"), bars[0].Time)
	assert.InDelta(t, 122.44, bars[0].Close, 1e-9, "拆股前 ÷10，与 yahoo close 同口径")
	assert.InDelta(t, 123.44, bars[0].High, 1e-9)
	assert.InDelta(t, 121.44, bars[0].Low, 1e-9)
	assert.Equal(t, int64(10000), bars[0].Volume, "成交量 ×10")
	assert.Equal(t, day("2024-06-10"), bars[3].Time)
	assert.InDelta(t, 121.79, bars[3].Close, 1e-9, "拆股当日及之后不折算")
	assert.Equal(t, int64(10000), bars[3].Volume)
	for i, b := range bars {
		assert.Equal(t, "NVDA", b.Symbol)
		assert.Equal(t, "1d", b.Interval)
		if i > 0 {
			assert.True(t, bars[i-1].Time.Before(b.Time), "升序")
		}
	}
}

func TestFetchHistoryEndBeforeLaterSplitStillAdjusted(t *testing.T) {
	srv, _ := serveFixture(t)
	bars, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("NVDA", day("2024-06-03"), day("2024-06-07"))
	require.NoError(t, err)
	require.Len(t, bars, 5)
	assert.InDelta(t, 115.0, bars[0].Close, 1e-9, "end 早于拆股日，仍须按其后的拆股折算")
	assert.InDelta(t, 120.888, bars[4].Close, 1e-9)
}

func TestFetchHistorySkipsBadRowsAndRejectsBadSplit(t *testing.T) {
	serve := func(body string) *Client {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return NewWithBaseURL(testKey, srv.URL)
	}

	c := serve(`[
	  {"date": "bad", "open": 1, "high": 1, "low": 1, "close": 1, "volume": 1, "splitFactor": 1},
	  {"date": "2024-06-03T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": null, "volume": 1, "splitFactor": 1},
	  {"date": "2024-06-04T00:00:00.000Z", "open": null, "high": 1, "low": 1, "close": 1, "volume": 1, "splitFactor": 1},
	  {"date": "2024-06-05T00:00:00.000Z", "open": 2, "high": 3, "low": 1, "close": 2, "volume": null, "splitFactor": 1}
	]`)
	bars, err := c.FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.NoError(t, err)
	require.Len(t, bars, 1, "坏日期 / close 缺失 / OHL 缺失的行跳过")
	assert.Equal(t, day("2024-06-05"), bars[0].Time)
	assert.Equal(t, int64(0), bars[0].Volume, "volume 缺失记 0")

	c = serve(`[{"date": "2024-06-05T00:00:00.000Z", "open": 2, "high": 3, "low": 1, "close": 2, "volume": 5, "splitFactor": 0}]`)
	_, err = c.FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid splitFactor", "宁可无数据也不写入折算错误的价格")

	// 拆股当日价格缺失：该行不输出，但因子仍须累乘，否则此前价格错一个倍数。
	c = serve(`[
	  {"date": "2024-06-03T00:00:00.000Z", "open": 100, "high": 100, "low": 100, "close": 100, "volume": 1, "splitFactor": 1},
	  {"date": "2024-06-04T00:00:00.000Z", "open": null, "high": null, "low": null, "close": null, "volume": null, "splitFactor": 2}
	]`)
	bars, err = c.FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.NoError(t, err)
	require.Len(t, bars, 1)
	assert.Equal(t, 50.0, bars[0].Close, "缺价行的 splitFactor 仍参与折算")
}

func TestFetchHistoryStatusErrorsRedacted(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{404, `{"detail": "Error: Ticker 'ZZZZ' not found"}`, "not found: Error: Ticker 'ZZZZ' not found"},
		{403, `{"detail": "token ` + testKey + ` invalid"}`, "not retryable"},
		{401, `{"detail": "unauthorized"}`, "not retryable"},
		{429, `{"detail": "slow down"}`, "rate limited (retryable)"},
		{500, `oops ` + testKey, "HTTP 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := NewWithBaseURL(testKey, srv.URL).FetchHistory("ZZZZ", day("2024-06-01"), day("2024-06-30"))
		srv.Close()
		require.Error(t, err, "status %d", tc.status)
		assert.Contains(t, err.Error(), tc.want)
		assert.True(t, strings.HasPrefix(err.Error(), "tiingo: "))
		assert.NotContains(t, err.Error(), testKey, "错误文本不得含 token")
	}
}

func TestFetchHistoryTransportErrorRedacted(t *testing.T) {
	_, err := NewWithBaseURL(testKey, "http://127.0.0.1:1").FetchHistory("AAPL", day("2024-06-01"), day("2024-06-30"))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testKey)
	assert.True(t, strings.HasPrefix(err.Error(), "tiingo: "))
}

// 进程级 https_proxy 下必须直连（经 7897 代理 TLS 必然失败，设计 §2.1）。
// ⚠ 不能用「设代理环境变量后请求仍到达 httptest」来测：Go 的 ProxyFromEnvironment
// 对 localhost/127.0.0.1 本就不走代理，那种写法有无 Proxy:nil 都会通过。故做结构断言。
func TestClientTransportIgnoresProxy(t *testing.T) {
	tr, ok := New(testKey).hc.Transport.(*http.Transport)
	require.True(t, ok, "Transport 须为 *http.Transport")
	assert.Nil(t, tr.Proxy, "Proxy 必须为 nil：不继承 HTTP(S)_PROXY")
}

func TestFetchHistoryQuotaExceededIsRetryable(t *testing.T) {
	g := gateWith(policy.Policy{Quota: &policy.Quota{Limit: 1, Window: time.Hour}}, policy.NewMemStore())
	policy.SetDefault(g)
	t.Cleanup(func() { policy.SetDefault(testDefaultGate) })

	srv, log := serveFixture(t)
	c := NewWithBaseURL(testKey, srv.URL) // 构造时快照 Default()
	_, err := c.FetchHistory("NVDA", day("2024-06-03"), day("2024-06-14"))
	require.NoError(t, err)
	_, err = c.FetchHistory("NVDA", day("2024-06-04"), day("2024-06-14")) // 不同 key，避开合并
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retryable")
	_, _, n := log.last()
	assert.Equal(t, 1, n, "超额时不得发出 HTTP")
}
```

- [ ] **Step 4: 运行确认失败**

Run: `go test ./internal/collector/tiingo/ -count=1`
Expected: 编译失败 `undefined: NewWithBaseURL` / `topicDaily`。

- [ ] **Step 5: 实现** `client.go`

```go
// Package tiingo 直连 Tiingo 日线 API（GET /tiingo/daily/<ticker>/prices，Token 头鉴权）。
// 角色 = 美股价格备源（设计 docs/superpowers/specs/2026-09-30-tiingo-source-design.md）：
// prism 美股价格链 yahoo→tiingo→twelvedata 的第二跳，以及 serve 分析循环的最后一个外部兜底。
//
// 凭证只走 Authorization 请求头、绝不进 URL（同 twelvedata ADR#7：error 会经 prism
// 报告外发到 Telegram，*url.Error 携带完整 URL）；所有 error 出口经 wrapErr 脱敏断链。
package tiingo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
)

const (
	defaultBaseURL = "https://api.tiingo.com"
	// topicDaily 的配额（40 次/时）登记在 policy 内置表，本包不持有节流状态。
	topicDaily = "tiingo.daily"
	// maxBody 防御性上限：不带 endDate 的全量日线约 60 字节/行，30 年 ≈ 0.5MB。
	maxBody = 32 << 20
)

type Client struct {
	apiKey  string
	baseURL string
	hc      *http.Client
	gate    *policy.Gate
}

// New 指向生产端点。
func New(apiKey string) *Client { return NewWithBaseURL(apiKey, defaultBaseURL) }

// NewWithBaseURL 允许注入自定义端点（测试用 httptest server）。gate 在构造时快照
// policy.Default()：测试里的 policy.SetDefault 必须发生在本函数之前才生效。
func NewWithBaseURL(apiKey, baseURL string) *Client {
	// Proxy=nil 强制直连：经本机代理 127.0.0.1:7897 访问 api.tiingo.com 实测 TLS 握手
	// 必然失败，而部分 launchd 任务设置了进程级 https_proxy（设计 §2.1）。
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second, Transport: tr},
		gate:    policy.Default(),
	}
}

// wrapErr 是本包唯一的 error 出口：加前缀、抹掉 apiKey、用 %v 断链
// （留链则 errors.Unwrap 可取回未脱敏原文）。空 key 不替换。
func (c *Client) wrapErr(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if c.apiKey != "" {
		msg = strings.ReplaceAll(msg, c.apiKey, "<redacted>")
	}
	return fmt.Errorf("tiingo: %s", msg)
}

// mapPolicyErr 把 policy 哨兵错误换成本包的临时错误；必须在 policy.Fetch 返回处调用。
// 临时性绝不可映射成永久性：文本显式写 retryable。
func (c *Client) mapPolicyErr(err error) error {
	if errors.Is(err, policy.ErrTimeout) || errors.Is(err, policy.ErrQuotaExceeded) {
		return c.wrapErr("temporary gate failure (retryable): %v", err)
	}
	return err
}

// priceRow 用指针区分 null 与 0。
type priceRow struct {
	Date        string   `json:"date"`
	Open        *float64 `json:"open"`
	High        *float64 `json:"high"`
	Low         *float64 `json:"low"`
	Close       *float64 `json:"close"`
	Volume      *float64 `json:"volume"`
	SplitFactor *float64 `json:"splitFactor"`
}

// FetchHistory 经 Gate 拉取 [start, end] 闭区间（按日）的日线，已折算为 yahoo close 口径。
// 返回值 slices.Clone：Gate 命中缓存时不复制，多个调用方共享底层数组。
func (c *Client) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	key := fmt.Sprintf("%s|%s|%s", symbol, start.Format("2006-01-02"), end.Format("2006-01-02"))
	out, err := policy.Fetch(c.gate, topicDaily, key, func() ([]core.OHLCV, error) {
		return c.fetchHistory(symbol, start, end)
	})
	if err != nil {
		return nil, c.mapPolicyErr(err)
	}
	return slices.Clone(out), nil
}

func (c *Client) fetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	// 刻意不带 endDate：end 之后的拆股必须可见，否则折算漏因子（设计 §2.4）。
	q := url.Values{"startDate": {start.Format("2006-01-02")}}
	u := fmt.Sprintf("%s/tiingo/daily/%s/prices?%s", c.baseURL, url.PathEscape(toTicker(symbol)), q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, c.wrapErr("%s: build request: %v", symbol, err)
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.wrapErr("%s: %v", symbol, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, c.wrapErr("%s: read body: %v", symbol, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.statusErr(symbol, resp.StatusCode, raw)
	}
	var rows []priceRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, c.wrapErr("%s: decode: %v", symbol, err)
	}
	out, err := normalize(symbol, rows, start, end)
	if err != nil {
		return nil, c.wrapErr("%v", err)
	}
	return out, nil
}

// statusErr 按运维动作分类：404 标的不存在；400/401/403 是配置/权限问题，重试无意义；
// 429 可重试。detail 优先取 Tiingo 的 {"detail": "..."}，否则取截断的 body。
func (c *Client) statusErr(symbol string, code int, body []byte) error {
	var e struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &e)
	detail := e.Detail
	if detail == "" {
		detail = strings.ToValidUTF8(string(body[:min(len(body), 200)]), "")
	}
	switch code {
	case http.StatusNotFound:
		return c.wrapErr("%s: not found: %s", symbol, detail)
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return c.wrapErr("%s: permission/config error (not retryable): HTTP %d: %s", symbol, code, detail)
	case http.StatusTooManyRequests:
		return c.wrapErr("%s: rate limited (retryable): %s", symbol, detail)
	default:
		return c.wrapErr("%s: HTTP %d: %s", symbol, code, detail)
	}
}

// normalize 把 Tiingo 原始价折算为 yahoo chart close 口径（拆股调整、不含分红，设计 §2.4）
// 并截取 [start, end]（按日，闭区间）。折算须在截取之前对全量行进行：
// t 日 O/H/L/C ÷ t 之后（不含 t）所有行 splitFactor 之积，Volume × 同一积。
// 坏日期的行整行丢弃；O/H/L/C 任一缺失的行不输出，但其 splitFactor 仍参与累乘——
// 拆股当日恰好缺价时若连因子一起丢，此前全部价格会错一个倍数。
// 任一 splitFactor 非有限或 ≤0 → 整段失败。
func normalize(symbol string, rows []priceRow, start, end time.Time) ([]core.OHLCV, error) {
	type bar struct {
		t time.Time
		r priceRow
	}
	var bars []bar
	for _, r := range rows {
		if len(r.Date) < 10 {
			continue
		}
		t, err := time.Parse("2006-01-02", r.Date[:10])
		if err != nil {
			continue
		}
		bars = append(bars, bar{t, r})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].t.Before(bars[j].t) })

	lo := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	hi := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]core.OHLCV, 0, len(bars))
	cum := 1.0
	for i := len(bars) - 1; i >= 0; i-- {
		b := bars[i]
		hasPrice := b.r.Open != nil && b.r.High != nil && b.r.Low != nil && b.r.Close != nil
		if hasPrice && !b.t.Before(lo) && !b.t.After(hi) {
			vol := 0.0
			if b.r.Volume != nil {
				vol = *b.r.Volume
			}
			out = append(out, core.OHLCV{
				Symbol: symbol, Interval: "1d", Time: b.t,
				Open: *b.r.Open / cum, High: *b.r.High / cum, Low: *b.r.Low / cum, Close: *b.r.Close / cum,
				Volume: int64(math.Round(vol * cum)),
			})
		}
		sf := 1.0
		if b.r.SplitFactor != nil {
			sf = *b.r.SplitFactor
		}
		if math.IsNaN(sf) || math.IsInf(sf, 0) || sf <= 0 {
			return nil, fmt.Errorf("%s: invalid splitFactor %v on %s", symbol, sf, b.t.Format("2006-01-02"))
		}
		cum *= sf
	}
	slices.Reverse(out)
	return out, nil
}
```

- [ ] **Step 6: 运行确认通过（含 race）**

Run: `go test ./internal/collector/tiingo/ -count=1 -race -v`
Expected: 全部 PASS。

- [ ] **Step 7: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/tiingo/client.go internal/collector/tiingo/client_test.go internal/collector/tiingo/main_test.go internal/collector/tiingo/testdata/nvda_split_sample.json
git commit -m "feat(tiingo): 直连日线客户端，拆股折算对齐 yahoo 口径"
```

---

### Task 5: `collector.Collector` 实现

**Files:**
- Create: `internal/collector/tiingo/collector.go`
- Modify: `cmd/atlas/gate_wiring_test.go`（`collectorCtors` 加 `"tiingo.New": true`）
- Test: `internal/collector/tiingo/collector_test.go`

**Interfaces:**
- Consumes: Task 3 `Supported`；Task 4 `Client`、`New`、`(*Client).FetchHistory`；`collector.Config`、`core.Quote`
- Produces:
  - `type Collector struct{ client *Client; now func() time.Time }`
  - `func NewCollector(apiKey string) *Collector`
  - 方法：`Name() string`（`"tiingo"`）、`SupportedMarkets() []core.Market`、`Init(collector.Config) error`、`Start(context.Context) error`、`Stop() error`、`FetchQuote(string) (*core.Quote, error)`、`FetchHistory(string, time.Time, time.Time, string) ([]core.OHLCV, error)`

- [ ] **Step 1: 写失败测试** `collector_test.go`

```go
package tiingo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

var _ collector.Collector = (*Collector)(nil)

func testCollector(t *testing.T, body string) (*Collector, *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&n, 1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Collector{client: NewWithBaseURL(testKey, srv.URL), now: func() time.Time { return day("2026-09-30") }}, &n
}

func TestCollectorRejectsUnsupportedWithoutHTTP(t *testing.T) {
	c, n := testCollector(t, "[]")
	for _, sym := range []string{"600036.SH", "0700.HK", "^GSPC", "GC=F", "BTC-USD"} {
		_, err := c.FetchHistory(sym, day("2026-09-01"), day("2026-09-30"), "1d")
		require.Error(t, err, sym)
		assert.Contains(t, err.Error(), "unsupported symbol")
		_, err = c.FetchQuote(sym)
		require.Error(t, err, sym)
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(n), "不支持的代码不得发出 HTTP、不占配额")
}

func TestCollectorRejectsNonDaily(t *testing.T) {
	c, n := testCollector(t, "[]")
	_, err := c.FetchHistory("AAPL", day("2026-09-01"), day("2026-09-30"), "1h")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval")
	assert.Equal(t, int32(0), atomic.LoadInt32(n))
}

func TestCollectorFetchQuoteFromLastTwoBars(t *testing.T) {
	c, _ := testCollector(t, `[
	  {"date": "2026-09-28T00:00:00.000Z", "open": 99, "high": 101, "low": 98, "close": 100, "volume": 10, "splitFactor": 1},
	  {"date": "2026-09-29T00:00:00.000Z", "open": 100, "high": 106, "low": 99, "close": 105, "volume": 20, "splitFactor": 1}
	]`)
	q, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	assert.Equal(t, "AAPL", q.Symbol)
	assert.Equal(t, core.MarketUS, q.Market)
	assert.Equal(t, 105.0, q.Price)
	assert.Equal(t, 100.0, q.PrevClose)
	assert.Equal(t, 5.0, q.Change)
	assert.InDelta(t, 5.0, q.ChangePercent, 1e-9, "百分数口径，同 yahoo RegularMarketChangePercent")
	assert.Equal(t, 100.0, q.Open)
	assert.Equal(t, 106.0, q.High)
	assert.Equal(t, int64(20), q.Volume)
	assert.Equal(t, day("2026-09-29"), q.Time, "Time 是 K 线日期：让使用方看出不是实时价")
	assert.Equal(t, "tiingo_eod", q.Source)
}

func TestCollectorFetchQuoteSingleBarAndEmpty(t *testing.T) {
	c, _ := testCollector(t, `[{"date": "2026-09-29T00:00:00.000Z", "open": 1, "high": 1, "low": 1, "close": 105, "volume": 1, "splitFactor": 1}]`)
	q, err := c.FetchQuote("AAPL")
	require.NoError(t, err)
	assert.Equal(t, 105.0, q.Price)
	assert.Equal(t, 0.0, q.PrevClose)
	assert.Equal(t, 0.0, q.ChangePercent)

	c, _ = testCollector(t, `[]`)
	_, err = c.FetchQuote("AAPL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recent bars")
}

func TestCollectorMetadataAndInit(t *testing.T) {
	c := NewCollector("k")
	assert.Equal(t, "tiingo", c.Name())
	assert.Equal(t, []core.Market{core.MarketUS}, c.SupportedMarkets())
	assert.Error(t, c.Init(collector.Config{}), "空 key 报错")
	assert.NoError(t, c.Init(collector.Config{APIKey: "k2"}))
	assert.NoError(t, c.Start(context.Background()))
	assert.NoError(t, c.Stop())
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/tiingo/ -run TestCollector -count=1`
Expected: 编译失败 `undefined: Collector` / `NewCollector`。

- [ ] **Step 3: 实现** `collector.go`

```go
package tiingo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// Collector 把 Client 适配为 collector.Collector，供 serve 注册为美股最后一个外部兜底。
type Collector struct {
	client *Client
	now    func() time.Time
}

func NewCollector(apiKey string) *Collector {
	return &Collector{client: New(apiKey), now: time.Now}
}

func (c *Collector) Name() string                    { return "tiingo" }
func (c *Collector) SupportedMarkets() []core.Market { return []core.Market{core.MarketUS} }
func (c *Collector) Start(context.Context) error     { return nil }
func (c *Collector) Stop() error                     { return nil }

func (c *Collector) Init(cfg collector.Config) error {
	if cfg.APIKey == "" {
		return errors.New("tiingo: api_key is required")
	}
	c.client = New(cfg.APIKey)
	return nil
}

// FetchHistory 只支持日线；不在白名单的代码直接拒绝，不发请求、不占配额。
func (c *Collector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if interval != "1d" {
		return nil, fmt.Errorf("tiingo: unsupported interval %q (only 1d)", interval)
	}
	if !Supported(symbol) {
		return nil, fmt.Errorf("tiingo: unsupported symbol %s", symbol)
	}
	return c.client.FetchHistory(symbol, start, end)
}

// FetchQuote 用最近两根日线拼装行情（免费档无实时）。Time 取 K 线日期、Source 标
// tiingo_eod，使用方据此能看出这不是实时价。
func (c *Collector) FetchQuote(symbol string) (*core.Quote, error) {
	if !Supported(symbol) {
		return nil, fmt.Errorf("tiingo: unsupported symbol %s", symbol)
	}
	now := c.now()
	bars, err := c.client.FetchHistory(symbol, now.AddDate(0, 0, -10), now)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("tiingo: %s: no recent bars", symbol)
	}
	last := bars[len(bars)-1]
	q := &core.Quote{
		Symbol: symbol, Market: core.MarketUS,
		Price: last.Close, Open: last.Open, High: last.High, Low: last.Low, Volume: last.Volume,
		Time: last.Time, Source: "tiingo_eod",
	}
	if len(bars) >= 2 {
		prev := bars[len(bars)-2].Close
		q.PrevClose = prev
		q.Change = last.Close - prev
		if prev != 0 {
			q.ChangePercent = q.Change / prev * 100
		}
	}
	return q, nil
}
```

`cmd/atlas/gate_wiring_test.go` 的 `collectorCtors` 映射加入 `"tiingo.New": true,`（它在构造时快照 `policy.Default()`，与 twelvedata 同类）。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/tiingo/ ./cmd/atlas/ -count=1 -race`
Expected: 两个包 `ok`。

- [ ] **Step 5: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/tiingo/collector.go internal/collector/tiingo/collector_test.go cmd/atlas/gate_wiring_test.go
git commit -m "feat(tiingo): collector.Collector 实现（白名单拒绝不发请求，EOD 行情）"
```

---

### Task 6: prism 美股价格链改为有序多跳

**Files:**
- Modify: `internal/prism/refresh.go`（`TwelvedataClient` 定义之后加 `PriceHop`；`Refresh`、`refreshEngine`、`refreshEdgar`、`fetchCloses` 的 `td TwelvedataClient` 参数改为 `hops []PriceHop`）
- Modify: `internal/prism/refresh_test.go`
- Modify: `cmd/atlas/prism.go`、`cmd/atlas/prism_test.go`

**Interfaces:**
- Consumes: Task 4 `tiingo.New`；既有 `twelvedataClientOrNil`（`cmd/atlas/prism.go:161`）、`errFallbackNoData`
- Produces:
  - `type PriceHop struct{ Name string; Client TwelvedataClient }`
  - `func Refresh(cfg config.PrismConfig, store Store, lix …, us USClient, ak …, ed EdgarClient, ts TushareClient, usHops []PriceHop, now time.Time) Report`（仅 `td` 位置的类型改变）
  - `cmd/atlas`：`func usPriceHops(collectors map[string]config.CollectorConfig) []prism.PriceHop`

- [ ] **Step 1: impact 分析**

Run: `npx -y gitnexus@latest impact "fetchCloses" --direction upstream --repo atlas` 与 `npx -y gitnexus@latest impact "Refresh" --direction upstream --repo atlas`
补查：`grep -rnE '\bRefresh\(|fetchCloses\(' --include='*.go' internal/prism cmd/atlas`
Expected: `Refresh` 生产调用方仅 `cmd/atlas/prism.go:202`；测试调用 ~50 处中只有 `refresh_test.go` 的 `TestRefreshUSPriceFallsBackToTwelvedata` 与 `TestRefreshUSPriceTwelvedataEmptyIsNotSuccess` 传非 nil 的 `td`，其余传字面量 `nil`（对 `[]PriceHop` 仍可编译）。若为 HIGH/CRITICAL，在提交说明中记录并告知人类。

- [ ] **Step 2: 写失败测试**（追加到 `internal/prism/refresh_test.go`）

```go
func hopTD(closes []core.OHLCV, err error) *fakeTD {
	td := &fakeTD{closes: map[string][]core.OHLCV{}, fail: map[string]error{}}
	if err != nil {
		td.fail["NVDA"] = err
	} else {
		td.closes["NVDA"] = closes
	}
	return td
}

func TestFetchClosesSecondHopSucceeds(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	closes := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	tg, td := hopTD(nil, errors.New("tiingo 403")), hopTD(closes, nil)

	got, deg, err := fetchCloses(us, []PriceHop{{"tiingo", tg}, {"twelvedata", td}}, "NVDA", now.AddDate(-1, 0, 0), now)
	require.NoError(t, err)
	assert.Equal(t, closes, got)
	assert.Equal(t, "NVDA: yahoo price failed (yahoo 503), twelvedata fallback ok", deg)
}

func TestFetchClosesFirstHopSucceedsSkipsRest(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	closes := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	tg, td := hopTD(closes, nil), hopTD(closes, nil)

	_, deg, err := fetchCloses(us, []PriceHop{{"tiingo", tg}, {"twelvedata", td}}, "NVDA", now.AddDate(-1, 0, 0), now)
	require.NoError(t, err)
	assert.Contains(t, deg, "tiingo fallback ok")
	assert.Empty(t, td.calls, "首跳成功后不得再调后续跳（省配额）")
}

func TestFetchClosesEmptyHopFallsThrough(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	closes := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	tg, td := hopTD(nil, nil), hopTD(closes, nil) // tiingo 零行

	_, deg, err := fetchCloses(us, []PriceHop{{"tiingo", tg}, {"twelvedata", td}}, "NVDA", now.AddDate(-1, 0, 0), now)
	require.NoError(t, err)
	assert.Contains(t, deg, "twelvedata fallback ok", "零行视为失败，继续下一跳")
}

func TestFetchClosesAllHopsFail(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	tg, td := hopTD(nil, errors.New("tiingo 403")), hopTD(nil, errors.New("td 429"))

	_, _, err := fetchCloses(us, []PriceHop{{"tiingo", tg}, {"twelvedata", td}}, "NVDA", now.AddDate(-1, 0, 0), now)
	require.Error(t, err)
	assert.Equal(t, "price history: yahoo 503; tiingo fallback: tiingo 403; twelvedata fallback: td 429", err.Error())
}

func TestFetchClosesNoHopsKeepsPrefix(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	yerr := errors.New("yahoo 503")
	us := &fakeUS2{failPrice: map[string]error{"NVDA": yerr}}

	_, _, err := fetchCloses(us, nil, "NVDA", now.AddDate(-1, 0, 0), now)
	require.Error(t, err)
	assert.Equal(t, "price history: yahoo 503", err.Error())
	assert.ErrorIs(t, err, yerr, "无备用跳时保持 %w 链（与现状一致）")
}

func TestFetchClosesSingleHopMatchesLegacyFormat(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	td := hopTD(nil, errors.New("td 429"))

	_, _, err := fetchCloses(us, []PriceHop{{"twelvedata", td}}, "NVDA", now.AddDate(-1, 0, 0), now)
	require.Error(t, err)
	assert.Equal(t, "price history: yahoo 503; twelvedata fallback: td 429", err.Error(), "与改动前单跳格式逐字一致")
}
```

把两处既有测试的实参由 `td` 改为 `[]PriceHop{{Name: "twelvedata", Client: td}}`：

```go
	rep := Refresh(engineCfg(), store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, []PriceHop{{Name: "twelvedata", Client: td}}, now)
```

（`TestRefreshUSPriceFallsBackToTwelvedata` 与 `TestRefreshUSPriceTwelvedataEmptyIsNotSuccess` 各一处。）

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/prism/ -count=1`
Expected: 编译失败 `undefined: PriceHop`。

- [ ] **Step 4: 实现** —— `internal/prism/refresh.go`

在 `TwelvedataClient` 定义之后加入：

```go
// PriceHop 是美股价格降级链上的一跳（设计 docs/superpowers/specs/2026-09-30-tiingo-source-design.md §3.5）。
// Client 沿用 TwelvedataClient 这个窄接口：tiingo 与 twelvedata 的 FetchHistory 签名相同。
type PriceHop struct {
	Name   string
	Client TwelvedataClient
}
```

把 `Refresh`、`refreshEngine`、`refreshEdgar` 签名中的 `td TwelvedataClient` 改为 `usHops []PriceHop`（`refreshEngine`/`refreshEdgar` 内命名为 `hops`），函数体内对 `td` 的传递全部改为传 `usHops`/`hops`。`fetchCloses` 整体替换为：

```go
// fetchCloses 取美股价格：yahoo 失败时按 hops 顺序逐跳重取同一段（spec §2 美股·价格链；
// 2026-09-30 起为 yahoo→tiingo→twelvedata）。零行视为该跳失败并继续下一跳（兜底跳只在主源
// 已失败时触发，「零行」无法与「符号不被接受」区分）。EPS 链路不受影响，备用跳只补价格。
// hops 为空时错误保持 "price history: %w"；有跳时错误串按跳顺序列出每跳原因。
func fetchCloses(us USClient, hops []PriceHop, symbol string, start, end time.Time) ([]core.OHLCV, string, error) {
	closes, err := us.FetchHistory(symbol, start, end, "1d")
	if err == nil {
		return closes, "", nil
	}
	if len(hops) == 0 {
		return nil, "", fmt.Errorf("price history: %w", err)
	}
	msg := fmt.Sprintf("price history: %v", err)
	for _, h := range hops {
		fb, fbErr := h.Client.FetchHistory(symbol, start, end)
		if fbErr == nil && len(fb) == 0 {
			fbErr = errFallbackNoData
		}
		if fbErr == nil {
			return fb, fmt.Sprintf("%s: yahoo price failed (%v), %s fallback ok", symbol, err, h.Name), nil
		}
		msg += fmt.Sprintf("; %s fallback: %v", h.Name, fbErr)
	}
	return nil, "", errors.New(msg)
}
```

`cmd/atlas/prism.go`：在 `twelvedataClientOrNil` 之后加入

```go
// usPriceHops 按 tiingo → twelvedata 组装 prism 美股价格备用跳。tiingo 需 enabled 且有 key；
// twelvedata 沿用既有判据（有 key 即用）。未配置的跳不加入，避免 typed-nil 接口进链。
func usPriceHops(collectors map[string]config.CollectorConfig) []prism.PriceHop {
	var hops []prism.PriceHop
	if tc := collectors["tiingo"]; tc.Enabled && tc.APIKey != "" {
		hops = append(hops, prism.PriceHop{Name: "tiingo", Client: tiingo.New(tc.APIKey)})
	}
	if td := twelvedataClientOrNil(collectors["twelvedata"].APIKey); td != nil {
		hops = append(hops, prism.PriceHop{Name: "twelvedata", Client: td})
	}
	return hops
}
```

并把 `runPrismRefresh` 中

```go
	td := twelvedataClientOrNil(cfg.Collectors["twelvedata"].APIKey)
```

替换为

```go
	usHops := usPriceHops(cfg.Collectors)
```

`prism.Refresh(pcfg, store, lix, yh, ak, ed, ts, td, time.Now())` 改为 `prism.Refresh(pcfg, store, lix, yh, ak, ed, ts, usHops, time.Now())`；import 加入 `"github.com/newthinker/atlas/internal/collector/tiingo"`。

`cmd/atlas/prism_test.go` 追加：

```go
func TestUSPriceHopsOrderAndSkips(t *testing.T) {
	names := func(hs []prism.PriceHop) []string {
		var out []string
		for _, h := range hs {
			out = append(out, h.Name)
		}
		return out
	}
	assert.Empty(t, usPriceHops(nil))
	assert.Equal(t, []string{"tiingo", "twelvedata"}, names(usPriceHops(map[string]config.CollectorConfig{
		"tiingo": {Enabled: true, APIKey: "k"}, "twelvedata": {APIKey: "k2"},
	})))
	assert.Equal(t, []string{"twelvedata"}, names(usPriceHops(map[string]config.CollectorConfig{
		"tiingo": {Enabled: false, APIKey: "k"}, "twelvedata": {APIKey: "k2"},
	})), "tiingo 未启用不入链")
	assert.Equal(t, []string{"twelvedata"}, names(usPriceHops(map[string]config.CollectorConfig{
		"tiingo": {Enabled: true}, "twelvedata": {APIKey: "k2"},
	})), "tiingo 缺 key 不入链")
	for _, h := range usPriceHops(map[string]config.CollectorConfig{"tiingo": {Enabled: true, APIKey: "k"}}) {
		assert.NotNil(t, h.Client)
	}
}
```

（按需在 `prism_test.go` import `prism` 与 `config` 包。）

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/prism/ ./cmd/atlas/ -count=1 -race`
Expected: 两个包 `ok`。

- [ ] **Step 6: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/prism/refresh.go internal/prism/refresh_test.go cmd/atlas/prism.go cmd/atlas/prism_test.go
git commit -m "feat(tiingo): prism 美股价格链改为有序多跳 yahoo→tiingo→twelvedata"
```

---

### Task 7: serve 装配注册 tiingo + 配置示例

**Files:**
- Modify: `cmd/atlas/collectors.go`（baostock 注册块之后、`wireQlibWarehouse` 之前）
- Modify: `cmd/atlas/collectors_test.go`
- Modify: `configs/config.example.yaml`（`twelvedata` 块之后）

**Interfaces:**
- Consumes: Task 1 有序 `GetAll`；Task 5 `tiingo.NewCollector`；`(*app.App).GetCollectors()`
- Produces: 无新接口

- [ ] **Step 1: 写失败测试**（追加到 `cmd/atlas/collectors_test.go`）

```go
// tiingo 是美股最后一个外部兜底：必须注册在 tushare/baostock 之后（Registry 按注册顺序返回）。
func TestBuildCollectors_RegistersTiingoLast(t *testing.T) {
	cfg := config.Defaults()
	cfg.Collectors = map[string]config.CollectorConfig{
		"yahoo":     {Enabled: true},
		"eastmoney": {Enabled: true},
		"tushare":   {Enabled: true, APIKey: "tok"},
		"tiingo":    {Enabled: true, APIKey: "tk"},
	}
	cfg.Prism.Enabled = true
	application := app.New(cfg, zap.NewNop())

	cleanup, err := buildCollectors(cfg, application, zap.NewNop())
	if err != nil {
		t.Fatalf("buildCollectors: %v", err)
	}
	defer cleanup()

	var got []string
	for _, c := range application.GetCollectors() {
		got = append(got, c.Name())
	}
	want := []string{"yahoo", "eastmoney", "tushare", "baostock", "tiingo"}
	if !slices.Equal(got, want) {
		t.Fatalf("注册顺序 = %v, want %v", got, want)
	}
}

func TestBuildCollectors_SkipsTiingoWhenUnconfigured(t *testing.T) {
	for name, tc := range map[string]config.CollectorConfig{
		"未启用": {Enabled: false, APIKey: "tk"},
		"缺 key": {Enabled: true},
	} {
		cfg := config.Defaults()
		cfg.Collectors = map[string]config.CollectorConfig{"yahoo": {Enabled: true}, "tiingo": tc}
		application := app.New(cfg, zap.NewNop())
		cleanup, err := buildCollectors(cfg, application, zap.NewNop())
		if err != nil {
			t.Fatalf("%s: buildCollectors: %v", name, err)
		}
		cleanup()
		if slices.Contains(collectorNames(application), "tiingo") {
			t.Errorf("%s: 不得登记 tiingo", name)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run 'Tiingo' -count=1`
Expected: `TestBuildCollectors_RegistersTiingoLast` FAIL（缺 tiingo）。

- [ ] **Step 3: 实现** —— `cmd/atlas/collectors.go` 在 baostock 注册块之后加入：

```go
	// 美股价格最后一个外部兜底（设计 docs/superpowers/specs/2026-09-30-tiingo-source-design.md §3.6）：
	// 注册在 tushare/baostock 之后、qlib 之前；Registry 按注册顺序返回，故它排在兜底链末尾。
	// 非美股形态的代码在 collector 内直接拒绝，不发请求、不占配额。
	if collectorCfg, ok := cfg.Collectors["tiingo"]; ok && collectorCfg.Enabled && collectorCfg.APIKey != "" {
		application.RegisterCollector(tiingo.NewCollector(collectorCfg.APIKey))
		log.Info("tiingo collector registered (US price last hop)")
	}
```

import 加入 `"github.com/newthinker/atlas/internal/collector/tiingo"`。

`configs/config.example.yaml` 在 `twelvedata` 块之后加入：

```yaml
  tiingo:
    enabled: false
    api_key: ""        # Tiingo token，只在 runtime configs/config.yaml 填写
    markets: ["US"]
    # 日线一天只更新一次，建议在顶层覆盖缓存 TTL（内置默认随 collector.cache.ttl）：
    # collector:
    #   topics:
    #     tiingo.daily:
    #       ttl: 6h
```

- [ ] **Step 4: 运行确认通过，全量回归**

Run: `go test ./cmd/atlas/ ./internal/... -count=1 && go vet ./cmd/atlas/ ./internal/collector/... ./internal/prism/`
Expected: 全部 `ok`，vet 无输出。

- [ ] **Step 5: 提交**（先 code-simplifier + detect-changes）

```bash
git add cmd/atlas/collectors.go cmd/atlas/collectors_test.go configs/config.example.yaml
git commit -m "feat(tiingo): serve 注册 tiingo 为美股最后一个外部兜底"
```

---

### Task 8: 集成冒烟测试与上线演练

**Files:**
- Create: `internal/collector/tiingo/client_integration_test.go`
- Create: `internal/prism/tiingo_integration_test.go`

**Interfaces:**
- Consumes: Task 4 `New`；Task 6 `fetchCloses`、`PriceHop`；测试内的 `fakeUS2`
- Produces: 无

- [ ] **Step 1: 写集成测试** `internal/collector/tiingo/client_integration_test.go`

```go
//go:build integration

package tiingo

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实调用 Tiingo（约 2 次）。运行：
// ATLAS_TIINGO_TOKEN=... go test -tags integration ./internal/collector/tiingo/ -run Integration -v
func TestTiingoIntegration(t *testing.T) {
	tok := os.Getenv("ATLAS_TIINGO_TOKEN")
	if tok == "" {
		t.Skip("ATLAS_TIINGO_TOKEN 未设置")
	}
	c := New(tok)
	now := time.Now()
	for _, sym := range []string{"AAPL", "BRK.B"} {
		bars, err := c.FetchHistory(sym, now.AddDate(0, 0, -30), now)
		require.NoError(t, err, sym)
		require.NotEmpty(t, bars, sym)
		for _, b := range bars {
			assert.False(t, math.IsNaN(b.Close) || b.Close <= 0, "%s %s close=%v", sym, b.Time, b.Close)
		}
	}
}
```

- [ ] **Step 2: 写 prism 演练** `internal/prism/tiingo_integration_test.go`（yahoo 用 fake 强制失败，tiingo 走真实 API，不碰生产配置与数据库）

```go
//go:build integration

package prism

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/tiingo"
)

func TestFetchClosesTiingoLiveDrill(t *testing.T) {
	tok := os.Getenv("ATLAS_TIINGO_TOKEN")
	if tok == "" {
		t.Skip("ATLAS_TIINGO_TOKEN 未设置")
	}
	now := time.Now()
	us := &fakeUS2{failPrice: map[string]error{"NVDA": errors.New("drill: yahoo down")}}
	closes, deg, err := fetchCloses(us, []PriceHop{{Name: "tiingo", Client: tiingo.New(tok)}}, "NVDA", now.AddDate(0, 0, -30), now)
	require.NoError(t, err)
	assert.NotEmpty(t, closes)
	assert.Contains(t, deg, "tiingo fallback ok")
}
```

- [ ] **Step 3: 运行**（需人类提供 token 到环境变量；本机直连 api.tiingo.com）

Run: `ATLAS_TIINGO_TOKEN=<token> go test -tags integration ./internal/collector/tiingo/ ./internal/prism/ -run 'Integration|LiveDrill' -count=1 -v`
Expected: PASS；不设变量时 SKIP。

- [ ] **Step 4: 提交**（先 code-simplifier + detect-changes）

```bash
git add internal/collector/tiingo/client_integration_test.go internal/prism/tiingo_integration_test.go
git commit -m "test(tiingo): 真实 API 冒烟与 prism 降级演练（integration 标签）"
```

- [ ] **Step 5: 交由人类执行的上线动作**（写生产配置、部署，不在自动执行范围内）

1. runtime `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml` 增加 `collectors.tiingo: {enabled: true, api_key: <token>, markets: ["US"]}`，可选 `collector.topics."tiingo.daily".ttl: 6h`。
2. 部署二进制（`bash scripts/ops/deploy.sh`，部署前按其注释先 `rsync -n -i` 预演）。
3. 重启 serve：`launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.serve`，日志应出现 `tiingo collector registered (US price last hop)`。
4. 次日 prism 报告：Yahoo 正常时不应出现 tiingo 字样；出现 `tiingo fallback ok` 即降级生效。

---

## Self-Review 记录

- **Spec 覆盖**：§3.1 client → Task 4；§3.2 白名单 → Task 3；§3.3 collector → Task 5；§3.4 policy → Task 2；§3.5 prism → Task 6；§3.6 Registry/serve → Task 1、7；§3.7 配置 → Task 7；§4 错误表 → Task 4（状态码、坏行、splitFactor、脱敏）与 Task 5（白名单）；§5 测试 → 各任务；§6 上线 → Task 8。
- **与 spec 的有意差异**：
  1. §5「直连」测试改为结构断言（`Transport.Proxy == nil`）：spec 写的「设代理变量后请求仍到达 httptest」对 loopback 恒通过，测不出任何东西。
  2. §3.6「订正 tushare 2nd hop / baostock 3rd hop 日志」**不做**：有序化后 A 股标的的兜底顺序仍是 yahoo（最先注册）先于 tushare，该说法依旧不字面成立；改日志不在本需求范围，留作观察项。
  3. `normalize` 对 O/H/L 任一缺失的行也不输出（spec 只写「单行字段不可解析 → 跳过」），避免 0 价格进入策略计算；但**该行的 `splitFactor` 仍参与累乘**——spec 未写到这个边界，拆股当日缺价时连因子一起丢会让此前全部价格错一个倍数。
- **自查中修掉的计划缺陷**：`serveFixture` 初稿在 handler goroutine 里无锁写「最后一次请求」、测试主体再读，经网络的同步对 race detector 不可见，`-race` 会误报 ⇒ 改为 `reqLog` 加锁；`Start(nil)` 改为 `context.Background()`。
- **类型一致性**：`topicDaily`、`Client`、`New`、`NewWithBaseURL`、`Supported`、`toTicker`、`Collector`、`NewCollector`、`PriceHop{Name, Client}`、`usPriceHops` 在 Task 2–8 中一致；`PriceHop.Client` 类型为 `prism.TwelvedataClient`，`*tiingo.Client` 满足。
