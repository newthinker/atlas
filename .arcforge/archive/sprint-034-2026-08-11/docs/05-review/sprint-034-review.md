# Sprint 034 Code Review —— hestia M1b-2 解析层

- **审查者**：qa-agent-11（两轮：常规 + 跨视角对抗，lens = Skeptic / Architect / Minimalist）
- **对象**：`internal/hestia/` @ `9e773118ba6f9ca95a3e113054f9164e26b40bca`（分支 `feat/hestia-parse`），基线 `6c51f788f1600aa4366a215b099ee6f929cb7ba5`
- **规模**：14 文件 / +4149 −2；生产 ~1370 行（strip 88 / sections 145 / amount 213 / profiles 328 / extract 428 / parse 170），测试 ~2760 行
- **基线读数（本人实测，主工作区）**：`go test ./internal/hestia/ -count=1 -v` → **358 === RUN / 358 --- PASS / 0 --- FAIL / exit 0**；`go vet ./internal/hestia/...` → **exit 0，无输出**
- **隔离纪律**：全部变异与探针在 `git worktree add --detach` 的一次性副本上执行，收尾 `worktree remove --force` + `prune`；主工作区 `git status --porcelain internal/hestia/` 空，HEAD 未变

## 结论：**REJECT**（1 CRITICAL + 3 WARNING）

三个 lens 无实质分歧（Skeptic 判 PASS、Minimalist 判 PASS、Architect 判 CONTESTED），差别在于**只有 Architect 检查了 `detectExtractor` 的判据盲区**——不是有人认为它不是问题。CRITICAL-1 由本人**独立复现并加强**（见下，我的读数比 lens 的更不利），故按共识确认的缺陷判 REJECT，而非 CONTESTED。

需要说明的是：**这份交付的论证密度、变异实证与自我否证质量是本仓库迄今最高的一档**。8 个任务零返工不是运气——T4 推翻了需求文档关于交替顺序的断言并给出真正的失效模式，T5 用消融实验证明两条看似重复的测试互补，T6 复核并更正了自己上一个任务里写错的注释，T8 如实上报「B1 那一格不构成证据」而没有换个说法凑成证据。下面的 CRITICAL 不是这份工作松懈的产物，恰恰相反：它藏在这个包**自己声明要防的那一类错**的正后方，需要跑变异才看得见。

---

## CRITICAL

### CRITICAL-1 · `detectExtractor` 的判据在「恰好丢掉社融两节」处有静默盲区

**位置**：`internal/hestia/sections.go:128`（判据）、`sections.go:32`（锚定）、`sections.go:42-59`（放大器）
**归属任务**：**TASK-003**

**机制**：`detectExtractor` 的判据是二元组 `(len(secs), hasTSF)`，而 **v2 恰好 = v1 + 社融两节**。于是「丢掉且仅丢掉这两节」正好落在合法的 `(6, false)` 上——守卫在**它最该守住的那个方向**上没有信号。丢掉别的任何组合都会撞到 7 板块而响亮失败。

失锚的成因是 `(?m)^` 要求标题落在行首，而 `spaceRE`（strip.go:30）**折叠但从不删除**行首空白。

**本人独立复现**（隔离副本，2025 真实样本，只在两处 `<strong>一、` / `<strong>二、` 前插入 `&nbsp;`，其余一字未动）：

```
基线:      err=<nil>  extractor="rule@v2"  values=54
变异后:    板块数=6，sec[0].Title="三、广义货币增长8.5%"
           Parse err=<nil>   extractor="rule@v1"   values=27
           含 tsf_stock ? false
           Meta.validate() = <nil>          ← 过 M1b-1 全部校验
```

一份 v2 报告被**静默**判成 v1，产出 27 个各自正确的合法字段，过 `Meta.validate`，另外 27 个社融字段在 M1b-1 的语义里读作「本期模板本就没有」。

