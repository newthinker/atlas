# M2b Sheets 投影 —— 需求分析（Arcforge 侧）

- **需求文档**：`~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-16-hestia-m2b-sheets.md`（1958 行）
- **上游 spec**：`superpowers/specs/2026-09-16-hestia-m2b-sheets-design.md`
- **目标仓库**：**atlas（本仓库）单仓库** —— 与上一 sprint M3 的跨三仓库形态不同
- **分析日期**：2026-09-16

---

## 1. 这份需求文档已经做完了什么

它是 `writing-plans` 的产出，**不是待细化的需求，而是一份已经拆好的实施计划**：

- 11 条全局约束（C1–C11）逐条带 spec 出处
- 12 个任务的 Files / Interfaces / 依赖 / 分步 TDD 流程，每步带可运行命令与预期输出
- 7 条验收判据，带 2026-09-16 的实测基线数字（将写 1282 / 一致 34 / 库缺 819）

⇒ **Arcforge 的 Step 2/3 不应重做这件事。** 本轮 Leader 的增值点只有三处：

1. 把 12 个任务映射成 Arcforge 任务图（依赖 / wave / scope / context_from）
2. 把「步骤 + 断言」转成**可被验证者逐条对照的 `done_criteria`** —— 需求文档写的是"怎么做"，DoD 要写的是"怎么算做完了"
3. 识别需求文档没有覆盖的 Arcforge 侧风险（见 architecture-decisions）

## 2. 任务图

```
001 AST守卫递归
 ├── 002 Store.AllPeriods ──┐
 └── 003 子包骨架+表头解析 ──┼── 004 选行+字段映射 ─┐
      ├── 005 diff 计算 ────┤                      │
      └── 006 API薄壳+httptest ┘                    │
                              └── 007 push编排+dry-run ┬── 008 建年度表
                                                       ├── 009 CLI 子命令
                                                       └── 010 ingest 接线 ── 011 配置与文档
```

| wave | 任务 | 可并行性 |
| --- | --- | --- |
| 1 | 001 | 单任务，**必须最先**（见下） |
| 2 | 002 / 003 | 并行，scope 不重叠 |
| 3 | 004 / 005 / 006 | 并行，004 写 `internal/hestia/*`，005/006 写 `sheets/*` |
| 4 | 007 | 汇聚点（依赖 005+006） |
| 5 | 008 / 009 / 010 | 并行，分别写 `sheets/*`、`cmd/atlas/*`、`internal/hestia/ingest*` |
| 6 | 011 | 收尾 |

**001 必须最先，理由不是依赖而是机制**：先改守卫意味着**加子包那一刻守卫立刻要求登记**——登记这个动作被强制，而不是靠实施者记得。顺序反过来的话，子包先落地、守卫后改，中间那段守卫是瞎的。这条是需求文档明写的，也是本任务图唯一一处"依赖关系之外的排序理由"。

## 3. scope 互斥分析（validator 会校验，这里先自查）

**三个文件被多个任务写**，但都靠依赖串行错开，不会同时在途：

| 文件 | 写它的任务 | 是否可能并发 |
| --- | --- | --- |
| `internal/hestia/store_test.go` | 001 / 002 / 004 | 否 —— 001→002→004 是依赖链 |
| `internal/hestia/sheets/client.go` | 006 / 008 | 否 —— 006→007→008 |
| `internal/hestia/sheets/push.go` `push_test.go` | 007 / 008 | 否 —— 007→008 |

⇒ DAG 调度下安全。但**若有人手工并行放行 006 与 008 就会撞车**，这一条写进 plan.md 提示。

## 4. 覆盖率口径（`packages`）与互斥口径（`writes`）的分歧点

大部分任务两者相同。**只有三个任务需要拆开**：

| 任务 | `packages`（宽，进 `-coverpkg`） | `writes`（窄，只列真写的） | 分歧理由 |
| --- | --- | --- | --- |
| 004 | `internal/hestia` + `internal/hestia/sheets` | 不含 `sheets/` 下已有文件 | 只**新建** `sheets/row.go`，其余 sheets 文件只读消费 |
| 009 | `cmd/atlas` | 同左 + `cmd/atlas/hestia.go` | 无分歧，列全 |
| 011 | **非 Go 路径** | `configs/*.yaml` + `CONTRACTS.md` + vault 两份 md | 纯文档任务，见 AD-M2b-2 |

## 5. 需求文档中不进 Arcforge 任务图的部分

**TASK-012「真实验收（人执行）」不建任务文件。** 它的六步全部需要真实 Google 凭据、真实表、真实断网，且 spec §10 的判据四/五/六/七都要人对着真表确认。建成任务只会卡在 `blocked_human`。

⇒ 处理方式：写进 `03-progress/plan.md` 的结转段与 final-report 的「未完成/需人执行」节，并在 TASK-011 的 DoD 里要求**把 §10 七条判据的表格骨架写进 CONTRACTS**，供人执行后回填实测值。

## 6. 与上一 sprint（M3）的形态差异

| | M3 | M2b（本轮） |
| --- | --- | --- |
| 仓库 | 跨 atlas / loom / nanoclaw | **单仓库 atlas** |
| `dev_done` 门禁 | 五个任务在目标仓库，atlas 侧只有文档 ⇒ 门禁量不到真交付 | **真实触发**（仓库根有 `go.mod`） |
| `verify_baseline` | 够不到目标仓库（PENDING #9） | **完全覆盖** |
| 验证者可跑的东西 | 要去别的仓库实跑 | `go test ./...` 在本仓库直接跑 |

⇒ **M3 记录的多数跨仓库机制风险本轮不适用。** 但 TASK-011 要写 Obsidian vault 里的两份 md，那部分重新引入了"交付物在仓库外"的问题，见 AD-M2b-2。
