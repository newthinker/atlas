# Prism AKShare 数据源 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 经 aktools HTTP 侧车接入 AKShare,作为 A/H 公司估值时序主源与 A 股指数的自动降级兜底,茅台/腾讯恢复上墙。

**Architecture:** 新建 `internal/collector/akshare` HTTP client(调本地 aktools);`internal/prism` 编排层新增 `refreshAkshare` 路径与自动降级链(`Report.Degraded` 可观测);分位无官方值,读回 store 序列后用既有 `valuation.RollingPercentile` 本地计算(5Y+10Y)。store/API/Web 零改动。

**Tech Stack:** Go 1.24.4 / net/http / testify + httptest;部署侧 Python 3.11 独立 venv(`scripts/akshare/.venv`)+ aktools + launchd。

**Spec:** `docs/superpowers/specs/2026-07-24-prism-akshare-source-design.md`(决策记录与口径取舍以 spec 为准)。

## Global Constraints

- module `github.com/newthinker/atlas`,go 1.24.4;**勿升级 `modernc.org/sqlite`(固定 v1.38.2)**;构建/测试遇 toolchain 问题加 `GOTOOLCHAIN=local`。
- 测试风格: testify(`assert`/`require`)+ httptest;测试文件顶部 Context Checkpoint 注释映射验收条目(仿 `internal/prism/refresh_test.go` 现状)。
- 数值缺失约定: 内存 `math.NaN()`,store 写 NULL 读回 NaN(不变)。
- **⚠ AKShare 接口名/字段名为实现期 live 校验点**(该库随上游变动是常态): 本计划中 `stock_a_indicator_lg`/`stock_hk_valuation_baidu`/`stock_index_pe_lg` 的接口名、字段键(`trade_date`/`pe_ttm`/`日期`/`滚动市盈率` 等)按当前 akshare 文档编写,首次真实运行若不符,以 aktools 实际响应修正常量并同步测试,commit message 注明(仿 M1 Task 3 的 mcw 处理惯例)。
- `Refresh` 函数签名本计划会加一个参数(AkshareClient)——属破坏性变更,Task 4 一并修全部既有调用点与测试,`go build ./...` 必须始终可过。
- 每 Task 单独 commit,格式 `feat(prism): ...`;提交前跑 gitnexus detect_changes(索引 stale 时注明由人工代跑)。
- 中文注释风格与现有代码一致;不动与本计划无关的代码。
- 覆盖率门禁 changed-package ≥80;cmd/atlas 若触发 AD-6 整包口径按既有处置模板(见 wisdom)。

## File Structure

```
internal/config/config.go                     修改: PrismInstrument+FallbackSource;PrismConfig+AkshareBaseURL
internal/config/prism_test.go                 修改: 补默认值/字段用例
internal/collector/akshare/client.go          新建: Client/New/get/映射/解析 helpers
internal/collector/akshare/stock.go           新建: FetchStockValuationSeries(A股+HK)
internal/collector/akshare/index.go           新建: FetchIndexValuationSeries
internal/collector/akshare/client_test.go     新建
internal/collector/akshare/stock_test.go      新建
internal/collector/akshare/index_test.go      新建
internal/prism/refresh.go                     修改: Store+Series、AkshareClient、refreshAkshare、降级链、Report.Degraded
internal/prism/refresh_test.go                修改: fakeStore+Series、fakeAkshare、新用例
cmd/atlas/prism.go                            修改: akshare client 构造注入 + Degraded 告警文本
cmd/atlas/prism_test.go                       修改: Degraded 告警用例
scripts/akshare/setup.sh                      新建: 幂等建独立 venv + 安装 + freeze 锁定
scripts/akshare/requirements.txt              新建
deploy/launchd/com.newthinker.atlas.aktools.plist  新建
configs/config.example.yaml                   修改: akshare_base_url + 池 source/fallback_source 变更
docs/deployment.md                            修改: 追加 aktools 部署段(只追加不重排)
```

---

### Task 1: 配置扩展(FallbackSource + AkshareBaseURL)

**Files:**
- Modify: `internal/config/config.go`(PrismConfig/PrismInstrument/ApplyDefaults,文件尾部 M1 新增区)
- Test: `internal/config/prism_test.go`(追加用例)

**Interfaces:**
- Consumes: 既有 `PrismConfig`/`PrismInstrument`/`ApplyDefaults`。
- Produces(Task 4/5/6/7 消费): `PrismInstrument.FallbackSource string`(mapstructure `fallback_source`);`PrismConfig.AkshareBaseURL string`(mapstructure `akshare_base_url`,默认 `http://127.0.0.1:8180`)。

- [ ] **Step 1: 追加失败测试**

在 `internal/config/prism_test.go` 追加:

```go
// Context Checkpoint: akshare 配置默认值与字段 → TestPrismConfigAkshareDefaults
func TestPrismConfigAkshareDefaults(t *testing.T) {
	c := PrismConfig{}
	c.ApplyDefaults()
	assert.Equal(t, "http://127.0.0.1:8180", c.AkshareBaseURL)

	c2 := PrismConfig{AkshareBaseURL: "http://127.0.0.1:9999"}
	c2.ApplyDefaults()
	assert.Equal(t, "http://127.0.0.1:9999", c2.AkshareBaseURL, "显式值不被覆盖")
}

func TestPrismInstrumentFallbackSourceTag(t *testing.T) {
	f, ok := reflect.TypeOf(PrismInstrument{}).FieldByName("FallbackSource")
	require.True(t, ok)
	assert.Equal(t, "fallback_source", f.Tag.Get("mapstructure"))
}
```

(顶部 import 补 `reflect`、`require`,以文件现状为准。)

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/config/ -run 'TestPrismConfigAkshare|TestPrismInstrumentFallback' -v`
Expected: FAIL(undefined field)

- [ ] **Step 3: 最小实现**

`internal/config/config.go`:

```go
// PrismInstrument 追加字段(其余字段不动):
	FallbackSource string `mapstructure:"fallback_source"` // 主源失败时的兜底源,如 "akshare";空=无兜底