**可照抄的复现配方（锚点选择是成败关键）**：锚点必须用 **`<strong>一、` / `<strong>二、`**——这两个串在整份 HTML 里**各只有 1 处命中**（本人实测 `len(locs)=2`），故 `strings.Replace(…, 1)` 必然落在板块标题行上。用裸的「一、」做锚点会命中正文里的其它位置，替换掉的不是标题行 ⇒ 只有一处真失锚 ⇒ 板块数 = **7** ⇒ `detectExtractor` 撞到 `7 sections, tsf_section=true` 而**响亮失败**。Leader 独立复现时正是停在这一步（7 板块 + 报错），**那不是 CRITICAL 不成立，是锚点没落到标题行**。判据请锚定两个可直读的量：**板块数必须 = 6**，且 **`sec[0].Title` 不再是社融标题**（本人实测为 `"三、广义货币增长8.5%"`）。

**这正是 `sections.go:115-119` 自己点名要防的那一类**：

> 误判 v1 会让 completeness（M1b-3）用少字段的必填集，于是「解析漏了」被当成「本期模板本就没有」——那正是 M0 复盘时列为最危险的一类错，因为它完全无声。

**触发面比 lens 估计的更宽（本人读数更不利）**：Architect 报「两份样本各有 8 行以空白开头」；我实测是 **2025 样本 578 行 / 2020 样本 361 行**以空白开头。⇒ 该站点模板**普遍**输出行首空白，机制是活的，只是至今没落到板块标题那几行上。而 `detectExtractor` 自己的注释把「页面抓残了」列为一等场景。

**为什么判 CRITICAL 而非 WARNING**：三条判据同时成立——(1) 完全静默，产出物过下游每一道现有闸门；(2) 它是本包**自述最危险**的那一类错；(3) M1b-3 的 completeness profile 将**整个建立在 `extractor` 这个单点之上**，闸门的正确性依赖一个当前无守卫的假设。
**为什么今天没有数据风险**：`Parse` 尚未接线到 `Store.Save`（M1b-4 才接），本 Sprint 的导出面净增量只有 `Parse` 一个函数。⇒ 这是**在接线之前必须关掉的洞**，不是已经泄漏的洞。

**修复判据（可测）**：板块序号必须**从「一、」起连续**。这不是新假设——本人实测两份真实样本均满足（2025 一–八、2020 一–六），是样本已验证的既有性质。约 5 行 `strings.HasPrefix` 即把上述反例（`secs[0]` 为「三、」）挡下，并把「被丢弃的前缀」这个不可观测量变成可观测量。它严格强于现有的板块计数检查（中间丢一节也会被抓）。

---

## WARNING

### WARNING-1 · `mustMatch` 无唯一性校验，约 30 条清单模板走最左优先且该选择完全无守卫

**位置**：`internal/hestia/extract.go:104`（9 个生产调用点：170/179/193/206/220/252/298/308/370）
**归属任务**：**TASK-005**

`extract.go` 开头的纪律 2 自述「孪生句一律**按捕获组挑，并要求唯一**，不靠最左优先」，理由写得极准：「相邻句子往往量级相近、格式完全正确，选错了下游没有任何校验拦得住」。但这条纪律**只落实在 `selectRMBBalance` / `selectRMBCumulativeFlow` 两族**（存贷款的总量余额句与期内合计句）。社融存量 8 项、社融增量 8 项、货币 3 项、存款分部门 4 项、贷款作用域 5 项等约 30 条清单模板一律走 `mustMatch` = `FindStringSubmatch` = 最左优先，**且没有任何命中数检查**。

**本人实测的一对变异**（隔离副本，vet 红绿都查）：

| 变异 | 读数 | 含义 |
|---|---|---|
| A：`mustMatch` 改取**最后一个**匹配 | `vet_exit=0 test_exit=0` **358 PASS / 0 FAIL**（= 基线） | **SURVIVED** ⇒「取第一个」这个选择**零测试覆盖** |
| B：`mustMatch` 改为**命中数必须为 1**，否则报错 | `vet_exit=0 test_exit=0` **358 PASS / 0 FAIL** | ⇒ 补上守卫**不会打红任何真实样本**，修复是非破坏性的 |

