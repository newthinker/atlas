# TASK-006 验证报告（Sprint M2b · QA 返工第 3 轮）

- **验证者**：test-m2b-c
- **判定对象**：`verify_baseline.head = 6ed3984cf89733aa457e4405060675d3394e46bf`
  ——与派验时的 master 逐字相同，**本轮无漂移**，不需要 `--ack-drift`。
- **取证树**：`git worktree add --detach ../wt-006r3 6ed3984cf89733aa457e4405060675d3394e46bf`（收尾已拆）。
- **判定范围**：**仅 `fix_items[1]`**。上一轮判 REJECT 时我写明「`[1]` 之外的一切都已达标，不要动」，
  dev 照做了——本轮恰 5 个文件、其余五条一字未动，故**不重验已 PASS 的五条**。
- **结论：VERIFIED**，附一条**未阻断的已知缺口**（第 4 节，建议后续补一条测试）。

---

## 0. 基线数字（全部我自己重跑）

| 项 | 实测 | 与 leader 所报 |
| --- | --- | --- |
| `go test ./... -count=1` | **65 ok / 0 FAIL** | 一致 |
| `internal/hestia/sheets` | **95.3%** | 一致 |
| `cmd/atlas` | **78.0%** | 一致 |
| `internal/hestia` | **96.6%** | 一致 |
| `go vet ./...` | 空 | 一致 |
| 声明 `writes` 内 gofmt | 空 | 一致 |

**改动范围自核**：`git diff --numstat 96272b9..6ed3984` ⇒ 恰 5 个文件
（`cmd/atlas/hestia_sheets.go` 9/0、`cmd/atlas/hestia_sheets_test.go` 13/0、`diff.go` 13/2、
`push.go` 25/7、`push_test.go` 35/0），**全部落在声明 `writes` 内**。

> ℹ️ **一条不算缺陷的观察**：`gofmt -l cmd/atlas/` 并非空，命中
> `backtest_test.go` 与 `crisis_test.go`。我核过：两者**本轮未改**
> （`git diff --numstat 96272b9..6ed3984 --` 对这两个文件为空），在 `96272b9` 上就已不合 gofmt
> （最后改动者是 `d27ca9d` / `f5d7b82`，均非本 sprint），且**不在 006 的 `writes` 内**。
> ⇒ 不计入本轮判定。但把 `./cmd/atlas` 加进 `packages` 之后，这个包的存量 gofmt 违规
> 就落进了 006 的声明覆盖口径里，值得该包的维护者知道。

---

## 1. `fix_items[1]` 三步逐步核

我上一轮给的路线 A 有三步，**三步都做到了**：

| 步 | 要求 | 交付 | 判定 |
| --- | --- | --- | --- |
| ① | 修订 `Diff` 注释，使契约与代码一致 | 改成有条件的恒等式：`len(out) == len(合法行) × len(cols)`，并显式写出「**不是** `len(rows) × len(cols)`」 | **PASS** |
| ② | 给跳过加可观测出口，`Diff` 签名不动 | `Result.DroppedCells`；由 `diffTab` 算、`Push` 逐张累加；`Diff` 签名确实未动（改的是 `diffTab` 的返回元组） | **PASS** |
| ③ | 补测试 | `TestPushReportsDroppedCellsWhenMonthOutOfRange`（含反空洞与新不变量断言） | **PASS** |

**②的做法我核过算术**：`diffTab` 返回 `len(rows)*len(cols) - len(changes)`。
`Diff` 对每个合法行 × 每列恰产出一条 ⇒ 差值 `= (len(rows) − len(合法行)) × len(cols)`，
正是被丢弃的格数；且 `Diff` 不可能**多**产出，故该值恒 ≥ 0。算术正确。

**①还多做了一件我没要求的事**，值得记下来：注释里把「这条不变量被收窄过一次而文字没跟着改」
本身写了进去，并给出通则——「收窄不变量时，宣称它的那段文字和依赖它的那段代码必须一起改，
否则文档本身会变成下一条假溯源」。这把一次性的订正变成了可复用的约束。

---

## 2. 你点名的两处「比要求更强」——我独立判，两处都对

### 2.1 判据取**性质**不取**成因**：对

`DroppedCells` 算的是「期望 − 实得」，不是「数几行越界」。dev 的理由是
「将来出现第二种让 `Diff` 少产出的情形，这个字段同样会亮」。

