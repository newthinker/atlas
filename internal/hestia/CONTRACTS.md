# `internal/hestia` 的契约与机制清单

**这份表回答一个问题：这个包有哪些地方靠自觉，而不是靠机制。**

来源：Sprint 033 QA 终审（`qa-agent-10`，三视角对抗轮），全部条目附实测依据。
之所以随代码入库而非留在评审报告里——报告会随 Sprint 归档，而这些契约不会随之失效。

## 结论

| 类别 | 条数 | 含义 |
|---|---|---|
| **(a) 有机制** | 16 | 有测试/检查守住，违反会红 |
| **(b) 有契约无机制** | 6 | 注释里写了，没有任何测试能验证调用方是否遵守 |
| **(c) 连契约都没有** | 5 | 既无检查也无声明 |

⇒ **11 处靠自觉，其中 4 处违反后完全静默**（下表加粗的 #2 #3 #10 #14 #19）。

> **M1b-1.5（2026-08-10）关闭了 #1 #7 #8 #21 四条**，另在 H10 上落了组合校验、
> 在 Save 上加了「Passed=true 但 Values 为空」的拒绝。逐条见下表与文末。

「完全静默」= 违反后不报错、不崩溃、数据看起来正常，只是**错的**。

## 清单

| # | 条目 | 类 | 依据 | 违反后果 |
|---|---|---|---|---|
| 1 | `DB()` 写入绕过 | **a 已闭合** | M1b-1.5：`DB()` 返回 `Reader` 只读接口；`TestDBReturnsReadOnlyHandle` 用反射查**静态**返回类型 | 剩余面：显式 `s.DB().(*sql.DB)` 断言仍可绕过，但那是 review 看得见的 |
| **2** | 写口守卫对导出 `var`/`const`/`type` 失明 | c | 守卫只看 `*Store` 方法集与 `*ast.FuncDecl` | **全静默** |
| **3** | G9 并发契约（同键 Save 串行化） | b | `fields.go:18-31`；`-race` 明确看不见 SQL 层 TOCTOU | **全静默**，且**登记的后果本身写错了**（见下） |
| 4 | G9 契约文本自身无存在性守卫 | c | `TestPackageDocDeclaresUnits` 只守单位四词 | 整段可删，零测试转红 |
| 5 | `Check.Reason`「Skipped 必填」· `Passed=true` | a | `store.go:357-361` + `TestSaveRejectsSkippedWithoutReason` | 已闭合 |
| 6 | 同上 · `Passed=false` | b | `store.go:343-345` 明文声明刻意不查 | 仅进 pending JSON，消费者是人；论证成立 |
| 7 | `Passed=true` 空 `Checks`（22 字符旁路） | **a 已闭合** | M1b-1.5：`checkReportConsistency` 加空报告拒绝 + `TestSaveRejectsPassedWithNoChecks` | 已闭合 |
| 8 | `Check.Value` 非有限值 | **a 已闭合** | M1b-1.5：`checkReportValues` + `TestSaveRejectsNonFiniteCheckValue`（NaN/±Inf 三例） | 已闭合。仍在入口拒绝、两张表都没有——**这是有意的**：Value 为 NaN 说明闸门实现有 bug，应响亮失败，那一期可在修完后重跑 |
| 9 | 单位约定 · 注释存在性 | a | `TestPackageDocDeclaresUnits` | 已闭合 |
| **10** | 单位约定 · 数值实际符合单位 | c | 无任何量级/区间检查 | **全静默**：亿元当万亿元写 = 10000× 误差，REAL 亲和性正确故无类型信号 |
| 11 | `Meta` 三处同序 | a | reflect ×2 + 落盘验证 ×1 | 已闭合 |
| 12 | pending 八列（第四处同序） | a | `TestSavePendingColumnsMatchTheirValues` | 已闭合 |
| 13 | 字段名字面量只准在 `fields.go` | a | `TestFieldNamesAppearOnlyInFieldsGo` | 捕获上限测试自己写明（变量拼接/`Sprintf`/`[]byte` 往返抓不到） |
| **14** | `ingested_at` 不得 `ORDER BY`/`MAX()` | b | 契约见 H5；测试钉的是**理由**不是禁令 | **全静默**；且包内已有一处违反 |
| 15 | `ingested_at` 用 RFC3339Nano | a | 单点支点 | 已闭合 |
| 16 | pending 表列漂移 | a | `TestSavePendingFailsLoudlyOnDriftedPendingTable` | 确定性响亮失败 |
| 17 | `Values`「键不存在即缺失」 | a | NaN/Inf 闸门 + 白名单 + 落 NULL 测试 | 已闭合。空 `Values` 合法为刻意 |
| 18 | 观测表列**名**漂移 | a | `verifyObservationsSchema` | 已闭合（只覆盖业务列半边） |
| **19** | 观测表列**类型**/主键漂移 | c | 无任何检查 | **全静默**，`m2` 建成 TEXT 时 `MAX{356.71, 9.5} = 9.5` |
| 20 | `period`/`published_at` 形态 | a | 两个正则 + 五条测试（含尾随换行用例） | 已闭合；日历有效性与组合合法性另有登记 |
| 21 | `caliber_version` / `extractor` 取值 | **a 已闭合** | M1b-1.5：`validCaliberVersions` / `validExtractors` 白名单 + 两条拒绝测试 | 已闭合。用枚举而非形态正则——`^\d{4}-\d{2}$` 会放行 `2024-07` 这种虚构版本 |
| 22 | `caliber_version` 跨口径同比无效 | c | `fields.go:71-72` 声明，全包无消费者 | 属下游，**未登记任何落点** |
| 23 | `bitemporal` spec 与视图同源 | a | 单一 spec 同喂两侧 + `verifyCurrentView` | 已闭合 |
| 24 | 全等比对依赖驱动**逐字保存**视图文本 | c | `store.go:109-110`「已实测」 | 驱动升级规范化空白 ⇒ 既有库全打不开 |
| 25 | 观测表 PK 与 spec 同源 | a（非派生） | PK 是 `schema.go:55` 手写字面量，第四份拷贝 | 当前有测试兜住 |
| 26 | `Store.now` 注入点 | a | `now` 未导出 + Save 无条件覆写 + 三条测试 | 已闭合 |
| 27 | `insert` 失败不落 pending | b | 「一个入口、两个目的地」只对**过闸失败**成立 | 已就 OutOfOrder 登记 |
| 28 | 错误路径一律返回 `Outcome{}`（`Verdict=New`） | b | 观察级已登记 | — |

