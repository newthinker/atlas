# Sprint M2b · 第一轮 Code Review（常规）

- 审查者：qa-m2b
- 对象：master `d42435241b3c7a24c10d2b016db17c364b325631`，起点 `6297fee`，29 文件 / +4199 / −70（不含 `.arcforge/`）
- 基线自跑：`GOTOOLCHAIN=local go test ./... -count=1` ⇒ exit 0、**65 ok / 0 FAIL**（本轮实跑，不引用他人数字）

## 结论：**REJECT**

一条 CRITICAL：`values.get` 依赖 Google API 的默认渲染选项，而该默认返回**格式化后的文本**，
比对链路无 string 分支 ⇒ 幂等比对在真表上可能完全失效。修复是一行代码加一条测试，
而不修的代价是真表验收（判据四）几乎注定失败，且失败形态是「每次 apply 重写上千格」这种
烧配额且难归因的形态。另有 2 条 WARNING、5 条 SUGGESTION。

**能力当前在生产上禁用**（`configs/hestia.yaml` 里 `hestia_sheets` 命中 0），所以没有正在发生的损害。

---

## 一、CRITICAL-1：`values.get` 未指定 `valueRenderOption`，幂等比对可能全面失效

**位置**：`internal/hestia/sheets/client.go:116-123`（`read`）、`:235-245`（`readTitle`）

```go
vr, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
```

### 证据链（每一条都是我自己跑出来的观察，不是推理）

| # | 事实 | 求证方式 |
|---|---|---|
| 1 | 全仓 **0 处**设置 `ValueRenderOption` | `grep -rn "ValueRenderOption" --include="*.go" internal/ cmd/` ⇒ 空 |
| 2 | 该参数的默认值是 `FORMATTED_VALUE` | Google 官方生成库 `sheets-gen.go:11695-11696` 字面写 `The default render option is FORMATTED_VALUE.` |
| 3 | `FORMATTED_VALUE` 返回**格式化后的显示文本** | 同处文档的例子：`A1` 是 `1.23`、`A2` 是 `=A1` 且设为货币格式 ⇒ `A2` 返回 **`"$1.23"`**（带引号即字符串）。`UNFORMATTED_VALUE` 才返回数字 `1.23` |
| 4 | `ValueRange.Values` 是 `[][]interface{}` | `sheets-gen.go:9562`；JSON 解码后字符串成 Go `string`、数字成 `float64` |
| 5 | `toFloat` **没有 string 分支** | `diff.go:111-123`，只有 float64 / float32 / int / int64 |
| 6 | `toFloat` 失败 ⇒ `sameValue` 退化为字符串比较 | `diff.go:100-108`：`return fmt.Sprint(a) == fmt.Sprint(b)` |
| 7 | 年度表**带格式** | `client.go:159-163` 自述：复制模板「自带 54 列表头、单位行、AJ–BB 的 19 个公式**与格式**」 |
| 8 | 全部测试替身返回 **JSON 数字** | `push_test.go:65` 的 `999.0`/`300.0`、`client_test.go:192` 的 `412.5` |
| 9 | `sameValue` / `toFloat` **零直接测试** | `grep -n "sameValue\|toFloat" internal/hestia/sheets/*_test.go` ⇒ 空 |
| 10 | 真表验收（判据四：三类计数）**从未跑过** | `CONTRACTS.md:4158` 标 `_（待人回填）_`，`TASK-011-verification.md:96` 确认判据四～七留空是对的 |

### 后果

表里任一数值列设了格式（千分位、固定小数位、百分比、会计负数），该列的现值读回来是格式化文本，
与库值的 `fmt.Sprint` 不相等 ⇒ 每格判 `WillWrite`。实测比对：

| 单元格格式 | 表显示 | `fmt.Sprint(库值)` | `sameValue` |
|---|---|---|---|
| Automatic（无格式） | `255800` | `255800` | true |
| 千分位 | `255,800` | `255800` | **false** |
| 两位小数 | `255800.00` | `255800` | **false** |
| 百分比（同比列） | `10.70%` | `10.7` | **false** |
| 三位小数 | `251.310` | `251.31` | **false** |
| 会计负数 | `(933)` | `-933` | **false** |

