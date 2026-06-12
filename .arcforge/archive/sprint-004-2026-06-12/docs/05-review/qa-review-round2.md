# QA Code Review — Round 2 (跨视角对抗式验证)
sprint: sprint-004 | reviewer: qa-agent-1 (authoritative adjudication) | rounds: 2
scope: cmd/atlas/export_ohlcv.go(+_test.go), scripts/qlib_eval/build_data.py(+tests/), Makefile, README.md

> 本文件由 qa-agent-1 在三视角对抗审查后**亲自裁定重写**。第一版 round2 仅含
> Minimalist 视角，遗漏了 team-lead 明确要求的「数据正确性 / 运维 / 防呆」三视角；
> 且并行 Architect 视角产出 (code-review-qa-agent-1.md) 给出 2 个 CRITICAL，与当时
> qa-verdict.md 的 PASS 自相矛盾。下方为逐条取证后的统一裁定。

---

## 一、数据正确性视角

### DC-1 [SUGGESTION] 前复权价跨日重建数值漂移，README 复权口径未披露
- 文件: README.md:59-63（复权口径节）
- 事实: `make qlib-data` 每次对 [SIGNAL_FROM, today] **全量**重拉 eastmoney fqt=1 前复权。
  前复权基准是「最新一次除权」——当出现**新除权事件**时，全部历史 bar 的前复权值会
  整体平移。因此「昨天的数据包」与「今天的数据包」对**同一历史日期**的 open/close
  可能不同。后果: (a) 评估结果跨重建日不可复现; (b) 若信号生成日与评估日之间发生
  除权，信号当时所见价格与评估时所见价格口径不同（虽两者都来自同一 FetchHistory，
  但取数时刻不同）。
- 取证: E2E 实跑 D.features 逐值匹配源 CSV（同一时刻自洽，**当前结果正确**）；该问题
  是跨时刻语义，非当前数值错误。
- 建议: README「复权口径」补一句披露「每次重建以当时最新除权为基准，历史前复权值
  会随新除权平移；评估结果应理解为某一重建快照下的口径」。非阻塞。

### DC-2 [INFO] factor=1 假设的破裂边界
- 文件: export_ohlcv.go:30,138; prices.py（评估侧不乘 $factor）
- 裁定: 当前事件研究链路内 factor=1 **自洽且正确**——价格源头已前复权、评估端从不乘
  $factor。破裂仅发生在未来若有人 (a) 混入未复权价、或 (b) 引入依赖 qlib $factor 语义
  的跨标的算子时。属设计假设记录，非缺陷。

---

## 二、运维视角

### OPS-1 [WARNING] 无原子换包：dump_bin 中途崩溃留下混龄 bundle
- 文件: build_data.py:main()(137-158), Makefile:41-43
- 事实: `dump_all` 直接就地写 atlas_cn。若中途崩溃（OOM/磁盘满/Ctrl-C），部分 instrument
  的 .bin 为本次新值、其余为上次旧值。
- 取证（降级理由，非 CRITICAL）:
  - dump_bin ALL_MODE 对每个 .bin 用 `tofile(path)` **截断重写**（dump_bin.py:269，append
    仅 UPDATE_MODE），故**重跑不会翻倍**——二次 make qlib-data 后 close.day.bin 仍 5272B、
    calendar 仍 1317 行（实测）。
  - dump_bin 非 0 → build_data RuntimeError → make **响亮失败**（非 0 退出，cron 2>&1 可捕获）。
  - cron 只跑 `make qlib-data`，**不自动接 signal-eval**；混龄包仅在「用户在下次成功重建前
    手动跑 signal-eval」时被静默读到。
- 建议（加固，fast-follow）: build_data 写临时目录 → verify_bundle → 成功才 `shutil.move`
  到最终路径。非阻塞。

### OPS-2 [WARNING] DEFAULT_QLIB_SCRIPTS 硬编码绝对路径
- 文件: build_data.py:17 `= "/Users/zuowei/workspace/python/qlib/scripts"`
- 事实: 任何第二台机器 / CI 静默落到不存在路径，失败为 subprocess 启动时
  FileNotFoundError，错误信息不指向根因。
- 建议: 默认改 `os.environ.get("QLIB_SCRIPTS_DIR")`，缺失时 raise 明确 ValueError。
  当前由 Makefile 不覆盖该参数、本机存在该路径而被掩盖。非阻塞（单机已交付路径可用）。

