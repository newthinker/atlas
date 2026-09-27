# 银行股关键指标月度监控 设计

日期：2026-09-26 · 状态：已确认设计，待实施计划

## 1. 目标与范围

定期检查配置列表中银行股的三项关键监管指标，生成统计报告并经 Telegram 推送：

| 指标 | 含义 | 预警方向 |
| --- | --- | --- |
| 不良贷款率（NPL） | 不良贷款 / 贷款总额 | 越高越差 |
| 拨备覆盖率 | 贷款损失准备 / 不良贷款 | 越低越差 |
| 核心一级资本充足率（CET1） | 核心一级资本 / 风险加权资产 | 越低越差 |

**人类已确认的决策（2026-09-26）**

- 市场：A 股 + 港股；**一期仅支持 A+H 银行**，港股条目须显式映射到 A 股代码（`a_share_ref`），纯港股银行配置校验拒绝。
- 报告内容：阈值预警 + 单行趋势（环比/同比）**和** 横向排名 + 分布统计，两者都要。
- 节奏：**每月固定推送**一次（无论是否有新披露）。
- 方案：**无状态单次命令**（不落库），launchd 每月触发。

**不做（YAGNI）**：美股银行、纯港股银行、本地入库/历史存档、多数据源合并、交互式查询。

## 2. 数据源（已实测）

东方财富「主要指标」经本地 aktools 侧车调用：

```
GET http://127.0.0.1:8180/api/public/stock_financial_analysis_indicator_em?symbol=600036.SH&indicator=按报告期
```

2026-09-26 对 600036.SH 实测（REPORT_DATE 2026-06-30，共 102 期，回溯至 1999）：

| 指标 | 字段 | 实测值 |
| --- | --- | --- |
| 不良贷款率 | `NONPERLOAN` | 0.94 |
| 拨备覆盖率 | `BLDKBBL` | 385.1 |
| 核心一级资本充足率 | `HXYJBCZL` | 14.07 |
| 报告期 | `REPORT_DATE` | `2026-06-30 00:00:00` |

注意的易混字段（**不得**使用）：`LOAN_PROVISION_RATIO`（拨贷比 3.63）、`NEWCAPITALADER`（资本充足率 18.33）、`FIRST_ADEQUACY_RATIO`（一级资本充足率 16.59）。

港股接口 `stock_financial_hk_analysis_indicator_em` 对 03968 / 02388 实测**无**上述银行字段，故港股走 A 股映射。

⚠ 字段名是 live 校验点（AKShare 随上游变动是常态），在代码中集中为常量并由 live 冒烟测试守护。

## 3. 架构

```
launchd（每月 1 日 09:00）
  └─ atlas bank report --config config.yaml --bank-config configs/bank-monitor.yaml [--dry-run]
       ├─ bank.LoadConfig        读取银行列表、阈值、数据源地址
       ├─ Source.Fetch(symbol)   调用东方财富（经 aktools）→ []Observation（按期）
       ├─ bank.Analyze           每家银行：最新期、环比、同比、预警判定
       ├─ bank.Summarize         同期横向排名、均值/中位数、未更新名单
       ├─ bank.Render            生成纯文本，按 4000 字符分段
       └─ Sender.SendText        复用 notifiers.telegram；--dry-run 只打印到 stdout
```

### 3.1 包 `internal/bank/`

| 文件 | 职责 |
| --- | --- |
| `config.go` | `Config`（mapstructure 标签）、`LoadConfig(path)`（独立 viper 实例，仿 `internal/crisis/config.go`）、`validate()` |
| `types.go` | `Observation{Period time.Time; NPL, Coverage, CET1 float64}`（缺失为 NaN）、`BankResult`、`Alert`、`Summary` |
| `source.go` | `Source` 接口 `Fetch(symbol string) ([]Observation, error)`；`EMSource` 实现（持有 aktools base URL 与 `http.Client`，超时 60s） |
| `analyze.go` | 纯函数：最新期、环比（上一期）、同比（上年同一报告期）、阈值与恶化幅度预警 |
| `summary.go` | 纯函数：按报告期分组，排名、均值、中位数、最优/最差，筛出未披露最新期的银行 |
| `render.go` | 纯函数：报告文本与分段 |

`EMSource` 不复用 `internal/collector/akshare.Client`（其 `get` 未导出）；为避免改动他人包的导出面，本包内自带最小 HTTP 调用。若实施时发现该包已有导出的通用调用入口，改为复用。

### 3.2 命令 `cmd/atlas/bank.go`

- `atlas bank report`，flag：`--bank-config`（默认 `configs/bank-monitor.yaml`）、`--dry-run`。
- 依赖注入结构 `bankReportDeps{source bank.Source; sender bank.Sender; now func() time.Time; out io.Writer}`，仿 `crisisEvalDeps`。
- `buildBankSender()` 仿 `buildCrisisSender()`（`cmd/atlas/crisis.go:428`）：读主配置 `notifiers.telegram`，未启用或缺凭据 → nil，此时退化为打印并在 stderr 提示。
- `bank.Sender` 为窄接口 `SendText(string) error`，`*telegram.Notifier` 直接满足。

### 3.3 配置 `configs/bank-monitor.yaml`

不含密钥，可随部署同步。

```yaml
source:
  aktools_url: http://127.0.0.1:8180
rank_by: npl            # npl | coverage | cet1
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: CN_A, symbol: 601658.SH, name: 邮储银行}
  - {market: HK,   symbol: 3968.HK,   name: 招商银行H, a_share_ref: 600036.SH}
thresholds:
  npl_max: 1.5          # %，> 即预警
  coverage_min: 150     # %，< 即预警
  cet1_min: 8.5         # %，< 即预警（7.5% 监管下限 + 1pp 缓冲）
  deterioration:        # 环比恶化超过以下幅度即预警，单位百分点
    npl_up: 0.10
    coverage_down: 20
    cet1_down: 0.50
```

