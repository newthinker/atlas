# TASK-003 验证报告 — 标题正则、期次映射与条目提取

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = 7576ad328ef527322bdb5830299e81a8e84654c4`（== 当前 HEAD）
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-3 7576ad328ef527322bdb5830299e81a8e84654c4`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移核验 —— **代码与 discovery 双零漂移**

```
$ git rev-parse HEAD                                   7576ad328ef527322bdb5830299e81a8e84654c4
$ jq -r '.verify_baseline.head' TASK-003.json          7576ad328ef527322bdb5830299e81a8e84654c4
$ shasum -a 256 .arcforge/discoveries/TASK-003.json    8c116b3d2c59b6daa00003ba4c502827a19d120ab2a9639f4f710ed0d602abc3
$ jq -r '.verify_baseline.discovery_sha256'            8c116b3d2c59b6daa00003ba4c502827a19d120ab2a9639f4f710ed0d602abc3
```
两者均逐字相同 ⇒ **本次未使用任何 `--ack-*`**。dev 吸取 TASK-002 教训（更新 `commit` 字段前后各查一次
状态），这次窗口没被撞上。

**discovery 指针核对**（不只看文件名）：`.task == "TASK-003"`、
`.commit == 7576ad32…` == `verify_baseline.head`、`.by == "dev-agent-50"`、
`.files_modified` 与 `git show --numstat 7576ad3`（`discover.go 127/6`、`discover_test.go 262/2`）
逐项吻合 ⇒ 指针确实指向本任务产物。numstat 与 `writes` **两项一致，无越界**。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据 | 判定 |
|---|---|---|---|---|
| F1 | `parsePeriod` 四种映射；与 `periodEndMonth` 一致 | `TestParsePeriod`（5 例） | M9（annual→`-01`）、M10（h1→`-07`）KILLED | **PASS** |
| F2 | `scanPage` 从真实快照提取，**每个字段**被断言；p1 提取为空 | `TestScanPage` 三子测试 | M1、M12 KILLED（详见 §3.1） | **PASS** |
| B1 | **期次段可选**：`2025年金融统计数据报告`→`(2025-12, annual)` | `TestParsePeriod` | M3（改回必填）KILLED | **PASS** |
| B2 | `13月` 必须 `ok == false`（语义校验） | `TestParsePeriodRejects/13月` | M4 KILLED；M5 另守 `0月` 下界 | **PASS** |
| E1 | 同页干扰项不被收录 + 另拒三条 | `TestParsePeriodRejects` + `TestScanPage/同页的干扰项不被收进来` + `TestScanPageFiltersRatherThanMisses` | M2 KILLED（详见 §3.2） | **PASS** |
| N1 | `resolveURL` 抽出后 `TestPageURL` 必须仍绿 | 实测 PASS | M13/M14（详见 §4） | **PASS** |
| N2 | RED 因**预期原因**失败 | 独立复现两次（§5） | — | **PASS** |
| N3 | gofmt/vet 无输出、整包绿、覆盖率 ≥ 92.1%、导出面守卫不打红 | §6 | — | **PASS** |

## 2. N3 的命令与输出

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/   → 无输出，exit 0
$ gofmt -l internal/hestia/                     → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...              → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.816s  coverage: 92.7% of statements
$ go tool cover -func | grep discover.go
parsePaging 100.0% | pageURL 100.0% | resolveURL 100.0% | parsePeriod 100.0% | scanPage 100.0%
$ go test -run 'TestStoreExposesNoWriteMethods|TestPackageExposesNoWriteFunctions|TestPageURL' -v
--- PASS: TestPageURL            --- PASS: TestPageURLRejectsUnparsableInput
--- PASS: TestStoreExposesNoWriteMethods   --- PASS: TestPackageExposesNoWriteFunctions
```
覆盖率 **92.7% ≥ 92.1%**（也 ≥ Leader 给的水位 92.5%），`discover.go` 五个函数各 100.0%。
`Candidate` 是结构体类型，导出面守卫两条均未打红 ⇒ 确认 `writes` 不含 `store_test.go` 是对的。
**N1 的 `TestPageURL` 全程绿**，重构未改变行为。

## 3. 变异/消融独立复验（harness 自写）

`scratchpad/test25-TASK-003-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写、默认**全 sha**；
变异在 `/tmp/mut-036-3` 隔离 worktree。四道闸：基线闸（`--- PASS` = 540 全绿）、生效闸、
**编译失败闸**（0 命中）、**计数自证 13 == 13 → OK**。加补充变异 M14，共 **14 条，14/14 KILLED**。

