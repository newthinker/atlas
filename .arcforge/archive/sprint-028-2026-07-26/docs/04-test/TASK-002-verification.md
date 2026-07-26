# TASK-002 验证报告 — prism 存储层扩展

- **验证者**: test-agent-7 (Reality Checker)
- **被验对象**: commit `018446d` / package `./internal/storage/prism`
- **assignment_epoch**: 1
- **判定**: ✅ **PASS (verified)**
- **验证纪律**: 全部结论锚定我本人实跑的命令输出与文件内容；未采信 dev-agent-16 的任何自述数字。
  所有实验在 `t.TempDir()` 或隔离 git worktree 内进行，**未触碰任何 runtime 库**，
  验证结束后主工作树 `internal/`、`cmd/` 零残留（`git status --porcelain` 为空）。

---

## 1. 实跑证据摘要

| 命令 | 结果 |
|---|---|
| `GOTOOLCHAIN=local go test ./internal/storage/prism/ -count=1 -cover` | `ok 1.813s coverage: 81.3% of statements`（门槛 `dev_minimum`=80） |
| `... -count=40 -run 'Concurrent\|Idempotent\|Busy'` | `ok 49.342s` 全绿 |
| `GOTOOLCHAIN=local go build ./...` | OK |
| `GOTOOLCHAIN=local go vet ./internal/storage/prism/` | OK |
| 相邻包 `./internal/prism ./internal/storage/... ./cmd/...` | 全绿（sankey 例外见 §7，非本任务） |

---

## 2. 三条硬红线（生产数据安全）

### 红线 1：迁移只做 `ADD COLUMN` ✅
全文件 DDL 清单（`grep 'ALTER|DROP|RENAME|CREATE TABLE|CREATE INDEX|DELETE FROM|UPDATE '`）：
5 条 `CREATE TABLE IF NOT EXISTS` + 1 条 `ALTER TABLE fundamental_q ADD COLUMN`（sqlite.go:158）。
**无任何 DROP / RENAME / 表重建 / DELETE / 数据 UPDATE 语句。**

### 红线 2：迁移后既有数据逐值不变 ✅（做了比 DoD 更强的穷尽验证）
DoD 只要求「行数 + 抽样行值」，dev 的 `TestMigratePreservesExistingData` 也只做抽样。
我在隔离 worktree（checkout 018446d）内加临时探针，**穷尽比对**而非抽样：

预置 2 instrument + 500 valuation_daily + 84 fundamental_q（含 NULL、负数、
极小浮点、非 ASCII 名称），走**真实生产 `prism.Open()`** 触发 migrate，
迁移前后按旧列全量取出、区分 `NULL` 与值、带上类型、排序后 sha256：

```
instrument              行数 2→2     sha256 181759750764dbf0→181759750764dbf0  一致=true
valuation_daily         行数 500→500 sha256 8544992e64a00e65→8544992e64a00e65  一致=true
fundamental_q(仅旧列)    行数 84→84   sha256 60062424d6e0f75f→60062424d6e0f75f  一致=true
旧列 cid 顺序原封不动 + 恰好追加 5 列 = true
追加的 5 列 = [gross_profit rnd sgna operating_income income_tax]
===== 存量数据完全未变 = true =====
```
探针文件已删除，worktree 已 `git worktree remove`。

### 红线 3：新列在 `const schema` 中位于 `source` 之后 ✅
`sqlite.go:52-57`：`source TEXT,` 之后依次是 `gross_profit / rnd / sgna /
operating_income / income_tax`，位于 `PRIMARY KEY` 之前。

**并做了变异测试验证这条约束确实是 load-bearing**（隔离模块，未改仓库源码）：
```
[生产写法:新列在 source 之后] 新建库与迁移库列序一致 = true
[变异体:新列插在中间      ] 新建库与迁移库列序一致 = false
    fresh   : [... revenue gross_profit rnd sgna operating_income income_tax net_income ...]
    migrated: [... revenue net_income eps_diluted equity diluted_shares source gross_profit ...]
```
`TestMigratedSchemaMatchesFreshSchema` 用 `ORDER BY cid` 取出后对 `[][2]string`
做 `assert.Equal`（顺序敏感），确认能抓住该漂移；并有
`require.NotEmpty(t, info, "空列表说明表根本没建,比较将失去意义")` 防止
「两表都不存在 → 平凡通过」的空洞比较。**这是一条合格的护栏，不是摆设。**

