# Sprint M3 · warp-hestia 解读 skill — 进度

**需求**：`~/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-06-hestia-m3-warp-hestia.md`
**基线锚**：`fe95ac707ea46f9939bb6554261830a95f1b93e1` · **当前 master**：`41a19d8`（TASK-008 merge）
**调度**：`dag` · **autonomy**：`dod-gate` · **阶段**：Step 5 收尾（**7/8 `verified`**，TASK-008 在验，全部交付完毕）

## 三个已裁决的 DoD 缺陷（都由 dev 发现，都是我写错）

| # | 缺陷 | 载体问题 | 处置 |
|---|---|---|---|
| ① | DoD 字面「`spool_write_allow` 两处都加」与需求 line 569「`config.local.yaml` **由人改**」冲突 | 任务已 `in_progress`，leader **无权写** done_criteria（实测 DENY：合法写者 `["dev-*"]`） | 裁 B；把消歧文本发给 dev-m3-b，**由 owner 自己写进 done_criteria** |
| ② | DoD 要求改 `container.json` 使挂载生效——但它**不是真相源**，手改**零运行时效果**（真相源是中央 DB `container_configs`，`materialize` 每次覆盖、`buildMounts` 从不读文件、backfill 永不回读） | 同上 | 裁「DB 留给人」（与 `mount-allowlist.json` 同批，两者必须一起做才生效）；由 dev-m3-c 写进 done_criteria |

| ③ | DoD 要求新建 `type History`，而 `validate.go:35` 已有**导出接口** `type History interface`（`Validate` 的公开签名参数类型 + `NoHistory` 的类型）⇒ 编译期直接红；而同一 DoD 又明令 `validate.go` diff 为空 ⇒ **自相矛盾，dev 无路可走** | 任务在 `blocked_clarification`，leader 同样写不了 done_criteria | 裁 **A：改名 `ContractHistory`**（与 `Contract`/`BuildContract`/`WriteContract` 对仗）；答复写进 `questions[0].answer`，由 dev 转回 `in_progress` 后自己落进 DoD |

**③的连带订正**：改名让 AST 守卫 `want` 的**字典序插入位置也变了**——`ContractHistory.FileName`/`.JSON` 要插在
`"Contract.JSON"` 之后、`"DefaultSignals"` 之前（实测既有 34 项，`Contract.JSON` 在 index 5、`DefaultSignals` 在 6、
`HealthSummary` 在 11），**不是** DoD 原文写的「`HealthSummary` 之后」。总数仍 **38**。
另批准 dev 两条自裁：`omitzero` 替代 `omitempty`（`omitempty` 对 `len==0` slice 也省略 ⇒ 做不到「非 monthly 时该键为 `[]`」，
**需求原文自己的接线断言会因此变红**）、`passing()` 替代 `ValidationReport{Passed: true}`（`store.go:937` 拒绝零 checks）。
并认可它指出「受影响测试是 3 条不是 1 条」且用 `ElementsMatch` 不放宽成 `Contains`。

**②连带查出一条影响更大的**：`projectRoot = process.cwd()` + skills bind mount ⇒ **容器只看得见主 checkout
工作树里的 skill**，worktree 里的 `warp-hestia` 对它不存在。现成反证：`warp-research` 在 `fork/main` 有、
主 checkout 没有，于是容器 `.claude-shared/skills/` 的 9 个链接里**没有它**。
⇒ 「PR 合并 + checkout 切回 `main`」这步人执行前置的理由从「保持整洁」变成「**容器读不到别处**」，
已钉进 TASK-005/006/007 的 DoD 与 TASK-008 §D 结转清单（连同「为什么 agent 不能做」）。
⚠️ 不影响本 sprint 交付：开发与 `unittest` 都在 worktree 里跑，不经容器。

## 当前在途

