# TASK-009 验证报告（pricewatch 子命令与 launchd）

> 恢复说明：本文件是 test-pw-b 于 2026-10-20T12:17Z 判 VERIFIED 时的报告原文。它在 12:51:49Z 被 test-pw-a 的报告覆盖（见文末附节），12:5xZ 经 Leader 要求由 test-pw-b 原样恢复。§1–§7 一字未改，未重新运行，结论未改。

- 验证者：test-pw-b（由 Leader 经逃生边 `verifying → verifying` 从 test-pw-a 改派；`verify_baseline` 未刷新）；`assignment_epoch`=1
- **被测树**：`feature/pricewatch` @ `5fc087c921c58c7e23dab9d5dc1b27bb5ac5fcfb`（= `verify_baseline.head`；出裁决前主仓库 HEAD 未变），我自建的独立 worktree `../wt-verify-TASK-009-b`（detached）。没有动 test-pw-a 的 `../wt-verify-TASK-009`
- 文件 sha256（= dev 提交 `e292f1690ac6f229bb4103b1984df8ffb13ff79f`）：`cmd/atlas/pricewatch.go` `c92b0cbe54122f5acc3e483cf4cff424a19504e1226f8e6c6cdb277e63a7837b`、`cmd/atlas/pricewatch_test.go` `b193635f86a7a05b988f0954fa430cd3399273c2f6df9237b26761713588f9c0`、`deploy/launchd/com.newthinker.atlas.pricewatch.plist` `1529eee9a89cad5b340be2573d6a6667d723a87cdb1c3e97b38b3d631d1ed378`
- discovery sha256 `e530320e16f9b5f7e131ca84d50340446deb4f0485d8a18754483434f24588f4` = 基线，无漂移
- 范围：`e292f16 feat(TASK-009)` 只改 `writes` 声明的三个文件
- **test-pw-a 的半成品**（testpwa-t009-mut/、testpwa-t009-suite.txt、testpwa-t009.cov）**没有采信、也没有使用**。本报告的所有数字都是我在自己的 worktree 上重新采的
- 工具链：go1.24.4 darwin/arm64，`GOTOOLCHAIN=local`

## 结论：VERIFIED

## 1. 门禁（non_functional[0]）

| 命令 | 结果 |
|---|---|
| `go build ./...` | rc=0 |
| `go test -count=1 ./internal/pricewatch/... ./cmd/atlas/...` | 两包都输出 `ok`（pricewatch 2.7s、cmd/atlas 2.6s） |
| `go vet ./cmd/atlas/` | 无输出，rc=0 |
| `gofmt -l pricewatch.go pricewatch_test.go` | 无输出 |

### cmd/atlas 覆盖率：两把独立的尺

| 尺 | 口径 | 结果 |
|---|---|---|
| ① `go test -cover` 报数 | 工具链 | 79.9% |
| ② 自写脚本，按 profile 逐块求和（同一块取最大计数去重） | 全部语句 | **1410/1764 = 79.93%** |
| ③ 自写 `go/ast` 程序：用每个 FuncDecl 的函数体行区间划分 profile 块 | FuncDecl 内的语句 | **1410/1754 = 80.39%**（= `go tool cover -func` 的 total 80.4%） |

- ② 与 ③ 之差恰好 10 条语句，都落在 FuncDecl 之外（package 级 var 的函数字面量），**全部未覆盖**：pricewatch.go:39/40/42–44/46–50/51 共 7 条（三个测试注入点的真实实现），version.go:18 共 3 条（已有代码）。这与 dev 的说法一致，现在由一个不依赖 `go tool cover` 的 AST 划分独立复现。
- 三个数都 ≥ coverage_floor 78。
- pricewatch.go 本文件 85/100 = 85.0%。未覆盖的是注入点的真实实现、UnsupportedItems 告警、NewStore/Stats/Sync 出错分支、logger 回退，以及 `cmd.Context()==nil` 和 `stop` 两个分支。

## 2. done_criteria 覆盖矩阵

