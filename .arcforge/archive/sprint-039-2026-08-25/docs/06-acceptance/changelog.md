# Changelog · Sprint M1c-2（离线标定）

`fba0feb` → `204a0a8`　|　10 个非合并提交，4 个合并点　|　五个任务全部 `accepted`，`rework_count` 全为 0

## Added

- **`hestia backfill calibrate` 子命令**（`cmd/atlas/hestia.go`）：读 M1c-1 的回填产物，
  产出字段分布报告供人工填 `MagnitudeRanges`。两个 flag：`--dir`（必填）、`--allow-incomplete`。
- **`hestia.Calibrate(d CalibrateDeps) error`** —— 包的第 20 个导出标识符，已在
  `store_test.go` 的全导出面精确相等名单里登记。
  ⚠️ **`Out == nil` 报错而非 `io.Discard`**：一个「把报告写出来」的函数默认丢弃输出，
  是把调用方的疏漏变成了合法配置。同一个 `CalibrateDeps.Out` 字段因此有两套契约
  （`collectSamples` 收 nil 合法、`Calibrate` 收 nil 报错），注释与测试都钉住了这个区别。
- **`collectSamples`**（`internal/hestia/calibrate.go`）：218 篇快照分四格 ——
  可解析 / 本迭代不解析 / 解析失败 / 未归类，**四格计数每次都打（含 0）**。
- **`computeFieldStats` / `renderCalibrateReport`**（`internal/hestia/calibrate_report.go`）：
  分位数用 **nearest-rank 不插值**；建议区间用**加性余量** `[min-span, max+span]`。
- **`CalibrateResult.Records []SampleRecord`**：加性扩展，`Samples` 改由它派生
  （`samplesFromRecords`）—— 同一事实不留两份副本。
- **`internal/hestia/CONTRACTS.md` 新增 Sprint M1c-2 一节**（8 条：A1 A2 B1 B2 C1 C2 C3 D1）。
- **`.arcforge/docs/06-acceptance/calibrate-report.md`**（556 行）—— 本 sprint 唯一的人类可读交付物。

## Changed

- 🔴 **`Thresholds.StockContinuityMax` 从 `float64` 改为 `map[string]float64`**（按 `period_type` 分档）。
  五档：`monthly 0.02` / `q1 · h1 · q1_q3 · annual 各 0.15`。
  - **旧格式标量在 `Unmarshal` 层被大声拒绝**（`expected type 'map[string]float64'`），
    不是静默退化 —— 计划假设的「静默空 map」经实测**不成立**，故未写那段死分支。
  - **真正的静默陷阱是「写了 map 但漏一档」**（viper 的 merge 语义会让漏掉的档保留默认值）
    ⇒ `LoadConfig` 加了五档齐全检查，**报错点名缺哪一档**。
  - 缺档时闸门记 `skipped{no_threshold:<pt>}` 而非 `failed`。
- **`configs/hestia.yaml:68-73`**：`stock_continuity_max` 从一行标量改成一块五键。
- **两条会过期的注释已订正**（`validate.go:343`、`ingest_test.go:485`）。

## Fixed

- **`TestStockContinuityDetectsJump` 的边界格静默退化**：四格原用 `validMeta()`（⇒ `period_type=h1`），
  分档后「恰好在阈值上」那格的阈值从 0.02 变成 0.15，**断言退化成平凡真而无一变红**。
  修法是 `atBoundary` 列 + `require.Equal(DefaultThresholds().StockContinuityMax[tt.periodType], *c.Value)`
  —— **精确相等，不用 `InDelta`**（容差会把边界测没）。
  ⚠️ 验证者实测：该守卫**还顺带兜住了同一张表的 `InDelta` 容差参数**（向下畸变 ×0.999 在
  `tol=1e-2` 且无该守卫时**完全静默**）。

## 🔴 Not Done（本 sprint 的第二个目标未达成）

> **本次标定对 `StockContinuityMax` 按 `period_type` 分档，给不出任何实测证据。**
> **两档取值仍是未经真实数据验证的占位数。**

`tsf_stock` 在本语料只有 4 期样本，且四期的 `period_type` **两两不同**
⇒ 无一档有 ≥2 期相邻样本 ⇒ 环比一节整节 `—`。

⚠️ **按本次语料 M1c-3 也做不到**，除非先支持解析社融存量报告（v1 期次的 `tsf_stock` 在那 138 篇里）。

## 已知问题（详见 `final-report.md`）

**两条 M1c-3 前置**（不是普通结转，缺口与 M1c-3 的输入完全重合）：

1. **R2-01 [HIGH]** 分档的载荷前提「相邻两期一律相隔 12 个月」在本语料上为假
   （`q1` 有 36 个月缺口；Parse 失败另造一个 24 个月缺口）。
   `0.15` 在实测增长率下只容得下 22.63 个月 ⇒ **缺口处误拦**；
   而环比表**不带间隔** ⇒ M1c-3 照 `max+余量` 定阈值会**漏放正常增长 2.6 倍的跳变**。
2. **R2-06 [MEDIUM]** 完整性校验实际只覆盖 **25/218 = 11.5%**，而报告不报这个分母
   ⇒ 读起来像「完整性没问题」，实际 88.5% 从未校验 —— 其中 **138 篇正是 M1c-3 的输入**。

其余 8 条（R1-01…04 / R2-02…05）+ 6 条验证发现，见 `final-report.md` §五。

## 质量数据

| 项 | 值 |
|---|---|
| 两包测试 | PASS **1231** / FAIL 0 |
| 全量 `go test ./...` | 退出码 0，64 个 ok 包 |
| `go vet ./...` | 退出码 0，输出 0 字节 |
| 覆盖率 `internal/hestia` | 94.810% → **95.514%** |
| 覆盖率 `cmd/atlas` | 75.610% → **75.644%** |
| Code Review | 两轮，**综合 PASS**，10 条发现，**无 CRITICAL** |

## 兼容性

- **配置不兼容**：`stock_continuity_max` 必须从标量改成五键 map，
  **旧格式会在 `LoadConfig` 硬失败**（有明确错误信息，不是静默）。
- **未改动 `configs/hestia.yaml` 的 `magnitude_ranges`** —— Global Constraints：区间由人填。
