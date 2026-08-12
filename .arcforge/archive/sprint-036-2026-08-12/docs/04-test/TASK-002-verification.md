# TASK-002 验证报告 — index 快照与分页模板解析

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = 0597fcaae306d299aea593957b696d69770641c4`（== 验证期间的当前 HEAD）
- **TASK-002 自身提交**：`99c8c6a1fcbabd7fa1af2e4f6baa1118235b828e`
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-2 0597fcaae306d299aea593957b696d69770641c4`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移核验 —— 代码零漂移；**discovery 漂移已查清并 ack**

### 代码侧：零漂移
`git rev-parse HEAD` = `0597fcaae306d299aea593957b696d69770641c4` == `verify_baseline.head`，逐字相同。
`git show --numstat 99c8c6a` → `discover.go 68/0`、`discover_test.go 163/0`、`testdata/README.md 51/0`、
`pboc-index-p1.html 640/0`、`pboc-index-p2.html 640/0` —— 与 `writes` **五项逐项一致，无越界**，
且**全部 +N/-0 零删除行**（结构上不可能放松既有断言）。

### discovery 侧：**派验后 51 秒被改写**，已取回基线版逐字段比对

```
verify_baseline.discovery_sha256 = 06a18b383ad916b7403ea418f9ee824d24a2574d615c5984491385ced792df97
我读到的当前值                   = eb8883ba5e2db21eee978ba47295423780aa865eeedd4b0bae5ce7691e44212b
基线 at   = 2026-08-12T02:06:36Z（10:06:36 CST）
文件 mtime = 2026-08-12 10:07:27 CST      ⇒ 派验后 51 秒
```

这正是 CLAUDE.md 的 AD-29 记录过的形态（「Leader 派验后 53 秒 discovery 即被改写」）。
discovery 是**未跟踪文件、无 git 历史**，本来取不回基线版；我在共享 scratchpad 里找到了
dev-agent-50 留的副本 `dev50-TASK-002-discovery-BASELINE-06a18b38.json`，
**其 sha256 精确等于 `06a18b38…`** ⇒ 确认是基线那一版，据此做了真实差异比对：

```
$ diff <(jq -S . <基线副本>) <(jq -S . .arcforge/discoveries/TASK-002.json)
diff 行数 = 15，唯一变化在 .verification.code_simplifier
（从一个字符串 → 扩写成一个对象：outcome / how_i_verified_no_change /
  findings_by_subagent[4] / attribution_note / why_the_reply_was_terse）
```

**逐字段核对：零个自证数字被改动。**

| 字段 | 结果 |
|---|---|
| `.verification.coverage` / `.suite` / `.test_case_count` / `.numstat_from_commit` | 相同 |
| `.verification.ablation.result` / `.snapshot_blob_sha256` / `.all_numbers_sampled_at` | 相同 |
| `.verification.red_output_verbatim_1` / `_2` / `.export_guards` / `.gofmt` / `.vet` | 相同 |
| `.commit` / `.base` / `.files_modified` / `.no_existing_assertion_relaxed` | 相同 |
| `key_findings` 条数 8→8、`decisions` 条数 6→6、`ablation.mutations` 条数 9→9 | 相同 |

改动的**结论也没变**：基线版写「⇒ **未做任何改动**」，新版写「**无改动** —— 一个字节都没改」。
新增的是**归属标注**（哪几条是子代理的判断、哪几条是 dev 亲验）与子代理回复含糊的成因说明
（TeammateIdle hook 按 session 粒度判定身份，对子代理产生可复现误判）。

⇒ **这是扩写而非改数，不触及任何 DoD 判定原料**，故我以
`--ack-discovery-drift eb8883ba5e2db21eee978ba47295423780aa865eeedd4b0bae5ce7691e44212b` 显式确认放行。
**`ack` 的应答不留任何痕迹（`transitions.jsonl` 全文件 `ack` 出现 0 次），本节是唯一留痕处。**

### 三方独立比对，结论一致

本节结论来自**我自己**的差异比对，做出时尚未收到任何答复。事后 dev-agent-50 与 Leader
各自独立给出了变化面，三方结论**逐字一致**（唯一变化字段 `verification.code_simplifier`，
DoD 自证数字一字未变）。

值得记的是三方都**拒绝互相代填 `--ack-discovery-drift` 的值**并各自说明了同一条理由：
`--ack-*` 要求填**当前值**而非记录值，其全部意义就是「必须真去看过现状才填得出」，
代填等于把守卫作废。本报告里的 `eb8883ba…` 是我自己 `shasum -a 256` 取的。

