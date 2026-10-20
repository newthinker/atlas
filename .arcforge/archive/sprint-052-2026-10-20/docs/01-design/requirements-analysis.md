# 需求分析（sprint 2026-10-20 Pricewatch 资产价格波动提醒）

需求源：`docs/superpowers/specs/2026-10-20-pricewatch-design.md`（spec，§N）+ `docs/superpowers/plans/2026-10-20-pricewatch.md`（plan，Task N）。两者均为人类已审阅的产物（AD-1）。

## 需求条目（R 编号供追溯矩阵使用）

| R | 需求 | 出处 |
|---|---|---|
| R1 | 独立 YAML 配置：全局/资产级 `check_interval`、storage、telegram、ai、notify_defaults、health、strategies、assets；资产 = strategy 规则 + 资产 `rules`（同 id 整条替换、新 id 追加、strategy 在前）；规则缺 notify 取 notify_defaults；symbol 大写化，Pair=Symbol+Quote | spec §3；plan T1 |
| R2 | 启动校验，任一失败报错退出：规则 id 同资产不重复；type/window 取值合法；threshold>0；step≥0；max_per_tier≥1；repeat_interval>0；资产至少一条规则；strategy 引用存在；check_interval>0；heartbeat_at 合法或空 | spec §3；plan T1 |
| R3 | Telegram bot_token/chat_id 只接受 `${ENV}`，明文报错，展开为空报错；proxy 原样 | spec §2 反目标、§3 |
| R4 | 默认值：ai.timeout=60s，health.fail_alert_after=30m，heartbeat_at 空=关闭 | plan T1 |
| R5 | stock/fund items 非空时只 warn 并跳过，不影响 crypto | spec §2；plan T1 |
| R6 | 规则语义：drop_from_high `x=(H−P)/H×100`、rise_from_low `x=(P−L)/L×100`；H/L 含影线且并入现价 | spec §4 |
| R7 | 档位：`x≥threshold` 满足，`tier=1+floor((x−th)/step)`，step=0 恒 1 档；1e-9 容差 | spec §4；plan Review Focus 1 |
| R8 | 滚动窗口 24h/7d/30d/4y(1461 天)/all；24h、7d 用 5m 线，30d/4y/all 用日线 | spec §4、§6 |
| R9 | 分档状态机四种情况（清零保留 last_sent_at / 新触发带防抖 / 更深档立即 / 档内重复受 max_per_tier 与 repeat_interval） | spec §5 |
| R10 | 同资产同轮多条规则合并为一条消息、AI 只调一次 | spec §5 |
| R11 | 发送失败：被发送规则状态不更新（下轮重试）；同轮清零照常落盘 | spec §5；plan Review Focus 4 |
| R12 | SQLite 本地存 K 线（日线全量、5m 保留 8 天）+ 规则状态 + kv；入库按主键幂等；重启后状态延续、不重复提醒 | spec §6；plan Review Focus 2/3 |
| R13 | 补数：分段拉取、逐段换源（某段不足 90% 换下一源，全不够取最多）、本地为空从起点回填、日常增量、所有源 0 根的段跳过 | spec §6；plan T4 |
| R14 | 数据不足 vs 缺失：历史不够长缩窗并在消息注明「窗口数据仅 N 天」；有效窗口缺口 >10% 本轮跳过该规则并记日志 | spec §6 |
| R15 | 取价按 sources 顺序，失败或价格为 0/NaN/Inf 换下一源 | spec §2、§6；plan Review Focus 5 |
| R16 | AI：仅有消息要发时调用；输入含现价/触发规则/多周期概况/最近 60 根 1m/最近 24h 5m/方向提示；JSON 输出、校验；超时/出错/解析失败 ⇒ 消息照发、写「AI 分析不可用」；模型沿用主配置 llm 段 | spec §7 |
| R17 | 消息格式：标题图标 🔻/🔺/🔀、【AI 结论】【触发规则】【多周期】【AI 分析】顺序、纯文本 SendText | spec §8；plan T5 |
| R18 | 健康告警：任一资产连续取价失败、或任一规则因缺口连续跳过，超过 fail_alert_after 发一次「监控异常」，恢复发「已恢复」 | spec §6、§9 |
| R19 | 每日心跳：heartbeat_at 发「监控仍在运行」（各资产现价与最近成功时间、触发中的规则及档位、过去 24h 提醒条数）；睡眠错过则醒来补发，当天不重复 | spec §9 |
| R20 | 子命令 `atlas pricewatch run / once [--dry-run] / backfill [--symbol]`，`--pricewatch-config` 默认 `configs/pricewatch.yaml`；dry-run 只打印不发送不写状态 | spec §9；plan T8 |
| R21 | 常驻调度：按各资产 check_interval；SIGTERM 退出 | spec §9；plan T8 |
| R22 | launchd plist：KeepAlive{SuccessfulExit:false} + ThrottleInterval；https_proxy/no_proxy；env 占位；配置错误退出码非 0 | spec §9 |
| R23 | 示例配置 `configs/pricewatch.yaml` 照抄 spec §3 | plan T1 |
| R25 | 工程约束：`GOTOOLCHAIN=local`、sqlite 固定 v1.38.2、`-race`、`go vet`、`gofmt`、覆盖率门槛（config `dev_minimum` 80 / 任务级 `coverage_floor`） | plan Global Constraints；arcforge.config.json |
| R24 | 上线（真实 Telegram/LLM、部署 plist、launchctl 加载）——人类动作，不入 DoD（AD-9） | spec §9 |

## 复杂度与依赖
- 新增独立包 `internal/pricewatch`（11 个源文件）+ `cmd/atlas/pricewatch.go` + 配置/plist；只新增，不改既有符号（唯一接线是 cmd 包内 `init()` 注册子命令，与 crisis 同法）。
- 依赖链基本串行：规则/状态机 → {配置, 存储} → {补数, 消息} → AI → 引擎 → 调度 → 子命令。整体复杂度：中等（引擎与补数为复杂项）。

## Leader 读码核实的前提（影响 DoD）
- `crypto.Provider` 只有 `FetchQuote(symbol)` / `FetchHistory(symbol,start,end,interval)`，无分页；okx `limit=300`、binance `limit=1000`。
- okx 的 `FetchQuote`/`FetchHistory` 对 `ParseFloat` 错误一律吞掉 ⇒ 价格 0 真实可达（Review Focus 5 不是假想）。
- okx 日线 `1D` 用 OKX 默认对齐（非 UTC 0 点）而 binance `1d` 是 UTC 0 点 ⇒ 逐段换源时同一 pair 的日线时间戳可能两种对齐并存（AD-13）。
- 最新一根 K 线是未收盘的；增量补数若从「最新一根之后」开始，该根的 high/low 永久冻结在首次入库那一刻（AD-12）。
- okx/binance 构造函数不在 `gate_wiring_test.go` 的 `collectorCtors`（不快照 policy.Default()）；`ensurePolicyGate()` 仍按计划调用。
- `llm.Provider.Chat(ctx, ChatRequest{SystemPrompt, Messages, MaxTokens, Temperature, JSONMode})`；`telegram.New(token, chat, telegram.WithProxy(p))` + `SendText`。
