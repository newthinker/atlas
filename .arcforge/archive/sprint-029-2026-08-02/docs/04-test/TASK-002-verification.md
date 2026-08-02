# TASK-002 验证报告 —— V(Visa) 类别维度 EPS 回退

- **验证者**:test-agent-11 | **裁决:PASS(verified)**
- **被验对象**:HEAD `a73110e`(裁决前复核未变),branch master,`assignment_epoch=1`
- **方法**:一切结论均由本人重跑/重测取得;dev 与 leader 的结论一律只作**待验假设**

---

## 0. 裁决摘要

8 条 done_criteria **全部 PASS**。交付路径的行为已在**真实 SEC 数据**上端到端验证正确。

需要 leader 知悉的三点(**均不构成阻塞**):
1. dev 报「M12(28→12)KILLED 10/10」属实,但**杀死它的是字面量断言**,不是行为断言 —— 其行为用例对该常量取值**无判别力**(§4.1)。
2. 我自拟 14 条变异,**6 条存活**。逐条查证后**均非等价变异**,而是「防御性分支 + V 真实数据不可达」或「设计决策未被钉住」;不影响已交付行为(§4.2)。
3. `.claude/hooks/task-completed.sh` 在工作区被改(+8 行,另有 `.bak`)。运行时资产应对全体 agent 只读,与本任务无关,请 leader 确认来源(§8)。

---

## 1. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 可行性验证(EPS 是否存在于实例文档) | `TestVInstanceEPSIsAlwaysClassDimensioned` + 对照组 `TestVInstanceNetIncomeHasUndimensionedFact`;我另在**实时下载**的 5 份原件上复核 | **PASS** |
| functional[1] | 解析并接入取数链,fixture 须真实抓取 | `TestParseClassFactsRealTenK` / `DominantClassPicksClassA` / `IsPermutationInvariant` / `ClassEPSFallback` / `DerivesQ4` / `DoesNotMixBasic`;fixture 真实性见 §3 | **PASS** |
| functional[2] | 四家(COST/CRM/WMT/AVGO)不受影响 | `TestFetchCompanyFactsWithEPSNeverTouchesArchives`(结构性:零请求) + `UnaffectedByClassEPSPath` + 对偶 `NoEPSFixturesDoEnterFallback`;live 逐值比对见 §6 | **PASS** |
| boundary[0] | 成本显式评估 + 12/28 上限取舍 | `RespectsLookbackCap` / `CapsAreIndependent` / `SkipsPre2019NotFoundTail`;28 边界独立复核见 §2 | **PASS**(注 §4.1) |
| error_handling[0] | 降级不 panic、不让 refresh 失败 | `InstanceNotFound` / `SubmissionsError` / `MalformedInstance` / `BodyLimit` / `AllUnavailable` | **PASS** |
| non_functional[0] | 变异:每条 functional 至少一条翻红 | 我独立复现 dev 的 8 条关键变异,全部 10/10 KILLED,对照组 0/10(§4) | **PASS** |
| non_functional[1] | 覆盖率 ≥94.1% + 全包 `-race` | **5 次采样均 94.7%**;`-race -count=2` 通过;全仓 `go test ./...` exit 0;build/vet/gofmt 干净 | **PASS** |
| non_functional[2] | live 验证 | 我自建库 4 次运行,其中 2 次 **5 ok / 0 failed / 0 degraded**(另 2 次为无关网络 EOF,§6) | **PASS** |

---

## 2. ① 28 这个数字 —— 独立向 SEC 核实(leader 重点关注项)

**结论:28 属实,是数据侧硬边界,不是未经检验的天花板。**

我不复用 dev 的探测结果,直接拉 `data.sec.gov/submissions/CIK0001403161.json`,按 `reportDate` 降序对
**36 份** 10-K/10-Q 逐份 curl 其 `_htm.xml`(与 `instanceURL` 同一推导规则):

| 序号(0基) | reportDate | HTTP |
|---|---|---|
| 0 … 27 | 2026-03-31 … 2019-06-30 | **全部 200**(1.13–4.13 MB) |
| 28 | 2019-03-31 | **404** |
| 29 … 35 | 2018-12-31 … 2017-06-30 | **全部 404** |

可用的恰好是最近 **28** 份,第 29 份起连续 404,与 dev 所述完全一致。
(首轮有 4 个 URL 返回 curl code `000`,重试后均为 200 —— 代理抖动,已排除。)

