# TASK-007 验证报告（QA 返工复验，test-pw-b）

- 任务：单资产一轮检测引擎（internal/pricewatch/engine.go）；`assignment_epoch`=3，`rework_count`=1。QA 返工由 Leader 13:22:47Z 改派给 dev-pw-c，原作者 dev-pw-a
- **被测树**：`feature/pricewatch` @ `8ed58bc17d3179e32c56b9d5caf9199484dcc32e`（= `verify_baseline.head`；出裁决前主仓库 HEAD 未变），独立 worktree `../wt-verify-TASK-007-b2`（detached），主工作区未改动
- 文件 sha256：`engine.go` `40f48a179b1294599c3fc3843d04166cf3730841dd02394f96eaac82a2e22014`、`engine_test.go` `16a6898f3e0b04fc5c29eda5650b1f08314eec56945b678808a7801f3e0c26dc`（= 返工提交 `3d087fed77aaf606255f7e6fc53ccc06131f40c4`）；依赖的 `health.go` `c3f8bbde…`（= TASK-011 返工 `dfae3ab`）
- discovery sha256 `938665b1c98b090a342337fa2fc25ec24e4cca96c61645af5a21116dc1ba3dc0` = 基线，无漂移
- **discovery 其余字段未被改动（可计数证明）**：`jq 'del(.qa_fix)' TASK-007.json | shasum -a 256` 得到 `d4254c2a88122655fcc6b23b706a5d6a89b1fb63d4d5a75a21ae356b0e73d24d`，**与我上一轮（09:38Z）记录的 verify_baseline.discovery_sha256 逐字相同**。dev-pw-c 只新增了 `qa_fix` 段，其内含 `author: dev-pw-c` 与 `provenance`（说明改派及上文字段归属 dev-pw-a）。顶层没有 `provenance` 键，归属说明写在 qa_fix 段内
- 范围：`3d087fe fix(TASK-007)` 只改 `writes` 的两个文件（engine.go +4、engine_test.go +81）。分支基于 `ebc366e`，先合入 TASK-011 的 `dfae3ab`（91efa2f），与任务文件 fix_items 写明的开工方式一致
- 工具链：go1.24.4 darwin/arm64，`GOTOOLCHAIN=local`

## 结论：VERIFIED

## 0. 历轮摘要与本轮对照

| 轮次 | 被测树 | 验证者 | 结论 | 要点 |
|---|---|---|---|---|
| 首轮 | 8cf608a | test-pw-a | REJECTED（task_defect） | `TestEngine_ConcurrentSnapshot` 读写零重叠（200/0），四处去锁全部存活 |
| 返工 1 | e9b963b | test-pw-b | VERIFIED（09:38Z） | 读侧持续到写侧结束。探针 5 次：读约 15 万次，读侧观察到价格翻转 18–19/19 次、不同的 Sent24h 值 11/11 个；四处去锁加三种部分加锁 5/5 红，只有新用例能抓到；44 个变异 42 KILLED |
| QA 第 2 轮 | — | QA | review_fix | R1-1 引擎侧：健康消息 SendText 失败只记 warn，「监控异常」一次失败就永久丢失（AD-30） |
| **本轮** | 8ed58bc | test-pw-b | **VERIFIED** | track 只在送达或 dry-run 写出之后 `Ack`，失败不 Ack，下一轮重发；新增 `TestCheckAsset_HealthSendFailureRetries`；整份 DoD 重新验，并发快照用例在新接口下仍然有效 |

## 1. 门禁重跑

| 命令 | 结果 |
|---|---|
| `go test -race -count=1 -coverprofile ./internal/pricewatch/` | `ok`（29.6s），包覆盖率 **94.3%** |
| `go vet ./internal/pricewatch/` | 无输出，rc=0 |
| `gofmt -l engine.go engine_test.go` | 无输出 |
| `-race -run 'TestCheckAsset\|TestEngine' -v` | 顶层 **17 PASS**（上一轮 16 个，加本轮新增 1 个）、0 FAIL |
| `-race -count=10 -run '^(TestEngine_ConcurrentSnapshot\|TestCheckAsset_HealthSendFailureRetries\|TestCheckAsset_HealthAlerts)$'` | `ok`，无 flaky |
| engine.go 函数覆盖 | `track` **100%**（上一轮 93.8%）；CheckAsset 91.8%；Snapshot、Sent24h 100% |

