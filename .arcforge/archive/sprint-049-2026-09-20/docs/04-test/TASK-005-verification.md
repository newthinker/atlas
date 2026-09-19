# TASK-005 验证报告 — 写改追加：裸 INSERT + 冲突按 value 分流

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（9/9 done_criteria 通过，逐条有针对性变异证据）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `e4aa5034075c5d08f4e9ce0873e3b128d6a374e7`（master）
- **verify_baseline 核对**：`head` 与 `discovery_sha256`（`86915b304fbed88cf1f3ee399ed011751b30be5eee9c9c234fc5e6fdac1618eb`）均与记录值一致。**零漂移**。

> 🔴 **§6 是运维必须读的一节**：本任务把一条**静默覆盖**的路径变成了**会中断采集的响亮失败**。行为是对的，但现场表现会变。

---

## 1. 亲跑结果

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `GOTOOLCHAIN=local go build ./...` | 0 | 通过 |
| `GOTOOLCHAIN=local go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | 两包均 `ok` |
| `go test -cover -coverpkg=./internal/crisis ./internal/crisis` | 0 | **93.7%**（`dev_minimum` 80） |
| `go vet ./internal/crisis ./cmd/atlas` | 0 | 通过 |
| 变异对照组 / 收尾对照 | 0 | **551 PASS / 0 FAIL**（两次同值） |

⚠️ `gofmt -l` 报了 `cmd/atlas/crisis_test.go`。**查实为既存**：差异在第 1044 行（注释对齐），基线 `46c401c` 上同样未格式化，与本次改动的 582–597 行无关。不计入判定。

## 2. 越界申报 —— 时序、必要性、充分性三项独立核实

改动范围 `git diff --numstat 46c401c..e4aa503`：`store.go 72/11`、`store_test.go 199/11`、**`cmd/atlas/crisis_test.go 6/1`**。三者都在 `writes` 内。

### 2.1 时序：`writes` 确在 `dev_done` 之前补齐

**看审计行而非快照**——快照只答「此刻是什么」，答不了「几时补的」：

```
{"op":"update","by":"dev-m4c-a","at":"2026-09-19T01:17:13Z","keys":["writes"],
 "changes":[{"key":"writes","added":["./cmd/atlas/crisis_test.go"],"removed":[]}]}
{"from":"in_progress","to":"dev_done","by":"dev-m4c-a","at":"2026-09-19T01:30:08Z"}
```

⇒ 补齐早于交棒 **12 分 55 秒** ✓ 符合「dev 必须在 `transition dev_done` 之前自己落盘」。

### 2.2 必要性：不只看「红了几条」，要看**红的理由**

实现全在、唯独 `cmd/atlas/crisis_test.go` 退回 `46c401c` ⇒ `./cmd/atlas` **5 FAIL**（与 dev 自报一致）。但「5 条红」只说明有影响，故我取了失败文案：

```
crisis: nfci/2026-07-08 already has a different value for fetched_at
2026-07-11T00:00:00.000000000Z (have -0.5, got 0.2):
the same fetch reported two values; refusing to overwrite
```

⇒ 红的理由**正是 C6 本身**。`seedReplayWatch` 原本就在**同一个 `fetched_at` 下改写同一 `(ts, indicator)` 的值**——那恰是新写语义要拦的情形。**改夹具是让夹具停止做 C6 禁止的事，不是绕过守卫**；相对的选项（放宽写路径）会把守卫本身拆掉。这个判断成立。

### 2.3 充分性：实质改动只有一行

交付态 `./cmd/atlas` 全绿；`cmd/atlas/` 的实质 diff（剔除注释）**只有一行**：

```
-  Source: "test", FetchedAt: "2026-07-11T00:00:00.000000000Z"
+  Source: "test", FetchedAt: "2026-07-12T00:00:00.000000000Z"
```

> ⚠️ **方法学如实记**：我第一次跑充分性用的是 `git checkout -- <file>` 还原，得到 FAIL。那是**我的取证错误**——前一步 `git checkout 46c401c -- <file>` 已把旧版**暂存进 index**，而 `git checkout -- <file>` 是从 **index** 还原，拿到的仍是旧版。必须写 `git checkout <sha> -- <file>`。若我就此报「充分性不成立」，那会是一条由**我的还原方式**制造的假缺陷。同 TASK-004 §5 的取样误差，是同一族：**先确认探针打中了目标，再解读它的输出。**

## 3. done_criteria 覆盖矩阵（9/9，逐条有针对性变异）

| # | 完成标准（摘要） | 对应测试 | **针对性**变异 | 判定 |
| --- | --- | --- | --- | --- |
| functional[0] | 追加而非覆盖：裸表 2 行，`Observation` 看到 11 | `TestUpsertAppendsNewRevision` | **S1** 冲突检测忽略 `fetched_at` → 红 | **PASS** |
| functional[1] | 同三段同值幂等 | `TestUpsertSameTripleSameValueIsNoop` | **S2** 从不吞掉（同值也报错）→ 红 | **PASS** |
| functional[2] | 同三段异值响亮失败，文案含 indicator + ts，整批回滚 | `TestUpsertSameTripleDifferentValueFails` | **R5** 分流改坏 → 红；**R6** 不回滚 → 红 | **PASS** |
| functional[3] | 源码钉死：不得出现 `INSERT OR REPLACE/IGNORE` | `TestWritePathHasNoReplaceOrIgnore` | **R4** 改回 `OR REPLACE` → 红 | **PASS** |
| boundary[0]a | 裸 INSERT 造 NULL 前置行，再写非 NULL ⇒ 报错 | `TestUpsertNullVersusNonNullConflicts` | **R6** → 红 | **PASS** |
| boundary[0]b | 两边都 NULL 判「相同」，**直接调用比对函数** | `TestSameObservationValueNullSemantics` | **R7** 两边 NULL 判不同 → 红；**R5** → 红 | **PASS** |
| boundary[1] | 批量中途冲突 ⇒ 整批回滚，行数与写前一致 | `TestUpsertRollsBackWholeBatch` | **R5 / R6** → 红 | **PASS** |
| error_handling[0] | 空切片是 no-op，不报错 | `TestUpsertEmptySliceIsNoop` | **S4** 空切片改成报错 → 红（见注） | **PASS** |
| non_functional[0] | 既有契约改写，**不得靠放宽写路径变绿** | `TestStoreUpsertIdempotentAndWindows`（已改写）+ 契约注释 | **R5 / R6** → 红；**R4** 守着「不得改回 OR REPLACE」 | **PASS**（§5） |

> **注（S3 存活不是缺陷）**：`S3`（删掉空切片早返回）零变红。DoD `error_handling[0]` 明文允许两种实现——「不开事务也不报错（**或开事务立即提交**）」，所以两者都该通过；该断言钉的是**可观察契约**（不报错）而非实现选择。**S4** 证明它非空洞。

**实现要点核对**（`store.go`）：`defer tx.Rollback()` + 冲突时直接 `return` ⇒ 整批回滚；`prev *float64` 承接可空列；`sameObservationValue` 的 `a == nil && b == nil` 处理「两边都 NULL 判相同」（SQL 里 `NULL != NULL`，不特判会判成异值）。

## 4. Leader 指定的两项抽验

### 4.1 X1 的限定成立（dev 主动不笼统报 4/4）

dev 自报「改回 `OR REPLACE` 只被源码钉死那条抓住、行为测试没红（因为只改 INSERT 语句时分流逻辑还在）」。

| **R4** 只把 `INSERT INTO` 改成 `INSERT OR REPLACE INTO`（分流逻辑保留） | **只有 `TestWritePathHasNoReplaceOrIgnore` 红** |
| --- | --- |
| **R5** 把分流逻辑改坏（异值也当同值吞掉） | **4 条行为测试红**：`TestUpsertSameTripleDifferentValueFails` / `TestUpsertRollsBackWholeBatch` / `TestStoreUpsertIdempotentAndWindows` / `TestSameObservationValueNullSemantics` |

⇒ **限定属实**，且 X2 确实存在——分流逻辑有 4 条行为断言在守。dev 主动划出这条限定而不报 4/4，是诚实的自我设限。

### 4.2 豁免清单升级：两个方向都钉住

清单从「整文件前缀放行」升级为 **`(文件, SQL 片段)` 逐处匹配**（`allowedBare []bareRead`）。两个方向各打一次：

| **R1** 同文件（`store.go`）新增一处**不同**的裸读 | 守卫**红**（匹配不上 `sqlFragment`） |
| --- | --- |
| **R2** 同文件新增一处**相同**的裸读（SQL 片段一致，只超总数） | 守卫**红**（`assert.Len` 总数超标） |

⇒ **两半都成立**。R2 这一半最容易漏——若只做前缀匹配而不校总数，复制粘贴一处已豁免的裸读就会静默通过。

## 5. 既有契约改写 —— 方向正确，未放宽写路径

`non_functional[0]` 明令「**严禁**改回 `OR REPLACE`/`OR IGNORE` 或在写路径加容忍来让它变绿」。核实：

- `store_test.go:6` 的契约注释已同步改写：由「同 `(ts,indicator)` 重复 upsert **覆盖而非报错**」改为「同三段主键且 value 相同的重复 upsert 被吞掉；**异值则报错**（TASK-005/C6 契约变更）」。
- `TestStoreUpsertIdempotentAndWindows` 改为：同三段异值 ⇒ `require.Error`，断言文案含 `IndVIX` 与 `2026-07-03`，并断言**整批回滚后旧值 17 不变**（原断言是 `18.0`，即期望覆盖成功）。
- 写路径没有任何容忍分支；`TestWritePathHasNoReplaceOrIgnore`（经 R4 证明有牙）在源码层钉住这一条。

⇒ 改写方向**符合 DoD 明令**，不是靠放宽实现变绿。

## 6. 🔴 生产故障模式变化（运维必读）

Leader 的口径经我独立核实**属实**，并补两点细化：

| 事实 | 核实方式 |
| --- | --- |
| `fetched_at` 生产来源是 `NowStamp(now())`，**纳秒精度** | `dates.go:13` + `timeLayout = "2006-01-02T15:04:05.000000000Z07:00"` |
| **一次运行内所有行共用一个 stamp** | `ingest.go:60` `stamp := NowStamp(ig.now())` 取一次后传给全部 ingestor（`ingestFredSeries` 的 `stamp` 是入参） |
| 各源指标互不重叠 | `fredDirect`（vix/hy_oas/t10y2y）、nfci 单独、spread 派生 sofr_effr、yahoo（move/usdjpy） |

⇒ **跨运行重跑是追加**（不同纳秒 stamp ⇒ 不同主键），**不会误报**。
⇒ **但同一次运行内**，若上游对同一 `(日期, 指标)` 返回两个不同值，新实现会整批报错。

**补的细化（Leader 未提、我查的）——中断范围比「整批」更大**：调用链是 `ingestFredSeries → ig.upsert → UpsertObservations`，错误上抛后 `IngestAll` 立即 `return nil, err`。而 **vix 在 `fredDirect` 首位** ⇒ 它一炸，**该轮后续指标（含 yahoo 那两个）都不再抓取**，不是只丢一个指标的批次。

**这是对的行为**（C6 的全部目的就是暴露「同一次取回给出两个值」），但它把一个**静默覆盖**换成了**会中断当轮采集的响亮失败**——属交付内容的一部分，运维需要提前知道，否则第一次触发时会被当成采集器故障。

## 7. 🔴 一条覆盖缺口：写路径裸读的**行为**必要性无测试守护

`store.go` 的冲突检测查的是**基表**：

```go
`SELECT value FROM macro_observations WHERE ts = ? AND indicator = ? AND fetched_at = ?`
```

守卫的豁免注释给的理由是「视图按定义只有当前行，据它判『不存在』而插入会直接撞主键」。**我验了这个理由，它成立**——但发现**没有行为测试在守它**。

**R3（把冲突检测改走 `v_macro_current`）⇒ 只有 `TestNoBareTableReadsOutsideMigration` 变红，零行为测试变红。**

为区分「不坏」与「没测到」，我写了探针直接观察：

| 场景：先写 10@07-14 → 再写 11@08-14 → **重写更旧的 10@07-14**（采集器补跑历史） | 结果 |
| --- | --- |
| **交付态**（冲突检测走基表） | `err = nil`，正确吞掉，裸表 2 行 ✓ |
| **变异态**（冲突检测走视图） | **`UNIQUE constraint failed: macro_observations.ts, macro_observations.indicator, macro_observations.fetched_at (1555)`** |

⇒ 理由为真，缺陷为真，**而触发它需要「更新修订已存在时重跑更旧批次」这个场景，现有测试无一覆盖**。

**这不是 DoD 缺口**（DoD 未要求这条），也不影响裁决。但它是一条真实风险，理由有二：

1. 补跑历史是**真实生产场景**（`cmd/atlas/crisis.go:182` 的 `manual_backfill` 路径就是）。
2. **TASK-006 正要把 `obsSelect` 改成 `obsFrom()`**——dev 的移交里明写「那处写路径裸读不能跟着改走视图」。目前拦住它的只有源码守卫，而守卫报的是「豁免清单对不上」这个**记账理由**，不是行为理由。读到那条失败信息的人未必明白为什么不能改。

**建议**：在 TASK-006 或收尾任务里补一条行为断言——「更新修订已存在时，重写更旧的修订必须被吞掉而非撞主键」。它在 R3 下会变红，正好给那处裸读一个行为层的守卫。

## 8. 结论

9 条 done_criteria 全部通过，**每条都有针对性变异证据**（11 个变异 + 2 个探针，全部通过有效性闸 `vet=0`；对照/收尾均 551 PASS）。越界申报的时序、必要性（含**红的理由**）、充分性三项独立复核均成立。Leader 指定的两项抽验（X1 限定、豁免清单两方向）结论均为**属实**。既有契约按 DoD 明令方向改写，未放宽写路径。生产故障模式的变化经核实并补了中断范围的细化。

**裁决：VERIFIED。**

**移交 Leader 三件事**：
1. **§6 的生产行为变化**建议进 final-report 的运维须知：中断范围是**整轮采集**而非单个指标批次。
2. **§7 的覆盖缺口**建议并入 TASK-006：补一条「重写更旧修订被吞掉」的行为断言，给写路径裸读一个行为层守卫。
3. `cmd/atlas/crisis_test.go` 的 **既存** gofmt 问题（第 1044 行）——非本任务引入，可并入既有的「范围外待办」。

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 e4aa5034075c5d08f4e9ce0873e3b128d6a374e7
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas
GOTOOLCHAIN=local go test -count=1 -cover -coverpkg=./internal/crisis ./internal/crisis

# 越界的必要性实验（⚠️ 还原必须用 `git checkout <sha> --`，不可用 `git checkout --`）
git worktree add --detach ../wt-verify-TASK-005 e4aa5034075c5d08f4e9ce0873e3b128d6a374e7
cd ../wt-verify-TASK-005
git checkout 46c401cf15df8c9922c5023c823d8182d2b3707e -- cmd/atlas/crisis_test.go
GOTOOLCHAIN=local go test -count=1 ./cmd/atlas        # 期望 5 FAIL，文案为 C6
git checkout e4aa5034075c5d08f4e9ce0873e3b128d6a374e7 -- cmd/atlas/crisis_test.go
GOTOOLCHAIN=local go test -count=1 ./cmd/atlas        # 期望全绿

# 变异
python3 <scratchpad>/test-m4c-a-T005-mutate.py   # R1-R7
python3 <scratchpad>/t005b.py                    # S1-S3
git worktree remove --force ../wt-verify-TASK-005
```
