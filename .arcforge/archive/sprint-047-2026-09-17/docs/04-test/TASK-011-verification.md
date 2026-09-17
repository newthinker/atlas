# TASK-011 验证报告 — M2b 配置与文档（`hestia_sheets` 段、CONTRACTS `Sprint M2b-2`、vault 回写内容）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `d42435241b3c7a24c10d2b016db17c364b325631`（= `verify_baseline.head`）；交付 commit `d3d1f5794e95f8dcacba9eec73125d59cdc2beaf`；前一 commit `477664a5651449490ddc602c090501bfd2ab9ead`
- discovery sha256: `e711bb4e75394b5472e14ace0f797b0a8fa60083b65a6df67e789215df1034da`（= `verify_baseline.discovery_sha256`，开工与判定前各现读一次，均一致 ⇒ **无漂移，未用 `--ack-drift`**）
- provenance：dev-m2b-c 自己完成（`assignment_epoch` 1），非接手。DoD 明写不跑 code-simplifier。
- 范围核对: `git diff --numstat 477664a..d3d1f57` **恰为** `writes` 三文件（config.example.yaml 21/0、TASK-011-vault-content.md 145/0、CONTRACTS.md 174/0，三行即全部输出），**删除列全 0**；commit subject 为 `docs(TASK-011): …` ⇒ 合约定。
- **本任务 docs-only，未建 worktree**（没有要跑的测试套件）；需要实跑的两项（样例配置装载、全仓回归）在主仓库干净树上做，`.arcforge/` 之外的工作树无改动。

## 结论：**VERIFIED**（6/6 PASS）

DoD 六条全是 `verify_by: review`，没有可跑的断言。所以本次判定的方法是：**把文档里每一个可机器求值的声称都自己重算一遍**，散文部分逐条对照需求原文与代码事实。下表「我实测」一列的数字与结论，全部是我自己跑出来的，没有引用 dev 的 discovery 或 Leader 派验消息里的任何现成结果。

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `config.example.yaml` 新增 `hestia_sheets` 段，含需求给的三条 🔴 注释原文（留空=禁用；密钥放 runtime 树外因 `rsync --delete`；两份 config.yaml 永不同步，给两个绝对路径） | review | **逐字比对**：从需求原文 `plans/2026-09-16-hestia-m2b-sheets.md` 的 `## TASK-011` 节取出全部注释行（5 行），逐行在交付文件里求值 ⇒ **缺失 0 行**。交付另有 13 行是 dev 新增的（见下「额外加分项」）。两个绝对路径与「永不同步」均在（见 boundary[0]）。⚠️ 措辞更正见下「关于 DoD 字面」① | PASS |
| functional[1] | `CONTRACTS.md` 新开 `## Sprint M2b-2`，六项逐条 + 判据表（一～三填、四～七留空）+ C1–C11 落地对照表 | review | 六项齐全（① 导出面增量与两条守卫新 `want` 列全 ② AST 守卫改递归 ③ 选行 77→61 ④ 1e-9 容差理由 ⑤ dry-run 零写证明 ⑥ §10 七条判据表），C1–C11 对照表 11 行齐全。**表内每一个数字与行号我都重算了**，逐项见下「自证复算表」——含 Leader 点名要核的那条历史断言 | PASS |
| functional[2] | `docs/hestia-m2b/TASK-011-vault-content.md` 两段可直接粘贴 | review | 两段标题齐全：`## → Projects/Hestia/README.md`（:11）与 `## → M2b-Sheets-接入手册.md`（:51）。第一段含「动作一」替换 M2b 行为 ✅ 并写实测、「动作二」更新下一步措辞。第二段含新增 `## 六·五、投影已上线（2026-09-17）`（:58）、`## 七、三条待决已实现`（:105）以及 D1/D2/D3 三小节各一段 `**已实现**（2026-09-17）` 追加，四处引用均指向 CONTRACTS `## Sprint M2b-2`。末尾另有 5 条「粘贴后自查」。**文档里嵌的实测数字我全部复跑核对**（见自证复算表末四行） | PASS |
| boundary[0] | **C10** 两个绝对路径：`<repo>/configs/config.yaml` 与 `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml`，文档明写「永不同步」 | review | 两处都有：`config.example.yaml` 的注释块内逐字给出两条路径；`CONTRACTS.md` 的「**C10 的两个绝对路径**」小节用代码块再给一次。「永不同步」在两处均为加粗原文。支撑事实我独立核过：`deploy.sh:87` 确为 `rsync -a -m --delete`，`:100` 确有 `--exclude='/configs/config.yaml'` | PASS |
| error_handling[0] | CONTRACTS **纯追加**（`git diff --numstat` 删除行 0），不改既有节 | review | 比「删除行 0」更强的证据：`git diff --unified=0` 显示**只有一个 hunk** —— `@@ -4039,0 +4040,174 @@` ⇒ 174 行全部追加在原文件末尾（原 4039 行）之后，**零删除、零插入到文件中间**，4039 + 174 = 4213 与交付后行数逐数对上。既有节结构上不可能被改动 | PASS |
| non_functional[0] | commit subject `docs(TASK-011):`；三文件都在 `writes` 内；`docs/hestia-m2b/` 本 sprint 新建；不跑 code-simplifier | review | subject 逐字为 `docs(TASK-011): hestia_sheets 配置段、CONTRACTS Sprint M2b-2、vault 回写内容` ✅；numstat 三文件与 `writes` 三项一一对应，无越界；`docs/hestia-m2b/` 的首次出现 commit 就是 `d3d1f57`，在 sprint 起点 `1c7af81` 上 `git ls-tree` 命中 **0** ⇒ 确属新建；不跑 simplifier 已在 discovery `degradations` 申报 | PASS |

