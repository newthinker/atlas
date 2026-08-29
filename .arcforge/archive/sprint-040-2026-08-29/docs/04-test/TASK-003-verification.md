# TASK-003 验证报告 — extractor 取值域与必填集：新增 4 个值

- **验证者**：test-m1c3a-v2
- **判定**：**VERIFIED**（8/8 条 done_criteria 通过）
- **判定对象**：`verify_baseline.head` = `cea6b3cb4c172c17adea7cf8a3a224b605f75d93`
  （交付 commit `5372f8ed1a6f15088c516e2f5c819f595e516cfc`，merge 进 master 后的 sha）
- **交付物指纹**（我在隔离副本上独立计算，与 discovery 记录逐字一致）：
  - `types.go` `d5f515ce479f2d359d60cbd331a8b473b654eb97bbfc7d52ef4d6c53e72a4a0f`
  - `required.go` `2cfa8f72e18a6ea6a911ece4e2adabe4c01224181f43ede6b281be381edf6941`
  - `required_test.go` `ca03ef81464e5b0cdf42cb913b945b781ab58e58ed926d68cf74d6d4d2d710e0`

---

## 0. 我自己重采的数字（全部亲跑，不引用 dev 自述）

| 指标 | 我实测 @ `cea6b3c` | 我实测 @ 当前 HEAD `20c05ea` | dev 报（@ `5372f8e`） |
|---|---|---|---|
| `go test ./internal/hestia/ -count=1` | rc=0，0 FAIL | rc=0，0 FAIL | ok, 0 FAIL |
| RUN 计数 | **1013 = 504 顶层 + 509 子测试** | 1013 = 504 顶层 + 509 子测试 | 1013 = 504 + 509 |
| 覆盖率（`-coverprofile` 逐块累加） | **95.5581%（1678/1756）** | 95.5581%（1678/1756） | 95.5581%（1678/1756） |
| `gofmt -l internal/hestia/` | 空 | — | 空 |
| `go vet ./internal/hestia/` | 空 | — | 空 |
| `git show --numstat 5372f8e` | +302 / −19 | — | +302 / −19 |

计数口径自洽校验：`504 + 509 == 1013` 成立（两把独立的尺）。覆盖率由 coverprofile
逐块累加得出（`go tool cover -func` 的 total 只给一位小数 `95.5%`，不足以判 ≥95.5% 门槛）。

**关于 HEAD 漂移**：我开工时 HEAD = `cea6b3c`（等于 baseline），收尾时已前进到
`20c05ea`（`f337f88` TASK-001-fix 改 `profiles_test.go`）。
`git diff --stat cea6b3c..HEAD -- <本任务 writes 三文件>` **为空** ⇒ 漂移落在本任务声明范围**之外**。
判定对象未变。我的全部消融跑在 `git archive cea6b3c` 出来的隔离副本上，锚钉的是全 sha。
两个锚上的覆盖率与 RUN 计数**逐字相同**（TASK-001-fix 只收紧了断言，未改变被覆盖的语句集）
—— 此行是为下游着想：若 QA 用当前树重测，不会与我报的数对不上。

`discoveries/TASK-003.json` 的 sha256 = `75b55b9c…` 与 `verify_baseline.discovery_sha256`
一致 ⇒ dev 在 `verifying` 窗口内**未改动** discovery。

---

## 1. done_criteria 覆盖矩阵（8 条，逐条）