| # | 完成标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `pricewatch` 下有 run/once/backfill；`--pricewatch-config` 是持久 flag、默认 `configs/pricewatch.yaml`；once 有 `--dry-run`、backfill 有 `--symbol` | test | `TestPricewatchCmd_Registered`；P19（默认值）、P20（非持久）、P22（不注册 backfill）KILLED | PASS |
| functional[1] | 取价顺序 okx、binance；回填顺序 binance、okx（AD-20）；`kraken` 报错且含名字；ai 关闭 → Advisor nil；工厂失败 → nil 并记 1 条 warn | test | `TestPricewatchProviders_Order`（两种 sources 顺序 + 共用实例）、`_Unknown`、`TestPricewatchAdvisor`（observer 断言恰好 1 条 warn）；P03/P04/P05（排序）、P06/P07（未知源）、P08/P09/P10（AI）KILLED | PASS（另见 §5-1 的装配层发现） |
| functional[2] | plist（AD-27） | manual | §3 逐项核对 | PASS |
| error_handling[0] | 配置不存在或校验失败时 once 报错且不发消息；backfill `--symbol` 不存在时报错，大小写不敏感 | test | `TestPricewatchOnce_BadConfig`（文件不存在/校验失败/未知源 × dry-run 开关两种，断言 error、sender 未构造、无输出）、`TestPricewatchBackfill_Symbol`；P13/P14/P15/P16、P18（加载配置前就构造 sender）、P21 KILLED | PASS |
| error_handling[1] | `ensurePolicyGate()` 先于任何 provider 构造 | review | §4-1；P01/P02 被辅助测试 GateFirst 抓到 | PASS |
| error_handling[2] | run 收到 SIGTERM/SIGINT 时取消 ctx 并以 nil 返回 | review | §4-2；P17/P24 被辅助测试 ReturnsNilOnCancel 抓到 | PASS |
| non_functional[0] | build/test/vet；覆盖率 ≥78 | test | §1 | PASS |
| non_functional[1] | 冒烟（AD-19） | manual | §6，我自己重跑 | PASS |
| QA-11（非 DoD） | once 把 RunOnce 的错误作为命令错误返回 | test | `TestPricewatchOnce_PropagatesCheckError`；P11 KILLED | PASS |

## 3. plist 逐项核对（functional[2]，manual）

- `plutil -lint`：**OK**
- 用 `plutil -convert json` 提取后核对：
  - ProgramArguments = `[/Users/zuowei/workspace/runtime/atlas/bin/atlas, --config, …/configs/config.yaml, pricewatch, run, --pricewatch-config, …/configs/pricewatch.yaml]`。首项是绝对路径 ✓，含连续的 `pricewatch run` ✓，配置路径都是绝对路径
  - WorkingDirectory `/Users/zuowei/workspace/runtime/atlas` ✓
  - KeepAlive `true`、ThrottleInterval `60` ✓（AD-27 人类裁决）；RunAtLoad true；未设 ExitTimeOut
  - EnvironmentVariables：http_proxy/https_proxy = `http://127.0.0.1:7897`，no_proxy = `localhost,127.0.0.1` ✓
  - `PRICEWATCH_TELEGRAM_BOT_TOKEN=REPLACE_WITH_BOT_TOKEN`、`PRICEWATCH_TELEGRAM_CHAT_ID=REPLACE_WITH_CHAT_ID`，只是占位 ✓；按 bot token 的形态（`数字:长串`）搜索，命中 0 条
- 首项、WorkingDirectory、https_proxy 与 `com.newthinker.atlas.serve.plist` 逐字一致
- `configs/pricewatch.yaml` 的 telegram 只用 `${PRICEWATCH_TELEGRAM_*}`，变量名与 plist 一致

## 4. review 项

1. **e[1] 闸门先于构造**：`okx.New()` / `binance.New()` 在 cmd/atlas/pricewatch.go 里只出现在 `pricewatchProviderCtors` 的两个函数字面量中（internal/pricewatch 里 0 处）。这两个字面量只在 `pricewatchProviders` 里被调用，而该函数第一行就是 `ensurePolicyGate()`。run/once/backfill 三个入口都经由 `newPricewatchApp` 走到它（只有 185、198、221 三个调用点），没有绕过的路径。**PASS**
2. **e[2] 信号**：`runPricewatchRun` 用 `signal.NotifyContext(parent, syscall.SIGTERM, syscall.SIGINT)` 得到 ctx，交给 `Runner.Run(ctx)` 并原样返回其结果。TASK-008 已验证 Runner 在 ctx 取消时返回 nil。`main.go` 在 Execute 返回 nil 时不调用 `os.Exit(1)`，所以退出码为 0。cmd/atlas 里另一处 `signal.Notify` 在 serve.go，与本命令无关。**PASS**。变异 P25（只监听 SIGINT）存活是预期的：这是 review 项，测试只能验证「ctx 取消 → nil」，验证不了是哪个信号触发的取消。我是读代码确认 SIGTERM 在列表里的。

## 5. 独立变异（32 个，`go test -overlay`，隔离 worktree）

