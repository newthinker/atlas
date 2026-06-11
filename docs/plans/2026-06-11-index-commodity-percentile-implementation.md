# 指数/大宗商品采集 + 历史百分位监控策略 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使 atlas 能监控国际指数（^GSPC/^IXIC/^DJI/^HSI）、A 股指数、国际商品期货（GC=F 等），并对全部资产提供价格历史百分位策略、对股票+指数提供 PE 估值百分位策略（理杏仁多市场兜底）。

**Architecture:** 扩展现有 yahoo/eastmoney/lixinger 采集器（不新建采集器）；新增 `internal/valuation` 纯函数包承载分位计算与 PE 序列重建；新增 `price_percentile`、`pe_percentile` 两个策略；app 装配层负责估值分位编排（A 股理杏仁 / 美港 Yahoo EPS 重建→理杏仁兜底）与 AssetTypes 绑定校验。信号链路（router/notifier/backtest）零改动。

**Tech Stack:** Go 1.21、标准库 testing + httptest（沿用现有测试风格，无第三方断言库）、Yahoo chart & fundamentals-timeseries 接口、东方财富 push2 接口、理杏仁开放平台 API。

**设计依据（必读）：** `docs/plans/2026-06-11-index-commodity-percentile-design.md`（rev6 终版）

**执行纪律：**
- 严格 TDD：先写失败测试 → 验证失败 → 最小实现 → 验证通过 → 提交
- 提交信息格式 `feat(scope): ...` / `test(scope): ...`，每个 Task 至少一次提交
- 全部 Task 完成后、最终集成提交前：运行 code-simplifier sub-agent（全局规范）
- 所有新增 HTTP 调用必须可注入 baseURL（httptest 模式，参照 `lixinger_httptest_test.go`）

---

## Chunk 1: 核心类型 + 采集层扩展

### Task 1: core 类型扩展（AssetCrypto / EPSPoint / PEPercentile）

**Files:**
- Modify: `internal/core/types.go`（AssetType 常量块 :19-25、Fundamental 结构 :79-94）

数据结构无行为，不单独立测（编译即验证；消费方测试覆盖）。

- [ ] **Step 1: 修改 types.go**

在 AssetType 常量块（`AssetCommodity` 之后）追加：

```go
	AssetCrypto    AssetType = "crypto"
```

在 `Fundamental` 结构的 `Source` 字段前追加：

```go
	// PEPercentile is the position of current PE in its historical series,
	// 0-100. Negative means unavailable. Source encodes how it was obtained:
	// "lixinger_cvpos", "reconstructed", or "method:fallback_reason".
	PEPercentile float64
```

在文件末尾（或 OHLCV 之后）新增：

```go
// EPSPoint is one point of a trailing-twelve-month diluted EPS series.
type EPSPoint struct {
	Date time.Time
	EPS  float64
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add internal/core/types.go
git commit -m "feat(core): add AssetCrypto, EPSPoint and Fundamental.PEPercentile"
```

### Task 2: yahoo 符号校验与 URL 编码（指数 ^ 与期货 =F）

**Files:**
- Modify: `internal/collector/yahoo/yahoo.go`（validSymbol :21、FetchQuote URL :96、FetchHistory URL :160-161）
- Test: `internal/collector/yahoo/yahoo_test.go`

- [ ] **Step 1: 写失败测试（表驱动校验 + URL 编码断言）**

在 `yahoo_test.go` 追加：

```go
func TestValidateSymbol_IndexAndFutures(t *testing.T) {
	cases := []struct {
		symbol string
		wantOK bool
	}{
		{"AAPL", true}, {"600519.SH", true}, {"0700.HK", true},
		{"^GSPC", true}, {"^IXIC", true}, {"^HSI", true},
		{"GC=F", true}, {"CL=F", true}, {"SI=F", true},
		{"", false}, {"^", false}, {"=F", false},
		{"^GSPC.SH", false}, {"GC=X=F", false}, {"AAPL; DROP", false},
	}
	for _, c := range cases {
		err := validateSymbol(c.symbol)
		if (err == nil) != c.wantOK {
			t.Errorf("validateSymbol(%q) ok=%v, want %v", c.symbol, err == nil, c.wantOK)
		}
	}
}

func TestFetchQuote_EscapesIndexSymbol(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketPrice":5000,"chartPreviousClose":4990,"symbol":"^GSPC"},"timestamp":[1700000000],"indicators":{"quote":[{"open":[4995],"high":[5010],"low":[4980],"close":[5000],"volume":[1000]}]}}]}}`))
	}))
	defer srv.Close()

	y := NewWithBaseURL(srv.URL)
	if _, err := y.FetchQuote("^GSPC"); err != nil {
		t.Fatalf("FetchQuote(^GSPC) error: %v", err)
	}
	if !strings.Contains(gotPath, "%5EGSPC") {
		t.Errorf("request path %q does not percent-encode ^ as %%5E", gotPath)
	}
}
```

注：JSON fixture 字段需与 `yahoo.go` 现有 chart 响应解析结构一致，执行时以现有测试 fixture 为准对齐。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/yahoo/ -run 'TestValidateSymbol_IndexAndFutures|TestFetchQuote_Escapes' -v`
Expected: FAIL（`^GSPC`、`GC=F` 被现有正则拒绝）

- [ ] **Step 3: 最小实现**

