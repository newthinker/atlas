# 需求 ↔ DoD 双向追溯矩阵 · M1c-3b

**需求文档:** `2026-08-31-hestia-backfill-load.md`（1408 行）
**任务:** `.arcforge/tasks/TASK-001..010.json`
**生成时间:** 2026-08-31
**采样锚:** `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`

---

## A. Global Constraints（9 条）→ DoD

| # | 约束 | 承接位置 | 判定 |
|---|---|---|---|
| G1 | Go 1.24.4、`internal/hestia` | 环境已核实（`go version` = go1.24.4，`go.mod` = 1.24.4） | ✅ 前提，非验收项 |
| G2 | **无新增依赖** | 全部 10 任务 `non_functional[0]`：「`go.mod` / `go.sum` 不得出现在本任务的实际改动里」 | ✅ 覆盖 |
| G3 | **业务字段名字面量只许在 `fields.go` 与 `_test.go`** | **有反馈回路，不单列**——由 `TestFieldNamesAppearOnlyInFieldsGo` 机械守卫（**已核实存在**：`internal/hestia/fields_test.go:185`），而全部任务的 `non_functional[0]` 已要求 `go test ./internal/hestia/... -count=1` 全绿 | ✅ 覆盖（间接） |
| G4 | **包的导出面精确相等** | 同上，由 `TestPackageExposesNoWriteFunctions` 守卫（**已核实**：`internal/hestia/store_test.go:399`）。**且 TASK-003 `error_handling[2]` 显式点名**：新增 `MergeConflict`/`MergedObservation` 会让它转红，去补期望清单、**不要改成非导出**。这也是 003/006 的 `writes` 含 `store_test.go` 的原因 | ✅ 覆盖（直接+间接） |
| G5 | `Meta` 七字段三处同序 | **本迭代不新增 `Meta` 字段** ⇒ 不触发。TASK-006 只**装配** `Meta.ArticleID`（赋值，非增字段）。若 dev 发现需要新增字段 → 应转 `blocked_clarification` | ⚠️ 条件性不适用，已说明 |
| G6 | 注释带 milestone 前缀 | 全部 10 任务 `non_functional[0]` 末段 | ✅ 覆盖 |
| G7 | gofmt / vet / test 干净 + **两个既有欠账文件的例外** | 全部 10 任务 `non_functional[0]` 首段，含「判据是『这两个文件之外没有新增项』」的原话 | ✅ 覆盖 |
| G8 | **工具只产出依据，不做人的判断** | 三处承接：TASK-005 `boundary[0]`（54 项人工填 + 逐项理由进 discovery）、TASK-007 `functional[0]`（`Long` 文案逐字照抄「永不碰生产库、永不改 configs」）、TASK-010 `error_handling[1]`（生产库 sha256 相等） | ✅ 覆盖 |
| G9 | 测试文件 `import` 按需增补 | 琐碎实现细节，不设 DoD | ✅ 有意不覆盖 |

## B. 前置（回归基线）→ DoD

| 要求 | 承接位置 | 判定 |
|---|---|---|
| TASK-001 第一步采基线 | TASK-001 `error_handling[0]` | ✅ |
| **不要引用文档/spec 里的行数**（会漂的锚） | TASK-001 `error_handling[0]` 末段：「把 `wc -l` 的值与**你采样时的 HEAD 全 sha** 一并写进 discovery。**不要引用需求文档或 spec 里的行数**」 | ✅ |
| `--allow-incomplete` 是必需的 | TASK-001 / 005 / 010 的 `CORPUS_NOTE` 段 | ✅ |

## C. Task 1–8 的验收点 → DoD

