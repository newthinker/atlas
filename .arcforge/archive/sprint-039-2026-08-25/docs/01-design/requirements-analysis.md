# M1c-2 离线标定 · 需求分析

来源：`hestia/docs/superpowers/plans/2026-08-24-hestia-calibrate.md`（1445 行）
分析于 master `fba0feb`（Sprint 038 归档提交）。

## 一、目标

**用 M1c-1 抓到的快照产出字段分布报告，供人工填 `MagnitudeRanges`；同时把 `StockContinuityMax` 改成按 `period_type` 分档。**

⚠️ **工具只产出依据，不改 `configs/hestia.yaml`** —— 区间由人填。这条是硬约束：
标定的输出是**证据**不是**决定**。

## 二、🔴 计划与现实的三处偏差（Leader 核实，非转述）

### 偏差 1：前置假设已过期 —— 产物已存在，且**不应重跑**（人类已确认「复用不重跑」）

计划写「Sprint 038 交付的是工具，`runtime/atlas/data/` 下没有 backfill 目录，**T2 开工前必须先真跑一次**」。

**核实**：

```
/Users/zuowei/workspace/runtime/atlas/data/backfill   ✗ 不存在（计划指定的路径）
~/hestia-backfill-2026-08-14/                          ✓ 452 文件
atlas/data/hestia-backfill-2026-08-14/                 ✓ 452 文件（备份，manifest sha256 与原始逐字相同）
```

**不重跑的理由不是省时间**：

1. 站点在漂 —— Sprint 038 实测**两天内 p146 掉了 2 条**（3 篇变 1 篇，另两篇下移到 p147）⇒ **重抓拿不到同一份快照**；
2. 现有产物**经 QA 全量复核**：218/218 sha256 逐篇一致、与 151 页 index + 82 页 search 快照**双向闭合**、`manifest.title` 与文件内 `<title>` 218/218 逐字相同、两个差集为 0 且**非空集平凡为真**；
3. 重跑会让标定结果与**那份已验证的语料脱钩** —— 而标定产出的是要填进生产配置的区间。

⇒ **`--dir` 指向 `data/hestia-backfill-2026-08-14`。**

### 偏差 2：🔴 `completed_at` 缺失，而 `--allow-incomplete` 是**语义错配**

Global Constraints：「**`CompletedAt` 缺失 ⇒ 报错**，除非显式 `--allow-incomplete`」。

**核实** —— 那份 manifest 的顶层字段：

```
articles  failed  from  only_in_index  only_in_search  pages_scanned  scanned_at  search_pages_scanned
```

**没有 `completed_at`。** 但它**不是夭折的回填** —— 它是完整的（QA 全量复核合格），只是**早于 Sprint 038 的 TASK-010 引入那个字段**，`CONTRACTS.md` 已注明「字段缺失 ⇒ 出自 TASK-010 之前，cutover 按 2025-09 理解」。

⚠️ **所以带 `--allow-incomplete` 跑，会把一份完整产物标记成「可能不完整」。**
`--allow-incomplete` 的语义是「我知道这趟回填夭折了，仍要标定」，而这里的事实是
「这趟回填是完整的，只是产出时还没有这个字段」。**两者在命令行上无法区分。**

🔴 **我最初拟的处置方案 (a) 是错的，被上个 sprint 自己的注释推翻** —— 记在这里因为它是本迭代第一个「看起来正常的错误」：

我拟的 (a) 是「若 manifest 含 `scanned_at` 且 `failed` 为空 ⇒ 认定完整、放行」。而 `CompletedAt` 的注释（TASK-010 写的）明说：

> 进程在第 218 篇被杀，与跑完 400 篇正常退出，产出的 manifest **结构上无法区分** ——
> 两者都是合法 JSON、sha256 全对、`articles[]` 与磁盘完全闭合。
> **下游做的一切闭合性检查在夭折的产物上同样全绿。**

⇒ **(a) 正是被这段话点名无效的那类检查。** 我用「产物内部的自洽」去证明「这趟跑完了」，
而那个字段存在的全部理由就是**自洽证明不了这件事**。

### 定案：用 `--allow-incomplete`，但**证据与理由必须进报告**

先厘清语义 —— 计划在两处用了**不同的**说法：

| 出处 | 语义 |
|---|---|
| Global Constraints | 「`CompletedAt` 缺失 ⇒ 报错，除非 `--allow-incomplete`」= **允许缺标记** |
| T4 测试注释 | 「默认必须拒绝**夭折的** manifest」= **允许夭折** |

