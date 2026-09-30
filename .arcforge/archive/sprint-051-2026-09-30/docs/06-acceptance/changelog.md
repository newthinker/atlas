# Changelog — feature/tiingo-source（2026-09-30）

## 新增
- `internal/collector/tiingo`：Tiingo 美股日线备源。直连（`Proxy=nil`，不继承进程 `https_proxy`）、`Authorization: Token` 头鉴权、不带 `endDate` 取全量以折算其后的拆股、价格口径对齐 yahoo `quote.close`（拆股调整、不含分红）、所有错误出口 `tiingo: ` 前缀 + 截断前脱敏 + `%v` 断链。
- 白名单：仅 `^[A-Z]{1,5}([.-][A-C])?$` 且路由为美股的代码发请求；A 股/港股/指数/期货/加密/外国交易所后缀一律拒绝、不占配额。
- policy 内置主题 `tiingo.daily`：小时配额 40（跨进程文件账本，prism 与 serve 共用）。
- `configs/config.example.yaml`：`collectors.tiingo` 示例（默认关闭）与 TTL 6h 建议。
- 集成测试（`-tags integration`，`ATLAS_TIINGO_TOKEN`）。

## 变更
- `collector.Registry.GetAll()` 由 map 随机序改为**按首次注册顺序**（同名重注册保持原位）。依赖首个 collector 的 `buildArbitrator`、backtest、api 由随机选变为确定选首个注册者（生产配置下为 yahoo）。
- prism 美股价格降级链：`yahoo → twelvedata` 改为有序多跳 `yahoo → tiingo → twelvedata`；降级文案附前序失败跳原因（`…, tiingo failed (<err>), twelvedata fallback ok`）；只有 twelvedata 一跳或无备源时文案/错误串与此前逐字一致。
- `atlas serve` / `atlas watchlist`：tiingo 启用且有 key 时注册为最后一个外部兜底（tushare/baostock 之后、qlib 之前）。

## 兼容性
- 未配置 `collectors.tiingo`（或未启用 / 缺 key）：prism 与 serve 行为与此前一致（Registry 顺序确定化除外）。