**我认同，而且这不只是风格问题**：数成因的实现要在 `Diff` 内部数越界行，
那就得改 `Diff` 的签名或加出参——而「`Diff` 签名不动」正是路线 A 的约束之一。
取性质则天然落在 `diffTab` 这一层，因为**只有那一层同时握着 `rows`、`cols` 与结果**。
两个约束在这里是同一个解，不是巧合。

我用变异验证了它确实钉的是性质而非成因：**P2**（把公式写成 `len(rows) - len(changes)`，
漏乘列数）⇒ **KILLED**。说明这个算式本身被断言精确守住，不是随便一个非零值都能过。

### 2.2 `tally` 刻意不碰 `DroppedCells`：**dev 是对的，你的提醒会引入缺陷**

你提醒「加了新字段要同步 `tally`」，dev 的结论与你相反。**我独立推演 + 变异，结论站 dev 这边。**

机制是这样：`tally` 每次调用都 `res.WillWrite, res.Same, res.AbsentInDB = 0, 0, 0` 再从
`res.Changes` **全量重算**。而 `Push` 的调用序列是：

```
① present 循环：res.Changes += …；res.DroppedCells += dropped
② toWrite := tally(&res)
③ missing 循环：res.Changes += …；res.DroppedCells += dropped
④ toWrite = tally(&res)          ← tally 被第二次调用
```

**被丢掉的格根本不在 `res.Changes` 里，`tally` 重算不出来。** 所以若 `tally` 也把
`DroppedCells` 清零：②会抹掉①的累计，④会抹掉③的累计，**该字段将恒为 0**——
即这个功能会完整地、静默地失效。

**变异实测 N3**（在 `tally` 里加 `res.DroppedCells = 0`）⇒ **KILLED**，
红的正是 `TestPushReportsDroppedCellsWhenMonthOutOfRange`。
⇒ dev 不仅判断对，还**为这条与 leader 相反的判断单独立了守卫**，使它不再依赖任何人记得。

---

## 3. 两列并排表（左 = 我独立变异实测，右 = 交付自称）

表述用「测试函数名 + 被守代码块特征」，行号只作辅助。**左列是我自己跑出来的，不是读交付读来的。**

| # | 被守的代码块（函数 + 关键语句） | **左列：我的实测（观察）** | **右列：交付自称** | 比对 |
| --- | --- | --- | --- | --- |
| N1 | `diffTab` / `return changes, len(rows)*len(cols)-len(changes), nil` | 改成恒返 0 ⇒ 只红 `TestPushReportsDroppedCellsWhenMonthOutOfRange` | 同 | ✅ |
| N2 | `Push` / present 循环的 `res.DroppedCells += dropped` | 不累加 ⇒ 只红 `TestPushReportsDroppedCellsWhenMonthOutOfRange` | 同 | ✅ |
| N3 | `tally` / **不**碰 `DroppedCells` | 加 `res.DroppedCells = 0` ⇒ 只红同一条 | 同 | ✅ |
| N4 | `formatResult` / `if res.DroppedCells > 0 ⇒ Fprintf(未比对)` | 删掉打印 ⇒ 只红 `TestFormatResultAnnouncesDroppedCells` | 同 | ✅ |
| N5 | `Diff` / `r.Month<1 \|\| r.Month>12 ⇒ continue`（回归） | 删钳制 ⇒ 红 `TestDiffSkipsRowsWithMonthOutOfRange` **与** `TestPushReportsDroppedCellsWhenMonthOutOfRange` 两条 | 同 | ✅ |
| **P1** | `formatResult` / 打印的**非零条件** | 改成恒打印 ⇒ 红 `TestFormatResultAnnouncesDroppedCells` | 交付表无此行 | ⚠️ 我补测，守得住 |
| **P2** | `diffTab` / 丢格**算式**的精确性 | 漏乘列数 ⇒ 红 `TestPushReportsDroppedCellsWhenMonthOutOfRange` | 交付表无此行 | ⚠️ 我补测，守得住 |
| **P3** | `Push` / **missing 循环**的 `res.DroppedCells += dropped` | **🔴 SURVIVED（0 条红）** | 交付表无此行 | 🔴 **未被守住**，见第 4 节 |

**交付表 5 行，我逐行独立复现，5 行全部相符。** 我另补 3 行，其中 2 行守得住、**1 行未守住**。

### 关于「左列是不是抄的」

