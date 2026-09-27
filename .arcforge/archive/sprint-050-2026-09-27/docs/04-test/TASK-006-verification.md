# TASK-006 验证报告（verifier: test-bk-a）

**最终结论：VERIFIED（第 3 轮，QA r1 返工 @ `7234cd3f2e883a7f8d68d99fe749064ce6ff74dc`；依据是 fix_items 中的 FIX-1、FIX-2，以及原 7 条 DoD 的回归）**
（历次结论：第 1 轮 717f14d REJECTED；第 2 轮 459d9b1 VERIFIED；随后 QA 第 1 轮 REJECT，转入 review_fix；本轮是第 3 轮）

## 第 3 轮复验（QA r1 返工：FIX-1 / FIX-2）

- 被验对象：`7234cd3f2e883a7f8d68d99fe749064ce6ff74dc`，与 verify_baseline.head 以及判定时的主仓库 HEAD 相同。
- discovery sha256：`99acb67b30bdc0dafa62d06567e847d3fa35d09326111bb62365a91f0ffd9480`，与 baseline 一致。
- 验证环境：隔离 worktree `../wt-verify-TASK-006`（detached，钉在上述全 sha）。
- 改动范围：`git diff --numstat 8fdd59ba008f81562b1cf789674cc376ebe99fe7 7234cd3f…` 显示 bank.go 为 11/4、bank_test.go 为 28/9，都在 `writes` 范围内。

### 重采
- `go vet ./cmd/atlas/ ./internal/bank/` 无输出。
- `go test -count=1 -v ./cmd/atlas/` rc=0，PASS 425、FAIL 0，`=== RUN` 425。比第 2 轮的 423 多出 2 行，因为 TestRunBankReportSends 拆成了两个子测试。
- 用例数两把尺一致：源码中 `^func Test` 为 15 个，输出中本任务顶层 PASS 也是 15 个。子测试 10 个，分别是 DryRun 2、Sends 2、NoSender 2、BuildBankSender 4。
- `go test ./internal/bank/` ok。
- **覆盖率两把尺（重点 ④）**：按 profile 语句求和为 1312/1655 = **79.27%**；`-func` total 为 **79.4%**。两者都 ≥ coverage_floor 78。bank.go 的语句 64/64 全部覆盖。
- **只跑本任务用例、多 seed（重点 ⑤）**：`-run Bank -count=3`，`-shuffle` 取 seed 1 到 10 各跑一次，结果全部 rc=0。整包在 -shuffle 下存在既有的顺序依赖，已登记，不属于本任务，见第 1 轮观察项 1。

### FIX-1：launchd 生产路径的 bankExit(2)（重点 ①）
- 新增 `TestRunBankReportSends/部分失败`：非 dry-run，sender 注入成功，601658.SH 返回 500。断言 runBankReport 返回 NoError、factoryCalls==1、sender 收到 1 条且内容含「· 邮储银行 拉取失败：」、stdout 为空、`exits == []int{2}`。这些断言覆盖了 QA 给出的全部 4 个断言项。
- **验收变异 W1**：把 `if code != 0 {` 改成 `if code != 0 && bankDryRun {`。整包 rc=1，只有 `TestRunBankReportSends/部分失败` 及其父测试转红。**KILLED**。
- **消融**：把 bank_test.go 换成 8fdd59b 的旧版本、bank.go 保持 W1 变异，整包 rc=0，**变异存活**。对照组（bank.go 不变异 + 旧测试）同样 rc=0。由此可以确认，W1 被抓到完全是因为新增的这个用例，与 QA 报告中「修复前变异存活」的实测结果一致。

### FIX-2：全部失败摘要补 H 股附注，计数用独立变量（重点 ②）
- 实现方式：用 `nFailed` 单独计数；每个失败主体之后，按 `bank.writeAliases` 的格式追加 `  （别名 同 代码）`。全部失败的判定、摘要里的「全部 N 家」、错误文案、部分失败的判定，都改用 nFailed。
- 测试：
  - TestBankReportAllFailed 断言「全部 2 家」（注：这里 failed 切片有 3 行，所以这个断言能证明计数不含附注行）、错误文案为「全部 2 家银行拉取失败」，并断言附注行**紧跟在**「· 招商银行：…」之后。
  - TestRunBankReportAllFailed（bankE2ECfg，含 H 股，两家都失败）断言摘要中含「（招商银行H 同 600036.SH）」。
