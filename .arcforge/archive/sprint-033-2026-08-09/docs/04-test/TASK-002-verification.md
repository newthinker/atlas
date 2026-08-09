# TASK-002 验证报告 —— 类型与 Meta 校验 (types.go)

- **验证者**：test-agent-21
- **判定**：**PASS（verified）**
- **被验 commit**：`a087b989777f028f08b69b3fc690e83426262271`（全 sha；与 `verify_baseline.head` 一致，无漂移）
- **验证环境**：隔离 detached worktree `.worktrees/wt-verify-TASK-002` @ `a087b98`
- **验证时间**：2026-08-08（v2：补入「单侧误写 vs 完整互换」三组对照实测）

## 自测结果（本验证者独立运行，非复用 dev 数字）

| 项 | 结果 |
|---|---|
| `GOTOOLCHAIN=local go vet ./internal/hestia/` | exit 0 |
| `GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -v` | **PASS=23 / FAIL=0 / SKIP=0** |
| 覆盖率（`-coverprofile` + `cover -func`） | 包语句覆盖 **100.0%**；`types.go:63 validate` **100.0%**（门禁 80） |
| 全仓 `go build ./...` | exit 0 |
| `./internal/macro/bitemporal/` 回归 | ok |

基线与收尾**背对背重跑**一致（均 23/0/0），收尾 `git status --porcelain` 为空，
两个文件 sha256 与变异前逐字节相同（`types.go=0184263c…55bd`、`types_test.go=64d8d8b8…b407`）。

## 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | Meta 七字段声明顺序为跨任务契约；结构体上方注释写明约束并指出另两处位置 | `TestMetaFieldOrderIsCrossTaskContract`（reflect 钉顺序 + 字段数） | 变异 M7 对调 Extractor/CaliberVersion → KILLED；`types.go:16-25` 注释点名 `schema.go 的 metaColumns` 与 `store.go 的 insert` | **PASS** |
| functional[1] | `Meta.validate()` 接受合法值；Observation / Check / ValidationReport / Outcome 按需求文档定义 | `TestMetaValidateAcceptsValid`（三种 period_type）、`TestObservationValuesRepresentAbsence`、`TestValidationReportAndOutcomeShape` | 变异 M15 将 `Passed` 改名 → `go vet` 报 `unknown field Passed`，字段存在性被编译钉住 | **PASS** |
| functional[2] | `CheckStatus` 及 CheckPassed/CheckFailed/CheckSkipped 完整 | `TestCheckStatusConstants` | 变异 M13 改 `CheckPassed` 取值 → KILLED | **PASS** |
| boundary[0] | validate 忽略 IngestedAt；判据是「零值与任意值结果相同」 | `TestMetaValidateIgnoresIngestedAt`（4 种取值 × 合法/非法两路，断言错误串完全一致） | 变异 M9 把 IngestedAt 加进必填 → KILLED | **PASS** |
| error_handling[0] | 六个必填项**逐个单独取空**各报错、指名字段；**断言须能区分 period 与 period_type** | `TestMetaValidateRejectsEmptyRequired`（6 个子测试，`assert.EqualError` 比对完整串） | 见「关键判据 ②」——M3/M4/M5 全部 KILLED，且三组对照实测证明断言强度 load-bearing | **PASS** |
| error_handling[1] | period_type 非法值报错；合法集合在代码中显式列出 | `TestMetaValidateRejectsBadPeriodType`（6 个非法值含大小写/空格变体） | `validPeriodTypes` 显式列于 `types.go:38`；变异 M6（枚举恒真）、M14（混入 quarterly）均 KILLED | **PASS** |
| error_handling[2] | G1 形态校验：published_at 匹配 `YYYY-MM-DD`、period 匹配 `YYYY-MM`；用例须含指定五类输入 | `TestMetaValidateRejectsMalformedPublishedAt`（9 例）、`TestMetaValidateRejectsMalformedPeriod`（8 例） | 见「关键判据 ①」——独立探针 + M1/M2/M10/M11/M12 五个变异全 KILLED | **PASS** |
| non_functional[0] | `validate` 小写不导出 | 全文件调用 `m.validate()`，导出即整包编译失败 | `grep` 确认 `func (m Meta) validate() error`，包内无导出的 `Validate` | **PASS** |

## 关键判据 ① —— published_at / period 形态校验（error_handling[2]）

先核对契约来源原文：`internal/macro/bitemporal/lookup.go` 的包注释确实写着
「喂进 `2026-7-15`（少补零）或纯数字版本号（`9` > `10`）会静默判错、零告警」，
且「明确不在这里加形态校验……由建表方与写入方保证」。Store 就是那个写入方，契约成立。

**不只看正则写对没有**——我写了一份独立探针测试（不依赖 dev 的测试文件），
自己构造 DoD 点名的输入直接调 `validate`，确认每一条**实际被拒**且错误信息可读：

