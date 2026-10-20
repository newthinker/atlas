# Pricewatch：资产价格波动提醒 — 设计

日期：2026-10-20
状态：待审阅

## 1. 为什么做

用户在 okx 做加密货币**双币赢**，以「低买」为主，「高卖」为辅。下单时机取决于价格是否处在相对低点（或高点），所以需要一个工具在价格明显波动时及时提醒，同时也想知道中长期的价格位置。

收到提醒后，用户要据此决定「现在下单」还是「再等等」，所以消息里要先给出 AI 对短期走势的判断和建议，再给出细节。最终由用户自己判断。

atlas 现有的东西都对不上这个需求：
- `price_percentile` 只看历史分位，不看短时涨跌。
- `internal/alert` 只监控系统指标（HTTP 错误率等），表达式没有标的维度。
- watchlist 里也没有加密货币。

所以本设计在 atlas 里新增一个独立模块 `pricewatch`，有自己的配置文件和子命令。组织方式参照 `crisis`（独立配置 + 独立 SQLite + 子命令）。

## 2. 目标 / 非目标 / 反目标

**目标**
- 按资产类别手动配置监控对象。第一版只实现加密货币。
- 短、中、长期价格规则都写在配置文件里，可以按资产定制。
- 规则触发后发 Telegram 消息，消息里带 AI 的下单建议。
- 告警间隔、每档最大次数、检测间隔都可以配置。

**非目标（第一版不做）**
- 股票和基金的实际监控。配置格式里留好位置，写了也只打 warn 并跳过。
- 自动下单、对接 okx 双币赢接口。
- Web 界面、历史回测、规则命中统计。

**反目标（技术上能跑，但出现以下任一情况就算失败）**
- **因为 AI 或某个数据源故障而错过一个低点**：AI 失败时提醒照常发出；一个数据源失败时切到下一个。
- **刷屏**：价格在阈值附近来回波动时，每 5 分钟一条消息。
- **静默死亡**：监控挂了用户不知道，还以为「没消息 = 没波动」。
- **密钥进仓库**：Telegram token 只能以 `${ENV}` 的形式写进配置。

## 3. 配置文件格式

文件放在 `configs/pricewatch.yaml`，也可以通过 `--pricewatch-config` 指定。下面是 BTC 的完整示例，对应用户提出的 a/b/c 三组规则：