```go
// validSymbol matches stock symbols (AAPL, 600519.SH, 0700.HK),
// index symbols (^GSPC) and futures symbols (GC=F).
// Validation is purely syntactic and intentionally decoupled from the
// phase-1 coverage list (see design §2.1).
var validSymbol = regexp.MustCompile(`^(\^[A-Za-z0-9]{1,10}|[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?|[A-Za-z]{1,6}=F)$`)
```

FetchQuote/FetchHistory 的 URL 构造处将 `yahooSymbol` 替换为 `url.PathEscape(yahooSymbol)`（两处，import `net/url`）。**注意**：这两个函数现有局部变量名为 `url`，会遮蔽 `net/url` 包——同步将局部变量改名为 `reqURL`，避免作用域陷阱。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/yahoo/ -v`
Expected: 全部 PASS（含既有用例不回归）

- [ ] **Step 5: 提交**

```bash
git add internal/collector/yahoo/
git commit -m "feat(yahoo): accept index(^) and futures(=F) symbols with URL escaping"
```

### Task 3: yahoo FetchEPSHistory（历史 EPS(TTM) 序列）

**Files:**
- Create: `internal/collector/yahoo/eps.go`
- Test: `internal/collector/yahoo/eps_test.go`
- Modify: `internal/collector/yahoo/yahoo.go`（Yahoo 结构体加 `epsBaseURL` 字段；`New` 设默认值；新增 `NewWithBaseURLs(chartURL, epsURL string)` 测试构造器）

- [ ] **Step 1: 写失败测试**

```go
func TestFetchEPSHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "type=trailingDilutedEPS") {
			t.Errorf("missing type param: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{"type":["trailingDilutedEPS"]},
			"timestamp":[1672444800,1680307200],
			"trailingDilutedEPS":[
				{"asOfDate":"2022-12-31","reportedValue":{"raw":6.11}},
				{"asOfDate":"2023-03-31","reportedValue":{"raw":5.89}}
			]}]}}`))
	}))
	defer srv.Close()

	y := NewWithBaseURLs(srv.URL, srv.URL)
	pts, err := y.FetchEPSHistory("AAPL",
		time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchEPSHistory: %v", err)
	}
	if len(pts) != 2 || pts[0].EPS != 6.11 || pts[1].Date.Format("2006-01-02") != "2023-03-31" {
		t.Errorf("unexpected points: %+v", pts)
	}
}

func TestFetchEPSHistory_EmptyAndIndexSymbol(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"timeseries":{"result":[{"meta":{}}]}}`))
	}))
	defer srv.Close()
	y := NewWithBaseURLs(srv.URL, srv.URL)
	if pts, err := y.FetchEPSHistory("SOMESTOCK", time.Now().AddDate(-5, 0, 0), time.Now()); err == nil && len(pts) > 0 {
		t.Errorf("expected empty result, got %+v", pts)
	}
	// 指数符号不应发起请求，直接报错（设计 §2.5：仅个股）
	if _, err := y.FetchEPSHistory("^GSPC", time.Now().AddDate(-5, 0, 0), time.Now()); err == nil {
		t.Error("expected error for index symbol")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/yahoo/ -run TestFetchEPSHistory -v`
Expected: FAIL（FetchEPSHistory 未定义）

- [ ] **Step 3: 实现 eps.go**

```go
package yahoo

// defaultEPSBaseURL is Yahoo's fundamentals-timeseries endpoint. Unofficial:
// subject to anti-bot and schema changes; failures degrade per design §5.
const defaultEPSBaseURL = "https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries"

// FetchEPSHistory returns the trailing-diluted-EPS (TTM) quarterly series for
// a single equity. Index symbols (^ prefix) carry no filings and are rejected.
func (y *Yahoo) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	if strings.HasPrefix(symbol, "^") {
		return nil, fmt.Errorf("eps history unavailable for index symbol %s", symbol)
	}
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/%s?type=trailingDilutedEPS&period1=%d&period2=%d",
		y.epsBaseURL, url.PathEscape(y.toYahooSymbol(symbol)), start.Unix(), end.Unix())
	// 构造请求：复用与 FetchQuote 相同的 UA/Accept 头（抽出 newRequest helper 或复制三行）
	// 解析：timeseries.result[0].trailingDilutedEPS[] → {asOfDate, reportedValue.raw}
	// asOfDate 按 "2006-01-02" 解析；reportedValue.raw <= 0 的点保留（valuation 层负责剔除语义）
	// 按 Date 升序排序后返回；result 为空或字段缺失返回空 slice + nil error 由调用方按点数门槛判定
	...
}
```

（执行者按上述注释写完整解析逻辑；JSON 结构体局部定义在函数内或文件级私有类型。）

`yahoo.go` 改动：

```go
type Yahoo struct {
	client     *http.Client
	config     collector.Config
	baseURL    string
	epsBaseURL string
}
// New(): epsBaseURL: defaultEPSBaseURL
// NewWithBaseURL(u): 两个都设为 u（兼容既有测试）
// NewWithBaseURLs(chartURL, epsURL string) *Yahoo  // 新增
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/yahoo/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/yahoo/
git commit -m "feat(yahoo): add FetchEPSHistory via fundamentals-timeseries endpoint"
```

### Task 4: A 股指数代码表 + eastmoney 指数 secid

**Files:**
- Create: `internal/collector/indexes.go`（共享表，eastmoney 与 selector 共用，避免 import 环）
- Create: `internal/collector/indexes_test.go`（IsAShareIndex 表驱动小用例：`000300.SH`→true、`000001.SH`→true、`000001.SZ`→false、`600519.SH`→false）
- Modify: `internal/collector/eastmoney/eastmoney.go`（parseSymbol :99-115）
- Test: `internal/collector/eastmoney/eastmoney_test.go`

- [ ] **Step 1: 写失败测试**

eastmoney_test.go：

```go
func TestParseSymbol_AShareIndexes(t *testing.T) {
	e := New()
	cases := []struct{ symbol, wantCode, wantMarket string }{
		{"000300.SH", "000300", "1"}, // 沪深300 → secid 1.000300
		{"000905.SH", "000905", "1"}, // 中证500
		{"000001.SH", "000001", "1"}, // 上证指数（指数表命中）
		{"000001.SZ", "000001", "0"}, // 平安银行（个股，走后缀规则）
		{"399001.SZ", "399001", "0"}, // 深证成指
		{"600519.SH", "600519", "1"}, // 个股不受影响
	}
	for _, c := range cases {
		code, market := e.parseSymbol(c.symbol)
		if code != c.wantCode || market != c.wantMarket {
			t.Errorf("parseSymbol(%q) = (%s,%s), want (%s,%s)",
				c.symbol, code, market, c.wantCode, c.wantMarket)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/eastmoney/ -run TestParseSymbol_AShareIndexes -v`
Expected: 现有实现下 `000300.SH` 等按后缀规则已返回 ("000300","1")——本用例可能直接通过。**若直接通过，仍需创建共享表**（Step 3），因为 selector（Task 5）与 DetectType（Task 6）依赖表做"指数 vs 个股"判别，secid 映射只是顺带验证。把断言重点放在 `000001.SH`（指数）与 `000001.SZ`（个股）市场前缀正确区分。

- [ ] **Step 3: 实现共享表 internal/collector/indexes.go**

```go
package collector

// AShareIndexSecIDs maps A-share index symbols (watchlist form) to Eastmoney
// secids. Membership in this map is also how the rest of the codebase tells
// an A-share index apart from an equity with the same numeric code.
var AShareIndexSecIDs = map[string]string{
	"000001.SH": "1.000001", // 上证指数
	"000016.SH": "1.000016", // 上证50
	"000300.SH": "1.000300", // 沪深300
	"000905.SH": "1.000905", // 中证500
	"399001.SZ": "0.399001", // 深证成指
	"399006.SZ": "0.399006", // 创业板指
}

// IsAShareIndex reports whether symbol is a known A-share index.
func IsAShareIndex(symbol string) bool {
	_, ok := AShareIndexSecIDs[symbol]
	return ok
}
```

eastmoney `parseSymbol` 开头增加：

```go
	if secid, ok := collector.AShareIndexSecIDs[symbol]; ok {
		parts := strings.SplitN(secid, ".", 2)
		return parts[1], parts[0]
	}
```

（eastmoney 包已 import `internal/collector`，无新依赖。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/... -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/indexes.go internal/collector/eastmoney/
git commit -m "feat(eastmoney): A-share index secid table shared via collector package"
```

### Task 5: selector 路由与市场归属

**Files:**
- Modify: `internal/collector/selector.go`
- Test: `internal/collector/selector_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestMarketForSymbol_IndexAndCommodity(t *testing.T) {
	cases := []struct {
		symbol string
		want   core.Market
	}{
		{"^GSPC", core.MarketUS}, {"^IXIC", core.MarketUS}, {"^DJI", core.MarketUS},
		{"^HSI", core.MarketHK},
		{"^N225", core.MarketUS}, // 表外 ^ 符号默认 US（warning 由 app 层负责）
		{"GC=F", core.MarketUS}, {"CL=F", core.MarketUS},
		{"000300.SH", core.MarketCNA},
		{"AAPL", core.MarketUS}, {"BTC-USDT", core.MarketCrypto},
	}
	for _, c := range cases {
		if got := MarketForSymbol(c.symbol); got != c.want {
			t.Errorf("MarketForSymbol(%q) = %v, want %v", c.symbol, got, c.want)
		}
	}
}

func TestSelectForSymbol_IndexAndCommodityRouteToYahoo(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto") // 复用 selector_test.go 既有 helper
	for _, sym := range []string{"^GSPC", "^HSI", "GC=F"} {
		if c := SelectForSymbol(reg, sym); c == nil || c.Name() != "yahoo" {
			t.Errorf("SelectForSymbol(%q) -> %v, want yahoo", sym, c)
		}
	}
	if c := SelectForSymbol(reg, "000300.SH"); c == nil || c.Name() != "eastmoney" {
		t.Errorf("000300.SH should route to eastmoney")
	}
}
```

（注意：selector_test.go 既有 `fakeCollector` 的方法是**指针接收者**，值字面量不实现 Collector 接口；务必复用既有 `newRegistryWith` helper，自建时必须用 `&fakeCollector{...}`。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/ -run 'TestMarketForSymbol_Index|TestSelectForSymbol_Index' -v`
Expected: FAIL（`^GSPC` 现走 default → US 可能恰好对，但 `^HSI` 应为 HK 会 FAIL；`GC=F` 路由断言取决于 stub）

- [ ] **Step 3: 实现**

selector.go 增加：

```go
// indexMarkets pins the market for the phase-1 international index list.
// Symbols with a ^ prefix not present here default to MarketUS; the app
// assembly layer logs a warning for such bindings (design §2.3).
var indexMarkets = map[string]core.Market{
	"^GSPC": core.MarketUS,
	"^IXIC": core.MarketUS,
	"^DJI":  core.MarketUS,
	"^HSI":  core.MarketHK,
}

func isIndexSymbol(upper string) bool     { return strings.HasPrefix(upper, "^") }
func isCommoditySymbol(upper string) bool { return strings.HasSuffix(upper, "=F") }

// KnownIndexMarket reports whether a ^-prefixed symbol is in the phase-1
// index list and its market. The app assembly layer warns on unknown ones.
func KnownIndexMarket(symbol string) (core.Market, bool) {
	m, ok := indexMarkets[strings.ToUpper(symbol)]
	return m, ok
}
```

`SelectForSymbol` 的 switch 中、A 股分支之后 crypto 分支之前插入：

```go
	case isIndexSymbol(upper), isCommoditySymbol(upper):
		if c, ok := reg.Get("yahoo"); ok {
			return c
		}
```

`MarketForSymbol` 的 switch 中、A 股分支之后插入：

```go
	case isIndexSymbol(upper):
		if m, ok := indexMarkets[strings.ToUpper(symbol)]; ok {
			return m
		}
		return core.MarketUS
	case isCommoditySymbol(upper):
		return core.MarketUS
```

注意：A 股指数（`000300.SH`）不带 `^`，仍按 `.SH/.SZ` 走 CNA 分支，无需改动。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/selector.go internal/collector/selector_test.go
git commit -m "feat(selector): route index/futures symbols to yahoo with market mapping"
```

### Task 6: DetectType 识别指数/期货 + app 类型映射

**Files:**
- Modify: `internal/app/app.go`（Type 常量 :30-39、DetectType :552-559）
- Test: `internal/app/app_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestDetectType_IndexAndCommodity(t *testing.T) {
	cases := []struct{ symbol, want string }{
		{"^GSPC", TypeIndex}, {"^HSI", TypeIndex},
		{"000300.SH", TypeIndex}, {"000001.SH", TypeIndex},
		{"000001.SZ", TypeStock}, {"600519.SH", TypeStock},
		{"GC=F", TypeFuture},
		{"BTC-USDT", TypeCrypto}, {"AAPL", TypeStock},
	}
	for _, c := range cases {
		if got := DetectType(c.symbol); got != c.want {
			t.Errorf("DetectType(%q) = %q, want %q", c.symbol, got, c.want)
		}
	}
}

func TestAssetTypeOf(t *testing.T) {
	cases := []struct {
		appType string
		want    core.AssetType
	}{
		{TypeStock, core.AssetStock}, {TypeIndex, core.AssetIndex},
		{TypeETF, core.AssetETF}, {TypeFund, core.AssetFund},
		{TypeFuture, core.AssetCommodity}, {TypeCrypto, core.AssetCrypto},
		{TypeBond, ""}, // 一期不支持 → 空值，装配层按"全跳过+warning"处理
	}
	for _, c := range cases {
		if got := assetTypeOf(c.appType); got != c.want {
			t.Errorf("assetTypeOf(%q) = %q, want %q", c.appType, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run 'TestDetectType_Index|TestAssetTypeOf' -v`
Expected: FAIL（TypeIndex 常量与 assetTypeOf 未定义）

- [ ] **Step 3: 实现**

Type 常量块追加 `TypeIndex = "指数"`。同步在 `DetectMarket`（app.go:537-549）的 switch 中、`.HK` 分支之后加一条 `case upperSymbol == "^HSI": return MarketHShare`，避免 UI 市场标签（美股/H股）与 `collector.MarketForSymbol` 的 HK 归属不一致（既有测试 `TestApp_DetectMarketAndType` 补一行用例）。DetectType 重写：

```go
func DetectType(symbol string) string {
	upperSymbol := strings.ToUpper(symbol)
	switch {
	case strings.HasPrefix(upperSymbol, "^"), collector.IsAShareIndex(symbol):
		return TypeIndex
	case strings.HasSuffix(upperSymbol, "=F"):
		return TypeFuture
	case strings.Contains(upperSymbol, "-USD") || strings.Contains(upperSymbol, "-USDT") ||
		strings.HasPrefix(upperSymbol, "BTC") || strings.HasPrefix(upperSymbol, "ETH"):
		return TypeCrypto
	default:
		return TypeStock
	}
}

// assetTypeOf maps the Chinese UI type label to the core asset type used by
// strategy AssetTypes declarations. Empty means unsupported in phase 1.
func assetTypeOf(appType string) core.AssetType {
	switch appType {
	case TypeStock:
		return core.AssetStock
	case TypeIndex:
		return core.AssetIndex
	case TypeETF:
		return core.AssetETF
	case TypeFund:
		return core.AssetFund
	case TypeFuture:
		return core.AssetCommodity
	case TypeCrypto:
		return core.AssetCrypto
	default:
		return ""
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/app/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/
git commit -m "feat(app): detect index/futures asset types with core mapping"
```

### Task 7: lixinger FetchValuationPercentile（cn/hk/us × company/index 分派）

**Files:**
- Create: `internal/collector/lixinger/valuation.go`
- Test: `internal/collector/lixinger/valuation_test.go`（httptest 模式，参照 `lixinger_httptest_test.go`）

- [ ] **Step 1: 写失败测试**

```go
func TestEndpointFor(t *testing.T) {
	cases := []struct {
		symbol       string
		wantEndpoint string // 相对路径
		wantCode     string
	}{
		{"600519.SH", "cn/company/fundamental/non_financial", "600519"},
		{"000300.SH", "cn/index/fundamental", "000300"},
		{"0700.HK", "hk/company/fundamental", "00700"},
		{"AAPL", "us/company/fundamental", "AAPL"},
		{"^GSPC", "us/index/fundamental", "SPX"},
		{"^HSI", "hk/index/fundamental", "HSI"},
		{"GC=F", "", ""}, // 商品无估值分位
	}
	for _, c := range cases {
		ep, code := endpointFor(c.symbol)
		if ep != c.wantEndpoint || code != c.wantCode {
			t.Errorf("endpointFor(%q) = (%q,%q), want (%q,%q)",
				c.symbol, ep, code, c.wantEndpoint, c.wantCode)
		}
	}
}

func TestFetchValuationPercentile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "pe_ttm.y5.cvpos") {
			t.Errorf("missing metric in body: %s", body)
		}
		// 成功码约定：与既有 lixinger.go 一致（code 0 = 成功，非 0 = 失败）。
		// ⚠️ 实现首日核对项：用真实 API 验证成功码/metrics 键名两个约定，
		//    若真实 API 与既有代码不符，统一修正全包并更新所有 fixture。
		w.Write([]byte(`{"code":0,"data":[{"date":"2026-06-10","stockCode":"600519",
			"pe_ttm":{"y5":{"cvpos":0.2345}}}]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	got, err := lx.FetchValuationPercentile("600519.SH", 5)
	if err != nil {
		t.Fatalf("FetchValuationPercentile: %v", err)
	}
	if got < 23.44 || got > 23.46 { // cvpos 0-1 → 0-100
		t.Errorf("percentile = %v, want 23.45", got)
	}
}

func TestFetchValuationPercentile_Unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":403,"message":"no permission"}`)) // 权限不足（非 0 = 失败）
	}))
	defer srv.Close()
	lx := NewWithBaseURL("test-key", srv.URL)
	if _, err := lx.FetchValuationPercentile("AAPL", 5); err == nil {
		t.Error("expected error on permission failure")
	}
	if _, err := lx.FetchValuationPercentile("GC=F", 5); err == nil {
		t.Error("expected error for commodity symbol")
	}
}
```

（`NewWithBaseURL` 若 lixinger 包尚无此构造器，按 `lixinger_httptest_test.go` 现有注入方式对齐；没有则新增。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/collector/lixinger/ -run 'TestEndpointFor|TestFetchValuationPercentile' -v`
Expected: FAIL（函数未定义）

