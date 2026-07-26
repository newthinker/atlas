# TASK-005 验证报告 — EDGAR XBRL 分部营收解析器

- **验证者**: test-agent-7 (Reality Checker)
- **被验对象**: commit `ee65b69` / package `./internal/collector/edgar`
- **assignment_epoch**: 1
- **判定**: ✅ **PASS (verified)** —— 附 2 项测试健壮性发现（见 §6），均**不影响功能正确性**
- **纪律**: 结论只锚定我本人实跑输出；未采信 dev-agent-15 自述。
  全部实验在隔离 worktree（checkout `ee65b69`）内进行，**全程未触网**，
  验完 `git worktree remove`，主工作树 edgar 包零残留。

---

## 1. 实跑证据

| 命令 | 结果 |
|---|---|
| `GOTOOLCHAIN=local go test ./internal/collector/edgar/ -count=1 -cover` | `ok  coverage: 93.1% of statements` |
| `... -count=5` | `ok` 全绿 |
| `GOTOOLCHAIN=local go vet ./internal/collector/edgar/` | 无输出 |
| `GOTOOLCHAIN=local go build ./...` | OK |

**触网隔离**：`segments_test.go` 内 `sec.gov` 仅出现在 2 处注释，无任何真实 URL；
全部请求经 `httptest` 双 server + `NewWithBaseURLs` 注入。
另：`NewWithBaseURL`（单 host）刻意把 `archivesURL` 也指向同一 host，
使既有测试构造的 client 即便调用 `FetchSegmentRevenue` 也打不到真实 www.sec.gov —— 这是个好设计。

---

## 2. 变异测试（本次验证的核心手段）

DoD 的多数条款是「某类干扰必须被排除」。**这类要求无法靠"测试通过"证明**——
测试可能因为别的原因通过。故我对每条护栏各做一次变异，看既有测试套是否变红。
共 15 个变异体，在隔离 worketree 内逐个施加、跑全量、`git checkout` 还原。

### 2.1 被抓住（10 个，护栏 load-bearing ✅）

| 变异 | 破坏的护栏 | 变红的测试 |
|---|---|---|
| M1 关联改为「边读边查」（假设 context 先于 fact） | AD-11 流末关联 | `FactBeforeContext`, `MultiPeriod` |
| M4 tag 优先级改为「首次出现者胜」 | tag 链优先级 | `TagPriority`, `SingleAxisOnly` |
| M5 期间下界改开区间 `days > 70` | 70 端点 | `PeriodFilter` |
| M6 期间上界改开区间 `days < 380` | 380 端点 | `PeriodFilter` |
| M7 重述择新改为「按处理顺序后者胜」 | FilingDate 择新 | `LaterFilingWins` |
| M8 回看上限 12→20 | AD-11 回看上限 | `LookbackCap` |
| M9 archives host 不带 UA | 双 host UA | `RequestFlow` |
| M12 member 不去命名空间前缀 | localName 截断 | 7 个测试 |
| M13 submissions 不筛 form | form 筛选 | `RequestFlow` |
| M14 不筛 `reportDate > since` | since 增量 | `RequestFlow` |

**M1 尤其关键**：它证明 `TestFetchSegmentRevenueFactBeforeContext` 不是摆设。
该测试还**断言了 fixture 前提本身**：
```go
require.Less(t, factAt, ctxAt, "fixture 前提:该 fact 必须排在其 context 之前,否则本用例是空洞的")
```
这正是把「不假设 context 先于 fact」这条**否定式要求**转成**可证伪条件**的正确做法 ——
后人若重排 fixture，测试会当场报错而不是静默失效。

### 2.2 等价变异（3 个，非缺口）

