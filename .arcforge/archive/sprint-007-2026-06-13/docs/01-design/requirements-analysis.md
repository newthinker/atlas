# 需求分析 — Lixinger Collector 修复重写

## 来源
- 计划：`docs/superpowers/plans/2026-06-13-lixinger-collector-rewrite.md`
- 设计 spec：`docs/superpowers/specs/2026-06-13-lixinger-collector-rewrite-design.md`
- 真实 API 验证：用配置中的真实 token + 官方 skill 包 `~/Downloads/lixinger-open-skill`
  逐接口实测确认端点/参数/字段/响应信封。

## 问题陈述
`internal/collector/lixinger` 与理杏仁真实开放 API 几乎全面不符，7 个方法仅「个股 PE 估值分位」
端点选对（但仍被 code 判定与解析层误杀）。lixinger 在系统中有两个角色：
1. eastmoney 的 fallback（quote/history/fund）；
2. 估值分位的唯一主数据源（`FetchValuationPercentile`）。

## 根因（已实测）
1. **响应信封判定反了**（7 处）：真实成功 `code:1`，失败 `code:0`+error；现写 `code!=0` 报错。
2. **端点错误**：`cn/stock/real-time`、`cn/stock/hq`、`cn/fund/nav`、`cn/fund/fundamental`、
   `cn/fund/nav/history` 均不存在（404）。
3. **参数形状错误**：`metrics`→`metricsList`；K线/基金净值需单数 `stockCode`。
4. **指标名错误**：`roe_ttm`/`dividend_yield_ratio`/`market_value` 非法 → `dyr`/`mc`；ROE 不提供。
5. **响应字段解析错误**：基金净值字段 `netValue`；日期 RFC3339；估值分位为扁平 dotted key。
6. **指数估值缺权重段**：指数需 `pe_ttm.y{N}.mcw.cvpos`。
7. **缺必需请求头**：SKILL.md 要求 User-Agent。

## 正确 API 事实
| 能力 | 端点 | 关键参数 | 响应要点 |
|---|---|---|---|
| K线/行情 | `cn/company/candlestick` | 单数 stockCode + type(fc_rights) | RFC3339 日期、newest-first |
| 个股估值/基本面 | `cn/company/fundamental/non_financial` | metricsList: pe_ttm/pb/ps_ttm/dyr/mc + cvpos | 扁平 key |
| 指数估值 | `cn/index/fundamental` | metricsList: pe_ttm.yN.mcw.cvpos | 扁平 key |
| 基金净值 | `cn/fund/net-value` | 单数 stockCode | netValue, RFC3339 |
| 基金概况 | `cn/fund/profile` | stockCodes | c_name/f_c_name/inception_date/op_mode |
| 基金经理 | `cn/fund/manager` | stockCodes | managers[]{name,departureDate} |
| 基金回撤 | `cn/fund/drawdown` | 单数 stockCode + granularity | value(负) |

## 设计决策（与需求方确认）
- 范围：7 方法全部按 skill 文档修正确。
- FetchQuote：无实时 API → candlestick 最新收盘近似，标 `lixinger-delayed`。
- 基金元数据：多接口聚合填满 FundInfo。
- 测试：httptest mock + 真实 fixture。
- 速率限制：完整 SKILL.md 退避重试 + 配置开关（默认开）。

## 复杂度评估
中等。单 package 内顺序重构，6 个新文件 + 重写 valuation + 改 lixinger.go/serve.go/config。
无新外部依赖，无 core 类型改动。