| # | 完成标准 | 对应测试 | 我实际跑的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `validExtractors` 增 4 值；常量定义在 `types.go`；`profiles.go` 不做任何改动 | `TestValidExtractorsAcceptsMonthlyAndTSFStandalone`（含 4 条逐字面量 `assert.Equal` 钉常量**值**） | 探针实测 `validExtractors(7) = [rule@v1 rule@v2 llm-fallback@v1 rule-monthly@v1 rule-monthly@v2 tsf-stock@v1 tsf-flow@v1]`；`git show --name-only` 改动文件仅三个，`profiles.go` 不在其中 | PASS |
| functional[1] | `requiredFields` 增 4 分支，字段数 25/52/18/9；全部从模板表派生，禁手写清单 | `TestRequiredFieldsMonthlyDropsOnlyFXSection`、`TestTSFStandaloneFieldsPartitionTSFSection`、`TestTSFStandaloneFieldsDeriveFromTemplateTables`（哨兵法） | 我的独立探针直接打印：`25 / 52 / 18 / 9`（不经 dev 的断言）；读 `required.go` 确认 `tsfStockFields`/`tsfFlowFields` 遍历 `tsfStockItems`/`tsfFlowItems`，月报走 `without(季报, fxSectionFields())` 递归派生 | PASS |
| functional[2] | 返回的切片必须是 `Clone` 或新建，不得交出底层数组 | `TestRequiredFieldsReturnsCopy`（本任务扩到六个 extractor，整体比对 `fieldOrder`） | 读代码逐分支确认：`without` 每次 `make` 新建、`tsfStockFields`/`tsfFlowFields` 每次 `make`、`extractorV2` 走 `slices.Clone`。测试实跑 7 条 RUN 全 PASS。⚠️ 守卫有效性见 §4 观察 A | PASS |
| boundary[0] | 月报差集**恰好**是 `fx_reserve` 与 `fx_rate`；反向差集为空 | `TestRequiredFieldsMonthlyDropsOnlyFXSection`（`ElementsMatch` + 反向遍历） | 探针实测 `diff v1: q\m=[fx_rate fx_reserve] m\q=[]`、`diff v2: q\m=[fx_rate fx_reserve] m\q=[]`；**消融 R-M2 复跑确认** `require.Len(25/52)` 在数量仍为 2 的错误实现下**保持绿**，只有 `:194 ElementsMatch` 与 `:276` 红 | PASS |
| boundary[1] | `stock ∪ flow == tsfSectionFields()` 的 27 键，交集为空，双向包含 | `TestTSFStandaloneFieldsPartitionTSFSection` | 探针实测 `|st|=18 |fl|=9 |sec|=27`，三个差集 `st\sec`、`fl\sec`、`sec\(st∪fl)` **全空**，`st∩fl` 空；三个总量归属逐个为 true。**消融 R-M1 + 我的新变异 N1 联合确认**三条逐字面量锚各自都在守（见 §3） | PASS |
| boundary[2] | 未知 extractor 仍返回 `nil`，`llm-fallback@v1` 既有行为一字不变 | `TestRequiredFieldsRejectsAmbiguousExtractor`（本次 diff 未触及该函数） | 探针实测 `llm-fallback@v1 = 0 (nil=true)`；`git show` 的删除行仅 5 行且全在旧版 `TestRequiredFieldsReturnsCopy` 内，该测试函数**一字未动**；**我的新变异 N3 确认**守卫有效（`return nil` → `return []string{}` 被 `:126/:127/:128` 三条 `assert.Nil` 独家杀掉） | PASS |
| error_handling[0] | 错误信息由白名单本身拼出，逐字出现全部合法值（**按 7 验收**，leader 已裁定） | `TestExtractorEnumErrorListsEveryValidValue`（7 条逐字 `Containsf` + `assert.Len(validExtractors, 7)`） | 探针实测真实错误串：`hestia: unknown meta.extractor "rule@v9" (want rule@v1｜rule@v2｜llm-fallback@v1｜rule-monthly@v1｜rule-monthly@v2｜tsf-stock@v1｜tsf-flow@v1)` —— 7 个值逐字在列 | PASS |
| non_functional[0] | gofmt/vet 空、整包测试全绿、覆盖率 ≥95.5%；无新增依赖；注释引用任务编号带 milestone 前缀 | — | 见 §0 表（95.5581% ≥ 95.5%）；`git show --name-only \| grep go.mod\|go.sum` 无匹配；新增行里 4 处 `TASK-003` **全部**带 `M1c-3a` 前缀，无裸编号 | PASS |

---

## 2. 「新增的四个值此刻没有生产调用方」——我独立核实

leader 要求我自己确认 dev 说的消费者真的存在，不只信自述。

