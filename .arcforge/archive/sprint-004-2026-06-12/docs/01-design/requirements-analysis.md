# 需求分析 — sprint-004 自建 qlib 数据包（2026-06-12）

**需求源**: docs/plans/2026-06-12-qlib-data-bundle-implementation.md（rev4，plan 评审环 3 轮两 Chunk Approved）
**设计依据**: docs/superpowers/specs/2026-06-12-qlib-data-bundle-design.md（rev4，spec 评审环 4 轮 Approved，用户已批准）
**规划模式**: spec+plan 双定稿（superpowers brainstorming→writing-plans 全流程产物），本阶段仅做任务图重组。

## 目标

`make qlib-data` 从 atlas 采集器构建 qlib 数据包（atlas_cn），使 `make signal-eval` 默认 2021-2026 区间产出非空评估——解决社区包截止 2020-09 的过旧问题（sprint-003 验收实测 1457 信号全丢）。核心原则：**评估数据与信号生成数据同源**。

## 需求清单

| ID | 需求 | plan Task | scope |
|----|------|-----------|-------|
| R1 | export-ohlcv 核心（qlib CSV 约定/符号三形式契约/基准与失败语义/默认集 resolver） | T1 | cmd/atlas |
| R2 | cobra 接线 + Makefile qlib-data + test_makefile 泛化 | T2 | cmd/atlas + scripts/qlib_eval |
| R3 | build_data.py（dump_bin 编排 --data_path/--exclude_fields + 产物校验 + 日期推导） | T3 | scripts/qlib_eval |
| R4 | QLIB_DIR 切换 + README/crontab + e2e 验收 | T4 | scripts/qlib_eval（+Makefile/README） |

## 已钉死口径（spec/plan 结论，Dev 不得重新决策）

fqt=1 前复权 + factor=1；`--data_path`（非 csv_path）+ `--exclude_fields symbol,date`；符号三形式（000300.SH → SH000300 → sh000300.csv）契约锚定 symbols.py；Makefile 只传 --from + --symbols $(SIGNAL_SYMBOLS)；基准缺失/失败硬错误（CLI 层校验清单含基准，核心层纯执行）；300ms 礼貌延迟；build_data 校验不重写 instruments/calendar；instruments 为 tab 三字段大写格式。

## 环境事实（沿用 sprint-003 沉淀）

- Python 一律 scripts/qlib_eval/.venv/bin/python；hook 从仓库根跑 pytest（conftest.py 已存在）
- qlib 已装（pyqlib 0.9.8.dev31 / pandas 2.3.3）；真实数据包 e2e 本 Sprint **必跑**（与 003 不同——qlib cn_data 与采集网络均已验证可用）
- golden 不可用既有 makeBars（O/H/L/V 全零，plan 评审 C1-2）

## 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| scope 交叠严重（cmd/atlas×2 + scripts/qlib_eval×3） | 低 | 任务依赖链串行（见 design-spec 任务图） |
| e2e 依赖外网拉行情 | 低 | sprint-003 验收已实跑通同链路；失败时按降级语义报错不静默 |
