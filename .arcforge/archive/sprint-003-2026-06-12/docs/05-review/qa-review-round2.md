# QA Round 2 — 跨视角对抗式 Review (sprint-003)

变更规模 Large（1953 行，跨语言）→ 启用三视角：数据正确性 / 运维 / 防呆。
方法：单 session 内以三套独立 lens 逐项证伪；外部 codex/gemini CLI 未在本环境启用 →
按规范退回纯 Claude 跨视角（已在 verdict 注明能力降级）。

---

## 视角 A：数据正确性

1. **周末跳日 vs DatetimeIndex 对齐** — 无错位。`align_entry`(prices.py:46) 用
   `index.searchsorted(signal_date, side="right")` 在**真实交易日 DatetimeIndex** 上取严格次日，
   不做任何日历日推算；horizon 用 positional `entry.index+h`（event_study.py:78）同样基于交易日序列。
   周末/停牌的跳日由索引本身承载，不存在"+h 天"误算。✅

2. **两套索引（标的 positional vs 基准日期）混用是否错位** — 已正确解耦。
   标的 exit 用 positional `entry.index+h` 得 `prices.index[exit_idx]` 的真实日期，
   再以该 `exit_date` 去基准做 `_last_le`（event_study.py:86）。起点同理用 `entry.date`。
   `test_benchmark_aligns_to_last_available_before_entry` 用"基准缺 entry 日"场景钉死：
   起点取 ≤1/3 的 1/2=3000（非 1/4=3030），错取即翻车。✅

3. **float %.2f round-trip 对收益计算的影响** — **零影响**（关键澄清）。
   收益计算的价格来自 qlib（`source.history/benchmark`），CSV 的 `price` 列**不参与**任何收益数学：
   `collect_outcomes`(evaluate.py:72-77) 仅消费 CSV 的 `date/strategy/action/confidence`。
   故 `%.2f` 价格舍入不影响 return/excess。唯一受 `%.2f` 影响的是 `confidence` 的桶边界（见 R1 SUGGESTION）。✅

   - **[SUGGESTION] event_study.py:86-90** — 若 `exit_date` 超过基准最后日期，`_last_le` 返回基准末行
     （所有日期 ≤ exit_date），静默用陈旧基准收盘而非标记数据缺口；与起点侧的 `<0` 防御不对称。
     正常 CSI300 覆盖全区间，边缘场景，记录备查。

## 视角 B：运维

1. **qlib 目录缺失/部分缺失/symbol 无数据**
   - 全缺失：`main` 启动即 `check_qlib_dir`→打印 `get_data_hint` 下载指引并 `return 1`
     （evaluate.py:113-115），`test_main_exits_when_qlib_dir_missing` 覆盖。✅
   - 单 symbol 无数据：`collect_outcomes` 逐 symbol `try/except`→`data_gaps++` 继续（:67-70）。✅
   - **[WARNING] evaluate.py:56** — `bench = source.benchmark()` **无 try/except**。基准（CSI300）
     数据缺失或 qlib 报错时整跑崩溃抛原始 traceback，与逐 symbol 的友好降级不对称。建议包裹并给出
     "基准数据缺失，无法计算超额"明确提示。

2. **Makefile 无网络 / 无 bin/atlas**
   - `signal-eval: export-signals: build`（Makefile）依赖链正确：build 失败或 atlas 拉数失败
     （无网络）→ make 以非 0 中止，不进入 evaluate。✅
   - 统一走 `QLIB_PY=scripts/qlib_eval/.venv/bin/python`（系统 python3 已损坏），
     `test_signal_eval_uses_venv_python_not_bare_python` 钉死。✅

3. **空信号真实运行** — 见 R1 [WARNING] evaluate.py:122-123，header-only CSV 崩溃。运维态可达。

## 视角 C：防呆

1. **Excel 重存（BOM/引号变化）** — `read_signals` 未用 `utf-8-sig`，BOM 使首列变 `﻿symbol`→
   表头校验失败抛 `ValueError: header mismatch`（已实测）。**失败响亮且信息明确**，可接受；
   - **[SUGGESTION] report.py:20** — 可改 `encoding="utf-8-sig"` 兼容 Excel 往返，提升易用性。

2. **空信号文件（仅 header）** — `read_signals` 正确返回 0 行 DataFrame（实测）；但下游 evaluate
   崩溃（同 R1 WARNING）。读取层防呆到位，入口层缺一道空集短路。

3. **同一信号日重复行** — 不去重，两条均评估并计入聚合。引擎正常不产重复；手改 CSV 时按行计数可辩护。
   非缺陷，记录。

---

## 三视角 high-severity 共识？— 无

- 无任一视角发现会"静默产出错误数字"的 CRITICAL。两个 WARNING（空信号崩溃、基准无 try/except）
  均为**真实运行的降级路径在退化态下崩溃**，而非污染正确结果；三视角对其严重度判断**一致**（非 high、
  但建议修）。无分歧 → 不构成 CONTESTED。

Round-2 结论：**PASS（无 high-severity）**，2 个 WARNING 列为非阻塞建议修复。
