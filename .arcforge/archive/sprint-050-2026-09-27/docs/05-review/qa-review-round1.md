# QA Review Round 1 — 银行股关键指标月度监控（atlas bank report）

- 审查者：qa-bk-a（2026-09-27）
- 范围：`git diff b2f85462aa32f7766a07dba5938ea9c8d4440830..8fdd59ba008f81562b1cf789674cc376ebe99fe7`（21 个新文件，+2755/-0）
- 取证树：工作树 HEAD = `8fdd59ba008f81562b1cf789674cc376ebe99fe7`（`git rev-parse HEAD` 现读），被审文件无未提交改动；行号均按该 sha
- 需求：spec `docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md`；R14–R17 人类裁决偏差、计划的有意偏差（integration 标签、排名标题、空预警渲染）不计缺陷；cmd/atlas `-shuffle` 下 TestBackfillLoadRequiresDBFlag 为范围外既有问题，未计入
- 严重度映射：`code_review.severity_threshold = warning`；本报告 HIGH/CRITICAL → CRITICAL，MEDIUM → WARNING（阻断），LOW → SUGGESTION（不阻断）

## 审查方式与降级说明

| 轮次 | 执行者 | 产物 |
| --- | --- | --- |
| 第一轮 常规 | qa-bk-a 本体通读全部源码 + `go vet` / `go test -cover` / `go test -race` | 本报告 §2 |
| 第二轮 Skeptic | 只读子代理 | scratchpad `qa-bk-a-skeptic.md`（144 行，4 MEDIUM / 7 LOW） |
| 第二轮 Architect | 只读子代理 | scratchpad `qa-bk-a-architect.md`（175 行，1 MEDIUM / 9 LOW / 1 INFO） |
| 第二轮 Minimalist | 只读子代理 | scratchpad `qa-bk-a-minimalist.md`（116 行，9 LOW） |
| 跨模型 | `codex exec -s read-only`（60 秒探针 rc=0、返回 PROBE_OK，**本次可用**，PENDING-MECHANISMS #5 没有复现） | scratchpad `qa-bk-a-codex.md`（1 LOW） |

- 3 个 lens 子代理的派发工具调用被用户中断，没有拿到返回值；但三份结论文件都已完整写出（`wc -l` 144/175/116），我是从文件取的结论。子代理用 `go test -overlay` 或 `git archive` 副本做实验，事后 `git status --porcelain` 只剩开审前就有的 `.arcforge/` 条目，**仓库未被改动**。
- lens 的 MEDIUM 结论都由我本体独立复核过（§3），并按复核结果重新定级，**没有照抄**。

## 1. 基线实测（qa-bk-a 本体，sha 8fdd59ba）

```
$ go vet ./internal/bank/ ./cmd/atlas/                 → 无输出
$ go test -count=1 -cover ./internal/bank/             → ok  coverage: 99.7% of statements
$ go test -count=1 -cover -run Bank ./cmd/atlas/       → ok
$ go test -count=1 -race ./internal/bank/              → ok
```
Architect lens 另测：bank.go 的 6 个函数 `-func` 覆盖率均为 100%；`-shuffle` 取 5 个种子跑 `-run Bank` 都通过。**覆盖率 100% 不代表退出码的每一行映射都有断言守着**，见 FIX-1。

## 2. 第一轮常规审查：总体评价

- **代码质量**：包职责切分清楚（source → Analyze → Summarize → Render/Split → deliver）；纯函数和 I/O 分开；注释都写明了约束的「为什么」。minimalist 实测注释里的数值声称全部属实，例如 `0.67-0.57 = 0.10000000000000009`、`236.04-256.04 = -20.000000000000028`，以及「超过 12 个元素才能区分 SliceStable」。staticcheck 零输出，没有死代码。
- **安全**：Telegram 错误由 notifier 的 `wrapRedacted` 脱敏；aktools 的 URL 不含密钥；symbol 经 `url.Values` 编码。**未发现注入或泄密**。
- **正确性**：R14–R17 的实现逐条对得上：
  - R14：bank.go:80-82
  - R15：analyze.go:59-77、render.go:101-103
  - R16：summary.go:53-66
  - R17：render.go:36-38

  阈值判定用严格不等号，`round4` 由边界测试守着；minimalist 做了去掉 round4 的变异，`恰等于恶化阈值不预警` 这条转红，说明 round4 确实被守住。
- **测试质量**：有 golden 逐字节比对，也保留了独立于 `-update` 的子串规格断言；e2e 用 httptest 起假 aktools，验证了 A+H 只拉一次。Skeptic 的 7 个变异里 5 个被杀，存活的 2 个是等价变异或无需求约束的并列规则。**但 launchd 生产路径上的退出码 2 没有守卫**（FIX-1）。

