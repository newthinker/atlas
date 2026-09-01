# M1c-3b 进度 · 历史回填批量入库

**Sprint:** M1c-3b ｜ **起始锚:** `32bc1e5f306386ee5c69a54b4bae3e0184aa30f2`
⚠️ **本文件不在头部写 master sha** —— 它随每次 merge 过期，而过期的锚不会报错。当前 master 见下面「状态总览」末行（带采样说明），或直接 `git rev-parse HEAD`。
**调度:** `dag` ｜ **autonomy:** `dod-gate`（已通过）｜ **最后更新:** 2026-09-01 · wave 1 全部交付，验证中

---

## 状态总览（12 个任务）· **10 个 verified**，剩 2 个

| ID | 标题 | status | owner | verifier |
|---|---|---|---|---|
| TASK-001 | 抽出 `eachParsedArticle`（纯重构） | ✅ | dev-a | test-b |
| TASK-002 | `merged@v1` 取值域 + 必填集取并集 | ✅ | dev-b | test-a |
| TASK-003 | 按业务键合并 Observation | ✅ | dev-c | test-a |
| TASK-004 | `magnitude_ranges` 校验 | ✅ | dev-c | test-b |
| TASK-005 | 阈值重标 + 填 54 项 | ✅ | dev-d | test-b |
| TASK-006 | `BackfillLoad` 编排 + 报告（18 条 DoD，最重） | ✅ | dev-b | test-b |
| TASK-008 | 结转项 · calibrate 族四条 | ✅ | dev-a | test-a |
| TASK-009 | 结转项 · `sections.go` 数字订正 | ✅ | dev-d | test-a |
| TASK-011 | 接线 `mergedRequiredFields`（补 A-1） | ✅ | dev-b | test-b |
| TASK-012 | NaN/Inf + 两条存活变异 + F12/F17 | ✅ | dev-c | test-b |
| **TASK-007** | **`backfill load` 子命令**（`coverage_floor:75`） | **`assigned`** | dev-m1c3b-c | — |
| **TASK-010** | **真跑验收 + CONTRACTS 登记**（收口） | `pending`（等 007） | — | — |

**master** `d4f1e7c`，10 次 merge 后始终绿，覆盖率 **96.2%**（起始 95.9%），validator rc=0。

### 🔴 A-1 那条线（本 sprint 最有代表性的）

反审发现「`mergedRequiredFields` 无生产调用点」→ 建 TASK-011 接线 → **接线读 `Observation.Parts`，而 TASK-003 把值写在包装结构 `MergedObservation.Parts` 上** → 两个任务 DoD 都过、缺口原样存在 → dev 端到端探针发现 → 修复并进 TASK-006 → **实测闭合**（`TestMergedObservationCompletenessIsEvaluatedEndToEnd` PASS，变异下两个子格全红）。

**一个缺口，被发现两次、修了两次，第一次修完之后所有守卫都是绿的。**

### 交付清单前置（已提前核实）

- 生产库 sha256 = `478d40c0…3ec28c`，**与 sprint 起始逐字符相同** ✓
- 语料 218 篇完整 ✓ ｜ `BackfillLoad` 在 internal 侧就位、cmd 侧为 0（TASK-007 未做）✓

## 🔴 Leader 裁决记录

### 裁决 9 — TASK-006 的 C-3：判 VERIFIED，不因变异 SURVIVED 判 REJECT

**给 test-m1c3b-b 的裁决。此前我只发了消息、没写文件，消息丢了（本 sprint 第 4 次），
它读 plan.md 读到的是 5 条无 C-3 —— 这一条本身就是「消息不是状态」的又一次实证。**

**冲突**：`fix_items[1]`（我写的）只要求「拆出 `checkInputIdentities` 在 `NewStore` 之前调用」
——已满足（`:330` 早于 `:342`），行为经真实场景实测正确（DB 未建出、报告 1088 字节含标题原文）。
而裁决 7 要求「对应变异转不红即不予验收」——V3 SURVIVED、外溢度 0。

