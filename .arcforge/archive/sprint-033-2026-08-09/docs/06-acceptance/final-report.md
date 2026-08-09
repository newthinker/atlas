# Sprint 033 Final Report — M1b-1 `internal/hestia` types + store

**分支** `feat/hestia-store`（基于 `origin/master` `d7c9c69`）· **终态 HEAD** `823ca15`
**任务** 7/7 accepted · validator exit=0 · 返工 4 次（T1 一次、T4/T5/T6 各一次，后三次来自 QA）

## 交付

| | |
|---|---|
| 代码 | 10 commit · `internal/hestia` 新增包 · 11 files / +3561 行 |
| 测试 | **66 顶层 / 86 含子测试 / 0 FAIL / 0 SKIP / 0 DATA RACE** |
| 覆盖率 | 89.3%（`go test -cover`）／89.0%（`cover -func`，门禁口径） |
| 回归 | 全仓 64 包 0 FAIL · `go.mod`/`go.sum` 未动 · 既有包零改动 |

架构与决策见 `changelog.md`；交接缺口见 `01-design/handoff-*.md`（H1–H11）。

## 质量过程

**独立 reviewer 在 DoD 阶段补入 5 条判据**（G1/G2/G3/G4+G5/G10），需求文档均未覆盖，全部在交付中闭合。其中 G4 的实证最有力：**删掉 `PRIMARY KEY` 整行，需求文档全部测试依然全绿**——它的示例测试显式跳过了那一行。

**QA 两轮审查发现 4 条真缺陷**，逐任务验证全部发现不了——它们藏在任务之间：

| | 缺陷 | 为什么单任务验证看不见 |
|---|---|---|
| C1 | 视图漂移静默存活（CRITICAL） | 要同时看 `verifyObservationsSchema` 查什么、`CREATE VIEW IF NOT EXISTS` 的语义、以及「视图是下游读取入口」这个包级事实 |
| C2 | G10 守护被错拼状态绕过 | 要并排看 `checkValues`（白名单）与 `checkReportConsistency`（单值黑名单）**在同一条 Save 路径上用了两种强度** |
| C3/C4 | pending 八列与 `refreshArticleID` 无守护 | 要发现 H2 宣称的「三处同序已闭合」**只对观测表成立** |

四条均已修复，并由 test-agent-20/21 与 QA 的 Skeptic lens **三方独立取证**。

## 方法论沉淀

### 自证机制本身会骗人的四种形态（本 Sprint 各撞至少一次）

| 形态 | 谁撞的 | 表现 |
|---|---|---|
| 「0 红」被编译失败伪造 | dev-41 ×2 | `if false {` 让变量未使用 ⇒ 编译失败 ⇒ PASS=0，脚本打印「无 FAIL（存活）」 |
| **「全红」被语法错伪造** | dev-41 | 删 `PRIMARY KEY` 整行留下尾逗号 ⇒ 6 条红，其中 **5 条与 PK 无因果** |
| 变异工具静默改坏文件 | dev-41 | perl 分隔符冲突把首行变成 `if` ⇒ 三条自证全部挡不住 |
| **取错退出码** | leader / dev-41 / dev-42 各一次 | `\| tail` / `\| head` 之后取 `$?`；用 stderr 文案当判据 |

⇒ 判据从三条自证扩到**四条**（加文件完整性），且 `go vet` **红绿都要查**——

> 只在存活时查 vet，等于默认「红了就是测试起作用了」，而编译失败恰恰让所有测试都不跑、计数塌成 0，**看起来像全红**。**任何让被测代码根本没运行的破坏，都会同时伪造全绿与全红两种极端。**

并且 **PASS 计数用「低于基线」而非「等于 0」**——部分文件编译失败可能只塌一部分。

### KILLED 不等于因果唯一

- **查因果，不数条数**：连带伤害的标志是「红的测试与变异点**无因果关系**」。反例：M9 红 18 条但同因（`passing()` 返回的就是 `CheckPassed`），属合法多杀。
- **消融才证明因果唯一**：同一变异 + 停用那条测试 ⇒ 完全存活 ⇒ 闭合它的因果唯一落在这条上。**KILLED 只说明「有东西抓住了它」。**
- **INVALID 是关于变异写法的结论，不是关于被测代码的结论**：test-agent-21 的 M4 因正则贪婪匹配判 INVALID，**若就此收工，重做的 M4' 那条存活变异就会被漏掉**。同理 test-agent-20 更正 dev 的「N2 编译不过」——换个写法（让参数而非局部变量未使用）就编译通过且被杀。

### fixture 强度决定判据有效性

