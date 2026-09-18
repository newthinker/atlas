# TASK-007 验证报告 —— hestia-warp launchd plist 入库

# 第 4 轮（QA 返工 W-5：补 Go 守卫）

- 验证者：test-m4-b
- 判定：**VERIFIED**
- 判定对象：master @ `e6f4e9d493fb9e3bcebb0adef9a9edc3fbae8101`（= verify_baseline.head；判定时主仓库 HEAD 相同；discovery sha256 `e29cab47…0578` 与基线一致）
- assignment_epoch：4，rework_count：1
- 验证环境：隔离 worktree（detached @ 上述全 sha，已拆）
- 判定范围：**仅本轮 W-5 产物**。前三轮结论以本报告后面保留的第 1–3 轮原文为准；dev-m4-b 声明未复核 dev-m4-c 所写的前三轮 discovery 内容，我同样不据其转述改判前三轮。
- 生产动作：未执行任何 launchctl / install-services.sh

## R4-1. 范围核对

| 检查 | 结果 |
|---|---|
| 本轮 merge（`3d94a52`）相对第一父 numstat | 仅 `cmd/atlas/hestia_test.go 31/0` |
| plist 自第 3 轮基线 `6d5230e` 起 | `cmp` 逐字节相同（未动） |
| `scripts/ops/install-services.sh` 自 `6d5230e` 起 | `cmp` 逐字节相同（未动） |
| 基线 HEAD 与本轮 merge 之间 | 只隔 TASK-001 的 QA 返工 `a680de1`（改 `internal/hestia/`），与本任务无文件重叠；`3d94a52..e6f4e9d` 在 `cmd/atlas`、`deploy/launchd`、`scripts/ops` 上无任何改动 |
| 声明范围 | `writes` 已含 `./cmd/atlas/hestia_test.go`、`packages` 已含 `./cmd/atlas`（dev 在 `dev_done` 前自补，Leader 已认可）⇒ 无越界 |

plist 现状复核：`plutil -convert json` 后 `EnvironmentVariables` 键集合 = `['PATH']`，顶层无任何含 `proxy` 的键。

## R4-2. non_functional[0] 的 W-5 追加要求

| 要求 | 取证 | 判定 |
|---|---|---|
| cmd/atlas 新增测试断言本 plist 不含任何 `_proxy`/`_PROXY` 后缀键，**按后缀判、不枚举键名** | `TestHestiaWarpPlistSetsNoProxyKeys`：`strings.HasSuffix(strings.ToLower(k), "_proxy")` 遍历全部键；变异 P2（`ALL_PROXY`）、P3（`ftp_proxy`）——两个**未被任何枚举覆盖**的键——均被杀死 | PASS |
| 含肯定式锚点，避免解析失败时否定式断言平凡为真 | 三重锚点：`require.NotEmpty(keys)`、`require.Contains(keys,"PATH")`、PATH 首段须含 `/.nvm/versions/node/` 且以 `/bin` 结尾。变异 P7（删掉整个 `EnvironmentVariables` dict）被第一条杀死、P8（dict 里只剩一个代理键、无 PATH）被第二条杀死、P5/P6（首段被换掉、或是 nvm 目录但不以 `/bin` 结尾）被第三条杀死 | PASS |
| 解析失败不得空集平凡通过 | 变异 P9：插入含非法双连字符的 XML 注释——**`plutil -lint` 退出码 0（判为合法）**，而守卫因 `plistEnvKeys` 对解析错误 `require.NoError` 而红。dev 在 discovery 里的这条声明我独立复现成立 | PASS |
| 复用既有解析辅助、不另建同形口径 | 测试直接调既有 `plistEnvKeys` / `plistEnvValues`（`hestia_test.go:407`），与 `TestHestiaPlistSetsProxyKeysForSheets` 同一口径 | PASS |
| 未执行 launchctl | `launchctl list \| grep -c hestia-warp` ⇒ 0；`ls ~/Library/LaunchAgents \| grep -c hestia-warp` ⇒ 0 | PASS |

