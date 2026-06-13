# 美股 signal-eval（atlas_us 全管线）实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 signal-eval 事件研究管线支持美股——atlas 导出美股策略信号 + OHLCV → 自建 `atlas_us` qlib 包做事件研究，相对 ^GSPC 算超额。

**Architecture:** 镜像 sprint-009 HK 模式，给「市场差异」加 US 分支：Go `export_ohlcv.go` 的 4 个市场键控点（`toQlibInstrument`/`benchmarkForMarket`/`inMarket`/`--market` 校验）+ Python `symbols.py` 对称镜像 + `prices.py` region 参数化 + Makefile `atlas_us` target + config US watchlist。不碰已跑通的 CN/HK 路径。

**Tech Stack:** Go 1.21（cobra CLI、标准库 testing）；Python 3.11（pandas、pytest，qlib 惰性导入）。

**设计依据（必读）：** `docs/plans/2026-06-13-atlas-us-signal-eval-design.md`（rev2.1 终版）

**⚠ 设计补充（计划阶段发现）：** 设计 §2.1 只展示了 `toQlibInstrument`，但「镜像 HK」意味着 `export_ohlcv.go` 内 HK 当年触碰的全部市场键控函数都要加 US 分支——`benchmarkForMarket`（→`^GSPC`）、`inMarket`（裸 ticker + `^GSPC/^IXIC/^DJI`）、`runExportOHLCV` 的 `--market` 白名单校验（`cn`/`hk`→加 `us`）、`--market` flag 帮助文案。本计划 Task 2 覆盖这些。

**执行纪律：** 严格 TDD；Go 测试 `go test ./cmd/atlas/`；Python 测试 `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/`（系统 python3 损坏，必须用 venv）；全部 Task 完成后、最终提交前运行 code-simplifier sub-agent。

---

## Chunk 1: Go export-ohlcv 美股支持

### Task 1: toQlibInstrument 美股分支 + 契约测试 reject→accept 迁移

**Files:**
- Modify: `cmd/atlas/export_ohlcv.go`（import 块 :3-21、`toQlibInstrument` :59-76）
- Test: `cmd/atlas/export_ohlcv_test.go`（`TestToQlibInstrument_Contract` :73-100）

- [ ] **Step 1: 改测试（reject→accept 迁移 + 锚定负向）**

`TestToQlibInstrument_Contract` 的 accept `cases` 末尾追加 US：

```go
		{"AAPL", "AAPL"},   // 美股裸 ticker 恒等
		{"MSFT", "MSFT"},
		{"GOOGL", "GOOGL"}, // 5 字符，命中 [A-Z]{1,5} 上界
		{"^GSPC", "GSPC"},  // 美股指数剥离 ^
		{"^IXIC", "IXIC"},
		{"^DJI", "DJI"},
```

reject 列表**移除 `"AAPL"`、`"^GSPC"`**，并补锚定负向样本，改为：

```go
	for _, bad := range []string{"GC=F", "BTC-USDT", "0700.HK.X", "^HSTECH", "AAPL123", "AAPL.B", "aapl", "TOOLONG"} {
```

（`AAPL123`/`AAPL.B` 验证 `[A-Z]{1,5}` 全串锚定；`aapl` 小写须拒；`TOOLONG`=7 字符超上界须拒；`^HSTECH` 既非 HK 已知指数也非 US 白名单，仍拒。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run TestToQlibInstrument_Contract -v`
Expected: FAIL（`AAPL`/`^GSPC` 当前被 reject，新 accept 用例报错）

- [ ] **Step 3: 实现**

`export_ohlcv.go` import 块加 `"regexp"`（当前无；按字母序插在 `"path/filepath"` 之后、`"slices"` 之前）。

`toQlibInstrument` 上方加包级 var：

```go
// usTickerRe matches a bare US ticker: 1-5 uppercase letters. Anchored full-match
// so AAPL123 / AAPL.B are rejected. Mirrored in symbols.py (re.fullmatch).
var usTickerRe = regexp.MustCompile("^[A-Z]{1,5}$")
```

`toQlibInstrument` 的 `^HSCE` case 之后、`return "", error` 之前插入：

```go
	case symbol == "^GSPC", symbol == "^IXIC", symbol == "^DJI":
		return strings.TrimPrefix(symbol, "^"), nil // 美股指数剥离 ^
	case usTickerRe.MatchString(symbol):
		return symbol, nil // 美股裸 ticker 恒等
