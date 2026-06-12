# 需求 ↔ DoD 追溯矩阵 — sprint-003

**生成**: 2026-06-12 | **需求源**: plan（9 Task）→ 8 arcforge 任务

## 正向：需求 → 任务

| 需求 | plan | 任务 | 覆盖要点 |
|------|------|------|----------|
| R1 引擎盖戳+计数 | T1 | TASK-001 | stampStub 覆写断言 / SkippedBars 计数 / 零回归 |
| R2 墙钟修复 | T2 | TASK-002 | ctx.Now 断言 / 两处修复 / 零回归 |
| R3 export-signals | T3+T4 | TASK-003 | golden CSV 逐字节 / 动态白名单 / warm-up 过滤 / cobra+Makefile / 错误路径 |
| R4 脚手架+符号 | T5 | TASK-004 | 三正例五拒绝 / 不依赖 qlib / README 骨架 |
| R5 入场对齐 | T6 | TASK-005 | 严格次日开盘 / 顺延上界 / 尾部 None / lazy import |
| R6 事件研究 | T7 | TASK-006 | 手工验算收益+超额 / sell 规避 / 聚合+胜率口径 / 分桶 / 越界 NA |
| R7 报告+CLI | T8 | TASK-007 | 严格 schema+行号 / 报告三节 / 缺数据 exit(1)+指引 / 非 A 股缺口 |
| R8 端到端 | T9 | TASK-008 | signal-eval target / 双语言全绿 / 缺数据指引路径 / README 口径 |

## 反向

8 任务全部回溯 R1-R8，无凭空 DoD。plan T9 的 code-simplifier（Dev 流程）与 gitnexus（QA prompt）由流程承接。

## plan 验收对照 → 承接

| plan 验收项 | 承接 |
|-------------|------|
| 引擎盖戳（故意写错 stub） | TASK-001 functional[0] |
| 白名单 pe_band 显式报错+清单 | TASK-003 functional[1]（动态判定） |
| warm-up：from 后首日即有信号 | TASK-003 golden（from 过滤边界） |
| 事件研究口径全合成单测 | TASK-005/006 全部 functional |
| pytest 不依赖 qlib/数据包 | TASK-004/005/006/007 non_functional |
| make signal-eval 产出报告 | TASK-008（数据缺失降级口径 ADR-S3-4） |

## 机器检查结论

- 孤儿需求/凭空 DoD：无。verify_by：test 24 / review 3（README 类）/ manual 0。
- 跨语言注意：Python 任务（004-008）走 hook pytest 门禁（无覆盖率百分比），DoD 矩阵核对补偿——reviewer 重点核此口径是否够严。
- **独立 reviewer 反审（2026-06-12）**: 初判 NEEDS_REVISION，5 必改 + 1 顺手全部采纳：H1 TASK-003 全量注册 5 策略（CLI 拒绝路径可达）；H2 TASK-004 补 conftest.py + DoD 验证命令与 hook 同款（从仓库根执行，否则全部 Python 任务门禁阻断——跨语言新风险）；H3 TASK-005 边界双侧自包含（== 保留 / > 丢弃，钉死比较符）；M1 TASK-006 补基准最近前值对齐 + 负索引防护；M2 三桶累积 n=3/2/1；L1 TASK-008 补 vet。修订后判定 PASS。
