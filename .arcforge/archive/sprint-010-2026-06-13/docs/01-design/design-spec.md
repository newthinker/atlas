# 设计规格 — 美股 signal-eval

> 浓缩自 `docs/plans/2026-06-13-atlas-us-signal-eval-design.md` rev2.1 +
> implementation plan。本文件是 DoD 与验证的设计依据。

## 1. 契约对称规则（Go ↔ Python）

| 输入 | toQlibInstrument / to_qlib_instrument | 说明 |
|---|---|---|
| `AAPL` | `AAPL` | 裸 ticker 恒等 |
| `GOOGL` | `GOOGL` | 5 字符，命中上界 |
| `^GSPC` | `GSPC` | 指数剥离 `^` |
| `^IXIC` | `IXIC` | 同上 |
| `^DJI` | `DJI` | 同上 |
| `AAPL123` | **reject** | 非 `[A-Z]{1,5}` 全串锚定 |
| `AAPL.B` | **reject** | 含 `.`，锚定失败 |
| `aapl` | **reject** | 小写 |
| `TOOLONG` | **reject** | 7 字符超上界 |
| `^HSTECH` | **reject** | 非 HK 已知指数、非 US 白名单 |

- Go：`var usTickerRe = regexp.MustCompile("^[A-Z]{1,5}$")`（包级，Task 1 引入，Task 2 复用）。
- Python：`re.fullmatch(r"[A-Z]{1,5}", symbol)`。
- 插入位置：两侧均在 `^HSCE` 分支之后、`return error`/`raise ValueError` 之前。

## 2. US 市场键控（Go export_ohlcv.go）

```
benchmarkForMarket("us")        → "^GSPC"   （hk→^HSI；default→benchmarkSymbol=CN）
inMarket(sym, "us")             → sym∈{^GSPC,^IXIC,^DJI} || usTickerRe.MatchString(sym)
--market 白名单                  → cn | hk | us（其余 reject："unknown market ... want cn, hk or us"）
--market flag help              → "Market bundle: cn (A-share), hk (Hong Kong) or us (US)"
```

文案收尾（无测试，一致性）：`exportOHLCVCmd.Long`、`--symbols` help 去掉写死的 "A-share"。

## 3. Python region 参数化

```python
QlibPriceSource(provider_uri, start, end, benchmark="000300.SH", region="cn")
  self._region = region
  _ensure_init: qlib.init(provider_uri=..., region=self._region)
evaluate.py: --region default="cn"; main 透传 region=args.region
```
向后兼容：region 缺省 = cn，CN/HK 零变化。构造不触发 qlib（惰性 import 保持）。

## 4. Makefile target（镜像 hk）

```makefile
QLIB_CSV_US_DIR  ?= qlib_csv_us
QLIB_DATA_US_DIR ?= $(HOME)/.qlib/qlib_data/atlas_us
SIGNAL_SYMBOLS_US ?= AAPL,MSFT,NVDA,GOOGL,AMZN,META,JNJ,JPM,^GSPC

qlib-data-us:  export-ohlcv --market us → build_data.py --target-dir atlas_us
signal-eval-us: export-signals → evaluate.py --benchmark ^GSPC --region us
```
守门测试断言：target 存在、`--benchmark ^GSPC`、`--region us`、`atlas_us`、走 `VENV_PYTHON`、
`.PHONY` 首行含两 target。

## 5. config US watchlist

8 个股（AAPL/MSFT/NVDA/GOOGL/AMZN/META/JNJ/JPM，绑 price_percentile+pe_percentile）+
`^GSPC` 指数（仅 price_percentile）。须与 `SIGNAL_SYMBOLS_US` 一致。

## 6. region="us" 风险与降级（设计 §4.1）

端到端 `make qlib-data-us && make signal-eval-us` 须产出非空 markdown 报告（5/20/60 日
相对 ^GSPC 超额）。若 region="us" qlib 报错/读空 → `--region us` 回退 `--region cn`
（HK 先例：cn region 可跑非 cn 包），记录于提交信息。