**危害探针**（本人构造的合法存款板块正文，把单月分部门句排在期内累计分部门句之前）：

```
extractDepositSection err = <nil>
  deposit_household_ytd = 21000    (golden 期望 146400)
  deposit_corp_ytd      = 11000    (golden 期望  23100)
  deposit_nbfi_ytd      =   500    (golden 期望  -64100 → 注意符号也翻了)
  抽出字段数 = 7
```

字段名全对、量级合理、**无任何报错**。`deposit_nbfi_ytd` 那一项连符号都反了——而 `amount.go:32-34` 正把「`deposit_nbfi_ytd` 从 −7446 变成 +7446，量级正确、方向相反，七道闸门一道都拦不住」写为本解析器最难发现的一类错。

**这个形态今天不可触发，但不是因为有机制**：本人实测两份真实样本上，全部分部门模板命中数均为 1；而 2020 h1 样本**已经含同体例的单月孪生句**（板块级：`6月份，人民币存款增加2.9万亿元` / `6月份，人民币贷款增加1.81万亿元`，且期内累计句碰巧排在前面）。⇒ 分部门行今天没有单月孪生句，是**这一期的排版事实，不是契约**。月报形态（T6 已因同一原因显式拒绝）正是它会咬人的地方。

**修复判据（可测）**：`mustMatch` 改用 `FindAllStringSubmatch`，`len != 1` 时报错并带上命中数与模板；补一条常驻断言「两份真实样本上每条清单模板命中数恰为 1」。变异 B 已证明这不会打红既有套件。

### WARNING-2 · `Parse` 放行空的 / 形态非法的 `PubDate`，错误延后到 `Store.Save` 才现场

**位置**：`internal/hestia/parse.go:101-107`；机制在 `strip.go:76-88`
**归属任务**：**TASK-006**

**两个独立 lens（Skeptic 与 Architect）各自命中同一条**，且各自给了实测。按本 Sprint 确立的判据（两人独立撞到 ⇒ 系统性缺口而非一条观察），本条按系统性分量记——与 1e-6 容差那条同型（那条也是两个 dev 在不同任务上独立命中）。

`metaContent` 的第二返回值是**刻意设计**的——`strip.go:80-81` 明写它存在的理由是「站点确实会输出 `content=""`，调用方需要能分辨是站点没填还是选择器写错了」。而唯一的生产调用点只判 `ok`，`content=""` 走 `ok=true` 那一支。

**本人实测**（2025 真实样本，只把 PubDate 挖空）：

```
Parse err = <nil>
  Meta.PublishedAt = ""   Values 键数 = 54
  补 ArticleID 后 Meta.validate() = hestia: meta.published_at must not be empty
```

对照：`ArticleTitle` 挖空**会**在 Parse 内响亮失败（`parseTitle` 接住，`unrecognized report title ""`）。⇒ 这不是设计选择，是**漏了一处**。空 `content` 也不是假想输入——`TestMetaContentPresentButEmpty` 用的 `SiteDomain` 就是真实的 `content=""`。

两个方向的偏离：(1) 全包对「读不出就报错」执行得一丝不苟，唯独这个字段是「读到什么算什么」，而它是**唯一一个逐字来自外部 HTML、不经任何模板**的字段；(2) `publishedAtRE` 就在同包 `types.go:59`，而错误最终由 `Store.Save` 抛出——现场在 store，根因在 HTML 的 `<meta>`，届时 raw HTML 早已不在手上。

**修复判据（可测）**：判据改为 `!ok || pubDate == ""`（两种情形给不同措辞，把那个第二返回值真正用起来）；可一并加 `publishedAtRE` 形态校验（正则已存在，不引入新概念）。断言：空 content 与 `2026-1-15` / `2026-01-15 09:30:00` 这类形态，`Parse` 必须直接 error 且信息含 `PubDate`。

