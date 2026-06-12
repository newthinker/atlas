# Notifier 接线修复 — 实施计划

> 日期：2026-06-12
> 来源：percentile_step 部署验证发现的预存 bug——`notifiers:` 配置整节死配置，
> `cmd/atlas/serve.go` 从未注册任何通知器，所有信号 `"notifiers":0` 静默落库不外发。
> 验证证据：deploy.log 三轮分析 22 条 routed 全部 notifiers=0；serve.go grep notifier 注册代码零命中。

**Goal:** serve 启动时把 `cfg.Notifiers` 接线到 `app.RegisterNotifier`，使 enabled 的
telegram/email/webhook 通知器真实生效；配置不完整时优雅降级（warn+跳过，不阻断启动）。

## 现状事实（已核实）

- `App.RegisterNotifier(n notifier.Notifier)`（app.go:124）存在，registry 已接入 router（app.go:98），`app.GetStats()["notifiers"]` 回显注册数（app.go:554）。
- 构造器齐备：`telegram.New(botToken, chatID)`、`email.New(host, port, username, password, from, to)`、`webhook.New(url, headers)`。
- `config.NotifierConfig` 字段齐备（Enabled/BotToken/ChatID/URL/Host/Port/Username/Password/From/To/Headers），`cfg.Notifiers` 为 `map[string]NotifierConfig`。
- serve.go 已有同构先例：collector 按 `cfg.Collectors[key].Enabled` 注册（serve.go:98-132）、strategy init 失败 warn+跳过（serve.go:295）。

## 设计决定

1. **装配函数**：serve.go 新增私有函数 `registerConfiguredNotifiers(cfg *config.Config, application *app.App, log *zap.Logger) int`（返回注册数），`runServe` 在 collector 注册后调用。函数化是为了 cmd/atlas 包内可测（serve_test.go 先例已存在）。
2. **按 key 分派与必填校验**（enabled=true 才处理）：
   - `telegram`：必填 `bot_token`、`chat_id` → `telegram.New`
   - `email`：必填 `host`、`from`、`len(to)>0` → `email.New`
   - `webhook`：必填 `url` → `webhook.New(url, headers)`
   - 未知 key：warn「unknown notifier type」跳过
3. **降级语义**：必填缺失 → warn（指明缺哪个字段）+ 跳过，不 fail 启动（与 strategy init 失败同语义）。`RegisterNotifier` 返回 err（重名）同样 warn+跳过。
4. **可观测性**：每成功注册一条 info（含 notifier 名）；结束输出 info 总数；**存在 enabled=true 配置但注册数为 0 → warn「all configured notifiers failed to register; signals will not be delivered」**（直击本次事故的静默失效）。
5. **范围外（YAGNI）**：不做 `${VAR}` 环境变量展开（config 现状如此）、不改 notifier 包/接口、不做发送重试、不新增通知器类型、不做 config.Validate 层校验（启动期 warn 已覆盖；email/webhook 字段组合多，校验层硬拒绝反而伤灵活性）。
6. **构造器直用，不走 `Notifier.Init`**（现有单测/调用形态均为 New 直用；Init 是历史接口残留，本次不动）。

## 测试（cmd/atlas 包内，表驱动；不外发网络）

`registerConfiguredNotifiers` 用例（断言返回值与 `application.GetStats()["notifiers"]`）：
1. telegram enabled 字段齐 → 注册 1
2. telegram enabled 缺 chat_id → 0（warn 路径，不 panic）
3. enabled=false（三类各一）→ 0
4. webhook enabled 有 url → 1；email enabled 缺 to → 0
5. 未知 key enabled → 0 不 panic
6. telegram + webhook 同时 enabled 且字段齐 → 2
7. cfg.Notifiers 为 nil/空 map → 0 不 panic

## 端到端验收（交付验证用，不进单测）

webhook 指向本地 `httptest`/临时 HTTP server + 精简 watchlist 启动 serve，触发一轮分析，
断言：routed 日志 `"notifiers":1`、本地 server 收到信号 payload（不打扰真实 telegram）。

## 交付物

- `cmd/atlas/serve.go`（装配函数 + 调用）、`cmd/atlas/serve_test.go`（表驱动用例）
- `configs/config.example.yaml` notifiers 节补「必填字段」行尾注释（telegram: bot_token+chat_id；email: host+from+to；webhook: url）
- 全局规范收尾：code-simplifier → go vet/test 全量 → gitnexus analyze