// PrismConfig 追加字段:
	AkshareBaseURL string `mapstructure:"akshare_base_url"` // aktools 本地侧车地址

// ApplyDefaults 追加:
	if c.AkshareBaseURL == "" {
		c.AkshareBaseURL = "http://127.0.0.1:8180"
	}
```

- [ ] **Step 4: 跑全包测试确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/config/ -count=1 -v`
Expected: 新旧用例全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/prism_test.go
git commit -m "feat(prism): akshare base URL and per-instrument fallback_source config"
```

---

### Task 2: akshare collector 基座 + A 股个股时序

**Files:**
- Create: `internal/collector/akshare/client.go`、`internal/collector/akshare/stock.go`
- Test: `internal/collector/akshare/client_test.go`、`internal/collector/akshare/stock_test.go`

**Interfaces:**
- Consumes: 无(纯新包)。
- Produces(Task 3 同包扩展、Task 4 消费,签名必须一致):

```go
package akshare // import "github.com/newthinker/atlas/internal/collector/akshare"

type ValuationPoint struct {
	Date              time.Time
	PETTM, PB, PSTTM  float64 // 缺失 = NaN
}
func New(baseURL string) *Client
func (c *Client) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]ValuationPoint, error)
```

**领域说明**: aktools 把 AKShare 每个函数暴露为 `GET {base}/api/public/{接口名}?{参数}`,返回 JSON 数组(每行一个对象)。A 股个股用乐咕 `stock_a_indicator_lg`(参数 `symbol=600519`,返回**全历史**,字段 `trade_date`/`pe`/`pe_ttm`/`pb`/`ps`/`ps_ttm`/...)——客户端按 [start,end] 过滤。⚠ 字段名 live 校验点。

- [ ] **Step 1: 写失败测试**

```go
// internal/collector/akshare/stock_test.go
package akshare

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint:
//   A股走 lg 接口+符号映射+窗口过滤 → TestFetchAStockSeries
//   缺字段→NaN、乱序→升序           → TestFetchAStockMissingAndOrder
//   不支持的 market → error          → TestFetchStockUnsupportedMarket

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func lgServer(t *testing.T, wantSymbol string, rows []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_a_indicator_lg", r.URL.Path)
		assert.Equal(t, wantSymbol, r.URL.Query().Get("symbol"))
		json.NewEncoder(w).Encode(rows)
	}))
}

func TestFetchAStockSeries(t *testing.T) {
	srv := lgServer(t, "600519", []map[string]any{
		{"trade_date": "2015-01-05", "pe_ttm": 10.0, "pb": 3.0, "ps_ttm": 5.0}, // 窗口外,应被过滤
		{"trade_date": "2026-07-22", "pe_ttm": 22.5, "pb": 8.1, "ps_ttm": 10.2},
		{"trade_date": "2026-07-23", "pe_ttm": 22.7, "pb": 8.2, "ps_ttm": 10.3},
	})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2, "2015 年的行应被窗口过滤")
	assert.Equal(t, 22.5, pts[0].PETTM)
	assert.Equal(t, 8.2, pts[1].PB)
	assert.Equal(t, "2026-07-23", pts[1].Date.Format("2006-01-02"))
}

func TestFetchAStockMissingAndOrder(t *testing.T) {
	srv := lgServer(t, "600519", []map[string]any{
		{"trade_date": "2026-07-23", "pe_ttm": 22.7}, // 乱序 + 缺 pb/ps
		{"trade_date": "2026-07-22", "pe_ttm": 22.5, "pb": 8.1},
	})
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("600519.SH", "CN_A", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date), "输出必须升序")
	assert.True(t, math.IsNaN(pts[1].PB), "缺字段→NaN")
	assert.True(t, math.IsNaN(pts[0].PSTTM))
}

func TestFetchStockUnsupportedMarket(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.FetchStockValuationSeries("XXX", "US", day(2026, 1, 1), day(2026, 7, 23))
	assert.ErrorContains(t, err, "unsupported market")
}
```

```go
// internal/collector/akshare/client_test.go
package akshare

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Context Checkpoint: 非 200 响应→含状态码与片段的 error → TestGetHTTPError
func TestGetHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("akshare internal boom"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.FetchStockValuationSeries("600519.SH", "CN_A",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.ErrorContains(t, err, "500")
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/collector/akshare/ -v`
Expected: FAIL(package 不存在 / undefined: New)

- [ ] **Step 3: 实现**

```go
// internal/collector/akshare/client.go
// Package akshare 经本地 aktools HTTP 侧车调用 AKShare 数据接口
// (设计: docs/superpowers/specs/2026-07-24-prism-akshare-source-design.md)。
// ⚠ 接口名与字段键为 live 校验点: AKShare 随上游变动是常态,首次真实运行若不符,
// 以 aktools 实际响应修正本包常量并同步测试。
package akshare

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ValuationPoint 是一天的估值指标,缺失字段为 NaN(store 写入时转 NULL)。
// akshare 无官方分位,分位由 prism 编排层本地计算。
type ValuationPoint struct {
	Date             time.Time
	PETTM, PB, PSTTM float64
}

// Client 是 aktools 本地侧车的 HTTP 客户端。
type Client struct {
	baseURL string
	hc      *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 60 * time.Second}, // lg 全历史响应较大
	}
}

// get 调用 aktools 的 /api/public/{api} 并解析 JSON 数组响应。
func (c *Client) get(api string, params url.Values) ([]map[string]any, error) {
	u := c.baseURL + "/api/public/" + api
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.hc.Get(u)
	if err != nil {
		return nil, fmt.Errorf("akshare: %s: %w", api, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("akshare: %s: HTTP %d: %s", api, resp.StatusCode, string(body))
	}
	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("akshare: %s: decode: %w", api, err)
	}
	return rows, nil
}

// fnum 取 row 中第一个存在且为数值的键,均缺失返回 NaN。
func fnum(row map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := row[k].(float64); ok {
			return v
		}
	}
	return math.NaN()
}

