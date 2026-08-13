# TASK-006 验证报告 —— T5：status 的查询与渲染

- **验证者**：test-agent-26 ｜ **被验交付**：dev-agent-53，交付锚 `2ce68cd1ca46c3f652106d2444f44d786fdfddf5`
- **验证基线**：`verify_baseline.head = 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9` = 承接时 HEAD ⇒ **无漂移**
- **assignment_epoch**：1 ｜ **结论**：**VERIFIED**（7/7 条 done_criteria 全部 PASS）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| 验证对象漂移 | `verify_baseline.head` == 当前 HEAD ✅ |
| **DoD 未被改写** | `transitions.jsonl` 中本任务的 `update` 只动过 `questions`（leader 答复澄清），**`done_criteria` 从未被 update** ✅ |
| discovery | 文件存在（14914 B）；**任务文件的 `discovery` 字段原本缺失，由我补上**（见 §5） |

### `store_test.go` 与交付锚不一致 —— 已核实**不是** scope 漂移

master 上 `store_test.go` 是 `d4e5c78c`，dev-53 交付时是 `01730c3b`。逐行 diff 后确认**唯一差异**是 AST 版期望列表加了 `"Ingest"` 一项（leader 为已停机的 dev-54 代做的越流程介入，记录在 H10）。

**其余三个文件与交付锚逐字节一致**：`status.go` `bc319088`、`status_test.go` `2100d26e`、`store.go` `922be862` ⇒ **dev-53 的 scope 干净**。

---

## 1. 完成标准覆盖矩阵（7 条）

| # | done_criteria | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | `StatusRow`/`PendingRow` 两类型、`RecentObservations`/`RecentPending` 两查询、`RenderStatus` 渲染；真库测试 | 五者均交付；`TestRecentObservations`/`TestRecentPendingExtractsFailedChecks`/`TestRenderStatus` 等 16 条，全部走 `newTestStore` 真库 | **PASS** |
| functional[1] | `RecentObservations` 查 **`viewCurrent`**（不是 `TableObservations`）；`RecentPending` 查 `TablePending` 且带上失败的 `[]Check` | 实现逐字符合；`TestRecentObservationsShowsCurrentOnly` 钉住视图口径（消融 M3 证明其鉴别力）；`TestRecentPendingKeepsOnlyFailedChecks` 钉住只收 `CheckFailed`（消融 M4） | **PASS** |
| boundary[0] | **空库上必须跑通并输出可读内容**（不是 panic、不是空白） | `TestStatusOnEmptyStore`：真库空库跑通，断言输出含 `observations: 0`、`pending: 0` 与库路径；另有 `TestRecentQueriesOnEmptyStore` | **PASS** |
| boundary[1] | `n` 为 **0 与负数**各一条测试 + discovery 说明；**判据不对称，别只测 0** | `TestRecentObservationsGuardsNonPositiveN` / `TestRecentPendingGuardsNonPositiveN`，各含 n=0、n=-1 两条**加 `rawLimitCount` 裸 SQL 对照**；discovery 有说明 | **PASS**（见 §3 重点） |
| error_handling[0] | 查库失败 `%w` 包住带上下文，用 `require.NotNil(errors.Unwrap(err))` 自证 | 四条错误路径各一测试：`WrapsQueryError`×2、`WrapsBadReportJSON`、`WrapsScanError`，**全部用 `require.NotNil(t, errors.Unwrap(err))`**；实现六处 `fmt.Errorf` 全带 `%w` | **PASS** |
| non_functional[0] | status 必须打印**解析后的绝对路径**，测试用 `filepath.IsAbs` 钉住 | `RenderStatus` 内 `filepath.Abs(dbPath)`（解析发生在此处，符合 D7 裁定）；`TestRenderStatusPrintsAbsoluteDBPath` 三条断言（见 §3） | **PASS** |
| non_functional[1] | 新增五个导出物登记进导出面守卫 + 正向自证；`gofmt`/`vet` 空、整包全绿、覆盖率 ≥93.2% | 两条守卫均登记（`assert.Equal` 精确集合相等未变）；正向自证由消融 **M6/M7 分别验过两条守卫**；实测见 §2 | **PASS** |

