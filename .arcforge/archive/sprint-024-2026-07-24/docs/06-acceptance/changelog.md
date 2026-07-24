# Prism M1 Changelog

## feature/prism-m1(2026-07-23 ~ 2026-07-24)

### 新增
- `internal/storage/prism`: SQLite store(instrument/valuation_daily,WAL,NaN↔NULL)
- `internal/collector/lixinger`: FetchValuationSeries 估值时序(公司/指数 .mcw,cvpos×100)
- `internal/valuation`: ReconstructPESeries + RollingPercentile(提取 alignPE/sortedEPSWithGate)
- `internal/prism`: Refresh 编排(理杏仁增量路径:时区安全守卫、理杏豆零请求;美股 yahoo 引擎路径:全量重算+EPS≤0 熔断)
- `cmd/atlas`: `atlas prism refresh` 子命令(telegram 失败告警,复用 crisis helper)
- `internal/api`: GET /api/prism/board、/api/prism/series(NaN→null,status 标签,500 脱敏)
- `internal/api/handler/web`: /prism/board(HTMX 卡片+group 筛选)、/prism/detail(vendored ECharts 双图)、/static
- `configs/config.example.yaml` prism 段(8 标的);`deploy/launchd/com.newthinker.atlas.prism-daily.plist`(每日 08:30);docs/deployment.md Prism 部署段

### 修复(QA review_fix)
- CRITICAL: serve 磁盘模板缺 prism 模板致全站启动失败(补磁盘双份+真实路径回归测试)
- MAJOR: prism-daily.plist 配置路径对齐 configs/config.yaml;refreshLixinger 时区混算;refreshEngine 亏损标的熔断
- MINOR: API/Web 500 响应脱敏;死字段清理

### 已知限制
- 启用 ATLAS_API_KEY 时 /prism/detail 图表 401(继承性缺陷,deployment.md 已注明,M2 横切整改)
- 美股 yahoo 路径无 PB/PS/10Y 分位(M1 口径);理杏仁美股指数仅标普500
