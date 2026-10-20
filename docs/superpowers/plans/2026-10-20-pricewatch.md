# Pricewatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 atlas 里新增 `pricewatch` 模块：按配置监控加密货币价格的短/中/长期波动，分档提醒，经 Telegram 发送带 AI 下单建议的消息。

**Architecture:** 新包 `internal/pricewatch`，结构参照 `internal/crisis`：独立 YAML 配置、独立 SQLite、`cmd/atlas` 下的子命令。规则计算和提醒状态机写成纯函数；采价复用 `internal/collector/crypto` 的 provider，AI 复用 `internal/llm`，发送复用 `internal/notifier/telegram`。

**Tech Stack:** Go；viper（配置）；modernc.org/sqlite v1.38.2；cobra（子命令）；launchd（托管）。

**Spec:** `docs/superpowers/specs/2026-10-20-pricewatch-design.md`。动手前先读 spec，计划里的「§N」都指 spec 的章节。

## Global Constraints

- 跑 go 命令一律带 `GOTOOLCHAIN=local`。sqlite 固定为 `modernc.org/sqlite v1.38.2`，不升级。
- 第一版只实现 `assets.crypto`。`stock`/`fund` 下的 items 非空时只打 warn 并跳过（§2）。
- Telegram 的 `bot_token`/`chat_id` 只接受 `${ENV}` 写法：写成明文直接报错退出，展开后为空也报错（§3）。
- AI 出问题时提醒照常发出，AI 段写「AI 分析不可用」（§7）。
- 发送失败时，被发送规则的状态不更新（§5）。
- 消息是纯文本，用 `(*telegram.Telegram).SendText` 发送（§8）。
- 提交信息格式为 `<type>(pricewatch): <描述>`，结尾加 `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`。
- 每次提交前：
  - 先用 `code-simplifier:code-simplifier` 子代理简化本任务改动的文件（用户全局规则）；
  - 再跑 `node .gitnexus/run.cjs detect-changes --scope all --repo .`（项目规则）。
- 修改已有符号之前先跑 gitnexus `impact`。本计划只新增文件，唯一的已有符号改动在 Task 8 的 `cmd/atlas`，只是注册子命令。
- 工作分支 `feature/pricewatch`，在独立的 worktree 里开发。

## Review Focus

1. **阈值边界与浮点误差**：价格正好落在阈值上（如 x = 2.0）时必须触发第 1 档；x = 3.9999999999、阈值 2、档距 2 时应算第 2 档。档位计算要带 1e-9 的容差。→ Task 2 `TestTier_Boundaries`
2. **重复、乱序或重叠的 K 线**：分段拉取时，相邻两段边界上的 K 线会重复，provider 返回的顺序也可能是乱序。入库要按主键幂等，极值不能被重复数据影响。→ Task 3 `TestUpsertCandles_Idempotent`
3. **进程重启发生在一轮行情中途**：重启后不能把同一档再当成「新触发」发一遍。→ Task 3 `TestRuleState_SurvivesReopen` 与 Task 7 `TestCheckAsset_RestartNoDuplicate`
4. **Telegram 发送失败**：状态不推进，下一轮重新发；但同一轮里的清零必须照常落盘。→ Task 7 `TestCheckAsset_SendFailureKeepsState`
5. **provider 返回价格为 0、NaN 或 Inf**：视同这个数据源失败，换下一个源。不能拿 0 去计算跌幅（那会得到 100% 跌幅）。→ Task 7 `TestCheckAsset_BadPriceFallsBack`

---

## File Structure

| 文件 | 职责 |
|---|---|
| `internal/pricewatch/config.go` | 配置类型、`LoadConfig`、校验、规则组合并、`${ENV}` 展开 |
| `internal/pricewatch/rule.go` | `Window`/`RuleType` 常量、跌幅/涨幅计算、档位计算 |
| `internal/pricewatch/state.go` | §5 状态机 `Decide`（纯函数） |
| `internal/pricewatch/store.go` | SQLite：K 线、规则状态、kv |
| `internal/pricewatch/sync.go` | K 线分段补数（逐段换源）、修剪 5 分钟线、窗口统计 |
| `internal/pricewatch/message.go` | 提醒、心跳、健康告警三种消息的文本 |
| `internal/pricewatch/advisor.go` | AI 提示词、调用、JSON 解析 |
| `internal/pricewatch/engine.go` | 单资产一轮检测 `CheckAsset`、健康追踪 |
| `internal/pricewatch/runner.go` | 调度循环、每日心跳 |
| `cmd/atlas/pricewatch.go` | `atlas pricewatch run/once/backfill` 子命令 |
| `configs/pricewatch.yaml` | spec §3 的示例配置 |
| `deploy/launchd/com.newthinker.atlas.pricewatch.plist` | launchd 托管配置 |

