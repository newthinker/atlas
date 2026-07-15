# Cassandra 危机监控历史回放报告设计 v1.1

> **基线**：通知设计 v1.0–v1.2 已交付（渲染器 `internal/crisis/notify_render.go`、消息家族、页脚规则）；
> CLI `crisis replay`（调参验证，输出转移线）已存在并经 sprint-019 回测验证。
> **动机**：2026-07-15 手工回放 2008 危机月报暴露的能力缺口——渲染器包内私有导致只能临时测试文件 hack、
> 发送靠手工 curl、无总结、无可视化。将该流程产品化。
> **用户确认的决策**：新子命令 `crisis report`；--send 硬上限+确认门；telegram 标准总结 + 独立 HTML
> 详细报告（内联 SVG 点阵图/折线图/报表）；一次 Sprint 全量交付。

## 1. CLI 规格

```
atlas crisis report --from YYYY-MM-DD --to YYYY-MM-DD --form daily|monthly [--send]
                    [-c config] [--crisis-config path]
```

| 行为 | 规格 |
|---|---|
| 默认输出 | 报告正文逐条打印 stdout + 标准总结；**始终**生成 HTML 落盘 `reports/crisis-replay-<from>-<to>.html` 并打印路径 |
| `--send` | 文本报告逐条发 telegram（间隔 3s）→ 标准总结 → HTML 以 sendDocument 附件发送（caption=总结首行）。发送失败单条记 stderr 继续（沿评估链错误语义） |
| 量控 | `--send` 且报告条数 > 31 → 启动前直接报错退出：`daily 回放共 N 条报告，超过 --send 上限 31。请缩短周期，或用 --form monthly，或去掉 --send 输出到 stdout。`（stdout 模式无限制） |
| 回放标记 | 每条 telegram 消息前缀行 `【历史回放 <日期或月份> · 非实时告警】`，由 cmd 层拼接（渲染器不感知回放） |
| 状态门控 | **忽略**——`--form` 指定什么就生成什么（daily=每交易日、monthly=每月首交易日），首行如实打印回放态（如 `CRISIS 日报 第 N 日`） |
| 参数校验 | from ≤ to、必填、form 枚举；from 早于库内最早数据 → 报错提示实际可用起点 |

## 2. 回放引擎（internal/crisis，导出，双消费方）

```go
// ReplayDay 一个回放交易日的完整评估快照。
type ReplayDay struct {
    Date      string
    Res       *DayResult
    StateDays int // 当前状态连续评估日数（含当日）
}

// ReplayRange 从库内最早观测日起暖机逐日重放（内存历史、零写入），返回 [from,to] 窗口。
func ReplayRange(cfg *Config, sr SeriesReader, from, to string) ([]ReplayDay, error)
```

- **暖机口径**：交易日历以 vix 观测日为准；从库内最早 vix 观测日起 `EvalDay` + `NewMemHistory` 逐日推进
  （2026-07-15 已实证与既有 CLI replay 对 2006–2009 全期窗口转移线逐条对齐）。
- **既有 `crisis replay` 子命令重构为调用本引擎**（cmd 只留转移线/统计的打印），消除两套回放循环。
  **统一暖机语义（v1.1 决策）**：既有 replay 原为从 `--from` 冷启动（空 MemHistory），`from` 落在
  危机中段时期初态失真；重构后 replay 同样走暖机引擎，视为口径修复。黄金对照收窄为**全期窗口**
  （from ≤ 库内最早数据日，如 2006-01-01..2009-12-31）输出格式与统计**逐字节不变**；
  from 晚于最早数据日的窗口，期初态以暖机结果为准（行为变化，预期内）。
- 数据不足处理：暖机期内 EvalDay 的 NO_DATA/STALE 由既有规则自然处理，不特判。

## 3. 文本报告生成（cmd 层组装 + 既有渲染器）

- **daily**：对窗口内每个交易日 d，构造回放版 NotifyContext：
  `Res`=当日、`StateDays`=引擎值、`PrevDay`=前一回放日的 `Res.Results` 转 Evaluation（Indicator/Status/Value）、
  `SummaryDue=false`、`NewStale/StaleLastObs/Trends` 空 → `renderDaily`。差异行（较昨日）真实可用。
  窗口首日无前日 → PrevDay 空 map（diffLine 输出"无变化"，可接受）。
- **monthly**：窗口内每月首交易日 → `Trends` 用 `sr.Window(ind, d, 21)`（空窗口省略行）→ `renderMonthly`。
- 渲染函数为包内私有 → cmd 无法直调。**新增导出装配入口**（internal/crisis）：

```go
// ReplayReport 渲染一个回放日的指定形式报告（忽略消息矩阵门控）。
func ReplayReport(cfg *Config, form string, day ReplayDay, prev *ReplayDay, sr SeriesReader) (string, error)
```

（form ∈ "daily"|"monthly"；内部分发 renderDaily/renderMonthly 并完成 NotifyContext 组装，保持渲染纯函数。）

## 4. 标准总结（internal/crisis 新渲染器，导出）

```go
func RenderReplaySummary(cfg *Config, days []ReplayDay) string
```

格式（单条 ≤4096）：

