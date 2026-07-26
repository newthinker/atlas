
## dev-agent-18 / TASK-004(prism refresh 明细打印)2026-07-25

### L1 门禁 DENY 必须先分类,再行动
`transition dev_done` 的 DENY 有三种截然不同的根因,处置方式互斥,**先分类再动手**:
1. **他人未提交 WIP 污染**(本次首次 DENY):`internal/storage/prism/sqlite.go:114 undefined: strings`
   —— 门禁按整包编译,任何并发 agent 的半成品都会误伤同包/下游任务。
   处置:**不要去改别人的文件**(会与对方在写的编辑冲突),等对方补齐后重试即可。
2. **框架口径缺陷**(本次第 2/3 次 DENY):整包覆盖率低于阈值,而本任务改动函数已 100%。
   处置:采集**文件级/函数级**证据请 Leader 放行,**绝不写空洞测试堆整包覆盖率**。
3. 真实质量问题 —— 才是需要返工的那类。
误判代价不对称:把 (1)(2) 当成 (3) 会产生大量无关改动,污染他人范围。

### L2 隔离验证:用 detached worktree 把「我的问题」与「他人污染」切开
共享工作区里 `go test` 的失败无法区分是谁的锅。**不要用 `git stash`**——它会连带回滚其他
agent 的在制品,是破坏性的。正确手法:
```bash
git worktree add --detach <tmpdir> HEAD   # 只读隔离副本,不动共享工作区
cd <tmpdir> && GOTOOLCHAIN=local go build ./... && go test ./<pkg>/ -count=1
git worktree remove --force <tmpdir>
```
本次据此证明 `ef541b7` 自身 BUILD OK + 测试全绿,DENY 纯属他人 WIP 污染。已固化为 AD-20。

### L3 两次 SendMessage 无回应 → 落盘,不要干等也不要自行其是
inbox 是通知,会丢会误路由(本 sprint 实测 idle hook 被误投到我的只读 code-simplifier 子线程
四次,**本体收不到催办,而本体才是唯一有写权限的一方**)。正确做法是把阻塞转成
`blocked_clarification` + `questions[]` 落盘,让阻塞在 `/arcforge-status`、validator 里可见。
副作用要预判:停在 `in_progress` 会让 idle hook 把我误判成「在干活」。

### L4 `blocked_clarification` 的出边是 `in_progress`,且是 dev-* 专属边——可自恢复
被答复后**不要等 Leader 改回 `assigned`**(边表里根本没有 `blocked_clarification → assigned`,
Leader 想帮也帮不了),自己走:
```bash
jq -r '.transitions | to_entries[] | select(.key|startswith("blocked_clarification"))' .arcforge/write-matrix.json
# blocked_clarification->in_progress -> ["dev-*"]
```
本次 Leader 开的是**时限窗口**,若我干等这条往返,窗口会空转掉。**卡住时先查 write-matrix
自己有没有出边**,比发消息问快一个数量级。自恢复时顺手把 Leader 的答复写进
`questions[0].answer` 闭合澄清环——否则会留下悬空提问(Leader 以为自己写了,实际没写)。

### L5 过期快照驱动决策是并发下的通用坑,不分角色
Leader 两次基于过期读发指令(按 `in_progress` 快照派活,而我已转 `blocked_clarification`,
导致首次 `dev_done` 被「非法迁移」拒;以为 answer 已落盘而实际未写)。
Dev 侧有 `--expect-epoch` 做锁内机制化断言,**Leader 侧的「读快照→发指令」目前只有自觉**。
对我的启示:收到任何基于状态的指令,**先 jq 直读核实当前状态再执行**,不要照着指令里的
状态假设行动——本次正是这样才发现「两个 transition 早已做完,无需重做」。

### L6 时间感知不可靠,不要用确定口吻写时长
我在 `questions[]` 里写了「24h 内无变化」,实际是分钟级,被 Leader 如实指正。
agent 没有可靠时间基准,**只写可核实的事实**(「两次 SendMessage 后状态无变化」),
不写主观时长。该文本已落盘进 sprint retro 记录,事后需专门更正——写时严谨比事后补救便宜。
