# 待同步清单（sprint-028 累积）

运行时资产（`.claude/hooks/`、`.claude/scripts/`、`.claude/settings*.json`）与 `CLAUDE.md`
对全体 agent 只读。本文件累积本 sprint 发现的、需要**会话外**经「改 project-template/ → TDD →
人类确认 → 同步」路径处理的项，Step 7 并入 `06-acceptance/final-report.md` 的固定段落。

> 单独立文件的原因：这些项此前散落在 `wisdom/learnings-leader.md` 的各条教训里，
> 到交付时要靠回忆捞取。**一个只在结尾才需要的清单，必须从第一项就有固定归处。**
>
> 放在 `06-acceptance/` 而非 `03-progress/`：写通道 deny-by-default 拒绝了
> `docs/03-progress/pending-hooks-sync.md`（规则表里 03-progress 只精确登记了 `plan.md`，
> 而 `docs/06-acceptance/*` 对 leader 通配可写）。**拒绝是对的**——它把我推回了这份清单
> 本来就该在的位置：它是验收产物，不是进度产物。

## 1. CLAUDE.md 状态机边表与正文不一致

`blocked_clarification` 在正文「澄清环」里写明 `in_progress → blocked_clarification → assigned`，
但状态机表格的边定义中**没有 `blocked_clarification → assigned` 这条边**。
Leader 答复 `questions[]` 后要把任务改回 `assigned`，走的是一条表上不存在的边。

## 2. docs-only 门禁对「先提交后转态」死锁

`task-completed.sh` 的 docs-only 分支要求「声明范围内有**未提交**变更」。
若 dev 先 `git commit` 再 `transition dev_done`，门禁查不到未提交变更 → DENY，
而变更已进历史、无法退回未提交态。**顺序反了没有补救路径。**
（已在 memory 中记录为操作纪律，但门禁本身仍应允许「已提交且属于本任务」的情形。）

## 3. `packages` 字段双用途冲突

同一字段同时驱动两件事：**scope 互斥调度** 与 **覆盖率门禁的 `-coverpkg` 合并统计**。
只读消费某包的任务（如 TASK-011 之于 `./internal/prism/sankey`）声明它才符合 AD-6 的接线口径，
但这会**阻止该包的其他任务并行**——即使本任务一行都不改它。
建议拆分为 `packages`（门禁口径）与 `writes`（互斥口径）。

## 4. test/dev 角色定义缺「自愈轮询」循环（本 sprint 实测）

CLAUDE.md 开篇原则写明：

> 「即使通知丢失，各角色通过轮询自己负责的状态也能发现待办、自愈推进。」

**实测未兑现，且已发生两次**（同一 agent，同一形态）：

| 次数 | 任务 | 派验时刻 | 表现 |
|---|---|---|---|
| 1 | TASK-016 | 12:45:00Z | **23 分钟内从未收到通知**，自述「刚从你这条消息才知道」 |
| 2 | TASK-018 | 13:33:09Z | 派验后仍报「还没派给我（`dev_done`、verifier 仍空）」 |

两次磁盘状态全程正确。**单次可以说是偶然，两次说明这是常态而非异常。**

原始记录：TASK-016 于 12:45:00Z 派验、`verifier` 与 `verifying` 均已原子写入磁盘，
test-agent-6 **23 分钟内从未收到通知**，自述「刚从你这条消息才知道」。
磁盘状态全程正确——**真相源解决了「状态是否正确」，没解决「谁会去看」**。

自愈的前提是有人在轮询，而 test/dev 角色定义里**没有任何「扫描 owner==自己的在途任务」的循环**。
这个能力只写在原则里，没写进任何角色的行为定义。

附带：本次靠「checkpoint 时间早于接手时刻」这个信号发现，**纯属侥幸**——
只要该 agent 在这段时间因任何别的事更新过 checkpoint，信号就消失。
活性判据应锚定**本轮派发**（该任务的报告文件、checkpoint 中出现该 task id），
而非 agent 的全局活动痕迹。

