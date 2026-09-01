# M1c-3b 任务拆分方案

## 1. 编号映射（🔴 与需求文档一一对应，不要重编号）

需求文档里每个 Task 的 Step 都给了**写死的 commit 命令**（`refactor(TASK-001): ...`），
而 `dev_done` 门禁要求提交信息锚定 `<type>(TASK-ID):`。**重编号会让 dev 照抄文档时用错 ID
而门禁静默放行**（它匹配不到就当作「本任务无提交」，两个集合双双为空 ⇒ 报绿）。

⇒ **TASK-001…007 与需求文档 Task 1…7 逐一对应，编号相同。**
只有文档的 Task 8 被拆开（它 7 个文件、8 条结转项，超 Realistic Scope）：

| 本 sprint | 需求文档 | 说明 |
|---|---|---|
| TASK-001 | Task 1 | 抽出 `eachParsedArticle` 共用管道（纯重构） |
| TASK-002 | Task 2 | `merged@v1` 的取值域与必填集 |
| TASK-003 | Task 3 | 按业务键合并 Observation |
| TASK-004 | Task 4 | 堵住 `MagnitudeRanges` 打错字段名的静默失效 |
| TASK-005 | Task 5 | 阈值重标（人工填值 + 默认值同步） |
| TASK-006 | Task 6 | `BackfillLoad` 编排与核对报告 |
| TASK-007 | Task 7 | `backfill load` 子命令 |
| **TASK-008** | Task 8 的一部分 | 结转项 · calibrate 族四条（R9 / 1a / 9 / 矛盾标签） |
| **TASK-009** | Task 8 的一部分 | 结转项 · sections + parse 两条（R10 / R11） |
| **TASK-010** | Task 8 的一部分 | 真跑验收 + CONTRACTS 登记 + 移交承接句 |

⚠️ TASK-008/009/010 的 commit message 用**它们自己的编号**，不是 `TASK-008` 一个。
需求文档 Task 8 的那条 `docs(TASK-008): ...` 命令**只适用于 TASK-010**。

## 2. 任务图

| ID | packages（覆盖率口径，宽） | writes（互斥口径，窄） | deps | wave |
|---|---|---|---|---|
| 001 | `./internal/hestia` | `calibrate.go` `calibrate_test.go` | — | 1 |
| 002 | `./internal/hestia` | `types.go` `required.go` `required_test.go` `types_test.go` | — | 1 |
| 004 | `./internal/hestia` | `thresholds.go` `thresholds_test.go` | — | 1 |
| 009 | `./internal/hestia` | `sections.go` `sections_test.go` `parse.go` `parse_test.go` | — | 1 |
| 003 | `./internal/hestia` | `backfill_load.go` `backfill_load_test.go` `store_test.go` | 001, 002 | 2 |
| 005 | `./internal/hestia` | `thresholds.go` `thresholds_test.go` `config_test.go` `configs/hestia.yaml` | 004 | 2 |
| 008 | `./internal/hestia` | `calibrate.go` `calibrate_report.go` `calibrate_test.go` `calibrate_report_test.go` | 001 | 2 |
| 006 | `./internal/hestia` | `backfill_load.go` `backfill_load_report.go` `backfill_load_test.go` `backfill_load_report_test.go` `store_test.go` | 003 | 3 |
| 007 | `./cmd/atlas` | `cmd/atlas/hestia.go` `cmd/atlas/hestia_test.go` | 006 | 4 |
| 010 | `./internal/hestia` `./cmd/atlas` | `internal/hestia/CONTRACTS.md` | 001–009 | 5 |

（`writes` 里未写目录前缀的一律指 `./internal/hestia/`，任务 JSON 里是全路径。）

### 2.1 为什么 `writes` 必须是**文件级**而不是包级

十个任务全部落在 `internal/hestia` 一个包里。若 `writes` 声明成 `./internal/hestia`，
validator 的 `scope-mutex` 会判**任意两个任务互斥** ⇒ 全 sprint 退化为完全串行，
`max_dev_agents: 4` 一个也用不上。

⇒ `writes` 逐文件列，`packages` 仍是包（覆盖率口径不变）。
这正是 CLAUDE.md「两个口径别混用」那一节说的形态。

⚠️ 由此每个任务都会命中告警级 `scope-writes-outside-packages`（条数恒等于 `writes` 长度）。
**这是已知的零信息量告警，不要为消它去改 `packages`**——把 `configs/hestia.yaml` 这类
非 Go 路径塞进 `-coverpkg` 会弄坏覆盖率门禁。

### 2.2 scope 互斥逐格核对（DAG 模式，就绪即派，需查跨 wave 并发）

