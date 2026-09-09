# 需求 ↔ DoD 双向追溯矩阵（Hestia M3）

**需求文档**：`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-06-hestia-m3-warp-hestia.md`
**生成**：2026-09-08 · Leader · 基线锚 `fe95ac707ea46f9939bb6554261830a95f1b93e1`

## 0. 编号映射（两套编号并存，比对时必须换算）

| 需求文档 | Arcforge | 说明 |
|---|---|---|
| TASK-001 | **TASK-001 + TASK-002** | 原任务横跨 2 package / 7 文件，超 Realistic Scope，拆为 `internal/hestia` 与 `cmd/atlas` |
| TASK-002 | TASK-003 | loom |
| TASK-003 | TASK-004 | nanoclaw 分支/挂载/触发段 |
| TASK-004 | TASK-005 | skill 文档 |
| TASK-005 | **TASK-006 + TASK-007** | 脚本拆为 `prepare.py` 与 `verify.py`（reviewer 反审：5 条业务义务 + 5 条边界装不进 8 条 DoD 上限） |
| （无对应） | **TASK-008** | Arcforge 新增：CONTRACTS `## Sprint M3` §A–§D + 全 sprint 数字对账（记录员模式，解决 reviewer O9「无 owner 的产物」） |
| TASK-006 | **不做（结转）** | AD-M3-2；人类 2026-09-08 确认 |

## 1. 正向：需求义务 → DoD 覆盖

### Global Constraints

| # | 需求原文义务 | 覆盖处 | 状态 |
|---|---|---|---|
| G1 | 三仓库三分支三 PR（atlas `master`；loom `feat/spool-source-param`；nanoclaw 自 **`main`** 切 `feat/warp-hestia`） | 001/002 non_functional 交付流程；003 nf1+nf2；004 fn1；006 nf1 | ✅ |
| G1b | ⚠️ 本机 nanoclaw checkout 不含 warp-research，skill 工作必须基于 `main` | 004 fn1（`fork/main` 切 worktree + `ls warp-research/SKILL.md` 命中） | ✅ |
| G2a | Atlas `Parse`/`Validate`/`Save` 不动 | 001 nf1（`git diff --stat` 为空 + `Save` 不动） | ✅ |
| G2b | `store.go` 不新增方法 | 001 nf1（`git diff -- store.go` 为空） | ✅ |
| G2c | 新导出 `BuildHistory`/`WriteHistory` 登记 AST 守卫 | 001 fn4（+4 项 ⇒ 37，`-run ExposesNoWrite` PASS） | ✅ |
| G2d | `internal/hestia` 覆盖率 ≥ **96.6%** | 001 nf1；002 nf1（不因本任务回退） | ✅ |
| G2e | 业务字段名字面量只在 `fields.go` 与 `_test.go` | 001 boundary（`grep -nE` 判据 + `"monthly"` 等三例外） | ✅ **初稿曾漏，已补** |
| G3a | `reviewed: false` **无条件覆写**不得放松 | 003 fn1（`TestArchiveSourceHestiaAllowed` 断言 payload 声明 `true` 也被改回）+ fn2（🔴 明写） | ✅ |
| G3b | `source` 白名单只有 `web-research` / `hestia` | 003 fn2（闭集）+ fn1（未知值 DENIED 且不落盘） | ✅ |
| G3c | 既有 `TestArchive*` / `TestTaint*` 全绿 | 003 nf1（35 基线 + 4 = 39，`grep -c` 复核） | ✅ |
| G3d | `spool_write_allow` 模板与 `config.local.yaml` **两处**都改 | 003 fn3（两处，且注明 local 是 gitignored 本机文件） | ✅ |
| G4a | skill 脚本只用 Python 标准库 | 006 fn2（`grep -n 'import '` 自查贴输出） | ✅ |
| G4b | 派生算法与 Atlas `Evaluate` 同源（`_mom` 优先 → monthly `_ytd` ÷ MM → 3/6/9/12） | 006 fn2 + fn3（含 `monthly_average` 函数级订正测试） | ✅ |
| G4c | 三期 golden 温度 2 / 0 / 1 必须通过 | 006 fn3（🔴 钉住，`temp_known` 恒 4） | ✅ |
| G4d | frontmatter 不写 `reviewed` / `source`（Spool 覆写） | 006 fn4；005 fn1（SKILL.md §3 不做） | ✅ |
| G5a | SKILL.md 明写模型只改 `<!-- narrative -->` 段 | 005 fn1（🔴 模型边界必须明写） | ✅ |
| G5b | `verify.py` 校验行不一致即拒绝写回 | 006 boundary（三态：未改 0 / 改表 1 / 只改叙述 0） | ✅ |
| G6 | 一次会话只处理一份契约 | 005 fn1（SKILL.md §3）；004 fn3（`CLAUDE.local.md` 触发段第四条） | ✅ |
| G7 | 注释里引用任务编号**带 milestone 前缀** | 全部六个任务 description 的「任务编号映射」表 | ✅ **初稿曾与门禁冲突，已补映射表** |
| G8a | 集成冒烟是**人执行**的 | AD-M3-2；002 fn3（CONTRACTS §D 显式写明结转） | ✅ 结转 |
| G8b | agent 不改 `mount-allowlist.json` | 004 boundary（🔴 只写片段供人粘贴，agent 绝不改） | ✅ |
| G8c | agent 不 `launchctl kickstart` | 003 nf1（⚠️ selvage 重启是人执行，agent 不执行） | ✅ |
| G8d | agent 不在 Warp 会话里替人说话 | 结转（属需求 TASK-006 范围，本 sprint 无任务触及 Warp 会话） | ✅ N/A |
| G9 | 提交前跑 `code-simplifier`（Go + Python） | 001/002 交付流程第 3 步；006 nf1（Python 脚本） | ✅ |

