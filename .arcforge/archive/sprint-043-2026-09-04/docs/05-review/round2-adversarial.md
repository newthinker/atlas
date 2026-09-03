# Sprint M1d（043）第二轮 Code Review · 跨视角对抗审查

- 审查者：qa-m1d　时间：2026-09-03　对象同第一轮（`ae088eb..f3d6eb2 -- internal/hestia cmd/atlas configs deploy`，HEAD `f3d6eb282c83e0ca730b1713907f5220114ee86b`）
- 形态：三个**只读** lens 子代理（Skeptic / Operator / Architect，各自独立 context，未读第一轮报告），全部在 scratchpad 副本上做实验，主仓库与 `wt-qa-m1d` 零改动（各自 `git status` 空）。
- **跨模型降级**：`cross_model: auto` 且 `codex` CLI 可用，实际调用两次——第一次 `codex exec` 挂在 stdin（后台无 stdin），第二次接 `</dev/null` 后跑起来但**命中用量上限**（`You've hit your usage limit … try again at Sep 10th, 2026`，token 37,326，未产出任何发现）。按 CLAUDE.md 核心原则 4 退回纯 Claude 跨视角；final-report 应注明。
- 证据分档：**[核实]** = 我在 `wt-qa-m1d` 上独立复现；**[机制]** = lens 报告有实测输出、我核过机制未重跑；**[转述]** = 仅引用 lens 报告

## 1. 三个 lens 的独立结论

| lens | verdict | CRITICAL | WARNING | SUGGESTION | 主要关注 |
|---|---|---|---|---|---|
| Skeptic | CONTESTED | 1 | 6 | 5 | 变异实测：哪些性质没有会红的断言 |
| Operator | CONTESTED | 2 | 6 | 5 | launchd 无人值守、切换清单 §5 七步能否照跑、日志真相 |
| Architect | CONTESTED | 0 | 5 | 8 | 错误语义模型自洽性、接缝位置、扩展面 |

三者一致给 CONTESTED；三者各自独立命中的共同点：**token 进日志**（Skeptic/Operator 评 CRITICAL）、**改稿后快照幂等失效**（Skeptic/Architect 均实测）、**主配置装载失败被打成「未配置」**（三者都提）、**Duplicate 的 P2 值误导**（Skeptic/Architect）。

## 2. 合并去重后的发现（按严重度）

### [CRITICAL] A1 · `deploy.sh` 的 `rsync --delete` 未排除 `configs/config.yaml`——从 worktree 执行 §5 第 4 步会删掉运行时密钥配置
- 来源：Operator。**[核实]**：
  - `configs/config.yaml` 被 `.gitignore:30` 忽略、不在 git 内、**worktree 里不存在**（`ls wt-qa-m1d/configs/config.yaml` ⇒ No such file）。
  - `scripts/ops/deploy.sh:35-52` 的排除表有 `/data/`、`/logs/` 等，**没有** `/configs/config.yaml`。
  - 我用与脚本逐字相同的排除表做 `rsync -a -m --delete -n`（只模拟）从 `wt-qa-m1d/` 到运行时目录：输出含 **`deleting configs/config.yaml`**（同时 `deleting bin/atlas`、`deleting atlas`）。运行时文件未动（仍 18618 B，Aug 2）。
  - 从**主仓库**执行：主仓库与运行时的 `config.yaml` sha256 同为 `b162ef04…` ⇒ 覆盖成同内容，**今天无害**——这解释了它此前为什么没炸。
- 为什么是 CRITICAL：spec §5 第 4 步写的就是 `bash scripts/ops/deploy.sh`；本 sprint 所有 agent（含 Leader 给我的指令）都在 worktree 里干活，「在 worktree 里跑一下部署」是自然动作。删掉之后 `deploy.sh:57-61` 只打一行 WARNING（紧跟 rsync 大段输出），随后 hestia 每次唤起打 `notify: disabled (telegram not configured)`、exit 0，**serve/crisis/prism 等其余服务同时失去密钥**——形态与 M1d 立项要堵的「静默不发、没有东西变红」完全同形。
- 归属：`scripts/ops/deploy.sh` 不在任何 M1d 任务的 `writes` 里，**`review_fix` 路由不到**；但它是 M1d 交付物（切换清单）的执行前提。
- 建议：`deploy.sh` 加 `--exclude='/configs/config.yaml'`（并在头注释写明「含密钥不入库、不同步」）；§5 第 4 步注明「**必须在主仓库、HEAD == 合并锚**下执行，禁止在 worktree 执行」。两处都要，前者堵机制、后者堵清单。**须在 §5 第 4 步之前落地**。