- [ ] **Step 3: 实现 valuation.go**

```go
package lixinger

// usHKIndexCodes maps phase-1 international index symbols to Lixinger codes.
// Candidate values — verify against the basic-info/samples API on first
// implementation day and freeze (design §2.4).
var usHKIndexCodes = map[string]struct{ endpoint, code string }{
	"^GSPC": {"us/index/fundamental", "SPX"},
	"^IXIC": {"us/index/fundamental", "COMP"},
	"^DJI":  {"us/index/fundamental", "DJI"},
	"^HSI":  {"hk/index/fundamental", "HSI"},
}

// endpointFor returns the Lixinger fundamental endpoint (relative path) and
// the Lixinger security code for a watchlist symbol. Empty endpoint means
// valuation percentile is not available for this symbol class.
func endpointFor(symbol string) (endpoint, code string) {
	switch {
	case collector.IsAShareIndex(symbol):
		return "cn/index/fundamental", strings.SplitN(symbol, ".", 2)[0]
	case strings.HasSuffix(symbol, ".SH"), strings.HasSuffix(symbol, ".SZ"):
		// 金融股（银行/券商/保险）走 non_financial 会失败 → 调用方按不可用降级（一期边界）
		return "cn/company/fundamental/non_financial", strings.SplitN(symbol, ".", 2)[0]
	case strings.HasPrefix(symbol, "^"):
		if m, ok := usHKIndexCodes[symbol]; ok {
			return m.endpoint, m.code
		}
		return "", ""
	case strings.HasSuffix(symbol, ".HK"):
		// 0700.HK → 00700（理杏仁港股 5 位代码）
		c := strings.TrimSuffix(symbol, ".HK")
		for len(c) < 5 {
			c = "0" + c
		}
		return "hk/company/fundamental", c
	case strings.HasSuffix(symbol, "=F"), strings.Contains(symbol, "-USD"):
		return "", ""
	default: // 美股个股
		return "us/company/fundamental", symbol
	}
}

// FetchValuationPercentile returns the PE-TTM historical percentile (0-100)
// for a stock or index, using Lixinger's cvpos metric. lookbackYears maps to
// the closest supported granularity (y3/y5/y10).
func (l *Lixinger) FetchValuationPercentile(symbol string, lookbackYears int) (float64, error) {
	endpoint, code := endpointFor(symbol)
	if endpoint == "" {
		return -1, fmt.Errorf("lixinger: valuation percentile unsupported for %s", symbol)
	}
	gran := "y10"
	switch {
	case lookbackYears <= 3:
		gran = "y3"
	case lookbackYears <= 5:
		gran = "y5"
	}
	metric := fmt.Sprintf("pe_ttm.%s.cvpos", gran)
	// POST {baseURL}/{endpoint}，body: {token, date:"latest", stockCodes:[code], metricsList:[metric]}
	//   ⚠️ 请求体键名（metricsList vs metrics）属实现首日核对项，与成功码一起对照真实 API 固化
	// 解析 data[0] 按 metric 路径逐层下钻（map[string]any），cvpos ∈ [0,1] → ×100 返回
	// code != 0 / data 为空 / 字段缺失 → 返回 (-1, error)（含权限不足场景）
	...
}
```

