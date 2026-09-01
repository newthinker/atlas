# Changelog — M1c-3b（sprint-041）

**范围** `32bc1e5..70eedc4`（32 提交，24 文件 +4030 −99）  **交付于** `70eedc4caf3502df9f25f125c7dcc546d06320d3`

## 新增能力

- **`atlas hestia backfill load`**：解析 218 篇央行文章 → 按 `(period, period_type, published_at)` 合并 → 过闸 → 写入新建的权威库，报告到 stdout。
- **合并观测 `merged@v1`**：字段取并集、冲突不静默；必填集按参与合并的 extractor 动态取并集（`mergedRequiredFields`）。
- **人类可核对的核对报告**：四道恒等式、extractor 分组、字段数分布、`PartialCoverage` 逐条判因。

## 真跑结果（语料 `data/hestia-backfill-2026-08-14`）

```
218 篇 → 199 可解析 → 96 观测（54 单篇 + 42 合并组）
入权威表 79 / 落 pending 17
```

## QA REJECT 后的修复（4 条 CRITICAL）

| # | 缺陷 | 修法 |
|---|---|---|
| C-1 | `stock_continuity` 拿「最近已接受的一期」冒充「相邻上一期」，错扣 4 个有效期次且构成正反馈 | 不相邻时跳过（不放宽——放宽需要一个没有测量支撑的系数）。权威表 75→79 |
| C-2 | 恒等式失败时报告整份不输出 ⇒ 打印标题原文那段生产路径不可达 | 拆出 `renderLoadReport`，先渲染再校验 |
| C-3 | 恒等式一二的加数在建库前已定，却排在 `NewStore` + 96 次 `Save` 之后才校验 | 拆出 `checkInputIdentities` 在 `NewStore` 之前调用 |
| C-4 | 恒等式三恒真（`Merged=len(groups)`，两计数器必然穷尽划分） | 换成与库里 `merged@v1` 行数的**跨两表异源**比对 |

⚠️ **C-2 与 C-3 本身冲突**（一个要提前失败别产生副作用，一个要失败时也印报告，而早退路径上没有 `writeLoadReport` 可用）—— 拆出纯渲染层才让两者共存。

## 一个未被任何验收判据发现的事实

C-1 影响的不止被拒的 4 期：重放全部 96 组，`non_adjacent_prior` 共 **20 条** —— 4 条伪环比超限被拒（可见），**另 16 条碰巧没超限、静默通过**（不可见）。**假通过是假拒绝的 4 倍。**

## 不变量

- 生产库 `/Users/zuowei/workspace/runtime/atlas/data/hestia.db` **全程未触碰**，sha256 `478d40c0…c28c` 与 sprint 起始逐字符相等。
- `configs/hestia.yaml` 的开关未自动切换（人类动作）。
- 运行时资产（`.claude/hooks/`、`.claude/scripts/`、`settings*.json`）未改动。