所有测试文件放在对应源文件旁边，命名为 `*_test.go`。

---

### Task 1: 配置加载与校验

**Files:**
- Create: `internal/pricewatch/config.go`, `internal/pricewatch/config_test.go`, `internal/pricewatch/testdata/valid.yaml`, `configs/pricewatch.yaml`
- 同时提交 spec：`docs/superpowers/specs/2026-10-20-pricewatch-design.md`

**Interfaces:**
- Produces:
  ```go
  type NotifyPolicy struct {
      RepeatInterval time.Duration `mapstructure:"repeat_interval"`
      MaxPerTier     int           `mapstructure:"max_per_tier"`
  }
  type RuleCfg struct {
      ID, Type, Window string
      ThresholdPct float64 `mapstructure:"threshold_pct"`
      StepPct      float64 `mapstructure:"step_pct"`
      Notify       *NotifyPolicy
  }
  type AssetCfg struct {
      Symbol, Name, Strategy string
      CheckInterval time.Duration `mapstructure:"check_interval"`
      Rules []RuleCfg
  }
  type Config struct {
      CheckInterval  time.Duration
      Storage        struct{ Path string }
      Telegram       struct{ BotToken, ChatID, Proxy string } // 展开后的值
      AI             struct{ Enabled bool; Timeout time.Duration }
      NotifyDefaults NotifyPolicy
      Health         struct{ FailAlertAfter time.Duration; HeartbeatAt string }
      Strategies     map[string][]RuleCfg
      Assets         struct {
          Crypto struct{ Sources []string; Quote string; Items []AssetCfg }
          Stock, Fund struct{ Items []AssetCfg }
      }
  }
  // Asset 是合并、展开后可直接使用的资产
  type Asset struct {
      Symbol   string        // 大写，如 "BTC"
      Name     string
      Pair     string        // Symbol+Quote，如 "BTCUSDT"（crypto.Provider 的入参）
      Quote    string
      Interval time.Duration // 资产自己的 check_interval，没写则取全局
      Rules    []Rule        // 见 Task 2
  }
  func LoadConfig(path string) (*Config, error)
  func (c *Config) CryptoAssets() []Asset
  func (c *Config) UnsupportedItems() []string // stock/fund 下非空项的 "stock:XXX" 列表，供调用方打 warn
  ```
  `Rule` 和 `ParseRule` 在 Task 2 定义。本任务先写在 `rule.go` 里，Task 2 再补全测试；Task 1 和 Task 2 可以合并成一次提交。

- [ ] **Step 1: 写失败测试** `config_test.go`
  - `TestLoadConfig_Valid`：加载 `testdata/valid.yaml`（内容就是 spec §3 的示例，env 由 `t.Setenv` 设置）。断言：
    - BTC 解析出 6 条规则；
    - ETH 的 `day_drop` 阈值是 3，`Interval == 10*time.Minute`；
    - BTC 的 `Interval == 5*time.Minute`；
    - `ath_drawdown.Notify == {24h, 2}`，`week_drop.Notify == {1h, 3}`（取自 `notify_defaults`）；
    - `Pair == "BTCUSDT"`。
  - `TestLoadConfig_SymbolNormalized`：`symbol: btc` 解析后 `Symbol == "BTC"`。
  - `TestLoadConfig_Rejects`：表驱动，每行改坏一处，断言返回 error 且信息里含括号里的字样。
    - 重复的规则 id（`duplicate rule id`）
    - `type: foo`
    - `window: 2d`
    - `threshold_pct: 0`
    - `step_pct: -1`
    - `max_per_tier: 0`
    - `repeat_interval: 0s`
    - 资产没有任何规则
    - 引用了不存在的 strategy
    - `bot_token` 写成明文 `abc`（`${ENV}`）
    - `${PW_TOKEN}` 展开后为空
    - `check_interval: 0s`
    - `heartbeat_at: "25:00"`
  - `TestLoadConfig_Defaults`：`ai.timeout` 不写时为 60s；`health.fail_alert_after` 不写时为 30m；`heartbeat_at: ""` 合法（表示关闭）。
  - `TestUnsupportedItems`：`stock.items` 里有 `600519` 时返回 `["stock:600519"]`，同时 `CryptoAssets()` 的结果不受影响。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestLoadConfig|TestUnsupported'` → FAIL（未定义）。

