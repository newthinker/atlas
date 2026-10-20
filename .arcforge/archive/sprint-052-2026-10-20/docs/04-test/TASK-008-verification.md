# TASK-008 验证报告（返工复验，test-pw-b）

- 任务：调度与每日心跳（internal/pricewatch/runner.go）；`assignment_epoch`=2，`rework_count`=1
- **被测树**：`feature/pricewatch` @ `1244e2a2dc36c939b77a3d5a735120e652628fc9`（= `verify_baseline.head`；出裁决前主仓库 HEAD 未变），独立 worktree `../wt-verify-TASK-008-b`（detached），主工作区未改动
- 文件 sha256：`runner.go` `87b875a99f15b17cd75da44ccd13d24d54a4c98e144d5cffdda15f783c4f78ee`（与上一轮相同，**未改**）；`runner_test.go` `3e0f7a18c00e603abfe8421fdfd39d838fa36e66a6b449ec984d72d5bb47110d`（= 返工提交 `17e1906631a0f70b5652000f5fa8cc20795c42f7`）
- discovery sha256 `950a05f1bac449fbc2422351cb3ed7c1ff9843da9f4ce74827af6b46d55505c8` = 基线，无漂移
- 范围：`git diff --stat 0f218bb..1244e2a` 只有 `runner_test.go`（+63/−0，纯新增），在 `writes` 内；提交主题 `test(TASK-008)` / `merge(TASK-008)` 符合约定
- 工具链：go1.24.4 darwin/arm64，`GOTOOLCHAIN=local`

## 结论：VERIFIED

## 0. 上一轮摘要与本轮对照

| | 上一轮（test-pw-a @ 0f218bb） | 本轮（test-pw-b @ 1244e2a） |
|---|---|---|
| 结论 | REJECTED（task_defect） | **VERIFIED** |
| 缺陷 | DoD f[3] 的生产路径 `r.Engine.CheckAsset` 从未执行（所有测试都注入替身 check，checkAsset 覆盖 66.7%）；R22（该行改成 `return nil`）下全部测试照绿 | 新增 `TestRunner_RunOnceUsesEngine`、`TestRunner_RunUsesEngine`，用真实 Engine，断言发出 1 条提醒、day_drop 状态推进、Snapshot 价格为 90；checkAsset 覆盖 **100%** |
| R22 | 存活 | **KILLED，且只有新用例能抓到**（§3） |
| R21（取消后仍发心跳，非 DoD） | 存活 | KILLED，只有 `TestRunner_NoHeartbeatAfterCancel` 能抓到 |
| 其余 DoD / 串行 / 稳定性 | PASS | PASS（整份重新逐条验，并复核串行重叠与负载稳定性，见 §2、§4） |

## 1. 门禁重跑

| 命令 | 结果 |
|---|---|
| `go test -race -count=1 -coverprofile ./internal/pricewatch/` | `ok`（29.4s），包覆盖率 **93.7%** |
| `go vet ./internal/pricewatch/` | 无输出，rc=0 |
| `gofmt -l runner.go runner_test.go` | 无输出 |
| runner.go 函数覆盖 | Run / RunOnce / checkAsset / dueHeartbeat 100%，step 93.3%，heartbeat 80%（未覆盖的是 GetKV/PutKV 出错时的日志分支） |
| `-race -count=5 -run 'TestRunner\|TestDue' -v` | 14 个顶层测试 × 5 = **70 PASS**、0 FAIL |

## 2. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 守卫证据（m0–m23 = test-pw-a 的 R1–R24 变异体，Y1–Y7 是本轮新增） | 判定 |
|---|---|---|---|---|
| functional[0] | dueHeartbeat：08:59 false、09:00 且 last 空 true、当天已发 false、次日 10:30 true、at 空恒 false、Asia/Shanghai 按本地日期 | `TestDueHeartbeat` | m12、m13（按 UTC 取日期/时刻）、m14（去掉当天已发判断）、m15（`!Before` 改 `After`）、m16、m17（at 空或非法时返回 true）KILLED | PASS |
| functional[1] | 真实时间 Tick=10ms、30/90ms，前者 ≥ 2×后者；`-count=5` 通过 | `TestRunner_Schedules`（另有可控时钟的 `ScheduleNoDrift`） | m4–m6 KILLED；稳定性见 §4 | PASS |
| functional[2] | 两次 tick 都过 09:00 只发 1 条心跳；kv 为当天；内容来自 Snapshot/Sent24h | `TestRunner_HeartbeatOncePerDay`、`TestRunner_HeartbeatDateIsLocal` | m10（不写 kv）、m11（kv 写 UTC 日期）、m18（Sent24h 写死 0）、m19（Snapshot 传 nil）、m23（不用 Engine.Now）、Y7 KILLED | PASS |
| functional[3] | RunOnce 每个资产**恰调一次 CheckAsset**；出错不阻断其余；Run 首轮立即 | `TestRunner_RunOnce`（替身）+ **`TestRunner_RunOnceUsesEngine`、`TestRunner_RunUsesEngine`（生产路径）** + `TestRunner_FirstRoundImmediate` | **m21（R22）KILLED，只有新用例能抓到**；m7、m8、Y1（调两次）、Y2（首错即返回）、Y3（只检第一个资产）KILLED；m1、Y5（首轮等一个 Tick）KILLED | PASS |
| boundary[0] | 心跳发送失败不写日期，下一 tick 重试成功才写 | `TestRunner_HeartbeatRetryAfterSendFailure` | m9、m22、m10 KILLED | PASS |
| error_handling[0] | ctx 取消后 1 秒内返回 nil，之后不再调用 CheckAsset | `TestRunner_StopsOnCancel`、`TestRunner_CancelledBeforeStart` | m2（返回 ctx.Err）、m3（step 不检查 ctx）KILLED | PASS |
| non_functional[0] | race/vet/gofmt/包覆盖率 ≥80% | §1 | — | PASS |
| 约束（TASK-007） | 同一资产不得并发 CheckAsset | `TestRunner_SerialPerAsset` | 见 §4 | PASS |

