# Sprint 037 Code Review — 综合裁决

<!-- 落盘者: qa-agent-14 | 落盘时间: 2026-08-13 -->
<!-- 审查执行者: qa-agent-13（三个 Claude lens：Skeptic / Architect / Minimalist）；本实例只做合并落盘，未重做审查 -->
<!-- 仓库: /Users/zuowei/workspace/go/src/github.com/newthinker/atlas | master HEAD: bb825defffd332cf6886ef32def33d7f05c455a7 -->

## 裁决：REJECT

有 1 条 CRITICAL、4 条 WARNING。按 `arcforge.config.json` 的 `code_review.severity_threshold = "warning"`，达到返工阈值。

三个 lens 在 high-severity 发现上**没有分歧**（不是 CONTESTED）：CRITICAL-1 由 Skeptic 单独命中并做了变异实证，另外三条 lens 交叉命中（见「交叉命中」一节）。

| 级别 | # | 标题 | 命中的 lens |
|---|---|---|---|
| CRITICAL | 1 | 排班守卫不限定作用域，键名打错/跨 dict 错配均全绿 | Skeptic |
| WARNING | 1 | `StopReason` 在有候选时被整个丢掉 | Skeptic + Minimalist |
| WARNING | 2 | `HasPeriod` 已无生产消费者，文档仍说「Discover 用它判停」 | Minimalist + Architect |
| WARNING | 3 | `--force` 漏掉第三层幂等，且 flag help 承诺了做不到的事 | Skeptic + Architect |
| WARNING | 4 | `ingest` 没有 cwd 守卫 | Architect |

---

## 关于本报告的行号锚（落盘前逐条复跑过）

Leader 交回的清单里有两处行号锚与当前 HEAD 不符，本报告用**复跑确认过的**坐标，并在此显式记下差异，免得后人拿两份坐标对不上：

| 原清单写的 | 实际（HEAD `bb825de` 复跑） |
|---|---|
| `cmd/atlas/hestia.go:60`（`--force` flag help） | `cmd/atlas/hestia.go:72-73` |
| `internal/hestia/status.go:24-35`（cwd 守卫） | 文档注释 `:23-31`，解析动作在 `RenderStatus` 的 `:32-36` |

其余锚（`hestia_test.go:396-414` / `:417`、`ingest.go:61,66-71`、`store.go:216`、`discover.go:331-336`、`store.go:568+`）逐条 `sed -n` 核过，与描述一致。

---

## CRITICAL-1 — 排班守卫不限定作用域

**位置**：`cmd/atlas/hestia_test.go:396-414`（`TestHestiaPlistSchedulesThreeTimes`），判据实现在 `:417` 的 `plistIntsUnderKey`。

**问题**：守卫在**全文档**范围数 `<key>Hour</key><integer>N</integer>` 与 `<key>Minute</key>`，再按下标把两个独立列表配对：

```go
hours, minutes := plistIntsUnderKey(t, raw, "Hour"), plistIntsUnderKey(t, raw, "Minute")
require.Len(t, hours, 3, "spec 定的是每日三个时点；解析不出三个说明排班键被改坏或删掉了")
...
got[i] = [2]int{hours[i], minutes[i]}
```

它**从不断言**这些整数位于 `<key>StartCalendarInterval</key>` 之下，也不断言 Hour/Minute 出自同一个 `<dict>`。

**可复现的观察**（本实例落盘前复跑，非转录）：

```
$ grep -rn 'StartCalendarInterval' --include='*.go' .
cmd/atlas/hestia_test.go:386:// ⚠️ **本条是我自己踩出来的**：改时刻那一步我误把整个 `StartCalendarInterval` 数组
exit=0
```

全仓 Go 代码里该键名**只出现在一行注释里，没有任何一处断言**。

**变异实测**（Skeptic 在隔离副本上做，主工作区零改动）：

