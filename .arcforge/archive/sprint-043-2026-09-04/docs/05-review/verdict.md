# Sprint M1d（043）终审 verdict

> 本文件含**两轮判决**。第一轮由 **qa-m1d** 于 2026-09-03 作出（**CONTESTED**）；该实例随后因账号
> 额度耗尽停机，Leader 改派 **qa-m1d-b** 作复审（不重做两轮全审）。**生效判决是文首的复审终审**。
> 第一轮原文完整保留在文末「## 第一轮终审（qa-m1d，2026-09-03）」节，仅标题层级整体下沉一级，
> **正文一字未改**。

## 复审终审（qa-m1d-b，2026-09-04）

- 复审者：qa-m1d-b　对象：`master @ 290370feaa7e69b1eb533ca3c0a0683000853de4`
- 实测环境：独立 detached worktree `../wt-qa-m1d-b` @ `290370f`，`GOTOOLCHAIN=local`，主工作区零改动
  （每个变异窗口内 + 收尾各校验四个被变异文件在主工作区的 sha256 与 `git status --porcelain`，全程不变）
- 证据分档：**[已独立核实]** = 我在自己的 worktree 上跑过或直读代码/文件确认；
  **[机制已核数字未核]** = 我核过机制成立、引用他人报告的数字；**[仅转述]** = 只引用他人结论

### Verdict：**PASS**（对 M1d 代码交付），带**一条 accepted 之前必须完成的记录更正**（非 `review_fix` 轮次）

三句话版本：**A 组四条已全部闭合**，我用六条变异 + 一次行为复现独立确认，不是转述。
**A2 的处置足够**。**A1 的处置时序对、但登记的修法内容不完整**，照它办事的人仍会丢 30177 项文件；
这需要 Leader 改两处记录的文本，**不需要退回任何任务、不消耗 `max_iterations`**。

### 1. A 组四条：逐条闭合确认（全部 **[已独立核实]**）

我在自己的 worktree 上重跑了六条变异。**六条全部 KILLED**，且逐条核对了失败断言的文本与变异的
因果对应（不止看红/绿）：

| 变异 | 落点 | 转红的用例 | 失败文本（因果自证） |
|---|---|---|---|
| M-A3 副本查重恒未命中 | `snapshot.go:61` | `TestSaveSnapshotDivergedTwiceIsUnchanged` | `"[…a1.20260904T…, a1.20260905T…, a1.html]" should have 2 item(s), but has 3` |
| M-A8utc 去 `now.UTC()` | `snapshot.go:70` | `TestSaveSnapshotDivergedKeepsBothVersions` | 文件名不等（钉住 `…T083015Z`） |
| M-A4 分子改回 `len(errs)` | `ingest.go:202` | `TestIngestP1SendFailureIsReported` | 实测打出 **`2/1 期失败`**，不含 `1/1 期失败` |
| M-A8a `send P0`→`send P2` | `ingest.go:293` | `TestIngestNotifyFailureOnPendingIsLoud` | 实际串为 `…: send P2: notify: boom`，不含 `(<id>): send P0: notify: ` |
| M-A7 Duplicate 折回「入库」 | `notify.go:68` | `TestRenderP2DuplicateSaysValuesNotWritten` | 实际串 `…/monthly 入库 Duplicate…` 不含「未写入」 |
| M-A5 装载失败折回 `nil, nil` | `hestia.go:282` | `TestBuildHestiaSenderFromMainConfig` + `TestHestiaIngestPrintsNotifyStatus` | 两个子用例均 `An error is expected but got nil` |

> **有效性闸命中一次并被挡下**：`"send P0"` 在 `ingest.go` 出现 **2** 次（288 行是注释、293 行是代码），
> harness 的「模式须恰 1 次」闸拒绝落盘、**未打印 KILLED**；重定位到 `renderP0(obs, rep), "send P0"`
> 后才生效。若不设这道闸，第一次会被记成「变异未致红 ⇒ SURVIVED」的假发现。

**A3 的行为复现（[已独立核实]，比变异更直接）**：我在 worktree 里写了一个临时测试跑前一位 QA
报告的原始场景 —— `v1/v2/v2/v2` 四次调用、每次相隔一天：

```
kinds=[written diverged unchanged unchanged]  文件数=2  文件=[a1.20260905T083015Z.html a1.html]
```

与 Leader 判据表要求的 `written/diverged/unchanged/unchanged + 目录恰 2 文件`**逐项相符**；
返工前该场景实测为 4 文件。临时测试文件跑完即删，`git status` 空。