- `grep -rn "requiredFields(" --include="*.go" internal/ | grep -v _test.go` ⇒ 生产侧只有
  **`internal/hestia/validate.go:459`** 一处（`gateCompleteness` 内），加上 `required.go` 内部
  两处递归自调用（月报分支）。dev 报的消费者**属实存在**。
- `grep -rn "validExtractors"` ⇒ 生产侧唯一消费者是 **`types.go:228`** 的
  `checkEnum("meta.extractor", …)`（`Meta.validate` 内）。属实。
- 我读了 `gateCompleteness` 全文：它只做成员判定（`for _, f := range req { if _, ok := in.obs.Values[f] }`），
  `req == nil` 时返回 `CheckSkipped`。

⇒ 分层属实：四个新值进了取值域与必填集表，但没有任何生产路径会**产生**它们
（`detectExtractor` 是 TASK-004、`extractFields` 是 TASK-006、`Parse` 分派是 TASK-007）。
这是刻意的，dev 已在 `types.go` 注释、commit message、discovery 三处写明，不构成缺陷。

---

## 3. 消融证据（全部我亲跑，隔离副本）

**方法**：`git archive cea6b3cb4c172c17adea7cf8a3a224b605f75d93 | tar -x` 到 `mktemp -d`，
harness 由我独立实现（不复用 dev 的脚本），锚点做成可覆写变量 `ARCFORGE_MUT_REF`，**钉全 sha**。
每个变异：逐字替换（锚点出现次数必须恰为 1）→ 打印变异体 diff 逐字核对（语义闸）→
`go build` 通过（语法闸）→ 跑全套 `-v` → 解析**哪条断言**红（不只看红不红）→ 还原并校验。

**卫生指纹**：每个变异窗口内 + 收尾各校验一次主工作区三文件 sha256 与 `git status --porcelain`，
**全程未变**（收尾 `diff` 两份指纹文件与两份 status 快照，均无差异）。隔离副本已 `rm -rf` 拆除。

### 3.1 复跑 dev 报的三个（核实它的自述）

| ID | 变异 | 我实测结果 | 与 dev 自述比对 |
|---|---|---|---|
| R-M1 | 三个社融总量划反（`tsf_flow_ytd`→存量、`tsf_stock_yoy`→增量），数量仍 18/9 | **KILLED**；红的**只有** `TestTSFStandaloneFieldsPartitionTSFSection`，断言行 **`:234` `:235`**。并集双向包含（`:217/:221`）、交集空（`:227`）、`require.Len(18/9)`（`:209/:210`）**全部保持绿** | 逐字一致 |
| R-M2 | `fxSectionFields()` 换成两个利率字段（`FieldRateIBO`/`FieldRateRepo`，数量仍为 2） | **KILLED**；断言行 **`:194`**（`ElementsMatch`）与 **`:276`**（FX 对绑）红，`require.Len(25/52)`（`:185`）**保持绿** | 逐字一致 |
| R-M11 | `tsfStockFields()` 返回前 `slices.Reverse` | **SURVIVED**（rc=0，零条红） | 一致 |

⚠️ **一个强旁证**：dev 自披露 code-simplifier 改过 `required_test.go` 之后它**重跑了全部消融**，
首轮数字作废。我复跑得到的断言行号（`234/235`、`194/276`）与它 discovery 里记的**逐字相同**
—— 首轮跑在旧版 `required_test.go` 上、行号必然不同 ⇒ **discovery 里给的确实是重采值**，
不是首轮遗留。这条不是靠它的声明，是靠行号对得上。

### 3.2 R-M11「等价变异」判读——我的独立复核结论：**同意**

先问「这个变异真的破坏了 DoD 要守的那个性质吗」：

- DoD boundary[1] 要的性质是**划分正确**（并集 == 27、交集空、三个总量归属对），与顺序无关。
  顺序反转后这四条性质**逐条仍然成立**。
- 唯一生产消费者 `gateCompleteness`（我亲自读过）只做成员判定；顺序仅影响
  `firstN(missing, 3)` 抽样出哪三个名字进 `Reason` 文案，不影响 `passed`/`failed`。
