# TASK-009 验证报告 — M2b CLI 接线（`hestia sheets push` 子命令与 ingest 侧投影装配）

> **本文件含两轮**：**第一轮首验**（2026-09-17 01:12Z，VERIFIED）在下方「第一轮」各节；
> QA 判 REJECT 后 009 进 `review_fix`，**第二轮返工复验**（2026-09-17，VERIFIED）见文末
> 「# 第二轮：返工复验」。**第二轮才是当前生效的判定。**

## 第一轮：首验（2026-09-17 01:12:42Z）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `477664a5651449490ddc602c090501bfd2ab9ead`（= `verify_baseline.head`）；交付两个 commit `a2d0267cdce18586ab908dd818ab06d11fa59028`（feat）+ `121764cdae8a4e543b60e22a947fb502c6ebbe53`（refactor / code-simplifier）；base `3770e9482129981cd7a0608e55fadd2c090203ed`
- discovery sha256: `c456c2a70a249e5889522af430a52fc7edf26a6575fad7a477b728f7ee0caaa8`（= `verify_baseline.discovery_sha256`，开工与判定前各现读一次，均一致 ⇒ **无漂移，未用 `--ack-drift`**）
- provenance：**真开发，不是接手**（owner 全程 dev-m2b-c，`assignment_epoch` 1）。三文件 sha256 现读 `0efac794…`（hestia_sheets.go）/ `3f23241d…`（hestia.go）/ `388ed02a…`（hestia_sheets_test.go），与 discovery 声明一致。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v009` @ `477664a5651449490ddc602c090501bfd2ab9ead`（全 sha 锚）；另建过一个临时 base worktree @ `3770e9482129981cd7a0608e55fadd2c090203ed` 复核覆盖率基线，已拆
- 范围核对: `git diff 3770e94..477664a --numstat` **恰为** `writes` 三文件（hestia.go 33/10、hestia_sheets.go 244/0、hestia_sheets_test.go 563/0，三行即全部输出）；两个 commit 各自的 numstat 也只含这三个文件，refactor 那个只动测试文件（34/32）⇒ 无越界申报、go.mod 与 internal 下任何文件均未被触碰。

## 结论：**VERIFIED**（6/6 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `sheets push` 挂在 `hestia` 下；五个 flag；默认 dry-run 末尾固定一句 `未写入任何内容；确认后加 --apply`；缺表时提示「将新建工作表」 | test | dev 4 条。验证者 `TestX_CommandTreeReachableFromRoot` 从 **rootCmd 逐层下钻** `hestia → sheets → push`（判据是「按真实路径找得到」，不是「变量非 nil」——后者对一个忘了 `AddCommand` 的实现同样为真），断言 `RunE` 非 nil、五个 flag 全在 push 上、且 `--period-type` 的 `DefValue` 为空串。`TestX_DryRunLinePresentThenAbsent`：dry-run 时 DoD 那句**逐字出现且在末尾**（`HasSuffix`），`--apply` 时它必须消失——真写了还说「未写入任何内容」是最坏的一种假话。`TestX_FormatResultMissingTabsLine`：缺表提示只在 `MissingTabs` 非空时出现，不缺表时 `NotContains` | PASS |
| functional[1] | `formatResult` 三类计数**分开三行**各带数字（DoD 点名验证者直接对纯函数断言）；`push --all` 经测试缝跑通 | test | dev 2 条。验证者 `TestX_FormatResultThreeCountLines` 对纯函数逐行断言：`将写 7 格` / `一致跳过 3 格` / `库缺跳过 5 格` 三行**整行相等**（不是在整段里 Contains），另数一遍「以这三个前缀开头的行恰为 3 行」钉住不得合并；行数 `2 行` 单独一行且按 sheet/row 去重；三种 `Kind` 的明细行各自报出类别；`(空)` 而非 `<nil>`；最后同一输入连调两次**逐字节相同**以证其为纯函数 | PASS |
| functional[2] | `runHestiaIngest`：凭据非空 ⇒ 建 client 并填 `deps.ProjectSheets`（`Options{Apply:true, CreateSheets:true}`）；留空 ⇒ 保持 nil（C9） | test | **DoD 字面的「内部调 `BuildSheetRows`+`sheets.Push`」经 Leader 裁决为写 DoD 时的错，我独立复核后同意**——见下「关于 DoD 字面的两处偏差」①。按正确读法验：`TestX_ProjectorPassesRowsThroughUnchanged` 用**哨兵 rows**（1999 年，库里不可能有）注入，断言 Push 收到的**就是传进去的那一批**（`require.Equal(mine, *gotRows)`），钉住闭包不得自己重算；`Options` 用**整结构体相等**而非逐字段。`TestX_IngestDepsProjectorIsTheWiredOne` 走完整装配路径再验一遍（只测 `sheetsProjector` 证明不了「它被接上了」）。`TestX_ProjectorNilIsLiteralNil`：凭据留空必须是**字面量 nil**，并直接断言 `deps.ProjectSheets != nil` 为 **false**——那正是 ingest 侧 C9 的唯一判据形式，一个「装了 nil 的函数值」会从这里穿过去 | PASS |
| boundary[0] | flag 校验在**开库与建客户端之前**（无库环境也能跑）；`--period/--period-type` 给定 ⇒ 只推该期 | test | dev 4 条。验证者 `TestX_FlagValidationPrecedesStore` 用**指向不存在目录的配置**他证顺序：五种非法组合（都不给 / 只给 period / all 与 period 同给 / period 格式非法 / period-type 非法）各断言两件事——错误里含预期 needle，**且不含 `nonexistent-dir-xyz`**（若校验跑在开库之后，报的会是开库失败）。成对的反向断言 `TestX_ValidFlagsStillReachStore`：flag 都合法时配置错误必须真的被带出来——**少了这一条，一个「永远不开库」的实现也能让前一组全绿**。`TestX_PeriodFilterReachesPush` 他证到 Push 实际收到的 rows：给 `2025-12` 只收到 1 行且年月正确，不给 period 收到全部 3 行 | PASS |
| error_handling[0] | CLI 里 `credentials_file` 留空 ⇒ 打印 `hestia_sheets 未配置（credentials_file 留空）` 并退出 **0** | test | dev 1 条。验证者 `TestX_DisabledPrintsOneLineAndExitsZero`：`require.NoError`（退出码 0，能力禁用不是故障）+ stdout **逐字节恰为** `"hestia_sheets 未配置（credentials_file 留空）\n"`（`require.Equal` 整串，不是 Contains——多打别的东西也会红） | PASS |
| error_handling[1] | 🔴 投影 panic 必须经闭包内 `defer recover()` 转成 error 返回（**不是吞掉**），走 C8「只打印不返回」路径 | test | dev 2 条（建 client panic / Push panic）。验证者 `TestX_ProjectorRecoversAllPanicShapes` 补三种 dev 未覆盖的形态：**runtime panic**（nil 指针解引用）、**`panic(nil)`**（Go 1.24.4 下会被 recover 成 `*runtime.PanicNilError`，漏兜就会穿透）、**panic 值是 error**（断言原始值留在错误里）。`TestX_PushPanicKeepsIngestGreen` 独立构造端到端：`Ingest` 返回 nil、`runs[0].Outcome == RunIngested`、取出以 `sheets: ` 开头的整行断言**恰一行**且 `HasPrefix("sheets: 投影失败（不影响入库）: ")`、并含 `投影 panic` 与注入的原始 panic 值（证明是**转成 error** 而不是吞成空错误）、进程未崩 | PASS |
| non_functional[0] | `go test ./cmd/atlas/ -count=1` 全绿；`gofmt`/`go vet` 零输出；覆盖率不低于门禁门槛；code-simplifier | manual | **净态**（验证者夹具移出后采）：`go test ./... -count=1` exit 0、**65 ok / 0 FAIL**；`go vet ./...` exit 0 无输出；`gofmt -l` 对**三个 writes 文件**空输出。**含夹具**：`go test ./cmd/atlas/ -v` exit 0，顶层 **289 PASS / 0 FAIL / 0 SKIP**，两把独立的尺一致（顶层 RUN 289 == PASS+FAIL+SKIP 289）。dev 的 25 个测试两把尺也一致（`grep -c '^func Test'` = 25、去重函数名计数 = 25）。**覆盖率我在两棵树上各自复跑，不引用他人数字**：门禁口径（`go test ./cmd/atlas -coverpkg=./cmd/atlas` + `go tool cover -func total`）在 base `3770e94` 得 **76.8%**、在交付树 `477664a` 得 **78.0%**（+1.2pp）；`coverage_floor=77`，门禁比较式 `[ "${TOTAL%.*}" -lt "$DEV_MIN" ]` 实跑得 `78 >= 77` ⇒ 放行。**code-simplifier**：dev 在提交之后跑、改动单列为 `121764c`；子代理回复称「无新增」而 sha256 显示它改了测试文件，dev 以 `git diff` 为准未采信——我复核该 commit 的 numstat 为 `34/32` 且**只动测试文件**，实现文件逐字节未变 | PASS |

## 变异测试

16 个变异，作用在**隔离 worktree 副本**的 `cmd/atlas/hestia_sheets.go` 与 `cmd/atlas/hestia.go` 上，主工作区一个字节不碰。
流水：落盘 → `gofmt -e` 语法闸 → `go vet` → **打印变异 diff 逐字核对**（语义闸）→ `go test ./cmd/atlas/` → `git checkout` 还原 → 校验 worktree 与主工作区两处 sha256。有效性闸（编译失败 / 非零退出但零条 `--- FAIL` ⇒ 判无效并立即早退）本轮未命中。收尾两文件 sha256 均回到 `0efac794…` / `3f23241d…`。

**结果：16 KILLED / 0 SURVIVED。**

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| P1 flag 校验挪到开库之后 | KILLED | dev 4 条 + 验证者 X6 / X10 |
| P2 `recover` 吞掉 panic（不转 error） | KILLED | dev 2 条 + 验证者 X4 / X5 |
| P3 去掉整个 `defer recover`（panic 穿透） | KILLED | dev 1 条（**靠 panic 崩溃，见「诚实记录」②**） |
| P4 凭据留空时退出非零 | KILLED | dev 1 条 + 验证者 X8 |
| P5 三类计数合并成一行 | KILLED | dev 1 条 + 验证者 X11 |
| P6 闭包 `Options` 改成 `Apply:false` | KILLED | dev 1 条 + 验证者 X1 / X2 |
| P7 闭包 `CreateSheets` 改成 false | KILLED | dev 1 条 + 验证者 X1 / X2 |
| P8 闭包忽略入参 rows、传 nil 给 Push | KILLED | dev 1 条 + 验证者 X1 / X2 |
| P9 凭据留空返回**非 nil** 空闭包 | KILLED | dev 2 条 + 验证者 X3 |
| P10 `filterRowsByPeriod` 改回拆 period 比较 | KILLED | dev 1 条 + 验证者 X9 |
| P11 缺表提示无条件打印 | KILLED | dev 1 条 + 验证者 X12 |
| P12 dry-run 那句无条件打印 | KILLED | dev 1 条 + 验证者 X13 |
| P13 `cellDisplay` 不特判 nil（显示 `<nil>`） | KILLED | **只有验证者 X11** |
| P14 建了 sheets 命令但不挂到 hestia 下 | KILLED | 12 条（dev 8 + 验证者 4） |
| P15 `hestiaIngestDeps` 不挂 `ProjectSheets` | KILLED | dev 2 条 + 验证者 X2 / X5 |
| P16 `--period-type` 给默认值 `monthly` | KILLED | **只有验证者 X15**（见「诚实记录」①） |

## 关于 DoD 字面的两处偏差（均判「不是缺口」，理由独立核实过）

① **`functional[2]` 的「内部调 `BuildSheetRows`+`sheets.Push`」是 DoD 写错了。** 我没有引用 Leader 的结论，而是自己去读了 `internal/hestia/ingest.go` 的投影块：ingest **自己**先 `rows, berr := buildSheetRows(ctx, d.Store)`，再把 rows 传进 `d.ProjectSheets(ctx, rows)`。闭包若再调一次就是重算并忽略入参。还有一条**结构性佐证**：闭包签名是 `func(context.Context, []sheets.Row) error`，**手上根本没有 `*Store`**，而 `hestia.BuildSheetRows(ctx, st)` 必须要一个 Store ⇒ 它在这里**调不动**。按字面判会误红。`BuildSheetRows` 在 CLI push 那条路径上确实归本任务调（`hestia_sheets.go:114`），已核。

② **dry-run 那句实际输出带 `dry-run：` 前缀**（`dry-run：未写入任何内容；确认后加 --apply`）。DoD 要的那句作为**后缀逐字完整存在**，我的断言同时验了「逐字出现」与「在末尾」。判满足。

## 诚实记录：三处值得写下来的地方

① **变异 P16 只被验证者的夹具杀掉，而它打的正是 discovery 明确声称的一条性质。** `key_findings[1]` 写着「`--period-type` **刻意不给默认值**——给了默认值，『只给 `--period`』就不再是错误，而那正是要拦的输入」。我把默认值改成 `monthly` 之后，dev 的 `TestSheetsPushPeriodNeedsType` **仍然是绿的**。成因是测试隔离：`sheetsExec` 的 `t.Cleanup` 会把五个全局 flag 变量重置成零值，而在该用例之前已有别的用例调过 `sheetsExec` ⇒ cobra 注册的默认值早被抹掉，**这条性质在 dev 的测试里不可观测**。我的 `TestX_CommandTreeReachableFromRoot` 直接读 `Flags().Lookup("period-type").DefValue`，不受运行时重置影响，所以杀得掉。**声称为真，但无测试守卫**——与 TASK-008 的 N10 同型，建议移植。

② **P3（删掉整个 `defer recover`）是靠 panic 崩溃杀的，不是靠断言。** panic 穿透会终止整个测试二进制，于是只留下一条 `--- FAIL`。这仍是有效的 KILLED（不可能静默通过），但证据来源是语言语义而非断言，值得写明而不是让 KILLED 这个词盖过去。对照之下 **P2（把 `recover` 改成吞掉）被 4 条断言杀**，那才是「转成 error 而非吞掉」这条要求真正被守住的证明。

③ **Leader 要我做的「`filterRowsByPeriod` 行为等价复核」，没有原版可以比对。** `git log -S filterRowsByPeriod --all` 只有 `a2d0267` 一个 commit，而 `git show a2d0267:cmd/atlas/hestia_sheets.go | grep -c Atoi` = **0** ⇒ 那两条 `Atoi` 错误分支**从未进入 git**，删除发生在开发过程的工作树里。所以我把复核换成两问：
- **当前实现在可达域内是否正确**：`TestX_FilterRowsByPeriodBoundaries` 十例表驱动（规范形态命中、跨年跨月分得开、非补零 `2025-6` 不放行、多一位不放行、空串/乱码/超短串得空集不 panic）。
- **假设的原分支是否真的不可达**：`TestX_DeletedAtoiBranchesAreUnreachable` 不读代码，而是**对校验正则求值**——七种会让 Atoi 版出问题的 period（`abcd-ef` / `20` / `2025-6` / `2025-13` / `2025-00` / `yyyy-mm` / `2025_12`）逐个走 CLI，全部在 **flag 校验阶段**就被拒（错误含「格式非法」且不含开库失败的痕迹）；反向再验一条合法 period 不被格式校验拦下，免得上面那组退化成「全拒」。
⇒ **删除正当**（`^\d{4}-(0[1-9]|1[0-2])$` 保证前四位与月份两位都是十进制数字，`Atoi` 不可能失败）。但**精确表述不是「行为等价」**：`"2025-6"` 在 Atoi 版会被当成 6 月而命中，在当前版得空集——两版在这个输入上行为不同，只是该输入被正则拦在 CLI 之外。discovery 的理由写的是「格式不对的 period 匹配不上任何行，行为与原来的 `return nil` 等价」，这对「Atoi 失败」的输入成立，对 `"2025-6"`（Atoi 成功但格式非规范）不成立。结论不变，理由需要这样限定。

## 报给 Leader 的一条观察（非缺陷；**已于判定后订正**）

`coverage_floor` 原落盘为**字符串** `"77"` 而不是数字 `77`。当时能工作是因为门禁用的是 bash 的 `[ "${TOTAL%.*}" -lt "$DEV_MIN" ]`，对 `"77"` 按整数处理（我实跑 `78 >= 77` 确认放行）。若将来有人把这行改成 jq 数值比较，字符串形态会静默失效。

**订正记录（2026-09-17T01:17:01Z）**：Leader 读到本条后请我顺手改（任务已 `verified`，owner_table 里这一格的合法写者是 `test-*`，我是当时唯一写得动的人）。经
`task TASK-009 update --json-field coverage_floor=77` 落盘，`jq '.coverage_floor|type'` 现读为 `number`、值仍为 `77`，`transitions.jsonl` 留有 `op:"update"` 审计行（`before {"type":"string","len":2}` → `after {"type":"number","value":77}`）。门禁比较式复跑仍为 `78 >= 77` ⇒ 放行。**纯类型订正，不改语义，本报告的 VERIFIED 判定不受影响。**

⚠️ **顺带修正一处事实**：Leader 请我订正时给的理由是「归档里 7 个先例全是数字，009 与全部先例不一致」。我扫了全部归档（`find .arcforge -name 'TASK-*.json'` 逐个取该字段与其 `type`），实际分布是 **16 个 number / 5 个 string**——字符串形态在归档里**有 4 个先例**：sprint-037 的 TASK-007（`"93"`）与 TASK-008（`"75"`）、sprint-041 的 TASK-007 与 TASK-010（均 `"75"`）。所以「与全部先例不一致」不成立；准确说法是**数字是多数形态（16/21）、字符串是有先例的少数形态**。订正本身仍然值得做（类型统一能让「将来改用 jq 数值比较」这条失效路径消失），但理由要换成这个。

## 验证者记录的实际行为（DoD 未规定，不判红）

- **`--period-type` 不改变推哪一行**（Leader 裁决 2/5）：`sheets.Row` 只有 `Year/Month/Cells`，过滤只能按年月。我实测 `TestX_PeriodFilterReachesPush` 确认 `--period 2025-12` 命中的是年月匹配的那一行，与 period-type 取值无关。该 flag 兑现的是「不许猜」。
- **两个包级测试缝**（`newSheetsClient` / `sheetsPush`）都在 `cmd/atlas` 包内，而写口守卫只扫 `internal/hestia` ⇒ 新增包级变量不触发守卫，本任务确未改 `store_test.go`（numstat 已证）。
- **`recover` 包住建 client 与 Push 两步**，比 DoD 要求（只兜 Push）更严。我用 `TestX_ProjectorRecoversAllPanicShapes` 的三种形态与 dev 的 client-panic 用例共同覆盖了两条路径。
- **`sheetsEntryFirstRow = 4` 与 sheets 包未导出的 `entryRowOffset` 是同一事实的两个副本**（discovery `degradations` 已申报）。只影响排版显示的月份，不影响写哪一格。当前无机制保证两者同步。
- 整个 `cmd/atlas` 包有两个文件不过 `gofmt -l`（`backtest_test.go`、`crisis_test.go`）。**我在 base 树 `3770e94` 上独立复跑，命中的正是同样这两个文件** ⇒ 与本任务无关，是既有状态，且它们不在 `writes` 里。

## 测试质量评审

- **他证而非自证**：flag 校验的顺序由「配置指向不存在目录时报的是哪一类错」他证，不是看代码里 `if` 的位置；投影装配由「Push 实际收到的 rows 与 opts」他证，不是看闭包里写了什么。
- **成对的反向断言**是这份测试里最值得称道的设计：`ValidatesFlagsBeforeOpeningStore` 配 `PropagatesConfigError`、`DryRunLine` 配 `ApplyDropsDryRunLine`、`ProjectorPushesWithApplyAndCreate` 配 `ProjectorNilWhenCredentialsEmpty`。每一对里单独一条都能被「什么都不做」的实现骗过，成对就不能。
- **断言精确**：`Options` 用整结构体 `Equal`、禁用那句用整串 `Equal`、rows 用切片 `Equal`。无空洞断言。
- **mock 合理**：两个包级测试缝注入的是真实调用路径上的函数值；端到端用例用真 `hestia.Ingest` 加 fake Fetcher，跨包读 `internal/hestia/testdata` 的真实语料而不复制（避免两份需要同步的语料）。
- 偏弱处见「诚实记录」①的那一条。

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-v009 477664a5651449490ddc602c090501bfd2ab9ead
cd ../wt-m2b-v009
GOTOOLCHAIN=local go test ./... -count=1                                    # 65 ok / 0 FAIL
GOTOOLCHAIN=local gofmt -l cmd/atlas/hestia_sheets.go cmd/atlas/hestia_sheets_test.go cmd/atlas/hestia.go   # 空
GOTOOLCHAIN=local go vet ./...                                              # exit 0，无输出
GOTOOLCHAIN=local go test ./cmd/atlas -count=1 -coverpkg=./cmd/atlas -coverprofile=/tmp/c.out \
  && GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | grep total:         # 78.0%
# 基线对照（须在另一棵树上跑，锚同样是全 sha）
git worktree add --detach ../wt-base 3770e9482129981cd7a0608e55fadd2c090203ed
cd ../wt-base && GOTOOLCHAIN=local go test ./cmd/atlas -count=1 -coverpkg=./cmd/atlas -coverprofile=/tmp/b.out \
  && GOTOOLCHAIN=local go tool cover -func=/tmp/b.out | grep total:         # 76.8%
```