- [ ] **Step 3: 实现 `LoadConfig`**
  - 用 `viper.New()` + `Unmarshal`，写法同 `internal/crisis/config.go:115`。
  - `${ENV}` 只对 `telegram.bot_token` 和 `telegram.chat_id` 做严格匹配 `^\$\{[A-Za-z0-9_]+\}$`；`telegram.proxy` 原样使用。
  - 规则合并：先取 strategy 的规则，资产 `rules` 中同 id 的整条替换，新 id 追加，顺序保持 strategy 在前。
  - 规则的 `Notify == nil` 时取 `NotifyDefaults`。
  - `configs/pricewatch.yaml` 照抄 spec §3 的示例。

- [ ] **Step 4:** 同一条命令 → PASS。

- [ ] **Step 5: 提交** `feat(pricewatch): 配置格式与校验`（连同 spec 一起提交）

---

### Task 2: 规则计算与提醒状态机

**Files:**
- Create: `internal/pricewatch/rule.go`, `internal/pricewatch/rule_test.go`, `internal/pricewatch/state.go`, `internal/pricewatch/state_test.go`

**Interfaces:**
- Produces:
  ```go
  type RuleType string // "drop_from_high" | "rise_from_low"
  type Window string   // "24h" | "7d" | "30d" | "4y" | "all"
  func (w Window) Span() time.Duration // 24h, 168h, 720h, 1461*24h；all 返回 0，表示不设起点
  func (w Window) Bar() string         // 24h、7d 用 "5m"；30d、4y、all 用 "1d"
  type Rule struct {
      ID string; Type RuleType; Window Window
      ThresholdPct, StepPct float64; Notify NotifyPolicy
  }
  func ParseRule(c RuleCfg, def NotifyPolicy) (Rule, error)
  // Measure 按 §4 计算 x（百分数）；high/low 已包含现价
  func Measure(t RuleType, price, high, low float64) float64
  func Tier(x, thresholdPct, stepPct float64) int // 未达阈值返回 0

  type RuleState struct { Tier, SentInTier int; LastSentAt time.Time }
  type SendKind int // SendNone, SendNew（新触发或进入更深一档）, SendRepeat
  func Decide(prev RuleState, tier int, now time.Time, p NotifyPolicy) (next RuleState, kind SendKind)
  ```

- [ ] **Step 1: 写失败测试**
  - `TestMeasure`：
    - drop：price 50000、high 100000 → 50
    - rise：price 40000、low 20000 → 100（spec §4 的 BTC 例子）
  - `TestTier_Boundaries`（Review Focus 1）：
    - `Tier(2.0,2,2)==1`
    - `Tier(1.9999,2,2)==0`
    - `Tier(3.9999999999,2,2)==2`
    - `Tier(4.0,2,2)==2`
    - `Tier(6.1,2,2)==3`
    - `Tier(55,50,0)==1`（step 为 0 时恒为 1 档）
    - `Tier(100,100,50)==1`、`Tier(150,100,50)==2`
  - `TestWindow`：Span/Bar 的取值同上面的 Interfaces。
  - `TestDecide`：表驱动，覆盖 spec §5 的每个分支。设 `p={1h,3}`，`T0` 为某个固定时刻。
    1. `prev{0,0,zero}`, t=0 → SendNone，状态不变
    2. `prev{2,1,T0}`, t=0 → SendNone，`next{0,0,T0}`（清零但保留 LastSentAt）
    3. `prev{0,0,zero}`, t=1 → SendNew，`next{1,1,now}`
    4. `prev{0,0,T0}`, t=1, now=T0+30m → SendNone，状态不变（防抖）
    5. `prev{0,0,T0}`, t=1, now=T0+1h → SendNew
    6. `prev{0,0,T0}`, t=2, now=T0+1m → SendNew（第 2 档及以上不受防抖限制）
    7. `prev{1,1,T0}`, t=3, now=T0+1m → SendNew，`next{3,1,now}`
    8. `prev{2,1,T0}`, t=2, now=T0+59m → SendNone
    9. `prev{2,1,T0}`, t=1, now=T0+1h → SendRepeat，`next{2,2,now}`（价格回落但仍在提醒过的档内，Tier 保持 2）
    10. `prev{2,3,T0}`, t=2, now=T0+5h → SendNone（已达 max_per_tier）

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestMeasure|TestTier|TestWindow|TestDecide'` → FAIL

