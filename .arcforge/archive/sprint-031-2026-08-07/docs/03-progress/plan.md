# Sprint 032 进度 — collector policy gate

**最后更新**：2026-08-06 18:0x（全体 teammate 触及 session limit，重置 17:50 Asia/Shanghai）

## 全局

**19 个任务：15 verified / 2 verifying / 1 in_progress / 1 assigned**

- HEAD = `c638833`，**代码全部已提交**，工作区无未提交的生产代码改动
- **全量 `go test ./...`：62 个包全绿、0 FAIL**（Leader 于 teammate 下线后现跑核实）
- `go build ./...` 通过

## 在途四条线（恢复顺序）

| 任务 | 状态 | owner / verifier | epoch | 阻塞点 |
|---|---|---|---|---|
| TASK-018 | `in_progress` | dev-agent-38 | 2 | **⚠ 最优先**：实现全完成、代码已提交（`253c1bc`+`4c92611`）、23 变异跑完，只差它自己 transition `dev_done`。**Leader 无权代转**（A6 死锁教训：执行迁移边不含 leader）。恢复时告知「yahoo scope 冲突已解，可转」 |
| TASK-019 | `assigned` | dev-agent-36 | **2** | 须用 `--expect-epoch 2` 重新认领（曾认领后被 Leader 收回改 DoD） |
| TASK-007 | `verifying` | v=test-agent-16 | 1 | 验证者重新开工，交付 `415f900` |
| TASK-016 | `verifying` | v=test-agent-17 | 3 | 验证者重新开工，交付 `c638833` |

## 已完成的 15 个

001-006（policy 包五件套 + 配置接线）、008（twelvedata）、009（tushare）、010（lixinger）、011（serve AST 装配守护）、012（删 CachedCollector）、013（AST 防回潮）、014（文档）、015（eastmoney）、017（baostock）

## 返工统计

| 任务 | rework | 原因类别 |
|---|---|---|
| TASK-001 | 2 | task_defect ×2 |
| 002/003/004 | 1 | task_defect |
| TASK-007 | 1 | **规格后到**（不计熔断额度） |
| TASK-015 | 1 | task_defect ×1 + **规格后到 ×1**（后者不计） |
| TASK-016 | 1 | task_defect（复合句第二分句无守护） |
| TASK-017 | 1 | **规格后到**（不计） |

**计数口径（本 Sprint 确立）**：规格后到不计入 `max_rework` 熔断额度，规格一直在则计。理由：熔断的立论是「反复返工大概率是 criteria 本身不可实现」，而后加条款产生的返工不携带这个信号。

## 一个贯穿整个 Sprint 的缺陷模式

**缓存键时间精度**——四次分别发现，其中第四次（yahoo）时该任务**已经通过验收**：

| 时机 | 包 | 发现者 |
|---|---|---|
| dev 仍 `in_progress` | crypto | qa-agent-8 点状检查 |
| 派验时补 criteria | baostock | test-agent-16 |
| 收到通知后 grep | eastmoney | dev-agent-35 |
| **同一次 grep 顺带** | **yahoo（已 verified）** | dev-agent-35 |

qa-agent-8 随后扫完剩余两家（twelvedata/lixinger 用 `Format("2006-01-02")`，按天，天然不受影响），**范围已封闭**。

代价：三轮 reject + 一次 review_fix。若首次发现时就扫全仓，四家可一次补齐。

## 剩余步骤

1. 四条在途线收口
2. Step 6 QA 第二轮终审（焦点：八家一致性、错误映射「两种模式各有依据」）
3. Step 7 `final-report.md` + `changelog.md`
4. 全部转 `accepted` → `/arcforge-archive`
