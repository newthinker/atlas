# 待决机制变更（Arcforge）

**这份文件只放「等人拍板的机制变更」，不放证据、不放教训、不放叙事。**

超过 40 行就是失败了 —— 它存在的全部理由是 `wisdom/decisions-leader.md` 已经 32KB、
`wisdom/_digest.md` 已经 88KB，而下一个 leader 需要的是这几行。

⚠️ **改动落点是 Arcforge 上游仓库**（`newthinker/ArcForge`）—— 本仓库是消费项目，无 `project-template/`。

---

## 当前待决（上游 sprint-010 实测 + atlas M1.5/Sprint 044 实测，2026-09-05 记）

| # | 变更 | 落点 |
|---|---|---|
| 1 | `teammate-idle.sh` 的 `test-*` 分支补 F6 防空转出口：现在任意 `dev_done` 无条件并入 `MINE`，而 `dev_done→verifying` 是 leader 专属边 ⇒ 等派验期间无限唤醒（实测一分钟四次）。相邻 `qa-*` 分支有现成同构写法 | `project-template/hooks/teammate-idle.sh` |
| 2 | 逃生边把判定依据写进审计行：现在只记时间，文件层面无从区分「有证据的快」与「没证据的急」（上游十条逃生边有两条事后证明误判） | `project-template/hooks/arcforge-write.sh` |
| 3 | `stale-dispatch` 处置补一条判据：委托子代理致卡死时无 failure 通知（卡 running 不转 idle），只能靠「零文件产物 + worktree 未被触碰」识别。**atlas M1.5 第二实例**（dev-m15-b 的 code-simplifier 子代理被 idle hook 循环 5 次，靠子代理自报） | `global/agents/*.md`、`teammate-idle.sh` |
| 4 | **会话/消息挂起无告警**：M1.5 一个 sprint 内 Leader 27m + dev 45m + dev 23m + verifier 57m，M1d Leader 两次；形态一致（工具调用之间被挂起、被挂起方无间隙感知），`in_progress` 无阈值 ⇒ 只能靠催办。候选：merge 请求超阈值无回执 ⇒ 自动 `blocked_clarification` | 上游议题 |
| 5 | 任务 ID 跨 sprint 复用致门禁 `git log --grep` 全集含旧提交（只 WARN，无害）；`capabilities.codex_cli: true` 与实际不符（QA `codex exec` 30 分钟零输出）——探测缺「可运行」验证 | `task-completed.sh`、`arcforge-init` |
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
