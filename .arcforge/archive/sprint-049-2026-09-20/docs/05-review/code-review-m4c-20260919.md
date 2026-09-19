# Code Review — sprint M4-crisis（crisis 库双时态迁移）

- 审查者：`qa-m4c-a`
- base：`e799bf5a41d9049b0a987f189fab3be50b987186`（master），范围 `git log --oneline 06a86ca..HEAD` + 未提交的 `internal/hestia/CONTRACTS.md`（+219/−0）
- 套件：`go test ./internal/crisis/... ./cmd/atlas/... ./internal/macro/bitemporal/...` **全绿**；`go vet` rc=0
- 主工作区完整性：审查全程 `internal/crisis/store.go` sha256 恒为 `f081dd6dfdd73c99edccd5a56b01716291e91b928e1f59e530d54fddfbd1e52c`，`git status --porcelain` 与开工时逐字相同（变异一律在 `git archive HEAD` 的隔离副本里做）

## 结论：**CONTESTED**

理由，三条都要一起读：

1. 有一个 **HIGH** 级发现（R-1，启动守卫不校验视图定义），我已用可复现实验证实其后果是**静默读到迁移前的冻结数据、rc=0、无任何错误**。
2. 但它**不是在途缺陷**：当前生产库不受影响（`migrate.go` 的步骤表先 `DROP VIEW` 再 RENAME，视图按构造正确），本 sprint 的八个任务也没有一条 DoD 承诺过这件事。它是**相对于本仓库自己的先例（hestia）缺的一道防御**，而 `store.go` 的注释**点名引用了那个先例的两个函数、只实现了其中一个**。
3. 🔴 **第二轮对抗审查实际上没有执行**（见文末「审查过程的失效」）。所以我无法声称「多视角一致」——这份结论的置信度只等于一个 reviewer。

⇒ 不给 PASS（Reality Checker：有未解决的 HIGH/WARNING）；不给 REJECT（没有任何已交付的东西是坏的，生产健康）。**需要人判断**：把 R-1 开成后续任务，还是在本 sprint 内闸掉。

---

## 第一轮：常规审查

### 🔴 R-1 [HIGH] 启动守卫只校验**表**形状，不校验**视图**定义 —— 视图指向错表时静默返回冻结数据

- 位置：`internal/crisis/store.go:70-98`（`verifyBitemporalShape`）、调用点 `store.go:43-50`
- 先例：`internal/hestia/store.go:79-84` 同时调 `verifyObservationsSchema` **与** `verifyCurrentView`；后者（`hestia/store.go:109`）把库里的视图 SQL 与 `currentViewDDL(spec)` 派生的期望值**逐字比对**，理由写得很清楚：`CREATE VIEW IF NOT EXISTS` 对已存在的视图**不替换**。
- crisis 没有 `verifyCurrentView` 的对应物。而 `store.go:64-65` 的注释写的是「同 hestia 的先例（`internal/hestia/store.go` 的 `verifyObservationsSchema` / `verifyCurrentView`）」——**两个函数都点了名，只实现了第一个**。这正是本 sprint 反复出现的形状：守卫描述的集合比它实际检查的集合大。

**失败场景（已实测，非推理）**

用本 sprint 的二进制造一个正常迁移过的库，再把视图改写成指向 `macro_observations_v1`（就是 `migrate.go:117-119` 注释里写明「ALTER TABLE RENAME 会**静默改写视图定义**，已实测」的那个结果形态）：

```
库中视图定义 : CREATE VIEW v_macro_current AS SELECT * FROM macro_observations_v1 o WHERE ...
NewStore     : 通过（守卫放行）
Observation  : value=10 fetched_at=2026-07-14T00:00:00Z      ← 迁移前的冻结值
```

同一个库、视图正确时的对照：

```
NewStore     : 通过（守卫放行）
Observation  : value=99 fetched_at=2026-08-14T00:00:00Z      ← 正确的当前行
```

⇒ **两种库在守卫、退出码、输出形状上完全同形，只有值不同。** 迁移之后写入的一切对读路径永久不可见，而 `crisis_evaluations` 会照常把基于冻结数据的判定落盘。

**为什么现有测试抓不住**：`TestReadsFailLoudlyWhenViewMissing`（`store_test.go:507-520`）覆盖的是视图**被删**（响亮失败）。没有任何测试覆盖视图**在、但指向错表**。判据区分的是「有/无」，而危险状态是「有且错」——F7 的一般形式。

