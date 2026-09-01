# TASK-004 验证报告 —— magnitude_ranges 校验未知字段/倒置区间/缺 unit

- **验证者**：test-m1c3b-b
- **任务状态**：verifying（`assignment_epoch=1`）
- **判定对象树**：`e02056079baaa0c33bf25254e7cfe18b971547a7`（= `verify_baseline.head`，全部实测数字采于此树）
- **报告落盘时主仓库 HEAD**：`053d1a96240819f4ca3394512226c85ff74ebea9`（验证期间前移，见 §5）
- **结论**：✅ **VERIFIED** —— 10 格全部 PASS，**含 `functional[0]` 的「逐字节照抄」子句**（Leader 事后给出需求文档路径，已完成 sha256 级比对，见 §7）。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 增加逐项校验，三类拒绝；**实现整段照抄需求文档 Task 4 Step 3（含注释）** | 三类拒绝由 f[1..3] 逐条消融验证；40 行块 **sha256 与需求文档原文完全相同**（§7） | ✅ PASS |
| functional[1] | 未知字段名 ⇒ error 含该键名 | `TestMagnitudeRangesRejectUnknownField`；消融 **M5** 独家转红 | ✅ PASS |
| functional[2] | `Min >= Max` ⇒ error 含字段名 | `TestMagnitudeRangesRejectInvertedRange`；消融 **M7** 独家转红 | ✅ PASS |
| functional[3] | `Unit` 为空 ⇒ error 含 `unit` | `TestMagnitudeRangesRequireUnit`；消融 **M6** 独家转红 | ✅ PASS |
| boundary[0] | 空表仍然合法 | `TestEmptyMagnitudeRangesStillValid`；消融 **M2** 下它是唯一保持绿的 | ✅ PASS（附保留，见 §3.1） |
| boundary[1] | 未知键那轮 range map、其余遍历 `fieldOrder` | `thresholds.go:237`（range map）/ `:244`（range fieldOrder） | ✅ PASS（注释位置有问题，见 §4.1） |
| error_handling[0] | 三条错误信息说清后果，照抄原文 | 程序化子串比对，三段引文逐字命中（见 §2.3） | ✅ PASS |
| error_handling[1] | 不改 `configs/hestia.yaml` | 提交 numstat 仅 2 文件；`TestRealHestiaConfigLoads` PASS | ✅ PASS |
| non_functional[0] | gofmt / vet / test / 覆盖率 / 无新依赖 | 见 §2.4 | ✅ PASS |
| non_functional[1] | 交付流程 AD-4（merge 先于 dev_done） | merge `23:49:21Z` < dev_done `23:51:18Z`，早 1 分 57 秒 | ✅ PASS |

---

## 2. 证据

### 2.1 消融实验（隔离 worktree，主工作区零改动）

判定对象树钉死全 sha `e02056079baaa0c33bf25254e7cfe18b971547a7`，共 9 个变异，每个都过语法闸（`gofmt -e`）+ 生效闸（sha256 变化）+ diff 逐字核对，收尾核实还原（`git status` 干净）。基线：五条测试全 PASS。

| 变异 | 改动 | 转红的测试 | 说明 |
|---|---|---|---|
| M1 | 删掉 `if len(t.MagnitudeRanges) > 0` 守卫 | **（无，全绿）** | 见 §3.1 |
| M2 | 退化成「非空即拒」 | RejectUnknownField / RejectInvertedRange / RequireUnit / **AcceptValidTable** | `EmptyMagnitudeRangesStillValid` 是唯一绿的 ⇒ 它确实守着「空表 ⇒ 合法」 |
| M2' | 两轮循环全部通过后仍拒（非空且合法 ⇒ 被拒） | **仅 AcceptValidTable** | 见 §3.2，第 5 条测试独家价值的证据 |
| M3' | 第二轮循环只跑 `fieldOrder[:1]` | RejectInvertedRange / RequireUnit（**AcceptValidTable 仍 PASS**） | 见 §4.3 |
| M4 | `r.Min >= r.Max` 改成 `r.Min > r.Max` | **（无，全绿）** | 见 §4.2 |
| M5 | 未知字段名不再报错 | 仅 RejectUnknownField | functional[1] 真在守 |
| M6 | 缺 unit 不再报错 | 仅 RequireUnit | functional[3] 真在守 |
| M7 | 倒置区间不再报错 | 仅 RejectInvertedRange | functional[2] 真在守 |
| （M3） | 原设计的锚点在文件中出现 2 次，被唯一性闸挡下 ⇒ SKIP，由 M3' 替代 | — | — |