```
【回放总结 <from> ~ <to>】
状态：<期初态> 起步 · 期间转移 <k> 次
<逐条转移：YYYY-MM-DD FROM → TO>（无转移则「转移：无」）
各态停留：CRISIS 394 日 · WATCH 0 日 ·（仅列出现过的态）
指标极值（期间最差读数）：
vix 80.9（2008-11-20）· move 264.6（2008-10-10）· hy_oas 2147bp（2008-12-15）· …
（逐指标一条，用 formatReading；"最差"方向 = 各指标触发方向：vix/move/hy_oas/nfci/sofr_effr 取
 期间最大值、t10y2y 与 usdjpy 取期间最小值——与各自红灯方向一致）
AMBER 峰值：<n>/7（YYYY-MM-DD）
STALE 统计：<ind> 缺数 <n> 交易日（仅列非零者；全零则省略整行）
—
历史回放，非实时告警；阈值为当前配置，非事后调参。
```

尾注为回放专用常量（不复用 notifyFooter；含"历史回放"限定）。禁词约束沿用。

## 5. HTML 详细报告（internal/crisis 新文件，html/template + 内联 SVG，零新依赖）

`func RenderReplayHTML(cfg *Config, days []ReplayDay, sr SeriesReader) (string, error)` → cmd 落盘。

结构（自包含单文件、无外链、`prefers-color-scheme` 亮暗兼容）：

1. **头部**：期间、form 无关（HTML 始终全量日粒度）、生成时间、当前配置阈值摘要表。
2. **状态时间线点阵图**：SVG，x=交易日序、每日一个色块（NORMAL 绿/WATCH 黄/BREWING 橙/CRISIS 红），
   月份刻度轴，悬停 title=日期+状态。
3. **7 指标折线图**：每指标一幅 SVG（polyline），含 amber/red 阈值横线（读 cfg，usdjpy/nfci 等按各自
   规则可省略阈值线）、STALE/缺数日在 x 轴打点标记、y 轴用 formatReading 同款量纲；
   sofr_effr 全期无数据 → 该幅省略并注明"该指标自 2018-04 起才有数据"。
4. **月度汇总表**：月份 × {各指标 min/max/月末值、AMBER 天数、STALE 天数、月末状态}。
5. **状态转移明细表**：日期、FROM→TO、当日触发指标（detail 摘要）。

图表规范：SVG viewBox 响应式、色板同 emoji 语义（绿/黄/橙/红）、表格 overflow-x 容器。

## 6. telegram 扩展（internal/notifier/telegram）

```go
func (t *Telegram) SendDocument(path, caption string) error
```

multipart/form-data 上传 bot API `sendDocument`（chat_id、caption ≤1024 截断、文件名取 basename），
复用既有 proxy/token 配置与错误语义。`Sender` 接口不动（crisis 包不感知；cmd 用类型断言取
`interface{ SendDocument(string, string) error }`，断言失败降级为总结尾附文件路径提示）。

## 7. 约束

- 零新第三方依赖；GOTOOLCHAIN=local；渲染纯函数纪律沿用（引擎/渲染不做 IO，DB 读经 SeriesReader，落盘/发送在 cmd）。
- 回放全程零写入（内存历史；生产 crisis.db 只读）。
- 禁词/4096/页脚规则：文本报告沿用消息家族既有规则；总结与 HTML 用回放专用尾注。
- 既有 replay 子命令**全期窗口**行为不变（黄金对照）；非全期窗口期初态改为暖机语义（v1.1 口径修复）。

## 8. 测试要点

1. **引擎黄金对照**：ReplayRange 与重构前 replay CLI 对 2006-01-01..2009-12-31（全期窗口）的转移序列
   逐条一致（重构前先把现输出录成 golden）。另加暖机语义用例：from 晚于最早数据日时，
   窗口期初态等于全期回放在该日的状态（而非冷启动 NORMAL）。
2. StateDays 连续性：转移日=1、次日=2；窗口切片不影响计数（暖机期计入）。
3. 量控落界：31 条恰通过、32 条拒绝（消息字面值）。
4. daily PrevDay 链：差异行反映前一回放日；窗口首日"无变化"。
5. 总结：极值方向逐指标（t10y2y 取最小的落界）、转移列表、STALE 统计非零才列、≤4096（2006–2009 全期）。
6. HTML：golden 片段断言（点阵图色块数=交易日数、折线图 polyline 点数、月度表行数、sofr_effr 省略注记）；
   不做整文件快照。
7. SendDocument：httptest 假 bot API 断言 multipart 字段；caption 截断落界（1024）。
8. --send 全链路用注入 fake sender（禁真发）；发送失败继续语义。

## 9. 实施拆分建议（arcforge，6 任务）

| # | 内容 | 包 |
|---|---|---|
| 1 | ReplayRange 引擎 + 既有 replay 子命令重构（黄金对照） | internal/crisis + cmd |
| 2 | ReplayReport 装配（daily/monthly + PrevDay 链） | internal/crisis |
| 3 | RenderReplaySummary | internal/crisis |
| 4 | RenderReplayHTML（SVG 点阵/折线/表） | internal/crisis |
| 5 | telegram SendDocument | internal/notifier/telegram |
| 6 | report 子命令装配（参数/量控/发送/落盘） | cmd/atlas |
