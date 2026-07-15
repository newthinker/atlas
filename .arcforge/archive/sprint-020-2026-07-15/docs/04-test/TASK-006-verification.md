# TASK-006 验证报告 — notify_render（三）日报（较昨日）与周报（退出进度）

- **验证者**: test-agent-1
- **提交**: 569933e（notify_render.go +61：colorWord/diffLine/renderDaily/renderWeekly；test +79）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: REJECTED（NEEDS WORK）
- **一句话**: 高质量提交——diffLine 三级判据各有区分用例且变异全拦截、字面值对设计 §5.3/5.5/6.5/6.6、浮点直等注释双写。唯一 gap 正对应 Leader 概念#3：renderWeekly 退出进度的 watch_exit_days 未用异值锁定，硬编码 20 变异静默通过（testConfig.WatchExitDays=20，测试只断言默认值）。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → exit 0；`go test ./internal/crisis/` → ok，coverage 93.6%
- colorWord/diffLine/renderDaily/renderWeekly 函数级 100%
- 变异矩阵：
  | 变异 | 目标测试 | 结果 |
  |---|---|---|
  | diffLine 删 continue（迁移/读数互斥） | TestDiffLineLevels/TestRenderDaily | FAIL ✓ |
  | diffLine 去 inAbnormal 守卫（非异常区变值） | TestDiffLineLevels | FAIL ✓ |
  | diffLine 顺序反转（append→prepend，AllIndicators 序） | TestDiffLineLevels | FAIL ✓ |
  | renderDaily 去页脚 | TestRenderDaily | FAIL ✓ |
  | **renderWeekly M 硬编码 20（watch_exit_days）** | TestRenderWeekly | **ok ✗ 未拦截** |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | renderDaily 首行"[P1] 📍 {STATE} 日报 第 {StateDays} 日 · MM-DD"、正文"异常指标："、尾注 diffLine+"盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日" | TestRenderDaily：HasPrefix"[P1] 📍 BREWING 日报 第 5 日 · 07-18\n\n异常指标：\n🔴 信用 hy_oas 618bp"、含盘中提示、HasSuffix 页脚 | PASS |
| functional[1] | diffLine 状态迁移优先"{ind} 转{色}（原{色}）"；读数变化仅当日异常区；顺序 AllIndicators | TestDiffLineLevels：exact"较昨日：move 转白（原绿） · hy_oas 转红（原黄）"+NotContains"+118bp"(迁移互斥)/"vix"(非异常区)/"sofr"(缺PrevDay)。三级变异均 FAIL 拦截 | PASS |
| functional[2] | renderWeekly 首行"[P1] 📅 Cassandra 周报 · MM-DD 当周 · {STATE} 已持续 N 个评估日"；退出进度"触发条件已连续解除 {ClearStreak} 日（回 NORMAL 需连续 {watch_exit_days} 日）"；尾注"下次周报：下周一 · 状态变更即时通知" | TestRenderWeekly：首行 ✓、"退出进度：...解除 8 日（回 NORMAL 需连续 20 日）"、尾注 ✓、页脚 ✓。ClearStreak=8 已锁。**但 watch_exit_days 只断言 testConfig 默认值 20——硬编码 20 变异静默通过，"来自 cfg" 未锁** | **FAIL** |
| boundary[0] | 完全无变化 → "较昨日：无变化" | TestRenderDaily nc2：全绿 PrevDay 仅 vix 无变化 → "较昨日：无变化" | PASS |
| boundary[1] | PrevDay 缺行的指标不参与 diffLine | TestDiffLineLevels：sofr(RED 异常但缺 PrevDay)→NotContains"sofr" | PASS |
| non_functional[0] (test) | renderDaily/renderWeekly 以 notifyFooter 结尾 | 两测试 HasSuffix(notifyFooter)；去页脚变异 FAIL | PASS |
| non_functional[1] (test) | build ./... + test 全绿 | exit 0、绿 93.6% | PASS |

## Leader 六点核查回复
1. **diffLine 三级各有区分用例 PASS**：迁移/读数互斥（删 continue FAIL）、非异常区变值否定（去 inAbnormal FAIL）、AllIndicators 序（顺序反转 FAIL，exact-string 真锁）。三级独立变异全拦截。
2. 字面值对设计：**逐字符对齐**（"第 N 日"/"盘中 JPY 监测运行中（每 30 分钟）"/"触发条件已连续解除 N 日（回 NORMAL 需连续 M 日）"/"下次周报：下周一 · 状态变更即时通知"，全角括号正确）。
3. **renderWeekly M 值来自 cfg——这是拒因**：M 取 cfg.StateMachine.WatchExitDays 是对的，但测试只用 testConfig 默认 20，硬编码 20 变异静默通过，"防硬编码 20" 未锁。
4. boundary[0]"较昨日：无变化" + boundary[1] 缺 PrevDay 跳过：均 PASS。
5. 浮点直等（d != 0 不加 epsilon）：代码注释（notify_render.go:231-232）+ discovery decisions 双写到位。语义 Leader 已批准，不判 FAIL。
6. 页脚：renderDaily/renderWeekly 均以 notifyFooter 结尾，去页脚变异 FAIL。

## detect_changes（Leader 代跑）
low、affected 空；输出中 cmd 符号为 T8 未提交插入的位移伪影，与本任务无关（本任务 diff 仅 notify_render 两文件）。build ./... exit 0（cmd/atlas 当前可编译）。

## 拒绝原因（reject_reason）
functional[2]：renderWeekly 退出进度的 watch_exit_days 未用异值锁定——硬编码 20 变异静默通过（testConfig.WatchExitDays=20，测试仅断言默认值）。与 T2 max/T3 eps/T4 三级同类：配置注入须用非默认值锁定（对比 T5 用 7/12/25 互异值正确锁 exit_days）。其余 6 条全 PASS。

## 建议修复方向（小改，仅测试，约 2 行）
TestRenderWeekly 用非默认 WatchExitDays 锁注入：
```go
cfg.StateMachine.WatchExitDays = 25
msg = renderWeekly(cfg, NotifyContext{Res: res, StateDays: 18, ClearStreak: 8})
assert.Contains(t, msg, "回 NORMAL 需连续 25 日") // 硬编码 20 在此 FAIL
```
（或直接把现有 20 断言改用非默认值）

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 caab292）

**判定：VERIFIED（PASS）**

上轮 6 条 PASS 沿用（生产代码未动）；本轮复核 functional[2] 的 watch_exit_days 注入锁。

| 复核点 | 证据 | 判定 |
|---|---|---|
| functional[2] watch_exit_days 异值注入锁 | caab292 加 `cfg.StateMachine.WatchExitDays = 25` + assert "回 NORMAL 需连续 25 日"。亲跑 PASS。**变异确认**：硬编码 `20` 后 TestRenderWeekly 正确 FAIL（此前静默通过）——防硬编码已锁 | PASS |
| diff 范围 | 仅 notify_render_test.go +4 行，无生产代码、无越界 | PASS |

**结论**：watch_exit_days 注入锁到位（同 T5 异值锁法），变异确认防硬编码有效。7 条 DoD 全 PASS。TASK-006 verified。
