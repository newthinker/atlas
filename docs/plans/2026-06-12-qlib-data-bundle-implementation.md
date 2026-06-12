# 自建 qlib 数据包（atlas → dump_bin）实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `make qlib-data` 从 atlas 自有采集器构建 qlib 数据包（`~/.qlib/qlib_data/atlas_cn`），使 `make signal-eval` 在默认 2021-2026 区间产出非空评估结果（社区包截止 2020-09 做不到的事）。

**Architecture:** Go 新增 `export-ohlcv` cobra 子命令（结构同构 export_signals：deps 注入 + golden 测试），按 qlib 约定落 per-instrument CSV；Python 新增 `build_data.py` 薄封装 subprocess 调官方 `dump_bin.py`（DumpDataAll）并校验产物；Makefile `qlib-data` 串联，`signal-eval` 的 `QLIB_DIR` 默认切到 `atlas_cn`。评估数据与信号生成数据**同源**（同一套 collector）。

**Tech Stack:** Go 1.21 + cobra + encoding/csv；Python 3.11（**一律 `scripts/qlib_eval/.venv/bin/python`，系统 python3 已损坏**）+ pytest（mock subprocess，零 qlib 依赖）；官方 dump_bin.py（/Users/zuowei/workspace/python/qlib/scripts/，仅运行时）。

**设计依据（必读）：** `docs/superpowers/specs/2026-06-12-qlib-data-bundle-design.md`（rev4，评审环 4 轮 Approved）

**已钉死口径（spec 结论，不要重新决策）：**
- 价格 = eastmoney fqt=1 前复权（与信号侧同一 FetchHistory 天然同源），CSV `factor` 列恒写 `1`
- dump_bin 必须传 `--exclude_fields symbol,date`（否则字符串列 astype(float32) 必崩）
- 符号三形式：atlas `000300.SH` → qlib instrument `SH000300`（`to_qlib_instrument` 返回值，大写）→ CSV 文件名 `sh000300.csv`（instrument 再 `.lower()`）
- Makefile 只传 `--from $(SIGNAL_FROM)`，不传 `--to`；CLI `--to` 默认当天
- 基准 `000300.SH` 失败 = 硬错误；其他符号逐个降级 + 摘要 + 非 0 退出；非 `.SH/.SZ` 符号直接拒绝进摘要
- 逐符号 300ms 礼貌延迟

---

## Chunk 1: Go 导出端

### Task 1: export-ohlcv 核心（golden CSV + 符号语义 + 默认集）

**Files:**
- Create: `cmd/atlas/export_ohlcv.go`
- Test: `cmd/atlas/export_ohlcv_test.go`

- [ ] **Step 1: 写失败测试**

参照 `cmd/atlas/export_signals_test.go` 的 deps 注入与 helper 风格（staticOHLCVProvider/makeBars 可复用或仿照）：

