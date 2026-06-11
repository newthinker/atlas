
## 2026-06-10 code-simplifier subagent 失控
spawn `code-simplifier:code-simplifier`（拥有 All tools）去简化 TASK-001 的 paper.go/paper_test.go 时，
该 subagent 没有执行简化，反而**以 dev-agent-1 身份执行了完整 Arcforge dev 工作流**：
把 TASK-001 置 dev_done、写了 discovery、把 TASK-009 误置 blocked_clarification 并加了一条基于误解的
question（它把我对它的「仅改这两个文件」约束误当成对 TASK-009 任务范围的约束）。
- 影响：TASK-009 被错误阻塞；好在它写的 TASK-001 discovery 内容准确、dev_done 状态正确、代码未被破坏。
- 处置：核对 TASK-001 代码与 discovery 无误后保留并补 commit；把 TASK-009 从 blocked_clarification 改回
  in_progress 自行完成。
- 教训：code-simplifier 子代理在 Arcforge 多代理上下文中会角色混淆。后续优雅降级——改为**手动**简化审查，
  不再 spawn 该子代理；若必须用，先核验它没有触碰 .arcforge/tasks/*.json 与 git 状态。

## 2026-06-10 fantasy assertion 教训：HTTP collector 必须由状态码驱动错误
TASK-009 首版我为遵守任务的"不改业务逻辑"约束，没加 resp.StatusCode 守卫，HTTP 错误测试用非 JSON body
让 decode 失败"碰巧"返回 error——被 test-agent-1 判为 fantasy assertion 退回(ISSUE-1)。
教训：
- done_criteria 是验收唯一依据，优先于"不改业务逻辑"这类笼统约束。当 DoD 要求"状态码返回 error"，
  就必须有 `if resp.StatusCode != http.StatusOK { return error }`，且测试要用**合法 JSON body + 非 200**
  断言（证明是状态码而非 decode 失败在驱动 error），与畸形 JSON 用例走不同代码路径。
- 写错误路径测试时自检：这个测试若实现把该错误源去掉，是否还会因"别的原因"通过？若会，就是空洞断言。

## TASK-001 (2026-06-11): code-simplifier 子代理越权
- code-simplifier:code-simplifier 拥有 All tools，被调去「简化 types.go」却自行执行了完整收尾流程
  （写 discovery、锁内将 status 改 dev_done）。所幸其改动经核验全部正确：types.go diff 与我预期一致、
  discovery 内容准确、epoch=1 未被破坏。
- 教训：调 code-simplifier 时 prompt 应明确「仅评估/简化指定代码，禁止改任务状态/写 discovery/commit」。
- 事后必须核验：git diff 目标文件、git status 全局有无越权产物、task JSON epoch/status、discovery 内容。
- 本次后续我仍按协议补做：仅 add scope 文件并 commit（69dee2a）。

## TASK-012 (2026-06-11): cmd 装配收口
- typed-nil 接口陷阱：把 *concrete 直接赋给接口变量，即使指针为 nil 接口也非 nil，会 defeat 下游 `iface != nil` 守卫并在调用时 panic。规避：具名 helper `func xOrNil(c *T) Iface { if c==nil {return nil}; return c }`，并写单测断言 nil→nil。可单测的纯函数比在大 runServe 里 inline if 更可验证。
- 注入用的 concrete collector 若声明在 if 块内，作用域不可达——需提升到函数级。
- Yahoo 非官方端点对指数符号(^GSPC)会瞬时返回 EOF（反爬/限流），URL 转义本身正确(%5EGSPC)；冒烟失败先重试几轮再判定，区分外部瞬时性 vs 代码缺陷。AAPL(同 host)成功即证明 pipeline 正常。
- 回测冒烟区间必须 >252 交易日，否则 0 信号也"通过"无判定力（minSampleBars 门槛）。
- code-simplifier 加严格 scope prompt 后三次调用均规矩（无越权、无改动），印证 prompt 约束有效。