⇒ 幂等性失效：每次 `--apply` 重写全部数值格；dry-run 的「一致跳过 N 格」对这些列恒为 0；
C5「只写变化的格」的意图落空。1282 格的写入量每轮重复，对 Sheets API 配额是持续压力。

**这条性质在现有测试里结构性不可观测**——所有替身都返回 JSON 数字，真 API 返回什么从未被断言过，
也没有一行注释记录过这个假设。这与本 sprint 已记录的三条「声称为真但无测试守卫」是同一族，
但危害更大：那三条是「性质为真而无守卫」，这条是**假设可能为假而无守卫**。

### 修复清单

1. `client.go` 的 `read()` 与 `readTitle()` 各加 `.ValueRenderOption("UNFORMATTED_VALUE")`。
   保持 `WriteCells` 的 `RAW` 不变（写入侧口径已正确）。
2. 补一条测试：替身对 `values.get` 返回**字符串**形态的数值（如 `["1月","2025-02-14","412.50"]`），
   断言 `Diff` 把它判成 `Same` 而非 `WillWrite`。这条测试在修复前必须红。
3. 给 `sameValue` / `toFloat` 补直接单测，覆盖 string 输入。
4. 在 `diff.go` 的 `sameValue` 注释里写明「current 的类型取决于 `valueRenderOption`，
   本包要求调用方用 `UNFORMATTED_VALUE`」，让假设成文。

### 我在这条上的自我订正（必须留痕）

我第一版的论证用了**自己编的数值** `4305000`，据此得出「大额列因 `fmt.Sprint` 输出科学计数法
`4.305e+06` 而永不匹配」，一度要写成 CRITICAL 的主论据。**用真库 `data/hestia.db` 复算后被证伪**：
真库 2019-12/annual 那行的 21 个 REAL 值（最大 `tsf_flow_ytd = 255800`）`fmt.Sprint` **全部输出十进制、
零个科学计数法**。二分实测得出切换阈值恰是 **1,000,000**（`999999` ⇒ `"999999"`、`1000000` ⇒ `"1e+06"`）。

⇒ 科学计数法这条降级为 SUGGESTION-3（当前不触发、数据增长会触发）。CRITICAL-1 的成立不依赖它，
只依赖「表里设了格式」这一条，而模板表带格式是代码注释自述的事实。

教训归档：我拿构造的例子去支撑对真实系统的判断，而那个例子的量级不在真实数据的域内。
判据应是「我的样本取自真实数据吗」，不是「我的例子算对了吗」。

---

## 二、WARNING-1：C8 的 panic 防护有缺口，缺口正落在 TASK-009 与 TASK-010 的任务边界上

**位置**：`internal/hestia/ingest.go:483`（调用点）vs `cmd/atlas/hestia_sheets.go:161-165`（recover）

```go
if d.ProjectSheets != nil {
    rows, berr := buildSheetRows(ctx, d.Store)        // ← recover 之外
    ...
    } else if perr := d.ProjectSheets(ctx, rows); perr != nil {   // ← recover 之内
```

生产路径上唯一的 `recover()` 在 `sheetsProjector` 返回的闭包里（`grep -n "recover()" internal/hestia/ cmd/atlas/`
确认 `internal/hestia/` 下 **0 处**）。`buildSheetRows` 在闭包**外**调用，它的 panic 会穿透
`ingestOne` → `Ingest` → 打断整轮入库——**正是 C8 存在的目的**。

panic 面：`sheets_project.go:127-128`
```go
year, _ := strconv.Atoi(obs.Meta.Period[:4])
month, _ := strconv.Atoi(obs.Meta.Period[5:7])
```
定长切片，`Period` 短于 7 字符即 panic。可达性：`Meta.validate()`（`types.go:226`）校验
`\A[0-9]{4}-[0-9]{2}\z`，但那是 **Save 时**校验，`buildRow` 走的是**读路径**，不重新校验。
ADR-0003 的写口守卫只约束 Go 代码，不防迁移脚本 / 手工 SQL / 旧版本写入的行。

