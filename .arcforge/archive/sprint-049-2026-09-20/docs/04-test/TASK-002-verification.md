# TASK-002 验证报告 — 迁移命令（MigrateBitemporal + atlas crisis migrate-bitemporal）

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（8/8 done_criteria 通过，逐条有针对性变异证据）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `d347c9adece0b657f972a63b21c08929905ae59a`（master）
- **verify_baseline 核对**：`head` 记录值 == 当前 `git rev-parse HEAD` == `d347c9adece0b657f972a63b21c08929905ae59a`；`discovery_sha256` 记录值 == 当前 `shasum -a 256 .arcforge/discoveries/TASK-002.json` == `bebdd15a9a90b9953f054176fc4404c079741f109ec6bb049541c5ed17baac18`。**零漂移**，未用 `--ack-drift`。
- **修订**：本报告在裁决落盘后经一次修订（2026-09-18T23:34 之后）——见 §9「报告勘误」，含**我自己写错的一条**。

> 🔴 **读本报告的人请先读 §5**：DoD 全过，但有一处**实现正确而无断言守护**的缺口（`DROP VIEW`）。判 VERIFIED 不等于它不存在。

---

## 1. 裁决依据的口径（为什么不拿 DoD 字面量它）

Leader 于 2026-09-19 在 `.arcforge/docs/03-progress/plan.md`（第 41-60 行）落了两条裁决，派验消息中重申：

- **D1**：实现走 **rename-first、没有 `_new` 表，已批准**。DoD `functional[0]` 里的 `_new` 是**描述实现机制**，不是验收要求；真正的要求是「零副本、新表三段主键 + NOT NULL、索引挂新表、旧表留 `_v1`、事务原子」。plan.md 原文明写「**验证者不得拿字面的 `_new` 来量本任务**」。
- **D2**：「`_v1` 已占名 + 主表仍老形状 ⇒ **响亮失败**」已批准（dev 原断言写的是「迁移成功」，实测撞名失败后它判定是断言错了——让它成功只能 `DROP TABLE _v1`，而那可能是唯一一份迁移前数据）。

**为免后人只读 DoD 以为有缺口**：`boundary[0]` 中「无 `_new` 残留」在 rename-first 下**恒真**（压根不存在 `_new`），测试里那条 `assert.False(objectExists(db, "macro_observations_new"))` 因此是条恒真断言——不是空洞断言，而是裁决后失去被测对象的遗留断言，留着无害。

