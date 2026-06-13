# signal-eval 基准参数化（支持港股）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 signal-eval 加 `--benchmark` 参数（atlas 形式，默认 `000300.SH`），使事件研究可对 atlas_hk 评估（基准 `^HSI`），A股路径零回归。

**Architecture:** `QlibPriceSource.benchmark()` 由硬编码 `SH000300` 改为按构造参数经 `to_qlib_instrument` 解析；`evaluate.py` 暴露 `--benchmark` 并透传；Makefile 加 `signal-eval-hk` target。消费侧 `collect_outcomes` 已 benchmark-agnostic，无需改。

**Tech Stack:** Python（qlib 惰性导入 + pytest）、Make。

设计依据：`docs/superpowers/specs/2026-06-13-signal-eval-benchmark-param-design.md`。

已知事实：
- `QlibPriceSource` 惰性 import qlib（构造不触发 qlib.init）；`benchmark()` 现写死 `["SH000300"]`。
- `to_qlib_instrument`：`000300.SH`→`SH000300`、`^HSI`→`HSI`（已支持，PR #28）。
- export-signals 离线仅支持 `ma_crossover`/`price_percentile`；`--strategies` 必填。
- 本环境 eastmoney 屏蔽，A股 export-signals 跑不动；港股走 yahoo 可达。

---

## File Structure

| 文件 | 改动 |
|---|---|
| `scripts/qlib_eval/qlib_eval/prices.py` | `QlibPriceSource.__init__` 加 `benchmark="000300.SH"`；`benchmark()` 用 `to_qlib_instrument(self._benchmark)`；Protocol docstring 泛化 |
| `scripts/qlib_eval/tests/test_prices.py` | QlibPriceSource benchmark 参数存储 + 转换契约测试 |
| `scripts/qlib_eval/evaluate.py` | `--benchmark` 参数；`_meta` 用之；构造 QlibPriceSource 透传 |
| `scripts/qlib_eval/tests/test_report.py` | `--benchmark` 解析 + `_meta` 反映 测试 |
| `Makefile` | `SIGNAL_BENCHMARK` 变量；signal-eval 传 `--benchmark`；新增 signal-eval-hk target |

每个任务结束 `pytest scripts/qlib_eval/ -q` 全绿。

---

### Task 1: QlibPriceSource 基准参数化（prices.py）

**Files:**
- Modify: `scripts/qlib_eval/qlib_eval/prices.py`
- Modify: `scripts/qlib_eval/tests/test_prices.py`

- [ ] **Step 1: 写基准参数测试（先 RED）**

在 `scripts/qlib_eval/tests/test_prices.py` 末尾追加：

```python
def test_qlib_price_source_benchmark_param():
    """QlibPriceSource 存储基准(atlas 形式)且默认为 A股 CSI300；构造不触发 qlib。"""
    from qlib_eval.prices import QlibPriceSource
    from qlib_eval.symbols import to_qlib_instrument

    s_default = QlibPriceSource(provider_uri="x", start="2021-01-01", end="2021-12-31")
    assert s_default._benchmark == "000300.SH"
    assert to_qlib_instrument(s_default._benchmark) == "SH000300"

    s_hk = QlibPriceSource(
        provider_uri="x", start="2021-01-01", end="2021-12-31", benchmark="^HSI"
    )
    assert s_hk._benchmark == "^HSI"
    assert to_qlib_instrument(s_hk._benchmark) == "HSI"
```

- [ ] **Step 2: 运行确认 RED**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_prices.py -q`
Expected: FAIL（`QlibPriceSource() got unexpected keyword 'benchmark'` / 无 `_benchmark` 属性）。

- [ ] **Step 3: 实现 prices.py 基准参数化**

把 `scripts/qlib_eval/qlib_eval/prices.py` 的 `QlibPriceSource.__init__` 替换为：

```python
    def __init__(self, provider_uri: str, start: str, end: str, benchmark: str = "000300.SH"):
        self._provider_uri = provider_uri
        self._start = start
        self._end = end
        self._benchmark = benchmark  # atlas 形式（000300.SH / ^HSI），benchmark() 内转 qlib instrument
        self._initialized = False
```

把 `QlibPriceSource.benchmark` 方法替换为：

```python
    def benchmark(self) -> pd.DataFrame:
        self._ensure_init()
        from qlib.data import D  # 惰性 import

        from .symbols import to_qlib_instrument

        df = D.features(
            [to_qlib_instrument(self._benchmark)],
            ["$open", "$close"],
            start_time=self._start,
            end_time=self._end,
        )
        return self._normalize(df)
