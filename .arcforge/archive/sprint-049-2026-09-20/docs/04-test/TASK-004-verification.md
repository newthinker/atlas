# TASK-004 验证报告 — 读全部改走 v_macro_current（含 EvalDates）

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（8/8 done_criteria 通过）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `46c401cf15df8c9922c5023c823d8182d2b3707e`（master）
- **verify_baseline 核对**：`head` 与 `discovery_sha256`（`eb05ed690964b7e68eb1444f714d276fbde21224c355cba986d4928bf13afd2f`）均与记录值一致。**零漂移**。

> 🔴 **读本报告的人必须先知道一件事（§1）**：本任务有一个补提交 `0e261cf` 在 `dev_done` **之后**才提交，**没有经过任何机械门禁**。下面所有测试与覆盖率数字**是我自己跑出来的，不是门禁盖的章**。

---

## 1. 未经门禁的补提交 —— 本报告的数从哪来

提交链：

```
983c3cf  test(TASK-004): RED —— 读路径仍走裸表，五个读取点各自失败
5664ef7  feat(TASK-004): 五个读取点全部改走 v_macro_current
31b575d  merge(TASK-004)
0e261cf  test(TASK-004): 夹具补最新日修订与插入顺序   ← 在 dev_done 之后提交，未过门禁（+27/-7）
46c401c  merge(TASK-004): 夹具加强
```

`dev_done` 门禁只在 `transition dev_done` 那一刻执行过一次（当时 93.8%），`0e261cf` 的 27 行**不在它的量程内**。Leader 取此路径是为省一次 `rework_count` + 全套复验，并明确要求我自行复跑。