| 变异 | 为何等价 |
|---|---|
| M2b `len(Measures) == 1` → `>= 1` | `<divide>` 单位的 measure 是**孙元素**，直接子 measure 数为 **0**，`==1` 与 `>=1` 对它同为 false；其余单位恰好 1 个 |
| M10 instant 判定去掉 `StartDate == ""` 一半 | instant 型 context 既无 `startDate` **也无** `endDate`，剩下的 `EndDate == ""` 仍然拦住 |
| M11 上限判定挪到 `parseErr` 之后 | 实测 3 个截断点（元素边界 3533B / 512B / fact 前 8140B）**全部**返回 `XML syntax error: unexpected EOF` ——`encoding/xml` 因根元素 `<xbrl>` 未闭合，任何截断都必然报错，「截断却解析成功」在本实现下不可达 |

M11 的防御性排序（先判上限再判解析错）本身是对的，注释里的理由也成立，只是
在 `encoding/xml` 下该场景不可构造。**无需改动**。

### 2.3 存活且为真实缺口（2 个）→ 见 §6

---

## 3. Done Criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 变异验证 | 判定 |
|---|---|---|---|---|
| **F0** | submissions 双重筛选（form + reportDate>since）；AD-3 实例 URL 推导（`/Archives/` 大写 A、CIK 整数、accession 去连字符、`.htm→_htm.xml`） | `RequestFlow`（逐路径全等断言） | M13/M14 均被抓 ✅ | **PASS** |
| **F1** | 只收单维且轴匹配的 context；交叉维排除；轴不匹配排除；instant 排除；member 去前缀 | `SingleAxisOnly` | M12 被抓 ✅；**M3 存活**（§6.1）；轴不匹配经 M3 实验确认仍被排除 | **PASS**（功能已由我直接验证） |
| **F2** | 期间过滤 70~100 / 350~380，274d 累计期排除 | `PeriodFilter`（**9 个成员四端点对偶 + 366 闰年**） | M5/M6 均被抓 ✅ | **PASS** |
| **F3** | AD-2 一份 10-Q 产 **2 条** SegmentPeriod，各自 Period 正确 | `MultiPeriod`（`require.Len(got,2)` + 逐期数值） | M1 被抓 ✅ | **PASS** |
| **F4** | AD-11 单位过滤：非 USD（含 `<divide>` USD/share）排除 | `UnitFilter` | **M2a 存活**（§6.2）；功能经我直接验证正确 | **PASS**（同上） |
| **B0** | instant 安全跳过不 panic；跨报告取 FilingDate 更晚者；回看上限 12 | `SingleAxisOnly` / `LaterFilingWins` / `LookbackCap` | M7/M8 被抓 ✅ | **PASS** |
| **E0** | 404 跳过可观测不中断整家；64MB 上限超限跳过不 OOM；submissions 非 200 返回带 CIK/URL 的 error | `InstanceNotFound` / `BodyLimit` / `SubmissionsError` / `MalformedInstance` / `RaggedSubmissions` | — | **PASS** |
| **N0**(review) | 单遍解析、禁全树、双 host UA、无并发、`NewWithBaseURLs` 且既有测试零改动、§1.2 规则注释保留 | 见 §4 | M1/M9 被抓 ✅ | **PASS** |

**7 组 done_criteria 全部满足。**

---

## 4. `non_functional`（verify_by: review）逐项核实

| 要求 | 核实方式 | 结论 |
|---|---|---|
| 单遍 `Decoder.Token()` 遍历 | 代码为单个 `for { dec.Token() }`，`DecodeElement` 只解单个 context/unit 子树（各几百字节） | ✅ |
| 禁止 `Unmarshal` 建全树 | `grep 'ReadAll\|xml.Unmarshal'` → **零匹配** | ✅ |
| 不假设 context 先于 fact | contexts/units/pending 三者分别收集，循环**结束后**统一关联；M1 变异被抓住 | ✅ |
| 两个 host 都带 UA | 两处请求共用 `httpGet`，其中无条件 `req.Header.Set("User-Agent", …)`；M9 变异被 `RequestFlow` 抓住 | ✅ |
| 请求间无并发 | `grep 'go func\|errgroup\|WaitGroup'` → **零匹配**，`for _, f := range filings` 串行 | ✅ |
| `NewWithBaseURLs` 双 host + `NewWithBaseURL` 语义不变 | `TestSegmentClientConstructors` 三构造器断言；`client_test.go` **本次 commit 未修改**，**17 个既有测试函数逐函数 md5 全部未改动** | ✅ |
| §1.2 解析规则注释完整保留 | `segments.go` 文件头 1~37 行完整保留 7 条规则，并标注 live 实测校准 | ✅ |
| **零值陷阱排查**（我主动加的） | 三个构造器全部汇入 `NewWithBaseURLs` 并设 `maxInstanceBytes: defaultMaxInstanceBytes`；若某构造器漏设，`io.LimitReader(body, 0)` 会让**每一次**抓取都判为超限 —— 实际无此问题 | ✅ |