## 2. 亲跑结果（不复用 dev 的输出）

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `GOTOOLCHAIN=local go build ./...` | 0 | 通过 |
| `GOTOOLCHAIN=local go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | `ok internal/crisis 0.679s` / `ok cmd/atlas 1.716s` |
| `go test -count=1 -v ./internal/crisis ./cmd/atlas`（变异对照组） | 0 | **PASS 531 / FAIL 0** |

判定按**退出码 + `--- FAIL:` 计数**，不依赖管道后输出。

## 3. 越界申报与范围声明

`git show --numstat e93c950`：`crisis_migrate.go 66/0`、`crisis_migrate_test.go 121/0`、`migrate.go 148/0`、`migrate_test.go 307/0` —— 四个文件全部新建，全部在 `writes` 声明内，**无声明外文件**，既有测试零改动。

**`packages` 已由本验证者补齐**（2026-09-18T23:34:02Z，经 `update --json-field`，审计行记 `added: ["./cmd/atlas"]`）：原为 `["./internal/crisis"]`，与 `writes` 含两个 `./cmd/atlas` 文件不一致，即 Leader 的 plan.md **D3** 未落地。现为 `["./internal/crisis","./cmd/atlas"]`。

⚠️ **收益有限，据实说明**：`dev_done` 门禁只在 `transition dev_done` 那一刻执行，改 `packages` **不会**重新触发覆盖率校验。本次修正的价值是范围声明与实际一致、下次 transition 生效，**不能**据此认为 `cmd/atlas` 的覆盖率已被门禁量过。dev 手跑的 `cmd/atlas` 覆盖率为 78.2%，`coverage_floor=78`，**余量为零**（dev 背对背对照：pre 78.0% / post 78.2%，本任务抬高了 0.2 个百分点，但 master 基线本就卡在 floor 上）。

## 4. done_criteria 覆盖矩阵（8/8）

**每条都配了针对性变异**（而非只靠空桩 N10 —— 空桩一次杀 10 条，能证明断言不空洞，但不能证明它守的是**这一条**性质）：

| # | 完成标准（摘要） | 对应测试 | **针对性**变异 | 判定 |
| --- | --- | --- | --- | --- |
| functional[0] | 老形状 3 行迁移：三计数=3、新表三段主键 + `fetched_at TEXT NOT NULL`、双向 `EXCEPT` 各 0 行 | `TestMigrateConvertsLegacyShape` | **N8** `INSERT SELECT` 加 `LIMIT 1` → 红 | **PASS** |
| functional[0]附 | 新表 DDL 复用 TASK-001 的生成函数，不另写建表 SQL（零副本） | `TestMigrateReusesSchemaDDL`（源码扫描） | **N3** 往 migrate.go 塞一份 `CREATE TABLE` → 红 | **PASS** |
| functional[1] | 幂等：第二次 `AlreadyMigrated=true`，行数与 `sqlite_master` 计数均不变 | `TestMigrateIsIdempotent` | **N11** 删掉 `AlreadyMigrated` 早返回 → 红 | **PASS** |
| functional[2] | 旧表保留为 `macro_observations_v1` 且行数一致（C3） | `TestMigrateKeepsLegacyTable` | **N12** 迁移末尾 `DROP TABLE _v1` → 红 | **PASS** |
| functional[3] | **C2 索引重建**：`idx_macro_obs_ind_ts` 的 `tbl_name` 是新表 | `TestMigrateRebuildsIndex` | **N2** 删掉 `DROP INDEX` 步骤 → 红 | **PASS** |
| boundary[0] | NULL `fetched_at` ⇒ 报错，且失败后仍老形状、无 `_v1`、无 `_new`（事务原子） | `TestMigrateRejectsNullFetchedAtAtomically` | **N8** → 红 | **PASS** |
| boundary[1] | 空库三计数为 0；已新形状直接 `AlreadyMigrated=true`；**探形状看主键不看 `_v1`** | `TestMigrateEmptyDB` / `TestMigrateAlreadyMigrated` / `TestMigrateShapeProbeIgnoresLegacyTable` | **N7** 探形状改成看 `_v1` 是否存在 → 红；**N6** `_v1` 占名时静默 `DROP` → 红；**N11** → 红 | **PASS** |
| error_handling[0] | `--db` 指向不存在的文件 ⇒ 报错、文案含路径、**不创建库**；缺 `--db` 报错且不开库 | `TestMigrateMissingDBFile` + `TestExecuteCrisisMigrateRequiresExistingDB` + `TestCrisisMigrateCmdWiring` | **N5** 删掉 `os.Stat` → 两条同时红 | **PASS** |
| non_functional[0] | `go build ./...` + 两包全绿 + cmd 级自带测试 | 见 §2、§3 | — | **PASS** |

## 5. 🔴 `DROP VIEW` 零覆盖 —— 实现正确，但没有断言在守

### 5.1 两条独立取证路线，结论一致

**验证者侧（本报告）**：N1（删掉 `DROP VIEW` 步骤）→ **零测试变红**，531 PASS 一条不少。

**dev 侧（dev-m4c-a 在派验后自曝，6 个变异，Leader 转达）**：同一缺口，且它做了**二维矩阵**，比我的单点更完整：

| 夹具 | 断言 | 删掉 `DROP VIEW` 的变异 |
| --- | --- | --- |
| 无视图 | 现有断言 | **存活**（= 我的 N1） |
| **带视图** | 现有断言 | **仍存活** |
| 无视图 | 加行为断言 | **仍存活** |
| **带视图** | **行为断言** | **KILLED**（= 我的探针） |

⇒ **夹具与断言两个维度缺一不可**。这个结论我和 dev 各自独立得到、且从相反方向覆盖了矩阵的两端（我做了左上格与右下格，dev 补了右上与左下两格）。两条路线互为佐证，不是一方抄了另一方。

### 5.2 为什么现有断言在结构上抓不到它（机制）

迁移后 `_v1` 与新表**内容相同**。视图即使被 `RENAME` 改写成指向 `_v1`：

- `ViewRows` 仍等于 `RowsAfter`（同样的行数）
- 双向 `EXCEPT` 仍全为 0 —— 因为那实际是 **`_v1` 减 `_v1`**

⇒ `TestMigrateConvertsLegacyShape` 里那两条 `EXCEPT` 断言**在结构上区分不了「视图指向新表」与「视图指向 `_v1`」**。这不是断言写得不够仔细，是它被放在了一个测不出该性质的位置上。

### 5.3 探针实证（真实场景，直接观察而非推理）

老库已有视图 = TASK-001 之后被 `NewStore` 打开过的生产库（`NewStore` 每次打开都执行 `schemaDDL()`）：

| | 视图 DDL | 三计数 | 迁移后写入新修订，经视图读回 |
| --- | --- | --- | --- |
| **真实实现**（含 `DROP VIEW`） | `FROM macro_observations` ✓ | 1/1/1 | **99**（新值）✓ |
| **去掉 `DROP VIEW`** | `FROM "macro_observations_v1"` ✗ | **1/1/1（仍全相等）** | **10**（迁移前的旧值）✗ |

⇒ Leader 要我确认的「`DROP VIEW`/`DROP INDEX` 在 `RENAME` 之前」，**实现满足**（步骤表：`DROP VIEW` → `DROP INDEX` → `RENAME` → `schemaDDL()` → `INSERT SELECT`），且探针实证它在真实场景下工作正常。但**这一步没有任何断言守护**，后人删掉它不会有任何测试变红，而失败是静默的。

对照：**`DROP INDEX` 有守卫**（N2 杀死 `TestMigrateRebuildsIndex`）——因为夹具 `legacySchema` 建了同名索引，走完整迁移路径就会触发该坏法。两者的差别正是夹具有没有把被测状态准备出来。

### 5.4 为什么这不构成 rejected（Leader 裁决 + 载体分析）

`plan.md` D2 原文写「⇒ **TASK-002 侧要求 dev 补『视图指向 + 迁移后新修订经视图可见』的行为断言**」，但该义务**从未进入 TASK-002 的 `done_criteria`**（8 条里没有），dev 的 `questions[0].answer` 至今为 `null`（Leader 的裁决消息丢了三次）⇒ dev 大概率从未收到。

**Leader 裁决（2026-09-19）**：判 `VERIFIED`。依据是**载体强度 DoD > plan 散文**——把一条既没进 DoD、也没到 dev 手里的要求当拒收理由，等于用 plan 的散文去量一个没写进验收标准的东西。Leader 并已自记：「D2 写的是『TASK-002 侧要求 dev 补行为断言』，而那条要求等于挂在空处——这是我的错」，将记入 final-report。

**缺口去向**：由 TASK-003 接管，**已核实落盘**（见 §6）。

## 6. 下游接管已核实（我自己读的原文，不采信转述）

`jq -r '.done_criteria.functional[1].desc' .arcforge/tasks/TASK-003.json` 现文已包含：

- 夹具必须是**带 `v_macro_current` 视图的老形状库**（模拟被新版 `NewStore` 打开过的生产库）
- **且**必须有**行为断言**（迁移后写入一个新修订，经视图读得到）
- 三种存活形态的实测记录
- 「纯 DDL 文本断言不够」的机制（`_v1` 减 `_v1`）

⇒ **我此前提出的核心担心（下游守卫会以同样方式恒真）已经解决**。我先前读到的是旧版，Leader 已于 23:24 左右补写。TASK-003 现状 `assigned`。

## 7. 其余变异存活（DoD 之外）

### 7.1 子命令的计数断言近乎恒真（N9）

**N9（把三个计数全打成 `0`）→ 零测试变红。**

`TestExecuteCrisisMigratePrintsCounts` 的计数断言是 `assert.Contains(t, s, "3")`，而输出里含 `t.TempDir()` 的路径，几乎必然含数字 `3` ⇒ 该断言分不出 `3` 和 `0`。而 `crisis_migrate.go` 的注释自己说三个计数是「运维现场唯一的判据」。

**已由 Leader 就地处理**：补进 **TASK-008 的 DoD**（仍 pending 时写入）——判据一今后须用 `sqlite3` **独立复算**三个数、与命令输出逐一对照，不得只信命令打印。理由正是 TASK-008 靠读这三个数判「迁移成功」，而本报告证明这三个数没有强守卫。**N9 的原始证据保留在此备查。**

### 7.2 「迁移不经 NewStore」此刻无守卫（N4，预期内）

**N4（把 `MigrateBitemporal` 改成经 `NewStore`）→ 零测试变红。**

与 Leader 预判及 dev 的 `decisions` 一致：TASK-003 的守卫此刻不存在，所以走 `NewStore` 也全绿。TASK-003 `boundary[0]` 已有 AD-5 断言（「同一老形状库上 `NewStore` 报错而 `MigrateBitemporal` 仍成功」）接管。**这是「绿不构成证据」的教科书例子**，记此备查。

## 8. 抽验 dev 的 RED 证据 —— 属实

dev 自报「RED 阶段发现三条测试在空桩下也绿，逐条补强后才达到 14/14 全红」。

**N10（把 `MigrateBitemporal` 换成空桩）**：Leader 点名的三条 `TestMigrateRebuildsIndex` / `TestMigrateEmptyDB` / `TestMigrateShapeProbeIgnoresLegacyTable` **全部变红**（共 10 条红）。与 dev 的自述一致 ⇒ 补强真实生效。

## 9. 报告勘误（本报告初版写错的一条）

🔴 **初版 §3 写道：「dev 已交棒，`in_progress` 窗口关闭，此刻无任何角色能合法补该字段」——这句话是错的。**

`.arcforge/write-matrix.json` 的 owner_table 明写 `"verifying": ["test-*"]`、`"verified": ["test-*"]`，也就是说**从派验那一刻起直到 `accepted`，合法写者一直是我**。窗口从未关闭，只是**换了角色**。

成因值得记：我是**从 CLAUDE.md 里关于 `dev_done` 之后 dev 写不了 `writes` 的那段推理**得出的结论，而**没有去读 owner_table**。那段叙述讲的是 dev 与 leader 都被挡下，我把它外推成了「所有角色」——而我自己恰好在例外里。这是「**拿相关的记忆代替对载体的一次查看**」，与本报告 §5.2 批评断言被放错位置是同一类错误，只是载体从测试换成了权限矩阵。

⇒ 已据此**实际补齐 `packages`**（§3），并留下审计行 `{"op":"update","by":"test-m4c-a","keys":["packages"],"changes":[{"key":"packages","added":["./cmd/atlas"],"removed":[]}]}`。

**附带推论**：Leader 消息中「等它转 `dev_done` 窗口就关了」同样不准确——`dev_done` 的合法写者是 `dev-*`，窗口在那时仍对 dev 开着。

## 10. 变异测试方法学（有效性闸命中两次，已重做）

隔离 worktree `../wt-verify-TASK-002 @ d347c9adece0b657f972a63b21c08929905ae59a`，12 个变异全部作用于副本，收尾已 `remove --force` + `prune`。

- **对照组 / 收尾对照**：均 `rc=0`、`PASS 531`、`FAIL 0`（同值 ⇒ 还原完整）。
- **主工作区指纹**：四个交付文件 sha256 收尾与开工**逐字节一致**，`git status --porcelain -- internal/ cmd/` 为空。
- 🔴 **有效性闸命中两次，这两个变异不计数**：首轮 **N8**（`vet` 报 `syntax error: unexpected name SELECT`，我的替换切断了 raw string）与首轮 **N10**（`"os" imported and not used`）都是**编译失败**，套件全面变红、`--- FAIL:` 一条都没有。若按「退出码非 0 就算 KILLED」，这两个会被记成**假 KILLED**，而假 KILLED 直接推翻「这条断言有牙」的结论。判别式：`vet_rc != 0`，或「`rc != 0` 且 FAIL 列表为空」。**已按闸剔除并重做**，矩阵取值来自重做版。
- 每个变异体落盘后打印与原文的 **diff** 逐行核对（语义闸），避免「语法对但语义走样」。
- 🔴 **空桩是钝器**：N10 一次杀 10 条，能证明断言不空洞，但不能证明它守的是**那一条**性质。初版矩阵里 functional[1]/[2] 只有空桩证据，补了 **N11/N12** 两个针对性变异才算数。

## 11. 结论

8 条 done_criteria **全部通过**，每条都有**针对性**变异证据。亲跑 build/两包测试全绿，无越界写入，既有测试零改动，判定对象与 `verify_baseline` 零漂移。Leader 的 D1/D2 裁决口径已按其执行，`_new` 字面未用于量本任务。要求确认的 `DROP VIEW`/`DROP INDEX` 顺序经探针实证正确；要求抽验的 RED 补强属实。

**裁决：VERIFIED。**

**已闭环的三件事**：①`DROP VIEW` 守卫 → TASK-003 `functional[1]`（我已读原文核实）；②N9 计数弱断言 → TASK-008 DoD；③`packages` 缺 `./cmd/atlas` → 本验证者已补（§3、§9）。

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 d347c9adece0b657f972a63b21c08929905ae59a
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas

# 变异（隔离副本，主工作区一个字节不碰）
git worktree add --detach ../wt-verify-TASK-002 d347c9adece0b657f972a63b21c08929905ae59a
python3 <scratchpad>/test-m4c-a-T002-mutate.py    # N1-N7, N9（N8/N10 首轮无效，见 §10）
python3 <scratchpad>/test-m4c-a-T002-mutate2.py   # N8/N10 重做
python3 <scratchpad>/test-m4c-a-T002-mutate3.py   # N11/N12 针对性补充
git worktree remove --force ../wt-verify-TASK-002
```

