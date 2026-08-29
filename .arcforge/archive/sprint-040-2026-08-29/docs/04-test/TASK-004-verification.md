# TASK-004 验证报告 — detectExtractor 判据重构：板块集合 × period_type

- **验证者**：test-m1c3a-v2
- **判定**：**VERIFIED**（8/8 条 done_criteria 通过）
- **判定对象**：`verify_baseline.head` = `33f3ed7cb4afabec0c1dae5d1ca2416d9421955f`
  （交付 commit `23aa880c7488e0374e85eb62baa89b77c9a6f6c1`）
- **收尾时 HEAD 未漂移**（仍是 `33f3ed7`）；`discoveries/TASK-004.json` 的 sha256
  `a8a27740…` 与 `verify_baseline.discovery_sha256` 一致 ⇒ dev 在 `verifying` 窗口内未改 discovery。
- **交付物指纹**：我在隔离副本上独立计算的 7 项 sha256 与 discovery 记录**逐字一致**
  （`sections.go e0c71d17…` / `sections_test.go d557e1cc…` / `parse.go 9aafa293…` /
  `extract_test.go 603f8a50…` / 三份 testdata `f97fa9aa…` `6425aea2…` `420f38d9…`）。
  三份新快照 **CRLF 行数均为 0**（我用 `git archive 33f3ed7` 取的副本上直接数）。

---

## 0. 我自己重采的数字（全部亲跑）

| 指标 | 我实测 @ `33f3ed7` | dev 报（@ `23aa880`） |
|---|---|---|
| `go test ./internal/hestia/ -count=1` | rc=0，**0 FAIL** | ok，0 FAIL |
| RUN 计数 | **1037 = 512 顶层 + 525 子测试** | 1037 = 512 + 525 |
| 覆盖率（coverprofile 逐块累加） | **95.6204%（1703/1781）** | 95.6204%（1703/1781） |
| `gofmt -l internal/hestia/` | 空 | 空 |
| `go vet ./internal/hestia/` | 空 | 空 |
| `git show --numstat 23aa880` | +1339 / −59（7 文件，逐项对得上） | 同 |

计数口径自洽校验：`512 + 525 == 1037` 成立。覆盖率由 coverprofile 逐块累加
（`go tool cover -func` 只给一位小数 `95.6%`，判不了 95.5% 门槛的精确余量）。

---

## 1. done_criteria 覆盖矩阵（8 条，逐条）

