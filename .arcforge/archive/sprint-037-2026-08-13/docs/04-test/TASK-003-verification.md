# TASK-003 验证报告 —— `Store.HasArticle`（一级幂等键，查两张表）

- **验证者**：test-agent-26
- **被验交付**：dev-agent-54，提交 `2e93115 feat(hestia): Store.HasArticle 一级幂等键，查两张表`
- **验证基线**：`verify_baseline.head = 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf` = 承接时 HEAD ⇒ **无漂移，未用 `--ack-drift`**
- **assignment_epoch**：1（承接时记下，出裁决时携带）
- **结论**：**VERIFIED**（6/6 条 done_criteria 全部 PASS，无未覆盖项）

---

## 0. 承接核实（判定之前先确认判定对象对不对）

| 核项 | 命令/依据 | 结果 |
|---|---|---|
| 验证对象漂移 | `verify_baseline.head` vs `git rev-parse HEAD` | `67249ffb…` == `67249ffb…` ✅ 无漂移 |
| **DoD 未被改写** | `jq -S -c '.done_criteria' \| shasum -a 256` | `3f65bb6f5840d146` == wave1 开工前预读基线 ✅ **未变** |
| 方案 C 时序 | `git merge-base --is-ancestor 2e93115 HEAD` | YES ✅ 代码确已在 HEAD 上 |
| 实际改动 vs `writes` 声明 | `git diff --stat 63ac5b6 2e93115` | `store.go` + `store_test.go`，与 `writes` **逐字一致**，无越界 ✅ |
| discovery 存在 | `jq 'has("discovery")'` | true ✅ |

**关于 `dev_done` 时写通道打的两组 WARN**：经核，全部指向 2026-06/07 跨 Sprint 复用同号 TASK-003 的历史提交
（`cmd/atlas/executor.go`、`internal/router/` 等），写通道自身已判定「早于本轮开工时刻，不参与范围漂移判定」。
本轮实际改动只有上述两文件。**⚠️ 这组 WARN 在任务号跨 Sprint 复用时会恒定出现，照它去查会查到一批六月的无关提交。**

---

## 1. 完成标准覆盖矩阵

| # | done_criteria | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | `HasArticle(ctx, articleID) (bool, error)`，**同时查** `TableObservations` 与 `TablePending`，任一命中即 true；真库测试 | `TestHasArticle/观测表里的命中`、`TestHasArticle/pending_里的也命中`（均 `newTestStore` 真库） | **PASS** |
| functional[1] | 三条各一测试：只在 observations→true；只在 pending→true；都没有→false | `TestHasArticle` 的三个子测试（实跑三条全绿） | **PASS** |
| boundary[0] | 空 `articleID` 行为钉住，**且在 discovery 里说明理由** | `TestHasArticleOnEmptyID`（`(false, nil)`）+ `discovery.decisions[0]`（三条理由，按承重排序） | **PASS** |
| error_handling[0] | 查库失败用 `%w` 包住底层错误并带上下文；用 `require.NotNil(t, errors.Unwrap(err))`，**不得**用 `NotErrorIs` | `TestHasArticleWrapsQueryError`：`ErrorIs(context.Canceled)` + `ErrorContains("art-2025-12")` + **`require.NotNil(t, errors.Unwrap(err))`**；全文件无 `NotErrorIs` | **PASS** |
| non_functional[0] | 两条导出面守卫**登记**（非放宽），`assert.Equal` 一字不动只加一项；**正向自证**：删掉新增项确认转红 | reflect 版 5→6、AST 版 12→13，均仍是 `assert.Equal` 精确集合相等；消融 A1/A2 **我已独立复现** | **PASS** |
| non_functional[1] | `gofmt`/`vet` 空、整包 `-count=1` 全绿、`-race` 绿、覆盖率 ≥ 93.2% | 实测见 §2 | **PASS** |

**无未覆盖项。** `functional[1]` 之外 dev 另加了计划要求的 `TestHasArticleSeesSupersededRows`（查全表而非当前行视图），
是 DoD 未要求的加分项，其不可替代性由消融 A3 实证（见 §3）。

---

## 2. 实跑证据（主工作区，`67249ffb…`）