| 条目 | 结论 | 档 |
|---|---|---|
| **A3 + A8**（TASK-002，`d241978`→`290370f`） | **闭合**。`findSnapshotCopy` 逻辑正确：只在 `<id>.html` 字节不同后才 glob `<id>.*.html`，逐个 `bytes.Equal`，命中返 Unchanged 且 Path 指向命中副本；glob/read 失败均响亮返回并各有用例。第二次改稿（v3）仍正确落新副本并报 diverged | [已独立核实] |
| **A4 + A8**（TASK-005，`ec93e13`→`208a77c`） | **闭合**。分子改 `len(failedPeriods)`；`send P0` 与 `(<id>): snapshot: ` 两条 wrap 前缀各有能转红的断言 | [已独立核实] |
| **A7**（TASK-004，`bef5572`→`687e43a`） | **闭合**。`renderP2` 只对 `Duplicate` 改措辞，其余三种 Verdict 由同一用例反向断言仍是「入库 <Verdict>」。`bitemporal` 已是 `hestia` 包既有依赖（`schema.go`/`store.go`/`types.go`），**未引入新依赖边** | [已独立核实] |
| **A5**（TASK-007，`aba2fb3`→`b47e440`） | **闭合**。`buildHestiaSender() (hestia.Sender, error)`；`runHestiaIngest` 在 `openHestia` 之后、通道状态行之前直接返回，用例断言输出**不含** `notify:` 与 `no new reports`；未配置三形态仍 `nil, nil` | [已独立核实] |

### 2. 门禁复核（全部 **[已独立核实]**，锚 `290370f`）

| 项 | 我的实测 | 与 Leader 报数 |
|---|---|---|
| `go test ./internal/hestia/... ./cmd/atlas/... -count=1 -cover` | 两包 ok；**96.5%** / **76.3%** | 一致 |
| `gofmt -l internal/hestia cmd/atlas` | 仅 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | 一致（既有欠账） |
| `go vet ./internal/hestia/... ./cmd/atlas/...` | 零输出 | 一致 |
| 五个不动文件 `git diff --stat 4916106 290370f` | 空（0 行） | 一致 |
| `go.mod` / `go.sum` diff `ae088eb..290370f` | 空 | 一致 |
| 任务状态 | 8/8 `verified` | 一致 |
| `tasks/transitions.jsonl` | 124 行 = **93 迁移** + 31 update；93 条落在 **13 种边**上，逐种与 `write-matrix.json` 的合法写者**完全匹配，零越权** | 一致 |
| 返工轮次 | 4 条 `verified→review_fix`（002/005/004/007，均 `leader`、`task_defect`），`rework_count` 最大 **2** < `max_rework` 3 | 一致 |

### 3. 🔴 对 Leader 提问的正面回答：B 组处置是否足以转 PASS？

**A2（telegram 错误含 bot token）——处置足够，我同意归到第 6 步之前。** 依据：

- M1d 对 `internal/notifier/` 的 diff **为空** ⇒ 确非本 sprint 引入 **[已独立核实]**。
- `cmd/atlas/crisis.go:340,526` 已有同形暴露（`warning: notify failed: %v`）⇒ pre-existing 成立 **[已独立核实]**。
- `telegram.go:304,321` 的 `apiURL` 含 token 且用 `%w` 包 `*url.Error` ⇒ 错误文本必然带 token **[已独立核实]**；
  `sendDocument`（`:373`）同形。
- 泄露面是本机日志文件，而第 6 步正是它**第一次真实经代理发送**的时刻 —— 时序正确。
- 关于定级分歧：qa-m1d 的 WARNING 是**范围判断**（不是 M1d 引入、不在 M1d writes 内），两个 lens 的
  CRITICAL 是**运维风险判断**（M1d 新开了唯一无人值守、每天三次的出口）。**两者不矛盾**，
  不需要谁让步；用「不阻断 M1d 验收 + 阻断第 6 步」同时满足两方，是这条分歧的正确出口。

**A1（`deploy.sh` 的 `rsync --delete`）——时序对，但登记的修法内容不完整，因此现在还不够。**

我做了背对背干跑（`-n`，同一时刻两次、同一目标只换源，逐字抄 `deploy.sh:35-53` 的排除表），
运行时文件全程未动 **[已独立核实]**：

| 源 | `deleting` 条数 |
|---|---|
| worktree（`wt-qa-m1d-b`） | **30202** |
| 主仓库 | **8** |