运行：`go test -count=1 -v -run 'TestHestiaWarpPlistSetsNoProxyKeys|TestHestiaPlistSetsProxyKeysForSheets' ./cmd/atlas/` ⇒ 两条均 PASS；`go test -count=1 ./cmd/atlas/` ⇒ ok；`go vet ./cmd/atlas/` ⇒ 通过。

## R4-3. 区分力变异（我自己设计，作用于隔离 worktree 的 plist 副本；逐个 sha256 还原一致，收尾 `git status --porcelain` 与开始时相同，HEAD 未变）

| 变异 | 结果 | 杀死它的断言 / 说明 |
|---|---|---|
| P1 加 `http_proxy` | KILLED | 后缀断言（`实际出现 "http_proxy"`） |
| P2 加 `ALL_PROXY`（大写、未枚举） | KILLED | 后缀断言 |
| P3 加 `ftp_proxy`（未枚举） | KILLED | 后缀断言 |
| P4 加 `no_proxy` | KILLED | 后缀断言 |
| P5 PATH 首段换成 `/usr/local/bin` | KILLED | 锚点三 |
| P6 PATH 首段是 nvm 目录但不以 `/bin` 结尾 | KILLED | 锚点三 |
| P7 删掉整个 `EnvironmentVariables` dict | KILLED | 锚点一（`Should NOT be empty`） |
| P8 dict 里只留 `https_proxy`、无 PATH | KILLED | 锚点二（先于后缀断言拦下） |
| P9 插入含非法双连字符的 XML 注释（`plutil -lint` rc=0） | KILLED | `plistEnvKeys` 的 `require.NoError`，报「守卫不能在非法 XML 上静默返回部分结果」 |
| **P10 把 `http_proxy` 放到 plist 顶层（`EnvironmentVariables` 之外）** | **SURVIVED** | **等价变异，不计**：launchd 的 job 键集合里没有 `http_proxy`，顶层多一个未知键不会给进程设任何环境变量；DoD 与裁决的对象都是 `EnvironmentVariables`，守卫的作用域与之一致 |

有效变异 9 个，9 个全部 KILLED。

## R4-4. 说明

- 本轮判定不依赖 dev-m4-b 对前三轮内容的转述；前三轮的结论、证据与我在第 2 轮记下的「未做删键实跑对照」这一强度限制均保持不变（见下面保留的原文）。
- discovery `provenance.subagent_anomaly` 记了一次子代理「报告无动作、实际改了文件」。我核对的是**基线树上的最终文件**，不受该异常影响；本报告所有数字均采自 `e6f4e9d…8101`。

## R4-5. 复现命令（锚为全 sha）

```bash
B=e6f4e9d493fb9e3bcebb0adef9a9edc3fbae8101
git worktree add --detach ../wt-verify "${B}" && cd ../wt-verify
GOTOOLCHAIN=local go test -count=1 -run TestHestiaWarpPlistSetsNoProxyKeys ./cmd/atlas/
# 区分力：在 worktree 的 plist 副本里给 EnvironmentVariables 加 ALL_PROXY，再跑上面一条 ⇒ 红
```

---

# 第 3 轮（人类裁决「删掉代理键」后复验）

- 验证者：test-m4-b
- 判定：**VERIFIED**
- 判定对象：master @ `6d5230e893a329870a943261858cbbc5a8bdeeae`（= verify_baseline.head，判定时主仓库 HEAD 相同；discovery sha256 `f4280476…7509` 与基线一致）
- assignment_epoch：3
- 生产动作：**未执行**任何 launchctl / install-services.sh，未唤起 agent
- nanoclaw 取证锚：`e7c6278354b13f4e57c9ceb3dece6165d6d8bc6b`，一律用 `git -C … show "${C}:path"` 读该 commit 的 blob（非工作树）；取证时该仓库 HEAD 仍为此 sha、工作树干净

## R3-1. Done Criteria 覆盖矩阵

