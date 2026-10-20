# QA Review 第 1 轮（常规审查）— pricewatch sprint

- 审查者：qa-pw ｜ 日期：2026-10-20
- 审查对象：`git diff fb1fc62c2a795c470a9baeafd1bdd9502efa317d..53d255acd5368815720f464818f6c8d9eccee987`（分支 feature/pricewatch）
- 取证环境：隔离 worktree（detached @ 53d255acd5368815720f464818f6c8d9eccee987），全部探针/变异经 `go test -overlay` 注入，worktree `git status --porcelain` 每步核对为 0 行；主工作区未改动一个字节
- 基线：`GOTOOLCHAIN=local go test ./internal/pricewatch/ ./cmd/atlas/` 在该 sha 上两包均 ok

## 0. 结论摘要

| 类别 | 结论 |
|---|---|
| 代码质量 | 整体清晰、注释到位、错误都带上下文；无 CRITICAL |
| 安全 | 通过：token 只接受 `${ENV}`（config.go:198-210，错误不回显原值）；Telegram 以纯文本发送（无 parse_mode，AI 文本里的 Markdown 元字符不会致 400）；telegram 网络错误已脱敏（telegram.go `wrapRedacted`）；`data/` 已 gitignore |
| 性能 | 无明显问题；见 R1-5（主回填源不可用时每轮全量重取） |
| 并发 | Runner 单 goroutine 串行；HealthTracker/Engine 快照有锁；无问题 |
| 两个 simplifier 提交 | **语义不变**（逐行对照见 §3） |
| 必修项 | QA-8、QA-9(AD-29)、QA-14 **确认**；新发现 R1-1（健康告警发送失败即永久丢失）、R1-2（配置键拼错静默生效，可静默关掉心跳） |
| 降级 | QA-6 由「必修」降为 LOW（证据见 §2 QA-6） |

## 1. 清单之外的新发现（第 1 轮）

### R1-1 WARNING —「监控异常」一次发送失败即永久丢失（静默死亡）
- 位置：`internal/pricewatch/health.go:51-55`（`Fail` 在返回事件前就置 `s.down = true`）+ `internal/pricewatch/engine.go:293-300`（`track` 发送失败只记 warn）
- 证据（读代码，第 2 轮另有 3 个独立来源实测复现）：`Fail` 第一次满 `After` 时置 `down=true` 并返回 Down；`track` 调 `SendText` 失败 → 只打日志；此后每轮 `Fail` 因 `s.down` 为真返回空事件 ⇒ 不再重发。对 `gap:`/`sync:` 类故障，进程活着、心跳照发且心跳不列出健康故障 ⇒ 用户看到的是「一切正常」。
- 触发条件现实：plist 把 okx/binance/Telegram 都走同一个 `127.0.0.1:7897` 代理，数据侧与发送侧故障高度相关。
- 现有测试只覆盖「发送成功」：`TestCheckAsset_HealthAlerts`（engine_test.go:544）——**本 sprint 同族模式**（行为只在顺利的替身条件下被测）。
- 建议修法：发送成功（或 DryRun 写出）后才确认 down。例：`HealthTracker` 增 `MarkDown(key)`，`Fail` 在 `!s.down && 满 After` 时**每次**返回 Down，`track` 发送成功后调 `MarkDown`；补用例「Telegram 在第 30 分钟失败、第 35 分钟恢复 ⇒ 第 35 分钟补发异常」。
- 归属：**TASK-011**（health.go 接口）+ **TASK-007**（engine.go track）

### R1-2 WARNING — 配置键拼错被静默忽略，可静默关闭心跳
- 位置：`internal/pricewatch/config.go:71`（`v.Unmarshal`，非严格）
- 证据：`heartbeat_at` 拼错 ⇒ 值为空 ⇒ 语义恰为「关闭心跳」；`step_pct` 拼错 ⇒ 0 ⇒「不分档」；资产级 `check_interval` 拼错 ⇒ 静默用全局。均无任何提示（Architect lens 用 overlay 探针实测）。心跳是 spec §9 / AD-28 指定的静默死亡兜底，被一个拼写错误静默关掉。
- 修法可行性已验：我在 overlay 探针里对 `internal/pricewatch/testdata/valid.yaml` 与 `configs/pricewatch.yaml` 分别跑 `viper.UnmarshalExact`，**两者 err=nil**，即改用 `UnmarshalExact` 不破坏现有配置。
- 建议修法：`v.UnmarshalExact(&cfg)`；另在 `heartbeat_at` 为空时启动打一条 Warn。补一条「未知键 ⇒ 报错」用例。
- 归属：**TASK-002**