（注意 `postJSON` 内部已把响应 decode 进平铺的 `lixingerResponse` 并消费了 body，无法"只借它发请求"——而该结构承载不了 `pe_ttm.y5.cvpos` 嵌套。参照 `postJSON` 的 POST 三段式**自写**发送+解析，响应 decode 进 `map[string]any` 后按 metric 路径下钻；或抽一个返回原始 body 的 `postJSONRaw` 变体供两处共用。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/collector/lixinger/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/lixinger/
git commit -m "feat(lixinger): multi-market valuation percentile (cn/hk/us company+index)"
```

---

## Chunk 2: valuation 包 + 双策略 + app 装配 + 配置

### Task 8: internal/valuation 纯函数包

**Files:**
- Create: `internal/valuation/percentile.go`、`internal/valuation/reconstruct.go`
- Test: `internal/valuation/percentile_test.go`、`internal/valuation/reconstruct_test.go`

- [ ] **Step 1: 写失败测试（percentile）**

```go
func TestPercentileRank(t *testing.T) {
	cases := []struct {
		name    string
		series  []float64
		current float64
		want    float64
	}{
		{"middle", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5.5, 50},
		{"lowest", []float64{1, 2, 3, 4}, 0.5, 0},
		{"highest", []float64{1, 2, 3, 4}, 9, 100},
		{"all-equal", []float64{3, 3, 3, 3}, 3, 0}, // strictly-less 口径
		{"single", []float64{7}, 7, 0},
	}
	for _, c := range cases {
		if got := PercentileRank(c.series, c.current); got != c.want {
			t.Errorf("%s: PercentileRank = %v, want %v", c.name, got, c.want)
		}
	}
	if got := PercentileRank(nil, 1); got != -1 {
		t.Errorf("empty series should return -1, got %v", got)
	}
}
```

- [ ] **Step 2: 写失败测试（reconstruct）**

```go
func bars(start time.Time, closes ...float64) []core.OHLCV {
	out := make([]core.OHLCV, len(closes))
	for i, c := range closes {
		out[i] = core.OHLCV{Close: c, Time: start.AddDate(0, 0, i)}
	}
	return out
}

func quarterlyEPS(start time.Time, eps ...float64) []core.EPSPoint {
	out := make([]core.EPSPoint, len(eps))
	for i, e := range eps {
		out[i] = core.EPSPoint{Date: start.AddDate(0, 3*i, 0), EPS: e}
	}
	return out
}

func TestReconstructPEPercentile_StepAlignment(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 8 个季度 EPS 全为正（满足门槛），价格恒定 100：
	// EPS 上升 → PE 下降 → 当前 PE 处于序列低位
	eps := quarterlyEPS(start, 4, 4, 4, 4, 5, 5, 5, 5)
	closes := bars(start.AddDate(0, 0, 1), repeat(100, 700)...) // ~23 个月日线
	got, err := ReconstructPEPercentile(closes, eps)
	if err != nil {
		t.Fatalf("ReconstructPEPercentile: %v", err)
	}
	if got > 50 {
		t.Errorf("rising EPS with flat price should put current PE in lower half, got %v", got)
	}
}

func TestReconstructPEPercentile_NotEqualToPricePercentile(t *testing.T) {
	// 回归用例：EPS 变动期，重建 PE 分位 ≠ 价格分位（设计 §6）
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	eps := quarterlyEPS(start, 2, 2, 2, 2, 8, 8, 8, 8) // EPS 跳升
	closes := bars(start.AddDate(0, 0, 1), linear(100, 200, 700)...)
	pePct, err := ReconstructPEPercentile(closes, eps)
	if err != nil {
		t.Fatal(err)
	}
	pricePct := PercentileRank(closesOf(closes), closes[len(closes)-1].Close)
	if diff := pePct - pricePct; diff > -1 && diff < 1 {
		t.Errorf("PE percentile (%v) should differ from price percentile (%v)", pePct, pricePct)
	}
}

func TestReconstructPEPercentile_Errors(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 有效季度点不足（<8）→ ErrInsufficientEPS（数据缺失 → 调用方可兜底）
	if _, err := ReconstructPEPercentile(bars(start, 100, 100), quarterlyEPS(start, 4, 4, 4)); !errors.Is(err, ErrInsufficientEPS) {
		t.Errorf("want ErrInsufficientEPS, got %v", err)
	}
	// 当前 EPS ≤ 0（真实亏损）→ ErrNonPositiveEPS（调用方直接跳过，不兜底）
	eps := quarterlyEPS(start, 4, 4, 4, 4, 4, 4, 4, 4, -1)
	if _, err := ReconstructPEPercentile(bars(start.AddDate(2, 0, 0), 100, 100), eps); !errors.Is(err, ErrNonPositiveEPS) {
		t.Errorf("want ErrNonPositiveEPS, got %v", err)
	}
	// 亏损季度剔除：中间一个负 EPS 季度的交易日不进 PE 序列，剩余 ≥8 个有效点仍可计算
	eps2 := quarterlyEPS(start, 4, 4, -2, 4, 4, 4, 4, 4, 4)
	if _, err := ReconstructPEPercentile(bars(start.AddDate(0, 1, 0), repeat(100, 800)...), eps2); err != nil {
		t.Errorf("one loss quarter among 8 valid should still compute, got %v", err)
	}
}
```

（`repeat`/`linear`/`closesOf` 为本测试文件内 5 行以内的小工具函数。）

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/valuation/ -v`
Expected: FAIL（包不存在）

- [ ] **Step 4: 实现**

percentile.go：

```go
// Package valuation provides pure functions for historical-percentile
// computations used by the price_percentile and pe_percentile strategies.
package valuation

// PercentileRank returns the percentage (0-100) of series values strictly
// less than current. Returns -1 for an empty series.
func PercentileRank(series []float64, current float64) float64 {
	if len(series) == 0 {
		return -1
	}
	less := 0
	for _, v := range series {
		if v < current {
			less++
		}
	}
	return float64(less) / float64(len(series)) * 100
}
```

reconstruct.go：

