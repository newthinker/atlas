# TASK-008 验证报告 —— Sprint 035 收尾：豁免边界修补与契约登记

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`master @ 722aa2728723b573537160b602bb06a03b3169b4`（全 sha）
  —— **不是** `verify_baseline` 记录的 `e6395619`，理由见 §0
- **结论**：**VERIFIED（8/8 PASS）**，经 `--ack-drift` + `--ack-discovery-drift` 显式确认漂移

**消融：6 个变异全部 KILLED**（含我追加的 M5），计数自证三批均 ✓ 一一对应。

---

## 0. ⚠️ 漂移核实 —— **发现两处，派单未提，我按 AD-29 处置**

```
$ jq -r .verify_baseline.head        → e6395619edb7a7335893851baabb37408c038922
$ git rev-parse HEAD                 → 722aa2728723b573537160b602bb06a03b3169b4   ✗ 不一致
$ jq -r .verify_baseline.discovery_sha256 → 4591f7d962c03e209f6fe31bda70fc92587a4b1e3f69a6b46ccfea49e3729a5a
$ shasum -a 256 .arcforge/discoveries/TASK-008.json
                                     → 04c4cbaf0b160a33035228db46ee1ffe61dfd32a6efc378f74ca6cfdd57faff5   ✗ 不一致
```

**时序**（这是关键）：

| 事件 | 时刻 |
|---|---|
| 基线写入（`dev_done → verifying`） | `12:16:53Z` = `20:16:53 +0800` |
| **提交 `722aa27`** | `20:17:20 +0800` —— **基线之后 27 秒** |
| **discovery 被改写** | `20:17:33` —— **基线之后 40 秒** |

⇒ **实际是三轮交付（`e45164d` → `e6395619` → `722aa27`），派单说的是两轮。**

**漂移落在声明范围内**：`722aa27` 改的是 `internal/hestia/CONTRACTS.md`，**在 `writes` 里** ⇒
按 AD-29 规则这是「范围内的真漂移」，需显式确认而非 INFO 放行。

**我看过差异后的判断**：`722aa27` 是 **+3 行文档**（置换盲区表补第三行「两组同时互换」
+ 一句解释），**无代码、无断言变更**。discovery 相应更新为 `commit: 722aa27`、
`suite.tree: 722aa27`，并把第三行写进 `addendum.R2_13_reproduced_by_me`。

**处置**：**判定对象取 `722aa27`（真实交付物）而非基线记录的 `e6395619`** ——
AD-29 存在的全部意义就是「判定对象与最终交付物必须是同一个 commit」。
若我验 `e639561` 而 `722aa27` 出货，我的 VERIFIED 覆盖的就是一棵不出货的树。
两个 ack 参数取**当前值**，是我实际看过之后填的。

## 0.1 scope

```
$ git diff --numstat 4547631..722aa27
131	1	internal/hestia/CONTRACTS.md
36	0	internal/hestia/thresholds.go
79	0	internal/hestia/thresholds_test.go
8	2	internal/hestia/validate.go
```

三次提交累计改 4 文件 == `writes` 四项，无越界。
（`validate.go` 在 addendum 到达时由 dev 自己经写通道补进 `writes` —— 那是当时唯一合法的路径，
`dev_done` 状态下 leader 无写权。）

---

## 1. 事项① —— 门禁盲区：**独立补跑通过（第三重核实）**

派单说明了盲区成因：状态机**没有 `dev_done` 自环边、也没有 `dev_done → in_progress`**
⇒ dev 无法让自动门禁重跑，那句 `Coverage: 92.1%` 跑的是第一轮的树。
**我在真实交付树 `722aa27` 上独立跑了一遍**：

```
gofmt -l internal/hestia/                              → 无输出
go vet ./internal/hestia/                              → 无输出
go build ./...                                         → 通过
go test -count=1 -coverpkg=./internal/hestia           → ok  coverage: 92.1%（门禁口径）
go test -count=1 -cover                                → ok  coverage: 92.1%（包级口径）
go test -race -count=1                                 → ok
```

覆盖率 **92.1% ≥ DoD 要求的 92.0%** ✓。三方（dev 手工 / Leader / 我）三次独立测量一致。

> **这个盲区值得写进流程**：它与本 Sprint 的 AD-035-1（「门禁在不含交付物的树上成功」）**同族**，
> 但成因不同 —— 那次是**工作区**不对，这次是**状态机没有让门禁重跑的边**。
> 前者靠 worktree 纪律解决，后者需要一条 `dev_done → dev_done` 自环边（或允许
> `dev_done → in_progress` 回退）。dev 主动申报是对的，但**申报只能让人补跑，不能替代机制**。

