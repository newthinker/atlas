# 港股 qlib 数据包扩展（atlas_hk）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把港股股票/ETF/指数纳入 config watchlist，经 yahoo 采集 OHLCV，构建独立于 atlas_cn 的 qlib 数据包 `atlas_hk`，并产出港股 watchlist 分析报告。

**Architecture:** export-ohlcv 增加 `--market cn|hk` 参数，按 market 选择 watchlist 子集、基准与 qlib instrument 命名；港股因交易日历不同单独 dump 成 atlas_hk；行情全部走 yahoo（.HK 股票/ETF + ^HSI/^HSCE 指数）。

**Tech Stack:** Go (cobra CLI), Python (qlib/dump_bin), Make。

设计依据：`docs/superpowers/specs/2026-06-13-hk-qlib-bundle-design.md`。

已验证事实（实测）：
- yahoo 可取 `.HK` 股票/ETF（0700/2800/2828/3033/3181.HK）+ `^HSI`(24718)/`^HSCE`(8374)；`^HSTECH` 404（不纳入，3033.HK 代理）。
- `SelectForSymbol("^HSI")` 经 `isIndexSymbol` 分支 → yahoo；`.HK` → 默认分支 → yahoo。yahoo `validSymbol` 正则接受 `^HSI`/`.HK`。**OHLCV 路由已就绪，无需改 selector。**
- Go `fmt.Sprintf("%05s", "0700")` == `"00700"`、`"2800"` == `"02800"`（确实零填充）。

---

## File Structure

| 文件 | 改动 |
|---|---|
| `cmd/atlas/export_ohlcv.go` | `toQlibInstrument` 增 .HK/^HSI/^HSCE；新增 `benchmarkForMarket`/`inMarket`；`resolveOHLCVSymbols`/`requireBenchmark` 加 market 参数；`exportOHLCVParams` 加 `Benchmark`；`executeExportOHLCV` 用 `p.Benchmark`；CLI 加 `--market` flag |
| `cmd/atlas/export_ohlcv_test.go` | HK 命名契约样本；HK market 选择/基准用例；更新既有 resolve 调用签名 |
| `scripts/qlib_eval/qlib_eval/symbols.py` | `to_qlib_instrument` 镜像 HK 命名 |
| `scripts/qlib_eval/tests/test_symbols.py` | HK 契约样本 |
| `configs/config.yaml` | watchlist 加 4 ETF + 2 指数 |
| `Makefile` | 新增 `qlib-data-hk` target |
| `scripts/qlib_eval/analyze_watchlist.py` | 指数识别加 HK(HSI/HSCEI) + CSI 前缀 |

每个任务结束 `go build ./...` 通过、相关测试全绿。

---

### Task 1: toQlibInstrument 支持 .HK / ^HSI / ^HSCE（Go）

**Files:**
- Modify: `cmd/atlas/export_ohlcv.go`（`toQlibInstrument`）
- Modify: `cmd/atlas/export_ohlcv_test.go`（`TestToQlibInstrument_Contract`）

- [ ] **Step 1: 加 HK 契约用例（先 RED）**

在 `cmd/atlas/export_ohlcv_test.go` 的 `TestToQlibInstrument_Contract` 的 `cases` 切片里，`{"930713.CSI", "CSI930713"}` 之后加：

```go
		{"0700.HK", "HK00700"},   // 港股股票：HK + 5 位补零
		{"2800.HK", "HK02800"},   // 港股 ETF
		{"^HSI", "HSI"},          // 恒生指数
		{"^HSCE", "HSCEI"},       // 国企指数（HSCEI）
```

并确认 `^HSTECH` 仍被拒绝：在同函数的 `for _, bad := range []string{...}` 列表里加 `"^HSTECH"`：

```go
	for _, bad := range []string{"AAPL", "^GSPC", "GC=F", "BTC-USDT", "0700.HK.X", "^HSTECH"} {
```

> 注意：原列表含 `"0700.HK"` 作为「非 A 股拒绝」样本——本任务起 `.HK` 变为合法，必须从 bad 列表移除 `"0700.HK"`（上面已用 `"0700.HK.X"` 占位一个仍非法的样本）。