dev-42 自查发现弱 fixture 让「比对 `fieldOrder`」这条 DoD **等于完全没验**（M2' 在弱 fixture 下 43/43 全绿）。test-agent-20 补了 **C0 控制组**（弱 fixture + 正确实现 = 全绿），才排除「弱 fixture 本身跑不过」这个混淆、使对照闭合。

dev-42 的 C1 fixture 也栽过一次：两个 `period_type` 用同一 `published_at` ⇒ **危害根本没复现**，而测试照样绿、变异照样被杀。抓住它的是测试里的**前提自证**。

### 「固定文案里恰好含有针」——本包第 3 次同形

| | 断言 | 为什么恒真 |
|---|---|---|
| T2 | `Contains(err, "period")` | 错误串 `meta.period_type must...` 含子串 |
| T3 | `Contains(ddl, "period")` | 被下一行 `Contains(ddl, "period_type")` 蕴含 |
| **T5（新）** | `Contains(err, "fail")` / `Contains(err, "")` | 前者被错误模板 want-list 里的 `"failed"` 满足；后者**恒真** |

**第三次尤其值得看**：恰恰是**最难诊断的两个输入**（漏填零值、词形错拼）上这条要求无保护。

⇒ **建议立包级规约**：对可能出现在固定文案中的短标识符，禁用子串断言，改断言整片段（如 `fmt.Sprintf("%s=%q", field, value)`）。

### 测试名集合差，而不是比数字

test-agent-21 用 `comm -23` 证明返工全程**无任何测试被删**：

> 返工最容易的退化是「改实现顺手删掉挡路的旧测试」，它在「总数变多了」的表象下**完全隐形**——61→66 光看数字排除不掉「删 2 加 7」。

### 混合信号无法通过「减去地板」净化

dev-42 提出「先量出伪影地板再减掉」，dev-41 实测推翻：地板 t0=4 / tA=5 / tB=6，因为 `missing-discovery` 只对 `verified` 生效，而**被测变量恰恰就是 status**。

> **地板 = f(被测变量)。** 按该补丁 tB 会被读成「2 条真实新问题」，实际是 0 条——**这个补丁会制造一个不存在的问题，方向和它想防的正好相反。**

⇒ **按规则名切片，只读被测规则的计数**，不读总数、不读 exit code。

### 团队行为

**几乎每条重要发现都伴随一次自我更正，没有一条是被要求的**：dev-41 自陈「显式 pathspec 只限制文件不限制 hunk」与一次绕过写通道；dev-42 撤回「派给 test-\* 可解」的推荐（「我把一个双向的约束只查了一个方向就下了推荐」）与自己提的地板补法；test-agent-21 撤回两处过期判断；test-agent-20 更正 dev 的 N2 记录且方向**对交付有利**。

dev-41 补的那条对称性尤其：**它在自己结论被证实之后，仍去指出它当初也没自证**——「两者的差别是运气的位置，不是方法的严谨度」。

它还给出了这一切得以发生的前提：

> **「靠量而不是靠同意」的前提是把量的入口留出来**——互相纠正之所以可能，是因为双方都留了可查的把手；**只给结论不给过程，对方只能选择相信或不相信。**

## Leader 的失误（3 处，均由 teammate 先发现）

1. **派单遗漏 discovery 字段指针** —— 只说了「写 discovery 文件」，没说「同时设字段」。而 dev 转 `dev_done` 后即失去写权限，validator 的 `missing-discovery` 是阻断级 ⇒ 卡住两次（一次 3 小时）。
2. **DoD 里 git 基准写 `master` 而非 `origin/master`** —— 本地 master 落后三个 merge，照抄会把 M1a 的 10 个文件算进本 Sprint 范围。**命令原文相同、锚不同、结论不同，而错的那个不会自我暴露。**
3. **开三个 `writes` 完全相同的 review_fix 前没跑 validator** —— 规则本来能拦住。**拦在派发前只会阻止我开第三个任务，拦在事后就成了死锁**（详见 D8）。

第 3 条的直接代价：两个 dev 在同一文件并行，dev-41 的整文件 `git add` 把 dev-42 的 C1 一并提交进 TASK-005 的 commit。**发现它的是测试计数差额**——dev-42 只加 1 条测试却从 61/75 变成 64/84。

## 逃生开关使用记录

**三次 `ARCFORGE_SKIP_VALIDATE=1`**（TASK-004/005/006 的派验），`transitions.jsonl` 中 `skip_validate: true` 留痕。

用它是因为 D8 的死锁：validator 阻断我的所有派发边，而解锁需要先派验——互为前提。**这三次派发未经任务图校验**，审计时应能一眼看到。

保留该开关的设计前提值得复述：**「只出声不落盘的开关就是后门」**。

