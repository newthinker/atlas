# TASK-007 验证报告 — notify_render（四）月报（趋势区）与 P2 运维速报

- **验证者**: test-agent-1
- **提交**: 9cf9e34（notify_render.go +76：trendLine/nextMonthlyDue/renderMonthly/renderOpsAlert；test +100）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: REJECTED（NEEDS WORK）
- **一句话**: 高质量提交——趋势行/速报字面值逐字对设计 §5.4/§6.4，三个 cfg 注入异值锁全到位、通道映射非示例用例齐全、跨月滞后已覆盖、6 个关键变异拦截。唯一 gap：boundary[0] 的「为空」子例未锁——测试用 delete 只测了「缺失」(!ok)，`len(tr.Window)==0` 分支变异静默通过（dev 注释误把 delete 标为「空窗口」）。

## 亲跑证据
- `go build ./...` exit 0；`go test ./internal/crisis/` ok，coverage 93.9%
- trendLine/nextMonthlyDue/renderMonthly/renderOpsAlert 函数级 100%
- 变异矩阵：
  | 变异 | 目标测试 | 结果 |
  |---|---|---|
  | watch_amber_count 硬编码 3 | TestRenderMonthly | FAIL ✓ |
  | daily_max_lag 硬编码 4 | TestOpsAlertLagInjection | FAIL ✓ |
  | weekly_max_lag 硬编码 12 | TestOpsAlertLagInjection | FAIL ✓ |
  | 通道 去 USDJPY→Yahoo | TestRenderOpsAlert | FAIL ✓ |
  | nfci weekly 特例去除 | TestRenderOpsAlert | FAIL ✓ |
  | renderMonthly 去页脚 | TestRenderMonthly | FAIL ✓ |
  | **boundary[0] 去空窗口守卫（len==0）** | TestRenderMonthly | **ok ✗ 未拦截** |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | renderMonthly 首行"[P1] 📅 Cassandra 月报 · YYYY-MM · {STATE} 已持续 N 个评估日"；单一趋势区 AllIndicators 序（无分区标题） | TestRenderMonthly：HasPrefix"[P1] 📅 Cassandra 月报 · 2026-08 · NORMAL 已持续 63 个评估日\n\n近 21 个交易日趋势（走势 · 月变化 · 5y分位）：\n"；NotContains"异常指标："/"其余指标：" | PASS |
| functional[1] | 趋势行=emoji 层名 指标 读数 sparkline 箭头+月变化 [· 5y分位][· tag][· 非色彩说明] 符 §5.4/§6.4 | TestRenderMonthly：hy_oas"267bp ▃…↗+2bp · 3% · 自满(COMPLACENCY)"（读数-sparkline 空格、箭头-Δ 无空格）；⚪ move 行带"· 数据断更(STALE)" | PASS |
| functional[2] | 尾注"AMBER 计数 {n}（触发 WATCH 需 ≥{watch_amber_count}）· 下次月报：{下月} 月首个交易日" | TestRenderMonthly："AMBER 计数 2（触发 WATCH 需 ≥3）· 下次月报：9 月首个交易日"；watch_amber_count 异值 5 注入锁，硬编码 3 变异 FAIL | PASS |
| functional[3] | renderOpsAlert 两行；nfci 用 weekly_max_lag_days；move/usdjpy→Yahoo 其余→FRED | TestRenderOpsAlert：move 全行逐字 + Yahoo；nfci"滞后 14 日 > 阈值 12 日"+FRED；usdjpy→Yahoo、vix→FRED。通道去 USDJPY/nfci 特例变异均 FAIL | PASS |
| boundary[0] | 趋势窗口缺失**或为空** → 月报省略该指标行 | TestRenderMonthly：`delete(nc.Trends, IndMOVE)`→NotContains"move"。**只覆盖「缺失」(!ok 分支)；「为空」(len(Window)==0 分支)无用例**——去 len==0 守卫变异静默通过。代码正确但断言缺 | **FAIL** |
| boundary[1] | StaleLastObs 缺失→"无历史观测"降级；月报日期不可解析→尾注降级"下月首个交易日" | TestRenderOpsAlert（vix 无观测→"无历史观测"）+ TestNextMonthlyDue/TestRenderMonthly（bad-date→"下月首个交易日"） | PASS |
| non_functional[0] (test) | renderMonthly 以 notifyFooter 结尾；renderOpsAlert 不含页脚/"非交易信号" | renderMonthly HasSuffix 页脚（去页脚变异 FAIL）；renderOpsAlert NotContains"非交易信号" | PASS |
| non_functional[1] (test) | build+test 全绿 | exit 0、绿 93.9% | PASS |

## Leader 五点核查回复
1. functional[1] 趋势行格式：**逐字符对齐**（读数-sparkline 空格、sparkline-箭头空格、箭头-Δ 无空格；"267bp ▃… ↗+2bp · 3% · 自满(COMPLACENCY)"结构对）。
2. 三 cfg 注入异值锁：watch_amber_count(5)/daily_max_lag(3)/weekly_max_lag(10) 均异值断言，三硬编码变异全 FAIL。
3. 通道映射：usdjpy→Yahoo、vix→FRED 非示例用例齐全；nfci weekly 特例分支有用例；去 USDJPY/nfci 特例变异 FAIL。
4. 三降级路径（无历史观测/bad-date 尾注/空窗口省略）+ 月报不分区负向 + 速报无页脚负向——除「空窗口」外均到位；**空窗口子例只测了「缺失」未测「为空」**（本次拒因）。
5. 滞后跨月：nfci 用例 2026-06-30→2026-07-14=14 日**已跨月**，daysBetween 跨月正确；TestNextMonthlyDue 另有跨年(12月→1月)。#5 已充分覆盖。

## 拒绝原因（reject_reason）
boundary[0] 明列「趋势窗口缺失**或为空**」两子例。测试 `delete(nc.Trends, IndMOVE)` 只走 `!ok`（缺失）分支；`len(tr.Window)==0`（为空）分支无用例——去该守卫变异静默通过。dev 测试注释把 delete 误标为「空窗口」，实为「缺失」。代码正确、仅断言缺一半。其余 7 条全 PASS。

## 建议修复方向（小改，仅测试，约 2 行）
TestRenderMonthly 加「在 map 中但 Window 为空」用例：
```go
nc.Trends[IndT10Y2Y] = Trend{Window: nil, Delta: 0} // ok=true 但空窗口
assert.NotContains(t, renderMonthly(cfg, nc), "t10y2y") // 为空也省略（锁 len==0 守卫）
```

## detect_changes（Leader 代跑）
low、affected 空、仅 notify_render 两文件、无越界。

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 e71159e）

**判定：VERIFIED（PASS）**

上轮 7 条 PASS 沿用（生产代码未动）；本轮复核 boundary[0]「为空」子例。

| 复核点 | 证据 | 判定 |
|---|---|---|
| boundary[0]「为空」分支 | e71159e 加 `nc.Trends[IndT10Y2Y] = Trend{Window: nil}` + NotContains"t10y2y"，且注释改正区分「缺失(!ok)」vs「为空(len==0)」两分支。亲跑 PASS。**变异确认**：去 `len(tr.Window)==0` 守卫后 TestRenderMonthly 正确 FAIL（此前静默通过） | PASS |
| diff 范围 | 仅 notify_render_test.go +5/-1，无生产代码、无越界 | PASS |

**结论**：boundary[0] 两子例（缺失+为空）现各有独立用例并经变异确认。8 条 DoD 全 PASS。TASK-007 verified——渲染层（T1–T7）全部收官，T9 终局切换可放行。