### R1-3 LOW — `fail_alert_after` / `ai.timeout` 未校验
- 位置：`internal/pricewatch/config.go:96-131`
- 证据（读代码）：`fail_alert_after: 0` 被接受 ⇒ `HealthTracker.After=0` ⇒ 每次瞬时失败都立即一对「异常/已恢复」（刷屏方向）；负值同。`ai.timeout: 0` 被接受 ⇒ `Advisor.Timeout=0` 不设超时，只剩 provider 自己的 HTTP 超时兜底。
- 建议：`fail_alert_after > 0`、`ai.timeout > 0`（ai.enabled 时）纳入校验。
- 归属：TASK-002

### R1-4 LOW — AI 只回一个方向时另一方向静默缺席
- 位置：`internal/pricewatch/advisor.go:122-139`、`engine.go:189-198`（codex 跨模型发现，我读代码确认）
- 证据：涨跌都触发时 `Sides=["低买","高卖"]`；`parseAdvice` 只校验「每条 side 在 allowed 里」，不校验「每个 side 都有」⇒ 只回「低买」也通过；消息里没有「高卖」结论，也不写「AI 分析不可用」。提醒本身照发，不触反目标，故 LOW。
- 建议：缺方向时该方向写「AI 分析不可用」，或整体按 AI 失败处理。
- 归属：TASK-006

### R1-5 WARNING — 主回填源（binance）故障对健康检查不可见，且每轮全量重取历史段
- 位置：`internal/pricewatch/sync.go:173-197`（`fetch`：只要任一源成功就不返回 error，哪怕 0 根）
- 证据：Skeptic lens 实测——binance 恒报错、okx 按真实 `/market/candles` 只给近 1440 根时，`Sync` 返回 nil，7d 窗口缺口 28.6% ⇒ `week_*` 规则跳过约 2 天（冷启动或停机 >5 天后）；`sync:` 健康键永不触发，用户只会收到不指明根因的「数据缺口」告警（而该告警又可能被 R1-1 吞掉）。我补充推理：binance 不可达时上线日永远记不下来（`errs` 使 `leadingEmpty=false`），1d 从 2010 起约 16 个老段每轮都被重取（binance + okx 各一次）；若 binance 表现为超时（10s）而非快速报错，单资产一轮可达数分钟。
- 同族：sync_test.go 的 okx 替身模拟了「两端不含、300 根/次」但**没有 1440 根上限**（`grep 1440 internal/pricewatch/*_test.go` 0 处）。
- 建议：`fetch` 在 best 覆盖 <90% 且有源报错时，照常返回 best 供入库，**同时**返回该错误（或作为 warning 上抛），让 `sync:` 健康键能报出「binance 不可用」；okx 替身加 1440 根上限并补「binance 全挂」用例。
- 归属：**TASK-004**

### R1-6 LOW — 心跳不足以兜底「部分故障」
- 位置：`internal/pricewatch/message.go:176-193`（Architect、Skeptic 两 lens 独立提出）
- 证据：「最近成功」只显示 `15:04`，24h 前成功与刚刚成功同形；心跳不列出当前处于 Down 的健康键；被缺口跳过的规则在心跳里只是「不在触发中」。
- 建议：LastOK 超过 1 个检测周期时带日期/「已 N 小时未成功」；心跳追加「当前异常：…」。
- 归属：TASK-005（格式）+ TASK-007（Snapshot 携带 Down 键）

## 2. 已知清单 QA-1..QA-15 逐条独立判定