worktree 那 30202 条的构成：`scripts/qlib_eval/.venv` **30177** 条、`.pytest_cache` 8、`cmd` 5、`bin` 2、
**`configs/config.yaml` 1**、其余零星。主仓库那 8 条全是过期的 `aktools_log.log*`（无害）。
成因是同一条：**源树缺少 gitignore 的运行时内容 × `--delete`**；排除表里已有
`/scripts/akshare/.venv/` 与 `/scripts/baostock/.venv/` 两条，**唯独漏了 `/scripts/qlib_eval/.venv/`**。

⇒ **已登记的修法「加 `--exclude='/configs/config.yaml'`」只闭合 30202 分之 1。**
一个照着 CONTRACTS §A7 与 final-report §9.0 办事的人，会以为问题解决了，然后丢掉整个 qlib_eval 虚拟环境。
本项目 CLAUDE.md 自己记着「处方写下了却没人读得到」是实测过的失效模式；**这里更糟一层——
处方被读到了，但它本身是不完整的**。

**结论：PASS 成立，但 `accepted` 之前 Leader 必须把 A1 的记录改成完整修法。** 这是一次文本更正，
不是代码返工，不消耗 `code_review.max_iterations`。建议写进记录的完整修法（按堵得由浅到深）：

1. `deploy.sh` 排除表补 **两条**：`--exclude='/configs/config.yaml'` 与 `--exclude='/scripts/qlib_eval/.venv/'`。
2. **更强的是堵成因**：`deploy.sh` 开头加 linked worktree 判别并**拒绝执行** ——
   `git rev-parse --git-dir` != `--git-common-dir` ⇒ 报错退出。本仓库 `arcforge-write.sh` 已用
   同一判别式且有测试，可直接照抄。第 1 条只堵两个**已知症状**，这条堵的是**成因**：
   下一个被 gitignore 的运行时目录出现时，第 1 条不会保护它。
3. §5 第 4 步注明「**必须在主仓库、`HEAD` == 合并锚下执行**，禁止在 worktree 执行」。

**为什么 A1 不该阻断 `accepted`**：它不在本次 diff 内，也不在任何 M1d 任务的 `writes` 内
（8 个任务的 `writes` 里唯一含 "deploy" 的是 TASK-007 的 `deploy/launchd/…plist` **路径**，
不是 `scripts/ops/deploy.sh`）**[已独立核实]** ⇒ `review_fix` 路由不到。扣住 `accepted` 不会让它
更快被修，只会卡住任务图。**它该阻断的是切换本身（§5 第 4 步），不是 M1d 的验收。**

### 4. 复审新增的一条 SUGGESTION（不阻断）

**A5 的契约变更使「主配置装不上」现在会阻塞入库** `cmd/atlas/hestia.go:309` **[已独立核实]**。
返工前：主配置装不上 ⇒ 不发通知、照常入库；返工后 ⇒ `runHestiaIngest` 直接返回，**不入库**。
这是 Leader 拍板的取舍，方向我同意（响亮优于静默）。代价是有界的、值得写进 CONTRACTS：
数据**不会永久丢失**（没入库 ⇒ `HasArticle` 为假 ⇒ 下次唤起重新发现，launchd 每天 3 次），
失败是响亮的（err.log + 非零退出）。仅需在 §A6 的契约更正里补一句这个副作用，供运维预期。

### 5. 对另三条挂账的意见（Leader 要求核对归类）

1. **007 错误串 `%s`(cfgFile) 无守卫、H-A5c 存活判等价变异 —— 同意，且验证者的理由准确。**
   我直读了 `TASK-007-verification.md:39,45` 与测试文件：`cmd/atlas/hestia_test.go` 里
   `hestia notify` 出现 **0** 次 ⇒ 外层前缀确无断言；两条用例都用「文件不存在」造失败，
   `os.PathError` 内层已带完整路径，去掉外层 `%s` 后 `Contains cfgFile` 仍过 **[已独立核实]**。
   验证者**已自行标明**这个「等价」是**在被测输入下**成立的，并给出杀死它的输入（YAML 非法）——
   限定条件写在了报告里，不是一句无条件的「等价变异」。归类正确。
2. **007 的 DoD 原文被 A5 推翻 ⇒ 登记 CONTRACTS §A6 契约更正 —— 同意。** TASK-007 的
   `done_criteria.functional[1]` 白纸黑字写着「`buildHestiaSender() hestia.Sender`：
   `loadConfigOrDefaults()` 出错或 … ⇒ 返回**字面量 nil**」**[已独立核实]**，而交付是
   `(hestia.Sender, error)` 且装载出错返回错误。DoD 文本与交付相悖属实，走契约更正是对的
   （`verified` 之后 DoD 文本已无人可改，CONTRACTS 是唯一落点）。