```

error 文案改为含 US：

```go
	return "", fmt.Errorf("not a supported A-share/HK/US symbol: %s", symbol)
```

注释（:55-58 的 doc）补一句 US 规则。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/atlas/ -run TestToQlibInstrument_Contract -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/export_ohlcv.go cmd/atlas/export_ohlcv_test.go
git commit -m "feat(export-ohlcv): map US tickers/indices to qlib instruments"
```

### Task 2: benchmarkForMarket / inMarket / market 校验的美股分支

**Files:**
- Modify: `cmd/atlas/export_ohlcv.go`（`benchmarkForMarket` :31-36、`inMarket` :41-48、`runExportOHLCV` market 校验 :302-303、`--market` flag help :239-240）
- Test: `cmd/atlas/export_ohlcv_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestBenchmarkForMarket_US(t *testing.T) {
	if got := benchmarkForMarket("us"); got != "^GSPC" {
		t.Errorf("benchmarkForMarket(us) = %q, want ^GSPC", got)
	}
	if got := benchmarkForMarket("cn"); got != "000300.SH" { // 不回归
		t.Errorf("cn regressed: %q", got)
	}
}

func TestInMarket_US(t *testing.T) {
	in := map[string]bool{
		"AAPL": true, "MSFT": true, "GOOGL": true, "^GSPC": true, "^IXIC": true, "^DJI": true,
		"600519.SH": false, "0700.HK": false, "^HSI": false, "GC=F": false, "AAPL.B": false,
	}
	for sym, want := range in {
		if got := inMarket(sym, "us"); got != want {
			t.Errorf("inMarket(%q, us) = %v, want %v", sym, got, want)
		}
	}
	// CN/HK 不回归
	if !inMarket("600519.SH", "cn") || !inMarket("0700.HK", "hk") {
		t.Error("cn/hk inMarket regressed")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run 'TestBenchmarkForMarket_US|TestInMarket_US' -v`
Expected: FAIL（us 分支未实现，benchmarkForMarket(us) 回退 cn 基准）

- [ ] **Step 3: 实现**

`benchmarkForMarket` 加 us 分支（保持现有 hk + default-cn 结构）：

```go
func benchmarkForMarket(market string) string {
	switch market {
	case "hk":
		return "^HSI"
	case "us":
		return "^GSPC"
	default:
		return benchmarkSymbol
	}
}
```

`inMarket` 加 us 分支（复用 Task 1 的 `usTickerRe`）：

```go
func inMarket(symbol, market string) bool {
	switch market {
	case "hk":
		return strings.HasSuffix(symbol, ".HK") || symbol == "^HSI" || symbol == "^HSCE"
	case "us":
		return symbol == "^GSPC" || symbol == "^IXIC" || symbol == "^DJI" || usTickerRe.MatchString(symbol)
	default:
		return strings.HasSuffix(symbol, ".SH") ||
			strings.HasSuffix(symbol, ".SZ") ||
			strings.HasSuffix(symbol, ".CSI")
	}
}
```

`runExportOHLCV` 的 market 校验（:302）改为接受 us：

```go
	if exportOHLCVMarket != "cn" && exportOHLCVMarket != "hk" && exportOHLCVMarket != "us" {
		return fmt.Errorf("unknown market %q (want cn, hk or us)", exportOHLCVMarket)
	}
```

`--market` flag help（:240）改为 `"Market bundle: cn (A-share), hk (Hong Kong) or us (US)"`。

- [ ] **Step 4: 运行确认通过 + 全包回归**

