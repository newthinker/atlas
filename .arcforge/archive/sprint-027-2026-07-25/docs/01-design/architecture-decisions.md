# Sprint 027 架构决策记录

| ID | 决策 | 理由 |
|---|---|---|
| AD-1 | 改动全部收敛 yahoo 包内 do() 门,零调用方改动 | prism/engine/quote 全路径自动受益;避免在 refresh 层散布限流逻辑 |
| AD-2 | 节流为实例级而非进程级 | cmd 每次 refresh 构造单实例,等效运行级全局;避免包级全局状态污染测试 |
| AD-3 | 重试耗尽返回最后响应而非 error | 保持调用点 unexpected status 错误语义零变更(回归风险最小) |
| AD-4 | throttle 持锁 sleep | 并发调用方天然串行化成均匀节奏,无需额外队列 |
| AD-5 | 不做代理方案(plist proxy env) | 引入代理进程可用性新故障面,劣于客户端自愈(设计文档已记录) |
| AD-6 | Retry-After 只解析整数秒 | yahoo 实际形态;http-date 解析失败回退指数退避,YAGNI |
