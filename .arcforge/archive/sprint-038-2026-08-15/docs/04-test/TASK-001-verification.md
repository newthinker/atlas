# TASK-001 验证报告

- **验证者**：test-agent-27
- **任务**：index 侧：列表项扫描与三种报告标题识别
- **被验树**：`master @ 62b924300415fb109e7355aea0785b4c1ab4903b`（= `verify_baseline.head`，无漂移）
- **discovery sha256**：`547480bf…aadee4`，与 `verify_baseline.discovery_sha256` 逐字节一致（无判定期漂移）
- **验证方式**：独立隔离 worktree `../wt-verify-T1T3`（detached @ 62b9243）+ 8 组消融实验
- **结论**：**VERIFIED（8/8 条 done_criteria 全部通过）**

---

## 一、验证方法学

判据不是「有没有测试」，是「**改坏它会不会有东西变红**」。因此除跑套件外，做了 8 组消融
（A1–A8）与 6 组针对 TASK-003 的对照，每组都记录**哪些测试红**而不只是红不红。

消融 harness（`scratchpad/t27-ablate.sh`）四道闸：
1. **变异生效闸**：替换脚本未改到任何字节 ⇒ 退出 9，不计任何结论
2. **语法闸**：`gofmt -e` 失败 ⇒ 退出 8，不计 KILLED
3. **编译/vet 闸**：`go vet` 失败 ⇒ 退出 8，不计 KILLED（A4 首版即被此闸拦下并重做为 A4b）
4. **主工作区指纹闸**：每个变异窗口前后校验主仓库被变异文件 sha256 未变 + worktree `git diff` 空

全部 8 组的指纹闸与还原核实均通过（`backfill_scan.go` sha256 恒为 `264be20f1f94…`）。

统计一律用 `python3` / `grep -c`，**未使用裸 `sort -u` / `uniq`**（macOS 默认 locale 会把任意两个汉字判等）。

---