- 变异：

| ID | 变异 | 结果 |
|---|---|---|
| W2 | 去掉别名附注 | KILLED（AllFailed 两个用例） |
| W3 | 全失败判定改回 `len(failed) == len(results)` | KILLED：3≠2，于是走进报告路径，不再返回错误 |
| W4 | 摘要里的「全部 N 家」改用 len(failed) | KILLED（显示成「全部 3 家」） |
| W5 | 错误文案改用 len(failed) | KILLED |
| W6 | 部分失败的判定改回 `len(failed) > 0` | SURVIVED，**等价变异**：len(failed)>0 与 nFailed>0 永远同真同假 |
| W7b | 附注去掉两格缩进 | KILLED（紧跟行的等值断言） |
| W8 | 去掉 `nFailed++` | KILLED（7 个用例转红） |

（W7 的第一版把附注里的名称换成了 r.Name，导致 `a` 未使用，vet 不通过，按无效处理，改为 W7b。）
- **消融**：用旧测试配合 W2（去掉附注），整包 rc=0，说明旧测试守不住别名附注；新测试能把它杀掉。

### 回归（重点 ③）
- 同一套 V1–V27 在 7234cd3 上重跑，脚本为 mut_tbka_task006q.py。V7 的锚点随代码改动换成了 `nFailed`，其余变异不变。结果 **26 个 KILLED**。V15 仍是等价变异：失败主体的指标全是 NaN，Alerts 不会为它预警。
- 包括：V1（`!bankDryRun` 取反，现在转红 10 条）、V24、V25（D1 全部成功）、V2/V9/V19（退出码 2）、V6（逐家列出），都仍然 KILLED。
- 收尾时 worktree 干净，主仓库指纹一致。

### 第 3 轮判定矩阵
| 项 | 判定 |
|---|---|
| FIX-1 | PASS（W1 KILLED，消融证明被抓到靠的是新用例） |
| FIX-2 | PASS（W2–W5、W7b、W8 KILLED；W6 等价） |
| 原 DoD 7 条回归 | PASS（V 系列 26/27 KILLED，另 1 个等价；vet、测试全绿；覆盖率两把尺都 ≥78） |

---

## 第 2 轮复验（返工 r1）

- 被验对象：`459d9b109f1f32cd6085e3c83ee4c1364529d9d8`，与 verify_baseline.head 以及判定时的主仓库 HEAD 相同。
- discovery sha256：`a08a445e6d307932ebeb9583575e3e39692ee9675b2a3f5220564fceff0c9d80`，与 baseline 一致。
- 验证环境：隔离 worktree `../wt-verify-TASK-006`（detached，钉在上述全 sha）。

### bank.go 一字节未变，其余条目沿用第 1 轮结论
- `git rev-parse <sha>:cmd/atlas/bank.go` 在 717f14d 和 459d9b1 上都是 `a9489bff6524057ad923852e170ddc3a43b9cc12`。
- `git diff --stat 717f14d…459d9b1` 只改了 bank_test.go（+25 −10），在 `writes` 范围内。
- 改动内容：TestRunBankReportNoSender 拆成「全部成功」「部分失败」两个子测试。「部分失败」的断言与原用例逐字相同；「全部成功」新增了 `require.Error`、`⚠️ 预警 (4)`、别名行、stderr 原因、`exits` 为空这几项断言。