- [ ] **Step 3: 实现**
  - `Tier`：`if x+1e-9 < th {return 0}; if step<=0 {return 1}; return 1+int(math.Floor((x-th)/step+1e-9))`
  - `Decide`：严格按 spec §5 的四种情况，按顺序判断。

- [ ] **Step 4:** 同一条命令 → PASS

- [ ] **Step 5: 提交** `feat(pricewatch): 规则计算与分档状态机`

---

### Task 3: SQLite 存储

**Files:**
- Create: `internal/pricewatch/store.go`, `internal/pricewatch/store_test.go`

**Interfaces:**
- Consumes: `core.OHLCV`、`RuleState`
- Produces:
  ```go
  type Store struct{ /* db *sql.DB */ }
  func NewStore(path string) (*Store, error) // DSN 与建目录写法照抄 crisis.NewStore（WAL + busy_timeout）
  func (s *Store) Close() error
  func (s *Store) UpsertCandles(pair, bar string, cs []core.OHLCV) error // PK(pair,bar,ts)，INSERT OR REPLACE
  func (s *Store) LatestCandle(pair, bar string) (time.Time, bool, error)
  // CandleStats 返回 [from, to] 内的 max(high)、min(low)、最早一根的时间、根数；from 为零值表示不设下界
  func (s *Store) Stats(pair, bar string, from, to time.Time) (CandleStats, error)
  func (s *Store) FirstCloseSince(pair, bar string, from time.Time) (float64, bool, error) // 多周期涨跌用
  func (s *Store) PruneCandles(pair, bar string, before time.Time) error
  func (s *Store) GetRuleState(pair, ruleID string) (RuleState, error) // 不存在时返回零值
  func (s *Store) PutRuleState(pair, ruleID string, st RuleState) error
  func (s *Store) GetKV(key string) (string, error) // 心跳日期等
  func (s *Store) PutKV(key, val string) error
  type CandleStats struct { High, Low float64; First time.Time; Count int }
  ```
  时间统一存成 UTC 的 Unix 毫秒。

- [ ] **Step 1: 写失败测试**（用 `t.TempDir()` 建库）
  - `TestUpsertCandles_Idempotent`（Review Focus 2）：同一批 3 根 K 线乱序写入两次，再写入一根时间重复但 high 更高的 → `Count==3`，`High` 取更新后的值。
  - `TestStats_Range`：只统计 [from, to] 内的 K 线；from 为零值时统计全部；空区间返回 `Count==0`。
  - `TestRuleState_SurvivesReopen`（Review Focus 3）：Put `{2,1,T0}` → Close → NewStore 同一路径 → Get 结果相等（时间按毫秒比较）。
  - `TestPruneCandles`：早于 before 的被删，其余保留。
  - `TestKV`：Get 不存在的键返回 `""`，写后可读出。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestUpsert|TestStats|TestRuleState|TestPrune|TestKV'` → FAIL

- [ ] **Step 3:** 实现。三张表：`candles`、`rule_state(pair, rule_id PK, tier, sent_in_tier, last_sent_ms)`、`kv`。

- [ ] **Step 4:** → PASS

- [ ] **Step 5: 提交** `feat(pricewatch): sqlite 存储`

---

### Task 4: K 线补数与窗口统计

**Files:**
- Create: `internal/pricewatch/sync.go`, `internal/pricewatch/sync_test.go`

**Interfaces:**
- Consumes: `crypto.Provider`（`internal/collector/crypto/provider.go`）、`*Store`、`Window`
- Produces:
  ```go
  type Syncer struct { Providers []crypto.Provider; Store *Store; Now func() time.Time }
  // Sync 把 pair/bar 补到当前时刻：从本地最新一根接着补，本地为空时从 backfillStart(bar) 开始
  func (s *Syncer) Sync(ctx context.Context, pair, bar string) error
  type WindowStats struct {
      High, Low float64 // 已并入现价
      Days     int      // 有效窗口的天数（历史不够长时小于窗口长度）
      Short    bool     // 历史不够长（数据源最早一根晚于窗口起点）
      GapRatio float64  // 有效窗口内缺失 K 线的比例
  }
  func WindowStatsFor(st *Store, pair string, w Window, price float64, now time.Time) (WindowStats, error)
  ```