| 变异 | 结果 | 实测死因（唯一/主要杀手） |
|---|---|---|
| M1 `scanPage` 恒返回 `nil, nil` | KILLED | `第 2 页提取出报告条目`：`Should NOT be empty, but was []` |
| M2 正则去掉末尾「报告」 | KILLED | **只被** `TestParsePeriodRejects/2026年7月金融统计数据情况` 与 `/2026年上半年金融统计数据简报` 杀 |
| M3 期次段改回必填（去掉 `?`） | KILLED | `TestParsePeriod/2025年金融统计数据报告` + `ReturnsEveryReportOnPage` |
| M4 语义校验去掉上界 `n > 12` | KILLED | **只被** `/2026年13月金融统计数据报告` 杀 |
| M5 语义校验去掉下界 `n < 1` | KILLED | **只被** `/2026年0月金融统计数据报告` 杀 |
| M6 `return nil, err` → `return out, err` | KILLED | **只被** `TestScanPageReturnsNoPartialResultOnError` 杀 |
| M7 `tagRE` 不再剥标签 | KILLED | **只被** `TestScanPageStripsInlineTags` 杀 |
| M8 找到第一条就 `break` | KILLED | **只被** `TestScanPageReturnsEveryReportOnPage` 杀 |
| M9 annual 期末月 → `-01` | KILLED | `TestParsePeriod/2025年…` |
| M10 h1 期末月 → `-07` | KILLED | `TestParsePeriod/2026年上半年…` |
| M11 「跳过非报告」改成报错 | KILLED | `第 1 页没有报告条目` 等五处 `Received unexpected error` |
| M12 `ArticleID` 取 `m[1]`（整个 href） | KILLED | `Expect "/goutongjiaoliu/…/index.html" to match "^\d{14,}$"` |
| M13 ref 错误串同时含两个串 | KILLED | **只被** `assert.NotContains` 杀（§4） |
| **M14【补】** 把 base 那句改成 `bad url (base)` | KILLED | **base 侧 `Contains` 杀的，不是 NotContains**（§4） |

### 3.1 F2 —— 我上一轮提的「平凡为真」已被落实，且我做了正向验证

我在 TASK-003 开工前提醒过：「`scanPage` 每字段被断言 + p1 提取为空」这个组合，
**一个恒返回 `nil` 的实现能让整条 DoD 全绿**。dev 的实现里 `require.NotEmpty` 在场，
且注释明写了互补关系。我做了**消融的消融**验证这条风险**真实存在**、并确认是 `NotEmpty` 排除的：

```
M1（scanPage 恒返回 nil）下：
$ go test -run 'TestScanPage/第_1_页没有报告条目' -v   → --- PASS: TestScanPage     ← 平凡通过
$ go test -run 'TestScanPage/第_2_页提取出报告条目' -v → Error: Should NOT be empty  ← 被 NotEmpty 拦下
```
⇒ 风险确实存在（p1 那条单独看在空集上平凡为真），`require.NotEmpty` 是**唯一**排除它的断言。
dev 还额外补了 `TestScanPageFiltersRatherThanMisses`，把「干扰项被拒」从
「结果里没有它」升级为「提取器**确实看见了**它 → `parsePeriod` 拒了它」的完整因果链
——这一条我没要求，是它自己补的，属于同一族缺口的另一处。

### 3.2 E1 —— dev 挖出的三个「守卫在场而无效」，我逐条复验成立

**① 「报告」二字原本无人守，且 DoD 的归因是错的。** M2（删掉末尾「报告」）的失败测试集合
**只有** dev 新补的那两条（`2026年7月金融统计数据情况`、`2026年上半年金融统计数据简报`）
⇒ 这两条确实承重；反过来说，**没有它们 M2 就存活**（「只被 X 杀」逻辑上等价于「删掉 X 则存活」，
无需再跑一遍）。DoD `error_handling[0]` 写的「实现靠『金融统计数据报告』六个字必须紧跟期次段
挡住它」把两个独立机制混成一句，实测为错——挡干扰项的是**「紧跟」结构**，「报告」挡的是另一类。
dev 已在 `discover.go` 与 `discover_test.go` 的注释里纠正并标注「别记混」。