// fdate 取 row 中第一个存在的日期键,支持 "2006-01-02" 与 ISO8601 前缀。
func fdate(row map[string]any, keys ...string) (time.Time, error) {
	for _, k := range keys {
		s, ok := row[k].(string)
		if !ok {
			continue
		}
		if len(s) >= 10 {
			if d, err := time.Parse("2006-01-02", s[:10]); err == nil {
				return d, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("akshare: no parsable date in row (keys tried: %v)", keys)
}

// clipSort 按 [start,end] 闭区间过滤并升序排序。
func clipSort(pts []ValuationPoint, start, end time.Time) []ValuationPoint {
	out := pts[:0]
	s, e := start.Format("2006-01-02"), end.Format("2006-01-02")
	for _, p := range pts {
		d := p.Date.Format("2006-01-02")
		if d >= s && d <= e {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
```

```go
// internal/collector/akshare/stock.go
package akshare

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// mapAStockSymbol: "600519.SH"/"000001.SZ" → "600519"(乐咕接口用裸六位代码)。
func mapAStockSymbol(symbol string) string {
	if i := strings.IndexByte(symbol, '.'); i > 0 {
		return symbol[:i]
	}
	return symbol
}

// FetchStockValuationSeries 返回个股 [start,end] 日频估值时序,按 market 分派数据源。
func (c *Client) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]ValuationPoint, error) {
	switch market {
	case "CN_A":
		return c.fetchAStock(symbol, start, end)
	case "HK":
		return c.fetchHKStock(symbol, start, end) // Task 3 实现
	default:
		return nil, fmt.Errorf("akshare: unsupported market %q for %s", market, symbol)
	}
}

// fetchAStock: 乐咕 stock_a_indicator_lg 返回全历史,客户端过滤窗口。
// ⚠ 字段键 live 校验点: trade_date/pe_ttm/pb/ps_ttm(ps 兜底)。
func (c *Client) fetchAStock(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	rows, err := c.get("stock_a_indicator_lg", url.Values{"symbol": {mapAStockSymbol(symbol)}})
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(rows))
	for _, row := range rows {
		d, err := fdate(row, "trade_date", "date")
		if err != nil {
			return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
		}
		pts = append(pts, ValuationPoint{
			Date:  d,
			PETTM: fnum(row, "pe_ttm"),
			PB:    fnum(row, "pb"),
			PSTTM: fnum(row, "ps_ttm", "ps"),
		})
	}
	return clipSort(pts, start, end), nil
}
```

(Task 2 阶段 `fetchHKStock` 先放 stub: `return nil, fmt.Errorf("akshare: hk not implemented yet")`,Task 3 替换——保证本 Task 可编译且 US market 用例可过。)

- [ ] **Step 4: 跑测试确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/collector/akshare/ -count=1 -v`
Expected: 4 用例全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/akshare/
git commit -m "feat(prism): akshare collector with A-share valuation series via aktools"
```

---

### Task 3: 港股个股(百度双指标合并)+ A 股指数时序

**Files:**
- Modify: `internal/collector/akshare/stock.go`(实现 fetchHKStock)
- Create: `internal/collector/akshare/index.go`
- Test: `internal/collector/akshare/stock_test.go`(追加)、`internal/collector/akshare/index_test.go`

**Interfaces:**
- Consumes: Task 2 的 `get`/`fnum`/`fdate`/`clipSort`。
- Produces(Task 4 消费): `func (c *Client) FetchIndexValuationSeries(symbol string, start, end time.Time) ([]ValuationPoint, error)`;`fetchHKStock` 完成(经 `FetchStockValuationSeries` market="HK" 暴露)。

**领域说明**: 百度股市通 `stock_hk_valuation_baidu`(参数 `symbol=00700`、`indicator=市盈率(TTM)`|`市净率`、`period=全部`)**每次只返回一个指标**(字段 `date`/`value`),需两次调用按日期合并;PSTTM 恒 NaN。指数用乐咕 `stock_index_pe_lg`(参数 `symbol=沪深300` 中文名),取**滚动市盈率**(加权口径,近似理杏仁 mcw,差异已在 spec 注明),字段键为中文(`日期`/`滚动市盈率`)。⚠ 均为 live 校验点。

- [ ] **Step 1: 写失败测试**

`stock_test.go` 追加:

```go
// Context Checkpoint: HK 双指标按日期合并、单边缺失→NaN → TestFetchHKStockMerge
func TestFetchHKStockMerge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_hk_valuation_baidu", r.URL.Path)
		assert.Equal(t, "00700", r.URL.Query().Get("symbol"))
		assert.Equal(t, "全部", r.URL.Query().Get("period"))
		switch r.URL.Query().Get("indicator") {
		case "市盈率(TTM)":
			json.NewEncoder(w).Encode([]map[string]any{
				{"date": "2026-07-22", "value": 22.1},
				{"date": "2026-07-23", "value": 22.4},
			})
		case "市净率":
			json.NewEncoder(w).Encode([]map[string]any{
				{"date": "2026-07-23", "value": 3.1}, // 07-22 无 PB → NaN
			})
		default:
			t.Errorf("unexpected indicator %q", r.URL.Query().Get("indicator"))
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchStockValuationSeries("0700.HK", "HK", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.Equal(t, 22.1, pts[0].PETTM)
	assert.True(t, math.IsNaN(pts[0].PB), "该日仅有 PE → PB NaN")
	assert.Equal(t, 3.1, pts[1].PB)
	assert.True(t, math.IsNaN(pts[1].PSTTM), "HK 无 PS,恒 NaN")
}
```

```go
// internal/collector/akshare/index_test.go
package akshare

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint:
//   指数走 lg 中文名映射+滚动市盈率 → TestFetchIndexSeries
//   未登记指数 → error              → TestFetchIndexUnknownSymbol

func TestFetchIndexSeries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/public/stock_index_pe_lg", r.URL.Path)
		assert.Equal(t, "沪深300", r.URL.Query().Get("symbol"))
		json.NewEncoder(w).Encode([]map[string]any{
			{"日期": "2026-07-23", "滚动市盈率": 14.6},
			{"日期": "2026-07-22", "滚动市盈率": 14.5}, // 乱序
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	pts, err := c.FetchIndexValuationSeries("000300.SH", day(2026, 7, 1), day(2026, 7, 23))
	require.NoError(t, err)
	require.Len(t, pts, 2)
	assert.True(t, pts[0].Date.Before(pts[1].Date))
	assert.Equal(t, 14.6, pts[1].PETTM)
	assert.True(t, math.IsNaN(pts[0].PB), "指数兜底仅 PE,PB/PS NaN")
}

func TestFetchIndexUnknownSymbol(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.FetchIndexValuationSeries("^GSPC", day(2026, 1, 1), day(2026, 7, 23))
	assert.ErrorContains(t, err, "no lg index mapping")
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/collector/akshare/ -run 'TestFetchHK|TestFetchIndex' -v`
Expected: FAIL(hk stub error / undefined: FetchIndexValuationSeries)

- [ ] **Step 3: 实现**

替换 `stock.go` 的 stub:

```go
// mapHKSymbol: "0700.HK" → "00700"(百度接口用五位代码)。
func mapHKSymbol(symbol string) string {
	code := symbol
	if i := strings.IndexByte(symbol, '.'); i > 0 {
		code = symbol[:i]
	}
	for len(code) < 5 {
		code = "0" + code
	}
	return code
}

// fetchHKStock: 百度股市通每次一个指标,两次调用按日期合并。PSTTM 恒 NaN。
// ⚠ live 校验点: 接口名/indicator 取值/date+value 字段键。
func (c *Client) fetchHKStock(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	code := mapHKSymbol(symbol)
	fetch := func(indicator string) (map[string]float64, error) {
		rows, err := c.get("stock_hk_valuation_baidu",
			url.Values{"symbol": {code}, "indicator": {indicator}, "period": {"全部"}})
		if err != nil {
			return nil, err
		}
		m := make(map[string]float64, len(rows))
		for _, row := range rows {
			d, err := fdate(row, "date")
			if err != nil {
				return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
			}
			m[d.Format("2006-01-02")] = fnum(row, "value")
		}
		return m, nil
	}
	pe, err := fetch("市盈率(TTM)")
	if err != nil {
		return nil, err
	}
	pb, err := fetch("市净率")
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(pe))
	for ds, v := range pe {
		d, _ := time.Parse("2006-01-02", ds)
		p := ValuationPoint{Date: d, PETTM: v, PB: math.NaN(), PSTTM: math.NaN()}
		if b, ok := pb[ds]; ok {
			p.PB = b
		}
		pts = append(pts, p)
	}
	return clipSort(pts, start, end), nil
}
```

(`stock.go` import 补 `math`。)

```go
// internal/collector/akshare/index.go
package akshare

import (
	"fmt"
	"math"
	"net/url"
	"time"
)

// lgIndexName 是 Prism symbol → 乐咕指数中文名映射(兜底范围内按需登记)。
// ⚠ live 校验点: 名称须与 stock_index_pe_lg 的 symbol 取值一致。
var lgIndexName = map[string]string{
	"000300.SH": "沪深300",
	"000905.SH": "中证500",
}

// FetchIndexValuationSeries 返回 A 股指数 [start,end] 日频 PE(滚动市盈率,加权口径,
// 近似理杏仁 mcw——差异见 spec §3)。PB/PSTTM 恒 NaN。
func (c *Client) FetchIndexValuationSeries(symbol string, start, end time.Time) ([]ValuationPoint, error) {
	name, ok := lgIndexName[symbol]
	if !ok {
		return nil, fmt.Errorf("akshare: no lg index mapping for %q", symbol)
	}
	rows, err := c.get("stock_index_pe_lg", url.Values{"symbol": {name}})
	if err != nil {
		return nil, err
	}
	pts := make([]ValuationPoint, 0, len(rows))
	for _, row := range rows {
		d, err := fdate(row, "日期", "date")
		if err != nil {
			return nil, fmt.Errorf("%w (symbol %s)", err, symbol)
		}
		pts = append(pts, ValuationPoint{
			Date:  d,
			PETTM: fnum(row, "滚动市盈率"),
			PB:    math.NaN(),
			PSTTM: math.NaN(),
		})
	}
	return clipSort(pts, start, end), nil
}
```

- [ ] **Step 4: 跑全包测试确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/collector/akshare/ -count=1 -v`
Expected: 全部用例 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/akshare/
git commit -m "feat(prism): akshare HK stock (baidu merge) and A-share index series"
```

---

### Task 4: refreshAkshare 路径(增量 + 本地分位)

**Files:**
- Modify: `internal/prism/refresh.go`
- Test: `internal/prism/refresh_test.go`(追加)

**Interfaces:**
- Consumes: Task 2/3 `akshare.ValuationPoint` 与两个 Fetch 方法;既有 `valuation.RollingPercentile(dates, values, years, minPoints)`、`rollingMinPoints=252`、`prismstore.SeriesData{Dates []string; PETTM []float64; ...}`。
- Produces(Task 5/6 消费):

```go
// Store 接口追加(读回历史算分位):
	Series(symbol, from string) (*prismstore.SeriesData, error)
type AkshareClient interface {
	FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error)
	FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error)
}
func refreshAkshare(cfg config.PrismConfig, store Store, ak AkshareClient, inst config.PrismInstrument, now time.Time) error
// Refresh 签名变更: Refresh(cfg, store, lix, us, ak, now) —— case "akshare" 分派接入
```

**分位算法**(spec §3): akshare 无官方分位。取 store 已存全历史(`Series(symbol, "")`)的 (D, PETTM),剔除 PE 为 NaN 的行,与新拉取点合并(同日新值覆盖旧值)、升序去重后,对合并序列跑 `RollingPercentile(…, 5, 252)` 与 `(…, 10, 252)`,**只为新拉取的日期**写行(历史行不回写)。

- [ ] **Step 1: 写失败测试**

`refresh_test.go` 追加(既有 `fakeStore` 需补 `Series` 方法——同文件顶部 fakeStore 定义处加字段与方法,所有既有用例无需改动):

```go
// fakeStore 追加字段:
	series map[string]*prismstore.SeriesData // symbol → 预置历史(Series 返回)
// newFakeStore() 初始化追加: series: map[string]*prismstore.SeriesData{}
// fakeStore 追加方法:
func (f *fakeStore) Series(symbol, from string) (*prismstore.SeriesData, error) {
	if sd, ok := f.series[symbol]; ok {
		return sd, nil
	}
	return &prismstore.SeriesData{Symbol: symbol}, nil
}

type fakeAkshare struct {
	stockCalls map[string][2]time.Time // symbol → [start,end]
	indexCalls map[string][2]time.Time
	stockPts   []akshare.ValuationPoint
	indexPts   []akshare.ValuationPoint
	fail       map[string]error
}

func (f *fakeAkshare) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error) {
	if f.stockCalls == nil {
		f.stockCalls = map[string][2]time.Time{}
	}
	f.stockCalls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return f.stockPts, nil
}
func (f *fakeAkshare) FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error) {
	if f.indexCalls == nil {
		f.indexCalls = map[string][2]time.Time{}
	}
	f.indexCalls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return f.indexPts, nil
}

// Context Checkpoint:
//   akshare 公司路径增量+落库           → TestRefreshAkshareStockIncremental
//   本地分位: 历史读回+新点合并计算     → TestRefreshAkshareLocalPercentile
//   指数标的走 Index 方法               → TestRefreshAkshareIndexDispatch
//   空拉取→零写入仍成功                 → TestRefreshAkshareEmptyFetch

func akCfg(inst config.PrismInstrument) config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{inst}}
	c.ApplyDefaults()
	return c
}

