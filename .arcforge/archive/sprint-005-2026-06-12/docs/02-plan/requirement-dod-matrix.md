# 需求 ↔ DoD 双向追溯矩阵 — percentile_step Sprint

> 需求编号源自设计文档（D=design.md，P=implementation plan）。
> DoD 记法：`T{n}.{维度}{序}`，维度 f=functional b=boundary e=error_handling nf=non_functional。

## 正向：需求 → DoD

| 需求 | 来源 | 覆盖 DoD | 设计§6 测试条目 |
|---|---|---|---|
| R1 买入侧步进序列+恢复重算（防死锁） | D§3 | T1.f1 | 1,2 |
| R2 卖出侧对称 | D§3 | T1.f2 | 3 |
| R3 key 独立性（buy/sell、策略独立、strong 同侧共享） | D§2 | T1.f3 | 4 |
| R4 策略级步长优先/回退/类型异常回退/全局0+策略step启用 | D§2 rev4 | T1.f4 | 10 |
| R5 两级 step 禁用、坏分位元数据 → 冷却回退+debug 日志+零回归 | D§5 | T1.b1, T1.b2, T1.nf1 | 5,8 |
| R16 静态过滤通用前置：被抑制分位信号不写门控状态 | D§4（reviewer 反审补充） | T1.f5 | — |
| R6 分位信号不查不更新冷却戳（完全替代） | D§1 | T2.f1（+T1.nf1 后半） | 6 |
| R7 RouteBatch 同判定防旁路 + nil-registry 守卫 | D§4 / P-Task2 | T2.f2, T2.b1 | — |
| R8 Clear 操作同步清理步进状态 | D§4 | T2.f3, T2.f4, T2.nf1 | 7 |
| R9 GetStats 回显门控状态 | D§4 | T2.f5 | — |
| R10 策略 Init 读 params 并经 Metadata 携带（端到端，两策略） | D§2 rev4 | T3.f1/b1/b2, T4.f1/b1/b2 | 10 |
| R11 config 字段 + 负值校验 + 默认 0 | D§2 | T5.f1, T5.b1, T5.e1 | — |
| R12 app.New() 接线修复死配置 bug（含行为变更注记） | D§2 装配点 | T6.f1-f4, T6.nf1 | 9 |
| R13 cooldown_hours: 0 = 禁用冷却 | D§2 | T6.b1 | — |
| R14 配置文件交付更新（watchlist + example） | D§7 | T7.f1-f3 | — |
| R15 判定与状态写入单临界区原子性 | D§4 | T1.nf1 | — |

**孤儿需求检查：无**（设计 §6 全部 10 条测试均有 DoD 对应；§5 边界表 5 行分别由 T1.b1/T1.b2/T1.f3/T1.f3/范围注记覆盖）。

## 反向：DoD → 需求

| DoD | 对应需求 |
|---|---|
| T1.f1-f5, b1-b2, nf1 | R1-R5, R15, R16 |
| T2.f1-f5, b1, nf1 | R6-R9 |
| T3.\*, T4.\* | R10 |
| T5.\* | R11 |
| T6.f1-f4, b1, nf1-nf2 | R12, R13 |
| T7.f1-f3 | R14 |
| T7.nf1（code-simplifier） | 全局规范（~/.claude/CLAUDE.md 提交前必跑）+ 计划执行纪律 |
| T7.nf2（vet/test/gitnexus） | 计划 Task 5 Step 3 + 项目 CLAUDE.md gitnexus 规范 |
| 各任务零回归条目 | D§5「与现状完全一致」+ 计划验收对照 |

**凭空 DoD 检查：无**（T7.nf1/nf2 锚定全局与项目级规范，非凭空）。

## 任务图手工校验（validator 降级，见 ADR-7）

| 规则 | 结果 |
|---|---|
| DAG 无环 | ✅ 001→002→006→007；005→006；003/004→007 |
| wave 序（task.wave > max(依赖.wave)） | ✅ w1:{001,003,004,005} w2:{002} w3:{006} w4:{007} |
| scope 非空且同 wave 两两互斥 | ✅ router / price_percentile / pe_percentile / config 互斥；002 与 001 同包但跨 wave（dag 就绪条件保证不同时在途） |
| context_from 闭合（⊇ dependencies，引用均存在） | ✅ |
| epoch / rework / questions 初始不变量 | ✅ 全部 0 / 0 / [] |
| 完成必有产物、失败必有原因 | N/A（全部 pending） |

## verify_by 分布提示

T7 的 review/manual 占比偏高（5 条中 3 条 review）——属配置交付与收尾性质，符合预期；其余任务均以 test 为主，适合全自动流程。