```go
var (
	// ErrInsufficientEPS: 有效（EPS>0）季度点 < MinEPSPoints，数据缺失，调用方可走理杏仁兜底。
	ErrInsufficientEPS = errors.New("valuation: insufficient positive EPS points")
	// ErrNonPositiveEPS: 当前 EPS(TTM) ≤ 0，真实亏损，PE 分位无意义，调用方直接跳过（不兜底）。
	ErrNonPositiveEPS = errors.New("valuation: current EPS is non-positive")
)

const MinEPSPoints = 8

// ReconstructPEPercentile rebuilds the historical PE series by aligning each
// daily close with the latest EPS(TTM) point at or before it (step function),
// drops days whose EPS <= 0, and returns the percentile of the current PE.
func ReconstructPEPercentile(closes []core.OHLCV, eps []core.EPSPoint) (float64, error) {
	// 1. eps 升序排序；统计 EPS>0 的点数 < MinEPSPoints → ErrInsufficientEPS
	// 2. 当前 EPS = 最后一个点；<=0 → ErrNonPositiveEPS
	// 3. 遍历 closes：二分/线性找 Date <= bar.Time 的最近 eps 点；无 → 跳过；EPS<=0 → 剔除
	// 4. 剔除后 PE 序列为空 → 同样返回 ErrInsufficientEPS（数据缺失语义统一进兜底链，
	//    绝不能让 PercentileRank 的 -1 带着 nil error 冒充"重建成功"）
	// 5. 当前 PE = 最后 close / 当前 EPS；return PercentileRank(peSeries, currentPE)
	...
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/valuation/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/valuation/
git commit -m "feat(valuation): percentile rank and PE-series reconstruction"
```

### Task 9: price_percentile 策略

**Files:**
- Create: `internal/strategy/price_percentile/strategy.go`
- Test: `internal/strategy/price_percentile/strategy_test.go`

- [ ] **Step 1: 写失败测试**

```go
func ctxWithCloses(closes []float64) strategy.AnalysisContext {
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	ohlcv := make([]core.OHLCV, len(closes))
	for i, c := range closes {
		ohlcv[i] = core.OHLCV{Symbol: "TEST", Close: c, Time: start.AddDate(0, 0, i)}
	}
	return strategy.AnalysisContext{Symbol: "TEST", OHLCV: ohlcv, Now: time.Now()}
}

func TestAnalyze_SignalBands(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{Params: map[string]any{}}) // 默认参数 25/75/10/90

	// 300 根K线（>252 门槛）。构造当前价为历史极低位 → strong_buy
	closes := make([]float64, 300)
	for i := range closes {
		closes[i] = 100 + float64(i%50)
	}
	closes[299] = 50 // 历史最低
	sigs, err := s.Analyze(ctxWithCloses(closes))
	if err != nil || len(sigs) != 1 || sigs[0].Action != core.ActionStrongBuy {
		t.Fatalf("want strong_buy, got %+v err=%v", sigs, err)
	}
	if sigs[0].Confidence < 0.8 || sigs[0].Confidence > 0.95 {
		t.Errorf("strong zone confidence out of [0.8,0.95]: %v", sigs[0].Confidence)
	}
	if _, ok := sigs[0].Metadata["percentile"]; !ok {
		t.Error("missing percentile metadata")
	}

	// 中位 → 无信号
	closes[299] = 125
	if sigs, _ := s.Analyze(ctxWithCloses(closes)); len(sigs) != 0 {
		t.Errorf("mid percentile should yield no signal, got %+v", sigs)
	}

	// 历史最高 → strong_sell
	closes[299] = 500
	if sigs, _ := s.Analyze(ctxWithCloses(closes)); len(sigs) != 1 || sigs[0].Action != core.ActionStrongSell {
		t.Errorf("want strong_sell, got %+v", sigs)
	}
}

func TestAnalyze_InsufficientHistory(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	if sigs, err := s.Analyze(ctxWithCloses(make([]float64, 100))); err != nil || len(sigs) != 0 {
		t.Errorf("‹252 bars must yield no signal, got %+v err=%v", sigs, err)
	}
}

func TestRequiredData(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{Params: map[string]any{"lookback_years": 3}})
	rd := s.RequiredData()
	if rd.PriceHistory != 3*252 {
		t.Errorf("PriceHistory = %d, want %d", rd.PriceHistory, 3*252)
	}
	if len(rd.AssetTypes) != 6 { // stock/index/etf/fund/commodity/crypto
		t.Errorf("AssetTypes = %v", rd.AssetTypes)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/strategy/price_percentile/ -v`
Expected: FAIL（包不存在）

- [ ] **Step 3: 实现 strategy.go**

```go
// Package price_percentile signals when the current price sits at an extreme
// of its own multi-year distribution. Applies to every asset class.
package price_percentile

type Strategy struct {
	lookbackYears int
	low, high     float64
	extremeLow    float64
	extremeHigh   float64
}

func New() *Strategy {
	return &Strategy{lookbackYears: 5, low: 25, high: 75, extremeLow: 10, extremeHigh: 90}
}

func (s *Strategy) Name() string        { return "price_percentile" }
func (s *Strategy) Description() string { return "Price position in its own multi-year history" }

func (s *Strategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		PriceHistory: s.lookbackYears * 252,
		AssetTypes: []core.AssetType{
			core.AssetStock, core.AssetIndex, core.AssetETF,
			core.AssetFund, core.AssetCommodity, core.AssetCrypto,
		},
	}
}

func (s *Strategy) Init(cfg strategy.Config) error {
	// 注意：YAML 经 viper 解析后数值可能是 int 也可能是 float64，
	// ma_crossover.Init 只处理 .(int) 单形态，不能照抄。用本地 helper：
	//   func numParam(p map[string]any, key string, def float64) float64 {
	//       switch v := p[key].(type) {
	//       case int: return float64(v)
	//       case float64: return v
	//       }
	//       return def
	//   }
	// 读取 lookback_years/low/high/extreme_low/extreme_high 后校验
	// extreme_low < low < high < extreme_high，违反返回 error
	...
}

const minSampleBars = 252 // 不足 1 年样本不出信号（新上市资产防误报）

func (s *Strategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if len(ctx.OHLCV) < minSampleBars {
		return nil, nil
	}
	closes := make([]float64, len(ctx.OHLCV))
	for i, b := range ctx.OHLCV {
		closes[i] = b.Close
	}
	cur := closes[len(closes)-1]
	p := valuation.PercentileRank(closes, cur)

	action, conf := s.classify(p)
	if action == "" {
		return nil, nil
	}
	return []core.Signal{{
		Symbol: ctx.Symbol, Action: action, Confidence: conf,
		Price:  cur,
		Reason: fmt.Sprintf("price at %.1f%% of %d-year range", p, s.lookbackYears),
		Strategy: s.Name(), GeneratedAt: ctx.Now,
		Metadata: map[string]any{
			"percentile": p, "lookback_years": s.lookbackYears, "sample_size": len(closes),
		},
	}}, nil
}

// classify maps a percentile to (action, confidence); "" means no signal.
// Bands per design §3.1: extreme→0.8+linear(≤0.95), normal→0.6-0.8 linear.
func (s *Strategy) classify(p float64) (core.Action, float64) {
	switch {
	case p < s.extremeLow:
		return core.ActionStrongBuy, min(0.95, 0.8+0.15*(s.extremeLow-p)/s.extremeLow)
	case p < s.low:
		return core.ActionBuy, 0.6 + 0.2*(s.low-p)/(s.low-s.extremeLow)
	case p > s.extremeHigh:
		return core.ActionStrongSell, min(0.95, 0.8+0.15*(p-s.extremeHigh)/(100-s.extremeHigh))
	case p > s.high:
		return core.ActionSell, 0.6 + 0.2*(p-s.high)/(s.extremeHigh-s.high)
	}
	return "", 0
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/strategy/price_percentile/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/strategy/price_percentile/
git commit -m "feat(strategy): price_percentile strategy for all asset classes"
```

### Task 10: pe_percentile 策略

**Files:**
- Create: `internal/strategy/pe_percentile/strategy.go`
- Test: `internal/strategy/pe_percentile/strategy_test.go`

- [ ] **Step 1: 写失败测试**