### 需求 TASK-001（→ Arcforge 001 + 002）

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| R1-1 | `history.go` 九个符号 + 结构与 JSON tag | 001 fn1 | ✅ |
| R1-2 | 四条 `history_test.go` 测试 | 001 fn2 | ✅ |
| R1-3 | `ingest.go` **先侧车后契约** | 001 fn3 | ✅ |
| R1-4 | `TestIngestWritesHistoryBesideContract` | 001 fn3 | ✅ |
| R1-5 | M2a `TestIngestNoContractOnDuplicate` 相应调整 | 001 fn3（要求写进 discovery `decisions` 说明调整方式） | ✅ |
| R1-6 | `cmd/atlas` `contract emit` 同产侧车 | 002 fn1 | ✅ |
| R1-7 | `--stdout` 只打契约不打侧车 | 002 fn2 + boundary | ✅ |
| R1-8 | AST 守卫 +4 | 001 fn4 | ✅ |
| R1-9 | ⚠️ `"monthly"` 字面量不受字段名守卫约束 | 001 fn1 + boundary（三例外） | ✅ |

### 需求 TASK-002（→ Arcforge 003，loom）

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| R2-1 | 三条 archive 测试 + 一条 taint 测试 | 003 fn1（逐条列出断言） | ✅ |
| R2-2 | `injectTaint` 改签名，**12 处**既有调用点全改 | 003 fn2（实测 12 处） | ✅ |
| R2-3 | `archive.go` 在 `hasFrontmatter` **之后**读 source | 003 fn2 | ✅ |
| R2-4 | `configs/README.md` 字段表 + `source` 说明段 | 003 fn3 | ✅ |
| R2-5 | `go test ./...` 全绿；PR | 003 nf1 | ✅ |

### 需求 TASK-003（→ Arcforge 004，nanoclaw）

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| R3-1 | 从 `main` 切 `feat/warp-hestia`；`warp-research` 先例在 | 004 fn1 | ✅ |
| R3-2 | ⚠️ 先看 `git ls-files groups/cli-with-warp` 判断是否被跟踪 | 004 description（**已实测：不被跟踪**，`.gitignore:15`）+ nf1 | ✅ **由猜测升级为实测** |
| R3-3 | `container.json` 加队列挂载（保留既有 vault 项） | 004 fn2 | ✅ |
| R3-4 | `CLAUDE.local.md` 触发段五条 | 004 fn3 | ✅ |
| R3-5 | mount-allowlist 片段（人粘贴，agent 不改） | 004 boundary | ✅ |
| R3-6 | ⚠️ 队列根不存在时 mount-security 行为待冒烟第一步看日志 | 004 boundary（明写「未验证，留待冒烟」） | ✅ 结转 |

### 需求 TASK-004（→ Arcforge 005，skill 文档）

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| R4-1 | `SKILL.md` frontmatter + §1/§2/§3 | 005 fn1 | ✅ |
| R4-2 | 六步齐全、不跳步不并行 | 005 fn1 | ✅ |
| R4-3 | Step 2 两个边界（侧车缺失 / 已在 processing） | 005 boundary | ✅ |
| R4-4 | `note-format.md` 六块 | 005 fn2 | ✅ |
| R4-5 | `glossary.md` 五块 | 005 fn3 | ✅ |
| R4-6 | `methodology.md` **提炼不整篇复制** | 005 fn4（给了可判的判据：字数 + 结构重组） | ✅ |

