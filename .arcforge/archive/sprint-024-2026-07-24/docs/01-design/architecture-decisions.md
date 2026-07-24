# Prism M1 架构决策(ADR 摘要)

1. **单体融入而非独立服务**:Prism 作为 Atlas 子模块(单用户/单机 launchd 场景,无需独立部署)。
2. **SQLite(modernc, 固定 v1.38.2)+ WAL**:与 crisis 等既有模块一致;NaN↔NULL 约定统一缺失语义。
3. **双数据路径**:理杏仁(现成估值+官方分位,增量拉取控理杏豆成本)/ yahoo 引擎(美股公司重建 PE,每日全量重算保证幂等、无增量状态)。
4. **接口窄化注入**:`prism.Store/LixingerClient/USClient`、api 层 `PrismStore`、web 层 `PrismProvider` 均为消费方定义的窄接口 → fake 可测,不 mock SQL。
5. **前端零新 CDN**:ECharts vendor 进二进制(embed),layout 既有 tailwind/htmx CDN 不动。
6. **告警复用**:失败告警复用 crisis 的 telegram sender 构造;部分失败 exit 0 + 告警,致命错误非 0(launchd 语义)。
7. **阈值配置化**:低/高估百分位默认 15/85,PrismConfig 可覆盖,API 响应回传供前端渲染。