## 二、done_criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 我跑出的证据 | 判定 |
|---|---|---|---|---|
| functional[0]a | p146 上识别**三种**报告，标题**逐字**断言（不许只断「命中 3 条」） | `TestScanBackfillPageFindsThreeReportKinds` | 三条独立 `assert.Contains` 逐字写死标题 + `require.Len(Reports,3)` + `assert.Len(Items,15)`。**语料独立核对**：p146 里三种标题确实各一篇（见 §三）。消融 A2 致红。 | ✅ PASS |
| functional[0]b | 标题取 `title=` 属性、**用合成用例**单独钉住 | `TestScanBackfillPageTakesTitleAttributeNotLinkText` | **消融 A1**（把捕获组从 title 属性改到链接文本）：18 条 RUN 中**仅这一条**红，p146 / p18 / p1 三份真实快照的全部断言照样绿 ⇒ 独立复现了 DoD 说的「真实语料上零守卫」，该合成用例是唯一守卫。 | ✅ PASS |
| functional[1] | article_id **不设位数下界**，19 位 + 7 位各一条真实样本 | `TestScanBackfillPageArticleIDHasNoDigitFloor`（2 子用例） | p146 → `2025092212550537670`（19 位）；p18 → `5868082`（7 位）。**两个 id 我都从语料里独立抽出核对过**。 | ✅ PASS |
| functional[2] | 日期取紧随 `</a>` 的 `<span class="hui12">`，产出 `YYYY-MM-DD`；期望值自核 | `TestScanBackfillPageExtractsPublishedDate` | 正则判据确为 `</a>\s*</font>\s*<span class="hui12">`（不是宽泛的 `class="hui12"`）。**我从样本独立核出**：三种报告同日 `2020-03-11`，整页首条 `2020-03-25`、末条 `2020-03-11` —— 与测试断言逐字一致。 | ✅ PASS |
| functional[3] | **13 类**干扰项零命中，与正向断言**成对** | `TestScanBackfillPageRejectsLookalikes` | 干扰项逐条数得 13（A组7 + B组3 + C组1 + D组2）。6 条正例与 13 条干扰项在**同一份合成页面、同一次断言**里（`ElementsMatch` 多一条少一条都红）⇒ 阴性断言不会在空集上平凡为真。**消融 A7**（放宽成 `.*统计数据报告`）与 **A8**（去掉「期次段紧跟」）各自只让本测试红；且**逐条 `NotContains` 亲自报错**（A7 红在小额贷款公司×2、厦门市、吉林省；A8 红在厦门市、吉林省），不是只靠 `ElementsMatch` 兜底。 | ✅ PASS |
| boundary[0] | 0 条列表项 ⇒ **报错**，**只能用合成用例**，与 HTTP 404 分开判 | `TestScanBackfillPageErrorsOnZeroListItems`（2 子用例） | 两个子用例都是**代码里写死的合成 HTML 字面量**（维护页 / 软 404 短响应），未拿真实页面「碰巧没遇到」充数。错误信息含 `layout`。**消融 A3**（守卫失效）：仅本测试红。 | ✅ PASS |
| boundary[1] | 有列表项但 0 条报告 ⇒ 空切片 + nil error；**与 boundary[0] 互补，不得合并** | `TestScanBackfillPageNoReportsIsNotAnError` | 确为**两个独立顶层测试函数**（`backfill_scan_test.go:261` 与 `:280`），未合并。**消融 A5**（把「0 条报告」也判成错误）：本测试两个子用例红，而 `ErrorsOnZeroListItems` **保持绿** ⇒ 两条守卫机制上互补，任一被删都有东西红。 | ✅ PASS |
| boundary[2] | 条目数 == `istitle="true"` 数，不等 ⇒ 报错；基准不得用 `class="hui12"`；不得硬编码「每页恒 15」 | `TestScanBackfillPageCountMustMatchIstitle`（3 子用例） | 基准确为 `backfillIstitleMark = istitle="true"`。**我独立数**：p146 / p18 / p1 各 `istitle="true"`=15、`class="hui12"`=21、`<span class="hui12">`=15 —— 若用 `class="hui12"` 作基准，守卫会 15≠21 恒假报错。**消融 A4b**（守卫恒不触发）：仅本测试红。断言只针对**具名快照**且注释注明「不是每页恒 15」，无全局硬编码。 | ✅ PASS |
| non_functional[0] | `gofmt -l` 空、`go vet` 空、`go test -count=1` 全绿、不降低既有覆盖率 | — | 三项全部满足（见 §四）。**背对背覆盖率**：pre=93.6% → post=93.7%（+0.1pp），**128 个既有函数逐函数百分比完全相同，无一下降**。`backfill_scan.go` 自身 100%。 | ✅ PASS |

### 额外（DoD 未要求的自补守卫）

| 项 | 测试 | 证据 | 判定 |
|---|---|---|---|
| href 补不成绝对 URL ⇒ 报错，不静默跳过 | `TestScanBackfillPageFailsOnUnparsableHref` | **消融 A6**（改成 `continue` 静默跳过）：仅本测试红 ⇒ 这是真守卫，不是空声明。 | ✅ 有效 |

---

## 三、三处「DoD 文本与实现不一致」的核实结果

Leader 事先提示的三处，我逐一独立核实，**均确认实现正确、不判缺陷**：

### 1. 列表项正则（spec §3.1 原式是真 bug）

实现用的是订正版 `[^>]*?\stitle="`，**已确认在 62b9243 的代码里**（`backfill_scan.go:71`）。

我做了 **消融 A2**（还原成 spec 原式 `[^>]*title="`），结果精确复现了 Leader 指出的形状：

```
FAIL: FindsThreeReportKinds / TakesTitleAttributeNotLinkText /
      ArticleIDHasNoDigitFloor（含 2 子用例）/ ExtractsPublishedDate / RejectsLookalikes
PASS: CountMustMatchIstitle（3 子用例全绿）  ← 计数守卫接不住这个 bug
PASS: ErrorsOnZeroListItems / NoReportsIsNotAnError / FailsOnUnparsableHref
```