func TestRefreshAkshareStockIncremental(t *testing.T) {
	store, ak := newFakeStore(), &fakeAkshare{stockPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 22.7, PB: 8.2, PSTTM: math.NaN()},
	}}
	store.latest["600519.SH"] = "2026-07-22"
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Name: "贵州茅台", Type: "stock", Market: "CN_A", Group: "A股公司", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	win := ak.stockCalls["600519.SH"]
	assert.Equal(t, "2026-07-23", win[0].Format("2006-01-02"), "增量从 latest+1")
	require.Len(t, store.upserts["600519.SH"], 1)
	assert.Equal(t, 22.7, store.upserts["600519.SH"][0].PETTM)
	assert.Equal(t, "2026-07-23", store.upserts["600519.SH"][0].D)
}

func TestRefreshAkshareLocalPercentile(t *testing.T) {
	// 预置 300 天历史(PE 全为 10),新点 PE=20 → 新点在窗口内为最大值,pctl_5y=100;
	// 样本 300>=252 满足 minPoints。
	hist := &prismstore.SeriesData{Symbol: "600519.SH"}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		hist.Dates = append(hist.Dates, base.AddDate(0, 0, i).Format("2006-01-02"))
		hist.PETTM = append(hist.PETTM, 10.0)
	}
	store := newFakeStore()
	store.series["600519.SH"] = hist
	store.latest["600519.SH"] = hist.Dates[len(hist.Dates)-1]

	newDay := base.AddDate(0, 0, 300)
	ak := &fakeAkshare{stockPts: []akshare.ValuationPoint{{Date: newDay, PETTM: 20.0, PB: math.NaN(), PSTTM: math.NaN()}}}
	now := newDay.AddDate(0, 0, 1)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, now)
	require.Empty(t, rep.Failed)
	rows := store.upserts["600519.SH"]
	require.Len(t, rows, 1, "只写新点,历史行不回写")
	assert.InDelta(t, 100.0, rows[0].Pctl5Y, 1e-9, "新点为窗口最大值→100 分位")
	assert.InDelta(t, 100.0, rows[0].Pctl10Y, 1e-9)
}

