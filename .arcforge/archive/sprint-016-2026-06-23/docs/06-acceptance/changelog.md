# Changelog — IC/IR Eval Pipeline

## feature/ic-ir-eval-design (2026-06-23)

### Added
- `scripts/qlib_eval/qlib_eval/ic.py`：时序 IC 计算核心（纯 pandas，零 qlib）
  - `HORIZONS=(5,20,60)`、`forward_returns`（next-open 前向收益）
  - `instrument_ic`（单标的时序 IC + t_stat + 非重叠 t_stat）
  - `ic_summary_by_instrument`、`watchlist_summary`（ICIR/广度）
- `scripts/qlib_eval/qlib_eval/baseline.py`：oracle + reversal baseline 因子 + load_prices（惰性 qlib）
- `scripts/qlib_eval/ic_evaluate.py`：CLI 入口 + collect_ic（可注入价格源）
- `Makefile`：`signal-ic`、`baseline-scores` 两个 target
- 测试：test_ic.py / test_baseline.py / test_ic_report.py + test_makefile.py 增量（共 117 passed）

### Changed
- `scripts/qlib_eval/qlib_eval/report.py`：增量加 `read_scores`（scores.csv 严格校验）、`render_ic_report`（markdown，含重叠 t-stat 告诫 + NaN 守护）
- `docs/ops/qlib-warehouse-runbook.md`：加「时序 IC 评估（方向②前置）」章节 + 读数告诫

### Notes
- 不改动 event_study.py / 既有 signal-eval 管线 / evaluate.py。
- 已知约束（推迟）：重复 (date,symbol) 不去重——留待方向② sidecar 集成处理。

### Commits
2ae38c7 → a9efe85 → a3e5269 → d43354a → 013c588 → 99ad940 → 242d05c → cffde70