---

## 3. AD-10 修正的独立复现（未采信 dev 结论，自建实验）

在隔离模块中复刻**修复前**的 `Open`（schema 初始化直接 `db.Exec`，无重试），
DSN 与生产完全一致（`journal_mode(WAL)&busy_timeout(5000)`），30 轮 × 8 并发：

```
[修复前(无重试)/非WAL存量库] 失败 3/240 次 Open
    x3  prism: init schema: database is locked (5) (SQLITE_BUSY)
[修复前(无重试)/已WAL库    ] 失败 0/240 次 Open
[修复后(有重试)/非WAL存量库] 失败 0/240 次 Open
```

**结论：dev-agent-16 的修正成立，且我复现出的数字与其自述完全一致（3/240 与 0/240）。**
故障点确实在 schema 初始化的 SQLITE_BUSY（WAL 转换需独占且不走 `busy_timeout`），
错误位置字符串 `prism: init schema` 亦吻合；已是 WAL 的库不触发，
这也解释了为何 runtime 库（早已 WAL）风险有限。

### 但 AD-10 的原始判断并未被推翻
dev 称 duplicate-column 容错「必要但不充分」。我独立测量该分支**是否真的会触发**
（覆盖率无法区分「ALTER 成功」与「duplicate 被容错」，两者走同一后继块）：
复刻 `migrate()` 的 check-then-act，30 轮 × 8 并发分类计数：

```
ALTER 成功=150   duplicate(被容错)=54   其他错误=0   Open 失败=0
```
**duplicate 竞态每轮平均触发 1.8 次 —— AD-10 的容错分支是 load-bearing 的活代码，
不是防御性死代码。** 两个故障点都真实存在，dev 的表述准确。

### 重试的安全性核实
- **幂等安全**：`execRetryBusy` 只被喂 `const schema`，其内容经逐条核对为
  5 条 `CREATE TABLE IF NOT EXISTS`，重复执行无副作用。
- **不吞错**：`sqlite.go:125-134` 的 `err` 在函数作用域声明，重试耗尽后
  `return err` 返回的是**最后一次真实的 BUSY 错误原文**，非 nil、无包装丢失。
  `TestExecRetryBusy` 用真实持锁连接（`busy_timeout(0)`）构造，正向断言
  「锁在窗口内释放 → 成功」、反向断言「锁不放 → 返回 BUSY」、
  「非 BUSY 错误立即返回不进重试」、「nil 不误判」。
- **无误判风险**：`isBusyErr` 匹配 `"database is locked"`；SQLITE_LOCKED 的文案是
  `"database table is locked"`，不含该子串，不会被误吞。
- 退避总时长 20ms×(1+…+10)=1.1s，Open 失败前的额外延迟有界，可接受。

---

## 4. AD-9 核实 ✅

| AD-9 要求 | 实现 | 证据 |
|---|---|---|
| PK = `(instrument_id, period_end, segment_key)` | ✅ | sqlite.go:69 |
| `fiscal_period` 可空（无 NOT NULL） | ✅ | sqlite.go:65 |
| `LatestSegmentPeriodEnd` = `SELECT MAX(period_end)`，**不含 JOIN** | ✅ | sqlite.go:404，单表无 JOIN |

`TestSegmentRoundtripAndAnchor` 对 PK 语义做了**双向锁定**：
正向「同 (period_end,segment_key) 换 fiscal_period 标签 → 不新增行（3 行）」，
反向「同 fiscal_period 标签、不同 period_end → 各自成行（4 行）」——
后者正是 AD-9 要防的「标签碰撞静默覆盖」，反向断言到位。
锚点与 fundamental_q 解耦亦被显式断言（`SELECT COUNT(*) FROM fundamental_q ... require.Zero`）。