func TestRefreshAkshareIndexDispatch(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{indexPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 14.6, PB: math.NaN(), PSTTM: math.NaN()},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, now)
	assert.Empty(t, rep.Failed)
	_, usedIndex := ak.indexCalls["000300.SH"]
	assert.True(t, usedIndex, "index 类型必须走 FetchIndexValuationSeries")
	assert.Empty(t, ak.stockCalls)
}

func TestRefreshAkshareEmptyFetch(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{} // 返回空切片
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	assert.Empty(t, store.upserts["600519.SH"], "空拉取零写入")
}
```

(import 补 `akshare "github.com/newthinker/atlas/internal/collector/akshare"`;既有全部 `Refresh(...)` 调用点在本 Task 一并加 `ak` 实参——传 `&fakeAkshare{}` 或 `nil` 均可,选 `&fakeAkshare{}` 防 nil 解引用。)

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/prism/ -v`
Expected: FAIL(Refresh 参数不符 / undefined: AkshareClient)

- [ ] **Step 3: 实现**

`refresh.go` 变更:

```go
// import 追加:
	"sort"
	"github.com/newthinker/atlas/internal/collector/akshare"

// Store 接口追加一行:
	Series(symbol, from string) (*prismstore.SeriesData, error)

// 新增接口:
// AkshareClient is the subset of *akshare.Client used by Refresh.
type AkshareClient interface {
	FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error)
	FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error)
}

// Refresh 签名与分派变更(us 后插入 ak;lixinger 降级链 Task 5 再加):
func Refresh(cfg config.PrismConfig, store Store, lix LixingerClient, us USClient, ak AkshareClient, now time.Time) Report {
	// switch 追加:
	case "akshare":
		err = refreshAkshare(cfg, store, ak, inst, now)
}

// incrementalStart 抽取 lixinger 路径既有的「增量起点+日历日零请求守卫」逻辑供两路复用:
// 返回 (start, skip, err);skip=true 表示已是最新零请求。
func incrementalStart(store Store, id int64, lookbackYears int, now time.Time) (time.Time, bool, error) {
	latest, err := store.LatestDate(id)
	if err != nil {
		return time.Time{}, false, err
	}
	start := now.AddDate(-lookbackYears, 0, 0)
	if latest != "" {
		d, perr := time.Parse("2006-01-02", latest)
		if perr != nil {
			return time.Time{}, false, fmt.Errorf("bad latest date %q: %w", latest, perr)
		}
		start = d.AddDate(0, 0, 1)
		if start.Format("2006-01-02") >= now.Format("2006-01-02") {
			return time.Time{}, true, nil
		}
	}
	return start, false, nil
}
// (refreshLixinger 内联的同款逻辑改为调用 incrementalStart——行为不变,既有测试是安全网。)

// refreshAkshare: akshare 路径——增量拉取 + 本地滚动分位(akshare 无官方分位)。
func refreshAkshare(cfg config.PrismConfig, store Store, ak AkshareClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	var pts []akshare.ValuationPoint
	if inst.Type == "index" {
		pts, err = ak.FetchIndexValuationSeries(inst.Symbol, start, now)
	} else {
		pts, err = ak.FetchStockValuationSeries(inst.Symbol, inst.Market, start, now)
	}
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return nil // 无新数据,零写入
	}

	// 本地分位: 历史(剔除 NaN PE)+ 新点合并(同日新值覆盖),升序后整段算 5Y/10Y 分位。
	hist, err := store.Series(inst.Symbol, "")
	if err != nil {
		return fmt.Errorf("read back series: %w", err)
	}
	peByDay := make(map[string]float64, len(hist.Dates)+len(pts))
	for i, ds := range hist.Dates {
		if !math.IsNaN(hist.PETTM[i]) {
			peByDay[ds] = hist.PETTM[i]
		}
	}
	newDays := make(map[string]akshare.ValuationPoint, len(pts))
	for _, p := range pts {
		ds := p.Date.Format("2006-01-02")
		newDays[ds] = p
		if !math.IsNaN(p.PETTM) {
			peByDay[ds] = p.PETTM
		}
	}
	days := make([]string, 0, len(peByDay))
	for ds := range peByDay {
		days = append(days, ds)
	}
	sort.Strings(days)
	dates := make([]time.Time, len(days))
	pe := make([]float64, len(days))
	idx := make(map[string]int, len(days))
	for i, ds := range days {
		d, perr := time.Parse("2006-01-02", ds)
		if perr != nil {
			return fmt.Errorf("bad stored date %q: %w", ds, perr)
		}
		dates[i], pe[i], idx[ds] = d, peByDay[ds], i
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)
	p10 := valuation.RollingPercentile(dates, pe, 10, rollingMinPoints)

	rows := make([]prismstore.ValuationRow, 0, len(newDays))
	for ds, p := range newDays {
		row := prismstore.ValuationRow{
			D: ds, PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM,
			Pctl5Y: math.NaN(), Pctl10Y: math.NaN(),
		}
		if i, ok := idx[ds]; ok { // PE 为 NaN 的新点不参与分位序列,保持 NaN
			row.Pctl5Y, row.Pctl10Y = p5[i], p10[i]
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].D < rows[j].D })
	return store.UpsertValuations(id, rows)
}
```

