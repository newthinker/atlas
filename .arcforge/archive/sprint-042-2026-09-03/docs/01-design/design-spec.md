# 任务图设计 · Sprint M1c-4

> 技术设计在上游 spec（`2026-09-01-hestia-backfill-finalize-design.md`）与需求文档里，**本文件不复述**。
> 这里只定义 arcforge 层的任务图：切分、依赖、wave、scope 声明。

## 1. 任务切分与需求文档 Task 的对应

需求文档的 13 个 Task **一一对应** 13 个 TASK-00N，编号相同。两处边界调整（理由见 AD-M1c4-3）：

| | 需求文档原划分 | 本 sprint 的划分 | 变更理由 |
|---|---|---|---|
| TASK-004 | 「只加字段与断言，不接抽取」 | 加字段 **+ 模板表加 `momField` 列 + yaml 占位填表** | `TestTemplateTablesCoverAllFields` / `TestShippedConfigLoadsAndIsCalibrated` 使原划分留下跨 wave 红测试 |
| TASK-005 | 模板表带 mom 字段 + 抽取路由 | **只做抽取路由**（`sectorCaliberOf` / `pick()` / 三处 extract） | 模板表的数据结构改造已并入 TASK-004 |

其余 11 个任务的射程与需求文档逐字一致。

## 2. 依赖图（`scheduling: dag`——依赖全部 `verified` 即就绪，不等整个 wave）

```
wave 1   TASK-001 (strip.go)          TASK-004 (fields/profiles/yaml 扩容)
            │                              │
wave 2   TASK-002 (periodAlt) ────────────┤        ← 依赖 001(基线) + 004(profiles.go 串行)
            │                              │
wave 3   TASK-003 (moneyRE)               │        ← 依赖 002
            │                              │
wave 4   TASK-005 (抽取路由) ◄────────────┘        ← 依赖 003 + 004
            │
wave 5   TASK-006 (必填集口径感知)                  ← 依赖 004 + 005
            │
wave 6   TASK-007 (闸门按族校验)                    ← 依赖 006
            │
wave 7   TASK-008 (PrecedingAll)  TASK-009 (残差分布)  TASK-011 (路由断言)
            │                        │                    │
wave 8      │                     TASK-010 (人工标定)     │   ← 依赖 009 + 004
            │                        │                    │
wave 9      └────────── TASK-012 (真跑验收 + CONTRACTS) ───┘   ← 依赖 008 + 010 + 011
                                     │
wave 10                        TASK-013 (人工切库)             ← 依赖 012
```

**约束 `本任务.wave > max(依赖.wave)` 逐条成立。**

### 2.1 依赖的两种来源（都要声明，但成因不同）

| 类型 | 例子 | 若漏声明会怎样 |
|---|---|---|
| **接口依赖**（要用上游产出的符号） | 006 依赖 004 的 `fieldOrder` 22 字段；007 依赖 006 的 `caliberFamilies`；009 依赖 007 的 `depositResidualOf` | 编译失败——**响亮，会被发现** |
| **同文件串行**（scope 互斥） | 002/003/005 都写 `profiles.go`；006/007/008 都写 `validate.go` | 并行派发 → validator `scope-mutex` 阻断，或（若绕过）**merge 冲突** |
| **基线依赖**（背对背比对要建立在上游之上） | 002 依赖 001 | 🔴 **静默**——001 改变全部 218 篇的文本，若 002 先做，它的背对背基线在 001 落地后失效，而**没有任何东西会报错** |

⚠️ 第三类是需求文档特意点明的（Task 1「为什么排第一」）。它不产生编译错误、不产生冲突，**只产生一份看起来正常的错误基线**。

## 3. Scope 声明表（`packages` 宽 / `writes` 窄，见 AD-M1c4-2）

全部任务 `packages: ["./internal/hestia"]`（TASK-010 另加 `configs`，TASK-012/013 见下）。

