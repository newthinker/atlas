# 危机监控（Cassandra）通知频率与机制 参考手册

> 日期：2026-07-23（依据 `internal/crisis/` + `cmd/atlas/crisis.go` 现行实现整理）
> 适用：macro-crisis-monitor 模块上线后的日常运维——回答「什么状态下会收到什么消息、多久一条」
> 关联：`docs/plans/atlas-macro-crisis-monitor-design.md`（总体设计）、
> `docs/plans/2026-07-14-crisis-notification-templates-design.md`（消息模板定稿，v1.1/v1.2 后续修订）、
> `docs/deployment.md`（部署要点与服务清单）

## 0. 一句话心智模型

**唤起频率 ≠ 通知频率。** launchd 多时点唤起只为覆盖数据发布窗口，幂等去重保证
每个交易日至多评估一次；通知条数由系统状态分级降噪——状态越差消息越密
（NORMAL 月报 → WATCH 周报 → BREWING/CRISIS 日报），结构化消息每日至多 1 条。

## 1. 调度频率（launchd 三个 plist，真相源 `deploy/launchd/`）

| 任务 | 频率（本地 +0800） | 作用 |
|---|---|---|
| `crisis-daily` | 每日 22:45 / 23:45 / 次日 07:30 三时点唤起 | 完整评估 + 发通知 |
| `crisis-nfci` | 每周三 21:00 / 22:00 | 仅刷新周频 NFCI 数据，不评估不发通知 |
| `crisis-intraday-jpy` | 每 30 分钟（`StartInterval 1800`） | 盘中 JPY 急跌监控 |

- **每交易日至多评估一次**：`executeCrisisEvalDaily`（`cmd/atlas/crisis.go`）先查
  `HasSystemEvalForDate` 幂等——第 2+ 次唤起打印 `already evaluated` 空跑。
- **数据未齐空跑**：4 个必要 FRED 日频序列（vix / hy_oas / t10y2y / sofr_effr）
  缺 T+1 观测则退出等下次唤起。
- **盘中任务近零成本**：非 BREWING/CRISIS 态启动即退出，不拉行情。

## 2. 通知频率（按系统状态分级降噪）

`crisis.Messages`（`internal/crisis/notify.go`）用互斥 switch 保证**结构化消息每日至多 1 条**：

| 状态 / 事件 | 消息 | 频率 |
|---|---|---|
| 状态变更日 | 变更消息（进 CRISIS `[P0] 🚨`，其余 `[P1] ⚠️`） | 当日替代日报/周报/月报 |
| BREWING / CRISIS | `[P1] 📍` 日报 | 每交易日 1 条 |
| WATCH | `[P1] 📅` 周报（含退出进度 n/20） | 每周一 1 条 |
| NORMAL | `[P1] 📅` 周报（无退出进度行） | 每周一 1 条 |
| NORMAL | `[P1] 📅` 月报（含 21 观测日趋势 sparkline） | 每月首个交易日 1 条；与周一撞日只发月报 |
| 指标新进入 STALE | `[P2] 🔧` 运维速报 | 事件触发，每指标 1 条 |
| 盘中 JPY 急跌 | `[P0] 🚨` 盘中速报 | 仅 BREWING/CRISIS 态，每日至多 1 条 |

- **P2 速报去重**：仅「昨日非 STALE、今日 STALE」的指标发一次（`buildNotifyContext`），
  持续 STALE 不重复发。
- **盘中速报触发**：JPY=X 实时价对 5 观测日前收盘的周环比跌破 `red_wow_pct`（−3%）；
  以评估行 `usdjpy_intraday` 做每日一次去重，**先落库去重行再发送**——通知丢失也不会重复告警。
- **月报/周报不加第 4 个 plist**：`summaryKind` 在 daily eval 内判断——NORMAL
  当月首个交易日发月报、其余周一发周报（撞日只发月报）；WATCH 周一发周报。

## 3. 通知机制

- **单渠道 Telegram 纯文本**：`crisis.Sender` 接口只有 `SendText`，由
  `buildCrisisSender` 复用主配置 `notifiers.telegram` 凭据；未配置时退化为打印到
  stdout（本地试运行）。无 parse_mode，emoji / sparkline 都是普通字符。
- **优先级不走渠道路由**：紧急度只体现在 `[P0]/[P1]/[P2]` 文字前缀（设计 §4.4：
  notifier 无 Priority 概念，本期不公共化）。
- **发送失败不阻断**：评估先落库再发送，`SendText` 失败仅打 warning——
  状态可由 `atlas crisis status` 自愈获取（文件真相源）。
- **通知上下文取「截至昨日」历史**：`buildNotifyContext` 必须在
  `AppendEvaluations` 之前调用（PrevDay / StateDays / ClearStreak 语义依赖此顺序）。

## 4. 防误报 / 防噪声机制

| 机制 | 规则 | 实现 |
|---|---|---|
| 季末抑制 | SOFR-EFFR 在季末最后 3 + 季初 2 个交易日内不触发 | `InQuarterEndWindow`（`suppress.go`） |
| 非对称去抖 | 升级立即生效；降级需连续 3 观测日（`demote_hysteresis_days`） | `ApplyHysteresis`（`suppress.go`） |
| 状态退出冷却 | CRISIS→WATCH 10 天、WATCH→NORMAL 20 天、BREWING→WATCH 10 天，态内重新累积 | `statemachine.go` |
| STALE 降级 | 日频超 4 天 / NFCI 超 12 天无新观测 → 不参与共振计数 | `staleFor`（`suppress.go`） |

## 5. 排障速查

| 现象 | 先查 |
|---|---|
| 当日没收到任何消息 | NORMAL/WATCH 态非周一（且 NORMAL 非月初）属正常静默；再查 `logs/crisis-daily.out.log` 是否 `data not ready` 或 `already evaluated` |
| 收到 `[P2]` 断更速报 | 对应数据源（FRED/Yahoo）连通性；Yahoo 需经本地代理（plist 内 `http_proxy`） |
| 疑似漏发/重发 | 通知不落库、评估落库——以 `atlas crisis status` 与 `crisis_evaluations` 表为准 |
| 调阈值后想预估告警频率 | `atlas crisis replay --from ... --to ...`（只读重放，不写库不发通知） |
