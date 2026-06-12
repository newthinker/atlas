# QA 聚焦复审（Round 3）最终裁定 — sprint-004
reviewer: qa-agent-1（本人直接审查，只读 lens，未 spawn 子代理） | date: 2026-06-12
scope: commit 14cb5e5（TASK-004 review_fix：FP-1 残留 CSV 防呆 + DC-1 前复权跨日漂移披露）

## VERDICT: PASS

仅复审 FP-1/DC-1 闭合 + 无回归（不重复 round1/2 全量）。前序结论见 qa-verdict.md。

---

## ① FP-1 闭合 — PASS（独立复现确认）
- 修复：`build_data.verify_csv_dir_symbols(csv_dir, expected_instruments)` 在 dump 前用
  **外部预期符号集**独立比对磁盘 .csv 文件集合（`actual - expected`，即我此前指出缺失的 ⊇ 方向），
  发现残留即 raise。`main` 增 `--expected-symbols`（atlas 形式经 to_qlib_instrument 转 instrument，
  不传则跳过保持向后兼容）；Makefile qlib-data 传 `--expected-symbols $(SIGNAL_SYMBOLS)`。
- **独立复现（Leader 指定的反例）**：构造 csv 目录含 sh600519(预期)+sh000001(残留)，
  跑 `build_data.py --expected-symbols 600519.SH,000300.SH`：
  - 抛 `ValueError: ... 含非预期符号的残留 CSV ['SH000001'] ...；请先清理（rm .../* 后重跑）`
  - **raise 早于任何 dump_bin 调用**：目标 bundle 目录未被创建（实测 `/tmp/fp1test/bundle` 不存在）。
  - 消息指明多余符号名 + 给出清理指引。✓
- 反向（happy path）无误报：真实 `make qlib-data`（qlib_csv 恰为预期 2 符号）正常构建
  2 instruments、区间 [2021-01-04, 2026-06-12]，无误拦。✓
- 单测 +4（test_verify_csv_dir_symbols_passes_exact / _rejects_extra /
  _main_expected_symbols_mismatch_aborts_before_dump / _match_proceeds）均覆盖该语义。

## ② DC-1 闭合 — PASS（表述准确）
README 复权口径节新增「前复权跨日漂移（重要披露）」，准确表述：
- 前复权价以「最新一期」为基准回溯调整；每日全量重建后，新除权除息 → 历史区间前复权值**整体平移**；
- 同一历史日期 open/close 在今/昨数据包里可能不同 → 跨日两份报告同一信号绝对收益小幅差异，
  属固有特性非数据错误；
- **横向（同一份数据包内）对比结论不受影响**；如需可复现绝对数值，固定某次构建快照、勿每日重建。
表述与我 DC-1 原始发现一致、措辞准确、含可操作建议。✓

## ③ 无新问题 — PASS
- pytest（hook 同款）：**48 passed**（原 44 + FP-1 4），零回归。
- go build ./... / go vet ./cmd/atlas：ok。
- 零 qlib 依赖门禁完好：新增 `from qlib_eval.symbols import to_qlib_instrument` 指向纯字符串
  映射模块（symbols.py 无任何顶层 import），守门用例 `assert "qlib" not in sys.modules` 仍随 48 passed 通过。
- 向后兼容：--expected-symbols 缺省 "" 时跳过校验，直接调用方不受影响。
- 边界核查：若预期符号的 CSV 缺失（actual ⊂ wanted），export-ohlcv 早已降级非 0 退出、
  make 在 build_data 前即停，故不会以缺符号进 dump——无新增 missing 方向缺口。

---

## 结论
FP-1（防呆残留 CSV）与 DC-1（前复权跨日漂移披露）均已**真实闭合**，修复未引入新问题。
sprint-004 review_fix 复验通过。OPS-1/OPS-2 仍按 CARRYOVER 处理（不在本轮范围）。
建议 Leader 推进 TASK-004 verified→（final-report）→accepted 终态。
