# 部署说明 — sprint-002（2026-06-11）

无 DB 迁移、无新依赖（Go module 无变更）。

## 配置变更（均可选，不改配置直接升级 = 新策略不启用，行为不变）

| 配置项 | 说明 |
|--------|------|
| `strategies.price_percentile` | enabled + params（lookback_years/low/high/extreme_low/extreme_high，默认 5y/25/75/10/90） |
| `strategies.pe_percentile` | 同上（默认 20/80/10/90）；需配合 lixinger api_key 或 yahoo 启用才有数据 |
| watchlist 新类型 | `type: "指数"`/`"期货"` 支持 ^GSPC/GC=F/A 股指数符号 |

## 启用建议（按依赖）

1. 仅价格分位：启用 price_percentile + watchlist 加指数/期货项即可（仅需 yahoo/eastmoney）
2. PE 分位：另需 `collectors.lixinger.api_key`（A股/指数必需；美/港个股为兜底路径）
3. **上线前手工终验**（final-report 遗留项）：真实 serve 挂 ^GSPC 观察 price_percentile 信号产生；有 LIXINGER_API_KEY 时核对 usHKIndexCodes 四个指数代码（lixinger/valuation.go 已注明）

## 回滚

- 新策略：`strategies.*.enabled: false` 即完全停用
- 指数/期货监控：从 watchlist 移除对应项即可，采集层扩展对存量符号零影响（全量回归已证）