## 5. 写通道对未识别参数应 fail 而非静默忽略（本 sprint 实测，代价最大）

`arcforge-write.sh` 中 **`--append` 根本不存在**（全文件 grep 零结果）。
`wisdom learnings` 本身就是 `>>` 追加、`doc` 是 `atomic_write` 全量覆盖，
两者都把 `--append` 当多余位置参数**静默忽略**。

后果：我在 wisdom 上用过一次、看到内容确实被追加，遂认定这是通用语法；
用到 `doc` 上时**整份 `architecture-decisions.md` 被覆盖**（580 行 26 条 → 29 行 1 条）。

**静默忽略把「用错命令」变成了「命令成功但语义不是你要的」，这是最难自查的一类失败。**
未识别参数应直接 `fail`。

## 6. `.arcforge/docs/` 不在 git 跟踪范围，无版本兜底

第 5 项能恢复纯属运气：scratchpad 里恰好留着一份完整副本，而那是走写通道的中间产物、**不是备份**。
`git status` 对该目录显示 `??`。若换个写法（heredoc 直接进写通道），26 条决策连同全部修正说明
会**永久丢失**，且 AD 自查会显示差集为空（引用与条目一起消失），**没有任何一层会告警**。

建议：`.arcforge/docs/` 纳入版本控制，或在写通道的全量覆盖路径上留一份上一版快照。

## 7. `dev_done` 后的追加提交会让验证对象静默漂移（本 sprint 实测）

TASK-018 的 dev 在 `dev_done`（13:31:21Z）之后又提交了 `d055490`（13:38:07Z），
而 verifier 在 13:39:30Z 判定 VERIFIED——**判定对象与最终交付物不是同一个 commit**。

这次实质无碍（diff 仅注释，断言逐字节未变，我与 verifier 各自独立核过），
但**没有任何机制会告警**。若 dev 改的是断言，VERIFIED 就失效了而无人知晓。

更细的一层由 verifier 自查发现：它的**变异测试跑在 `a8c8354` 的 worktree 上**，
而**注释与断言核查直接读工作树**（彼时已是 `d055490`）——两边都真实，但报告统一署成了 `a8c8354`。
83 秒的时间差。已在报告加 §8 更正，写明验证范围实为 `a8c8354..d055490` 区间。

**建议堵在验证者侧而非 dev 侧**（verifier 的意见，我同意）：

> 验证者承接时记下 `HEAD`，转 `verified` 时校验 HEAD 未变；变了就自动 diff 并要求确认。

理由：dev 在 `dev_done` 后修正一条**错误的**理由注释这件事本身是对的
（「一条错误的『为什么』比没有注释更有害——它会让后人基于错误理由改动断言」），
堵在 dev 侧会误伤这类有价值的追加。堵在验证者侧只是让判定者知道「我的判定对象变了」。
**与 epoch 机制同思路——不禁止变化，只保证变化不会被静默吞掉。**

## 8. 覆盖率门禁用「整包合并」衡量增量工作，在既有低覆盖仓库里不可达（TASK-011 实测阻塞）

`task-completed.sh:110-133` 以任务 `packages` 的**整包 `-coverpkg` 合并覆盖率**对比 `dev_minimum`（80）。
TASK-011 实测被 DENY 于 **71.6%**，而实测证据显示这与该任务的质量无关：

| 事实 | 数字 |
|---|---|
| TASK-011 **之前**同口径基线（worktree @ `a8c8354`） | **71.1%** |
| TASK-011 **之后** | **71.6%**（**提升 0.5pp，不是回归**） |
| 该任务新增的 13 个函数（`sankey.go` 全部 + `jf`/`jfSlice`） | **全部 100.0%** |
| `setupRoutes`（接线处） | 38.5% → **40.7%**（新增 4 行分支正反两向均覆盖） |
| 拉低者 `runServe` | **0.0%**（仓库本就无任何单测，它会真正起服务器） |

