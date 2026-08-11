# Sprint 035 · 第一轮 Code Review（常规审查）

| | |
|---|---|
| 审查者 | qa-agent-12 |
| 被审 | `4547631dc8fdf03aeb97e84635a4174a8f5cf05c`（分支 `master`） |
| 基线 | `125ad896fb096f7766cbb3c958ba2635a311c6ba` |
| 范围 | `git diff 125ad89..4547631 -- internal/hestia/`（10 文件 +2303/-7） |
| 实验环境 | 隔离 detached worktree，主工作区**逐字节未改动**（收尾 sha256 + `git status --porcelain` 双重核实通过） |

**本轮结论：无 CRITICAL。1 MAJOR + 7 MINOR，全部为潜伏项或措辞项。**
对抗轮的发现见 `sprint-035-review-round2-adversarial.md`；**verdict 在那一份**。

---

## 0. 基线（自证数字，采自最后一次改动之后的同一棵树）

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/     # 无输出
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.821s  coverage: 92.0% of statements
$ ... -v | grep -c -- '--- PASS'   ->  475
$ ... -v | grep -c -- '--- FAIL'   ->  0
```

口径注明（F4）：92.0% 是 `go test -cover` 的**包级**数字，与 `-coverpkg`、`cover -func` 的 total 可差 0.1pct。

---

## 1. 通过项（每条附证据，非转述）

### 1.1 scope 声明与实际改动一致 —— 无未申报越界

逐提交比对 `git show --numstat` 与各任务 `writes`：

| 提交 | 实际改动文件 | 是否 ⊆ 对应任务 `writes` |
|---|---|---|
| `234baea` T1 | store_test / thresholds{,_test} / types | ✅ |
| `e24c062` T2 | required{,_test} | ✅ |
| `c693177` T3 | store / store_test / validate | ✅ |
| `0918324` T4 | store_test / validate{,_test} | ✅ |
| `1716dfc` T5 | validate{,_test} | ✅ |
| `9ec8a6f` T6 | validate{,_test} | ✅ |
| `4547631` T7 | CONTRACTS / thresholds{,_test} / validate{,_test} | ✅ |

**七个提交全部落在声明范围内，无一处越界。**

### 1.2 导出面守卫未被放松

`TestStoreExposesNoWriteMethods` 与 `TestPackageExposesNoWriteFunctions` 两条**都仍是精确集合相等**（`assert.Equal` 名单），不是弱化成「包含」。三个新增导出函数（`Validate` / `DefaultThresholds` / `Store.Preceding`）各留了一段登记说明与不构成写口的论证。

⇒ 这正是该守卫的设计意图在生效，**不是绕过**。

### 1.3 spec 符合性逐条核对

| spec 条目 | 状态 | 证据 |
|---|---|---|
| 七道闸门 ID 与顺序 | ✅ | `TestGatesMatchContractedCheckIDs` 用字面量钉住 `knownCheckIDs()`；`gates` 表顺序即报告顺序 |
| `Check.Value` 单位（§7） | ✅ | `TestCheckValueUnitsFollowSpec`。**期望值来源正确**：`-1800` 出自 spec §7 举例、`-1203` 出自 M0 契约样本，**独立于本实现**；且 yoy 期望值在测试内**独立重算**，未复用 `yoyFields()`——没有用被验对象的尺子量被验对象 |
| D1 必填集派生不手写 | ✅ | `required.go` 遍历 `tsfStockItems`/`tsfFlowItems`；三个总量字段手写并注明原因，由 golden 键集比对钉住 |
| D2 窄接口拿历史 | ✅ | `History` 单方法；`var _ History = (*Store)(nil)` 编译期绑定 |
| D3 建机制不填值 | ✅ | `MagnitudeRanges` 空 + `Range.Unit`；`TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated` 绊线 |
| D4 闸门失败不是 error | ✅ | 全部闸门返回 `Check`；`TestGatesRejectMalformedData` 断言 `require.NoError` |
| §7.1 五行合成映射 | ✅ | `TestDepositSumCombinesTwoCriteria` **6 行**（五行 + 「绝对值超标时不谈漂移」），逐行断言 Status/Reason/Value |
| §8.1 ULP 契约 | ⚠️ | 契约注释已写、测试已建，**但守卫不观察生产算术**——见 round2 的 R2-4 |
| §8.3 双重跳过理由 | ✅ | `TestStockContinuitySkipReasons` 四种构造，v1 期次报 `absent_field:tsf_stock` 而非 `no_prior_period` |

### 1.4 `Store.Save` 接线

`TestValidateOutputIsAcceptedBySave`（v2 全字段 + v1 无社融，真库，落 `TableObservations`）与 `TestFailedValidationLandsInPending` 均实跑通过。
⚠️ 但该测试注释声称的是一条**全称性质**（「Validate 的产出必须能被 Save 接受」），实测有反例 —— 见 round2 的 R2-3。

### 1.5 未发现「为求绿而放松既有断言」

`types.go` 的 `checkEnum` 前缀外移**零测试改动**（F1 已登记该守卫本就量不出「输出逐字不变」）；`store_test.go` 的两处名单改动均为**登记式扩充**且保持精确相等；`validate_test.go` 的 `wantGateIDs` 是字面量，随闸门增加而**手动**扩充（注释明写这是刻意代价）。

---

## 2. 本轮发现

### R1-1 · MAJOR — 口径豁免只按 `Period` 匹配，**不含 `PeriodType`**

**位置**：`thresholds.go:121` `exemptionFor(period, checkID)`；调用点 `validate.go:93`

同一个 `Period` 字符串下的 `annual` 与 `monthly` 是**两个合法且不同的期次**，这一点在本仓库有三处独立确认：
观测表主键含 `period_type`；`Store.Preceding` 刻意 `AND period_type = ?`（注释：「月度与半年度是两条独立序列」）；`CONTRACTS.md` H10 复核结论明写 `2025-12/monthly` 与 `2025-12/annual` 都是真实期次、业务键不同。

**唯独豁免把 `Period` 单独当成键。**

**证据**（隔离 worktree 探针，一条 `Period: "2025-12"` 的豁免）：

```
period=2025-12 period_type=annual   -> status=skipped reason="caliber_exemption:2025-01" passed=true
period=2025-12 period_type=monthly  -> status=skipped reason="caliber_exemption:2025-01" passed=true
```

⇒ 为年报写的一次性豁免**同时**豁免了同月的月度序列。spec 4.6.3 要求「按 (期次, 检查 ID) **精确**指定」，而「期次」在本仓库的其余所有位置都是 `(period, period_type)`。

**为何不判返工**：TASK-007 `boundary[0]` 的字面要求是「豁免配在 2025-01 而观测是 2026-06 时照常 failed」，该测试存在且通过 —— **DoD 字面已满足**；且当前无配置装载器（M1b-4）、无任何已配置豁免。
**建议**：给 `CaliberExemption` 加 `PeriodType string` 并纳入匹配键，或在 `validate()` 里显式声明「豁免跨 period_type 生效」。**M1b-4 引入 YAML 装载前必须定案**——那一刻它从潜伏变成可配置。

---

### R1-2 · MINOR — F10 已被实测证伪的理由，原样写进了生产代码注释

**位置**：`store.go:224`（生产注释）、`store_test.go:1645`（测试注释）

两处都写着「不挡住非正数，一次 `n=0` 的调用会把**整个序列拉回来**」。F10 已记载 test-agent-24 证伪了这个成因，但**只登记了 DoD 文本那一份**；代码里的两份原样交付了。

**证据**（去掉守卫、把 `n` 直接喂给 `LIMIT`，库内 3 期）：

```
LIMIT 0  返回行数 = 0      <- 注释声称这里会「拉回整个序列」
LIMIT -1 返回行数 = 3
LIMIT 2  返回行数 = 2
```

**守卫 `if n <= 0` 本身正确且必要**（`n=-1` 确实拉回全部），错的只有理由。而理由是别人复现时的唯一入口 —— 与 F8/F10/F16 同族。
**建议**：改成「SQLite 的 `LIMIT -1` 表示不限制，故负数会拉回整个序列；`LIMIT 0` 返空，但一并挡住更简单」。

---

### R1-3 · MINOR — `Thresholds.validate()` 只校验豁免，五个数值阈值一个都不校验

**位置**：`thresholds.go:87-115`。函数文档写着「配置错了应当**立刻响亮失败**，而不是让闸门带着错阈值跑完，产出一份看起来正常的报告」。

**证据**：

```
Thresholds{}.validate()          = <nil>
四个阈值全 -1 的 validate()      = <nil>
Range{Min:100,Max:1}.validate()  = <nil>
```

`Thresholds{}` 零值下每一期都会被判 failed 流向 pending（Skeptic lens 实测：deposit/corp/yoy 三道全 failed），而这正是文档承诺要拦下的形态。M1b-4 接线时忘调 `DefaultThresholds()` 是最容易犯的一种。
**建议**：`validate()` 增「四个比例阈值与 `YoYSanityMax` 须 > 0；每条 `Range` 须 `Min <= Max`」。

---

### R1-4 · MINOR — 本 Sprint 新增 4 个导出 type + 1 个导出 var，两道导出面守卫**都看不见**

**证据**（`go doc -all` 在基线与交付态两侧 diff）：

```
+ func (s *Store) Preceding(...)          <- 守卫可见，已登记
+ func DefaultThresholds() Thresholds     <- 守卫可见，已登记
+ func Validate(...)                      <- 守卫可见，已登记
+ type CaliberExemption struct            <- 两道守卫都看不见
+ type History interface                  <- 同上
+ type Range struct                       <- 同上
+ type Thresholds struct                  <- 同上
+ var NoHistory History = noHistory{}      <- 同上
```

这是 CONTRACTS #2（c 类，「**全静默**」）已登记的盲区，**不是新缺陷**。问题在两处措辞：
① `store_test.go` 的注释写「`History` 与 `NoHistory` 都不是 `FuncDecl`，不进本条视野，**无需登记**」——「无需登记」把一个已知盲区读成了「它们没事」；
② Sprint 034 一节有「本 Sprint 导出面净增量恰好是 `func Parse`」的先例，**Sprint 035 一节没有记录导出面增量**。

**建议**：CONTRACTS Sprint 035 一节补一行导出面净增量，并把那句注释改成「落在 #2 的盲区内，本条守不住」。

---

### R1-5 · MINOR — 三处带数量词的注释已过期（Minimalist lens 发现，我已核实）

| 位置 | 写的 | 实际 |
|---|---|---|
| `validate.go:434` | 「其余**四道**各自按 absent 跳过」 | **6 道**（5 道 `absent_field:*` + `magnitude_sanity` 的 `not_calibrated`） |
| `validate_test.go:371` | 「其余**四道**各自按 absent/未标定跳过」 | 循环实际覆盖 6 道 |
| `validate_test.go:470` | 「deposit_sum 的**四条**测试都要用它」 | `depositWith` 有 **5** 个使用者 |

前两条写于 `0918324`（当时只有五道闸），T5/T6 加闸后过期；第三条写下时即错。
**建议**：去掉数量词（「其余闸门」不会过期，「其余四道」必然过期）。

---

### R1-6 · MINOR — `historyDepth = 6` 的注释把「拍的数」写成了「派生的数」

**位置**：`validate.go:55-60`，注释称「取两道闸需求的较大者（… deposit_sum 的漂移**要 6 期**）」。
代码里唯一的期数门槛是 `minDriftHistory = 3`（`validate.go:184`），均值对 `len(hist)` 取多少算多少 —— **没有任何地方要求 6 期**。
**建议**：改成「6 是漂移均值的取样窗口（硬下限见 `minDriftHistory`）」。

---

### R1-7 · MINOR — `TestReportAlwaysContainsEveryGate:403` 那条 require 的理由不成立

注释称「`gates` 表本身必须恰好是这七道 —— **少一道时下面的逐行比对会跟着缩水而发现不了**」。
但下面的逐行比对（`:434`）比的是**字面量** `wantGateIDs`，**本来就不会缩水**。该 require 与 `TestGatesMatchContractedCheckIDs`（同一份字面量 vs `knownCheckIDs()`）实质重复。
结论对（钉住 `gates` 是好的）、理由错。**建议改理由，不必删**——它给出的失败信息比 6 个子测试同时红更直接。

---

### R1-8 · MINOR — CONTRACTS 边界守卫收口表的**标签**宽于它实际用的规则

表头称「`validate.go` 现有 **12 个比较运算符**（按最宽正则扫描后逐个标注）」。
实测：关系运算符（`< > <= >=`）**10 行 12 个**——**数字是对的**；但若按字面的「比较运算符」，还应含 6 处相等比较（`total == 0`、`prev == 0`、`len(in.prior) == 0` ×2、`len(missing) == 0`、`len(MagnitudeRanges) == 0`）。

⚠️ **纠正 Minimalist lens 的措辞**：它报「表漏 6 个」，暗示守卫缺口。实测那 6 处**全部有测试**（`TestCorpLoanSkipsOnZeroDenominator` 等），所以**不是守卫缺口，是穷尽性声明的标签问题**——正是 F19 方法论第 3 条（「写下的规则必须能被别人原样跑出你表里的那个集合」）的再次出现，而这次出现在同一条方法论的产物上。
**建议**：把标签收窄成「12 个**关系**运算符」。**不要删表**。

---

## 3. 对验证团队清单 A 的裁定（Leader 点名要我决定的那条）

**问题**：`validate_test.go:281`「全部用 2 的幂……**否则**测的是浮点舍入」与 `:592`「`0.03` 不精确**故**改用自定义阈值」，与 `validate.go:22` 新写的契约注释（「取决于参与运算的量是否精确，**不取决于算路长短**」）并存于同一个包。

**裁定：不要求本 Sprint 统一措辞；只要求把其中一条登记为待改，且明确区分两者。理由如下。**

**① 首先要避免的是 F25 那个错误本身。** F25 记载：Leader 基于「`0.02` 不精确」这一正确实测，下了「统一改成 X」的指令，而 dev-agent-46 实测后**只改一处**并申报收窄执行 —— 因为那几处用的是 **2 的幂，措辞本来就对**。**「统一措辞」这个指令形状本身就是 F25 抓到的错误。** 我不重复它。

**② 两处的性质不同，处置也应不同：**

| | `:281`「用 2 的幂，**否则**测的是浮点舍入」 | `:592`「`0.03` 不精确，**故**用自定义阈值」 |
|---|---|---|
| 事实部分 | **对**（`1/16`、`3/32` 确实精确可表示） | **对**（`0.03` 确实不精确） |
| 因果部分 | **过强**：把充分条件写成必要条件（test-agent-24 实测非 2 的幂同样 KILLED） | **错**：真机制是均值→相减的**算路为链**（F22），不是 `0.03` 的可表示性 |
| 照它办的后果 | **无害**——按更严的规则选数，结论仍成立 | **有害**——照它推理的人会以为「换个精确可表示的阈值就安全」，而在链式算路下仍会静默失效 |

⇒ **只有 `:592` 需要改**，因为它是唯一一条**照着办会出错**的。`:281` 建议软化为「2 的幂是**充分**条件（实测非 2 的幂同样能杀死变异），选它是为了让论证一眼可核」，优先级低。

**③ 为什么不现在返工**：两处都是纯注释，不改变任何断言；而 TASK-004/005 已 `verified`，为措辞重开返工环不划算。**建议随 M1c 首次触碰 `validate_test.go` 的任务一并改**，并写进 CONTRACTS 浮点契约一节（该节已有正确表述，只需点名这两行是待收敛的旧措辞）。

---

## 4. 对清单 B / C / D 的处置

- **B（`TestReportKeepsEveryGateUnderExemption` 的 `assert.Equal(knownCheckIDs(), gotIDs)` 形似自指）**：确认**不是**问题。两侧不同源——左侧来自 `gates` 表，右侧来自 `Validate` 实际产出的报告，它验的是「豁免分支没让闸门从报告里消失」。Minimalist lens 另用变异 E1–E4 证明它与 `TestCaliberExemptionRecordsSkipNotPass` **互为唯一守卫**（E2 只有前者接得住，E4 只有后者接得住），删任一条都开洞。**两条都要留。**
- **C（跨任务守卫缺口 F1/F9/F12/F13/F14/F15）**：裁定**恰当**，逐条复核了不返工的依据。其中 F12/F17 的依据已按 F20 从「时点性」升级为「结构性」（绊线在 `thresholds_test.go:58`，我已核实那段提醒确实落地，含三件待补事项）。**唯一补充**：F14/F15 指出的两条平凡为真测试，在本轮对抗中被证明还漏掉了更根本的一层（见 round2 R2-2），建议一并登记。
- **D（未跑 `npx gitnexus analyze`）**：不属我范围，未处置。

---

## 5. 本轮未发现的问题（明说，免得被读成遗漏）

逐条查过且认为**没有问题**：`requiredFields` 的 `slices.Clone` 防别名泄漏；`scanObservation` 用 `sql.NullFloat64` 把 NULL 还原成「键不存在」（双向都有测试，且「把 0 读丢」与「把 NULL 读成 0」两个方向分别断言）；`Preceding` 的降序 / `period_type` 隔离 / `period <` 严格边界（三条变异全部 KILLED）；`gateCompleteness` 的 `unknown_extractor` 是刻意的失败信号且实现正确；七道闸的零分母守卫三处都在；「`Save` 是唯一写入口」在本次改动中**未被削弱**（`Preceding` 只读、走已收窄的 `Querier`）。

---

## 环境完整性

主工作区 `internal/hestia/{validate,store,thresholds}.go` 的 sha256 与开工时**逐字节一致**，`git status --porcelain` 与开工时**逐字一致**（唯一差异项 `.arcforge/write-matrix.json` 是 Leader 登记 token 造成的，开工前即存在）。全部实验在 detached worktree 内进行，收尾已 `git worktree remove` + `prune`。