**可达性（这条决定它是 HIGH 不是 CRITICAL）**：我**没有找到**当前代码里能造出该状态的路径——`migrate.go` 的步骤表先 `DROP VIEW`，顺序是对的。它的到达路径是未来的：任何后续对 `macro_observations` 的 `ALTER TABLE ... RENAME`、半个回滚、手工修库、或用**视图定义有变更的未来版本**建的库被本版打开（这正是 hestia 注释里给的那个理由）。

**附带发现**：生产迁移日志（`docs/07-deploy/m4c-migration-log.md:138-151`）的双向 `EXCEPT` 判据**在原理上无法区分这两种状态**——迁移当时 `v_macro_current` 与 `macro_observations_v1` 内容按构造完全相同，视图指向任一张表，`EXCEPT` 都得 0。那个 `value+1` 正对照证明的是「尺子对**内容**差异有牙」，不是「视图读的是哪张表」。日志 `:162` 的形状清单对索引记了 `tbl=`、对视图**只记了名字**。
⚠️ 我**没有**核实生产库的视图当前指向哪张表（受「不碰生产库」约束）。按代码路径它应当是对的。要闭合只需一条只读查询：
`sqlite3 <生产库> "SELECT sql FROM sqlite_master WHERE type='view' AND name='v_macro_current';"`，确认其中出现的是 `macro_observations` 而非 `macro_observations_v1`。

---

### R-2 [MEDIUM] `TestWritePathHasNoReplaceOrIgnore` 的作用域是单个文件，而它声称守住的是「写路径」

- 位置：`internal/crisis/store_test.go:744-751`；对应 TASK-005 `functional[3]`
- 它是本 sprint 唯一一个**没有正向对照**的源码扫描守卫（两条 `NotContains`，零条正向断言）。

**变异实测（隔离副本，`git archive HEAD`）**

| 变异 | 结果 |
| --- | --- |
| M1：`store.go` 内把 `INSERT` 改成 `INSERT OR REPLACE` | `TestWritePathHasNoReplaceOrIgnore` 红，**且只有它红** |
| M2：把该 SQL 搬到同包新文件 `write_sql.go` 并改成 `INSERT OR REPLACE` | **全绿**（`ok internal/crisis 0.586s`） |

M1 的价值在于它顺带证明了：**`INSERT OR REPLACE` 回归在行为层零覆盖**——因为 `INSERT` 只在 `errors.Is(err, sql.ErrNoRows)` 分支里执行（`store.go:176-180`），事务内永远不会真撞冲突，所以 `OR REPLACE` 是行为惰性的。⇒ 这个源码守卫是该回归的**唯一探测器**，而它的作用域是一个硬编码文件名。

**同 sprint 的对照**：`TestNoBareTableReadsOutsideMigration`（`store_test.go:555`）与 `TestObsSpecIsSingleInstance`（`schema_test.go:143`）都用 `filepath.Glob("*.go")` 扫全包，且都有正向对照（`assert.Len(found, len(allowedBare))` / `assert.Equal(1, total)`），空集不会空真通过。同一位作者在同一 sprint 里写对了两次、写窄了一次。

建议：改成扫全包 + 加一条正向对照（断言 `INSERT INTO macro_observations` 恰出现 1 次）。

---

### R-3 [MEDIUM] `CONTRACTS.md` §G2 的运维警告已被本 sprint 自己的守卫证伪（未提交件，现在改代价最低）

- 位置：`internal/hestia/CONTRACTS.md:4672`（未提交）
- 原文：「**源码仓库目录下也有一个 `data/crisis.db`**（一个 7 月的旧库）。在源码目录误跑 `crisis replay` 会读到它，**输出有数据、有状态机摘要、不报任何错**——看起来和跑对了完全一样。这是本次迁移里最容易骗过人的一个目标。」

**实测证伪**（隔离副本 + 本 sprint 二进制，未碰仓库内的 `data/crisis.db`）：

```
$ atlas crisis replay --from 2026-06-01 --to 2026-07-10 ...
Error: crisis: macro_observations in data/crisis.db still has the legacy two-part primary key ...
rc=1
```

成因：仓库内 `data/crisis.db` 是**两段主键**（我在拷贝上查过 DDL），且 `crisis_evaluations` **0 行**。TASK-003 的守卫正是为拦这个而写的，它拦住了。

