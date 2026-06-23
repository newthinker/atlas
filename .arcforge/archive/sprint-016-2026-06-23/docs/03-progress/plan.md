# IC/IR Eval Sprint — 进度

## 状态：已交付验收（全部 7 任务 accepted，2026-06-23）

QA verdict PASS；一轮 review_fix 折入 W1(report NaN 守护)+W3(makefile 测试)，W2(重复键去重)推迟到 sidecar 集成。
最终回归 117 passed。验收报告见 06-acceptance/final-report.md。剩余：等用户决定 /arcforge-archive 与是否建 PR。


| Task | commit | status |
|---|---|---|
| T1 | 2ae38c7 | verified |
| T2 | a9efe85 | verified |
| T3 | a3e5269 | verified |
| T5 | d43354a | verified |
| T4 | 013c588 | verified |
| T6 | 99ad940 | verified |
| T7 | 242d05c | verified |

全量回归：113 passed, 1 warning（预期 ConstantInputWarning）。手工 transition-audit：全部状态由 Leader 落盘，dev/test 无 .arcforge/ 写权限，无越权/绕过写入。


人类选择「补强测试，不改实现」：reviewer 的 gaps 1/2/3/5 + 守门覆盖 + 坏行行号/空文件 + Pearson≠Spearman
已折进 T1/T2/T4/T5/T6 的 DoD（全部为补测试，实现按计划不变；重复 (date,symbol) 仅文档标注为已知约束）。

执行模型（degraded，write-hook 缺失）：Leader 同步 dispatch dev-agent（per-DAG）→ test-agent 验证 →
Leader 独占 .arcforge/ 状态写入与 discovery 落盘；dev/test 不写 .arcforge/。

## 降级提示
- ⚠️ validator 缺失 → Leader 手工校验任务图（见下「手工 validator 校验」）。
- ⚠️ write-hook 缺失 → 以 with-task-lock.sh 原子写 task JSON。
- ⚠️ config.language=go vs 实际 Python → 验证用 pytest（计划指定命令），非 Go coverage hook。

## 任务图
| Task | 标题 | wave | deps | status | scope（packages） |
|---|---|---|---|---|---|
| T1 | forward_returns (ic.py) | 1 | — | pending | ic.py, test_ic.py |
| T2 | instrument_ic (ic.py) | 2 | T1 | pending | ic.py, test_ic.py |
| T3 | summaries (ic.py) | 3 | T2 | pending | ic.py, test_ic.py |
| T5 | baseline.py | 3 | T2 | pending | baseline.py, test_baseline.py |
| T4 | report.py | 4 | T3 | pending | report.py, test_ic_report.py |
| T6 | ic_evaluate.py CLI | 5 | T4,T5 | pending | ic_evaluate.py, test_ic_report.py |
| T7 | Makefile + runbook | 6 | T6 | pending | Makefile, runbook.md |

## 手工 validator 校验（degraded）
- DAG 无环：T1→T2→{T3,T5}；T3→T4→T6→T7；T5→T6。✓ 无环。
- wave 序：每个 task.wave > max(deps.wave)。T6 deps{T4=4,T5=3}→wave5>4 ✓。全部满足。✓
- scope 互斥（在途）：唯一并行点 T3∥T5 → {ic.py,test_ic.py} vs {baseline.py,test_baseline.py} 不相交 ✓。
  T4↔T6 共享 test_ic_report.py 但 T6 deps T4，绝不同时在途 ✓。
- context_from 闭合：所有引用的上游 task 均存在 ✓。
- 单 owner / epoch：初始 epoch=0，待派发。

## 调度计划（dag）
- 就绪即派：T1 先行 → verified 后 T2 → verified 后 T3 与 T5 并行（2 dev）→ T4 → T6 → T7。
- 团队：dev-agent-1 / dev-agent-2 + test-agent-1。

## 进度日志
- 2026-06-22 需求分析 + 任务拆分 + DoD 完成；追溯矩阵双向闭合；手工 validator 通过。等待独立 reviewer + 人类确认。