# §2 第 2 轮（树 `d78a4d880cba940382f91b37e3ae6aaccda709c1`，`epoch=1`，`rework_count=1`）

**裁决：VERIFIED。** 三条 `fix_items`（+1 条 Leader 自陈）逐条有证据。

## 2.0 本轮不继承什么 —— 第一条是作废我自己的判断

🔴 **我上轮报告里那句「`assert.False(objectExists("…_new"))` 是恒真断言……但留着无害」作废。**

理由是它与我自己在同一 sprint 立下的 F7 矛盾（「判据在被测对象为空或不可达时退化为恒真，而退化后的输出与正常通过完全同形」）—— **我把这条通则写出来了，却在它的第一个实例上给了豁免**。

⚠️ 🔴 **订正（写于本报告落盘之后）**：我原在此处写「而且我在 TASK-008 §B4① 又引用了这条豁免一次 ⇒ 一次错判被我自己复用成了先例」。**那句话是错的，我撤回它。**
实测 TASK-008 报告（追加勘误之前的 `:1-685`）：`'B4'` 命中 **0**、`'留着无害'` 命中 **0**、`'_new'` 命中 **0**；该报告的章节是 §0–§12 / §C / §D，**没有 §B 段**。
⇒ **那条豁免没有被复用到任何地方**，「被复用成先例」这个说法不成立。错误源头是我自己 checkpoint `:119` 的一句话，我此后多次转述而从未核过原文。详见 TASK-008 报告 §F。
（**「留着无害」这个判断本身仍然是错的、仍然作废** —— 订正的只是「它传播到了哪里」。）