| 文档位置 | 关键验收点 | 承接 | 判定 |
|---|---|---|---|
| Task 1 | `eachParsedArticle` 签名逐字一致 | 001 `functional[0]` | ✅ |
| Task 1 Step 4 ⚠️ | **`res.Periods++` 位置不得改**（错了 199→161 而单测全绿） | 001 `boundary[0]`，标 🔴 | ✅ |
| Task 1 Step 2 ⚠️ | fixture 复用既有构造器，不新写 | 001 `boundary[1]` | ✅ |
| Task 1 Step 6 | **背对背基线逐字节一致 = 唯一验收判据** | 001 `error_handling[0]`，标 🔴 + 给了完整背对背命令 | ✅ |
| Task 2 | `mergedRequiredFields` 取并集、按 `fieldOrder` 排序去重 | 002 `functional[1]` `boundary[1]` | ✅ |
| Task 2 背景段 | **为什么不能硬套 v2**（`2020-01\|monthly` 只 2 篇，by-design 缺席记成 failed ⇒ completeness 废掉） | 002 `boundary[0]`，标 🔴 且要求**原样落进函数注释** | ✅ |
| Task 2 | `requiredFields(merged)` 返回 nil + `default` 注释区分两种成因 | 002 `functional[3]` | ✅ |
| Task 2 Step 4 ⚠️ | 逐项表态测试转红是**它在正常工作**，去补表态不要改断言 | 002 `error_handling[0]` | ✅ |
| Task 3 | 合并、取并集、稳定排序、`DroppedIDs` | 003 `functional[0..3]` | ✅ |
| Task 3 | **单篇不得改写成 `merged@v1`** | 003 `boundary[0]`，标 🔴 | ✅ |
| Task 3 | **冲突不静默取值**（字段归属表错了必须响亮失败） | 003 `error_handling[0]`，标 🔴 | ✅ |
| Task 4 | 三类拒绝 + **空表仍合法** | 004 `functional[1..3]` + `boundary[0]`（标 🔴） | ✅ |
| Task 4 | 错误信息必须说清后果（三段原话） | 004 `error_handling[0]` | ✅ |
| Task 5 | 五档新值 + 钉住具体数字的测试 | 005 `functional[0..2]` | ✅ |
| Task 5 🔴 | **0.02→0.05 不是「为了让数据通过而放宽」**的完整论证 | 005 `boundary[1]`，标 🔴 | ✅ |
| Task 5 ⚠️ | **q1/h1/q1_q3 三档 n=0，是继承不是标定** | 005 `boundary[1]` 末段 | ✅ |
| Task 5 Step 5 ⚠️ | 54 项**人工判断**、三条原则、单位约定 | 005 `boundary[0]`，标 🔴 + **逐项理由进 discovery** | ✅ |
| Task 5 Step 4 ⚠️ | `configs/hestia.yaml` 保留原两条 ⚠️ 警示 + `config_version` 递增 | 005 `functional[3]` | ✅ |
| Task 6 | `BackfillLoad` 9 条实现要点 | 006 `functional[1]`（点名最易漏的 4/5/6 三条）+ `functional[2]` | ✅ |
| Task 6 (6) 🔴 | **按期次升序**否则漂移检测恒 `no_prior_period` 而零告警 | 006 `functional[1]`，标 🔴 | ✅ |
| Task 6 | 拒绝已存在的 DB + 理由 | 006 `boundary[0]`，标 🔴 | ✅ |
| Task 6 | 四道恒等式 + 不成立时 `writeLoadReport` 报错 | 006 `boundary[1]` + `error_handling[0]` | ✅ |
| Task 6 | `--allow-incomplete` 在 load 里代价更重的警告 | 006 `error_handling[1]` | ✅ |
| Task 6 | 单期失败不中断整批（`errors.Join`） | 006 `boundary[2]` | ✅ |
| Task 7 | `--db` required 且**不给默认值**的理由 | 007 `functional[2]`，标 🔴 | ✅ |
| Task 7 ⚠️ | `hestia.go` 不得 import `path/filepath` | 007 `boundary[0]`，标 🔴 | ✅ |
| Task 8 Step 1 | 重采一切自证数字 + 采样锚 | 010 `functional[0]` + `error_handling[0]` | ✅ |
| Task 8 Step 2 | 四道恒等式 + `merged@v1`=42 + 冲突=0 | 010 `functional[1]`，标 🔴 | ✅ |
| Task 8 Step 2 ⚠️ | **42/107/96 只能在真语料上核，本步是唯一验收点** | 010 `functional[1]` 末段，标 🔴 | ✅ |
| Task 8 Step 2b | 交叉验证 + **不齐 3 篇是已知例外**（算进分母得假失败） | 010 `functional[2]` | ✅ |
| Task 8 Step 2b ⚠️ | `tsf_stock` 应为 79，不是则「合并不影响标定」被推翻，停下来查 | 010 `functional[2]` 末段 | ✅ |
| Task 8 Step 3 | 7–9 条结转项 | **见下面 D 节**（拆到三个任务） | ✅ |
| Task 8 Step 4 | CONTRACTS A–E 五小节 | 010 `functional[3]` | ✅ |
| Task 8 Step 4 ⚠️ | **两个 `### F.`、两个 `### G.`，结构核对必须先锚再切** | 010 `boundary[1]`，标 🔴 | ✅ |