**补充发现(文档衰减,低危,非阻塞):** 28 是**当日**的硬边界。每季新增一份申报后,可用集会增长,
而 cap 固定 28 ⇒ 它将从「恰好等于可用总数」变成**真实截断窗口**。届时
`classeps.go:54` 的「把这个数字调大只会多拿一批必然 404 的 URL」将不再成立(约 3 个月后)。
28 份 ≈ 7 年,仍足以覆盖 `pctl_5y` 的 5 年需求,故不影响功能,建议后续补一句注释说明。

---

## 3. ④ fixture 真实性 —— 逐字节比对(AD-27 变异坑第六类)

**结论:fixture 确系真实抓取,且忠实代表生产形态。**

- 实时重新下载 5 份原件(10-K 2 475 453 B 等),把每份 fixture 拆成顶层元素块
  (`context` / `unit` / 事实元素),逐块在原件中做**精确子串**查找:

  | fixture | 元素块 | ctx/unit/fact | 逐字节缺失 |
  |---|---|---|---|
  | instance_v_fy2025_10k.xml | 145 | 18/18/109 | **0** |
  | instance_v_fy2025q1_10q.xml | 90 | 12/7/71 | **0** |
  | instance_v_fy2025q2_10q.xml | 174 | 24/8/142 | **0** |
  | instance_v_fy2025q3_10q.xml | 184 | 24/8/152 | **0** |
  | instance_v_fy2026q2_10q.xml | 205 | 28/7/170 | **0** |
  | **合计** | **798** | | **0** |

- **全文档 vs 提取件平价**(在包内临时测试中直接调用**被测代码本身**):

  | 文档 | 全件 facts | fixture facts | 逐值一致 | dominant(全件/fixture) |
  |---|---|---|---|---|
  | 10-K | 69 | 69 | ✅ | CommonClassAMember / 同 |
  | Q1/Q2/Q3/FY26Q2 | 45/90/96/96 | 45/90/96/96 | ✅ | 同 |

  聚合后 EPS 10 条逐值一致(8.28/9.73/10.20/2.39/2.29/2.40/2.58/2.32/2.69/3.14)。

- **第六类坑不适用**:全件里 EPS/股本事实引用的 context **0 个 instant、0 个多维、轴 100% 为
  StatementClassOfStockAxis** —— 提取件与生产形态一致,不是「fixture 与生产不同」。

(临时测试文件已删除,包内 48 个文件 sha256 与验证前**逐一相同**。)

---

## 4. ② ③ 变异测试 —— 全部独立重跑

harness 自建:sha256 快照还原 + **施加后先 `go vet` 确认可编译再计数**(AD-27 §3),
每条 n=10,记「10 次中红了几次」。**对照组(no-op 变异)0/10** —— 环境本身不产生假红。

### 4.1 复现 dev 声明的 8 条关键变异:全部 10/10 KILLED

| 变异 | 红/10 | 实际杀死它的用例 |
|---|---|---|
| M12 cap 28→12 | 10/10 | `TestClassEPSAndSegmentCapsAreIndependent` **仅此一条** |
| M13 改用 maxSegmentFilings | 10/10 | `RespectsLookbackCap` + `SkipsPre2019NotFoundTail` |
| M4 `return members[0]` | 10/10 | `TestDominantClassIgnoresAlphabeticalOrder` |
| M1 去掉轴匹配 | 10/10 | `TestParseClassFactsRespectsAxis` |
| M3 候选不限 hasEPS | 10/10 | `TestDominantClassIgnoresMembersWithoutEPS` |
| M10 单份 404 即整批失败 | 10/10 | 3 条降级用例 |
| M6 触发条件恒真 | 10/10 | `NeverTouchesArchives` + `UnaffectedByClassEPSPath`(12 个子用例) |
| M7 触发条件恒假 | 10/10 | 13 个子用例 |
| M5 / M8 / M9 / M11(另测) | 各 10/10 | 见 §4.2 表 |

**③ dominantClass 的补丁确实生效**:M4「`return members[0]`」被
`TestDominantClassIgnoresAlphabeticalOrder` 稳定杀死(10/10)。该用例构造
「正确答案 CommonClassZMember 不在字母序首位」,是能杀死此变异的**必要**构造 ——
dev 对这个 TASK-001 同款陷阱的识别与修补都成立。