```
gofmt -l internal/hestia/          → 空
GOTOOLCHAIN=local go vet ./internal/hestia/ → 空
GOTOOLCHAIN=local go build ./...   → OK
go test ./internal/hestia/ -count=1 -cover  → ok  coverage: 93.3% of statements   (门槛 93.2% ✅)
go test ./internal/hestia/ -count=1 -race   → ok  3.829s                          ✅
go tool cover -func | grep HasArticle → store.go:280: HasArticle  100.0%          ✅
```

六条本任务测试全部 `--- PASS:`（只读 `--- PASS:` 行，不读整体退出码）：

```
--- PASS: TestStoreExposesNoWriteMethods      --- PASS: TestHasArticleSeesSupersededRows
--- PASS: TestPackageExposesNoWriteFunctions  --- PASS: TestHasArticleOnEmptyID
--- PASS: TestHasArticle                      --- PASS: TestHasArticleWrapsQueryError
    --- PASS: TestHasArticle/观测表里的命中
    --- PASS: TestHasArticle/pending_里的也命中
    --- PASS: TestHasArticle/没见过的返回_false
```

### ⚠️ 计数口径（三个数都对，别对不上账）

| 数 | 出处 | 口径 | 树 |
|---|---|---|---|
| **584** | dev-54 的 discovery | 全部 PASS（含子测试） | `task/TASK-003` @ 基于 `63ac5b6` |
| **614** | 本报告 | 全部 PASS（含子测试） | `67249ffb…`（含已合入的 TASK-001/002） |
| **282** | 本报告 / leader | **顶层** PASS（`^--- PASS:` 无缩进） | `67249ffb…` |

`614 − 584 = 30` 全部来自并行 wave 已合入的 TASK-001/002，**不是漂移，是并行 wave 的正常结果**。
FAIL = 0。**本报告的判定锚定在 `67249ffb…`。**

---

## 3. 消融独立复现（我自己重跑了全部六条，不是抽查）

**方法**：`git worktree add --detach ../wt-verify-TASK-003 67249ffb…` 隔离副本内变异，主工作区一个字节不碰；
每个变异体先打印 diff 逐字核对、设编译错误闸，跑完 `git checkout --` 还原。

| # | 变异 | dev 声称 | **我实测** | 外溢 |
|---|---|---|---|---|
| A1 | AST 版期望切片删 `"Store.HasArticle"` | 红在 :420，reflect 版仍绿 | 红在 **:420**；**reflect 版 = PASS** ✅ | 281+1=282 ✅ |
| A2 | reflect 版期望切片删 `"HasArticle"` | 红在 :375，AST 版仍绿 | 红在 **:375**；**AST 版 = PASS** ✅ | 281+1=282 ✅ |
| A3 | 实现改查 `viewCurrent` | **只有** SeesSupersededRows 红在 :1980 | 只有 `TestHasArticleSeesSupersededRows` 红在 **:1980** ✅ | 281+1=282 ✅ |
| A4 | 去掉 UNION ALL 的 pending 半边（同步去实参） | **只有** pending 子测试红在 :1942 | 只有 `TestHasArticle/pending_里的也命中` 红在 **:1942**；**无编译错误** ✅ | 281+1=282 ✅ |
| A5 | `%w` → `%v` | 红在 :2042(ErrorIs) 与 :2044(NotNil Unwrap) | 红在 **:2042 与 :2044** ✅ | 281+1=282 ✅ |
| A6 | 去掉两处 `WHERE article_id = ?` 谓词（改恒真） | 红在 :1951 与 :2017 | 红在 **:1951**（没见过的返回 false）与 **:2017**（OnEmptyID）✅ | 280+2=282 ✅ |

**六条全部逐字复现，无一处对不上。** 三点重点核实（leader 点名）：

1. **A1/A2 的「另一条仍绿」是真的** —— 这是「两条守卫互补不可互替」这个结论的**全部依据**，我实测确认：
   A1 时 reflect 版 PASS / AST 版 FAIL，A2 时 **恰好反过来**。互补性成立。
2. **A4 语义纯净** —— harness 设了编译错误闸（`cannot use` / `too many arguments` / `undefined` / `build failed`），
   **未触发**；FAIL 恰好只有目标子测试。dev-54 自述第一版曾以「实参多一个」的错误理由致红并作废重做——
   **那一步无人能验证它做没做，但它修正后的结论我独立复现了，语义污染确实不存在**。
3. **A6 证明空 ID 那条不是空断言** —— `TestHasArticleOnEmptyID` 先填满两张表再问空串，
   所以恒真谓词的实现会让它红在 :2017。若它在空库上问，任何实现都返回 false ⇒ 平凡为真。

