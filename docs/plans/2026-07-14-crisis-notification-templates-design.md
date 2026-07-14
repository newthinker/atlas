# Cassandra 危机监控通知模板设计

**版本**：v1.0（brainstorming 定稿）
**日期**：2026-07-14
**上游**：`docs/plans/atlas-macro-crisis-monitor-design.md` v0.2 §3.3/§4.4/§5 的通知细化
**下游**：回写 `docs/plans/2026-07-13-macro-crisis-monitor-impl.md` Task 12/14/15

## 1. 需求基线（已确认）

- 主消费场景：telegram 点开细读、作为判断输入——正文结构化优先，首行兼顾推送横幅扫读。
- 信息密度：**异常优先、全量保留**——异常指标带完整读数排前，正常指标仍逐行列出。
- 视觉形式：**emoji 色点 + 自然文本**，走 `SendText` 纯文本路径（无 markdown 解析风险）。
- 月报迷你趋势：**sparkline + 月变化量**并列。
- 解读深度：状态变更附**一句查表语义**（概率化措辞），不给操作建议（上游设计 §5 边界）。
- 家族结构：**方案 B 两级家族**——结构化正文（变更/日报/月报/周报）+ 单行速报（P2 运维、盘中 P0）。

## 2. 消息类型矩阵

| # | 消息 | 家族 | 前缀 + emoji | 触发条件 | 页脚 |
|---|------|------|--------------|----------|------|
| 1 | 状态升级 | 结构化 | `[P0] 🚨`（进 BREWING/CRISIS）/ `[P1] ⚠️`（进 WATCH） | 状态转移且严重度上升 | ✓ |
| 2 | 状态降级 | 结构化 | `[P1] ✅` | 状态转移且严重度下降 | ✓ |
| 3 | 日报 | 结构化 | `[P1] 📍` | BREWING/CRISIS 态无变更日 | ✓ |
| 4 | 月报 | 结构化 | `[P1] 📅` | NORMAL ∧ 当月首个交易日 | ✓ |
| 5 | 周报 | 结构化 | `[P1] 📅` | WATCH ∧ 周一 | ✓ |
| 6 | 运维速报 | 速报 | `[P2] 🔧` | 指标**新进入** STALE 当日（持续 STALE 不重复） | ✗ |
| 7 | 盘中速报 | 速报 | `[P0] 🚨` | BREWING/CRISIS 盘中 JPY wow ≤ 红阈（每日一次） | ✗ |

- 严重度序 `NORMAL < WATCH < BREWING < CRISIS`，升/降级按序数比较；降级也通知（解除信号有价值），仅 P1。
- 页脚（边界声明）只挂结构化家族：速报是事实陈述，不构成风险判断。
- P2 去重：对比前一评估日指标状态，仅"昨日非 STALE、今日 STALE"的指标发一次。

## 3. 首行规范（全家族统一）

```
[前缀] emoji 事件短语 · 关键事实 · MM-DD
```

一行说清"发生了什么 + 多严重 + 哪天"；盘中速报的日期带时分（本地时区）。

## 4. 结构化骨架（五段）

```
首行
（空行）
语义句                    ← 仅状态变更；日报/月报/周报省略
（空行）
异常区：触发共振：/仍异常：/异常指标：   ← 🔴🟡 指标，严重度降序
其余区：其余指标：                    ← 🟢 指标 + ⚪ 指标（末尾）
（空行）
尾注（状态持续 / 较昨日 / 退出进度 / AMBER 计数 / 下一评估）
—
页脚
```

- 异常区标题按方向措辞：升级 = `触发共振：`，降级 = `仍异常：`，日报/周报 = `异常指标：`。
- 无异常指标时省略异常区，其余区标题改为 `7 指标全绿：`。
- **月报特例**：异常区 + 其余区合并为单一**趋势区**（`近 21 个交易日趋势：`，见 §5.4），按 `AllIndicators` 顺序、每行带 sparkline 与月变化，不做异常/正常分区（NORMAL 态下分区意义弱，趋势可比性优先）。

### 4.1 语义句查表

写死为包级常量表（键 = 转移方向），措辞概率化；含天数处用 `%d` 占位、渲染时注入 `state_machine` 配置值（避免 YAML 调参后文案失真）：