同步修改其余调用点: `cmd/atlas/prism.go` 的 `prism.Refresh(pcfg, store, lix, yh, time.Now())` 加实参(本 Task 先传 `akshare.New(pcfg.AkshareBaseURL)`,Task 6 完善 Degraded 告警);`refresh_test.go` 全部既有 `Refresh(` 调用点加 `&fakeAkshare{}`。

- [ ] **Step 4: 跑测试 + 全仓编译**

Run: `GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -v && GOTOOLCHAIN=local go build ./...`
Expected: 新增 4 用例 + 既有全部用例 PASS(incrementalStart 提取以既有理杏仁用例为安全网);全仓编译过

- [ ] **Step 5: Commit**

```bash
git add internal/prism/ cmd/atlas/prism.go
git commit -m "feat(prism): akshare refresh path with incremental fetch and local rolling percentiles"
```

---

### Task 5: 指数自动降级链 + Report.Degraded

**Files:**
- Modify: `internal/prism/refresh.go`(Report + Refresh 分派)
- Test: `internal/prism/refresh_test.go`(追加)

**Interfaces:**
- Consumes: Task 4 `refreshAkshare`、Task 1 `PrismInstrument.FallbackSource`。
- Produces(Task 6 消费): `Report{Refreshed int; Failed []string; Degraded []string}`——Degraded 元素格式 `"SYMBOL: lixinger failed (<原因>), akshare fallback ok"`,兜底成功计入 Refreshed。

- [ ] **Step 1: 写失败测试**

```go
// Context Checkpoint:
//   主源败+兜底成→Degraded+Refreshed  → TestRefreshFallbackDegraded
//   双源皆败→Failed 含两个原因        → TestRefreshFallbackBothFail
//   主源成→不触碰 akshare             → TestRefreshFallbackNotTriggered

func fbInst() config.PrismInstrument {
	return config.PrismInstrument{Symbol: "000300.SH", Name: "沪深300", Type: "index",
		Market: "CN_A", Group: "A股指数", Source: "lixinger", FallbackSource: "akshare"}
}

func TestRefreshFallbackDegraded(t *testing.T) {
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("quota exhausted")}}
	ak := &fakeAkshare{indexPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 14.6, PB: math.NaN(), PSTTM: math.NaN()},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "兜底成功计入 Refreshed")
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "000300.SH")
	assert.Contains(t, rep.Degraded[0], "quota exhausted")
	require.Len(t, store.upserts["000300.SH"], 1, "兜底行已写入")
}

func TestRefreshFallbackBothFail(t *testing.T) {
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("lix down")}}
	ak := &fakeAkshare{fail: map[string]error{"000300.SH": errors.New("aktools down")}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "lix down")
	assert.Contains(t, rep.Failed[0], "aktools down")
}

func TestRefreshFallbackNotTriggered(t *testing.T) {
	store, lix, ak := newFakeStore(), &fakeLix{}, &fakeAkshare{}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, now)
	assert.Empty(t, rep.Failed)
	assert.Empty(t, rep.Degraded)
	assert.Empty(t, ak.indexCalls, "主源成功不得触碰兜底源(零多余请求)")
}
```