### 需求 TASK-005（→ Arcforge 006，脚本）

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| R5-1 | 夹具 10 文件 + 手写 `existing-*.md` | 006 fn1 | ✅ |
| R5-2 | `prepare.py` / `verify.py` 接口与模块结构 | 006 fn2 | ✅ |
| R5-3 | `test_prepare.py` 全部测试类 | 006 fn3 | ✅ |
| R5-4 | 🔴 **需求自标订正**：`2022-07` 那条走 `_mom` 不是 `_ytd÷MM`，真样本要另造内联最小契约调 `monthly_average` | 006 description（🔴 单列）+ fn3（**必须包含**） | ✅ **需求原文自己标的订正，已单列** |
| R5-5 | 笔记形态：14 键 + 五标记 + 批注保留 + `--print-name` | 006 fn4 | ✅ |
| R5-6 | `verify.py` 三态 | 006 boundary | ✅ |
| R5-7 | 输入不合法 exit 非零 + stderr | 006 error_handling | ✅ |
| R5-8 | PR（`--repo newthinker/nanoclaw --base main`） | 006 nf1 | ✅ |

### 交付前检查清单

| # | 义务 | 覆盖处 | 状态 |
|---|---|---|---|
| C1a | atlas：覆盖率 / `Save` 不动 / 守卫 / `contract emit` 产侧车 | 001 nf1 + fn4；002 fn1 | ✅ ⚠️ 需求写「守卫 **+2**」而 Step 5 自己列了 **4** 项、CONTRACTS §B 骨架也写 `+4` ⇒ 「+2」是需求内部笔误，DoD 按 **+4** 写 |
| C1b | **真语料回归数字一个不变** | 001 nf1（`backfill load --allow-incomplete` exit 0；`218 = 217 + 1` · `217 = 213 + 4` · `97 = 76 + 21` · 字段冲突 0 · 口径路由违反 0 逐字相同；另 `find` 查空目录） | ✅ **初稿曾漏，已补** |
| C2 | loom：四条新测试 / 既有绿 / 两处白名单 / selvage 重启 | 003 fn1/fn3/nf1（重启结转） | ✅ |
| C3 | nanoclaw：PR 合并 / checkout 在 main / unittest 全绿 / 三期 golden | 006 fn3 + nf1（合并与 checkout 切换属人执行，结转） | ✅ 部分结转 |
| C4 | 冒烟五条 / 批注保留验过 / `examples/` 回填 | **结转**（需求 TASK-006）；批注保留由 006 fn4 在**脚本层**验过 | ✅ 结转 |
| C5 | CONTRACTS `## Sprint M3` §A–§D；vault 回写 | 002 fn3（§A/§D 写实，§B/§C 骨架）；§B 值由 Leader 在 Step 7 补；vault 回写**结转** | ✅ 部分结转 |
| C6 | 一切自证数字采于最后一次代码改动之后 | 全部八任务交付流程（🔴 统一重采）+ TASK-008 boundary（采锚并核空） | ✅ |

## 2. 反向：DoD → 需求出处（凭空 DoD 检查）

逐条核过，**无凭空业务义务**。以下 DoD 条目在需求文档里没有直接对应，但**不是凭空**——它们是本项目 CLAUDE.md 的既有机制纪律，出处已在条目内注明：

| DoD 条目 | 出处 |
|---|---|
| commit subject 必须 `^[a-z]+\(TASK-00X\):`（🔴 与需求原文的 `feat(M3 TASK-001):` 冲突） | CLAUDE.md + `task-completed.sh` 实读；sprint-045 AD-2 |
| merge 必须在 `dev_done` 之前 | `task-completed.sh` 的 `git log --grep` 不带 `--all` |
| discovery 与指针必须在 `dev_done` 之前落定 | AD-29 `verify_baseline` + discovery 时机守卫 |
| worktree 隔离；谁建谁拆 | CLAUDE.md「每 dev 独立 worktree」 |
| 跨仓库任务写交付记录文档 | AD-M3-1（本 sprint 新裁决，人类 2026-09-08 确认） |
| 锚一律写全 sha | CLAUDE.md「验证/隔离命令里的锚必须钉全 sha」 |
| `container.json` 改前备份 | 本 sprint 新增（未跟踪运行时数据，git 无法回滚） |
| TASK-004 可能零提交是预期结果 | 本 sprint 新增（`groups/*` 被 gitignore 的推论） |

## 3. 机器检查结论

### 3.1 Leader 自查（追溯矩阵）
- **孤儿需求**：初稿 **3 个**，已全部补入：G2e 业务字段名字面量守卫 → 001 boundary；
  G7 milestone 前缀与门禁 commit 格式冲突 → 八任务各一张编号映射表；
  C1b 真语料回归数字一个不变 → 001 non_functional。