本轮交付表左右列**全部一致**，所以上一轮那个「分歧只能由真跑产生」的结构性证据这次不适用。
我的判据换成：**我独立跑出了交付表里没有的第 6–8 行，其中 P3 与交付自述相反**
（交付称五个变异全 KILLED，我这里有一个 SURVIVED）。
⇒ 若交付的左列是抄右列而来，它不可能在 N1–N5 五行上与我逐行吻合；
而它在我发现 P3 之后仍与我在那五行吻合，说明那五行确是真跑。
**P3 不属于交付声称的五个之列，它是我扩大变异集后才出现的**，不构成对交付表真实性的反证。

---

## 4. 🔴 一条未阻断的已知缺口：`missing` 循环的累加无守卫

**观察**：把 `Push` 第 6 步（建表之后那个 `for _, name := range missing` 循环）里的
`res.DroppedCells += dropped` 改成 `_ = dropped`，**整个 `sheets` 包与 `cmd/atlas` 全绿，
变异存活**。

**这一行的覆盖块计数是 `push.go:202.4,203.31 = 1`——它被执行过。**
⇒ 这是一个**「覆盖是绿的、守卫是空的」**的标准实例：既有用例（如
`TestPushCreatesMissingTabsBeforeWriting`）确实走过这一行，但走过时 `dropped` 恒为 0，
所以**没有任何断言能区分累加发生与否**。

> 这恰好是本 sprint 那条方法论的下一级：`fix_items[8]` 说「覆盖率百分比不能替代对指名块的
> 逐块核对」；这一例说明**逐块计数也不能替代变异**——块计数 1 与守卫为空可以同时成立。

**为什么我判 PASS 而不是 REJECT**（这是我的判断，写明理由供你复核）：

1. **代码本身是对的，两条路径都写了累加**，缺的是一条测试，不是一段实现。
   这与上一轮不同——上一轮缺的是整个「计数」要求，外加一条**内容为假**的导出函数注释。
2. **我上一轮给的路线 A 第 3 步原文是「补一条测试：越界行进入 `Push` ⇒ `Result` 里能看到
   它被跳过了」**。那条测试存在且有效。P3 是我**超出自己设定的验收线**扩大变异集后才找到的。
   拿一条我没提出过的要求去否决按我的要求做完的交付，不公平，也会让验收线变得不可预期。
3. **可达性**：触发它需要「`Apply` + `CreateSheets` + 缺表 + 新表里恰有越界月份的行」，
   而生产路径上 `sheets_project.go:130` 的 `periodYearMonth` 已校验 `month ∈ [1,12]` 并报错，
   越界行根本到不了 `Diff`。这是**纵深防御之上的纵深防御**。

**建议的后续处置（约 10 行测试，不必单开一轮）**：给
`TestPushReportsDroppedCellsWhenMonthOutOfRange` 加一个
`Options{Apply: true, CreateSheets: true}` 且缺表、并让**新表**那一档里含越界月份行的用例，
断言 `DroppedCells` 把新表那部分也算进去了。折进任何一轮后续返工即可。

---

## 5. 跨任务副作用：TASK-009 的守卫仍在，**我的结论是不必重验**

你提的结构性问题是真的：`cmd/atlas` 现在与 009 被验时不是同一棵树，而 `verify_baseline`
只覆盖各自的声明范围，**没有任何机制会因此告警**。所以我做了人工替代核查——
按我自己的方法论，**不看包级 78.0% 这个数字**，而是回去读 009 的 `done_criteria` 逐条验性质：

| 009 的判定性质 | 现树上的核查 | 结论 |
| --- | --- | --- |
| `formatResult` 三类计数**分开三行**各带数字 | `TestFormatResultSeparatesThreeCounts` **PASS**；新增那行是**追加的第 4 行**且条件为 `DroppedCells > 0`，不动原三行 | 仍成立 |
| 默认 dry-run，**末尾固定一句** `未写入任何内容；确认后加 --apply` | 🔴 这是唯一真有顺序风险的一条，我写探针实测：`DroppedCells=8` 时输出**末行仍是** `"dry-run：未写入任何内容；确认后加 --apply"`。机制上也稳：`sheetsDryRunLine` 由调用方在 `hestia_sheets.go:156` 打印，而新增行在 `formatResult` 内部（:154 先打） | 仍成立 |
| `credentials_file` 留空 ⇒ 打印未配置并退出 0 | `TestSheetsPushPrintsDisabledAndExitsZero` PASS | 仍成立 |
| flag 校验在开库与建客户端之前（三种错法） | `TestSheetsPushPeriodNeedsType` / `TestSheetsPushRejectsMalformedPeriodFlags` PASS | 仍成立 |
| 投影 panic 不得打断整轮 ingest | `./cmd/atlas` 全包 `ok`，无 FAIL | 仍成立 |
| `cmd/atlas` 覆盖率 ≥ `coverage_floor` | 009 的 `coverage_floor` 是 **77**，现测 **78.0%** | 达标 |

