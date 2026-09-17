# TASK-008 验证报告 — 缺失年度表的创建（CreateYearTab 四步 + push 第 6 步接线）

> **本文件含两轮**：**第一轮首验**（2026-09-17 00:34Z，VERIFIED）在下方「第一轮」各节；
> QA 判 REJECT 后 008 进 `review_fix`，**第二轮返工复验**（2026-09-17，VERIFIED）见文末
> 「# 第二轮：返工复验」。**第二轮才是当前生效的判定。**
> ⚠️ 第一轮里我提出的验证者夹具 N7，**在第二轮被 dev 指出有缺陷并修正**，详见第二轮。

## 第一轮：首验（2026-09-17 00:34:04Z）

- 验证者: test-m2b-b　　时间: 2026-09-17（接替 test-m2b-a，前任因账号额度耗尽停机）
- 判定对象: master @ `3770e9482129981cd7a0608e55fadd2c090203ed`（= `verify_baseline.head`）；交付 commit `b119a5a51c6ddac5a7c86128e797aada80e5691b`；base `e0f4f8b4fdca4575d7b1bd1010126e85c69d8697`
- discovery sha256: `5458c9b36928a6099288e254dba6a7f82cea4c02c792afb711337db40812494b`（= `verify_baseline.discovery_sha256`，开工与判定前各现读一次，均一致 ⇒ **无漂移，无需 `--ack-drift`**）
- provenance：代码由 **dev-m2b-b** 编写并提交，该实例在 code-simplifier 子代理批次中因额度耗尽中断，Leader 按 AD-21 改派（`assignment_epoch` 1→2），**dev-m2b-c 接手未改一行代码**、未再跑 simplifier。验证者用**内容判据**核实：四文件在交付 commit、baseline 树、验证 worktree 三处 sha256 逐字节一致（`f0005847…` / `d15fbc37…` / `e7ab1aae…` / `f489c86b…`），与 discovery 声明的值一致。discovery 另申报了一处时间线差异（pre-simplifier 快照 00:02 vs commit 时间 07:47，三个文件在快照之后还被改过），前版原文不可回捞，故以合入版逐行审读代替 diff 审查。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v008` @ `3770e9482129981cd7a0608e55fadd2c090203ed`（全 sha 锚）
- 范围核对: `git diff e0f4f8b..b119a5a --numstat` **恰为** `writes` 四文件（client.go 112/6、push.go 93/30、push_test.go 200/14、store_test.go 6/1，四行即全部输出）；`git show --numstat b119a5a` 同值 ⇒ 无越界申报、go.mod/go.sum 无变化。

## 结论：**VERIFIED**（5/5 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `CreateYearTab(ctx, templateTab, newTab string, year, index int) error`；请求体断言四步齐全：`duplicateSheet` / `2021年` / `2021 年 ·` / `"index":3` | test | dev `TestCreateYearTabDoesAllFourSteps`（DoD 字面的拼接串判据）+ `TestCreateYearTabSendsIndexZero`。验证者 `TestW_CreateYearTabFourStepsStructurally` 把 batchUpdate 请求体**解析成结构**逐字段核，而不是在拼接串里找子串——`Contains` 只证明某处出现过这串字，证不了它出现在正确的 request、正确的字段上，也证不了请求恰好是四个：断言 `len(requests)==4`、① `duplicateSheet.sourceSheetId == 模板 id` 且 `newSheetId == max+1`、② `fields=="title"` 且 `properties.title=="2021年"` 且 `sheetId==新表`、③ `updateCells` 的 range 是新表的 A1（`startRowIndex 0 / endRowIndex 1`）、`fields=="userEnteredValue"`、值**逐字等于** `2021 年 · 金融数据追踪`、④ `fields=="index"` 且 `index==3`。`TestW_CreateYearTabIndexZeroIsSent` 结构化取 index 值 + 原始 JSON 双判据。签名逐字核对 client.go 的函数声明 | PASS |
| functional[1] | push 接线：`Apply && CreateSheets` 且缺表 ⇒ `duplicateSheet` **先于**首个写数据请求；模板固定 `"2024年"`；index 按「现有年份表升序中第一个 > year 的位置，无则末尾」——**验证者用 {2023年,2024年,2026年} + 新 2025 断言 `index==2`** | test | **DoD 点名给验证者的那条**：`TestW_PushPlacesNewTabByYearOrder` 自构响应集（验证者自己的 `vTabs`/`vResp`，模板 sheetId 用 777 而非 dev 的 12345），实测 `index==2`。顺序：`TestW_PushCreatesBeforeWritingAndWritesNewTab` 断言**两个**建表请求的下标都小于唯一数据写请求的下标（dev 的 `require.Less(iDup, iWrite)` 只比第一个）。边界：`TestW_PushPlacesNewTabAtEdges` 两支——新年份最小 ⇒ index 1（非年度表「说明」占着表序 0，答案不是 0）、最大 ⇒ index 3（末尾）。累积：`TestW_PushPlacesTwoNewTabsCumulatively` 断言连建两张的 index 依次为 `[1, 3]`，钉住「后一张必须把前一张已插入的位置算进本地表序」。模板常量：`templateYearTab = "2024年"` 逐字核对，且所有建表请求的 `sourceSheetId` 都等于 2024年 的 id | PASS |
| boundary[0] | `CreateSheets=true` 但不缺表 ⇒ 零 `duplicateSheet`；`Apply=false` 时即使缺表也**不建表** | test | dev `TestPushCreateSheetsWithoutMissingTabsDuplicatesNothing` + 007 既有 `TestPushDryRunReportsMissingTabsWithCreateFlag`（复跑仍绿）。验证者 `TestW_PushCreateSheetsNoMissingDoesNothing`：断言**一个 `<ss>:batchUpdate` 都没发**（比「零 duplicateSheet」更强，连空批次也不许有）。`TestW_PushDryRunNeverCreates`：缺两张 + `CreateSheets=true` + `Apply=false` ⇒ `MissingTabs` 列全、`requireOnlyGETs` 逐条断言、零写体、零建表批次 | PASS |
| error_handling[0] | 模板表不存在 ⇒ error 含模板名；`duplicateSheet` 4xx ⇒ 后续三步不做 | test | 模板缺失：dev `TestCreateYearTabErrorsWhenTemplateMissing`；验证者 `TestW_CreateYearTabMissingTemplate` 加两条——错误里**同时**含模板名与目标表名，且 `rec.methods` **恰为** `["GET <spreadsheets.get>"]`（连模板标题那次 GET 都没发，不只是「没发写请求」）。4xx：dev `TestCreateYearTabStopsWhenBatchUpdateFails`；验证者 `TestW_CreateYearTabStopsOnBatchFailure` 对 **403 与 400 各验一次**，断言只发过 1 个写请求且它就是那个四步合一的 batchUpdate。**验证者独立核实了 dev `decisions[2]` 的论证**：四步在一次 batchUpdate 里，API 原子生效，所以「后续三步不做」的正确可观测形式就是「写请求恰 1 个」，而不是「找到三个被跳过的请求」——按字面去找后者会找不到，这不是缺口。push 侧补一条 `TestW_PushWritesNothingWhenCreateFails`：建表失败时已有表的格也一个都不写 | PASS |
| non_functional[0] | 写口守卫先红后绿 + 注释段；006/007 既有测试仍绿；`go test ./internal/hestia/... -count=1` 全绿、`gofmt`/`go vet` 零输出；code-simplifier | manual | **守卫名单**：python 解析 `store_test.go` 的 `want`，两把独立的尺——字符串字面量 **51** 条、逗号 50 个且无尾逗号 ⇒ 元素 **51**，两者一致；`items == sorted(items)` 为真；`sheets.Client.CreateYearTab` 在第 **40** 位（0-based），邻居 `WriteHistory` / `sheets.Client.ReadEntryArea`，符合字节序；相对 007 基线 50 项净增 1。**守卫此刻真的在守**（不只是名单对）：变异 N15 往 sheets 包加一个未登记的导出方法 `Client.DeleteTab`，`TestPackageExposesNoWriteFunctions` **立刻变红**。**注释段**：`store_test.go` 的「为什么名单里多了 sheets.*」那节末尾追加了 TASK-008 段落，逐句说明它改的是 Google 表格结构而非库、拿不到 `*Store`/`*sql.DB`、字节序位置理由。**红阶段**：discovery `verification.red_phase` 有两份原始输出（编译红 `c.CreateYearTab undefined` 四行；守卫红 `+ "sheets.Client.CreateYearTab"` 与 `--- FAIL: TestPackageExposesNoWriteFunctions`，同批 `TestStoreExposesNoWriteMethods` PASS）。**006/007 既有测试**：逐条比对 007 交付版与当前 `push_test.go` 的测试函数清单——7 条中**保留 6、删除 1**（`TestPushCreateSheetsHookRunsBeforeWrite`，已在 discovery `key_findings[7]` 申报，见下「诚实记录」①）、新增 7 条；006 的测试文件（`client_test.go` / `diff_test.go` / `header_test.go`）不在本任务 diff 里。**净态**（验证者夹具移出后采）：`gofmt -l internal/hestia/` 空输出、`go vet ./internal/hestia/...` exit 0 无输出、`go test ./... -count=1` exit 0 且 **65 ok / 0 FAIL**；两条守卫测试单跑 2/2 PASS。**含夹具**：`go test ./internal/hestia/sheets/ -v` 顶层 **51 PASS / 0 FAIL / 0 SKIP**（两把尺一致），覆盖率 91.3%（净态 89.1%）。**code-simplifier**：原作者跑过，前版原文不可回捞，以合入版逐行审读代替——验证者读完四文件全部新增，无冗余抽象、无未使用符号 | PASS |

## 变异测试

15 个变异，作用在**隔离 worktree 副本**的 `client.go` 与 `push.go` 上，主工作区一个字节不碰。
流水：落盘 → `gofmt -e` 语法闸 → `go vet` → **打印变异 diff 逐字核对**（语义闸）→ `go test` → `git checkout` 还原 → 校验 worktree 与主工作区两处 sha256。
有效性闸：编译失败 / 非零退出但零条 `--- FAIL` ⇒ 判无效并**立即早退**，不打印 KILLED。**这道闸当场救了一次**：首版 N2「删掉③改标题请求」让 `newTitle` 变成未使用变量、编译失败，若没有早退会被记成 KILLED；已换成能编译的等价意图变异（③ 写模板原标题而非新标题）。收尾时两文件 sha256 均回到 `f0005847…` / `d15fbc37…`。

| 变异 | 结果 | 谁杀的 |
|---|---|---|
| N1 去掉 ② 改名请求 | KILLED | dev 1 条 + 验证者 W1 / W3 |
| N2 ③ 写模板原标题而非新标题（漏掉年份替换） | KILLED | dev 1 条 + 验证者 W1 / W3 |
| N3 去掉 ④ 排位请求 | KILLED | 8 条（dev 3 + 验证者 5） |
| N4 去掉 ① 复制模板请求 | KILLED | dev 3 条 + 验证者 W1 / W3 |
| N5 去掉 `ForceSendFields`（index 0 被吞） | KILLED | dev `SendsIndexZero` + 验证者 W2 |
| N6 标题恒走拼接分支（不做年份替换） | KILLED | dev 1 条 + 验证者 W1 / W3 |
| N7 新表 id 用 `maxID` 而不是 `maxID+1` | KILLED | **只有验证者 W1**（见「诚实记录」②） |
| N8 模板不存在时不报错、照样往下建 | KILLED | dev 1 条 + 验证者 W8 |
| N9 建表挪到写数据之后（= 007 的 M5 等价变异） | KILLED | dev `CreatesMissingTabsBeforeWriting` + 验证者 W7 / W10 |
| N10 `createYearTabs` 不把已建的表插进本地表序 | KILLED | **只有验证者 W6**（见「诚实记录」②） |
| N11 index 算法 `y > year` → `y >= year` | 🔴 SURVIVED | **等价变异**，见下 |
| N12 建完表不补做第 3、4 步（新表的行不写） | KILLED | dev 1 条 + 验证者 W7 |
| N13 dry-run 短路挪到第 6 步之后 | KILLED | 007 既有 `DryRunReportsMissingTabs…` + 验证者 W12 |
| N14 去掉第 2 步的 `CreateSheets` 守卫 | KILLED | 007 既有 `TestPushRefusesMissingTabs` |
| N15 新增未登记的导出方法 `Client.DeleteTab` | KILLED | `TestPackageExposesNoWriteFunctions`（守卫此刻真在守） |

### N11 存活的判定（等价变异，不构成缺口）

把 `y > year` 改成 `y >= year` 只在「现有表序里已经有一张 `<year>年`」时才产生不同结果。而 `missing` 的定义就是「不在 `have` 里的表名」，`createYearTabs` 的本地表序 `order` 又从 `existing` 克隆、只追加已建的新表——所以 `y == year` 这个分支在 `Push` 路径上**结构性不可达**。不是断言缺口。

## 诚实记录：三处值得写下来的地方

① **discovery 里有一句事实声称是假的，但它支撑的结论成立。** `key_findings[7]` 写着「TASK-007 的 discovery 提到过该测试名 1 次；**007 已是 verified，其 verification.md 未引用该测试名**」。验证者实测：`grep -c` 在 `docs/04-test/TASK-007-verification.md` 上命中 **2 行**——变异表里 M5 的「谁杀的」列写着 `dev CreateSheetsHookRunsBeforeWrite`，M10 那行也提到它。也就是说，007 报告中**变异 M5 的唯一杀手，被本任务删掉了**。
这句话若成立，本该引出一个真问题：删了那条测试，M5 所守的性质（建表挂点必须在写之前）是不是没人守了？验证者没有停在推理上，而是**把 M5 的等价变异重跑了一遍**——这就是 N9。结果 **KILLED**，由 dev 的 `TestPushCreatesMissingTabsBeforeWriting` 与验证者的 `TestW_PushCreatesBeforeWritingAndWritesNewTab` / `TestW_PushWritesNothingWhenCreateFails` 三条同时杀掉。⇒ **守卫真空不存在，删除是安全的**；错的只是 discovery 里那半句支撑理由。按分工，验证者的核实结论落在本报告而不是去改 dev 的 discovery（`verifying` 窗口内改 discovery 会让判定原料漂移）。

② **两个变异只被验证者夹具杀掉。** N7（新表 sheetId 用 `maxID` 而非 `maxID+1`，会与模板表撞 id）——dev 的四步测试断言了 `"sourceSheetId":12345`，但没断言 `newSheetId`，所以 id 撞了也不红。N10（连建两张时不更新本地表序，第二张的 index 会算错）——这正是 discovery `key_findings[5]` 明确声称的行为（「每建一张就 `slices.Insert` 进本地表序，后一张的 index 才把前一张算进去」），**声称为真但无测试守卫**，dev 的用例只建过一张或只看第一张的 index。两者都不构成 DoD 缺口（DoD 没点名这两条性质），但都是真实的断言缺口，建议后续任务把验证者夹具里的对应两条移植进交付测试。

③ **验证者自己的仪器错了一次。** 夹具首版用 `strings.HasSuffix(method, ":batchUpdate")` 判「建表请求」，而写数据走的是 `<spreadsheet>/values:batchUpdate`——同一个后缀。于是数据写请求被当成建表请求解析（解析出空的 `requests` 列表），两条用例红。成因是**拿一个高度相关的可观测量代替了性质本身**：后缀相同不等于端点相同。订正为精确路径比对（`"POST " + pathBatchUpdate`）后全绿，**本报告的全部数字都是订正之后统一重采的**。

## 报给 Leader 的一条口径问题（非本任务缺陷）

`TASK-008.packages` 只有 `./internal/hestia/sheets`，而 `writes` 含 `./internal/hestia/store_test.go`——后者属于 `internal/hestia` 包。DoD `non_functional[0]` 要求的写口守卫（`TestPackageExposesNoWriteFunctions`）就住在那个包里，于是 **`dev_done` 门禁跑 `go test <packages>` 时跑不到它**：门禁量的是 sheets 包，而 DoD 点名的那条测试在父包。本次 dev 自己单独跑了守卫（discovery `verification.green` 有留痕），验证者也独立复跑并用变异 N15 证明它在守，所以本任务无实质风险。但这是**任务拆分时的声明口径缺口**：凡是「改父包 store_test.go 的守卫」类任务，`packages` 都该把父包一并列上，否则门禁对 DoD 的这一条结构性失明。

## 验证者记录的实际行为（DoD 未规定，不判红）

- **陈旧注释**：`push_test.go:23` 的 DoD↔测试映射注释仍指向已被本任务删除的 `TestPushCreateSheetsHookRunsBeforeWrite`。读注释的人会去找一个不存在的测试。建议顺手清理。
- **`createYearTabs` 对非年度表名有防御分支**（`"%q 不是年度表名"`），但 `missing` 由 `tabName(r.Year)` 生成、恒为 `<4 位数>年`，该分支在 `Push` 路径不可达。验证者 `TestW_CreateYearTabsRejectsNonYearName` 包内直接调它补上了覆盖。
- **`tabYear` 的边界**：`TestW_TabYearBoundaries` 表驱动九例——`2021年` ✓、`1999年` ✓；`说明` / `2021`（无「年」）/ `2021年 `（尾空格）/ ` 2021年`（首空格）/ `202年` / `20211年` / `2021年度` 全部 ✗。正则 `^(\d{4})年$` 两端锚定，不接受任何空白，与表名约定 `fmt.Sprintf("%d年", year)` 严格对齐。
- **模板标题替换的四种形态**（`TestW_CreateYearTabTitleVariants`）：带年份有后半句 ⇒ 只换年份；不带年份 ⇒ 拼 `<year> 年 · <原标题>`；空标题 ⇒ 拼成 `2021 年 · `（尾部有空格，可接受但略怪）；`  2024年 · 追踪`（年份前有空白）⇒ 正则 `^\s*\d{4}\s*年` 照样命中，结果 `2021 年 · 追踪`，前导空白被一并吃掉。
- **新表录入区全空 ⇒ 它的行全是 `WillWrite`**，且写请求里出现 `'2023年'!A15` / `'2025年'!A15` 两个新表 range，`len(Data) == WillWrite` 成立，不变量 `W+S+A == rows×labels` 成立。

## 测试质量评审

- **他证而非自证**：建表「先于」写数据是从 httptest 服务端收到的请求顺序看出来的，不是检查代码里 `if` 的位置；dry-run「没建表」同样由服务端逐条 GET 断言。
- **断言精确**：验证者侧全部是结构化字段相等或整切片相等（`[]int64{1, 3}`），dev 侧是带独立 needle 的 `Contains`（漏任一步即红）。无 `assert.True` 之类空洞断言，无 mock 掩盖真实路径——httptest 是唯一出口。
- **`newTestClientFailingPOST` 的设计合理**：GET 照常按响应表查、任何 POST 回指定状态码，只让写动作失败，从而把「失败后还发不发第二个写请求」变成可观测的。
- **dev 的 DoD↔测试映射注释**逐条对得上（除上文那条陈旧行）。
- 偏弱处见「诚实记录」②的两条断言缺口。

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-v008 3770e9482129981cd7a0608e55fadd2c090203ed
cd ../wt-m2b-v008
GOTOOLCHAIN=local go test ./internal/hestia/... -count=1 -cover   # 96.6% / 89.1%
GOTOOLCHAIN=local go test ./... -count=1                          # 65 ok / 0 FAIL
GOTOOLCHAIN=local gofmt -l internal/hestia/                       # 空
GOTOOLCHAIN=local go vet ./internal/hestia/...                    # exit 0，无输出
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 \
  -run 'TestPackageExposesNoWriteFunctions|TestStoreExposesNoWriteMethods'   # 2/2 PASS
```