**我全部亲跑，不采信任何人报的数**：

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `GOTOOLCHAIN=local go build ./...` | 0 | 通过 |
| `GOTOOLCHAIN=local go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | 两包均 `ok` |
| `go test -cover -coverpkg=./internal/crisis ./internal/crisis` | 0 | **93.8%**（`dev_minimum` 80） |
| `go vet ./internal/crisis ./cmd/atlas` | 0 | 通过 |
| `gofmt -l` 本任务两文件 | — | 空 |
| 变异对照组 / 收尾对照 `-v` 两包 | 0 | **543 PASS / 0 FAIL**（两次同值） |

⇒ 结论不变，但**证据链的源头是验证者而非门禁**。记此备查：若后人以为这份绿是门禁出的，那是误读。

## 2. 改动范围与「不该改的」

`git diff --numstat a950f75..46c401c`：

```
18	2	internal/crisis/store.go
207	0	internal/crisis/store_test.go
```

- 两个文件都在 `writes` 声明内，**无越界**。
- `store_test.go` **删除 0 行** ⇒ `boundary[1]`/`error_handling` 要求的「既有测试零改动」**结构性成立**，不是「跑绿了就算」。
- `store.go` 只有 **2 行删除**：`obsSelect` 常量的 `FROM`、`EvalDates` 内联查询的 `FROM`。

**与我在交付前做的独立普查逐条对上。** 我在基线 `a950f75` 上数出 **5 个数据读取点**（`Observation` :127 / `LatestObservation` :132 / `SeriesWindow` :150 / `SeriesSince` :166 走 `obsSelect`，加 `EvalDates` :176 的**内联**查询）。交付恰好改了这 5 个的 FROM 来源，一个不多一个不少。

**我钉的「不该改的」全部被遵守**（`git diff … -- internal/crisis/migrate.go` 输出 0 行）：

| 不该改的 | 现状 |
| --- | --- |
| `migrate.go:108` / `:136` 的 `SELECT COUNT(*) FROM macro_observations` | **仍在基表** ✓ —— 它数的是**全部修订**，走视图就只数当前行，迁移三计数当场失去意义（这是 TASK-008 判据一的地基） |
| `migrate.go:140` 的 `SELECT COUNT(*) FROM v_macro_current` | 本就该查视图（ViewRows），未动 ✓ |
| `store.go:109` 的 `INSERT OR REPLACE`（写） | 未动 ✓ |
| 两处 `sqlite_master` 探形状（元数据不是数据） | 未动 ✓ |

## 3. done_criteria 覆盖矩阵（8/8）

| # | 完成标准（摘要） | verify_by | 对应测试 | 证据 | 判定 |
| --- | --- | --- | --- | --- | --- |
| functional[0] | 四个读方法只看到最新修订，逐值期望 | test | `TestReadsSeeOnlyLatestRevision` | RED 复现 5 处变红（§4）；**Q5** obsSelect 改回裸表 → 红 | **PASS** |
| functional[1] | `EvalDates` 不返回重复日期；RED 在指定全 sha 可复现 | test | `TestEvalDatesDeduplicatesRevisions` | RED 复现 :444（§4）；**Q4** 见 §6 —— **须与 functional[2] 合看** | **PASS** |
| functional[2] | 包内裸表守卫，词边界正则，豁免逐行且条数恰为 N | test | `TestNoBareTableReadsOutsideMigration` | **Q1/Q2/Q3/Q6** 四向验证（§5） | **PASS** |
| boundary[0] | 四个 `obsSelect` 调用点的 WHERE/ORDER/LIMIT 与参数逐字未变 | **review** | — | `git diff` 中这四处**一行都没出现**（§7） | **PASS** |
| boundary[1] | 单版本数据下与改动前逐值相同；既有测试零改动全绿 | test | `TestSingleRevisionReadsUnchanged` + numstat | **Q7b** 视图漏行 → 红（§5）；`store_test.go` 删除 0 行 | **PASS** |
| error_handling[0] | 视图缺失时响亮失败且文案含 `v_macro_current` | test | `TestReadsFailLoudlyWhenViewMissing` | RED 复现 :469；**Q4/Q5** 均杀死 | **PASS** |
| non_functional[0] | `go build ./...` + 两包全绿 | test | 见 §1 | — | **PASS** |
| non_functional[1] | `obsSelect` 与 `EvalDates` 各留注释说明「为什么换 FROM 而非每处加 WHERE」 | **review** | — | 两处注释俱在（§8） | **PASS** |

## 4. RED 独立重跑 —— 两种，结论都成立

**先核锚**：`git merge-base --is-ancestor 983c3cf HEAD` ⇒ **可达**。两次 merge 都用 `--no-ff` 保留了历史，所以「在 RED sha 上重跑」这条 DoD 要求是可执行的。

### 4.1 `983c3cf` 原样（该 sha 自带的测试文件）

4 红 1 绿。绿的是 `TestSingleRevisionReadsUnchanged`——它断言的是「**无变化**」，守卫/视图缺席时「无变化」为真，**性质上就该绿**（同 TASK-003 那 5 条的性质，不是断言太弱）。

### 4.2 加强夹具 × 旧实现（`46c401c` 的 `store_test.go` 打在 `983c3cf` 的 `store.go` 上）

**5 处变红，行号 408/416/424/426/430，与 dev 自报逐点一致**：

| 行 | 断言 | expected | **actual** |
| --- | --- | --- | --- |
| 408 | `Observation` 取最新修订 | 11 | **10** |
| **416** | `LatestObservation` 的 **value** | 21 | **20** |
| 424 | `SeriesWindow` 的 dates | `[01-02, 01-03]` | **`[01-03, 01-03]`** |
| 426 | `SeriesWindow` 的 values | `[11, 21]` | **`[21, 20]`** |
| 430 | `SeriesSince` 行数 | 2 | **4** |

⇒ **我在交付前提出的问题已闭合。** 我当时实测 `LatestObservation` 与 `SeriesWindow` 在 DoD 原夹具上**改动前就成立**、不可能变红；现在它们都确定性变红了。

**其中 `:416` 是 dev 补出的「那半步」，不是我提的**，如实记明：我只说了「夹具再加一行最新日修订」，而 dev 进一步发现**还须让新修订先落库**——裸表的 `ORDER BY ts DESC LIMIT 1` 在同一天多行里取的是**扫描顺序**的某一行，旧修订先插会碰巧得到 21（对），新修订先插才得 20（错）。顺序写反，这条断言会悄悄失去牙而看不出来。`:416` 的 `actual=20` 就是这半步的直接证据。

## 5. 裸表守卫 —— 四个方向都验了

Leader 要我确认「跳过行注释」这个被批准的边界**没有把守卫掏空**。只验一头证明不了这件事，故两个方向都打：

| 变异 | 构造 | 守卫 | 含义 |
| --- | --- | --- | --- |
| **Q1** | 包内非测试文件里塞一条**真的**裸表查询 | **红** | 边界没掏空守卫 ✓ |
| **Q2** | 同一条 SQL 放进**行注释**（含「视图本身就是 SELECT * FROM macro_observations」这种解释） | **绿** | 边界切在正确位置 ✓ |
| **Q3** | 写成**行尾注释**（`var zzX = 1 // SELECT ts FROM macro_observations`） | **红** | 已知边界：`strings.HasPrefix(trimmed,"//")` 只跳整行注释 ⇒ 行尾注释被判命中。这是**假阳方向**，属安全侧 |
| **Q6** | 删掉 `migrate.go` 的一条 `COUNT` 查询（豁免清单少一条） | **红** | 「条数恰为 N」两个方向都钉住：多了是新增裸读，少了是清单没跟上 ✓ |