- 30 个首轮变异加 2 个修正版（P26/P27 首版因变量未使用编译失败，改为 P26b/P27b）。每个变异先过 `go vet -overlay`。源文件 sha256 跑前跑后一致；worktree `git status --porcelain` 前后均为 0 行。
- **最终 26 KILLED / 4 SURVIVED**（首轮计数自洽：26+2+2=30）。dev 自报 20/20：我的变异集覆盖了它全部 20 个形态，结果一致；存活的 4 个都在它的集合之外。

| 存活变异 | 判断 |
|---|---|
| P25 只监听 SIGINT | review 项，见 §4-2 |
| P28 去掉 UnsupportedItems 的 warn | 非 DoD：stock/fund 跳过时的告警没有测试 |
| P26b **Syncer 用取价顺序**（okx 在前）回填 | **非 DoD，低严重度**，见下 |
| P27b Engine 用回填顺序取价 | **等价变异**：Engine 按 `Cfg.Assets.Crypto.Sources` 的名字查 provider，与 `Providers` 的顺序无关（TASK-007 decisions 已写明，TASK-007 的 E15 守着这一点） |

1. **装配层：回填顺序真正接到 Syncer 上这一步没有测试**（P26b）。DoD f[1] 写的是「装配 helper 单测」，helper `pricewatchProviders` 返回的两种顺序已被 P03–P05 守住，**字面要求满足**。但 `newPricewatchApp` 里把 `backfill` 交给 `Syncer.Providers` 这一行，换成 `price` 也全绿。这与 TASK-008 的 R22 同形（helper/替身测了，生产接线没测）。**后果有限**：Syncer 逐段按 Providers 顺序取数，报错或不足 90% 就换下一个源（sync.go:51），okx 前面的历史段会回退到 binance，数据不会缺失。代价是每段多一次注定失败的 okx 请求（耗配额），并且近期段改由 okx 供数。建议补一个断言 `app.syncer.Providers` 的顺序为 binance、okx。
2. **plist 占位如果没替换就安装，会静默失效**：`REPLACE_WITH_BOT_TOKEN` 是非空字符串，能通过「`${ENV}` 展开后不为空」的校验。进程正常启动，但每次发送都失败（只记 Warn），连健康告警和心跳也发不出去，正撞上 spec §2「静默死亡」这个反目标。plist 注释已写明要替换。建议在配置校验里拒绝 `REPLACE_WITH_` 前缀，或在 run 启动时发一条自检消息、失败就退出。非 DoD。
3. 冒烟时 cwd 下没有生成 `data/collector-quota.json`（discovery 说不带 -c 时账本会落在 cwd 的 data/ 下）。我的冒烟 cwd 里没有 data/ 目录。不影响 DoD，只是与 discovery 的描述不符，未深究。
4. 测试 `TestPricewatchRun_ReturnsNilOnCancel` 调用 `pricewatchRunCmd.SetContext(ctx)` 之后没有复原，ctx 会留在全局命令上。目前包内没有别的测试依赖它，属于 AD-16 那类顺序依赖的潜在来源。非 DoD。

## 6. 冒烟（non_functional[1]，我自己重跑）

- 时间 2026-10-20 约 12:10Z。二进制：在 worktree @ `5fc087c921c58c7e23dab9d5dc1b27bb5ac5fcfb` 执行 `go build -o <scratchpad>/test-pw-b-T009-smoke/atlas ./cmd/atlas`
- 准备：cwd = `<scratchpad>/test-pw-b-T009-smoke`；临时配置复制自 `configs/pricewatch.yaml`，只改两处（`diff` 核对过）：`ai.enabled: false`、`storage.path` 指向临时目录下的 pw.db。**不带 -c**。环境 `PRICEWATCH_TELEGRAM_BOT_TOKEN=placeholder PRICEWATCH_TELEGRAM_CHAT_ID=placeholder`，环境里有 https_proxy
- `./atlas pricewatch backfill --symbol BTC --pricewatch-config <tmp>/pricewatch.yaml`：**rc=0，用时 8s**，stdout 为「`BTC 1d: 3352 根`」「`BTC 5m: 2304 根`」，stderr 0 行。**1d 3352 > 3000 ✓**
- `./atlas pricewatch once --dry-run --pricewatch-config <tmp>/pricewatch.yaml`：**rc=0，用时 8s**，stderr 0 行。stdout 有两条提醒：
  - 「🔺 BTC 81,393 USDT（okx）… · 4年窗口：较 4 年最低价 15,476 上涨 425.9%（第 7 档）… 【多周期】24h −1.4% · 7d −2.3% · 30d −6.0% · 距历史最高 −35.5% · 距 4 年最低 +426%」
  - 「🔀 ETH 2,384 USDT（okx）… 历史窗口 下跌 51.9%（第 1 档）/ 4年窗口 上涨 122.1%（第 1 档）/ 1周窗口 下跌 5.8%（第 1 档）」
  - 末尾两行：「BTC 81392.6 USDT · 触发中：cycle_rise 第7档」「ETH 2384.39 USDT · 触发中：ath_drawdown 第1档、cycle_rise 第1档、week_drop 第1档」
  - AI 结论行为「AI 分析不可用」（ai.enabled:false）✓
  - **未调用 Telegram 的依据**：token 是占位值，真调用必然失败，Engine 会记 Warn（zap production 写到 stderr），而 stderr 是 0 行；另外代码层面，DryRun 时 Engine 只写 Out（TASK-007 已验证，m5/m6 KILLED）
