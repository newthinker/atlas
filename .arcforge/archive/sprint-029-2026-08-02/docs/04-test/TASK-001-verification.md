# TASK-001 验证报告 —— EDGAR 多 unit tag 确定性选择

- **验证者**：test-agent-10
- **被验任务**：TASK-001，`assignment_epoch=2`，owner `dev-agent-27`
- **承接/裁决时 HEAD**：`02598e02de27cc76309c9e42b0e7a876c3ff97a9`（两次核对一致，未变）
- **分支**：`feature/TASK-001-edgar-multiunit`
- **裁决**：**VERIFIED（PASS）**

心智模型声明：默认判定 NEEDS WORK。本报告中每一条 PASS 都锚定我**本人实际运行**的命令输出，
dev 与 leader 的既有结论一律当作待检验的断言而非依据。全部 9 条 done_criteria 独立复核通过，
另有 5 个我自行设计的变异全部 KILLED，未发现任何阻断性缺陷。

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 变异佐证 | 判定 |
|---|---|---|---|---|
| functional[0] | 多 unit 确定性选择，与 map 插入顺序无关 | `TestSelectUnitIsPermutationInvariant`（6 case × 全排列）+ `TestFetchCompanyFactsMultiUnitSelectsDeterministically`（32 次重复） | M1/M2 10/10；**我加的 M7/M8/M9 各 10/10** | **PASS** |
| functional[1] | Q4 EPS = FY − ΣQ1..Q3 | `TestFetchCompanyFactsMultiUnitDerivesQ4EPS` | M3 10/10 | **PASS**（锚点值须更正，见 §3） |
| functional[2] | `epsTags` 回退链对首选 tag 缺席生效 | `TestFetchCompanyFactsTagFallback`（既有，tag 整个缺席）+ `TestFetchCompanyFactsMultiUnitFallbackChain`（新，units 全空 + 回退 tag 自身多 unit） | **我加的 M4b 双红 10/10、M10 10/10** | **PASS** |
| functional[3] | 单 unit（AVGO 形态）回归，与改前逐值一致 | `TestFetchCompanyFactsMultiUnitMatchesSingleUnit` + 9 个 golden 基线逐字节未变 | M6 红 23 个测试 10/10 | **PASS** |
| boundary[0] | 跨 unit 同期间冲突值行为明确定义并断言 | `TestFetchCompanyFactsMultiUnitConflictingPeriods`（哨兵 99.9/88.8/77.7 全季不得出现） | M5（竞争实现：合并全部 unit）10/10 | **PASS** |
| error_handling[0] | 全部 unit 无数据 → 视为 tag 缺失，不 panic、不产 ±Inf | `TestFetchCompanyFactsMultiUnitFallbackChain` + `assertNoInf` 逐字段 | 由 M4b 一并覆盖 | **PASS** |
| non_functional[0] | AD-27 变异测试 | 见 §2 | 11 个变异全 KILLED 10/10，对照组 0/10 | **PASS** |
| non_functional[1] | 覆盖率 ≥93.8%、`-race`、golden 变动须说明 | 94.1%（×8 采样全同）、`-race -count=5` 绿、全仓绿、golden 零变动且**可证明** | — | **PASS** |
| non_functional[2] | live：COST/CRM/WMT degraded→ok | 见 §4，我自建 HEAD 二进制 + 全新 DB 独立复现 | live 对照组同环境复现 4 degraded | **PASS** |

---

## 2. 变异测试（独立重跑 + 自行扩充）

**harness 自建**（未复用 dev 的），依 AD-27 第 3/4 条逐条设防：

