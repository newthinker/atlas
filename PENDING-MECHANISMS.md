# 待决机制变更（Arcforge）

**这份文件只放「等人拍板的机制变更」，不放证据、不放教训、不放叙事。**

超过 40 行就是失败了 —— 它存在的全部理由是 `wisdom/decisions-leader.md` 已经 32KB、
`wisdom/_digest.md` 已经 88KB，而下一个 leader 需要的是这几行。

⚠️ **改动落点是 Arcforge 上游仓库**（`newthinker/ArcForge`）—— 本仓库是消费项目，无 `project-template/`。

---

## 当前待决（上游 sprint-010 实测 + atlas M1.5/Sprint 044 + **M2a/Sprint 045**，2026-09-06 记）

| # | 变更 | 落点 |
|---|---|---|
| 1 | `teammate-idle.sh` 的 `test-*` 分支补 F6 防空转出口：现在任意 `dev_done` 无条件并入 `MINE`，而 `dev_done→verifying` 是 leader 专属边 ⇒ 等派验期间无限唤醒（实测一分钟四次）。相邻 `qa-*` 分支有现成同构写法 | `project-template/hooks/teammate-idle.sh` |
| 2 | 逃生边把判定依据写进审计行：现在只记时间，文件层面无从区分「有证据的快」与「没证据的急」（上游十条逃生边有两条事后证明误判） | `project-template/hooks/arcforge-write.sh` |
| 3 | `stale-dispatch` 处置补一条判据：委托子代理致卡死时无 failure 通知（卡 running 不转 idle），只能靠「零文件产物 + worktree 未被触碰」识别。atlas M1.5 第二实例；**M2a +2**：code-simplifier 子代理被 idle hook 以父实例名循环（自报后靠 Leader 直发消息令其返回）、另一个**挂 2.9h 后带产物返回**（期间父实例收不到消息、不被唤醒；Leader 直发消息「排队待下一工具轮次」永不送达）⇒ 处方：先写正文提交再派终检；只审不改的终检由本体做 | `global/agents/*.md`、`teammate-idle.sh` |
| 4 | **会话/消息挂起无告警**：M1.5 Leader 27m + dev 45m/23m + verifier 57m；**M2a：Leader 88m / 7h16m / 55m（累计 ≈ 9.6h）、dev 54m / 65m、verifier ~3h，且新形态「**工具批次本身被挂起**」（6 个并行 Bash 里部分 2h13m 后才执行，agent 侧无间隙）；三任 dev 连续在同一任务上挂起 ⇒ Leader 改走**记录员模式**（Leader 采数起草、记录员只落盘/提交/迁移）一次通过。已实证有效的处方：dev 20 分钟无回执 ⇒ 自己转 `blocked_clarification` 写文件级信号（两次生效）；Leader 见分支有形状正确的提交 + 预演无冲突 ⇒ **不等请求直接 merge** | 上游议题 |
| 5 | 任务 ID 跨 sprint 复用致门禁 `git log --grep` 全集含旧提交（只 WARN，无害）；`capabilities.codex_cli: true` 与实际不符（M1.5：`codex exec` 30 分钟零输出；**M2a：`--version` 秒回但 `codex exec` 秒回 usage limit 至 9/10**）——探测缺「可运行且有配额」验证 | `task-completed.sh`、`arcforge-init` |
| 7 | **跨仓库写通道 fail-open 且静默（M3 实撞）**：在另一个也装了 arcforge 的仓库目录里 `cd` 着调 `bash .claude/hooks/arcforge-write.sh`，相对路径解析到**那边**的脚本、写进**那边**的 `.arcforge/`，**退出码 0、零告警**。linked-worktree DENY 只比 `--git-dir` vs `--git-common-dir`，覆盖不到这条 ⇒ 「文件系统是唯一真相源」被静默架空。处方候选：脚本自检 `git rev-parse --show-toplevel` 是否等于它自己所在的仓库根 | `project-template/hooks/arcforge-write.sh` |
| 8 | **Leader 的工作区编辑会阻塞每一个 dev 转 `dev_done`（M3 实撞）**：`task-completed.sh` 的 `OTHERS`（他人在途 scope，用来把别人的改动从当事人头上减掉）**只从在途任务 JSON 的 `writes`/`packages` 里取** ⇒ Leader 在主仓库工作区的编辑**不属于任何任务，减不掉**，于是对每个正要转 `dev_done` 的 dev 都表现为「你越界了」，且指向一个它**既不能补声明**（错误归属，会让范围外的真漂移从此不告警）**也不能撤回**（会抹掉 Leader 的在途改动）的文件。dev 侧无解，只能等 Leader 提交。⇒ 纪律侧：Leader 的工作区编辑随手提交；机制侧可考虑把「不属于任何任务的未提交文件」与真漂移区别提示 | `project-template/hooks/task-completed.sh` |
| 9 | **跨仓库任务的 `verify_baseline` 够不到目标仓库（M3 实测）**：基线只锚 atlas 的 `head` + `discovery_sha256`，**不锚 loom / nanoclaw 的 commit**。TASK-005 实测 nanoclaw 在派验后 66 秒多了一个 commit，它被发现**只因为 dev 同时也改了 atlas 侧交付文档**才触发 `--ack-drift`；**只改目标仓库则基线完全一致、零告警，验证者会静默地判另一棵树**，且 worktree 工作树默认就是新版。已在 TASK-006/007 的 DoD 里要求 verifier 自己前后各记一次目标仓库 HEAD——**那是纪律不是机制** | `project-template/hooks/arcforge-write.sh`（可考虑：`dev_done → verifying` 时把 discovery 里声明的目标仓库 commit 一并写进基线） |
| 10 | 🔴 **门禁的覆盖率尺系统性偏高，可能在真实覆盖率低于门槛时放行（M3 实测）**：`task-completed.sh` 取 `go tool cover -func | grep total:`，而 **`-func` 只聚合具名函数声明**，**包级 `var` 里的匿名函数字面量被静默丢弃**——cobra 惯用法 `var xCmd = &cobra.Command{Run: func(...){…}}` 正是这个形状。atlas `cmd/atlas` 实测：`version.go` 那个块 3 条语句**全部未覆盖**被丢掉 ⇒ `-func` 报 **76.8%**，而 profile 直接数语句是 **76.6804%**（1118/1458），剔除该块复算 `1118/1455 = 76.8385%` **与 `-func` 精确吻合**。⚠️ **被丢弃的是未覆盖语句 ⇒ 偏差单向偏高、随 cobra 命令数规模化、且在任何输出里都不显形**。本 sprint 无影响（保守口径仍 ≥ 门槛），但机制上门禁可以放行不达标的交付。处方：把该行换成从 coverprofile 直接求和，或改用 `go test -cover` 行内值 | `project-template/hooks/task-completed.sh`（约 `TOTAL=$(go tool cover -func …)` 那行） |
| 11 | **write-guard 的运行时资产判定拦的是命令文本、不是写入目标（M3 实撞，verifier 报告）**：`write-guard.sh` 的 `.claude/` 检测（约 245 行）对 `.tool_input.command` 的**文本**求值，命中「同一行内出现写动词 ∧ 出现 `.claude/(hooks\|scripts\|settings)`」即 DENY——**写入目标是什么完全不参与判定**。于是在 discovery / 验证报告 / checkpoint 的**正文里引用 hook 路径**（写通道调用形态、门禁缺陷锚点）会被整条拦下，尽管目标是 `.arcforge/checkpoints/`。两点放大：① 写通道白名单（约 262 行）位于该检测**之后**且只豁免 `.arcforge/` 侧，够不到这里；② 约 229 行把项目绝对路径折叠成相对 `.claude/`，而「跨仓库任务必须用 atlas **绝对路径**调写通道」正是 M3 要记的经验，正中判定面。**代价小（改措辞或把写动词与路径拆到两行即可）但第一次撞上不明显**：DENY 文案说的是「运行时资产只读」，而当事人在写 checkpoint，对不上。⚠️ **不建议改成按写入目标判定**——那要解析命令语义，正是这道启发式刻意不做的事（文件头注释已声明「宁漏不误拦」）。便宜的处方只有两个：DENY 文案补一句「命中的是命令文本里的路径字面量，未必是你的写入目标」，或对 `arcforge-write.sh … --file` 形态豁免 | `project-template/hooks/write-guard.sh`（DENY 文案，非判定逻辑） |
| 6 | **teammate→leader 消息延迟 20–25 分钟 ×2（cron tick 正常到达 ⇒ 通道延迟非挂起）**；派发/派验通知丢失 ×2 靠 idle hook 重扫兜住。文件真相源全部兜住，但 Leader 侧回执协议因此退化为「以文件为准、消息只作催办」 | 上游议题 |
## 已落地（M1c-4 提出，2026-09-03 由 PR #6 合入上游 `main` @ `496bbf0`）

`update --expect-status`、validator 规则 `archive-mutated` 与 `unregistered-writer`、
派验回执「请回一句确认收到」—— 四项均已在本仓库核实生效（2026-09-04）。

⚠️ 后两项是**审计而非拦截**：`>>` 绕 write-guard、未登记名字免验 token 两个洞都**不堵**
—— hook 无法可靠区分调用者身份。原则是**不禁止变化，只保证变化不会被静默吞掉**。
## 明确不做（结论仍有效，勿重开）

- **不给 `in_progress` 加 `stale-dispatch` 阈值** —— 刻意的，dev 正常干活 p90 就 65 分钟
- **不改 `git log --grep` 的实现** —— 判据走文档：按 sprint 起点截断 + 只看 `%s`（标题），
  因为它可能命中 body（`0981d90` 标题里根本没有 TASK-014）

## 证据

`.arcforge/wisdom/decisions-leader.md`，grep 这些锚：
`expect-status` / `第三个写通道缺口` / `两个机制缺口，都由 QA 主动报告而非利用` / `派验要求回执`

当前待决三条的完整证据在上游归档：
`ArcForge/.arcforge/archive/sprint-010-2026-09-04/docs/06-acceptance/final-report.md` 第 4 节（P3 / P1 / P4）