### 重采
- `go vet ./cmd/atlas/ ./internal/bank/` 无输出。
- `go test -count=1 -v ./cmd/atlas/` rc=0，PASS 423、FAIL 0，`=== RUN` 423。比第 1 轮的 421 多出的 2 行，就是 NoSender 新增的两个子测试。
- 本任务顶层用例 15 个，与源码中 `^func Test` 的 15 个一致。
- `go test ./internal/bank/` ok。
- 覆盖率两把尺：profile 语句求和 1308/1651 = **79.22%**，`-func` total **79.4%**。与第 1 轮完全相同，因为被测代码没变。
- 只跑本任务用例（`-run Bank`），seed 1–10 各一次、每次 `-count=3`，全部 rc=0。

### 上轮缺陷复验：V24 / V25
| 变异 | 默认顺序整包 | 只跑本任务用例，seed 1–10 各跑一次 |
|---|---|---|
| V24：全成功且无 sender 时返回 nil（退出码 0） | rc=1，只有 `TestRunBankReportNoSender/全部成功` 及其父测试转红 | 10/10 转红，每次红的都只有这两行 |
| V25：全成功且无 sender 时调用 bankExit(0) | rc=1，转红范围同上 | 10/10 |

全量回归：用同一脚本 mut_tbka_task006.py 在 459d9b1 上重跑 V1–V27，**26 个 KILLED**。V15 仍是等价变异，原因与第 1 轮相同。V3、V4 现在各红 3 条，因为两个 D1 子测试都会被触发。收尾时 worktree 干净，主仓库指纹一致。

### 第 2 轮覆盖矩阵
| # | 判定 | 依据 |
|---|---|---|
| functional[0]–[2]、boundary[0]、error_handling[1]、non_functional[0] | PASS | bank.go 未变，第 1 轮的证据仍然有效；变异回归中对应的 V 系列仍然 KILLED |
| error_handling[0]（D1） | **PASS** | 「全部成功」和「部分失败」两种形态都有断言；V3、V4、V5、V24、V25 全部 KILLED |

### 订正：第 1 轮报告的出处写错了
第 1 轮观察项 1 写「discovery 里写的『go test -shuffle=on ok』只用了一个随机 seed」，这句话挂错了对象。**TASK-006 首轮 discovery 里根本没有 shuffle 的说法**（dev 核实过 grep 结果为 0，Leader 也已订正）。这句只用一个随机 seed 的表述，实际出自 **TASK-004 和 TASK-005 的 discovery**，我对主仓库里这两份文件 grep 得到的都是 `shuffle=on ok`，测的对象是 internal/bank。在 internal/bank 上，我在 TASK-004 和 TASK-005 的验证中都另外跑了 seed 1–10，全部通过，所以那两处的结论本身不受影响，只是自证方式偏弱。观察项 1 里关于 cmd/atlas 整包存在既有顺序依赖的事实，以及归因，都不变。

---

## 第 1 轮（717f14d，REJECTED）——原文保留（出处错误见第 2 轮的订正）

- 被验对象：`717f14dd7dc8ec6d06089eaabac91f56587d4e6c`（= verify_baseline.head = 判定时的主仓库 HEAD）
- discovery sha256：`2ac6afc730236c3509a7665a557b9fcfd6cf4c0b96ab460b064ca9030383216e`（= baseline）
- 验证环境：隔离 worktree `../wt-verify-TASK-006`（detached，钉在上述全 sha）；基线对照用临时 worktree，钉在 `6d83a0d8c2f9a5cf8b05f4261f39194d4b8f7e40`，用完已拆除。go1.24.4 darwin/arm64。
- 结论：**REJECTED（task_defect）**。error_handling[0]（D1）的测试没有覆盖「全部拉取成功 + 拿不到 sender」这一主形态，两个违反 D1 的变异都存活。实现本身正确，其余 6 条通过。

## 范围与自证锚点
- `git diff --stat 6d83a0d8c2f9a5cf8b05f4261f39194d4b8f7e40 717f14dd7dc8ec6d06089eaabac91f56587d4e6c` 只改了 2 个文件：bank.go +162、bank_test.go +373，与 `writes` 一致。
- 这两个文件在 discovery 的测量点 `103bb45b71f4285cc80319ad55b52430d71aea27` 与合入点 717f14d 上 blob 完全相同。
- 下面的数字全部是我重新采集的。