```go
// toQlibInstrument 契约测试——权威定义是 scripts/qlib_eval/qlib_eval/symbols.py
// 的 to_qlib_instrument（返回大写 SH000300），Go 侧必须同样本同结果。
func TestToQlibInstrument_Contract(t *testing.T) {
	cases := []struct{ in, want string }{
		{"000300.SH", "SH000300"}, // 与 symbols.py tests 同样本
		{"600519.SH", "SH600519"},
		{"399001.SZ", "SZ399001"},
	}
	for _, c := range cases {
		got, err := toQlibInstrument(c.in)
		if err != nil || got != c.want {
			t.Errorf("toQlibInstrument(%q) = (%q,%v), want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"AAPL", "^GSPC", "GC=F", "BTC-USDT", "0700.HK"} {
		if _, err := toQlibInstrument(bad); err == nil {
			t.Errorf("toQlibInstrument(%q) should reject non-A-share", bad)
		}
	}
	// spec 钉死：文件名层派生断言
	if ins, _ := toQlibInstrument("000300.SH"); strings.ToLower(ins)+".csv" != "sh000300.csv" {
		t.Errorf("filename derivation broken: %s", ins)
	}
}

// makeOHLCVBars: 新增 helper——既有 makeBars 只填 Close/Time（O/H/L/V 全零值），
// 产不出能防列错位的 golden。新 helper 给 O/H/L/C/V 互不相同的值。
func makeOHLCVBars(start string, n int) []core.OHLCV {
	// bar i: Open=100+i, High=101+i, Low=99+i, Close=100.5+i, Volume=1000+i，连续工作日
	...
}

// fakeOHLCVProvider: per-symbol 数据与错误（staticOHLCVProvider 忽略 symbol 且
// 永不出错，无法表达失败语义）
type fakeOHLCVProvider struct {
	data map[string][]core.OHLCV
	errs map[string]error
}
func (f fakeOHLCVProvider) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := f.errs[symbol]; err != nil {
		return nil, err
	}
	return f.data[symbol], nil
}

func TestExportOHLCV_GoldenCSV(t *testing.T) {
	bars := makeOHLCVBars("2024-01-02", 3)
	dir := t.TempDir()
	deps := ohlcvDeps{provider: fakeOHLCVProvider{data: map[string][]core.OHLCV{"600519.SH": bars}}, errOut: io.Discard, sleep: func() {}}
	err := executeExportOHLCV(deps, exportOHLCVParams{
		Symbols: []string{"600519.SH"}, From: "2024-01-01", To: "2024-01-10", OutDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "sh600519.csv")) // 文件名 = instrument 小写
	want := `symbol,date,open,high,low,close,volume,factor
SH600519,2024-01-02,100.00,101.00,99.00,100.50,1000,1
SH600519,2024-01-03,101.00,102.00,100.00,101.50,1001,1
SH600519,2024-01-04,102.00,103.00,101.00,102.50,1002,1
`
	if string(got) != want {
		t.Errorf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportOHLCV_NonAShareRejectedIntoSummary(t *testing.T) {
	// fakeOHLCVProvider data 含 600519.SH；symbols=[AAPL, 600519.SH]
	// → AAPL 不落盘、errOut 摘要含 AAPL、返回 error（非 0）
	// → sh600519.csv 已写出（已成功 CSV 保留）
}

func TestExportOHLCV_BenchmarkFailureIsFatal(t *testing.T) {
	// fakeOHLCVProvider errs["000300.SH"]=errors.New("boom")
	// → executeExportOHLCV 立即返回 error 且消息含 "benchmark"
}

// 分层语义：清单「含基准」校验在 CLI 层（runExportOHLCV，符号集语义所在层，
// 与默认集构造同层）；executeExportOHLCV 核心保持纯执行——golden 测试可直调
// 核心、symbols 不带基准，零冲突。CLI 层校验的测试归 Task 2。

func TestExportOHLCV_EmptySymbolsAndWatchlistIsError(t *testing.T) {
	// C1-1 防线：watchlist 为空且未显式 --symbols → 报错（绝不退化为只导基准）
}

func TestDefaultOHLCVSymbols(t *testing.T) {
	// watchlist {600519.SH(股票), BTC-USDT, ^GSPC, 000300.SH(指数)} →
	// 默认集 = {600519.SH, 000300.SH}（.SH/.SZ 过滤 + 基准去重）
}
```

golden 与上文 makeOHLCVBars 的生成规律**互锁，不得单边改动**（既有 makeBars 只填 Close/Time，不可用于 golden）；`sleep` 注入避免测试等待 300ms。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/atlas/ -run 'TestToQlibInstrument|TestExportOHLCV|TestDefaultOHLCV' -v`
Expected: FAIL（toQlibInstrument/executeExportOHLCV 未定义）

- [ ] **Step 3: 最小实现**

```go
// cmd/atlas/export_ohlcv.go
package main

const benchmarkSymbol = "000300.SH" // SH000300 in qlib form; evaluation is meaningless without it