| # | 完成标准 | 对应测试 | 我实际跑的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 抓三份快照进 `testdata/`，原 `file` 名记进 discovery | — | 三份快照在 commit 内（+507/+441/+438）；discovery 的 `files_modified` 逐份记了原名（`articles/5837479.html` 等）；CRLF=0 | PASS |
| functional[1] | 签名增 `periodType`；判据改为四维、**六条出口**如 description 所列 | `TestDetectExtractorNewLayouts`（6 条出口逐条走） | 读代码逐条比对 description 的 switch：**六条出口与实现逐条一致**（含「有外汇节的月报判 `rule@v1`」这一格——布局全景表「6 节 3 月报 → rule@v1」正是它）。我的独立探针实打五份快照 ⇒ 判定全部正确 | PASS |
| functional[2] | `coreSectionKeywords()` **从 `sectionRules` 派生**，不手写第二份清单 | `TestCoreSectionKeywordsIsDerivedNotHandwritten`（逐字面量）+ `TestCoreSectionKeywordsFollowsSectionRules`（哨兵） | 探针实打 `coreSectionKeywords() = [广义货币 人民币存款 人民币贷款 加权平均利率]`；读代码确认遍历 `sectionRules` 按 `!v2Only && keyword != fxSectionKeyword` 过滤 | PASS |
| boundary[0] | 三份新快照判定正确；**既有 6 节 v1 / 8 节 v2 全绿不变** | `TestDetectExtractorNewLayouts` | **我的独立探针**（不复用 dev 的断言）：`q1q3` 5节 hasFX=t hasTSF=f → `rule@v1`；`2025-08` 5节 f/f → `rule-monthly@v1`；`2020-04` 4节 f/f → `rule-monthly@v1`；既有 `annual` 8节 t/t → `rule@v2`、`h1` 6节 t/f → `rule@v1`（**结论不变**） | PASS |
| boundary[1] | 🔴 真截断仍被拒（下界缝）+ 上界显式决定 | `TestDetectExtractorRejectsTruncatedFXOutsideMonthly`、`TestDetectExtractorAcceptsExtraIrrelevantSections` | 消融 **R-M1**（让 `periodType` 恒为 monthly）⇒ KILLED，**锚定单跑红、RUN=1**，红在 `:686 require.Error`，**而它前面的前提断言 `:682 require.Empty(missingCoreSections)` 保持绿** ⇒ DoD 点名的 `require`-先红陷阱**确实没有发生**。上界见 §3 | PASS |
| boundary[2] | 缺**核心**板块 ⇒ 报错，**不论 periodType** | `TestDetectExtractorRejectsMissingCoreSection`（遍历 5 种 periodType） | 我的新变异 **N3**（把核心闸挪到 FX 闸之后）⇒ KILLED，红在 `:273 assert.Contains`（不是 `:272 require.Error`）⇒ 该用例守的是**哪一类错误**，不只是「报没报错」 | PASS |
| error_handling[0] | 错误信息列出**缺了哪几个**核心板块；两类错误措辞**可区分** | `TestDetectExtractorErrorsAreDistinguishable`（交叉断言）、`…RejectsMissingCoreSection/缺多节时逐个列出` | 探针实打错误串含缺失清单；N3 独立确认「可区分」这条真在守（见上）；dev 的 P3（只报第一个缺失项）我未复跑，但 N3 从另一侧覆盖了同一条断言 | PASS |
| non_functional[0] | gofmt/vet/test 绿、覆盖率 ≥95.5%；无新增依赖；越界文件已申报；注释带 milestone 前缀 | — | 见 §0（95.6204% ≥ 95.5%）；`go.mod`/`go.sum` 未出现；**实际改动 7 文件与 `writes` 声明 `diff` 逐条一致**；`detectExtractor` 全部 22 个调用点分布在 4 个文件，**全部在 `writes` 内**（无遗漏的受影响文件）；`sectionsV1/V2` 除墓碑注释外**无残留引用**；本次 diff 新增的 10 处 `TASK-004` 引用全部带 `M1c-3a` 前缀 | PASS |

⚠️ **关于 milestone 前缀我差点报错**：`grep` 单行匹配打出一个「裸 `TASK-004`」，实为
`sections.go:110-111` 的注释**跨行**（「随 M1c-3a 的 / TASK-004 换判据一起删除」），前缀在场。
我用「单行内是否含前缀」代替了「引用是否带前缀」这个性质，读原文才发现。包内其余裸引用
全部是往届 sprint（M1b-4b / M1c-1 / M1c-2）的既有注释，不在本次 diff 内。

---

## 2. 🔴 待裁决项：第七条出口「有社融增量节、没有存量节 ⇒ 报错」

**裁决结论：保留。与 Leader 的批准同向——但我实测出 dev 给的理由有一半不成立，
而修正后的理由比原理由更强。**

### 问题 ①「旧判据真的提供了这个保护吗」⇒ **只提供了一部分。dev 的描述过强。**

我在 `git archive 20c05ea` 的隔离副本上直接调**旧** `detectExtractor` 探针（只打印，不断言）：

```
PROBE-OLD 6节：增量+核心四+外汇（无存量）   len=6 ⇒ 判成 rule@v1 ← **保护缺失**
PROBE-OLD 7节：同形态+一个无关节            len=7 ⇒ 报错(受保护)
PROBE-OLD 8节：同形态+两个无关节            len=8 ⇒ 报错(受保护)
```

旧判据是 `!hasTSF && len(secs)==6 → rule@v1`。「有增量节、无存量节」的输入若**恰好是 6 节**，
旧判据同样**静默判成 `rule@v1`** —— 与新判据去掉守卫后的行为**一模一样**。
保护只在 `len != 6` 时存在，是**节数依赖**的。

🔴 **最能说明问题的是：dev 自己写的那条用例，输入正好是 6 节**
（`sections_test.go:262-266`，「一、社会融资规模增量…六、国家外汇储备」）。
⇒ 那条用例所守的性质，**在旧代码里根本不成立**。