- 而 `tsf-stock@v1` 此刻**没有任何生产路径**能进 `gateCompleteness`（§2），也无测试覆盖那条文案。

⇒ 严格说 M11 不是数学上完全等价（它会换掉 `Reason` 里抽样的字段名），但该差异在当前代码下
**不可观察**。dev 的判读准确，且它自己就写明了这一点。**加顺序断言反而会把一个偶然属性
冻结成契约**（`tsfStockFields` 的自然顺序本就与 `fieldOrder` 不同）。标注 SURVIVED 是对的。

### 3.3 我自己设计的新变异（生成集之外，3 个）

拿生成处方的变异去验证由它导出的规则，结构上不可能失败，故全部另起。

| ID | 变异 | 它破坏的 DoD 性质 | 结果 | 哪条断言红 |
|---|---|---|---|---|
| **N1** | `FieldTSFStock` ↔ `FieldTSFFlowYTD` 对调（与 M1 换的是**另一对**总量） | boundary[1] 归属正确 | **KILLED** | `:233` `:235` |
| **N2** | `tsfStockFields()` 改为返回包级**共享缓存**（不再每次新建，内容全对） | functional[2]「必须是 Clone 或新建」 | KILLED（**但杀手另有其人**，见 §4-A） | `:217 :221 :233 :234`（Partition）+ `:253 :255 :256`（Derive）；**`TestRequiredFieldsReturnsCopy` 未红** |
| **N3** | `requiredFields` 的 `default` 分支 `return nil` → `return []string{}` | boundary[2]「`llm-fallback@v1` 既有行为一字不变」 | **KILLED** | `:126` `:127` `:128`（三条 `assert.Nil` 独家） |

**N1 的价值**：dev 的 M1 只点亮了 `:234`（`tsf_stock_yoy`）与 `:235`（`tsf_flow_ytd`），
**没有证明 `:233`（`tsf_stock`）也在守**。N1 换掉另一对，点亮 `:233` 与 `:235`。
两次合起来：三条逐字面量锚**各自都有至少一个变异独家点亮它**，无一冗余。

**N3 的价值**：boundary[2] 是「既有行为一字不变」这类最容易被当成不必验的条目。
`return []string{}` 是个真实的失误形态——空切片会让 `gateCompleteness` 从 `skipped`
变成 `passed`（`req == nil` 不再成立，`missing` 为空 ⇒ 直接过闸），把「无从校验」
静默变成「校验通过」。守卫抓住了。

---

## 4. 两条观察（**不构成缺陷**，但请 leader 知悉）

### 观察 A：functional[2] 的守卫判的是「`fieldOrder` 有没有被改」，不是「返回值是不是新建的」

N2 让 `tsfStockFields()` 返回包级共享缓存——内容全对、但违反 DoD 字面的「必须是 `Clone` 或新建」。
套件确实红了，但**因果不对**：

```
# N2 变异下，锚定单跑（^…$ 防前缀匹配）
go test ./internal/hestia/ -count=1 -run '^TestRequiredFieldsReturnsCopy$' -v
  ⇒ rc=0，7 条 RUN = 1 顶层 + 6 子测试，--- PASS
```

`TestRequiredFieldsReturnsCopy` 先跑，它把返回值全写成 `"tampered"`（即污染了缓存），
然后比对 `fieldOrder`——`fieldOrder` 没变，**它判绿**。红的是排在它后面、拿到被污染缓存的
另外两个测试。⇒ **这个变异是被测试间的副作用污染意外杀掉的**，不是被 functional[2] 的
守卫杀掉的。若 `ReturnsCopy` 排在文件最后，或若没有别的测试消费 `tsfStockFields()`，它会 SURVIVED。

**为什么仍判 PASS**：DoD functional[2] 的括号明确把关注点限定在 `fieldOrder`
（「`fieldOrder` 是 DDL、INSERT 列、白名单的共同真相源」），而**实现本身完全正确**
——我逐分支读过代码，四个新分支全部 `make` 新建。DoD 要的性质在交付物上成立。