| # | 完成标准 | 取证 | 判定 |
|---|---|---|---|
| functional[0] | lint；Label / ProgramArguments / WorkingDirectory | `plutil -lint` OK；plist→JSON 后与第 2 轮 JSON 去掉三个代理键**完全相等**（python `==` 为 True），第 1 轮已逐项 extract 取证的值不变 | PASS |
| functional[1] | StartInterval / RunAtLoad / 日志路径 | 同上（顶层键集合 = EnvironmentVariables, Label, ProgramArguments, RunAtLoad, StandardErrorPath, StandardOutPath, StartInterval, WorkingDirectory，无增删） | PASS |
| functional[2] | install-services.sh | 基线与第 1 轮 `1a89e27a` 的该文件 `cmp` 相同；返工增量 `c4d651c..6d5230e` numstat 仅 plist `16/16` | PASS |
| boundary[0] | EnvironmentVariables 只含 PATH，无 `_proxy`/`_PROXY` 后缀键；PATH 首段存在且有 pnpm | JSON 键集合 = `['PATH']`；`k.lower().endswith('_proxy')` 命中 `[]`；含 `proxy` 子串的键 `[]`；`ls -d /Users/zuowei/.nvm/versions/node/v22.22.0/bin` 存在，`test -x …/pnpm` 通过 | PASS |
| error_handling[0] | 头注释三段 + 每句事实可核且为真 + nanoclaw 引用写明 commit | 见 R3-2 逐句表 | PASS |
| non_functional[0] | 未执行 launchctl | `launchctl list \| grep -c hestia-warp` ⇒ 0；`ls ~/Library/LaunchAgents \| grep -c hestia-warp` ⇒ 0 | PASS |

回归：隔离 worktree @ 6d5230e 跑 `GOTOOLCHAIN=local go test -count=1 -run 'Plist|InstallServices|Launchd' ./cmd/atlas/` ⇒ ok（worktree 已拆）。

## R3-2. error_handling[0] 逐句核对