⇒ §G2 把一个**迁移前**的坑写成了迁移后的现状，而且是写在「两个会骗人的运维坑」这种运维直接照着做的位置。危害方向：运维被告知会遇到静默成功，实际遇到的是响亮失败，容易被误读成新故障；同时它**低估了本 sprint 交付的守卫的价值**。
⚠️ 坑的**类别**（相对 `storage.path` + 诱饵库）是真的，该留；假的是它描述的**症状**。

---

### R-4 [MEDIUM] `docs/deployment.md` 的部署后判据：结论今天成立，但写下的理由不是真正起作用的那个

- 位置：`docs/deployment.md:292`
- 原文判据：「首行为 `system state: …`，不是 `rc=0`」，理由是「`system state:` 这一行在全新空库上**构造上不可能出现**（空库恒为 `no evaluations yet — …`），所以它是这里唯一可用的判据」。

它要防的失效是「**验错了库**」，而它实际区分的是「**库空不空**」。空库只是验错库的一个实例。今天之所以够用，主要不是因为那句理由，而是因为**唯一一个有数据的诱饵库（仓库内 `data/crisis.db`）是老形状、被形状守卫挡在前面**（见 R-3 实测）。两道保护里，文档只写了较弱的那一道。

证据上的问题：那组实测（`deployment.md:295-301` 的三行表）声明是「2026-09-19，**临时目录**，未碰生产」。临时目录里不存在 `data/crisis.db`，⇒ **这个判据只在它成立的那个样本上被求值过**。

顺带一个一步可达的小坑：守卫的补救文案把它**被调用时的路径**原样回显——在错目录下它会打出 `run: atlas crisis migrate-bitemporal --db data/crisis.db`，即**指示运维去迁移那个诱饵库**。危害有限（诱饵是 7 月的开发遗留），但这是「守卫指路指向错误对象」。

建议：判据改成钉住**身份**而不是**非空**，例如核对首行的 `as of <日期>` 与 eval days 数，或直接比 inode（`CONTRACTS.md` §G1 自己给的就是「核实方法是比 inode，不是比路径字符串」——两份文档在这里不一致）。

---

### R-5 [LOW] `Store.AsOf` 的文档承诺覆盖「读方法」，实际只覆盖观测读

- 位置：`internal/crisis/store.go:100-112`（「`AsOf` 返回一个只读副本，其读方法只看到 t 时刻及之前的修订」）
- `asOf` 只经 `obsFrom()`（`store.go:223-228`）生效。`crisis_evaluations` 的五个读方法（`RecentSystemEvals` / `RecentIndicatorEvals` / `LatestSystemEval` / `HasSystemEvalForDate` / `HasIndicatorEvalForDate`，共用 `evalSelect`，`store.go:338-339`）**完全不受影响**，尽管该表有 `eval_at` 这个现成的修订轴。
- 当前无调用方踩到：`crisis replay` 走 `MemHistory`、零写入、不读该表。这是 LOW 而不是更高的唯一理由。
- 风险形态：`st.AsOf(t).History(ctx)` 是合法调用、编译通过、不报错，得到的是**as-of 的观测 + 当前的评估**混合时间轴。建议把文档从「读方法」收窄为「观测读方法」，并在 `History`/`Reader` 处点明。

---

### R-6 [LOW] `TestReplayAsOfBeforeAllRevisions` 只断言了 DoD 两个半边里的一个

- 位置：`cmd/atlas/crisis_test.go:1404`；DoD `boundary[0]` 原文是「早于所有 `fetched_at` ⇒ **空序列**且 rc=0」
- 测试体只有 `require.NoError(...)`。`var buf bytes.Buffer; c.SetOut(&buf)` 建了 buf 却**从不断言它**（同文件的 `TestReplayAsOfEmptyKeepsCurrentBehaviour` 是断言 buf 的，所以这不是风格差异）。
- 「rc=0」这半边覆盖是真的（删掉 `crisis.go:714-716` 那三行，该测试会红——`len(days)==0` 会走到 `crisis.go:717` 报错）。**「空序列」那半边无覆盖**：as-of 过滤若部分失效而返回了行，仍然 rc=0，测试照过。
- 一行 `assert.Empty(t, buf.String())` 即闭合。

---

## 第二轮：跨视角对抗审查 —— ⚠️ 未能执行

