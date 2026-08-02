# Sprint-030 进度(M3.5a 数据源与降级链)

> 真相源 = `.arcforge/tasks/*.json`;本文件为 Leader 汇总视图。
> QA verdict: REJECT(2 CRITICAL + 5 MAJOR)→ review_fix 收敛中 | max_rework=3

## 当前状态(2026-08-02 08:00)

| 任务 | 状态 | rw | 负责 | 本轮内容 |
|------|------|----|------|---------|
| TASK-001 | in_progress | 2 | dev-agent-32 | S2 固化「窗口无关」用例(复验唯一存活突变) |
| TASK-002 | **verified** | 1 | dev-agent-33 | C1 apikey 外泄已修(四重变异+结构性核验) |
| TASK-003 | **verified** | 0 | dev-agent-34 | — |
| TASK-004 | **verified** | 0 | dev-agent-33 | — |
| TASK-005 | in_progress | 1 | dev-agent-32 | C2/M1/M2 已完成;M3b + ErrRateLimited 消费点收尾 |
| TASK-006 | **verified** | 1 | dev-agent-34 | D1-D4 已过;D5-D8 待 T005 验收后一次性派(不单开轮) |

## 待办队列

1. TASK-005 dev_done → test-agent-14 复验(C2 是本 Sprint 最严重数据缺陷,复验要重)
2. TASK-001 S2 dev_done → test-agent-14 复验
3. 两者 verified 后 → TASK-006 转 review_fix 派 D5-D8(港股标注/§9 措辞校准/行号引用改函数名/交易日数校准)
4. 全部 verified → 提交返工产物 → qa-agent-7 定向复审(只看 fix 项)→ Step 7 交付归档

## final-report 必录项

- 跨模型对抗轮未执行(codex usage limit 至 2026-08-20)→ 退回纯 Claude 三 lens,覆盖削弱项。
- DoD 修订:TASK-002 functional[0] 的 apikey 从 query 改 Authorization 头(安全优先,写通道不允许 dev_done 后改 done_criteria,故以派验消息+本报告为准)。
- 三例流程摩擦及对策(门禁 OTHERS 集不含 verified / 变异派生目录 / 追加需求与 owner 移交时序)——已记 wisdom,机制侧对策需人类会话外落地。
- 遗留未实证:baostock 真桥取数(上游 10030 不可达)、港股 hk_daily 取数(4/5 位形态未归一 + 限频)、A 股经 tushare 恢复数据(限频容量)。
- 后续任务(仅一个):限频感知退避,含 ErrRateLimited 退避消费 + A 股批量断源容量 + 路由层跳序与市场过滤 + 港股 symbol 归一(届时修订 ADR#8)。