**这是跨任务视角才看得见的缺口**：TASK-010 的验证者发现「`ProjectSheets` 的 panic 会穿透 Ingest」，
Leader 把义务转给 TASK-009，TASK-009 在自己的 `writes`（`cmd/atlas`）里加了 recover——
**它够不到 `ingest.go:483`**，那是 TASK-010 的文件。两个任务各自都做对了，合起来仍有半边没兜住。

**建议**：把 recover 上移到 `ingest.go` 的 `if d.ProjectSheets != nil` 块外层（或在块内用一个
带 recover 的小函数包住两步）。这样防护不依赖装配方记得加，且组装与投影两步都被兜住。
配套测试：注入一个必 panic 的 `buildSheetRows` 替身（它已是包级变量 `var buildSheetRows = BuildSheetRows`，
注入缝现成），断言 `Ingest` 不崩且入库成功。

---

## 三、WARNING-2：ingest 自动投影对每期做一次全库组装 + 全量推送，API 调用随候选数线性放大

**位置**：`internal/hestia/ingest.go:482-490`（在 `ingestOne` 内）、`ingest.go:257-259`（`for _, c := range cands` 循环）

`ingestOne` 每期调用一次投影，而每次投影是：
- `buildSheetRows` ⇒ `AllPeriods` + 逐期 `Current`，读**全库**
- `sheetsProjector` 闭包每次 **新建一个 Sheets client**（`newSheetsClient`，含读凭据文件与 JWT 交换）
- `sheets.Push` ⇒ `Tabs` 1 次 GET + 每张年度表 2 次 GET（`ReadHeader` + `ReadEntryArea`）

设年度表 T 张、本轮候选 N 期，则 GET 次数为 `N × (1 + 2T)`。T=8、N=40（空库首跑或 `--force`
翻满 `max_pages`）⇒ **约 680 次 GET + 40 次 token 交换**。Google Sheets API 的读配额是分钟级的，
这个量级会触发 429；而按 C8，429 只会打印一行「投影失败（不影响入库）」，**静默**。

写入量是正确的（第 k 次投影时库里只有前 k 期，后续期次的格在后面的轮次才写），问题只在读放大。

**建议**（任选其一，都不违反 C8）：
- 把投影移出 `ingestOne`，在 `Ingest` 的候选循环**之后**做一次。语义更符合「投影没有下游、可再生」。
- 或保留每期投影，但把 client 提到循环外复用（`IngestDeps` 里存一个 lazily-built client）。

---

## 四、SUGGESTION

1. **`fmt.Sprint` 的科学计数法阈值是 1,000,000**（实测）。真库当前最大值 255800，未触发。
   但 `tsf_flow_ytd` 是亿元单位的年度累计社融增量（2019 年为 255800），随经济增长会上行。
   到 10⁶ 时 `sameValue` 的字符串回退路径会给出 `"1e+06"`。修好 CRITICAL-1 后该路径不再被数值列走到，
   风险随之消失——这是**优先修 CRITICAL-1 的又一个理由**。

2. **dry-run 在缺表场景下系统性低估影响面**。`push.go:171` 的 `!opts.Apply` 短路在建表之前，
   缺表那些年份的行完全不进 `res.Changes` ⇒ `formatResult` 打印的行数与「将写 N 格」只算了已有表。
   `push_test.go:206` 的 `require.Equal(t, 1, res.WillWrite)` 连注释一起把该行为钉死了，
   所以这是**有意识的设计**，不是疏漏。但 dry-run 的全部意义是预判影响面，此处它给的数字会误导。
   建议：在 `formatResult` 的「将新建工作表」那行后补一句「新表的行未计入上述计数」。

