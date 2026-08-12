# Changelog — Sprint 036（Hestia M1b-4a）

**范围**：`internal/hestia` · **基线** `f5a17d5`（92.1%）→ **`c101d61`**（93.2%）

## 新增

- **`discover.go`** — 从 PBOC 发布页发现未抓取的报告期次
  - `parsePaging` 解析 `jumpTo(this,'N','M','tmpl')` 分页控件，`pageURL` 按模板拼页（第 1 页不套模板）
  - `parsePeriod` 把标题映射成 `(period, periodType)`：`上半年→h1`、`N月→monthly`、无期次段`→annual`
  - `scanPage` 从 index 页提取 `Candidate{ArticleID, URL, Title, Period, PeriodType}`
  - `Discover` 主循环：翻页直到命中已入库期次或翻满 `MaxPages`（空库首跑翻满，spec §4.3）
  - 六个函数全部 **100%** 语句覆盖
- **`config.go`** — `LoadConfig` 从 YAML 装载配置
  - **未写的阈值保持 `DefaultThresholds()` 的值，不退化为零值**（本任务最要紧的一条）
  - `Config.validate()` 五道防线；`LoadConfig` 与 `validate` 均 **100%**
- **`Store.HasPeriod`** 与 `PeriodChecker` 接口 — 查 `v_hestia_current` 而**不**查 `hestia_pending`（pending 期次允许重试）
- **`Fetcher`** 接口与绕代理的 PBOC client（非 200 带状态码报错、10MB 上限、`NewRequestWithContext`）
- **`CaliberExemption.PeriodTypes`** — 口径豁免的键补上期次类型（破坏性变更：`exemptionFor` 改为三参数）

## 修复

- **`Store.HasPeriod` 的 `go doc` 缺陷** — 文档注释与 `func` 之间的空行导致 `go doc` 只输出签名，
  而 `gofmt` **与** `go vet` **双双不响**。修后 `go doc` 输出完整 20 行注释。
  ⚠️ **修复本身尚无守卫**，已列入 M1b-4b。
- **存量 F8**（跨 Sprint）— `require.NotErrorIs(t, errors.Unwrap(err), err)` **平凡为真**
  （unwrapped 为 nil 时 `errors.Is(nil, x)` 恒 false）。改为 `require.NotNil(t, errors.Unwrap(err))`，
  两处；原写法作为反面教材留在注释里。
- **跨 Sprint 任务编号歧义** — `thresholds.go` 的「故留到 T7」是 M1b-3 的已兑现记录，
  改为「M1b-3 的 TASK-007（已兑现，见 `:141`）」，并在 CONTRACTS 立约定：
  **注释引用任务编号一律带 milestone 前缀**。

## 契约

`internal/hestia/CONTRACTS.md` 新增 Sprint 036 一节（五项）：对方案报告 4.1 的三处实测修正、
discover 的三条判据、「pending 不可见」在两处含义相反、**留给 M1b-4b 的一个张力**、milestone 前缀约定。
取证归属标注 ✅亲验 / 📋转述。

## 已知缺口（人类已定案，见 final-report 第三节）

| 缺口 | 影响 | 决定 |
|---|---|---|
| **季报（一季度/前三季度）两侧文法都不认** | 每年 **2/12 篇静默消失，零报错** | **4b 之前补上支持**（需动四处，新开任务） |
| **月报 discover 收下但 `Parse` 恒拒** | 每年 **8/12 篇永久重试循环** | 4b 跳过并记日志、不中止本轮 + 重试记账 |
| **修订版结构上不可达** | Store 侧双时态设计当前无生产者 | 暂不支持，写进契约 |

⚠️ **这三条都不会让任何测试变红** —— 仓库里两份 index 快照恰好只含 h1 一种形态。

## 质量

```
271 顶层测试 / 577 含子测试 / 0 FAIL      -race 绿
93 条变异 / 90 KILLED / 3 SURVIVED（三条存活均经独立验证）
gofmt 0 项    go vet 0 行    零返工 / 零 rejected / 零 blocked
```