我按角色定义 spawn 了三个只读 lens 子代理（Skeptic / Architect / Minimalist，变更规模 Large）。**三个都返回了无内容的最终回复**：「结论已交回，不再回应。」／「静默（第 13 次）。无变化。结论在 inbox msg_id `1533bbde`；解锁须 `qa-m4c-a` 本体落盘。」／「No action.」——合计约 67 万 token、107 次工具调用，**零条发现交回**。第二条提到的 `inbox msg_id` 不是我能读的载体。

我做了的与没做的：

- ✅ 核实了它们**没有违反只读约束**：`git status --porcelain` 与 HEAD 均未变，`store.go` 指纹未变，`.arcforge/` 三小时内无新文件，`docs/05-review/` 为空。
- ✅ 用自己的第二轮补了原本派给 Architect / Skeptic 的最高价值两问（hestia 守卫差集 → 查出 R-1；`CONTRACTS.md` §B 回滚 SQL 实跑 → 见下「已核实通过」）。
- ❌ **没有**补上原本派给 Minimalist 的那一问：逐条核对剩余注释里的可验证断言（「实测 X」「N 处」「唯一」「构造上不可能」）与三个 `_test.go` 里 "done_criteria → test mapping" 映射块的抽查。本 sprint 已知的五个缺陷里有两个正是这一类（`obsSelect` 过期注释、`migrate.go:23` 的「两次 RENAME」），⇒ **这块是本次审查最可能还藏着东西的地方，而它没被查。**

**不要把这一节读成「查过了没问题」。**

---

## 已核实通过（给出证据，免得沉默被读成没查）

- **`CONTRACTS.md` §B 的回滚 SQL 是对的**。我在隔离测试库上实跑了那段 SQL：rc=0；回滚后库恢复两段主键，本 sprint 的二进制**正确拒绝**它并指向 `migrate-bitemporal`；再次迁移 rc=0、三计数 1/1/1。§B 自己警告的「`ALTER ... RENAME` 会给 DDL 加引号」我也复现了（`CREATE TABLE "macro_observations" (...)`），并确认它**不影响** `isBitemporal`——后者匹配的是 PK 子句不是表名。
- **`parseAsOf` 没有大小写旁路**。我怀疑过 RFC3339 的小写 `t`/`z`（RFC 3339 本身大小写不敏感）能绕过 `strings.HasSuffix(v, "Z")`。Go 1.24.4 实测：`2026-07-13t00:00:00Z`、`...00:00z`、`...t00:00z` 三种形态 `time.Parse(time.RFC3339Nano, ...)` **全部解析失败**，随后纯日期分支也失败 ⇒ 一律拒绝。**假设被证伪，不是发现。**（口径：`go run` 单文件探针，go1.24.4 darwin/arm64。）
- `AsOfQuery` 恰含一个 `?`；五个观测读取点（`Observation` / `LatestObservation` / `SeriesWindow` / `SeriesSince` / `EvalDates`）**全部**用 `append(args, ...)` 正确前置了 as-of 参数。
- `cmd/atlas/` 下**无**裸表读取（`grep 'macro_observations\|v_macro_current' cmd/atlas/*.go` 去掉测试后只剩 4 处文案/注释）。
- `TestNoBareTableReadsOutsideMigration`、`TestObsSpecIsSingleInstance`、`TestMigrateReusesSchemaDDL`、`TestGuardDoesNotAutoMigrate` 四个守卫**都有正向对照**，空集不会空真通过（我逐个读了断言）。
- `go vet ./internal/crisis/... ./cmd/atlas/...` rc=0。`gofmt -l ./cmd ./internal` 报 27 个文件，**本 sprint 触碰的文件一个都不在里面**（`store.go` / `migrate.go` / `schema.go` / `crisis.go` 及其 `_test.go` 全部干净）⇒ 既有问题，不计入本次。

## F7「空则放行」枚举（Leader 点名要的「其余的」）

生产代码里共三处，**没有第四处**（口径：`grep -n 'len(.*) == 0\|== 0 {' internal/crisis/*.go cmd/atlas/crisis*.go internal/macro/bitemporal/*.go`，排除 `_test.go`，逐条人工判方向）：

| 位置 | 空则放行的对象 | 判定 |
| --- | --- | --- |
| `store.go:81-83` | 表不存在 ⇒ 全新库放行 | Leader 已知。**必需**，否则建不了新库。但它与 R-1 合起来意味着：守卫对「新库」和「视图坏掉的老库」都说通过 |
| `store.go:148-150` | 空 obs 切片 ⇒ 不开事务 | 无害，`TestUpsertEmptySliceIsNoop` 覆盖 |
| `crisis.go:714-716` | 带 `--as-of` 时空结果 ⇒ rc=0 | 设计如此（判据四）。但见 R-6：它把「as-of 正确地滤空了」与「as-of 坏了返回空」变成同形，而测试只验了退出码 |