### 2.2 五条测试基线（判定对象树）

```
--- PASS: TestMagnitudeRangesRejectUnknownField
--- PASS: TestMagnitudeRangesRejectInvertedRange
--- PASS: TestMagnitudeRangesRequireUnit
--- PASS: TestEmptyMagnitudeRangesStillValid
--- PASS: TestMagnitudeRangesAcceptValidTable
```

### 2.3 error_handling[0]：程序化比对，非目测

从 DoD 正文正则抽出 4 段引文，用一次性探针取三条**实际**错误串，逐字判子串：

| DoD 引文 | 逐字出现在 |
|---|---|
| 「gateMagnitudeSanity 对未知键是跳过，配置会\*\*静默失效\*\*——字段名的唯一真相源是 fields.go 的 fieldOrder」 | UNKNOWN ✅ |
| 「倒置的区间会让该字段每一期都失败，而理由串读起来像数据错、不像配置错」 | INVERTED ✅ |
| 「它只出现在失败理由串里、不影响判定，正因如此会被漏填」 | NOUNIT ✅ |
| 「非法」（DoD 正文用词「不只是『非法』」，非要求的原文） | ❌ 无 —— **正确地未命中，反向证明比对未跑偏** |

三条实际错误串（探针实测输出）：

```
[UNKNOWN]  hestia: magnitude_ranges 含未知字段 "m2_typoed": gateMagnitudeSanity 对未知键是跳过，
           配置会**静默失效**——字段名的唯一真相源是 fields.go 的 fieldOrder
[INVERTED] hestia: magnitude_ranges[m2] 的 min(1000) >= max(0): 倒置的区间会让该字段每一期都失败，
           而理由串读起来像数据错、不像配置错
[NOUNIT]   hestia: magnitude_ranges[m2] 缺 unit: 它只出现在失败理由串里、不影响判定，正因如此会被漏填，
           而漏填之后失败信息少了唯一能判断「是不是单位搞错了」的那一项
```

三条都说清了**后果**（静默失效 / 每期都失败且理由串误导 / 少了判断单位的唯一依据），不只是说「非法」，且前两条还指出了**怎么修**（真相源在 `fields.go` 的 `fieldOrder`）。符合 DoD 「读者正是那个在填 54 项的人」的意图。

### 2.4 门禁项（全部采于判定对象树 `e0205607`）

| 项 | 实测 | DoD 判据 | 判定 |
|---|---|---|---|
| `go test ./internal/hestia/... -count=1 -cover` | ok, **coverage: 95.9%** | 不低于 95.9%（基线 @ `32bc1e5`） | ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出 | ✅ |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | 这两个之外无新增项 | ✅ |
| `go test ./cmd/... -count=1` | ok；`TestRealHestiaConfigLoads` PASS | — | ✅ |
| go.mod / go.sum 在本任务提交中的改动 | **0 行** | 不得出现 | ✅ |
| 提交 numstat | `thresholds.go +40/-0`、`thresholds_test.go +79/-0`，仅此 2 文件 | 与 dev 自报一致 | ✅ |

补充：探针实测 `len(fieldOrder) == 54`，与 dev 给 TASK-005 的「54 个字段」一致；`fieldOrder[0] == "tsf_stock"`。

### 2.5 交付流程（non_functional[1]，AD-4）

| 事件 | 时刻（UTC） |
|---|---|
| feat 提交 `8454167a35c8a43ebe68733027bf849498b7d920` | 2026-08-31 23:46:11 |
| dev `update`（挂 discovery 指针） | 2026-08-31 23:47:25 |
| **merge `e02056079baaa0c33bf25254e7cfe18b971547a7`** | **2026-08-31 23:49:21** |
| **`in_progress → dev_done`** | **2026-08-31 23:51:18** |
| `dev_done → verifying` | 2026-08-31 23:52:22 |

merge 早于 `dev_done` **1 分 57 秒** ⇒ `task-completed.sh` 的门禁量到了真实代码，AD-4 满足。