---

# 第二轮：返工复验（QA 返工轮）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `b53708ff99d50db033ab44b2458ce75ba7e91e22`（= 新的 `verify_baseline.head`）；返工两个 commit `5447d05`（fix 主体）+ `b99ec04`（code-simplifier）
- discovery sha256: `2143eb1242322e31627681532826eb63d4a090525dc25c4aae33e38a60e856c6`（判定前现读一致 ⇒ **无漂移**）
- `assignment_epoch` 1 · `rework_count` 1 · 迁移带 `--expect-epoch 1`
- 范围核对: 返工净效果为 `hestia_sheets.go` 40/6、`hestia_sheets_test.go` 156/0、`hestia_test.go` 51/14、plist 16/0 ⇒ 逐个比对 `writes` 五项，**全在声明范围内，无越界**
- 🔴 **全程在钉死的 worktree 上取证**：`git worktree add --detach ../wt-m2b-r009 b53708ff…`。活 master 上 dev-m2b-c 正在改 `internal/hestia/sheets/`，`go test ./...` 会把在途改动一起编译进来，所以本轮所有数字都不是在 master 上跑的。

## 第二轮结论：**VERIFIED**（6 条 `fix_items` 全部达成）

## fix_items 覆盖矩阵

| # | 修复要求 | 我的验证方式与结果 | 判定 |
|---|---|---|---|
| [0] | plist 的 `EnvironmentVariables` 补代理键，参照同仓 `crisis-daily.plist` 形态 | 我没有读 XML 文本比对，而是用 `plutil -convert json` 把两份 plist 都解析出来**逐键比对**：`hestia-ingest` 与 `crisis-daily` 的 `EnvironmentVariables` **四个键同值、完全一致**（`http_proxy` / `https_proxy` 均 `http://127.0.0.1:7897`，`no_proxy` 为 `localhost,127.0.0.1`，`PATH` 同）。变异 **S1**（删 `http_proxy`）/ **S2**（改成别的端口）/ **S3**（删 `no_proxy`）**全部 KILLED**。⚠️ 仓库外那半截见下「三条事实状态」① | PASS |
| [1] | 守卫判据反转并改名，保留肯定式锚点与阳性对照 | **逐条比对改名前后两版**，不只数条数（数目相等也可能是换了更弱的断言）：原有的 `require.NotEmpty(keys, …)` 前置锚点与 `assert.Contains(keys, "PATH", …)` **原样保留**；阳性对照子测试（拿 `crisis-daily` 跑同一个解析器、并特别检查最易漏的 `no_proxy`）**整块保留**；否定式的 `NotContainsf(lower,"proxy")` 换成三条具名键的 `Containsf`；**另新增两条值级 `assert.Equal`**（旧版一条值级断言都没有）。⇒ **强度不降反升**。静态断言 8 条，与派验消息所数一致。⚠️ 一处偏离字面见下「关于 fix_items[1] 的偏离」 | PASS |
| [2] | cmd 侧 client 懒建复用 | 实现用 `sync.Once` + 闭包捕获 `client`/`build`。变异 **S4**（去掉复用、每次重建）KILLED by `TestSheetsProjectorBuildsClientOnce`；变异 **S5**（改成提前建）KILLED by `TestSheetsProjectorBuildsLazily`。⚠️ S5 在全量跑时只报一条 `TestSheetsProjectorRecoversClientPanic` —— 那是 panic 崩溃终止了测试二进制，**把真正的守卫遮住了**；我单跑 `BuildsLazily` 才确认它自己也会红 | PASS |
| [3] | 给 Sheets 调用加超时 | `sheetsCallTimeout = 2 * time.Minute`，**四处** `context.WithTimeout`（投影侧建 client 与 Push、CLI 侧同两步）。变异 **S6 / S7 / S8** 分别去掉前三处 ⇒ **全部 KILLED**。**S9（去掉 CLI 侧建 client 那处）SURVIVED** ⇒ 实现四处齐全，测试只守住三处，见下「一个存活变异」 | PASS |
| [4] | `TestSheetsPushDefaultsToDryRun` 补 `Greater(WillWrite,0)` 护栏 | **dev 没按字面做，改成了更好的方案，且 discovery 有明确交代**（`decisions` 里逐条写了理由）：那条 CLI 用例的库是 `t.TempDir()` 空库，直接加 `Greater(WillWrite,0)` 只会让它红，而它本身是有价值的**接线测试**。dev 的做法是原用例保留 + 加注释降级声明它不是 dry-run 的主守卫 + 加一条 `Contains(out,"将写 0 格")` 证明排版跑到了，另起 `TestPushSheetsDryRunWithRowsSendsOnlyGETs` 喂真行、从 `sheetLabels()` 同源生成齐全 35 列表头，在 `WillWrite>0` 的前提下要求仍然零写请求。**判为带理由的替代，不是没做** | PASS |
| [5] | 明细行钉死整行 | 三条 `require.Equal` 整行逐字钉死（`2025年` / `2026年` / `2020年` 各一条，覆盖三种 `Kind`）。变异 **S10**（明细行分隔符两空格→一空格）与 **S11**（`changeVerdict` 文案「将写」→「待写」）**均 KILLED** ⇒ 实测证明整行 `Equal` 确实比原来的 `Contains(got,"AF")` 强：后者在这两个变异下都不会红 | PASS |

