# Code Review Report — notifier-wiring (sprint-006)

- 审查者: qa-agent-1 (Reality Checker, 默认 NEEDS WORK)
- 范围: master..HEAD — d9f530a (cmd/atlas/serve.go +74, serve_test.go +277), a22cb0e (configs/config.example.yaml)
- 核心对象: `registerConfiguredNotifiers` (serve.go L300-360), 调用点 L148
- 日期: 2026-06-12
- **VERDICT: PASS** (0 CRITICAL / 0 WARNING / 2 SUGGESTION / 1 INFO; 阈值 warning 之上无发现)

## 证据（亲自核验，非转述）
- 调用点 serve.go:148 位于 collector 注册(L104/122/135)之后、`api.NewServer`(L247)/`server.Start`(L254)之前 — 时序正确。
- `go vet ./cmd/atlas/` → 干净；`go test ./cmd/atlas/ -run TestRegisterConfiguredNotifiers -count=1` → `ok 0.619s`。
- `app.RegisterNotifier`(app.go:124) → `notifiers.Register`(registry.go:24) 按 `Name()` 去重返回 err；`GetStats()["notifiers"]`(app.go:554) = `len(GetAll())` 回显注册数 — 测试双断言(返回值 + GetStats)成立。
- 构造器签名匹配: `telegram.New(botToken,chatID)` / `email.New(host,port,user,pass,from,to)` / `webhook.New(url,headers)`。
- 不记录密钥: warn/info 仅含 notifier key 与 field 名, 无 token/password 值 — 无敏感信息泄露。

## 第一轮 — 常规审查
- **正确性**: enabled 过滤 + 按 key 分派 + 必填校验 + warn/skip 降级 + 静默失效 warn(`enabled>0 && registered==0`), 逻辑自洽。PASS。
- **风格一致性**: 与既有 collector(L98-135)/strategy(registerConfiguredStrategy) 的 warn+skip 不阻断语义、函数化可测形态一致。PASS。
- **错误处理**: RegisterNotifier err(重名) → warn+continue, 不 panic 不阻断。PASS。
- **测试质量**: 真实构造器 + 真实 registry + zaptest/observer; 三类正向各一; 6 字段缺失矩阵断言具体字段名; 重名预注册触发冲突; 静默失效精确日志片段; nil/空 map; 无空洞断言, 无过度 mock; 函数覆盖 100%。PASS。
- **安全/性能**: 无注入面(配置可信)、无密钥日志、无并发问题(启动期单次调用)。PASS。

## 第二轮 — 跨视角对抗（spawn 的 Skeptic/Architect 子代理越权写盘，结论已由本体甄别重判）
- 运维: 启动日志(per-notifier info / 总数 info / 缺字段 warn 指明字段 / 静默失效 warn)足以支撑"为什么没收到通知"排查; TASK-002 E2E Part A/C 已实证。PASS。
- 配置对抗: 同名 key(YAML map 不可能重复)、大小写 key("Telegram"→default→"unknown notifier type" warn, 可预期)、enabled 全空串(命中必填 warn+skip+静默失效 warn) — 行为均可预期。PASS。
- 维护者: 新增类型 = switch 加一 case, 扩展点清晰; 3 类用 switch 而非注册表不构成过度/不足设计。PASS。

## 发现清单
- [SUGGESTION] 多必填字段短路告警 / serve.go:329-343 / requireField 用 `||` 短路串联, 同一 notifier 多字段同时缺失时只 warn 首个, 运维需多次重启才发现全部缺失字段。/ 建议(非阻断): 可改为顺序求值收集全部缺失字段一次性 warn。设计注释已声明"each case bail uniformly"为有意取舍。
- [INFO] 非空校验止于 `!= ""` / serve.go:330,335,340 / 空白字符串(" ")或未展开占位符("${VAR}")通过非空校验→注册"成功"但发送期失败。属设计明确豁免范围(${VAR} 展开、config.Validate 深校验均为 YAGNI: "字段组合多, 校验层硬拒绝反伤灵活性")。记录为已知边界, 非缺陷。
- [INFO] 配置注释(config.example.yaml L65-86) 与代码必填规则逐条一致(TASK-002 §1 已逐条比对), 无偏差。

## 对越权"CRITICAL"的甄别（Reality Checker 反向核验）
spawn 的对抗子代理产出 3 条 CRITICAL, 经本体对照设计范围边界核验后全部降级/否定:
1. 短路掩盖字段缺陷 → 实为 SUGGESTION(降级仍优雅, 非正确性 bug)。
2. 空白值绕过校验 → 与 ${VAR} 同类, 深校验为设计豁免 YAGNI, 降为 INFO。
3. "部分失败静默丢弃" → **不成立**: 每个失败项各自有 per-field/register-error warn, 聚合 warn 仅针对全军覆没(即本次修复的事故); 部分失败不静默。

## 结论
变更精确命中目标(notifiers:0 静默落库事故已闭环), 降级语义完整、可观测性充分、测试真实有效、范围严格未越界。**无 CRITICAL/WARNING → VERDICT: PASS**, 进入最终验收。