### WARNING-3 · `fields.go` 单位约定不覆盖 `fx_reserve` / `fx_rate`（已登记项的**处置判定**）

**位置**：`internal/hestia/fields.go:6-12`；守卫 `fields_test.go:244-254`
**归属**：不属本 Sprint 任何任务的 `writes`（`fields.go` 是 M1b-1）⇒ **Leader / M1b-3 入口**

这条 Leader 已列为已知项 #4，我不重复报告，只判定**处置是否得当**：

- **登记本身正确**：T7 不改 `fields.go` 是对的（越界），T4/T7 两人独立撞到并各自登记，链路是通的。
- **但「登记后不动」这个处置在本条上不成立**，理由是代价不对称：现在改是在单位约定段补一句「例外：`fx_reserve` 万亿美元、`fx_rate` 元/美元」——**纯注释、0 行代码**，让唯一真相源回到它自称的位置；等 M1b-3 照着错误前提写完 field→量级区间表再改，就是改一张已上线的闸门表，**而那张表错了不会响亮失败**（区间放宽而已）。
- `TestPackageDocDeclaresUnits` 之所以绿，只因它只查 4 个字串**出现**，不查覆盖完整性；`TestGoldenUnitsMatchFieldClass` 之所以绿，只因 `3.36 < 1000` 这个上界碰巧也放行万亿美元。两道守卫都在这条上失明。

⇒ **建议在 M1b-3 开工前先落这一句注释**，并把该测试升级为「`fieldOrder` 每个字段都能归入某一类」的覆盖性检查。

---

## SUGGESTION

| # | 位置 | 内容 | 读数 |
|---|---|---|---|
| S2 | `strip.go:18`、`strip_test.go:61,202` | 注释里的简写 `[一-十]` 是**码点区间**（U+4E00–U+5341，1346 个码点），与实现 `[一二三四五六七八九十]+` 两个方向都不同 | 2025 样本：实现 **8** 命中 / 简写 **7** 命中（**漏掉「四、」**）；且 `上、`/`为、` 在简写下**误命中** |
| S3 | `amount.go:160` | `parsePlainNumber` 的文档说「只有两类这样用」，实际是 **3 类 4 字段**（漏了 `extractFXSection` 的 `fx_reserve` / `fx_rate`）。该函数的全部立论正是「用它的地方必须少且明确、可 grep、可枚举」——台账少列一项恰是这个设计唯一吸收不了的失效模式。另：`fx_reserve` 的单位 `万亿美元` 写死在 `fxReserveRE` 里、从不捕获，故 `checkUnit` 对它**从不执行**（形态上安全——单位变了正则就不命中——但同样该写进台账） | 生产消费者：`extract.go:374` ← `extractRateSection` + `extractFXSection` |
| S4 | `CONTRACTS.md` | 本 Sprint **一行未动**（`git log 6c51f78..HEAD -- CONTRACTS.md` 空），而 T7 新产出的 14 条分类（a×9 / b×3 / c×2）只活在 `discoveries/TASK-007.json` 里。该文件开篇自述入库理由正是「报告会随 Sprint 归档，而这些契约不会随之失效」——把新分类留在 discovery 里，正是它被创建来防止的那件事。表头计数 16/6/5 现在只覆盖包的一半 | 建议 Leader 把 T7 的分类折进 CONTRACTS.md |
| S5 | `parse_test.go:118` | `TestCaliberForIsOrderIndependent` 的旋转循环是 `for shift := 1; shift < len(original)`，表长 2 ⇒ **只执行 1 次**；表若缩到 1 条，该测试变成平凡通过而无自证。属「空集平凡满足」族 | 建议加一行 `require.GreaterOrEqual(t, len(original), 2, ...)`。影响低（`TestCaliberFor` 钉住绝对值） |
| S6 | `profiles.go:134` vs `:174` | `tsfFlowRE` 与 `sectorFlowRE` 函数体**逐字节相同**，无注释论证为何分成两个 | lens 实测：删掉 `tsfFlowRE` 并重指调用点 → 358 PASS / 0 FAIL / exit 0 |
| S7 | `parse_test.go:65` | `TestParseTitlePadsMonth` 被 `TestParseTitle` 严格包含（后者已含 `2026年6月`/`2026年1月` 两条 + 每条的 `\A\d{4}-\d{2}\z` 断言） | lens 消融：删掉它后变异 `%02d`→`%d`，`TestParseTitle` 仍红 2 个子测试。建议删函数、把 bitemporal 重复键那段理由移到用例上 |

