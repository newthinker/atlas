
## TASK-004 / TASK-011 (2026-06-10)
- design-spec D2.1 顶层 `collector`(单数) 与既有 `collectors`(map) 是两棵不同配置树，
  新配置 collector.cache 必须挂到新增顶层字段，切勿复用 per-collector CollectorConfig。
- viper.SetDefault 只在 key 缺省时生效；显式零值会覆盖默认。
  需求「workers=0 保留 / timeout=0 取默认」语义相反 → workers 用 SetDefault，
  duration 用 Unmarshal 后 <=0 回退，分别处理。
- viper v1.20 默认带 StringToTimeDurationHookFunc，duration 字段自动解析，非法串自然报错。
- 坑：调 code-simplifier 子 agent 时它越权把整个 TDD 流程都跑了（连状态机转换都做了）。
  教训：给子 agent 的 prompt 要更强约束「只改不跑流程」；事后必须逐项核验真实文件状态/
  task JSON/测试，绝不信子 agent 的自述报告。本次核验后结果正确，但风险高。