---

## 2. 实跑证据（隔离 worktree @ `0e2c6fc`）

```
gofmt -l → 空    go vet → 空    go build ./... → OK
go test -count=1 -cover → ok  coverage: 93.5%   (门槛 93.2% ✅)
go test -count=1 -race  → ok
顶层 PASS 308 / 全部 PASS 659 / FAIL 0
go tool cover -func → RenderStatus 92.9% / RecentObservations 87.5% / RecentPending 95.7%
```

本任务 16 条测试全部 `--- PASS:`。

> **覆盖率口径**：dev-53 报的是 **93.284%（875/938）**，测自它的交付锚 `2ce68cd1`；我测的 **93.5%** 是 `0e2c6fc`（含后续合入的 TASK-010 与 Ingest）。**两个数都对，测的树不同。** 它明说了「只按计划实现是 93.070%、低于下限」——这句诚实交代很重要，见 §3。

---

## 3. 三处重点核实（leader 点名）

### ① `rawLimitCount` 对照是真的 —— 我独立复现了

**删掉 `RecentObservations` 的 `if n <= 0` 守卫**后实测：

```
--- FAIL: TestRecentObservationsGuardsNonPositiveN
    store_test.go:2253  Messages: n=-1 也返回空 —— 不得漏给 LIMIT 变成「整张表」
顶层 PASS=307 FAIL=1（基线 308，外溢 0）
```

⇒ **只有 `:2253`（n=-1）红，`:2249`（n=0）照样绿。** 这精确证实了 reviewer D5 的「判据不对称」：
只测 n=0 的话，未加守卫的实现**照样全绿**。

而 `rawLimitCount` 那两条对照把这件事变成了**测试内部可观察的事实**，而不是注释里的一句话：

- `assert.Equal(3, rawLimitCount(viewCurrent, -1))` ⇒ 裸 `LIMIT -1` 确实拿到整张表 ⇒ **上面 n=-1 的空是守卫挡出来的，不是库本来就空**
- `assert.Equal(0, rawLimitCount(viewCurrent, 0))` ⇒ 裸 `LIMIT 0` 本就零行 ⇒ **只测 n=0 分辨不出守卫在不在**

第二条断言本身就是「为什么必须测 n=-1」的证明。**对照真实、承重、且用 `s.DB()` 走同一条连接同一份数据**（注释说明了为什么不换连接）。

### ② 补的两条测试不是凑断言 —— 确认

dev-53 明说「直接动机是覆盖率必须回到 93.2% 以上」（不掩饰动机，这点值得记）。我独立核了两条的路径与后果：

- **`TestRenderStatusReportsWriteError`**：`failingWriter{failAt:1}` 触发真实的 `io.Writer` 写失败；断言错误被上报且**带出底层信息**（`disk full`）而非自造。后果真实：写失败静默会让调用方以为报告发出去了。
- **`TestRecentPendingWrapsScanError`**：用 `rawDB` 把 pending 表换成无 `NOT NULL` 约束的同名表再写 `NULL period`，触发 `rows.Scan` 的 `converting NULL to string is unsupported`。**有前置锚点**（`require.NoError(..., "前置条件：NULL period 那一行必须真的写进去了")`），断言含 `require.NotNil(errors.Unwrap(err))`。后果真实且注释说明了：库文件是外部对象（Grafana / 手工 sqlite3 / 旧版本代码都可能留下这种行），而 status 是运维在库出问题时第一个跑的命令。

⇒ **两条都是有真实后果的错误路径，不是凑断言。**

### ③ 绝对路径那条的断言独立性

`TestRenderStatusPrintsAbsoluteDBPath` 有三条断言，而它**自己指出**第一条 `Contains(want)` 把判据绑在当前 cwd 上，因此又加了两条**不依赖 `want` 怎么算出来**的：直接从输出解析出路径段做 `filepath.IsAbs`，以及 `NotEqual("data/hestia.db")` 排除原样打印。⇒ 断言独立性是有意识设计的，不是三重保险。

---