## 非功能重采
| 检查 | 结果 |
|---|---|
| `go vet ./cmd/atlas/ ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile ./cmd/atlas/` | rc=0；`--- PASS` 421 行，`--- FAIL` 0 行，`--- SKIP` 0 行；`=== RUN` 也是 421 |
| `go test -count=1 ./internal/bank/` | ok |
| 本任务用例计数 | 尺 1（输出）：`^--- PASS: Test(Bank|RunBank|BuildBank)` 共 15 个。尺 2（源码）：bank_test.go 中 `^func Test` 共 15 个。两者一致。另有子测试 6 个：DryRun 2 个、BuildBankSender 4 个 |
| gate_wiring | 整包测试中 TestEntrypointsWireGateBeforeCollectors、TestServeWiresPolicyGateBeforeCollectors、TestScanWiring* 等 33 个 Gate/Wir 相关用例全部 PASS（重点 ⑦） |
| **覆盖率，两把尺（PENDING #10）** | **尺 1**：我用 Python 按 profile 块去重后对语句数求和，得 1308/1651 = **79.22%**，与 `go test -cover` 报的 79.2% 一致。**尺 2**：`go tool cover -func` 的 total 是 **79.4%**。同时刻采的基线 6d83a0d：尺 1 为 1248/1591 = 78.44%，尺 2 为 78.6%。两把尺都 ≥ coverage_floor 78。-func 比 profile 求和高 0.2 个百分点，偏向与 PENDING #10 描述的一致。bank.go 60/60 条语句全部覆盖，6 个函数都是 100% |
| `bank report --help` 冒烟 | 用 `go build` 构建后执行，输出包含 `--dry-run` 和 `--bank-config string ... (default "configs/bank-monitor.yaml")` |

## 重点核查
- **① --dry-run 走 runBankReport 端到端**：TestRunBankReportDryRun 的两个子测试都断言 factoryCalls=0，并检查 stdout 中有报告。V1（把 `!bankDryRun` 取反）KILLED，6 个用例转红；V14（dry-run 时也去构造 sender）KILLED。
- **② D1**：部分失败时的 D1 已经测到。V3（不返回错误）、V4（原因写到 stdout）、V5（先调 bankExit 再返回错误）都 KILLED。**但全部拉取成功时的 D1 没有测到**，见下文「缺陷」。
- **③ bankExit**：部分失败时 `exits == [2]`，恰好调用一次；全部成功时为空。V2、V9、V19 都 KILLED。
- **④ D14 全部失败时逐家列出**：TestBankReportAllFailed 按行断言每一家。V6（只列第一家）KILLED。
- **⑤ 非法配置时不拉取也不推送**：TestRunBankReportBadConfig 断言 hits=0、factoryCalls=0、msgs 为空。
- **⑥ 多段推送**：150 家的用例把推送结果与 `bank.Split` 的完整段序列逐段比对，并检查每段 ≤ 4000。V10（不分段）、V11（逆序推送）都 KILLED。
- **⑦** 见上表 gate_wiring 一行。
- **⑧ 包级变量复原**：setupBankE2E 用 t.Cleanup 复原 5 个变量；TestBuildBankSender 复原 cfgFile 和 policy.Default()；TestBankReportHelp 复原 rootCmd 的 args/out 以及 help flag。只跑本任务用例（`-run Bank`）时，seed 1–10 各跑一次、每次 `-count=3` 在同一进程内重复执行，全部 rc=0。另外把本任务各用例分别与受害用例放在一起跑（见观察项 1），都是 rc=0。因此本任务没有造成串扰。**但整包在 -shuffle 下存在既有串扰**，见观察项 1。

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 全成功：退出码 0、只拉一次、推送一条且含三段文本；部分失败：退出码 2 且报告照推 | TestBankReportAllOK、TestBankReportPartialFailure、TestRunBankReportSends | V9、V13 KILLED | PASS |
| functional[1] | 全部失败：返回错误，并推送一条逐家列出的摘要 | TestBankReportAllFailed、TestRunBankReportAllFailed、TestBankReportAllFailedSendError | V6、V7、V8 KILLED | PASS |
| functional[2] | dry-run：sender 构造 0 次、打印报告；bankExit 以 2 调用恰一次或不调用；`!bankDryRun` 取反须转红 | TestRunBankReportDryRun | V1、V2、V14、V19、V20 KILLED | PASS |
| boundary[0] | 150 家时多段按序推送、每段 ≤ 4000、与 Split 一致 | TestBankReportLongSplits | V10、V11 KILLED | PASS |
| **error_handling[0]** | **D1：非 dry-run 且 sender 为 nil ⇒ 完整报告打到 stdout、stderr 写原因、返回错误，不调用 bankExit(0/2)** | TestRunBankReportNoSender（**只覆盖部分失败**） | V3、V4、V5 KILLED；**V24、V25 SURVIVED** | **FAIL** |
| error_handling[1] | 推送失败时的错误文案；非法配置时不拉取、不推送 | TestBankReportSendError、TestRunBankReportBadConfig | V12、V26 KILLED | PASS |
| non_functional[0] | 命令注册、flag、编译期断言、vet、测试全绿、覆盖率 ≥78、--help | TestBankCommandRegistered、TestBankReportHelp、TestBuildBankSender；bank.go:18 有编译期断言 | V16–V18、V21–V23、V27 KILLED | PASS |