**校验规则**（失败则启动报错退出、不推送）：

- `banks` 非空；`market ∈ {CN_A, HK}`；symbol 不重复。
- `CN_A` 的 symbol 须为 `^\d{6}\.(SH|SZ)$`；`HK` 须为 `^\d{4,5}\.HK$` 且 `a_share_ref` 必填并满足 A 股格式。
- 阈值全部 > 0；`rank_by` 取值合法（缺省 `npl`）。

## 4. 计算口径

- **最新期**：该银行三项指标至少一项非 NaN 的最近报告期。
- **环比**：与紧邻的上一个报告期比较；**同比**：与上一年同月同日的报告期比较；找不到对应期则显示为 `—`，不参与恶化预警。
- 变动一律以**百分点（pp）**表示（三项指标本身都是百分比）。
- **阈值预警**：`NPL > npl_max`、`Coverage < coverage_min`、`CET1 < cet1_min`，严格不等号（等于阈值不预警）。
- **恶化预警**：`ΔNPL环比 > npl_up`、`−ΔCoverage环比 > coverage_down`、`−ΔCET1环比 > cet1_down`。
- **A+H 去重**：拉取按 A 股代码去重（同一 `a_share_ref` 只拉一次）；横向统计、排名、预警按**主体**计一次，H 股条目在报告中以「同 600036.SH」附注出现。
- **同期统计**：取所有主体最新期中最晚的那个报告期作为「统计期」；只统计最新期等于统计期的主体（n），其余列入「未更新」。均值/中位数跳过 NaN。

## 5. 报告格式

纯文本（`SendText` 不设 parse_mode；纯文本下对齐不可靠，故用列表式而非表格）：

```
🏦 银行关键指标月报 2026-10-01
覆盖 12 家（统计期 2026-06-30：11 家；未更新：1 家）

⚠️ 预警 (3)
· 某银行 不良率 1.62% > 1.50%
· 某银行 拨备覆盖率 142% < 150%
· 某银行 CET1 环比 -0.62pp（超 0.50pp）

📊 同期统计 2026-06-30（n=11）
不良率    均值 1.21% | 中位 1.25% | 最优 招商 0.94% | 最差 X 1.62%
拨备覆盖  均值 238% | 中位 210% | 最优 招商 385% | 最差 Y 142%
CET1     均值 11.2% | 中位 10.8% | 最优 招商 14.07% | 最差 Z 8.9%

🏷 排名（按不良率升序）
1. 招商银行 不良 0.94%(环比-0.01/同比-0.02) 拨备 385%(-6/-27) CET1 14.07%(-0.4/+0.1)
2. …
（招商银行H 同 600036.SH）

ℹ️ 未更新/失败
· 某银行 最新 2026-03-31（未披露 2026-06-30）
· 某银行 拉取失败：aktools timeout
· 某银行 字段缺失：HXYJBCZL（疑似数据源结构变化）
```

- 预警区为空时显示「⚠️ 预警 (0) 无」。
- 分段：按行累加，单段不超过 4000 字符（Telegram 上限 4096 留余量），不在行内截断。

## 6. 错误处理与退出码

| 情形 | 行为 | 退出码 |
| --- | --- | --- |
| 配置非法 / 读取失败 | 报错，不推送 | 1 |
| 全部主体拉取失败（aktools 不可用） | 推送一条错误摘要 | 1 |
| 部分主体拉取失败 | 报告照常推送，失败项列入「未更新/失败」 | 2 |
| 某指标字段缺失 | 显示 `N/A`，报告末尾标注字段名；**不当作 0** | 0 |
| Telegram 发送失败 | 报错（token 已由 notifier 脱敏） | 1 |
| 全部成功 | — | 0 |

## 7. 测试（TDD）

- `analyze` / `summary`：表驱动覆盖：环比/同比缺期、NaN、阈值等号边界、恶化幅度边界、A+H 去重、各主体最新期不一致、全部 NaN。
- `EMSource`：`httptest` 回放录制的真实响应片段（600036），断言字段映射、日期解析、字符串数值兼容、字段缺失 → NaN、非 200 → error。
- `render`：分段每段 ≤ 4000 且不截断行；对完整样例做 golden 比对。
- `cmd` 层：注入 fake Source/Sender，覆盖第 6 节退出码表与推送条数、`--dry-run` 不调用 Sender。
- live 冒烟（`//go:build live`）：真实调用 aktools，断言 600036.SH 三字段均非 NaN 且最新期在近 9 个月内。

## 8. 部署

- 新增 `deploy/launchd/com.newthinker.atlas.bank-monthly.plist`：`StartCalendarInterval {Day:1, Hour:9, Minute:0}`，`EnvironmentVariables.no_proxy=localhost,127.0.0.1`，Telegram 走主配置 `notifiers.telegram.proxy`；日志到 `logs/bank-monthly.{out,err}.log`。
- 新增 `configs/bank-monitor.yaml`（示例清单：招商银行、邮储银行、招商银行H）。
- 上线前手工执行一次 `atlas bank report --dry-run` 核对输出。

## 9. 风险

| 风险 | 缓解 |
| --- | --- |
| AKShare/东方财富字段名变更 | 字段常量集中；缺失显式 `N/A` + 报告标注；live 冒烟测试 |
| 部分银行（如农商行）某指标未披露 | NaN 贯穿，统计跳过，报告显示 `N/A` |
| 季报披露口径差异（一季报/三季报部分银行不披露 CET1） | 同上；最新期定义取「至少一项非 NaN」 |
| aktools 侧车停机 | 全失败推送错误摘要，退出码 1 留痕 |