| 行 | 陈述 | 取证 | 判定 |
|---|---|---|---|
| 8 | 刻意不设任何代理键 | 键集合仅 PATH（R3-1） | 真 |
| 8 | 本 job 自己不需要出网 | 触发脚本 @6d5230e 无 curl/wget/http；只有 `find` 与 `pnpm run chat` | 真 |
| 9 | 取证自 nanoclaw commit e7c6278… | 全 sha 写明，满足 DoD「写明取证 commit」 | 真 |
| 10 | `pnpm run chat` = `tsx scripts/chat.ts` | `package.json:23` `"chat": "tsx scripts/chat.ts"` @e7c6278 | 真 |
| 10–11 | chat.ts 只 `net.connect(<DATA_DIR>/cli.sock)`、写 `{ text }` 等回复 | chat.ts:33 `net.connect(socketPath())`，socketPath = `path.join(DATA_DIR,'cli.sock')`；外部 import 仅 `net`、`path` | 真 |
| 12 | chat.ts 本身不读 process.env | chat.ts 中 `process.env` 0 命中 | 真 |
| 12–13 | 它导入的 config.ts 及依赖链（env.ts、log.ts、install-slug.ts、timezone.ts）里无代理变量、子进程或 HTTP 调用 | **我用独立仪器复算**：从 chat.ts 起按 import/export-from/动态 import/require 递归解析相对路径，本地闭包恰为 `chat.ts, config.ts, env.ts, install-slug.ts, log.ts, timezone.ts`（与注释列举一致）；外部模块仅 net/path/os/fs/crypto；对闭包全文扫 `proxy\|child_process\|execSync\|spawn\|exec(\|fetch(\|https?.get/request\|undici\|axios`，命中只有 config.ts:30 一行注释（`stamped onto every spawned container`）；config.ts 读的 process.env 均为非代理变量（ASSISTANT_NAME、HOME、CONTAINER_* 、ONECLI_*、TZ 等），log.ts 读 LOG_LEVEL | 真（注释措辞「无代理变量」而非「不读 env」，与事实一致） |
| 14 | pnpm 按 packageManager 取 10.33.0，corepack 缓存已有 | `packageManager: pnpm@10.33.0`；`~/.cache/node/corepack/v1/pnpm/10.33.0` 存在（本机状态） | 真 |
| 15–16 | 容器由常驻 daemon（另一个 LaunchAgent，跑 dist/index.js）经 container-runner.ts 起 | `~/Library/LaunchAgents/com.nanoclaw-v2-5657aeae.plist` ProgramArguments = node + `nanoclaw/dist/index.js`（本机状态）；@e7c6278 `src/index.ts` 导入 router/host-sweep，二者均 `import { wakeContainer } from './container-runner.js'`；container-runner.ts:168 `spawn(CONTAINER_RUNTIME_BIN, args, …)` | 真 |
| 16–17 | 容器出网代理由 OneCLI gateway 注入 HTTPS_PROXY | container-runner.ts:483 `// OneCLI gateway — injects HTTPS_PROXY + certs so container API calls` | 真 |
| 18 | 连不上 Anthropic 该查 daemon / gateway | 由上两行推出的排障指引，非独立事实陈述 | 与上文一致 |
| 19–20 | hestia-ingest.plist 设了代理键（5447d05，Sheets 投影出网） | ingest plist @6d5230e 第 43/57/59 行 no_proxy/http_proxy/https_proxy；`5447d05 fix(TASK-009): QA 返工——plist 补代理键…` | 真 |
| 22–23 | crisis / refresh-cnhk / prism-daily 三份都设了 http_proxy/https_proxy/no_proxy | @6d5230e 键序：crisis-daily `no_proxy PATH http_proxy https_proxy`；refresh-cnhk `http_proxy https_proxy no_proxy PATH`；prism-daily `http_proxy https_proxy no_proxy` | 真 |
| 23–24 | crisis-daily 里 no_proxy 与两代理键之间隔着 PATH | 同上键序 | 真 |
| 26 | PATH 首段是 nvm node v22 bin（pnpm 在那里） | R3-1 boundary[0] | 真 |
| 27–30 | nvm 目录消失 ⇒ stderr `pnpm: command not found`，`\|\| true` 使退出码 0 | 第 2 轮已实跑（`env -i PATH=/usr/bin:/bin`、临时队列 1 件、NANOCLAW_DIR 空临时目录 ⇒ `line 53: pnpm: command not found`，rc=0）；触发脚本 `c4d651c..6d5230e` 未变 | 真 |
| 50–52 | StartInterval 段 | 第 2 轮已核为真，本轮该段未改 | 真 |

## R3-3. 证据强度与遗留

- **未做「删掉代理键后实跑一次」对照**：需唤起真实 agent，属生产动作，归人类的计划验收判据五实测。本判定对「代理键到不了容器」的支撑是 nanoclaw@e7c6278 的代码直读 + 本机 LaunchAgent 配置，非运行时观测。
- 本机状态类陈述（corepack 缓存、daemon LaunchAgent 内容）不在任何仓库，是取证时刻（2026-09-17）的快照，日后可能变化。
- 范围外待办（第 1 轮已报，Leader 已记）：hestia-ingest.plist 自身头注释过期，未变。

---

# 第 2 轮（返工复验，保留原文）

- 验证者：test-m4-b
- 判定：**REJECTED**，reason_class=`dod_defect`（本任务第 2 次）
- 判定对象：master @ `c4d651c8abb0463fd9be070df93b44be3fcd5373`（= verify_baseline.head，判定时主仓库 HEAD 相同；discovery sha256 `97fe3c4e…36aa` 与基线一致）
- assignment_epoch：2，rework_count：1
- 生产动作：**未执行**任何 launchctl / install-services.sh；未唤起 nanoclaw（下文实测脚本时 PATH 无 pnpm、NANOCLAW_DIR 指向空临时目录）

## R2-1. 键值与范围（不采信 dev 回显，自己再比一次）