## 自证复算表（每一行都是我自己跑的，不引用他人结论）

| 声称（出自 CONTRACTS / vault 文档 / 派验消息） | 我的实测方法 | 结果 | 判定 |
|---|---|---|---|
| CONTRACTS 纯追加，删除 0 | `git diff --unified=0` 看 hunk 头 | 单 hunk `@@ -4039,0 +4040,174 @@`，末尾追加 | ✅ **强于声称** |
| AST 守卫 38 → 51，净 +13、零移除 | 解析 sprint 起点 `1c7af81` 与 HEAD 两版 `want`，做**集合差** | 38 → 51；新增 13 项（`sheets.*` **11** + `BuildSheetRows` + `Store.AllPeriods`）；移除 **0** | ✅ 逐项与 CONTRACTS 所列一致 |
| reflect 守卫 14 → 15，只加 `AllPeriods` | 同上 | 14 → 15；新增集合恰为 `{AllPeriods}`；移除 0 | ✅ |
| 🔴 **历史断言**：TASK-001 改 AST 守卫递归那次「两个数字一个都没动（38→38）」 | 用同一把尺解析三个历史点的 `want` 长度：`1c7af81`（001 的父）/ `5a2c1a2`（001 本身）/ `28fca4c`（001 的 merge） | AST 三点**恒为 38**，reflect 三点**恒为 14** | ✅ **属实**（这是本节最有价值的一条，见下「诚实记录」③） |
| 判据表「一～三已填、四～七留空」 | 数 `待人回填` 与 `✅ 2026-09-17 dev-m2b-c` 两个串，再数判据行数做自洽校验 | 4 空 + 3 填 = 7 行，与判据行数 7 相等 | ✅ 自洽 |
| `TestW_PushDryRunNeverCreates` 不在仓库 | 全仓 `grep -rn "TestW_" --include='*.go'` | 命中 **0** | ✅（我有第一手事实，见「诚实记录」①） |
| ⑤ 的三条 dry-run 测试行号 117 / 167 / 240 | 对三个文件逐个 `grep -n "^func <name>("` 取实际行号比对 | **三条全中** | ✅ |
| `requireOnlyGETs` 在 `push_test.go:92` | 同上 | 92 | ✅ |
| 选行 77 → 61 | 自己数 testdata 的 JSON 顶层条数；再实跑那条测试 | testdata **77** 条；`TestSelectRowsCollapsesSeventySevenToSixtyOne` **PASS** | ✅ |
| 两个库 runtime 77 / repo 76 及 mtime | `sqlite3 <db> "select count(*) from hestia_observations"` + `stat` | runtime **77**（mtime 2026-09-16 07:55）／repo **76**（2026-09-02 20:44） | ✅ 逐数与 mtime 均吻合 |
| `sameValue` 在 `diff.go:100-108`、用相对容差 `1e-9*scale` | 直读该文件 | `func sameValue` 在 **100**、闭括号在 **108**，`scale := max(1, …)` 在 104、`<= 1e-9*scale` 在 105 | ✅ 范围精确 |
| C8 落点 `ingest.go:482-489` | 直读 482–489 八行 | 恰为 `if d.ProjectSheets != nil {` 到其闭括号的完整投影块 | ✅ 精确 |
| 未决项①：`hestia_sheets` 只被 `config.go:85` 读 | 直读该行 + 全仓 grep | `config.go:85` 命中 `hestia_sheets` | ✅ |
| 未决项②：主配置无 `HestiaSheets` 字段 | `grep -rn HestiaSheets internal/config/` | 命中 **0**；`HestiaConfig` 只有 `ConfigPath` 一个字段 | ✅ |
| 未决项③：`configs/hestia.yaml` 进 git 且不在 rsync 排除表 | `git ls-files` + 数 deploy.sh 里 `hestia.yaml` | 已跟踪；排除表里 `hestia.yaml` 命中 **0**，而 `/configs/config.yaml` 在 `:100` | ✅ 三条全属实 |
| 三条 🔴 注释「逐字取自需求原文」 | 从需求原文 `## TASK-011` 节抽出全部注释行，逐行在交付文件里求值 | 需求 5 行，交付里**逐字全在**，缺失 **0** | ✅ |
| vault 文档内嵌数字：全仓 65 ok / 0 FAIL | 当前 HEAD 实跑 `go test ./... -count=1` | **65 ok / 0 FAIL**，exit 0 | ✅ |
| vault 文档内嵌数字：`internal/hestia` 96.6%、`sheets` 89.1% | 当前 HEAD 实跑 `-cover` | **96.6%** / **89.1%** | ✅ |
| vault 文档内嵌数字：`cmd/atlas` 78.0%、历史水位 76.8%、floor 77 | 我在 TASK-009 验证时已在两棵树上各跑一遍 | 交付树 78.0%、base 76.8%，`coverage_floor` 现为数字 77 | ✅ |
| vault 文档内嵌数字：导出面 +13、守卫 14/38 → 15/51 | 见本表第 2、3 行 | 一致 | ✅ |
| `docs/hestia-m2b/` 本 sprint 新建 | `git log --diff-filter=A` + sprint 起点 `git ls-tree` | 首次出现即 `d3d1f57`；起点命中 0 | ✅ |
| discovery 申报「vault 笔记本身未被修改」 | 读 `degradations` | 第 2 条明写「vault 两个笔记本身**未被修改**（AD-M2b-2：dev 不直接写 vault）……『是否真的贴进去了』不在本任务的可验证范围内，需人确认后才算闭环」 | ✅ **在**（Leader 第 4 条要确认的就是这条） |