- **c 类（sed 未命中）**：每个变异断言 `txt.count(old) == 1`，不等于 1 立即报 `PATTERN MISS` 并跳过计数——不会把「根本没改」记成 KILLED。
- **b 类（编译失败伪装成存活）**：施加后先 `go build ./...` + `go vet`，不过则还原并记 `COMPILE FAIL`，不进入计数。M1 正因此必须整体替换比较器（保留 `a, b := ...` 会因未使用变量编译失败）。
- **a 类（还原失败导致变异累积）**：每个变异**前**断言 `sha256(client.go) == BASE_SHA`，**后**还原并再次断言逐字节相等；全部结束后 final sha 校验 + 重跑全包。
- **第五类（执行环境错误）**：**对照组与变异组跑在完全相同的档位**（同为 `go test ./internal/collector/edgar/ -count=1 -v` 全包），实测 **0/10 红**。
- **「红对地方」**：不只数红了几次，而是解析 `--- FAIL:` 收集**具体哪些用例**红，逐个核对是否落在该变异应当命中的守护上。

关于 `git diff --quiet` 不可用：**我认同 dev 的处置**。`client.go` 带有本任务未提交改动
（验证期间工作树对该文件确有改动），`git diff --quiet` 恒为假，套用它只会诱使改用
`git checkout --` 而那会抹掉整个任务产出。sha256 快照比对在有未提交改动时依然严格，
是此处唯一正确的自检方式。我的 harness 采用同一方案并**实测还原成功**。

### 2.1 dev 声称的 6 个（我独立重跑）

| 变异 | 我的结果 | 红在何处 | 与 dev 声称是否一致 |
|---|---|---|---|
| M1 `bestUnitName` 比较器恒 false | **KILLED 10/10** | 含 `TestSelectUnitIsPermutationInvariant` | 一致 |
| M2 `unitsOf` 退回 `for-range-return` 原缺陷 | **KILLED 10/10** | 5 个 MultiUnit 用例全红 | 一致 |
| M3 Q4 推导 `FY−ΣQ` → `FY+ΣQ` | **KILLED 10/10** | 含 `DerivesQ4EPS` | 一致 |
| M4 `firstQuarterlyTag` 去掉 tag 循环 | **KILLED 10/10** | Golden + RevenueTagSkipsUnusable，**未红 FallbackChain** | **形态差异，见下** |
| M5 `selectUnit` 改为合并全部 unit | **KILLED 10/10** | 含 `ConflictingPeriods` | 一致 |
| M6 `selectUnit` 对单 unit 返回 nil | **KILLED 10/10** | **23 个**用例全红，含 `MatchesSingleUnit` + Golden | 一致 |

**M4 的差异已查清，不是 dev 的问题。** 我最初的 M4 只把 `for _, tag := range tags` 换成
`tags[0]`，**保留了末行 `return firstTag(tags...)` 兜底**——而 `firstTag` 自身也遍历全部 tag，
于是回退路径仍然通畅，`FallbackChain` 当然不红。我随即补做 **M4b（连 `firstTag` 兜底一并去掉，
即真正的「去掉回退链」）**：

```
M4b-firstQuarterlyTag-only-first-tag: KILLED 10/10
  failing: MultiUnitFallbackChain:10, TagFallback:10, Golden:10,
           MainFlow:10, RevenueFallback:10, RevenueTagSkipsUnusable:10
```

**dev 关于 M4 的说法成立**；差的是我第一次的变异形态偏弱，不是守护缺失。

### 2.2 我额外设计的 5 个（超出 dev 声称范围，用于探查守护是否只是「整体绿」）

| 变异 | 结果 | 红在何处 | 探查目的 |
|---|---|---|---|
| M4b `firstQuarterlyTag` 完全去回退链 | **KILLED 10/10** | FallbackChain + TagFallback **双红** | 验证 functional[2] 两条链各自有牙 |
| M7 末级 tiebreak `a.name < b.name` → `>` | **KILLED 10/10** | PermutationInvariant | 第三级打分是否被钉住 |
| M8 删掉 `total` 这一级 tiebreak | **KILLED 10/10** | PermutationInvariant | 第二级打分是否被钉住 |
| M9 打分改用 `total` 而非 `usable` | **KILLED 10/10** | PermutationInvariant | 第一级打分是否被钉住 |
| M10 `firstTag` 只取 `tags[0]` | **KILLED 10/10** | TagFallback | tag 链另一入口是否被钉住 |