3. **依赖增量偏重**。`google.golang.org/api v0.250.0` 带进 18 个新间接依赖，含 gRPC v1.75.1、
   OpenTelemetry 5 个模块、`cloud.google.com/go/auth` 3 个、`golang.org/x/crypto`。
   对一个只用 Sheets REST 的能力而言偏重，但官方库难以避免，属合理权衡。记录事实，不阻断。
   （`spf13/pflag` 从 indirect 升为 direct 是因为测试文件直接 import 它，正常。）

4. **`sheetLabels()` 零直接测试**（`cmd/atlas/hestia_sheets.go:177`，全仓只在两个调用点出现）。
   它把 `hestia.SheetColumns` 的 35 个 Label 抽出来喂给 `ResolveHeader`，是 C3 的输入。
   `SheetColumns` 本身有 `TestSheetColumnsCoverEntryArea` 守长度与首尾，但「抽取不丢不乱序」无守卫。

5. **`sheets_project.go:127-128` 的定长切片建议加读侧防御**。即使 WARNING-1 的 recover 上移了，
   `buildRow` 对畸形 period 返回 `year=0, month=0` 会导致 `Row.Month-1 = -1` 的负索引传进 `Diff`
   （`cellAt` 对负数返回 nil，不 panic，但会产出一条 `Row = 3` 的越界写请求）。
   建议 `buildRow` 改用 `strconv.Atoi` 的错误返回，或在 `assembleRows` 里校验 period 形态。

---

## 五、Leader 交办的 7 条已知清单：逐条处置

### ① 三条「声称为真但无测试守卫」——**移植清单（本轮最高优先级产出）**

三份夹具都在 scratchpad，我已逐条定位到具体断言。**建议以 `review_fix` 派回 dev 移植**：

| 变异 | 性质 | 夹具来源 | 移植到 | 加什么断言 |
|---|---|---|---|---|
| N7 | 新表 `sheetId` 用 `maxID+1`，不与模板撞 id | `test-m2b-b-TASK-008-fixture.go.txt:138-143` | `internal/hestia/sheets/push_test.go` | 关键是**让模板表持有最大 sheetId**（夹具用 `vTplID = 777`，而 dev 夹具用 12345 不是最大值，所以变异不红）。断言 `reqs[0].DuplicateSheet.NewSheetId == 模板id+1` 且 `SourceSheetId == 模板id` |
| N10 | 连建两张时第二张的 `index` 把第一张已插入的位置算进去 | 同上 `:232-248`（`TestW_PushPlacesTwoNewTabsCumulatively`） | 同上 | 现有 `{说明,2024年,2026年}` 缺 `{2023年,2025年}` ⇒ 断言两次建表的 index 序列 `require.Equal(t, []int64{1,3}, ...)`。忘了更新本地表序会得 `{1,2}` |
| P16 | `--period-type` 无默认值 | `test-m2b-b-TASK-009-fixture.go.txt:386` | `cmd/atlas/hestia_sheets_test.go` | `require.Equal(t, "", cur.Flags().Lookup("period-type").DefValue)`。**必须读 `DefValue` 而不是运行时变量**——`sheetsExec` 的 `t.Cleanup` 把 flag 全局变量重置成零值，cobra 注册的默认值早被抹掉，这条性质在运行时变量上结构性不可观测 |

移植时需一并带上夹具的辅助函数 `vTabs` / `vResp` / `vNewTabIndexes`（`:28-36`、`:112-120`），
它们是结构化取值（把 batchUpdate 请求体解析成结构逐字段核），比字符串 `Contains` 强。

### ② 两条「变异被杀但不是被断言杀」——我的判断：**P3 不必补，M6 已有守卫，M11 可补**

- **P3**（删掉整个 `defer recover`）：**不必补**。recover 不存在时，注入 panic 的那三条测试
  （`TestSheetsProjectorRecoversClientPanic` 等）必然崩溃 ⇒ 必然红，检出是确定性的。
  而语义部分（「转成 error 而非吞掉」）已由 P2 的 4 条断言守住，其中
  `require.Contains(t, err.Error(), "boom in client")`（`hestia_sheets_test.go:394`）
  钉住了「原始 panic 值留在错误里」。两条合起来性质完整。