---

# 第二轮：返工复验（QA REJECT 后第 1 轮）

- 验证者: test-m2b-b　　时间: 2026-09-17
- 判定对象: master @ `042c739d93c2c875f246487d63454f7886d651ac`（= 新的 `verify_baseline.head`）
- discovery sha256: `b4c96e345c6b577780280d00d467629d1063e8408778167a0f4923173ac259ea`（现读一致 ⇒ **无漂移**）
- `assignment_epoch` 2 · `rework_count` 1 · 迁移带 `--expect-epoch 2`
- 本轮 008 自己的交付：`ba6c589`（test 主体，92/0）+ `891a95f`（code-simplifier，2/2），**两个 commit 都只改 `internal/hestia/sheets/push_test.go`** ⇒ 在 `writes` 内，无越界
- 🔴 **`fix_items[0][1][2]` 不在本轮 commit 里**：dev 声称它们「已在 TASK-006 的返工提交 `5af1701` 中做掉」（两个任务的 `writes` 都含 `push_test.go`，而 006 当时在途）。Leader 在 `fix_items[0]` 里只确认了 CRITICAL-2 那一条，**[1] 与 [2] 是 dev 自己声称的，我独立核实**——核实方式是变异，不是读 commit message。
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-r008`（活 master 上 TASK-010 正在返工，不在那里取证）

## 第二轮结论：**VERIFIED**（4 条 `fix_items` 全部达成）

## fix_items 覆盖矩阵

| # | 要求 | 我的验证方式与结果 | 判定 |
|---|---|---|---|
| [0] | CRITICAL-2 模板表自污染（Leader 已确认在 006 做掉，本任务不必重做） | 我没有只看 Leader 的确认，而是造变异 **U7**（把 `CreateYearTab` 里「清空新表录入区」那一步整块删掉）⇒ **KILLED**，由 `TestCreateYearTabClearsNewTabEntryArea` 与 `TestCreateYearTabClearsEvenWhenTemplateHasData` 两条守。⇒ 修复与守卫都确实在场 | PASS |
| [1] | 在 `Push` 层让建表失败、断言 `WriteCells` 未被调用；原恒真断言改注释或删除 | 变异 **U3**（`createYearTabs` 失败后不 `return`、继续往下走到写数据）⇒ **KILLED** by `TestPushStopsWhenCreateYearTabFails`。这正是 `push.go:177-179` 那条零覆盖分支所守的性质，也正是原恒真断言「自称在守、实际没守」的那一条 ⇒ 核心要求达成。恒真断言那半句见下「我的第二个判断被实测否定」 | PASS |
| [2] | 移植验证者夹具 N7 与 N10 | **N7：dev 没有照抄，而是指出我的夹具有缺陷并修对了**——对照实验确证，见下「本次最有价值的一条」。**N10**：变异（`createYearTabs` 不把已建的表插进本地表序）⇒ **KILLED** by `TestPushPlacesTwoNewTabsCumulatively` | PASS |
| [3] | 补 `push.go` 其余零覆盖的 error 传播分支；给 `newTestClient` 加按路径注入 4xx 的粒度 | 五个变异逐块打：**U1**（`Tabs` 失败不返回）/ **U2**（已有表 `diffTab` 失败不返回）/ **U4**（新表 `diffTab` 失败不返回）/ **U5**（`WriteCells` 失败不返回）/ **U6**（`createYearTabs` 收到非年度表名不报错）⇒ **全部 KILLED**。我自跑 `go tool cover -func` 确认 `push.go` 的 **六个函数全部 100.0%**；`internal/hestia/sheets` 覆盖率 **94.3%**（返工前 91.0%）。按路径注入 4xx 的 `newTestClientFailPath` 已在场（006 那轮加的），读路径 4xx 这个此前从未走过的粒度现在走得到了 | PASS |

## 🔴 本次最有价值的一条：被验方指出了验证者夹具的缺陷，而且指对了

dev 在 `5af1701` 的 commit message 里写：

> `fix_items[6]` N7 我没照抄验证者夹具：它把模板设成 777 而其余表是 10/12，**模板仍是最大值**，「用 `tplID+1` 代替 `maxID+1`」这个变异在它那儿同样不红。

**这条批评成立，而且是算术上必然的。** 我首验时写的 `vTabs` 给出 `{说明: 10, 2024年: 777, 2026年: 12}`，于是 `maxID` 与 `tplID` **都是 777**，`maxID+1` 与 `tplID+1` **都是 778** —— 两个算法在我的夹具下产出完全相同的 `newSheetId`，任何基于它的断言都无法区分。dev 改成 `{说明: 10, 2024年: 777, 2026年: 9000}`，`maxID+1 = 9001` 而 `tplID+1 = 778`，两者分叉。

我做了对照实验：同一个变异，两份夹具各跑一次。

| 变异 | dev 的新测试（模板**非**最大 id） | 我的旧夹具（模板**即**最大 id） |
|---|---|---|
| **T1** `newID := tplID + 1` | **KILLED** | 🔴 **不红** |
| **T2** `newID := maxID`（我首验时用的那个） | **KILLED** | KILLED |

⇒ **dev 的修法严格更强**：它杀得掉两个变异，我的只杀得掉一个。我首验时之所以判 N7「KILLED」，是因为我选的变异恰好是那个在我夹具下也能分叉的（`maxID` 而非 `maxID+1`），**运气而非设计**。

这条值得单独写出来的理由：验证者的夹具被当作「更强的判据」移植进交付测试，而它自己带着一个未声明的盲区。**移植前没有人再验一次那份夹具本身**——这次是被验方在照抄前读懂了它才发现的。

## 我的两个判断错误

### ① 对照实验首版无效，我差点据此否掉一条正确的批评

我最初的做法是把旧夹具原样放回当前树、直接跑 T1 变异。它**红了**，于是第一反应是「dev 的批评不成立，我的夹具明明杀得掉」。

追下去才发现红的是另一条断言：`require.Len(reqs, 4, "恰四步，不多不少")` —— **TASK-006 的返工给 `CreateYearTab` 加了第 5 步（清空新表录入区）**，所以我的旧夹具在**未经任何变异的当前树上本来就是红的**。那个红与 T1 毫无关系。

⇒ **对照组必须先确认它在未变异的树上为绿**，否则它的「红」不承载任何信息。把「恰四步」适配成「恰五步」之后重做，才得到上面那张表。这个错误如果没查下去，我会写出一段「被验方的批评不成立」的错误结论，而它读起来完全合理。

### ② 我猜那条改后的断言仍然恒真，被实测否定

`fix_items[1]` 指出原来的 `require.Len(writeBodies(rec), 1, "duplicateSheet 失败后不许再发任何写请求")` 是恒真的。dev 把它换成了 `require.Equal(t, 0, countBodiesContaining(rec, "valueInputOption"), "CreateYearTab 报错后不许再发写数据请求")`。

我推理：那个用例**直接调 `CreateYearTab`、不经过 `Push`**，而写数据请求是 `WriteCells` 发的，所以在这个用例里它本就不可能出现 ⇒ 新断言同样恒真。

我没有停在推理上，造了变异 **V1**：让 `CreateYearTab` 在 `batchUpdate` 失败后**额外发一次写数据请求**（语义荒谬，唯一目的是测那条断言的鉴别力）。结果 **KILLED** —— 断言变红了。

**我的推理错了。** 新断言确实能区分。它相对原断言的真正价值在于**判据的种类换了**：原来数的是「写请求的条数」，那个数字对「四步拆成两个 batchUpdate」这种**合法重构**会假红、而对「失败后继续写数据」这种**真缺陷**不敏感；改后只问「有没有写数据请求」，对重构稳健、对缺陷敏感。

## 净态（钉死的 worktree 上取证）

| 项 | 结果 |
|---|---|
| 全仓 `go test ./... -count=1` | exit 0，**65 ok / 0 FAIL** |
| `gofmt -l`（4 个 writes 文件） | 空 |
| `go vet ./internal/hestia/...` | exit 0 |
| `internal/hestia/sheets` 覆盖率 | **94.3%**（返工前 91.0%） |
| `push.go` 各函数 | **六个全部 100.0%** |
| `push_test.go` 的 Test 函数 | **22**（与 dev 所报一致） |
| 写口守卫 `want` | 仍 **51**（本轮未动 `store_test.go`） |
| 收尾 `git status --porcelain` | 空（旧夹具已移除、全部变异已还原） |

## 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-m2b-r008 042c739d93c2c875f246487d63454f7886d651ac
cd ../wt-m2b-r008
GOTOOLCHAIN=local go test ./... -count=1                               # 65 ok / 0 FAIL
GOTOOLCHAIN=local go test ./internal/hestia/sheets -count=1 -coverprofile=/tmp/c.out \
  && GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | grep -E 'push\.go|total:'   # 六函数 100%，94.3%
# N7 的对照：模板 id 必须**不是**最大值，两个算法才分叉
GOTOOLCHAIN=local go test ./internal/hestia/sheets -count=1 -v \
  -run 'TestCreateYearTabDerivesNewIDFromMaxNotTemplate|TestPushPlacesTwoNewTabsCumulatively'
```