**⇒ 不必重验 009。** 理由是三条合取：改动对 `cmd/atlas` 是**纯追加**（9/0 与 13/0，
删除行数为 0 ⇒ 既有断言不可能被删改）；新增输出**条件挂在一个在 009 全部夹具里恒为零值的字段上**；
009 的每条具名判据我都在现树上逐条实测仍成立（含唯一有顺序风险的那条，用探针证了）。

⚠️ 但请把这次记成**人工补位**而不是「机制没问题」：这次结论是绿的，靠的是有人想起来去查。
换一个不知道 009 存在的验证者，或者换一个**有删除行**的改动，同样的盲区会给出同样安静的绿。

---

## 6. 变异清单与自证

全部变异在我自己的 detached worktree（`../wt-006r3` @ `6ed3984`）内，主工作区 `internal/`
与 `cmd/` 一字节未碰。**10 个变异：8 KILLED、1 SURVIVED、2 判无效（同一对锚点的两次尝试）。**

| 变异 | 内容 | 结果 |
| --- | --- | --- |
| N1 | `diffTab` 恒报 0 丢格 | KILLED（1） |
| N2 | present 循环不累加（首版：留下未使用变量） | **编译闸命中 ⇒ 判无效，不记 KILLED** |
| N2′ | 同上，改 `_ = dropped` 过编译 | KILLED（1） |
| N3 | `tally` 清零 `DroppedCells` | KILLED（1） |
| N4 | dry-run 不打印丢格 | KILLED（1） |
| N5 | 去掉月份钳制（回归） | KILLED（2） |
| P1 | 丢格行改为恒打印（去掉非零条件） | KILLED（1） |
| P2 | 丢格算式漏乘列数 | KILLED（1） |
| P3 | missing 循环不累加（首版：未使用变量） | **编译闸命中 ⇒ 判无效** |
| P3′ | 同上，改 `_ = dropped` 过编译 | **🔴 SURVIVED（0 条红）** |

**自证四条**：

1. **编译闸**：变异体落盘后先 `go vet`（覆盖 `./internal/hestia/sheets/` 与 `./cmd/atlas/`），
   非空输出即判「变异无效」，harness 固定打印「**不是 KILLED**」。N2/P3 两次都是这样被挡下的，
   改对后重跑才取得结果——**被弃的运行不进表**。
2. **无测试变红时** harness 固定打印「**🔴 SURVIVED，不是 KILLED**」，不与 KILLED 混淆。
3. **文件完整性**：变异前后对 5 个文件 `shasum -a 256` 比对 ⇒ **IDENTICAL**；
   worktree `git status --porcelain` 为空。两个探针（`Diff` 顺序探针、`cmd/atlas` 末行探针）均已 `rm`。
4. **主工作区未污染**：主仓库 `git status --porcelain` 全程只有 `.arcforge/` 条目。

**复现命令**（锚钉全 sha）：

```bash
git worktree add --detach /tmp/wt-006r3 6ed3984cf89733aa457e4405060675d3394e46bf
cd /tmp/wt-006r3
GOTOOLCHAIN=local go test ./... -count=1                                   # 65 ok / 0 FAIL
GOTOOLCHAIN=local go test ./internal/hestia/sheets/ -count=1 -coverprofile=/tmp/c.out
grep -E 'push\.go:202\.4' /tmp/c.out        # 计数 1——但该行的累加无守卫，见第 4 节
```

---

## 结论

**VERIFIED。**

`fix_items[1]` 的三步（文档订正 / 可观测出口 / dry-run 打印）全部达标，
交付自称的五个变异我**逐行独立复现，五行全部相符**，另补三个变异其中两个守得住。
你点名的两处「比要求更强」的选择**都是对的**——特别是 `tally` 不碰 `DroppedCells` 那条，
dev 的结论与 leader 的提醒相反而**正确**，且它为这条相反判断单独立了守卫（N3 实测 KILLED）。

**一条未阻断的已知缺口**：`Push` 第 6 步 `missing` 循环里的累加无守卫（P3 变异存活），
而该行覆盖块计数为 1——**覆盖是绿的、守卫是空的**。代码本身正确，缺的是一条约 10 行的测试，
建议折进任何一轮后续返工，不必为它单开一轮。理由与判 PASS 的依据见第 4 节。