**⚠ 需要澄清 dev 的表述(§0 第 1 点)**:M12 只被
`assert.Equal(t, 28, maxClassEPSFilings)` 这条**字面量断言**杀死。行为用例
`TestFetchClassEPSRespectsLookbackCap` 是**自引用**的(`nFilings = maxClassEPSFilings + 12`,
断言 `Len(paths) == maxClassEPSFilings`),常量改成 12 时它同步变化、照样通过 ——
**它对常量取值没有判别力**,它验的是「只取最近 N 份」这个机制(该机制由 M13 证明有效)。
「KILLED 10/10」字面属实,但把它当作「28 这个取值被行为验证过」会高估。
28 的正当性来自数据侧(§2),现在已由我独立坐实。

**②「最老的不被请求」确有判别力**:`RespectsLookbackCap` 断言最旧一份的 URL 不出现在
请求记录中,M13 下 40 份取 12 份 → 断言长度失败,10/10 红。
**「24 个连续 404 全部尝试并跳过而 4 个可用的仍产出 EPS」也有判别力**:M10 下 10/10 红。

### 4.2 我自拟 14 条变异:8 条 KILLED,**6 条 SURVIVED**

| 变异 | 红/10 | 判定 |
|---|---|---|
| M5 去掉 isSingleQuarter 复核 | 10/10 | KILLED |
| M8 取最晚 filed | 10/10 | KILLED |
| M9 去掉按值分组 | 10/10 | KILLED |
| N8 完全跳过 earliestFiledPerValue | 10/10 | KILLED |
| M11 去掉超限判定 | 10/10 | KILLED |
| N10 先判解析错再判截断(顺序对调) | 10/10 | KILLED ← 该顺序是 load-bearing 的,已被钉住 |
| N15 dominantClass 改按**事实条数**排序 | 10/10 | KILLED |
| N16 `member==""` 守卫恒假 | 0/10 | **等价变异**(已查证:`classRawFacts(all,"")` 必返空,仅多一行日志) |
| **N1** 平票 `>` → `>=` | **0/10** | SURVIVED |
| **N12** dominantClass 用 **sum** 替代 **max** | **0/10** | SURVIVED |
| **N2** 去掉 instant context 守卫 | **0/10** | SURVIVED |
| **N3** `len(Members)!=1` → `<1`(放行交叉维) | **0/10** | SURVIVED |
| **N6** 无条件覆盖 sharesFacts | **0/10** | SURVIVED |
| **N13** tag 链改为跨 tag 合并(破坏「口径不混用」) | **0/10** | SURVIVED |

**逐条查证(AD-27:存活变异禁止默认归类为等价)**:

- **N12(最值得注意)**:discovery 的 `decisions[]` 明确主张「用 max 而非 sum,因为
  inline XBRL 重复渲染 + diluted/basic 两条 tag 会让求和按重复次数放大」。
  该主张**正确但无任何测试钉住**。我实测真实数据:

  | member | max | sum | n |
  |---|---|---|---|
  | CommonClassAMember | 2.085e9 | 1.245e11 | 68 |
  | CommonClassB1Member | 2.45e8 | 5.884e9 | 68 |
  | CommonClassB2Member | 1.2e8 | 5.392e9 | 60 |
  | CommonClassCMember | 2.9e7 | 7.56e8 | 68 |

  max/sum/count **三种排序在 V 上都选中 Class A** ⇒ 真实数据天然无判别力
  (`IgnoresAlphabeticalOrder` 每类只给 1 条事实,sum==max,故也杀不掉)。
  **非等价变异**,但不影响当前交付。建议后续补一条「某类别条数极多但单值小」的用例。
- **N1**:仅在两类别 max 股本**精确浮点相等**时行为不同(平票尾巴方向反转)。
  代码注释自述「类别名升序只是消除并列用的全序尾巴,不承载语义」,严重度低。
- **N2 / N3**:这两处是**防御性**守卫。§3 实测全文档:EPS/股本事实引用的 context
  **0 instant、0 多维**,故 V 的真实数据**永远走不到**这两个判定 ——
  与 dev 首轮 M1/M3/M4 存活**同一根因**,dev 补了三条针对性用例覆盖轴匹配与候选过滤,
  但**「恰好一个 explicitMember」这半个条件仍未被覆盖**。非等价,严重度低(守卫写法本身正确)。
- **N6**:`len(classShares)!=0` 这个保护分支的**假支**无用例(需要「有类别 EPS 但无类别股本」的
  构造)。非等价,严重度低。
- **N13**:「EPS/股本各自沿回退链取首个有条目的 tag,选中后其余不参与」在类别路径上未被钉住
  (V 只出现一个 EPS tag ⇒ 合并与取首个等价)。非等价,严重度低。

