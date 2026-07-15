# TASK-002 验证报告 — ClearStreakDays 导出助手

- **验证者**: test-agent-1
- **提交**: f0c85cf（statemachine.go +22 / statemachine_test.go +29，纯新增）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: REJECTED（NEEDS WORK）
- **一句话**: build+test 全绿、coverage 91.2%，但 error_handling[1]（RecentSystem 错误上抛）的错误分支客观未覆盖（profile count=0），boundary[1]（回看受 max 限制）也未被任何用例 exercise——两条 verify_by=test 判据缺对应断言，且 errHistory fake 就在同文件、补测成本极低。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → 通过（exit 0）
- `GOTOOLCHAIN=local go test ./internal/crisis/ -count=1` → ok，coverage 91.2%（包级≥80%）
- ClearStreakDays 函数覆盖率 **90.0%**——缺口即错误分支
- 覆盖 profile 证据：`statemachine.go:160.16,162.3 1 0`（`if err != nil { return 0, err }` 块执行 **0 次**）
- 单跑 TestClearStreakDays → PASS

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | 最新两行 any_trigger=false、第三行 true→返回2（true 中断计数） | TestClearStreakDays 第一段：3 行(true,false,false 时序)→assert n==2 | PASS |
| boundary[0] | 空历史→0 | TestClearStreakDays：ClearStreakDays(NewMemHistory(),20)→assert n==0 | PASS |
| boundary[1] | 回看深度受 max 参数限制（RecentSystem(max)） | **无断言**。测试恒用 max=20 且历史 ≤3 行，max 从未作为上限被 exercise。ClearStreakDays 只透传 max 给 RecentSystem(max)（RecentSystem 经 headN 截断，见 memhistory.go:33），但「max 真被透传而非被忽略/硬编码」无测试锁定——若 `RecentSystem(max)` 改成 `RecentSystem(999)` 无用例能发现 | **FAIL** |
| error_handling[0] | detail 非法 JSON 保守中断计数而非上抛（同 systemDetailStreak） | TestClearStreakDays 第三段：h2 含 "not-json" 行 + 一条 false 行→require.NoError（未上抛）+ assert n==1（坏行处中断）。断言真实 | PASS |
| error_handling[1] | RecentSystem 返回错误时原样上抛 | **无断言，且错误分支 profile 覆盖=0**（客观未执行）。同文件 statemachine_test.go:167 已有 errHistory{err} fake（RecentSystem 恒报错），补测仅需 `ClearStreakDays(errHistory{err:assertErr},20)` + assert ErrorIs——dev 已在 error_handling[0] 证明会测错误路径，却独漏这条 | **FAIL** |
| non_functional[0] (test) | build ./... + test ./internal/crisis/ 全绿 | 亲跑通过，coverage 91.2% | PASS |

## detect_changes（Leader 代跑）
non_functional 无 review 条目；detect_changes 由 Leader 代跑（risk low、变更符号落 internal/crisis、零受影响流程为新符号预期），作影响面证据，无越界。

## 拒绝原因（reject_reason）
两条 verify_by=test 判据缺对应断言：
1. **error_handling[1]**：RecentSystem 错误上抛路径 profile 覆盖=0（statemachine.go:161 `return 0, err` 从未执行）。errHistory fake 已在同文件，补测 2 行即可锁定「上抛而非吞错」。
2. **boundary[1]**：max 限制未被 exercise，「max 真透传」无测试锁定（可加：append >max 行后 ClearStreakDays(h, 小于行数的 max) 断言计数封顶）。

## 建议修复方向（小改，仅测试，约 6 行）
在 TestClearStreakDays 追加两个子用例：
```go
// error_handling[1]: RecentSystem 错误原样上抛
_, err = ClearStreakDays(errHistory{err: assertErr}, 20)
assert.ErrorIs(t, err, assertErr)

// boundary[1]: 回看深度受 max 限制（3 行全 false，max=2 → 封顶 2）
h3 := NewMemHistory()
h3.Append([]Evaluation{clearStreakEval("2026-07-06", false)})
h3.Append([]Evaluation{clearStreakEval("2026-07-07", false)})
h3.Append([]Evaluation{clearStreakEval("2026-07-08", false)})
n, err = ClearStreakDays(h3, 2)
require.NoError(t, err)
assert.Equal(t, 2, n) // max=2 截断，第三行不计
```
（assertErr 已是 statemachine_test.go 内既有变量）

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 00c1baf）