**外溢度全部为 0**：基线 282 顶层 PASS，六个变异体的 `PASS + FAIL` 恒等于 282，无一条无关测试被牵连。

**卫生自证**：变异前后主工作区 `store.go`/`store_test.go` 的 sha256 逐字节一致
（`a04f31e8…` / `028a7a19…`，与 dev-54 报的值同）；`git status --porcelain internal/hestia/` 空；worktree 已 `git worktree remove`（谁建谁拆）。

---

## 4. 判定

**VERIFIED**。6/6 条 done_criteria 全部 PASS，无未覆盖项，无缺陷。

实现与测试的质量高于门槛：注释把「为什么含 pending」「为什么查全表而非视图」「为什么空串返回 false 而非报错」
的**理由**都写进了代码，而不只是写了行为；`TestHasArticleOnEmptyID` 主动避开了平凡为真的写法；
`%w` 自证用了 `require.NotNil(t, errors.Unwrap(err))` 而不是跨 Sprint 存活过一轮的 `NotErrorIs` 写法。

---

## 5. DoD 之外的观察（**按规矩不据此判定**，攒给下游）

以下三条 DoD 均未要求，**不影响本次 VERIFIED**，登记供 TASK-005/006 与后续 Sprint 参考。

### O1（对 TASK-005/006 有直接约束）Duplicate 的「什么都没写」只活在 `Verdict` 里

dev-54 主动交出的精确化，我核实属实：`--force` 重抓一个已在观测表且 `published_at` 未变的期次时，
`Save` 走 `Duplicate` 分支 —— **不插新行、只 `refreshArticleID`、返回 `Outcome{Verdict: Duplicate}` 与 `nil` error，
且 `Table` 仍是 `TableObservations`**（`store.go:431-439`）。

⇒ **ingest 报告必须透传 `Verdict`**。只看 `err == nil` 和 `Table` 的话，**一次 Duplicate 和一次真正的新增在日志里长得一模一样**。

### O2 `SeesSupersededRows` 覆盖的是 Revision 路径，**不覆盖** Duplicate 路径 —— 两者方向相反

这一条容易被误读，特此写清（dev-54 已诚实声明盲区，我确认其描述准确并补上区分）：

| 路径 | 行为 | 旧 `article_id` 还命中 `HasArticle` 吗 | 有测试吗 |
|---|---|---|---|
| **Revision**（`published_at` 变了） | `insert` **新增行**，旧行保留 | **仍命中** ✅ | `TestHasArticleSeesSupersededRows` |
| **Duplicate**（`published_at` 没变） | `refreshArticleID` **原地 UPDATE 覆盖** `article_id`（`store.go:462-466`） | **不再命中** ⚠️ | 无（已知盲区） |

看到「旧 article_id 仍命中」的测试存在，很容易以为**所有**旧 id 都仍命中——**并非如此**。
dev-54 的判断（若下游要「见过的 id 永远见过」，需要的是一张 article_id 历史表，而不是改 `HasArticle`）我认同：
在 `HasArticle` 里补救会与 `refreshArticleID` 的「一行只记最后一次看到的 URL」语义打架。

### O3 `article_id` 上没有索引（当前规模下**无害**，仅登记）

`schema.go` 两张表的主键都是 `(period, period_type, published_at[, ingested_at])`，`article_id` 不在主键内、也无
`CREATE INDEX` ⇒ `HasArticle` 的两处 `WHERE article_id = ?` 都是全表扫描。

**这不是缺陷**：央行报告约 15 期/年（12 月报 + 2 半年报 + 1 年报）加修订行，十年也只有数百行，
SQLite 全表扫描是微秒级，而 ingest 一天调用三次。**按可预见的数据规模，加索引没有收益。**
登记的唯一理由是：将来若把这张表用于更高频的数据源，这里是第一个该看的地方。

---

## 6. 我用到的命令（复现用，锚已钉全 sha）

```bash
git rev-parse HEAD                                  # 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf
git merge-base --is-ancestor 2e93115 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf
git diff --stat 63ac5b628d8375e7f15835920c3a817292ddae1b 2e93115 -- internal/hestia/
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -v      # 顶层 PASS 计数取 '^--- PASS:'
git worktree add --detach ../wt-verify-TASK-003 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf
```