## D. 结转项分配（覆盖完整性核对）

⚠️ **需求文档自身的数字不一致**：标题写「**7 条**结转项闭合」、交付清单也写「7 条结转项逐条核过」，
而 Step 3 的表实际有 **8 个编号项**（1a / 2 / 4 / 5 / 6 / 7 / 8 / 9）**外加一行无编号的「矛盾标签」= 9 项**。
**Leader 按 9 项全部分配**（多分配无害，漏分配是静默失效）：

| 结转项 | 内容 | 承接任务 |
|---|---|---|
| 2（R9） | `stockContinuityRates` 业务键唯一性断言 | TASK-008 `functional[0]` |
| 1a | `2022-05` 理由串前半段说假话 → 改标签（**不补解析器**） | TASK-008 `functional[1]` |
| 9 | `calibrate.go:186` 的「23 篇」→ 19 | TASK-008 `functional[2]` |
| （无编号） | 矛盾标签统一为「M1c-4 的兜底工作量」（**6 处**） | TASK-008 `functional[3]` |
| 4（R10） | `sections.go:116`「共 74 篇」→ 55 + **两个 55 是不同集合**的说明 | TASK-009 `functional[0..1]` |
| 5（R11） | `checkPeriodTypeSupported` 死代码**不得删** + 防线注释 | TASK-009 `functional[2]`（守卫测试**已核实存在**：`parse_test.go:429`） |
| 6 | 两处措辞遗留（CONTRACTS G-6） | TASK-010 `boundary[0]` |
| 7 | `git log --grep` 在复用编号的仓库里不是范围判据（G-7） | TASK-010 `boundary[0]` |
| 8 | 合成 fixture 前缀假设（**未动 `periodAlt` 就原样保留**） | TASK-010 `boundary[0]` |
| **移交 1b** | `2022-05` 解析器缺口（**只改一处会全绿而无效**） | TASK-010 `boundary[2]` 明写移交 M1c-4 |
| **移交 3** | 按节部分抽取 | TASK-010 `boundary[2]` 明写移交 M1c-4 |

## E. 交付前检查清单（8 条）→ DoD

| # | 清单条目 | 承接 | 判定 |
|---|---|---|---|
| 1 | 四道恒等式成立，`merged@v1` = 42 | 010 `functional[1]` | ✅ |
| 2 | `calibrate` 输出与基线**逐字节一致** | 001 `error_handling[0]` | ✅ |
| 3 | 字段冲突数 = **0** | 010 `functional[1]` + 003 `error_handling[0]` | ✅ |
| 4 | `config_version` 已递增 | 005 `functional[3]` | ✅ |
| 5 | 结转项逐条核过 + 移交项带承接句 | D 节全表 + 010 `functional[3]` 的 E 小节 | ✅ |
| 6 | 覆盖率 ≥ 95.9% | 全部任务 `non_functional[0]` + 010 `non_functional[0]` | ✅ |
| 7 | **自证数字采于最后一次改动之后** | 010 `error_handling[0]`（判据按路径收窄，见 AD-6） | ✅ |
| 8 | **生产库未被改动** | 010 `error_handling[1]`（sha256 = `478d40c0…28c`） | ✅ |

## F. 机器检查结论