---

## 5. live 数据对账

fixture 数值取自真实文档，测试逐值断言（`InDelta ±1`）：

| 期间 | 分部 | 断言值 | leader 提供的 live 基准 | 一致 |
|---|---|---|---|---|
| 2026-01-01~03-31 | Productivity | 35,013,000,000 | $35.013B | ✅ |
| 同上 | IntelligentCloud | 34,681,000,000 | $34.681B | ✅ |
| 同上 | MorePersonalComputing | 13,192,000,000 | $13.192B | ✅ |
| 2025-01-01~03-31 | IntelligentCloud | 26,751,000,000 | （去年同季） | ✅ |
| 同上 | Productivity | 29,944,000,000 | （去年同季） | ✅ |

**FY2025 年报基准（$120.810B / $106.265B / $54.649B）未被任何 fixture 使用** ——
DoD functional[3] 只要求 10-Q 多期场景，故不构成缺口；
FY 长度期间的处理由 `instance_periods.xml` 的 350/366/380 天样本覆盖。
若后续想验证「一份 10-K 含 3 个财年」，需另造 10-K fixture（**非本任务 DoD 范围**）。

---

## 6. 两项测试健壮性发现（功能正确，但断言不具判别力）

两者**同属一类**：断言写了、也通过了，但**通过的原因不是它想验证的那条规则**。
我已分别用独立探针确认**生产行为本身正确**，故不构成 reject。

### 6.1 交叉维排除的断言是**顺序依赖**的（M3 存活）

`instance_mini.xml` 中，合法 fact `C_q3_ic`（34.681B，偏移 7684）排在
交叉维 fact `C_cross`（12B，偏移 8842）**之前**，且两者用**同一个 tag**（优先级同为 0）。
采纳逻辑 `if prev, seen := adopted[mk]; seen && prev <= f.priority { continue }` 是
**先到先得**，于是即便交叉维 context 被错误收录，其值也永远轮不上。

实测三方对照：

| 场景 | IntelligentCloudMember 解析值 |
|---|---|
| 原代码 + 原 fixture | 34,681,000,000 ✅ |
| **M3 变异（允许交叉维）+ 原 fixture** | **34,681,000,000** ← 断言仍通过，缺陷漏网 |
| 原代码 + 交叉维 fact 前置 | 34,681,000,000 ✅（**证明护栏本身正确**） |
| M3 变异 + 交叉维 fact 前置 | **12,000,000,000** ← 此时既有测试套确实变红 |

即 `assert.NotEqual(t, 12000000000.0, …, "交叉维 context 必须排除")` 目前
**无论交叉维护栏在不在都会通过**。

**独立验证功能正确性**：我另造探针（干扰 fact 与合法 fact 同 tag、干扰在前），
原代码返回 34681 —— **交叉维排除确实生效，DoD functional[1] 满足**。

**建议修法（1 行）**：把 `C_cross` 的 fact 移到 `C_q3_pbp`/`C_q3_ic` 的 fact **之前**。
dev 已在 tag 优先级（"低优先级的 Revenues 排在前面"）和 fact/context 逆序两处
主动用过这个手法，此处只是漏了一致应用。

### 6.2 单位过滤的断言被 **tag 优先级掩蔽**（M2a 存活）

