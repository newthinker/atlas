# 需求分析 — Notifier 接线修复 Sprint（sprint-006）

> 需求/设计：`docs/plans/2026-06-12-notifier-wiring-implementation.md`（含现状事实核实、设计决定、测试清单）。
> 来源：sprint-005 部署验证发现的预存死配置 bug（运行时实证：22 条 routed 全部 notifiers=0）。
> ECC 不可用降级：需求由 Leader 实地侦察后撰写（构造器签名/装配点/config 字段均已核实），无歧义点，不再 brainstorming。

## 核心功能

| # | 功能 |
|---|---|
| F1 | serve 启动注册 enabled 的 telegram/email/webhook 通知器（registerConfiguredNotifiers） |
| F2 | 必填字段缺失/未知类型/重名注册 → warn+跳过，不阻断启动 |
| F3 | 可观测性：逐个注册 info、总数 info、enabled>0 但注册 0 → 静默失效 warn |
| F4 | config.example.yaml notifiers 节补必填字段注释 |

## 非功能

- 单测不外发网络（构造即注册，不调用 Send）。
- 既有 cmd/atlas 用例零回归；变更包覆盖率 ≥80%。

## 模块与依赖

cmd/atlas（F1-F3）→ configs（F4，依赖 F1 定稿的必填字段语义）。notifier/config/app 三包零改动。
