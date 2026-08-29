# M1c-3a 终验收报告 —— hestia 解析覆盖

**Sprint**：M1c-3a · 解析覆盖 ｜ **周期**：2026-08-25 → 2026-08-29
**基线**：`7bccab44`（sprint-039 归档）→ **交付**：`0c9f4e87`（master）
**改动**：47 个 commit、30 个文件（16 × `internal/hestia`、14 × `testdata`）
**需求文档**：`hestia/docs/superpowers/plans/2026-08-25-hestia-parse-coverage.md`

---

## 一、目标与结果

**目标**：把解析覆盖率从 22/218 提上去 —— 接上 55 篇月报、138 篇社融独立报告、修 3 条已知失败。

| 观察项 | 基线 @`4a12794` | **交付 @`0c9f4e8`** |
|---|---|---|
| 尝试解析 | 25 期 | **199 期** |
| 非社融 `n`（`m2`） | 22 | **50** |
| 社融 `n`（`tsf_stock`） | 4 | **79** |
| 环比一节 | 整节 `—` | **annual 6 / monthly 68** |
| 基线那 3 条失败 | 3 篇 | **全部归零** |
| 包覆盖率 | — | **95.9%** |
| 测试 | — | **1220 条 RUN**（556 个顶层 `Test` 函数） |

🔴 **「解析失败 3 → 38 篇」是可见性提高，不是回退。** 那 138 篇社融报告此前被
`calibrate.go` **硬过滤掉，既不贡献样本也不产生失败**；另有 4 期被误标为「没有累计数据」。
**从「静默写销」变成「响亮失败」是本 sprint 最重要的一类改进**，而它在计数上表现为失败数上涨。
详见 `06-acceptance/calibrate-AFTER-report.md`。

**四格恒等式**：待解析 199 + 本迭代不解析 19 + 标题解析不出 0 = **218** ✓

---

## 二、任务完成情况（12/12 `accepted`）

| ID | 标题 | 返工 |
|---|---|---|
| TASK-001 | 累计前缀：`periodAlt` 与 `cumulativePeriods` 两处硬卡点 | 1 |
| TASK-002 | 社融存量/增量独立报告的整篇抽取包装 | 0 |
| TASK-003 | extractor 取值域与必填集：新增 4 个值 | 0 |
| TASK-004 | `detectExtractor` 判据重构：板块集合 × `period_type` | 0 |
| TASK-005 | 模板措辞变体：企业贷款锚点 + M2 全角括号 | 0 |
| TASK-006 | `extractFields` 按 extractor 决定板块适用性 | 0 |
| TASK-007 | `parseTitle` 加 kind、`Parse` 三路分派、删 monthly 分支 | 0 |
| TASK-008 | 真跑 calibrate 验收、回归测试与 CONTRACTS 登记 | **3**（= `max_rework`） |
| TASK-009 | 分部门口径守卫：拒绝把当月分部门值装进 `*_ytd` | 0 |
| TASK-010 | calibrate 放行社融两种 kind，月报按口径四类分流 | 1 |
| TASK-011 | 两条 HIGH：板块路径作用域切分 + 锚点缺席不得静默放行 | 0 |
| TASK-012 | `periodAlt` 认「N-M月」+ 截断守卫豁免 + `validExtractors` 逐项表态 | 1 |

**validator**：`✓ 任务图校验通过（12 个任务，规则 19 条）`，1 条告警（`orphan-obligation`，
TASK-006 —— **已核实该义务确实交付**，见 `sections.go:311-328`，订正按要求留在原地未删错话）。

---

## 三、Code Review：三轮，中途撤回 PASS 改判 REJECT

| 轮次 | 裁决 | 要点 |
|---|---|---|
| v1 | ~~PASS~~ | **已撤回，QA 自陈「那是错的」** |
| v2 | **REJECT** | 3 条 HIGH（R1/R2/R3）+ 3 条 MEDIUM，我与 lens 复验结论一致 |
| 第三轮 | **PASS** | 三条 HIGH 全部闭合，9/9 变异 KILLED 且 9/9 预期命中 |

**三条 HIGH**：

- **R1** `extractTSFFlowSection` 板块路径无作用域切分 ⇒ **静默产出错位 18.8× 的数据**
  （`ytd=252100` 配 `rmb_loan=13400`），而板块路径正是 v2 月报的 going-forward 格式。
- **R2** `checkSectorCaliber` 锚点缺席即放行，而抽取不依赖锚点 ⇒ 守卫可被绕过。
- **R3** `periodAlt` 不认 `N-M月` ⇒ 4 期可恢复的数据被贴上「不存在」的假标签**并被指示写销**。

⚠️ **QA 撤回自己的 PASS 是本 sprint 质量上最关键的单次动作。** 它同时记明了
v1 里自己的三处错误（选择偏差写成无条件结论、变异射程说宽了、措辞过宽），
以及第三轮自己**仪器坏了两次**（都被自证闸挡下，「本会变成两条假的 CRITICAL」）。

---

## 四、🔴 交付前发现并已修复：核心验收产物的数字过期

**在转 `accepted` 后写本报告时发现**：`calibrate-AFTER-report.md` 采样于 `f74dc49d`
（08-26 21:26），而三条 HIGH 的修复在此**之后**才合入（`cc8baf6` / `e600b39` / `f280d3c`）。

⇒ 该报告的「195 / 34 / 23」是**修复前**的值，真值是「**199 / 38 / 19**」。
两份交付物一度自相矛盾：**CONTRACTS.md 用的是 19（对），AFTER 报告用的是 23（错）**。