| 会同时在途的组合 | 冲突？ |
|---|---|
| {001, 002, 004, 009} | 无 —— calibrate / types+required / thresholds / sections+parse 四族不相交 |
| {002, 004, 009, 008} | 无 —— 008 在 001 verified 后才就绪，与 002/004/009 不相交 |
| {004, 009, 008, 003} | 无 |
| {009, 008, 003, 005} | 无 —— 005 的 thresholds 与 004 串行（deps），004 此时已 verified |
| {009, 008, 005, 006} | 无 —— 006 的 backfill_load 与 003 串行 |
| {008, 005, 006→007} | 无 —— 007 只碰 cmd/atlas |

**串行边有三处，都由 deps 保证而非靠运气**：
`004 → 005`（同写 `thresholds.go`）、`003 → 006`（同写 `backfill_load*.go`）、
`001 → 008`（同写 `calibrate.go`）。

### 2.3 wave 序核对（`本任务.wave > max(依赖.wave)`）

`003(2)>1` `005(2)>1` `008(2)>1` `006(3)>2` `007(4)>3` `010(5)>4` ✓

## 3. 嵌入实测数字的产物 ↔ 谁负责重采

（CLAUDE.md 记的坑：「依赖变化后谁重采」这个角色不存在 ⇒ 数字过期四轮无人发现。
拆任务时逐个问「它在谁的 `writes` 里」，答不出的就是无人重采的。）

| 产物 | 嵌的数字 | 在谁的 writes 里 | 谁重采 |
|---|---|---|---|
| `thresholds.go` 注释 | 0.02613 / 0.13338 / n=68 / n=6 | TASK-005 | TASK-005 dev |
| `calibrate.go:186` 注释 | 「真语料 19 篇」 | TASK-008 | TASK-008 dev |
| `sections.go:116` 注释 | 「共 55 篇」 | TASK-009 | TASK-009 dev |
| `CONTRACTS.md` M1c-3b 节 | 42 / 107 / 96 / 161 | TASK-010 | TASK-010 dev |
| **`.arcforge/docs/06-acceptance/final-report.md`** | 全部 | **没有 dev owner** | **Leader，且必须现采**（见 AD-7） |

## 4. Realistic Scope 核对

⚠️ **本表的条数是 `jq` 实数的，不是估的**（初稿写成 5–8 条是拍的，与实际不符，已订正）：

```bash
jq -r '[.id,(.packages|length),(.writes|length),
        ((.done_criteria.functional|length)+(.done_criteria.boundary|length)
        +(.done_criteria.error_handling|length)+(.done_criteria.non_functional|length))]|@tsv' \
  .arcforge/tasks/*.json
```

| ID | packages 数 | writes 文件数 | DoD 总条数 |
|---|---|---|---|
| 001 | 1 | 2 | 9 |
| 002 | 1 | 4 | 9 |
| 003 | 1 | 3 | **11** |
| 004 | 1 | 2 | **10** |
| 005 | 1 | 4 | **10** |
| 006 | 1 | 5 | **12** |
| 007 | 1 | 2 | 7 |
| 008 | 1 | 4 | **10** |
| 009 | 1 | 4 | 8 |
| 010 | 2 | 1 | **11** |

### 4.1 达标项

- **≤1 package**：全部达标（010 的 2 个是**覆盖率口径**——它的 DoD 要求跑全量测试；它的 `writes` 只有 1 个文件）
- **≤5 文件**：全部达标（最大是 006 的 5 个）

### 4.2 🔴 未达标项：`done_criteria` 总条数 ≤ 8

**六个任务超限**（003/004/005/006/008/010，最多 12 条）。如实报告，不粉饰。

**为什么不继续拆**：

1. **条数多不等于范围大**。真正衡量范围的两个指标（package 数、改动文件数）全部达标。
   条数多是因为**需求文档本身是 step 级计划**——它给出的验收点就是密集的
   （光 Task 6 的实现要点就 9 条）。
2. **合并条目会降低可验证性**。DoD 的消费者是 Test Agent，它**逐条对照**。
   把「四道恒等式」「拒绝已存在的 DB」「单期失败不中断」压进一条，
   验证者只能给这一条一个整体结论 —— 而这三件事任一不成立，结论都该不同。
3. **按文件再拆会撞进 scope 互斥**。003/006 同写 `backfill_load.go`，008 与 001 同写
   `calibrate.go`，这三对已经是靠 `dependencies` 串行的；继续拆只会增加串行边，
   减少并行度，而不减少任何一个任务的实际工作量。

**⇒ 提请人类在 dod-gate 决定**：接受超限，或指定要拆的任务。

## 5. 团队编制

wave 1 有四个可并行任务 ⇒ **dev × 4**（`max_dev_agents: 4` 用满），**test × 2**。
QA 在全体 `verified` 后单独 spawn。