## 2. fix_items 核对矩阵

| fix_item | 落地 | 测试守卫 | 变异证据 | 判定 |
|---|---|---|---|---|
| 开工方式（先合 TASK-011-qa1 再接线） | 分支 = ebc366e + merge dfae3ab（91efa2f）+ 3d087fe；8ed58bc 合入 | — | — | ✓ |
| R1-1 引擎侧（AD-30）：送达或 dry-run 写出后才 Ack，失败不 Ack | engine.go:293–305：dry-run 写出 Out 后 `Ack`；`SendText` 出错记 warn 后 `return`；成功后 `Ack` | `TestCheckAsset_HealthSendFailureRetries` 三个子例 + 原有的 `TestCheckAsset_HealthAlerts` | dev 列的 4 个（K1 成功后不 Ack、K2 失败也 Ack、K3 dry-run 不 Ack、K4 发送前就 Ack）**全部 KILLED**；另加 K5（只 Ack Down）、K6（只 Ack Up）、K7（Ack 空事件）、m6b（去掉 dry-run 分支）全部 KILLED | ✓ |
| 补用例：真实 HealthTracker + 会失败的 sender；Down 失败后补发、Up 失败后同样补发 | 子例 1：30m 发 Down 失败，31m 重发仍失败，32m 送达，33m 不再发；Up 走同样的 34–37m 序列；全程 Down 恰 3 次。子例 2：Down 没送达就恢复，不发孤立的「已恢复」。子例 3：dry-run 4 轮只写出 1 次，Sender 0 次 | 同上 | 同上 | ✓ |
| **没有替身遮蔽**（本 sprint 第三次同族问题） | `Engine.health` 是 **具体类型字段 `HealthTracker`**（engine.go:41），不是接口，测试**无法**注入替身；engine_test.go 里没有任何代码碰 `health`（grep 只命中一行注释）；用例走 `newEngFixture` 的真实 Engine 和 `f.run`，发送失败用的是 fixture 的 `engSender.setErr`。另外，K 系变异改的是 engine.go 里真实的 Ack 调用，能被抓到本身就说明测试经过了真实的 tracker 状态机 | — | — | ✓ |
| 可选 R1-6（心跳列出当前 Down 键） | **未做**（discovery qa_fix.decisions 已说明） | — | — | 可选 |

## 3. done_criteria 覆盖矩阵（整份重新验）

| # | 完成标准 | 对应测试 | 守卫证据（本轮变异，m 编号沿用 test-pw-a 的 m0–m39，已迁移到新 engine.go） | 判定 |
|---|---|---|---|---|
| functional[0] | 两规则合成 1 条；状态 `{tier,1,now}`；LLM 1 次；五个数；Sent24h 1→25h 后 0 | `TestCheckAsset_TriggersAndMerges` | m8、m18–m22 KILLED（五个数上一轮已独立手算） | PASS |
| functional[1] | 发送失败时状态不变；清零照常落盘；恢复后补发 | `TestCheckAsset_SendFailureKeepsState` | m2、m3、m9 KILLED | PASS |
| functional[2] | 重启不重发 | `TestCheckAsset_RestartNoDuplicate` | m34 KILLED | PASS |
| functional[3] | 缺口跳过、状态不变、LLM 0 次 | `TestCheckAsset_GapSkipsRule` | m0、m23、m39 KILLED；m1 被 GapExactlyTenPercent 抓到 | PASS |
| boundary[0] | 坏价格换源 | `TestCheckAsset_BadPriceFallsBack` | m10–m15 KILLED | PASS |
| error_handling[0] | AI 失败照发；nil 时不调 AI | `TestCheckAsset_AIFailureStillSends` | m34、m16/m17/m31（TwoSidesAdvice）KILLED | PASS |
| error_handling[1] | 三类健康告警各恰 1 条异常、1 条恢复 | `TestCheckAsset_HealthAlerts`（Ack 接线后仍绿）+ `HealthSendFailureRetries` | m23、m24、m25、m38、K1、K5–K7 KILLED | PASS |
| non_functional[0] | race/vet/gofmt/覆盖率；DryRun；Snapshot；**Snapshot 与 CheckAsset 并发无 data race** | §1；`TestCheckAsset_DryRun`、`TestEngine_Snapshot`、**`TestEngine_ConcurrentSnapshot`** | m4、m5、m7、m6b、m36、m37 KILLED；**并发：见 §4** | PASS |