## 缺陷：D1 的主形态「全部拉取成功 + 拿不到 sender」没有测试守卫
- DoD error_handling[0] 写的是「……返回错误（退出码 1），**不调用 bankExit(0/2)**」。这里的「0」对应的正是全部拉取成功的情形。人类裁决 D1 要防的，也正是这个最常见的场景：月报全部拉取成功，但 telegram 没配，于是进程以 0 退出，launchd 把一份没推送出去的月报记成成功。
- 唯一的 D1 用例 TestRunBankReportNoSender 让 601658.SH 返回 500，也就是只测了部分失败（code=2）这一种形态。
- 证据（每个变异都作用在隔离 worktree 上，vet 通过）：
  - **V24**：把 `if noSender != nil {` 改成 `if noSender != nil && code != 0 {`，也就是全部成功且没有 sender 时返回 nil、退出码 0。整包 421 个用例**全绿，变异存活**。
  - **V25**：全部成功且没有 sender 时先调 `bankExit(0)`，再返回错误。**存活**。
  - 行为探针（只在隔离树里跑，跑完即删）：用 `setupBankE2E(bankE2ECfg, false, errors.New("主配置未启用 notifiers.telegram"))`，不设任何失败代码。原实现返回 `err=银行月报未推送：…`、`exits=[]`，符合 D1。V24 下 `err=<nil>`，即退出码 0，**恰好是 D1 要禁止的行为**。V25 下 `exits=[0]`。探针在原实现上通过，在两个变异上都失败，说明它有判别力。
- 修复方向（只改测试，bank.go 不需要动）：给 TestRunBankReportNoSender 加一个「全部成功」子场景（不传 failSymbols），断言 `err != nil`、`e.exits` 为空、stdout 有报告、stderr 写了原因。上面的探针可以直接照抄。修好后 V24、V25 应当转红。

## 变异表
脚本：scratchpad/tbka/mut_tbka_task006.py，由我独立设计。每个变异的锚点只匹配一处，先 `go vet ./cmd/atlas/` 通过再跑整包测试；每轮用 `git checkout` 还原，并比对主仓库指纹。收尾时 worktree 干净，主仓库指纹一致。