## 3. 发现清单（复核后定级）

### WARNING（阻断）

**[MEDIUM] FIX-1 — cmd/atlas/bank.go:83-85：生产主路径「非 dry-run、有 sender、部分失败 ⇒ 退出码 2」没有端到端守卫**
- 失败场景：launchd 每月跑的是非 dry-run 路径。如果把 `if code != 0 {` 回归成 `if code != 0 && bankDryRun {`，生产上部分银行拉取失败时进程会以 0 退出，launchd 记为成功，而全部测试仍然是绿的。
- 证据（qa-bk-a 本体实测，overlay 替换 bank.go，仓库未改）：
  ```
  $ diff cmd/atlas/bank.go qa-bk-a-self/bank_mut.go
  83c83
  < 	if code != 0 {
  ---
  > 	if code != 0 && bankDryRun {
  $ GOTOOLCHAIN=local go test -count=1 -overlay ov_mut.json -run 'Bank' ./cmd/atlas/
  ok  	github.com/newthinker/atlas/cmd/atlas	1.368s      ← 变异存活
  ```
  原因：`bankExit` 的调用只在 `TestRunBankReportDryRun/部分失败`（bank_test.go:250-253）里被断言。非 dry-run 的部分失败只有「无 sender」这一种形态（bank_test.go:284-292，期望 error、不调 bankExit）。TASK-006 的 DoD 只要求 dry-run 形态，所以 **DoD 本身已满足，这是 DoD 之外的守卫缺口**，落在 spec §6 退出码表的一行上。
- 共识：Architect 发现；本体变异复现；其余 reviewer 没有覆盖这一点，也没有人提出异议。

### SUGGESTION（不阻断，按价值排序）