**这些存活项不构成退回理由**:DoD non_functional[0] 要求的是「每条 functional 至少一条变异
从 SURVIVED 翻 KILLED」,该要求已满足;上述均为**超出 DoD 的加测**,且无一影响已交付行为。

---

## 5. ⑤ segment 路径逐值等价 —— 已实测(不再是推断)

`recentFilings` 新增 `limit` 参数。

- **源码层结构性证明**:全仓仅 2 个调用点 —— `segments.go:128` 传 `maxSegmentFilings`
  (即原写死值),`classeps.go:347` 传 `maxClassEPSFilings`。函数体内原
  `if len(out) > maxSegmentFilings` 改为 `> limit`。参数代入后**逐字等价**。
- **实测**:建 `HEAD~1` git worktree,把同一份临时测试分别放进两边,对**真实 SEC**
  拉取 MSFT(`StatementBusinessSegmentsAxis`)与 AAPL(`ProductOrServiceAxis`)的分部营收,
  各跑 2 轮:

  ```
  prev1 vs head1 : IDENTICAL
  prev2 vs head1 : IDENTICAL
  head1 vs head2 : IDENTICAL
  prev1 vs prev2 : IDENTICAL
  ```
  (MSFT 17 期 × 3 分部、AAPL 若干期 × 6 分部,含 PeriodStart/End/FilingDate/Form/逐分部数值。)

  ⚠ **首轮出现过一次 35 vs 36 行的差异,复跑两次证伪为网络抖动**(某份实例文档取数失败被
  静默跳过 ⇒ 少一期 + 相邻期 filing_date 回退)。这与 dev 的 WMT false alarm 属同一类:
  **单份取数失败会静默改变输出且无错误**,live 对照必须多跑。dev 「无法测量」的标注是当时诚实的,
  现已由本次实测**闭合**:分部路径逐值未变。

---

## 6. live 验证(自建库,严禁指向 data/prism.db)

- config `{scratchpad}/livecheck/t11verify.yaml`(仅改 db_path)、db `t11verify.db`(全新建)。
  **全程未创建也未指向 `data/prism.db`**(核实其仍不存在)。
- 4 次运行:**第 3、4 次均 `prism refresh: 5 ok, 0 failed, 0 degraded`**。
  第 1、2 次各有一次 AVGO degraded,原因分别是 **Yahoo 价格 EOF** 与 **companyfacts EOF**
  ——两次落在**不同端点**、均为传输层 EOF,与本改动无关(V 在这两次里同样成功)。
- 每次日志均出现:`edgar: CIK 1403161 using class-dimensioned EPS from member CommonClassAMember (33 eps / 33 shares facts from 28 filings)`。
- DB 实测:

  | symbol | valuation_daily 天数 | pctl_5y | 起始日 |
  |---|---|---|---|
  | **V** | **1377** | **1125** | **2021-01-29** |
  | AVGO | 1599 | 1347 | 2020-03-13 |
  | COST | 2513 | 2261 | 2016-07-26 |
  | CRM | 2182 | 1930 | 2017-08-25 |
  | WMT | 2513 | 2261 | 2016-07-26 |

  V 的 `fundamental_q` 71 行中 **31 行有 eps_diluted**(改动前 0 行),期末 2008-06-30 ~ 2026-03-31。
  相对 engine 兜底(704–705 天 / 452–453 点 / 起 2023-10-02),**双轴严格优于**,起始日 `2021-01-29` 吻合。
- **逐值锚定 SEC 原文**:2024-12-31=2.58、2025-03-31=2.32、2025-06-30=2.69、
  2025-09-30=2.61(=10.20−(2.58+2.32+2.69),派生)、2024-09-30=2.65(=9.73−(2.39+2.29+2.40))、
  2026-03-31=3.14。另取一份**未用作 fixture** 的申报(v-20251231)独立复核:
  SEC 原文 Class A 稀释 EPS **3.03**,与库中 2025-12-31 一致。
  同季 B1=4.71 / B2=4.61 / C=12.11 —— **选错类别会得到 1.5~4 倍的错值**,
  `dominantClass` 确属 load-bearing,dev 对 B2 的 0 值陷阱的警告成立。

### 回归:四家逐值比对(用正确基线)

⚠ 我起初误用 `probe.db` 作基线,它是 **TASK-001 之前**的库,差异巨大 —— 那是 TASK-001 的效果,不是本任务的。
正确基线是 `verify.db`(TASK-001 后 = TASK-002 前)。