---

## S1 专节 · `section.has` —— 判：**删除，且 T3 的保留理由已过期**

**位置**：`internal/hestia/sections.go:20`　**归属**：TASK-003（若 Leader 一并开 review_fix 则捎带；否则可作为独立清理）

**技术事实**（本人 grep 复核）：`grep '\.has('` → **4 处全在 `sections_test.go`**（197/198/199/282），**生产 0 处**。lens 删除实验：357 PASS / 0 FAIL / exit 0，单测试差额就是 `TestSectionHas` 自身。

**历史上下文（lens 的 grep 结构上看不到，由 Leader 补入）**：T3 交付时**刻意保留**它，用途声明为「给 T5 在定位到板块之后判断可选句式」；dev-agent-45 在 T5 报告里专门记了一笔「不是死代码，记一笔免得后续被清理掉」；test-agent-22 在 T5 验证时列为 P4 并确认。**它预警过的事现在正在发生。**

**但真相是三段，不是两段**：T3 留了它 → **T5 最终没用上**（T5 discovery 明记「七类句式都靠固定锚点定位，没有需要先定位板块再判断可选句式的场景」）→ 现在无人调用。**「留给 T5」这个理由已经过期，而没人回头撤销那个保留决定。**

**判据不采用「当下有没有调用者」，而采用 Leader 建议的「M1b-3 会不会用到」——我的判定是：结构上不会，而且比「不会用」更强一层。**

1. **M1b-3 结构上够不到它**。M1b-3 是入库与校验，消费的是 `Observation`（`Meta` + `Values`）；`section` 是解析层的**未导出类型**，M1b-3 拿不到 `[]section`，也不该拿到——那等于让校验层重新解析 HTML。
2. **更强的一层：`has` 所体现的「可选句式」概念与本包已交付的设计相抵触。** `extract.go` 的纪律 3 是「**任何模板未命中一律报错**」，`extractFields` 要么全成要么返回 nil。「本板块有没有某个可选句式」正是这个设计**明确拒绝**的模型——它对应的是「字段可缺失」，而该包把「缺失」的语义整个让给了 `extractor` 版本。⇒ `has` 不只是暂时没人用，它是**另一套设计的残件**。
3. **它什么时候会回来，以及那时该怎么做**：只有当 M1b-3 选了 Q4 的出路 (ii)（让抽取变成部分成功、逐字段记命中/未命中）时，「可选句式」才重新成为一等概念。**届时应当带着确定的消费者重新引入**，而不是让 14 行生产代码 + 一句已被证伪的文档注释在这里再挂两个 Sprint——那句注释现在指名了一个**不存在**的消费者，比没有注释更糟（本包自己的判据：恒响的检查会被训练成忽略，说谎的注释同理）。

⇒ **删除 `section.has` 与 `TestSectionHas`，把 `sections_test.go:282` 那处前置条件改写成 `require.Contains(t, secs[5].Body, …)`。** 并在提交信息里写明「T3『留给 T5』的保留理由已随 T5 交付而过期，本次撤销该保留决定」——否则下一个人会重新发现同一件事再争一次，而这已经是第二次了。

---

## 已确认**不是**缺口（附读数，供后续 review 不重复提出）

