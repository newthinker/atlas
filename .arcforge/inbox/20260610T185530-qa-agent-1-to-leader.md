# qa-agent-1 → team-lead （QA 终审完成：VERDICT = CONTESTED，需人工介入）

两轮 Code Review 完成。报告：`.arcforge/docs/05-review/{qa-review-round1,qa-review-round2,qa-verdict}.md`。
checkpoint：`.arcforge/checkpoints/qa-agent-1-checkpoint.md`。
我未改任何代码、未改 task JSON / plan.md（CONTESTED 的 task 状态流转由你裁定）。

## 客观证据（实跑）
`go build ./...` ✅ ｜ `go vet ./...` ✅ ｜ `go test ./...` 全包绿 ✅ ｜ `-race`(app+broker+collector) ✅ ｜ `gofmt -l` 改动文件干净 ✅

## VERDICT: CONTESTED
CRITICAL 0 ｜ WARNING 3 ｜ INFO 2。全部缺陷 fail-safe（不崩溃、不产生错误订单）。

### 需你决策的 high-severity（CONTESTED 根因）
**W1 生产执行链路 inert** — `serve.go:142` 仅注册 ma_crossover，其 Signal 不设 Price(`strategy.go:86,102`)→ `sig.Price=0` → `Execute` 返回 "price must be positive" 被 SubmitSignal 吞掉 → **真实运行永不下单**。单测/e2e 全靠硬编码 `Price:100`(`executor_test.go:171…`)掩盖（fantasy-pass）。
裁定点：「填充 Signal.Price 是否属本 sprint 范围」？
- 属本 sprint → 建议把 TASK-002/003（或新增 strategy 任务）置 `review_fix`，fix_items={策略层用 bar.Close 填 Signal.Price｜补"Price 未填→不下单"断言}。
- 越界 → 记 `wisdom/issues.md` 留下一 sprint，本 sprint 此项放行。

### 建议本轮一并 review_fix（fail-safe 但真实）
- **W2** `app.go:369-377`：executor.SubmitSignal 不受 router.Route(cooldown/dedup) 约束，W1 修复后→重复信号重复下单，仅 RiskChecker 兜底。
- **W3** `config.go:372` vs `execution.go:185`：execution.mode="" 校验通过但运行 ErrInvalidExecutionMode，Load 不补默认 → broker 启用漏写 mode 时静默不下单。

### INFO（可选）
- I1 `executor.go:75` confirm 模式仅入队却日志 "signal executed"，无自动 confirm。
- I2 `paper.go:188-191` CancelOrder 非终态分支不可达死代码。

## 已验证修复（PASS，附证据）
- ISSUE-1(StatusCode) 彻底解决：eastmoney(4)+lixinger(6)+yahoo(2) 共 10 条 fetch 路径全部守卫 + defer Body.Close。
- ISSUE-3(Execute Price 缺陷) 已修：`execution.go:167` 市价单携带 Price。
- 并发安全 / 缓存防污染 均过 -race 与审查。

11 个任务现状全 `verified`。等待你对 W1 的范围裁定与（若需）派发 review_fix。qa-agent-1 转入待命。