## Sprint 033 QA 判定 REJECT（2 CRITICAL + 7 WARNING）的处置

Sprint 033 的 7 个任务在 QA 本体裁决落盘**之前**就已 accepted 并归档——**质量门禁顺序被绕过了一次**，这些发现因此没有走返工流程。

**M1b-1.5（2026-08-10）补上了这次返工**：两个 CRITICAL 全部关闭，WARNING 5 关闭，另外处置了 H10 与「空 Values 占坑」，并趁窗口未关收窄了 `DB()`。剩余 WARNING 逐条见下。

### CRITICAL —— 均已关闭

**✅ C-1 `rep.Checks[].Value` 的非有限值让该期数据两张表同时消失**（#8）

`checkValues` 只看 `Observation.Values`，而 `savePending` 第一件事是 `json.Marshal(rep)`。`store.go:294-299` 花四行论证「NaN 必须在入口拦」，**理由逐字就是这个后果——隔壁一个字段没覆盖**。

且必然触发：那段注释自己点名来源是「比率型 0/0」，而 `Check.Value` 恰是比率闸门的实测值。

修法 ~6 行：`checkReportValues(rep)`。

> **已实施**（commit `419c149`）。拦在入口而非净化后放行：Value 为 NaN 说明闸门实现有 bug，应当响亮失败，那一期数据可在修完闸门后重跑补回，而被掩盖的 bug 不会。

**✅ C-2 `caliber_version`/`extractor` 枚举了合法值却只查非空**（#21）

`types.go:31-32` 用与 `period_type` **完全相同的文体**列出合法值，实测 `"garbage"` / `"2099-99"` 均 `err=nil` 落盘。

而 `fields.go:71-72` 写着 M1 口径断裂「只能靠 `caliber_version` 标注」——**它是那个断裂点的唯一防线**。

与 Sprint 033 的 C2 返工（G10 白名单化）是同一条推理，只是没走到 `Meta` 上。

> **已实施**（commit `24b1fd4`）。顺带把两处列对应测试的 `extractor-sentinel` 换成 `rule@v1`——它们靠「六/七个值互不相同」验证列不错位，两个测试都自带 fixture 强度自检。

### WARNING

| # | 条目 | 状态 |
|---|---|---|
| 1 | 未知键 + `Passed=false` ⇒ 两表皆空 | **留** —— 与 C-1 同型但后果轻；修它要改「输入非法一律拒绝」这条一致规则，需单独讨论 |
| 2 | 漂移检查只比列名，类型漂移静默放行（#19） | **留** —— 需扩展 `verifyObservationsSchema`，属 schema 检查范畴 |
| 3 | 写口守卫对导出 `var` 失明（#2） | **留** —— 只改测试 ~15 行，但与本轮生产代码改动无关 |
| 4 | `INSERT OR IGNORE` 变异存活 | **留** |
| 5 | `Passed=true` 空 `Checks` 直进权威表（#7） | **✅ 已关闭**（commit `e4c25f3`） |
| 6 | G9 契约登记的后果与实测不符（#3） | **留** —— 改契约文本前需重新做并发实测，不宜顺手改 |
| 7 | `verifyCurrentView` 对纯书写差异假阳性（#24） | **留** |

### 零成本时间窗 —— ✅ 已执行（2026-08-10，commit `63ebb06`）

**`DB()` 收窄为只读接口**（#1）——实测**包外零调用方**，且与 M1b-4 的 `article_id` 幂等查询不冲突（只读足够）。一旦有了外部调用方，这个窗口就关了。

窗口在关闭前被利用：`DB()` 现返回 `Reader`（仅 `QueryContext` / `QueryRowContext`）。

> 实施时踩到一个陷阱值得记下：守护测试的直觉写法 `var h any = s.DB(); h.(interface{ Exec(...) })`
> **测不出任何东西**——Go 的类型断言看动态类型，而 `Reader` 里装的仍是 `*sql.DB`，
> 那样写在收窄之后也照样红。要查的是 `DB()` 的**静态**返回类型，故改用
> `reflect.TypeOf((*Store)(nil)).MethodByName("DB").Type.Out(0)`。