**判定：VERIFIED（PASS）**

上轮 4 条 PASS 沿用（生产代码未动）；本轮复核两 FAIL 修复点。

| 复核点 | 证据 | 判定 |
|---|---|---|
| error_handling[1] RecentSystem 错误上抛 | 00c1baf 加 `ClearStreakDays(errHistory{err:assertErr},20)` + assert.ErrorIs(err,assertErr)。错误分支块 `statemachine.go:160.16,162.3` 覆盖由 0→**1**，ClearStreakDays 函数覆盖 90%→**100%**。**变异确认**：临时 `return 0,err`→`return 0,nil` 后 TestClearStreakDays FAIL | PASS |
| boundary[1] 回看受 max 限制 | 00c1baf 加 3 行全 false + `ClearStreakDays(h3,2)` → assert n==2（max=2 封顶第三行不计）。**变异确认**：临时 `RecentSystem(max)`→`RecentSystem(999)` 后测试在 statemachine_test.go:283 FAIL（expected 2），证明真能拦「忽略 max/硬编码」回归 | PASS |
| diff 范围 | `git show --stat 00c1baf`：仅 statemachine_test.go +13 行，无生产代码、无越界 | PASS |
| detect_changes | 沿用上轮 Leader 代跑结论（仅测试追加，无符号影响面变化） | PASS |

**结论**：两 FAIL 修复到位，均经变异测试确认断言非空洞（ClearStreakDays 覆盖 100%）。6 条 DoD 全 PASS，包级 coverage 91.4%。TASK-002 verified。

---

## 三次验证（QA CRITICAL review_fix，2026-07-15，epoch=3，rework=2，提交 2955906）

**判定：VERIFIED（PASS）**

背景：QA Skeptic 轮发现 ClearStreakDays 缺态内校验，CRISIS 康复尾段的 trigger-free 天数会污染 WATCH 退出进度（Leader 亲测：mixed 下应为 1、实际 19）。修复=签名加 state 参数 + 循环态内 break + cmd 调用点传 StateWatch。

| 复核点 | 证据 | 判定 |
|---|---|---|
| mixed 子例真实性 + 变异 | TestClearStreakDays 新增 mixed：18 CRISIS(false)+1 WATCH(false) 喂 StateWatch → assert 1。**独立变异**去 `if e.SystemState != state { break }` 后 FAIL（expected 1, actual 19）——态内 scoping 真锁 | PASS |
| 与 systemDetailStreak 口径同构 | statemachine.go:161 循环：`e.SystemState != state → break` 在前、`unmarshal err‖AnyTrigger → break` 在后；与 systemDetailStreak(137-149) 同序（state 校验 → unmarshal 保守中断）。谓词差异(AnyTrigger vs pred)属正常 | PASS |
| 既有子例适配无回归 | 5 既有子例（n=2 true中断/空=0/坏detail=1/错误上抛/max=2）全传 StateWatch，断言值不变，TestClearStreakDays PASS。坏detail 行改 SystemState=StateWatch 确保测 unmarshal-break 而非被 state-break 遮蔽 | PASS |
| 消费链回归 | TestBuildNotifyContext（ClearStreak=2：历史 1 WATCH false + 今日）+ TestBuildNotifyContextClearStreakConditions 均 PASS；新签名下 buildNotifyContext 传 crisis.StateWatch，语义准确 | PASS |
| 两包 build+test + 覆盖 | build ./... exit 0；internal 93.8%/cmd 绿；ClearStreakDays 函数级 100% | PASS |
| discovery 记录 | decisions 含签名变更、根因（跨状态污染）、同构裁决、mixed+变异自检记录 | PASS |
| 唯一调用方 | grep 确认 ClearStreakDays 仅 cmd/atlas/crisis.go:393 一处调用，已更新新签名；无旧签名残留 | PASS |

**结论**：QA CRITICAL（C1）修复正确、态内计数经变异锁死、与 systemDetailStreak 同构、消费链无回归。TASK-002 verified——**全链路交付重新齐备，9/9 verified。**
