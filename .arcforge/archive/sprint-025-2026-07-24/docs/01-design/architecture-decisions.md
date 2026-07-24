# M1.5 架构决策(ADR 摘要,详见 spec 决策记录表)

1. aktools HTTP 侧车(否决子进程桥/纯 Go 直调): Go 侧最干净,运维 +1 launchd 服务可接受。
2. 覆盖范围 A/H 公司主源 + A 股指数兜底(用户扩展决策)。
3. 编排层自动降级链(方案 A,否决 collector 包装的静默降级与泛化源链): 降级可观测(Report.Degraded 进告警),与 source 分派结构同构。
4. 分位本地计算: akshare 无官方 cvpos;复用 valuation.RollingPercentile,10Y 能力反超 engine 路径;兜底期口径混合为已确认取舍(连续性优先)。
5. venv 隔离部署(用户明确要求): scripts/akshare/.venv 专用,setup.sh 幂等 + freeze 锁定。