| # | 变异 | 断言结果 | `plutil -lint` |
|---|---|---|---|
| M1 | 键名打错一字母 `StartCalendarInterval` → `StartCalendarIntervall` | `hours=[15 17 21] minutes=[30 30 30]` → **全绿 PASS** | OK |
| M2 | Hour/Minute 跨 dict 错配（实际排班 3 次/天 → 约 86 次/天） | **全绿 PASS** | OK |

**后果与该测试自述要防的完全相同**：launchd 忽略未知键 ⇒ job 永不被日历唤起 ⇒「装得上、`launchctl list` 看得见、日志目录空着，而一切看起来都正常」。`plutil -lint` 挡不住——两种变异都是合法 plist。

**这是本 Sprint 第四个假通过，且就在为修第三个而写的那条测试里。**

**修法**：像 `plistEnvKeys` 那样限定作用域——先认出 `StartCalendarInterval` 后面那个 `<array>`，逐 `<dict>` 收成对的 `{Hour, Minute}` 返回 `[][2]int`，缺任一字段即 `require.Fail`。M1 在「找不到该键 ⇒ 0 个 interval」转红，M2 在「dict 缺字段」转红。**同时补 `internal/hestia/CONTRACTS.md:731` 那张表**——它只列了「删掉」，读者会据此以为改名也被守着。

---

## WARNING-1 — `StopReason` 在有候选时被整个丢掉

**位置**：`internal/hestia/ingest.go:61`（赋值）、`:66-71`（唯一消费点）。

```go
cands, stop, err := Discover(ctx, d.Fetch, known, d.Cfg.Discover)   // :61
...
if len(cands) == 0 {                                                // :66
    fmt.Fprintf(d.Out, "no new reports (stopped: %s)\n", stop)      // :70
    return nil
}
```

`stop` 全函数只有这两处引用，而打印那处关在 `if len(cands) == 0` 里。

**可复现的观察**：

```
$ grep -rn 'stopped:' --include='*_test.go' .
exit=1                    # 零命中：没有任何测试断言过这行输出

$ grep -rn 'StopExhausted' --include='*.go' .
internal/hestia/discover.go:253:	// StopExhausted：把站点的 totalPages 翻完了，没有更多页可翻。
internal/hestia/discover.go:254:	StopExhausted StopReason = "exhausted"
internal/hestia/discover.go:376:		return out, StopExhausted, nil
exit=0                    # 全在 discover.go：注释 / 定义 / 返回，无任何测试断言
```

**为什么要紧**：`ingest.go:51-52` 的注释自述「真正要担心的是反过来——`stopped: max_pages` 意味着窗口外可能还有没发现的期次」，而 **`max_pages` 且有候选恰是它的主要形态**（空库首跑必然如此）。首跑之后第 3 页以外的历史**永久不可达**，而这条信息在唯一会发生它的那一轮被静默吞掉，退出码 0。

**修法**（Architect 给的方向）：`stop == StopMaxPages` 时改写 **stderr**；**不要改退出码**——首跑必然 max_pages，改退出码会变成假红。并补一条断言渲染行 + 原因的测试。

---

## WARNING-2 — `HasPeriod` 已无生产消费者，文档仍说「Discover 用它判停」

**位置**：`internal/hestia/store.go:216`。

```go
// HasPeriod 回答某期是否已在权威表里。Discover 用它决定翻页何时停。
```

TASK-011 之后 `Discover` 用的是 `HasArticleInObservations`，`PeriodChecker` 已删。`store_test.go:359/493/509` 仍说「Discover 经 PeriodChecker 窄接口消费它」。

**这与 Leader 预登记的 QA-PRE-1 / 1b / 1c 是同一族**，详见 `.arcforge/docs/02-plan/qa-preregistered-items.md`（171 行）：同一句过期结论「Verdict 恒为 `New` / `Duplicate` 不可达」在 `ingest.go:161`、`ingest_test.go:287-291`、`store.go:216` **共三份副本**，而 `ingest_test.go:705` 的绿测试直接反证它。`ingest_test.go:288` 那个坐标 `discover.go:303-318` 现在指向的正是它自己的反驳。

**修法**：见 QA-PRE-1d——**删掉行号锚**，改用符号名 + 字串锚（`file:line` 是注释版的 `HEAD`：写下时正确，读它时早已漂走）。