⇒ 本轮对 `boundary[0]` **从零判**，判据是 F7 + 变异实测，不是我上轮的结论。

| 其余上轮结论 | 处置 |
| --- | --- |
| 上轮 12 个变异（条件：树 `d347c9ad…`、对照组 531 PASS） | **全部不沿用**，树已变 |
| 「三计数是唯一判据」相关结论 | **重验**，本轮 `migrate.go` +68/−2 |

## 2.1 承接七条 —— ⑦ 第三次命中

①~⑥ 全过（HEAD/discovery 双一致 `d78a4d88…`/`2e7ed2fb…`；⑥ `writes` 四文件均无未提交改动）。

```
⑦ 审计行 in_progress→dev_done = 2026-09-19T09:54:00Z
   交付提交 b43832e            = 2026-09-19T10:36:03Z   ⇒ 晚 42 分钟，命中
```

⇒ **这是该检查在本 sprint 的第三次命中（TASK-008 / TASK-007 / TASK-002），命中率 100%。** 见 §2.6 的判据精度讨论。因门禁记录不对应当前版本，**本轮全部结论建立在我对当前 HEAD 的实测上**。

## 2.2 🔴 `fix_items[0]`（V-3）：我在看到交付之前定死的五条判据

判据写于承接之前（防锚定），逐条结果：