3. **§B 措辞（「导出函数面未改；新增导出类型 1（`Sender`）、字段 3」）进 CONTRACTS §C —— 同意。**
   `bitemporal` 是既有依赖，`Sender` 是本 sprint 唯一新增导出类型 **[已独立核实]**。

### 6. 复审小结

**A 组清零（六条变异 + 一次行为复现，全部我自己跑的）；门禁八项逐项复现，与 Leader 报数一致；
93 条迁移零越权。M1d 的代码交付判 PASS。** 唯一未了的是 A1 的**记录内容**：它登记的修法
只覆盖了实际影响的 1/30202，必须在 `accepted` 之前改对；A2 的处置我认为已经足够。

**我没有做的事**（避免被读成已核）：没有重跑 qa-m1d 的三个 lens 子代理，没有重跑 A6/A9/A10 那些
挂账项的实测，没有验证切换清单 §5 七步在真实运行时的可执行性 —— 这三块沿用第一轮结论，档位
**[仅转述]**。

---

## 第一轮终审（qa-m1d，2026-09-03）

> 以下为 qa-m1d 原报告全文（原标题：`# Sprint M1d（043）终审 verdict`），标题层级整体下沉一级，正文未改。

- 审查者：qa-m1d　时间：2026-09-03　对象：`ae088eb253b64b36e10558a02587e3fa657f5f3e..f3d6eb282c83e0ca730b1713907f5220114ee86b -- internal/hestia cmd/atlas configs deploy`
- 两轮报告：`round1-review.md`（常规）、`round2-adversarial.md`（Skeptic / Operator / Architect 三视角；codex 跨模型因用量上限未产出，已降级）

### Verdict：**CONTESTED**

判据（code-review skill）：有 high-severity 发现，且 reviewer 间**有分歧** ⇒ CONTESTED，需 Leader / 人类判断。具体两处分歧与一处路由不到：

1. **A2 token 进日志**：事实三方一致（[核实]），严重度分歧——Skeptic/Operator 评 CRITICAL，我评 WARNING（pre-existing 于 telegram 包、crisis 同暴露、泄露面是本机日志）。
2. **A1 deploy.sh `--delete` 会删运行时 `config.yaml`**：Operator 评 CRITICAL，我 `rsync -n` 独立复现（[核实]）；它**不在 diff 内、不在任何 M1d 任务的 `writes` 里**，`review_fix` 路由不到，但它是 spec §5 第 4 步的执行前提。是否阻断 M1d accepted，只能由人定。
3. WARNING 级 A3–A8 里，A3（改稿后幂等失效）与 A4（`2/1 期失败`）是本 diff 内的实测缺陷、各 ≤ 10 行可修；A5/A7 是设计取舍；A6 是 spec 接受的取舍；A8 是测试缺口。按 `severity_threshold: warning`，这些未解决前不能给 PASS。

**数据正确性**：三方一致——`Parse/Validate/Store` 一行未动（diff 空 [核实]），入库路径不受影响；两包测试全绿、覆盖率 96.4% / 76.2%、vet 零输出、无新依赖、零新增导出函数（[核实]）。本 sprint 三次返工补的三条守卫（yaml 原文、send 在 Fprintf 之后、两行接线）都是能转红的形态；我补做的「删 `WithProxy`」变异红（[核实]）。

### CRITICAL 逐条（文件:行号 + 复现命令）

#### A1 · `scripts/ops/deploy.sh:35-52`（不在 M1d diff 内，在 §5 第 4 步执行路径上）
复现（只模拟，`-n` 不写任何字节；排除表与脚本逐字相同）：
```bash
WT=/Users/zuowei/workspace/go/src/github.com/newthinker/wt-qa-m1d   # 任何没有 configs/config.yaml 的树
RT=/Users/zuowei/workspace/runtime/atlas
rsync -a -m --delete -n -v --exclude='/.git/' --exclude='/.worktrees/' --exclude='/.idea/' --exclude='/.vscode/' \
  --exclude='/.gitignore' --exclude='/.gitnexus/' --exclude='/.github/' --exclude='/.kanban/' \
  --exclude='/.arcforge/' --exclude='/.claude/' --exclude='/.codex/' --exclude='/.agents/' \
  --exclude='/arcforge.config.json' --exclude='*.go' --exclude='/go.mod' --exclude='/go.sum' --exclude='/cover.out' \
  --exclude='/docs/' --exclude='/AGENTS.md' --exclude='/CLAUDE.md' --exclude='/README.md' \
  --exclude='/scripts/qlib_eval/tests/' --exclude='/scripts/qlib_eval/conftest.py' --exclude='/scripts/qlib_eval/.pytest_cache/' \
  --exclude='/scripts/qlib_warehouse/tests/' --exclude='/scripts/qlib_warehouse/.pytest_cache/' \
  --exclude='__pycache__/' --exclude='*.pyc' --exclude='.DS_Store' --exclude='/data/' --exclude='/logs/' \
  --exclude='/scripts/akshare/.venv/' --exclude='/scripts/baostock/.venv/' \
  --exclude='/qlib_csv/' --exclude='/qlib_csv_hk/' --exclude='/qlib_csv_us/' --exclude='/fundamentals_csv/' --exclude='/fundamentals_csv_us/' \
  --exclude='/signals*.csv' --exclude='/reports/' "$WT/" "$RT/" | grep 'deleting configs/config.yaml'
## 输出：deleting configs/config.yaml
```
修法：`deploy.sh` 加 `--exclude='/configs/config.yaml'`；§5 第 4 步注明只能在主仓库、HEAD == 合并锚下执行。