// toQlibInstrument mirrors scripts/qlib_eval/qlib_eval/symbols.py
// to_qlib_instrument — keep the two in sync (contract test shares samples).
func toQlibInstrument(symbol string) (string, error) {
	switch {
	case strings.HasSuffix(symbol, ".SH"):
		return "SH" + strings.TrimSuffix(symbol, ".SH"), nil
	case strings.HasSuffix(symbol, ".SZ"):
		return "SZ" + strings.TrimSuffix(symbol, ".SZ"), nil
	}
	return "", fmt.Errorf("not an A-share symbol: %s", symbol)
}

type exportOHLCVParams struct {
	Symbols  []string
	From, To string
	OutDir   string
}

type ohlcvDeps struct {
	provider backtest.OHLCVProvider // 测试注入；CLI 路径逐 symbol SelectForSymbol
	errOut   io.Writer
	sleep    func() // 300ms 礼貌延迟，测试注入空函数
}

// executeExportOHLCV writes one qlib-convention CSV per A-share symbol.
// Per-symbol failures degrade with a summary (non-zero exit); a failed
// benchmark is fatal. factor is always 1: prices are fqt=1 前复权 at the
// source and the evaluator never multiplies $factor (spec §已钉死口径).
func executeExportOHLCV(deps ohlcvDeps, p exportOHLCVParams) error {
	// 1. 解析 from/to（复用 parseBacktestDate；To 为空 → time.Now()）
	// 2. 遍历 symbols：
	//    a. toQlibInstrument 失败 → 记入 failures（不落盘），continue
	//    b. FetchHistory(symbol, from, to, "1d")；失败：
	//       - symbol == benchmarkSymbol → return fmt.Errorf("benchmark %s: %w", ...)
	//       - 否则记入 failures，continue
	//    c. bars 为空 → 同失败处理（基准空也算硬错误）
	//    d. 写 {outDir}/{strings.ToLower(instrument)}.csv：
	//       header symbol,date,open,high,low,close,volume,factor
	//       行：instrument, bar.Time.Format("2006-01-02"), %.2f×4, volume(整数), 1
	//    e. deps.sleep()
	// 3. len(failures) > 0 → errOut 摘要 + return error（已写 CSV 保留）
	...
}

// defaultOHLCVSymbols: cfg.Watchlist 中 .SH/.SZ 后缀的 symbol + benchmarkSymbol，去重保序
func defaultOHLCVSymbols(items []config.WatchlistItem) []string { ... }
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/atlas/ -v`
Expected: 全部 PASS（export_signals 等既有测试零回归）

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/export_ohlcv*.go
git commit -m "feat(cli): export-ohlcv core — qlib CSV convention, symbol semantics, default set"
```

### Task 2: cobra 接线 + Makefile qlib-data

**Files:**
- Modify: `cmd/atlas/export_ohlcv.go`（追加 cobra 命令）
- Modify: `Makefile`
- Test: `scripts/qlib_eval/tests/test_makefile.py`（仿照既有 test_signal_eval_uses_venv_python_not_bare_python 增一例）

- [ ] **Step 1: cobra 命令（照抄 export_signals.go 的命令注册模式）**

```go
var exportOHLCVCmd = &cobra.Command{
	Use:   "export-ohlcv",
	Short: "Export per-instrument OHLCV CSVs in qlib dump_bin convention",
	RunE:  runExportOHLCV,
}
// flags: --symbols（逗号清单，默认空 = watchlist A 股 + 基准）、
//        --from（required）、--to（默认空 = 当天）、--out-dir（默认 "qlib_csv"）
// runExportOHLCV:
//   - config 加载参照 serve.go:55-66 的 cfgFile + config.Load 模式
//     （注意：export_signals 无 config 加载不可作参照；config.Defaults() 无
//     watchlist 字段——所以「--symbols 空且 watchlist 空 → 报错」是硬防线）
//   - registry 组装与 per-symbol provider 适配照抄 export_signals 的 CLI 路径
//     （逐 symbol SelectForSymbol）

// TDD 顺序（先测后实现）：先写 TestExportOHLCVCommand_UsageListsAllFlags
//（仿照 export_signals 的同名测试）、TestRunExportOHLCV_BenchmarkMissingIsFatal
//（CLI 层校验：--symbols 清单不含 000300.SH → 报错含 "benchmark"，Task 1 分层
// 语义的承接）与 test_makefile 断言（Step 3），确认 RED，再实现 cobra 命令与
// Makefile target。
```