## 独立复核：`b99ec04` 让断言数少了 1 条

派验消息要我形成自己的判断，所以我没用总数，用的是**集合差**——它直接指出消失的是哪一条。

**结果**：消失的唯一一条是 `cmd/atlas/hestia_test.go` 里的裸 `require.NoError(t, err)`（无文案那条），新增 0 条。

读两版 `plistEnvValues` 确认它是 `os.ReadFile(path)` 的错误检查，而那次 `os.ReadFile` 本身是冗余的。我用**三条实测**而不是推理来判它：

| 实测 | 结果 |
|---|---|
| `plutil -convert json -o - /nonexistent/no-such.plist` | 退出码 **1** ⇒ 新版的 `require.NoError(t, err, "plutil 转 JSON 失败")` 仍会捕获「文件读不到」 |
| 对真实文件同一命令 | 退出码 0，正常输出 |
| 往 stdin 喂一份含 `X` 键的 plist、同时给真实 path | 输出里**没有 `X`**，顶层键是真实文件的 ⇒ 旧版 `cmd.Stdin = bytes.NewReader(raw)` **确是死代码** |

⇒ **删的是冗余代码连同它的检查，覆盖面未缩**，不是弱化。结论与派验消息一致，但证据是实测。

**顺带记一个口径事实**：派验消息报 379→378，我算全包 `cmd/atlas` 是 748→747、两个改动文件是 374→373。三个口径的**差值都是 1**，集合差指出的是同一条。这也再次说明「断言总数零删减」这个判据在**有实际代码删除的 refactor 轮里会失效**——「删冗余代码带走它的检查」与「删一条守卫」在计数上完全同形，而集合差能一步指出是哪一条。