- **`extract.go` ↔ `profiles.go` 的捕获组下标耦合**：`selectRMBBalance` 返回的 `m` 被按 `m[2..5]` 取用，看似脆弱。**本人变异实测**：在 `balanceRE` 中间插入一个捕获组 → `vet_exit=0`，**339 PASS / 19 FAIL**，12 个顶层测试转红（含 `TestBalanceRegexpClassifiesCurrencyQualifier` / `TestExtractFieldsOnV2Sample` / `TestParseRealSamples`）。下标一漂就红，**有效守卫存在**（靠 `TestBalanceRegexpClassifiesCurrencyQualifier` 逐个断言 `m[1..5]`，而非靠 `NumSubexp`）。
- **孪生句四道防线**（本外币/外币口径、期次前缀、0 命中不退邻句、≥2 命中不静默选）：lens 变异 M14/M15/M16/M17 全部 KILLED。
- **币种丢失**：不成立。每个字段的币种由构造方式钉死（`selectRMBBalance` 只取「人民币」那句并要求唯一；`fxReserveRE` 写死「万亿美元」），无任何字段的币种运行时可变。
- **`Values` 缺 provenance**：**不构成「现在不改以后付大代价」**。每个字段由**恰好一条**模板产出（`TestTemplateTablesHaveNoDuplicateFields` + `collector.set` 拒重复，双向钉死），「字段名 → 模板 → 句式」在 `profiles.go` 的静态表里是确定映射，闸门报字段名即可反查；将来要加是纯追加（不改 `Store.Save`、不改 schema）。按本项目禁投机性设计的规范，**不建议现在做**。
- `singletonFields()` / `allTemplateRegexps()` 生产引用 0 处、仅测试消费——但各自注释写明了理由（覆盖度记账 / 「新增模板时请一并登记」的邻接性），属**有论证的冗余**，保留。

---

## 对 Leader 四个重点问题的直接回答

### Q1 · 四层边界是否真的干净？

**边界划分本身是干净的**：`detectExtractor` 与 `sectionRules[0]` 共用 `tsfSectionKeyword` 常量而非各写一份；`extractFields` 不对「detect 已保证 v2 必有社融两节」做隐式假设，而是重新 `findSection` 并报错；`store.go` 与解析层零耦合（`TestParseDoesNotTouchStorage` 走 `go/parser` 而非子串匹配）。已登记的三条跨层契约（`(?m)^` 依赖块级换行、`section.Title/Body` 已 TrimSpace、`splitSections` 返回 nil 依赖紧邻的 `detectExtractor`）都有测试守护。

**但有一处未登记的接缝，而三条 (c) 级隐式耦合全部落在它上面**：**`stripHTML` 的输出形态契约只声明了「块级标签变换行」这一维，没有声明「空白」这一维。**

| # | 依赖 | 类 | 破坏后果 |
|---|---|---|---|
| C-a | `sectionTitleRE` 的 `(?m)^` 依赖「标题行**行首无空白**」 | **c** | **CRITICAL-1** |
| C-b | `splitSections` 静默丢弃首个标题之前的全部正文，丢了多少**无任何一层可观测** | **c** | CRITICAL-1 的放大器：被吞掉的内容连「正文里提过社融」这种兜底判据都够不到 |
| C-c | profiles.go 全部模板**零空白容忍**（`，同比` 要求严格相邻），而 `spaceRE` 只折叠、从不删除空白 | **c** | 实测为**响亮失败**（`余额340.29&nbsp;万亿元` → `moneyRE` 不命中 → mustMatch 报错），危害有限，但这条前提没被任何注释或测试声明过 |

另有一条 (b)：`extractorV1/V2` ⊆ `types.go` 的 `validExtractors`（`types.go:99` 写了「新增 rule@vN 时必须同步更新这里」，但只由两份样本间接覆盖；加 `rule@v3` 而忘改 types.go，在拿到 v3 样本之前零测试转红——届时 `Save` 会响亮拒绝，故是 (b) 不是缺陷）。

### Q2 · 错误处理是否有一条可陈述的规则？

**有，而且是本包最值得称道的地方。** 十四处 error 分支的措辞、每处「为什么不降级」的理由、两处静默跳过都写在**静态表**而不是控制流里，一以贯之：