---

## 5. Done Criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| **F0** | 旧 schema 库 Open 成功，5 新列全部存在 | `TestOpenMigratesFundamentalQ`（`pragma_table_info` 逐列 `assert.Contains`） | **PASS** |
| **F1** | 迁移不损坏存量数据；行数与抽样行值不变；新字段读回 NaN | `TestMigratePreservesExistingData` + **穷尽 sha256 探针**（§2 红线 2） | **PASS**（强于 DoD 要求） |
| **F2** | 新 5 字段 NaN↔NULL 双向往返 | `TestUpsertFundamentalsRoundtrip`（扩断言：3 个 NaN 断言 + 2 个非 NaN 值断言 + 5 个逐字段相等） | **PASS** |
| **F3** | 分部 PK(AD-9)/幂等/manual 覆盖/(period_end,segment_key) 升序/fiscal_period 可空/锚点 MAX 不 JOIN 且推进 | `TestSegmentRoundtripAndAnchor`（6 个子场景，含 AD-9 正反双向） | **PASS** |
| **F4** | UpsertPrices 幂等、from 过滤、日期升序、close NaN↔NULL | `TestPriceSeriesFrom`（幂等后 `Len==3`、升序切片相等、NaN、覆盖写 172.5→180.0） | **PASS** |
| **B0** | 无分部锚点 `("",nil)`；未知 symbol `ErrNotFound`；from="" 返回全部 | `TestLatestSegmentPeriodEndEmpty`（含未知 instrument 9999 亦不报错）+ `TestPriceSeriesBoundaries`（`errors.Is` + 错误文本含 symbol）+ `TestPriceSeriesFrom`（from="" 3 行） | **PASS** |
| **B1** | AD-10 duplicate 容错不使 Open 失败；连续两次 Open 幂等；并发 Open ×N 全成功；新建库与迁移库 `pragma_table_info` 列名+类型一致 | `TestIsDuplicateColumnErr`（真实 sqlite 错误，正+反+nil）、`TestOpenIsIdempotent`、`TestConcurrentOpenAllSucceed`（8 并发 + `Len(names,15)` 防重复加列）、`TestMigratedSchemaMatchesFreshSchema`（5 张表逐 cid）、`TestExecRetryBusy` | **PASS** |
| **N0** | 全绿；既有 valuation/board/series/instrument 测试零断言修改；discovery 记录回滚安全性论证 | 实跑全绿；9 个既有测试函数逐函数 md5 比对全部「未改动」；全仓无 `SELECT *` | **PASS** |

**8/8 条 done_criteria 全部有对应测试且断言非空洞。**

### 既有测试零断言修改（独立确认）
`git show 018446d -- sqlite_test.go` 的 2 行 `-` 均位于
`TestUpsertFundamentalsRoundtrip` 的 `rows` 字面量内，是**同行扩字段**
（`... DilutedShares: 24.6e9, Source: "edgar"}` 拆成两行并插入 5 个新字段），
**原有断言一条未删、未改**。逐函数 md5 比对（`018446d^` vs `018446d`）：
`TestLatestDateEmptyThenAdvances` / `TestUpsertValuationsNaNRoundtrip` /
`TestBoardReturnsLatestPerInstrument` / `TestSeriesFrom` / `TestUpsertInstrumentIdempotent` /
`TestUpsertValuationsIdempotent` / `TestOpenCreatesNestedParentDirs` /
`TestSeriesUnknownSymbolReturnsErrNotFound` / `TestUpsertFundamentalsEmpty`
—— **全部「未改动」**。

### 回滚安全性论证核实
dev 称「仓库所有 SELECT 都显式列出列名、无 `SELECT *`」。
独立核实：`grep -rn --include='*.go' 'SELECT \*' .` → **零匹配**（exit 1）。
prism 包 12 处 SELECT 逐条查看，全部显式列名或聚合函数。
**论证成立**：`ALTER TABLE ADD COLUMN` 只追加列，M2 版二进制的显式列名 SELECT
不受影响，降级回滚无需任何 DDL 动作。

---

