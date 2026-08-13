# TASK-007 验证报告 —— T4：一级键、`--force` 与错误路径的行为守卫

- **验证者**：test-agent-26 ｜ **交付**：dev-agent-52，提交 `076998be05fcc8961a5d28c18a1767cea70132dd`
- **验证基线**：`verify_baseline.head = 076998be…` = 承接时 HEAD ｜ **assignment_epoch**：1
- **结论**：**VERIFIED**（7/7 条 done_criteria 全部 PASS）

---

## 0. 承接核实（本任务是本 Sprint 承接状态最干净的一个）

| 核项 | 结果 |
|---|---|
| HEAD 漂移 | 无 ✅ |
| **discovery 内容漂移** | 无 ✅（`bdb37f8206eca4be` 与基线一致） |
| **DoD 未被改写** | 与我承接前存的 wave4 基线 **`diff` 逐字一致**（不只是指纹相同）✅ |
| 实际改动 vs `writes` | `ingest_test.go` **+163/-0**，与声明**逐字一致**、纯新增、无越界 ✅ |
| **`discovery` 指针** | ✅ **已挂，且是 dev-52 自己挂的** —— 本 Sprint 除 TASK-003 外第一个不需要我补的（见 §5） |

---

## 1. 完成标准覆盖矩阵（7 条）

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 第一步把 `syntheticIndex` 改成变参，TASK-005 两处调用同步改 | **无需改动**：它在 TASK-005（`0e2c6fc`）时**就已经是 `entries ...indexEntry` 变参**。DoD 这条的前提已过期（见 §4）。⇒ 纯新增 163 行、0 删除是正确结果 | **PASS**（前提已不成立） |
| functional[1] | 一级键三情形 A/B/C 各一条守卫 | A `TestIngestLeavesNoRowOnFetchOrParseFailure`；B `TestIngestFetchesUnseenArticleEvenWhenStoreIsNotEmpty`；C 复用既有 `TestIngestSkipsSeenArticleUnlessForce` 并在注释块里指明。**不写「端到端支持修订版」的测试** —— 确认未写 | **PASS** |
| boundary[0]① | `--force` 重跑 **pending** 期次 ⇒ `New` 落观测表 | `TestForceOnPendingPeriodLandsInObservations` | **PASS** |
| boundary[0]② | `--force` 重跑**已在观测表**的期次 ⇒ **同时**断言「走到了 Save」与「结果是 Duplicate」，**不得只数行数** | `TestForceOnObservedPeriodIsDuplicate`：**②-a**（走到 Save）= `NotContains("no new reports")` + `NotEmpty(outPeriods)`；**②-b**（Duplicate）= `Contains(bitemporal.Duplicate)`；行数断言只作补充。**两半在注释里显式标号** | **PASS** |
| error_handling[0] | 单期失败不中断整批，**两个断言缺一不可** | `TestIngestContinuesAfterOneFailure`：`require.Error(err, "有期次失败时整批必须返回非零")` **且** `assert.True(has, "第一条失败之后，第二条仍然要被处理并入库")` | **PASS** |
| non_functional[0] | 每条守卫注释写明它守的是哪条 spec 定案 | 三情形有统一注释块（「spec 第 2 节把它拆成三种情形」+ 逐条列 A/B/C）；守卫②标注「闭合 reviewer 的 B4」；并引「计划 716 行原话」说明这是行为守卫而非实现测试 | **PASS** |
| non_functional[1] | 消融自证，**必须包含守卫②**；贴出失败输出的具体那一行 | **我独立复现，见 §2** | **PASS** |
| non_functional[2] | 覆盖率不低于 `coverage_baseline` | 实测 **93.5%** == `coverage_baseline: 93.5` ⇒ 满足「不低于」 | **PASS** |

> ⚠️ **两个覆盖率字段口径不同，本报告按 DoD 用的是前者**：`coverage_baseline: 93.5`（DoD 的判据）vs `coverage_floor: 93`（**门禁**的判据，dev-52 于 `17:40:01` 填）。**门禁过了不等于 DoD 满足** —— 若实测落在 93.0–93.4 之间，门禁会放行而本条 DoD 不满足。本次实测 93.5，两者都过。

---

## 2. DoD 专属消融：我独立复现，红得**恰好在指定的那一半**

DoD 要求：把 `ingest.go:58` 的 `known = neverSeen{}` 改回 `d.Store`，确认**是守卫②转红**、且**红在「走到 Save」那半**。

```
--- FAIL: TestForceOnObservedPeriodIsDuplicate     ← 唯一转红的测试
    ingest_test.go:724
        Error: "no new reports (stopped: seen_article)"
顶层 PASS=320  FAIL=1   （基线 321，外溢 0）
```

`:724` 经核对正是：

```go
assert.NotContains(t, out.String(), "no new reports",
    "②-a：--force 必须穿透 Discover 的判停，否则候选为空、根本走不到 Save")
```

⇒ **三项要求逐条命中**：① 只有守卫②红；② 红在 ②-a（走到 Save）那半，不是 ②-b 也不是兄弟断言；③ 失败输出可读（`stopped: seen_article` 直接说出了它停在哪）。**外溢度为 0。**

📌 这条消融的价值在于它证明的东西很具体：**守卫②真的钉住了 TASK-011 带来的新行为**（`neverSeen{}` 让 `Discover` 不判停）。没有它，「`--force` 现在能重跑已观测期次」这句话在测试层面无人守。

---

## 3. 它发现了一个**既有测试的盲区**，我核实成立

`TestIngestLeavesNoRowOnFetchOrParseFailure` 的注释指出：