```

把 `PriceSource` Protocol 的 `benchmark` docstring 由 `"""SH000300 的 close 序列。"""` 改为：

```python
        """基准 instrument 的 open/close 序列（A股 SH000300 / 港股 HSI 等）。"""
```

- [ ] **Step 4: 运行确认 PASS**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_prices.py -q`
Expected: PASS（含既有 align_entry / no-qlib-at-module-level 用例）。

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_eval/qlib_eval/prices.py scripts/qlib_eval/tests/test_prices.py
git commit -m "feat(qlib): parameterize QlibPriceSource benchmark (default 000300.SH)"
```

---

### Task 2: evaluate.py 暴露 --benchmark（evaluate.py）

**Files:**
- Modify: `scripts/qlib_eval/evaluate.py`
- Modify: `scripts/qlib_eval/tests/test_report.py`

- [ ] **Step 1: 写 --benchmark 解析 + _meta 测试（先 RED）**

在 `scripts/qlib_eval/tests/test_report.py` 末尾追加：

`evaluate` 模块经 conftest.py 注入 sys.path，既有 test_report.py 顶部已 `import evaluate`（line 15）。沿用：

```python
def test_parse_args_benchmark_default_and_override():
    a = evaluate._parse_args(["--signals", "s.csv"])
    assert a.benchmark == "000300.SH"
    b = evaluate._parse_args(["--signals", "s.csv", "--benchmark", "^HSI"])
    assert b.benchmark == "^HSI"


def test_meta_reflects_benchmark_arg():
    args = evaluate._parse_args(["--signals", "s.csv", "--benchmark", "^HSI"])
    meta = evaluate._meta(args, 5)
    assert meta["benchmark"] == "^HSI"
```

（test_report.py 顶部已 `import evaluate`，无需新增导入。）

- [ ] **Step 2: 运行确认 RED**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_report.py -q`
Expected: FAIL（`_parse_args` 无 `benchmark` 属性 / `_meta` benchmark != ^HSI）。

- [ ] **Step 3: 实现 evaluate.py --benchmark**

在 `scripts/qlib_eval/evaluate.py` 的 `_parse_args` 中，`--max-defer` 那行之后加：

```python
    p.add_argument("--benchmark", default="000300.SH",
                   help="基准 symbol（atlas 形式：A股 000300.SH / 港股 ^HSI）")
```

把 `_meta` 的 `"benchmark": "SH000300",` 改为：

```python
        "benchmark": args.benchmark,
```

把 `main` 中构造 `QlibPriceSource` 处：

```python
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end)
```

改为：

```python
    source = QlibPriceSource(provider_uri=os.path.expanduser(args.qlib_dir),
                             start=start, end=end, benchmark=args.benchmark)
```

- [ ] **Step 4: 运行确认 PASS**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_report.py -q`
Expected: PASS（含既有 benchmark_error 降级 / _meta 用例）。

- [ ] **Step 5: 全量 pytest 不回归**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/ -q`
Expected: 全 PASS。

- [ ] **Step 6: 提交**

```bash
git add scripts/qlib_eval/evaluate.py scripts/qlib_eval/tests/test_report.py
git commit -m "feat(qlib): evaluate.py --benchmark flag (threads to QlibPriceSource + meta)"
```

---

### Task 3: Makefile signal-eval --benchmark + signal-eval-hk target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 加 SIGNAL_BENCHMARK 变量 + signal-eval 传参 + signal-eval-hk target**

在 `Makefile` 的 `SIGNAL_TO ?= 2026-06-01` 之后加：

```makefile
SIGNAL_BENCHMARK ?= 000300.SH
```

`.PHONY` 行追加 `signal-eval-hk`。

把 `signal-eval` target 的 evaluate 行：

```makefile
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals.csv \
	  --qlib-dir $(QLIB_DIR) --out $(SIGNAL_OUT)
```

改为（追加 `--benchmark`）：

```makefile
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals.csv \
	  --qlib-dir $(QLIB_DIR) --benchmark $(SIGNAL_BENCHMARK) --out $(SIGNAL_OUT)
```

在 `signal-eval` target 之后新增（recipe 行用 **Tab** 缩进）：