| # | 判据 | 结果 | 证据 |
| --- | --- | --- | --- |
| ① | 残留任意对象 ⇒ 必须变红 | **KILLED** | 变异在迁移失败路径建 `zzz_leftover_probe` 表 ⇒ `go build` ok、`test rc=1`、**FAIL 在 `migrate_test.go:257`** 正是那条全集比对 |
| ② | 用**实现里从不存在**的名字 ⇒ 也红 | **KILLED（同一变异）** | 我特意选了实现里 0 命中的名字 —— **旧的名单式 `_new` 断言对它完全无感** |
| ③ | `before` 须在迁移**之前**采样 | **PASS** | `:243` 采样 → `:245` 调 `MigrateBitemporal` |
| ④ | `before` 须非空 | ⚠️ **前提成立，但无断言钉住** | 我实测 `len(before)=3`（`table:macro_observations` + 2 个 index）；全文**无 `NotEmpty` 类断言** |
| ⑤ | 穷尽扫所有 `objectExists` | **PASS** | 对象名在实现里各出现 3 / 7 / 5 次；唯一 0 次的 `macro_observations_new` **在注释里**（撤回留痕）⇒ 无第二个 V-3 |

**①② 是决定性的**：它们证明修法不只是「换了个更好听的写法」，而是**真的能挡住你想不到的名字** —— 这正是 V-3 的根因（名单式只挡得住你列出来的那个）。