---

## 2. 事项② —— 两条追加（单独评价，**不构成 DoD 判定依据**）

### 2.1 R2-14：两个方向**探针实测全部成立**

```
[被纠正的半句] 拼错 ID "deposit_summ" ⇒ err=true
   ⇒ 原注释「拼错的 ID 因为不在旧列表里反而被放行」确实**不可能**发生，纠正正确
[方向① 少了新 ID]  合法 ID 用缺项白名单校验 ⇒ err=true          （响亮 ✓）
[方向② 留着已删 ID] 已删 ID 用过期白名单校验 ⇒ err=false        （静默通过 ✓）
                    该豁免命中的真实闸门数 = 0                  ⇒ 死配置，无声 ✓
```

方向②的「静默」是双重的：既通过校验、又**永不命中**（`exemptionFor` 比对的是真实 `gates`）。
两个方向都成立，且「哪个静默」标对了。

### 2.2 R2-13：我**独立复跑三行 + 加了一个对照组**

```
原值: 企业短期=48100 中长期=88200 住户存款=146400 财政存款=6579
对照（不改）              Passed=true   failed闸门数=0
企业短期 ↔ 中长期          Passed=true   failed闸门数=0
住户存款 ↔ 财政存款         Passed=true   failed闸门数=0   （146400/6579 = 22.3 倍）
两组同时互换              Passed=true   failed闸门数=0
```

三行全部复现（不是转述）。**我另加的对照行揭示一个表述问题，见 §7 发现 1。**

---

## 3. 事项③ —— R2-3 的加强：**正当，而且我实测了它的前提**

DoD `functional[1]` 的字面是「拒绝**只**豁免 completeness 一个 ID 的形态」；
dev 取了更强的「**含** completeness 即拒」并申报。

**「窄读堵不住」这个前提我没有采信，而是把实现改成窄读实测**：

```go
// 窄读：只拒绝 SkipChecks 恰好等于 ["completeness"]
if len(ex.SkipChecks) == 1 && ex.SkipChecks[0] == checkCompleteness {
```
```
cfg.validate() 是否拒绝 ["completeness","yoy_sanity"] ? false
残缺期次（只有 1 个字段）⇒ rep.Passed=true  failed闸门数=0  ⇒ 会进权威表? true
```

⇒ **窄读满足 DoD 的字面，却达不到 DoD 的目的** —— 同一个洞换个写法就绕过去了。
强规则**同时**满足字面判据（单元素集被拒）与意图。**加强正当。**

**代价已写清**（`CONTRACTS.md:455`）：

> **#2 的代价（已知，登记备查）**：现在**完全不能**豁免 `completeness`。若 M1b-4 出现
> 「模板变更导致某几个字段合法消失、需要单独放行 completeness」的真实需求，
> 需要的是更细的粒度（例如按字段豁免）而不是放开这条校验。

### 3.1 回归：既有豁免测试**全部未变红**（DoD boundary[0] 最容易写坏的一格）

```
--- PASS: TestThresholdsRejectMalformedExemptions        （4 子测试全 PASS）
--- PASS: TestExemptionForMatchesPeriodAndCheckExactly
--- PASS: TestCaliberExemptionRecordsSkipNotPass
--- PASS: TestCaliberExemptionDoesNotLeakToOtherPeriods
--- PASS: TestExemptionRejectsUnknownCheckID
--- PASS: TestReportKeepsEveryGateUnderExemption
--- PASS: TestThresholdsRejectWholePeriodSkip            （4 子测试全 PASS）
--- PASS: TestCheckCompletenessIDMatchesGates
```

它们用的 `SkipChecks` 是 `[deposit_sum]` / `[deposit_sum,stock_continuity]` /
`[monetary_hierarchy]` / `[monetary_hierarchy,yoy_sanity]` —— 既不含 completeness
也不覆盖全部，故不受新校验影响。**M3 消融**（把集合覆盖换成 `len > 5`）⇒
`六道闸门（不含 completeness）仍合法` 立刻转红，证明「不是数量阈值」这条被守住。

---

## 4. 事项④ —— 弱断言（第 10 次、且是新形态）：**修对了**

原断言 `Contains(err, "整期跳过")` 的问题是：completeness 那条规则的文案里**也含这四个字**。

**M1 实测**（去掉「覆盖全部」校验）：