## 🔴 诚实记录：三处必须写下来的地方

### ① 派验消息里给的验证命令是**假绿**，我换了命令重跑

Leader 在派验消息里写：「**YAML 没写坏**：`cmd/atlas/hestia_health_test.go:132` 真会装载 `config.example.yaml`，跑一下 `go test ./cmd/atlas/ -run Health`」。

**行号是对的**——`hestia_health_test.go:132` 确实是 `config.Load("../../configs/config.example.yaml")`。**但那条命令跑不到它**：装载样例配置的测试叫 `TestExampleConfigDeclaresHestiaRules`（`:131`），函数名里**没有 `Health` 这个子串**。`-run Health` 实际只跑到四条 `TestBuildHestiaHealth_*`，而这四条**一条都不装载 `config.example.yaml`**。

我照那条命令跑，得到的是 `ok`——**绿的，但绿的原因与 YAML 写没写坏毫无关系**。换成 `-run TestExampleConfigDeclaresHestiaRules` 重跑，仍是 PASS ⇒ **结论不变（YAML 确实没写坏），但验证路径必须换**。这是「命令跑了、退出码对、数字合理，而它度量的不是你要的那个性质」这一族的典型样本，记在这里是因为下次有人照抄那条命令，会在一个 YAML 真的写坏的 sprint 里同样拿到绿色。

### ② `TestW_PushDryRunNeverCreates` 那件事，我是当事人

CONTRACTS ⑤ 末尾写「验证者在 008 验证期间提到过一条 `TestW_PushDryRunNeverCreates`，**它不在仓库里**（全仓 `TestW_` 前缀命中 0），多半是它在自己 worktree 里的临时测试」。

**这条完全属实，而且我能给出第一手证据**：那正是**我**在 TASK-008 验证时写的验证者夹具，只存在于 `wt-m2b-v008` 那棵已拆除的 worktree 里，原文现存于 scratchpad 的 `test-m2b-b-TASK-008-fixture.go.txt`。我复跑 `grep -rn "TestW_" --include='*.go' .` 得 **0**。dev 的处置是对的：CONTRACTS ⑤ 的表里只列了三条实测存在的测试，没有把一个不存在的名字当真写进契约文档。**这正是「验证者夹具不进交付」这条纪律的必然结果**——它也说明，验证报告里的测试名被下游当作仓库事实引用时，会产生一个真实的误导风险。建议：验证报告里提到自构夹具时，统一标注「只存在于验证 worktree」。

### ③ 那条历史断言值得单独说：它是本 sprint 文档增量里最有价值的一条

CONTRACTS ② 写「TASK-001 把 AST 守卫改成递归那次，数字一个都没动（38 → 38），因为改它的时候 `internal/hestia/sheets/` 还不存在」。

我用同一把尺解析了三个历史点的 `want` 长度，AST 恒为 **38**、reflect 恒为 **14** ⇒ **断言属实**。