另有 **Q7b**（为 `boundary[1]` 补的）：把视图改成漏行（`schema.go` 加 `AND o.value > 100`）⇒ `TestSingleRevisionReadsUnchanged` **红**。

> ⚠️ 方法学如实记：Q7 首次我用的是 `AND o.value > 12`，而该测试夹具的值是 15/16，**变异没打到它的夹具上**，于是测试存活。**那是我的变异没选好，不是断言没牙**——换成 `> 100` 后立刻变红。若我当时就此报「boundary[1] 无守卫」，那会是一条由**我的**取样误差制造的假缺陷。

## 6. DISTINCT 规避路径 —— 我与 dev 各自独立提出，实测成立

我在 TASK-004 派验前向 Leader 提过：`EvalDates` 去重有两条路，**改走视图**或**只加 `DISTINCT`**；单看 `functional[1]` 后者就能过。dev 独立造了同一个变异。我实测：

**Q4**：`EvalDates` 改回 `SELECT DISTINCT ts FROM macro_observations …`

| 测试 | 结果 |
| --- | --- |
| `TestEvalDatesDeduplicatesRevisions`（functional[1]） | **绿** —— 确实被 DISTINCT 蒙混过去 |
| `TestNoBareTableReadsOutsideMigration`（functional[2]） | **红** |
| `TestReadsFailLoudlyWhenViewMissing`（error_handling[0]） | **红**（不读视图了，视图缺失自然不报错） |

⇒ **`functional[1]` 必须与 `functional[2]` 合起来判**，单看去重断言不足以逼出「改走视图」。而且抓手比预期多一个：`error_handling[0]` 也会红。**两条独立路线从相反方向得到同一结论**，不是一方抄了另一方——记此，免得后人当成一个来源的两次复述。

## 7. boundary[0]（review）：四个调用点逐字未变

`git diff a950f75..46c401c -- internal/crisis/store.go` 里，`Observation` / `LatestObservation` / `SeriesWindow` / `SeriesSince` 四处调用点**一行都没有出现**——变的只有 `obsSelect` 常量定义（+ 其上方新增注释）与 `EvalDates` 的内联查询。C5 要求的「WHERE/ORDER/LIMIT 与参数逐字未变」**结构性成立**。

这正是「换 FROM 而非每处加 WHERE」这个设计的可验证收益：改动面收敛到一处常量，四个调用点根本不必碰。

## 8. non_functional[1]（review）：两处注释