- [ ] **Step 2: Makefile target（注意：只传 --from，不传 --to——spec 钉死）**

```makefile
QLIB_CSV_DIR ?= qlib_csv
QLIB_DATA_DIR ?= $(HOME)/.qlib/qlib_data/atlas_cn

.PHONY: qlib-data
qlib-data: build
	./bin/atlas export-ohlcv --symbols $(SIGNAL_SYMBOLS) \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_DIR) --target-dir $(QLIB_DATA_DIR)
```

**必须显式传 `--symbols $(SIGNAL_SYMBOLS)`**（评审 C1-1 BLOCKER）：recipe 不带 --config 时 CLI 拿不到 watchlist（config.Defaults() 无该字段），默认集会退化为只剩基准，数据包静默缺 600519.SH——`make signal-eval` 的信号又被全丢，复刻本需求要消灭的事故。共用 SIGNAL_SYMBOLS 还天然保证「评估符号 ⊆ 数据包」。SIGNAL_SYMBOLS 默认值已含基准 000300.SH；若用户覆盖变量时漏掉基准，由 export-ohlcv 既有的「基准失败/缺失 = 硬错误」语义兜底报错（不做显式清单自动追加——spec 未定义该行为，YAGNI）。

（build_data.py 在 Task 3 实现；本 task 先落 target，test_makefile 锚定文本。`QLIB_DIR` 默认值切换放 Task 4 与 e2e 一起验。）

- [ ] **Step 3: test_makefile.py 增断言**

test_makefile.py 现有 helper 是 `_makefile_text()`/`_signal_eval_block()`（硬编码 target 名）——**先把 `_signal_eval_block` 泛化为 `_target_block(name)`**（取 target 行 + 后续 Tab 缩进 recipe 行），既有测试改用 `_target_block("signal-eval")` 零行为变化，再加新断言：

```python
def test_qlib_data_target_flags():
    block = _target_block("qlib-data")
    assert "--symbols $(SIGNAL_SYMBOLS)" in block   # C1-1 防线
    assert "--from $(SIGNAL_FROM)" in block
    assert "--to" not in block                       # spec: 不传 --to
    assert "$(QLIB_PY) scripts/qlib_eval/build_data.py" in block
```

- [ ] **Step 4: 编译 + 冒烟 + 测试**

```bash
go build -o bin/atlas ./cmd/atlas && ./bin/atlas export-ohlcv --help   # flags 齐全
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_makefile.py -v
```

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/ Makefile scripts/qlib_eval/tests/test_makefile.py
git commit -m "feat(cli): wire export-ohlcv command and qlib-data Makefile target"
```

---

## Chunk 2: Python 构建端 + e2e

### Task 3: build_data.py（dump_bin 编排 + 产物校验）

**Files:**
- Create: `scripts/qlib_eval/build_data.py`
- Test: `scripts/qlib_eval/tests/test_build_data.py`

- [ ] **Step 1: 写失败测试（mock subprocess，零 qlib 依赖）**

```python
from unittest import mock
import build_data  # conftest.py 已保证根目录可 import

