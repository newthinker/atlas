# Changelog — sprint-006 Notifier 接线（2026-06-12）

## Fixed
- **`notifiers:` 配置死配置预存 bug**：serve 启动从未注册任何通知器，信号 routed 但 `"notifiers":0` 静默零外发。现按 `cfg.Notifiers` 注册 enabled 的 telegram/email/webhook（d9f530a）。

## Added
- 通知器装配降级语义：必填字段缺失（telegram: bot_token+chat_id；email: host+from+to；webhook: url）、未知类型、重名注册 → warn+跳过，不阻断启动。
- 可观测性：逐个注册 info、注册总数 info、`enabled>0 但注册数 0` 静默失效 warn。
- config.example.yaml notifiers 节必填字段与降级语义注释（a22cb0e）。