## 交给 M1b-2 的两条复核结论（M1b-1.5 期间产生）

### H8 的严重性需下修 —— 「每期都发生」不成立

H8 称「54 个字段横跨至少两篇央行报告……**同期、通常同日发布、URL 不同**……频率：不是一次性迁移，而是**每期都发生**」。

**核对 `testdata/` 里的真实样本后，这个前提对近期报告不成立：**

| 样本 | 板块数 | 社融 |
|---|---|---|
| `pboc-2020-06-h1.html` | **6** | **不在本篇**（当时单独发布） |
| `pboc-2025-12-annual.html` | **8** | **一、二节就是社融存量与增量** |

2025 年报告的一、二节标题即「社会融资规模存量同比增长8.3%」「全年社会融资规模增量累计为35.6万亿元」，**27 个社融字段全在同一篇里**——M0 的解读文档正是从这一篇做出完整社融存量结构表的。

因此：

- **不阻塞 M1b-2 的 `rule@v2` 解析**——一篇报告就是全的
- **阻塞 M1c 回填**（处理 2020 至社融并入前的期次时才遇到）
- 「与 H6-3 构成永不收敛的循环」只在真的去抓第二篇时出现；**是否抓第二篇是 M1b-2/M1c 的范围决策，不是既成事实**

原判断来自设计文档的字段清单推断，未回头核对 testdata。H8 仍是真问题，只是时机与频率被高估了。

### H10 的 `2026-12/monthly` 用例复核为合法

H10 的实测把 `2026-12/monthly` 与 `2026-06/annual`、`2026-03/h1` 并列为「组合非法」。**M1b-1.5 只落实了后两类**：

已实施的规则是 `h1`→期末月必须 `06`、`annual`→必须 `12`，`monthly` 任意月份放行。

理由：央行每年 1 月的年报同时含全年与 12 月单月数据，解析器若把两者都抽出来，`YYYY-12/monthly` 就是真实期次；且它与 `YYYY-12/annual` 的业务键本就不同（`period_type` 是主键的一部分），不会混淆。禁止它会拦掉合法数据。

## Sprint 034（M1b-2 解析层）新增与变更

本节由 Leader 于归档前折入，来源：TASK-007 discovery 的 `contracts_classification`（14 条）
+ QA 两轮评审（`05-review/sprint-034-review.md` 与 `-round2.md`）实证的增补。

> **为什么折进来**：QA 两轮都指出「本 Sprint 一行未动本文件，T7 的分类只活在 discovery 里」，
> 而本文件开篇自述的入库理由正是「**报告会随 Sprint 归档，而这些契约不会**」。

**本 Sprint 对 `internal/hestia` 的导出面净增量恰好是 `func Parse(raw []byte) (Observation, error)`**
（`go doc` 在 `6c51f78` 与交付态两侧 diff，差异恰好一行）。

### a 类 —— 有机制（守卫经 grep 逐条核实存在，非转述）

| # | 条目 | 守卫 |
|---|---|---|
| 29 | 导出面精确相等（新增导出物必须同步名单） | `TestPackageExposesNoWriteFunctions`（`store_test.go:368`） |
| 30 | `Parse` 不碰存储 | `TestParseDoesNotTouchStorage`（`go/parser` 断言无 `database/sql` import、无 `.Save(`） |
| 31 | `extractorV1/V2` 常量 ↔ `detectExtractor` 返回值绑定 | `TestExtractorConstantsMatchDetect` |
| 32 | `monthly` 期次零样本 ⇒ **显式拒绝而非猜** | `TestParseRejectsMonthlyUntilSampled` |
| 33 | `Parse` 产出的 `Observation` 是未完成的（`ArticleID`/`IngestedAt` 留空），直接喂 `Save` 必被拒 | `Meta.validate` + `TestParseMetaPassesM1b1Validation` |
| 34 | `findSection` 只匹配 Title——**正文提及 ≠ 板块主题** | `TestFindSectionResolvesAllT5Keywords` / `…IgnoresBodyOnlyMentions` |
| 35 | `section.Title/Body` 两端已 TrimSpace（下游因此不再 Trim） | `TestSplitSectionsTrimsTitleAndBody` |
| 36 | 板块数硬断言 8/6（**多切与少切同样是 bug**） | `TestSplitSectionsOnRealSamples` 的 `require.Len` |
| 37 | 未知版式一律 error 不降级 | `TestDetectExtractorRejectsUnknownLayout` |
| **38** | **板块序号必须从「一、」起连续**（Sprint 034 返工新增） | `checkSectionOrdinals` + `TestDetectExtractorRejectsNonConsecutiveOrdinals`。QA 消融实证：恒返回 nil ⇒ 375 PASS / **5 FAIL** |
| **39** | **每条清单模板恰好命中一次**（返工新增） | `mustMatch` 的 `len(all) != 1` 报错 + `TestListTemplatesHitExactlyOnceOnRealSamples`。QA 消融：退回最左优先 ⇒ 378 PASS / **2 FAIL** |
| **40** | **`published_at` 形态在 Parse 与 Save 共用同一条 `publishedAtRE`**（返工新增） | `Parse` 的 `!ok \|\| pubDate == ""` + 形态校验；`TestParseRejectsBadPubDate` |