**我们的产物落在两者之间：缺标记，但不夭折。** flag 的机制语义（缺标记）覆盖我们的情况，
所以用它是对的；而「夭折」那层含义会误导读报告的人。

⇒ **T4 的 DoD 须包含**：`--allow-incomplete` 生效时，报告里必须打印**为什么允许**，
且该说明能容纳「缺标记但有外部完整性证据」这一情形。本次的外部证据是：

```
真跑 exit 0、8m24s、451 次请求（= 151 index + 82 search + 218 articles）
reconcile：80 期 / 218 篇 / 0 缺篇 / 0 未归类
两个差集均为 0 且非空集平凡为真（218 条全部 source=both）
```

⚠️ **注意这三条里只有第一条是有效证据** —— 后两条是「产物内部自洽」，按上面那段注释，
它们**在夭折的产物上同样成立**。**唯一能证明「这趟跑完了」的是进程正常退出这个进程外事实**，
而它记在 Sprint 038 的 discovery 与真跑日志里（已归档，仍可读），**不在产物里**。

⇒ **这恰好证明 `CompletedAt` 的设计是对的**：外部证据不随产物走，下一个拿到这份目录的人看不到它。

### 偏差 3：`StockContinuityMax` 引用点是 **13 处**不是 9 处，计划漏了一条会过期的注释

计划列 9 处并提醒 `ingest_test.go:485` 那条注释会过期。实际 grep 出 13 处，**漏的那条是**：

```go
// validate.go:343
// ⚠️ StockContinuityMax 的 2% 未经真实数据验证——M0 的两份样本里只有一份
```

改成分档后，「2%」这个说法与「未经验证」这个状态**都会变**（本迭代的产出正是「用真实数据验证」）。

⚠️ 这是 Sprint 038 抓了三次的形状：**注释描述的是当时的意图，代码改了它不会跟着改**。

## 三、本迭代的主要风险（计划自己点名，我确认）

🔴 **旧格式 YAML 会静默停用整道闸门。** `stock_continuity_max` 从标量改 map 后，
旧格式（`stock_continuity_max: 0.02`）**不报错** —— viper 把标量塞进 map 得到**空 map**
⇒ 每期 `skipped{no_threshold}`、`Passed` 仍 true、**数据照常入库**。**编译期抓不到**。

⇒ 这与 Sprint 038 的主线**同形**：一道闸在场、但不设防，而且**沉默**。
计划的处置（`Config.validate()` 拒绝空 map，错误信息点名 `scalar`，
**测试 `want` 用 `"scalar"` 而非字段名**——字段名在「旧格式」与「缺键」两种错误里都出现、区分不开）
是对的，DoD 须保留这个精度。

## 四、四个任务与依赖

```
T1（StockContinuityMax 分档）   不依赖产物，可先做
T2（批量解析）→ T3（分布报告）→ T4（cobra + CONTRACTS + 真跑验收）
```

| # | 交付 | 关键约束 |
|---|---|---|
| T1 | `thresholds.go` / `config.go` / `validate.go` / `configs/hestia.yaml` + 5 个测试文件 | 两档取值（monthly 0.02、其余四种 0.15）；**装载时五种齐全、运行时容忍缺键** |
| T2 | `calibrate.go`（新建） | `--dir` 而非 `--manifest`；**社融两篇不计入失败表**（守卫构造刻意：测试里那两篇文件根本不存在） |
| T3 | `calibrate_report.go`（新建，**纯函数**） | 分位数 nearest-rank 不插值；**建议区间用加性余量**（乘性在负值字段上会把实测最小值排除在外）；**`n < 3` 不给建议** |
| T4 | `cmd/atlas/hestia.go` + CONTRACTS + 真跑 | 导出入口 `Calibrate`，形态与 `BackfillFetch` 一致 |

## 五、两条计划里最值得保留的判断

1. **`n` 列不可省** —— 27 个非社融字段有 78 期样本，27 个社融字段只有 10 期。
   **「这个差异不写在脸上，填表的人会用同样的信心对待两种区间。」**
2. **`n < 3` 不给建议** —— **「过窄的建议比没有建议更危险：它看起来是个结论。」**

⇒ 两条都是「输出会被当成结论」这一族，与 Sprint 038 的 registry D 组同源。