- **TASK-001…005 全部 `verified`（5/8）**，全部已 merge 进 master。
- **TASK-006 `in_progress`**（dev-m3-c，`08:08:46Z` 认领，epoch 1）——夹具 **13** 个 + `golden/` 期望 **2** 个 + `prepare.py`
  + `test_prepare.py` 已在 nanoclaw worktree `wt-warp-hestia` 的**未跟踪** `scripts/` 下成形，尚未提交。
  ⚠️ `in_progress` **刻意不设 stale 阈值**（dev 正常干活实测 p90 就 65 分钟），不要按「久了」催。
- **TASK-007 待 006 `verified`**（`dag` 就绪即派）。它是 **dev 任务**，验证已许诺给 test-m3-b。
- **TASK-008** wave 5，记录员模式，待 001–007 全 `verified`。
- 巡检 cron 每 6 分钟一轮、**六项**（不只 `stale-dispatch`）——扩项理由见下面「我漏答 31 分钟」那条。
- 派发通知里都要了「回一句确认收到」——它不防止通知丢失，只把发现丢失的时间从阈值级压到一句话。

## 🔴 高亮

- 无 `blocked_human` 任务。
- **需求 TASK-006（集成冒烟）本 sprint 不做**——结转（AD-M3-2，人类 2026-09-08 确认）。
  义务载体：**TASK-008** 写的 `CONTRACTS.md ## Sprint M3 §C/§D`（不是只放 final-report——归档后没人再读它）。
- **任务数 6 → 8**：独立 reviewer 反审判 NEEDS WORK 并给出 17 项，其中 5 条实质业务义务 + 5 条边界缺口
  装不进 8 条 DoD 上限 ⇒ 拆 TASK-006/007；另一条「无 owner 的产物」⇒ 新建 TASK-008。

## 任务状态

| ID | 标题 | 仓库 | wave | deps | status | owner | verifier |
|---|---|---|---|---|---|---|---|
| TASK-001 | history 侧车（**`ContractHistory`**）+ ingest 接线 + AST 守卫 34→**38** | atlas | 1 | — | ✅ **`verified`**（返工一轮，`dod_defect` 不计 rework） | dev-m3-a | test-m3-a |
| TASK-002 | `contract emit` 同产侧车 | atlas | 2 | 001 | ✅ **`verified`**（零缺陷） | dev-m3-a | test-m3-b |
| TASK-003 | Spool `source` 白名单 + 写白名单加路径 | loom | 1 | — | ✅ **`verified`** | dev-m3-b | test-m3-b |
| TASK-004 | 分支/队列挂载/触发段 | nanoclaw | 1 | — | ✅ **`verified`** | dev-m3-c | test-m3-a |
| TASK-005 | skill 文档（SKILL.md + 三 references） | nanoclaw | 2 | 004 | ✅ **`verified`**（带显式漂移 ack） | dev-m3-c | test-m3-a |
| TASK-006 | 夹具 + `prepare.py` + `test_prepare.py` | nanoclaw | 3 | 001,002,004,005 | ✅ **`verified`**（8/8，三锚零漂移） | dev-m3-c | test-m3-a |
| TASK-007 | `verify.py` 四态 + 统一开 PR + worktree 收尾 | nanoclaw | 4 | 006 | ✅ **`verified`**（8/8，换四任验证者） | dev-m3-c | test-m3-d |
| TASK-008 | CONTRACTS `## Sprint M3` §A–§D + 数字对账（记录员模式） | atlas | 5 | 001–007 | 🟣 **`verifying`** | dev-m3-a | test-m3-d |

## 编号映射（两套编号并存）

需求 001 → 本 sprint **001 + 002**；需求 002 → 003；需求 003 → 004；需求 004 → 005；
需求 005 → **006 + 007**；需求 006 → **不做（结转）**；**TASK-008 是 Arcforge 新增的**（需求无对应编号）。
代码注释用**需求编号**，atlas commit subject 与 `.arcforge/` 用 **Arcforge 编号**。
需求自身两种前缀写法（`M3 的 TASK-001` / `M3 TASK-002`）**都接受**，验证者不得因「的」字判不符。

## 并行度