**M7/M8/M9 是本次验证的增量价值**：它们证明 `bestUnitName` 的**三级打分每一级都被单独守住**，
而非「整体一起绿」。AD-27 的教训是「红了好几个 ≠ 红对地方」；这三个变异每次都**只**红
`PermutationInvariant` 一个用例，精确落在该规则的守护上。

**合计：11 个变异，全部 KILLED 10/10；对照组同环境同档位 0/10 红；还原经 sha256 逐字节验证。**

### 2.3 结构性保证的论证（leader 要求重点核实）——**成立**

dev 的论证是：输出层断言**原理上**无法确定性杀死「取第一个 unit」，因为正确输出恰好等于
「碰巧选中 `USD/shares`」的输出；故把决策提为纯函数 `bestUnitName(units, names []string)`，
候选顺序由调用方提供，测试枚举全部排列 ⇒ `return names[0]` 必然命中一个「败者排首位」的排列。

我逐环核对，**论证成立**：

1. **打分是全序**：三级比较 `usable desc → total desc → name asc`，而 unit 名在 map 中互不相同，
   末级必然分出胜负 ⇒ 不存在并列。对全序取最大值与折叠顺序无关，故返回值与 `names` 排列无关。
   （`sort.Slice` 虽非稳定排序，但在严格全序下结果唯一，不影响该结论。）
2. **排列枚举确实覆盖全部情形**：`permutations` 为朴素递归全排列；6 个 case 中
   4 个 2-unit（各 2 个排列）、2 个 3-unit（各 6 个排列），逐一断言同一 `want`。
3. **每个 case 都存在「败者排首位」的排列**：6 个 case 的 unit 数均 ≥2，故 `names[0] != want`
   的排列必然存在 ⇒ 变异 `return names[0]` 在该排列上必然返回错误答案，**p(kill)=1，与 map 随机性无关**。

这是 AD-27「**结构性保证 > 概率性压制**」的正确落点：若把 map 迭代内联进函数内部，
该保证就退化为 `0.124^N` 的概率性压制。

---

## 3. leader 的两个待复核项

### 3.1 `functional[1]` 锚点 5.871 vs 存储 5.87 —— **leader 的解释成立，且我可以给出更强的证据**

leader 判断「实现正确、锚点算错」。**成立**。我从三个独立来源交叉确认：

**(a) 存储值逐位吻合 2dp 口径。** 直读 DB 取全精度：

```
sqlite> select printf('%.17g', eps_diluted) ... COST period_end='2025-08-31';
5.870000000000001
```

```
18.21 - (4.04+4.02+4.28)                             = 5.870000000000001   ← 逐位相同
18.21 - (4.041+4.019+4.279)                          = 5.871
18.21 - (4.04143936379922+4.01900711642983+4.27869287394157) = 5.8708606458293815
```

**(b) EDGAR 原始值确为 2dp（我直接拉了 SEC API 核实，非转述）：**

```
GET data.sec.gov/api/xbrl/companyconcept/CIK0000909832/us-gaap/EarningsPerShareDiluted.json
  {start 2024-09-02, end 2024-11-24, val 4.04,  fp Q1, form 10-Q}
  {start 2024-11-25, end 2025-02-16, val 4.02,  fp Q2, form 10-Q}
  {start 2025-02-17, end 2025-05-11, val 4.28,  fp Q3, form 10-Q}
  {start 2024-09-02, end 2025-08-31, val 18.21, fp FY, form 10-K}
```

**(c) 关键补充：DoD 里 4.041/4.019/4.279 这三个数，本身就是「缺陷产物」。**
leader 说它们来自 `net_income/shares` 推算——我在**修复前**的库里直接找到了它们：

| period_end | probe.db（修复前） | verify.db（修复后） | net_income / diluted_shares |
|---|---|---|---|
| 2024-11-24 | `4.04143936379922` | `4.04` | 1798000000 / 444891000 = 4.041439… |
| 2025-02-16 | `4.01900711642983` | `4.02` | 1788000000 / 444886000 = 4.019007… |
| 2025-05-11 | `4.27869287394157` | `4.28` | 1903000000 / 444762000 = 4.278692… |