## 判定落盘后的补验（Leader 在我裁决之后追问的四点）

我在 04:17:46Z 转 `verified`，Leader 的两条派验消息到得更晚，其中第二条提出了四点我判定时**没有专门验**的内容。补验结论：**已落盘的判定不受影响，四点全部支持它**；但其中一条 dev 给的**理由**经实测不成立，记在这里。

### ① `891a95f`（simplifier 那笔）的断言集合差

Leader 更正了 provenance：`042c739` 这个 merge **含两笔不是一笔**（`ba6c589` + `891a95f`），而因为后者只改了前者新增的那些行，合并后相对 `b53708f` 的净差仍是 `92 增 0 删` ⇒ `git show --numstat HEAD` 在两个不同对象上给出**相同输出**，那个检查在这一对上恰好没有区分力。

我用集合差复核 `ba6c589 → 891a95f`：

| 项 | 结果 |
|---|---|
| 断言**消失** | **0 条** |
| 断言**新增** | **0 条**（集合完全相同） |
| Test 函数 | 22 → 22 |
| 注释行 | 113 → 113 |

⇒ simplifier 那笔断言零弱化，独立证实。（条数我按「行首是断言语句」算得 94，dev 与 Leader 报 95，是「行内包含 `require.`/`assert.`」口径，差 1 为注释里提到断言名的行；**集合差为空这个结论与口径无关**。）