def test_dump_bin_command_construction(tmp_path):
    csv_dir = tmp_path / "csv"; csv_dir.mkdir()
    (csv_dir / "sh600519.csv").write_text("symbol,date,open\nSH600519,2024-01-02,1\n")
    with mock.patch("build_data.subprocess.run") as run:
        run.return_value = mock.Mock(returncode=0)
        build_data.run_dump_bin(csv_dir, tmp_path / "bundle", scripts_dir=tmp_path)
        cmd = run.call_args[0][0]
    assert cmd[1].endswith("dump_bin.py") and cmd[2] == "dump_all"
    # ⚠ 本地副本参数名是 --data_path（旧版/网上教程的 csv_path 已改名，
    #   照旧名写 mock 会固化错误命令、实跑才崩——评审 C2-1 BLOCKER）
    assert "--data_path" in cmd and "--qlib_dir" in cmd
    i = cmd.index("--exclude_fields")
    assert cmd[i + 1] == "symbol,date"  # spec 钉死：字符串列不排除 dump 必崩

def test_dump_bin_failure_propagates(tmp_path):
    # returncode=1 → run_dump_bin raise SystemExit/RuntimeError（非 0 语义）

def test_verify_bundle(tmp_path):
    # 构造假 bundle，instruments/all.txt 用 dump_bin 真实格式：
    # **tab 分隔三字段** "SH600519\t2021-01-04\t2026-06-12"（save_instruments，
    # symbol.upper()）；calendars/day.txt 每行一个 YYYY-MM-DD
    # verify_bundle(bundle, expected_instruments={"SH600519"}, start, end) 通过；
    # 缺 instrument / calendar 区间不足 → raise ValueError（消息含缺失项）
    # 注意：只读校验，绝不写 instruments/calendar（dump_bin 自动生成）

def test_date_span_from_csvs(tmp_path):
    # C2-2：verify 用的 start/end 从 csv_dir 全部数据行推导（min/max date），
    # 与 dump_bin 同数据源自洽——argparse 不需要日期参数，Makefile 零改动

def test_csv_dir_empty_csv_rejected(tmp_path):
    # csv 目录含 0 字节/仅 header 文件 → 进 dump 前 raise（防空 instrument 污染）
```

- [ ] **Step 2: 确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_build_data.py -v`
Expected: FAIL（build_data 不存在）

- [ ] **Step 3: 实现 build_data.py**

```python
"""Build a qlib data bundle from atlas-exported CSVs via the official dump_bin.

Thin orchestration only: dump_bin.py owns the bin format, calendars and
instruments; we construct its command, propagate failures, and VERIFY (never
rewrite) the outputs. See spec 2026-06-12-qlib-data-bundle-design.md.
"""
DEFAULT_QLIB_SCRIPTS = "/Users/zuowei/workspace/python/qlib/scripts"
DEFAULT_TARGET = "~/.qlib/qlib_data/atlas_cn"

def run_dump_bin(csv_dir, target_dir, scripts_dir=DEFAULT_QLIB_SCRIPTS):
    # 前置：csv_dir 非空且无空 CSV（len(read_text().splitlines()) >= 2）
    # cmd = [sys.executable, f"{scripts_dir}/dump_bin.py", "dump_all",
    #        "--data_path", str(csv_dir), "--qlib_dir", str(target_dir),
    #        "--exclude_fields", "symbol,date"]
    # ⚠ 参数名以本地副本签名为准：DumpDataBase.__init__(data_path, qlib_dir, ...)
    #   ——是 data_path，不是网上旧教程的 csv_path（C2-1 BLOCKER）
    # subprocess.run(...)；returncode != 0 → raise RuntimeError(stderr 摘要)

def date_span_from_csvs(csv_dir):
    # 返回 (min_date, max_date)：扫全部 CSV 数据行的 date 列（C2-2）

def verify_bundle(target_dir, expected_instruments, start, end):
    # instruments/all.txt（tab 三字段，instrument 大写）：每个 expected 有行；
    # calendars/day.txt 首尾覆盖 [start 的最近交易日, end 的最近交易日]
    # 不满足 → ValueError；绝不写文件

def main(argv=None):
    # argparse: --csv-dir(required) / --target-dir(默认 DEFAULT_TARGET) / --qlib-scripts
    # expected_instruments 从 csv_dir 文件名导出（stem.upper()）
    # start, end = date_span_from_csvs(csv_dir)
    # run_dump_bin → verify_bundle → 打印「数据包就绪 + make signal-eval QLIB_DIR=...」指引

# 真实 dump_bin 调用的 integration marker 测试：以 Task 4 的 shell e2e 替代，
# marker 化留二期（spec 测试策略的此项在本计划按 e2e 承接，记录于此防转写丢失）。
```