```
published_at="2026-7-15"            已拒: hestia: meta.published_at "2026-7-15" must match YYYY-MM-DD
published_at="2026/07/15"           已拒: …
published_at="2026-07-15T00:00:00Z" 已拒: …
published_at=" 2026-07-15"          已拒: …   (前导空格)
published_at="2026-07-15 "          已拒: …   (尾随空格)
period="2026-6"                     已拒: hestia: meta.period "2026-6" must match YYYY-MM
period="2026/06" / " 2026-06" / "2026-06 "  已拒
```

探针跑完即删除，工作区已还原（`git status --porcelain` 为空）。

配套变异证明这些检查是**活的**、且被断言守住（非「正则写对但没被调用」或「调用了但错误被吞」）：

| 变异 | 手法 | 结果 |
|---|---|---|
| M1 | 删除 published_at 形态校验块 | **KILLED**（PASS 23→22）by `TestMetaValidateRejectsMalformedPublishedAt` |
| M2 | 删除 period 形态校验块 | **KILLED**（23→22）by `TestMetaValidateRejectsMalformedPeriod` |
| M10 | `periodRE` 锚点 `\A..\z` 弱化为 `(?m)^..$` | **KILLED**（23→22）by `…MalformedPeriod`（尾随换行用例生效） |
| M11 | `publishedAtRE` 去掉尾锚 `\z`（RFC3339 会被前缀匹配放行） | **KILLED**（23→22）by `…MalformedPublishedAt` |
| M12 | `periodRE` 放宽为 `[0-9]{1,2}`（`2026-6` 会被放行） | **KILLED**（23→22）by `…MalformedPeriod` |

M10/M11/M12 尤其有价值：它们是**语义弱化**而非整块删除，正是真实劣化最可能的形态。

## 关键判据 ② —— period / period_type 子串陷阱（error_handling[0]）

### 复现方向：必须用「单侧误写」，不是「完整互换」

**这一条请照抄方向，别自己改。** 派单最初给的复现手法是「把 `period` 与 `period_type` 的校验
**互换**」，dev-agent-42 在交付里指出该方向会误导，Leader 随后更正。我把两个方向**都实测了**
（同一 worktree、同一 commit、背对背，`go vet` 均 exit 0）：

| 组合 | 总计 | `period` 子测 | `period_type` 子测 | 说明 |
|---|---|---|---|---|
| **A) `assert.Contains` + 单侧误写**（只把 `period` 标签写成 `period_type`） | **PASS=23 FAIL=0** | **PASS（假绿）** | PASS | **缺陷 100% 存活**，六子测试全绿 |
| **B) `assert.Contains` + 完整互换** | PASS=21 FAIL=2 | **PASS（假绿）** | **FAIL** | **只被半数拦住** |
| **C) `assert.EqualError`（本任务实际写法）+ 单侧误写** | PASS=21 FAIL=2 | **FAIL** | PASS | **被 `period` 子测精确杀死** |

**为什么 B 会误导**：完整互换在弱断言下仍有一个子测试红。验证者若按 B 复现，会看到「Contains
也拦住了」，进而推出「弱断言其实没问题、DoD 这条多余」——**结论错，而实验本身没做错**。
真正暴露弱断言危害的是 A：错误串 `"hestia: meta.period_type must not be empty"` **包含**子串
`"period"`，于是 `Contains(err, "period")` 恒真，`period` 子测试永远绿。

⇒ **A 与 C 的对比才是判据②的证明**：同一个实现缺陷（单侧误写），
`Contains` 下**完全存活**、`EqualError` 下**被杀死**。
DoD「断言须能区分二者」不是形式上满足，而是**经实验确认起了作用**。

### 正向变异（实现变异，测试应红；断言为本任务实际的 EqualError）

| 变异 | 手法 | 结果 |
|---|---|---|
| M3 | `period` 必填标签**单侧**误写成 `period_type` | **KILLED**（23→21），由 `TestMetaValidateRejectsEmptyRequired/period` 杀死 |
| M4 | 两者标签**完整互换** | **KILLED**（23→20），`period` 与 `period_type` 两子测试同时红 |
| M5 | 两者**取值**互换、标签不动 | **KILLED**（23→20） |

判定落到「哪一条红了」，不是「红了几条」：M3 红的正是 `period` 子测试，即弱断言的盲区所在。

## 变异测试汇总

**15 个变异，14 个有效变异全部 KILLED，0 存活；1 个（M10 首次尝试）被有效性闸判为无效并重做。**
另有 3 组断言强度对照实验（A/B/C），不计入变异分。

有效性闸按纪律三条自证：变异 diff 非空 + `go vet` exit 0 + `--- PASS` 计数与基线 23 比对。
M10 首次尝试因 perl 转义把源文件写坏，`go vet` 报 `illegal character U+005C` → harness
**判为无效变异、不记 KILLED**（编译失败会让套件全红，正是最容易伪造 KILLED 的形态），
换转义方式重做后才取得有效结果。每个变异窗口结束均 `git checkout` 还原并核验 sha256 复原。