**② 「报错时不得返回部分结果」原本写了却没守住。** M6 的失败测试集合**只有**
`TestScanPageReturnsNoPartialResultOnError`（死因：`Expected nil, but got: []hestia.Candidate{…1 条…}`），
**`TestScanPageFailsOnUnresolvableURL` 在 M6 下是绿的** —— 印证 dev 的诊断：
那个场景第一条就失败，`out` 本来就是 nil，两种写法返回值完全相同。
这正是「只看 KILLED 不看因果就会以为旧断言一直有效」的实例。

**③ `tagRE` 在真实快照上是空操作。** M7 的失败测试集合**只有**
`TestScanPageStripsInlineTags`（合成页面）⇒ 没有那条，删掉 `tagRE` 全部基于快照的测试照样绿。

**④ 同源的第四条：`break` 版能全绿。** M8 的失败测试集合**只有**
`TestScanPageReturnsEveryReportOnPage`（合成页面）⇒ p2 恰好只有 1 条报告时，
「遍历每一条」的字段断言只有一条可遍历，形同虚设。这与我提的平凡为真是同一族。

## 4. 「净增强而非放松」的评价 —— 结论成立，**但注释举的例子是错的**

Leader 要我独立判断 dev 给的第三条依据：

> 原先「`bad url` 不是 `bad base url` 的子串」只是**串的偶然形状**在替断言把关，
> 现在「两支必须可分」是**被断言的性质**。

**结论成立**：M13（让 ref 错误串同时含 `bad url` 与 `bad base url`）**只被
`assert.NotContains(err, "bad base url", "ref 出错不该报成 base 出错")` 杀死**，
其余断言全绿 ⇒ 这条新增断言承重，守的是别处守不到的性质。

**但 `discover_test.go` 里那条注释举的例子是错的**，我实测证否：

> 注释原文：「谁把 base 那句改成『bad url (base)』，**两条就会同时绿**而无人察觉。」

M14 实测该变异：
```
--- FAIL: TestPageURLRejectsUnparsableInput/index_url_不可解析
    Error: "hestia discover: bad url (base) \"://nope\"…" does not contain "bad base url"
```
⇒ 那个变异让 **base 子测试变红**，不是「两条同时绿」。原因是 `bad url (base)` 并不包含
子串 `bad base url`，base 侧那条既有的 `Contains` 就把它拦下了。

**净影响**：`NotContains` 守的类别比注释声称的**更窄** ——它守的是「ref 的错误串**同时**含
两个标记」这一类（M13），而注释举的那个例子已被既有断言覆盖。
**这不改变「净增强」的判定**（M13 证明它确实杀掉了别人杀不掉的东西），但注释的**理由**需要更正，
否则后人会照着一个错的例子去理解这条断言在守什么。

⇒ **这是本次交付里唯一的实质缺陷，属注释层面**（`结论对、理由错` 的又一例），
建议并进 T7 顺手改，不值得为它返工。

### 删除行核实（本 Sprint 第一个有删除的交付）

`discover.go -6`：全部是 `pageURL` 里被 `resolveURL` 取代的实现行（DoD `non_functional[0]` 要求的抽取），
逐行看过 diff，无功能删除。
`discover_test.go -2`：两条因错误串改变而失效的 `Contains`（`bad index url`→`bad base url`、
`bad paging template`→`bad url`），两条 `require.NotNil(errors.Unwrap(err))` **一字未动**，
且新增 `assert.NotContains`。⇒ **净增强，非放松**，与 Leader 的判定一致。

## 5. N2 —— RED 独立复现两次