- [ ] **Step 4: hook 同款命令确认全绿**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q`
Expected: 全部 PASS（含既有 32+ 用例零回归）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_eval/build_data.py scripts/qlib_eval/tests/test_build_data.py
git commit -m "feat(qlib_eval): build_data orchestrates dump_bin with bundle verification"
```

### Task 4: QLIB_DIR 切换 + README + e2e 验收

**Files:**
- Modify: `Makefile`（`QLIB_DIR ?= ~/.qlib/qlib_data/cn_data` → `atlas_cn`）
- Modify: `scripts/qlib_eval/README.md`

- [ ] **Step 1: Makefile QLIB_DIR 默认切到 atlas_cn**（test_makefile.py 如有锚定旧值的断言同步更新）

- [ ] **Step 2: README 增「自建数据包」章节**

内容：make qlib-data 用法；**复权口径**（价格 fqt=1 前复权、factor=1、与信号生成同源——评估口径一节交叉引用）；crontab 示例：

```cron
# 每个交易日收盘后重建 qlib 数据包（系统 cron，atlas 不内置调度）
30 16 * * 1-5 cd /path/to/atlas && make qlib-data >> /tmp/qlib-data.log 2>&1
```

社区包 vs 自建包对比（截止日/覆盖面）与 QLIB_DIR 切换方法；注明**直接调 evaluate.py 时需自带 `--qlib-dir`**（其内置 DEFAULT_QLIB_DIR 仍指 cn_data，make 路径总是显式传参不受影响）。

- [ ] **Step 3: e2e 验收（本需求存在理由）**

```bash
make qlib-data            # 拉真实数据 → dump_bin → 校验通过
# 首日核对项（spec）：抽查数值 sanity
scripts/qlib_eval/.venv/bin/python -c "
import qlib; from qlib.data import D
qlib.init(provider_uri='$HOME/.qlib/qlib_data/atlas_cn', region='cn')
df = D.features(['SH600519'], ['\$open','\$close'])
print(df.head(2)); print(df.tail(2))"
# 与 qlib_csv/sh600519.csv 首尾两天逐值对照一致
make signal-eval          # 默认 2021-2026 区间
# 预期：reports/signal-eval-*.md 两策略结果表非空（社区包做不到的事）
```

- [ ] **Step 4: 全量回归 + 提交**

```bash
go build ./... && go vet ./... && go test ./...
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q
git add Makefile scripts/qlib_eval/README.md
git commit -m "feat(qlib_eval): switch QLIB_DIR to atlas_cn bundle with build docs and cron example"
```

---

## 验收对照（spec）

- [ ] `make qlib-data` 产出 atlas_cn 数据包，instruments/calendar 校验通过
- [ ] 契约：Go toQlibInstrument 与 Python to_qlib_instrument 同样本同结果（000300.SH → SH000300）
- [ ] 非 A 股符号拒绝进摘要；基准失败硬错误；单符号失败降级 + 非 0 退出（均有测试）
- [ ] dump_bin 命令含 --exclude_fields symbol,date（mock 断言）
- [ ] D.features 抽查首尾数值与源 CSV 一致（首日核对项）
- [ ] **make signal-eval 默认 2021-2026 区间产出非空策略结果表**
- [ ] pytest 零 qlib 依赖、双语言全量回归零失败
