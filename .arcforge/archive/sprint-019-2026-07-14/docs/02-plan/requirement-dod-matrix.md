# 需求 ↔ DoD 双向追溯矩阵（Sprint 019 · 宏观危机监控）

> 需求源：`docs/plans/2026-07-13-macro-crisis-monitor-impl.md`（R 编号对应 01-design/requirements-analysis.md 的核心功能与 NFR）
> 机器检查目标：无孤儿需求（每个 R 至少一个 DoD 覆盖）、无凭空 DoD（每条 DoD 可回溯到需求文档章节）。

## 正向：需求 → 覆盖任务（DoD 维度）

| R# | 需求 | 需求文档位置 | 覆盖任务（DoD） |
|---|---|---|---|
| R1 | 7 指标采集入库、canonical units | 存储单位约定（行 23–37）、Task 6 | TASK-006 functional 1–4、boundary |
| R2 | FRED API 客户端（重试/缺失值） | Task 2（行 910–1138） | TASK-002 全维度 |
| R3 | yahoo 放行 JPY=X（唯一既有 symbol 改动） | Task 3（行 1139–1200） | TASK-003 functional/boundary + impact 前置（NFR review） |
| R4 | 全阈值进 YAML、typed struct（偏差 2） | Global Constraints、Task 4 | TASK-004 全维度；TASK-009 NFR（代码零阈值字面量） |
| R5 | 7 指标三色规则（双轨、基线锚点） | 设计 §3.1、Task 9 | TASK-009 functional 1–5、boundary |
| R6 | 抑制/防抖/新鲜度（季末、STALE、滞回） | 设计 §3.2、Task 8 | TASK-008 全维度；TASK-011 functional 4（编排应用） |
| R7 | 四态状态机、历史行重建计数、语义补充 | 设计 §3.3、语义补充（行 191–195）、Task 10 | TASK-010 functional 1–5、boundary |
| R8 | 单日评估编排、8 行 Evaluation（偏差 1 ts 列） | Task 11、三处偏差 1 | TASK-011 functional；TASK-001 functional 3（schema） |
| R9 | CLI backfill/eval/status（幂等、齐备门、CSV 导入） | Task 7/12、设计 §4.3 | TASK-007、TASK-012 全维度 |
| R10 | replay 回测共用引擎 + 三段历史验收 | Task 13、设计 §6 | TASK-013 functional + manual |
| R11 | telegram 通知（P0/P1/P2、月报周报、页脚、禁词） | 设计 §3.3/§4.4/§5、Task 14 | TASK-014 全维度 |
| R12 | intraday JPY + 3 个 launchd plist + 部署 | 设计 §4.3、Task 15 | TASK-015 全维度 |
| R13 | sqlite v1.38.2 固定、GOTOOLCHAIN=local | Global Constraints | 各任务 NFR 的测试命令前缀；TASK-001（store 依赖） |
| R14 | 幂等三层（upsert 覆盖/当日评估跳过/intraday 去重） | Global Constraints、设计 §4.3 | TASK-001 boundary 1、TASK-012 functional 2、TASK-015 functional 3 + boundary |
| R15 | 密钥不入库（config.yaml gitignored / env 覆盖） | Global Constraints | TASK-007 error_handling（key 获取路径）；QA 审查项 |
| R16 | 文案禁"必然/一定/即将"、概率表述、边界声明页脚 | Global Constraints、设计 §3.3/§5 | TASK-014 boundary 1、functional 2 |
| R17 | 时间列 TEXT 约定（日期/固定宽度时间戳） | Global Constraints | TASK-001 functional 1（NowStamp）、schema |
| R18 | 每任务 detect_changes + code-simplifier；Task 3 前置 impact | Global Constraints、项目 MUST | 流程性约束（Dev prompt 注入 + QA 核对）；TASK-003 NFR |
| R19 | 范围外五项不得顺手实现 | 范围外（行 4580–4583） | QA 审查清单项（reviewer/QA 核对，非任务 DoD） |

**孤儿需求检查**：R1–R17 均有任务 DoD 覆盖；R18/R19 为流程性/负向约束，由 Dev prompt 与 QA 清单承接（不适合单任务 DoD）。✅ 无孤儿。

## 反向：DoD → 需求（凭空 DoD 检查）

15 个任务全部 DoD 条目均直接摘自实施方案对应 Task 节的测试用例、行为要点或 Global Constraints（各 task JSON 的 description 标注了方案行号区间）；无一条 DoD 缺乏需求出处。✅ 无凭空 DoD。

## verify_by 分布

- `manual` 共 3 条，集中在三个收口任务（TASK-007 一阶段人工验收、TASK-013 三段历史回测、TASK-015 部署试运行）——均为需要真实外部数据/真实部署的验收，符合「不做 fantasy assertion」原则，列入终验收清单。
- `review` 共 2 条（TASK-003 impact 前置、TASK-004 零阈值字面量），QA 第一轮清单核对。
- 其余全部 `test`，Dev 转单测、Test Agent 矩阵核对。