### b 类 —— 有契约，无机制

| # | 条目 | 为何不做成机制 |
|---|---|---|
| 41 | golden 期望值必须**手工抄自原文**，不得由解析器生成 | 无法用测试区分「人抄的」与「机器生成后粘进来的」——字面量完全相同。**可机械化的那半已做**：golden 先于全部实现落盘，判据 `git log --diff-filter=M -- golden_test.go` 当前仍为空。剩余敞口是「将来有人修改它」 |
| 42 | golden 比对的 **`1e-6` 容差本身无守卫** | **这条可低成本机制化，建议做**：改成 1e-2 **不会有任何测试转红**，而那足以让真实抽取错误（48100 抽成 48101）漏网。两个 dev 在不同任务上独立命中 ⇒ 系统性。⚠️ 它同时有**依据缺失**这一层：**这个数从来没有人算过**——`1e-6` 太松会漏真错，严格相等又会被 ULP 噪声打红，**两个方向都不成立说明它是默认值不是算出来的**。修法分两步：① 机制＝严格相等 + 显式豁免清单；② 依据＝先逐项枚举哪些换算 bit-exact（**`4.81×10000` 不精确，而 `14.64/15.91/2.39` 精确** ⇒ 一份「某几个值通过」的证据**不能外推**） |
| **43** | **`rule@v1` 是否定式结论，序号校验只把碰撞面收窄、没有关闭** | 见下方专条 |

> #### 关于 #43 —— 归档产物里的表述已被更正，以本条为准
>
> **⚠️ `06-acceptance/final-report.md`（已随 sprint-034 归档）与 QA round2 首版把这条写成
> 「社融两节位于报告**开头**」，那是错的。** QA 在修订版里直接喂 `detectExtractor` 实测了
> 三种布局各丢后缀两节：
>
> ```
> 社融在 一二（当前布局）→ 6 节 → 报错（安全）
> 社融在 四五（中间布局）→ 6 节 → 报错（安全）   ← 序号出缺口，被序号校验接住
> 社融在 七八（末尾布局）→ 6 节 → 静默返回 rule@v1   ← 唯一失守
> ```
>
> ⇒ **中间布局是安全的；唯一失守的是末尾**——那是丢掉它们之后序号仍连续的唯一位置。
> 准确的前提是「社融**不在末尾**」，不是「在开头」。
>
> **根因不是位置，是推断方向**：`rule@v1` 是**否定式**结论（不是 v2 ⇒ 那就是 v1），
> 而否定式结论会碰撞。**序号校验把洞从「任意两节丢失」收窄到「末尾两节丢失」——收窄不等于关闭。**
>
> **⛔ 不要加「断言社融节序号为 1 与 2」这条测试**（Leader 曾建议，QA 判否，两条独立理由）：
> ① **它对自己声称保护的场景不可能触发**——样本是固定的，将来某版把社融挪到末尾时这两份
> 样本一个字节都不变，该断言**恒绿**。⇒ 文档形状的检查穿了机制的外衣，与
> `TestPackageDocDeclaresUnits` 只查四个字串、`TestFootnoteSectionNeverWinsFindSection`
> 对自称守护的性质无鉴别力是同一类。② **它钉的还是错的不变量**——中间布局安全，
> 断言「序号为 1 与 2」会在一次无害改版上假红。
>
> **真修法（约 10 行，今天就可测）**：把 v1 判定从否定式改成**肯定式**——判 v1 之前
> 正向确认那 6 个标题匹配 v1 的关键词画像。判据即上面那个合成用例。**它不依赖社融在哪
> ——前提被消掉，而不是被断言。**
>
> **为何判「登记」而非「现在就改」**（与 #45 `fields.go` 判「现在就该改」用的是同一条判据：
> **下游会不会照这个前提施工，且施工出的错是否静默**）：
>
> | | #45 单位缺口 | 本条 |
> |---|---|---|
> | 下游照它施工 | **会**（M1b-3 必写量级区间表，唯一可参照的分类器就是那段约定） | **不会**（M1b-3 消费 `Observation`，`section` 未导出，够不到板块位置） |
> | 错了是否静默 | **会**（区间放宽，不响亮失败） | 触发者是**央行改版**这个外部事件，届时本就要人工介入 |
> | 现在改的成本 | **0 行代码**（纯注释） | ~10 行生产代码 + 测试，且改的是版式判定语义 |
>
> ⇒ **本条应与下方「completeness 闸门二选一」一起设计**——那本来就是同一个问题：
> **我们如何正向识别一个模板版本。**

### c 类 —— 连契约都没有