| 转移 | 语义句 |
|------|--------|
| →WATCH（升级） | 领先层或多指标共振异常。观察期：提高警觉，尚无行动含义。 |
| WATCH→BREWING | 信用与流动性双红共振。历史样本中，此组合出现后 3–12 个月内系统性风险显著抬升（样本量小，存在失效可能）。 |
| →CRISIS | 情绪层双红：危机进行中。此阶段执行预案而非预测。 |
| CRISIS→WATCH | 情绪层连续 %d 个交易日回落至绿。危机状态解除，转入观察期。 |
| BREWING→WATCH | 信用/流动性共振解除并稳定 %d 个交易日。回到观察期。 |
| WATCH→NORMAL | 全部触发条件解除并稳定 %d 个交易日。回到常态。 |

### 4.2 页脚（包级常量）

```
—
风险状态提示（概率语言），非交易信号；指标基于有限历史样本，可能失效；操作决策不在本模块范围。
```

## 5. 七类完整示例

### 5.1 状态升级（进 BREWING，P0）

```
[P0] 🚨 状态升级 WATCH → BREWING · 07-14

信用与流动性双红共振。历史样本中，此组合出现后 3–12 个月内
系统性风险显著抬升（样本量小，存在失效可能）。

触发共振：
🔴 信用 hy_oas 612bp · 5y分位 98% · 压力(STRESS)
🔴 流动性 sofr_effr +28bp · 持续 5 个交易日

其余指标：
🟡 旁证 usdjpy 161.7 · 空头拥挤(CROWDED)
🟢 情绪 vix 18.2 · 5y分位 41%
🟢 情绪 move 88.1 · 5y分位 55%
🟢 领先 t10y2y +35bp · 5y分位 62%
🟢 领先 nfci -0.52 · 5y分位 18%

WATCH 已持续 12 个评估日 → BREWING · 下一评估：下一交易日
—
风险状态提示（概率语言），非交易信号；指标基于有限历史样本，可能失效；操作决策不在本模块范围。
```

### 5.2 状态降级（P1）

同骨架：首行 `[P1] ✅ 状态解除 BREWING → WATCH · 09-02`，语义句取降级查表，异常区标题 `仍异常：`，尾注 `BREWING 共持续 34 个评估日 · 下一评估：下一交易日`。

### 5.3 日报（BREWING/CRISIS）

```
[P1] 📍 BREWING 日报 第 5 日 · 07-18

异常指标：
🔴 信用 hy_oas 618bp · 5y分位 98% · 压力(STRESS)
🔴 流动性 sofr_effr +31bp · 持续 9 个交易日
🟡 旁证 usdjpy 158.9 · 周跌 2.1%

其余指标：
🟢 情绪 vix 19.0 · 5y分位 47%
🟢 情绪 move 96.2 · 5y分位 71%
🟢 领先 t10y2y +30bp · 5y分位 55%
🟢 领先 nfci -0.31 · 5y分位 44%

较昨日：hy_oas +6bp · usdjpy 转黄（原绿）
盘中 JPY 监测运行中（每 30 分钟）· 下一评估：下一交易日
—
（页脚同 4.2）
```

### 5.4 月报（NORMAL）

```
[P1] 📅 Cassandra 月报 · 2026-08 · NORMAL 已持续 63 个评估日

近 21 个交易日趋势（走势 · 月变化 · 5y分位）：
🟢 情绪 vix 15.0 ▁▂▂▃▂▁▂ ↘-2.3 · 12%
🟢 情绪 move 69.6 ▂▂▁▁▂▂▂ ↘-4.1 · 8%
🟢 流动性 sofr_effr -10bp ▃▃▂▃▃▃▃ →+1bp
🟡 信用 hy_oas 267bp ▃▂▂▁▁▁▁ ↘-18bp · 3% · 自满(COMPLACENCY)
🟢 领先 t10y2y +35bp ▂▃▃▅▅▅▆ ↗+9bp · 62%
🟢 领先 nfci -0.52 ▂▂▂▂▁▂▂ →-0.02 · 18%
🟡 旁证 usdjpy 161.7 ▅▅▆▆▇▇▇ ↗+2.4 · 空头拥挤(CROWDED)

AMBER 计数 2（触发 WATCH 需 ≥3）· 下次月报：9 月首个交易日
—
（页脚同 4.2）
```