```go
func peCtx(pePct float64, source string) strategy.AnalysisContext {
	return strategy.AnalysisContext{
		Symbol: "TEST", Now: time.Now(),
		Fundamental: &core.Fundamental{Symbol: "TEST", PEPercentile: pePct, Source: source},
	}
}

func TestAnalyze_PEBands(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{}) // 默认 20/80/10/90

	cases := []struct {
		pct    float64
		want   core.Action // "" = 无信号
	}{
		{5, core.ActionStrongBuy}, {15, core.ActionBuy},
		{50, ""}, {85, core.ActionSell}, {95, core.ActionStrongSell},
	}
	for _, c := range cases {
		sigs, err := s.Analyze(peCtx(c.pct, "lixinger_cvpos"))
		if err != nil {
			t.Fatal(err)
		}
		if c.want == "" && len(sigs) != 0 {
			t.Errorf("pct=%v: want no signal, got %+v", c.pct, sigs)
		}
		if c.want != "" && (len(sigs) != 1 || sigs[0].Action != c.want) {
			t.Errorf("pct=%v: want %s, got %+v", c.pct, c.want, sigs)
		}
	}
}

func TestAnalyze_MethodMetadata(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	sigs, _ := s.Analyze(peCtx(5, "lixinger_cvpos:yahoo_eps_insufficient"))
	if len(sigs) != 1 {
		t.Fatal("expected one signal")
	}
	if sigs[0].Metadata["method"] != "lixinger_cvpos" ||
		sigs[0].Metadata["fallback_reason"] != "yahoo_eps_insufficient" {
		t.Errorf("metadata = %+v", sigs[0].Metadata)
	}
}

func TestAnalyze_Unavailable(t *testing.T) {
	s := New()
	_ = s.Init(strategy.Config{})
	// Fundamental 缺失 / PEPercentile 负值 → 无信号
	if sigs, _ := s.Analyze(strategy.AnalysisContext{Symbol: "T"}); len(sigs) != 0 {
		t.Errorf("nil fundamental should yield no signal")
	}
	if sigs, _ := s.Analyze(peCtx(-1, "")); len(sigs) != 0 {
		t.Errorf("negative percentile should yield no signal")
	}
}

func TestRequiredData_AssetTypes(t *testing.T) {
	s := New()
	rd := s.RequiredData()
	if !rd.Fundamentals || len(rd.AssetTypes) != 2 { // stock + index
		t.Errorf("RequiredData = %+v", rd)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/strategy/pe_percentile/ -v`
Expected: FAIL

- [ ] **Step 3: 实现**

结构同 price_percentile（阈值默认 20/80/10/90，`classify` 逻辑相同——**不要抽公共基类，两个策略各自独立**，阈值语义将来可能分化，重复 30 行换边界清晰）。差异点：

```go
func (s *Strategy) RequiredData() strategy.DataRequirements {
	return strategy.DataRequirements{
		// PriceHistory 必须声明：buildFundamental 的 PE 重建复用 analyzeSymbol
		// 取回的 ohlcv，窗口由绑定策略的 max PriceHistory 决定——不声明则
		// 单独绑定本策略时只拿到 1 年数据，重建出"1 年 PE 分位"与 5 年文案错位
		PriceHistory: s.lookbackYears * 252,
		Fundamentals: true,
		AssetTypes:   []core.AssetType{core.AssetStock, core.AssetIndex},
	}
}

func (s *Strategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
	if ctx.Fundamental == nil || ctx.Fundamental.PEPercentile < 0 {
		return nil, nil
	}
	p := ctx.Fundamental.PEPercentile
	action, conf := s.classify(p)
	if action == "" {
		return nil, nil
	}
	method, fallbackReason, _ := strings.Cut(ctx.Fundamental.Source, ":")
	md := map[string]any{"pe_percentile": p, "method": method, "lookback_years": s.lookbackYears}
	if fallbackReason != "" {
		md["fallback_reason"] = fallbackReason
	}
	price := 0.0
	if n := len(ctx.OHLCV); n > 0 {
		price = ctx.OHLCV[n-1].Close
	}
	return []core.Signal{{
		Symbol: ctx.Symbol, Action: action, Confidence: conf, Price: price,
		Reason:   fmt.Sprintf("PE at %.1f%% of its %d-year history (%s)", p, s.lookbackYears, method),
		Strategy: s.Name(), GeneratedAt: ctx.Now, Metadata: md,
	}}, nil
}
```

- [ ] **Step 4: 运行确认通过 + 提交**

Run: `go test ./internal/strategy/... -v` → PASS

```bash
git add internal/strategy/pe_percentile/
git commit -m "feat(strategy): pe_percentile strategy for stocks and indexes"
```

### Task 11: 既有策略补 AssetTypes 声明

**Files:**
- Modify: `internal/strategy/ma_crossover/strategy.go:34-39`、`internal/strategy/pe_band/strategy.go:27-29`、`internal/strategy/dividend_yield/strategy.go:31-36`
- Test: 各自 strategy_test.go 加一个断言用例

- [ ] **Step 1: 写失败测试**（每个包一个小用例，断言 `RequiredData().AssetTypes` 非空且内容正确：ma_crossover 6 类全资产；pe_band/dividend_yield 仅 `[stock]`）

- [ ] **Step 2: 确认失败 → 实现 → 确认通过**

ma_crossover 的 RequiredData 增加 `AssetTypes: []core.AssetType{core.AssetStock, core.AssetIndex, core.AssetETF, core.AssetFund, core.AssetCommodity, core.AssetCrypto}`；pe_band 与 dividend_yield 增加 `AssetTypes: []core.AssetType{core.AssetStock}`。

Run: `go test ./internal/strategy/... -v` → PASS

- [ ] **Step 3: 提交**

```bash
git add internal/strategy/
git commit -m "feat(strategy): declare AssetTypes on existing strategies"
```

### Task 12: app 装配层 — AssetTypes 绑定校验 + 动态历史窗口

**Files:**
- Modify: `internal/app/app.go`（analyzeSymbol :300-337、新增 effectiveStrategies / historyWindow 方法）
- Test: `internal/app/app_test.go`

- [ ] **Step 1: 写失败测试**

```go
// fakeStrategy 实现 strategy.Strategy，可配 AssetTypes 与 PriceHistory
func TestEffectiveStrategies_FiltersByAssetType(t *testing.T) {
	a := New(...) // 按 app_test.go 现有构造方式
	a.strategies.Register(&fakeStrategy{name: "stock_only",
		assetTypes: []core.AssetType{core.AssetStock}})
	a.strategies.Register(&fakeStrategy{name: "all_assets"}) // AssetTypes 空 = 不限

	item := WatchlistItem{Symbol: "GC=F", Type: TypeFuture,
		Strategies: []string{"stock_only", "all_assets"}}
	got := a.effectiveStrategies(item)
	if len(got) != 1 || got[0] != "all_assets" {
		t.Errorf("effectiveStrategies = %v, want [all_assets]", got)
	}
	// 二次调用不重复告警（warnedBindings 去重）——通过 zap observer 或计数 stub 断言仅 1 条 warning
}

func TestHistoryWindowDays(t *testing.T) {
	a := New(...)
	a.strategies.Register(&fakeStrategy{name: "pp", priceHistory: 5 * 252})
	item := WatchlistItem{Symbol: "AAPL", Type: TypeStock, Strategies: []string{"pp"}}
	if d := a.historyWindowDays(item); d < 1825 { // 5y 交易日 → ≥5y 自然日
		t.Errorf("historyWindowDays = %d, want >= 1825", d)
	}
	// 无策略声明时回退 365（现状兼容）
	if d := a.historyWindowDays(WatchlistItem{Symbol: "X"}); d != 365 {
		t.Errorf("default window = %d, want 365", d)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run 'TestEffectiveStrategies|TestHistoryWindow' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

```go
// effectiveStrategies returns the item's bound strategies minus those whose
// AssetTypes declaration excludes the item's asset type. Mismatches are
// logged once per (symbol,strategy) pair (design §3.3).
func (a *App) effectiveStrategies(item WatchlistItem) []string {
	at := assetTypeOf(item.Type)
	out := make([]string, 0, len(item.Strategies))
	for _, name := range item.Strategies {
		s, ok := a.strategies.Get(name)
		if !ok {
			out = append(out, name) // 未注册的留给 engine 报错路径
			continue
		}
		decl := s.RequiredData().AssetTypes
		if len(decl) == 0 || at != "" && contains(decl, at) {
			out = append(out, name)
			continue
		}
		a.warnOnce("binding:"+item.Symbol+":"+name,
			"strategy skipped: asset type not supported",
			zap.String("symbol", item.Symbol), zap.String("strategy", name),
			zap.String("asset_type", item.Type))
	}
	return out
}