「三条自证」在变异部分只用于**排除假 KILLED**；本报告唯一的「0 红」结论出现在对照实验 A，
其 PASS 计数 23 与基线**严格相等**且 `go vet` exit 0，符合「报存活须三条自证」的要求。

## 声明范围一致性

`git show --numstat a087b98` = `internal/hestia/types.go`(+133/-0)、`internal/hestia/types_test.go`(+216/-0)，
与 `writes` 声明的两个路径**逐一相符**，无越界改动、无未申报文件。

## 跨任务契约落盘确认

- ✅ **结构体注释**：`types.go:16-25` 有「# 字段声明顺序是跨任务契约」小节，明确点名另两处位置
  （`schema.go` 的 `metaColumns`、`store.go` 的 `insert`），并说明「列都是 TEXT，错位写入不触发任何
  数据库错误」这一静默后果。
- ✅ **discovery**：`key_findings[0]` 完整记录七字段顺序与对应列名，`notes_for_downstream` 分别给出
  TASK-003 与 TASK-005 的落地要求。
- ✅ **机制保护**：`TestMetaFieldOrderIsCrossTaskContract` 用 reflect 钉住顺序与字段数（M7 已证其有效）。
  **但它只保护 types.go 这一端**——无法发现 `metaColumns` 或 `insert` 单方面写错序。

## 发现的问题

**无阻断性问题。** 以下为观察项，不影响判定：

1. **（观察，非缺陷）`types_test.go:200` 的 `Passed: false` 从未被断言**——即 IDE 诊断里的
   `unused write to field Passed`。核实过：该字段的**存在性与类型**由编译钉住（M15 改名即
   `go vet` 失败），而本任务中 `ValidationReport` 是纯数据结构、无任何逻辑读取 `Passed`，
   因此「无行为断言」是恰当的覆盖层级，不构成漏断言。真正需要对 `Passed` 取值做断言的是
   **闸门实现（M1b-3）**那个任务。
2. **（已声明的残留缺口）只校形态、不校日历有效性。** 实测确认：`period="2026-13"` / `"2026-00"` /
   `"9999-99"`、`published_at="2026-02-31"` **全部通过 validate**。这与 dev 在 `decisions` 里的声明
   完全一致，且已写进 `types.go:53-54` 的注释——是**明示的已知缺口，不是隐瞒**。形态校验对
   「字典序失效」这一 G1 目标而言是充分的，故不影响本任务判定。

## 给 Leader 的建议（不阻断本任务）

1. **日历有效性缺口需要一个明确的归属任务。** dev 建议归给闸门层（M1b-3），我认同，
   但请确认 M1b-3 的 DoD 里**确实有这一条**——否则它会从两边同时掉出去：
   types.go 说「留给闸门层」，闸门层没写，就没人做了。
2. **TASK-003 / TASK-005 的 DoD 应各补一条同序断言。** 本任务的 reflect 测试只守住 types.go 一端。
   建议 TASK-003 断言 `metaColumns` 与 reflect 取到的 Meta 字段名（转 snake_case）**逐一相等**。
3. **本报告「关键判据 ②」的三组对照数据请保留进 wisdom。** 它记录的是一个可复现的认知陷阱：
   *一个正确的结论配了错误的复现理由，而错误理由的实验会「看起来成功」*。
   下次遇到子串关系的字段名（如 `id` / `id_type`、`name` / `name_zh`）会原样重现。

## 对 DoD 本身的意见

**DoD 质量很高，没有发现错误。** 两点值得记下：

- `error_handling[0]` 主动点出「`period` 是 `period_type` 的子串，需求文档示例的 `assert.Contains`
  在实现搞混时照样通过」——这一条**经三组对照实验证实为真**（见 A/C 对比）。
  DoD 不只写「要测什么」还写「**不能怎么测**」，直接拦掉了照抄需求文档就会踩的坑。
- `boundary[0]` 把判据写成「零值与任意值**结果相同**」而不是「都不报错」，同样堵死了一个弱断言。

两点措辞级建议（非缺陷）：

- `error_handling[2]` 列举的必测输入里没有**尾随换行**（`"2026-07-15\n"`）。dev 主动加了这两条用例，
  而它们恰好杀死了 M10（锚点由 `\A..\z` 弱化为 `(?m)^..$`）——真实且隐蔽的劣化路径。
- `error_handling[0]` 若要给出复现手法，应写明「**单侧误写**」而非「互换」，理由见判据②的 B 行。

## 结论

八条 done_criteria **逐条有对应测试、逐条 PASS**，无未覆盖项；
23/23 通过、覆盖率 100%、14 个有效变异全部被杀、声明范围与实际改动一致、无基线漂移。
判定 **verified**。