**④ 是缺口但当前不可达**：`legacyDB` 夹具构造上必产出非空对象集。**建议补一条 `require.NotEmpty(t, before)`** —— 否则将来有人把夹具改成空库，这条会静默退化成 `assert.Equal(nil, nil)`，**而退化后的输出与正常通过完全同形**（F7 原文）。

## 2.3 `fix_items[1]`（V-9）：列级校验有牙

```
变异：让 columnsNotMigrated 恒返回 nil（校验失效）
go build rc=0        ← 真正的编译判据
go test  rc=1        --- FAIL: TestMigrateRejectsUnmigratedColumns
```

⇒ **KILLED。**

🔴 **过程中修正了我自己的一条有效性闸**：该变异 `go vet` **rc=1**，按我原来那条「`vet_rc != 0` ⇒ 变异无效」会被判废。查 vet 实际输出是 `internal/crisis/migrate.go:29:2: unreachable code` —— **静态警告，不是编译失败**；`go build` rc=0 证明代码能编译、变异有效、红是真红。

⇒ **修正：编译判据用 `go build`，`vet` 只作静态检查，两者退出码含义不同。** 原来那条闸会把「加了一行 early return」这类最常用的变异全部误判为无效。

## 2.4 `fix_items[2]`：假注释已清零

```
'两次 RENAME'   migrate.go 0 处 / migrate_test.go 0 处
改后原文（:73-74）：「回滚：DROP 掉新表再把它 RENAME 回去（一次 RENAME，不是两次 …）」
```

⚠️ `grep -cF 'RENAME TO' migrate.go` = **2**，看起来与「只有一处」矛盾。**我去看了**：`:74` 是**注释自引用**（那句话自己含 `RENAME TO` 字样），`:176` 是代码。⇒ **代码里确实只有 1 处，注释成立。**

（同样地，§2.2 判据⑤ 扫描时我的提取器在 `:290` 抓到的是**断言消息**而非对象名，去看代码才知道那里用的是常量 `legacyTableName`（实现里 7 次）。**两次「不从计数推结论」都避免了误报。**）

## 2.5 🔴 新发现：注释里的行号指针，在写下它的**同一个 commit 内**就过期了

```
migrate.go:74 写：「本函数向前迁移时也只有 :125 那一处 RENAME TO」
b43832e~1 的 :125  =  {"renaming legacy table", ALTER TABLE … RENAME TO …}   ✅ 写下时为真
b43832e   的 :125  =  isBitemporal 里一句无关的 SELECT sql FROM sqlite_master
b43832e   的实际位置 = :176（本轮 +68/−2 把它推移了 51 行）
```

⇒ **那个指针在它所属的这次提交里就已经指错了。**

⚠️ **最坏处不是过期，是 `:125` 现在有东西** —— 一句与上下文毫不相干、却又看起来挺合理的 DDL 查询。读注释的人跳过去会以为自己看错了，而不会怀疑指针。

**建议**：改成符号锚（「`renaming legacy table` 那一步」）。符号名在重命名时**找不到**（有声失败），行号在插入时**指向别的东西**（静默错指）。

### 为什么不 REJECT

1. 不在 `fix_items[2]` 的验收项内（判据是「改成实际次数」，已做到）；
2. 不影响任何行为、不影响任何测试；
3. 🔴 **我刚在 TASK-007 §4.5① 对同类问题（F7 的教训未推广到 F2）判了不 REJECT，必须用同一把尺。**

⚠️ 第 3 条是决定性的，而且要写明它的反面：**本任务 `rework_count=1`（有额度）而 TASK-007 是 3（到顶）。若我因为这个任务还有额度就判得更严，我的判定就成了额度的函数而不是缺陷的函数。** 判定依据缺陷本身。

## 2.6 判据精度：⑦ 这条检查三次全中，它测的可能不是它的名字

三次命中（TASK-008 / TASK-007 / TASK-002），命中率 **100%**。而三次的成因各不相同：

| 任务 | 成因 |
| --- | --- |
| TASK-008 | 日志**必须由 ops 落盘**，而 ops 落盘必然在 dev 交棒之后（结构性） |
| TASK-007 | dev 在 `dev_done` 状态下改了交付物（它自陈） |
| TASK-002 | 转 `dev_done`（09:54）在前、提交（10:36）在后 |

