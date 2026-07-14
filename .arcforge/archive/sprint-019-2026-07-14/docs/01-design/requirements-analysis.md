# 需求分析 — 宏观危机监控（Cassandra）

> Sprint 019 · 2026-07-13 · Leader 生成
> 需求源（唯一）：`docs/plans/2026-07-13-macro-crisis-monitor-impl.md`（实施方案）
> 设计文档：`docs/plans/atlas-macro-crisis-monitor-design.md`（v0.2，已经过需求/边界评审，commit 400af4b）

## 降级说明（capabilities）

- ECC 不可用 → 未走 `/multi-plan`。设计 v0.2 已经历独立评审并修订，实施方案明确「执行者不需重新决策」，故**不重新 brainstorm**，直接以实施方案为唯一需求源。
- Go validator（`validator/`）不存在 → Leader 以脚本手工校验任务图（DAG/wave/scope 互斥/单 owner）。
- `arcforge-write.sh` hook 不存在 → `.arcforge/` 状态由 Leader 单写者维护，写入经 `with-task-lock.sh` 临界区；dev/test agent 一律不写 `.arcforge/`。

## 核心功能列表

1. **数据采集**：7 个市场压力指标（vix/move/sofr_effr/hy_oas/t10y2y/nfci/usdjpy）从 FRED + Yahoo 采集，按 canonical units 入 sqlite（新库 `data/crisis.db`）。
2. **FRED 客户端**：新 collector 包（key、指数退避重试、"." 缺失值过滤）。
3. **Yahoo 放宽**：`validSymbol` 正则支持货币符号 `JPY=X`（唯一触碰既有 symbol 的改动）。
4. **配置驱动阈值**：全部阈值进 `configs/crisis-monitor.yaml`，typed struct（偏差 2），代码零阈值数字。
5. **三色规则引擎**：7 指标绝对阈值轨 + 分位轨（≥60 观测，偏差 3），纯函数（SeriesReader 窄接口）。
6. **抑制与防抖**：季末窗口抑制（sofr_effr）、新鲜度 STALE、降级滞回（升级立即/降级需连续）。
7. **四态状态机**：NORMAL/WATCH/BREWING/CRISIS，连续日计数从 `crisis_evaluations` 历史行重建，进程无状态。
8. **单日评估编排**：EvalDay = 规则 → 抑制/防抖 → 状态机 → 8 行 Evaluation（7 指标 + 1 系统行，含 `ts` 数据日列，偏差 1）。
9. **CLI**：`atlas crisis backfill/eval/status/replay`（+ `eval --mode intraday|nfci`），平铺在 `cmd/atlas/crisis.go`。
10. **回测**：replay 共用同一引擎（MemHistory），三段历史验收（2020/2024/2008，误报段 2015–19）。
11. **通知**：telegram 复用，[P0]/[P1]/[P2] 文案前缀（非路由），页脚边界声明，禁止"必然/一定/即将"。
12. **部署**：3 个 launchd plist（daily 多时点/nfci 周三/intraday-jpy 半小时），幂等兜底唤起精度。

## 非功能性需求

- sqlite 固定 `modernc.org/sqlite v1.38.2`；所有 go 命令前缀 `GOTOOLCHAIN=local`。
- 时间列 TEXT：观测日 `YYYY-MM-DD`、时间戳固定宽度 UTC RFC3339（字典序=时间序）。
- 幂等：upsert 主键 (ts,indicator)；当日已评估跳过；intraday 每日一次去重。
- 密钥不入库不入文件：FRED key 走 `configs/config.yaml`（gitignored）/ env `FRED_API_KEY`。
- 覆盖率门禁：changed-package ≥80%（arcforge.config.json）。

## 模糊/缺失需求点（已由方案裁决，无待澄清项）

- 三处偏差（`ts` 列、typed config、分位最小窗 60）已在方案中核实并锁定，执行者不得重新决策。
- 状态机两处语义解读（AMBER 计数含 RED、非色彩退出共振）已写死。
- 人工依赖：FRED key 已配置；HY OAS 2006 起历史 CSV 快照是第二阶段回测验收硬前提（人工提供）。

## 范围外（不得顺手实现）

Grafana 面板、strategy 层联动、notifier 优先级路由公共化、假日日历、FRED collector 注册进 Registry。
