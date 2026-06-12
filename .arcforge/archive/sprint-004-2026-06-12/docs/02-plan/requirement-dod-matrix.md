# 需求 ↔ DoD 追溯矩阵 — sprint-004

**生成**: 2026-06-12 | **需求源**: plan rev4（4 Task）→ 4 arcforge 任务（1:1 映射，仅调度重排）

## 正向

| 需求 | 任务 | 覆盖要点 |
|------|------|----------|
| R1 核心导出 | TASK-001 | 契约（含文件名派生）/golden 互锁/resolver 空集报错/非 A 股拒绝/基准 fatal/300ms 注入 |
| R2 接线 | TASK-002 | UsageListsAllFlags/CLI 层基准校验/Makefile 四断言（--symbols+--from+无 --to+build_data）/_target_block 泛化 |
| R3 构建编排 | TASK-003 | --data_path 钉死断言/--exclude_fields/日期推导/tab 三字段只读校验/空 CSV 拒绝/失败透传 |
| R4 收口 e2e | TASK-004 | QLIB_DIR 切换/README 四要素/e2e 三连（建包→数值抽查→非空结果表，必跑） |

## 反向

4 任务全部回溯 R1-R4，无凭空 DoD。plan 验收对照 7 条 → 承接：契约（001）、符号语义三类（001+002）、--exclude_fields（003）、数值抽查（004）、非空结果表（004）、双语言回归（004 boundary）、qlib-data 产出（004）。

## 机器检查结论

- 孤儿需求/凭空 DoD：无。verify_by：test 16 / review 1（README）/ manual 0。
- 评审遗产：plan 本身经 3 轮评审（2 BLOCKER+4 MAJOR+9 项修订），DoD 直接引用钉死结论（--data_path、makeBars 不可用、--symbols 必传等以加粗写入任务 description 防 Dev 踩回原坑）。
- **独立 reviewer 反审（2026-06-12）**: 初判 NEEDS_REVISION（轻量）。13 条钉死结论全部承接✓、验收对照 7 条承接✓、依赖图三项核查（001∥003 零交集、002 双 scope 与 hook 2e 分流兼容、e2e 必跑依据）全部成立✓。必改 2 项已采纳：M-1 TASK-001 补「非基准 A 股失败降级」用例（防全 fatal 错误实现穿过验收）；M-2 TASK-004 README 补至五要素（含社区包对比）。修订后判定 PASS。