---

## WARNING-3 — `--force` 漏掉第三层幂等，且 flag help 承诺了做不到的事

**位置**：`internal/hestia/ingest.go:58`（第 1 层 `neverSeen`）、`:113`（第 2 层 `HasArticle`）、`internal/hestia/store.go:568+`（第 3 层 `refreshArticleID`）、`cmd/atlas/hestia.go:72-73`（flag help）、`internal/hestia/ingest.go:22`（`Force` 字段注释）。

`--force` 穿不透 `Save` 的 `Classify → Duplicate → refreshArticleID`：同篇 `published_at` 不变 ⇒ 恒判 Duplicate ⇒ **只刷 `article_id`，新抽 Values 一个不写**，返回 nil、退出码 0。

**复核发现（重要，决定了修法）**：该取舍在 `refreshArticleID` 自己的注释里**已登记且写得很清楚**：

```go
// # 只更新 article_id 意味着重跑抽取的新值会被丢弃（G3，已登记的取舍）
//
// 上线 rule@v2 后回填重跑历史是必然操作，届时每期 published_at 都没变 → 全判
// Duplicate → 走到这里 → **v2 新抽出来的字段一个都没写进去，extractor 列还写着
// 旧抽取器**，而 Save 返回 nil，运维看到「N 期处理完毕、零错误」。
```

并有 `TestSaveDuplicateDiscardsRicherValues` 钉住。**问题不在行为，在于 `--force` 是本 Sprint 新增的、第一个会批量触发它的调用方**，而两处文案都把它描述成「重来一遍」：

- `cmd/atlas/hestia.go:73`：`"bypass the article_id idempotency key; use after changing thresholds"`
- `internal/hestia/ingest.go:22`：`Force bool // 绕过一级幂等键，用于改了阈值后重跑`

「改了阈值后重跑」正是 G3 会静默丢弃结果的那个场景。

**修法**：**行为不改**（Duplicate 不覆盖是登记在案的取舍），只改文案——flag help 与 `Force` 字段注释点名「Duplicate 期次的重抽取值会被丢弃」；并更正 `internal/hestia/CONTRACTS.md:591`「M1c 标定 MagnitudeRanges 后重跑已入库期次有出路」这句（Architect 判它为假）。

---

## WARNING-4 — `ingest` 没有 cwd 守卫

**位置**：`internal/hestia/status.go:23-36`（`RenderStatus`，有守卫）对照 `hestia ingest` 路径（无）。

`status` 侧把 dbPath 解析成绝对路径再打印，注释写明了理由：

```go
// dbPath 会被解析成绝对路径再打印：它是相对路径（约束 C8），在错误的 cwd 下
// 会静默指向另一个不存在的库，然后如实报告「0 期」—— 看起来像数据没进来，
// 实际是查错了地方。
```

**而 `ingest` 完全没有**。`NewStore` 先 `MkdirAll` 再建库 ⇒ 错误 cwd 下 `hestia ingest` 会**新建空库 → 翻满 MaxPages → 全量入库 → 逐期打印正常 → 退出码 0**，真库停在旧数据无任何提示。同一个失效模式，`status` 挡了、`ingest` 没挡，而 `ingest` 是会写数据的那一个。

**修法**：`openHestia`（`cmd/atlas/hestia.go:78+`）返回解析后的绝对路径，两条 `RunE` 各打一行。

---

## 交叉命中（比单点更有分量）

三处发现由**两个独立 lens 各自捞到**，彼此没有共享 context：

| 发现 | lens A | lens B | 两者路径 |
|---|---|---|---|
| WARNING-1 `StopReason` | Skeptic | Minimalist | Skeptic 逐行读 `ingest.go`；Minimalist 找「无消费者的值」 |
| WARNING-2 `HasPeriod` 过期注释 | Minimalist | Architect | Minimalist 找死代码；Architect 查接口一致性 |
| WARNING-3 `--force` 三层幂等 | Skeptic | Architect | Skeptic 找隐含假设；Architect 查分层边界 |

