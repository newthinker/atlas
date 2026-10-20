# TASK-011 验证报告（QA 返工复验，test-pw-c）

## 前两轮摘要（原文已被本报告覆盖）

| 轮次 | 验证者 / 被测树 | 结论 | 要点 |
|---|---|---|---|
| 第 1 轮 | test-pw-a @ `f3d3223` | REJECTED（task_defect） | non_functional 的「并发调用 Fail/OK」只测了 Fail 一半：OK 在 `wg.Wait()` 之后串行调用，删掉 OK 的锁后 -race 5/5 存活 |
| 第 2 轮 | test-pw-b @ `ccb5ff2044723b7e5d1f8142aa97a9befdf29a21` | VERIFIED | 新增 `TestHealth_ConcurrentFailOK`；16 个变异 16/16 KILLED |
| QA round2 | `.arcforge/docs/05-review/qa-review-round2.md` §2 | REJECT → review_fix | R1-1：Fail 在发送前就置 down=true ⇒ 一次发送失败，「监控异常」永久丢失。修法见 AD-30 |

## 本轮被测对象

- 被测树：`8ed58bc17d3179e32c56b9d5caf9199484dcc32e`（feature/pricewatch，等于 `verify_baseline.head`）；隔离 worktree `../wt-verify-TASK-011-c`（detached）。
- 返工提交 `dfae3abb21382fd353d8852b15cbc3831564e0f2`（health.go +27/−6、health_test.go +122/−44，合入 `77c49ef`），之后是 TASK-007 的 Ack 接线 `8ed58bc`。在被测树上 `git diff dfae3ab HEAD` 对两个声明文件输出为空，改动全部在 `writes` 内。
- discovery sha256 `42697de8cc820b1a6728ecb250219cf42a36023dd987c4276b15c91c2cb357e5` 与 baseline 相同。
- 工具链 `GOTOOLCHAIN=local`。

## 结论：VERIFIED

附 2 条非阻断说明（§6）。

## 1. 门禁对照

| 项 | 第 2 轮（ccb5ff2） | 本轮（8ed58bc） |
|---|---|---|
| `go test -race -count=1 -cover ./internal/pricewatch/` | ok，93.1% | ok，**94.3%** |
| health.go 函数覆盖 | Fail/OK 100% | **Fail/OK/Ack 100%** |
| `go vet` / `gofmt -l internal/pricewatch/` | 无输出 | 无输出 |
| `TestHealth_*` | 11 | **13 PASS** |
| `go test -race -count=20 -run TestHealth_` | — | ok（并发用例无 flaky） |
| engine 侧 `TestCheckAsset_HealthAlerts` / `HealthSendFailureRetries`（TASK-007） | — | PASS |

## 2. 实现与 AD-30 逐条对照（`health.go` @ 8ed58bc）

| AD-30 条款 | 实现 | 测试 |
|---|---|---|
| Fail 满 After 且该键尚未确认 Down 时，**每次**都返回 Down（不改状态） | `Fail`：`if s.down \|\| now.Sub(s.since) < h.After { None }`，否则返回 Down，不写 down | `TestHealth_UnackedDownRepeats`（30m/35m/90m 都返回 Down） |
| Down 被 Ack 后不再返回 Down | `Ack`：Down → `s.down = true` | 同上（Ack 后 95m 无事件）、`DownAfterThreshold` |
| OK 在该键已确认 Down 时**每次**返回 Up | `OK`：`s.down` 为真时返回 Up，不删键 | `TestHealth_UnackedUpRepeats` |
| Up 被 Ack 后该键回到正常 | `Ack`：Up 且 `s.down` 时删键 | `UnackedUpRepeats`（Ack 后 OK 无事件，重新从零计时） |
| 未确认 Down 就恢复 ⇒ 无事件并清除计时 | `OK`：`!s.down` 时删键、返回 None | `TestHealth_RecoverBeforeDownAcked`（恢复后重新从头计 30m） |
| Engine 只在送达（或 DryRun 写出）后 Ack；发送失败不 Ack | `engine.go:280-305` `track`（TASK-007 范围，只读核对） | `TestCheckAsset_HealthSendFailureRetries` |