- **凭空 DoD**（业务义务层面）：**0 个**。

### 3.2 独立 reviewer 首审 —— **NEEDS WORK**，17 项，全部核实为真并修复

| 类 | 项 | 我写的 | 实测 | 危害 |
|---|---|---|---|---|
| 错数字 | U3 | 守卫 33 ⇒ +4 = **37** | **34** ⇒ **38** | 精确相等断言：照 37 凑会**删掉一个已登记符号** |
| 错锚 | U4 | `hestia_test.go:116-124` | `TestHestiaCmdDoesNotResolveDBPath` **135-145** | 验证者看到不相干的测试 |
| 错锚 | U5 | `writeAtomic` 在 `queue.go` | 定义在 **`snapshot.go:97`** | 同上 |
| 错计数 | U1 | SKILL.md §3「五条」 | **4** 条 | dev 会硬造第五条 |
| 错计数 | U2 | 「五个标记」而自列 6 项 | **6**（现为 7，含 seal） | 同上 |
| 不可测 | U6 | 「字数显著少于」「结构是重组的」 | 无阈值 | 验证者无法判 PASS/FAIL |
| 不可测 | U7 | 只钉 `_mom`/`÷6`/订正样本 | `÷3`/`÷9`/`÷12` 零断言 | 派生分支半数不设防 |
| **零覆盖** | O1 | frontmatter「14 个必需键」 | 那 14 个是需求测试里的**弱断言**，**一个派生指标都不含** | `hh_short_monthly`/`bill_ratio`/4 signal 等 13 键零覆盖 |
| **零覆盖** | O2 | 照抄需求的 `test_deterministic` | 那只做**自比**，需求要的是**与期望逐字节相同** | 「输出钉住」**没有任何条目能让它变红**：表乱序、表头错、少一列都全绿 |
| 零覆盖 | O3 | 只要求 TASK-005 把修订路径**写进文档** | 没有条目要求**脚本实现它** | `is_revision` 路径零实现零测试 |
| 零覆盖 | O4/O5 | 只列函数名 | 判读提示、三列结构、口径标注无断言 | 「口径标注」是跨口径对比禁忌的落地载体 |
| 零覆盖 | O6 | — | TASK-003（纯 Go）无 code-simplifier | 违反需求 line 27 全局规范 |
| **无 owner** | O9 | 「Leader 在 Step 7 补 §B」 | 无 owner、无验收条目 | 归档同形前科：验收数字过期**四轮**无人发现 |
| **击穿防线** | B5 | verify.py 三态 | 缺「校验行被删」 | 模型删 `<!-- check: -->` 可 exit 0 ⇒ 直接击穿「模型只改 narrative」 |
| 边界 | B1/B2/B6 | — | 解析失败⇒0 做除数；`temp_known` 恒钉 `== 4`（unknown 场景零覆盖）；无批注标题 | — |
| 锚 | B7/G5 | 「侧车存在」当 001+002 生效的证据 | 不记构建树 sha | 与本 sprint「锚一律全 sha」自相矛盾 |
| 图 | G2 | 006 `context_from` 缺 001 | — | dev 得自己猜侧车形状 |

**U3 的错因值得单记**：我跑的是 `grep -n 'BuildContract\|WriteContract'`，**只看到了行、从未数过项数**，
却写下「实测现 33 项」——「我查了 X」写在了实际查 X 之前。

### 3.3 独立 reviewer 复审 —— 15/17 落地，找出**我在修复中新引入的 4 处**

| # | 我在修复中写的 | 实测 | 危害 |
|---|---|---|---|
| **N1** | 「修订夹具：对同一 period **二次 emit 即可，零成本**」 | `runHestiaContractEmit` 只有 `st.Current`→`st.PriorPublishedAt`（纯 `SELECT`）→`BuildContract`→`WriteContract`，**从不写库** ⇒ 二次 emit 产出**逐字节相同**的契约 | 与 U3 **完全同类**：一条写死的、语气笃定的错误指令，dev 会照做并得到错误结果 |
| **B5(1)** | 「按 `## ` 切分出 N，断言 check 行数 == N」 | spec §6.2 骨架机器区 **3 个 `## `** 但只有 **2 条 check**（`## 信号` 段不发） | **装反了的闸**：照 spec 正确实现的笔记当场 `2 != 3` 判红；且连标题一起删时 N 与 count 同减恒等（真绕过）；narrative 里写 `## ` 会虚增 N（八问框架天然诱导） |
| N2 | 夹具「= **10 个文件**」+「另加一份修订契约」 | 二者自相矛盾 | **在修 U1/U2/U3 三个计数错误的同一批改动里长出的第四个计数错误** |
| N3 | 「`## ` 集合与源文件 `## `/`### ` 集合交集 ≤ 1」 | 八问在 `###`（344-499）、五链在 `###`（522-570）；dev 把八问写成 `## ` 最自然 ⇒ 交集 = 8 | 又一道装反的闸：判红合格文档 |
| N4 | 「六个任务各加裁决」 | 裁决进了 description，但**义务没进 `done_criteria`**（003–008 全 0） | 验证者逐条对照的是 `done_criteria` ⇒ 没有条目会让它变红 |