- [ ] **Step 2: 运行确认 RED**

Run: `go test ./cmd/atlas/ -run TestToQlibInstrument_Contract -count=1`
Expected: FAIL（`.HK`/`^HSI`/`^HSCE` 当前返回 error）。

- [ ] **Step 3: 实现 toQlibInstrument 的 HK 分支**

把 `cmd/atlas/export_ohlcv.go` 的 `toQlibInstrument` 替换为：

```go
// toQlibInstrument mirrors scripts/qlib_eval/qlib_eval/symbols.py
// to_qlib_instrument — keep the two in sync (the contract test shares samples).
// A-share: 600519.SH->SH600519, 399001.SZ->SZ399001, 930713.CSI->CSI930713.
// HK: 0700.HK->HK00700 (HK + 5位补零), 2800.HK->HK02800; index ^HSI->HSI,
// ^HSCE->HSCEI. Every other symbol is rejected.
func toQlibInstrument(symbol string) (string, error) {
	switch {
	case strings.HasSuffix(symbol, ".SH"):
		return "SH" + strings.TrimSuffix(symbol, ".SH"), nil
	case strings.HasSuffix(symbol, ".SZ"):
		return "SZ" + strings.TrimSuffix(symbol, ".SZ"), nil
	case strings.HasSuffix(symbol, ".CSI"):
		return "CSI" + strings.TrimSuffix(symbol, ".CSI"), nil
	case strings.HasSuffix(symbol, ".HK"):
		return "HK" + fmt.Sprintf("%05s", strings.TrimSuffix(symbol, ".HK")), nil
	case symbol == "^HSI":
		return "HSI", nil
	case symbol == "^HSCE":
		return "HSCEI", nil
	}
	return "", fmt.Errorf("not a supported A-share/HK symbol: %s", symbol)
}
```

- [ ] **Step 4: 运行确认 PASS**

Run: `go test ./cmd/atlas/ -run TestToQlibInstrument_Contract -count=1 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/export_ohlcv.go cmd/atlas/export_ohlcv_test.go
git commit -m "feat(qlib): toQlibInstrument supports .HK/^HSI/^HSCE"
```

---

### Task 2: to_qlib_instrument 支持 HK（Python，与 Go 对称）

**Files:**
- Modify: `scripts/qlib_eval/qlib_eval/symbols.py`
- Modify: `scripts/qlib_eval/tests/test_symbols.py`

- [ ] **Step 1: 加 HK 契约用例（先 RED）**

在 `scripts/qlib_eval/tests/test_symbols.py` 的 `test_to_qlib_instrument` 末尾（`930713.CSI` 断言之后）加：

```python
    assert to_qlib_instrument("0700.HK") == "HK00700"   # 港股股票
    assert to_qlib_instrument("2800.HK") == "HK02800"   # 港股 ETF
    assert to_qlib_instrument("^HSI") == "HSI"           # 恒生指数
    assert to_qlib_instrument("^HSCE") == "HSCEI"        # 国企指数
```

并在 `test_to_qlib_instrument_rejects_non_ashare` 的 bad 元组中：移除 `"0700.HK"`（现已合法）、加入 `"^HSTECH"`：

```python
    for bad in ("AAPL", "^GSPC", "GC=F", "BTC-USDT", "^HSTECH"):
```

- [ ] **Step 2: 运行确认 RED**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_symbols.py -q`
Expected: FAIL（HK 断言报 ValueError）。

- [ ] **Step 3: 实现 symbols.py 的 HK 分支**

把 `scripts/qlib_eval/qlib_eval/symbols.py` 的 `to_qlib_instrument` 函数体替换为：

```python
def to_qlib_instrument(symbol: str) -> str:
    """A 股: 600519.SH->SH600519、399001.SZ->SZ399001、930713.CSI->CSI930713。
    港股: 0700.HK->HK00700（HK + 5 位补零）、2800.HK->HK02800；指数 ^HSI->HSI、
    ^HSCE->HSCEI。其余（美股 AAPL、^GSPC、^HSTECH、GC=F、BTC-USDT 等）raise ValueError。
    """
    if symbol.endswith(".SH"):
        return "SH" + symbol[:-3]
    if symbol.endswith(".SZ"):
        return "SZ" + symbol[:-3]
    if symbol.endswith(".CSI"):
        return "CSI" + symbol[:-4]
    if symbol.endswith(".HK"):
        return "HK" + symbol[:-3].zfill(5)
    if symbol == "^HSI":
        return "HSI"
    if symbol == "^HSCE":
        return "HSCEI"
    raise ValueError(f"not a supported A-share/HK symbol: {symbol!r}")
