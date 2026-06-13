# 需求 ↔ DoD 双向追溯矩阵

来源：`docs/superpowers/plans/2026-06-13-lixinger-collector-rewrite.md`（自查覆盖节）

## 正向：每条需求 → 覆盖任务

| # | 需求（根因/能力） | 覆盖任务 | DoD 锚点 |
|---|---|---|---|
| R1 | 响应信封 code:1=成功（修反转判定） | TASK-001 | functional[0..1] parseEnvelope |
| R2 | 退避重试 1/2/4/8/16s + 4xx 不重试 + 开关 | TASK-001 / TASK-006 | TASK-001 boundary/error + TASK-006 functional |
| R3 | 删除遗留反转语义测试文件 | TASK-002 | non_functional |
| R4 | FetchValuationPercentile 扁平 key + 个股 | TASK-002 | functional[0] |
| R5 | 指数估值 .mcw 权重段 | TASK-002 | functional[1] |
| R6 | FetchHistory candlestick/单数/type/RFC3339 | TASK-003 | functional[1] + 既有 history_test |
| R7 | FetchQuote candlestick 近似/delayed | TASK-003 | functional[0] |
| R8 | FetchFundamental metricsList/dyr/mc/ROE 留零 | TASK-004 | functional[0..2] |
| R9 | 基金 net-value/profile/manager/drawdown 聚合 | TASK-005 | functional[0..2] |
| R10 | 基金子接口部分失败降级 | TASK-005 | boundary[0] |
| R11 | 配置开关接线 | TASK-006 | functional[0] |
| R12 | 旧 postJSON/postJSONRaw/digFloat/旧端点移除 | TASK-002/003/004/005 | 各 non_functional |
| R13 | core 类型不改动（ROE 仅置零） | TASK-004 | functional[2] + non_functional |
| R14 | 全量构建/测试/vet 全绿 + 覆盖率 | TASK-007 | functional + non_functional |

→ 无孤儿需求（R1-R14 均有任务覆盖）。

## 反向：每个任务的 DoD → 对应需求

| 任务 | DoD 是否全部映射到需求 | 凭空 DoD？ |
|---|---|---|
| TASK-001 | R1,R2 + 团队覆盖率规范 | 无 |
| TASK-002 | R3,R4,R5,R12 | 无 |
| TASK-003 | R6,R7,R12 | 无 |
| TASK-004 | R8,R12,R13 | 无 |
| TASK-005 | R9,R10,R12 | 无 |
| TASK-006 | R2,R11 | 无 |
| TASK-007 | R14 | 无 |

→ 无凭空 DoD（覆盖率/编译/vet 类条目来源为团队规范 coverage/tdd，已注明 verify_by）。

## 机器检查结论
孤儿需求：0；凭空 DoD：0。矩阵闭合。