**处置**：背对背重跑（同一时刻、同一语料、同样 flag），报告已订正落盘并保留原文对照。
**自证**：PRE 重跑与报告原先嵌入的 185 行**逐字节一致** ⇒ 差异全部归因于代码改动。

### 为什么它躲过了三道检查

TASK-008 后两轮返工都是**文档改动**，验证者据「docs-only ⇒ 数字与前两轮逐字一致」放行。
**这个推断对 `go test` 成立、对本报告不成立**：报告的数字不来自被改动的文档，
而来自**另一批文件**（`extract.go` / `profiles.go` / `sections.go`）的改动。

⚠️ **dev 自查、验证者三轮、QA 三轮，三方都在看「本轮改动」，而危害来自「采样锚之后的全部改动」**
—— 两者在最后两轮里恰好不相交。

⇒ **结转纪律**：「本轮改动是 docs-only」**不蕴含**「一切自证数字仍然有效」。
判据是 `git diff --numstat <采样锚> <当前 HEAD>`，**不是「我这轮改了什么」**。

### 一条不要误读的观察

PRE/POST 的**字段分布段除首行外逐字节一致** ⇒ **R1/R2 在这份 2026-08-14 语料上不改变任何抽取值**。
⚠️ 这**不是**「修复没必要」——它们修的是 going-forward 格式，**going-forward 的意思就是这份语料还没有**，
而那天到来时没有任何东西会提醒你。⚠️ 也**不是**「已确认语料不含该格式」——
我观察到的是「输出没变」，不是「该路径没被走到」，两者不等价。

---

## 五、知识产物

| 产物 | 体量 | 用途 |
|---|---|---|
| `internal/hestia/CONTRACTS.md` `## Sprint M1c-3a` 节 | +528 行 | F 节 21 条机制发现、G 节 8 条结转项（各带承接句） |
| `06-acceptance/contracts-checklist.md` | **162 条** | TASK-008 `functional[2]` 的内容依据 |
| `06-acceptance/calibrate-AFTER-report.md` | 652 行 | 真跑验收 + 修复前后背对照（本轮订正） |
| `06-acceptance/transcription-originals.md` | 98 行 | 转录原文存档 + 4 条使用说明 |
| `06-acceptance/baseline-blindspot-audit.md` | — | 基线盲区审计 |

⚠️ CONTRACTS.md 里有**两个 `### F.`、两个 `### G.`**（M1c-1 的在 1060 行）——
**结构核对必须先锚 `## Sprint M1c-3a` 再切**，本 sprint 已因此数错三次。

---

## 六、结转 M1c-3b（8 条，本 sprint 明确不修）

1. 🔴 **`2022-05` 的活缺陷** —— 已知、可修、本 sprint 不修（全语料只此 1 期）
2. 🔴 **R9：`stockContinuityRates` 的重复业务键** —— 「不入库所以安全」覆盖不到它
3. **按节部分抽取** —— 让 2023-07/08/10/11 那 4 篇能产出 `loan_flow_ytd`
4. **R10：`sections.go:116` 的「共 74 篇」不可复现**，旧判据逐字复刻实测 55
5. **R11：`checkPeriodTypeSupported` 的拒绝分支是死代码**，但防线仍有效
6. **两处措辞遗留**
7. **`git log --grep '<任务号>'` 在复用编号的仓库里不是范围判据**
8. **合成 fixture 的前缀选择依赖同一类假设**（test-m1c3a-v1 提出）

**每条都带「必须放进某个任务的 `done_criteria`」承接句** —— 因为
`done_criteria` 是本 sprint 反复确认的**唯一强载体**：写在 `description` / `decisions` /
inbox 里的义务，**对 dev 和 verifier 都不可见**（validator 的 `orphan-obligation` 即为此设）。

---

## 七、机制层面的教训（已进 CONTRACTS F 节，此处只列最重的三条）

1. **「merge 必须在 `dev_done` 之前」是机制决定的，不是流程偏好**：
   `task-completed.sh` 的 `git log --grep` **全文件 `--all` 出现 0 次** ⇒ 只走 HEAD 祖先链
   ⇒ **未合并分支上的 commit 对门禁结构性不可见**，两个集合双双为空所以「报绿」。
   ⚠️ 而 idle hook 的解锁文案在此状态恒为「推进 `dev_done`」，**方向相反**；
   本 sprint **两个 dev 各自独立撞到**，正确行为完全靠自觉。
2. **锚必须钉不会漂的东西**：本 sprint 出现四个载体 —— commit sha、覆盖率基线、
   清单条数、**文档行号**。过期的锚**不会报错，只会静默指到一段无关的话**。
3. **自证数字必须在最后一次改动之后统一重采**（第四节即本条的实例）。

---

### 待同步 hooks 清单(人类执行)

| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 sprint **未改动**任何 `project-template/hooks/`、`project-template/scripts/`、`templates/CLAUDE.md.template` 或 `validator/` | — |

**核查依据**（全量搜索，独立于我对本 sprint 的记忆）：

```bash
git diff --name-only 7bccab448ecc477dce0b83e16eff98fd3fb430d3 master \
  | grep -E '^(project-template/|templates/|\.claude/|CLAUDE\.md$|validator/)'
# → 0 处
```

改动范围仅 `internal/hestia/**`（16 个源文件 + 14 个 testdata）与
`.arcforge/docs/02-plan/` 一个文件。**运行时资产零漂移，无需人类同步动作。**

⚠️ 收官 `grep` 核查范围已包含运行时根 `CLAUDE.md`（上式的 `CLAUDE\.md$` 分支），
不只扫 `global/` 与 `templates/CLAUDE.md.template`。