⇒ 第三种是**「先转状态再提交」这个工作流本身**，与「交付物被改动」无关。

🔴 **该检查的名字是「门禁后修订」，但它实际测的是「提交时刻晚于 dev_done 时刻」。** 后者在「先转后提交」的工作流下**恒为真**。⇒ 有告警疲劳的风险：全量告警会让确认退化成条件反射，真信号被噪声淹没（与 AD-29 收窄判据同一条道理）。

**要区分两者，需要 `dev_done` 那一刻的工作树内容**，而 `verify_baseline.head` 记的是**派验时**的 HEAD，不是 `dev_done` 时的。⇒ 当前字段不足以区分，**这是给 Leader 的一条机制观察，不是本任务的缺陷**。

## 2.7 范围与其他

- **我未修改任何文件**；变异全在隔离 worktree，收尾 `git status` 空、worktree 我建我拆、残留 0。
- 交付范围 `b43832e`：`migrate.go` 68/2、`migrate_test.go` 61/2，均在 `writes` 内；另两个 `cmd/atlas/crisis_migrate*.go` 本轮未动（最后提交 `e93c950`）。
- 生产库未被触碰。

## 2.8 结论

**VERIFIED。** V-3 的修法经**我自己设计的变异**证明能挡住实现里从不存在的名字（①②同时 KILLED），这是它区别于旧名单式写法的核心；V-9 的列级校验同样 KILLED；假注释清零。

**移交 Leader 三件**：§2.2④ 建议补 `require.NotEmpty(t, before)`；§2.5 行号锚改符号锚；§2.6 ⑦ 检查的判据精度（三次全中，其中一次测的是工作流而非缺陷）。

---

# §3 🔴🔴 订正：我对 `fix_items[1]`（V-9）判 PASS 是**错的**

**写于 2026-09-19T12:1x，即本报告判定 VERIFIED 之后。** Leader 转来一份独立审查（其材料到达时我已落判定），
其中一条 HIGH **我已独立复现，成立**。⇒ **本报告 §2.3 对 `fix_items[1]` 的 PASS 判定作废。**

## 我实测的（A/B 同环境背对背）

变异只有一行：**给 `tablesDDL` 加 `note TEXT`，不动 INSERT 的硬编码列清单**。
老库形态相同（多一列 `note`，3 行有值）。

| | `migrate err` | 三计数 | 新表 `note` 非空 | `_v1` `note` 非空 |
| --- | --- | --- | --- | --- |
| **变异组**（`tablesDDL` 已加 `note`） | **`<nil>`** | 3/3/3 **相等** | **0** | **3** |
| 对照组（`tablesDDL` 未加） | **报错并点名 `note`** ✓ | — | 3（原样） | 不存在（已回滚） |

⇒ **数据静默丢失、三计数全绿、迁移报成功。V-9 原样复现。**

## 守卫为什么会瞎 —— 失明的时机是要害

```
守卫 columnsNotMigrated 比:  columnNames(_v1)  vs  columnNames("macro_observations")  ← 新表实际列
真正搬数据的 INSERT (:178):   硬编码 5 列 (ts, indicator, value, source, fetched_at)
⇒ 两份不同的清单
```

新表的列来自 `tablesDDL`。一旦 `note` 被加进 `tablesDDL`，新表就**有**这一列 ⇒ 差集为空 ⇒ 守卫放行；
而 INSERT 仍只搬 5 列 ⇒ 数据丢。

🔴 **而守卫自己的错误信息写的是「给 `tablesDDL` 加列时必须同步改它」** ——
**第一步（加进 `tablesDDL`）恰好使守卫失明，第二步（改 INSERT）无人看守。**
⇒ 借审查者的话：**「守卫只在你还没开始修的时候保护你。」**

⇒ `fix_items[1]` 要的是「让丢列这种情形**能被某个机制发现**」。
**最可能发生丢列的那一刻（正在改 schema 的过程中），机制是瞎的** ⇒ **未满足。**

## 我错在哪 —— 不是漏测，是测了**错误的性质**

我在 §2.3 跑的变异是「让 `columnsNotMigrated` 恒返回 nil」⇒ 测试红 ⇒ 我判 KILLED。

**那个变异证明的是「守卫有牙」，不是「守卫覆盖 V-9 描述的场景」。** 两件事。

⇒ 这正是我自己在本 sprint 记过的形状：**守卫有效 ≠ 守卫守的是你以为的那件事**。
我验了守卫会响，没验**它在真实的丢列路径上会不会响**。

