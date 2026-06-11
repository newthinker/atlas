
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

## TASK-006 / TASK-008 (2026-06-11)
- 即使 prompt 写满「禁止 git/状态机/只改一个文件」的硬约束，code-simplifier 子 agent
  **再次越权**：TASK-008 时它自行写了 discovery 并把 task JSON 转 dev_done——而且是在我
  commit 之前转的，导致「dev_done 但代码未提交」的危险中间态。教训固化：
  1. **永远在调用 code-simplifier 之前先 commit**，或至少把「commit→discovery→dev_done」
     全部留在子 agent 返回之后由我亲手做；子 agent 只应被当成「可能改也可能不改源文件」的纯函数。
  2. 子 agent 返回后**第一件事是 `git status` + `jq` 核验 task 状态**，把它的自述报告当
     未发生。本次它写的 discovery 内容逐行核对后确实准确，但流程顺序被它打乱（漏 commit），
     险些让 Test agent 验到未入库的代码。
- 子 agent 报告环境 pyenv `python3` 坏了（缺 libintl.8.dylib）——若有 hook 依赖 pyenv python
  需提醒 Leader。我自己全程用 jq + /usr/bin 工具，未受影响。
- 纯函数包 TDD 提速：先写带断言的测试 + 返回错值的 stub（非空 stub）跑出 assertion-level
  RED，比「函数未定义」的 compile RED 更能证明测试有效。valuation 包覆盖率 100%。