```
① 把 discover.go 回退到 TASK-002 版（0597fca），测试保持交付版：
internal/hestia/discover_test.go:203:17: undefined: parsePeriod   （及 scanPage 数处）

② 只补到 parsePeriod 一族、仍缺 scanPage：
internal/hestia/discover_test.go:258:15: undefined: scanPage      （共 5 处）
```
两次均**因预期原因失败**（`undefined: parsePeriod` / `undefined: scanPage`），
**未被 `imported and not used` 污染**，与 dev 记录的 `red_output_verbatim_1/_2` 同形
（行号差异是因为 dev 的 RED 采于测试文件尚未写全的中间态）。

## 6. 「不可达分支」的处置 —— 我独立复验，认可 dev 的判断

dev 在 `parsePeriod` 里保留 `err != nil` 分支但**不为它编用例**，理由是
「编一个『看起来在测它』的只会制造假守卫」。我写了一个探针独立复验其不可达性：

```
2026年１２月金融统计数据报告（全角）        → 正则不匹配，到不了 Atoi
2026年٣月金融统计数据报告（阿拉伯-印度数字） → 正则不匹配
2026年１月金融统计数据报告                 → 正则不匹配
2026年123月金融统计数据报告（3 位，超 {1,2}）→ 正则不匹配
```
四种输入无一到达 `Atoi` ⇒ **不可达声称成立**，理由（Go 的 `\d` 只匹配 ASCII 0-9 + `{1,2}` 上限）也正确。

**我认可它不编用例的决定**：本项目反复验证的判据是「改坏它有东西会红吗」——
对一个不可达分支，任何用例都不可能真的守着它，编出来的只会是**看起来在场的假守卫**，
比留空更糟（它会让后人以为这里有覆盖）。dev 的处置是：注释写明不可达、写明
「一旦有人放宽长度上限就立刻可达」、并与 `parsePaging` 里**同形状但可达**的分支做对照
（差别只在 `\d+` 有没有长度上限）。这个对照本身就是最好的守卫说明。

## 6.5 🔴 判定后发现的次缺口 —— 同一形态的第二处，已实测确认

**发现时序**：本报告初版发出、TASK-003 已转 `verified` 之后，dev-agent-50 与 Leader 各自指出
`TestScanPage/同页的干扰项不被收进来` 存在同一形态的第二处平凡为真。**我独立复验，成立。**

```go
t.Run("同页的干扰项不被收进来", func(t *testing.T) {
    got, err := scanPage(readTestdata(t, "pboc-index-p2.html"), base)
    require.NoError(t, err)
    for _, c := range got {                      // ← 无前置锚点
        assert.NotContains(t, c.Title, "国新办", "…")
    }
})
```

实测（M1：`scanPage` 恒返回 `nil, nil`）：
```
$ go test -run 'TestScanPage/同页的干扰项不被收进来' -v   → --- PASS: TestScanPage   ← 假绿
$ go test ./internal/hestia/ -count=1                    → FAIL                     ← 整包被拦住
```

### 对交付物的影响：无

整包运行下由同组 p2 的 `require.NotEmpty` 兜住。**14 条变异无一逃逸**——我另外推演了「`scanPage`
把干扰项也收进来」这类变异：那时 `got` 非空，该子测试的 `NotContains` **会**红，它只在空集上失效。
⇒ 该断言是**冗余而非虚假**：它在它该做功的场景里做功，只是在「什么都没返回」的场景里说不了话，
而那个场景已由 `require.NotEmpty` 覆盖。

### 对验证流程的影响：这才是它的真实危害

按 DoD **逐条单跑**取证时会拿到假绿，并可能作为「E1 已验证」写进覆盖矩阵。

### ⚠️ 我核查了自己的取证是否被污染 —— **没有**

这是本节最要紧的一句，因为受害者本该是我。核查方法是回看本次全部实际执行的命令：

| 我跑过的 | 是否受影响 |
|---|---|
| 14 条变异 **全部对整包运行**（`go test ./internal/hestia/ -count=1 -v`） | 否——整包下该子测试非平凡 |
| 唯二的隔离运行：`-run 'TestScanPage/第_1_页没有报告条目'`、`-run '…第_2_页提取出报告条目'` | 否——那是我**故意**用来演示 p1 平凡为真的对照，且结论方向正确 |
| 其余 `-run` 只用于导出面守卫与 `TestPageURL` | 否 |