## 关于 fix_items[1] 的一处偏离（判为合理，且更好）

`fix_items[1]` 的字面要求是「改为：**断言 PBOC fetcher 仍用空 Transport**（性质的真正载体），并允许 plist 设代理键」。dev **没有**在 `cmd/atlas` 里加这条断言，而是在函数注释里指向 `internal/hestia` 的 `TestPBOCFetcherDoesNotProxyPBOC`。

我去核了那个测试是否真实存在、是否有效：

- 它**存在**（`internal/hestia/fetch_test.go:32`），当前 **PASS**。
- 它**比字面要求更强**：先 `t.Setenv("HTTP_PROXY", …)` / `t.Setenv("HTTPS_PROXY", …)` 把代理环境变量**设上**，再断言 PBOC fetcher 要么 `Proxy == nil`，要么 `Proxy(req)` 对 PBOC 的 URL 返回 nil —— **测行为而不是测结构**，实现换写法也不会假红。
- 它恰好正面回答了本次改动引入的那个问题：「plist 补了代理键之后，PBOC 抓取会不会被带偏」。

⇒ 判为合理偏离。在 `cmd/atlas` 里重复一条结构级断言既冗余、又是跨包窥探实现细节；性质的守卫留在它该在的包里更好。dev 在注释里写明了指向，可追溯。