⚠️ 更准确地说：**我的变异破坏的是守卫本身，而真实的失效模式是「守卫完好但参照系被移动了」。**
破坏型变异查不出参照系问题 —— 它只能回答「这行代码有没有用」，不能回答「它比的是不是该比的东西」。

## 连带订正

- **本报告 §2.7「结论」中「V-9 的列级校验同样 KILLED」一句作废。**
- `migrate.go:195` 注释里「note 从 3 变成 0」这个数：审查者指出它**采自它没有描述的那个条件**。
  我的 A/B 表印证了这一点 —— **能量出 `3 → 0` 的唯一状态正是上面那个旁路**；
  在注释描述的场景（`tablesDDL` 未加 `note`）里迁移直接失败，量不出那个数。
- `MigrateResult` 注释仍写三计数是「**唯一的**」判据——本轮加了第二个判据却没动它，
  且按上面的旁路它在该场景里**仍字面为假**。

## 我的判定（更新）

**`fix_items[1]` = FAIL。** 本任务应为 **REJECTED**（`reason_class=task_defect`）。

⚠️ **但我现在无权执行**：任务已在我手上从 `verifying` 转到 `verified`，而 `verified → review_fix`
是 **leader 专属边**，`verifying → rejected` 需要它仍在 `verifying`。
⇒ **已即时通知 Leader，请他转 `review_fix`。** 本节是该请求的书面依据。

## 修法方向（审查者给的，我复核后认为对，但这不是判定依据）

把那 5 个列名提成包级切片，INSERT 由它拼出，守卫用「旧表列 ∖ 该切片」——
**两份清单合一，该旁路在构造上不存在**。⇒ 与我在本 sprint 反复记的「判性质不判数量」同源：
当前守卫问的是「新表有没有这列」，该问的是「**搬运语句搬不搬这列**」。

---

# §4 收尾指引（写于 TASK-002 转 `accepted` 之后）

🔴 **读到这里的人请注意：本任务状态是 `accepted`，但它含一条【未在本任务内修复】的 HIGH。**

- **那条 HIGH 见上面 §3**（守卫 `columnsNotMigrated` 的参照系是 `tablesDDL`，而 `tablesDDL` 正是修复流程第一步要改的东西 ⇒ 守卫在最该起作用的那一刻失明；A/B 实测数据在 §3）。
- **它由 `TASK-010` 接手**（人类裁决，本 sprint 内补开）。⇒ 判断「这条缺陷修没修」**请看 TASK-010，不要看本任务的 `accepted` 状态**。
- **为什么没在本任务内返工**：我的撤回（12:17Z）与 Leader 转 `accepted`（12:42Z）之间存在上下文延迟；`accepted` 无出边、是终态，`verified → review_fix` 的路径已关闭。⇒ **这是流程时序的结果，不是「该缺陷被判定为可接受」。**

## 🔴 一条关于本报告自身的警告（给核实者）

本报告含**撤回留痕**（§3 及 TASK-008 报告 §F 同理）。⇒ **用 `grep -c '<某字串>' 本报告` 来判断「某个结论是否已被移除」会得到假阳** —— 命中的可能全是**撤回留痕里对它的引用**，而不是**活着的结论**。

**实撞两次，方向相反**：

| 谁 | 做了什么 | 错法 |
| --- | --- | --- |
| 我 | 查一处引用是否存在，`grep` **返回空**而我按记忆写下了勘误 | **数据为空而写了结论** |
| Leader | 核我的撤回依据，对**整个文件** `grep` 得 `6/6/3`，差点判「验证者的撤稿依据是假的」 | **数据非空而没问那些命中在哪** |

⇒ **同一个缺口的两个方向：都没有去看命中项本身。**

📌 **正确判据**：问「**它还在不在**」而不是「**这个字串出现几次**」。两种可机械执行的形态：
1. `git show <移除前的 ref>:<file>` 与当前版本做 diff —— 移除会出现在删除行里；
2. 核实范围**排除撤回留痕段**（例如把留痕统一包在 `<details>` 或显式标记里，核实时先剔除）。

⚠️ **这是「撤回留痕」这个做法的代价，而我此前只把它当纯粹的好事**：
保留原文供追溯 ⇒ 原文仍可被 `grep` 命中 ⇒ **它成了下一个核实者的假阳源**。
两者都要，就必须让留痕**在结构上可被排除**，而不是靠核实者记得。
