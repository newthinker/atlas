# TASK-004 复验报告（review_fix 轮）— FP-1 + DC-1

- **验证者**: test-agent-1 (Reality Checker)
- **commit**: 14cb5e5（review_fix；首轮 36d476d 已 verified）/ **package**: ./scripts/qlib_eval
- **rework_count**: 1 / **epoch**: 2
- **判定**: ✅ **VERIFIED**（两 fix_items 均有真实测试佐证 + 原 DoD 回归全绿）

## 一、fix_items 复验（聚焦项）

### ① FP-1 — 残留 CSV 防呆（WARNING）
- **实现**：`build_data.verify_csv_dir_symbols(csv_dir, expected_instruments)` 在 dump 前用**外部预期符号集**（`--expected-symbols` 经 `to_qlib_instrument` 转 instrument）独立比对磁盘 `*.csv` 文件集合，`actual - wanted` 非空即 `raise ValueError`（含多余符号名 + 清理提示）。main 流程在 `run_dump_bin` 之前调用；Makefile qlib-data 传 `--expected-symbols $(SIGNAL_SYMBOLS)`。
- **根因修复确认**：QA 指出原 main 从 csv 文件名反推 `expected`，旧 CSV 静默混入且 verify_bundle（⊆ 方向）也通过。新实现改用**外部清单**独立校验，方向正确。
- **真实测试（4 个，全 PASS）**：
  | 测试 | 断言 | 判定 |
  |---|---|---|
  | test_verify_csv_dir_symbols_passes_exact | 文件集=预期 → 不抛 | PASS |
  | test_verify_csv_dir_symbols_rejects_extra | 残留 sh000001.csv → ValueError 含 "SH000001" **且** 含 "清理" | PASS（fix_item ① 原文「raise 且消息提示清理」精确命中） |
  | test_main_expected_symbols_mismatch_aborts_before_dump | 残留 → main raise 且 **run.assert_not_called()** | PASS（**真实路径断言**：dump subprocess 从未被调用，非空洞断言） |
  | test_main_expected_symbols_match_proceeds | 文件集=预期 → run/verify_bundle each called once, rc==0 | PASS（防 happy-path 误报） |
- 实跑：`pytest test_build_data.py -k 'csv_dir_symbols or expected_symbols'` → **4 passed**

### ② DC-1 — README 前复权跨日漂移披露（SUGGESTION）
- README「### 复权口径」节新增「**前复权跨日漂移（重要披露）**」一段（scripts/qlib_eval/README.md:64-70）：阐明每日全量重建后新除权事件会令历史前复权值整体平移、跨日同日数值可能不同、横向对比不受影响、需可复现绝对值则固定快照勿每日重建。`grep '前复权跨日漂移'` = 1 命中。与 QA round2 DC-1 诉求逐点对应。

## 二、原 DoD 快速回归（fix 仅加 guard+README，未触 e2e 生成路径）

| # | 标准 | 证据 | 判定 |
|---|---|---|---|
| functional[0] | Makefile QLIB_DIR=atlas_cn + test_makefile 断言 | test_makefile.py **6 passed**（含 QLIB_DIR atlas_cn + qlib-data flags） | PASS |
| functional[1] | e2e make qlib-data 产出 atlas_cn（instruments/calendar 校验） | `~/.qlib/.../atlas_cn/instruments/all.txt` = SH000300+SH600519（tab 三字段大写）；calendar 2021-01-04~2026-06-12 | PASS（产物存在性回归） |
| functional[2] | D.features(SH600519) 首尾逐值一致 | 首轮 live 实证逐值全等（2021-01-04 open1785.77/close1782.79、2026-06-12 open1271.18/close1288.91）；本轮 fix 未触数据路径，bundle 产物在 | PASS（回归，无代码路径变更） |
| functional[3] | make signal-eval 报告含 ma_crossover+price_percentile 非空表 | reports/signal-eval-20260612.md：信号总数 1457；ma_crossover 6 数据行 + price_percentile 36 数据行 | PASS |
| boundary[0] | go build/vet/test ./... + hook 同款 pytest 全绿 | go build rc=0 / vet 干净 / test ./... 全 ok（cmd/atlas 含）; **pytest 48 passed**（44+4 FP-1） | PASS |
| non_functional[0] | README 五要素（review） | 建包用法(L39/45)/复权口径含fqt=1 factor=1+DC-1(L59-70)/crontab(L83)/evaluate 直调需--qlib-dir(L92-102)/社区vs自建对比+QLIB_DIR切换(L71-81) 全在 | PASS |

## 三、Reality Checker 取证小结
- FP-1 四测试无 fantasy assertion：rejects 断言多余符号名+清理提示，mismatch 用 `run.assert_not_called()` 实证「dump 前中止」，match 用 `assert_called_once()` 实证不误报——覆盖拒绝/中止/放行三态。
- build_data 新增 `from qlib_eval.symbols import to_qlib_instrument` 经冒烟确认零 qlib 依赖、仓库根可导入（import OK）。
- e2e 三条为产物存在性回归（团队约定）：fix commit diff 仅 Makefile/README/build_data.py guard+test，未改 export-ohlcv 数据生成或 evaluate 路径，bundle 与报告产物齐全且非空。

## 结论
两 fix_items 均以真实有意义测试/披露落实，原 DoD 6 项回归全绿（go 全量 + pytest 48 + e2e 产物在）。**判定 VERIFIED。**
