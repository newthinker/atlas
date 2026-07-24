# Prism AKShare 数据源设计(A/H 公司主源 + A 股指数兜底)

> 日期: 2026-07-24 | 关联: docs/prism/atlas_prism_design.md v2、sprint-024 验收记录
> 背景: 理杏仁公司端点时序 API 需购买 Open API(M1 验收实测,指数不受限);yahoo 对港股
> EPS 时序仅 5 个点(2023-09 起)且 qlib fundamentals_pit 为空表,engine 路径无法覆盖
> 0700.HK。茅台/腾讯在 M1 池中无数据。

## 目标

1. A/H 公司标的(600519.SH、0700.HK)经 AKShare 获得日频估值时序,board/detail 恢复展示。
2. A 股指数标的在理杏仁失败时自动降级到 AKShare,兜底不断数。
3. store/API/Web 零改动——新源只在采集与编排层出现。

## 决策记录(brainstorming 结论)

| 分叉 | 决策 | 理由 |
|---|---|---|
| 集成形态 | **aktools HTTP 侧车**(AKShare 官方 HTTP 封装,本地常驻) | Go 侧最干净,接口最全;运维 +1 个 launchd 服务可接受 |
| 覆盖范围 | A/H 公司(主源) + A 股指数(兜底) | 公司是痛点;指数兜底防理杏仁故障/理杏豆耗尽 |
| 兜底语义 | **编排层自动降级链**(方案 A) | 降级可观测(Report/告警),与 Prism source 分派结构同构;collector 保持纯粹。否决 collector 层包装(静默降级掩盖口径切换)与泛化源链(YAGNI) |

## §1 数据接口选型(全部为 ⚠ 实现期 live 校验点)

经 aktools 本地 HTTP(`http://127.0.0.1:8180/api/public/{接口名}`)调用:

| 用途 | AKShare 接口(以实现时版本为准) | 产出 | 符号映射 |
|---|---|---|---|
| A 股个股 | `stock_a_indicator_lg` (乐咕) | 日频 pe/pe_ttm/pb/ps 全历史 | `600519.SH → 600519` |
| 港股个股 | `stock_hk_valuation_baidu` (百度股市通) | 日频 PE-TTM/PB | `0700.HK → 00700` |
| A 股指数兜底 | 乐咕指数估值接口(`stock_index_pe_lg` 系,实现时确认) | 日频 PE(**加权口径**,近似理杏仁 mcw,差异文档注明) | `000300.SH → 乐咕指数名` |

- AKShare 接口名/字段随上游变动是常态——仿 M1 Task 3 惯例,在代码注释与
  discovery 标注 live 校验点,首跑失败以真实响应修正映射常量并同步测试。
- 符号映射表收敛在 collector 包内(单一处)。

## §2 架构组件

```
internal/collector/akshare/        新建: HTTP client
  client.go                        New(baseURL);FetchStockValuationSeries(symbol, market, start, end)
                                   FetchIndexValuationSeries(symbol, start, end)
                                   → []ValuationPoint{Date, PETTM, PB, PSTTM}(无官方分位字段;缺失=NaN)
internal/config/config.go          PrismInstrument 加 FallbackSource `mapstructure:"fallback_source"`
                                   PrismConfig 加 AkshareBaseURL(默认 http://127.0.0.1:8180)
internal/prism/refresh.go          AkshareClient 窄接口 + refreshAkshare 路径;
                                   Refresh 分派: source=="akshare" → refreshAkshare;
                                   source=="lixinger" 失败且 FallbackSource=="akshare" → 当场改走
                                   refreshAkshare,成功记入 Report.Degraded(新增字段 []string)
cmd/atlas/prism.go                 构造 akshare client 注入(base_url 取配置)
```

- `Report{Refreshed, Failed, Degraded}`——Degraded 兜底成功: 计入 Refreshed,同时
  进告警文本提示「主源失败已兜底」;双源皆败才进 Failed。
- store(`internal/storage/prism`)、JSON API、Web 页面零改动。

## §3 数据流与分位口径