dev 的附加决策（未知键 Ack(Down) 不造状态；未确认 Down 的键 Ack(Up) 不清计时；Up 未确认期间 Fail 不再返回 Down）与 AD-30 不冲突，都有 `TestHealth_AckEdgeCases` / `UnackedUpRepeats` 守卫。

## 3. done_criteria 覆盖矩阵（4 条，整份重验）

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | After=30m：29m 无、30m 恰 1 次 Down、60m 不重复 | `TestHealth_DownAfterThreshold`（调用方形态 failAck）、`TestHealth_DownJustBeforeThreshold`（30m−1ns） | PASS（按 AD-30 解读「不重复」= 已确认后不重复，见 §6-1） |
| functional[1] | Down 后 OK 恰 1 次 Up、再 OK 无事件；未达 After 就 OK 无事件 | `TestHealth_UpAfterAckedDown`、`TestHealth_TransientFailureSilent` | PASS（同上） |
| functional[2] | 不同键互不影响；Fail→OK→Fail 从第二次 Fail 重新计时 | `TestHealth_KeysIndependent`、`TestHealth_RestartTimingAfterOK` | PASS |
| non_functional[0] | race/vet/gofmt/覆盖率 ≥80%；多 goroutine 并发 Fail/OK 在 -race 下无 data race | §1；`TestHealth_ConcurrentSafe`、`TestHealth_ConcurrentFailOKAck` | PASS（三处去锁各自 5/5 红，见 §5） |

### health_test.go −44 行审计

逐行看了 `git diff dfae3ab^ dfae3ab` 的删除行，分三类：

1. **改写为调用方形态**（`h.Fail/h.OK` → `failAck/okAck`，断言值不变）：DownOnceAfterThreshold→DownAfterThreshold、UpOnceAfterDown→UpAfterAckedDown、KeysIndependent、RestartTimingAfterOK、ZeroValueUsable。functional[0..2] 的每个时间点断言都还在，没有丢守卫。
2. **`TestHealth_ConcurrentSafe` 的 `downs[k] != 1` 放宽为 `< 1`**：AD-30 下两个 goroutine 可能在任一方 Ack 之前同时拿到 Down，「恰 1 次」不再是正确不变量。DoD 的「恰 1 次」由串行用例 DownAfterThreshold 守着（变异 H2/H4/H9 证明它仍会变红）。
3. **`TestHealth_ConcurrentFailOK`（交替不变量 ups≤downs≤ups+1）被 `TestHealth_ConcurrentFailOKAck` 取代**：交替不变量在 AD-30 下同样不再成立。non_functional 只要求「-race 下无 data race」，新用例覆盖 Fail/OK/Ack 三者，且三处去锁各自都能独立致红（§5），比旧用例更强。

结论：删除没有让任何 DoD 条目失去守卫。

## 4. fix_items 核对矩阵

| fix_item | 落实 | 守卫（变异证据） |
|---|---|---|
| R1-1：只在 Ack 后推进；新增 Ack；未 Ack 的 Down/Up 每次返回；未确认 Down 就恢复 ⇒ 无事件并清计时 | §2 逐条一致 | H1 原缺陷（发送前置 down）KILLED；H2 Ack(Down) 不推进 KILLED；H3 Ack(Up) 不推进 KILLED；H4 未 Ack 的 Down 只返回一次 KILLED；H5 未 Ack 的 Up 只返回一次 KILLED；H6 未确认 Down 就恢复仍发 Up KILLED；H8 恢复时不清计时 KILLED |
| 补用例：未 Ack 重复、Ack 后不再、Up 同理、并发 Fail/OK/Ack -race 无竞争且确有重叠 | UnackedDownRepeats / UnackedUpRepeats / ConcurrentFailOKAck（start 屏障） | 去锁 L_Fail/L_OK/L_Ack 在 ConcurrentFailOKAck **单独进程**各跑 5 次：**5/5、5/5、5/5 红**，每次 DATA RACE 条数分别为 17 19 16 16 17 / 7 4 10 10 8 / 10 8 10 2 9 ⇒ 读写确有重叠 |
| 证据指向 qa-review-round2.md §2 | — | — |