```

- [ ] **Step 4: 运行确认 PASS**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_symbols.py -q`
Expected: 2 passed。

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_eval/qlib_eval/symbols.py scripts/qlib_eval/tests/test_symbols.py
git commit -m "feat(qlib): python to_qlib_instrument supports HK (symmetric with Go)"
```

---

### Task 3: export-ohlcv market 参数化（基准 + 选择 + --market）

**Files:**
- Modify: `cmd/atlas/export_ohlcv.go`
- Modify: `cmd/atlas/export_ohlcv_test.go`

- [ ] **Step 1: 写 market 选择/基准测试（先 RED）**

在 `cmd/atlas/export_ohlcv_test.go` 末尾追加：

```go
func TestResolveOHLCVSymbols_HKMarket(t *testing.T) {
	wl := []config.WatchlistItem{
		{Symbol: "0700.HK"}, {Symbol: "2800.HK"}, {Symbol: "^HSI"},
		{Symbol: "^HSCE"}, {Symbol: "600519.SH"}, // A股应被 hk market 排除
	}
	got, err := resolveOHLCVSymbols(nil, wl, "hk")
	if err != nil {
		t.Fatalf("hk resolve: %v", err)
	}
	want := map[string]bool{"0700.HK": true, "2800.HK": true, "^HSI": true, "^HSCE": true}
	for _, s := range got {
		if s == "600519.SH" {
			t.Error("A-share must be excluded from hk market set")
		}
		delete(want, s)
	}
	if len(want) != 0 {
		t.Errorf("hk set missing symbols: %v (got %v)", want, got)
	}
	// 基准 ^HSI 必须在内
	if !sliceContains(got, "^HSI") {
		t.Errorf("hk set must include benchmark ^HSI, got %v", got)
	}
}

func TestBenchmarkForMarket(t *testing.T) {
	if benchmarkForMarket("cn") != "000300.SH" {
		t.Errorf("cn benchmark = %q, want 000300.SH", benchmarkForMarket("cn"))
	}
	if benchmarkForMarket("hk") != "^HSI" {
		t.Errorf("hk benchmark = %q, want ^HSI", benchmarkForMarket("hk"))
	}
}

func sliceContains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}
```

并把既有的 `TestResolveOHLCVSymbols_Default`（若存在）中对 `resolveOHLCVSymbols(...)` 的两参调用改为三参，补 `"cn"`。用 grep 定位：

Run: `grep -n "resolveOHLCVSymbols(" cmd/atlas/export_ohlcv_test.go`
把每个调用 `resolveOHLCVSymbols(x, y)` 改为 `resolveOHLCVSymbols(x, y, "cn")`。

- [ ] **Step 2: 运行确认 RED（编译失败：函数签名/未定义）**

Run: `go test ./cmd/atlas/ -run 'TestResolveOHLCVSymbols|TestBenchmarkForMarket' -count=1`
Expected: 编译失败（`benchmarkForMarket` 未定义、`resolveOHLCVSymbols` 参数不符）。

- [ ] **Step 3: 实现 market 化（benchmarkForMarket / inMarket / 改 resolve+require+execute+CLI）**

在 `cmd/atlas/export_ohlcv.go` 中，`const benchmarkSymbol = "000300.SH"` 之后新增：

```go
// benchmarkForMarket returns the qlib-bundle benchmark per market: A-share uses
// CSI 300 (000300.SH), HK uses the Hang Seng Index (^HSI). The benchmark must
// fetch successfully or the export is fatal —评估无基准无意义。
func benchmarkForMarket(market string) string {
	if market == "hk" {
		return "^HSI"
	}
	return benchmarkSymbol
}