## 🔴 一个存活变异：CLI 侧建 client 的 deadline 无守卫

**S9**：去掉 `pushSheets` 里 `newSheetsClient` 那一层 `context.WithTimeout` ⇒ **现有测试无一变红**。

成因是两条路径的守卫不对称：投影侧的 `TestSheetsProjectorSetsDeadline` 同时 stub 了 `newSheetsClient` 与 `sheetsPush`、**两步都查 `ctx.Deadline()`**；而 CLI 侧的 `TestPushSheetsSetsDeadline` **只 stub 了 `sheetsPush`**，建 client 那步没人看。

**这是缺守卫不是缺实现**：我写了 `TestZ_PushSheetsSetsDeadlineOnBothSteps`（照投影侧那条的形状，两步都查），它在原实现上**全绿**、在 S9 变异体上**是唯一变红的那条**。另加 `TestZ_BothPathsUseSameTimeoutOrder` 断言两条路径的剩余时间落在同一量级（判据不查常量字面量——那等于把测试和实现的同一个字面量对一遍，零鉴别力，这一点 dev 自己在 discovery 里也写对了）。

判**建议级、不构成 REJECT**：`fix_items[3]` 的要求是「给 Sheets 调用加超时」，实现四处齐全，缺的是第四处的守卫。夹具原文在 `scratchpad/test-m2b-b-TASK-009r-fixture.go.txt`，可直接移植。