- **M6**（去掉 `d.ProjectSheets != nil` 判断）：**已有守卫，Leader 清单可以划掉**。
  我原本担心「把 `sheetsProjector` 改成返回非 nil 空闭包」会静默穿透 C9，查证后发现
  `TestSheetsProjectorNilWhenCredentialsEmpty`（`hestia_sheets_test.go:352-354`）用
  `require.Nil(t, sheetsProjector(hestia.Config{}))` 正面守住了返回字面量 nil。
  C9 的两半都有东西守：`cmd/atlas` 侧靠这条断言，`internal/hestia` 侧靠 nil 调用 panic。
- **M11**（组装挪到 nil 判断之外，Leader 在 plan.md:121 提过）：**这条才是真缺口**。
  010 discovery `key_findings[2]` 声称「nil 时连 `buildSheetRows` 都不调用」为真而无守卫，
  变异后行为差异只是多一次全库读，四项 C9 语义内不可区分。
  可补：`buildSheetRows` 已是包级变量（`ingest.go:492`），注入一个计数替身，
  断言 `ProjectSheets == nil` 时调用次数为 0。成本一条测试。

### ③ `packages` 口径缺口——**确认属实，建议进 final-report 而非本轮修**

TASK-003/005/006/007/008 的 `writes` 含 `internal/hestia/store_test.go`（父包），而 `packages`
只有 `./internal/hestia/sheets` ⇒ `dev_done` 门禁的 `go test <packages>` 结构性跑不到写口守卫。
本轮无实害（每个 dev 单独跑过、验证者复跑并用变异 N15 证明守卫在守）。
这是**拆任务时的声明口径错**，不是代码缺陷，改任务文件已无意义（全部 `verified`）。
Leader 已给出正确规则：「凡 `writes` 触及某包的文件，`packages` 就该把该包列上」。

### ④ 假绿的 `-run <pattern>` 命令——**交付物里 0 条同类，已清**

我搜了全仓 `-run` 命令并逐条判断来源：
- **本 sprint 交付的 29 个文件里 0 条 `go test -run` 命令**（`git diff` 的 `+` 行里只有两处提到
  `go test ./...` 与 `go test ./cmd/atlas/`，都不带 `-run`）。
- 验证报告里有 2 条 `-run`，**都用完整函数名**，我实跑核实两条都命中目标：
  `TestExampleConfigDeclaresHestiaRules` ⇒ `=== RUN` 列表命中、PASS；
  `TestSelectRowsCollapsesSeventySevenToSixtyOne` ⇒ 同样命中、PASS。
- 其余 `-run` 全在 `docs/plans/` 的历史文档里，不属本 sprint 交付。
- `CONTRACTS.md:3593` 的 `-run TestEvaluateGolden -v` 是 M2b 之前的内容，不在本次 diff。

⇒ Leader 那条假绿命令只存在于派验消息（已被验证者当场纠正并记入
`TASK-011-verification.md:56`），**没有进入任何交付物**。

### ⑤ 陈旧注释——**属实，建议随 review_fix 一并改**

`internal/hestia/sheets/push_test.go:23` 的 DoD 映射仍指向 008 已删除的
`TestPushCreateSheetsHookRunsBeforeWrite`。全仓 `grep` 该符号名命中 **1 处**，就是这条注释。
替换它的是 `TestPushCreatesMissingTabsBeforeWriting`（`push_test.go:338`）。
建议把该行改为指向新测试并注明「007 的占位测试已由 008 真接线替换」。

### ⑥ TASK-010 顺序用例的 glob 未排除侧车——**属实**