(import 补 `errors`,以文件现状为准。)

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/prism/ -run TestRefreshFallback -v`
Expected: FAIL(Report 无 Degraded 字段 / 降级未发生)

- [ ] **Step 3: 实现**

```go
// Report 变更:
type Report struct {
	Refreshed int
	Failed    []string // "SYMBOL: error" 摘要
	Degraded  []string // 主源失败但兜底成功的标的(可观测降级,进告警提示不算失败)
}

// Refresh 的 case "lixinger" 分支变更:
	case "lixinger":
		err = refreshLixinger(cfg, store, lix, inst, now)
		if err != nil && inst.FallbackSource == "akshare" {
			if fbErr := refreshAkshare(cfg, store, ak, inst, now); fbErr == nil {
				rep.Degraded = append(rep.Degraded,
					fmt.Sprintf("%s: lixinger failed (%v), akshare fallback ok", inst.Symbol, err))
				err = nil
			} else {
				err = fmt.Errorf("lixinger: %v; akshare fallback: %v", err, fbErr)
			}
		}
```

- [ ] **Step 4: 跑全包测试确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -v`
Expected: 全部用例 PASS(既有理杏仁/engine/akshare 用例无回归)

- [ ] **Step 5: Commit**

```bash
git add internal/prism/
git commit -m "feat(prism): automatic akshare fallback chain with observable Degraded report"
```

---

### Task 6: CLI 接线(akshare client 注入 + Degraded 告警)

**Files:**
- Modify: `cmd/atlas/prism.go`
- Test: `cmd/atlas/prism_test.go`(追加)

**Interfaces:**
- Consumes: Task 5 `Report.Degraded`;Task 1 `PrismConfig.AkshareBaseURL`;既有 `runPrismRefreshWith`/`prismRefreshDeps`/`textSender`。
- Produces: 告警语义扩展——Failed 或 Degraded 非空即发通知(exit 语义不变: 部分失败/降级均返回 nil)。

- [ ] **Step 1: 写失败测试**

`cmd/atlas/prism_test.go` 追加:

```go
// Context Checkpoint: 仅 Degraded 也发告警且 exit 0 → TestPrismRefreshNotifiesOnDegraded
func TestPrismRefreshNotifiesOnDegraded(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 8, Degraded: []string{"000300.SH: lixinger failed (quota), akshare fallback ok"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0], "兜底")
	assert.Contains(t, sender.sent[0], "000300.SH")
}
```

(import 补 `require`,以文件现状为准。)

- [ ] **Step 2: 跑测试确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestPrismRefreshNotifiesOnDegraded -count=1 -v`
Expected: FAIL(无告警发出)

- [ ] **Step 3: 实现**

`cmd/atlas/prism.go` 的 `runPrismRefreshWith` 变更(保持既有结构,只扩展消息组装):

```go
func runPrismRefreshWith(d prismRefreshDeps) error {
	rep := d.refresh()
	fmt.Fprintf(d.out, "prism refresh: %d ok, %d failed, %d degraded\n",
		rep.Refreshed, len(rep.Failed), len(rep.Degraded))
	if len(rep.Failed) == 0 && len(rep.Degraded) == 0 {
		return nil
	}
	var parts []string
	if len(rep.Failed) > 0 {
		parts = append(parts, "⚠️ Prism 刷新部分失败:\n"+strings.Join(rep.Failed, "\n"))
	}
	if len(rep.Degraded) > 0 {
		parts = append(parts, "ℹ️ Prism 主源降级(已兜底):\n"+strings.Join(rep.Degraded, "\n"))
	}
	msg := strings.Join(parts, "\n")
	if d.sender == nil {
		fmt.Fprintln(d.out, msg)
		return nil
	}
	if err := d.sender.SendText(msg); err != nil {
		fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
	}
	return nil
}
```

`runPrismRefresh` 中 akshare client 构造(Task 4 已加,确认为):

```go
	ak := akshare.New(pcfg.AkshareBaseURL)
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Refresh(pcfg, store, lix, yh, ak, time.Now())
		},
		...
	}
```

- [ ] **Step 4: 跑测试 + 编译**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestPrismRefresh -count=1 -v && GOTOOLCHAIN=local go build ./...`
Expected: 新旧 prism CLI 用例全 PASS(既有用例断言 "0 failed" 的输出格式若受 degraded 字样影响,同步微调断言),全仓编译过

- [ ] **Step 5: Commit**

```bash
git add cmd/atlas/prism.go cmd/atlas/prism_test.go
git commit -m "feat(prism): wire akshare client and degraded-fallback alerting into CLI"
```

---

### Task 7: 部署产物(独立 venv + aktools launchd + 配置 + 文档)

**Files:**
- Create: `scripts/akshare/setup.sh`、`scripts/akshare/requirements.txt`
- Create: `deploy/launchd/com.newthinker.atlas.aktools.plist`
- Modify: `configs/config.example.yaml`(prism 段)
- Modify: `docs/deployment.md`(追加 aktools 段,只追加不重排)

**Interfaces:**
- Consumes: Task 6 完成的 CLI;既有 launchd 家族约定(WorkingDirectory=/Users/zuowei/workspace/runtime/atlas,日志进 logs/)。
- Produces: 可部署的 aktools 侧车 + M1.5 验收清单。

- [ ] **Step 1: setup.sh + requirements**