// inMarket reports whether a watchlist symbol belongs to a market's qlib bundle.
// HK = .HK securities (stocks/ETF/REIT) + the two supported HK indexes; CN =
// .SH/.SZ equities and .SH/.SZ/.CSI indexes.
func inMarket(symbol, market string) bool {
	if market == "hk" {
		return strings.HasSuffix(symbol, ".HK") || symbol == "^HSI" || symbol == "^HSCE"
	}
	return strings.HasSuffix(symbol, ".SH") ||
		strings.HasSuffix(symbol, ".SZ") ||
		strings.HasSuffix(symbol, ".CSI")
}
```

把 `resolveOHLCVSymbols` 替换为（加 `market` 参数、用 `inMarket`/`benchmarkForMarket`）：

```go
// resolveOHLCVSymbols picks the symbol set to export for a market. An explicit
// --symbols flag wins as-is. Otherwise the set is the watchlist symbols of that
// market plus the market benchmark, order-preserved and de-duplicated. An empty
// derived set is an error — never silently degrade to benchmark-only.
func resolveOHLCVSymbols(flag []string, watchlist []config.WatchlistItem, market string) ([]string, error) {
	if len(flag) > 0 {
		return flag, nil
	}
	var picks []string
	for _, item := range watchlist {
		if inMarket(item.Symbol, market) {
			picks = append(picks, item.Symbol)
		}
	}
	if len(picks) == 0 {
		return nil, fmt.Errorf("no %s-market symbols in watchlist and no --symbols provided", market)
	}
	result := make([]string, 0, len(picks)+1)
	seen := make(map[string]bool)
	for _, s := range append(picks, benchmarkForMarket(market)) {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result, nil
}
```

把 `requireBenchmark` 替换为（加 market 参数）：

```go
// requireBenchmark enforces, at the CLI layer, that the resolved symbol set
// includes the market benchmark — without it strategy evaluation is meaningless.
func requireBenchmark(symbols []string, market string) error {
	bench := benchmarkForMarket(market)
	if slices.Contains(symbols, bench) {
		return nil
	}
	return fmt.Errorf("symbol set must include benchmark %s for evaluation to be meaningful", bench)
}
```

给 `exportOHLCVParams` 加 `Benchmark` 字段：

```go
type exportOHLCVParams struct {
	Symbols   []string
	From, To  string
	OutDir    string
	Benchmark string // 该 market 的基准；空则回退 benchmarkSymbol（cn）
}
```

在 `executeExportOHLCV` 内，把使用 `benchmarkSymbol` 的那一处（`if symbol == benchmarkSymbol {`）改为使用 params 的基准。在函数顶部（`from, err := ...` 之前）加：

```go
	benchmark := p.Benchmark
	if benchmark == "" {
		benchmark = benchmarkSymbol
	}
```

并把 `if symbol == benchmarkSymbol {` 改为 `if symbol == benchmark {`。

新增 CLI flag 变量与注册：在 `var ( exportOHLCVSymbols ... )` 块加 `exportOHLCVMarket string`；在 `init()` 里 `--out-dir` 注册之后加：

```go
	exportOHLCVCmd.Flags().StringVar(&exportOHLCVMarket, "market", "cn",
		"Market bundle: cn (A-share) or hk (Hong Kong)")
```

改 `runExportOHLCV`：把 `resolveOHLCVSymbols(exportOHLCVSymbols, cfg.Watchlist)` 改为
`resolveOHLCVSymbols(exportOHLCVSymbols, cfg.Watchlist, exportOHLCVMarket)`；把
`requireBenchmark(symbols)` 改为 `requireBenchmark(symbols, exportOHLCVMarket)`；在构造
`exportOHLCVParams{...}` 时加 `Benchmark: benchmarkForMarket(exportOHLCVMarket),`。

- [ ] **Step 4: 运行确认 PASS（含既有用例不回归）**

Run: `go test ./cmd/atlas/ -count=1`
Expected: 全 PASS（新 HK 用例 + 既有 cn 用例）。若 `TestResolveOHLCVSymbols_Default` 等仍报参数不符，按 Step 1 补 `"cn"`。

- [ ] **Step 5: 确认全仓编译**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 6: 提交**

```bash
git add cmd/atlas/export_ohlcv.go cmd/atlas/export_ohlcv_test.go
git commit -m "feat(qlib): market-parameterized export-ohlcv (cn/hk benchmark + selection)"
```

---

### Task 4: config.yaml watchlist 加港股 ETF + 指数

**Files:**
- Modify: `configs/config.yaml`

- [ ] **Step 1: 在 watchlist 的港股个股块之后追加 ETF 与指数**

在 `configs/config.yaml` 的 `# ---------- 港股个股 ----------` 那 5 行之后，追加：

```yaml
  # ---------- 港股基金（场内 ETF/REIT，行情走 yahoo）----------
  - {symbol: "2800.HK", name: "盈富基金",       type: "基金", strategies: [price_percentile]}
  - {symbol: "2828.HK", name: "恒生中国企业",   type: "基金", strategies: [price_percentile]}
  - {symbol: "3033.HK", name: "恒生科技ETF",    type: "基金", strategies: [price_percentile]}
  - {symbol: "3181.HK", name: "AI行业指数基金", type: "基金", strategies: [price_percentile]}

  # ---------- 港股指数（行情走 yahoo ^ 代码；HSTECH 无 yahoo 数据，由 3033.HK 代理）----------
  - {symbol: "^HSI",  name: "恒生指数",   type: "指数", strategies: [price_percentile]}
  - {symbol: "^HSCE", name: "国企指数",   type: "指数", strategies: [price_percentile]}
```

- [ ] **Step 2: 验证 config 可加载 + watchlist 含新标的**

Run:
```bash
go build -o bin/atlas ./cmd/atlas
./bin/atlas export-ohlcv --config configs/config.yaml --market hk --from 2026-06-01 --to 2026-06-10 --out-dir /tmp/hktest 2>&1 | grep -vE "FetchHistory failed|trying lixinger" | tail -5
ls /tmp/hktest/
```
Expected: 生成 `hk00700.csv hk02800.csv hk02828.csv hk03033.csv hk03181.csv hk03288.csv hk06886.csv hk09988.csv hk00883.csv hsi.csv hscei.csv`（11 个：5 股票 + 4 ETF + 2 指数，^HSI 即基准）。退出码 0。
（需网络；本环境 yahoo 可达。）清理：`rm -rf /tmp/hktest`

- [ ] **Step 3: 提交**

```bash
git add configs/config.yaml
git commit -m "feat(config): add HK ETFs and indexes to watchlist"
```

---

### Task 5: Makefile 新增 qlib-data-hk target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 加 HK 变量与 target**

在 `Makefile` 中 `QLIB_DATA_DIR ?= $(HOME)/.qlib/qlib_data/atlas_cn` 之后加：

```makefile
QLIB_CSV_HK_DIR  ?= qlib_csv_hk
QLIB_DATA_HK_DIR ?= $(HOME)/.qlib/qlib_data/atlas_hk
```

在 `.PHONY` 行追加 `qlib-data-hk`，并在 `qlib-data` target 之后加：

```makefile
# 港股自建 qlib 数据包：watchlist 港股集（.HK + ^HSI/^HSCE）→ atlas_hk（独立日历）。
# 需 --config 提供 watchlist；港股行情走 yahoo，基准 ^HSI。
qlib-data-hk: build
	./bin/atlas export-ohlcv --config configs/config.yaml --market hk \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_HK_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_HK_DIR) \
	  --target-dir $(QLIB_DATA_HK_DIR)
```

- [ ] **Step 2: 校验 Makefile 语法（dry-run）**

Run: `make -n qlib-data-hk`
Expected: 打印将执行的命令，无 "missing separator" 等语法错误。

- [ ] **Step 3: 提交**

```bash
git add Makefile
git commit -m "build(qlib): add qlib-data-hk target for atlas_hk bundle"
```

---

### Task 6: analyze_watchlist.py 指数识别加 HK/CSI

**Files:**
- Modify: `scripts/qlib_eval/analyze_watchlist.py`

- [ ] **Step 1: 泛化指数识别（让 HK 指数进相关性矩阵）**

在 `scripts/qlib_eval/analyze_watchlist.py` 的 `main()` 中，把：

```python
    idx_codes = [c for c in codes if c[2:5] in ("000", "399")]
```

替换为：

```python
    # 指数 instrument：A股 SH/SZ 的 000/399 段、中证 CSI 前缀、港股 HSI/HSCEI。
    def is_index(c: str) -> bool:
        return c[2:5] in ("000", "399") or c.startswith("CSI") or c in ("HSI", "HSCEI")
    idx_codes = [c for c in codes if is_index(c)]
```

- [ ] **Step 2: 语法检查**

Run: `scripts/qlib_eval/.venv/bin/python -c "import ast; ast.parse(open('scripts/qlib_eval/analyze_watchlist.py').read()); print('ok')"`
Expected: `ok`。

- [ ] **Step 3: 提交**

```bash
git add scripts/qlib_eval/analyze_watchlist.py
git commit -m "feat(qlib): analyze_watchlist recognizes HK/CSI indexes in correlation"
```

---

### Task 7: 集成 — 建 atlas_hk 包并分析（端到端验证）

**Files:**（无源码改动，验证 + 产物）

- [ ] **Step 1: 导出港股 OHLCV → qlib_csv_hk**

Run:
```bash
go build -o bin/atlas ./cmd/atlas
./bin/atlas export-ohlcv --config configs/config.yaml --market hk --from 2021-01-01 --out-dir qlib_csv_hk 2>&1 | grep -vE "FetchHistory failed|trying lixinger" | tail -6
ls qlib_csv_hk/ | wc -l
```
Expected: 11 个 CSV（5 股票 + 4 ETF + 2 指数），退出码 0。抽查 `head -2 qlib_csv_hk/hsi.csv`：close 应为恒生点位（~2万）。

- [ ] **Step 2: dump_bin 建 atlas_hk 包**

Run:
```bash
scripts/qlib_eval/.venv/bin/python scripts/qlib_eval/build_data.py \
  --csv-dir qlib_csv_hk --target-dir "$HOME/.qlib/qlib_data/atlas_hk"
```
Expected: 输出 `instruments: 11` 与区间，HK 独立日历。

- [ ] **Step 3: 港股 watchlist 分析报告**

Run:
```bash
scripts/qlib_eval/.venv/bin/python scripts/qlib_eval/analyze_watchlist.py \
  --qlib-dir "$HOME/.qlib/qlib_data/atlas_hk" --out reports/watchlist-hk-analysis.md
head -30 reports/watchlist-hk-analysis.md
```
Expected: 11 instruments 表 + 含 HSI/HSCEI 的相关性矩阵。

- [ ] **Step 4: 确认 atlas_cn 包未被影响（A股回归）**

Run: `./bin/atlas export-ohlcv --config configs/config.yaml --from 2026-06-01 --to 2026-06-10 --out-dir /tmp/cncheck 2>&1 | grep -vE "FetchHistory failed|trying lixinger" | tail -3; ls /tmp/cncheck | wc -l; rm -rf /tmp/cncheck`
Expected: 默认 market=cn 仍导出 24 个 A股 CSV（含 .CSI），退出码 0。

> 产物 `qlib_csv_hk/`、`reports/`、`bin/` 均被 `.gitignore` 忽略，本任务不提交代码。

---

## 自查覆盖

- 独立 atlas_hk 包（HK 日历）→ Task 5/7（build_data --target-dir atlas_hk）
- export-ohlcv --market cn/hk → Task 3
- 命名契约 HK#####/HSI/HSCEI（Go+Python 对称 + 契约测试）→ Task 1/2
- watchlist 加 4 ETF + 2 指数 → Task 4
- ^HSTECH 不纳入（拒绝测试 + 仅 2 指数）→ Task 1/2/4
- 基准按 market（cn=000300.SH/hk=^HSI，缺失硬错误）→ Task 3
- 行情走 yahoo（路由已就绪）→ 设计已验证，无需改 selector
- analyze 支持 HK 指数相关性 → Task 6
- A股零回归（默认 market=cn）→ Task 3 Step4 + Task 7 Step4