- [ ] **Step 1: 写失败测试**（provider 用 fake，按 `(start, end, interval)` 返回预设数据，并记录调用次数）
  - `TestSync_Chunks`：本地为空，bar="1d"，fake 一次最多返回 300 根 → 分段请求，相邻段首尾衔接，最终入库根数等于 fake 的总根数。
  - `TestSync_FallbackPerChunk`：fake A（排在前面，对应 okx）只有最近 400 天的数据，fake B 有 3000 天 → 老的分段由 B 补上，新的分段用 A。判断标准：一段返回的根数不到该段应有根数的 90%，就换下一个源；所有源都不够时，取根数最多的那个结果。
  - `TestSync_Incremental`：本地已有数据时，只请求「最新一根 → 现在」这一段。
  - `TestSync_Prune5m`：bar="5m" 时同步后删除 8 天以前的数据。
  - `TestWindowStats_Short`：日线只有 100 天，w=4y → `Short==true`、`Days==100`、`GapRatio==0`。
  - `TestWindowStats_Gap`：7d 窗口的 5 分钟线缺了 20% → `GapRatio≈0.2`（误差 0.01）。
  - `TestWindowStats_PriceIncluded`：现价高于所有 K 线的 high 时，`High==price`。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestSync|TestWindowStats'` → FAIL

- [ ] **Step 3: 实现**
  - 每段长度 = 300 × bar 的时长。`backfillStart("1d")` 为 2010-01-01 UTC，`backfillStart("5m")` 为 now−8d。
  - 一段在所有源都返回 0 根时（币还没上线），跳过这一段继续往后补。
  - 有效窗口起点 = max(窗口起点, 本地最早一根)；`all` 的窗口起点为零值。
  - 应有根数 = 有效窗口时长 ÷ bar 的时长。

- [ ] **Step 4:** → PASS

- [ ] **Step 5: 提交** `feat(pricewatch): K 线分段补数与窗口统计`

---

### Task 5: 消息文本

**Files:**
- Create: `internal/pricewatch/message.go`, `internal/pricewatch/message_test.go`

**Interfaces:**
- Produces:
  ```go
  type Trigger struct {
      Rule Rule; X float64; Tier, SentInTier int; Ref float64 // Ref 为窗口最高价（drop）或最低价（rise）
      ShortDays int // >0 时追加「（窗口数据仅 N 天）」
  }
  type Overview struct { Chg24h, Chg7d, Chg30d, FromATH, From4yLow float64 } // 百分数
  type SideAdvice struct {
      Side string `json:"side"`; Verdict string `json:"verdict"`
      TargetPrice float64 `json:"target_price"`; Term string `json:"term"`; Reasoning string `json:"reasoning"`
  }
  type Alert struct {
      Asset Asset; Price float64; Source string
      Triggers []Trigger; Overview Overview; Advice []SideAdvice // Advice 为 nil 表示 AI 不可用
  }
  func FormatAlert(a Alert) string
  func FormatHeartbeat(now time.Time, rows []HeartbeatRow, sent24h int) string
  type HeartbeatRow struct { Asset Asset; Price float64; LastOK time.Time; Active []string } // Active 形如 "day_drop 第2档"
  func FormatHealth(down bool, subject, detail string) string // down=true 为「监控异常」，false 为「已恢复」
  ```

