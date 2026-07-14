# 独立 reviewer 反审报告（dod-reviewer，只读需求文档）

> 完整原文见本 Sprint 会话记录；此处存档要点与 Leader 裁决对照。

## 反审结论

文档整体清晰、边界克制。发现 1 处自相矛盾 + 2 处易漏阻断点 + 若干需钉死边界。

## Leader 裁决对照表

| 反审发现 | 级别 | Leader 裁决 | 落点 |
|---|---|---|---|
| C.1/B.2 http_error_rate 需 5xx 维度，与 Snapshot"聚合求和"矛盾 | 阻断 | AD-13：status label 数字值额外产出 `<name>_2xx/_4xx/_5xx` 状态类键；runner 用 `_5xx`+基名两键 delta | TASK-201/203 |
| B.1 config.go:419/421/445 引用 Broker.Futu，删结构体编译阻断 | 阻断 | AD-15：live 校验改 Mode==live 直接报错 paper-only；WarnHardcodedSecrets 删 futu 条目 | TASK-101 |
| C.2/B.4 "排序以内存为基准"无确定基准 | 需澄清 | AD-14：定 generated_at ASC, id ASC；memory.List 补显式稳定排序 | TASK-301 |
| B.7 limit=0 语义（memory=不限制，LIMIT 0=空） | 高风险 | AD-14：limit=0=不限制，契约测试钉死；含 offset 越界、from/to 严格开区间、ErrSymbolNotFound sentinel | TASK-301 |
| 4b path 目录不存在未定义 | 含糊 | AD-14：NewSQLiteStore 父目录 MkdirAll，失败返 error | TASK-301 |
| B.5 优雅退出只测启动 | 易漏 | DoD 强化：必须有停止断言（有限时间返回、无泄漏）；Notify 失败不中断循环 | TASK-203 |
| 3b 负增量（计数器重启）/除零/首周期 | 边界 | DoD 强化：clamp 到 0、增量 0 不除零、首周期无基线不产出 | TASK-203 |
| C.3 AlertRule.For duration 解码前提 | 轻微 | 已核实：For 已是 time.Duration，viper 默认 hook 可解 "5m"；映射测试含字符串 duration 用例 | TASK-203 |
| B.3 默认 backend=sqlite 行为变化 | 提示 | 已覆盖（TASK-302 non_functional + PR 描述要求） | TASK-302 |
| B.6 "明确不做"被误实现风险 | 提示 | 已覆盖（任务 description 边界声明）；QA 阶段做反向验收 | QA 检查单 |
| C.4 test-integration 允许限流失败不可作门禁 | 轻微 | 已符合设计（手动冒烟，不进默认门禁） | 整体验收 |

全部阻断/澄清项已裁决并回写任务 JSON（TASK-101/201/203/301），追溯矩阵同步更新。