---

## 3. Leader 点名要验的两格

### 3.1 boundary[0]「加了校验之后空表仍然 PASS」—— PASS，但结论比表面复杂

Leader 指出「它在 dev 动手前就是 PASS 的，所以『它现在 PASS』不构成 dev 做对了的证据」。用消融回答：

- **M2**（校验退化成「非空即拒」）⇒ 四条红，`TestEmptyMagnitudeRangesStillValid` 是**唯一保持绿**的。⇒ 该测试确实在守「空表 ⇒ 合法」这个**行为**，不是摆设。**这一格 PASS。**
- **M1**（直接删掉 `if len(t.MagnitudeRanges) > 0` 守卫）⇒ **五条全绿**。

M1 说明：**那个守卫在语义上是冗余的**。空表时 `known` 照常构建、`range` 空 map 零次迭代、`range fieldOrder` 全部 `!ok → continue`，天然落到 `return nil`。所以：

> 该测试守的是「空表 ⇒ `validate()` 返回 nil」这个**行为**（会因「空表被判非法」而转红，这正是它要守的）；
> 它**不**守「校验只在 `len > 0` 时进行」这个**实现形态**（删掉守卫它不会红）。

DoD boundary[0] 的实质要求是前者（「空表仍然合法……不能因为加了校验就把它判非法」），故判 PASS。此处如实记录该区别，不作为扣分项 —— 守卫是需求文档给定的实现，dev 照抄正确。

### 3.2 第 5 条测试 `TestMagnitudeRangesAcceptValidTable` —— 批准理由成立，且拿到独家证据

Leader 的批准理由是「那 4 条对『非空且合法 ⇒ 走完两轮循环落到 return nil』覆盖为零」。核实：

- M2 下前三条也红，**证明不了**它的独家价值（前三条恰好也因该变异而红）。
- 故补做 **M2'**：在两轮循环全部通过之后才拒（精确模拟「非空且合法却被拒」）⇒ **只有 `AcceptValidTable` 转红**。

⇒ 它是**唯一**守着 TASK-005 那条路径的测试。若校验退化成「非空即拒」，TASK-005 的 54 项会全被挡在 `LoadConfig` 外，而唯一会喊的就是这条测试。**建议保留，判定为有效增补，非 scope 越界。**

---

## 4. 发现的问题（均不构成 reject，按严重度排序）

### 4.1 【已知文档缺陷，非交付缺陷】注释贴在了它所描述的循环的**上一个**循环上（`thresholds.go:235-237`）

```go
		// 遍历 fieldOrder 而不是 map：map 迭代顺序随机，同一份坏配置两次跑
		// 报出不同的那一项，会让排查变成猜谜（与 gateMagnitudeSanity 同理）。
		for name := range t.MagnitudeRanges {          // ← 这一轮正是 range map
```

该注释描述的是**下方**那个 `for _, f := range fieldOrder`（`:244`）。

- **行为完全正确**：DoD boundary[1] 要的正是「未知键那一轮**只能** range map，其余遍历 fieldOrder」，实现两条都满足。
- **危害是实打实的**：读代码的人看到「不要 range map」紧跟着一个 range map，会当成 bug 去「修」，**而修了就违反 DoD**。
- 同一段注释在 `validate.go:494-495` 的 `gateMagnitudeSanity` 里**贴对了**（下方确实是 `range fieldOrder`）。两处措辞经逐字 diff **不同**（「同一份数据」→「同一份坏配置」、「不同的越界字段」→「不同的那一项」、增补「与 gateMagnitudeSanity 同理」）⇒ 是**改写**而非机械复制，倾向于需求文档原文即如此。
- **有反馈回路，故不阻断**：若有人照着这条注释把 `for name := range t.MagnitudeRanges` 改成 `for _, f := range fieldOrder`，未知键检测会**完全失效**（遍历 fieldOrder 永远碰不到未知键）—— 而消融 **M5** 已证明 `TestMagnitudeRangesRejectUnknownField` 会立刻转红。⇒ 这个「误改」风险被测试挡住了，与 §4.2 那条**无**反馈回路的缺口性质不同。
- **归属已判定：问题在需求文档，不在交付。** 已取得需求文档原文（`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-31-hestia-backfill-load.md` 第 780-819 行）并程序化复核：该注释**在原文中就贴在 `for name := range t.MagnitudeRanges` 上面**。dev 逐字节照抄，无过错。

