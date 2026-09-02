# TASK-001 验证报告 —— stripHTML 去行首空白

- **验证者**：test-m1c4-a
- **判定对象**：`master @ 8d02371ca2bc9513dac1a625bf3a8c73d70a68c2`（= `verify_baseline.head`）
- **dev commit**：`4c9ca20848139abcffb0638071227ab460575231`（`8d02371` 的 parent）
- **验证 worktree**：`../wt-verify-TASK-001`（detached @ `8d02371ca2bc…`）
- **assignment_epoch**：1
- **结论：VERIFIED**

---

## 0. 基线核对（判定前）

| 项 | verify_baseline 记录 | 判定时实测 | 一致 |
|---|---|---|---|
| `head` | `8d02371ca2bc9513dac1a625bf3a8c73d70a68c2` | `git rev-parse HEAD` 同值 | ✅ |
| `discovery_sha256` | `c19db0f2218674ac…` | `shasum -a 256` 同值 | ✅ |

**⚠️ 一个必须写明的不同源问题（观察项，非缺陷）**：dev 的全部自证数字采于 `4c9ca20`，
而判定对象是 `8d02371`。两者之间隔着**已合入的 TASK-004**，它改了 `internal/hestia` 的
11 个文件（`fields.go` +72/-1、`profiles.go` +49/-21、`required.go`、`extract.go` …，
fieldOrder 54→76）。⇒ **dev 报的覆盖率、套件结果、语料 diff 都测自一棵不含 TASK-004 的树**。

处置：**本报告的每一个数字均由我在 `8d02371` 上独立重采**，不采信 discovery 里的数值。
重采后逐条结论与 dev 一致（唯一可见差别：语料输出由 dev 报的 186 行变为 208 行，
系 TASK-004 扩容所致，与本任务无关）。

---

## 1. done_criteria 覆盖矩阵（8 条，逐条对照）

| # | 维度 | 完成标准（摘要） | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `lineLeadSpaceRE = (?m)^ +`，插在 `spaceRE` **之后**、`blankLineRE` **之前**；注释写下顺序理由 | `strip.go:32` 定义、`:80` 调用，位于 `:61` 的 `spaceRE` 与 `:81` 的 `blankLineRE` 之间；**换序变异实测**杀掉 2 条断言（§3.2） | **PASS** |
| F1 | functional | `TestStripHTMLRemovesLeadingWhitespace` 存在且通过，四条断言齐全 | `strip_test.go:82-102`；实跑 `--- PASS`；四条断言逐条核对（§2） | **PASS** |
| B0 | boundary | **不得**放宽 `(?m)^` 锚定，只修喂给锚定的文本 | `sections.go:41` 的 `sectionTitleRE = (?m)^[一二三四五六七八九十]+、.*$` **原样未动**；`git show --numstat 4c9ca20` 证明 `sections.go` / `extract.go` 一行未碰 | **PASS** |
| B1 | boundary | NBSP 已在 `spaceRE` 字符类里，新正则只需处理普通空格（须读实际定义确认） | `strip.go:30` 实测为 `` `[ \t\x{00a0}]+` `` —— NBSP 与 tab 均在内 | **PASS** |
| E0 | error_handling | 🔴 **全语料背对背比对**（`verify_by: manual`），差异只出现在 `2023-05` | **我自己复现**：diff 仅 2 hunk / 4 行；变更行期次机器提取去重 = `{2023-05}`；剔除 2023-05 后两份输出**逐字节一致**（§4） | **PASS** |
| E1 | error_handling | 语料须用主仓库绝对路径 + `--allow-incomplete` | 照此执行，两个二进制退出码均 0、各 208 行（§4） | **PASS** |
| N0 | non_functional | gofmt / vet / 全绿 / 覆盖率 ≥96.1% / 无新增依赖 / 注释带 `M1c-4 的 TASK-001` 前缀 | 逐项实测，见 §5 | **PASS** |
| N1 | non_functional | 交付流程：worktree、显式 pathspec、**merge 先于 `dev_done`** | `4c9ca20` 是 `8d02371` 的 parent，且 `merge-base --is-ancestor` 报 ancestor；commit 信息锚定 `fix(TASK-001):` | **PASS** |

---

## 2. F1 四条断言逐条核对

DoD 要求的四条，`strip_test.go:82-102` 全部到位（且多一条 `\n三、制表`）：

| DoD 要求 | 实际断言 |
|---|---|
| `\n一、开头` 含于结果 | `assert.Contains(got, "\n一、开头")` ✅ |
| `\n二、次节` 含于结果 | `assert.Contains(got, "\n二、次节")` ✅ |
| `正文 内部 空格`（行**内**折叠不得被波及） | `assert.Contains(got, "正文 内部 空格")` ✅ |
| 结果不含 `\n `（任何一行不得以空格开头） | `assert.NotContains(got, "\n ")` ✅ |

断言非空洞：全部为对 `stripHTML` 实际返回值的内容断言，无 `assert true` 之类。

---

## 3. 变异实测（我自己跑，不采信 discovery 的结论）