### 5.5 周报（WATCH）

骨架同日报（不带 sparkline），首行 `[P1] 📅 Cassandra 周报 · 07-20 当周 · WATCH 已持续 18 个评估日`，尾注加退出进度：

```
退出进度：触发条件已连续解除 8 日（回 NORMAL 需连续 20 日）
下次周报：下周一 · 状态变更即时通知
```

### 5.6 运维速报（P2）

```
[P2] 🔧 move 数据源断更 · 07-14
最后观测 07-09（滞后 5 日 > 阈值 4 日），已标记 STALE 退出共振计数；恢复后自动回归。持续超一周需检查 Yahoo 通道。
```

### 5.7 盘中速报（P0）

```
[P0] 🚨 USD/JPY 盘中急跌 -3.4% · 07-18 14:30
现价 152.1（5 观测日前 157.5）· 系统状态 BREWING · 疑似 carry trade 快速平仓。今日此告警不再重复。
```

## 6. 渲染规则

### 6.1 状态 emoji 与非色彩状态

| Status | 显示 |
|---|---|
| GREEN / AMBER / RED | 🟢 / 🟡 / 🔴 |
| STALE | ⚪ + `数据断更(STALE)` |
| NO_DATA | ⚪ + `无数据(NO_DATA)` |
| SUPPRESSED_SEASONAL | ⚪ + `季末抑制` |

⚪ 指标排"其余指标"区末尾（已退出共振，视觉最弱）。

### 6.2 层名与排序

层名固定映射：vix/move→`情绪`、sofr_effr→`流动性`、hy_oas→`信用`、t10y2y/nfci→`领先`、usdjpy→`旁证`。

- 异常区：严重度降序（🔴 先于 🟡）；同级按冰山层序 **信用→流动性→情绪→领先→旁证**（深层异常优先看）。
- 其余区：固定 `AllIndicators` 顺序（vix, move, sofr_effr, hy_oas, t10y2y, nfci, usdjpy），⚪ 殿后。

### 6.3 数值格式（写死每指标一条）

| 指标 | 读数 | 月变化 |
|---|---|---|
| vix / move / usdjpy | 1 位小数 | ±1 位小数 |
| hy_oas | 整数 bp（无符号） | ±整数 bp |
| sofr_effr / t10y2y | 带符号整数 bp | ±整数 bp |
| nfci | 带符号 2 位小数 | ±2 位小数 |

5y 分位整数百分比（`98%`；`Pct5y<0` 时省略该片段）。tag 统一 `中文(英文)`：`压力(STRESS)` / `自满(COMPLACENCY)` / `空头拥挤(CROWDED)` / `倒挂后复陡(STEEPENING)`。sofr_effr 异常时附 `持续 N 个交易日`（N = 满足档位阈值的连续观测数）；usdjpy 异常且 wow 触发时附 `周跌 X.X%`。

### 6.4 sparkline（月报专用）

近 21 个观测 min-max 归一到 `▁▂▃▄▅▆▇█` 八阶；全平序列显示全 `▄`；观测不足 21 用可得长度。趋势箭头：`Δ = 当前 − 窗口首`，`|Δ|` 小于该指标显示精度一个单位（vix/move/usdjpy 0.1、bp 类 1bp、nfci 0.01）→ `→`，否则 `↗`/`↘`。

### 6.5 "较昨日"差异行（日报专用）

对比前一评估日指标行：状态迁移优先（`usdjpy 转黄（原绿）`），读数变化仅列当日异常区指标（`hy_oas +6bp`）；完全无变化 → `较昨日：无变化`。

### 6.6 退出进度（周报专用）

`触发条件已连续解除 N 日（回 NORMAL 需连续 %d 日）`——N = 系统评估行 detail 中 `any_trigger=false` 的连续日数（含当日），`%d` 取 `watch_exit_days`。

## 7. 约束