### ② 常量替换的实质

`pathHeader26` 出现 **4** 次、`pathEntry26` 出现 **3** 次，合计 **7** 行；手写的那两个路径字面量只剩 **2 处**，就是第 35、36 行的**常量定义本身** ⇒ 与 Leader 所核一致。

### 🔴 ③ dev 给这次替换的理由，经实测不成立

dev 的理由是：

> 手写字面量若与夹具表名脱钩 ⇒ `failPath` 注不进去 ⇒ 请求不再 4xx ⇒ `Push` 不再报错 ⇒ **用例仍然绿，只是绿的理由变了**。

我造了变异 **W1** 直接测这件事：把 `TestPushStopsWhenReadHeaderFails` 的 `failPath` 键从 `pathHeader26` 换成一个与夹具表名脱钩的手写字面量（`'2025年'!A3:AI3`，而夹具里只有 2026年）。

**结果：用例当场变红** —— `An error is expected but got nil`。

成因是这六个用例**每条都带 `require.Error`**：脱钩之后注入的 4xx 落在一个不会被请求的路径上，`Push` 顺利跑完不报错，于是 error 断言立刻红。⇒ **那个失效不是静默的，是会响的。**

**结论对、理由错**：把字面量换成常量本身是好的（消除重复、单一真相源、上游改表名时一处改全处跟），但它防的**不是** dev 所说的「静默失效」。这一条与我在 TASK-009 复验里指出的 `"2025-6"` 是同一族：结论正确会让判断永不被复查，而理由是别人复现时唯一的入口。