| ID | 变异 | 结果 |
|---|---|---|
| V1 | `!bankDryRun` 取反 | KILLED（6 个用例） |
| V2 | 部分失败时不调 bankExit | KILLED |
| V3 | D1 不返回错误 | KILLED |
| V4 | D1 的原因写到 stdout | KILLED |
| V5 | D1 先调 bankExit 再返回错误 | KILLED |
| V6 | 全部失败的摘要只列第一家 | KILLED |
| V7 | 全部失败时不返回错误 | KILLED |
| V8 | 全部失败时不推送摘要 | KILLED |
| V9 | 部分失败时返回 0 | KILLED |
| V10 | 不分段 | KILLED |
| V11 | 逆序推送 | KILLED |
| V12 | 段号从 0 开始 | KILLED |
| V13 | 忽略配置里的 aktools_url | KILLED |
| V14 | dry-run 时也构造 sender | KILLED |
| V15 | 失败的主体也参与预警 | SURVIVED，**等价**：失败主体经 Collect 后三项都是 NaN（TASK-004 已保证），Alerts 对 NaN 不会预警。与 dev 的 B17 结论一致 |
| V16 | 不检查 enabled | KILLED |
| V17 | 不检查 chat_id | KILLED |
| V18 | 错误信息不带主配置路径 | KILLED |
| V19 | 部分失败时以 1 调用 bankExit | KILLED |
| V20 | 没有 sender 时不打印 | KILLED |
| V21 | bank-config 默认值改名 | KILLED |
| V22 | dry-run flag 改名 | KILLED |
| V23 | bank-config 改成非持久 flag | KILLED |
| **V24** | **D1 只在部分失败时返回错误（全部成功且无 sender 时退出码 0）** | **SURVIVED** |
| **V25** | **全部成功且无 sender 时调用 bankExit(0)** | **SURVIVED** |
| V26 | 推送失败的错误不带原错误 | KILLED |
| V27 | bank 命令未注册 | KILLED |

共 27 个：24 个 KILLED，1 个等价，2 个存活（都在 error_handling[0]）。

## 观察项（不构成拒绝）
1. **cmd/atlas 整包在 -shuffle 下有既有的顺序依赖，不是本任务造成的。** 用 `go test -count=1 -shuffle=<seed> ./cmd/atlas/` 对 seed 1–10 各跑一次：717f14d 在 seed 1/2/3/5/9 失败；基线 6d83a0d 与它背对背跑，在 seed 3/9 失败。失败的都是 TestBackfillLoadRequiresDBFlag，报错为 `"hestia: reading config configs/hestia.yaml: …" does not contain "db"`。
   - 归因：在 717f14d 上用 `-skip Bank` 去掉本任务全部用例后，seed 1–10 的失败集合**完全相同**（1/2/3/5/9）。另外把本任务每个用例分别与该用例放在一起按顺序执行，全部 rc=0。两个 commit 的失败 seed 不同，是因为测试集合变了、打乱后的排列随之改变，不是本任务污染了它。
   - 疑似污染源：hestia_test.go:1139 在 `calExec` 里传了 `--db` 却没有复位 cobra 的 flag 状态，因此之后的 `bfExec` 不再触发「required」检查。该文件最后一次修改的提交是 a4a34bb，不在本任务的改动范围内。
   - discovery 里写的「go test -shuffle=on ok」只用了一个随机 seed，结果碰巧是绿的。建议 Leader 另立一项处理。它会让任何用 `-shuffle` 检验「包级变量无串扰」的门禁出现随机假红。
2. D1 分支的判断依据是 factory 返回的 error，而不是 sender 是否为 nil。如果 factory 返回 `(nil, nil)`，runBankReport 会把它当成 dry-run 处理：只打印，退出码 0。buildBankSender 的所有路径都不会返回 `(nil, nil)`，TestBuildBankSender 断言了三种错误路径都是「字面量 nil + error」，所以现在没有实际风险。但 DoD 原文「sender 构造返回 nil」与 (Sender, error) 这个签名之间存在这道缝，记在这里供参考。
3. discovery 提到 backtest_test.go 的 gofmt 问题是既有的，与本任务无关，我没有复核。

## 返工范围
只需要改 bank_test.go：给 D1 补上「全部成功」的场景。复验时我会重跑 V24、V25（应当转红）和 V3–V5，确认 bank.go 没有变化后，其余条目的结论可以直接沿用。
