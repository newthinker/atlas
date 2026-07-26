# TASK-006 验证报告 — refresh 接线（主干流新字段落库 + closes 落 price_daily）

- **验证者**: test-agent-7 (Reality Checker)
- **被验对象**: commit `34e695a` / package `./internal/prism`
- **assignment_epoch**: 1
- **判定**: ✅ **PASS (verified)**
- **纪律**: 结论只锚定我本人实跑的输出与文件内容；未采信 dev-agent-16 自述。
  探针在隔离 worktree（checkout `34e695a`）内进行，验完删除并 `git worktree remove`；
  主工作树无我的残留（`git status` 仅剩 dev-agent-15/17 的在途 WIP）。

---

## 1. 实跑证据

| 命令 | 结果 |
|---|---|
| `GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -cover` | `ok  coverage: 94.5% of statements`（基线 94.1%，门槛 80） |
| `... -count=10` | `ok` 全绿 |
| `GOTOOLCHAIN=local go vet ./internal/prism/` | 无输出 |
| `GOTOOLCHAIN=local go build ./...` | OK |

新增/改动函数覆盖率：`degrade` **100%**、`upsertPrices` **100%**、
`refreshEngine` 95.2%、`refreshEdgar` 91.2%。
未覆盖块逐行核对，**全部是既有错误路径**（`upsertMeta` 失败、`UpsertFundamentals` 失败、
`FetchHistory` 失败），且它们都正确返回 `""` 作为降级位——这些点发生在落价之前，
返回空降级说明是对的。**无新增逻辑未被覆盖。**

---

## 2. Done Criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| **F0** | `refreshEdgar` 落库携带 5 个主干流新字段，含 NaN 透传 | `TestRefreshEdgarStoresMainFlowFields`：12 季逐季断言 4 字段值 + 第 6 季 SGnA `math.IsNaN` + 第 5 季 SGnA=2.0（**对偶**：NaN 与非 NaN 各断言一次）+ 既有字段 Revenue/Source 未被接线破坏 | **PASS** |
| **F1** | `refreshEdgar` 取得 closes 后 `UpsertPrices`，日期与收盘价一致 | `TestRefreshEdgarUpsertsPrices`：逐点比对 `D` 与 `Close`（2 点），并 `assert.Empty(rep.Degraded)` | **PASS** |
| **F2** | `refreshEngine` 同样调用 `UpsertPrices` | `TestRefreshEngineUpsertsPrices`：逐点比对 | **PASS** |
| **B0** | 落价失败：`Refreshed` 仍计数、进 `Degraded` 非 `Failed` | `TestRefreshPriceUpsertFailureDegradesOnly`（edgar/engine **双子测试**） | **PASS**，详见 §4 |
| **E0** | lixinger/akshare 不调用 `UpsertPrices`（A/H 零变更） | `TestRefreshCNPathsNeverUpsertPrices` + **结构性论证**（§3） | **PASS** |
| **N0** | fake 同步实现全部新方法；既有测试断言零修改（`verify_by: review`） | 逐函数 md5 比对 + 断言计数（§5） | **PASS** |
| **N1** | 包测试全绿（`verify_by: test`） | 见 §1 | **PASS** |

**7/7 条 done_criteria 全部有对应测试且断言非空洞。**

---

## 3. A/H「结构性保证」论断核实 ✅（不只看负向断言）

dev 称签名不变让「A/H 零变更成为结构性保证而非仅测试保证」。**独立核实成立**：

`UpsertPrices` / `upsertPrices` 在包内的**全部**出现位置：
```
refresh.go:29   Store 接口声明
refresh.go:202  func upsertPrices(...)  定义
refresh.go:207  store.UpsertPrices(id, rows)   ← 唯一真实调用点
refresh.go:232  deg := upsertPrices(...)  ← refreshEngine
refresh.go:453  deg := upsertPrices(...)  ← refreshEdgar
```
即 `upsertPrices` **只有 2 个调用点**，全在 engine/edgar 两路。
`refreshLixinger` / `refreshAkshare` 函数体内 `grep -i price` **零匹配**。

**结论**：A/H 两路不是「碰巧没调用」，而是**代码路径上根本无法触达**落价逻辑。
这比 `assert.Zero(priceCalls)` 的负向断言强一个量级——负向断言只能证明
「当前这组输入下没调用」，结构性论证能证明「任何输入下都不可能调用」。
两者都在，护栏是双层的。

---

## 4. `boundary[0]` 是否只测了一半？✅ 不是，测了 5 件事

leader 提示这类「多个后果」的断言最易只测一半。逐条核对
`TestRefreshPriceUpsertFailureDegradesOnly`（edgar / engine 双子测试各跑一遍）：

1. `assert.Empty(t, rep.Failed, "落价失败不得让标的进 Failed")` ✅
2. `assert.Equal(t, 1, rep.Refreshed, "估值主流程仍算刷新成功")` ✅
3. `require.Len(t, rep.Degraded, 1)` + `Contains "NVDA"` + `Contains "disk full"` ✅
   （**Degraded 非空、含标的、含原始错误文本**三层）
4. `assert.NotEmpty(t, store.upserts["NVDA"], "估值行仍应落库")` ✅（额外）
5. `assert.Empty(t, store.prices["NVDA"])` ✅（额外，但见下方保留意见）

**三件必需的都在，另加两件。断言不空洞。**