| # | 条目 | 现状与建议 |
|---|---|---|
| 44 | 万亿→亿 换算的 **float64 表示误差**（`loan_corp_short_ytd` 低 1 ULP，相对误差 8.12e-17 **输入侧** / 1.51e-16 **输出侧**） | 与 #10（量级错，10000×）**不是一回事**，不能算已登记。**建议至少升级为契约**：「增量类字段由 万亿×10000 得出，个别值存在 ≤1 ULP 表示误差，**下游不得对其做精确相等比较**」。⚠️ **对 M1b-3 尤其要紧**：若某道闸用 `==` 或恒等式校验（如分项加总 == 总额），这个 ULP 会让它在毫无实际问题时报红。⚠️ 易踩：**Go 的无类型常量算术是精确的**，源码里写 `4.81*10000` 得到精确的 48100 ⇒「在 Go 里手算一遍验证」会得出相反结论 |
| 45 | `fields.go` 的单位约定**不覆盖** `fx_reserve`（万亿美元）与 `fx_rate`（元/美元） | 包注释声称三类覆盖，实际两个字段在三类之外；守卫 `TestPackageDocDeclaresUnits` **只查 4 个字串是否出现，不查覆盖完整性**，故缺口静默。**QA 判「现在就该改」**：现在改是纯注释 0 行代码；等 M1b-3 照错误前提写完 field→量级区间表再改，就是改一张已上线的闸门表，**而那张表错了不会响亮失败**（只是区间放宽）。建议②把该测试升级为「遍历 `fieldOrder`，每个字段都要能归入某一类」的覆盖性检查 |

### 状态变更

- **B3「`section.has` 不得用于板块定位」→ 关闭。** `section.has` 已在 Sprint 034 返工中删除（`grep` 0 命中），提交信息按要求写明「T3『留给 T5』的理由已随 T5 交付而过期」。**该条目曾被两次独立发现为死代码**（dev-45 在 T5 记「不是死代码，免得被清理」，QA 的 minimalist lens 在评审时判「删」）——保留理由过期而无人撤销，是它被反复发现的原因。

### M1b-3 开工前必须定案的一件事（QA round1 提出，本 Sprint 不定案）

**completeness 闸门在当前设计下恒真**：`extract.go` 的纪律是「任何模板未命中一律报错」，
`extractFields` 要么全成要么返回 nil ⇒ `Parse` 的输出键数**只可能是 54 或 27**。两条互斥出路：

- **(i) 认它是恒真的纵深防御** ⇒ 那就**别手写第四份字段划分表**，v1/v2 必填集应从
  `sectionRules` + `v2Only` **派生**（`fields.go:3-5` 自己写着「手写多份必然不同步」）；
- **(ii) 让抽取变成部分成功**（逐字段记命中/未命中）⇒ completeness 才有信号。

⚠️ 注意 `types.go:187` 的 `Check.Reason = "absent_field:<name>"` 与整套 pending 机制，
**读起来就是为 (ii) 准备的** —— **数据模型与 `extract.go` 的「全有或全无」纪律之间，
有一条从未被声明的张力**。当前设计下，央行改一句话会让整期 54 个字段全部落空，
没有「入 53 个、标 1 个 pending」的路径。

## Sprint 035 · M1b-3 validate

七道闸门与 `Validate` 落地。以下三节是**交付时的诚实状态**，不是缺陷清单。

### 七道闸门的实际防护力

交付时只有**三道半**真正拦得住东西：

| 闸门 | 状态 | 何时有声 |
|---|---|---|
| `monetary_hierarchy` | ✅ 有信号 | 现在 |
| `deposit_sum` | ⚠️ 弱信号 | 绝对值现在（±12% 容差，实测残差 7.6–9.1%，余量仅 3pct）；漂移判据待 M1c |
| `corp_loan_reconcile` | ✅ 有信号 | 现在 |
| `yoy_sanity` | ✅ 有信号 | 现在 |
| `stock_continuity` | ⏸ 恒 skipped | M1c 回填出连续序列后 |
| `completeness` | ⏸ 恒 passed | M1c 加 LLM 兜底、抽取变成部分成功后 |
| `magnitude_sanity` | ⏸ 恒 skipped | M1c 用回填分布标定 `MagnitudeRanges` 后 |

后三道都依赖 M1c 的回填数据，会在**同一时刻**一起从沉默变成有声。

### 边界守卫收口表

`validate.go` 现有 **12 个比较运算符**（按最宽正则扫描后逐个标注，方法论见
`findings-carryover.md` 的 F19；**空缺明写为空缺，含判定不补的那些及理由**）：

| # | 位置 | 归属 | 守卫 |
|---|---|---|---|
| 1–2 | `m2 > m1 && m1 > m0` | monetary_hierarchy | ✅ `TestMonetaryHierarchyRejectsEquality`（Sprint 035 收尾补） |
| 3 | `r > DepositSumTolerance` | deposit_sum | ✅ |
| 4 | `len(hist) < minDriftHistory` | deposit_sum | ✅ |
| 5 | `drift > DepositSumDriftMax` | deposit_sum | ✅ |
| 6 | `r <= CorpLoanTolerance` | corp_loan_reconcile | ✅ |
| 7 | `r <= StockContinuityMax` | stock_continuity | ✅ |
| 8 | `worst <= YoYSanityMax` | yoy_sanity | ✅ |
| 9 | `a > worst`（取最大者） | yoy_sanity | ❌ **裁定不补**：只影响并列时 `Reason` 里报哪个字段名，不影响判定 |
| 10 | `len(s) <= n`（firstN 截断） | 错误文案 | ❌ **裁定不补**：纯展示，不参与任何判定 |
| 11–12 | `v < r.Min \|\| v > r.Max` | magnitude_sanity | ❌ **留 M1c**：该闸恒 skipped，缺口当前影响为零；填表时必补 |