### 问题 ②「新判据真的会弄丢它吗」⇒ **是。**

消融 **R-M5**（整段删掉该守卫）⇒ KILLED，**只有 `:272 require.Error` 一条断言红**，
只有 `…RejectsUnknownLayout/有社融增量节但没有存量节` 一个用例红。
我的新判据探针也确认：带守卫时 6/7/8 节 × `annual`/`monthly` **六种组合全部报错**，
不依赖节数、不依赖 periodType。

### 问题 ③「实现是否真从 `sectionRules` 的 `v2Only` 派生」⇒ **是**（我用与 dev 独立的**双向**哨兵）

```
PROBE-NEW 哨兵(v2Only=true)  被 hasAnyTSFSection 认出 = true   ⇒ 真派生
PROBE-NEW 哨兵(v2Only=false) 被认出               = false  ⇒ 只看 v2Only，没顺手放宽
```

读代码确认 `hasAnyTSFSection` 遍历 `sectionRules` 取 `v2Only` 的条目，**不新增任何字面量**。

### 问题 ④「消融 M5 是否精确命中」⇒ **是**

全套只红 1 个用例、1 条断言（`:272`）。锚定单跑
`-run '^TestDetectExtractorRejectsUnknownLayout$/^有社融增量节但没有存量节$'` ⇒ rc=1、**RUN 行数=2**
（父+子，非 0 ⇒ 不是「`-run` 没匹配到」的假绿）。

### ⇒ 裁决

**保留这条出口。** 它满足 boundary[1] 的「要么报错，要么有明确理由接受」，有精确单一的守卫，
从真相源派生，不新增字面量。**而它的正确定性比 dev 陈述的更强**：

> 它不是「补回旧判据顺带提供的保护」，而是**新增一道旧代码从未完整提供的保护**
> —— 旧保护在 6 节形态上有洞，新守卫把那个洞也堵上了。

**建议（不阻断本次验收）**：`sections.go:317-321` 的注释写着
「旧判据靠 `len(secs)==8 && !hasTSF` 落进 default 报错」——该句对 8 节成立、**对 6 节不成立**，
而注释会被后人当作实测结论读。建议在后续任务里订正为「旧判据只在 `len != 6` 时挡得住」。
**我不在 `verifying` 窗口内改交付物**（判定对象漂移），故只记录。

---

## 3. Leader 点名的另外两条复核

### 3.1 上界理由③「多出的板块在抽取层结构性不可达」是否**做成了可核查的断言**

**结论：是，但要精确说明它验证了什么。**

那三条断言（`sections_test.go:723-727`）遍历 `sectionRules` × 多出的两节，
断言标题**不含**任何规则关键词。我的新变异 **N1a** 检验它的鉴别力：
把多出板块之一改成 `"六、广义货币补充说明"`（含核心关键词）⇒ **KILLED**，
锚定单跑 rc=1、**RUN=1**，红在 `:725` 那条 `assert.NotContainsf`。
⇒ **它确实是可核查的观察，不是一句声称**（Leader 要我验证的这点成立）。

**边界（我的独立观察）**：它验证的命题是「**这两个构造的板块**不被任何 `sectionRule` 认领」，
**不**验证理由③的结构性前提「`extractFields` 从不遍历所有板块」——
`grep` 确认该测试函数体内 **0 处** `extractFields`。后者是 `extract.go` 的既有实现形态，
本任务只读该文件，不在其守卫范围内。这不是缺口，是**射程说明**：
读那段注释的人可能以为「结构性不可达」整句都被钉住了，实际被钉住的是它在本例上的实例化。

⚠️ 我最初设计的 N1（在 `extractFields` 里加一个遍历 `secs` 的循环）**SURVIVED，但那个结果不可用**
——我加的是 `for _, sec := range secs { _ = sec }`，一个 no-op，**它并没有真的破坏
「多出板块不被读到」这个性质**。按「这个变异真的破坏了要守的性质吗」这一问，它不成立，
故我作废该结果并改做 N1a。记在这里是因为：**那个 SURVIVED 看起来完全像一条缺陷证据。**