> ⚠️ **给下一个读 `thresholds.go` 的人**：这处注释与其下方代码方向相反是**已知的文档缺陷**，不是实现 bug。**不要"修"它** —— DoD `boundary[1]` 明确要求「未知键那一轮**只能** range map」。真去改了，`TestMagnitudeRangesRejectUnknownField` 会立刻转红（消融 M5 已证）。Leader 已将此列入 final-report 的更正项。

### 4.2 【中】`min == max` 被拒这条约束**没有任何测试守着** —— 建议转 TASK-005 或后续任务补一条断言

- 消融 **M4**（`r.Min >= r.Max` → `r.Min > r.Max`）⇒ **五条测试全绿**。
- 而 dev 在 `interfaces_exposed` 里给 TASK-005 的约束明确写了「⚠️ 注意 `min == max` 也会被拒」，下游会直接依赖。
- 探针实测行为**是对的**：`{FieldM2: {Min: 300, Max: 300, Unit: "万亿元"}}` ⇒ 被拒，错误串为 `min(300) >= max(300)`。只是无断言守护。
- 现有 `TestMagnitudeRangesRejectInvertedRange` 用 `{Min: 1000, Max: 0}`，`>` 与 `>=` 都会拒 ⇒ 对边界零区分力。
- **为什么这条值得补，而「TASK-005 填表打错字」不值得额外加机制**：后者有反馈回路（DoD 要求填完跑 `LoadConfig`，当场报错且错误信息说清了后果）；前者**没有** —— 将来有人把 `>=` 放宽成 `>`，套件不会红，已填好的配置也不会因约束放宽而报错，缺陷会静默留存。

### 4.3 【低】dev 为第 5 条测试给出的**理由**被证伪（结论对、理由错）

dev 在 `decisions[2]` 写：「用三个不同板块的字段而不是一个，是为了让『循环只跑了一轮也算过』这种退化过不了关」。

- 消融 **M3'**（第二轮循环改成只跑 `fieldOrder[:1]`）⇒ `AcceptValidTable` **仍 PASS**。
- 原因：三项**全部合法**，循环跑一轮和跑三轮都返回 `nil`，该测试构造上检不出这种退化。要检出需要三项中至少有一项非法。
- **结论（这条测试有价值）成立**，但由 M2' 而非「三个字段」支撑。记录于此是因为**理由是下游复现时的唯一入口**，而结论正确会让理由永不被复查。
- 不在 DoD 内，不影响判定。

### 4.4 【信息】`code-simplifier` 口径核实通过

按 Leader 给的口径核对：全 discovery 提及 `code-simplifier` **仅 1 处**（`provenance`），措辞为「未返回任何实质结论」「**故不作为通过证据**」，声称的是「已核实它未修改任何文件」（能说的），**未**写成「审过没问题」（不能说的）。程序化扫描「审过/无问题/通过审查/已审/确认无/批准」等措辞，零命中。

独立复核：本任务提交只含 `thresholds.go` 与 `thresholds_test.go` 两个文件；当前主仓库未提交改动全部在 `.arcforge/` 下，无任何代码文件。⇒ dev 的处置与陈述均正确。

---

## 5. verify_baseline 漂移核查

| 项 | baseline 记录 | 当前 | 判定 |
|---|---|---|---|
| `head` | `e02056079baaa0c33bf25254e7cfe18b971547a7` | `053d1a96240819f4ca3394512226c85ff74ebea9` | 前移，见下 |
| `discovery_sha256` | `4fa6c7bf53c6df3779348d5fdd95e3b10743a5a350be685f35bfeb2cf13788b7` | 同值 | ✅ 判定原料未漂移 |

HEAD 前移由 TASK-001（`053d1a9`/`5094fcd`）与 TASK-002（`db19e80`/`87c4233`）合入造成，两者落在**同一个包** `internal/hestia`。按 AD-29「声明范围内的文件变了」判据核查：

```
git diff --numstat e0205607..HEAD -- internal/hestia/thresholds.go internal/hestia/thresholds_test.go
⇒ 空
```