### [CRITICAL→WARNING，分歧] A2 · Telegram 传输错误文本含 bot token，M1d 把它接进 out.log / err.log
- 来源：第一轮 W1；Skeptic、Operator 各自独立实测并评 **CRITICAL**；我评 WARNING。**[核实]**（探针见 round1 §4-W1）。
- 分歧点：我按「pre-existing 于 telegram 包、crisis.go:340,526 同样暴露、泄露面是本机 644 日志」评 WARNING；两个 lens 按「M1d 是第一个把它每天三次写进无人值守日志的路径，且 spec §5 第 1 步明写『不打印 token』、§6 又要求人去读 err.log」评 CRITICAL。**两边事实一致、只在严重度定级上分歧** ⇒ 这是 verdict 取 CONTESTED 的判据之一。
- Operator 补充 [机制]：超时（`Client.Timeout exceeded`）同样带 URL；P1 发送失败的 `notifyError` 也进 `errs`，同样暴露。
- 建议（三方一致）：在 `telegram.sendPayload` 里脱敏（只保留 `err.(*url.Error).Err`，或 `ue.URL` 改成 `…/bot***/sendMessage`），补测试 `NotContains(token)`。落点在 `internal/notifier/telegram`，不在 M1d 任务 `writes`，需 Leader 开小任务；**建议在 §5 第 6 步（第一次真实经代理发送）之前落地**。已写进日志的 token 若曾泄露应轮换（目前日志里没有：M1d 尚未部署）。

### [WARNING] A3 · 改稿后快照幂等失效：只与 `<id>.html` 比对，此后每次重抓都再落一份相同字节的时间戳副本并打一行 diverged
- 来源：Skeptic、Architect 各自实测。**[核实]**：`saveSnapshot` 喂 v1 → v2 → v2 → v2 ⇒ `kinds=[written diverged diverged diverged]`，目录 4 个文件（我用不同秒；同秒时 `writeAtomic` 的 rename 会**静默覆盖**上一份 alt，Skeptic 实测 3 个文件）。
- 触发面（Architect 指出，比「只在 --force」更宽）：正常路径「上次 Parse 失败 ⇒ 无行 ⇒ `HasArticle` false ⇒ 下次唤起重抓」同样进这条分支——若央行改稿后 Parse 仍失败，launchd 每天三次各落一份相同副本、每次打「diverged」，高信号事件被刷成噪声。
- 与 spec 的关系：§3.2「已存在且相同 ⇒ 跳过、不改 mtime」这条承诺在第二版上失效；§3.2 的表没说「与哪个已存在的比」，是设计没闭合，不是 dev 抄错。
- 建议：mismatch 后再 glob `<id>.*.html` 逐个 `bytes.Equal`，命中即 `unchanged`（Path 指向那份）；补一条三次调用的测试。落点 `snapshot.go`/`snapshot_test.go`（TASK-002 的 writes）。

### [WARNING] A4 · 汇总行 `%d/%d 期失败` 用 `len(errs)`，P1 发送失败让同一期计两次 ⇒ `2/1 期失败`
- 来源：Architect 实测。**[核实]**：Parse 失败 + `fakeSender{err}` 单候选 ⇒ 返回错误首行 `hestia ingest: 2/1 期失败 (2025-12): …`。
- 为什么重要：这是 err.log 里人读的第一行，「2/1」会让人去找不存在的第二期。既有测试无一断言该计数（`TestIngestP1SendFailureIsReported` 只看链里含 parse 与 notify）。
- 建议：`ingest.go:202` 用 `len(failedPeriods)`；补一行断言 `ErrorContains "1/1 期失败"`。落点 `ingest.go`（TASK-005 的 writes）。

