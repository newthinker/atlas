# M1.5 AKShare Changelog(2026-07-24)

### 新增
- `internal/collector/akshare`: aktools HTTP client——A 股个股(乐咕 lg 全历史)、港股个股(百度双指标合并)、A 股指数(乐咕中文键);字符串数值兼容与 schema 漂移守卫(全 NaN→告警)
- `internal/prism`: refreshAkshare 路径(增量 latest+1、本地 RollingPercentile 5Y/10Y 分位、只写新点);指数自动降级链(lixinger 失败→akshare 兜底,Report.Degraded 可观测);incrementalStart 提取复用
- `cmd/atlas`: Refresh 注入 akshare client;告警扩展(Failed/Degraded 分段,输出含 degraded 计数)
- `config`: prism.akshare_base_url、instruments[].fallback_source
- 部署: scripts/akshare/{setup.sh(lock 优先复现安装/--upgrade),requirements.txt}、com.newthinker.atlas.aktools.plist(隔离 venv,127.0.0.1:8180)、deployment.md AKShare 侧车段
- 池变更(example): 茅台/腾讯 source=akshare;沪深300/中证500 加 fallback_source=akshare

### 已知口径
- akshare 无官方分位→本地滚动分位;指数兜底日分位为混源序列排位(文档已注明);akshare 接口名/字段为 live 校验点(守卫兜底)