### F.1 孤儿需求（需求文档有、DoD 无）

**0 条阻断级。** 两条「有意不覆盖」已在上表标注并给出理由：
- **G5**（`Meta` 七字段同序）：本迭代不新增 `Meta` 字段 ⇒ 不触发；已写明 dev 若发现需新增应转 `blocked_clarification`
- **G9**（测试 import 按需增补）：琐碎实现细节

**3 条「有反馈回路，不必单列」**（G3 / G4 / 结转项 5 的守卫）——判据不是「有个叫这名字的测试」，
而是**已 grep 核实文件与行号**（`fields_test.go:185` / `store_test.go:399` / `parse_test.go:429`）。

### F.2 凭空 DoD（DoD 有、需求文档无）

**4 条，全部是 Leader 有意增补的流程约束**，来源与理由如下：

| DoD 条目 | 来源 | 为什么加 |
|---|---|---|
| 各任务 `CORPUS_NOTE`（语料用主仓库绝对路径） | AD-3 | 需求文档全篇写 `$PWD/data/...`，而 `git ls-files data/` = 0 ⇒ **worktree 里没有 `data/`** |
| 各任务 `non_functional[1]`（merge 在 `dev_done` 之前 + 不因 idle hook 催就转状态） | AD-4 | `task-completed.sh` 的 `git log --grep` 不带 `--all` ⇒ 未合并 commit 对门禁不可见 ⇒ **报绿而没量到代码** |
| TASK-005 `boundary[0]` 的「逐项理由进 discovery」 | AD-5 | 把「人工判断」在本流程里落地，且留痕 |
| TASK-010 `error_handling[0]` 的**按路径收窄**判据 | AD-6 | 文档原判据（numstat 全空）与「本任务要改 CONTRACTS.md」自相矛盾 |
| TASK-008/009/010 的「commit 用自己的编号」 | AD-1 | 文档 Task 8 被拆成三个任务，那条 `docs(TASK-008)` 只适用于 TASK-010 |

### F.3 不可测 / 含糊

**1 条需人类在 dod-gate 定夺**：TASK-005 `boundary[0]` 的「54 项人工填值」——
「填得对不对」没有机器判据，兜底靠 TASK-004 的三类校验（字段名 / 区间 / unit）+ Leader 抽查。
**这是需求文档本身的性质**（它明写「本步是人工判断」），不是 DoD 的缺陷。

### F.4 Realistic Scope

`done_criteria` 条数 **6 个任务超限**（最多 12 条）。详见 `01-design/design-spec.md` §4.2，
**提请人类在 dod-gate 决定**。package 数与文件数两项指标全部达标。


---

# G. dod-gate 后的补充（2026-09-01，人类裁决后）

独立 reviewer 反审报告（953 行）：`scratchpad/dod-reviewer-m1c3b-full-report.md`
**六条阻断级缺口，Leader 逐条独立核实后补入 DoD。**

## G.1 需求文档被证伪的两条事实前提

| 文档原话 | 真值（Leader 实测于 `32bc1e5`） | 承接 |
|---|---|---|
| Task 3 注释：三篇的 period / period_type / published_at **完全相同** | **假**。17 个发布事件被 `period_type` 拆开（存量报告标题恒为「N月…」⇒ `monthly`，同日另两篇季末写「一季度/上半年/前三季度」）。字段分布 `9→2 / 18→42 / 27→2 / 36→15 / 45→2 / 52→23 / 54→10`，**仅 33/96 完整** | **人类裁决：合并键不变，但强制暴露** → TASK-006 `PartialCoverage`、TASK-010 登记 17 事件 + 证伪判据 |
| Task 2 测试注释：三篇并集是「除外汇两字段外的全部 54 个字段」 | 并集是 **52**（`monthlyV1`=25 + 18 + 9）。文档 Task 3 的「27/18/9=54」用的是 **`rule@v1` 季报组**，两个数都对、不是同一组 | TASK-002 `functional[2]` 已订正 |

## G.2 六条阻断级缺口 → DoD 落点