```
Error: "hestia: caliber_exemptions[0] (2025-01) 不得豁免 completeness: …豁免它等价于整期跳过校验…"
       does not contain "跳过了全部"
Messages: 必须由『覆盖全部』那条规则拒绝——只断言共有的「整期跳过」会被 completeness 规则冒名满足

--- FAIL: TestThresholdsRejectWholePeriodSkip/枚举全部闸门即整期跳过     ← 自己转红了
--- PASS: TestThresholdsRejectWholePeriodSkip/只豁免_completeness_也是整期跳过
--- FAIL: TestThresholdsRejectWholePeriodSkip/两种拒绝理由必须可区分
--- PASS: TestThresholdsRejectWholePeriodSkip/六道闸门（不含_completeness）仍合法
```

输出本身就把机制演示出来了：**全 ID 输入确实落到了 completeness 规则上并报错** ——
旧断言会被它满足，新断言钉的是「跳过了全部」这截**只有正确规则才产生**的文案。**修对了。**

### 4.1 我追加的 M5：验证 dev「顺序不可颠倒」的**理由**

dev 的 `decisions[1]` 声称两条校验的先后不可颠倒（全 ID 集必然也含 completeness，
顺序反了会用**次要**理由掩盖**主要**事实）。这是一个「因为 X 所以 Y」，我把它变成实验：

```
M5 两条校验顺序对调 ⇒ KILLED
   「枚举全部」子测试转红，报出的正是 "不得豁免 completeness"（次要理由）
```

⇒ **理由成立**，且现有断言正好能抓住顺序错误 —— 这是加强断言带来的额外收益。

---

## 5. 事项⑤ —— PASS 计数：**不是「三种口径」，是两种口径 × 两棵树**

我在两棵树上分别数了：

| 口径 | @`4547631` | @`722aa27` |
|---|---|---|
| A：`^\s*--- PASS`（任意缩进） | **475** ← QA 的数 | **481** ← Leader 的数 |
| B：`^--- PASS`（仅顶层） | 226 | 228 |
| C：`^    --- PASS`（仅一级子测试） | 237 | 241 |
| B + C | 463 | **469** ← dev 的数 |
| 二级嵌套子测试（8 空格） | — | **12** |

⇒ **QA 的 475 与 Leader 的 481 是同一口径（A）在不同树上的结果**，
差的 6 恰是 TASK-008 新增的 6 条 PASS（`TestThresholdsRejectWholePeriodSkip` 1+4 与
`TestCheckCompletenessIDMatchesGates` 1，我逐条数过：`481 − 6 = 475` ✓）。
dev 的 469 = B+C，不含 12 条二级嵌套子测试。

**这个区分要紧**：「同一口径、不同树」是**树漂移**的信号，正是 AD-29 关心的东西；
把它归因成「口径差异」会把一个真信号读成噪声。本例中差异已被完整解释（+6 全部来自新测试），
**不是漂移问题** —— 但结论应当来自核对，而不是来自「反正口径不同」。

（权威判据始终是 `go test` 的 `ok`：我实测 FAIL 行数 = **0**。）

---

## 6. 事项⑥ —— 「两条追加不做消融」的理由：**成立，但要补一句**

**成立的部分**：我逐行核对 diff —— R2-14 的 `+8/-2` **全部落在 `knownCheckIDs` 的文档注释块内**，
无一行代码；R2-13 只改 `CONTRACTS.md`。⇒ **确无可施加变异的生产逻辑**，
对注释做变异会退化成 M9 型无效实验（变异判据而非被试）。理由正确。

**要补的一句**：「不做消融」**不等于**「不做验证」。注释与文档的内容是**可证伪的事实主张**，
正确的替代手段是**核实它所声称的事实**——
- R2-14 的两个方向：dev 直读 `checkEnum` 确认，我用探针实测（§2.1）
- R2-13 的三行数字：dev 独立复跑，我独立复跑 + 加对照组（§2.2）

两者都做了。**所以这不是「没验证」，是「换了正确的验证手段」** —— 这个区分值得写进方法论，
免得下次有人拿「注释无法消融」当成跳过验证的理由。

---