#### A2 · `internal/notifier/telegram/telegram.go:304,321` → `internal/hestia/ingest.go:189,202`（严重度有分歧）
复现（worktree 内临时测试，已删）：
```go
tg := telegram.New("SECRET-TOKEN-123", "42", telegram.WithProxy("http://127.0.0.1:<已关闭端口>"))
err := tg.SendText("x")
// telegram: failed to send message: Post "https://api.telegram.org/botSECRET-TOKEN-123/sendMessage": proxyconnect tcp: … connection refused
```
该 err 经 `notifyError` → `Ingest` 的 `%s FAILED: %v`（out.log）与返回错误（err.log）。修法：`sendPayload` 对 `*url.Error` 脱敏 + 测试 `NotContains(token)`。

### 给 Leader 的处置建议（按落点分组；reason_class 均为 `task_defect`）

**A. 本 diff 内、可走 `review_fix` 的（建议一轮合并处理，总改动 < 60 行）**
| 任务 | fix_items |
|---|---|
| TASK-002（`snapshot.go`/`snapshot_test.go`） | A3：mismatch 后再与 `<id>.*.html` 逐个 `bytes.Equal`，命中即 unchanged；补三次调用测试；A8：Diverged 用例改传非 UTC 时区断言文件名仍 `…Z` |
| TASK-005（`ingest.go`/`ingest_test.go`） | A4：汇总用 `len(failedPeriods)` 并断言 `1/1 期失败`；A8：补「落 pending + 发送失败 ⇒ `(<id>): send P0: notify: `」用例；把 `TestIngestFailsPeriodWhenSnapshotUnwritable` 改成 wrap 前缀形态 `(<id>): snapshot: `；（可选）通知失败打 `NOTIFY FAILED` 且不进 `failedPeriods`；（可选）`db:` 旁加 `snapshots: <abs>` |
| TASK-007（`hestia.go`/`hestia_test.go`） | A5：`cfgFile != "" && loadConfigOrDefaults` 出错 ⇒ 返回错误或三分文案，并改「unloadable ⇒ nil」用例为新契约 |
| TASK-004 或 TASK-005（`notify.go`） | A7：Duplicate 的 P2 措辞「已在库，本次抽取值未写入」（设计取舍，Leader 先拍板再派） |

**B. 不在 M1d 任务 `writes` 内、需新开任务或人类会话外处理（须在 §5 相应步骤之前）**
- A1 `scripts/ops/deploy.sh` 排除 `/configs/config.yaml` —— **§5 第 4 步之前**。
- A2 `internal/notifier/telegram` 错误脱敏 + 测试 —— **§5 第 6 步之前**（第一次真实经代理发送）。
- 清单文本：第 4 步「必须主仓库执行」、`install-services.sh` 会重装 hestia-ingest（bootstrap 挪到第 7 步或再 bootout）、第 3 步归档 out/err 日志、第 7 步基线取第 6 步之后、删运行时根目录残留 `atlas`（Aug 7）、`--only-period 2026-07` 约 10 月中旬后失效、`YYYY-12/06/03` 双命中两条 P2。

**C. 挂账（不阻断）**：A6 通知失败无重试（spec §4.3 接受；建议 M1.5 加退避重试或 `status` 显示最近 notify 失败）；A8 后两条变异（token/chat 对调、OnlyPeriod 只比年）；CONTRACTS §B「导出面未改」措辞改「导出函数面未改；新增导出类型 1、字段 3」。

### 若人类判定 A1/A2 不阻断 M1d
则本 diff 的阻断项只剩 A 组 WARNING；走一轮 `review_fix`（`max_iterations: 3` 内）后我复审，A 组清零即可转 PASS。