### OPS-3 [SUGGESTION] 无 warm-up lead-in：SIGNAL_FROM 同时充当数据起点与信号起点
- 文件: Makefile:7,41; README:153-155
- 事实: horizon-60 在区间头部缺前置数据、尾部（近 today）丢 horizon 窗口。README 已注明
  「越界计 NA」，但 build 阶段无提示。实测 report: horizon60 NA=85，属预期。
- 建议: build_data main() 当 end-start < ~365d 时打印 warning。非阻塞。

---

## 三、防呆视角

### FP-1 [WARNING] qlib_csv 残留旧符号 CSV 静默进入新包；verify_bundle 缺 ⊇ 校验
- 文件: export_ohlcv.go:76（仅 MkdirAll，不清目录）, build_data.py:123-125
- 事实: out-dir 跨运行复用，`os.Create` 只覆盖本次写的文件。SIGNAL_SYMBOLS 删一个符号后，
  旧 per-instrument CSV 残留，被 dump_bin 收入 atlas_cn。verify_bundle 只查
  `expected - instruments`（⊆ 方向），多出的杂散 instrument 静默通过。
- 取证（降级理由，非 CRITICAL）: evaluate.py:71 `for symbol, grp in signals.groupby("symbol")`
  ——评估**只遍历 signals 内符号（=SIGNAL_SYMBOLS）**，不遍历 bundle instruments。故杂散
  instrument 对评估结果**完全惰性**（不产生错误结果，仅多占空间 + 误导人工巡检）。
- 建议: Makefile recipe 前置 `rm -rf $(QLIB_CSV_DIR) && mkdir -p`，或给 export-ohlcv 加
  --clean-out-dir；并给 verify_bundle 补 ⊇ 反向校验。非阻塞。

### FP-2 [SUGGESTION] 并发 make qlib-data 无锁
- 文件: Makefile:41-43
- 事实: 两次并发共写 qlib_csv 与 atlas_cn，无锁 → 可能交错损坏。
- 取证: cron 示例为单条 16:30 周一至周五（单调度），并发需人工双跑，概率低。
- 建议: 可选加文件锁；当前低风险。非阻塞。

---

## 四、Minimalist 视角（代码洁净度，均非阻塞）

| # | 级别 | 文件:行 | 描述 |
|---|------|---------|------|
| M-1 | SUGGESTION | export_ohlcv.go:225 | `loadConfigOrDefaults` 单调用点提取造成假共享（serve.go/broker.go 同模式内联） |
| M-2 | SUGGESTION | export_ohlcv.go:214 | `requireBenchmark` 仅在 --symbols flag 路径有效（watchlist 路径已内置基准），可内联 |
| M-3 | SUGGESTION | export_ohlcv_test.go:32 | `makeOHLCVBars` 与 `makeBars` 周末跳过逻辑 ~60% 重复（有正当理由：需非零 OHLCV 防列错位） |
| M-4 | SUGGESTION | build_data.py:83 | `date_span_from_csvs` 绕过自定义 `_csv_data_rows`，手切 lines[1:] |
| M-5 | SUGGESTION | export_ohlcv.go:257 | 300ms 常量埋在匿名 lambda，宜命名 `politenessDelay` |
| M-6 | SUGGESTION | README.md:59,145 | factor=1/fqt=1 说明两节重复，第二处可改交叉引用 |

---

## 五、Skeptic 视角（逻辑/边界，已取证无 CRITICAL）
- 日期 min/max 字符串比较: YYYY-MM-DD 字典序 == 时间序，正确（build_data.py:92,130）。
- 基准空 bars: export_ohlcv.go:88-90 `len(bars)==0 → err`，基准走 92-93 硬错误，覆盖。
- %.2f 价格 / 整数 volume: golden 逐字节锁定，列序防错位（export_ohlcv_test.go:110-114）。
- Go/Python 契约: toQlibInstrument 与 to_qlib_instrument 同样本同结果（已逐样本核对，含 5 类非 A 股拒绝）。
- 结论: 无 skeptic 级 CRITICAL/WARNING。

---

## 严重度汇总（取证后裁定）
- CRITICAL: **0**（Architect 初判 F1/F2 两 CRITICAL 经取证降级为 OPS-1 / FP-1 WARNING）。
- WARNING: 3（OPS-1 原子换包、OPS-2 硬编码路径、FP-1 残留 CSV）——**均非阻塞，建议 fast-follow 加固**。
- SUGGESTION: 8（DC-1, OPS-3, FP-2, M-1..M-6 合并计）。
- 设计闪光点（保留）: 依赖注入 + 逐字节 golden、基准硬错误置于核心层、三形式符号契约跨语言测试、
  verify_bundle 只读且有 digest 测试、C1-1 显式 --symbols + Makefile 测试、cron 失败响亮。