- **wave 1 三条并行**：001（atlas）· 003（loom）· 004（nanoclaw）⇒ 3 个 dev
- **wave 2**：002（依赖 001）· 005（依赖 004）⇒ 2 个 dev
- **wave 3**：006 · **wave 4**：007 · **wave 5**：008
- nanoclaw 四任务（004→005→006→007）**串行复用同一个 worktree**：004 建，**007 拆**。

## 门禁与纪律（每个任务都要过）

1. atlas commit subject 锚定 `<type>(TASK-00X):`——需求原文的 `feat(M3 TASK-001):` **不匹配门禁**。
2. **merge 必须在 `dev_done` 之前**（`git log --grep` 不带 `--all`）。
   ⚠️ teammate-idle hook 在此状态的文案恒为「推进 dev_done」，**方向相反，不要照做**。
3. **discovery 与指针必须在 `dev_done` 之前落定**。
4. 跨仓库任务（003–007）证据落 `docs/hestia-m3/TASK-00X-*.md` 四节结构；**锚一律写全 sha**。
5. 一切 `.arcforge/` 读写在 **atlas 主仓库**执行。
6. **stale-dispatch 阈值**（从代码直读，非文档副本）：`assigned=15m` · `verifying=30m` · `blocked_clarification=30m`
   ⇒ CronCreate 周期取一半：派发后 **7m**、派验后 **15m**。

## 检查记录