### 3.2 探针 P6（`coreSectionKeywords` 倒序）SURVIVED 判为等价变异 ⇒ **我独立复核后同意**

我复跑（改用手写倒序循环，因为 `sections.go` 未 import `slices`——**语法闸正确拦下了我第一版**）：
rc=0，零条红，SURVIVED 复现。

先问「这个变异破坏了 DoD 要的那个性质吗」：

- functional[2] 要的是「**从 `sectionRules` 派生**」——倒序仍是派生，性质不变。
- error_handling[0] 要的是「列出**缺了哪几个**」——不是「按什么顺序列」。倒序后仍**全部**列出，
  而 `…RejectsMissingCoreSection/缺多节时逐个列出` 用的是逐个 `assert.Containsf`，与顺序无关。
- 顺序唯一的下游是错误信息里 `strings.Join(missing, ", ")` 的排列，不影响任何判定分支。

⇒ 顺序不构成契约，P6 是**等价变异而非漏网缺陷**。加断言会把偶然属性冻结成契约。
与 TASK-003 的 M11 同形，判读同样准确。

---

## 4. 消融证据（全部我亲跑，隔离副本，harness 独立实现）

**方法**：`git archive <全 sha> | tar -x` 到 `mktemp -d`，**两个**副本
（新 `33f3ed7` 判定对象 / 旧 `20c05ea` 用于裁决问题 ①）。harness 由我独立实现，
**未复用** dev 的 `isolate4-m1c3a-c.py`（复用被验方的工具会连它的盲区一起继承）。
锚点可覆写（`ARCFORGE_MUT_REF` / `ARCFORGE_MUT_BASE`），**钉全 sha**。
每个变异：逐字替换（锚点出现次数必须恰为 1）→ 打印变异体 diff 逐字核对（语义闸）→
`go build`（语法闸）→ 跑全套 `-v` → 解析**哪条断言**红 → 关键项**锚定单跑并核 RUN 行数** → 还原并校验。

**吸收自 dev 的一条自查**：锚定单跑要核 **RUN 行数**——`-run` 表达式拼错时 0 条 RUN 也是 `rc=0`，
会被读成「单跑通过」。我全部三次锚定单跑都核了（RUN=1 / 2 / 1，均非 0）。

**卫生指纹**：每个变异窗口内 + 收尾各校验一次主工作区 5 个文件 sha256 与 `git status --porcelain`，
**全程未变**（收尾 `diff` 两份快照零差异）。两个隔离副本已 `rm -rf` 拆除。

| ID | 变异 | 结果 | 哪条断言红 | 锚定单跑 |
|---|---|---|---|---|
| **R-M1** | 让 `periodType` 恒为 monthly（FX 守卫失效） | KILLED | `:686` `:782` `:817`；**`:682` 前提断言保持绿** | rc=1，RUN=1 |
| **R-M5** | 整段删掉「有增量节没存量节」守卫 | KILLED | **只有 `:272`**（精确命中） | rc=1，RUN=2 |
| **R-P6** | `coreSectionKeywords` 倒序 | **SURVIVED**（等价变异，见 §3.2） | — | — |
| **N1**（作废） | `extractFields` 加 no-op 遍历 | SURVIVED，**结果不可用**（未真正破坏目标性质，见 §3.1） | — | — |
| **N1a** | 多出板块标题改成含 `广义货币` | KILLED | `:725`（理由③那条断言） | rc=1，RUN=1 |
| **N2** | 让 `annual` 也豁免外汇节 | KILLED | `:782` `:817`；**`:824` 未红**（见 §5-A） | — |
| **N3** | 核心闸挪到 FX 闸之后（只换顺序，两道闸都在） | KILLED | **`:273`**（`assert.Contains`），非 `:272` | — |

**N3 的因果特别干净**：只红了「核心板块一个都没有」与「空文档」两个用例，
**没红**「缺核心板块之一（人民币贷款）」——因为那个输入**有**外汇节、FX 闸不触发，仍走核心闸。
⇒ 证明 `…RejectsUnknownLayout` 的 `assert.Contains(err, tc.want)` 真在守**错误的类别**，
不只是「有没有报错」。这是 error_handling[0]「两类措辞可区分」的独立确认，
用的是 dev 生成集之外的变异（顺序调换）。