**留给下游的提示**：这条守卫对「泄漏 `fieldOrder`」有效，对「返回其它共享数组」无效。
日后若有人给 `requiredFields` 加返回值缓存（一个很自然的"优化"），它不会喊。

### 观察 B：boundary[0] 的「反向差集为空」与 boundary[1] 的「交集为空」在配套断言下**恒真**

**推导**：设 `q = requiredFields(季报)`（无重复，27 或 54），`m = requiredFields(月报)`。
`require.Len(m, 25)` + `ElementsMatch(dropped, {fx_reserve, fx_rate})` 同时成立
⇒ `|q| − |q ∩ set(m)| = 2` ⇒ `|q ∩ set(m)| = 25`；又 `|set(m)| ≤ |m| = 25`
⇒ `set(m) ⊆ q` ⇒ **反向差集必为空**。
同理 boundary[1]：`|stock|=18`、`|flow|=9`、并集双向包含 27 个键
⇒ `27 ≤ |set(stock)| + |set(flow)| ≤ 27` ⇒ 两边各自无重复**且不相交** ⇒ **交集必为空**。

**把推导降落成观察**（消融实测，不止于纸面）：我在副本里删掉这两段断言，
基线仍全绿，再重跑三个相关变异：

```
消融后 R-M1  ⇒ KILLED（:224 :225，即原 :234 :235 前移）
消融后 R-M2  ⇒ KILLED（:194 :266，即原 :194 :276 前移）
消融后 N1    ⇒ KILLED（:223 :225，即原 :233 :235 前移）
```

无一从 KILLED 变 SURVIVED ⇒ 这两条断言在这批变异下**零边际杀伤力**。

**为什么仍判 PASS**：DoD boundary[0] 逐字要求了「且反向差集为空」，boundary[1] 逐字要求了
「双向包含断言」与「交集为空」——**dev 照 DoD 做了**，这是 DoD 的冗余而非 dev 的缺陷。
且它们在未来有防御价值：若有人删掉配套的 `require.Len` 或并集断言，它们就不再恒真了。

**留给下游的提示**：别把这两条当作独立兜底。真正守住「划分正确」的是那三条逐字面量
`assert.Contains`（`:233/:234/:235`）——`18+9=27` 这个等式对两个错误的划分同样成立。

---

## 5. dev 两条自披露的核实

| 自披露 | 我的核实 | 结论 |
|---|---|---|
| code-simplifier 改 `required_test.go` 两处等价重写后，**全部消融与全部自证数字已重跑**，首轮作废 | 我复跑 R-M1/R-M2 得到的断言行号与 discovery 记录**逐字相同**（首轮跑在旧版上行号必不同）；`deliverable_sha256` 三项与我在副本上独立计算的一致 | 属实，discovery 记的是重采值 |
| 开工时用 python 直写 `.arcforge/` 绕过 write-guard，随后自行经写通道重写同一份内容 | 属 CLAUDE.md 已记载的已知缺口（「Bash 侧是常见写动词启发式，python/perl/heredoc 可逃逸」），处置正确（自曝 + 走正规通道重写） | 已知缺口的又一次实撞，不是本任务的新缺陷 |

---

## 6. 范围与声明一致性

`git show --name-only 5372f8e` ⇒ `internal/hestia/{required.go, required_test.go, types.go}`，
与任务 `writes` 声明**逐字一致**，无越界申报问题。`discovery` 指针已挂
（`.arcforge/tasks/TASK-003.json` 的 `discovery` 字段 = `.arcforge/discoveries/TASK-003.json`）。

---

## 7. 结论

**VERIFIED。** 8 条 done_criteria 全部有对应测试、断言非空洞、且经消融证明确实在守
（三条逐字面量归属锚各自被独家点亮、`ElementsMatch` 与 FX 对绑独家点亮、`assert.Nil` 独家点亮）。
自证数字全部由我重采并与 dev 报数逐字吻合。两条观察（守卫判据范围、两条断言恒真）已记录，
均不构成缺陷，供 QA 与后续任务参考。