| # | Leader 定级 | 我的判定 | 证据 | 归属 |
|---|---|---|---|---|
| QA-1 NaN/Inf 阈值 | review_fix | **确认，LOW** | rule.go:89/92 `<= 0`/`< 0` 对 NaN 恒假，仍在；需用户在 YAML 写 `.nan/.inf` 才触发。修法便宜，随 review_fix 顺手修 | TASK-001 |
| QA-2 SentInTier 2→3 | review_fix | **确认，LOW** | `grep "SentInTier: 2" state_test.go` 0 处 | TASK-001 |
| QA-3 WAL/UTC 变异存活 | 非 DoD | **确认，LOW** | 未变化 | TASK-003 |
| QA-4 Rejects 断言空洞 | 非 DoD | **确认，LOW** | config_test.go 断言处仍是裸 `strings.Contains(err.Error(), c.want)`，未剥路径前缀 | TASK-002 |
| QA-5 X25 5m 键 | 非 DoD | **一半已闭合**：0dbad5f 已把 window.go 改用 `ListingKey`（我读 diff：函数体逐字相同、键格式不变）；**测试一半未补**：window_test.go:93-101 只有「5m 键 → 日线窗口不判 Short」，仍缺「5m 窗口读 5m 键 → 判 Short」，LOW | TASK-010 |
| QA-6 FormatAlert 整条 ≤4096 | **必修** | **降为 LOW**（见下） | | TASK-005 |
| QA-7 舍入跨档外观 | 低 | 确认 LOW | | TASK-005 |
| QA-8 上线日两道保护无测试 | **必修** | **确认，WARNING（必修）** | 我独立复现：sync.go 自 80a56f3 以来**未改**（`git log 80a56f3..HEAD -- sync.go sync_test.go` 为空）；overlay 变异 S29（删 sync.go:94 `leadingEmpty = false`）与 S30（删 sync.go:101）**在现有套件下均 ok（存活）**；用 `.arcforge/docs/02-plan/qa-8-probe.md` 的两个探针：S29 被 `TestProbeListingNotRecordedAllError` 杀、S30 被 `TestProbeListingNotRecordedOnError` 杀；原实现下两探针均 ok | TASK-004 |
| QA-9 段 90% vs 窗口 10%（AD-29） | **必修（人类裁决 A）** | **确认未落地，WARNING（必修）** | sync.go:17 仍只有 `coverageOK = 0.9`，无「与最短窗口相交的段 100%」分支。修法提示：5m 与 24h 相交段要求 100% 时，交易所真实缺 bar 会使该段每轮重取（只是请求成本，规则不受影响，可接受；建议在注释与 discovery 里写明） | TASK-004 |
| QA-10 A42/A04 | 非 DoD | A42 **已闭合**（engine_test.go:511 `TestCheckAsset_TwoSidesAdvice` 断言两方向各一条）；A04 确认 LOW | | TASK-006 |
| QA-11 RunOnce 错误只在替身测 | 非 DoD | **系统级已闭合**：我做变异「`checkAsset` 生产分支吞掉 Engine 错误」（runner.go:84 → `_ = r.Engine.CheckAsset(ctx, a); return nil`）：pricewatch 包**存活**，但 cmd 包 `TestPricewatchOnce_PropagatesCheckError` **杀死**（它走真实 Engine）。包内仍只有替身路径，LOW | TASK-008 |
| QA-12 ExitTimeOut | 低 | 确认 LOW | plist 无 ExitTimeOut（launchd 默认 20s < telegram 单次 30s 超时） | TASK-009 |
| QA-13 回填顺序无断言 | 低 | 确认 LOW | `grep app.syncer cmd/atlas/pricewatch_test.go` 0 处 | TASK-009 |
| QA-14 REPLACE_WITH_ 占位 | **必修** | **确认，WARNING（必修）** | `grep REPLACE_WITH internal/pricewatch cmd/atlas` 0 处；`expandEnv` 只判非空。首装未替换即「所有消息含心跳全部失败、只进 err.log」；心跳缺席本是兜底，但首装用户没有「应该收到心跳」的基线预期。推荐修法同 Leader | TASK-002 |
| QA-15 SetContext 未复原 | 低 | 确认 LOW | cmd/atlas/pricewatch_test.go `TestPricewatchRun_ReturnsNilOnCancel` 调 `pricewatchRunCmd.SetContext(ctx)` 无 Cleanup | TASK-009 |