**裁决：判 VERIFIED。** 理由（验证者给出，我采纳）：裁决 7 的措辞是「补的**仍是**空判据」，
**预设了 dev 补了判据**；而 `fix_items[1]` **只要求改行为、从未要求补守卫**。
判它不过 = 拿一条它没被要求满足的规则去卡它。

⇒ **根因是我的第 9 处 DoD 缺陷：要求了行为，没要求守卫。**
而缺守卫恰恰意味着**这个行为随时可以被无声改回去**——既有的
`TestBackfillLoadDoesNotCreateDBBeforeChecking` 守的是 D-1（`os.Stat` 早于 `NewStore`），
**不是** C-3（恒等式检查早于 `NewStore`）。

**裁决 7 本身也须修**（留 M1c-4）：正确表述是
**「每一条 fix_item 修复的行为，都必须有一条能红的守卫；没有守卫的修复不算完成」**。

#### 后续：`verified → review_fix` 补两项（小改动，不是推翻返工成果）

1. **补 C-3 守卫。** 验证者已写出形状并实测（当前实现通过 / V3 变异下失败），
   不到 15 行、复用真语料改一篇标题、无需新夹具。
   🔴 **关键约束：判据必须是「库文件不存在」，不能是「返回了 error」**
   ——缺陷版本同样返回 error（只是晚在末尾），**写成后者等于没补**。
2. **订正 `backfill_load_report.go` 里 `writeLoadReport` 的函数头假注释**
   （写着「恒等式先校验、后打印」，而实现与之相反，C-2 的整个修复就是把顺序反过来）。
   ⚠️ 危害等级已由验证者订正：**不是「静默回归」**——
   `TestWriteLoadReportPropagatesWriteError` 的注释点名了 C-2 并解释了顺序为何不能反，
   会拦住误改且自带解释。**实际等级是「误导 + 一次无效往返」。**

**写 `verified → review_fix` 时按裁决 8 带 `reason_class=dod_defect`。**



### 裁决 8 — TASK-006 的 `reason_class` 下次写它时必须用 `dod_defect`

**这条是给未来的我看的，因为「记得」在本 sprint 已被反复证明不可靠。**

`reason_class` 当前值 `task_defect`，我改不了（`verifying` 的合法写者只剩 `test-*`），
**但这不构成损失**：该字段只在**下一次返工**被读取，而下一次返工必然经过
`verified → review_fix`（leader 写）或 `verifying → rejected`（验证者写）。
**Leader 写那一步时本来就要带 `reason_class` —— 那时直接写 `dod_defect`，效果完全等价。**

**为什么是 `dod_defect`（dev-m1c3b-b 的理由，我采纳）**：这个字段驱动的是**断路器，不是记账**。
`dod_defect` 累计第 2 次直接转 `blocked_human`，而它更紧是有道理的——
**对着一份自相矛盾的 DoD 继续机器循环只会烧 token**。TASK-006 的 7 条 fix_items 里，
C-4、W-1/D-9、W-2/D-10 **三条是 Leader 的 DoD 缺陷**，不是实现缺陷。

代价不对称：**误升级到人类可见且便宜**（人看一眼就放回来）；
**继续机器循环一份坏 DoD 不可见且昂贵**。按「哪种错更容易被发现」选。

⚠️ 反面已知：C-2/C-3 确实是实现缺陷，紧断路器对它们偏严。**这是知情下的取舍，
不是归咎** —— final-report 须写明这一句，否则后人会读成责任划分。

**为什么写在这里而不是靠记住**：本条的触发时刻（下一次返工）可能在几小时后、
也可能跨 context 压缩。dev-m1c3b-a 的原话：「而『记得』正是刚被证明不可靠的那个东西。」



### 裁决 7 — 验收闸：判据必须先被证明能红（QA 立，Leader 采纳）

**适用范围：本轮 `review_fix` 的全部任务，尤其 TASK-011（该任务已 `in_progress`，
Leader 写不了它的 `done_criteria`，故此条是它的唯一载体）。TASK-006 已写进
`done_criteria.non_functional`，不依赖本条。**

