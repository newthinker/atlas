# Sprint M1d（043）第一轮 Code Review · 常规审查

- 审查者：qa-m1d　时间：2026-09-03　审查对象：`ae088eb253b64b36e10558a02587e3fa657f5f3e..f3d6eb282c83e0ca730b1713907f5220114ee86b -- internal/hestia cmd/atlas configs deploy`（14 文件 +1427/−41）
- 实测环境：detached worktree `../wt-qa-m1d` @ `f3d6eb282c83e0ca730b1713907f5220114ee86b`，`GOTOOLCHAIN=local`，主工作区未碰
- 证据分档：**[核实]** = 我在 worktree 上独立跑过或读过代码逐行确认；**[机制]** = 验证报告/CONTRACTS 有变异或数字，我核过机制未重跑；**[转述]** = 仅引用他人报告

## 1. 结论

**第一轮：0 CRITICAL / 2 WARNING / 5 SUGGESTION。** 两条 WARNING 都不在 M1d 的 DoD 范围内、也不是本 sprint 改动文件引入的缺陷（一条在 `internal/notifier/telegram`，一条是 spec §4.1 明写的「静默降级」形态的可观测性代价），故**不建议以它们退回任何任务**；建议作为交付后待办开小任务。

## 2. 门禁与契约（全部 [核实]，锚 `f3d6eb2`）

| 项 | 实测 |
|---|---|
| `go test ./internal/hestia/... ./cmd/atlas/... -count=1 -cover` | 两包 ok；`internal/hestia` **96.4%**（门槛 ≥ 96.3）；`cmd/atlas` **76.2%**（floor 75） |
| `go vet ./internal/hestia/... ./cmd/atlas/...` | 零输出 |
| 五个不动文件 `git diff --stat ae088eb f3d6eb2 -- store.go validate.go parse.go extract.go fields.go` | 空 |
| `go.mod` / `go.sum` | 无 diff（无新依赖） |
| 导出面 | **零新增导出函数/方法**（`TestPackageExposesNoWriteFunctions` 绿）；新增导出**类型** `hestia.Sender` 与三个导出字段 `IngestDeps.Notify` / `IngestDeps.OnlyPeriod` / `StorageCfg.SnapshotDir`——Leader 消息里「导出面不变」应读作「导出函数面不变」，与 CONTRACTS §B 口径一致 |
| 字段名字面量 | `notify.go` 全用 `FieldM2/FieldM1/FieldTSFStock/FieldDepositFlowYTD/FieldDepositFlowMoM` 常量；`TestFieldNamesAppearOnlyInFieldsGo` 绿 |
| `cmd/atlas/hestia.go` 不 import `path/filepath` | 新增 import 仅 `internal/notifier/telegram`；`TestHestiaCmdDoesNotResolveDBPath` 绿 |
| 9 条 `task/*-m1d` 分支 | `git branch --merged master` 全部已合入（可清理） |

## 3. Leader 点名的五个「同形漏点」逐条

| # | 点 | 判定 | 证据 | 档 |
|---|---|---|---|---|
| 1 | `saveSnapshot` diverged 路径在 ingest 层 | 有区分断言 | `TestIngestSnapshotDivergedOnForce` 断言 Out 含 `snapshot diverged from <annualID>` **且**目录恰 2 文件；`TestIngestSnapshotIdempotentOnForce` 反向断言 mtime 不变 + NotContains。验证者 M4（删打印）/M5（任何 Kind 都打）/M7（改写原文件）KILLED | [核实] 断言存在；[机制] 变异 |
| 2 | `renderP1` 只取首行 | 有区分断言 | `notify.go:47` `strings.Cut(err.Error(), "\n")`；`TestRenderP1KeepsFirstLineOnly` 的 `NotContains "second line"`；N2 KILLED | [核实] 代码；[机制] 变异 |
| 3 | `--only-period` × `--force` | 两道校验各有可转红用例 | 包级 `ingest.go:90` 在 Discover 前、`TestIngestOnlyPeriodRequiresForce` 断言 `f.calls` 空；cmd 级 `hestia.go:286-294` 在 `openHestia()` 前、`TestHestiaOnlyPeriodValidation` 把 cfg 指向不存在文件并 `NotContains` 该路径（顺序错误必露）；接线 `OnlyPeriod: hestiaOnlyPeriod` 由 `TestHestiaIngestWiresOnlyPeriod` 守（验证者 M7 KILLED） | [核实] 代码与断言；[机制] 变异 |
| 4 | plist `--config` 指向运行时路径 | 路径正确且运行时文件真实存在 | `TestHestiaPlistPassesMainConfig` 断言值恰为 `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml` 且在 `--hestia-config` 对之后；我 `ls` 运行时目录：该文件存在（mode 0600，18618 B）；`notifiers.telegram` 段 `enabled: true`、token/chat_id 非空、`proxy` 已设（只看布尔，未打印值）。**探针**：在本 HEAD 下把 `cfgFile` 指向该文件调 `loadConfigOrDefaults()` ⇒ err=nil，`buildHestiaSender() != nil` | [核实] |
| 5 | `buildHestiaSender` 代理注入 | 有区分断言（验证者变异表**未列**此项，我补做） | 我在 worktree 把 `telegram.New(…, telegram.WithProxy(nc.Proxy))` 改成 `telegram.New(nc.BotToken, nc.ChatID)`，并设 `HTTPS_PROXY=http://127.0.0.1:1` 保证不出外网 ⇒ `TestHestiaIngestWiresNotify` **红**（`hestia_test.go:1289`，connects ≥ 1 断言）。改动已还原，`git status` 空 | [核实] |