| 时间 | 项 | 结果 |
|---|---|---|
| 2026-09-09 | 派验 TASK-008 | test-m3-d；基线 `41a19d87…`/`bb8fee27…`。三个自证数我独立复算吻合（`## Sprint M3` 1 处 / 3655 行 / M3 节内 4 节） |
| 2026-09-09 | **merge TASK-008** | `19ba2ed0…` → master `41a19d87…`；**136/0 纯追加**，`git show` 的 diff 里以 `-` 开头正文行 **0** ⇒ 纯追加属实（不只看 numstat）；两包测试绿 |
| 2026-09-09 | 🔴 **澄清环：§B⑤「2/0/1」零命中** | dev **没有自己圆数字**而是转 `blocked_clarification`（`error_handling[0]` 生效）。裁 (a)：来源是 **TASK-006 验证报告 §2.5 第 128–130 行**（verifier 独立实测）。⚠️ dev 的真实错因是**换对了工具却没换搜索空间**——只搜七份 discovery。⇒ **「找不到」有两种成因：匹配式坏了、搜索空间不含它，换工具只排除第一种**。答复用时 8 分钟（上次同类漏答 31 分钟） |
| 2026-09-09 | **口径澄清（补进 `functional[1]`）** | **验证报告的 verifier 独立实测优先于 dev 自陈**，是升级不是退而求其次 |
| 2026-09-09 | 🔴 **我的 DoD 缺陷第 17 处** | §B⑥ 引错来源任务：nanoclaw PR #5 在 **TASK-007**，而 TASK-004 discovery 明写「`.gitignore` 双重命中、零提交无 PR」。累计 17 处**无一由我自查出** |
| 2026-09-09 | **TASK-007 VERIFIED** | 8/8。第四任 test-m3-d 拒绝用被验方的测试验被验方（同源=循环论证），直接调 `verify.py` 构造篡改样本；验「逐字一致」时**只照 note-format.md 散文自己实现一遍算法**复算 golden 全 MATCH |
| 2026-09-09 | 🔴 **验证者三连卡死 → 换第四任 + 缩范围**（人类拍板） | test-m3-b(87m)/test-m3-a(94m)/test-m3-c(62m) 同一形态。**我实测其工作量仅 84 tests/2.313s** ⇒ 证伪「活太重」，指向工具批次挂起。⚠️ **砍掉变异复算的理由是缩小调用窗口，不是那活重**——理由记错会让下次砍错东西。有效处方：**每步落一行 checkpoint**，第四任全程可见 |
| 2026-09-08 | 派发 TASK-007 | dev-m3-c（`dag` 就绪即派）。DoD 8 条，validator rc=0 |
| 2026-09-08 | 🔴 **裁决：`prepare.py --now` 缺省取当天** | test-m3-a 报、我实跑复现：`--now` 缺省空串 + SKILL.md Step 3 不传 ⇒ 实产笔记 `created`/`updated` **两个空值**，而 `note-format.md:40` 列它们为 vault 必需字段。⚠️ **这是我 DoD 的缺口（第 16 处）不是 dev 缺陷**——我只写了「断言键**存在**」，14 个键确实都在 ⇒ 不返工、不计 rework，折进 TASK-007。**判据是失败模式不对称**：改缺省 ⇒ 忘传时 golden 当场变红（响）；靠调用方传 ⇒ 忘传时产出必需字段为空的笔记（静默）。⚠️ 验证者给的理由「②要动已 verified 的 TASK-005 产物」**不成立**——①动的 `prepare.py` 同样已 verified |
| 2026-09-08 | **TASK-006 VERIFIED** | 8/8。三锚判定前后各记一次零漂移（含机制够不到的 nanoclaw）。验「2.10 表产于简化之后」**不审 harness 而在当前交付上跑自己的 6 个变异，红数六个逐个相同**；A/B/C 消融把 M14（专职断言恒真、只被 golden 杀）与另三个（新增断言是唯一闸）区分开 ⇒ **验的是判据本身而非结论**。其 harness 对锚点命中 ≠1 大声报错拒记结果，实测触发一次 |
| 2026-09-08 | 🔴 **派验窗口第三种形态** | dev 在**同一条消息**里既写「无待补项」又写「我会去改 discovery」——两句时间戳相同、都是真话，前者只描述已交付部分。前两种形态有时间差可比，这种没有 ⇒ 派验前**扫未来时**（我会/打算/接下来），不要只 grep「无待补项」。压住派验 10 分钟，dev 的订正落盘早于基线 **27 秒** |
| 2026-09-08 | **merge TASK-006** | `331fe42a…` → master `b5873ad8…`；`HEAD^2` 复核一致；单文件 442/0 零越界。merge 前现读核对 nanoclaw 锚与分支 commit 数 |
| 2026-09-08 | 🔴 **PENDING 第 11 条**（test-m3-b 报） | write-guard 的运行时资产判定对**命令文本**求值、写入目标不参与 ⇒ 正文里引用 hook 路径会被整条拦下。**它没绕过**。⚠️ 我复核时**当场复犯**：探测脚本第一条用例即被拦，第一反应是「我的写入目标不是运行时资产，绕过去无妨」 |
| 2026-09-08 | 派发 TASK-006 | dev-m3-c（建 worktree 者，nanoclaw 四任务串行复用）；`08:08:46Z` 认领 |
| 2026-09-08 | 🔴 **PENDING 第 11 条**（test-m3-b 报告） | write-guard 的运行时资产判定对**命令文本**求值，写入目标不参与 ⇒ 在 checkpoint / 验证报告 / discovery 的**正文里引用 hook 路径**会被整条拦下。**它没有绕过**（改写命令形态即可过），而是改措辞并要求记进 PENDING。理由原话：**一个我认定是误报的拦截，和一次真实拦截，在 hook 那里长得完全一样**——我判断它是误报这件事本身，不构成绕过它的授权。⚠️ Leader 复核时**当场复犯**：为取实证构造的探测脚本第一条用例就被同一道闸拦下，第一反应是「我的写入目标不是运行时资产，绕过去无妨」 |
| 2026-09-08 | 🔴 **PENDING 第 10 条** | 门禁覆盖率尺系统性偏高：`-func` 只聚合具名函数声明，cobra 的 `var xCmd = &cobra.Command{Run: func…}` 被静默丢弃；实测 `-func` 报 76.8% 而 profile 直接数语句 76.6804%，剔除该块复算 76.8385% **精确吻合**。⚠️ 丢的是未覆盖语句 ⇒ **偏差单向偏高、随 cobra 命令数规模化、任何输出里都不显形** |
| 2026-09-08 | validator（初稿 6 任务） | 退出码 **0**；`⚠` **0** 条 |
| 2026-09-08 | 追溯矩阵 | 孤儿需求 **3** 个（G2e / G7 / C1b），已全补 |
| 2026-09-08 | 独立 reviewer 首审 | **NEEDS WORK**，17 项 |
| 2026-09-08 | 修复后 validator（8 任务） | 退出码 **0**；`⚠` **0** 条 |
| 2026-09-08 | reviewer 复审 | **15/17 落地**，找出修复中**新引入的 4 处**（N1 二次 emit 事实错误 / B5 判据装反 / N2 计数矛盾 / N3 判据互斥 / N4 半修） |
| 2026-09-08 | 四处修复后 validator | 退出码 **0**；`⚠` **0** 条（8 任务） |
| 2026-09-08 | **人类确认门通过** | 放行 wave 1；关键路径保持串行 |
| 2026-09-08 | token 登记 | dev-m3-a / dev-m3-b / dev-m3-c / test-m3-a（明文只进 spawn prompt，未落盘） |
| 2026-09-08 | wave 1 派发 | 001→dev-m3-a · 003→dev-m3-b · 004→dev-m3-c，epoch 全为 1 |
| 2026-09-08 | CronCreate | 每 7 分钟跑 validator 报 `stale-dispatch`（阈值 assigned=15m 的一半以内） |
| 2026-09-08 | 三个 dev 全部**当场回执** | 回执协议生效（前两个 sprint 不加时分别丢过 48 分钟与 138 分钟） |
| 2026-09-08 | **澄清 ①**（dev-m3-b） | `config.local.yaml` 谁改：DoD 字面「两处都加」vs 需求 line 569「由人改」⇒ **裁 B（人改）**，由 dev 自己写进 DoD |
| 2026-09-08 | **TASK-004 VERIFIED** | 7/7 PASS、零漂移；verifier 把「前后 status 逐字相同」这条**无法独立复现**的证据换成了 **reflog** |
| 2026-09-08 | 派发 TASK-005 | dev-m3-c（它建的 worktree，最熟） |
| 2026-09-08 | 🔴 **我漏答 blocked_clarification 31 分钟** | dev 用对了文件级信号（PENDING 第 4 条的已实证处方），而**我的 cron 只查 `stale-dispatch`，它对未答复的 `blocked_clarification` 刻意不告警** ⇒ 信号发出了、Leader 侧是瞎的。**已把巡检从 1 项扩到 5 项** |
| 2026-09-08 | **TASK-005 VERIFIED** | 带显式 `--ack-drift`。verifier 把每条判据在两个版本上各跑一遍，并报出**跨仓库漂移缺口**（见 PENDING 第 9 条） |
| 2026-09-08 | 派验 TASK-002 | verifier=test-m3-b |
| 2026-09-08 | **merge TASK-002** | 显式 sha `7b38ef91…` → master `d62ff27b…`；2 文件 146 行零越界；merge 后自测两包绿、`cmd/atlas` **76.7%** |
| 2026-09-08 | **DoD 缺陷第七处** | TASK-002 `non_functional[1]` 写「本任务 **3** 个文件」而 `writes`/`estimated_files` 都是 **2**——初稿含 `CONTRACTS.md`，因 reviewer O9 移给 TASK-008 后**文件数未跟改**。dev-m3-a 发现 |
| 2026-09-08 | 🔴 **判定对象漂移（我批准的）** | TASK-005 第二轮交付发生在 `verifying` 期间 ⇒ discovery `10dd7fde…`→`b59695cf…`、head `1991c49e…`→`7e845847…`。选 **B（ack）不选 A（还原）**：我已 merge 第二轮 ⇒ 还原 discovery 会让它与 master 上的文档**互相矛盾**，而**矛盾不触发告警**。判据：**在「有痕的不一致」与「无痕的不一致」之间选有痕的** |
| 2026-09-08 | **TASK-002 VERIFIED** | 零缺陷。verifier 用**三步法证伪了我给的变异建议**（我建议的变异新旧写法都红，因缺陷自己把目录建了出来）；构造区分性变异才分开 |
| 2026-09-08 | TASK-001 返工 merge | 两个失败分支都补上（fix_items 只要求一条）；语句数 2783→**2785**/2880，未覆盖块 91→**89**。dev 实撞机制事实：`BuildHistory` 失败必须污染 **monthly** 行（`Validate` 自己也经 `History` 接口调 `Preceding`），并加防回归断言钉住 |
| 2026-09-08 | 🔴 **verifier 改派**（人类决定） | TASK-001 在 `verifying` 停留 84 分钟、`stale-dispatch` 连报十轮、`ListAgents` 每轮 `running` 但**零产物**、问询 6 分钟无回复 ⇒ 人类拍板改派 test-m3-b → **test-m3-a**。逃生边 `verifying→verifying`，`epoch` 不变、**`verify_baseline` 不刷新**（判定对象仍 `1c7af81`）。⚠️ 偏离 AD-21 判据（「联系不上且唤不回」），代价与理由已记 wisdom |
| 2026-09-08 | 🔴 **首个返工决定** | TASK-001 `verified → review_fix`（`dod_defect`，不计 rework）。verifier 报**枚举式 DoD 够不到的缺口**：`ingest.go:453/456` 两个 `count==0` 分支——「契约失败⇒侧车已在」有闸，**「侧车失败⇒契约不写」零断言**。判返工的理由是「现在补比下个 sprint 便宜」，不是「严重到必须」 |
| 2026-09-08 | 派验 TASK-005 | verifier=test-m3-a。⚠️ **我的调度失误**：裁决了一处 SKILL.md 改动却没等执行完就派验 ⇒ 走 review_fix 补 |
| 2026-09-08 | 覆盖率口径订正 | 精确复算 base **96.5505%** → head **96.6319%**，**实际上升 +0.08pp 不是持平**。「96.6」是显示值，四舍五入让上升与下跌同形 ⇒ 后续直接比语句数 |
| 2026-09-08 | **TASK-001 VERIFIED** | 8/8 PASS。verifier 做了**变异测试**：M-ORDER 证实顺序断言、M-GUARD-A/B **双向**证实守卫是精确相等断言、四个变异证实 boundary 断言在守；**背对背两轮**覆盖率对照 + 精确语句数复算；按内容锚独立解析 `want` base **34** → head **38** 零删除已排序 |
| 2026-09-08 | 派发 TASK-002 | dev-m3-a（wave 1 全部 verified ⇒ dag 就绪即派） |
| 2026-09-08 | **merge TASK-005** | 显式 sha `c73dbe3b…` → master `1991c49e…`；415 行单文件无越界；`HEAD^2` 复核一致 |
| 2026-09-08 | 🔴 **DoD 判据缺陷第六例** | 「`##` 标题集合交集 ≤ 1」用 `comm -12` 在 **CJK** 上不报错地给垃圾：实测假阳 **5**（子代理另一次 **3**），真值 **0**（`grep -Fxf`）。**我这轮刚改过这条判据，只改了比较对象、没考虑实现工具** ⇒ 判据 = 比较对象 × 求值方式，两者都要钉死 |
| 2026-09-08 | 🔴 **子代理阻塞父实例** | code-simplifier 跑完不返回 ⇒ dev-m3-c 卡 6 分钟（in-process 子代理只能前台运行）。子代理**推断方向反了**（以为父实例停机、建议收回改派）。判据：父 `running` + 子 `running` + 子已自报结论 = 子没返回。处方：Leader 直发消息令其返回 |
| 2026-09-08 | 派验 TASK-001 | verifier=test-m3-b；锚 `7e24b116…`。dev 因我提交 PENDING 使 HEAD 前进而**整批重采两遍**（「代码没变所以数字还有效」是推理，重采给的是观察） |
| 2026-09-08 | **TASK-003 VERIFIED** | 7/7 PASS。verifier 用**四把尺**验 39（要求两把）、**直接证实**「10 行是编译器截断上限」而非推理、在基线树独立求值「11 个调用点」并验「零处仍为单参」、15 处 sha 全 40 位 |
| 2026-09-08 | ⚠️ **消息延迟 ×3** | 两次催 merge + 一次催提交，**dev 每次现采的判据都对**，误判来自「采样 → 送达」之间我做完了动作。延迟不可消除，只能双方以文件为准 + 频繁现采 |
| 2026-09-08 | 🔴 **Leader 未提交编辑阻塞 dev** | 我挂着的 `PENDING-MECHANISMS.md` 被门禁判成 dev-m3-a 的 scope 漂移。`OTHERS` 只从在途任务的 writes/packages 取 ⇒ **Leader 的工作区编辑减不掉**，dev 侧无解。已提交解除 + 记 PENDING 第 8 条 |
| 2026-09-08 | **merge TASK-001** | 显式 sha `ac1acc5e…` → master `4c9e7fdc…`；`git rev-parse HEAD^2` 复核 == 核实的 sha（**处方首次生效**）；merge 后 master 自测全绿、覆盖率 **96.6%** |
| 2026-09-08 | 派验 TASK-003 | verifier=test-m3-b（第二个 test agent，与 TASK-004 并行验） |
| 2026-09-08 | 🔴 **派验太快，dev 补写窗口只剩 39 秒** | `03:31:27` dev_done → `03:32:06` 派验。我发消息时用的是脑子里的旧状态（写「已 `in_progress`」，实际已过两格）⇒ dev-m3-c 补写被 DENY。对照：dev-m3-b 窗口还开着，三处订正全写进去了 |
| 2026-09-08 | **merge TASK-003** | `09e9573f…`（⚠️ **不是** dev 报的 `537b0ef1…`）→ master `755016eb…`；421 行单文件无越界 |
| 2026-09-08 | 🔴 **merge 机制缺口实撞** | 我核实 `537b0ef1` 后用**分支名** merge，dev 在两条命令之间又提交 ⇒ **核实的对象 ≠ merge 的对象**。靠行数对不上才发现，无任何机制。处方：**merge 一律用显式全 sha** |
| 2026-09-08 | 🔴 **跨仓库写通道陷阱**（dev-m3-b 实撞） | 在别的 arcforge 仓库目录里 `cd` 着调写通道 ⇒ 写进**那边**的 `.arcforge/`，**退出码 0 零告警**。已记 PENDING-MECHANISMS 第 7 条 + 钉进 TASK-005/006/007 |
| 2026-09-08 | **DoD 计数错误第四例** | 「`injectTaint` 12 处调用点」实为 **11**（`grep -c` 含函数定义行）——dev-m3-b 查实 |
| 2026-09-08 | 派验 TASK-004 | verifier=test-m3-a；CronCreate 13 分钟（`verifying` 阈值 30m 的一半以内） |
| 2026-09-08 | **merge TASK-004** | `task/TASK-004-m3` @ `134f6351…` → master `0c3fbdc9…`；507 行单文件、无越界；merge 前四项判据各自现读复核 |
| 2026-09-08 | **澄清 ④**（dev-m3-c） | allowlist 粘贴后须重启 **nanoclaw 主进程**（`loadMountAllowlist` 进程内缓存）；冒烟排障关键字需求原文写错（该找 `Additional mount REJECTED`）⇒ 均钉进 TASK-008 §C/§D |
| 2026-09-08 | **澄清 ③**（dev-m3-a） | `History` 与既有导出接口撞车 ⇒ 裁 `ContractHistory`；连带订正守卫插入位置 |
| 2026-09-08 | **澄清 ②**（dev-m3-c） | `container.json` **不是真相源**，手改零效果；且容器只看得见**主 checkout** 的 skill ⇒ 裁「DB 留给人」，两条机制事实钉进 005/006/007/008 |
| 2026-09-08 | 基线覆盖率（锚 `fe95ac7`） | `internal/hestia` **96.6%** · `cmd/atlas` **76.6%** |