CRITICAL-1 是**单 lens 命中**（Skeptic），但它带变异实证（M1/M2 两个方向各一次实测），证据强度不依赖交叉。

---

## SUGGESTION（不返工，进 final-report 遗留清单）

- `internal/hestia/discover.go:331` `if limit > totalPages` 应为 `>=`（差一）；`cfg.MaxPages == totalPages` 时全站翻完却报 `StopMaxPages`，**假告警方向恰是 plist 注释叫运维警惕的那个**
- `internal/hestia/profiles_test.go:496` `TestQuarterlyPeriodsAreCumulative` 用**硬编码字面量列表**（第三份手写副本），加第六种期次时两处都不会响。Architect 给的零成本闭合：`regexp.MustCompile(`\A(?:`+periodAlt+`)\z`)` 须匹配 `cumulativePeriods` 的每个键
- `internal/hestia/types.go:31` `PeriodType string // 必填，monthly | h1 | annual` 是该字符串第三份副本（本 Sprint 修了 `Meta.validate` 与 `thresholds.go:126`，漏了这处），且它是 `go doc` 打出来的那一行
- `cmd/atlas/hestia.go:9` 注释「一个月一期，10 行覆盖近一年」加 q1/h1/q1_q3 后已不准
- `internal/hestia/store_test.go:2128-2129` DoD→test 映射引用的测试名对不上实际（`...LimitZeroVsNegative` vs 实际 `...GuardsNonPositiveN`）
- `RecentPending` 按 `period DESC` 排而文档称「最近 n 次尝试」
- `configs/hestia.yaml` 头部「改这个文件不需要重新编译」会诱导运维改 runtime 副本，而 `rsync -a --delete` 未排除 `configs/` ⇒ 下次 deploy 静默还原（**既有约定，非本 Sprint 回归**）

---

## 已知遗留，不算到本 Sprint 头上（逐条核过 base `076998be`）

- `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` 未过 gofmt——base 上就是
- `deploy/launchd/com.newthinker.atlas.aktools.plist` 是**非法 XML**（注释含 `--`），而 `plutil -lint` 报 OK
- validator 的 `scope-writes-outside-packages` 告警对本仓库标准形状是已知假阳

---

## harness 缺陷（进 final-report 待同步清单，人类会话外执行）

`.claude/hooks/teammate-idle.sh:76-86` 的 `qa-*` 分支**不按归属过滤**：

```bash
MINE=$(query_tasks 'select(.status == "verified") | .id')
```

`dev-*` 用 `.assigned_to == $me`、`test-*` 用 `.verifier == $me`，**唯独 `qa-*` 取全体 verified 任务** ⇒ 任何 `qa-*` 实例都被别人的任务钉住，唯一逃生口是 `05-review/` 下有一份 mtime 晚于全部 verified 任务的 `*.md`。**而只读 lens 被禁止写文件 ⇒ 结构上无法停机。**

**这是复发**：hook 自身注释记载 sprint-033 的 `qa-agent-10` 被连续唤醒**约 1500 次**，成因逐字相同。**那次补救只改了文案（加了「交回父实例落盘」那句），没动控制流**——而那个动作**不改变 `MINE`**，所以它只让空转变得可解释，不能终止它。本 Sprint Leader 已 `TaskStop` 两个空转的 lens。

**建议**：把 `qa-*` 分支改成按归属过滤，或给 lens 子代理显式标记走 `*)` 分支。

⚠️ 运行时 `.claude/` 只读，本项**不在本 Sprint 修**，走 `project-template/` → TDD → 人类确认 → 会话外同步。

---

## 跨模型对抗轮：降级申明

`arcforge.config.json` 的 `code_review.adversarial.cross_model` 是 `"auto"`。本实例实际探测（不是转述）：

```
$ command -v codex
/Users/zuowei/.nvm/versions/node/v22.22.0/bin/codex     # exit=0，可用
$ command -v gemini
                                                         # exit=1，不可用
```