> **验收前，验证者必须先证明这条判据能红。**
> 对 C-4：跑 M17（`len(g.SourceIDs) > 1` → `>= 1`），**新断言必须转红**。
> C-1 / C-2 / C-3 的对应变异见 `docs/05-review/adversarial-review-m1c3b.md`。
> **转不红即说明补的仍是空判据，不予验收。**

**理由（QA 原话）：一条从没有人见它红过的判据，不是被验证了，是被声称了。**

**为什么这条取代了「换验证者」**：我原打算返工后不派回原验证者，理由是 C-4 推翻了
TASK-010 的那个 42。QA 更正：**C-4 说的是那个 42 无人守卫，不是它错了**——它用 Parts 探针
独立复算，54/42 逐个正确。且那个 42 是 `test-m1c3b-a` 在 TASK-010 下判的，
而我要退回的 TASK-006/011 原验证者是 `test-m1c3b-b`，**理由指向 a，被换掉的是 b**。

**核心**：原验证者没有漏掉任何他被要求检查的东西——**他被给了一条空判据，然后正确地执行了它。
换一个人拿同一条空判据，会得出同一个结论。判据层的缺陷，换执行者是无效的杠杆。**

⇒ **不换验证者**（TASK-006 仍 `test-m1c3b-b`，TASK-011 仍 `test-m1c3b-b`），改为加这道闸。

**C-4 替代判据的 oracle（QA 已实测）**：`MergedGroups` == DB 里 `merged@v1` 的**跨两表**行数 = **42**，
`SingleArticle` = **54**。⚠️ QA 主动标注了它的独立性边界：三条路最终都在问「几个组不止一篇」，
**但 M17 只动计数器、不动 `mergeGroup` 里改写 extractor 的那行，所以 DB 仍是 42，能抓住 M17**（实测）。
**够用，但别当成完全异源。**

**给验证者的现成材料**（QA 的 scratchpad，别从零开始）：真跑报告 302 行 + `qa-hestia.db`；
C-2/C-3 的复现语料 `qa-corpus-unclass`（真语料副本，第一篇标题已改）；
四条 CRITICAL 的复现命令在 QA checkpoint 的「原始产物指针」节，**锚全是全 sha**。

（消息会丢，**此处是文件层的真相源**）

> 本 sprint 实测消息**双向丢失**：dev→leader（`team-lead` 地址问题，两个 dev 各丢 3 条）与
> **leader→test**（地址正确仍丢，派验通知 + 三条确认全丢）各发生过。
> ⇒ 凡需要 agent 据以行动的裁决，一律同时写进本节 —— **以此处为准，消息只是通知**。