dev-agent-50 补充的时序（与我从 mtime 推得的一致）：它在 `dev_done` 状态下先查了状态才动手写，
**查状态与写入之间隔着构造 JSON 的时间**，派验恰好落在中间。⇒ 这不是有人跳过了检查，
而是**检查与动作之间存在竞态窗口**；它建议给 `discovery` 子命令加 `--expect-sha256`
（与 `--expect-epoch` 同构）来关掉这个窗口。我认为这个方向是对的：本次是善意改写所以无害，
但同一个窗口同样容得下一次改数。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 证据 | 判定 |
|---|---|---|---|
| F1 | 抓存两份**真实**快照，与生产实现同构的 UA | 联网复抓比对（§2），**逐字节相同** | **PASS** |
| F2 | `parsePaging` 解析出模板与总页数，**解析而非写死** | `TestParsePaging` + `TestParsePagingFollowsChangedColumnID`；消融的消融（§4） | **PASS** |
| F3 | `pageURL` 三形态（1/2/408）+ **注释理由必须改写** | `TestPageURL`；注释已改写并警告；联网复验（§2） | **PASS** |
| B1 | p1 **无**报告条目、p2 **有** | 实测（§3） | **PASS** |
| E1 | 解析不到必须报错含 `paging`，不静默退化；**总页数非法分支须有用例** | `TestParsePagingFailsLoudly` + `RejectsBadTotal`（5 例）+ `RejectsTemplateWithoutPlaceholder`；M2/M3/M4/M5 KILLED | **PASS** |
| N1 | README 记两份快照的抓取日期与摘要，并写明**两点** | §3 | **PASS** |
| N2 | RED 因**预期原因**失败 | 独立复现两次（§5） | **PASS** |
| N3 | gofmt/vet 无输出、整包绿、覆盖率 ≥ 92.1% | §6 | **PASS** |

## 2. F1 / F3 —— 我联网做了独立的真实性对照（本任务最难验的一条）

DoD `functional[0]` 要求快照是**真实抓取**的。这条无法靠读文件证伪，所以我直接联网复抓：

```
$ curl --noproxy '*' -A 'Go-http-client/1.1' .../113469/index.html
HTTP=200 bytes=38147
$ tr -d '\r' < live-p1.html | shasum -a 256
ec7cdc220158efcd12669810ed17a29f6ebd8f06696fbb4221d27ec5c300151a
$ shasum -a 256 internal/hestia/testdata/pboc-index-p1.html
ec7cdc220158efcd12669810ed17a29f6ebd8f06696fbb4221d27ec5c300151a
$ diff <(tr -d '\r' < live-p1.html) internal/hestia/testdata/pboc-index-p1.html   → 逐字节相同
$ tr -cd '\r' < live-p1.html | wc -c → 67
```

**实时页去 CR 后与入库版逐字节相同**，且 `38147 − 67 = 38080` 与 README 的两列数字**逐数吻合**
⇒ 快照真实性、以及 README 关于 `core.autocrlf=input` 把 67 个 CRLF 规范化掉的解释，
**双双被独立证实**（不是采信 dev 的自述）。实时页的 `createDate`（`2026-08-10 18:20:52`）
与分页控件（`jumpTo(this,'408','1','/goutongjiaoliu/113456/113469/11040-%1.html')`）也完全一致。

**F3 的「注释理由必须改写」我也联网复验了**：

```
11040-1.html: HTTP=200 bytes=38147     ← 与 index.html 逐字节相同（cmp 通过）
index_1.html: HTTP=404 bytes=146
```
⇒ 计划原注释「模板生成的 `11040-1.html` **未必存在**（实测 `index_1.html` 是 404）」
确实是**用另一个 URL 的观察去论证**，理由是错的。`discover.go:47-58` 的注释已改写为真实理由
（不重复请求 / 不制造 URL 别名），并显式写了
「⚠️ 别拿 `index_1.html` 是 404 来论证这件事——那是**另一个 URL**，模板压根不生成它」。
DoD 这一条要求的正是这个，**满足**。

## 3. B1 / N1 —— 快照内容与 README

```
p1: jumpTo(this,'408','1',...)  「金融统计数据」出现 0 次  日期 2026-07-21 ～ 2026-08-10  charset=utf-8
p2: jumpTo(this,'408','2',...)  「金融统计数据报告」出现 2 次（另有干扰项，共 4 次）
                                日期 2026-06-28 ～ 2026-07-20（+ createDate 的 2026-08-10）
```
p1 **无**报告条目、p2 **有** ⇒ B1 满足，且与 README 表格逐项一致。

