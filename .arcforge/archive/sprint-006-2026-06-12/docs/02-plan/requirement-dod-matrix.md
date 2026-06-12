# 需求 ↔ DoD 追溯矩阵 — Notifier 接线 Sprint（sprint-006）

## 正向

| 需求 | 来源 | 覆盖 DoD |
|---|---|---|
| R1 启动注册 enabled 通知器（**三类成功路径各有正向用例**+RegisterNotifier） | 计划 §设计1-2 | T1.f1(telegram), T1.f2(email), T1.f3(webhook/headers nil/双注册/逐条 info) |
| R2 runServe 实际调用（装配点正确） | 计划 §设计1 | T1.f4（verify_by review） |
| R3 必填缺失逐字段/未知 key/重名 err → warn+跳过不阻断 | 计划 §设计2-3 | T1.b2, T1.e1 前半 |
| R4 disabled/空配置零注册不 panic | 计划 §测试3,7 | T1.b1 |
| R5 静默失效 warn（enabled>0 注册 0） | 计划 §设计4 | T1.e1 后半 |
| R6 单测无网络外发、零回归、覆盖率（覆盖率源自团队规范） | 计划 §测试 + config | T1.nf1 |
| R7 example 配置必填注释 | 计划 §交付物 | T2.f1 |
| R8 **端到端验收：webhook→本地 httptest，routed notifiers:1 + payload 实收** | 计划 §端到端验收 | T2.f2（verify_by manual，Test Agent 执行） |
| R9 收尾纪律（code-simplifier/vet/test/gitnexus） | 全局规范 | T2.nf1, T2.nf2 |

**孤儿需求：无**（reviewer 反审后补齐 email 成功路径、重名降级、逐条 info、E2E 四处遗漏与三处边界缺口）。
**凭空 DoD：无**（覆盖率条目已注明团队规范来源）。
**reviewer 反审处置记录**：NEEDS_WORK → 4 遗漏 + 1 不可测（f4 改 review）+ 3 边界缺口全部修订；独立清单 18 条全覆盖。

## 任务图手工校验（validator 降级）

DAG：001→002 无环 ✅；wave 序 1<2 ✅；packages 互斥非空（cmd/atlas / configs）✅；context_from ⊇ deps ✅；epoch/rework/questions 初始不变量 ✅。