harness 作用在**隔离的验证 worktree** 上，主工作区 `internal/hestia/strip.go` 的 sha256
在每个变异窗口后 + 收尾均校验为 `d133aefbd9693954700e53fbbaa149f7b8f41599a57ac161201239acc96a1e3b`（未变）。

**⚠️ 有效性闸救回一次假 KILLED**：首轮 harness 用绝对路径作 `go test` 的包参数，
实际报 `directory … outside main module`（`setup failed`），退出码同样是 1。
若不查输出就会把两个变异都误记成 KILLED。修正后加了 `setup failed|build failed|outside main module`
闸并补跑**对照组**，下列结果均为闸通过后的真结果。

### 3.0 对照组（未变异，master 树原样）
`PASS=10  FAIL=0`

### 3.1 变异 B：ablation —— 删掉 `s = lineLeadSpaceRE.ReplaceAllString(s, "")`

```
--- FAIL: TestStripHTMLRemovesLeadingWhitespace
--- FAIL: TestStripRealSampleTitlesSurviveLeadingWhitespace
    --- FAIL: …/&nbsp;
    --- FAIL: …/全角空格
--- PASS: TestDetectExtractorRejectsNonConsecutiveOrdinals   ← 5 个子例全绿
```

⇒ 两条新测试**非重言式**（去掉实现即红）。
⇒ **Leader 点名要复核的 ablation 成立**：`sections_test.go` 的 QA 反例改用「·」之后，
在没有 `lineLeadSpaceRE` 的树上**仍然绿** —— 它确与空白机制独立，dev 的结论属实。

### 3.2 变异 A：两步换序（`lineLeadSpaceRE` 提到 `spaceRE` 之前）

```
--- FAIL: TestStripHTMLRemovesLeadingWhitespace
--- FAIL: TestStripRealSampleTitlesSurviveLeadingWhitespace
    --- FAIL: …/&nbsp;
    --- PASS: …/全角空格          ← 不红
--- PASS: TestDetectExtractorRejectsNonConsecutiveOrdinals
```

⇒ 顺序不变式**确有守卫**；全角空格子例不敏感，与 dev 的解释一致
（`punctNormalizer` 在两条正则之前就把全角空格换成了 ASCII 空格）。

### 3.3 变异 C：消融 —— 换序 **且**把测试输入的 NBSP/tab 退回 ASCII 空格

这是 **Leader 点名要复核的第一处**：NBSP/tab 是不是它能守住顺序的**原因**。

```
--- PASS: TestStripHTMLRemovesLeadingWhitespace   ← 转绿
--- FAIL: TestStripRealSampleTitlesSurviveLeadingWhitespace/&nbsp;
```

⇒ 因果确立：**把 NBSP/tab 拿掉，换序变异就杀不动该测试了**。
⇒ 需求文档给的理由（「删掉第一个空格后剩一个」）**确被证伪**——`(?m)^ +` 的 `+` 贪婪，
纯 ASCII 输入换序后结果逐字相同。dev 订正后的注释（「两条正则字符集不同」）是**正确**的。

**NBSP 与 tab 真的进了测试输入**：`strip_test.go:95` 的字面为
`stripHTML([]byte("<p> 一、开头</p><p> 二、次节</p><p>\t三、制表</p>" …`
（Go 解释型字符串字面量，` ` = U+00A0、`\t` = U+0009）。变异 C 的 diff 已逐字打印核对。

---

## 4. E0/E1：全语料背对背比对（核心判据，我自己复现）

**同等条件的构造**：`after` = master 树原样构建；`base` = **同一棵 master 树，仅回退
`lineLeadSpaceRE` 那一行**后构建。⇒ 两者都含 TASK-004，除该行外逐字相同，
避免了「拿不含 TASK-004 的旧树当 base」会产生的假差异。

```
/tmp/atlas-{base,after}-t001 hestia backfill calibrate \
  --dir /Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14 \
  --allow-incomplete
```

- **确定性自检**：`base` 连跑两次输出 `cmp -s` **逐字节一致** ⇒ 输出确定，diff 是真差异。
- **两份输出各 208 行**，退出码均 0。

### 机器化判据（不靠肉眼看 diff）

| 判据 | 结果 |
|---|---|
| 变更行里出现的期次（`grep -oE '[0-9]{4}-[0-9]{2}' \| sort -u`） | **只有 `2023-05`** ✅ |
| `section ordinals are not consecutive` 出现次数 | base=**2** → after=**0** ✅（2 = 同一篇在汇总行与明细行各一次，与 Leader 核实一致） |
| 全部分类计数行（`- N × [标签] 原因：`）逐行 diff | **完全一致**，md5 同为 `c4aee8e717f11a57779f05854ff02b7d` ✅ |
| 「本迭代不解析」篇数 | base=**19** → after=**19**，未变 ✅ |
| **剔除含 `2023-05` 的行后两份输出** | **逐字节一致** ✅ ⇒ 未波及任何其它报告 |

### 对 Leader 三条订正的实测回应