**四处修复**（全部落盘核实）：
1. **N1** → 删「二次 emit」，改为二选一并要求 dev 写明选了哪条：(a) 在语料里**实测**找同期两个 `published_at`
   的期次（218 篇里是否真有 **无人验证过**，不许假定）(b) 照本仓库既有 `TestHestiaContractEmitRevisionPeriod`
   （`hestia_test.go:1570`）的配方 `Save` 一条同期、更晚 `PublishedAt`、不同 `ArticleID` 的观测再 emit。
2. **N2** → 不写死总数，以文档 ②节 `ls fixtures/` 实际输出为准，但五期必须齐。
3. **B5** → 判据**不从文档结构推 N**，改为**两级 + 条数写死**（三处逐字一致）：
   **① 分段 check `<!-- check: -->` 恰好 2 条**（`## 本期数据`、`## 前 12 期`；`## 信号` 段不发，依 spec §6.2）
   **② 封条 seal `<!-- seal: -->` 恰好 1 条**，覆盖「`<!-- machine-generated: begin -->` 下一行 → seal 行之前，
   **扣除** narrative 块（含两行标记）」。封条是删除类篡改的唯一防线：删 check 行 / 删整段 /
   改无 check 保护的 `## 信号` 段都会变红，而 narrative 被扣除 ⇒ 模型正常写叙述不受影响。
4. **N3** → 交集**只与源文件的 `## ` 集合**比（实测 12 个「一、二、三…」式章节名，没人会照抄）。
   另 **N4** → milestone 前缀义务补进 TASK-003/005/006/007 的 `done_criteria`。

**reviewer 的元观察（值得记进 wisdom）**：
> validator 报 `⚠` **0** 条、`orphan-obligation` 没有命中这五处 ⇒ **「validator 干净」不构成「无孤儿义务」的证据**，别把它当兜底。
> 「改数字」这个动作本身没有反馈回路——写下时无人复算。建议给 DoD 里每个自证数字加一句来源。

### 3.4 需求原文自身的问题（DoD 已按订正写）
1. 交付前清单写「守卫 **+2**」，Step 5 列了 **4** 项、§B 骨架也写 `+4` ⇒ 按 **+4**（TASK-008 §B 表下记为裁决）。
2. `feat(M3 TASK-001):` **不匹配本仓库门禁**（只认 `^[a-z]+\(TASK-001\):`）⇒ 按 `feat(TASK-001): M3 …`。
3. 「若 `groups/` 不被跟踪」是猜测 ⇒ **已实测不被跟踪**（`.gitignore:15`），并推出需求没写的一条：
   那两个文件**不在 worktree 里**，须改主 checkout。
4. 笔记文件名：需求 line 853/967/1012 三处一致用**无月份**版，上游 spec §6/§8.2 用带月份版 ⇒ 取无月份版。
5. milestone 前缀需求自身两种写法并存（line 25 `M3 的 TASK-001` vs line 453/537 `M3 TASK-002`）⇒ 两种都接受。
6. spec §2 声称「`Wiki/Macro/PBOC/` 由 Spool 首次写入时自动建」而 §0.3 说「目标目录都不存在」
   ⇒ **那是设计意图不是实测**，TASK-003 要求记明「spec 如此声称、本 sprint 未实证」。

### 3.5 最终状态
- **validator**：干净退出码 **0**，`⚠` 段 **0** 条（8 任务 / 21 条规则）。
- **每任务 DoD 条数**：001=8 · 002=6 · 003=7 · 004=7 · 005=8 · 006=8 · 007=6 · 008=7，全部 ≤ 8。
- **wave/deps 自洽**：006(w3) > max(001 w1, 002 w2, 004 w1, 005 w2)=2 ✓；007(w4) > 006(w3) ✓；008(w5) > 007(w4) ✓。
- **writes 两两互斥** ✓；worktree 移交链完整（004 建 → 005/006 复用 → **007 拆**）✓。