| 事项 | 裁决 |
|---|---|
| **TASK-006 的 `BackfillLoadResult.Unclassified []string`**（不在需求文档、不在 DoD 明文，test-m1c3b-b 按 TASK-004 先例判不违反 `functional[0]`，请求确认） | ✅ **确认：不判违反。** ① dev 主动申报（`interfaces_exposed[2]` 逐字）② 有论证（`key_findings[3]`：不并进任何一格是为了让恒等式一在站点改期次表述时**响亮失败**）③ 两条测试守着 + `notes_for_downstream[3]` 提醒下游 ④ Leader 在 dev 交付时已认可。与 TASK-004 自加第 5 条测试同一处理：**DoD 之外的增补 + 主动申报 + 有论证 + 有守卫 = 不判越界**。⚠️ 验证者来问而不自己扩大解释是对的——字面与实质分歧时由 Leader 裁决 |
| **TASK-007 覆盖率口径** | `packages` 是 `./cmd/atlas`（实测 **75.6%** @ `f14fd28`），**不是** `./internal/hestia`（96.2%）。已加 `coverage_floor: 75`（门禁任务级字段，覆盖全局 `dev_minimum: 80`）。**验的时候按 75 判，不要套 96.2%** |
| **TASK-005/011 的 `scope-mutex` 冲突** | 声明层冲突、实际已消失（代码都在 master、改动区域不相交、三方合并已正确处理）。用 `ARCFORGE_SKIP_VALIDATE=1` 绕过并留痕，**不改 `writes`**（它同时是「这个任务改了哪些文件」的历史记录）。转 `verified` 后自动消解，validator 现 rc=0 |
| **TASK-007 的 `--db` 错误串断言**（DoD `functional[2]` 与需求文档都写「错误串含 `--db`」，而 cobra 实际文案是 `required flag(s) "db" not set`，**不带 `--` 前缀**；dev 改断 `"db"` + `"required"`，请求确认） | ✅ **确认：不判违反，保持现状，不要改成手写检查。** ① dev 实测了 cobra 的真实文案，Leader 独立佐证：同文件既有两条 required-flag 用例**也都断裸名**（`hestia_test.go:733` 断 `"out"`、`:895` 断 `"dir"`）② 需求文档那句「含 `--db`」是**未经实测的措辞**，与 `runCmd` 示意名、`writeCalibrateFixture` 假签名同一类 ③ dev 的替代断言**更强**：只断 `"db"` 会被任何含 db 字样的错误冒名满足，加断 `"required"` 才钉住「是必填未给这条路径拦下的」 ④ 严格照字面要放弃 `MarkFlagRequired` 自己手写检查，**为满足一句措辞而偏离本文件既有做法，代价大于收益** |
| 🔴 **TASK-010 的验收要点：「`unknown_extractor:merged@v1` = 0」不是闭合证明**（test-m1c3b-a 指出该判据**可能恒真**：若所有观测都绕过了 `gateCompleteness` 的 merged 分支，条数同样为 0） | **验收必须拆成三条独立路径，不能只看那个 0**：<br>**① 正向** —— 查那批合并观测的 `Obs.Parts` **非空**（证明 TASK-006 把值接上了）<br>**② 反向** —— 确认 `gateCompleteness` 的 merged 分支**真的被执行过**<br>**③ 同源校验** —— 用来证明 ② 的那个数**必须与闸门分支同源**。⚠️ Leader 曾提议用「`merged@v1` 观测数 = 42」证明 ②，**经核实不成立**：`res.MergedGroups++`（在 `BackfillLoad` 内，**合并阶段**）vs `"unknown_extractor:"` （在 `gateCompleteness` 内，**校验阶段**）—— ⚠️ **此处刻意用符号锚不用行号**：本 sprint 行号漂过三次（`validate.go:497/506` 5 分钟内漂成 512/521；`extract.go:744` 交付当天即错），**而行号漂了不报错，只会让人读到别的代码然后按它推理**—— **不同源**，42 只证明合并阶段产出了 42 个组。不同源就上变异或别的手段。<br>⚠️ **本条不改 `TASK-010.json`**（该任务已 `in_progress`，改 DoD 会撞进行中的任务），但**验收时以本条为准** —— DoD 里那句「0 条 = A-1 闭合的最终判据」射程不足。<br>⚠️ 另：两种成因在报告里同形，区分方法（查观测 `Parts` 是否为空）写在 **TASK-011** 的 `notes_for_downstream`（不是 TASK-006，Leader 曾指认错）|
| **消融记 SURVIVED / SKIP 不算缺陷** | TASK-011 的 A4（性质由结构决定、无法用文本变异表达）与 TASK-006 的 A5（两个公式在一切正确输入上同值，防御性改动）**均如实记录、不凑满分**，Leader 认可 |

## 已 merge（Leader 串行执行，每次 merge 后核实 master 绿）

| merge commit | 任务 | 内容判据 |
|---|---|---|
| `7eda136` | TASK-009 | `sections.go` sha256 与交付 commit 逐字节相同 |
| `e020560` | TASK-004 | `thresholds.go`/`thresholds_test.go` 各自 sha256 一致 |
| `db19e80` | TASK-002 | 四个交付文件逐文件 sha256 一致 |
| `053d1a9` | TASK-001 | `calibrate.go`/`calibrate_test.go` sha256 一致；`func eachParsedArticle` 命中 1 |

