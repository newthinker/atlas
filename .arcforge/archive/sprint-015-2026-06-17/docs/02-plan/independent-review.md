# 独立验收评审 — digest PE% 列（只读需求，未看 DoD）

## 设计经源码核实成立
- `pe_percentile_display` 与 router 解耦：percentileOf 仅读 `percentile`/`pe_percentile`，effectiveStep 仅读 `percentile_step`；Go map 精确匹配无前缀误读 → 门控零影响。
- `Fundamental.PEPercentile` 哨兵 `-1`=不可用，值域 0–100；`>=0` 判断正确（排除 -1、保留合法 0.0）。
- 既有 digest 测试全用 Contains/围栏计数 → 加列不破。

## 已采纳并补入 DoD 的测试缺口
| # | 缺口 | 处置 |
|---|------|------|
| B4 | PEPercentile==0.0 是合法值(历史最低 PE，有价值买点)，必须显示 0.0%；`>0` 误写会静默吞 | TASK-001 + TASK-002 boundary 各加 0.0 用例 |
| B6 | 去掉 name=='' 提前返回后，唯一变行为的路径是「name 空+PE 有」；断言须用 `_,ok:=Metadata["name"];!ok` | TASK-002 boundary 加用例并指定断言方式 |
| F5 | 仅测 renderTable 单组，未测 formatBatch 分组+排序后 PE 列 | TASK-001 functional 加端到端用例 |
| N1 | router 零影响应是对照断言(带/不带键 Route 一致)，非仅 grep | TASK-002 non_functional 写明对照回归 |

## 可测试性提示（转人工/集成）
- CJK 等宽对齐视觉正确性 CI 不可断言 → 交付附真实 digest 截图（注意 PRICE 有值/PE 空导致 PRICE 变中间列被补空格、末列 PE 空的场景）。
- PEPercentile=0.0 真值需手工拉一个历史最低估值标的端到端验证；单测仅注入构造值证明判断逻辑。
- 新增测试**勿用逐行 `\s+$` 精确断言**（PRICE 有值/PE 空时末列空，行尾会有 PRICE 补空格分隔符，属正常）。