### ④ 「一个写请求都不许发」这条断言没有恒真

Leader 提醒别让 `[1]` 里刚处理掉的恒真陷阱在 `[3]` 复活。六个用例**每条都有**这条断言（各 1 条），且都配了 error 断言。

要证明它**不恒真**，光有「变异后它会红」不够——那六个变异（吞掉错误继续往下走）会让 error 断言与零写断言**同时**红，分不清是谁在守。所以我造了变异 **X1**：让 `Tabs` 失败时**仍然报错**（error 断言保持满足），只是在返回前多发一次写请求。

**结果 KILLED**，红的正是零写断言：

```
Error:    Should be empty, but was [{"data":[{"range":"'2026年'!A4","values":[["x"]]}],"valueInputOption":"R…
Messages: 一个写请求都不许发
```

⇒ **零写断言有独立鉴别力**，恒真陷阱没有复活。

### ⑤ 覆盖率门禁口径

`TASK-008.coverage_floor` 为 `null` ⇒ 走全局 `coverage.dev_minimum` = **80**（不是「没有地板」）。按门禁口径（`packages` = `./internal/hestia/sheets`）实跑：

```
total: 94.3%   ⇒ 94 >= 80，通过
```

## 🔴 我这一轮漏掉的一条：假溯源注释，而我手上早有能证伪它的数据