## 7. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融（致红子测试） | 判定 |
|---|---|---|---|---|
| functional[0] | 拒绝 `SkipChecks` 覆盖全部 `knownCheckIDs()` | `TestThresholdsRejectWholePeriodSkip/枚举全部闸门即整期跳过` | **M1** ⇒ 该子测试 + `两种拒绝理由必须可区分` | PASS |
| functional[1] | 同时拒绝只豁免 `completeness` | `…/只豁免 completeness 也是整期跳过` | **M2** ⇒ 该子测试 + 可区分那条；**M4/M4'** 常量脱节 ⇒ 同上 + `TestCheckCompletenessIDMatchesGates` | PASS |
| functional[2] | `CONTRACTS.md` 四项登记 | — | 逐节读过：①结构性根因 + 「留给 M1c」扩为五件 ②豁免三条边界含 R1-1 标注「M1b-4 必须定案」③ULP 守卫现状与**归因（DoD 欠规格而非实现偏离）**④`StockContinuityMax`「重新标定解决不了」 | PASS |
| boundary[0] | 合法部分豁免仍通过；既有测试不变红；新增「六道仍合法」 | `…/六道闸门（不含 completeness）仍合法` + 回归 8 条 | **M3** 换数量阈值 ⇒ 该子测试转红（`判据是集合覆盖关系而不是数量阈值；这里变红说明堵宽了`） | PASS |
| error_handling[0] | 两种拒绝理由各自可区分、指出第几条、说清为什么 | `…/两种拒绝理由必须可区分` + 两条断言各钉可区分文案 | **M1/M2/M5** 均由它参与致红 | PASS |
| non_functional[0] | 各做一次消融并核对致红断言；harness 带编译闸 + 计数自证 | — | 我独立跑 6 条（M1/M2/M3/M4/M4'/M5），四闸 + 计数自证三批 ✓ | PASS |
| non_functional[1] | 别把理由写强；注明哪些复核过哪些是转述 | — | discovery 与 CONTRACTS 均逐处标注（「本任务作者已复核」/「QA 实测、我未独立复跑」）；`key_findings[3]` 明确区分「代码层事实已直读」与「行为数字是转述」 | PASS |
| non_functional[2] | gofmt/vet/`-race`/`go build`/覆盖率 ≥ 92.0% | — | §1 实测 92.1% | PASS |

---

## 8. 两点观察（不影响判定）

### 8.1 置换盲区表缺一行对照

CONTRACTS 的三行结果都是 `Passed=true, failed=0` —— 而**对照组（完全不互换）也是** `true/0`
（我实测）。表的真实发现是「**与干净基线无差异**」，不是「failed=0」本身。
正文的机制解释（加总是置换不变量）是对的，只是表若加一行「对照（不互换）」会更难被误读。
**纯表述建议，交 QA 或 M1c 时顺手。**

### 8.2 「`dev_done` 之后继续改」本 Sprint 发生了两次，且第二次连派单都没跟上

第一次（e45164d → e6395619）dev 主动申报了，派单转述了。
**第二次（e6395619 → 722aa27）派单未提** —— 是 `verify_baseline` 机制把它拦下的。
这正是 AD-29 立项时说的场景（「dev 在 dev_done 之后又提交了一版，没有任何机制会告警」），
只是这次**有**机制、也**响**了。本例实质无碍（+3 行文档），但流程上说明：
**申报依赖人记得，机制不依赖。** 建议把「派验前重取一次 HEAD 与 discovery sha」写进 Leader 的派单动作。

## 9. 未据以判不通过的项

- validator 的 4 条 `scope-writes-outside-packages`（AD-035-4 已知假阳），整体 `exit=0`（8 个任务）。
- dev 申报未跑 `npx gitnexus analyze`（仓库级操作，同 TASK-007 留给 Leader）。

## 10. 主工作区完整性

```
b18c876007b987fc37ab396151f98f44899679f595236b22265764359fe9c0ad  thresholds.go
eef63f9eb46f6e298444be14218cecee6b8bcf4fbd08397816a836a079edd8c3  thresholds_test.go
aefe1c1d808964e03694f469fe83f9f0080a39edb37d7eab08111c90ed81d8f8  validate.go
a1f9179f217921fec2a1e6f360224bb8e429cd041ff1d2593b3c450accfaf2f6  CONTRACTS.md
$ git status --porcelain  → 仅 .arcforge/ 条目
```

与开工时逐字节一致；全部变异只在 `/tmp/mut-008` 内进行。

---

## 结论：**VERIFIED**（经显式 ack 漂移）

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对致红归因；
6 个变异全 KILLED，计数自证三批一一对应。

Leader 点名的六件事逐条成立，其中三处我给出了比派单更进一步的结果：
**③ 的「窄读堵不住」我改成窄读实测过**（`["completeness","yoy_sanity"]` 确实放行残缺期次进权威表）；
**④ 我追加 M5 验证了「顺序不可颠倒」这个理由本身**；
**⑤ 三个 PASS 数的真实成因是两种口径 × 两棵树，不是三种口径** —— 这个区分关系到别把树漂移读成噪声。

**另需 Leader 注意**：本次派验存在**未被告知的第三轮交付**（§0），
`verify_baseline` 机制按设计拦下并已由我显式确认；判定对象取的是真实交付树 `722aa27`。