```bash
#!/usr/bin/env bash
# scripts/akshare/setup.sh — 幂等创建 akshare 独立 venv 并安装依赖。
# 与 qlib_eval 的 venv 完全隔离(两者依赖树庞大且各自随上游更新)。
# 用法: bash scripts/akshare/setup.sh [python3 解释器,默认 python3.11]
set -euo pipefail
cd "$(dirname "$0")"
PY="${1:-python3.11}"
command -v "$PY" >/dev/null || PY=python3
[ -d .venv ] || "$PY" -m venv .venv
./.venv/bin/pip install --upgrade pip
./.venv/bin/pip install -r requirements.txt
# 版本冻结留档(升级时对比回滚依据)
./.venv/bin/pip freeze > requirements.lock
echo "OK: $(./.venv/bin/python -c 'import akshare; print("akshare", akshare.__version__)')"
echo "启动测试: ./.venv/bin/python -m aktools --help"
```

```
# scripts/akshare/requirements.txt
akshare
aktools
```

(版本以安装时最新为准,`requirements.lock` 由 setup.sh 生成留档——lock 文件加入 git 提交,升级经重跑 setup.sh + 验证后更新。)

- [ ] **Step 2: aktools launchd plist**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.newthinker.atlas.aktools</string>

  <!-- 隔离 venv 解释器;实参以实现时 aktools CLI 为准(⚠ live 校验: --host/--port 旗标名) -->
  <key>ProgramArguments</key>
  <array>
    <string>/Users/zuowei/workspace/runtime/atlas/scripts/akshare/.venv/bin/python</string>
    <string>-m</string>
    <string>aktools</string>
    <string>--host</string>
    <string>127.0.0.1</string>
    <string>--port</string>
    <string>8180</string>
  </array>

  <key>WorkingDirectory</key>
  <string>/Users/zuowei/workspace/runtime/atlas</string>

  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>

  <key>StandardOutPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/aktools.out.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/aktools.err.log</string>
</dict>
</plist>
```

Run: `plutil -lint deploy/launchd/com.newthinker.atlas.aktools.plist` → Expected: OK

- [ ] **Step 3: 配置示例变更**

`configs/config.example.yaml` prism 段: `enabled` 下追加 `akshare_base_url: "http://127.0.0.1:8180"`;池变更(茅台/腾讯改 akshare,两 A 股指数加 fallback):

```yaml
    - {symbol: "000300.SH", name: "沪深300", type: "index", market: "CN_A", group: "A股指数", source: "lixinger", fallback_source: "akshare"}
    - {symbol: "000905.SH", name: "中证500", type: "index", market: "CN_A", group: "A股指数", source: "lixinger", fallback_source: "akshare"}
    - {symbol: "600519.SH", name: "贵州茅台", type: "stock", market: "CN_A", group: "A股公司", source: "akshare"}
    - {symbol: "0700.HK",   name: "腾讯控股", type: "stock", market: "HK", group: "港股公司", source: "akshare"}
```

(其余标的行不动。)

- [ ] **Step 4: 部署文档**

`docs/deployment.md`: 服务清单表追加 `aktools | 常驻 | AKShare HTTP 侧车(127.0.0.1:8180,Prism A/H 公司与指数兜底数据源)` 一行;Prism 段之后追加「AKShare 侧车(aktools)部署要点」子节: `bash scripts/akshare/setup.sh` 建 venv → `launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.aktools.plist` → `curl 127.0.0.1:8180` 验证 → 日志路径 → 升级流程(重跑 setup.sh,对比 requirements.lock)→ 口径说明(兜底期指数分位为本地计算,与官方 cvpos 有方法论差异)。只追加不重排。

- [ ] **Step 5: Commit**

```bash
git add scripts/akshare/ deploy/launchd/com.newthinker.atlas.aktools.plist configs/config.example.yaml docs/deployment.md
git commit -m "feat(prism): aktools sidecar deployment (isolated venv, launchd, config, docs)"
```

- [ ] **Step 6: M1.5 验收(手动,记录结果)**

1. `bash scripts/akshare/setup.sh` 成功;launchd 拉起 aktools,`curl 127.0.0.1:8180` 可达。
2. runtime 配置同步池变更后跑 `atlas prism refresh`: 茅台/腾讯出现在 board,detail 曲线连续;⚠ 各 live 校验点(lg/baidu/index 接口名与字段)首跑核验,不符即修常量+测试。
3. 指数兜底演练: 临时改坏 lixinger api_key 跑 refresh → 指数当日行仍写入,告警含「主源降级(已兜底)」;恢复后次日主源续跑。
4. 二跑增量: akshare 标的零重复拉取(latest+1 语义,行数不变)。
5. 分位 sanity: 茅台 5Y/10Y 分位在 0-100 且与理杏仁官网同日数字方向一致(本地计算口径,允许数值差异)。

---

## Self-Review 记录

- **Spec 覆盖**: §1 接口选型→Task 2/3(含全部 live 校验点标注);§2 组件→Task 1(config)/2/3(collector)/4(refreshAkshare+Store.Series+Report 雏形)/5(Degraded)/6(CLI);§3 数据流与分位→Task 4(增量+合并算分位+仅写新点+NaN PE 剔除);§4 部署→Task 7(独立 venv/setup.sh/plist/文档);§5 测试→各 Task Step 1(降级链三态、零多余请求、httptest 全 mock);池配置→Task 7 Step 3;验收→Task 7 Step 6。「明确不做」清单未被任何 Task 违反。
- **占位符**: 无 TBD;aktools CLI 旗标与 AKShare 字段键为显式 ⚠ live 校验点(非占位,附修正路径);requirements 不钉版本是设计决策(lock 文件留档)。
- **类型一致性**: `akshare.ValuationPoint{Date, PETTM, PB, PSTTM}` 定义(T2)与消费(T4)一致;`AkshareClient` 两方法签名 T4 定义、T5/T6 消费一致;`Refresh(cfg, store, lix, us, ak, now)` 六参在 T4 变更并同步全部调用点,T5/T6 沿用;`Report.Degraded` T5 定义 T6 消费;`incrementalStart` 提取以既有 lixinger 用例为安全网。