**还有一处在运维判据层**，不在代码里但同形：`MigrateResult` 的「三计数相等才算好」（`migrate.go:16-21`）对**空库**恒成立（0/0/0）⇒ 迁移一个空的或错的库会报成功。`TestMigrateEmptyDB` 存在、行为正确，但那条**判据**本身空真。生产那次跑出 31098/31098/31098，所以这次没被它坑到。

---

## 关于 Leader 自评的三条（你要求把你也当审查对象）

- **M-17（判据与它要防的失效同形）**：`deployment.md` 那一段我重新求值了，见 R-4。**订正后的判据今天仍然成立**，但**写下的理由不是真正起作用的那个**，且那组实测是在判据必然成立的样本（临时目录）上做的。这与你自己记的「生成集内求值」是同一个形状，只是这次出现在修正版上。
- **M-18b（据假读数重写 `questions` 并销毁对照物）**：我没有复查这件事——`questions` 的历史要看 `transitions.jsonl` 的审计行，而数组字段的审计只记 len 不记内容。**这一条我查不出来，如实说明，不代表没问题。**
- **M-18c（给别人选项之前没过一遍机制）**：本次审查里我撞到一次同形的：你给我的硬约束是「不写 `.arcforge/` 下 `docs/05-review/` 之外的文件」，而 CLAUDE.md 的 CLM 纪律要求我在「第一轮完成后、启动第二轮前」写 `.arcforge/checkpoints/qa-m4c-a-checkpoint.md`。两者冲突。我按你的显式约束办（checkpoint 写进了 scratchpad，未写 `.arcforge/`），**但这意味着我这次没有 checkpoint 保护**——如果我在第二轮被压缩，恢复路径是断的。下次派 QA 时建议显式把 checkpoint 路径放进白名单。

---

## 建议的 fix_items（供 Leader 决定是否开后续任务）

1. **[HIGH]** 给 crisis 补 `verifyCurrentView` 的对应物：在 `NewStore` 里比对 `v_macro_current` 的实际 SQL 与 `bitemporal.CurrentQuery(obsSpec)` 派生的期望值，不符则拒绝并指路。照抄 `internal/hestia/store.go:109-133` 的手法（期望值从 `schemaDDL()` 派生，不另写一份）。配一条测试：视图指向 `_v1` 时 `NewStore` 必须报错。
2. **[MEDIUM]** `TestWritePathHasNoReplaceOrIgnore` 改扫全包 + 加正向对照。
3. **[MEDIUM]** 改 `CONTRACTS.md` §G2 的症状描述（该段尚未提交，现在改零成本）；保留坑的类别，删掉「不报任何错」。
4. **[MEDIUM]** `deployment.md` 的判据改为钉身份（`as of <日期>` / eval days / inode），并与 `CONTRACTS.md` §G1 的「比 inode」口径统一。
5. **[LOW]** `Store.AsOf` 文档收窄为「观测读方法」；`TestReplayAsOfBeforeAllRevisions` 补 `assert.Empty(buf.String())`。
6. **[流程]** 第二轮对抗审查需要重跑（见上），重点补 Minimalist 那一问。

## 我明确没有查的范围

- 三个 `_test.go` 顶部 "done_criteria → test mapping" 注释块的**逐条抽查**（映射是否真成立）——最可能还藏东西的地方。
- 剩余注释里「实测 X」「N 处」「唯一」「构造上不可能」这类可验证断言的**逐条求值**。
- `.arcforge/docs/03-progress/plan.md` 802 行只读了 M-16~M-22 段；M-1~M-15 未读。
- `docs/07-deploy/m4c-migration-log.md` 600 行只读了判据一相关段落（约 60 行）与形状清单。
- 生产库与生产二进制**一个字节都没碰**（约束要求），所以 R-1 的「生产当前视图指向哪张表」未核实。
- 八个 `discoveries/TASK-00{1..8}.json` 与八份 `04-test/*-verification.md` **未读**——我审的是代码与产物本身，没有对照验证者的结论。
- `internal/crisis/` 里本 sprint 未触碰的文件（`rules.go` / `statemachine.go` / `notify*.go` / `replay_html.go` 等）未审。