Run: `go test ./cmd/atlas/ -v`
Expected: 全部 PASS（既有 cn/hk 用例不回归）

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/export_ohlcv.go cmd/atlas/export_ohlcv_test.go
git commit -m "feat(export-ohlcv): us market branch for benchmark/inMarket/validation"
```

---

## Chunk 2: Python 评估端 + 编排 + 配置

### Task 3: symbols.py 美股对称镜像

**Files:**
- Modify: `scripts/qlib_eval/qlib_eval/symbols.py`（`to_qlib_instrument` :13-32）
- Test: `scripts/qlib_eval/tests/test_symbols.py`（accept :13-22、reject :25-28）

- [ ] **Step 1: 改测试（reject→accept + 锚定负向）**

`test_to_qlib_instrument`（accept 断言）追加：

```python
    assert to_qlib_instrument("AAPL") == "AAPL"    # 美股裸 ticker
    assert to_qlib_instrument("GOOGL") == "GOOGL"  # 5 字符上界
    assert to_qlib_instrument("^GSPC") == "GSPC"   # 美股指数剥离 ^
    assert to_qlib_instrument("^IXIC") == "IXIC"
    assert to_qlib_instrument("^DJI") == "DJI"
```

`test_to_qlib_instrument_rejects_non_ashare` 的循环**移除 `"AAPL"`、`"^GSPC"`**，补锚定负向，改为：

```python
    for bad in ("GC=F", "BTC-USDT", "^HSTECH", "AAPL123", "AAPL.B", "aapl", "TOOLONG"):
```

- [ ] **Step 2: 运行确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_symbols.py -v`
Expected: FAIL（`AAPL`/`^GSPC` 当前 raise ValueError）

- [ ] **Step 3: 实现**

`symbols.py` 顶部加 `import re`。`to_qlib_instrument` 的 `^HSCE` 分支之后、`raise ValueError` 之前插入（**用 `re.fullmatch` 锚定，与 Go `^[A-Z]{1,5}$` 等价**）：

```python
    if symbol in ("^GSPC", "^IXIC", "^DJI"):  # 美股指数剥离 ^
        return symbol[1:]
    if re.fullmatch(r"[A-Z]{1,5}", symbol):  # 美股裸 ticker 恒等（全串锚定）
        return symbol
```

`raise ValueError` 文案与 docstring 改为含 US。

- [ ] **Step 4: 运行确认通过 + 契约对称验证**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_symbols.py -v && go test ./cmd/atlas/ -run TestToQlibInstrument_Contract`
Expected: 两侧均 PASS（Go/Python 对相同 US 样本输出一致）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_eval/qlib_eval/symbols.py scripts/qlib_eval/tests/test_symbols.py
git commit -m "feat(qlib_eval): mirror US instrument mapping in symbols.py"
```

### Task 4: prices.py region 参数化 + evaluate.py --region

**Files:**
- Modify: `scripts/qlib_eval/qlib_eval/prices.py`（`QlibPriceSource.__init__` :62、`_ensure_init` :69-75）
- Modify: `scripts/qlib_eval/evaluate.py`（`_parse_args` :117、`QlibPriceSource` 构造 :162-163）
- Test: `scripts/qlib_eval/tests/test_prices.py`

- [ ] **Step 1: 写失败测试（region 存字段，不触发 qlib）**

`test_prices.py` 追加：

```python
def test_qlib_price_source_stores_region():
    from qlib_eval.prices import QlibPriceSource
    src = QlibPriceSource(provider_uri="/tmp/x", start="2021-01-01", end="2021-12-31",
                          benchmark="^GSPC", region="us")
    assert src._region == "us"

def test_qlib_price_source_region_defaults_cn():
    from qlib_eval.prices import QlibPriceSource
    src = QlibPriceSource(provider_uri="/tmp/x", start="2021-01-01", end="2021-12-31")
    assert src._region == "cn"  # 向后兼容：CN/HK 不传 region
```

（构造不调 `_ensure_init`，不 import qlib——pytest 全程 qlib-free 不变。）

- [ ] **Step 2: 运行确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_prices.py -k region -v`
Expected: FAIL（`__init__` 无 region 参数 / `_region` 字段不存在）

- [ ] **Step 3: 实现**

`prices.py` `__init__` 签名加 `region`（末位，默认 cn）：

```python
    def __init__(self, provider_uri, start, end, benchmark="000300.SH", region="cn"):
        ...
        self._benchmark = benchmark
        self._region = region
        self._initialized = False