- **禁词**：`必然/一定/即将`——语义句表与页脚集中为包级常量；单测遍历全部 7 类模板输出断言不含禁词、结构化消息含 `非交易信号`。
- **长度**：telegram 上限 4096 字符，结构化消息最坏约 600 字符，不做拆分（YAGNI）。
- **发送失败**：仅记 stderr，评估已落库（沿实施方案既有约定）。
- **纯文本**：全部走 `SendText`（无 parse_mode），emoji 与 `▁▂▃` 均为普通字符，无 markdown 400 风险。

## 8. 实现接口（回写实施方案的依据）

渲染保持纯函数，输入收拢为 `NotifyContext`（cmd 层组装）：

```go
type Trend struct {
    Window []Observation // 近 21 观测（可短）
    Delta  float64       // 当前 − 窗口首
}

type NotifyContext struct {
    Res         *DayResult
    StateDays   int                   // 状态持续评估日数
    SummaryDue  bool                  // 月报/周报到期（cmd 计算）
    NewStale    []string              // 今日新进入 STALE 的指标
    PrevDay     map[string]Evaluation // 前一评估日指标行（较昨日 & NewStale 依据）
    ClearStreak int                   // any_trigger=false 连续日数（周报退出进度）
    Trends      map[string]Trend      // 仅月报到期时组装（sparkline 数据）
}

func Messages(cfg *Config, nc NotifyContext) []string          // 消息 1–6
func FormatIntradayAlert(price, base, wow float64, state SystemState, at time.Time) string // 消息 7
```

**IndicatorResult 需新增两个字段**（否则渲染层拿不到 §6.3 的持续性/周环比片段）：

```go
type IndicatorResult struct {
    // ...既有字段...
    PersistDays int     // sofr_effr：满足当前档位阈值的连续观测数（规则层填充）
    Wow         float64 // usdjpy/vix：周环比（WowOK=false 时无效）
    WowOK       bool
}
```

规则层（rules.go 的 evalSOFREFFR/evalUSDJPY/evalVIX）在计算时顺手填充，评估行 detail JSON 同步携带（审计与"较昨日"对比可用）。

**cmd 层组装职责与注意点**：

- `PrevDay`：必须在 `AppendEvaluations` **之前**用 `RecentIndicatorEvals(ind, 1)` 取（否则取到的是当日行）。
- `ClearStreak`：在 `internal/crisis/statemachine.go` 增加导出助手 `ClearStreakDays(hist EvalHistory, max int) (int, error)`（复用 detail 解析）。
- `Trends`：仅 `SummaryDue ∧ NORMAL` 时经 `store.SeriesWindow(ind, target, 21)` 组装。
- 盘中告警文案改由 `FormatIntradayAlert` 生成（`executeCrisisIntraday` 调用），纳入禁词单测覆盖。

**对实施方案（2026-07-13-macro-crisis-monitor-impl.md）的影响**：

| Task | 改动 |
|------|------|
| Task 1 | `types.go` 的 `IndicatorResult` 增加 `PersistDays`/`Wow`/`WowOK` 字段 |
| Task 9 | `evalSOFREFFR`/`evalUSDJPY`/`evalVIX` 填充上述字段；指标行 detail JSON 携带 |
| Task 10 | `statemachine.go` 增加导出助手 `ClearStreakDays(hist, max)` |
| Task 12 | `executeCrisisEvalDaily` 通知段改为组装 `NotifyContext`（PrevDay 前置获取） |
| Task 14 | `notify.go` 与测试按本设计整体重写（语义句表、骨架、sparkline、差异行、退出进度） |
| Task 15 | `executeCrisisIntraday` 改用 `FormatIntradayAlert` |

## 9. 测试要点

1. 禁词与页脚：7 类消息全覆盖（含盘中）。
2. 排序：异常区严重度降序 + 冰山层序；⚪ 殿后。
3. sparkline 边界：全平序列、观测 < 21、空窗口（省略该行）。
4. 差异行：状态迁移措辞、无变化路径。
5. P2 去重：昨日已 STALE 不再发；昨日正常今日 STALE 发一次。
6. 语义句 `%d` 注入：改 YAML `crisis_exit_days` 后文案跟随。