`C_prior_pbp` 上挂 4 条营收 fact，但三条干扰项**全部**用低优先级 tag：

| unitRef | 所用 tag | 优先级 |
|---|---|---|
| `U_EUR` | `us-gaap:Revenues` | 1 |
| `U_UnitedStatesOfAmericaDollarsShare` | `us-gaap:Revenues` | 1 |
| `U_undeclared` | `us-gaap:Revenues` | 1 |
| `U_USD`（合法） | `RevenueFromContractWithCustomerExcludingAssessedTax` | **0** |

合法值靠 **tag 优先级** 就已稳赢，**与单位过滤是否生效无关**。
实测：把单位过滤整个关掉（`if true { usdUnits[xu.ID] = true }`），
`TestFetchSegmentRevenueUnitFilter` **依然全绿**。

**独立验证功能正确性**：我另造探针（4 条 fact **同 tag**、干扰在前），
原代码返回 **29944**（U_USD）—— EUR、`<divide>` USD/share、未声明单位
**三者确实都被排除**，AD-23① 的天真判定陷阱确实被避开，DoD functional[4] 满足。

**建议修法（3 行）**：把三条干扰 fact 的 tag 改成与合法 fact 相同的
`RevenueFromContractWithCustomerExcludingAssessedTax`，并置于其前。

#### 6.2.1 直接回答「`<divide>` 陷阱 fixture 是否真能让天真实现失败」

leader 明确要求确认这一点（"不是摆着好看"）。我照 AD-23① 描述的**天真实现**做了变异 ——
把 `<divide>` 的孙元素 measure 也纳入判定（`xml:"divide>unitNumerator>measure"`），
任一 measure 为 USD 即当作 USD 单位：

```
天真实现下 prior 期 Productivity = 29944000000.00   （合法 29944000000 / 每股值 3.24）
既有测试套结果: ok   ← 全绿，未变红
```

**结论：不能。该陷阱 fixture 目前确实"摆着好看"。**

机制：天真实现**确实**把 `U_UnitedStatesOfAmericaDollarsShare` 误判成了 USD 单位 ——
陷阱本身构造是对的。但那条每股值 fact 用的是 `us-gaap:Revenues`（优先级 1），
而合法 fact 用 `RevenueFromContractWithCustomerExcludingAssessedTax`（优先级 0），
**误判进来的 3.24 在 tag 优先级上直接出局**，永远浮不到结果里。

这与 §6.2 的「单位过滤整体关闭」实验（M2a）互为印证：后者是更宽松的变异，同样存活。
两者共同说明——**遮蔽点不在单位判定，而在 tag 优先级的先决淘汰**。

**这使 §6.2 的修法成为必需而非可选**：把三条干扰 fact 的 tag 改成与合法 fact 相同、
并置于其前。改后天真实现会取到 3.24，`UnitFilter` 立即变红，陷阱才真正生效。
（生产实现始终正确 —— 见 §6.2 的同 tag 探针，返回 29944。）

---

## 7. 判定

**verified（PASS）。**

理由：7 组 done_criteria 全部满足；15 个变异体中 **10 个被既有测试抓住**、
3 个为等价变异（非缺口）、2 个存活项**经我独立探针确认生产行为正确**，
故所有 DoD 描述的行为均已被证实成立；live 数值逐条对账吻合；
既有 `client_test.go` 17 个测试函数逐函数 md5 确认零改动；
单遍解析/禁全树/双 host UA/无并发/构造器零值陷阱等 review 项逐条核实通过；
覆盖率 93.1%；全程未触网。达到「压倒性证据」标准。

§6 两项为**测试健壮性**问题而非功能缺陷 —— 护栏在、行为对，只是断言在当前
fixture 下不具判别力，将来若有人破坏这两条护栏，测试不会变红。
合计 4 行 fixture 改动即可闭合，建议如 TASK-006 §8.1 的先例，指派到后续任务修复。

---

## 附录 A：TASK-014 验收用的两个变异体（可直接复现）