> **凡是从输入文本读来的东西，认不出就返回 error，绝不猜；静默跳过只允许发生在静态表已声明的位置（`sectionRule.v2Only`、`loanScope.totalField == ""`），因为那是「按设计不存在」而非「这次没读到」；panic 只留给「构造函数的 error 被忽略」这类编码错误，它不是输入的函数。**

**偏离点恰好一处**：`Parse` 对 `PubDate` 是「读到什么算什么」（WARNING-2）。`splitSections` 返回 nil 与 `scaleOf` 的 panic 都**在规则内**，论证充分且有测试守着。

### Q3 · 测试的守护强度（除 1e-6 外还有哪些「断言存在但守护不住」）

按 Leader 点名的三族给结论：

- **参数无守卫**：`mustMatch` 的「取第一个」——**变异实测 SURVIVED，358 PASS = 基线**（WARNING-1）。这是除 1e-6 之外**唯一一条我认为必须闭合**的同族问题。另有轻量三条：lens 实测 `blockTagRE` 的 17 个标签仅 `p` 受守护（压成 `(?:p)` → SURVIVED）、`spaceRE` 去掉 `\x{00a0}` → SURVIVED、`blankLineRE` `\n{3,}`→`\n{2,}` → SURVIVED。
- **空集平凡满足**：`TestCaliberForIsOrderIndependent` 的旋转循环（S5）。其余我逐条查过的都带自证（`assertMatchesGolden` 的 `require.NotEmpty(want)`、`TestParseValuesKeysAreDeclared` 与 `TestParseDoesNotTouchStorage` 各自的 `NotEmpty`/`NotZero`、`TestNoGreedyCaptureInTemplates` 的 `require.NotEmpty(lits)`），这一族总体做得很好。
- **竞争性错误路径**：我逐条验过 `TestParseRejectsMonthlyUntilSampled`（`Contains("monthly")` 把 `detectExtractor` 的竞争错误区分开）、`TestParseRejectsMissingMeta`（两个用例的 `Contains` 都有鉴别力）、`TestParseStopsAtDetectExtractor`（`Contains("0")`/`Contains("3")` 今天有鉴别力）——**这一族没有发现问题**。T5 在变异中自己抓出并修好的 `TestExtractFieldsRejectsUnknownExtractor`（`require.Error` 被「找不到板块」满足）是这族的教科书处置。
- 一条鉴别力不足但无危害的：`TestFootnoteSectionNeverWinsFindSection`（`sections_test.go:427`）对它自称守护的性质无鉴别力——lens 变异「`findSection` 改回 `s.has`」KILLED 了 7 条测试，**本条不在其中**；真正钉住该机制的是 `TestFindSectionIgnoresBodyOnlyMentions`。建议改注释或加强断言。

### Q4 · 对 M1b-3 的接口是否够用

**`Observation` / `Meta` / `Values` 的形状够用，`provenance` 与币种都不构成欠账**（理由见「已确认不是缺口」）。但有两件事必须在 M1b-3 写 profile / 写闸门表**之前**定案：

1. **completeness 闸门在当前设计下恒真**。`extract.go` 的纪律 3 是「任何模板未命中一律报错」，`extractFields` 要么全成要么返回 nil ⇒ `Parse` 的输出键数**只可能是 54 或 27**。两条互斥出路：
   - **(i) 认它是恒真的纵深防御** ⇒ 那就**别手写第四份字段划分表**，v1/v2 必填集应从 `sectionRules` + `v2Only` 派生（`fields.go:3-5` 自己写着「手写多份必然不同步」）；
   - **(ii) 让抽取变成部分成功**（逐字段记命中/未命中）⇒ completeness 才有信号。注意 `types.go:187` 的 `Check.Reason = "absent_field:<name>"` 与整套 pending 机制，**读起来就是为「同一 extractor 内某字段可缺失」准备的**——数据模型与 `extract.go` 的「全有或全无」纪律之间有一条**从未被声明的张力**。当前设计下，央行改一句话会让整期 54 个字段全部落空，没有「入 53 个、标 1 个 pending」的路径。