## 6. 覆盖率与对偶护栏

- **81.3%**（基线 81.0%，门槛 80%），无回退。
- 用 coverage profile 逐块核实而非只看总数：未覆盖块**全部是 IO/错误返回路径**
  （`MkdirAll` 失败、`sql.Open` 失败、`rows.Scan` 失败、`tx.Prepare` 失败等）。
  `migrate` 唯一未覆盖块是 `sqlite.go:159` 的非 duplicate ALTER 错误返回；
  `isBusyErr` / `isDuplicateColumnErr` / `execRetryBusy` / `toNull` / `fromNull` 均 **100%**。
- **对偶护栏充分**：每条关键 DoD 都有正向 + 否定成对断言 ——
  duplicate（认得出真错 / 不吞其他 DDL 错 / 不误判 nil）、
  BUSY（重试成功 / 耗尽如实返回 / 非 BUSY 不重试 / nil）、
  AD-9 PK（换标签不新增行 / 换 period_end 各自成行）、
  迁移（行数不变 / 值不变 / 新列为 NaN）、
  幂等（行数不增长 / 值被更新）。
  未见 `assert true` 类空洞断言，未见过度 mock（全部用真实 sqlite，
  错误一律由真实 sqlite 产生而非伪造字符串——这一点做得尤其好）。

---

## 7. 范围外发现（不影响本判定）

### 7.1 `internal/prism/sankey` 的红灯 —— 已确认为**观察窗口时序错觉，现已全绿**

验证过程中我实跑 `./internal/prism/sankey` 观察到 2 个失败
（`TestLoadTemplatesValidation/upper_case_segment_key`、`TestLoadTemplatesDuplicateCompany`），
当时判定其与 TASK-002 无关（018446d 与 d1d86ba 均未触碰 sankey，且该包在 c352c50 处全绿）。
该「无关」结论正确，但后续复核补充两点修正：

- **归因更正**：我当时写「dev-agent-18 的在途改动」，**这是错的**。
  实际 sankey 包归 **dev-agent-17（TASK-003，`packages=./internal/prism/sankey`）**。
  我是从工作区里一个未跟踪的 `learnings-dev-agent-18.md` 反推的，属不成立的推断。
- **状态更正**：那两个失败正是 TASK-003 **TDD 红灯中间态**的目标用例（先 RED 后 GREEN）。
  `caf8d9f`（2026-07-25 09:55:20 +0800，"reject upper-case segment keys and duplicate
  template companies"）提交后已全绿。我本人复跑核实：
  ```
  go test ./internal/prism/sankey/ -count=1 -cover → ok  coverage: 97.1% of statements
  git status --porcelain internal/prism/sankey/   → 空（工作区干净）
  ```
  我的观察窗口（sankey 实跑早于 09:55:20，本任务 verified 于 09:58:46）恰好跨了该提交时刻。

**教训（对应 AD-20）**：并发工作区中，他人未提交的 TDD 红灯 WIP 会让旁观者读到与自身
任务无关的失败。跨任务报红时应同时记录**观察时刻**与**归属任务**（从 `tasks/*.json`
的 `packages` 字段反查，而非从工作区文件名推断），否则易产生误归因。

### 7.2 轻微（不阻断，建议后续顺手修）
`sqlite_test.go:229` 的 done_criteria 映射注释提到 `TestMigrateToleratesDuplicateColumn`，
实际测试名为 `TestIsDuplicateColumnErr` —— 注释与实现命名漂移，不影响任何断言。

---

## 8. 判定

**verified（PASS）。**

理由：8/8 条 done_criteria 均有对应且断言非空洞的测试；三条生产数据安全红线
经穷尽比对（586 行逐值 sha256）而非抽样确认成立；AD-10 的根因修正经独立复现
且数字与 dev 自述完全吻合，同时另行证明 AD-10 原始的 duplicate-column 容错分支
同样是 load-bearing 的活代码；AD-9 的主键语义被正反双向锁定；既有测试逐函数 md5
确认零改动；覆盖率 81.3% 无回退且未覆盖块全为错误路径。达到「压倒性证据」标准。