⇒ 8 个有守卫、4 个明确无守卫（2 个裁定不补 + 2 个留 M1c）。

### ⚠️ 结构性根因：落 pending 的期次对依赖历史的闸门**永久不可见**

> **未过闸的期次落 `hestia_pending` ⇒ 不在 `v_hestia_current` ⇒ `Preceding` 看不见
> ⇒ 依赖历史的闸门以一份「只含过闸期次」的历史为基线，因而无法自愈。**

这条是 Sprint 035 QA round2 的 R2-1/R2-2，由 QA 与 Architect lens 独立收敛到同一根因。
**它不在 F1–F27 的任何一条里**，登记在此是因为**它的触发时刻晚于本 Sprint 归档**
（Review 报告会随 Sprint 归档，本文件不会）。

**代码层事实（本任务作者直读确认）**：`Store.Preceding` 查的是 `v_hestia_current`，
而该视图由 `bitemporal.CurrentQuery(spec)` 从 `hestia_observations` 派生；
`TablePending` 是另一张表（`schema.go:12` 自述「它的消费者只有人」）。
⇒ pending 行结构上进不了任何闸门的历史。

**行为后果（QA 实测数据，本任务作者未独立复跑，转述）**：

- **`deposit_sum` 的漂移判据在口径变更后永不恢复**。构造三期旧口径残差 2%、2025-04 起
  新口径下 11% 为正常值且无人写豁免 ⇒ 每一个新口径期次都 failed、都进 pending、都进不了
  历史，**均值永远冻结在 `0.0200`**（实测连续 5 期 `from 3-period mean 0.0200` 一字不变）。
  **这是一个没有出口的反馈环**，而漂移检测正是这道闸自称的实际价值。
- **`stock_continuity` 更糟：任意一期因任意理由落 pending，之后所有期次级联失败且偏离
  单调增大**。实测那次唯一的缺陷是 2025-02 把 m1/m2 抽反了——**一个与社融毫无关系的错误**
  ——却把后续全部钉死：基线 `from 400` 永久冻结，社融存量单调增 ⇒ 偏离单调增大。
  且 `Reason` 只说「from 400 to 412.09」，**不说 400 来自哪一期**，真因在报告里不可见。
- 佐证：`historyDepth 6→3` 与 `prior[0]→prior[len-1]`（取最近一期改成取最老一期）
  **两条变异整套 475 个测试都分辨不出来**。根因是 `fakeHistory.Preceding` 丢弃了
  `period`/`periodType`，且所有 prior 夹具用同一个 `validMeta()` ⇒
  **「相邻」在闸门测试里不是没测，是不可表达**。缺口精确落在接缝上。

**修的方向（QA 建议，未实施）**：`scanObservation` 已经把 `Meta.Period`/`PeriodType` 填好了，
信息就在 `in.prior[0]` 里、闸门只是没用；从 `obs.Meta` 推出期望的前一期，不匹配则
`skipped{gap:<实际期次>}`，并把「相邻」写进 `History` 接口契约。`deposit_sum` 侧可用
`Meta.CaliberVersion` 过滤基线——**这恰好给本文件 #22（`caliber_version` 全包无消费者、
「未登记任何落点」）提供了它一直缺的那个落点**。

### 留给 M1c 的五件事

1. **`MagnitudeRanges` 要用回填分布标定**，不得拍脑袋。填表时**另有三件事必须一并补**
   （遍历顺序守卫、`Min`/`Max` 两个边界方向、`Range.Unit` 的单位）——
   这段提醒已挂在 `thresholds_test.go` 的 `TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated`
   上，那条 `assert.Empty` 是**必然会响的绊线**：任何人填表都必须先撞红它。
   把提醒挂在绊线上，而不是挂在需要被记起来的文档里。

   #### 标定时**优先覆盖互为同组的分项字段**，因为那类错误现在没有任何闸门抓得住

   在此之前本节只说了「不得拍脑袋、要用回填分布标定」，**没说不标定会漏掉哪一类真实错误**。
   补上这层因果（QA round2 R2-13，下列实测由本文件作者独立复跑证实）：

   **加总是置换不变量** ⇒ `deposit_sum` 与 `corp_loan_reconcile` 对「同组内分项互相错位」
   **零鉴别力**：分项之和不变，残差不变，判定不变。实测（`golden2025` 真实值）：

   | 互换 | 两值 | 结果 |
   |---|---|---|
   | 企业短期 ↔ 中长期 | 48100 / 88200 | `Passed=true`，failed 闸门数 **0** |
   | 住户存款 ↔ 财政存款 | 146400 / 6579（**相差 22.3 倍**） | `Passed=true`，failed 闸门数 **0** |

   **为什么这条比它的 MINOR 标签更要紧**：`profiles.go:194-195` 自己写着「『短期贷款』在两个
   作用域里各出现一次，指向不同字段——这是全篇唯一需要作用域的地方，**也是本任务存在的主要
   理由**」（本文件作者已核对原文）。⇒ **分项错位是抽取层点名的头号风险，而校验层对它零覆盖。**

   **判 MINOR 仍是对的**：这不是实现缺陷 —— 加总闸在**数学上不可能**抓住置换。
   七道里唯一抓得住的是 `magnitude_sanity`（住户存款与财政存款差 22 倍，区间一填即可分辨），
   **而它恒 skipped 至 M1c**。⇒ 标定 `MagnitudeRanges` 时**优先覆盖互为同组的分项**：
   住户/企业/财政/非银存款，企业短期/中长期/票据。