**一处保留意见（不影响判定）**：第 5 条 `assert.Empty(store.prices["NVDA"], "失败的落价不得留下半截数据")`
实际断言的是 **fakeStore 自身的早返回行为**（fake 在 `priceErr` 命中时 `return err` 而不 append），
**不是被测代码的性质**。真正的写入原子性由真实 `Store.UpsertPrices` 的
`tx.Begin` + `defer tx.Rollback` 保证（我在 TASK-002 已验证）。
这条断言无害但属于「测试替身而非被测系统」，其通过不构成对生产原子性的证据。

---

## 5. 「既有断言零修改」独立确认 ✅

- **删除行**：`git show 34e695a -- refresh_test.go` 的 `-` 行**确为 1 行**，
  即 `newFakeStore` 的单行构造器（被多行版替代）。dev 所述属实。
- **逐函数 md5 比对**（`34e695a^` vs `34e695a`）：既有 **28 个** 测试函数
  **全部「未改动」，0 个改动**。
- **断言计数**：`assert.`/`require.` 出现次数 **138 → 174**（+36，全为新增），
  无净减少。

---

## 6. dev 主动披露的行为选择 —— 独立复现，与描述完全一致 ✅

leader 已裁决接受「价格落库点在估值计算之前，副作用是 EPS≤0 熔断标的仍写 price_daily」，
要求验证**实际行为与描述一致、熔断语义未变**。我在隔离 worktree 内加探针实测
（该行为**没有任何现有测试覆盖**，只能自行构造）：

```
Refreshed=0
Failed=[LOSS: valuation: current EPS is non-positive]
Degraded=[]
valuation rows=0   price rows=2   priceCalls=1
```

**逐条对照描述**：
- 熔断标的**仍写 price_daily**（2 行）→ 与披露一致 ✅
- 仍进 `Report.Failed`，错误语义仍为 `non-positive`（对齐 `ErrNonPositiveEPS`）✅
- `valuation_daily` **仍不写**（0 行）✅
- 不计入 `Refreshed`（0）✅

**熔断语义确实没变。** 代码层面也一致：`deg := upsertPrices(...)` 位于
`if e, ok := currentEPS(eps); ok && e <= 0 { return deg, valuation.ErrNonPositiveEPS }` 之前，
熔断分支照常返回错误。既有 `TestRefreshEngineNonPositiveCurrentEPS`
（本次 commit **未改动**，md5 确认）继续守护 Failed + valuation 不写。

**补充实测：熔断 + 落价同时失败时两者互不吞没**
```
Failed  =[LOSS: valuation: current EPS is non-positive]
Degraded=[LOSS: price_daily upsert failed (disk full), valuation unaffected]
```
两条独立记录，符合预期。

---

## 7. 既有回归网核实 ✅（leader 要点 5 成立）

`TestRefreshEdgarPath` 与 `TestRefreshFallbackNotTriggered` 各含一条
`assert.Empty(t, rep.Degraded)`，且**两者本次均未被改动**（md5 确认）。
由于 `fakeStore.priceErr` 默认为空 → `UpsertPrices` 返回 nil → `deg == ""`，
**若落价成功路径误记 Degraded，这两个既有测试会立即变红**。

这是一道**不需要新写测试就已存在的护栏**，dev 的说法成立，值得记录。

---

## 8. 两处观察（均不影响判定，供后续参考）

**8.1 已裁决的行为选择缺少测试锁定（建议补）**
「熔断标的仍写 price_daily」是一个**经 leader 明确裁决接受**的行为，但**当前无任何测试断言它**。
风险：后人若把 `upsertPrices` 挪到熔断判断之后（看起来是个合理的「修复」），
行为会静默改变而**没有任何测试变红**。既然这是有意为之的取舍，建议补一条测试把它钉住
——否则它只存在于 discovery 文档里，代码层面是不设防的。
（不作为 reject 理由：DoD 未规定落库时点。）

**8.2 兜底路径会重复落价一次（无害）**
edgar 在 `upsertPrices` **之后**失败（如 reconstruct 失败）且配置 `fallback_source: engine` 时，
engine 兜底会再落一次价。实测：
```
priceCalls=2  Degraded=[NVDA: edgar failed (reconstruct: ...), engine fallback ok]
```
真实 `Store.UpsertPrices` 按 `(instrument_id, d)` 幂等（`ON CONFLICT DO UPDATE`，TASK-002 已验证），
故**不产生重复行、无数据后果**，仅一次冗余写。记录备查，非缺陷。

---

## 9. 范围外

`internal/prism/sankey` 曾出现红灯（`periods_test.go` 引用未实现符号导致 build failed）。
按我在 TASK-002 后立的判据核实归属：`TASK-007 status=in_progress assigned_to=dev-agent-17
packages=./internal/prism/sankey` —— **在途任务的包，默认按 TDD 中间态处理**。
我复跑时该包已转绿（`ok`，dev-agent-17 已补 `periods.go`）。与 TASK-006 无关。

---

## 10. 判定

**verified（PASS）。**

7/7 条 done_criteria 均有对应且非空洞的测试；A/H 零变更经结构性论证确认为
「路径上不可触达」而非「碰巧未调用」；`boundary[0]` 三件必需后果全部断言（另加两件）；
dev 主动披露的熔断副作用经我独立构造复现，与描述逐条一致且熔断语义未变；
既有 28 个测试函数逐函数 md5 确认零改动、断言净增 36 条；
覆盖率 94.5% 无回退，新增函数 `degrade`/`upsertPrices` 均 100%，未覆盖块全为既有错误路径。
达到「压倒性证据」标准。

两处观察（§8）建议后续处理，均不构成退回理由。