**我从未单跑过那条干扰项子测试。** §1 覆盖矩阵里 E1 的判定依据是 M2 KILLED（`TestParsePeriodRejects`
两条新用例）+ M11 KILLED（`第 1 页没有报告条目` 等五处），**都不经过那条子测试**，故判定不受影响。

### 一处需要更正我在本报告初版里的措辞

初版 §3.1 我写 `TestScanPageFiltersRatherThanMisses`「把干扰项被拒升级为完整因果链」。
**这句话对，但读者可能误以为它也守着 `scanPage` 的过滤行为——它不守。** 实测：该测试在 M1 下
**照样 PASS**，因为它根本不调用 `scanPage`，只直接检验 `articleLinkRE` 与 `parsePeriod` 两个组件。
它守的是「提取器看得见干扰项 ∧ `parsePeriod` 拒绝它」这条**组件级**因果链，
`scanPage` 把两者组合起来的那一步由 M11 与 p1 的 `assert.Empty` 守着。

### 处置：**判定维持 VERIFIED**，修法并进 TASK-005

- **为什么不改判**：我已于 `verified` 落盘，而 `verified` 状态下 `test-*` **没有任何出边**
  （合法出边只有 leader 的 `verified → review_fix` 与 `verified → accepted`）⇒ 我已无权改判。
  即便有权，实质理由也支持维持：交付物无缺口、无变异逃逸、DoD 判据满足。
- **为什么不建议 `review_fix`**：**TASK-005 的 `writes` 本就含 `discover_test.go`**
  （`discover.go` / `discover_test.go` / `store_test.go`），一行 `require.NotEmpty` 顺手补进去
  **零返工成本**，比走一轮 `verified → review_fix → in_progress → dev_done → verifying` 便宜得多。
- 修法与去向已一并写进 `TASK-003` 的 `done_criteria.functional[1]`（我是 `verified` 的 owner，
  已落盘并 `jq` 核实；四组 DoD 条数 2/2/1/3 未变，只在 `functional[1]` 尾部追加）。

## 7. 观察项

1. **DoD `functional[1]` 的文本并未被更新。** Leader 在派单里说 dev 已把我那条平凡为真的要求
   补进 `done_criteria.functional[1]`，请我核实「真的按那条做了」。核实结果分两半：
   - **任务文件里没有**：全文检索 `NotEmpty`/`require.Len`/`空集`/`恒返回` 零命中，
     `functional[1]` 逐字仍是原文；唯一的「平凡为真」出现在 `error_handling[0]`，是 reviewer 关于
     干扰项的既有说明，与此无关。
   - **但测试真的实现了它**（§3.1 已证，`require.NotEmpty` 在场且是唯一承重者）。
   ⇒ **是 DoD 文本没跟上，不是东西没做**。判定按实现走，PASS。记此项是因为
   **归档后读 DoD 的人会看不到这条要求**，而它恰是 T5 复用 `scanPage` 时最该继承的一条。

2. **DoD `error_handling[0]` 的归因错误**（Leader 已自陈，M2 实测确认）见 §3.2①。
   DoD 的**判据**（干扰项必须被拒 + 另拒三条）可测且已满足，错的只是括号里的机制解释
   ⇒ 不构成 `dod_defect`，但文本值得更正。

3. **注释举的例子是错的**（§4），建议并进 T7。

4. 上述 1–3 有个共同形状：**都是「结论对、理由错」**。三处的结论全部正确、行为全部正确，
   错的都是**写下来的那个理由**——而理由是后人复现时的唯一入口。dev 这次自己抓到了两处
   （「报告」二字的归因、`tagRE` 的注释），第三处（`NotContains` 的例子）是它新写的注释里
   引入的。⇒ 值得记的是：**纠正一个错理由的同时，很容易写下一个新的错理由。**

## 8. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条有消融证据；14 条变异全部 KILLED，
其中 6 条精确定位到**唯一杀手**；我上一轮提的平凡为真风险已被落实，并由消融的消融正向验证；
dev 自挖的三个「守卫在场而无效」逐条复验成立；RED 两次独立复现；不可达分支的处置我独立复验并认可；
代码与 discovery 双零漂移；主工作区零污染。唯一实质缺陷是一条注释的举例错误（§4），建议并进 T7。