**如实申明**：
- `codex` **可用但未跑**。qa-agent-13 已用三个 Claude 视角（Skeptic / Architect / Minimalist）完成 `perspective_review`；它在落盘前结束，本实例的任务是合并落盘、明确被要求不重做审查。⇒ **本 Sprint 的跨模型轮是「有能力跑而没跑」，不是「跑过了」。**
- `gemini` **不可用**，该路径**降级为纯 Claude 跨视角**。

⇒ 本 Sprint 的对抗轮**只有视角多样性，没有模型多样性**。三个 lens 共享同一模型的系统性盲区，本报告的覆盖性结论应按这个前提打折。

---

## 一条待证伪判据的如实回答

结转发现 H48（`.arcforge/docs/02-plan/findings-carryover.md`）里有一条**自带证伪条件**的判据：

> 凡有两道守卫，把它们跑在同一批输入上比对结论；差集是攻击样本的候选集。
> **状态：待证伪，n=1。**

**如实写明**：三个 lens 的发现里**没有一条是用这个手法捞到的**。

| 发现 | 实际用的手法 |
|---|---|
| CRITICAL-1 | 逐行读 + 隔离副本变异实测（M1/M2） |
| WARNING-1 | 逐行读 `ingest.go` + `grep` 消费点 |
| WARNING-2 | 逐文件读 + coverprofile |
| WARNING-3 | 逐层追调用链 |
| WARNING-4 | 两条命令路径对照读 |

⇒ **QA 两轮没有提供第二个样本，该判据仍是「一个案例的归纳」，不得写成「已验证的方法」。** H48 的状态应保持 `待证伪，n=1` 不变。

---

## `review_fix` 清单（给 Leader 执行）

| 任务 | `fix_items` | `reason_class` |
|---|---|---|
| **TASK-009** | ① CRITICAL-1 排班守卫限定作用域（逐 `<dict>` 收 `{Hour, Minute}`，缺字段 `require.Fail`）+ 补 `CONTRACTS.md:731` 表；② WARNING-3 的**文案**（`cmd/atlas/hestia.go:73` flag help、`ingest.go:22` `Force` 注释、`CONTRACTS.md:591`），行为不改；③ WARNING-4 `openHestia` 返回绝对路径，两条 `RunE` 各打一行 | `guard_ineffective` |
| **TASK-011** | WARNING-1 `StopReason` 有候选时也要打（走 **stderr**，**不改退出码**）+ 补断言渲染行与原因的测试 | `guard_gap` |
| **TASK-007 或 TASK-011** | WARNING-2 三份过期结论副本（见 `qa-preregistered-items.md` 的 QA-PRE-1/1b/1c/1d，**删行号锚**，改符号名 + 字串锚） | `stale_premise` |

**建议全部派回 `dev-agent-52`**：三个文件它都熟——`ingest.go`/`ingest_test.go` 是它的 TASK-011/007，`hestia_test.go`/`CONTRACTS.md` 是它的 TASK-009。

⚠️ 上表 `reason_class` 三个取值（`guard_ineffective` / `guard_gap` / `stale_premise`）是 Leader 交回清单里给定的。写通道对 `review_fix` 的 `reason_class` 有取值校验（CLAUDE.md 判定表列的是 `task_defect` / `dod_defect` / `env_infra` / `no_progress`）——**若写通道拒绝，按判定表这三条均属 `task_defect`**（测试失效、实现与 done_criteria 不符）。

---

## 落盘元信息

- 审查执行：qa-agent-13（三 lens），本文件由 qa-agent-14 合并落盘，**未重做审查、未 spawn 子代理**
- 本实例落盘前**实际复跑**的判据：三条 `grep`（`StartCalendarInterval` / `stopped:` / `StopExhausted`）、五段 `sed -n` 行号核对、两条 `command -v`。所有引用的命令输出均为本次实跑结果，非转录
- 未复跑的：Skeptic 的 M1/M2 变异实验（在其隔离副本上做，副本已随其 session 结束；本报告转述其结论并明确标注来源）
- HEAD: `bb825defffd332cf6886ef32def33d7f05c455a7`