```

`_ensure_init` 的 `qlib.init` 用 `self._region`：

```python
        qlib.init(provider_uri=self._provider_uri, region=self._region)
```

`evaluate.py` `_parse_args` 加 `--region`：

```python
    p.add_argument("--region", default="cn", help="qlib region：cn（A股/港股）/ us（美股）")
```

`main` 构造 `QlibPriceSource` 透传（:162-163）：

```python
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end, benchmark=args.benchmark,
                             region=args.region)
```

- [ ] **Step 4: 运行确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v`
Expected: 全部 PASS（既有用例不回归）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_eval/qlib_eval/prices.py scripts/qlib_eval/evaluate.py scripts/qlib_eval/tests/test_prices.py
git commit -m "feat(qlib_eval): parameterize qlib region (default cn, us for US)"
```

### Task 5: Makefile 美股 target + 守门测试

**Files:**
- Modify: `Makefile`（`.PHONY` :1、变量块 :15-22、新增 `qlib-data-us`/`signal-eval-us` target）
- Test: `scripts/qlib_eval/tests/test_makefile.py`

- [ ] **Step 1: 写失败测试**

`test_makefile.py` 追加（沿用其 `_target_block`/`_var_defs`/`_expand`/`VENV_PYTHON` 工具）：

```python
def test_signal_eval_us_target_exists_and_correct():
    block = _target_block("signal-eval-us")
    assert block, "signal-eval-us target 缺失"
    defs = _var_defs(_makefile_text())
    expanded = _expand(block, defs)
    assert "export-signals" in block
    assert "--benchmark ^GSPC" in expanded
    assert "--region us" in expanded
    assert "atlas_us" in expanded
    assert VENV_PYTHON in expanded          # 必须走 venv python，非裸 python
    assert "evaluate.py" in block

def test_qlib_data_us_target_exists():
    block = _target_block("qlib-data-us")
    assert block, "qlib-data-us target 缺失"
    expanded = _expand(block, _var_defs(_makefile_text()))
    assert "--market us" in expanded
    assert "atlas_us" in expanded
    assert "build_data.py" in block

def test_us_targets_in_phony():
    first_line = _makefile_text().splitlines()[0]
    assert "signal-eval-us" in first_line and "qlib-data-us" in first_line
```

- [ ] **Step 2: 运行确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_makefile.py -k us -v`
Expected: FAIL（target/var 不存在）

- [ ] **Step 3: 实现**

`.PHONY`（第 1 行）末尾追加 `signal-eval-us qlib-data-us`。

变量块（HK 变量之后）加：

```makefile
QLIB_CSV_US_DIR  ?= qlib_csv_us
QLIB_DATA_US_DIR ?= $(HOME)/.qlib/qlib_data/atlas_us
# 美股 watchlist 标的（atlas 形式）：须与 configs/config.yaml 的美股集（裸 ticker + ^GSPC）一致。
SIGNAL_SYMBOLS_US ?= AAPL,MSFT,NVDA,GOOGL,AMZN,META,JNJ,JPM,^GSPC
```

新增两个 target（镜像 hk，export-ohlcv 走 `--market us` 从 watchlist 派生、export-signals 走显式 `--symbols`）：

```makefile
# 美股自建 qlib 数据包：watchlist 美股集（裸 ticker + ^GSPC）→ atlas_us（独立日历）。
qlib-data-us: build
	./bin/atlas export-ohlcv --config configs/config.yaml --market us \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_US_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_US_DIR) \
	  --target-dir $(QLIB_DATA_US_DIR) --expected-symbols $(SIGNAL_SYMBOLS_US)

# 美股事件研究：美股集信号 → 对 atlas_us 评估，基准 ^GSPC，region us。
signal-eval-us: build
	./bin/atlas export-signals --config configs/config.yaml --symbols $(SIGNAL_SYMBOLS_US) \
	  --strategies price_percentile,ma_crossover --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals_us.csv
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals_us.csv \
	  --qlib-dir $(QLIB_DATA_US_DIR) --benchmark ^GSPC --region us --out $(SIGNAL_OUT)
```