## reviewer 首审找出的、我自己没发现的

| # | 我写的 | 实测 | 危害 |
|---|---|---|---|
| U3 | 守卫 33 项 ⇒ +4 = **37** | **34** ⇒ **38** | 精确相等断言里的错数字：dev 照 37 凑会**删掉一个已登记符号** |
| U4 | filepath 守卫在 `hestia_test.go:116-124` | 真守卫在 **135-145** | 验证者按图索骥看到不相干的测试 |
| U5 | `writeAtomic` 在 `queue.go` | 定义在 **`snapshot.go:97`** | 同上 |
| U1 | SKILL.md §3「五条不做」 | **4** 条 | dev 会硬造第五条 |
| O2 | 「确定性：两次输出相同」 | 需求要的是**与期望逐字节相同** | 「输出钉住」整份 DoD **没有条目能让它变红** |
| O9 | CONTRACTS §B「Leader 在 Step 7 补」 | 无 owner、无验收条目 | 归档里有同形前科（验收数字过期四轮无人发现） |
| B5 | verify.py 三态 | 缺「校验行被删」 | **直接击穿「模型只改 narrative」这条核心防线** |

## reviewer 复审找出的、我在修复中新引入的

| # | 我修复时写的 | 实测 | 危害 |
|---|---|---|---|
| **N1** | 修订夹具「二次 emit 即可，零成本」 | `contract emit` **从不写库**，`PriorPublishedAt` 纯读 ⇒ 二次 emit 产出**逐字节相同**的契约 | 与 U3 同类：写死的错误指令，dev 会照做 |
| **B5(1)** | 「check 行数 == 按 `## ` 切分的 N」 | spec §6.2 机器区 **3 个 `## `** 但只有 **2 条 check** | **装反的闸**：照 spec 正确实现的笔记判红；且连标题删可绕过；narrative 写 `## ` 会虚增 N |
| N2 | 夹具「10 个文件」+「另加修订契约」 | 自相矛盾 | **修 3 个计数错误的同一批改动里长出的第 4 个** |
| N3 | 交集与源文件 `## `/`### ` 比 | 八问五链都在 `###` | 又一道装反的闸 |
| N4 | 「六任务各加裁决」 | 裁决进 description，**义务没进 done_criteria** | 验证者对照的是 done_criteria |