- **公司(source: akshare)**: 增量语义与理杏仁路径一致——LatestDate 锚点,首次回填
  `lookback_years`(A 股乐咕全历史截到该窗口;HK 接口有多长拿多长),增量 latest+1,
  日历日比较(沿用 M1 时区安全守卫口径)。
- **分位本地计算**: akshare 无官方分位。写入前从 `store.Series(symbol, "")` 读回
  已存序列,与新点合并后 `RollingPercentile(dates, pe, 5, 252)` 与 `(…, 10, 252)`
  计算新点的 pctl_5y/pctl_10y 再 upsert(仅为新增点计算,历史行不回写)。
  相比 engine 路径(仅 5Y)多出 10Y 分位能力。
- **指数兜底**: 主源(理杏仁官方 cvpos)成功照旧;失败当场切 akshare,兜底行分位本地
  算。恢复后主源对同日行幂等覆盖(ON CONFLICT DO UPDATE 既有语义)。
  口径混合为已知取舍: 数据连续性优先于口径纯度,deployment/设计文档注明。

## §4 部署运维

- **独立 venv 隔离部署**: 新建 `scripts/akshare/.venv`(Python 3.11+)专供
  `akshare + aktools`,与 qlib_eval 的 venv 完全隔离(两者依赖树庞大且各自随上游
  更新,共享必然互相牵连)。提供 `scripts/akshare/setup.sh`(幂等建 venv + pip 安装
  + 版本冻结 requirements.txt)供部署与升级复用;aktools 启动命令统一指向该 venv
  的解释器。
- 新增 launchd 常驻服务 `com.newthinker.atlas.aktools`: ProgramArguments 用隔离
  venv 解释器(`…/scripts/akshare/.venv/bin/python -m aktools --host 127.0.0.1 --port 8180`,
  实参以实现时 aktools CLI 为准),KeepAlive,仅绑 `127.0.0.1:8180`,日志
  `logs/aktools.{out,err}.log`;deployment.md 追加装载/验证段。
- aktools 不可用 → akshare 路径报错: 公司标的进 Failed、指数标的主源若正常则无影响
  (兜底未触发);均不中断其他标的(沿用部分失败语义)。
- prism-daily plist 无需改动。

## §5 错误处理与测试

- Go 侧全部 httptest mock aktools: 接口路径/参数断言、符号映射、增量窗口、本地分位
  计算、缺字段→NaN、乱序→升序;testify + Context Checkpoint 注释惯例。
- 降级链专项用例: 主源败→兜底成→Degraded 且计入 Refreshed;双败→Failed;
  主源成→不触碰 akshare(零多余请求)。
- 既有 prism/lixinger/storage 用例不回归;`go build ./...` 全仓通过。
- 覆盖率: changed-package ≥80(collector/akshare 与 internal/prism 均单包可达,
  预计不触发 AD-6;cmd/atlas 接线若触发按既有处置模板)。

## 池配置变更(runtime + example)

```yaml
    - {symbol: "600519.SH", name: "贵州茅台", type: "stock", market: "CN_A", group: "A股公司", source: "akshare"}
    - {symbol: "0700.HK",   name: "腾讯控股", type: "stock", market: "HK", group: "港股公司", source: "akshare"}
    - {symbol: "000300.SH", name: "沪深300", type: "index", market: "CN_A", group: "A股指数", source: "lixinger", fallback_source: "akshare"}
    - {symbol: "000905.SH", name: "中证500", type: "index", market: "CN_A", group: "A股指数", source: "lixinger", fallback_source: "akshare"}
```

## 验收标准(M1.5)

1. aktools 服务经 launchd 拉起,`curl 127.0.0.1:8180` 可达。
2. 首跑后茅台/腾讯在 board 出现卡片,detail 曲线连续;分位列非空(≥252 样本后)。
3. 指数兜底演练: 临时改坏 lixinger key 跑 refresh → 指数当日行仍写入,告警含 Degraded 提示;恢复 key 后次日主源续跑。
4. 二跑增量: akshare 标的零重复拉取(latest+1 语义)。

## 明确不做(YAGNI)

- 不做泛化多源链抽象;不迁移美股/指数主源;不做行级 provenance 字段;
- 不在本期处理「理杏仁 latest 单点累积」模式(留作 akshare 不可用时的 Plan C)。