leader 已把 §6 两项并入 **TASK-014**，并定验收标准为
**「改后这两个变异体真的会被杀死」**（而非「改了 fixture」）。
为使该标准可执行而非只能凭描述推导，此处给出确切配方。

**前置**：在隔离 worktree 内操作，**还原一律用 `git checkout --`，不要自建快照**；
每次施加变异前先确认基线全绿（否则分不清「变异被抓住」与「基线本来就红」）。

```bash
WT=<scratchpad>/wt-014
git worktree add -f $WT <被验 commit>
cd $WT
SEG=internal/collector/edgar/segments.go
GOTOOLCHAIN=local go test ./internal/collector/edgar/ -count=1   # 必须先 ok
```

### 变异 A —— 交叉维护栏失效（对应 §6.1）

```bash
git checkout -- internal/collector/edgar/
python3 -c "
s=open('$SEG').read()
s=s.replace('if len(xc.Members) != 1 || localName(strings.TrimSpace(xc.Members[0].Dimension)) != axis {',
            'if len(xc.Members) < 1 || localName(strings.TrimSpace(xc.Members[0].Dimension)) != axis {')
open('$SEG','w').write(s)"
GOTOOLCHAIN=local go test ./internal/collector/edgar/ -count=1
git checkout -- internal/collector/edgar/
```
语义：`!= 1` → `< 1`，使**首个 member 落在分部轴上的交叉维 context 被错误收录**
（0 member 的合并口径 context 仍被拦下，不会越界）。
- **修复前（本次交付）**：`ok` —— 存活，说明断言无判别力
- **TASK-014 验收要求**：必须变红，且失败的应是 `TestFetchSegmentRevenueSingleAxisOnly`

### 变异 B —— 单位判定退化为天真实现（对应 §6.2 / §6.2.1）

```bash
git checkout -- internal/collector/edgar/
python3 - <<'PY'
p='internal/collector/edgar/segments.go'
s=open(p).read()
s=s.replace('''type xmlUnit struct {
	ID       string   `xml:"id,attr"`
	Measures []string `xml:"measure"`
}''','''type xmlUnit struct {
	ID          string   `xml:"id,attr"`
	Measures    []string `xml:"measure"`
	NumMeasures []string `xml:"divide>unitNumerator>measure"`
}''')
s=s.replace('''			if len(xu.Measures) == 1 && localName(strings.TrimSpace(xu.Measures[0])) == "USD" {''',
'''			all := append(append([]string{}, xu.Measures...), xu.NumMeasures...)
			naiveUSD := false
			for _, m := range all {
				if localName(strings.TrimSpace(m)) == "USD" {
					naiveUSD = true
				}
			}
			if naiveUSD {''')
open(p,'w').write(s)
PY
GOTOOLCHAIN=local go test ./internal/collector/edgar/ -count=1
git checkout -- internal/collector/edgar/
```
语义：把 `<divide>` 的孙元素 measure 也纳入判定 —— 即 AD-23① 所述
「measure 含 USD 就算数」的天真判定，会把 `U_UnitedStatesOfAmericaDollarsShare`
（USD/share，每股口径）误判为营收单位。
- **修复前（本次交付）**：`ok` —— 存活（§6.2.1 实测）
- **TASK-014 验收要求**：必须变红，且失败的应是 `TestFetchSegmentRevenueUnitFilter`

### 附带建议：一并确认「补充变异 C」不回归

修 §6.1 时把 `C_cross` 的 fact 前移后，需确认**没有把 tag 优先级测试搞坏**——
即变异「tag 优先级改为首次出现者胜」仍应被 `TestFetchSegmentRevenueTagPriority` 抓住：
```bash
python3 -c "
s=open('$SEG').read()
s=s.replace('if prev, seen := adopted[mk]; seen && prev <= f.priority {','if _, seen := adopted[mk]; seen {')
open('$SEG','w').write(s)"
```
本次交付下该变异**被抓住**（M4），修 fixture 后应保持被抓住。