**TASK-009 不必重验**：其每条具名判据我在现树上逐条实测仍成立，含唯一有顺序风险的
「末尾固定一句」（探针证了 `DroppedCells=8` 时末行未变）。但这次的绿靠的是人工补位，
不代表那个跨任务盲区被机制堵上了。

---

## 附录：一条交付后才提出的限制（`DroppedCells` 的成因单一性前提）

**来源**：dev 经 leader 转达，它自己写不进注释——006 已进 `verifying`，改代码会让判定对象漂移，
**它主动没改是对的**（正是 AD-29 要防的）。裁决权在我，故记在这里。

**dev 的表述**：`Diff` 的少产出只有一个出口（返回切片的长度）；若将来 `Diff` 变成也能部分失败，
「期望 − 实得」就会把两种不同的事混成一个数。

**我的判断：前提成立，值得记录，但不构成本轮缺陷；且我认为更准确的表述不是「混成一个数」。**

逐层拆开：

| 方面 | 新增第二种少产出成因后会怎样 |
| --- | --- |
| **数值（magnitude）** | **仍然正确且完整**。`期望 − 实得` 等于「未被比对的格数」总量，与成因有几种无关。检测能力不丢——这正是 dev 当初选「取性质不取成因」的那个好处，它继续成立。 |
| **归因（attribution）** | **会变成假话**。`cmd/atlas/hestia_sheets.go` 里那行把成因**写死**在括号里：「（有行的月份越界，整行被跳过；……）」。出现第二种成因时，**数字是真的，而对这个数字的解释是假的**。 |

⇒ 准确的表述是：**`DroppedCells` 在数值上与成因无关，在打印出来的解释上却绑死了单一成因。**
不是「两件事混成一个数」（那会暗示数字不可信），而是「一个可信的数字配了一句可能不再可信的解释」。

**为什么这值得写下来**：两者**分处不同的包**——计算在 `internal/hestia/sheets/push.go` 的
`diffTab`，而写死成因的那句话在 `cmd/atlas/hestia_sheets.go`。**将来在 `sheets` 包里加第二条
少产出路径的人，本地没有任何信号提示 `cmd/atlas` 里有一句话因此变成了假的。**

这与本任务花了两轮修的那两处假溯源是**同一形状**：一句写下时为真的话，待在离「让它为真的那段代码」
很远的地方，而使它变假的改动不会碰到它。

**更值得一提的是**：dev 本轮**自己写下了防这件事的通则**——在 `Diff` 的注释里：

> 「收窄不变量时，**宣称它的那段文字和依赖它的那段代码必须一起改**，否则文档本身会变成下一条假溯源。」

它这次新增的那句 CLI 文案，恰好就是这条通则的一个未被应用的实例。**写下纪律不产生遵守纪律的能力**——
这一条本 sprint 已经记过，此处又添一例，而且是同一个人在同一个提交里。

**建议的处置（成本很低，不必单开一轮）**，二选一：

- **A（最小）**：在 `push.go` 的 `DroppedCells` 字段注释末尾加一句指路——
  「⚠️ 新增第二种少产出成因时，须同步修改 `cmd/atlas/hestia_sheets.go` 里写死成因的那句文案」。
  把「会变假的那句话」从「会使它变假的那段代码」处**指出来**，正是 dev 自己那条通则的做法。
- **B**：把 CLI 文案改成不断言唯一成因（如「未比对（当前已知成因：月份越界）」），
  让它在第二种成因出现时**退化为不完整而不是错误**。

**落点说明（机制事实，请 leader 注意）**：本条**无法**写进 `discoveries/TASK-006.json` 的
`degradations`。`arcforge-write.sh:982-989` 的 discovery 时机守卫在 `verified`/`accepted` 之后
**对所有角色（注释里明写含 leader 与 owner）**禁止覆盖既有 discovery，而 006 已于 06:40:40Z 转
`verified`、其 discovery 早已存在。⇒ 当前可写的载体只有：本验证报告（已写在这里）、
leader 自己的 `plan.md` / final-report / CONTRACTS，或下一轮返工时由新的 discovery 承载。

**对本轮判定的影响：无。** 当前 `Diff` 确实只有一个少产出出口，前提成立，交付正确。
本条是**面向未来的触发条件**，不是现存缺陷。