### [WARNING] A5 · 主配置**装载失败**与**未配置**折成同一句 `notify: disabled (telegram not configured)`
- 来源：第一轮 W2；三个 lens 全部独立命中。**[核实]** 代码；Operator [机制] 实测三种变体（`--config` 指向不存在文件 / YAML 解析失败 / 值为空）都打同一句且 exit 0。
- Architect 补充：这行正是 spec §5 第 6 步用来确认 `--config` 生效的判据，而它在最需要它的失败形态下说假话；`hestia_test.go` 的「unloadable ⇒ nil」用例把这个行为钉成了契约。
- 与 A1 的关系：A1 删掉 config.yaml 之后，日志就是这一句。
- 建议：`cfgFile != "" && err != nil` ⇒ cmd 层返回错误让 ingest 响亮失败（显式给了 `--config` 却读不了，不该静默降级），或至少三分文案 `no --config / config load failed: <err> / telegram disabled or incomplete`。落点 `hestia.go`（TASK-007 的 writes）。
- ⚠️ Operator 提到的「`bot_token: "${TG_TOKEN}"` 环境变量展开为空」一例**未核实**：我 grep `internal/config` 无 `ExpandEnv`，该子例按 [转述] 记。

### [WARNING] A6 · 通知失败无重试，且同篇不二次处理 ⇒ 丢了就永远丢；下一次唤起 exit 0、形态与正常完全同形
- 来源：Operator 实测（失败后重跑无 `--force`：`no new reports (stopped: seen_article)`，exit 0）。**[核实]** 读代码：`ingest.go:218-227` 的 `HasArticle` 跳过；`store.go:285-291` `HasArticle` **同时覆盖** observations 与 pending。
- 这是 spec §4.3 明写并接受的取舍（「同一篇不会二次处理，所以不会每次唤起都重复报错」），**不是实现缺陷**；但 plist 注释自述「代理未必在跑」，一个月唯一一次真事件若碰上代理没起，P2/P0 永不再发，之后三周日志与「一切正常」同形——这正是 §0.2 三周无人发现的那个文件形态。
- 建议（不阻断，建议挂账 M1.5）：`send()` 内 3 次退避重试；或 `hestia status` 打「最近一次 notify 失败：期次/时间」。

### [WARNING] A7 · `--force` 下 Duplicate 的 P2 把**本次重抽但被丢弃**的锚值写成「入库」
- 来源：Skeptic、Architect 独立读代码。[机制]：`IngestDeps.Force` 注释明写 Duplicate 只刷 article_id、新 Values 不写；而 `renderP2(obs, out)` 的数值来自本次 `obs`。切换清单第 6 步正是 `--force --only-period` 打一个已在权威表的期次，收到的 Telegram 会读成「这些值入库了」。央行改稿（数字变、发布日不变）时最会骗人，且「snapshot diverged」只到 Out 不到 Telegram。
- 建议：`renderP2` 对 Duplicate 换措辞（「已在库，本次抽取值未写入」）；可选把 `snap.Kind == diverged` 并进 P2/P0 一行。落点 `notify.go`/`ingest.go`。设计取舍，需 Leader 拍板。

### [WARNING] A8 · 测试缺口簇：五个变异 SURVIVED（Skeptic 实测，[转述]，我未重跑）
| 变异 | 位置 | 判定 |
|---|---|---|
| `wrap("snapshot")` 改 `"fetch"` | `ingest.go:238` | 已知（plan.md 证否账本 #3，Leader 判「不退回」）；TASK-005 的测试注释自己点名了这个坑却没回修 TASK-003 那条 |
| `"send P0"` 改 `"send P2"` | `ingest.go:291` | 无「落 pending + 发送失败」用例 |
| `telegram.New(nc.ChatID, nc.BotToken, …)` 对调 | `hestia.go:282` | 代理只看 CONNECT，看不见 path/body |
| 删 `now.UTC()` | `snapshot.go:59` | 夹具本身是 UTC，`ingest` 传的是本地时间 |
| `OnlyPeriod` 只比年份 | `ingest.go:173` | 两条候选是 2025-12 与 2020-06，年份就够区分 |
- 建议：前两条各补一条用例（≤ 15 行）；后三条挂账。