修复前 EPS tag 整个丢失（选中了 `pure`），Q1–Q3 只能走 `NetIncome/DilutedShares` 推算兜底；
**DoD 的锚点正是从这份被缺陷污染的数据里取的**。修复后取到 EDGAR 自报的 2dp 值，
FY−ΣQ 自然得 5.87。**实现正确，锚点口径有误。**

> **更正**：`functional[1]` 的锚点应为 **Q4 = 18.21 − (4.04 + 4.02 + 4.28) = 5.87**
> （IEEE754 双精度 `5.870000000000001`），依据 EDGAR 自报 2dp 季度值。
> 原写的 `18.21 − 12.339 = 5.871` 取自修复前 `NetIncome/DilutedShares` 的推算值，不成立。
> 测试文件内部（fixture 用 4.041/4.019/4.279、断言 5.871、容差 1e-6）是**自洽**的，无需改动。

**顺带一个未在 DoD 中声明的行为变化，提请 leader 知悉**：本修复使 COST/WMT/CRM 的
**Q1–Q3 EPS 也从「NI/shares 推算值」切换为「EDGAR 自报值」**（4.04143936… → 4.04）。
方向正确（自报值权威），但它是一处 DoD 未预期的数据面变化，会轻微改动历史
`valuation_daily` 数值与百分位基线。非缺陷，仅备案。

### 3.2 dev 纠正 leader 的两处 —— **均成立**

**① map 迭代非 50/50 —— 成立，我用 10 倍样本独立复现。**
dev 报 2 万次得 `pure` 0.8764 / `USD/shares` 0.1235。我写了独立的 Go 程序，
对同一 fixture 做 **20 万次**「重新反序列化 + 迭代取首个」：

```
N=200000
  pure         first 175280 times  p=0.8764
  USD/shares   first  24720 times  p=0.1236
  (7/8 = 0.8750, 1/8 = 0.1250)
```

**四位小数与 dev 完全吻合**，且与 AD-27 已坐实的机理一致：小 map 只有 1 个 bucket，
键按**插入顺序**占据槽位 0/1，迭代起点是 0~7 的随机偏移 ⇒ 先插入的键排首位 `P = 7/8`。
`pure` 在 fixture JSON 中列在前 ⇒ 它以 0.875 排首位。**「p=0.5」确系直觉估计，dev 的纠正正确**，
且这解释了 live 中 COST/WMT/CRM 为何**几乎每次**失败而非半数失败。

测试注释里 `multiUnitRuns = 32` 的取值据此也是合理的：单次逃逸率 ≈0.1236，
`0.1236^32 ≈ 9e-30`。且每次 `FetchCompanyFacts` 都会重新反序列化 ⇒ 32 次为独立抽样，成立。

**② golden 基线未变 —— 成立，而且我认为它是「可证明」而非「碰巧」。**
dev 说 9 个 golden fixture 全是单 unit，修复对其逐字节 no-op。我做了两层核实：

1. **枚举核实**：脚本扫描 `testdata/` 全部 18 个 `companyfacts*.json`，
   **只有本任务新增的 2 个是多 unit**，其余（含全部 9 个 golden）单 unit。
2. **结构性核实**：`len(units)==1` 时 `bestUnitName` 只有一个候选，`selectUnit` 返回的
   正是旧 `for range { return facts }` 返回的同一个切片 ⇒ **逐字节相同是可证明的结论，不是巧合**。
3. `git diff HEAD~1 -- testdata/golden/` 输出为**空**。
4. golden 测试确为「比对已提交基线」，只有 `-update-golden` flag 才重生成——**不是自我重生成的空断言**。

**关于「这是否足够作为 functional[3] 的证据」**：足够，但「基线没变」单独拿出来是**弱证据**
（没变也可能因为测试根本没牙）。**使它成立的是 M6**：把单 unit 路径改坏后，
Golden 连同 `MatchesSingleUnit` 等 **23 个用例 10/10 全红**。
「基线未变 + 基线有牙」两条合起来才构成 functional[3] 的完整证据，二者我都已实测。