- [ ] **Step 4: 运行确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_makefile.py -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add Makefile scripts/qlib_eval/tests/test_makefile.py
git commit -m "feat(makefile): signal-eval-us and qlib-data-us targets"
```

### Task 6: config 美股 watchlist + 收尾

**Files:**
- Modify: `configs/config.yaml`（watchlist，HK 块 :121-134 之后）
- 验收：build + 全量测试 + 端到端必跑项

- [ ] **Step 1: config.yaml 加美股 watchlist 块**

在 HK 指数块之后追加（与 SIGNAL_SYMBOLS_US 一致；个股绑全策略供实盘，^GSPC 仅 price_percentile）：

```yaml
  # ---------- 美股个股（行情走 yahoo；signal-eval 仅回放 OHLCV 策略）----------
  - {symbol: "AAPL",  name: "苹果",     type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "MSFT",  name: "微软",     type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "NVDA",  name: "英伟达",   type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "GOOGL", name: "谷歌",     type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "AMZN",  name: "亚马逊",   type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "META",  name: "Meta",     type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "JNJ",   name: "强生",     type: "股票", strategies: [price_percentile, pe_percentile]}
  - {symbol: "JPM",   name: "摩根大通", type: "股票", strategies: [price_percentile, pe_percentile]}
  # ---------- 美股指数（行情走 yahoo ^ 代码；signal-eval 基准）----------
  - {symbol: "^GSPC", name: "标普500",  type: "指数", strategies: [price_percentile]}
```

- [ ] **Step 2: 编译 + 全量回归**

```bash
go build -o bin/atlas ./cmd/atlas && go test ./cmd/atlas/
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q
./bin/atlas export-signals --config configs/config.yaml --symbols AAPL,^GSPC \
  --strategies price_percentile,ma_crossover --from 2021-01-01 --to 2024-01-01 --out /tmp/us_smoke.csv && head -3 /tmp/us_smoke.csv
```

Expected: 编译通过、两侧测试全绿、export-signals 对美股产出非空 CSV（首行 header + AAPL/^GSPC 信号行）。

- [ ] **Step 3: 运行 code-simplifier**（全局规范）

Task tool: `subagent_type: "code-simplifier:code-simplifier"`，prompt 列出本计划全部改动文件（export_ohlcv.go(+test)、symbols.py(+test)、prices.py(+test)、evaluate.py、Makefile、test_makefile.py、config.yaml）。

- [ ] **Step 4: 端到端必跑项（region="us" 风险验收，设计 §4.1）**

```bash
make qlib-data-us      # 导出美股 OHLCV → dump 成 atlas_us 包
make signal-eval-us    # 对 atlas_us 评估，产出 reports/signal-eval-*.md
```

Expected: 产出非空 markdown 报告，含美股标的的事件研究（5/20/60 日相对 ^GSPC 超额）。
**若 `region="us"` 异常**（qlib 报错/读空）：把 signal-eval-us 的 `--region us` 回退为 `--region cn`（设计 §4.1 已论证 HK 用 cn 跑通非 cn 包），记录于提交信息。

- [ ] **Step 5: gitnexus + 最终提交**

```bash
npx gitnexus analyze && go vet ./...
git add -A
git commit -m "feat: US market support for signal-eval pipeline (atlas_us)

Implements docs/plans/2026-06-13-atlas-us-signal-eval-design.md"
```

---

## 验收对照（design §1.1/§4.3）

- [ ] `toQlibInstrument`/`to_qlib_instrument` 两侧对 `AAPL→AAPL`、`^GSPC→GSPC` 输出一致；`AAPL123`/`AAPL.B` 两侧均 reject（锚定契约）
- [ ] 既有契约测试中 `AAPL`/`^GSPC` 已从 reject 迁为 accept，无 CI 红
- [ ] `benchmarkForMarket("us")=^GSPC`、`inMarket` 美股分支、`--market us` 校验通过；cn/hk 不回归
- [ ] `QlibPriceSource` region 默认 cn（CN/HK 零变化）、可传 us
- [ ] `make signal-eval-us` 对真实 atlas_us 包产出非空报告（region=us 验收，异常则回退 cn）
- [ ] Go + Python 全量测试通过；pytest 仍全程 qlib-free