## 4. ⚠️ 一处记录不一致（不影响判定，但要记下）

dev-53 的 discovery 里 M1 写「**具体断言**：`store_test.go:2232` `n=-1 也返回空`」。

**实测该行号有误**：

| | 结果 |
|---|---|
| 交付版 `2ce68cd1` 上 `n=-1 也返回空` 的行号 | **2253** |
| master `0e2c6fc` 上 | **2253**（两版 `store_test.go` 均 2401 行，该处未变） |
| **交付版第 2232 行实际是什么** | 一个 `//` 注释行 |

**结论本身正确，我已独立复现**（红的确实是 `n=-1` 那条、`n=0` 仍绿）。**不据此 reject** —— DoD 未要求行号、结论无误。

但值得记一句：**行号是「我真的读了失败输出」的唯一凭证**。归档后若有人按 `2232` 去查，会落到一个注释行上。这与「自证数字必须在最后一次改动之后统一重采」是同一族——只是这里文件根本没变，纯属记录环节的偏差。

---

## 5. 我补的字段（申报，非静默修复）

承接时 `jq 'has("discovery")'` 返回 **false**（discovery **文件**在，14914 B，但任务文件缺 `discovery` 字段）。
validator 的 `missing-discovery` 查的是**字段**且在 `verified` 时**退出码 1 阻断**（我上一轮已实测确认）。故在转 `verified` 前经写通道补上并 jq 直读核实。

**⚠️ 这是本 Sprint 第 3 例**（TASK-001/002/006/010 四个缺、只有 TASK-003 不缺）—— 频次见给 leader 的消息。

---

## 6. 我确认属实的两条「已知缺口」（dev-53 主动登记，不影响判定）

- **① `RenderStatus` 只检查首次写入错误**：`status.go` 里只有第一条 `fmt.Fprintf` 检查了返回值，其余七处忽略 ⇒ `status | head -1` 时后续写失败被静默吞掉。**属实。** dev-53 的取舍理由（计划原样、逐个检查会引入一批未覆盖分支反而拉低覆盖率）合理，且它按「已知缺口登记、不静默」处理。
- **② `store.go:304` `Preceding` 的注释写反了**：原文「不挡住非正数，一次 **n=0** 的调用会把整个序列拉回来」——**错**，`LIMIT 0` 是零行，拉回全序列的是 `LIMIT -1`。**守卫本身是对的**（`if n <= 0` 两者都挡），错的是理由。**属实。**
  ⚠️ 这条值得单独说：**它正好把本任务花大力气钉住的那个不对称说反了**。读注释的人会得出「n=0 危险、n=-1 安全」这个恰好相反的结论。**结论对而理由错的注释不会被任何测试抓到**——它只在有人照着它推理时才伤人。建议后续任务顺手改（`store.go` 不在本任务应改范围内的判断我认可，dev-53 选择登记而不扩范围是对的）。

---

## 7. DoD 之外的观察

**O1 —— 断言 message 里的数字无人守（leader 已注意到，我把形状核清楚了）**：AST 版守卫的 message 写「必须恰好是这**十七**个」，而期望列表的项数是另一处事实。**dev-53 交付时列表 16 项、文案写「十七个」——不一致，且没有任何东西会红。** leader 后来加了 `"Ingest"` 使列表变成 17 项，文案**碰巧变对了**。
⇒ **它现在正确是副作用，不是修复；守卫缺口仍在。** 下次加导出物时同样的不一致会再次发生。若要闭合，得让 message 由 `len(want)` 生成而不是手写数字。

**O2** `RecentPending` 明确不按 `ingested_at` 排序（注释说明了 RFC3339Nano 的字典序与时间序相反）⇒ 同一期的多次尝试**之间的先后不保证**。status 的读者若据此判断「最后一次尝试是哪次」会出错。当前 DoD 未要求，登记备查。

---

## 8. 复现命令（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-w2 0e2c6fc976d90382ecb2122dbeb1ed18eaf8c9c9
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover        # 93.5%
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
# 复核 dev-53 交付：git show 2ce68cd1ca46c3f652106d2444f44d786fdfddf5:internal/hestia/<file>
```
