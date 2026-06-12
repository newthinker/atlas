# 部署说明 — sprint-003（2026-06-12）

无 DB 迁移、无 atlas 运行时配置变更（评估管线是离线工具链，不影响 serve）。

## 新工具链使用

```bash
make export-signals                 # 导出 signals.csv（默认 600519.SH,000300.SH 2021-2026）
make signal-eval                    # 导出 + 评估 → reports/signal-eval-YYYYMMDD.md
```

## 一次性环境准备

1. Python venv 已在仓库内预置规范（scripts/qlib_eval/.venv，README 有重建命令）；**勿用系统默认 python3**（已知 dyld 损坏）
2. qlib 数据包（评估必需，README 详述）：
   `python -m qlib.cli.data qlib_data --target_dir ~/.qlib/qlib_data/cn_data --region cn`
   缺失时 evaluate.py 会打印此命令并 exit(1)（不静默）
3. pyqlib 仅运行时依赖（pip install pyqlib 或本地副本 -e 安装），pytest 不需要

## 验收待办（数据包就位后人工一次）

- `make signal-eval` 产出完整报告：含 ma_crossover 与 price_percentile 两节、样本数>0、口径与数据缺口说明齐全（final-report 遗留项）

## 回滚

纯增量工具链：删除 scripts/qlib_eval 与 Makefile 两个 target 即完全移除，对 serve/backtest 零影响（引擎盖戳与 ma_crossover 时戳修复保留——它们是正确性修复）。
