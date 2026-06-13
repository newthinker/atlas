# 需求分析 — signal-eval 基准参数化（支持港股）

## 来源
计划 `docs/superpowers/plans/2026-06-13-signal-eval-benchmark-param.md`；
设计 `docs/superpowers/specs/2026-06-13-signal-eval-benchmark-param-design.md`。

## 问题
signal-eval 事件研究基准硬编码 SH000300（prices.py QlibPriceSource.benchmark），对 atlas_hk
评估时基准缺失 → 全部信号丢弃（实测 8129/8129）。

## 目标
给 signal-eval 加 `--benchmark` 参数（atlas 形式，默认 000300.SH），启用港股（atlas_hk + ^HSI）；
A股路径零回归。美股推迟另开一轮（参数化天然就绪，不建 atlas_us）。

## 复杂度
简单。单一 Python 模块为主（prices.py + evaluate.py）+ Makefile + 集成验证，4 任务线性。

## 技术要点
- benchmark 用 atlas 形式，QlibPriceSource 内经 to_qlib_instrument 转 qlib 形式。
- 消费侧 collect_outcomes 已 benchmark-agnostic（调 source.benchmark()），无需改。
- export-signals 离线仅 price_percentile/ma_crossover；港股行情走 yahoo。

## 边界
默认 000300.SH 零回归；基准取数失败沿用 benchmark_error 优雅降级；不支持的基准符号
（如 ^GSPC 本轮）→ ValueError 被既有 try/except 捕获。