| TASK | `writes`（文件级，互斥口径） |
|---|---|
| 001 | `strip.go` `strip_test.go` |
| 002 | `profiles.go` `profiles_test.go` `extract_test.go` |
| 003 | `profiles.go` `profiles_test.go` |
| 004 | `fields.go` `fields_test.go` `profiles.go` `profiles_test.go` `golden_test.go` `thresholds_test.go` `config_test.go` `configs/hestia.yaml` |
| 005 | `extract.go` `extract_test.go` `profiles.go` `profiles_test.go` `testdata/` |
| 006 | `required.go` `required_test.go` `validate.go` `validate_test.go` |
| 007 | `validate.go` `validate_test.go` `thresholds.go` `thresholds_test.go` |
| 008 | `store.go` `store_test.go` `validate.go` `validate_test.go` |
| 009 | `calibrate.go` `calibrate_report.go` `calibrate_report_test.go` |
| 010 | `configs/hestia.yaml` `thresholds.go` `fields.go` `config.go` `config_test.go` |
| 011 | `backfill_load_report.go` `backfill_load_report_test.go` |
| 012 | `CONTRACTS.md` `calibrate.go` + ClawdVault 的 ADR-0003 |
| 013 | `.arcforge/docs/07-deploy/` 下的切库记录（**无源码 writes**） |

**互斥核对**（同一 wave 内两两无交集）：
- wave 1：`{strip.go}` ∩ `{fields.go, profiles.go, yaml, ...}` = ∅ ✓
- wave 7：`{store.go, validate.go}` ∩ `{calibrate*.go}` ∩ `{backfill_load_report*.go}` = ∅ ✓
- 其余 wave 均单任务。

⚠️ **`profiles.go` 出现在 002/003/004/005 四个任务的 `writes` 里** —— 它们分属 wave 1/2/3/4，**任意时刻只有一个在途**，互斥成立。这正是为什么依赖链不能压缩。

## 4. 无源码任务的声明（CLAUDE.md「无代码任务声明」）

- **TASK-013** 无源码改动 ⇒ `packages` 指向文档路径，`done_criteria` **全部用对象形态**标 `verify_by: manual`（人执行的命令，无法自动验）。
- **TASK-010** 有源码改动（yaml + 三处订正），但**填值是人的判断** ⇒ 混合：数据采集与落盘条目标 `verify_by: test`，填值合理性标 `verify_by: review`。
- **TASK-012** 主要产出是文档，但含 `calibrate.go` 的措辞修改 ⇒ 真跑核对条目标 `verify_by: manual`，代码改动标 `verify_by: test`。

## 5. 团队编制

| 角色 | 实例名 | 数量 | 承接 |
|---|---|---|---|
| dev | `dev-m1c4-a` / `-b` / `-c` | 3 | 见 AD-M1c4-6；`-a` 走 `profiles.go` 主链（002/003/005），`-b` 走扩容与验证链（004/006/007），`-c` 走并行支线（001/009/011）与人工任务记录员（010/013） |
| test | `test-m1c4-a` / `-b` | 2 | 交叉验证，**不验自己队友刚交的**（同一 dev 连续两个任务时换验证者） |
| qa | `qa-m1c4` | 1 | 全体 verified 后两轮 review |

⚠️ 实例名必须带 `-m1c4`：矩阵已登记 120 个旧实例名（`dev-agent-1..57`、`dev-m1c3b-*` 等），复用会拿到别人的 token 语义。

## 6. 全局纪律（写进每个任务的 DoD，不指望 dev 记得）

1. **AD-3 语料绝对路径** + `--allow-incomplete`（需求文档的 `$PWD/data/...` 在 worktree 里必然失败）
2. **AD-4 merge 先于 `dev_done`**（门禁的 `git log --grep` 不带 `--all`）
3. `gofmt` 判据是「`cmd/atlas/backtest_test.go` 与 `crisis_test.go` **之外**没有新增项」——**不要顺手修那两个**
4. 覆盖率 **≥96.1%**（实测锚 `4670ccb`），报数时**同行写下你测量时的 HEAD 全 sha**
5. **无新增依赖**：`go.mod` / `go.sum` 不得出现在实际改动里
6. 注释里的任务编号**带 milestone 前缀**：写 `M1c-4 的 TASK-00N`
7. **背对背基线**：两份二进制**同一时刻并排跑**；跨时间点采样会产生假差异
8. 自证数字**采于最后一次改动之后**，以 `git show --numstat HEAD` 为准复核
9. 一切 `.arcforge/` 读写**必须 cd 回主仓库**（worktree 里是独立副本，写通道会 DENY 或静默写坏）
10. 收尾**自己拆 worktree**（谁建谁拆）
