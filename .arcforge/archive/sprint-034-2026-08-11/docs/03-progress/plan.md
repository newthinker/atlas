# Sprint 034 进度 — hestia M1b-2（解析层）

> 真相源是 `.arcforge/tasks/*.json`；本文件是 Leader 维护的可读视图，
> 与任务文件冲突时**以任务文件为准**。

## 当前状态（2026-08-10 08:45Z）

| 任务 | 标题 | 状态 | owner | verifier |
| --- | --- | --- | --- | --- |
| TASK-001 | 样本装载与 golden 基线 | `verified` | dev-agent-43 | test-agent-22 |
| TASK-002 | HTML strip 与文本规整 | `verified` | dev-agent-44 | test-agent-23 |
| TASK-003 | 板块切分与 extractor 探测 | `verified` | dev-agent-43 | test-agent-22 |
| TASK-004 | amount / ratio 数值原语 | `verified` | dev-agent-45 | test-agent-23 |
| TASK-005 | 模板表与字段抽取 | `in_progress` | dev-agent-45 | — |
| TASK-006 | Parse 组装、期次与口径推导 | `pending` | — | — |
| TASK-007 | 端到端与交接登记 | `pending` | — | — |

validator：exit=0，零告警。

## 本轮关键事件

### TASK-003 验证卡 4h51m —— 成因不是中断，是从未开始

test-agent-22 在 03:16:45Z 转完 T1 后按纪律重扫，当时 T3 仍是 `in_progress`、
verifier 为空，两个集合都空，**合规**转入空闲。派发发生在 03:36:40Z，**晚 20 分钟**。

`TeammateIdle` 是转 idle 前的一次性拦截而非心跳 ⇒ 放行即停机。
**它最后一次扫描是干净的，所以自愈在这个时序下结构性无效**——不是漏扫，
是「扫完 → 空闲 → 之后才派发」这个顺序本身没有兜底。

处置：按 validator 的 `stale-dispatch` 处置顺序**先重发消息、未改派**。事后证明必要——
它为 T1 逐行读过同两份 HTML 正文，那正是 T3 切分逻辑的判据，改派会丢掉这份上下文。

### D4 精确化：`scope-writes-outside-packages` 只对在途任务报

dev-agent-43 的预测经**转前记基线 / 转后重跑**双向核验成立：T3 转 `verified` 后
3 条告警全部消失。判别式是**状态**而非形状（T2/T4 声明形状同构但已 verified，无告警）。

⇒ 该告警**不可能靠调整 `packages`/`writes` 消除**，凡在途必报。该改的是规则本身。

### T3 暴露一条悬空承诺，已落到 T6

T3 接受 `splitSections` 无 error 签名，依据是「切分失败由紧邻的 `detectExtractor` 报错」。
test-agent-22 `grep -rn` 全仓实测：**两个函数的生产调用点都是 0**，「紧邻」是对 T6 的
**预期**而非事实 ⇒ T3 `error_handling[0]` 要求的保证**不存在于代码库任何位置**。

已补进 TASK-006 `error_handling[2]`，判据可测。

## Leader 需要记明的两处自己的问题

1. **本文件此前根本不存在** —— Step 3 要求初始化 `03-progress/plan.md`，Sprint 034 漏了，
   直到 T5 派发后才发现。前四个任务的进度全程只存在于 `tasks/*.json` 与消息里。
2. **TASK-005 的 DoD 是 9 条，超 Realistic Scope 上限（8）1 条，我决定不拆。**
   新增的 `non_functional[1]` 经 dev-agent-45 复核为低成本；拆开需重排依赖图并承担
   scope 互斥风险。已约定：若实测牵扯出板块切分重构，立即挪去 TASK-007。

## 下一步

T5 `dev_done` → **直接发消息唤醒 test-agent-22 派验**（不指望 idle hook，理由见上）。
T6 验证同样归 test-agent-22——它持有那条悬空承诺的全部上下文，换人验需重建。