| 检查 | 命令 / 结果 |
|---|---|
| 返工范围 | `git diff --numstat 1a89e27a35b4e0e7be81d7f9507f722e32b795c9 c4d651c8abb0463fd9be070df93b44be3fcd5373 -- deploy/launchd scripts/ops/install-services.sh` ⇒ 仅 plist `20/16` |
| 键值零改动 | 两棵树的 plist 各自 `plutil -convert json` 后 `cmp` ⇒ 逐字节相等（700 字节） |
| install-services.sh | 两棵树 `cmp` ⇒ 相同（第 1 轮 functional[2] 结论延续） |
| `plutil -lint`（c4d651c） | OK |
| launchctl / LaunchAgents | `launchctl list \| grep -c hestia-warp` ⇒ 0；`ls ~/Library/LaunchAgents \| grep -c hestia-warp` ⇒ 0 |

functional[0]/[1]/[2]、boundary[0]、non_functional[0]：键值与第 1 轮逐字节相同、第 1 轮已逐项取证 PASS，本轮复跑 lint 与未装载两项仍 PASS ⇒ **PASS**。

## R2-2. error_handling[0] 逐句核对（订正后 DoD）

| 注释行 | 陈述 | 取证（锚 c4d651c） | 判定 |
|---|---|---|---|
| 5 | 先判后唤起的理由见触发脚本头注释 | `hestia-warp-trigger.sh` 第 4–6 行确有 | 真 |
| 8 | 脚本经 `pnpm run chat` 唤起 nanoclaw | 触发脚本第 53 行 `cd "$NANOCLAW_DIR" && pnpm run chat 处理 hestia 队列 \|\| true` | 真 |
| **8–9** | **「代理键在这里是必须的：…容器里的 agent 要出网访问 Anthropic，而 launchd 不继承登录 shell 的环境 —— 不设就恒连不上」** | 见 R2-3 | **假** |
| 10–12 | hestia-ingest.plist 也设了 http_proxy/https_proxy/no_proxy，5447d05 起，守卫 TestHestiaPlistSetsProxyKeysForSheets | ingest plist 第 43/57/59 行三键在场；`5447d05 fix(TASK-009): QA 返工——plist 补代理键…`；`cmd/atlas/hestia_test.go:477` | 真 |
| 12–13 | 央行直连由 fetch.go 空 `Transport{}` 保证 | `internal/hestia/fetch.go:38 Transport: &http.Transport{}, // Proxy 留零值：直连` | 真 |
| 15–17 | crisis-daily 里 no_proxy 与两代理键之间隔着 PATH；prism-daily 无 PATH；三份 EnvironmentVariables 彼此不一样 | crisis-daily 键行号 no_proxy 26 / PATH 28 / http_proxy 32 / https_proxy 34；prism-daily 解析出的键仅 http_proxy/https_proxy/no_proxy；三份 XML 文本块两两 diff 均不同 | 真（观察：crisis-daily 与 refresh-cnhk 的**键值**经 `plutil -extract … xml1` cmp 完全相同，差异只在键序与注释；「彼此都不一样」按文本成立、按值不成立，措辞可更准，不扣分） |
| 20–24 | nvm 目录消失后 stderr `pnpm: command not found`，但 `\|\| true` 使退出码仍是 0 | 临时队列放 1 件、`env -i PATH=/usr/bin:/bin` 实跑基线脚本 ⇒ stderr `line 53: pnpm: command not found`，`rc=0` | 真 |
| 50 | 同 analysis / crisis-intraday-jpy 的做法 | 两者 `StartInterval` 均为 1800 | 真 |
| 51–52 | 入队契约在 processing/ 空闲时最多等一轮；空队列成本为 pending/ 与 processing/ 各一次 find | 脚本第 35–45 行：先对两目录各 `count_of`（各一次 find），pending=0 退出、processing≠0 跳过 | 真 |

## R2-3. 缺陷：「代理键必须，因为容器 agent 要出网访问 Anthropic」与代码事实不符

注释把本 plist 的代理键说成容器 agent 出网的**必要条件**（「不设就恒连不上」）。我读了 nanoclaw 仓库与本机配置（只读，未运行 agent），结论是**本 plist 的环境变量根本到达不了容器 agent**：