- 网络可达，正常完成。与 dev 在 10:40Z 的冒烟数字一致（1d 3352、5m 2304；规则命中集合相同，价格随时间小幅变动）

## 7. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-009-b 5fc087c921c58c7e23dab9d5dc1b27bb5ac5fcfb
cd ../wt-verify-TASK-009-b
GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test -count=1 ./internal/pricewatch/... ./cmd/atlas/... && GOTOOLCHAIN=local go vet ./cmd/atlas/
GOTOOLCHAIN=local go test -count=1 -coverprofile=c.out ./cmd/atlas/ && go tool cover -func=c.out | tail -1
GO111MODULE=off go run <scratchpad>/test-pw-b-T009-ruler/main.go c.out cmd/atlas      # AST 第二把尺
python3 <scratchpad>/test-pw-b-T009-mut/run.py "$PWD" <输出目录>                      # 30 个变异（P26b/P27b 另跑）
plutil -lint deploy/launchd/com.newthinker.atlas.pricewatch.plist
```

---

## 附：test-pw-a 的独立验证（改派后完成，12:51Z）

- **来源**：test-pw-a 在被 Leader 经逃生边改派之后醒来，按它自己的 checkpoint 完成了一次独立验证，并于 **2026-10-20T12:51:49Z** 用它的报告**覆盖**了本文件（覆盖事故：写通道 `doc` 子命令是全量覆盖写，不校验写者是否为当前 verifier）。任务状态没有受影响，仍是 `verified`（by test-pw-b，12:17:14Z）。Leader 已 TaskStop test-pw-a，并把它那份原样备份在 `/private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/b3c3a6ba-7de9-492d-9e24-953680e5a079/scratchpad/leader-pw/TASK-009-verification.test-pw-a.md`。以下只是摘要，**本节是转述，不是 test-pw-b 的验证结论**，细节以备份文件为准。
- 被测树相同：`5fc087c921c58c7e23dab9d5dc1b27bb5ac5fcfb`，隔离 worktree `../wt-verify-TASK-009`。
- **结论：VERIFIED**。F1–F3、E1–E3、N1、N2 全部 PASS；QA-11 PASS。
- 门禁与覆盖率：两包 `ok`；79.9% / -func 80.4% / profile 逐块 1410/1764=79.93%，与本报告一致。
- 变异：29 个，**KILLED 22 / SURVIVED 7**（P18、P19 信号层，review 项；P24 等价；P25、P27、P28、P29 非 DoD）。
- 冒烟（12:48Z 本人重跑）：backfill `BTC 1d: 3352 根`、`BTC 5m: 2304 根`，rc=0；once --dry-run rc=0，两条提醒，stderr 0 行。另外对 `run` 进程做了真实信号实测：SIGTERM 退出码 0、0.03s；SIGINT 退出码 0、0.04s（Telegram 代理指向本机不可达端口，不对外发请求）。
- 它报告的非 DoD 发现：
  - **P28：run 的心跳接线（`runner()` 把 `cfg.Health.HeartbeatAt` 传给 Runner）没有测试**。删掉它全部测试照绿，后果是生产里每日心跳永远不发。**本报告没有覆盖这一条**（我的变异集里没有这个形态），是它独有的发现，价值较高。
  - **P25：Syncer 用回填序这一接线没有测试**。这与本报告 §5-1 的 P26b 是同一条（编号不同）。
  - plist 未设 ExitTimeOut；Tick、UnsupportedItems warn 不是 DoD 要求。
- 它记录的判定时 HEAD 漂移：主仓库已前移到 `56c7c6f1cc122e54302d076bc90d2f648ea089a5`，本任务三个文件无变化。其中 `769bdbd`（TASK-002，QA-14）新增了对 `REPLACE_WITH_` 占位值的拒绝。按它的转述，**本报告 §5-2 的「占位静默失效」在该提交之后已改为会出声的失效**。这是 test-pw-a 的观察，我没有重新验证。