- [ ] **Step 1: 写失败测试**
  - `TestFormatAlert_Golden`：用 spec §8 示例里的数值构造 Alert，输出与下面的字面量**逐字相等**：

    ```
    🔻 BTC 68,120 USDT（okx）

    【AI 结论】低买：再等等 · 建议目标价 66,500 · 期限 3 天

    【触发规则】
    · 1天窗口：较 24h 最高价 71,040 下跌 4.1%（第 2 档）
    · 1周窗口：较 7d 最高价 72,300 下跌 5.8%（第 1 档，第 2/3 次提醒）

    【多周期】24h −3.6% · 7d −5.1% · 30d +2.4% · 距历史最高 −31.2% · 距 4 年最低 +312%

    【AI 分析】近 30 分钟仍在创新低、5 分钟线未见止跌

    AI 建议仅供参考
    ```
    其中 Advice 的 Term 为 "3 天"。
  - `TestFormatAlert_NoAI`：Advice 为 nil 时，AI 结论一行变成 `【AI 结论】AI 分析不可用`，并且不出现 `【AI 分析】` 段。
  - `TestFormatAlert_Icons`：只有 rise → 🔺；drop 和 rise 都有 → 🔀。rise 的规则行写成 `较 7d 最低价 X 上涨 Y%`。
  - `TestFormatAlert_TwoSides`：两条 Advice 各占一行结论，【AI 分析】按「低买：…」「高卖：…」分行列出。
  - `TestFormatAlert_ShortWindow`：`ShortDays=100` 的行尾追加「（窗口数据仅 100 天）」。
  - `TestFormatHeartbeat`、`TestFormatHealth`：断言包含关键字（「监控仍在运行」、资产名、「监控异常」、「已恢复」）。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestFormat'` → FAIL

- [ ] **Step 3: 实现**
  - 窗口的中文名：24h→「1天」、7d→「1周」、30d→「30天」、4y→「4年」、all→「历史」。参照价的标签：24h、7d、30d、4 年、历史。
  - 价格：带千分位；≥1000 不保留小数，≥1 保留 2 位，<1 保留 4 位。
  - 百分比：保留 1 位小数，并带正负号。负号用「−」（U+2212），与 spec 一致；`From4yLow` 不保留小数。
  - SentInTier > 1 时，档位写成「第 t 档，第 k/max 次提醒」。

- [ ] **Step 4:** → PASS

- [ ] **Step 5: 提交** `feat(pricewatch): 提醒/心跳/健康消息格式`

---

### Task 6: AI 分析

**Files:**
- Create: `internal/pricewatch/advisor.go`, `internal/pricewatch/advisor_test.go`

**Interfaces:**
- Consumes: `llm.Provider`（`internal/llm/interface.go`）、`Trigger`、`Overview`、`SideAdvice`、`core.OHLCV`
- Produces:
  ```go
  type AdviceInput struct {
      Asset Asset; Price float64; Triggers []Trigger; Overview Overview
      M1 []core.OHLCV // 最近 1 小时的 1 分钟线（调用方现取，不入库）
      M5 []core.OHLCV // 最近 24 小时的 5 分钟线
  }
  type Advisor struct { LLM llm.Provider; Timeout time.Duration }
  func (a *Advisor) Advise(ctx context.Context, in AdviceInput) ([]SideAdvice, error)
  func Sides(ts []Trigger) []string // 只有跌 → ["低买"]，只有涨 → ["高卖"]，都有 → ["低买","高卖"]
  ```

- [ ] **Step 1: 写失败测试**（fake `llm.Provider`）
  - `TestAdvise_ParsesJSON`：fake 返回 `{"advice":[{"side":"低买","verdict":"再等等","target_price":66500,"term":"3天","reasoning":"..."}]}` → 解析正确。断言请求里 `JSONMode==true`，并且 Messages 中包含现价和触发规则的 id。
  - `TestAdvise_Invalid`：表驱动，以下情况都返回 error：
    - 内容不是 JSON；
    - `advice` 为空；
    - verdict 不是「现在下单」或「再等等」；
    - side 不在 `Sides(triggers)` 里。
  - `TestAdvise_Timeout`：fake 阻塞直到 ctx 结束，`Timeout=50ms` → 在 1 秒内返回 error。
  - `TestSides`：三种组合。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestAdvise|TestSides'` → FAIL

- [ ] **Step 3: 实现**
  - 提示词措辞由实现者决定。但必须写明：用户在做 okx 双币赢、要分析的方向（`Sides`）、要求的输出 JSON 结构，以及 K 线（压缩成 `时间,开,高,低,收` 一行一根）。
  - `MaxTokens` 取 1500，`Temperature` 取 0.3。

- [ ] **Step 4:** → PASS

- [ ] **Step 5: 提交** `feat(pricewatch): AI 下单建议`

---

### Task 7: 单资产一轮检测与健康追踪

**Files:**
- Create: `internal/pricewatch/engine.go`, `internal/pricewatch/engine_test.go`