`internal/hestia/ingest_test.go:1756` 用 `filepath.Glob(.../pending/*.json)`，而 M3 的侧车
`.history.json` 同样以 `.json` 结尾、同样落在 `pending/` ⇒ glob 会同时命中侧车。
这削弱了「契约已落盘」这个判据：若侧车先于契约写、而投影挪到两者之间，glob 仍非空 ⇒ 假绿。
Leader 已记 M7 在 dev 用例下不红、靠验证者的第二重判据（stdout 已出现 `contract →`）才钉住。
**建议**：把 glob 改为排除侧车，或直接改用 `os.Stat` 查那个确切的契约文件名。

### ⑦ `CONTRACTS ③` 行号略偏——**属实，偏 3 行**

`CONTRACTS.md:4094` 写 `sheets_project_test.go:113-121`。实际：注释块起于 111、
`func` 在 **116**、闭括号在 **122**。写的范围既不是函数体也不是含注释的完整块。
不构成误导（测试名是对的，可直接搜到），建议顺手改为 `111-122` 或 `116-122`。

---

## 六、独立审查：C1–C11 与接口边界

| 检查项 | 结论 | 求证 |
|---|---|---|
| C2 子包不 import `database/sql` 或父包 | **守住** | 逐文件读 import 块：`client.go` 只有 context/errors/fmt/net/http/os/regexp + googleapi/option/sheets；其余三文件只有标准库。注：`go list -deps` 会显示 `database/sql/driver`，那是 google api 的传递依赖，不是直接 import，别用 `-deps` 当 C2 的判据 |
| C6 程序只碰 A–AI 这 35 列 | **守住，且是结构性守住** | `ReadHeader` 只读 `A3:AI3` ⇒ header 数组最多 35 项 ⇒ `ResolveHeader` 返回的列号 ≤ 34 ⇒ `WriteCells` 不可能写到 AJ 之后（那里是 19 个公式）。这条比断言更强，因为它靠范围限制而非检查 |
| 35 列映射唯一性 | **无重复** | 用 python3 解析 `SheetColumns` 字面量：35 条目 / 35 唯一 Label / 33 唯一 Field，零重复。（我第一次用 `grep -o` 数得「35 列却只有 12 个唯一标签」，自相矛盾——那是 `grep -o` 在 CJK 上的已知坏法，换 python3 重数） |
| C9 能力禁用 | **守住** | `NewClient` 空凭据返回 `(nil, nil)`；`sheetsProjector` 返回字面量 nil 并有 `require.Nil` 守卫；CLI 侧打印固定一句并**退出码 0** |
| 错误信息保真 | **好** | `wrapErr`（`client.go:147-153`）对 `googleapi.Error` 附 HTTP 状态码与状态文本，4xx/5xx 不被吞成一句「失败」 |
| 凭据不泄漏 | **未见泄漏** | 错误信息里出现的是凭据**路径**（`client.go:53`），不是内容；`spreadsheet_id` 不进任何日志或契约 |

---

## 七、给 Leader 的建议

建议以 `review_fix` 派回 **TASK-006（client.go 的归属任务）** 或新建修复任务，
`reason_class=task_defect`，`fix_items` 为：

1. `client.go` 的 `read()` / `readTitle()` 加 `.ValueRenderOption("UNFORMATTED_VALUE")`，
   并补一条「替身返回字符串形态数值 ⇒ `Diff` 判 Same」的测试（修复前必须红）。
2. `sameValue` / `toFloat` 补直接单测，覆盖 string 输入。
3. 把 C8 的 recover 上移到 `ingest.go` 的投影块外层，覆盖 `buildSheetRows`，
   并注入必 panic 的 `buildSheetRows` 替身验证。
4. 移植三条夹具断言（N7 / N10 / P16），见第五节①的表。
5. 改 `push_test.go:23` 的陈旧注释、`ingest_test.go:1756` 的 glob、`CONTRACTS.md:4094` 的行号。

第 1、2 条是 REJECT 的根据；3、4、5 是同批顺手做掉的。
WARNING-2（API 放大）与各 SUGGESTION 可进 final-report 的已知项，不必本轮修。