### [SUGGESTION] A9 · 切换清单 §5 与运行时环境（Operator，[机制]/[核实]）
- `install-services.sh:29-52` 会 bootout+bootstrap **全部 label**（含 hestia-ingest）——§5 第 4 步执行它就撤销了第 1 步的 bootout；15:30/17:30/21:30 若落在第 4→6 步之间，恰在第 5 步「删三件套→复制」中间被唤起会 `MkdirAll` 新建空库。建议把 bootstrap 挪到第 7 步或第 4 步后再 bootout 一次。
- out.log 现有约 60 行历史 `FAILED`（三周×3 次）；`--force` 恒走 `neverSeen` ⇒ 恒 `max_pages` ⇒ 每次往 err.log 写一行 WARNING ⇒ 第 7 步「err.log 不再增长」基线要在第 6 步**之后**取；建议第 3 步备份时把两份日志归档从零起。
- 运行时根目录残留 `atlas`（Aug 7，35 MB）与 `bin/atlas` 并存 **[核实]**：`ls -la /Users/zuowei/workspace/runtime/atlas/atlas` 存在；在运行时目录敲 `./atlas` 会拿到更旧的那个。建议第 4 步顺手删。
- `--only-period 2026-07` 有保质期：3 页 ≈ 60 天，约 10 月中旬后必报 `no candidate`；清单应写备选。`YYYY-12`（以及 6 月 h1、3 月 q1）会双命中、两条 P2（Architect 指出比「12 月」更宽）。
- 旧二进制 + 新 plist：`--config` 是根命令旧 flag，旧二进制照跑不报错、只是没通知——第 4 步 `--help | grep only-period` 是唯一判据（Operator 实测旧二进制 0 命中）。

### [SUGGESTION] A10 · 其余（Architect / Skeptic）
- `db:` 行旁加 `snapshots: <abs>`；`written` 时也打路径（第 6 步「三件事」第 1 件最可能因 cwd 错而出不来）。
- P1 正文期次与 article_id 印两遍。
- `Sender` 与 `crisis.Sender`、`prism textSender` 三份同形接口——消费者侧定义是 Go 惯例，**不建议共享**。
- `loadConfigOrDefaults` 顺带 `initPolicyGate`：`NewFileStore` 惰性、无磁盘副作用，可接受。
- CONTRACTS §B「导出面未改」应写「导出函数/方法面未改；新增导出类型 1（`Sender`）、导出字段 3」。
- `bytes.Equal → len==` 变异被杀是靠 ReadOnly 用例的 v1/v2 同长这个巧合；Diverged 用例可加一组同长异字节。
- 可删/过度设计：三个 lens 均**未发现**。

## 3. 已确认守住的性质（三个 lens 交叉，供 Leader 省一遍）
快照挪到 Parse 之后 / 忽略 saveSnapshot 错误 / P0-P2 条件互换 / 去掉 `errors.As` 不套娃 / `notifyError` 去 Unwrap / P1 发送失败不并入 errs / send 挪到 Fprintf 之前 / Duplicate 不发 P2 / OnlyPeriod 无 Force 放行 / 0 匹配走 no new reports / cmd 校验挪到 openHestia 之后 / plist 缺 `--config` / `Notify`、`OnlyPeriod` 两行断线 / 去掉 `WithProxy`——各自都有能红的断言。五个不动文件、go.mod、导出函数面、`path/filepath` 守卫——三方各自核过一致。Discover 内已按 ArticleID 去重（同一轮不会同 id 两次落盘）。

## 4. 第二轮小结
第二轮新增两条第一轮没看到的实测缺陷（A3 改稿后幂等失效、A4 `2/1 期失败`）与一条 diff 之外但在交付物执行路径上的 CRITICAL（A1 deploy.sh）。A2 的严重度定级 reviewer 间有分歧。综合 verdict 见 `verdict.md`。