// historyWindowDays converts the max PriceHistory (trading days) demanded by
// the item's strategies into calendar days, with the legacy 365 floor.
func (a *App) historyWindowDays(item WatchlistItem) int {
	maxBars := 0
	for _, name := range item.Strategies {
		if s, ok := a.strategies.Get(name); ok {
			if ph := s.RequiredData().PriceHistory; ph > maxBars {
				maxBars = ph
			}
		}
	}
	if maxBars <= 250 {
		return 365
	}
	// 交易日→自然日：真实折算系数 365/252 ≈ 1.448（×7/5 只折周末、漏节假日，
	// 对 5×252=1260 bars 会算出 1794 < 1825，不满足 5 年覆盖）
	return maxBars*365/252 + 30
}
```

`warnOnce`：App 增加 `warned sync.Map`，`LoadOrStore` 去重后 `logger.Warn`。

`effectiveStrategies` 内同步实现**表外指数 warning**（设计 §2.3，Chunk 1 Task 5 仅导出 `KnownIndexMarket` 查询，告警职责在此）：

```go
	if strings.HasPrefix(item.Symbol, "^") {
		if _, known := collector.KnownIndexMarket(item.Symbol); !known {
			a.warnOnce("unknown-index:"+item.Symbol,
				"index symbol outside phase-1 list, market defaults to US",
				zap.String("symbol", item.Symbol))
		}
	}
```

补充说明（行为边界，非遗漏）：未绑定 strategies 的 watchlist 项走 `Analyze`（全策略）路径，历史窗口回退 365 天（约 250 bars < 252 门槛），price_percentile 自然不出信号，AssetTypes 过滤也不介入——设计 §3.3 只约束"绑定"，此为既有行为，保持现状。

`analyzeSymbol` 改动两处：
1. `start := end.AddDate(0, 0, -a.historyWindowDays(item))`
2. 策略选择改用 `effective := a.effectiveStrategies(item)`，`len(effective) > 0` 时走 `AnalyzeWithStrategies(ctx, analysisCtx, effective)`；item.Strategies 非空但 effective 为空 → 直接 return（全部被过滤）

- [ ] **Step 4: 运行确认通过 + 提交**

Run: `go test ./internal/app/ -v` → PASS

```bash
git add internal/app/
git commit -m "feat(app): asset-type binding validation and dynamic history window"
```

### Task 13: app 装配层 — 估值分位编排（含兜底链路）

**Files:**
- Modify: `internal/app/app.go`（analyzeSymbol 的 AnalysisContext 组装 :332-337；App 结构增加 lixinger/yahoo 引用的注入点）
- Test: `internal/app/app_test.go`

依赖注入说明：App 通过两个窄接口持有估值数据源（避免依赖具体包），serve.go 装配时注入：

```go
type ValuationSource interface { // lixinger
	FetchValuationPercentile(symbol string, lookbackYears int) (float64, error)
}
type EPSSource interface { // yahoo
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}
// App 字段: valuationSrc ValuationSource; epsSrc EPSSource
// 注入: app.SetValuationSources(lx, yh)（nil 容忍：缺哪个对应路径不可用）
```

- [ ] **Step 1: 写失败测试（兜底链路编排，设计 §6 装配层）**

```go
func TestBuildPEPercentile_Paths(t *testing.T) {
	cases := []struct {
		name       string
		symbol     string
		market     core.Market
		eps        []core.EPSPoint // epsSrc stub 返回值
		epsErr     error
		valPct     float64 // valuationSrc stub 返回值
		valErr     error
		wantPct    bool   // PEPercentile >= 0
		wantSource string // 前缀匹配
	}{
		{"A股走理杏仁", "600519.SH", core.MarketCNA, nil, nil, 23.4, nil, true, "lixinger_cvpos"},
		{"美股主路径重建", "AAPL", core.MarketUS, validEPS8(), nil, 0, errors.New("unused"), true, "reconstructed"},
		{"美股EPS不足→兜底成功", "AAPL", core.MarketUS, nil, nil, 41.2, nil, true, "lixinger_cvpos:"},
		{"美股EPS不足→兜底也失败", "AAPL", core.MarketUS, nil, nil, -1, errors.New("no permission"), false, ""},
		{"美股真实亏损→直接跳过不兜底", "LOSS", core.MarketUS, lossEPS(), nil, 99, nil, false, ""},
		{"美/港指数走理杏仁", "^GSPC", core.MarketUS, nil, nil, 88.0, nil, true, "lixinger_cvpos"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := New(...)
			sv := &stubVal{pct: c.valPct, err: c.valErr} // 指针：亏损用例后续断言 sv.calls == 0
			a.SetValuationSources(sv, &stubEPS{pts: c.eps, err: c.epsErr})
			f := a.buildFundamental(c.symbol, TypeOfCase(c), sampleCloses(700))
			if c.wantPct != (f != nil && f.PEPercentile >= 0) {
				t.Fatalf("availability mismatch: %+v", f)
			}
			if c.wantPct && !strings.HasPrefix(f.Source, c.wantSource) {
				t.Errorf("Source = %q, want prefix %q", f.Source, c.wantSource)
			}
		})
	}
	// 关键断言：亏损用例中 stubVal 不能被调用过（不兜底）——stub 加调用计数
}
```

测试 helper 定义（写在 app_test.go，日期对齐是 load-bearing 约束）：

```go
// stubVal/stubEPS 带调用计数，亏损用例断言 stubVal.calls == 0
type stubVal struct {
	pct   float64
	err   error
	calls int
}
func (s *stubVal) FetchValuationPercentile(string, int) (float64, error) {
	s.calls++
	return s.pct, s.err
}

type stubEPS struct {
	pts []core.EPSPoint
	err error
}
func (s *stubEPS) FetchEPSHistory(string, time.Time, time.Time) ([]core.EPSPoint, error) {
	return s.pts, s.err
}

// 基准日期：EPS 点必须早于全部收盘 bar，否则阶梯对齐找不到点，
// PE 序列为空 → ErrInsufficientEPS，"主路径重建"用例会以费解方式失败。
var epsBase = time.Now().AddDate(-3, 0, 0)

// validEPS8: 8 个正 EPS 季度点（满足 MinEPSPoints），起点 epsBase
func validEPS8() []core.EPSPoint { /* quarterly from epsBase, EPS=4..5 */ }

// lossEPS: 8 个正点 + 最末 1 个负点（当前 EPS ≤ 0 → ErrNonPositiveEPS）
func lossEPS() []core.EPSPoint { /* same + final EPS=-1 */ }

// sampleCloses(n): n 根日线，起点 epsBase.AddDate(0,1,0)（晚于首个 EPS 点）
func sampleCloses(n int) []core.OHLCV { /* close=100+i%50 */ }

