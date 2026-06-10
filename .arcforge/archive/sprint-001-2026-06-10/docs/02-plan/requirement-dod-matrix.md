# 需求 ↔ DoD 追溯矩阵

**生成**: 2026-06-10 | **需求源**: docs/reviews/2026-06-03-project-status-and-optimization.md §五

## 正向：需求 → 任务 → DoD 覆盖

| 需求 | 子项 | 任务 | 覆盖 DoD（摘要） |
|------|------|------|------------------|
| R1 M4 paper 闭环 | paper 券商实现 | TASK-001 | Broker 接口实现/成交语义/资金头寸演化/并发安全 |
| R1 | 信号→执行接线点 | TASK-002 | SignalExecutor 接口/路由后提交/错误不中断 |
| R1 | serve 接线 + 适配 | TASK-003 | paper 模式构造链/Signal→OrderRequest/futu 维持现状/enabled=false 无副作用 |
| R2 并行化 | 并发配置 | TASK-004 | analysis.workers 读取与默认值 |
| R2 | worker pool | TASK-005 | workers>1 并行/<=1 串行兼容/单标的失败隔离/-race |
| R2 | LLM 仲裁超时 | TASK-004 + TASK-005 | arbitrator.timeout 配置；WithTimeout 包装/超时降级原信号 |
| R3 OHLCV 缓存 | 缓存配置 | TASK-004 | collector.cache.{enabled,ttl} 读取与默认值 |
| R3 | 装饰器实现 | TASK-006 | TTL 命中/key 区分/副本/容量上限/错误不缓存/-race |
| R3 | 接线 | TASK-007 | enabled 包装/disabled 不包装/扩展接口不破坏/selector 透明 |
| R4 采集器覆盖 | eastmoney | TASK-009 | 注入重构不变行为/Quote+History 解析/HTTP+JSON 错误/≥80% |
| R4 | lixinger | TASK-010 | 同上 + Fundamental 路径/≥80% |
| R4 | yahoo | TASK-011 | 同上/≥80% |
| R5 backtest CLI | 引擎接线 | TASK-008 | 真实引擎运行+统计输出/策略不存在/日期非法/空数据/拉取失败 |

## 反向：任务 → 需求（凭空 DoD 检查）

| 任务 | 对应需求 | 判定 |
|------|----------|------|
| TASK-001..003 | R1 | ✅ |
| TASK-004 | R2 + R3（配置聚合，单 package 原则） | ✅ |
| TASK-005 | R2 | ✅ |
| TASK-006..007 | R3 | ✅ |
| TASK-008 | R5 | ✅ |
| TASK-009..011 | R4 | ✅ |

## 机器检查结论

- **孤儿需求**: 无。R1-R5 均有任务与可测 DoD 覆盖。
- **凭空 DoD**: 无。全部 DoD 可回溯到 R1-R5 或其实现必要前提（如 TASK-002 接口解耦源自 R1 的架构决策 ADR-2）。
- **范围外确认**: FutuBroker 真实实现、live 下单、执行确认 UI 明确不在本 Sprint（ADR-7），需求文档 §五.5 的「实现 FutuBroker **或**先启用 paper-trading 模式」取后者，符合原文。
- **verify_by 分布**: test 39 条 / review 0 / manual 0 —— 全部可自动验证。
- **独立 reviewer 反审（2026-06-10）**: 初判 NEEDS_REVISION。已采纳修订：TASK-003 增加端到端链路验收（BUY→余额/持仓变化 + 风险拒绝场景）并替换原 review 级日志条目、修正歧义表述、补 0 数量边界；TASK-009/010 补「HTTP 200+业务错误码」用例；TASK-008 补测试离线确定性约束；TASK-005 panic 容错并入错误隔离条目；TASK-001 钉死成交价来源；design-spec D4 统一空数据/业务错误语义。修订后判定 PASS。