2. **`StockContinuityMax = 0.02` 未经真实数据验证**。M0 的两份样本只有一份含社融，算不出环比。

   ⚠️ **补记（QA round2 R2-6）：重新标定解决不了它。** `Preceding` **正确地**按 `period_type`
   隔离序列，但 `Thresholds` **没有 `period_type` 这根轴** —— 同一个 `0.02` 会用在
   monthly / h1 / annual 上，而三者的自然增速差一个量级。QA 实测同一份 `tsf_stock=442.12`：
   monthly passed、h1 failed(0.0420)、annual failed(0.0830)。

   **这不是编出来的增速**：本文件 `:121` 自己记着 2025 年报的一节标题就是
   「社会融资规模存量**同比增长 8.3%**」（本任务作者已复核该行确实存在）。
   ⇒ M1c 一旦回填出连续年序列，**每一期 annual 都必然被打进 pending**，
   而 golden2025 本身就是 annual 样本。
   单标量结构上服务不了三条周期不同的序列，**重标只会把失败方向从 annual 换到 monthly**。

   **方向**：`StockContinuityMax float64` 按 `period_type` 分档，或对非 monthly 直接
   `skipped{not_calibrated:<period_type>}`。**现在改是零成本，标定之后改就是改一张已上线的阈值表。**
3. **`Extractor` 需要携带模板版本**。`llm-fallback@v1` 只说了「用了 LLM」，没说抽的是 v1 还是
   v2 期次，所以 `requiredFields` 对它返回 nil、`completeness` 记 `skipped{unknown_extractor}`。
   这是**刻意的失败信号**——M1c 启用兜底的第一天就会撞上。
4. **依赖历史的闸门无法自愈**（见上一节的结构性根因）。它与前三条不同：前三条是「阈值/口径
   没标定」，这条是**反馈环没有出口**，不标定任何东西都解决不了。
5. **ULP 守卫需要改成观察生产算路**（见下面「浮点契约」一节）。

### 浮点契约

增量类字段由 万亿×10000 得出，个别值带 ≤1 ULP 表示误差（实测
`4.81×10000 = 48099.99999999999`）。**闸门一律不得对它们做精确相等比较。**
现有七道全是不等式或容差比较，误差被完全盖住（`corp_loan_reconcile` 的残差是
−1.16% 对 ±5% 容差，ULP 的相对占比约 5e-17）。见 `TestTrillionConversionCarriesULPError`。

⚠️ 验证这件事时**不能在源码里手算**：Go 的**无类型常量**算术是精确的，写 `4.81*10000`
得到精确的 48100，会得出「没有误差」的相反结论。必须用运行时变量。

#### ⚠️ ULP 守卫的现状：它**不观察它声称守卫的对象**（QA round2 R2-5）

`TestTrillionConversionCarriesULPError` 的失败文案写着「若这个等式成立说明**换算方式变了**」，
但它测的是自己现写的 `trillion := 4.81; got := trillion * 10000`，
**不经过 `amount.toYi()` / `scaleOf()` 任何一行** —— 它是对生产算术的**再实现**，不是对它的观察。
（本任务作者直读确认：该测试体内对 `toYi`/`scaleOf`/`amount`/`Parse` 的引用数为 **0**。）

QA 实测把 `amount.go:121` 的 `toYi` 改成整数化（**这正是「换算方式变了」**）后：
生产算路 `toYi(4.81 万亿元) = 48100`（ULP 误差已被消除），而该测试**仍全绿**
（PASS=475 等于基线）。⇒ **生产管线的误差被完全消掉，声称钉住它的测试一动不动。**

⚠️ **归因要写对：这不是 dev 的偏离。** TASK-007 的 DoD `non_functional[1]` 白纸黑字要求
「`trillion := 4.81` 用**变量**，`assert.NotEqual(48100.0, got)` + `assert.InDelta`」——
**是 DoD 本身指定了一条不观察生产的测试**，实现逐字照做了。

**修法（QA 给的，约 2 行，本任务未实施——`validate_test.go` 不在本任务 `writes` 内）**：
改从 `golden2025[FieldLoanCorpShortYTD]` 或 `Parse` 的输出取值再断言，
让断言的输入来自生产算路而不是测试自写的表达式。

**一条容易写错的相关措辞**：阈值边界用例成立的条件是**两边舍入到同一个 double**，
不是「比例精确可表示」——`0.02` 本身就不精确（位模式 `0x3f947ae147ae147b`，实为
`0.020000000000000000416`）。而它取决于**参与运算的量是否精确**，不取决于算路长短：
实测 `400→408` 成立、`123→125.46` 低 15 ULP 即失效。改边界用例的常量时，
必须保持参与运算的量为精确整数，否则测试会**静默失效**。

### `Check.Value` 的单位不统一，且这是刻意的