README（`testdata/README.md` +51/-0 纯追加）两节都在：
**「为什么两份都要留」**（第 1 页无报告是常态；只留有报告的那份，一个只扫第 1 页的实现照样全绿）
与 **「测试断言的是『翻页直到找到』，不是『报告在第 2 页』」**（含失效预测：报告在 p2 第 5 条、
p2 最旧 2026-06-28，再过两三周会掉到第 3 页，并给了更新办法）⇒ N1 满足。
README 另外主动纠正了计划「不带 User-Agent」这句措辞（curl 与 Go 各发一个不同的 UA，
两者都不是「不带」），这处纠正正确。

## 4. 变异/消融独立复验（harness 自写）

Harness：`scratchpad/test25-TASK-002-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写、默认**全 sha**
`0597fcaa…`，打印锚点与主仓库工作树 HEAD；变异作用在 `/tmp/mut-036-2` 隔离 worktree 上。
四道闸：基线闸（全绿，`--- PASS` = 514）、生效闸、**编译失败闸**、**计数自证 11 == 11 → OK**。

| 变异 | 结果 | 实测死因 |
|---|---|---|
| M1 `parsePaging` 写死栏目 ID 与页数 | KILLED | 见 §4.1 |
| M2 解析不到时静默退化成 `("/x-%1.html", 1, nil)` | KILLED | `TestParsePagingFailsLoudly` + `RejectsBadTotal` 的负数/非数字/空三子测试 |
| M3 零页校验放宽（`< 1` → `< 0`） | KILLED | **仅** `RejectsBadTotal/零页` |
| M4 丢掉 `Atoi` 的 `err != nil` 半边 | KILLED | **仅** `RejectsBadTotal/溢出_int64` |
| M5 去掉 `%1` 占位符检查 | KILLED | `RejectsTemplateWithoutPlaceholder` |
| M6 正则改捕**当前页**而非总页数 | KILLED（补跑） | `TestParsePaging`（`"1" is not greater than "100"`）+ `SamePagingOnEveryPage` + `FollowsChangedColumnID` |
| M7 第 1 页也套模板（`page <= 1` → `<= 0`） | KILLED | `TestPageURL` |
| M8 `bad index url` 的 `%w` 改 `%v` | KILLED | `RejectsUnparsableInput/index_url_不可解析`：`Expected value not to be nil` |
| M9 页码不代入模板（`Replace` 的 n 改 0） | KILLED | `TestPageURL` |
| **M10【我补】** 报错时同时返回 `1` 页 | KILLED | `assert.Zero`：`Should be zero, but was 1`，Messages「报错时不得同时返回一个看似可用的页数」 |
| **M11【我补】** `bad paging template` 的 `%w` 改 `%v` | KILLED | `RejectsUnparsableInput/模板不可解析` |

**12 条变异（含补跑的 M6）全部 KILLED，0 SURVIVED。**

M3 / M4 的「**仅**」是逐条核对失败测试集合得到的，与 dev 的声称一致 ——
其中 M4 精确证明「`err != nil ||` 那半个条件的唯一守卫是溢出用例」，
没有那一行它就是没人看得出来的死代码。

**闸起作用的一次实例**：M6 首轮因我的替换串在 bash 里转义不当而未匹配，
harness 记为 `INVALID(M6): 替换串未匹配` 并让计数自证不通过 —— **闸正确地没有把它记成
SURVIVED**（假 SURVIVED 的方向恰好是让人去加不必要的断言）。我用 python 精确替换后补跑，KILLED。

### 4.1 消融的消融 —— dev 的核心论断我做了正向验证

DoD `functional[1]` 说「解析而不是写死栏目 ID」。dev 声称「只用真实快照测**证不出**这句话」。
我在 M1（写死实现）下分别只跑两个测试：

```
$ go test -run '^TestParsePaging$' -v                          → --- PASS: TestParsePaging
$ go test -run '^TestParsePagingFollowsChangedColumnID$' -v    → --- FAIL
      Messages: 模板必须来自页面，不能写死 11040
      Messages: 总页数必须来自页面，不能写死 408
```

⇒ **只用真实快照的那三条断言，对写死实现完全没有鉴别力**（它们断的正是快照里那个栏目的
特征值）；`TestParsePagingFollowsChangedColumnID` 是 `functional[1]` **唯一**以正确理由承重的
断言。删掉它，那句 DoD 就退回成无实证的声明。dev 的论断成立。

### 主工作区完整性

变异窗口内 + 收尾双重核实，四个文件 sha256 与 `git status --porcelain` 前后逐字相同：
`discover.go 2c9b6354…`、`discover_test.go 113b0ba8…`、`p1 ec7cdc22…`、`p2 ea30a833…`。
变异树收尾 sha256 一致（`OK`）；`/tmp/mut-036-2`、`/tmp/verify-036-2` 均已 remove + prune。

## 5. N2 —— RED 独立复现两次

```
① 删掉整个 discover.go：
internal/hestia/discover_test.go:27:22: undefined: parsePaging   （及 6 处同形）