## 4. 发现清单

### [WARNING] W1 · Telegram 发送失败的错误文本含 bot token，M1d 把它接进了 out.log / err.log
- 文件:行号：`internal/notifier/telegram/telegram.go:304,321`（`apiURL` 含 token，`client.Post` 的 `*url.Error` 把完整 URL 写进 `Error()`）→ `internal/hestia/ingest.go:61-62`（`notifyError` 原样包住）→ `ingest.go:189`（`%s FAILED: %v` 打进 `Out`，即 launchd 的 `hestia-ingest.out.log`）与 `ingest.go:202`（返回错误 → cobra 打到 stderr，即 `hestia-ingest.err.log`）。
- 复现（[核实]，我在 worktree 跑过的探针，已删）：
  ```go
  tg := telegram.New("SECRET-TOKEN-123", "42", telegram.WithProxy("http://127.0.0.1:<已关闭端口>"))
  err := tg.SendText("x")
  // err = telegram: failed to send message: Post "https://api.telegram.org/botSECRET-TOKEN-123/sendMessage": proxyconnect tcp: dial tcp 127.0.0.1:50906: connect: connection refused
  ```
- 为什么在运行时会真的发生：plist 自己的注释写着「代理未必在跑（launchd 唤起时若 clash 没启动…）」，而 Telegram 恰恰**只**走那个代理（`config.yaml` 的 `proxy: http://127.0.0.1:7897`）。代理没起来的那次唤起，token 明文落进两份日志。
- 归属：**pre-existing** 于 telegram 包，`cmd/atlas/crisis.go:340,526` 的 `warning: notify failed: %v` 同样暴露。M1d 没有引入它，但新开了第三个（也是唯一无人值守、每天三次的）出口。
- 建议：在 telegram 包 `sendPayload` 里把错误中的 token 脱敏（`strings.ReplaceAll(err.Error(), t.botToken, "<redacted>")` 或统一包成 `telegram: failed to send message: <err.(*url.Error).Err>`），补一条测试断言错误文本 `NotContains(token)`。不在 M1d 改动文件内 ⇒ 建议开交付后小任务，不退回 M1d。

### [WARNING] W2 · `buildHestiaSender` 把「主配置装载失败」与「未配置」折成同一句 `notify: disabled (telegram not configured)`
- 文件:行号：`cmd/atlas/hestia.go:273-276`（`loadConfigOrDefaults()` 出错 ⇒ `return nil`，错误被丢弃）→ `hestia.go:302-306`（sender 为 nil 时只会打 `not configured`）。测试 `TestBuildHestiaSenderFromMainConfig/unloadable main config ⇒ nil` 把这个形态钉成了契约。
- 为什么值得一条 WARNING：M1d 立项的直接原因是「失败三周无人发现」。plist `--config` 路径打错、运行时 `config.yaml` 将来与新二进制 schema 不兼容、文件权限变化——这三种都会让 out.log 每天三次写「telegram not configured」，而真相是「装不上」。运维读到「未配置」会去查 `notifiers.telegram`，找不到问题。
- 缓解（[核实]）：切换清单 §5 第 6 步「三件事」当天能抓住；且本 HEAD 下运行时 `config.yaml` 实测可装载、sender 非 nil（见 §3-4）。所以**今天不会发生**，是将来的漂移面。
- 建议：装载失败时改打 `notify: disabled (main config: <err>)`，并把 `loadConfigOrDefaults` 的 err 同时写 `cmd.ErrOrStderr()`。仍不阻塞入库，与 spec §4.1「任一项缺失 ⇒ nil 静默降级」不冲突（那句说的是配置项缺失，不是文件装不上）。改动 ≤ 6 行 + 1 条测试；可并入 W1 的小任务。