**已全部修复**：N1 改为二选一配方（指向本仓库既有 `TestHestiaContractEmitRevisionPeriod`）；B5 改为**两级 + 条数写死**（分段 check 恰好 2 条 + 封条 seal 恰好 1 条，封条作用域**扣除 narrative 块**）；N2 不写死总数；N3 只比 `## ` 集合；N4 义务补进 003/005/006/007 的 done_criteria。

**reviewer 的元观察**：validator 报 `⚠` 0 条**不构成**「无孤儿义务」的证据——`orphan-obligation` 没命中这五处，别当兜底。

U3 的错因值得记：我跑的是 `grep -n 'BuildContract\|WriteContract'`，**只看到了行、从未数过项数**，
却写下「实测现 33 项」——「我查了 X」写在了实际查 X 之前。

## 已知需求原文问题（DoD 已按订正写）

1. 交付前清单写「守卫 **+2**」，Step 5 列了 **4** 项、§B 骨架也写 `+4` ⇒ 按 **+4**（TASK-008 §B 显式记为裁决）。
2. `feat(M3 TASK-001):` 不匹配本仓库门禁 ⇒ 按 `feat(TASK-001): M3 …`。
3. 「若 `groups/` 不被跟踪」是猜测 ⇒ **已实测不被跟踪**，并推出需求没写的一条：那两个文件
   **不在 worktree 里**，须改主 checkout。
4. 笔记文件名：需求三处一致用**无月份**版，上游 spec 用带月份版 ⇒ 取无月份版（TASK-005 裁决）。
5. milestone 前缀需求自身两种写法并存 ⇒ 两种都接受。