| # | 级别 | 文件:行 | 场景 → 问题 | 来源 / 复核 |
| --- | --- | --- | --- | --- |
| S1 | LOW | cmd/atlas/bank.go:95-102 | 配置了招商银行 + 招商银行H，两家都拉取失败 ⇒ 错误摘要里只有「· 招商银行：err」，H 股条目整条消失。这违反 render.go:88-89 自己写下的不变量（「主体不论处于哪一组都要写」）和 spec §4 A+H 附注的要求。标题串也与 render.go:22 重复，且不受 golden 守护 | Skeptic F10、Architect F2、Minimalist F3、codex，**四方一致**；本体直读 bank.go:95-102 确认只拼 `r.Name`/`r.Err`。**建议并入本轮返工**（FIX-2） |
| S2 | LOW | cmd/atlas/bank.go:103-106, 128-143 | 推送失败时报告正文既不进 stdout 也不进 stderr。「全部失败 + 推送失败」时只返回推送错误，「全部 N 家拉取失败」这一根因从日志里消失；多段推送第 k 段失败后，重跑会重复推送前 k-1 段 | Skeptic F4 定 MEDIUM、Architect F4 定 LOW。**我定 LOW**：无状态设计下，`--dry-run` 重跑就能完全恢复信息 |
| S3 | LOW | internal/bank/source.go:72-90；analyze.go:67-69 | 同一 REPORT_DATE 出现两行 ⇒ 环比和自己比，得 0，恶化预警被吞掉（本体复现 `DUP QoQ=0`，应为 +0.40）；`sort.Slice` 不稳定 | Skeptic 定 MEDIUM。**我降为 LOW**：对本地 aktools 做只读 GET，600036/601658/600919/002142/601398/000001 六家的 rows 与 uniq 相等（102/47/57/83/85/122），没有观察到重复。建议 parse 后按 Period 去重或报错，并改用 SliceStable |
| S4 | LOW | internal/bank/summary.go:69-74；render.go:47-50 | 当期主体的 rank_by 指标只有回退值时，这些主体排在末尾、彼此保持配置顺序（本体复现：「按CET1由优到劣」下 8.00% 排在 14.00% 前面），但行内没有「未参与排名」标注 | Skeptic 定 MEDIUM。**我降为 LOW**：「回退值排最后、保持配置顺序」是 TASK-004 DoD 的明文决定（与 D2 裁决联动），不是缺陷，只剩展示歧义；shipped 配置 `rank_by: npl` |
| S5 | LOW | cmd/atlas/bank.go:94-114 | 所有主体都 `Err=nil` 但没有数据（三个字段全部改名）⇒ 报告里全是「无可用数据 / 字段缺失」，退出码 0 | Skeptic F2 定 MEDIUM。**我定 LOW**：spec §6 明文规定「字段缺失 → 0」，§9 的缓解手段是「报告标注 + live 冒烟」，报告里确实会显示出来；如需把这种组合改成非零退出码，要由 Leader 或人类裁决 |
| S6 | LOW | internal/bank/config.go:139 | `.inf` 阈值能通过校验（+Inf > 0），效果是某项预警永远不触发或永远触发 | Skeptic F6；代码直读确认 |
| S7 | LOW | internal/bank/config.go:85 | 用的是 `Unmarshal` 而不是 `UnmarshalExact`：`rankby:` 这类键名拼错时会静默回落到 npl 或默认 URL | Architect F9；代码直读确认 |
| S8 | LOW | cmd/atlas/bank.go:148-157 | 没传 `--config` 时报的原因是「主配置未启用 notifiers.telegram」，实际原因是没有指定主配置（`loadConfigOrDefaults` 在 cfgFile=="" 时返回 Defaults，export_ohlcv.go:283-293 直读确认）；launchd 带了 `--config`，只影响人工执行 | Architect F3 |
| S9 | LOW | Makefile:126-127 | `test-integration` 没有包含 `./internal/bank/...`，spec §2 所说的 live 校验点没有入口能跑到它 | Architect F10；直读确认 |
| S10 | LOW | render.go:12, 157-181 | Split 按 rune 计数，Telegram 的限额按 UTF-16 计；超长单行会原样放行。真实数据下触发不到 | Skeptic F5（requirements-analysis 第 7 条已论证余量） |
| S11 | LOW | analyze.go:67-69 | 「环比」对上一个非 NaN 期的间隔没有上限（5 年前的值也算环比并能触发恶化预警），行内也不显示对比的是哪一期 | Skeptic F7；按 R15 原文「对该指标自己的上一个非 NaN 期」属于裁决范围，**不计缺陷**，只提示 Leader 确认是否有意 |
| S12 | LOW | 其余 | source.go:46-48 错误文本约 237 字符，含编码后的 URL，冗余；Fetch 不接 context，也没有总时限；导出切片 `Indicators` 可被外部修改；六处按下标对齐的指标定义；「ℹ️ 未更新/失败」标题下混入了领先主体；config_test.go:9 的 checkpoint 注释声称的内容多于测试实际覆盖；summary_test.go:92 的 Mean 是自比较；bank.go:17-18 的断言与 161 行重复 | Architect F5-F8、Skeptic F8/F9/F11、Minimalist F1-F9 |
| I1 | INFO | 部署 | runtime/atlas/configs/ 下还没有 bank-monitor.yaml，runtime 里的二进制也要按 8fdd59ba 重建；属于上线清单项，不是代码缺陷 | Architect I1 |

## 4. fix_items（交 Leader 执行 `verified → review_fix`，均属 TASK-006，reason_class=`task_defect`）

1. **FIX-1（必修，WARNING）**：在 cmd/atlas/bank_test.go 增加 runBankReport 级用例：非 dry-run、sender 注入成功、`601658.SH` 拉取失败。断言以下四点：
   - `require.NoError`
   - `factoryCalls == 1`
   - sender 收到 1 条，且含「邮储银行 拉取失败」
   - `e.exits == []int{2}`

   **验收变异**：把 bank.go:83 改成 `if code != 0 && bankDryRun {` 后，该用例必须转红（改动前这条变异存活，见 §3 的实测输出）。
2. **FIX-2（建议同轮，SUGGESTION S1，四方一致）**：全部失败摘要里为每个主体补上别名附注「（招商银行H 同 600036.SH）」。**验收反例**：用 bankE2ECfg（含 H 股）让两家都拉取失败，断言推送的摘要含「（招商银行H 同 600036.SH）」。改动前为红（本体直读 bank.go:95-102，确认只输出 Name/Err）。注意：architect 实测过「往 `failed` 切片里加一行」会破坏 `len(failed)==len(results)` 的计数判等，修复时计数须改用独立变量。Leader 可以决定不修，但需要在 final-report 里注明。

## 5. Verdict

**REJECT**。有 1 条 WARNING 级发现（FIX-1），由 Architect 发现、本体变异独立复现，没有 reviewer 提出异议。它不是逻辑错误，而是 launchd 生产路径上的一行退出码映射可以静默回归而测试全绿；修复只需加一个测试用例。除此之外没有 CRITICAL，业务逻辑、安全、R14–R17 的实现均未发现阻断问题。FIX-1 修复并通过验收变异后，预期可转 PASS。