**5 个顶层测试红，而 `boundary[2]` 的计数守卫一条都不红** —— 因为贪婪的 `[^>]*` 会吞进
`istitle="true"` 的尾巴，捕获组拿到字面量 `true`：**条数一条不少、日期一天不差，只有标题全错**。
这证实两条守卫互补、缺一不可，也证实这个 bug 只能靠标题类断言接住。

### 2. p146 字节数

任务描述写「实测 38201 字节」。**git 入库值实测 38066 字节、CR 数 = 0**（`core.autocrlf=input`
已把 CR 规范化掉），sha256 `837a607820750a57…`。以入库值为准，不据 38201 判不符。
（附：p18 = 38057 字节、p1 = 38080 字节，均 0 CR。）

### 3. 自补的 href 守卫

见上表：消融 A6 证实它有测试钉住，不是空声明。

---

## 四、跑出的原始证据

```
$ cd ../wt-verify-T1T3 && GOTOOLCHAIN=local
$ gofmt -l internal/hestia/          → 空
$ go vet ./internal/hestia/          → 空
$ go test ./internal/hestia/ -count=1
ok  github.com/newthinker/atlas/internal/hestia  1.025s

$ go test ./internal/hestia/ -count=1 -run '^TestScanBackfillPage' -v
18 条 RUN = 9 个顶层测试函数 + 9 个子测试；18 PASS / 0 FAIL / 0 SKIP
```

**背对背覆盖率对照**（三棵树同为 62b9243，`pre1` 去掉本任务两个 .go 文件，同一时刻并排跑）：

| 树 | 包总覆盖率 |
|---|---|
| `wt-t27-pre1`（无本任务） | 93.6% |
| `wt-verify-T1T3`（交付树） | **93.7%** |

逐函数比对：两侧共有的 **128 个既有函数百分比完全相同，无一下降**。`scanBackfillPage` = 100.0%。

**语料独立核对**（`python3`，未用裸 `sort -u`）：

| 样本 | 字节 | CR | `istitle="true"` | `class="hui12"` | `<span class="hui12">` | 首/末日期 |
|---|---|---|---|---|---|---|
| p146 | 38066 | 0 | 15 | 21 | 15 | 2020-03-25 / 2020-03-11 |
| p18 | 38057 | 0 | 15 | 21 | 15 | 2025-10-17 / 2025-09-22 |
| p1 | 38080 | 0 | 15 | 21 | 15 | 2026-08-10 / 2026-07-21 |

p146 三条目标标题及其日期（我独立抽取，非复用被验正则的写法）：
- `2020年2月金融统计数据报告` → 2020-03-11（id `2025092212550537670`，19 位）
- `2020年2月社会融资规模存量统计数据报告` → 2020-03-11
- `2020年2月社会融资规模增量统计数据报告` → 2020-03-11

p18 目标条目：`5868082`（7 位）/ `2025年前三季度金融统计数据报告`。

---

## 五、声明范围核对

`git show --numstat 97ae2a3` 实际改动 3 个文件：

```
147  0  internal/hestia/backfill_scan.go
352  0  internal/hestia/backfill_scan_test.go
640  0  internal/hestia/testdata/pboc-index-p146-2020.html
```

与 `writes` 声明**逐条一致，零越界、零删除**（纯新增，未触碰任何既有文件）。

validator 报的 3 条 `scope-writes-outside-packages` 是**告警级**，且是「Go 文件写进 writes
而 packages 是包路径」这一标准形状的已知假阳，不构成缺陷。

---

## 六、结论

**VERIFIED。** 8 条 done_criteria 全部通过，每条都有我自己跑出的证据；DoD 明写「这样写会假绿」
的三处（title 属性守卫、0 条列表项合成用例、boundary[0]/[1] 互补）经消融验证均为**真守卫**。
未发现任何缺陷。