```yaml
# 资产价格波动提醒（atlas pricewatch）
# 设计: docs/superpowers/specs/2026-10-20-pricewatch-design.md

check_interval: 5m              # 全局检测间隔，资产可单独覆盖。可写 5m / 10m / 30m …

storage:
  path: data/pricewatch.db

telegram:                       # 需求 3：token 与 chat id 写在本文件
  bot_token: "${PRICEWATCH_TELEGRAM_BOT_TOKEN}"   # 只接受 ${ENV} 写法，写明文直接报错退出
  chat_id: "${PRICEWATCH_TELEGRAM_CHAT_ID}"
  proxy: ""                     # 可选，语义同主配置 notifiers.telegram.proxy

ai:
  enabled: true
  timeout: 60s                  # 超时或失败：消息照发，AI 段写「AI 分析不可用」
  # 模型与密钥沿用主配置（--config）的 llm 段，这里不重复配置

notify_defaults:                # 需求 4，规则未写 notify 时使用
  repeat_interval: 1h           # 停在同一档时，两次提醒的最小间隔
  max_per_tier: 3               # 同一档最多提醒几次（含第一次）

health:
  fail_alert_after: 30m         # 连续取价失败超过这么久，发一条「监控异常」（只发一次，恢复后再发「已恢复」）
  heartbeat_at: "09:00"         # 每天本地时间这一刻发一条「监控仍在运行」；留空 = 关闭

# 可复用的规则组。规则只有两种类型：
#   drop_from_high: 现价较窗口内最高价的跌幅 ≥ threshold_pct
#   rise_from_low:  现价较窗口内最低价的涨幅 ≥ threshold_pct
# window: 24h | 7d | 30d | 4y | all（all = 本地存有的全部历史）
# step_pct: 档距，每多 step_pct 算一档，再提醒一次；0 = 不分档
strategies:
  crypto_dual_invest:
    # a. 长期
    - {id: ath_drawdown, type: drop_from_high, window: all, threshold_pct: 50,  step_pct: 5,
       notify: {repeat_interval: 24h, max_per_tier: 2}}
    - {id: cycle_rise,   type: rise_from_low,  window: 4y,  threshold_pct: 100, step_pct: 50,
       notify: {repeat_interval: 24h, max_per_tier: 2}}
    # b. 中期（1 周）
    - {id: week_drop, type: drop_from_high, window: 7d, threshold_pct: 5, step_pct: 5}
    - {id: week_rise, type: rise_from_low,  window: 7d, threshold_pct: 5, step_pct: 5}
    # c. 短期（1 天）
    - {id: day_drop,  type: drop_from_high, window: 24h, threshold_pct: 2, step_pct: 2}
    - {id: day_rise,  type: rise_from_low,  window: 24h, threshold_pct: 2, step_pct: 2}

assets:
  crypto:
    sources: [okx, binance]     # 取价顺序：第一个失败用下一个。okx 在前，因为用户在 okx 下单
    quote: USDT
    items:
      - {symbol: BTC, name: 比特币, strategy: crypto_dual_invest}
      - symbol: ETH
        name: 以太坊
        strategy: crypto_dual_invest
        check_interval: 10m     # 覆盖全局间隔
        rules:                  # 与 strategy 中同 id 的规则整条替换；新 id 追加
          - {id: day_drop, type: drop_from_high, window: 24h, threshold_pct: 3, step_pct: 3}
  stock: {items: []}            # 预留：非空时 warn「第一版不支持」并跳过
  fund:  {items: []}            # 预留，同上
```

**配置校验（启动时检查，任一失败就报错退出）**
- 规则 `id` 在同一资产内不能重复。
- `type` 和 `window` 必须是上面列出的取值。
- `threshold_pct` 必须大于 0，`step_pct` 不能小于 0。
- `max_per_tier` 不能小于 1，`repeat_interval` 必须大于 0。
- 资产必须至少有一条规则，可以来自 `strategy`、`rules` 或两者合并。
- Telegram 字段只能是 `${ENV}` 格式，展开后不能为空。

## 4. 规则语义

假设现价为 `P`，窗口内最高价为 `H`、最低价为 `L`。`H` 和 `L` 取窗口内 K 线的 high/low，包括影线，并把 `P` 也算进去。

- `drop_from_high`：`x = (H − P) / H × 100`
- `rise_from_low`：`x = (P − L) / L × 100`

当 `x ≥ threshold_pct` 时规则满足，此时档位 `tier = 1 + floor((x − threshold_pct) / step_pct)`。`step_pct` 为 0 时 `tier` 恒为 1。

窗口都是**滚动窗口**，从当前时刻往前数：24h、7d、30d，`4y` 按 1461 天算。

用 BTC 来核对（用户确认过的例子）：历史最高价 10 万时，`ath_drawdown` 在价格 ≤ 5 万时触发；过去 4 年最低价 2 万时，`cycle_rise` 在价格 ≥ 4 万时触发。

## 5. 分档与提醒频率（用户确认的状态机）

每个「资产 × 规则」保存一份状态：`tier`（已提醒过的最深一档）、`sent_in_tier`（这一档已经发了几次）、`last_sent_at`（上次发送时间）。

每轮检测按以下顺序处理：

设当前档位为 `t`（规则不满足时 `t = 0`）。下面四种情况互斥，按顺序判断：

1. **规则不满足**（`t = 0`）：如果 `tier > 0`，把 `tier` 和 `sent_in_tier` 清零，不发消息。`last_sent_at` **不清零**，第 2 步要用。
2. **从未满足进入满足**（`tier = 0` 且 `t ≥ 1`）。这一步带防抖，是设计方的决定：
   - 如果 `t ≥ 2`，或者距 `last_sent_at` 已满 `repeat_interval`（也包括从没发过），立即提醒，`tier = t`，`sent_in_tier = 1`。
   - 否则（`t = 1` 且还没满间隔）这一轮不发，状态不变，下一轮再判断。
   - 这样处理的原因：价格在阈值附近来回穿越时只会落在第 1 档，每条规则每个 `repeat_interval` 最多一条消息；而一下子跌进第 2 档以上属于真实的急跌，不压。