---

## 4. live 验证（我自建二进制 + 全新 DB 独立复现）

未复用 leader/dev 的任何二进制或 DB。`go build -o atlas-t10 ./cmd/atlas` 自 HEAD `02598e0` 构建，
config 由 `verify.yaml` 派生，`db_path` 指向**全新** `t10.db`。
**`data/prism.db` 全程确认不存在，且未被创建。**

**实验组（HEAD 代码）：**
```
prism refresh: 5 ok, 0 failed, 1 degraded
ℹ️ Prism 主源降级(已兜底):
V: edgar failed (reconstruct: valuation: insufficient positive EPS points), engine fallback ok
```

**对照组（同环境、同日、同 config 模板，唯一变量 = 把 `unitsOf` 退回原缺陷后重新编译）：**
```
prism refresh: 5 ok, 0 failed, 4 degraded
V / WMT / COST / CRM: edgar failed (...), engine fallback ok
```

因果归属确凿：**唯一变量即本修复**。（源码在建完对照二进制后即还原，sha256 逐字节验证一致。）

### 4.1 「不再报错」vs「真的算出来了」—— leader 的第 ② 层判据，我独立复核并**加强**

leader 指出 degraded→ok 只说明 reconstruct 没报错，须看行数是否从 fallback 水平跃到 edgar 水平。
**这一层成立**，而且我认为有比行数更硬的判据——**覆盖起始日**：

| symbol | 修复前 valuation 起始 / 行数 | 修复后（我的库）起始 / 行数 | price_daily 起始 / 行数 |
|---|---|---|---|
| COST | 2023-11-30 / 663 | **2016-07-26 / 2512** | 2016-07-26 / 2512 |
| WMT | 2023-10-31 / 684 | **2016-07-26 / 2512** | 2016-07-26 / 2512 |
| CRM | 2023-10-31 / 684 | **2017-08-25 / 2181** | 2016-07-26 / 2512 |
| AVGO（未坏） | 2020-03-13 / 1599 | 2020-03-13 / 1598 | 2016-07-26 / 2512 |
| V（范围外） | 2023-10-02 / 705 | 2023-10-02 / 704 | 2016-07-26 / 2512 |

**COST/WMT 的 PE 覆盖起始日从 2023-11/10（engine 兜底那两年的窗口）一路回到 2016-07-26，
恰好等于 price_daily 的首日，行数 2512/2512 = 价格日全覆盖，`pe_ttm` 无一为 NULL。**
这比「行数变大」更强：它说明整段十年价格历史都算出了 PE，而不只是多出几行。
CRM 起始 2017-08-25 稍晚，与其 EPS 历史起点一致，合理。
AVGO/V 的 −1 行差异是**跑动日期边界**（我的库最新日 2026-07-23 vs leader 的 2026-07-24），非回归。

### 4.2 Q4 EPS 缺失

| symbol | 修复前 Q4 缺失 | 修复后（我的库） |
|---|---|---|
| COST | 17/17 | **0/17** |
| CRM | 17/17 | **0/17** |
| WMT | 17/17 | **0/17** |
| AVGO | 0/8 | 0/8（未改坏） |
| V | 17/17 | 17/17（范围外） |

COST `2025Q4 / 2025-08-31` 的 `eps_diluted = 5.870000000000001`（修复前为 NULL）。

### 4.3 V 排除在范围外 —— 我独立核实，成立

```
CIK0001403161 / us-gaap / EarningsPerShareDiluted                        -> HTTP 404
CIK0001403161 / us-gaap / EarningsPerShareBasicAndDiluted                -> HTTP 404
CIK0001403161 / us-gaap / WeightedAverageNumberOfDilutedSharesOutstanding-> HTTP 404
```

三个 tag 确实全部 404 —— **回退链不是「没生效」而是无处可退**，非本缺陷所致，
V 留在 degraded 符合 DoD 预期。