- `obsSelect` 上方：写明换 FROM 而非每点加 WHERE 的理由（「后者是同一条规则的 N 份副本，加第 N+1 个读点时必漏 —— `EvalDates` 就是这么漏掉的」），并说明为何**保持显式列名而非 `SELECT *`**（视图是 `SELECT *`，用 `*` 会把将来新增的列带出来，而 `scanObservation` 按固定列序扫描）。
- `EvalDates` 上方：写明它是第五个读取点、**不经 `obsSelect`**，以及为何不加 `DISTINCT`（「就地补一条规则，下一个读点还得再补一次」）。

两处都指向了「计划原文把读取点数错成『只改 obsSelect 即可』」这件事，对后人有诊断价值。✓

## 9. 一条观察：词边界当前不 load-bearing（不影响判定）

DoD `functional[2]` 要求用**带词边界**的正则，理由是「朴素子串会被 `FROM macro_observations_v1` 假阳命中」。我核了一下这条在当前树上买到了多少：

```
非测试源码：词边界命中 2 条 / 朴素子串命中 2 条 -> 相同
`FROM macro_observations_v1` 字面量：只出现在 migrate_test.go（3 处），而守卫跳过测试文件
```

⇒ **词边界在当前树上与朴素子串同值**。原因是 `migrate.go` 写的是 `` `... FROM ` + legacyTableName ``（拼接）而非内联字面量。所以词边界是**前瞻性**要求，当前不 load-bearing——但它是对的，且一旦有人把那条拼接改成字面量就立刻变成 load-bearing。

dev 在注释里写的「实测：同一行文本，带词边界 0 次、朴素子串 1 次」是关于**正则性质**的陈述，**准确**，并未声称本包现有这样一行。不构成问题，记此以免后人误以为这个差别已经在本包生效。

## 10. 一条残留局限（同 TASK-001，非本任务缺陷）

裸表守卫是**源码字面量扫描**，对动态拼表名（`"SELECT … FROM " + tableName`）无效——与 TASK-001 `functional[3]` 的 import 别名可绕过同族。DoD 明文要求的就是正则扫描，故**符合 DoD**，不是缺陷；但下游任务须知这道闸的射程。

## 11. 结论

8 条 done_criteria 全部通过：6 条 `verify_by: test` 逐条有针对性变异或 RED 证据，2 条 `verify_by: review` 逐条读 diff / 读码核实。RED 在 `983c3cf` 上**两种方式**独立重跑均复现，加强夹具下 5 处变红与 dev 自报逐点一致。守卫的注释豁免边界**两个方向都验过**，没有掏空守卫。DISTINCT 规避路径实测成立，判定按「functional[1] + functional[2] 合看」执行。我交付前提出的两条无牙断言已闭合，且 dev 补出了我漏掉的「插入顺序」那半步。

**裁决：VERIFIED。**

**移交 Leader 两件事**：
1. `0e261cf` 未过门禁（§1）——本报告的绿是**验证者跑的**，建议在 final-report 记明这条路径的代价：省了一次 `rework_count`，换来的是那 27 行只有人工证据。
2. 词边界当前不 load-bearing（§9）与守卫对动态表名无效（§10）——都不是缺陷，但下游任务（尤其 TASK-005 要改 `UpsertObservations`）须知这道闸的射程。

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 46c401cf15df8c9922c5023c823d8182d2b3707e
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas
GOTOOLCHAIN=local go test -count=1 -cover -coverpkg=./internal/crisis ./internal/crisis

# RED 复现（两种）
git worktree add --detach ../wt-verify-TASK-004 46c401cf15df8c9922c5023c823d8182d2b3707e
cd ../wt-verify-TASK-004
git checkout -q 983c3cf62e628e67607ef4ee02f94214382539d5          # ① 原样 RED
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis
git checkout 46c401cf15df8c9922c5023c823d8182d2b3707e -- internal/crisis/store_test.go   # ② 加强夹具 × 旧实现
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis

# 变异
python3 <scratchpad>/test-m4c-a-T004-mutate.py    # Q1-Q6
git worktree remove --force ../wt-verify-TASK-004
```
