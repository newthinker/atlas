# 需求 ↔ DoD 追溯矩阵(M1.5 AKShare)

> 计划 Task N ↔ TASK-00N 一一映射。机器检查: 无孤儿需求、无凭空 DoD。

| 需求条目(计划/spec 出处) | DoD 落点 |
|---|---|
| T1 配置默认值/标签/显式保持 | TASK-001 functional×2+boundary×1 |
| T2 lg 接口/映射/窗口过滤/字段解析/NaN/HTTP错误/market 校验 | TASK-002 全部 7 条 |
| T3 HK 双指标合并/指数中文键/映射错误/单边缺失 | TASK-003 全部 7 条 |
| T4 增量/本地分位算法/分派/空拉取/签名变更涟漪/NaN 剔除 | TASK-004 全部 8 条 |
| T5 降级三态(Degraded/双败/零多余请求) | TASK-005 全部 6 条 |
| T6 输出格式/Degraded 告警/退化打印/warn 语义 | TASK-006 全部 6 条 |
| T7 四产物 review + M1.5 验收 manual×4 | TASK-007(无代码任务,全对象形态) |
| 全局: live 校验点标注 | TASK-002/003 non_functional(review) |
| 全局: Refresh 签名变更全仓可编译 | TASK-004 non_functional(test) |
| spec 明确不做清单 | 无任务违反(反向核对) |

## Realistic Scope 偏差(如实)

- TASK-004 跨 2 包(internal/prism + cmd/atlas 签名涟漪,计划原文如此)——串行链上无并发冲突;
- TASK-002/003 同包但 T3 依赖 T2 强制串行,in-flight 互斥成立;
- TASK-007 纯产物,全对象形态 verify_by review|manual(无代码任务声明)。

## M1 经验前置吸收

- Context Checkpoint 注释直接入 T2/3/4 review 条目(M1 reviewer 必改项模式);
- 测试断言必须实覆盖参数与返回值(M1 两次 reject 同型教训已写进计划测试代码);
- AD-6 预判: T4/T6 触碰 cmd/atlas 整包口径可能 DENY,按既有处置模板。