```makefile
# 港股事件研究：港股集信号 → 对 atlas_hk 评估，基准恒生指数 ^HSI。
# 港股行情走 yahoo；离线仅 price_percentile/ma_crossover 可回放。
signal-eval-hk: build
	./bin/atlas export-signals --config configs/config.yaml --symbols $(SIGNAL_SYMBOLS_HK) \
	  --strategies price_percentile,ma_crossover --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals_hk.csv
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals_hk.csv \
	  --qlib-dir $(QLIB_DATA_HK_DIR) --benchmark ^HSI --out $(SIGNAL_OUT)
```

- [ ] **Step 2: make -n 校验两 target 语法**

Run: `make -n signal-eval-hk && echo "---" && make -n signal-eval`
Expected:
- `signal-eval-hk` 打印 export-signals(--symbols 港股集 --strategies price_percentile,ma_crossover) + evaluate.py(--qlib-dir .../atlas_hk --benchmark ^HSI)，无 "missing separator"。
- `signal-eval` 打印的 evaluate 行含 `--benchmark 000300.SH`。

- [ ] **Step 3: 提交**

```bash
git add Makefile
git commit -m "build(qlib): signal-eval --benchmark + signal-eval-hk target (atlas_hk/^HSI)"
```

---

### Task 4: 集成 — 港股 signal-eval 产出非空事件研究

**Files:**（无源码改动，端到端验证 + 产物）

> 前置：atlas_hk 数据包已存在（`make qlib-data-hk` 已建）。本环境 yahoo 可达。

- [ ] **Step 1: 生成港股信号**

Run:
```bash
go build -o bin/atlas ./cmd/atlas
SYMS="0700.HK,9988.HK,0883.HK,6886.HK,3288.HK,2800.HK,2828.HK,3033.HK,3181.HK,^HSI,^HSCE"
./bin/atlas export-signals --config configs/config.yaml --symbols "$SYMS" \
  --strategies price_percentile,ma_crossover --from 2021-01-01 --to 2026-06-12 --out signals_hk.csv 2>&1 \
  | grep -vE "FetchHistory failed|trying lixinger" | tail -3
echo "信号数: $(($(wc -l < signals_hk.csv)-1))"
```
Expected: 退出 0，信号数 > 0（数千级）。

- [ ] **Step 2: 对 atlas_hk 评估（基准 ^HSI）**

Run:
```bash
scripts/qlib_eval/.venv/bin/python scripts/qlib_eval/evaluate.py \
  --signals signals_hk.csv --qlib-dir "$HOME/.qlib/qlib_data/atlas_hk" --benchmark ^HSI --out reports/
```
Expected: `报告已写入 reports/signal-eval-YYYYMMDD.md`。

- [ ] **Step 3: 校验报告非空（基准恒生、非全部丢弃）**

Run:
```bash
F=$(ls -t reports/signal-eval-*.md | head -1)
grep -E "基准|benchmark|丢弃|mean_ret|win_rate|价格/基准缺失" "$F" | head -20
```
Expected:
- 基准显示 `^HSI`（非 SH000300）。
- 「丢弃」数远小于信号总数（基准存在 → 不再全部丢弃）；出现 price_percentile/ma_crossover 的 mean_ret/win_rate 表（相对恒生的超额收益）。
- 与修复前对照：修复前同口径全部丢弃（8129/8129）；修复后应有大量非丢弃样本。

- [ ] **Step 4: A股默认基准零回归（单测层面）**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/ -q`
Expected: 全 PASS（默认 benchmark=000300.SH，A股 signal-eval 行为不变；本环境 eastmoney 屏蔽无法跑 cn 全链路，回归由单测覆盖：_parse_args 默认、_meta 默认、benchmark_error 降级）。

- [ ] **Step 5: 清理临时信号文件**

Run: `rm -f signals_hk.csv`
（`signals_hk.csv`/`reports/` 均被 .gitignore 忽略，本任务不提交。）

---

## 自查覆盖

- QlibPriceSource benchmark 参数化（默认 000300.SH） → Task 1
- benchmark() 用 to_qlib_instrument 解析（^HSI→HSI） → Task 1 + Task 4 集成
- evaluate.py --benchmark + _meta 反映 → Task 2
- 消费侧 collect_outcomes 不改（已 agnostic） → 设计已确认，无任务（无需改）
- Makefile signal-eval --benchmark + signal-eval-hk → Task 3
- 港股事件研究非空（基准恒生） → Task 4
- A股零回归（默认基准） → Task 2/4 单测
- 基准取数失败优雅降级（benchmark_error） → 既有逻辑不改，既有 test_report 用例保障
- 美股不在本轮（仅参数化就绪） → 非目标，无任务