② 只保留 parsePaging（Step 5-8 的中间态，并按 dev 的做法从 import 块去掉 net/url）：
internal/hestia/discover_test.go:137:15: undefined: pageURL      （及 2 处同形）
FAIL github.com/newthinker/atlas/internal/hestia [build failed]
```

两次都**因预期原因失败**，与 dev 记录的 `red_output_verbatim_1/_2` 同形。
② 尤其关键：**去掉 `net/url` 后并未出现 `"net/url" imported and not used`**
⇒ reviewer F3/R3 指出的计划缺口（Step 6 的 import 块多了 `net/url`）确实被 dev 按 Step
拆分 import 规避掉了，RED 信号未被污染。

## 6. N3 的命令与输出

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/    → 无输出，exit 0
$ gofmt -l internal/hestia/                      → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...               → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.809s  coverage: 92.5% of statements
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
ok  github.com/newthinker/atlas/internal/hestia  3.516s
$ go test -run 'TestStoreExposesNoWriteMethods|TestPackageExposesNoWriteFunctions' -v
--- PASS: TestStoreExposesNoWriteMethods
--- PASS: TestPackageExposesNoWriteFunctions
```
覆盖率 **92.5% ≥ 92.1%**；导出面守卫两条均 PASS ⇒ 确认
`parsePaging`/`pageURL`/`readTestdata` 全部不导出、`writes` 不含 `store_test.go` 是对的。

### ⚠️ 覆盖率的两个数字是**两棵不同的树**，不是两种口径

| 数字 | 测自哪棵树 | 谁测的 |
|---|---|---|
| **92.3%** | dev 的分支单独测，基线 `f5a17d5`（不含 T1/T6） | dev-agent-50，记在 discovery 的 `verification.coverage` |
| **92.5%** | 合并后的 master `0597fcaa`（T1/T6 一并计入） | 本报告全部数字的来源；Leader 独立复测同为 92.5% |

**口径完全相同**（都是 `go test ./internal/hestia/ -count=1 -cover`，`packages` 未变），
差的只是被测的树。两者都 ≥ DoD 下限 92.1%。

此处特别注明，是因为上个 Sprint 有过一次「**同一口径不同树**被误读成**三种口径**」的教训（F39）——
自证数字若不带「测自哪棵树」，读者无法从数字本身分辨这两种情形。

## 7. 观察项（不影响判定）

1. **快照的内容分布（p1 无报告、p2 有）目前没有任何测试守着它。**
   B1 是对交付物的判据，我已实测确认成立，故判 PASS。但 `discover_test.go` 只把两份快照
   喂给 `parsePaging`，**没有一条断言 p1 无报告条目 / p2 有报告条目**。将来若有人更新快照
   （README 已预告「再过两三周报告会掉到第 3 页」），把 p1 换成一份含报告的页面，
   T3/T5 的「翻页直到找到」测试会失去意义而无人告警。
   **建议交给 T3/T5 的验证者跟进** —— 那时报告条目提取已实现，钉住这个分布才有手段。

2. **`discovery` 字段缺失是系统性的**（TASK-006 的报告已提过）：TASK-002 同样
   `has("discovery") == false`，我在转 `verified` 前经写通道 `update` 补上并 jq 核实。
   目前 TASK-003/004/005/007 也都是 `false`（尚 `pending`，未到触发点）。

3. `git worktree list` 里四个历史遗留 `.worktrees/*`（m1/m3/phase2/phase3）与本 Sprint 无关，
   可在阶段边界清理。dev 在 discovery 里也报告了同名残留分支 `task/TASK-002` 的问题
   （核实 `master..task/TASK-002` 为空后删除重建，零提交丢失）——这类残留会随 Sprint 累积。

## 8. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应证据；快照真实性由**联网逐字节比对**独立证实，
不依赖 dev 自述；12 条变异全部 KILLED；「只用真实快照证不出解析而非写死」这一核心论断
由消融的消融正向验证；RED 两次独立复现且信号未被污染；主工作区零污染；
代码侧零漂移，discovery 漂移已取回基线版逐字段查清（仅扩写、零自证数字改动）并显式 ack。