> `TestIngestContinuesAfterOneFailure` 里那句 `assert.False(has)` 用的是 `HasPeriod`，而
> **`HasPeriod` 查 `v_hestia_current`，本来就看不见 pending** ⇒ 即使失败的那期真的落了一行 pending，
> 那句断言**照样绿**。它证明的是「没进权威表」，不是「没留下行」。

**我独立核实，成立**：
- `store.go` 的 `HasPeriod` SQL 是 `SELECT 1 FROM v_hestia_current WHERE period = ? AND period_type = ?` ⇒ 确实不含 pending
- TASK-003 有 `TestHasPeriodIgnoresPending` 专门把这个语义钉成契约

⇒ 这是「**守卫在场 ≠ 守卫有效**」的又一实例：那句 `assert.False(has)` 一直是绿的，而它**证明的命题比它看起来的弱**。

**它的补法比指出问题更值得记**：新守卫不用 `HasPeriod`，而是**直接数两张表的行数**（`countRows(TablePending)` 与 `countRows(TableObservations)` 都断言 `Zero`），**再跑一轮**证明「自然重试」真的发生（`assert.True(has, "A：重试后应当真的入库了 —— 这一半才证明「自然重试」")`）。⇒ 判据从「间接、看不见半张表」换成了「直接数行 + 行为二次验证」。

---

## 4. DoD `functional[0]` 的前提已过期（不影响判定，但值得记）

DoD 写「**第一步**是把 `syntheticIndex` 改成变参 —— 这是全计划唯一的破坏性改动，TASK-005 的两处调用同步改」。

**实测：它在 TASK-005 交付时（`0e2c6fc`）就已经是 `func syntheticIndex(t *testing.T, entries ...indexEntry) []byte`。**

⇒ dev-54 写 TASK-005 时就按变参写了，这条「破坏性改动」**在本任务开工前已经不存在**。dev-52 因此一行都不用改，**+163/-0 的纯新增正是正确结果**。

📌 **形态**：这与 leader 在派发前整体重写 `boundary[0]`（因 TASK-011 推翻了旧事实）是同一族 —— **计划文档里的「现状描述」会被上游任务悄悄改掉**，而 DoD 抄写它时不会带上时间戳。区别是 `boundary[0]` 那条被抓住了，这条没有；**没被抓住是因为它无害**（多写一句不存在的工作，而不是断言一个错误的行为）。

---

## 5. 一件正面的：`discovery` 指针这次不用我补

```
17:38:08  dev-52  update keys=["discovery"]     ← 自己挂
17:38:17  dev-52  → dev_done                     ← 9 秒后
```

⇒ **它在自己还有写权时主动挂上了指针**，绕开了本 Sprint 那个「Leader 派验插在写文件与挂指针之间」的窗口缺口（七个任务由我补）。这是继「Leader 派验前先查」之外的**第三条路，且不依赖窗口长度**。

顺带核实：`dev_done` 状态下 dev **仍能 `update`**（`17:40:01` 那次 `coverage_floor` 即是），⇒ 这条路的窗口比看起来更宽 —— **只要没转 `verifying` 就还来得及**。

---

## 6. 实跑证据（隔离 worktree @ `076998be`）

```
gofmt → 空    vet → 空    build ./... → OK
go test -count=1 -cover → ok  coverage: 93.5%   (coverage_baseline 93.5 ✅ / coverage_floor 93 ✅)
go test -count=1 -race  → ok
顶层 PASS 321 / 全部 PASS 681 / FAIL 0
```

⚠️ 主工作区此刻有 dev-53 的在途 TASK-008 文件（`cmd/atlas/hestia.go`、`hestia_test.go` 未跟踪），**故全部数据采自隔离树**；worktree 已 `remove`，零残留。

---

## 7. 复现（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-t007 076998be05fcc8961a5d28c18a1767cea70132dd
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover      # 93.5% / 321 顶层 / 0 FAIL
# §2 消融：删掉 ingest.go 的 `if d.Force { known = neverSeen{} }` ⇒ 仅守卫②红在 :724（②-a）
```

---

# 复验（QA `review_fix` 第 1 轮）—— **PASS**

- **判定对象** `b6b13a4ac3ab…`（= `verify_baseline.head`，**无漂移**）｜ discovery 无漂移（`67291dc9b558`）｜ `rework=1`
- **基线含本任务返工提交** `b6b13a4 fix(TASK-007): 清第三份过期结论副本` ✅｜ `writes` 收窄为 `ingest_test.go` 单文件，实际改动一致

## 唯一一条 `fix_items`：WARNING-2（第 2 份过期副本）⇒ **PASS**

原处那段「经 `Discover` 的 `HasPeriod` 过滤 ⇒ `Verdict` 恒为 `New`、`Duplicate`/`Revision` 当前不可达」
**已改写为引文 + 三重过期说明**（`ingest_test.go:295-298`）：

> 「⚠️ **原注释在这里写着一段事实陈述**（「…当前不可达」），**三重过期**：判停早已不用 `HasPeriod`、
> `Duplicate` 已被上面那条绿测试证明可达、而那个 `file:line` 坐标漂移之后**正好指向了自己的反驳**。」

- **旧行号锚 `discover.go:303-318` 残留 0** ✅（改用符号名 + 字串锚）
- **保留了「决定」、删除了「事实陈述」** ✅ 符合 dev-agent-54 给的那条护栏

📌 **这是本 Sprint 值得留下的订正写法**：不是删掉旧说法，而是**保留它并标注为假** ——
只删的话后人可能重新发明它。⚠️ 代价是**否定式 `grep` 会对它假阳**（我这轮就命中一次，读原文才排除）。

## 实跑
`vet`/`build`/`test ./...`/`-race` **全部 exit 0**；覆盖率 `internal/hestia` **93.6%**。