2. **CRITICAL-1 是 M1b-3 的前提条件，不只是解析层的小补丁**。「本期该有哪些字段」由 `extractor` 单点决定，`extractor` 由 `detectExtractor` 单点决定；判错时「缺失」的语义整体失效，而下游没有任何一层能发现（`Meta.validate` 放行、白名单放行、量级闸门看到的 27 个值全部正确）。**守住那个单点比守住闸门本身便宜一个数量级。**

---

## 审查过程本身的一条记录 · Minimalist lens 的自我证伪

Minimalist lens 在发出结论**之前**自己撤回了一条发现，原文：

> 我最初的假设「`currencyAlt` 的前缀不变量守错了属性」**被探针证伪**——Go 的 leftmost-**START** 规则让 `本外币`@pos0 胜过 `外币`@pos1（`alt=外币|本外币|人民币` → hits=2，currency 为「本外币」/「人民币」，与现行顺序结果相同），现有不变量是健全的。**我丢弃了这条发现，而不是把它发出来。**

**这条值得单独记，理由与它的内容无关，与它的形状有关。** 本 Sprint 有四次「结论对但理由错」（交替顺序、放松锚点取 1.36、「T5 喂固定输入」、`require.NotEmpty` 不可达），其中一条经三个人的手都没被验。**这是唯一一个在发出前自证伪并撤回的。**

它也顺带说明了本包 `TestNoAlternativeIsPrefixOfAnother` 那条不变量（「任何一项都不是另一项的**前缀**」）是**健全**的，不必因为「本外币 / 外币」看起来像包含关系而重新讨论——**已被独立探针验过一次，请勿再争**。

---

## 建议的 review_fix（供 Leader 决策）

`reason_class = task_defect`（实现与 done_criteria 不符：T3 的「未知版式一律 error 不降级」在 v2→v1 这一路上不成立）

| 任务 | fix_items（可测） |
|---|---|
| **TASK-003** | 1. `splitSections` / `detectExtractor` 增加「板块序号从「一、」起连续」校验，不满足即 error 且信息含实际首个序号与实际板块数。判据：以 2025 样本、仅在 `<strong>一、`/`<strong>二、` 前插入 `&nbsp;` 构造的输入，`Parse` 必须返回 error 且**不产出任何 Values**（当前返回 `err=nil` + 27 个字段）。2. 两份真实样本必须仍全绿（已实测：序号 一–八 / 一–六 均连续）。 |
| **TASK-005** | 1. `mustMatch` 改用 `FindAllStringSubmatch`，`len(all) != 1` 时 error，信息含命中数与模板。判据：变异「取最后一个匹配」必须打红（当前 SURVIVED，358 PASS = 基线）。2. 补常驻断言：两份真实样本上每条清单模板命中数恰为 1。3. 既有套件必须仍 358 PASS / 0 FAIL（已实测：加唯一性校验不打红任何真实样本）。 |
| **TASK-006** | 1. `Parse` 的 PubDate 判据改为 `!ok || pubDate == ""`，两种情形给不同措辞；建议一并加 `publishedAtRE` 形态校验。判据：把 2025 样本 PubDate 改成 `content=""` / `2026-1-15` / `2026-01-15 09:30:00`，`Parse` 必须 error 且信息含 `PubDate`，且**不产出任何 Values**（当前三种全部 `err=nil` + 54 个字段）。 |

**Leader 可选的另一条路**：CRITICAL-1 与 WARNING-1 都不影响本 Sprint 交付物的**当前正确性**（`Parse` 尚未接线到 `Save`，导出面净增量只有 `Parse`）。若倾向于不打断 8 个已 verified 任务，可将三条转为 M1b-3 的**开工前置条件**并写进 CONTRACTS.md。**但 CRITICAL-1 必须在 M1b-4 把 `Parse` 接到 `Save` 之前关闭**——它一旦接线就是静默写坏权威表的路径。这是范围决策，属 Leader 与人类的判断，不属我。