test-m2b-c 在 006 复验里发现 `push_test.go` 顶部的映射注释写着：

```
// [1] 恒真断言 + push.go:177-179 —— 已在 5af1701 做掉（TestPushStopsWhenNewTabDiffFails）
```

**而 `TestPushStopsWhenNewTabDiffFails` 守的是 182-184，不是 177-179。** 它发现于 04:19:11Z，我在 04:17:46Z 已判完 VERIFIED，早 85 秒——那一轮拦不下。Leader 的处置是不为一行注释重开 008，把它折进 006 第二轮（该文件在 006 的 `writes` 里）。

**我事后核了，它是对的**：

| push.go 行段 | 实际内容 | 我的变异结果 |
|---|---|---|
| 177–179 | `if err := createYearTabs(…); err != nil { return res, err }`（**建表本身**失败） | **U3** ⇒ KILLED by `TestPushStopsWhenCreateYearTabFails` |
| 182–184 | `changes, err := diffTab(…); if err != nil { return res, err }`（新表**补做 diff** 失败） | **U4** ⇒ KILLED by `TestPushStopsWhenNewTabDiffFails` |

⇒ **我自己跑出来的 U3/U4 结果，恰好就是那条注释的反例。** 我把它们当作「守卫在不在」的证据用掉了，却没有回头把「哪条测试守哪一段」与文件顶部那张映射注释对照一遍。

更值得记的是：**同一个文件里另一条注释（第 569 行）写着「补做 diff 时失败（push.go:182-184），这条是建表本身失败（:177-179）」——它与第 520 行自相矛盾。** 把两条注释并排读就能发现，不需要跑任何东西。

**这次遗漏的形状**：不是「没有证据」，而是**证据到手后没有做那一步对照**。我验的是「性质有没有守卫」，而注释的溯源准确性是另一个问题；两件事用的是同一批数据，我只回答了前一个。⇒ 判据补充：**跑完变异后，把「谁杀了谁」这张实测映射与交付里自称的映射注释对一遍**——前者是观察，后者是声称，它们本就该互相校验。