**缩小 `packages` 无法解决，反而更糟**：只留 `./internal/api/handler/api` → **52.4%**；
加上 `./internal/api` → **47.9%**（这两个包本身含大量既有未测 handler）。

门禁无逃逸路径（唯一例外是 docs-only 分支，而本任务有真实代码，标注 docs-only 等于伪造）。

**结论：在既有代码覆盖率远低于阈值的包上，任何新任务都无法通过该门禁，无论其自身质量多高。**
指标衡量的是「包的历史状态」，而任务能改变的只有「增量」。

修法建议（择一，需人类决策）：
1. 门禁改用**增量覆盖率**（只统计本任务 changed lines），这是唯一对症的修法；
2. 或允许任务级 `coverage_floor` 覆盖全局 `dev_minimum`，由 Leader 在拆分时按包的既有水位设定；
3. 降低全局 `dev_minimum` 是下策——它对所有任务同时放松，而问题只出在少数历史包袱重的包。

### 8.1 人类已选定的修法与可直接应用的 patch

**决策（2026-07-25，人类确认）**：任务级 `coverage_floor` 覆盖全局 `dev_minimum`。
`TASK-011.json` 已写入 `coverage_floor: "70"`（写通道 `--field` 只写字符串，
但门禁用 `[ ... -lt ... ]` 比较，bash 按数字处理，不影响功能）。

**为什么不能在 `task-completed.sh:13` 原地改**：该行执行时 `TASK_ID` 尚未解析（在 line 26）。
故应在 `TOTAL` 算出之后、阈值比较之前插入。

**patch**（插在 `if [ "${TOTAL%.*}" -lt "$DEV_MIN" ]` 那一行之前）：

```bash
# 任务级 coverage_floor 覆盖全局 dev_minimum（历史包袱重的包按其既有水位设定）
TASK_FLOOR=$(jq -r '.coverage_floor // empty' ".arcforge/tasks/${TASK_ID}.json" 2>/dev/null)
if [ -n "$TASK_FLOOR" ]; then
    echo "Task-level coverage_floor=${TASK_FLOOR} overrides dev_minimum=${DEV_MIN}"
    DEV_MIN="$TASK_FLOOR"
fi
```

**同步后的收尾动作**（Leader 执行）：
TASK-011 `blocked_human → assigned`（`assignment_epoch` 将 +1）→ 通知 dev-agent-17 重新认领。

> **注意这个修法只是止血，不是对症**。它让 Leader 手动为每个历史包袱重的包设水位，
> 而水位设多少本身没有客观依据（这里取 70 是因为基线恰好 71.1%）。
> **真正对症的是增量覆盖率**——只统计本任务 changed lines，
> 那样「新代码必须测」与「不为历史欠债买单」才能同时成立，且无需任何人工设定。
> 建议把本条当作过渡方案，增量覆盖率列入后续。

## 9. 写通道 `--field` 不支持嵌套路径，且静默失败（第 5 项的又一实例）

`task TASK-011 update --field "questions[0].answer=..."` **不报错、不落盘**。
Leader 对 dev 澄清请求的答复因此丢失，改走 plan.md 与 inbox 记录。

与第 5 项（`--append` 不存在却被静默忽略）同根：**写通道对不认识的输入一律沉默**。
两次的后果不同——`--append` 是「做了别的事」，本条是「什么都没做」——
但都属于「命令成功返回，语义不是你要的」。

## 10. `discovery` 在任何状态下都可写，包括 `verified` 之后（dev-agent-17 指出）

写通道的 `discovery` 分支只校验 `assigned_to==$ME` 或 `verifier==$ME` 或 leader，**与 `status` 无关**。