**Interfaces:**
- Consumes: 前面各任务的全部产物
- Produces:
  ```go
  type Sender interface{ SendText(text string) error } // *telegram.Telegram 天然满足
  type Engine struct {
      Cfg *Config; Store *Store; Syncer *Syncer; Providers []crypto.Provider
      Advisor *Advisor // nil 表示 AI 关闭
      Sender Sender; Now func() time.Time; Log *zap.Logger
      DryRun bool; Out io.Writer // DryRun 时把消息写到 Out，不发送，也不写规则状态（K 线照常入库）
  }
  func (e *Engine) CheckAsset(ctx context.Context, a Asset) error
  func (e *Engine) Snapshot() []HeartbeatRow // 心跳用：各资产最近一次的价格、成功时间、触发中的规则
  func (e *Engine) Sent24h() int
  ```
  健康状态（连续失败从何时开始、是否已经告警过）只放内存，进程重启后清空。这是我自己的决定，spec 没有规定。

- [ ] **Step 1: 写失败测试**（fake provider、fake Sender，Store 用临时库，Now 可控）
  - `TestCheckAsset_TriggersAndMerges`：同一轮同时满足 `day_drop` 和 `week_drop` → Sender 只收到 1 条消息，消息里两条规则都在；两条规则的状态都推进到 `{tier,1,now}`。
  - `TestCheckAsset_SendFailureKeepsState`（Review Focus 4）：Sender 返回 error → 被发送规则的状态不变；同一轮里某条规则从 tier 2 回落到不满足，这次清零照常落盘；下一轮 Sender 恢复正常后补发。
  - `TestCheckAsset_RestartNoDuplicate`（Review Focus 3）：第一轮发出 → 用同一个库新建 Engine → 价格不变的第二轮（未满 repeat_interval）不发送。
  - `TestCheckAsset_BadPriceFallsBack`（Review Focus 5）：第一个 provider 依次返回 Price 0、NaN、Inf（三个子测试），第二个返回正常价格 → 用第二个源的价格，消息里的来源名是第二个源。
  - `TestCheckAsset_AIFailureStillSends`：Advisor 返回 error → 照常发送，并且包含「AI 分析不可用」。
  - `TestCheckAsset_GapSkipsRule`：7d 窗口缺口 20% → week_* 规则本轮跳过、不发送，day_* 规则正常。
  - `TestCheckAsset_HealthAlerts`：
    - 所有源连续失败，`fail_alert_after=30m` → 第 29 分钟没有告警，第 30 分钟发 1 条「监控异常」，之后不重复；
    - 恢复后发 1 条「已恢复」；
    - 规则因缺口被连续跳过的情况同理。
  - `TestCheckAsset_DryRun`：消息写到 Out，Sender 没被调用，规则状态没变。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ -run 'TestCheckAsset'` → FAIL

- [ ] **Step 3: 实现**，`CheckAsset` 的顺序如下：
  1. 按 `Cfg.Assets.Crypto.Sources` 的顺序取价，价格必须 > 0 且是有限值；
  2. 同步 `1d` 和 `5m` 的 K 线，出错只记日志；
  3. 对每条规则：先算 `WindowStatsFor`，`GapRatio > 0.10` 就跳过；否则依次算 `Measure` → `Tier` → `Decide`；
  4. 不需要发送的状态变化（包括清零）立即落盘；
  5. 有需要发送的规则时：
     - 算 Overview：用 `FirstCloseSince` 算 24h/7d/30d 的涨跌，用 all 和 4y 窗口算距历史最高、距 4 年最低；
     - Advisor 不为 nil 时，现取 1 分钟线（60 根）调用 Advisor；
     - 生成消息并发送，发送成功后才写入被发送规则的状态。

- [ ] **Step 4:** → PASS；再跑一遍 `GOTOOLCHAIN=local go test ./internal/pricewatch/ -race`，必须 PASS。

- [ ] **Step 5: 提交** `feat(pricewatch): 单资产检测流程与健康告警`

---

### Task 8: 调度、心跳、子命令与部署

**Files:**
- Create: `internal/pricewatch/runner.go`, `internal/pricewatch/runner_test.go`, `cmd/atlas/pricewatch.go`, `cmd/atlas/pricewatch_test.go`, `deploy/launchd/com.newthinker.atlas.pricewatch.plist`

**Interfaces:**
- Consumes: `Engine`、`Store.GetKV/PutKV`、`FormatHeartbeat`；cmd 侧还用到 `loadConfigOrDefaults()`、`ensurePolicyGate()`、`factory.New(cfg.LLM)`、`telegram.New(token, chat, telegram.WithProxy(p))`、`okx.New()`、`binance.New()`
- Produces:
  ```go
  type Runner struct { Engine *Engine; Assets []Asset; HeartbeatAt string; Tick time.Duration }
  func (r *Runner) Run(ctx context.Context) error // ctx 取消时返回 nil
  func (r *Runner) RunOnce(ctx context.Context) error // 所有资产各跑一轮
  func dueHeartbeat(now time.Time, at string, lastDate string) bool // 本地时区；当天已过 at 并且 lastDate != 今天
  ```
  子命令（照 `cmd/atlas/crisis.go` 的写法注册）：
  - `atlas pricewatch run`
  - `atlas pricewatch once [--dry-run]`
  - `atlas pricewatch backfill [--symbol BTC]`
  - 持久 flag `--pricewatch-config`，默认 `configs/pricewatch.yaml`

- [ ] **Step 1: 写失败测试**
  - `TestDueHeartbeat`：
    - at="09:00"：08:59 → false；09:00 且 last="" → true；当天已发 → false；
    - 第二天 10:30（睡眠错过了 09:00）→ true；
    - at="" → 恒为 false。
  - `TestRunner_Schedules`：Tick=10ms，两个资产的 Interval 分别为 30ms 和 90ms，跑约 200ms → 前者的检测次数明显多于后者（断言前者 ≥ 2 × 后者）。
  - `TestRunner_HeartbeatOncePerDay`：可控 Now 跨过 09:00 两次 tick → Sender 只收到 1 条心跳，kv 里 `heartbeat_date` 为当天日期。
  - `TestPricewatchCmd_Registered`：rootCmd 下能找到 `pricewatch`，并且有 `run`、`once`、`backfill` 三个子命令。

- [ ] **Step 2:** `GOTOOLCHAIN=local go test ./internal/pricewatch/ ./cmd/atlas/ -run 'TestDueHeartbeat|TestRunner|TestPricewatchCmd'` → FAIL

- [ ] **Step 3: 实现**
  - Runner 每次 tick 检查各资产是否到期，到期的依次执行 `CheckAsset`，然后检查心跳。
  - cmd 的装配顺序：
    1. `ensurePolicyGate()` 必须在构造 provider 之前调用（见 `cmd/atlas/policy.go:21` 的注释）；
    2. 按 `sources` 名字构造 provider，目前只支持 `okx` 和 `binance`，其他名字报错；
    3. 对 `UnsupportedItems()` 的每一项打 warn；
    4. 如果 `ai.enabled` 为 true 但 `factory.New` 失败，打 warn 后不带 AI 运行；
    5. `run` 收到 SIGTERM 时取消 ctx。
  - `backfill` 对指定资产（不指定时对全部资产）执行 `Syncer.Sync`（1d 和 5m），并打印入库根数。
  - plist 照 `deploy/launchd/com.newthinker.atlas.serve.plist` 的写法：
    - 设置 `https_proxy` 和 `no_proxy`；
    - `KeepAlive` 用 `{SuccessfulExit: false}`，`ThrottleInterval` 设 60；
    - `ProgramArguments` 为 `atlas -c configs/config.yaml pricewatch run`；
    - 写入 `PRICEWATCH_TELEGRAM_BOT_TOKEN` 和 `PRICEWATCH_TELEGRAM_CHAT_ID` 这两个环境变量的占位。

- [ ] **Step 4:** → PASS；然后跑 `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./internal/pricewatch/... ./cmd/atlas/...`，全部 PASS。

- [ ] **Step 5: 手动冒烟**：设好两个 env 后跑 `GOTOOLCHAIN=local go run ./cmd/atlas -c configs/config.yaml pricewatch backfill --symbol BTC`，再跑 `... pricewatch once --dry-run`。
  - 期望：backfill 打印的日线根数超过 3000；dry-run 打印出指标，有规则满足时打印出消息文本；Telegram 不收到任何消息。

- [ ] **Step 6: 提交** `feat(pricewatch): 调度、心跳、子命令与 launchd`