| # | 缺口 | 落点 |
|---|---|---|
| **A-1** | `mergedRequiredFields` **无生产调用点** ⇒ 42 个合并观测 completeness 静默 skip（`gateCompleteness` → nil → `CheckSkipped`；而 `passed` 只在 `CheckFailed` 翻转） | **新建 TASK-011**（wave 2，deps 002）：`Observation.Parts` + `gateCompleteness` 分支 + `TestMergedCompletenessIsEvaluated` + `TestMergedPartsDoNotRoundTrip`。TASK-006 deps 加 011；TASK-010 加「skipped 条数为 0」判据 |
| **A-2** | `Out: nil → io.Discard` 背离同包 `Calibrate` 的相反契约（`calibrate.go:514`）⇒ 命令可能零字节输出 + exit 0 + 全绿 | TASK-006 `functional`（nil 即报错，背离须写 discovery）+ TASK-007 `functional`（`calExec` 断言 stdout 非空且含两个小节标题） |
| **A-3** | 见 G.1 | TASK-006 `PartialCoverage` + TASK-010 CONTRACTS §B |
| **A-4** | `configs/hestia.yaml` 的 54 项手填值**零自动守卫**（`grep --include=*.go` 零命中） | TASK-005 `functional`：`TestShippedConfigLoadsAndIsCalibrated`（读真配置，断言 54 项键集合与 `fieldOrder` 逐项相等 + `config_version` + 五档） |
| **D-1** | `os.Stat(DBPath)` 若晚于 `NewStore`（后者 `MkdirAll`+建文件）⇒ **第一次跑自己造库、第二次被自己拒** | TASK-006 `boundary`，并指出 `TestBackfillLoadRefusesExistingDB` **抓不到顺序错** |
| **白名单** | 加 `merged@v1` 进 `validExtractors` 打红**恰好三条**（reviewer 隔离 worktree 实测） | TASK-002 `functional`：`required_test.go:288`（七值→八值，**别改成遍历**）、`:320`（豁免表写理由）、**`parse_test.go:806`**（`exempt` 表）。`writes` 扩 `parse_test.go` |

## G.3 scope 归属调整

| 调整 | 理由 |
|---|---|
| TASK-002 `writes` **+ `parse_test.go`** | 那条红是 TASK-002 加白名单**直接造成**的；并进 TASK-009 会让 TASK-002 交付时全包是红的，且 `writes` 与实际写入不一致 ⇒ 范围外的真漂移不告警 |
| TASK-009 `writes` **− `parse.go` / `parse_test.go`** | **B-3**：`parse.go:284-296` 的防线注释**已逐字在位**（Leader 核实），本条正确交付是**零改动**。顺带解除与 TASK-002 的 wave 1 互斥 |
| **不采纳** reviewer 的「A-1 整块塞进 TASK-006」 | 那会让 TASK-006 的 `writes` 变成 **8 个文件**（远超 ≤5）。改为新建 TASK-011 承接**完整单元**（字段+闸门+两条断言），既满足「不可分」的论证，又不撑爆 TASK-006 |

## G.4 更新后的任务图

wave1: **001 002 004 009** ｜ wave2: **003 005 008 011** ｜ wave3: **006** ｜ wave4: **007** ｜ wave5: **010**

串行边：`004→005`（thresholds.go）、`003→006`（backfill_load*.go）、`001→008`（calibrate.go）、`002→011`（types.go）

`validator` **exit=0**（11 个任务）。

## G.5 已知未闭合项（人类 2026-09-01 授权接受）

- **`done_criteria` 条数超 Realistic Scope 的 ≤8**：现为 8–16 条，TASK-006 最重（16 条 / 5 文件 / wave 3）。
  真正的范围指标（≤1 package、≤5 文件）全部达标。理由见 `01-design/design-spec.md` §4.2。
- reviewer 分级为「🟠 可开工后补」的 8 条（A-5～A-8、D-2～D-4、D-7）与「🟡 锦上添花」的 2 条**未进 DoD**，
  作为 verifier 在 TASK-006 / TASK-010 验收时的**追问项**。清单见反审报告第 9 节。