## 5. 独立变异（隔离：worktree + `go test -overlay`）

harness：`/private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/b3c3a6ba-7de9-492d-9e24-953680e5a079/scratchpad/t011c-mut/run.py`。每个变异体都断言锚点恰好出现 1 次、打印 diff，并先过 `go vet -overlay`（INVALID 0 个）。主仓库 health.go/health_test.go 的 sha256 与 `git status --porcelain` 指纹在运行前后一致。

| 变异 | 命中 | 结果 |
|---|---|---|
| H1 原缺陷：Fail 返回 Down 前置 `s.down = true` | UnackedDownRepeats、RecoverBeforeDownAcked | KILLED |
| H2 Ack(Down) 不推进 | 8 个用例 | KILLED |
| H3 Ack(Up) 不推进 | 4 个用例 | KILLED |
| H4 未 Ack 的 Down 只返回一次（加 sent 标志） | UnackedDownRepeats | KILLED |
| H5 未 Ack 的 Up 只返回一次 | UnackedUpRepeats、ConcurrentFailOKAck | KILLED |
| H6 未确认 Down 就恢复仍发 Up | RecoverBeforeDownAcked | KILLED |
| H7 Ack 对未知键造状态 | AckEdgeCases | KILLED |
| H8 恢复时不清计时 | RecoverBeforeDownAcked、RestartTimingAfterOK | KILLED |
| H9 `<` → `<=`（30m 不触发） | 10 个用例 | KILLED |
| L_Fail 去锁（-race） | 全量 KILLED；ConcurrentSafe 单独 KILLED；FailOKAck 单独 5/5 | KILLED |
| L_OK 去锁（-race） | 全量 KILLED；**ConcurrentSafe 单独存活**（预期，它不调 OK）；FailOKAck 单独 5/5 | KILLED |
| L_Ack 去锁（-race） | 全量 KILLED；两个并发用例各自单独 KILLED | KILLED |

dev 自报 14/14：我独立复现了 leader 点名的全部变异，另加 H4/H5/H6/H9，结论一致。

## 6. 非阻断说明

1. **DoD 文本早于 AD-30**：functional[0] 的「60m 不重复」和 functional[1] 的「恰 1 次 Up」按字面是无 Ack 语义，与 fix_item R1-1「未 Ack 每次都返回」冲突。本报告按 AD-30 的调用方形态（送达后 Ack）解读，测试也是这么写的。在途 DoD 文本没有更新，后来的读者需要配合 AD-30 一起看。
2. **跨失败周期的迟到 Ack**：表征测试（overlay 新增文件，未落盘）验证了 Fail→Down（发送中）→OK（清除）→Fail（新周期）→旧 Down 迟到 Ack 这个序列。结果是新周期才失败 1 分钟就被标成已确认 Down，下一次 OK 返回 Up（用户会收到没有对应异常的「已恢复」）。**当前生产不可达**：`engine.track` 在同一调用内同步执行 Fail→send→Ack，`Runner` 在单个 goroutine 里串行执行各资产（runner.go:14-16 写明了这个约束）。但 `HealthTracker` 的文档注释写着「可被多个 goroutine 并发调用」——它能保证的只是数据竞争安全，同键并发时的语义并不保证。建议在注释里注明这一点，或给 Ack 带上周期标识。

## 7. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-011-c 8ed58bc17d3179e32c56b9d5caf9199484dcc32e
cd ../wt-verify-TASK-011-c
GOTOOLCHAIN=local go test -race -count=1 -cover ./internal/pricewatch/
GOTOOLCHAIN=local go test -race -count=20 -run TestHealth_ ./internal/pricewatch/
python3 <scratchpad>/t011c-mut/run.py "$PWD" <scratchpad>/t011c-mut <主仓库路径>
```
