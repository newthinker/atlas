# Changelog — Sprint 019 宏观危机监控（feature/crisis-monitor）

## 新增

- **FRED 采集器** `internal/collector/fred`：series/observations 客户端，指数退避重试（5xx/传输错误 ×3），"." 缺失值过滤，错误信息 api_key 脱敏（b8c6c13, dd2da2a, e2f2815）
- **crisis 引擎包** `internal/crisis`：
  - types/dates/store：六状态/四标签/四系统态类型，周内日近似交易日工具，sqlite WAL 双表存储（macro_observations 主键幂等 + crisis_evaluations 审计行含 ts 数据日列）（452c06d）
  - config：crisis-monitor.yaml typed 加载与校验，全部阈值配置化（bad1f55）
  - derive：SpreadBp/WowPct/MomChange/Percentile 纯函数（78ba314）
  - ingest：FRED+Yahoo 采集编排，单位换算，SOFR/EFFR 按日 join，Yahoo 失败降级（0a78d7a, f0e9ba2, 7664f49）
  - suppress：季末窗口抑制、新鲜度 STALE、非对称降级滞回（d59eb24, bc85498）
  - rules：7 指标三色规则（绝对阈值轨+分位轨 maxStatus 合成）（d8da972, 0a301e6）
  - statemachine/memhistory：NORMAL/WATCH/BREWING/CRISIS 四态转移，连续日计数历史行重建（5a0fa1c）
  - eval：EvalDay 单日编排（规则→抑制→滞回→状态机→8 行落库）（50aa92a, 1253f3d）
  - notify：[P0]/[P1]/[P2] 通知文案、月报/周报、边界声明页脚、禁词约束（08b407e）
- **CLI** `atlas crisis`：backfill（FRED/Yahoo 全量 + CSV 快照导入）、eval（daily 幂等五步/nfci/intraday JPY）、status、replay 回测（f104ba1, 38bd622, 3f4e64f, adaa3c5, 04ccad1, 263282f, 9850acc）
- **部署** `deploy/launchd`：crisis-daily（22:45/23:45/07:30）、crisis-nfci（周三 21:00/22:00）、crisis-intraday-jpy（1800s）三个 plist（9850acc）

## 变更

- `internal/collector/yahoo`：validSymbol 正则 `=F` → `=[FX]`，放行货币符号 JPY=X（76e9f59；impact LOW，3 调用方均包内）
- `internal/crisis/store.go`：新增 HasIndicatorEvalForDate（intraday 每日去重）（9850acc）

## 安全

- FRED api_key 双路径日志泄漏修复：传输错误与请求构造错误的 *url.Error 均剥离 URL 外层（dd2da2a, e2f2815，QA SEC-1）