两个 `writes` 文件在新 HEAD 上 sha256 **逐字节不变**（`476890b159570df5…` / `c43869f8ae07fdac…`，与 dev discovery 自报值一致）⇒ **声明范围内零漂移**，转 `verified` 无需 `--ack-drift`。

覆盖率的分母属于整棵树，故两棵树各采一次：

| 树 | 覆盖率 |
|---|---|
| `e02056079baaa0c33bf25254e7cfe18b971547a7`（判定对象） | **95.9%** |
| `053d1a96240819f4ca3394512226c85ff74ebea9`（报告落盘时） | **95.9%** |

---

## 6. 复现命令（锚一律钉全 sha，不写 HEAD/分支名）

```bash
git worktree add --detach <dir> e02056079baaa0c33bf25254e7cfe18b971547a7
cd <dir>
go test ./internal/hestia/... -count=1 -cover      # ⇒ ok, coverage: 95.9%
go test ./internal/hestia/ -count=1 -v -run 'TestMagnitudeRanges|TestEmptyMagnitudeRanges'
go vet ./internal/hestia/... ./cmd/...              # ⇒ 零输出
gofmt -l internal/hestia cmd/atlas                  # ⇒ 恰两个既有欠账文件
```

---

## 7. `functional[0]`「逐字节照抄」—— 已完成核实（补记）

**判定当时未能核实**：需求文档不在本仓库，全仓 + 全 scratchpad grep 无果；dev 的暂存副本 `scratchpad/task004-block-dev-m1c3b-c.txt` 与实现**同源**，不构成独立证据。当时依据三条判为不阻断（该条可测内容已由 f[1..3] 消融覆盖、DoD 内嵌三条原文逐字一致构成部分独立印证、唯一已知偏差有 M5 兜底），并在此明写「未核实」。

**判定之后 Leader 给出了路径，已补做核实，结论是 PASS**：

需求文档在 **hestia 仓**（不在 atlas，这是先前 grep 不到的原因）：
`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-08-31-hestia-backfill-load.md`，Task 4 Step 3 的 fenced code block 在第 **780-819 行**（40 行）。

**不采信转述版本，直接读文件**，与 `internal/hestia/thresholds.go` 第 221-260 行（40 行）做 sha256 级比对：

| | sha256 |
|---|---|
| 需求文档 780-819 行 | `5368b50feea1fed96b0e39a186857ddd18db897a1876e45ce779b18e5b728d9b` |
| 实现 thresholds.go 221-260 行 | `5368b50feea1fed96b0e39a186857ddd18db897a1876e45ce779b18e5b728d9b` |

⇒ **逐字节一致**，dev 自报的「与需求文档第 780-819 行逐字节比对一致（40 行）」属实。`functional[0]` **PASS**。

同一次比对独立复核了注释位置：`for name := range t.MagnitudeRanges` 在块内第 17 行，其上两行正是「遍历 fieldOrder 而不是 map……」⇒ **原文如此**（§4.1 据此改判为文档缺陷）。

## 8. 建议转下游的两项

1. **`min == max` 无测试守护 —— Leader 已建 TASK-012 承接，该契约已撤回。**（§4.2）

我经消融 M4（`>=` → `>` 五条全绿）发现；**dev-m1c3b-c 在交付后自查中独立跑出同一个存活变异**，并主动指出它打脸自己写进 `interfaces_exposed`、广告给 TASK-005 的那条契约。**两条独立路径得到同一结论**，证据强度远高于单方发现。

处置（Leader 裁决）：该契约**已撤回** —— 在 TASK-012 补上测试前，「`min == max` 也会被拒」不作为已保证的契约转达给 TASK-005。TASK-012 的 `functional[2]` 已写明补该用例，且 RED 判据**必须走变异**（当前代码上它本就 PASS，补的是守卫缺口而非行为缺口）。

⚠️ 不影响 TASK-004 的判定：它不在 TASK-004 的 `done_criteria` 内。行为本身经探针实测正确（`{Min:300, Max:300}` 被拒）。
2. **给 QA 阶段**：§4.1 的注释位置，与 §4.3 dev `decisions[2]` 中被证伪的理由（结论对、理由错 —— 理由是下游复现时的唯一入口）。