// TypeOfCase: case → app 中文类型标签（^ 前缀 → TypeIndex，其余 → TypeStock）
func TypeOfCase(c testCase) string { ... }
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run TestBuildPEPercentile -v` → FAIL

- [ ] **Step 3: 实现 buildFundamental**

```go
// buildFundamental assembles Fundamental.PEPercentile for one watchlist item
// when any bound strategy needs fundamentals. Returns nil when the item's
// asset class has no valuation path (commodity/crypto/fund).
// Path table per design §3.2:
//   CN stock/index      -> lixinger cvpos (no fallback chain: lixinger IS the source)
//   US/HK index         -> lixinger cvpos (sole path)
//   US/HK stock         -> yahoo EPS reconstruction; on missing-data errors
//                          fall back to lixinger; on ErrNonPositiveEPS skip
//                          entirely (loss-making, fallback would mislead)
func (a *App) buildFundamental(symbol, appType string, ohlcv []core.OHLCV) *core.Fundamental {
	at := assetTypeOf(appType)
	if at != core.AssetStock && at != core.AssetIndex {
		return nil
	}
	market := collector.MarketForSymbol(symbol)
	f := &core.Fundamental{Symbol: symbol, Market: market, Date: time.Now(), PEPercentile: -1}

	const lookback = 5 // 一期固定 5 年，与策略默认一致；后续可由策略参数下沉

	switch {
	case market == core.MarketCNA, at == core.AssetIndex:
		// A 股（股票+指数）与美/港指数：理杏仁唯一路径
		if a.valuationSrc == nil {
			a.warnOnce("lixinger:"+symbol, "valuation percentile unavailable: lixinger not configured",
				zap.String("symbol", symbol))
			return f
		}
		pct, err := a.valuationSrc.FetchValuationPercentile(symbol, lookback)
		if err != nil {
			a.warnOnce("lixinger:"+symbol, "valuation percentile fetch failed",
				zap.String("symbol", symbol), zap.Error(err))
			return f
		}
		f.PEPercentile, f.Source = pct, "lixinger_cvpos"
		return f

	default:
		// 美/港个股：Yahoo EPS 重建主路径。
		// epsSrc 未配置（yahoo 未启用）也算"主路径不可用·数据缺失"，
		// 直接进理杏仁兜底（设计 §5 口径），fallback_reason="yahoo_not_configured"
		if a.epsSrc == nil {
			if a.valuationSrc != nil {
				if pct, ferr := a.valuationSrc.FetchValuationPercentile(symbol, lookback); ferr == nil {
					f.PEPercentile, f.Source = pct, "lixinger_cvpos:yahoo_not_configured"
					return f
				}
			}
			a.warnOnce("pepct:"+symbol, "pe percentile unavailable: no eps source and no fallback",
				zap.String("symbol", symbol))
			return f
		}
		end := time.Now()
		eps, err := a.epsSrc.FetchEPSHistory(symbol, end.AddDate(-lookback, 0, -90), end)
		if err == nil {
			pct, rerr := valuation.ReconstructPEPercentile(ohlcv, eps)
			switch {
			case rerr == nil:
				f.PEPercentile, f.Source = pct, "reconstructed"
				return f
			case errors.Is(rerr, valuation.ErrNonPositiveEPS):
				return f // 真实亏损：直接不可用，不兜底（设计 §3.2/§5）
			}
			// ErrInsufficientEPS → 落入兜底
			err = rerr
		}
		// 数据缺失类失败 → 理杏仁兜底
		if a.valuationSrc != nil {
			if pct, ferr := a.valuationSrc.FetchValuationPercentile(symbol, lookback); ferr == nil {
				f.PEPercentile = pct
				f.Source = "lixinger_cvpos:" + fallbackReason(err)
				return f
			}
		}
		a.warnOnce("pepct:"+symbol, "pe percentile unavailable (primary and fallback failed)",
			zap.String("symbol", symbol), zap.Error(err))
		return f
	}
}
```

（`fallbackReason(err)`：`errors.Is(err, valuation.ErrInsufficientEPS)` → `"yahoo_eps_insufficient"`，否则 `"yahoo_eps_error"`。）

`analyzeSymbol` 组装处：当 `effective` 中任一策略 `RequiredData().Fundamentals` 为 true 时，`analysisCtx.Fundamental = a.buildFundamental(symbol, item.Type, ohlcv)`；同时设置 `analysisCtx.Market = collector.MarketForSymbol(symbol)`。

- [ ] **Step 4: 运行确认通过 + 提交**

Run: `go test ./internal/app/ -v` → PASS

```bash
git add internal/app/
git commit -m "feat(app): PE percentile orchestration with lixinger fallback chain"
```

### Task 14: 配置与 cmd 装配 + 回测冒烟

**Files:**
- Modify: `cmd/atlas/serve.go`（:142 附近策略注册块）、`cmd/atlas/backtest.go`（:80）
- Modify: `configs/config.example.yaml`
- Test: 手工验证 + 回测冒烟

- [ ] **Step 1: serve.go 注册两个新策略**

参照 ma_crossover 注册块写法：读取 `strategies.price_percentile` / `strategies.pe_percentile` 配置（enabled + params），`Init` 后 `strategies.Register(...)`。

注入估值数据源时必须规避 **typed-nil 接口陷阱**：serve.go 现状 `lixingerCollector` 是 `*lixinger.Lixinger`（:99，可能保持 nil 指针），直接传参会让接口非 nil 但内部指针为 nil，`buildFundamental` 的 nil 防护失效。同时 `yahooCollector` 声明在 :94 的 if 块内，作用域不可达，需把声明提升到函数级。正确写法：

```go
	var vs app.ValuationSource
	if lixingerCollector != nil {
		vs = lixingerCollector
	}
	var es app.EPSSource
	if yahooCollector != nil {
		es = yahooCollector
	}
	application.SetValuationSources(vs, es) // serve.go 中 App 实例变量名为 application
```

- [ ] **Step 2: backtest.go 注册 price_percentile（冒烟）**

```go
engine.Register(price_percentile.New())
```

（pe_percentile 依赖在线估值分位，回测引擎无该数据，不注册——在代码注释中说明。）

- [ ] **Step 3: 更新 config.example.yaml**

strategies 块追加（并**删除 pe_band 下从未生效的 `lookback_years`/`threshold_percentile` 两行**，design §3.2）：

```yaml
  price_percentile:
    enabled: true
    params: {lookback_years: 5, low: 25, high: 75, extreme_low: 10, extreme_high: 90}
  pe_percentile:
    enabled: true
    params: {lookback_years: 5, low: 20, high: 80, extreme_low: 10, extreme_high: 90}
```

watchlist 示例追加（design §7）：

```yaml
  - {symbol: "^GSPC",     name: "标普500",   type: "指数", strategies: [price_percentile, pe_percentile]}
  - {symbol: "GC=F",      name: "COMEX黄金", type: "期货", strategies: [price_percentile]}
  - {symbol: "BTC-USDT",  name: "比特币",    type: "加密货币", strategies: [price_percentile]}
```

（type 值用 app 层中文标签，与 `config.WatchlistItem.Type` 注释一致。）

- [ ] **Step 4: 编译 + 回测冒烟 + 全量测试**

```bash
go build ./...
go test ./... 2>&1 | tail -20        # 全部 PASS
# 回测冒烟：区间必须 > 252 个交易日（minSampleBars 门槛），否则 0 信号也"通过"，无判定力
go run ./cmd/atlas backtest price_percentile --symbol AAPL \
  --from 2021-01-01 --to 2026-06-01   # 策略名是位置参数（backtest.go: Use: "backtest [strategy]"）
# 预期：运行无错退出；窗口滚过 252 bars 后允许产生信号；0 trades 可接受但需人工
# 确认日志中出现过 percentile 计算（非因数据不足直接全程跳过）。
# （flag 名以 backtest.go 实际定义为准，执行时核对 --help）
```

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/ configs/config.example.yaml
git commit -m "feat(cmd): wire percentile strategies into serve/backtest with config"
```

### Task 15: 收尾 — 理杏仁指数代码核对 + 简化 + 文档

- [ ] **Step 1: 核对理杏仁指数代码**（需要 LIXINGER_API_KEY；无 key 时跳过并在提交信息注明）

用 `hk/index`、`us/index` 的 basic-info/samples 接口核对 `usHKIndexCodes` 中 SPX/COMP/DJI/HSI 四个候选代码，修正并固化（design §2.4）。

- [ ] **Step 2: 运行 code-simplifier**（全局规范，提交前必跑）

Task tool: `subagent_type: "code-simplifier:code-simplifier"`，prompt 列出本计划全部新增/修改文件。

- [ ] **Step 3: gitnexus_detect_changes / 全量回归**

```bash
npx gitnexus analyze && go vet ./... && go test ./...
```

确认变更只影响预期符号与执行流（项目规范）。

- [ ] **Step 4: README 功能清单补一行**（Multiple Strategies 行追加 Price/PE Percentile；Multi-Market 行追加 indexes & commodities）

- [ ] **Step 5: 最终提交**

```bash
git add -A
git commit -m "feat: index/commodity collection and historical percentile strategies

Implements docs/plans/2026-06-11-index-commodity-percentile-design.md (rev6)"
```

---

## 验收对照（design §1.2 / §6）

- [ ] `^GSPC`/`GC=F` 可加入 watchlist 并产生 price_percentile 信号（手工 serve 验证或集成测试）
- [ ] `000300.SH` 走 eastmoney 指数 secid，pe_percentile 走理杏仁 cn/index
- [ ] 美股个股 pe_percentile：主路径重建 → 兜底 → 双失败不出信号，三态均有测试
- [ ] 亏损股（EPS≤0）不出 PE 信号且不触发兜底（测试断言 stub 未被调用）
- [ ] `GC=F` 绑定 pe_percentile 时启动 warning + 跳过，不崩溃
- [ ] 全量 `go test ./...` 通过，无既有用例回归