- 12 位有效数字口径:**249 条逐值 IDENTICAL**。
- 与 leader 的 `final.db` 比对:**IDENTICAL**。
- **全精度(不截断)口径:249 条中仅 1 条不同** ——
  `WMT 2014-01-31: 0.453333333333333 → 0.453333333333334`。

---

## 7. dev 的 WMT false alarm —— 其解释成立(我给出了比 dev 更强的证据)

dev 用「同一二进制跑三次得 ...333/...334/...334」判定为既有抖动。该解释**成立**,三条独立证据:

1. **结构性**:`applyDuration` 用 `for _, f := range singleByEnd` 遍历 **Go map** 构建 `singles`
   (client.go:608),`durationEntries` 再按该顺序 `sum += s.val`。浮点加法不满足结合律 ⇒
   `a.val - sum` 末位随遍历序抖动。**这段代码在本次 diff 中逐字节未改**
   (client.go 仅新增 24 行回退块,`applyDuration`/`durationEntries` 与 HEAD~1 完全相同)。
2. **算术性(决定性)**:WMT FY2014 EPS=4.88,三季 1.14/1.24/1.14,3:1 拆股归一化先于季度化生效。
   对 3!=6 种求和序穷举:

   ```
   0.45333333333333337   ← 2/6 种顺序   (15 位有效数字 → 0.453333333333333)
   0.4533333333333336    ← 4/6 种顺序   (15 位有效数字 → 0.453333333333334)
   ```

   **恰好就是 dev 报告的那两个值**,且 4:2 的比例与它三次跑出 ...334 两次相符。
3. **实证**:§6 的全精度比对显示,TASK-002 前后 249 条 EPS 中**有且只有这一条**不同 ——
   若是本次改动引入的回归,不可能只影响 WMT 一个 2014 年的派生 Q4 而放过其余 248 条。

**⇒ 不是回归。** 12 位有效数字的 golden 约定正是为此设立,该约定继续有效。

---

## 8. 两个 limitation 标注的诚实性判断

| 标注 | 判定 |
|---|---|
| **进程内计时不可信,采用 45 s / 52.3 MB 冷启动数字** | **诚实且判断正确。** 我的 live 全流程(5 家含 V 的 28 份)仅 31–33 s,**比 dev 的 45 s 还快** —— 因为我此前刚 curl 过 36 个同源 URL,CDN/代理已预热。这**正面印证**了 dev 担心的效应。取保守值是对的。 |
| **共享常量副作用属推断而非实测** | **诚实。** 当时 live 环境 `segment_revenue` 恒 0 行,确实测不到;而它据此选择「在测不到的地方不动共享状态」(独立常量),是**正确的保守决策**——把不确定性设计掉,而不是赌它无害。该 limitation 现已由我 §5 的 HEAD~1 对照实测**闭合**。 |

两条标注**均未影响判定**,且属于「把不确定说成不确定」的良性标注,而非用 limitation 掩盖未做的工作。

---

## 9. 门禁复核(多次采样)

| 项 | 结果 |
|---|---|
| edgar 覆盖率 | **94.7% × 5 次采样**(全部相同,≥94.1% 水位) |
| `-race -count=2` (edgar) | 通过 |
| 全仓 `go test ./...` | exit 0,无 FAIL |
| `go build ./...` / `go vet ./...` / `gofmt -l` | 全部干净 |
| 工作区完整性 | 验证前后 edgar 包 48 个文件 sha256 **逐一相同**;临时 worktree 已移除 |

---

## 10. 建议(非阻塞,交 leader 决定是否立项)

1. `TestClassEPSAndSegmentCapsAreIndependent` 的注释宜说明:28 的**行为**判别力由 M13 承担,
   字面量断言只承担「不被随手改动」;避免后人误读为「28 已被行为验证」。
2. 补一条用例钉住 **max 而非 sum**(N12):构造「条数极多但单值小」的类别。
3. 补一条用例覆盖 `len(Members)!=1` 的**多维分支**(N3),与已有的轴匹配用例配对。
4. `classeps.go:54` 增注:28 是**当日**可用总数,随新申报增加将变为真实截断窗口(§2)。
5. **与本任务无关**:`.claude/hooks/task-completed.sh` 在工作区被改(+8 行)且存在
   `task-completed.sh.bak`;运行时资产对全体 agent 只读,请 leader 确认来源与是否需要回退。
   另 `docs/collector/atlas_collector_pattern_doc.md` 为未跟踪新文件,不在本任务 `files_modified` 内。