| 闸门 | Value | 单位 |
|---|---|---|
| `deposit_sum` | 残差占比 | 比例（0.0906） |
| `corp_loan_reconcile` | 残差绝对量（保留符号） | **亿元**（−1800） |
| `stock_continuity` | 环比变化率 | 比例 |
| `yoy_sanity` | 最大同比绝对值 | 百分数（25.0） |
| `completeness` | 缺失字段数 | 个 |
| `magnitude_sanity` | 越界字段的值 | 随字段 |
| `monetary_hierarchy` | nil | 判的是序关系，无单一实测值 |

`corp_loan_reconcile` 记亿元而非比例，是因为 spec 第 7 节与 **M0 契约样本已经这么记**
（样本里是 `-1203`）。下游是 Grafana 面板与 pending 人工复核，把 `1.16%` 读成
`-1203 亿元`是量纲错读。守卫见 `TestCheckValueUnitsFollowSpec`。

### 口径豁免

命中豁免记 `skipped{caliber_exemption:<version>}` 而**非 passed**，并**保留 Value**。
豁免与通过在数据上必须可分——把「这次没查」记成「查了没问题」等于伪造一次检查记录。
豁免按 `(期次, 检查 ID)` **精确匹配**，不做范围或前缀匹配（那会让一次性豁免变成永久后门）。
`SkipChecks` 的 ID 从 `gates` 派生校验，打错一个字会响亮失败而不是静默失效。

#### 豁免能有多宽：三条边界，两条已堵、一条待定案

豁免是本 Sprint 最后引入、测试最薄的一块：TASK-007 的四条测试全部针对
「命中/不命中/ID 拼错/记 skipped 不记 passed」，**没有一条针对「豁免能有多宽」**——
而这个机制的全部设计意图正是「不能太宽」（spec 4.6.3 约束 1）。

| # | 边界 | 状态 |
|---|---|---|
| 1 | `SkipChecks` 覆盖**全部** `knownCheckIDs()` = 字面意义的整期跳过 | ✅ Sprint 035 收尾已堵 |
| 2 | `SkipChecks` 含 **`completeness`** = 更便宜的整期跳过 | ✅ Sprint 035 收尾已堵 |
| 3 | 豁免键**缺 `PeriodType`**：同月的 annual 与 monthly 被同一条豁免同时命中 | ❌ **M1b-4 装载前必须定案** |

**#1/#2 为什么必须堵**：`thresholds.go` 自己的错误文案早就写着「豁免必须按检查 ID 精确指定，
**不是整期跳过校验**」，而实现只查 `len(SkipChecks) == 0`。**代码自述与实际行为矛盾比潜伏
缺陷更值得修**——错误文案会主动误导后人。QA 实测：填满七个 ID 时 `cfg.validate()=nil`、
`0/7` 闸门通过、数据进**权威表**。

**#2 的成因**（本任务作者已用自己的 `TestValidateHandlesEmptyValuesWithoutSpecialCase` 复核）：
其余六道遇缺字段一律降级 `skipped`，`completeness` 是七道里**唯一**会因数据缺失而 failed 的
一道 ⇒ 单独豁免它，一个几乎空白的期次就能整期过闸进权威表，而字面上完全符合「按检查 ID
精确指定」。**机制满足了约束的字面，而闸门集合的降级结构使它达不到约束的目的。**

⚠️ **判据是集合覆盖关系，不是数量阈值**：写成 `len(SkipChecks) > N` 会误伤正常的多闸豁免
（一次口径变更同时豁免六道是合法的，有测试钉住）。

**#2 的代价（已知，登记备查）**：现在**完全不能**豁免 `completeness`。若 M1b-4 出现
「模板变更导致某几个字段合法消失、需要单独放行 completeness」的真实需求，
需要的是更细的粒度（例如按字段豁免）而不是放开这条校验。

**另一条已知不足（QA round2 R2-4，未修）**：单期豁免盖不住 `historyDepth = 6` 的滑动窗口，
且**被豁免那期的数据仍被计入后续基线**（`depositResidual` 只读 `p.Values`，对豁免一无所知）。
运维会先配 1 条，然后眼看着后续几期继续失败，而失败理由 `drift_exceeded` 指向「残差漂移了」
而不是「你的豁免不够长」。Architect lens 实测「一次口径变更要配 4 条连续豁免」。

## 相关文档

- **`.arcforge/docs/05-review/qa-review-sprint033.md`** —— QA 终审报告（403 行，含每条的实测取证）

  ⚠️ **它不在 sprint-033 的归档目录里**——落盘时间（`Aug 9 20:04`）晚于归档提交 `6fd9107`，
  所以留在了运行时目录，**会随下一个 Sprint（M1b-2）一起归档到那个 Sprint 的目录下**。
  按归档目录找 sprint-033 的评审报告会找不到；这正是「本 Sprint 在无 code review 报告的
  情况下关闭」这一流程错误的物理痕迹。

- `.arcforge/archive/sprint-033-2026-08-09/docs/01-design/handoff-*.md` —— H1–H11 交接缺口
  （`handoff-to-later-iterations.md` / `handoff-additions-h8-h10.md` / `handoff-h11-cross-package.md`）

  其中 **H8 需在有生产数据前定案**（同期两篇报告的数据模型：54 个字段横跨社融与金融统计
  两篇报告，同期同日发布但 `article_id` 不同 ⇒ 第二篇判 Duplicate、字段静默丢弃，
  且与 M1b-4 的幂等检查构成永不收敛的重抓循环）。