---

## 5. fixture 真实性核查（AD-27 第六类：fixture 与生产不同则该形态的变异测试无效）

这是本任务最需要防的一类（守护的是生产中不存在的场景）。我拉取 SEC 生产数据逐项比对
discovery 的 `key_findings` 表：

| symbol | discovery 声称 | 我实测（live SEC，2026-07-26） | 一致 |
|---|---|---|---|
| COST | `pure[total=11 usable=3]` + `USD/shares[total=295 usable=144]` | 完全相同 | ✓ |
| WMT | `pure[total=3 usable=1]` + `USD/shares[total=303 usable=155]` | 完全相同 | ✓ |
| CRM | `pure[total=3 usable=1]` + `USD/shares[total=312 usable=155]` | 完全相同 | ✓ |
| AVGO | 单 unit `USD/shares[143]` | 完全相同 | ✓ |

**逐项吻合，无一处夸大。** 进一步核实形态本身：

- COST 的 `pure` 11 条**全部** `filed=2010-10-18`、`fp=FY`，覆盖 2007–2010 期间——
  确系「早期无量纲单位的陈旧重述」，其中 3 条 350~380 天故 `usable=3`。**与 discovery 描述一致。**
- WMT/CRM 的 `pure` 各含 1 条**可用单季**（`2009-05-01~2009-07-31, fp=Q2`），
  这正是 discovery 所说「WMT/CRM 抽中即被接受，灾难性」的来源。**属实。**
- fixture 把「COST 的 pure 含年度」与「WMT/CRM 的 pure 含单季」两种真实形态合成到一份里，
  是**两种生产形态的并集**，构成更严的最坏情况。

**一处需要透明记录的差异**：fixture 让 `pure` 的 `filed=2026-01-15` 晚于 `USD/shares`
**全部**条目；生产中 COST 的 `pure` `filed=2010-10-18` 只晚于其**对应期间（2008–2010）**
的原始申报，并不晚于 2025 年的申报。也就是说 **fixture 比生产更苛刻**。
方向是**加强**而非削弱（任何合并实现在 fixture 下必败），且它忠实保留了真正起作用的关系
——「陈旧重述条目的 `filed` 晚于同期原始申报，故在 `keepLatest` 中胜出」。
**因此 AD-27 第六类不成立**：被守护的形态在生产中真实存在。

---

## 6. 门禁复核（全部为我本人运行）

| 门禁 | 要求 | 我的实测 | 判定 |
|---|---|---|---|
| 覆盖率 | ≥93.8%（AD-27 第 1 条：多次采样） | **×8 采样全部 94.1%**，读数无抖动 | PASS |
| `-race` | 通过 | `go test -race -count=5 ./internal/collector/edgar/` exit 0 | PASS |
| 全仓回归 | 通过 | `go test ./...` exit 0，无 FAIL | PASS |
| 构建/静态检查 | 干净 | `go build ./...`、`go vet`、`gofmt -l`（空） | PASS |
| golden | 变动须解释 | 零变动，且**可证明**必然零变动（§3.2） | PASS |

关于覆盖率读数：AD-27 记载 sankey 包覆盖率本身非确定（10 次 97.5 / 2 次 97.8）。
本包**不存在**该现象——8 次采样全部 94.1%，且 94.1% 距 93.8% 门槛虽仅 0.3pp，
但读数确定，判定不受采样涨落影响。

新增代码的函数级覆盖：`selectUnit` **100.0%**、`bestUnitName` **94.7%**。
后者唯一未覆盖语句是 `client.go:285-287` 的 `if len(cands)==0 { return "" }`——
其唯一调用方 `selectUnit` 已在前面挡掉 `len(units)==0`，该分支**从唯一调用路径不可达**，
属防御性死分支，非覆盖缺口。

---

## 7. 修复完整性（我自行追加的检查）

DoD 只点名 `client.go:474`。我核查了包内是否残留同形态缺陷：

