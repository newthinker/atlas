# Hestia M3 · Arcforge 承载设计

本文件只描述**承载层设计**（任务怎么切、证据落在哪、门禁怎么过）。
业务设计以需求文档为唯一真相源，本文件**不复制**其接口签名与实现代码。

## 1. 任务图

```
wave 1        wave 2              wave 3
TASK-001 ───► TASK-002 ─────┐
(atlas         (atlas         │
 internal/      cmd/atlas)    ├──► TASK-006
 hestia)                      │    (nanoclaw scripts)
TASK-003                      │
(loom spool)                  │
TASK-004 ───► TASK-005 ───────┘
(nanoclaw      (nanoclaw
 分支/挂载)     skill 文档)
```

| Arcforge | 计划原任务 | 仓库 | wave | dependencies | 形态 |
|---|---|---|---|---|---|
| TASK-001 | TASK-001 上半 | atlas | 1 | — | Go（标准门禁） |
| TASK-002 | TASK-001 下半 | atlas | 2 | 001 | Go（标准门禁） |
| TASK-003 | TASK-002 | loom | 1 | — | 跨仓库（交付记录） |
| TASK-004 | TASK-003 | nanoclaw | 1 | — | 跨仓库（交付记录） |
| TASK-005 | TASK-004 | nanoclaw | 2 | 004 | 跨仓库（交付记录） |
| TASK-006 | TASK-005 | nanoclaw | 3 | 002, 005 | 跨仓库（交付记录） |

计划的 TASK-006（集成冒烟）**结转**，不建任务文件（理由见 `requirements-analysis.md` §6）。

## 2. 为什么把计划的 TASK-001 拆成两个

计划的 TASK-001 横跨 **2 个 package**（`internal/hestia` + `cmd/atlas`）、**7 个文件**，
两项都超出 Realistic Scope（≤1 package、≤5 文件、≤8 条 DoD）。拆法：

- **TASK-001**（`internal/hestia`，5 文件）：`history.go` / `history_test.go` 新建，`ingest.go` /
  `ingest_test.go` 接线，`store_test.go` 登记 AST 守卫。
  ⚠️ 守卫登记**必须与导出同任务**——`BuildHistory` 等一旦导出而 `want` 未更新，守卫测试当场变红。
- **TASK-002**（`cmd/atlas`，2 文件）：`contract emit` 同产侧车 + 对应测试。依赖 001 的导出面。

拆开的额外收益：TASK-002 是**独立可验的交付**（「回放也要产出侧车，消费者不分实时与回放」），
而不是 TASK-001 的尾巴。

## 3. 跨仓库任务的承载（AD-M3-1 的落地形态）

TASK-003…006 的真实代码在 loom / nanoclaw，**atlas 的 git 完全看不见**。承载方式：

每个跨仓库任务在 atlas 建**一份交付记录文档**，作为该任务的 `packages` 与 `writes`：

```
docs/hestia-m3/TASK-003-loom-spool-source.md
docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md
docs/hestia-m3/TASK-005-warp-hestia-docs.md
docs/hestia-m3/TASK-006-warp-hestia-scripts.md
```

**这份文档不是摘要，是证据载体**，固定四节：

1. **改动清单**：目标仓库里逐个文件的路径 + `git show --numstat <sha> -- <file>` 输出
2. **实测输出**：目标仓库里跑测试的**原样粘贴**（含命令、退出码、计数），采样锚写全 sha
3. **锚点**：目标仓库 `git rev-parse HEAD` 全 sha + 分支名 + PR 链接（若有）
4. **未做与理由**：计划里本任务范围内但没做的，逐条说明

`done_criteria` **全部**用对象形态标 `verify_by: manual`（触发门禁的「无代码任务」分支，
跳过 Go 门禁但仍要求「声明范围内确有变更」）。验证者**不以这份文档为准**——它按文档给的锚点
**去目标仓库实跑**，文档只提供可复现的入口。

## 4. 三条门禁纪律（写进每个任务的 description，不靠记性）

1. **atlas 提交信息必须锚定** `<type>(TASK-00X): ...`——门禁用 `git log --grep` 认领改动，
   只提 ID 不算，不合约定 BLOCKED（可 `--amend` 补救）。
2. **merge 必须在 `dev_done` 之前**——`task-completed.sh` 的 `git log --grep` **不带 `--all`**，
   只走 HEAD 祖先链 ⇒ 未合并分支上的 commit 对门禁结构性不可见，两个集合双双为空所以「报绿」。
   顺序固定为：worktree 提交 → 回主仓库请 Leader merge → **merge 落地后**才转 `dev_done`。
   ⚠️ teammate-idle hook 在此状态的解锁文案恒为「推进 dev_done」，**方向相反，不要照做**。
3. **`discovery` 必须在 `dev_done` 之前写完并挂指针**（`update --field discovery=`）——
   `dev_done` 之后写会让 `discovery_sha256` 漂移，`verified` 之后时机守卫无条件 DENY，dev 侧无出路。

## 5. worktree 隔离

- **atlas**（TASK-001/002）：按 CLAUDE.md 协议，`git worktree add -b task/TASK-00X ../wt-TASK-00X master`；
  一切 `.arcforge/` 读写 **cd 回主仓库**。
- **nanoclaw**（TASK-004/005/006）：本机 checkout 在 `feat/vendor-agent-reach-skill` 且工作区脏
  ⇒ **不碰它**。改用 `git worktree add -b feat/warp-hestia <临时目录> fork/main`（人类 2026-09-08 裁决）。
  TASK-004 建、TASK-006 完成后拆；005/006 复用 004 建的那个 worktree（故三者串行，不并行）。
- **loom**（TASK-003）：工作区有未提交改动（`.claude/hooks/arcforge-write.sh` 被改 + 一个未跟踪脚本），
  与 spool 无重叠 ⇒ 同样用 worktree 从 `main` 切 `feat/spool-source-param`，不碰当前工作区。

## 6. scope 互斥

| 任务 | packages | writes |
|---|---|---|
| TASK-001 | `internal/hestia` | 同 |
| TASK-002 | `cmd/atlas` | 同 |
| TASK-003 | `docs/hestia-m3/TASK-003-loom-spool-source.md` | 同 |
| TASK-004 | `docs/hestia-m3/TASK-004-nanoclaw-branch-mount.md` | 同 |
| TASK-005 | `docs/hestia-m3/TASK-005-warp-hestia-docs.md` | 同 |
| TASK-006 | `docs/hestia-m3/TASK-006-warp-hestia-scripts.md` | 同 |

两两互斥 ✓。四份跨仓库文档路径互不相同，故 wave 内可并行。

## 7. 测试策略

- **TASK-001/002**：TDD（RED→GREEN→REFACTOR），测试代码计划已给出，dev **照抄后逐行核对**是否与本仓库
  既有 helper 签名相符（`newTestStore` / `contractObs` / `pendingContracts` / `emitFixture` 均已核实存在）。
  覆盖率贴地 96.6%，错误分支必须有测试。
- **TASK-003**：loom `go test ./...` 全绿 + 四条新测试；12 处 `injectTaint(` 调用点全改。
- **TASK-005/006**：`python3 -m unittest`；三期 golden 温度 2 / 0 / 1 钉住。
- **验证**：Test Agent 按各任务文档给的锚点去目标仓库实跑，不接受「文档里写着绿」。
