# TASK-003 验证报告 — NewStore 漂移守卫

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（8/8 done_criteria 通过）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `a950f754fa0d45794adc0696cf01e9fd11a39fc9`（master）
- **verify_baseline 核对**：`head` 记录值 == 当前 `git rev-parse HEAD`；`discovery_sha256` 记录值 == 当前 `shasum -a 256` == `57970feaa5c72ffadfd2e4f21737c1c27c93078d1e274c9e90522d920abbb641`。**零漂移**，未用 `--ack-drift`。
- **真实库未触碰**：`data/crisis.db` mtime `Jul 14 19:51`、size `3883008`，与验证前一致（迁移会重写它，mtime 与 size 必变）。本报告全部实测走 `t.TempDir()` 级临时库与隔离 worktree。

> 🔴 **一条要订正的**：DoD `functional[1]` 里「纯 DDL 文本断言不够」这句**理由不成立**（要求本身没问题）。见 §4.2。

---

## 1. 亲跑结果

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `GOTOOLCHAIN=local go build ./...` | 0 | 通过 |
| `GOTOOLCHAIN=local go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | 两包均 `ok` |
| 变异对照组 / 收尾对照 `-v` 两包 | 0 | **PASS 538 / FAIL 0**（两次同值 ⇒ 还原完整） |

## 2. 先看夹具（我先验预测的陷阱，dev 没踩）

我在派验前向 Leader 提过一条可先验的预测：TASK-003 的 dev 若从 `migrate_test.go` 复制夹具，复制到的会是**不带视图**的 `legacySchema`——而本任务 DoD 恰好要求夹具必须带视图。**最省事的写法正好是错的那个，且写出来测试全绿。**

实际交付（`store_test.go` 的 `legacyDBWithView`）：

```go
path := legacyDB(t, rows...)                                   // 复用 TASK-002 的老形状夹具
db.Exec(`CREATE VIEW v_macro_current AS ` + bitemporal.CurrentQuery(obsSpec))
```

⇒ 在 TASK-002 夹具之上**加建视图**，且视图 SQL 由 `bitemporal.CurrentQuery(obsSpec)` **生成而非手抄**（手抄就是视图定义的第二个副本，同 AD-3）。**预测的陷阱没有发生**，处理得比要求更好一层。

## 3. done_criteria 覆盖矩阵（8/8）

| # | 完成标准（摘要） | verify_by | 对应测试 | 针对性变异 | 判定 |
| --- | --- | --- | --- | --- | --- |
| functional[0] | 老形状 ⇒ `NewStore` 报错，文案含 `migrate-bitemporal` + `macro_observations` + 库路径 | test | `TestNewStoreRejectsLegacyShape` | **P6** 守卫放行老库 → 红；**P9** 文案去掉 `migrate-bitemporal` → 红 | **PASS** |
| functional[1] 前半 | 迁移后的库正常打开；空库正常 | test | `TestNewStoreAcceptsMigratedDB` / `TestNewStoreAcceptsFreshDB` | **P8** → 红；**P10** 删「表不存在=全新库」早返回 → 红 | **PASS** |
| functional[1] 后半 | 两条迁移后不变量（索引 `tbl_name`、视图指向新表），**夹具带视图 ∧ 行为断言缺一不可** | test | `TestMigratedDBKeepsIndexAndViewOnNewTable` | **P3/P1/P2a** 三元组（见 §4.1）；**P11** 删 `DROP INDEX` → 红 | **PASS** |
| functional[2] | 守卫不自动迁移，`NewStore` 路径无改形语句 | **review** | `TestGuardDoesNotAutoMigrate`（源码扫描 + 全量 `sqlite_master` 快照） | **P4** 守卫挪到 `schemaDDL()` 之后 → 红 | **PASS**（§5） |
| boundary[0] 前半 | 探形状看主键含 `fetched_at`、不看 `_v1` | test | `TestNewStoreShapeProbeIgnoresLegacyTable` | **P7** 改成看 `_v1` 是否存在 → 红 | **PASS** |
| boundary[0] 后半 | **AD-5**：老库上 `NewStore` 报错而 `MigrateBitemporal` 仍成功 | test | `TestMigrateWorksOnDBRejectedByNewStore` | **P8** `MigrateBitemporal` 改经 `NewStore` → 红 | **PASS**（§6） |
| boundary[1] | `NewStore` 每条 error 返回路径前已 `db.Close()` | **review** | — | — | **PASS**（§7） |
| error_handling[0] | 既有错误路径不变（既有测试零改动且全绿） | test | `TestNewStoreBadPath`（零改动） | — | **PASS**（§8） |
| non_functional[0] | `go build ./...` + 两包全绿 | test | 见 §1 | — | **PASS** |
| non_functional[1] | 注释写明「为什么不自动迁移」并指向 hestia 先例；手跑实际文案 | **review** | — | — | **PASS**（§9） |

## 4. 「缺一不可」的独立复现 —— 本任务的核心主张

隔离 worktree `../wt-verify-TASK-003 @ a950f754fa0d45794adc0696cf01e9fd11a39fc9`，11 个变异，**全部通过有效性闸**（`go vet` 全 0，无「编译失败伪装成 KILLED」）。

### 4.1 两个维度各缺一次 —— DoD 的主张成立

| 变异 | 夹具 | 断言 | 结果 |
| --- | --- | --- | --- |
| **P3**（基准，= dev 的 V1） | 带视图 | 行为断言 | **KILLED** |
| **P1** | **不带视图**（换回 `legacyDB`） | 行为断言 | **SURVIVED** |
| **P2a** | 带视图 | **删掉行为断言**（只剩索引断言①） | **SURVIVED** |

三者都叠加同一个被测缺陷（删掉 `migrate.go` 的 `DROP VIEW`）。⇒ **夹具与行为断言两个维度缺任一都失效，齐备才杀得掉**——DoD `functional[1]` 的主张经我独立复现**成立**，不是复述 dev 的结论。

### 4.2 🔴 但 DoD 给的**理由**不成立（要求没问题，理由要订正）

DoD 原文：「**纯 DDL 文本断言不够**，因为迁移后 `_v1` 与新表内容相同 ⇒ 视图即使指向 `_v1`，行数与双向 `EXCEPT` 也全对（那是 `_v1` 减 `_v1`）」。

我加跑了一个对照：

| **P2b** | 夹具带视图 | 行为断言 **换成视图 DDL 文本断言** `assert.NotContains(viewDDL, "macro_observations_v1")` | **KILLED** |
| --- | --- | --- | --- |

⇒ **视图 DDL 文本断言对这个缺陷是有效的**。DoD 那句话把两种不同的断言混为一谈：

- 它给的论证（行数、双向 `EXCEPT` 全对）针对的是 **TASK-002 里那两条 `EXCEPT` 断言**——那些确实不够，因为它们比的是 `_v1` 减 `_v1`；
- 但「**视图 DDL 文本**断言」是另一回事：`ALTER TABLE RENAME` 会把视图 SQL 改写成引用 `macro_observations_v1`，文本比对当场就能看见（我在 TASK-002 的探针里同时用过 DDL 文本与行为两种断言，**两种都红**）。

**影响评估**：不影响本任务判定——DoD 要求的是行为断言，交付也用了行为断言，而行为断言**严格更强**（它还能挡住「视图名字对、语义被改坏」这类文本比对看不见的情况）。要订正的只是理由。

**为什么值得单列**：结论正确会让判断永不被复查，而**理由是别人推广时唯一的入口**。后人读这条 DoD 会得出「视图 DDL 文本断言对这类缺陷无效」——那是假的。建议在 final-report 里把那句改成「**行数与双向 `EXCEPT` 断言不够**（`_v1` 减 `_v1`）；DDL 文本断言可行但行为断言更强」。

### 4.3 其余针对性变异（全部 KILLED）

| 变异 | 被杀死的测试 |
| --- | --- |
| **P6** 守卫对老库直接放行（= dev 的 V5） | `TestNewStoreRejectsLegacyShape` + `TestMigrateWorksOnDBRejectedByNewStore` + `TestGuardDoesNotAutoMigrate`（3 条） |
| **P7** 探形状改成看 `macro_observations_v1` 是否存在（= V3） | `TestNewStoreShapeProbeIgnoresLegacyTable`（+ cmd 侧 4 条） |
| **P9** 错误文案去掉 `migrate-bitemporal` | `TestNewStoreRejectsLegacyShape` |
| **P10** 删掉「表不存在 = 全新库」早返回 | `TestNewStoreAcceptsFreshDB`（连带 60+ 条——所有建新库的测试） |
| **P11** 删 `migrate.go` 的 `DROP INDEX`（= V2） | `TestMigrateRebuildsIndex` + `TestMigratedDBKeepsIndexAndViewOnNewTable` |

**主工作区指纹**：三个相关文件 sha256 收尾与开工逐字节一致，`git status --porcelain -- internal/ cmd/` 为空。

## 5. V4 抽验 —— dev 的自曝属实

dev 自报「守卫挪到 `schemaDDL()` 之后」最初**存活**，原因是断言只比对 `macro_observations` 的表 DDL，而挪到之后时表 DDL 一字未变、变的是多出一个指向老表的视图；升级为全量 `sqlite_master` 快照后才杀死。

我两态都跑了：

| **P4** 守卫挪到 `schemaDDL()` 之后（**交付版**全量快照断言） | **KILLED** |
| --- | --- |
| **P5** 同一缺陷 + 断言降级为**只比表 DDL** | **SURVIVED** |

⇒ **精确复现**。自曝属实，升级为全量快照确是必要修复而非锦上添花。

`schemaSnapshot` 的写法也对：取 `type/name/tbl_name/sql` 全量逐行，而非逐个对象断言——正如其注释所说，逐个断言只能证明「我想到的那几个没变」，而漏掉的那个恰好就是缺陷所在。

## 6. AD-5：TASK-002 那个缺口确实被接管了

TASK-002 验证时我记过：**N4（把 `MigrateBitemporal` 改成经 `NewStore`）零测试变红**，因为守卫当时还不存在，走 `NewStore` 也全绿——「绿不构成证据」的教科书例子。

本任务 **P8** 用同一个变异重跑：`TestMigrateWorksOnDBRejectedByNewStore` **变红**（连带 11 条）。⇒ **那个缺口的接管者已就位并有牙。** 这是跨任务缺口移交被实测闭合的一例，建议在 final-report 里作为正面案例记一笔。

## 7. boundary[1]（review）：error 路径逐条核

`NewStore` 的全部返回路径（`internal/crisis/store.go:20-46`）：

| # | 路径 | 连接是否已建立 | `db.Close()` |
| --- | --- | --- | --- |
| 1 | `os.MkdirAll` 失败 | 否（还没 `sql.Open`） | 不需要 ✓ |
| 2 | `sql.Open` 失败 | 否（`db` 不可用） | 不需要 ✓（此时 Close 反而会 panic） |
| 3 | `db.Ping()` 失败 | 是 | **有** ✓ |
| 4 | `verifyBitemporalShape` 失败（**本任务新增**） | 是 | **有** ✓ |
| 5 | `db.Exec(schemaDDL())` 失败 | 是 | **有** ✓ |
| 6 | 成功 | 是 | 交给 `Store.Close` ✓ |

⇒ 「已建立连接之后的路径都要有」**逐条满足**。按 DoD 的明确提示，**未**使用「能否删除库文件」做判据（macOS 上 unlink 不受 open fd 阻塞，该判据恒真、零信息量）。

## 8. RED 的「3 红 5 绿」—— dev 的区分理由成立，且与 TASK-002 性质不同

dev 主张：那 5 条绿**不是断言太弱**，而是它们断言的就是「放行」，守卫不存在时本来就放行 ⇒ **性质上不可能在 RED 变红**。

**我判定该区分成立**，理由如下，并请后人**不要拿 TASK-002 的尺来量本任务**：

| | TASK-002 的三条（`RebuildsIndex` / `EmptyDB` / `ShapeProbeIgnoresLegacyTable`） | TASK-003 的这 5 条 |
| --- | --- | --- |
| 空桩/无守卫时为什么绿 | **空桩的返回值恰好满足断言**（三个 `0`、`AlreadyMigrated=false`、老库索引本来就叫那个名且挂在那张表上） | **断言的性质就是「无事发生」**，守卫缺席时「放行」为真 |
| 性质 | **断言不够强** | 断言强度与此无关 |
| 补强后 | **会变红**（dev 当时正是这么做的） | **补强也不可能在 RED 变红**，除非改成断言别的东西 |

**但「性质决定」不等于免于举证**——证明它们非空洞的正确手段是**变异**而非 RED。我的变异逐条做到了：

- `TestNewStoreAcceptsMigratedDB` ← **P8**
- `TestNewStoreAcceptsFreshDB` ← **P10**
- `TestNewStoreShapeProbeIgnoresLegacyTable` ← **P7**
- `TestMigratedDBKeepsIndexAndViewOnNewTable` ← **P3 / P11 / P2b**
- `TestNewStoreBadPath` —— DoD `error_handling[0]` 要求**零改动**的既有测试，其被测性质是「既有路径不变」，由 `git show --numstat`（该文件新增 196 行、删除 0 行，既有内容逐字未动）+ 全绿共同证明，不需变异。

## 9. non_functional[1]（review）：手跑实证我独立复现了

**不采信 discovery 里贴的文案**，用真实 CLI 重跑一遍（`configs/crisis-monitor.yaml` 为模板、只改 `storage.path` 指向临时库）：

**① 错误文案**——与 dev 自报**逐字一致**：

```
Error: crisis: macro_observations in <path> still has the legacy two-part primary key
(ts, indicator); this database predates the bitemporal migration and CREATE TABLE IF NOT
EXISTS does not change it. Automatic migration is an explicit non-goal — run:
atlas crisis migrate-bitemporal --db <path>
```

三要素齐备（表名 / 库路径 / 该跑的命令），人类可读、能指路。

**② 「被拒后无视图残留」**——这条才是守卫**位置**的证据，我用两种老库各跑一次：

| 临时库 | 跑前 `sqlite_master` | 跑后 |
| --- | --- | --- |
| **不带视图**的老库（= 两个真实库的形态） | table + 2 index | **仍无 `v_macro_current`** ✓ 复现 dev 那条 |
| **带视图**的老库 | table + 2 index + view | **逐项不变** ✓ |

⇒ 守卫确在 `schemaDDL()` **之前**生效，不只是注释里那么写。

**③ 爆炸半径**——Leader 的口径经我核实属实：

```
crisis.NewStore 非测试调用点：cmd/atlas/crisis.go:95（唯一，在 openCrisisStore 内）
openCrisisStore 调用点：crisis.go 5 处 + crisis_report.go 1 处
serve.go：无 crisis 引用 ⇒ serve 不触达
```

⇒ 守卫合入后，本地未迁移的 crisis.db 会让 `atlas crisis` 全族失败（backfill/eval/status/replay/report），而 `serve` 不受影响；`migrate-bitemporal` 本身直接开库不经 `NewStore`（AD-5），所以**出路没有被自己堵死**。

**④ 注释要求**：`verifyBitemporalShape` 的文档注释写明了「为什么不自动迁移」（启动时静默改结构会把一个本该被人盯着的动作变成副作用；迁移要搬全部历史、重建索引与视图、失败时需要人判断怎么回滚），并指向 hestia 先例 `internal/hestia/store.go` 的 `verifyObservationsSchema` / `verifyCurrentView`。✓

## 10. 现场状态（供 TASK-008 与人类参考，非本任务判定项）

守卫已合入 master ⇒ 两个真实库（仓库 `data/crisis.db`、runtime）均为**老形状**，现在任何 `atlas crisis` 子命令都会被守卫拒绝。这是**预期行为**，出路是 `atlas crisis migrate-bitemporal --db <path>`。

- `data/crisis.db` 本体 mtime `Jul 14 19:51`、size `3883008`，**未被迁移也未被修改**（迁移会重写它，两者必变）。
- 其 `-shm` mtime 为 `Sep 19 08:13`（守卫合入之后）——与「被守卫拒绝的一次打开」一致：`db.Ping()` 会建立连接并触碰 `-shm`，随后守卫拒绝、库本体一字不动。这正是我在 §9② 用临时库复现出来的形态，可作为佐证而非异常。
- **我全程未打开这两个库**，所有实测走临时库。迁移它们是不可逆写操作，归 TASK-008 与人类。

## 11. 结论

8 条 done_criteria 全部通过，其中 5 条 `verify_by: test` 逐条有**针对性**变异证据，3 条 `verify_by: review` 逐条读码/实跑核实。11 个变异全部通过有效性闸。Leader 指定的三项抽验结论：**「缺一」两形态独立复现成立**（§4.1）、**V4 自曝属实且两态精确复现**（§5）、**RED 3 红 5 绿的区分成立且与 TASK-002 性质不同**（§8）。TASK-002 遗留的 AD-5 缺口经 P8 实测**已被接管**（§6）。

**裁决：VERIFIED。**

**移交 Leader 一件事**：§4.2 —— DoD `functional[1]` 里「纯 DDL 文本断言不够」这句理由不成立，建议在 final-report 订正（要求不必改，改理由）。

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 a950f754fa0d45794adc0696cf01e9fd11a39fc9
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas

# 变异（隔离副本，主工作区一个字节不碰）
git worktree add --detach ../wt-verify-TASK-003 a950f754fa0d45794adc0696cf01e9fd11a39fc9
python3 <scratchpad>/test-m4c-a-T003-mutate.py     # P1-P11
git worktree remove --force ../wt-verify-TASK-003

# 手跑（临时库，勿指向 data/crisis.db）
# 以 configs/crisis-monitor.yaml 为模板、改 storage.path 指向临时老形状库，然后：
GOTOOLCHAIN=local go run ./cmd/atlas crisis status --crisis-config <临时配置>
```