1. **Step 6 判据**：我得到的与 dev 一致 —— **只有第 1 类（该篇自身的行）变化，第 2 类
   （因移动而 ±1 的汇总计数行）一次都没出现**。分类计数行集合完全一致（md5 相同）。
   这比订正后的判据**更严格**，不是不符。
2. **`2023-05` 仍未解析成功**：确认，错误由
   `section ordinals are not consecutive from 一: got 4 sections, section[0] is "二、"`
   变为 `人民币存款期内合计 not found among 2 candidate sentence(s) [5月份/人民币 5月份/外币]`。
   按 DoD 原文「两种都是进展」，**不因它仍在失败清单里判 NEEDS WORK**。板块切分缺陷已消除。
3. **`sections_test.go` 越界申报**：已在 `writes` 里（`.arcforge/tasks/TASK-001.json`
   的 `writes` 含该文件），且 ablation 已独立复核（§3.1）——**结论属实，不是只采信 dev**。

---

## 5. N0 非功能门禁（全部我自己跑于 `8d02371`）

| 项 | 判据 | 实测 |
|---|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 除 `backtest_test.go`/`crisis_test.go` 外**无新增项** | 输出恰为那两个文件 ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出，退出码 0 ✅ |
| `go test ./internal/hestia/... -count=1` | 全绿 | 退出码 **0** ✅ |
| **覆盖率** | ≥ 96.1% | `go tool cover -func` total = **96.1%** ✅（⚠️ 恰等于门槛，**零余量**） |
| 无新增依赖 | `go.mod`/`go.sum` 不在改动里 | `git show --numstat 4c9ca20` 仅 3 个文件，均非 go.mod/go.sum ✅ |
| 注释任务编号 | 带 `M1c-4 的 TASK-001` 前缀 | 三个文件各命中（`strip.go:63`、`strip_test.go:21,114`、`sections_test.go:360`）✅ |
| **全量回归**（集成验证，超出 DoD） | 未破坏其它包 | `go test ./...` **64 包全绿、0 FAIL**、退出码 0 ✅ |

### 声明范围 vs 实际改动（越界申报核对）

`git show --numstat 4c9ca20` 与 dev 报的数字**逐个相同**：

| 文件 | numstat | 在 `writes` 里 |
|---|---|---|
| `internal/hestia/strip.go` | `21 / 0` | ✅ |
| `internal/hestia/strip_test.go` | `62 / 0` | ✅ |
| `internal/hestia/sections_test.go` | `11 / 3` | ✅（越界项，`dev_done` 前已补进声明，Leader 已批准） |

合计 `94 insertions / 3 deletions`，与 `git diff 0cec895..8d02371 --stat` 的
`3 files changed, 94 insertions(+), 3 deletions(-)` 一致 ⇒ **无声明外的文件被改动**。

---

## 6. 测试质量评审

- **非空洞**：两条新测试在 ablation 下全红（§3.1），不是重言式。
- **不过度 mock**：无 mock/stub，`TestStripRealSampleTitlesSurviveLeadingWhitespace`
  直接读真实样本 `pboc-2025-12-annual.html` 做变异，属真语料级守卫。
- **合成 + 真语料双层**：合成用例覆盖行为（含 NBSP/tab 这两个**只有它守得住**的字符），
  真样本用例覆盖「确实救回了 8 个板块」这一后果，分层合理。
- **移走的证据有接管**：`sections_test.go` 的 QA 反例改手法后，「`&nbsp;` 前缀不再让标题
  脱锚」这份真语料证据由新测试 `TestStripRealSampleTitlesSurviveLeadingWhitespace` 接管；
  我已用 ablation 验证接管有效（去掉实现，两个子例全红）。

---

## 7. 观察项（不影响判定，供 Leader 参考）

1. **dev 自证数字与判定对象不同源**（§0）。本次结论不受影响，因为我全部重采了；
   但这是个**流程缺口**：按 AD-4，dev 必须先 merge 再 `dev_done`，而 merge 会把别的任务
   带进树里 —— dev 在 merge 前采的数字，与验证者判定的树天然可能不是同一棵。
   若 TASK-004 恰好把覆盖率压到 96.0%，dev 报的 96.1% 会**掩盖**这个问题而无人察觉。
   建议：把「`dev_done` 前在 merge 后的 master 上重采一次」写进后续任务的 DoD。
2. **覆盖率 96.1% 恰等于门槛，零余量**。后续任务若新增未覆盖语句会立刻破线。
3. 与本任务无关的既有欠账：`cmd/atlas/backtest_test.go`、`crisis_test.go` 的 gofmt
   （来自 `f5d7b82`），按 DoD 未动。

---

## 8. 结论

**8 条 done_criteria 全部 PASS**，证据均由验证者在判定对象树 `8d02371ca2bc…` 上独立产生：
背对背语料比对亲自复现且差异仅限 `2023-05`、三个变异实测（含一道消融）确认新测试非重言式
且顺序不变式确有守卫、Leader 点名的三处复核（NBSP 因果、ablation 独立性、numstat 同源）
全部核实属实、全量回归 64 包全绿。

**判定：VERIFIED**