3. **在本轮行情里跌进更深一档**（`tier ≥ 1` 且 `t > tier`）：立即提醒，`tier = t`，`sent_in_tier = 1`。这里的「更深」是相对本轮已提醒过的最深档 `tier` 说的，不需要额外字段。
4. **仍在已提醒过的档内**（`tier ≥ 1` 且 `1 ≤ t ≤ tier`）：如果 `sent_in_tier < max_per_tier` 且距上次发送已满 `repeat_interval`，重复提醒一次，`sent_in_tier` 加 1。

**合并发送**：同一轮里，同一个资产所有需要发送的规则合成一条消息，AI 也只调用一次。

**发送失败**：状态不更新。下一轮会重新判断，自然重试。

## 6. 数据与存储

- **现价**：按 `sources` 顺序取 ticker，复用 `internal/collector/crypto` 下的 provider。
- **本地 K 线**（SQLite，`data/pricewatch.db`）。极值要滚动更新，只存一个数不够，所以存的是 K 线：
  - **日线**：保存全部历史，用于 `all`、`4y`、`30d`。
  - **5 分钟线**：保留最近 8 天，用于 `7d`、`24h`。
  - 窗口的 H/L 从本地 K 线现算，可以加缓存，但真相源是 K 线。
- **补数**：
  - 首次运行或检测到缺口时，分页补齐。
  - 现有 provider 没有分页：binance 每次最多返回 1000 根，okx 的 `/market/candles` 每次 300 根且只有近期数据。所以长历史回填需要分页，并优先用历史最长的源。BTC 在 binance 有 2017 年以来的数据；历史最高价出现在近几年，不受影响。
  - 日常每轮只补最新的几根。
- **数据不足与数据缺失是两回事**：
  - **历史不够长**：数据源最早的一根 K 线晚于窗口起点，比如新币没有 4 年数据。这种情况把有效窗口缩成「最早一根 → 现在」照常计算，并在消息里注明「窗口数据仅 N 天」。`all` 窗口永远按这种方式处理。
  - **中间有缺口**：在有效窗口内，本应存在却没有的 K 线超过 10%，通常是补数失败。这种情况下这一轮跳过该规则并记日志。
  - 如果某条规则连续被跳过的时间超过 `health.fail_alert_after`，就像取价失败一样触发健康告警（见第 9 节）。
- **AI 用的分钟线**：提醒时现取最近 60 根 1 分钟线，不落库。

## 7. AI 分析

- **调用时机**：只有当一轮里有消息要发时才调用。
- **输入**：
  - 现价
  - 本轮触发的规则（窗口、方向、x、档位）
  - 多周期概况
  - 最近 1 小时的 1 分钟线
  - 最近 24 小时的 5 分钟线
  - 方向提示：只有跌类规则触发时按「低买」分析，只有涨类时按「高卖」，两类都有时两个方向都给。
- **输出**：要求 JSON 格式，解析失败也算 AI 失败。

  ```json
  {"advice": [{"side": "低买", "verdict": "现在下单|再等等",
               "target_price": 0, "term": "1天|3天|7天…", "reasoning": "…"}]}
  ```
- **模型**：使用主配置 `llm` 段指定的 provider。
- **失败处理**：超时（`ai.timeout`）、出错或解析失败时，消息照常发送，AI 结论那一行写「AI 分析不可用」。

## 8. 消息格式（用户确认的顺序）

纯文本，用现有 Telegram notifier 的 `SendText` 发送。示意：