```
$ grep -rn "\.Units" internal/collector/edgar/*.go | grep -v _test.go
client.go:140:  for _, facts := range t.Units {      ← fiscalYearEndMonth
client.go:236:  （注释）
client.go:545:  return selectUnit(t.Units)           ← 已修复
```

`client.go:140` 是**另一种形态**：它遍历**全部** unit 累加众数投票（非 `return` 首个），
天然与顺序无关，且第 155 行的并列处理「票数相同取较小月份」已是确定性的
（上一 sprint TASK-018/020 的产出）。**包内「取第一个 unit」仅此一处，已修复，无残留。**

---

## 8. 发现的问题（**均非阻断**，不影响裁决）

| # | 严重程度 | 描述 |
|---|---|---|
| 1 | 轻微（文档） | `multiunit_test.go:209` 残留旧表述「单次运行只有 **p=0.5** 暴露，靠重复运行也只能压到 2^-32」，与同文件 30–43 行实测的 **0.1235** 自相矛盾。dev 向 leader 声称「已改掉测试注释里最初写的 p=0.5」，实际漏了这一处。不影响任何断言，但 AD-27 的教训正是「未经验证的直觉数字经转述后取得与实测数字相同的书面地位」，建议顺手订正。 |
| 2 | 极轻微 | `bestUnitName` 的 `if len(cands)==0 { return "" }` 从唯一调用方不可达，是新增代码中唯一未覆盖语句。可删可留。 |
| 3 | 备案（非缺陷） | 本修复顺带使 COST/WMT/CRM 的 **Q1–Q3 EPS 从 NI/shares 推算值切换为 EDGAR 自报值**（见 §3.1 末）。方向正确但 DoD 未预期，会轻微改动历史 `valuation_daily` 与百分位基线。 |
| 4 | 备案（非缺陷） | `TestFetchCompanyFactsMultiUnitConflictingPeriods` 的哨兵断言用 `assert.Greater(math.Abs(eps-stale), tol)`；若某季 EPS 为 NaN，`NaN > tol` 为 false 会误报失败。当前 fixture 四季 EPS 均非 NaN（且已 `require.Len==4`），不触发。仅为潜在脆性。 |

### 明确**不作为**判定依据的两项（依 leader 指示，仅记一笔）

1. 提交落在 `feature/TASK-001-edgar-multiunit` 而非 master——遵循 CLAUDE.md 分支规范，
   与近期 sprint 直接提 master 的做法不一致。**流程问题，非质量问题**，由 leader 另行决定。
2. `detect_changes()` / `impact` 未跑——GitNexus MCP 不在 dev 的工具集，**也不在我的工具集**。
   dev 未擅自 `npx gitnexus analyze`，处置得当。我同样无法补跑。§7 的 `grep` 完整性检查
   是我能做到的替代，但它**不等价于**调用图级的影响面分析。

---

## 9. 裁决

**VERIFIED（PASS）。**

9 条 done_criteria 全部有对应测试、断言非空洞、且经变异实测证明有牙；
11 个变异全 KILLED 10/10，对照组同环境 0/10；
fixture 形态经 live SEC 数据逐项核实与生产一致（AD-27 第六类不成立）；
live 层实验组/对照组在我自建的同一环境下分别复现 `1 degraded` / `4 degraded`；
覆盖率、`-race`、全仓回归、golden 全部达标。

`functional[1]` 的锚点值 5.871 应更正为 **5.87**（依据见 §3.1）——**这是 DoD 的口径错误，
不是实现缺陷**，实现产出的 `5.870000000000001` 与 EDGAR 自报 2dp 值逐位吻合。

§8 的 4 项均为轻微/备案项，不构成退回理由。建议 leader 顺手处理第 1 项（注释订正），
并知悉第 3 项（Q1–Q3 EPS 口径切换）。

---

**裁决时复核**：HEAD = `02598e02de27cc76309c9e42b0e7a876c3ff97a9`（与承接时一致），
`git status` 对 `internal/` `cmd/` 无改动（变异 harness 已全部还原，sha256 逐字节验证）。