### QA-6 降级的证据
QA-6 的触发场景是「Term 5000 字」。**该场景在生产路径上已不可达**：
1. `advisor.go:135` term 超过 `maxTermRunes=20` 即整体判 AI 失败（注释直接引用 QA-6，TASK-006 已落第二道防线）；side/verdict 是枚举；
2. `engine.go:189-198` 每个方向只保留第一条 ⇒ FormatAlert 最多收到 2 条 advice；
3. 剩下唯一 AI 可控且不截断的字段是 `target_price`（`formatPrice(1e308)` 约 411 字符）。

实测（overlay 探针，生产配置 testdata/valid.yaml 的 6 条规则全部触发 + 现实行情值 + 两条 advice 的所有 AI 字段取最坏值：term=20 个 emoji、target_price=1e308、reasoning 各 9000 字含代理对）：**整条 = 4096 UTF-16 码元，恰好不超**。探针源码（scratchpad 非持久，贴在此）：

```go
func TestQAPW_QA6WorstCase(t *testing.T) {
	cfg, _ := LoadConfig("testdata/valid.yaml") // 需 PRICEWATCH_TELEGRAM_BOT_TOKEN/CHAT_ID 非空
	a := cfg.CryptoAssets()[0]
	var trig []Trigger
	for _, r := range a.Rules {
		trig = append(trig, Trigger{Rule: r, X: 55.5, Tier: 9, SentInTier: 3, Ref: 126000, ShortDays: 1000})
	}
	term := strings.Repeat("😀", 20)
	adv := []SideAdvice{
		{Side: "低买", Verdict: "现在下单", TargetPrice: 1e308, Term: term, Reasoning: strings.Repeat("理", 9000)},
		{Side: "高卖", Verdict: "现在下单", TargetPrice: 1e308, Term: term, Reasoning: strings.Repeat("😀", 9000)},
	}
	msg := FormatAlert(Alert{Asset: a, Price: 68120, Source: "binance", Triggers: trig,
		Overview: Overview{-3.6, -5.1, 2.4, -31.2, 312}, Advice: adv})
	// 结果：utf16Len(msg) == 4096
}
```

残余风险只在「非 AI 字段」：head（触发规则行数 × ~60 码元，加资产名）由**配置**决定，需约 60+ 条规则同时触发才超；行情值取荒谬量级（1e308）时 head 也会超（我第一次跑用了这种值得 5852，那是不现实输入，已弃用）。⇒ QA-6 降为 **LOW**：建议在 FormatAlert 末尾加一道整条兜底截断（便宜）并补 T34 恰 4096 边界用例，但不构成必修。若 Leader 仍按必修处理，我不反对（修法成本低），只是不应按 CRITICAL 叙述。

## 3. 两个 code-simplifier 提交（HEAD 最后两个）
- **0dbad5f**（TASK-010）：删除私有 `listingKey`，两处调用改 `ListingKey`。两者函数体逐字相同 `"listing:" + pair + ":" + bar`（sync.go:35），键格式不变 ⇒ 语义不变。
- **53d255a**（TASK-005，同时改 TASK-006 的 advisor.go）：抽 `directions`/`refVerb`。逐处对照 alertIcon、Sides、triggerLines、buildAdvicePrompt 四处原内联：分支条件（`== RiseFromLow` 判涨、其余判跌）、默认值、格式串均一致 ⇒ 语义不变。Minimalist lens 独立逐处对照，结论相同。
- 两提交后两包测试全绿（基线即在 53d255a 上跑）。

## 4. 其余 LOW（简述，不阻断）
- engine.go:91-95 `WindowStatsFor` 报错直接 continue，不计入任何健康键且快照照写 `lastOK=now`（TASK-007）。
- engine.go:162-166 发送成功后多条规则状态逐条写，非事务；中途失败下一轮重复提醒（TASK-007/TASK-003）。
- engine.go:201-221 多周期缺数据显示「0.0%」与真实持平同形（TASK-007/TASK-005）。