```
🔻 BTC 68,120 USDT（okx）

【AI 结论】低买：再等等 · 建议目标价 66,500 · 期限 3 天

【触发规则】
· 1天窗口：较 24h 最高价 71,040 下跌 4.1%（第 2 档）
· 1周窗口：较 7d 最高价 72,300 下跌 5.8%（第 1 档，第 2/3 次提醒）

【多周期】24h −3.6% · 7d −5.1% · 30d +2.4% · 距历史最高 −31.2% · 距 4 年最低 +312%

【AI 分析】近 30 分钟仍在创新低、5 分钟线未见止跌…（理由全文）

AI 建议仅供参考
```

标题图标：只有跌类规则触发时用 🔻，只有涨类用 🔺，两类都有时用 🔀。

## 9. 运行方式

- `atlas pricewatch run`：常驻进程，按各资产的 `check_interval` 调度。用 launchd plist（`KeepAlive`）托管，放在 `deploy/launchd/`。用常驻进程而不是像 crisis 那样由 launchd 定时触发，是因为间隔要在配置里能改，而且按资产可以不同。
- `atlas pricewatch once [--dry-run]`：只跑一轮。`--dry-run` 把消息打印到终端，不发送，也不写状态。用于调参和自测。
- `atlas pricewatch backfill [--symbol BTC]`：手动补历史 K 线。
- **健康告警**：以下两种情况持续超过 `health.fail_alert_after` 时，发一条「监控异常」消息，恢复后发一条「已恢复」。这样做是为了避免第 2 节说的静默死亡。
  - 任一资产连续取价失败；
  - 任一规则因数据缺口连续被跳过。
- **每日心跳**（用户要求）：每天 `health.heartbeat_at` 发一条「监控仍在运行」。内容包括：各资产的现价和最近一次成功检测的时间，当前处于触发状态的规则及档位，以及过去 24 小时发出的提醒条数。如果某天没收到这条消息，就说明进程停了、机器睡眠了，或者 Telegram 不通，这正是第 2 节反目标中健康告警覆盖不到的那部分。如果机器睡眠错过了发送时刻，醒来后补发一条，当天不重复发。
- **代理**：交易所 provider 用的是默认 `http.Client`，会读 `https_proxy` 环境变量。plist 照 `com.newthinker.atlas.serve.plist` 的写法设 `https_proxy`/`no_proxy`，不给 provider 新增代理参数。
- **配置错误**：配置校验失败时退出码非 0。plist 用 `KeepAlive: {SuccessfulExit: false}` 加 `ThrottleInterval`，避免配置错误时被无限快速重启。

## 10. 留给实现方决定

- 包结构。建议 `internal/pricewatch` 放规则、状态机和存储，`cmd/atlas/pricewatch.go` 放子命令。
- SQLite 表结构、缓存策略、调度器的实现。
- K 线分页的具体实现（直接改 provider，还是在 pricewatch 里分段调用）。
- AI 提示词的措辞。
- 测试：第 4、5 节的规则计算和状态机是纯函数，用表驱动测试覆盖分档、重复、清零、防抖和合并发送；数据源和 AI 用 fake。

## 11. 设计方自行决定的事项（请用户核对）

- 参考价默认用 okx，binance 做备用。
- 本地存 K 线，不只存极值：日线全量，5 分钟线保留 8 天。
- 防抖：规则清零后重新满足时，如果只在第 1 档，要等距上次发送满 `repeat_interval`；直接跌进第 2 档及以上则立即提醒。
- 数据缺口：缺口超过 10% 时这一轮跳过该规则；连续跳过的时间超过 `fail_alert_after` 时发健康告警。
- 同一资产同一轮触发的多条规则合并成一条消息，AI 只调用一次。
- Telegram 配置只接受 `${ENV}` 写法。
- 增加健康告警（连续取价失败 30 分钟）。
- 每日心跳默认在 09:00 发送，包含的内容见第 9 节。
- 交易所代理沿用 launchd plist 里的 `https_proxy` 环境变量。
- AI 输入的走势数据：最近 1 小时的 1 分钟线，加最近 24 小时的 5 分钟线。超时 60 秒。
- 常驻进程 + launchd `KeepAlive`，另提供 `once --dry-run` 和 `backfill` 两个子命令。