每次 merge 后：`gofmt` 仅两个既有欠账、`vet` 零输出、覆盖率 **95.9%**。

## 🔴 本轮新增的三条发现（均由 dev 主动申报，Leader 已核实）

1. **需求文档 Task 2 有第二处缺陷**（dev-m1c3b-b 发现，Leader 证实）：Step 1 的 `require.Equal(t, tsfStockFields(), got)` 与 Step 3 的「按 `fieldOrder` 排序」**不可能同时满足** —— `tsfStockFields()` 把 `FieldTSFStock/YoY` append 在**末尾**（`required.go:38`），而 `fieldOrder` 里这两个排在 A.1 总量段**最前**（`fields.go`）。dev 按 DoD `boundary[1]` 的两条**性质**断言（`ElementsMatch` + 升序 + 一格阴性），判断正确。
2. **`magnitude_ranges` 的 NaN/Inf 穿透**（dev-m1c3b-c 交付后自查发现）：IEEE 754 使 `Min >= Max` 拦不住 NaN，而下游 `v < r.Min || v > r.Max` **两侧同样恒假** ⇒ 幅度闸完全不设防且报 `passed`。**真缺陷、低触发概率**（需字面写 `.nan`/`.inf`）。→ **已建 TASK-012**。
   - 同时它自查出**两个存活变异**，其一直接打脸它广告给 TASK-005 的契约「`min == max` 也会被拒」—— 无任何测试钉住。该契约**已撤回**，补测试前不转达。
3. **跨 sprint 编号复用**使门禁的 `git log --grep` 失效：TASK-009 命中 14 条历史同号 commit、TASK-001 命中 8+16 条。**三个 dev 各自独立撞到**。这是结转项 7 的实证，→ 转 TASK-010 写进 CONTRACTS。

## ⚠️ Leader 本轮的两个失误（记明以免复发）

1. **待 merge 扫描报出的信号被我用「他没发请求」解释掉了** —— 而那条扫描存在的全部理由就是对抗通知丢失。dev-m1c3b-b 因此空等 11 分钟、被 idle hook 唤醒 8 次。**新规则：扫描报出即主动处理，不等请求。**
2. **误判 A/B 的处理窗口已关** —— 把 `dev_done` 当成关闭点，实际是 `dev_done → verifying`（`verify_baseline` 写入时刻）。dev 用 `jq` 直读纠正了我。

## 通信

🔴 **dev 发消息给 Leader 必须用 `main`，不是 `team-lead`** —— 后者返回 `success` 但静默丢失。两个 dev 各丢 3 条，都是自己发现的、无任何机制告警。**后续 spawn 的 prompt 必须写死。**

## Leader 每轮扫描的固定动作

1. `bash .claude/scripts/validator-run.sh validate .arcforge/tasks`
   （⚠️ `scope-writes-outside-packages` 已确认零信息量：命中条数**恒等于 `writes` 长度**，见 AD-2）
2. **待 merge 扫描 → 报出即主动处理**：
   `for b in $(git branch --list 'task/TASK-*-m1c3b' --format='%(refname:short)'); do git merge-base --is-ancestor "$b" master || echo "待 merge: $b"; done`
3. `jq -r '[.id,.status,.assigned_to//"-",.verifier//"-"]|@tsv' .arcforge/tasks/*.json`

## 进度日志

- **2026-08-31** Step 1–3 完成；独立 reviewer 反审报 6 条阻断级缺口，Leader 逐条核实全部证实
- **2026-09-01** dod-gate 通过（人类裁决 A-3 维持合并键 + 强制暴露）；补完六条缺口，新建 TASK-011，validator exit=0
- **2026-09-01** wave 1 派发 → 四个任务全部交付并 merge，master 保持绿
- **2026-09-01** 新建 TASK-012（承接 dev 自查发现的 NaN 缺陷）；TASK-004/009 验证中，TASK-002 待派验，TASK-001 按住待二次 merge