它的价值不在数字本身，而在它印证的那条推论：**守卫的盲区在触发它的代码出现之前不可观测**。也就是说「改完守卫、数字没变」这件事看上去像白做，实际是把守卫排在第一位的**唯一正确理由**——若等子包写完再改递归，那段窗口里新增的导出面守卫一项都看不见。这条本来极容易在复盘时被读成「TASK-001 没产出」，写进 CONTRACTS 之后就有据可查了。

## 关于 DoD 字面的一处措辞（不影响判定）

DoD `functional[0]` 写「含需求给的**三条 🔴 注释**原文（留空=禁用；密钥放 runtime 树外因 `rsync --delete`；两份 config.yaml 永不同步，**给两个绝对路径**）」。对照需求原文实际是：

- 「留空 = 能力禁用」是 yaml 块的**首行普通注释**，不带 🔴；
- 只有后两项各带一个 🔴 ⇒ 需求原文是 **1 条普通注释 + 2 条 🔴**，不是三条 🔴；
- 「**给两个绝对路径**」这一项**需求原文里没有**，它来自 DoD 自己的 `boundary[0]`。

交付把三项内容全部满足了（需求 5 行逐字全在 + 两条绝对路径按 `boundary[0]` 补上），所以判 PASS。这里记一笔只是为了让「三条 🔴」这个说法不被后人拿去当需求原文的描述。

## 额外加分项（需求未要求，dev 自己加的）

`config.example.yaml` 里多出一大段 `⚠️ **生效位置不是本文件。**`：说清 `hestia_sheets` 真正的读取方是 `internal/hestia/config.go` 的 `Config.HestiaSheets`，装载的是 `hestia.config_path` 指向的文件，而主配置 `internal/config` **没有** `HestiaSheets` 字段，照抄进 `configs/config.yaml` 顶层**不会有任何代码读它**、能力会静默保持禁用。

我独立核过这三条支撑事实（见自证复算表未决项①②③），全部属实。这段防的是「填错位置零反馈」——配置读取器对未知顶层键不报错，这恰好是 C9 静默降级最容易咬人的形状。需求没要求，但它是这次文档里实用价值最高的一段。

## 验证者记录的其它观察（不判红）

- **一处行号范围略偏**：CONTRACTS ③ 写 `TestSelectRowsCollapsesSeventySevenToSixtyOne`（`sheets_project_test.go:113-121`），实际 `func` 在 **116**、测试体 117-121、闭括号 122；113 落在注释段中间。读者按这个范围去看仍能定位到目标，不构成误导，但不如本节其它行号精确（其余我逐个核过的都精确命中）。
- **未决项的处置是正确的**：`hestia_sheets` 凭据落位列了三条出路（给主配置加字段并透传 / 把 `/configs/hestia.yaml` 加进 rsync 排除表 / 引入 gitignored 的 `configs/hestia.local.yaml`）而**未选定**。三条都需要人拍板，本任务 `writes` 不含 `configs/hestia.yaml` ⇒ 只登记不动手是对的。我按 Leader 的要求只核了三条陈述是否属实（属实），没有评判该选哪条。
- **判据四～七留空是对的**：它们要对真表、真网、真 ingest 跑。CONTRACTS 自己写了理由——「把它们编出来会让整张表看着像已验收」。同时按 AD-M2b-8，判据四的基线数字（1282/34/819）不进任何任务的 DoD，因为它随真表与库状态漂移。
- **vault 那 145 行不会自动生效**：discovery `degradations` 已明写笔记本身未被修改、需人粘贴后才闭环。**不要从「TASK-011 verified」推出「vault 已更新」。**

## 复现命令（锚一律全 sha）

```bash
# 范围与纯追加（单 hunk 即最强证据）
git diff --numstat  477664a5651449490ddc602c090501bfd2ab9ead..d3d1f5794e95f8dcacba9eec73125d59cdc2beaf
git diff --unified=0 477664a5651449490ddc602c090501bfd2ab9ead..d3d1f5794e95f8dcacba9eec73125d59cdc2beaf -- internal/hestia/CONTRACTS.md | grep '^@@'

# 样例配置真能装载（🔴 不是 -run Health，那条跑不到它）
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -run TestExampleConfigDeclaresHestiaRules -v

# 选行与全仓
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -run TestSelectRowsCollapsesSeventySevenToSixtyOne -v
GOTOOLCHAIN=local go test ./... -count=1                                    # 65 ok / 0 FAIL

# 守卫增量与历史断言（对三个历史点用同一把尺解析 want）
#   1c7af81 = TASK-001 的父提交，5a2c1a2 = TASK-001 本身，d4243524 = 本次 baseline
git show 1c7af81:internal/hestia/store_test.go   # AST 38 / reflect 14
git show 5a2c1a2:internal/hestia/store_test.go   # AST 38 / reflect 14 —— 改递归当场一个数没动
```