1. **`pnpm run chat` 只是本地 Unix socket 客户端。** nanoclaw `package.json` 第 23 行 `"chat": "tsx scripts/chat.ts"`；`scripts/chat.ts` 头注释「Sends the message through the CLI channel (Unix socket) to the wired agent… Preconditions: NanoClaw host service running」，实现 `net.connect(path.join(DATA_DIR, 'cli.sock'))`，只写 `JSON.stringify({ text })`；全文件 `process.env|PROXY|proxy` 命中 **0** 处 ⇒ 客户端不出网、也不把自己的环境转交给任何人。
2. **容器由另一个常驻 daemon 起，用的是 daemon 的环境。** 本机 `~/Library/LaunchAgents/com.nanoclaw-v2-5657aeae.plist` 的 ProgramArguments 为 `node …/nanoclaw/dist/index.js`，其 EnvironmentVariables 只有 `PATH`、`HOME`（**没有任何代理键**）。
3. **容器 API 调用的代理由 OneCLI gateway 注入。** `src/container-runner.ts:483`「OneCLI gateway — injects HTTPS_PROXY + certs so container API calls are routed through the agent vault」——这正是 dev 在 discovery `decisions[1]` 里看到的那一行；它说明容器的代理来源是 daemon 侧的 gateway，不是触发脚本进程的环境变量。
4. 客户端这一侧也无出网需求：`packageManager` 为 `pnpm@10.33.0`，corepack 缓存 `~/.cache/node/corepack/v1/pnpm/10.33.0` 已存在，不需要下载。

⇒ 删掉本 plist 的 http_proxy/https_proxy，容器 agent 访问 Anthropic 的路径不变；注释「不设就恒连不上」为假，会让下一个人以为「warp 连不上 Anthropic ⇒ 查这份 plist 的代理键」，排障方向错位（真正该查的是 nanoclaw daemon / OneCLI gateway）。

**证据强度声明**：以上为代码与配置直读，**未做「去掉代理键实跑一次」的对照**（那需要唤起真实 agent，属生产动作，不做）。结论依赖「chat.ts 只经 socket 传 text」与「容器由 daemon 起」两点，均有行号可核。

**为什么判 `dod_defect` 而非 `task_defect`**：订正后 DoD error_handling[0] 原文就规定了这个理由——「代理键为何必须（nanoclaw 容器里的 agent 要出网访问 Anthropic；launchd 不继承登录 shell 环境）」。dev 照 DoD 写，并在 discovery `decisions[1]` 如实标注「未独立实测该网络路径」。按 DoD 字面写必然写出假陈述，无法同时满足 DoD 与事实 ⇒ 根因在 DoD。按规则 `dod_defect` 累计第 2 次应转 `blocked_human`，我不为避开熔断而改判类别。

**需要人类/Leader 先定的事实问题**（不是 dev 能单方面改的）：这份 plist 的代理键到底要不要？
- 若按上面的代码事实，代理键对 agent 出网无作用 ⇒ 注释应改成「本 plist 的代理键不影响容器 agent（它由 nanoclaw daemon 起、代理由 OneCLI gateway 注入）；保留它们是为了 ___ / 或删掉」，这同时牵动 boundary[0]（DoD 要求设这三键）。
- 若人类掌握我没看到的出网路径（例如 chat 客户端之外还有进程继承本环境），请在 DoD 里写明那条路径，验证时再核。

## R2-4. 与第 1 轮的关系

第 1 轮缺陷（「与 hestia-ingest 相反 / 那份不设代理键」）**已修复**：注释现写「与 hestia-ingest.plist 相同」，逐项为真；`grep -cE '相反|不设代理|一个代理键都' ` 的旧措辞 0 命中。本轮新缺陷来自 DoD 订正时代入的另一条未核实前提。

## R2-5. 复现命令（锚为全 sha）