## 3. 返工点消融（`-race`，overlay）

| 变异 | 只跑新增三例 | 跳过新增三例（整包） | 结论 |
|---|---|---|---|
| m21 = R22（`return r.Engine.CheckAsset(ctx, a)` 改成 `return nil`） | **红**（RunOnceUsesEngine、RunUsesEngine） | **绿** | 只有新用例能抓到，生产路径的缺口已闭合 |
| m20 = R21（ctx 取消后仍发心跳） | **红**（NoHeartbeatAfterCancel） | **绿** | 只有新用例能抓到。新用例里另有「未取消时确实会发」的对照，排除了「本来就不到期」这种空断言 |

dev 自报的 R22/R21 消融**被独立复现**。

## 4. 复核上一轮已通过的部分

### 4.1 SerialPerAsset 真的有时间重叠（计数）

m0 = R1：每个到期的检测都起一个 goroutine，不等上一轮。`-race` 连跑 5 次，**5/5 红**，`SerialPerAsset` 每次报 **`concurrent CheckAsset peak = 6`**（5 次都是 6），`ScheduleNoDrift` 也同时变红。原实现在同一组测试里 5/5 绿，断言是 peak==1 且每个资产调用 ≥3 次。50ms 的检测期间会经过多个 10ms 的到期点，重叠确实发生，结果与上一轮一致。

### 4.2 真实时间用例的稳定性（6 个：Schedules、SerialPerAsset、StopsOnCancel、FirstRoundImmediate、RunUsesEngine、CancelledBeforeStart）

| 条件 | 结果 |
|---|---|
| 后台同时跑 2 份全包 `-race -count=3` 做负载，`-race -count=20` | 6 个用例各 **20/20 PASS** |
| `-race -cpu 1 -count=20` | 各 **20/20 PASS** |

新增的 `RunUsesEngine` 是真实时间用例（runFor 200ms），在负载下同样稳定。

### 4.3 心跳本地日期 / 补发 / 不重复 / 失败重试；首轮立即；到期无漂移；ctx 取消

都由 §2 对应的变异逐项抓到：本地日期 m11/m12/m13/m23；补发（睡眠错过）`TestDueHeartbeat` 次日 10:30 用例与 m14；不重复 m14/m10；失败重试 m9/m22；首轮立即 m1/Y5；到期无漂移 m4/m6（`ScheduleNoDrift` 精确断言 FAST=7、SLOW=3）；ctx 取消 m2/m3/m20。

## 5. 全量变异（31 个）

- 复用 test-pw-a 的 m0–m23（runner.go sha 未变，我逐个用 diff 核对过，都是对当前文件的单点改动），外加我新增的 Y1–Y7。每个变异先过 `go vet -overlay`，INVALID 0 个；`-race -run 'TestRunner|TestDue'`。
- **29 KILLED / 2 SURVIVED**，计数自洽（29+2+0=31）。源文件 sha256 跑前跑后一致；worktree `git status --porcelain` 为 0 行。
- 存活（都是非 DoD）：
  - **Y6：Engine 路径吞掉错误**（`_ = r.Engine.CheckAsset(...); return nil`）。新用例只断言「检测发生了」，没有断言 Engine 返回的错误会被 RunOnce 透传。RunOnce 的错误合并只在替身路径上有测试（`TestRunner_RunOnce` 的 errOn）。替身和生产路径共用同一个 `checkAsset` 返回值，所以按现在的代码结构，只有直接改生产那一行才会吞错，风险很低。但这条若退化，`pricewatch once` 遇到取价全失败或 Telegram 发送失败时仍会返回 0。建议补一个「真实 Engine、所有源取价失败 → RunOnce 返回 error」的断言。
  - **Y4：Engine 路径改传 `context.Background()`**。取消信号不再传进正在执行的 CheckAsset，影响 Syncer 和 LLM 调用的中断。DoD e[0] 的 1 秒上限是在替身上验的，不覆盖这一点；退化后的后果是关停变慢，不会出错。

## 6. 非 DoD 发现

1. Y6：RunOnce 在生产路径上的错误透传没有测试（见 §5）。
2. Y4：ctx 传入 Engine 没有测试（见 §5）。
3. 上一轮 §5 的「取消时会等正在跑的 CheckAsset，provider 不受 ctx 控制，最坏情况被 launchd 在 20s 后 SIGKILL」：runner.go 没改，这条已知限制不变，建议记入 final-report。

## 7. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-008-b 1244e2a2dc36c939b77a3d5a735120e652628fc9
cd ../wt-verify-TASK-008-b
GOTOOLCHAIN=local go test -race -count=1 -coverprofile=c.out ./internal/pricewatch/ && go tool cover -func=c.out | grep runner.go
GOTOOLCHAIN=local go vet ./internal/pricewatch/; gofmt -l internal/pricewatch/runner.go internal/pricewatch/runner_test.go
GOTOOLCHAIN=local go test -race -count=20 -run '^(TestRunner_Schedules|TestRunner_SerialPerAsset|TestRunner_StopsOnCancel|TestRunner_FirstRoundImmediate|TestRunner_RunUsesEngine|TestRunner_CancelledBeforeStart)$' ./internal/pricewatch/   # 另起 2 份全包 -race 作负载；再跑一次 -cpu 1
python3 <scratchpad>/test-pw-b-T008-mut/run.py "$PWD" <输出目录> <scratchpad>/testpwa-t008-mut   # 31 个变异 + 消融
```