---

## 5. 两条观察（**不构成缺陷**，供 Leader 与后续任务知悉）

### 观察 A：`TestMonthlyIsTheOnlyPeriodTypeExemptFromFX` 的 `assert.Equal(t, 1, exempt)`（`:824`）恒真

```go
var exempt int
for pt := range validPeriodTypes {
    t.Run(pt, func(t *testing.T) { … })
    if pt == periodTypeMonthly { exempt++ }   // ← 递增条件与 detectExtractor 的行为无关
}
assert.Equal(t, 1, exempt, "豁免的必须**恰好**是一个 period_type")
```

`exempt` 的递增条件是 `pt == periodTypeMonthly`，**不是「detectExtractor 真的豁免了它」**。
map 键唯一 + 前提断言 `require.True(validPeriodTypes[periodTypeMonthly])`
⇒ `exempt == 1` **恒真**，任何实现变化都不可能让它红。

**实测（不止于推导）**：我的 **N2**（让 `annual` 也豁免）⇒ KILLED，
但红的是 `:782` 与 `:817`（子测试 `/annual` 的 `require.Error`），**`:824` 没红**。

**为什么仍判 PASS**：那个性质**确实被守住了**——守它的是子测试里的 `require.Error`（`:817`），
不是 `:824`。DoD 也没有要求这条断言。
**留给下游的提示**：`:824` 的注释（「豁免的必须**恰好**是一个」）会让人以为这件事由它守着。
若日后有人重构掉那些子测试，`:824` 会留下「还有守卫」的错觉。

### 观察 B：理由③断言的射程比它上方的注释窄（详见 §3.1）

注释论证的是「`extractFields` 从不遍历所有板块 ⇒ 结构性不可达」，
断言钉住的是「这两个构造的板块不被任何规则认领」。前者是 `extract.go` 的实现形态
（本任务只读），后者才是被钉住的。**断言有鉴别力（N1a 证实），只是射程比论证窄。**

---

## 6. dev 自述的核实

| 自述 | 我的核实 | 结论 |
|---|---|---|
| 三份快照 CRLF 已在提交前规范化 | 我在 `git archive 33f3ed7` 的副本上直接数：三份 **CRLF 行数均为 0** | 属实 |
| M1 下 `:682` 前提断言保持绿（require 陷阱未发生） | 我复跑 R-M1，红名单为 `:686 :782 :817`，**`:682` 不在其中** | 属实 |
| M5 精确命中，只有那一条用例红 | 我复跑 R-M5：只红 1 个用例、1 条断言 `:272`；锚定单跑 RUN=2 | 属实 |
| P6 是等价变异 | 我独立复跑并从「破坏了哪个性质」重新推导 | 判读准确 |
| 签名变更打破的是**两个** writes 之外的文件，已在 `dev_done` 前补进声明 | `grep` 全部 22 个 `detectExtractor` 调用点，分布 4 文件，**全部在 `writes` 内**；实际改动与 `writes` `diff` 逐条一致 | 属实，无遗漏 |
| code-simplifier 回报「Done. No action.」但实际改了两个文件，dev 用 diff 核实并重跑全部消融 | 我复跑 R-M1/R-M5 得到的断言行号与 discovery 记录**逐字相同**（若那批数字采自 simplifier 改动之前，行号会对不上） | 重采属实 |
| 旧判据顺带提供「增量无存量」保护 | **部分不成立**（见 §2 问题 ①）：6 节形态下旧判据同样静默判 `rule@v1` | 结论对、理由过强，建议订正注释 |

---

## 7. 结论

**VERIFIED。** 8 条 done_criteria 全部有对应测试、断言非空洞，且经消融证明确实在守
（`:686`/`:272`/`:725`/`:273` 各自被独家或近独家点亮，且我对每个 KILLED 都确认了
「红的是不是它自称在守的那条」）。自证数字全部由我重采并与 dev 报数逐字吻合。

**第七条出口裁决：保留**——理由比 dev 陈述的更强（它堵的洞比它声称补回的更大）。
一条注释表述建议订正，不阻断验收。

两条观察（`:824` 恒真、理由③断言射程）已记录，均不构成缺陷。