```bash
B=c4d651c8abb0463fd9be070df93b44be3fcd5373
git show "${B}:deploy/launchd/com.newthinker.atlas.hestia-warp.plist" | sed -n 8,9p
git show "${B}:scripts/ops/hestia-warp-trigger.sh" | sed -n 53p
grep -n '"chat"' /Users/zuowei/workspace/ai/nanoclaw/package.json
sed -n 1,40p /Users/zuowei/workspace/ai/nanoclaw/scripts/chat.ts
sed -n 483,486p /Users/zuowei/workspace/ai/nanoclaw/src/container-runner.ts
plutil -extract EnvironmentVariables json -o - ~/Library/LaunchAgents/com.nanoclaw-v2-5657aeae.plist
```
（nanoclaw 为独立仓库，取证时 HEAD=`e7c6278354b13f4e57c9ceb3dece6165d6d8bc6b`、工作树干净（`git status --porcelain` 0 行）；复现时先 `git -C /Users/zuowei/workspace/ai/nanoclaw show e7c6278354b13f4e57c9ceb3dece6165d6d8bc6b:scripts/chat.ts`。LaunchAgents 下 daemon plist 不在任何仓库，是取证时刻的本机状态。）

---

# 第 1 轮（2026-09-17，保留原文）


- 验证者：test-m4-b
- 判定：**REJECTED**，reason_class=`dod_defect`
- 判定对象：master @ `1a89e27a35b4e0e7be81d7f9507f722e32b795c9`（= verify_baseline.head；判定时主仓库 HEAD 相同，两个声明文件与基线 `cmp` 逐字节一致；discovery sha256 `b702b45e…f41b` 与基线一致）
- assignment_epoch：1
- 生产动作：**未执行**任何 launchctl / install-services.sh

### 1. Done Criteria 覆盖矩阵（全部 verify_by: review，取证均亲跑）

| # | 完成标准 | 取证 | 判定 |
|---|---|---|---|
| functional[0] | lint；Label / ProgramArguments / WorkingDirectory | 基线 blob 导出后 `plutil -lint` ⇒ OK；`plutil -extract <k> raw`：Label=`com.newthinker.atlas.hestia-warp`，ProgramArguments.0=`/bin/bash`，.1=`/Users/zuowei/workspace/runtime/atlas/scripts/ops/hestia-warp-trigger.sh`，WorkingDirectory=`/Users/zuowei/workspace/runtime/atlas` | PASS |
| functional[1] | StartInterval / RunAtLoad / 日志路径 | StartInterval=`1800`，RunAtLoad=`false`，StandardOutPath=`…/runtime/atlas/logs/hestia-warp.out.log`，StandardErrorPath=`…/logs/hestia-warp.err.log` | PASS |
| functional[2] | install-services.sh 清单含标签；`bash -n`；diff 仅两处 | `git diff --numstat 1a89e27^1 1a89e27` ⇒ install-services.sh `2/1`：头注释新增一行（hestia-ingest 行之后）+ 循环清单 `hestia-ingest` 同行末尾追加 `com.newthinker.atlas.hestia-warp`，无其它改动；`bash -n` 通过；回归 `go test -count=1 -run 'Plist\|InstallServices' ./cmd/atlas/`（隔离 worktree @ 基线）⇒ ok | PASS |
| boundary[0] | 代理三键 + PATH 首段存在且含 pnpm | `-extract EnvironmentVariables json`：http_proxy/https_proxy=`http://127.0.0.1:7897`，no_proxy=`localhost,127.0.0.1`，PATH 首段 `/Users/zuowei/.nvm/versions/node/v22.22.0/bin`；`ls -d` 存在；`pnpm` 为软链 → `../lib/node_modules/corepack/dist/pnpm.js`，`test -x` 通过；触发脚本第 53 行确为 `pnpm run chat` | PASS |
| **error_handling[0]** | 头注释写明三段理由 | 见第 2 节 | **FAIL** |
| non_functional[0] | 未执行 launchctl | `launchctl list \| grep -c hestia-warp` ⇒ 0；`ls ~/Library/LaunchAgents \| grep -c hestia-warp` ⇒ 0 | PASS |

### 2. 缺陷：plist 头注释含事实错误（error_handling[0]）