同段注释还写着：**「刹车拦的是任务图问题，不是代码缺陷……装了刹车不等于不会再出那种洞」**——本 Sprint QA 抓到的 C1–C4 全是实现层的，刹车对它们一无所知。

## 待同步 hooks 清单（人类执行）

**本 Sprint 未改 `project-template/`**，以下为**新登记的机制需求**，需先在 ArcForge 仓实现 + TDD，再由人类同步。

| 编号 | 缺陷 | 修法方向 |
| --- | --- | --- |
| D1 | 漂移口径改为文件路径本身后，游离的 untracked 文件绊住每个任务 | Sprint 开工前扫一遍 untracked；或门禁忽略声明范围外的非代码文件 |
| D2 | 任务号跨 Sprint 复用污染 `COMMITTED_MINE` 匹配 | C5 那侧已有的时间下界没用到这一侧，是个不对称 |
| **D3+D7** | **`teammate-idle.sh` 同一 hook 的两个反面** | 见下 |
| D4 | `scope-writes-outside-packages` 形状级假阳 | 与 `task-completed.sh` 的 `pathScopesIntersect` 口径对齐（注释声称「三方统一」，实际这条没跟上） |
| D5 | 运行时矩阵缺 `verifying→verifying`（CLAUDE.md 三处写了它） | **已提供 jq 命令，至今未执行** |
| D6 | `dev-*` 在整个 `docs/` 树下无任何可写路径 | 记录员代理模式需覆盖「产出物落在别角色专属目录」 |
| D8 | scope 互斥死锁（validator 只拦 leader 派发边） | 派发前必跑 validator；或让 `dev_done` 不计入互斥口径 |
| D9 | 「显式 pathspec」只限制文件不限制 hunk | 纪律补：提交后 `git show --stat` + 按符号名 grep（**不依赖调度正确**） |
| D10 | 混合信号无法减去地板净化 | 按规则名切片输出，而非只给总数与 exit code |
| D11 | 写通道 `doc` 子命令接受任意路径且只能覆盖不能删除 | 校验扩展名；或提供受控的 `--retract` |

### D3+D7 —— 最重的一条，本 Sprint 代价最大

`teammate-idle.sh` 的保活条件**与任务归属脱钩**，产生两个相反的故障：

| | 表现 | 本 Sprint 代价 |
|---|---|---|
| **该唤醒的不唤醒** | `test-*` 分支的保活条件里**没有**「`verifier==本实例 && status==verifying`」，只看全局有无 `dev_done` ⇒ 已派给我的 `verifying` 任务**不构成保活条件** | test-agent-20 卡 6 小时、test-agent-21 卡 **6.5 小时** |
| **不该唤醒的猛唤醒** | `qa-*` 分支的 `MINE` = 全部 `verified` 任务（刻意不按 `assigned_to` 过滤），而闸门要求 `05-review/*.md` 存在且更新 ⇒ **目录为空时闸门整块跳过，第一次评审永远无法放行** | qa-agent-10 空转约 **1500 次** |

**test-agent-21 的表述是最准确的诊断**：

> 空闲后 `TeammateIdle` 不是心跳，它只在「转 idle 前」拦一次。**结构上我根本不可能自己发现这次派发。**

这更正了 Leader 的一个判断——我曾说「你的沉默和你的消失对我是同一件事」，那个框架预设了「你本可以发现」，**而在这个机制下它不能**。

**修法**：`test-*` 分支加「且 `verifier` 未指定或指向本实例」；`qa-*` 分支同理按归属过滤，并处理 `05-review/` 为空的首次评审场景。

**不要在症状层加信号**（让「已确认无活」对 leader 可见）——那只会把噪音变成可见的噪音，根因是 hook 在唤醒一个**结构上无法响应的对象**。

### 文档漂移

`templates/CLAUDE.md.template` 与 `write-matrix.json` 的边表**必须逐条对应**（CLAUDE.md 明写有测试做双向集合相等比对），而 `verifying→verifying` 在文档里有、矩阵里没有。**那个测试要么不存在、要么没跑、要么比对的不是这张表。**

**同步命令（人类执行）**：

    cd ~/workspace/go/src/github.com/newthinker/atlas
    jq '.transitions["verifying->verifying"] = ["leader"]' .arcforge/write-matrix.json > /tmp/m.json \
      && mv /tmp/m.json .arcforge/write-matrix.json

**消费项目（atlas 这类业务仓库）的矩阵漂移没有任何机制能检出**——`check-runtime-sync.sh` 要求本地存在 `templates/`，而消费项目只有运行时副本，故 exit 3「不适用」。

复制后运行 `bash project-template/scripts/check-runtime-sync.sh` 核对（agent 只呈现清单，不执行这些命令）。