## 4. 上一轮的并发快照用例在新接口下仍然有效

本轮 track 里新增的 `e.health.Ack` 走 HealthTracker 自己的锁，与 `e.mu` 无关，快照与发送记录的四处加锁没有变化。在新树上重跑锁变异（`-race`，只跑 `^TestEngine_ConcurrentSnapshot$` ×5；跳过该用例跑整组 ×2）：

| 变异 | 只跑并发用例 | DATA RACE 条数 | 跳过并发用例 |
|---|---|---|---|
| m26 recordSnap 去锁 | **5/5 红** | 3,3,3,3,3 | 0/2 红 |
| m27 Snapshot 去锁 | **5/5 红** | 4,3,3,3,3 | 0/2 红 |
| m28b sent 追加去锁（自动迁移失败后手工重写） | **5/5 红** | 5,5,5,5,5 | 0/2 红 |
| m29 Sent24h 去锁 | **5/5 红** | 4,4,4,5,5 | 0/2 红 |
| X2 Sent24h 写回在锁外 | 4/5 红 | 1,1,1,0,1 | 0/2 红 |
| X3 recordSnap 锁只罩住 nil 初始化 | **5/5 红** | 2×5 | 0/2 红 |
| X4 Snapshot 只锁 rows 分配 | **5/5 红** | 3,3,2,3,2 | 0/2 红 |

结论与上一轮相同：七种去锁或部分加锁形态，**只有**并发快照用例能抓到；X2 单次漏检，属 race detector 的概率性检出。原实现在 `-race -count=10` 下全绿。

## 5. 独立变异汇总（`go test -overlay`，隔离 worktree）

- 变异体：test-pw-a 的 m0–m39 写于旧版 engine.go（`bd46e7f5…`）。我用 difflib 取出每个变异的改动块，带上下文在新文件上重放；锚点在新文件里出现不唯一则判 INVALID。m6（track 里的 dry-run 分支，正好是本轮改动的地方）与 m28（锚点重复）自动迁移失败，手工重写为 m6b、m28b。另加我自己的 X1–X4，以及针对 Ack 的 K1–K8。
- 每个变异先过 `go vet -overlay`。源文件 sha256 跑前跑后一致；worktree `git status --porcelain` 为 0 行。
- **合计 49 KILLED / 3 SURVIVED**（逻辑类 42 + 锁类 7 KILLED；另有 2 个 INVALID，已由 m6b/m28b 替代）。存活的 3 个都是等价变异：
  - m35 `sameState` 不比较时间：同前两轮的辨析
  - X1 `Snapshot` 不复制 Active：同上一轮的辨析
  - K8 dry-run 分支里先 Ack 后写 Out（只是换了顺序）：dry-run 下写 Out 不可能失败，所以先后顺序不可观察
- dev 自报 4/4：是我 K1–K4 的子集，结论一致。

## 6. 非 DoD 发现

1. AD-30 的固有代价：Telegram 持续不可用时，未确认的 Down 会**每一轮都重发一次**，直到送达。这是「宁可重复、不可丢失」的设计取舍，是预期行为。
2. R1-6（心跳列出当前 Down 键）未做，属可选项。
3. 上一轮列出的「同一资产必须串行调用 CheckAsset」约束，已由 TASK-008 的 `SerialPerAsset` 守住（我在 TASK-008 复验中确认过，R1 peak=6）。

## 7. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-007-b2 8ed58bc17d3179e32c56b9d5caf9199484dcc32e
cd ../wt-verify-TASK-007-b2
GOTOOLCHAIN=local go test -race -count=1 -coverprofile=c.out ./internal/pricewatch/ && go tool cover -func=c.out | grep engine.go
GOTOOLCHAIN=local go vet ./internal/pricewatch/; gofmt -l internal/pricewatch/engine.go internal/pricewatch/engine_test.go
git show e9b963be7640d5e54e0f2f8a373c082265d24451:internal/pricewatch/engine.go > <dir>/engine.old.go
python3 <scratchpad>/test-pw-b-T007r-mut/run.py "$PWD" <输出目录> <scratchpad>/testpwa-t007-mut <dir>/engine.old.go logic   # 另跑 lock
jq 'del(.qa_fix)' <主仓库>/.arcforge/discoveries/TASK-007.json | shasum -a 256   # 应为 d4254c2a…
```