`deploy/launchd/com.newthinker.atlas.hestia-warp.plist` 第 8–9 行：

```
⚠️ **代理键在这里是必须的**，与 hestia-ingest.plist 的处境相反。
那份刻意一个代理键都不设，因为它直连央行（fetch.go 的空 Transport{}）。
```

**我亲自读基线树核实，结论为假**：

1. `deploy/launchd/com.newthinker.atlas.hestia-ingest.plist` @ 1a89e27 第 57–60 行设了 `http_proxy` 与 `https_proxy`（=`http://127.0.0.1:7897`），第 43–44 行设了 `no_proxy`。引入提交 `5447d05 fix(TASK-009): QA 返工——plist 补代理键…`。
2. 守卫 `cmd/atlas/hestia_test.go:477 TestHestiaPlistSetsProxyKeysForSheets` **要求**这三个键在场，并在注释里明文写着：原判据「plist 不得设任何代理键」及其理由「hestia 直连央行（NewPBOCFetcher 用空 Transport 绕开代理）」**是错的、已被推翻**——PBOC 直连由 `fetch.go` 的空 `Transport{}` 保证、与 plist 无关。

⇒ 本注释复述的恰好是那条被推翻并留有「免得后人又翻回去」警示的旧论证。它会误导下一个读者以为 hestia-ingest 没有代理键，并在别处照此推理。

**为什么判 `dod_defect` 而非 `task_defect`**：DoD 原文要求注释写明「代理键为何必须（**与 hestia-ingest 相反**）」，这个括号前提与仓库事实矛盾——按 DoD 字面写就必然写错，无法同时满足 DoD 字面与事实。根因在 DoD 括号（Leader 已自行披露其照搬自计划原文、未核实；本判定基于我对上述两个文件的直读，不基于该披露）。

**需要改成的事实**（建议写法，由返工 dev 定稿）：代理键必须——触发脚本经 `pnpm run chat` 唤起 nanoclaw，容器内 agent 要出网访问 Anthropic，launchd 不继承登录 shell 环境，不设即恒连不上。**这一点与 hestia-ingest.plist 相同而非相反**：后者也设了 http_proxy/https_proxy/no_proxy（M2b Sheets 投影出网，`5447d05`，守卫 `TestHestiaPlistSetsProxyKeysForSheets`）；PBOC 直连由 `internal/hestia/fetch.go` 的空 `Transport{}` 保证，与 plist 无关。同时 DoD error_handling[0] 的括号「与 hestia-ingest 相反」应改为与事实一致的措辞。

注释其余两段（不照抄 crisis/refresh-cnhk/prism-daily 的理由；PATH 写死 node 版本在 nvm 升级后静默失效）内容正确，满足 DoD。

### 3. 顺带发现（不属本任务 scope，提请 Leader）

`hestia-ingest.plist` **自己的**第 25 行注释仍写「这里**只有 PATH，一个代理键都没有**」、第 40 行指向已不存在的守卫名 `TestHestiaPlistSetsNoProxyKeys`（基线树全部 `*.go` 中 `git grep` 0 命中、仅存于 sprint-037 归档，现名为 `TestHestiaPlistSetsProxyKeysForSheets`）——同一文件第 47–56 行又写着补代理键的理由，前后自相矛盾。这很可能正是本任务注释错误的来源（读头注释会得出「没有代理键」）。该文件不在 TASK-007 的 packages 内，本判定不据此扣分。

### 4. 复现命令（锚为全 sha）

```bash
B=1a89e27a35b4e0e7be81d7f9507f722e32b795c9
git show "${B}:deploy/launchd/com.newthinker.atlas.hestia-warp.plist" | sed -n 8,9p
git show "${B}:deploy/launchd/com.newthinker.atlas.hestia-ingest.plist" | sed -n 41,61p
git show "${B}:cmd/atlas/hestia_test.go" | sed -n 477,510p
```
（zsh 下 `$B:c…` 会被当作历史修饰符，务必写 `"${B}:…"`。）