### [SUGGESTION] S1 · 「入库成功但通知失败」在 Out 里打成 `<period> FAILED:` 并计入 `N/M 期失败`
- `ingest.go:189-191,202-203`。数据已在库，Out 前一行已有 `2026-08 New → hestia_observations`，错误链带 `send P2: notify:`（spec §9 要求的区分成立，[核实] `TestIngestNotifyFailureIsLoudButNotCascading` 钉了前缀形态）。但需求 §TASK-010 验收表用 `grep FAILED out.log` 作判据（需求 1639/1651 行），第一眼会把它读成入库失败。建议文案 `NOTIFY FAILED` 或汇总里分列。不阻断。

### [SUGGESTION] S2 · `--only-period YYYY-12` 会同时命中 12 月月报与年报
- `ingest.go:173` 只比 `Period`；`ingest.go:163` 注释明写「12 月月报与年报的 period 都是 YYYY-12」。spec §4.4 定义就是「Period 不等于它的一律跳过」，**不是缺陷**；切换清单用 `2026-07` 不受影响。建议在 CONTRACTS 或 flag 帮助里加一句「12 月会命中两篇、发两条 P2」。

### [SUGGESTION] S3 · diverged 行的 `saved as <path>` 段无断言；`.tmp` 固定名与秒级时间戳
- `TestIngestSnapshotDivergedOnForce` 只 Contains `snapshot diverged from <id>`，路径段改错不会红。`writeAtomic` 的 `<path>.tmp` 与 `<id>.<秒>.html` 在两个进程同时处理同一篇时会互相覆盖——单进程 + launchd 单实例 + §5 第 1 步先 bootout 的前提下不发生。记录即可。

### [SUGGESTION] S4 · `loadConfigOrDefaults` 顺带 `initPolicyGate`
- `cmd/atlas/export_ohlcv.go:297`：hestia ingest 现在也会装一次 collector 配额表。[核实] `internal/collector/policy/quota_file.go:36` `NewFileStore` 只存路径、无 I/O，无实际副作用。记录即可。

### [SUGGESTION] S5 · 分支与 worktree 清理
- 9 条 `task/*-m1d`（含 `task/TASK-007-m1d-fix`）全部已合入 master，可 `git branch -d`。`.worktrees/` 下四个旧 feature worktree 与本 sprint 无关，不动。

## 5. 代码质量 / 架构 / 安全 / 性能 / 测试质量 小结

- **代码质量**：三个新文件职责单一（写盘 / 渲染 / 编排接缝），非导出、命名与既有风格一致；注释把「为什么」写在决策点上（`send P2` vs `notify: notify:`、字面量 nil vs nil 指针装接口）。无重复。
- **架构**：`Sender` 窄接口 + cmd 层构造，与 crisis 同形；`Parse/Validate/Store` 一行未动（diff 空）。`notifyError` 用 `errors.As` 做「不套娃」判定，错误链保留底层 err（`errors.Is(errBoom)` 断言在）。依赖方向 cmd → internal/hestia → 无新边。
- **安全**：W1（token 入日志）；渲染器输出不含任何密钥；`config.yaml` 0600。输入校验：期次正则 `^\d{4}-(0[1-9]|1[0-2])$` 两层。
- **性能/并发**：单进程串行；快照 `ReadFile` 整读比对，HTML 级别无压力。
- **测试质量**：本 sprint 三次返工都是「性质无区分断言」形态，返工后的三条守卫（yaml 原文、send 在 Fprintf 之后、两行接线）我逐条读了断言，都是能转红的形态；验证者变异表 KILLED 计数 [机制]；我补做的 `WithProxy` 变异 [核实] 红。剩余弱点只有 S3（路径段）。

## 6. 第一轮小结

无 CRITICAL；WARNING 两条均为交付后待办性质，不触发 `review_fix`。进入第二轮跨视角对抗审查（Skeptic / Operator / Architect + codex 跨模型）。