## 三条事实状态（请勿在 final-report 里读成「已完成」）

① **生产 plist 未同步**（我实测确证，不是转述）：`~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist` 的 `EnvironmentVariables` **只有 `PATH` 一个键**，而源树那份已有四个键，`diff -q` 不一致。⇒ **源树已改、生产未同步**。这是仓库外的人执行项，不算在 dev 头上，但在它被执行之前，launchd 下的 Sheets 投影**仍然会恒失败，且按 C8 是静默的**——也就是说 CRITICAL-3 所描述的生产故障**此刻依然存在**。

② **一处陈旧引用**：`deploy/launchd/com.newthinker.atlas.hestia-ingest.plist:40` 的注释仍写着「守卫：`cmd/atlas/hestia_test.go` 的 **`TestHestiaPlistSetsNoProxyKeys`**」，而那个测试本轮已改名为 `TestHestiaPlistSetsProxyKeysForSheets`。这行就在 dev 本轮新增代理键的正上方几行，属于改名时漏改的下游引用。按名字去找的人会扑空。

③ **`fix_items[4]` 的替代方案有交代**：discovery 的 `decisions` 里逐条写明了「没有按字面加 `Greater(WillWrite,0)`」以及为什么。按派验消息的要求，我确认「有理由不做」而非「没做」。

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-r009 b53708ff99d50db033ab44b2458ce75ba7e91e22
cd ../wt-m2b-r009
GOTOOLCHAIN=local go test ./... -count=1                       # 65 ok / 0 FAIL
GOTOOLCHAIN=local go test ./cmd/atlas -count=1 -coverpkg=./cmd/atlas -coverprofile=/tmp/c.out \
  && GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | grep total:    # 78.1%（floor 77）
# plist 逐键比对（不要读 XML 文本）
plutil -convert json -o - deploy/launchd/com.newthinker.atlas.hestia-ingest.plist
plutil -convert json -o - deploy/launchd/com.newthinker.atlas.crisis-daily.plist
# 生产那份（仓库外，确认未同步）
diff -q deploy/launchd/com.newthinker.atlas.hestia-ingest.plist \
        ~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist
```