好处是可以事后补记（本 sprint 就用到了：dev-agent-17 在 `blocked_human` 状态下补强了 TASK-011 的
契约字段）。**风险是验证者读到的 discovery 可能与它验证时看到的不是同一份**——
若有人在 `verified` 之后改 discovery，就出现「报告依据的证据已被改动」。

**与第 7 项是同一族**：第 7 项是代码 commit 漂移（`dev_done` 后追加提交），本项是证据文件漂移。
两者的共同形态是**判定对象在判定之后被静默改动**。

建议：`discovery` 在 `verifying` 开始后**只追加不修改**（或至少记录修改时间戳供验证者比对）。
与第 7 项建议的「验证者承接时记 HEAD」可合并为同一机制：
**验证者承接时对判定依据（代码 commit + discovery 内容）取快照，出报告时校验未变。**

> 本项由 dev-agent-17 主动指出，而它自己正是受益方（它刚利用这个口子做了有价值的补强）。
> **指出一个对自己有利的机制口子，比指出别人的问题更难。**

## 11. `verifying` 无 verifier 重绑定路径——AD-21 死锁本 sprint 首次真正发生

TASK-011 卡在 `verifying`，verifier 是已被停止的 test-agent-7。**三条路径全部 DENY**：

```
leader        verifying → rejected    DENY（合法写者 ["test-*"]）
leader        update verifier         DENY（verifying 状态的任何写入都归 test-*）
test-agent-8  任何写入                DENY（该任务 verifier 是 test-agent-7，不是你）
```

`verifying` 的出边只有 `->verified` / `->rejected`，**均 `test-*` 专属**；
而新 spawn 的 test 实例被 verifier 绑定校验（`arcforge-write.sh` 的 role-binding，
对 `transition` 与 `update` **都生效**）拦下。**闭环，无带内解法**，最终由人类直接编辑
任务 JSON 的 `verifier` 字段解开。

AD-21 当初记的是「verifier 实例死亡则无解」。本 sprint 三次 agent 失联都没触发它
（两次任务未派到该实例名下，一次产出已全部落盘）——**这次触发了**。

**中途更换 verifier 是可预见需求**（三次失联足以证明），却没有任何合法路径。建议二选一：
1. 给 `verifying` 加一条 leader 可用的逃生边（如 `verifying → dev_done` 或 `verifying → assigned`），
   使 Leader 能收回并重新派验；
2. 或允许 leader 在 `verifying` 状态下改写 `verifier` 字段（当前 rules 只允许 leader
   在 `transition` 模式写 verifier，而 leader 恰恰没有从 `verifying` 出发的 transition）。

方案 1 更干净：它复用既有的「Leader 调度边」概念，且与 epoch 机制一致。

## 12. `description` 承载义务而 `done_criteria` 不含 → 全员合规但义务落空（Leader 自身缺陷）

TASK-011 的 description 明确写「六项补测**并入本任务**」并为此扩展了 `packages`，
但**没有任何一条 `done_criteria` 提及**。结果：

- dev 按 DoD 交付（十条全 PASS），**无过失**；
- verifier 按 DoD 验证（十条全 PASS），**无过失**；
- 六项补测一件没做，其中两个是实证的真缺口
  （`gross_profit`→`OperatingIncome`、`sganda`→`RnD` 映射变异全套测试不红）。

**这直接违背 CLAUDE.md 的核心原则**：「完成标准（DoD）是一切测试的唯一依据」。
把实质义务写在那个依据之外，等于让它对整条流水线不可见。

**这不是 hook 缺陷，是 Leader 拆分纪律问题**，列在此处是因为它可以被机制化：
validator 可增加一条检查——**`description` 中出现「并入本任务」「顺带」「一并」
等承接性表述时，若 `done_criteria` 无对应条目则告警**。

硬规矩（已入 wisdom）：**凡是要求 dev 交付的东西，必须落进 `done_criteria`；
`description` 只承载背景与理由，不承载义务。**
