# TASK-010 验证报告 — ingest 接线：入库后投影 Sheets（C8 只记日志 / C9 nil 不投影）

> **本文件含三轮**：**第一轮首验**（00:19Z）、**第二轮返工复验**（03:30Z）、
> **第三轮返工复验**（CRITICAL-4，见文末「# 第三轮」）。三轮结论都是 VERIFIED，
> **第三轮才是当前生效的判定**。

## 第一轮：首验（2026-09-17 00:19:31Z）

- 验证者: test-m2b-b　　时间: 2026-09-17（接替 test-m2b-a，前任因账号额度耗尽停机）
- 判定对象: master @ `3770e9482129981cd7a0608e55fadd2c090203ed`（= `verify_baseline.head`）；交付 commit `3bf5372c59416e96bbe612310de56b3d3dcf17af`；base `e0f4f8b4fdca4575d7b1bd1010126e85c69d8697`
- discovery sha256: `7d454944e61a8112ad9c6c63d413e22186d489c3e3dfd429db4549450b493ecd`（= `verify_baseline.discovery_sha256`，开工与判定前各现读一次，均一致 ⇒ **无漂移，无需 `--ack-drift`**）
- provenance：代码由 **dev-m2b-a** 编写并提交（含它跑过的 code-simplifier），该实例额度耗尽后 Leader 按 AD-21 改派（`assignment_epoch` 1→2），**dev-m2b-c 接手未改一行代码**、未再跑 simplifier。验证者用**内容判据**核实这一点：交付三文件在 commit `3bf5372`、baseline 树 `3770e948`、验证 worktree 三处 sha256 逐字节一致（`20a8db13…` / `a44df783…` / `722efd5d…`），且与 discovery 声明的值一致 ⇒ 接手期间实现与测试都没被动过。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v010` @ `3770e9482129981cd7a0608e55fadd2c090203ed`（全 sha 锚，非 `HEAD`/分支名）
- 范围核对: `git diff e0f4f8b..3bf5372 --numstat` **恰为** `writes` 三文件（config.go 16/6、ingest.go 26/0、ingest_test.go 119/0，三行即全部输出）；`git show --numstat 3bf5372` 同值 ⇒ 无越界申报、go.mod/go.sum 无变化、`store_test.go` 未被触碰。

## 结论：**VERIFIED**（5/5 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `IngestDeps.ProjectSheets func(context.Context, []sheets.Row) error`（形状照 `Notify`，nil = 不投影）；`config.go` 的 `HestiaSheets` 含 `credentials_file` / `spreadsheet_id` | test | 形状：验证者 `TestV_ProjectSheetsFieldShape` 用**具名函数类型变量赋值**他证签名（形状不符本文件编译不过），并断 `IngestDeps` 零值的该字段为 nil。配置：dev `TestConfigHestiaSheetsSection` 验填值那支；验证者 `TestV_ConfigHestiaSheetsAbsentIsZero` 补两条缺口——**整段缺失**与**两键显式留空**都必须 `LoadConfig` 无错且得零值（C9 在配置侧的前提）。ingest.go 字段声明、config.go 两个 mapstructure tag 逐字核对 | PASS |
| functional[1] | 调用顺序 `order == []string{"contract","sheets"}`；投影用 `Options{Apply:true, CreateSheets:true}` | test | **前半句**（本任务范围内）：dev `TestIngestSheetsRunsAfterContract`；验证者 `TestV_OrderContractThenSheets` 用**双重内容判据**（回调触发时 `pending/*.json` 已落盘 **∧** stdout 已含 ` contract → `），两期候选 ⇒ `order == [contract, sheets, contract, sheets]`，且 `ProjectSheets` 收到的行数随库增长 1→2、`{2020:6, 2025:12}` 与两期候选精确一致。变异 M7（投影块挪到 `WriteContract` 之前）⇒ 该条红。**后半句**：Leader 在本任务 description 里明文裁决「需求 Step 3 的 `runHestiaIngest` 接线与 009 撞 `cmd/atlas/hestia.go`，**挪给 009**，本任务不碰 `cmd/atlas/`」。验证者核实**接收方载体确实存在且是强载体**：`TASK-009.done_criteria.functional[2]` 逐字写着「内部调 `BuildSheetRows`+`sheets.Push{Apply:true,CreateSheets:true}`」（009 当前 `pending`）⇒ 义务已转移，非本任务缺口 | PASS |
| boundary[0] | **C9**：`ProjectSheets==nil` ⇒ 不尝试投影、不报错、不打 WARN 以上日志、`Ingest` 返回 `nil` | test | dev `TestIngestSheetsDisabledWhenNil`；验证者 `TestV_C9NilIsSilent`：两期候选下 stdout 中**以 `sheets` 开头的行数为 0**、全文不含 `sheets` 字样、`Ingest` 返回 nil、两行 run 都是 `ingested`、且 `pending/` 里**契约照常 2 份**（证明跳过的只是投影，不是整段逻辑）。「不打 WARN 以上日志」是**结构性成立**：`grep -rn '"log' --include='*.go' internal/hestia/` 去掉测试文件后命中 **0** ⇒ 该包非测试代码里没有任何 `log` / `slog` 导入，投影块只有 `fmt.Fprintf(d.Out, …)` | PASS |
| error_handling[0] | **C8 两支都只打印不返回**：组装失败 ⇒ `sheets: 组装失败（不影响入库）: %v`；投影失败 ⇒ `sheets: 投影失败（不影响入库）: %v`（判据七拿这行判，一字不能改）。两支都：返回 `nil`、`runs[0].Outcome=="ingested"`、不发 P1 | test | dev 两条（`…FailureDoesNotChangeOutcome` / `…BuildFailureDoesNotChangeOutcome`）。验证者**独立注入两支各验一遍**，且判据比 dev 更严——dev 用带 `\n` 的 `require.Contains`，验证者按行 split 后取**以 `sheets` 开头的整行**做 `require.Equal`（逐字节相等），Contains 放过「这一行前面还粘着别的字」这类改动。`TestV_C8ProjectFailureOnlyPrints`：两期各打一行、**不多不少**、返回 nil、两行 run 皆 `ingested`、无 `[P1]`、契约照常 2 份。`TestV_C8BuildFailureOnlyPrints`：经包内 seam 注入组装失败，`ProjectSheets` **调用次数为 0**。`TestV_C8BuildFailureShortCircuitsProject`：两支都会失败时只打第一条（`else if` 语义）。`TestV_C8ErrorTextNotReformatted`：错误文本含 `%%`/`%s`/`%d` 时原样进 `%v`、不被二次格式化。实现侧逐行核：投影块两处均为 `fmt.Fprintf` 后继续往下走，**无 `return`、无 `errors.Join`、不改 `out.Verdict`、不碰 `fail(...)`**。变异 M1/M2（任一支改成 `return fail(...)`）、M3/M4/M5（改一字 / 全角括号改半角 / 删冒号后空格）、M9（丢 `%v` 参数）、M12（失败时发 `[P1]`）**全部 KILLED** | PASS |
| non_functional[0] | `go test ./internal/hestia/ -count=1` 全绿、`gofmt`/`go vet` 零输出；`ProjectSheets` 非 FuncDecl 不触发守卫、不改 `store_test.go`；提交前跑 code-simplifier | manual | **交付树净态**（验证者夹具移出后采）：`gofmt -l internal/hestia/` 空输出；`go vet ./internal/hestia/...` exit 0 且无输出；`go test ./... -count=1` exit 0、**65 个 ok、0 个 FAIL**。**含验证者夹具**：`go test ./internal/hestia/ -count=1 -v` exit 0，顶层 **854 PASS / 0 FAIL / 1 SKIP**（`TestMagnitudeRangesCoverEveryFieldWhenCalibrated`，既有 skip，与本任务无关）；两把独立的尺互验并自洽：`^--- PASS:` 计 854，`^=== RUN <名字无斜杠>` 计 855，差 1 恰为那条 SKIP，`855 == 854+0+1` 成立。覆盖率 `internal/hestia` 96.6%、`internal/hestia/sheets` 89.1%。**守卫未变**：python 解析 `store_test.go` 的两条守卫，AST 版 `TestPackageExposesNoWriteFunctions` 的 `want` 仍 **51** 项、reflect 版 `TestStoreExposesNoWriteMethods` 仍 **15** 项，`ProjectSheets`（结构体字段）与 `buildSheetRows`（未导出 var）均不在其中；`store_test.go` 不在本任务 diff 里。**code-simplifier**：由原作者 dev-m2b-a 跑过，留痕只有 sha256、前版原文不可回捞（git 里仅 `3bf5372` 一个 commit 触碰这三文件），故以**合入版逐行审读**代替 diff 审查——验证者读完 161 行新增，无冗余抽象、无未使用符号、无留下的调试代码；接手者未再跑 simplifier 系 Leader 指示，已在 discovery `decisions[1]` 与本报告 provenance 申报 | PASS |

## 变异测试

12 个变异，全部作用在**隔离 worktree 副本**的 `internal/hestia/ingest.go` 上，主工作区一个字节不碰。
每个变异的流水：落盘 → `gofmt -e` 语法闸 → `go vet ./internal/hestia/` → **打印变异 diff 供逐字核对**（语义闸）→ `go test ./internal/hestia/ -count=1 -timeout 300s` → `git checkout` 还原 → 校验 worktree 与**主工作区**两处 sha256。
有效性闸：编译失败 / `go test` 非零退出但零条 `--- FAIL` ⇒ 判定该变异体无效并**立即早退**，不打印 KILLED。全部 12 轮 `go vet` 均 exit 0 无输出；收尾时 worktree 与主工作区 `ingest.go` sha256 均回到 `20a8db13…`。

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| M1 C8 投影失败支改成 `return fail(...)` | KILLED | dev `…FailureDoesNotChangeOutcome` + 验证者 V1 / V6 |
| M2 C8 组装失败支改成 `return fail(...)` | KILLED | dev `…BuildFailureDoesNotChangeOutcome` + 验证者 V2 / V3 |
| M3 文案改一个字（投影失败 → 投影错误） | KILLED | dev 1 条 + 验证者 V1 / V6 |
| M4 文案全角括号 → 半角括号 | KILLED | dev 1 条 + 验证者 V1 / V6 |
| M5 文案冒号后空格删掉 | KILLED | dev 1 条 + 验证者 V1 / V6 |
| M6 去掉 `d.ProjectSheets != nil` 判断 | KILLED | 见下「诚实记录」①——**靠 nil 函数调用 panic**，不是靠 C9 断言 |
| M7 投影块挪到 `WriteContract` 之前（顺序颠倒） | KILLED | 验证者 V5（dev 的顺序用例未红，见「诚实记录」②） |
| M8 `else if` → `if`（组装失败后仍尝试投影） | KILLED | dev `…BuildFailureDoesNotChangeOutcome` + 验证者 V2 / V3 |
| M9 `%v` 参数换成固定串（丢失错误详情） | KILLED | dev 1 条 + 验证者 V1 / V6 |
| M10 投影块挪出 New/Revision 块（Duplicate 也投影） | KILLED | 验证者 V7（**首版存活，暴露了验证者自己的空断言**，见「诚实记录」③） |
| M11 组装挪到 nil 判断之外（C9 时仍调 `BuildSheetRows`） | 🔴 SURVIVED | 逐条对照 C9 后判为**语义内不可区分**，非缺口，见下 |
| M12 投影失败时发 `[P1]` 告警 | KILLED | dev `…FailureDoesNotChangeOutcome` + 验证者 V1 |

### M11 存活的判定（不构成 reject）

M11 把 `rows, berr := buildSheetRows(ctx, d.Store)` 挪到 `if d.ProjectSheets != nil` **之外**，于是 C9 场景下仍会做一次组装（含一轮 DB 查询），但打印分支仍在 nil 判断内。逐条对 C9 的四项要求求值：**不尝试投影**（`ProjectSheets` 仍不被调用）✓、**不报错** ✓、**不打日志**（打印语句仍在判断内）✓、**`Ingest` 返回 nil** ✓ ⇒ 它在 DoD 语义下与原实现不可区分，属合理存活，不是断言缺口。

附带发现：dev 的 discovery `key_findings[2]` 声称「nil 时**连 `buildSheetRows` 都不调用**」——该声称经代码核对为真（当前实现确实如此），但**没有任何测试守卫它**。这是 M11 能存活的直接原因。建议级（不影响本次判定）：若要把这条性质固化，可在 C9 用例里用同一个包内 seam 给 `buildSheetRows` 挂计数器并断言为 0。

## 诚实记录：三处「差点立不住」的地方

① **M6 是靠 panic 杀的，不是靠 C9 断言。** 去掉 nil 判断后 `d.ProjectSheets(ctx, rows)` 调用 nil 函数必然 panic，panic 会终止整个测试二进制，于是只留下一条 `--- FAIL`（崩溃时正在跑的那条无关测试 `TestIngestReportsStopReasonEvenWithCandidates`）。这仍是有效的 KILLED（变异不可能静默通过），但它说明「C9 被 nil 判断守着」这件事的**证据来源是语言语义而非断言**，值得写明，而不是让 KILLED 这个词把它盖过去。

② **M7 只被验证者的顺序用例杀。** dev 的 `TestIngestSheetsRunsAfterContract` 判据是「回调触发时 `pending/*.json` 非空」；M7 把投影挪到 `WriteContract` 之前、但仍在 `WriteHistory` **之后**，而侧车 `*.history.json` 同样落在 `pending/` 下 ⇒ dev 的 glob 仍非空、该用例不红。验证者 V5 因为加了第二重判据（stdout 里已出现 ` contract → ` 那一行）才把它钉住。建议级：dev 的那条 glob 可收窄到排除 `.history.json`。

③ **验证者自己犯的三个错，都已订正后统一重采。** 一是首版夹具把 `pending/*.json` 的**总数**当成契约数（每期落契约 + M3 侧车共 2 个文件，2 期是 4 不是 2）；二是 `TestV_DuplicateDoesNotProject` 首版用「同一批候选裸跑第二次」冒充 Duplicate 路径，实际被 `ingestOne` 的 `article_id` 幂等挡在 `Save` 之前，**那条断言是空的**——是变异 M10 存活把它暴露出来的，改用 `Force: true` 穿透两层幂等、并先断言输出里真的出现 `Duplicate` 之后，M10 变成 KILLED；三是变异 harness 首版用 `ln.split()[-1]` 取 FAIL 的测试名，取到的其实是耗时 `(0.01s)`，且没有编译失败闸（`--- FAIL` 为零时会把编译不过报成 SURVIVED）。**本报告里的全部数字都是这三处订正之后统一重采的**，不是分批拼起来的。

## 验证者记录的实际行为（DoD 未规定，不判红）

- **`ProjectSheets` panic 会穿透 `Ingest`**，不被 recover（`TestV_ProjectPanicPropagates` 记录）。装配方 TASK-009 若用第三方 client，panic 会打断整轮 ingest——与 C8「投影失败不阻断」的意图不完全一致，但需求未要求，留给 009 与 QA 判断。
- **Duplicate / OutOfOrder 期次不投影**：投影块在 `out.Table == TableObservations && (New || Revision)` 块内，与契约同一个门。`TestV_DuplicateDoesNotProject` 已钉住（Force 重跑两期 Duplicate ⇒ 投影调用次数不增、无 `contract →`、无 `sheets` 行）。
- **每条候选各投影一次**（不是每轮 ingest 一次）：两期候选 ⇒ 两次调用，第二次收到的行数为 2，即 `BuildSheetRows` 每次重新组装**全库**。多期回填时这会是 O(n²) 次组装，当前规模（每月一期）无碍，记录备查。
- 组装成功且投影成功时 **stdout 零投影输出**（安静路径），V5 已断言。

## 测试质量评审

- **断言精确、无空洞**：C8 两支是整行 `require.Equal`（逐字节），不是 `assert.True` 之类；C9 是「行数为 0」+「全文不含」双判据；顺序是切片整体相等。
- **mock 使用合理**：`ProjectSheets` 本就是函数字段，注入即真实调用路径；`buildSheetRows` 是包内未导出 `var` seam，用 `t.Cleanup` 还原，未导出且非 FuncDecl ⇒ 不扩大包导出面、不触发写口守卫。没有为了绕过难点而 mock 掉被测逻辑。
- **dev 的 DoD↔测试映射注释**逐条对得上，验证者逐条复核无错标。
- 唯一偏弱处是 dev 的 C8 用 `Contains` 而非行级相等、顺序用例的 glob 未排除侧车（上文②），均已由验证者夹具补强，且变异证明两处在当前实现下都杀得掉对应变异。

## 附录：验证者夹具关键片段（`zz_verify_m2b_b_test.go`，只存在于 worktree wt-m2b-v010，不进交付）

```go
// vSheetLines 取出 stdout 里**以 "sheets" 开头**的整行（已去掉尾部换行）。
// 用行首而非 Contains：能同时钉住文案与「这一行前面不许有别的字」。
func vSheetLines(out string) []string {
	var got []string
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "sheets") {
			got = append(got, ln)
		}
	}
	return got
}

// V1 —— C8 支一：ProjectSheets 失败。独立注入、行级精确相等、两期各一行。
func TestV_C8ProjectFailureOnlyPrints(t *testing.T) {
	deps, out, snd := vDeps(t) // 两条候选：2025 年报 + 2020 上半年报
	perr := errors.New(`Post "https://sheets.googleapis.com/v4": dial tcp 127.0.0.1:7890: connect: connection refused`)
	calls := 0
	deps.ProjectSheets = func(context.Context, []sheets.Row) error { calls++; return perr }

	require.NoError(t, Ingest(context.Background(), deps), "C8：投影失败不得让 Ingest 返回错误")
	vRequireC8Invariants(t, deps.Store, snd, 2) // 每行 run 都 ingested、无 [P1]
	require.Equal(t, 2, calls, "两期入库 ⇒ 每期投影一次")

	want := fmt.Sprintf("sheets: 投影失败（不影响入库）: %v", perr)
	lines := vSheetLines(out.String())
	require.Len(t, lines, 2, "两期各打一行，不多不少")
	for _, ln := range lines {
		require.Equal(t, want, ln, "整行逐字节相等（不是 Contains）")
	}
	require.Len(t, vContracts(t, deps.Cfg.Queue.Dir), 2, "两期契约照常落盘")
}

// V5 —— 顺序：双重内容判据（契约文件已落盘 ∧ stdout 已打出 contract 行）。
func TestV_OrderContractThenSheets(t *testing.T) {
	deps, out, _ := vDeps(t)
	var order []string
	var rowsPerCall [][]sheets.Row
	deps.ProjectSheets = func(_ context.Context, rows []sheets.Row) error {
		m, _ := filepath.Glob(filepath.Join(deps.Cfg.Queue.Dir, "pending", "*.json"))
		if len(m) > 0 && strings.Contains(out.String(), " contract → ") {
			order = append(order, "contract")
		}
		order = append(order, "sheets")
		rowsPerCall = append(rowsPerCall, rows)
		return nil
	}

	require.NoError(t, Ingest(context.Background(), deps))
	require.Equal(t, []string{"contract", "sheets", "contract", "sheets"}, order)
	require.Empty(t, vSheetLines(out.String())) // 成功路径不打印
	require.Len(t, rowsPerCall[0], 1)           // 行随库增长
	require.Len(t, rowsPerCall[1], 2)
	years := map[int]int{}
	for _, r := range rowsPerCall[1] {
		years[r.Year] = r.Month
	}
	require.Equal(t, map[int]int{2020: 6, 2025: 12}, years)
}

// V7 —— Duplicate 不投影。首版用「裸跑第二次」是空断言（被 article_id 幂等挡在 Save 之前），
// 由变异 M10 存活暴露；改用 Force 穿透两层幂等，并先断言真的走到了 Save。
func TestV_DuplicateDoesNotProject(t *testing.T) {
	deps, out, _ := vDeps(t)
	calls := 0
	deps.ProjectSheets = func(context.Context, []sheets.Row) error { calls++; return nil }
	require.NoError(t, Ingest(context.Background(), deps))
	require.Equal(t, 2, calls, "两期 New ⇒ 投影两次")

	out.Reset()
	dup := deps
	dup.Force = true
	require.NoError(t, Ingest(context.Background(), dup))
	require.Contains(t, out.String(), bitemporal.Duplicate.String(),
		"前置：必须真走到 Save 才谈得上 Duplicate 分支（否则这条断言是空的）")
	require.NotContains(t, out.String(), " contract → ", "Duplicate 不写契约")
	require.Equal(t, 2, calls, "Duplicate 不写契约，也不投影")
	require.Empty(t, vSheetLines(out.String()))
}
```

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-v010 3770e9482129981cd7a0608e55fadd2c090203ed
cd ../wt-m2b-v010
GOTOOLCHAIN=local go test ./internal/hestia/... -count=1 -cover   # 96.6% / 89.1%
GOTOOLCHAIN=local go test ./... -count=1                          # 65 ok / 0 FAIL
GOTOOLCHAIN=local gofmt -l internal/hestia/                       # 空
GOTOOLCHAIN=local go vet ./internal/hestia/...                    # exit 0，无输出
```

---

# 第二轮：返工复验（QA REJECT 后第 1 轮）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `c4bbb39e0818227e9cd7bf2e9f1705cf56b5472a`（= 新的 `verify_baseline.head`）；两个交付 commit `148e7ff`（fix 主体）+ `f3524df`（code-simplifier）
- discovery sha256: `5350a88f1d24a62c1f11e7418540ec0b250e0125cfa6d6c865739478367681ed`（= `verify_baseline.discovery_sha256`，开工与判定前各现读一次，均一致 ⇒ **无漂移**）
- `assignment_epoch` 2 · `rework_count` 1 · 迁移带 `--expect-epoch 2`
- 范围核对: 返工整体 `git diff --numstat d424352..c4bbb39` 为 4 个文件（ingest.go 25/2、ingest_test.go 94/1、sheets_project.go 34/5、sheets_project_test.go 50/5），逐个比对 `writes` 五项 ⇒ **全在声明范围内，无越界**（`config.go` 本轮未动）。越界申报已由 dev 在 `dev_done` 之前把 `sheets_project.go` / `sheets_project_test.go` 补进 `writes`。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-r010` @ `c4bbb39e0818227e9cd7bf2e9f1705cf56b5472a`

## 第二轮结论：**VERIFIED**（5 条 `fix_items` 全部达成 · 无新问题）

## fix_items 覆盖矩阵

| # | 修复要求 | 我的验证方式与结果 | 判定 |
|---|---|---|---|
| [0][1] | recover 上移到 ingest 侧，**组装与投影两步都经它**；配套注入必 panic 的 `buildSheetRows` 替身 | 实现新增 `recoverPanic(f func() error) (err error)`，`ingest.go` 的组装与投影两步各包一层。三个变异全部 KILLED：**R1** 把 `recover` 改成吞掉（`_ = recover()`）⇒ 两条 panic 用例同时红；**R2** 组装那步去掉包装 ⇒ `TestIngestSheetsBuildPanicDoesNotBreakIngest` 红；**R3** 投影那步去掉包装 ⇒ `TestIngestSheetsProjectPanicDoesNotBreakIngest` 红。**「裸闭包用例的红绿只取决于 ingest 自己」我用包边界证明**，比逐个拿掉 recover 更硬：`internal/hestia` 的测试文件里 `cmd/atlas` 出现 **0** 次，`go list -deps ./internal/hestia` 命中 **0** ⇒ cmd 侧那层 recover **在结构上不可能进入调用链**；再加 R3 能杀掉它，两条合起来证明它测的确实是 ingest 自己这一层 | PASS |
| [2] | `sheets_project.go` 定长切片 `Period[:4]` / `[5:7]` 加读侧防御 | dev 把「加防御」做成了**报错返回**而非「取不出来当 0」，理由是 `Year=0` ⇒ `tabName` 得 `"0年"` ⇒ 那张表必然缺，而 ingest 固定 `Apply+CreateSheets` ⇒ **会真的去建一张叫「0年」的工作表**。我核实这条推理链成立（`tabName` 的 `%d年` 格式、ingest 侧 `Options{Apply:true,CreateSheets:true}` 均在首验时核过），**判为加重而非偏离**。变异 **R4/R5**（形态错 / 数字错时折 0 不报错）均 KILLED。⚠️ 但另有两条性质无守卫，见下「两个存活变异」 | PASS |
| [3] | 补 M11 守卫：`ProjectSheets==nil` 时不调 `buildSheetRows` | 变异 **R8**（把整个组装块挪到 nil 判断之外）KILLED，由 `TestIngestSheetsDisabledDoesNotBuildRows` 杀。⚠️ 我首版 R8 锚点定错（只挪了 `var rows` 声明，`buildSheetRows` 调用仍在 if 内 ⇒ 等价变异，当时报 SURVIVED），订正后才是真变异 —— 见下「诚实记录」② | PASS |
| [4] | 顺序用例的 glob 排除 `.history.json` | **两条独立证据**：变异 **R9**（投影块挪到写契约之前，即首验时逃掉的那个 M7）⇒ **KILLED，且是被 dev 自己的 `TestIngestSheetsRunsAfterContract` 杀的** —— 首验时这个变异只有我的夹具杀得掉，现在交付测试集自己就守住了。变异 **R10**（让 `WriteHistory` 不再写出侧车，模拟「夹具不再产出侧车」）⇒ **KILLED 且命中 `TestIngestSheetsRunsAfterContract`** —— 这直接回答了「那两条常驻断言能不能响」：**能响**。dev 不只修了 glob，还给修复本身加了失效告警，而告警本身经实测有效 | PASS |
| — | **无新问题**（返工不得削弱既有守卫） | ① **断言集合比对**（不是数总数）：返工前后两版逐条做多重集合差，**消失 0 条**、新增 18 条 ⇒ 「原有断言一条没动」独立证实；simplifier 前后断言集合**完全相同**（零增零减）。② **回归变异**：把首验的 12 个变异在返工树上重跑（**已移出我的夹具，只用交付测试集**），**9 KILLED**，唯一存活的 M10 与首验时状态一致（首验时它也只被我的夹具杀）⇒ 非回退。③ 净态：全仓 **65 ok / 0 FAIL** exit 0、4 个改动文件 `gofmt -l` 空、`go vet` exit 0、覆盖率 **96.6% / 89.1%** 与首验持平 | PASS |

## 返工专项变异（10 个，隔离 worktree，主工作区 sha 每轮校验未变）

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| R1 `recoverPanic` 吞掉 panic | KILLED | Build/Project 两条 panic 用例 |
| R2 组装那步去掉 `recoverPanic` | KILLED | `TestIngestSheetsBuildPanicDoesNotBreakIngest` |
| R3 投影那步去掉 `recoverPanic` | KILLED | `TestIngestSheetsProjectPanicDoesNotBreakIngest` |
| R4 `periodYearMonth` 形态错时折 0 | KILLED | `TestBuildRowRejectsMalformedPeriod` |
| R5 `periodYearMonth` 数字错时折 0 | KILLED | 同上 |
| R6 去掉月份 1–12 范围检查 | 🔴 SURVIVED | 无（见下） |
| R7 `assembleRows` 忽略 `buildRow` 的 error | 🔴 SURVIVED | 无（见下） |
| R8 整个组装块挪到 nil 判断之外 | KILLED | `TestIngestSheetsDisabledDoesNotBuildRows` |
| R9 投影块挪到写契约之前（= 首验的 M7） | KILLED | `TestIngestSheetsRunsAfterContract` |
| R10 侧车不再写出（模拟夹具变化） | KILLED | 7 条，含 `TestIngestSheetsRunsAfterContract` |

## 🔴 两个存活变异：都是「缺守卫」，不是「缺陷」

这个区分不能靠读代码下结论，所以我写了四条夹具测试（`TestY_*`，只存在于验证 worktree）来把它钉死：**在原实现上全绿 ⇒ 实现是对的；在变异体上各自变红 ⇒ 缺的是守卫**。两边都实跑过。

**R6 —— 月份 1–12 范围检查无人守。** dev 的 `TestBuildRowRejectsMalformedPeriod` 四个子例是 `2025` / `2025-` / `""` / `2025-XX`，**没有一个是月份越界**。影响不是理论上的：`sheets.Row` 的行号是 `Month + entryRowOffset(3)`，而录入区只有第 4–15 行（`client.go` 的 `entryFirstRow=4` / `entryLastRow=15`）⇒ `Month=13` 会写到**第 16 行（录入区之外）**，`Month=0` 会写到**第 3 行（表头）**。我的 `TestY_BuildRowRejectsOutOfRangeMonth` 用 `2025-13` / `2025-00` / `2025-99` 三例 + 两端合法值的反向断言，能杀掉 R6。

**R7 —— `fix_items[2]` 的核心意图「整批停下」无人守。** 这条比 R6 更实质。fix_items[2] 的原话是「宁可整批停下，也不要静默产出一行垃圾」，而「整批停下」这件事发生在 `assembleRows` 里那三行错误传播上；dev 的两条新测试**只直接调 `buildRow`**，验到「它返回 error」为止。把 `row, err := buildRow(obs); if err != nil { return nil, err }` 改成 `row, _ := buildRow(obs)` 之后，**现有测试无一变红**——也就是说「返回了 error」有人守，「这个 error 会让整批停下」没人守。我的 `TestY_AssembleRowsStopsOnMalformedPeriod` 从 `assembleRows` 入口打（与同文件既有的 `TestBuildSheetRowsErrorsWhenCurrentMissing` 同形），断言整批报错、错误文本点名那一期、且返回 `nil` 不留半成品行。

**两条都判建议级、不构成 reject**：`fix_items[2]` 的字面要求是「加读侧防御」，dev 加了且比要求更强，实现经我实测正确。缺的是覆盖面。**建议把这两条夹具测试移植进交付测试集**（原文在 scratchpad 的 `zzr010.bak`）。

## 返工带来的一处改善（值得单独记）

第一轮报告里我记过：「**M6 是靠 nil 函数调用 panic 崩溃杀的，不是靠 C9 断言**，证据来源是语言语义而非断言」。

返工加上 `recoverPanic` 之后，nil 调用的 panic 不再让测试二进制崩溃 ⇒ **断言得以执行**。本轮回归实测 M6 被 `TestIngestSheetsDisabledWhenNil` 与 `TestIngestSheetsDisabledDoesNotBuildRows` **两条断言**杀掉。守卫从「语言语义」升级成了「真断言」——这是修 WARNING-1 顺带修掉的一个我在首验时只能记录、无法要求的弱点。

## 诚实记录

① **我的 R8 首版锚点定错，产出了一个等价变异。** 我想验「能力禁用时仍调 `buildSheetRows`」，写的锚点却只把 `var rows []sheets.Row` 这行声明挪到 if 外，而 `recoverPanic(...)` 调用仍在 if 内 ⇒ 语义没变，自然 SURVIVED。我没有就此判「M11 守卫失效」，而是**先把变异体打印出来逐行看**，发现是自己的锚点问题，订正为挪整个组装块后 KILLED。教训与首验那次「`HasSuffix(":batchUpdate")` 把数据写请求也算进来」同形：**变异存活的第一嫌疑人是变异本身，不是被测代码**。

② **simplifier 第 4 次回复不实，但这次它改对了一件事。** 它只回了一个词 `No.`，而两个测试文件 sha 都变了。我核了它的 diff：一处是把「收集路径列表 + 事后循环分类」改成「就地累加两个计数器」（行为等价，两条 `require.Positive` 断言原样保留）；另一处是**把 C4 的文档注释从 `mustBuildRow` 头上挪回 `TestBuildRowOmitsAbsentFields` 头上**——那是 dev 插入 helper 时插错了位置。我用内容判据核「一字未改」：同一个 4 行注释常量在 `148e7ff` 与 `c4bbb39` 两版里**各出现 1 次、sha256 相同**，差别只在它后面跟的第一个 `func` 是谁（`mustBuildRow` → `TestBuildRowOmitsAbsentFields`）⇒ **纯移动**。

③ **断言计数的口径差**：派验消息报 simplifier 前后 `require`/`assert` 均 **420**，我算得 **418**。核后确认是两把尺——420 是「行内出现 `require.`/`assert.` 即计」（包含口径），418 是「行首是断言语句」（语句口径），差 2 恰为注释里提到断言名的两行。**两把尺都没错，且两种口径下 simplifier 前后都不变**，结论一致。我另外用的集合差比对比两者都强：它能发现「删一条加一条」这种总数不变的改动，而本次结果是消失 0 条。

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-r010 c4bbb39e0818227e9cd7bf2e9f1705cf56b5472a
cd ../wt-m2b-r010
GOTOOLCHAIN=local go test ./internal/hestia/... -count=1 -cover   # 96.6% / 89.1%
GOTOOLCHAIN=local go test ./... -count=1                          # 65 ok / 0 FAIL
GOTOOLCHAIN=local gofmt -l internal/hestia/ingest.go internal/hestia/ingest_test.go \
  internal/hestia/sheets_project.go internal/hestia/sheets_project_test.go   # 空
# 返工新增的 4 条 + 改判据的 1 条
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -v -run \
 'TestIngestSheetsBuildPanicDoesNotBreakIngest|TestIngestSheetsProjectPanicDoesNotBreakIngest|TestIngestSheetsDisabledDoesNotBuildRows|TestBuildRowRejectsMalformedPeriod|TestBuildRowAcceptsWellFormedPeriod|TestIngestSheetsRunsAfterContract'
```

## 判定落盘时 HEAD 已前进：一次额外的复核

转 `verified` 时写通道打出 INFO：`HEAD 已从 c4bbb39e0818 前进到 15eee52b664b，但声明范围内无变更，判定对象未漂移`。我没有把这句话当结论，自己核了一遍：

- 中间两个 commit（`5af1701` + `15eee52`）是 **TASK-006 的 QA 返工**，只碰 `internal/hestia/sheets/` 下的 5 个文件（client.go / client_test.go / diff.go / diff_test.go / push_test.go）。
- 与 TASK-010 的 `writes` 五项**零交集**；我验过的四个文件在 `c4bbb39` 与 `15eee52` 上**逐字节 sha256 一致**。

⚠️ 但**「判定对象未漂移」不等于「判定仍然成立」**：`internal/hestia` 依赖 `internal/hestia/sheets`，而后者恰好被改了。`verify_baseline` 的判据是「声明范围内的文件变了」，依赖包的变化**刻意不在判据内**（否则多 dev 并行时会全量告警）。所以我在新 HEAD `15eee52b664be647047a236da273522e21f14280` 上又跑了一轮：

| 项 | 结果 |
|---|---|
| `internal/hestia` | ok，覆盖率 **96.6%**（不变） |
| `internal/hestia/sheets` | ok，覆盖率 **91.0%**（TASK-006 返工从 89.1% 提高） |
| 本轮判过的 6 条测试 | **全部 PASS** |
| 全仓 | **65 ok / 0 FAIL** |

⇒ 判定在新 HEAD 上仍然成立。记这一段是因为这条链路值得被下一个验证者知道：**基线漂移告警只覆盖「你声明的那些文件」，跨包的依赖变化要自己多跑一轮才知道。**

---

# 第三轮：返工复验（QA round2 CRITICAL-4）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `96272b98990dae6f931c564d814a6fdd6cabfe60`（= `verify_baseline.head`）；本轮交付 `ec24cbb`（+ merge `e299cbc`）
- discovery sha256: `3ea05e8983b22e318d35f7cf2358694d674f2f2e21390bb7683e54605fd56c09`（判定前现读一致 ⇒ **无漂移**）
- `assignment_epoch` 2 · `rework_count` **2** · 迁移带 `--expect-epoch 2`
- 范围: `ec24cbb` 五文件（config.example.yaml 13/18、internal/config/config.go 19/0、internal/config/config_test.go 47/0、internal/hestia/config.go 11/0、internal/hestia/config_test.go 53/0）⇒ **全在 `writes` 内**。越界项 `internal/config/*` 与 `configs/config.example.yaml` 由 dev 在 `dev_done` **之前**自行申报补进 `writes`，符合 CLAUDE.md「越界申报」节。
- 隔离 worktree: `wt-m2b-r010b` @ 基线

## 第三轮结论：**VERIFIED**（CRITICAL-4 两条修法达成 · 既有守卫未被破坏 · 无新问题）

## 判定矩阵

| 项 | 我的验证方式与结果 | 判定 |
|---|---|---|
| **修法 a**：`credentials_file` 非空 ⇒ `spreadsheet_id` 必须非空 | 实现在 `internal/hestia/config.go` 的 `validate()`，条件是 `CredentialsFile != "" && SpreadsheetID == ""` —— 只拦「配了一半」这一种。派验消息要我核「那四个绿的子用例是真的没误伤，不是碰巧绿」：**「没误伤」与「碰巧绿」在输出上取相同值，只有让它们该红时红才分得开**，所以我用变异打：**V1** 把条件放宽成「只要凭据非空就报错」⇒「两个都给 ⇒ 放行」红；**V2** 反转成「只要表 id 空就报错」⇒「两个都留空」与「整段不写」**同时**红；**V3** 把文案里的 `spreadsheet_id` 换掉 ⇒「配了一半」的 `Contains` 红。⇒ 四个子用例各有独立鉴别力 | PASS |
| **修法 b**：主配置检出 `hestia_sheets` 顶层键 ⇒ 报错并指路 | 实现是 `if v.InConfig("hestia_sheets")`，**只针对这一个键做显式探测，没有改成全局严格解析**（我逐行核过，`internal/config/config.go` 里无 `AllowUnknownFields` / 严格 decode 之类）。`InConfig` 只看配置文件本身，不受 `AutomaticEnv` 影响。三个子用例同样用变异打：**V4** 把探测键换成 `hestia` ⇒「hestia 段本身照常 ⇒ 不误伤」红；**V5** 改成无条件报错 ⇒「没有这个键 ⇒ 照常装载」红；**V6** 整块去掉 ⇒「主配置出现该键」红 | PASS |
| 🔴 **真实后果**（派验消息要我自己跑，不引用） | 我写了一次性探针，用**真实的**三份配置试装载（只打印错误文案，不打印文件内容——`configs/config.yaml` 里有真实凭据）：源树 `configs/config.yaml` ⇒ **拒绝**；运行时 `runtime/atlas/configs/config.yaml` ⇒ **拒绝**；`configs/config.example.yaml` ⇒ **放行**。⇒ 「合入后任何新构建都会拒绝装载当前配置」确证，且**两份都要挪**（不是一份） | PASS |
| **连带改动 TASK-011 的交付物** | `config.example.yaml` 里可复制的 `hestia_sheets:` 顶层键已删（`grep -c '^hestia_sheets:'` = **0**），只留指路注释；键与注释原文仍在 CONTRACTS 的 `## Sprint M2b-2`。上面那条探针的第三行正是它的正面证据：**样例配置仍能装载** ⇒ `TestExampleConfigDeclaresHestiaRules` 不会因修法 b 变红。dev 先申报后动手（把 `./configs/config.example.yaml` 补进 `writes`） | PASS |
| **`done_criteria` 回归**（本轮不得破坏前两轮的守卫） | 先核结构：`ec24cbb` **未触碰** `internal/hestia/ingest.go` 与 `sheets_project.go` —— 那是前两轮守卫的被测对象，一个字没动。仍不止于此，在**当前基线树**上重跑三个代表性变异：**R1** `recoverPanic` 吞掉 panic、**R9** 投影块挪到写契约之前、**R10** 侧车不再写出（失效告警）⇒ **全部仍 KILLED** | PASS |
| **净态**（我自己跑，不引用派验消息的数） | `gofmt -l` 对声明范围内 8 个 go 文件空输出；`go vet ./internal/hestia/... ./internal/config/...` exit 0；全仓 `go test ./... -count=1` exit 0、**65 ok / 0 FAIL**；覆盖率 `internal/config` **83.6%**、`internal/hestia` **96.6%**、`internal/hestia/sheets` **95.2%**、`cmd/atlas` **78.0%**。⚠️ gofmt **按声明范围跑**：全树跑会列出既有漂移文件，那不是本任务弄的 | PASS |

## 🔴 一个必须报的发现：merge 安全性的依据核错了对象

派验消息写：「**为什么现在没炸**：生产二进制是 Aug 7 的，`strings` 里 `internal/hestia` 命中 **0**。请你复核这一条——**它是「merge 是否安全」的全部依据**。」

我复核了，**结论成立，但理由核的是错的那个二进制**。

我先查 launchd 实际执行哪一个（读 `~/Library/LaunchAgents/…hestia-ingest.plist` 的 `ProgramArguments`）：

```
/Users/zuowei/workspace/runtime/atlas/bin/atlas  hestia ingest
    --hestia-config /Users/zuowei/workspace/runtime/atlas/configs/hestia.yaml
    --config        /Users/zuowei/workspace/runtime/atlas/configs/config.yaml
```

⇒ 跑的是 **`bin/atlas`**，不是派验消息核的 `runtime/atlas/atlas`。两者对照：

| 二进制 | mtime | `internal/hestia` | `hestia_sheets` | `HestiaSheets` |
|---|---|---|---|---|
| `runtime/atlas/atlas`（被核的那个） | 2026-08-07 08:06 | **0** | 0 | 0 |
| **`runtime/atlas/bin/atlas`（launchd 实际跑的）** | **2026-09-15 22:25** | **442** | **0** | **0** |

**判据 `internal/hestia` 命中 0 对实际运行的二进制为假**（442）。

**正确的判据是 `hestia_sheets` / `HestiaSheets` 在两个二进制上都命中 0**：修法 b 要探测 `hestia_sheets` 这个字符串、修法 a 要读 `HestiaSheets` 结构，两者都必然把符号编进二进制。两个都命中 0 ⇒ 生产二进制**不含本轮的检测逻辑** ⇒ 装载不会被拒。**结论仍然成立，只是它成立的原因与被给出的原因不同。**

**由此得到一个比原表述更精确的顺序约束**：

> **真正会炸的时刻是下一次部署，不是下一次构建。**

现在跑的 `bin/atlas`（09-15）构建于 M2b 的配置结构引入之前，所以它读到 `runtime/.../config.yaml:357` 的 `hestia_sheets` 时照旧无视。一旦有人重新 build 并把新二进制部署到 runtime，**hestia ingest 会在下一次 launchd 唤起时当场全挂**（装载期报错，连 C8 那条「只打印不阻断」的路径都走不到——那是 ingest 内部的，而这次是 ingest 还没起来）。

⇒ 人执行清单里那条「挪配置与重新 build 必须成对做」应改成**有方向的顺序**：**先挪两份配置，再部署**。反过来做会有一个「二进制已换、配置没挪」的窗口，而那个窗口里 hestia 是完全停摆的。

## 派验消息里另两条事实，我独立核过

| 文件 | `hestia_sheets` 顶层键 | 对应 hestia.yaml |
|---|---|---|
| 源树 `configs/config.yaml` | **:332 有** | `configs/hestia.yaml` 命中 **0** |
| 运行时 `runtime/atlas/configs/config.yaml` | **:357 有** | `runtime/atlas/configs/hestia.yaml` 命中 **0** |

两份都要挪，确证。（我只 grep 键名与行号，没有打印任何配置内容——那两份里有真实凭据。）

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-r010b 96272b98990dae6f931c564d814a6fdd6cabfe60
cd ../wt-m2b-r010b
GOTOOLCHAIN=local go test ./... -count=1                                     # 65 ok / 0 FAIL
GOTOOLCHAIN=local gofmt -l internal/hestia/ingest.go internal/hestia/config.go \
  internal/hestia/config_test.go internal/config/config.go internal/config/config_test.go   # 空
GOTOOLCHAIN=local go test ./internal/hestia/ ./internal/config/ -count=1 -v \
  -run 'TestConfigRejectsHalfConfiguredSheets|TestLoadRejectsHestiaSheetsInMainConfig'
# merge 安全性的**正确**判据（不是 internal/hestia，是 hestia_sheets）
plutil -convert json -o - ~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist | python3 -m json.tool | grep -A3 ProgramArguments
strings /Users/zuowei/workspace/runtime/atlas/bin/atlas | grep -c 'hestia_sheets'   # 0 ⇒ 不含检测
```

## 判定落盘后补入：修法 b 的错误文案指向一个放不住凭据的文件（条件性事实，非缺陷）

Leader 在我 06:10:24Z 判完之后报来一条事实，落在 CRITICAL-4 的范围内。**我独立复核，它属实**，且我用了比它更宽的匹配式（它说自己这个 sprint 已栽过三次 grep，所以我没有只 grep `hestia.yaml` 这个字面量）：

```
grep -n "hestia" scripts/ops/deploy.sh
  → 只命中两行**注释**（:10、:21），**没有任何一行是 exclude 规则**
grep -n "exclude" scripts/ops/deploy.sh | grep -iE "config|yaml|yml"
  → 只有 --exclude='/arcforge.config.json' 与 --exclude='/configs/config.yaml'
```

| 文件 | git | rsync 排除表 | 结果 |
|---|---|---|---|
| `configs/config.yaml` | untracked + **IGNORED**（`.gitignore:30`） | **显式排除**（`deploy.sh:100`），部署后另有 `chmod 600`（`:112`） | 受保护 |
| `configs/hestia.yaml` | **TRACKED** | **不在排除表**（任何形式都没有） | **每次部署被源树覆盖** |

⇒ 修法 b 的文案让人「整段挪去 `configs/hestia.yaml`」，而照做只有两个结果：**改源树那份 ⇒ 真实凭据进 git**；**只改运行时那份 ⇒ 下次 `deploy.sh` 覆盖回去，能力静默重新禁用**。

### 但这不是本任务的缺陷，理由是范围本身

- `fix_items[3]` 的**字面要求**就是「报错并**指明正确位置是 `configs/hestia.yaml``**」——dev 完全照做。
- `fix_items[4]` **明确把落位问题排除在本任务外**：「凭据落位三条出路的选择归人拍板，我不会在本 sprint 内替人做决定，你也不要动 `configs/config.yaml` 或 `configs/hestia.yaml` 的实际内容。」

⇒ **既不是 dev 的偏离，也不是修法的缺陷，而是 `fix_items[3]` 与 `[4]` 之间的一条缝**：[3] 要求文案指向一个具体位置，而「那个位置是否可用」被 [4] 划出了范围。**拆分本身制造了这条缝**——把「让它变响」与「给它地方可去」分给两个条目，而前者的产物必须引用后者尚未回答的东西。

**dev 自己的措辞最准，我原样引用**：

> 本轮只让「填错位置」变响，**没有回答「正确位置在部署流程里怎么活下来」**。

**「变响」与「有地方可去」是两件事，本轮只做到前一件。** 这个区分应当进 final-report，因为它决定了人执行清单那条的完整性：**挪配置不是一个动作，是「挪 + 让它在下次部署后还在」两个动作**，而第二个动作现在没有任何机制保证。

### 对已落盘判定的影响：无

两条修法本身的达成我已用 V1–V6 六个变异逐条证过（每个变异红的正是它该红的子用例），探针也确证了「装载期就红」这个要求的全部效果。本条是**文案所指位置的有效性依赖于一个尚未做出的部署决定**，判为**条件性事实**：

- 若人选择把 `configs/hestia.yaml` 也加进 rsync 排除表并 gitignore ⇒ 文案正确，无需改动；
- 若不改部署流程 ⇒ 文案是在指路去一个会被冲掉的地方，届时需要改文案或改流程。

⇒ **VERIFIED 不变**，但这条必须留在报告里，否则下一个读「已 VERIFIED」的人会以为凭据落位已经解决。

### 顺带：deploy.sh 自己早就写过同族的警告

`deploy.sh:20-21` 有一句与本条同构：

> 排除表补两条只堵住**已知**的两个症状；这道判别堵的是成因——**下一个被 gitignore 的运行时目录出现时，排除表不会保护它**。

`configs/hestia.yaml` 的情形是它的镜像：**不是「被 gitignore 而未被排除」，而是「被 tracked 且未被排除」** —— 同样一条「排除表是白名单式的、新增项默认不受保护」的性质，两个方向各咬一次。

## 再补：部署后停摆的范围是六个 launchd 任务，不止 hestia，也不止 serve

我先前写「hestia ingest 会当场全挂」，Leader 补「不只是它，`serve` 也起不来」。**两句都不完整。** 我把装载链路走了一遍：

**`config.Load` 全仓只有 3 个直接调用点**（`serve.go:69` / `broker.go:79` / `export_ohlcv.go:286`），但第三个在 `loadConfigOrDefaults()` 里，而**那是一个扇出点**——8 个调用点覆盖 `export-ohlcv` / `crisis` / `policy` / `watchlist` / **`hestia`** / `prism` / `export-signals`。

**`hestia ingest` 受影响的路径不是我以为的那条**：`openHestia()` 走的是 `hestia.LoadConfig(hestiaCfgPath)`（读 `hestia.yaml`，**不经主配置装载器**）。真正的路径是
`runHestiaIngest`（`hestia.go:344`）→ `buildHestiaSender()`（`:314-315`）→ `loadConfigOrDefaults()` → `config.Load(cfgFile)`。
而 `hestia.go:306-310` 的注释明确写着「**主配置装不上是错误，不折成 nil**」⇒ 它会返回 error 而不是静默降级。
⇒ **我那句结论是对的，但我当时没查路径。换一个装载方式，那句就错了。**

**实际停摆范围**（我读了全部 11 份 plist 的**完整** `ProgramArguments`，判据是「二进制是 `bin/atlas`」∧「传了 `--config` 或 `-c`」）：

| launchd 任务 | 命令 | 受影响 |
|---|---|---|
| `serve`（**常驻，PID 3677**） | `serve --config …/config.yaml` | 🔴 |
| `hestia-ingest` | `hestia ingest --hestia-config … --config …/config.yaml` | 🔴 |
| `crisis-daily` | `crisis eval --mode daily --config …/config.yaml` | 🔴 |
| `crisis-intraday-jpy` | `crisis eval --mode intraday --config …/config.yaml` | 🔴 |
| `crisis-nfci` | `crisis eval --mode nfci --config …/config.yaml` | 🔴 |
| `prism-daily` | `prism refresh -c …/config.yaml` | 🔴 |
| `aktools` / `baostock` / `analysis` / `refresh-cnhk` / `refresh-us` | python / shell 脚本，非 `atlas` 二进制 | 否（但 `refresh-market.sh` 内部若调 atlas 需另查） |

⇒ **部署后停摆的是 6 个任务，整个 atlas 定时体系加常驻服务**。原先的表述（先是「hestia ingest」、后是「serve 也是」）**每一次都只说中了其中一个**。

⚠️ **我在查这件事时犯了与被查对象同形的错**：第一次取 `ProgramArguments` 我写了 `a[1:4]`，只看前三个参数 ⇒ `crisis eval --mode` 后面的 `--config` 被我自己截掉了，差点得出「crisis 不传主配置、不受影响」。**这与 Leader 那次 `find -maxdepth 4 | head -1` 是同一个形状：在核查一个因截断而漏掉的结论时，自己又截断了一次。** 重取完整参数才看清六个都传了。

⇒ **人执行清单那条的完整版**：**先挪两份配置 → 确保 `configs/hestia.yaml` 不会被 rsync 冲掉 → 再部署**。反序或跳过中间那步，窗口期内**六个任务全停**，而其中 `serve` 是常驻的。
