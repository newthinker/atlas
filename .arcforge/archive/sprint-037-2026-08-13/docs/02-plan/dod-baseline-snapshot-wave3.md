# DoD 基线快照 · wave3+（`0e2c6fc` / 2026-08-12 16:49:58Z）

> **为什么补建**：Leader 自查发现 **TASK-004/007/008/009/011 的 DoD 被无审计修改至少 10 次**
> （用 `json.dump(open(p,"w"))` 直接写，`transitions.jsonl` 里 `op==update` 计数**全为 0**）——见 **H18**。
> test-agent-26 在核实该自查后**立即**为全部未验证任务建了基线。

## ⚠️ 这道基线补不了什么（verifier 自己划的，Leader 原样保留）

**① 它是「从现在起」的基线。** Leader 那 10 次改了什么**永久不可追溯** ——
`.arcforge/tasks/` 未被 git 跟踪 + 覆盖写 + 零审计，**三条路全断**（见 H2）。
能做的只有让**今后**的修改可见。

**② TASK-004 与 TASK-011 的基线是弱的。** 它们快照时已 `in_progress`，
捕获的是**此刻**而非**派发时** —— 若 dev 或 Leader 在开工后已改过，**那些改动已被固化进基线**。
wave1 那次是抢在 dev 改之前存的，这次没有那个时间窗。

⇒ **对这两个任务，验证时能说的只是「从 16:49:58Z 起没再变过」，不能说「与定稿时一致」。**

**口径**：`jq -S -c '.done_criteria' <file> | shasum -a 256`

| 任务 | 快照时 status | 基线 sha | 当前 sha | 基线强度 | 判定 |
|---|---|---|---|---|---|
| TASK-004 | in_progress | `eb21f4adbb1d9e11…` | `eb21f4adbb1d9e11…` | ⚠️ **弱** | **未变** ✅ |
| TASK-007 | pending | `ddb00d35bf4941aa…` | `ddb00d35bf4941aa…` | 强 | **未变** ✅ |
| TASK-008 | pending | `04cef643d2bb146c…` | `04cef643d2bb146c…` | 强 | **未变** ✅ |
| TASK-009 | pending | `bd093542ae05b784…` | `bd093542ae05b784…` | 强 | **未变** ✅ |
| TASK-011 | in_progress | `903906f63cd256b7…` | `903906f63cd256b7…` | ⚠️ **弱** | **未变** ✅ |

---

## 快照全文（`done_criteria`）

```json
{
  "TASK-004": {
    "boundary": [
      "🔴 **交替顺序陷阱（reviewer C2 实测，现有守卫抓不到）**：`periodAlt` 写成 `…|三季度|…` 时，Go 的 leftmost 规则会对 `前三季度人民币贷款…` **从「三」起匹、捕获到 `三季度`**，而 `cumulativePeriods[\"前三季度\"]` 查不到 ⇒ 报错；若为了让它绿而把 `三季度` 也登记进 `cumulativePeriods`，**等于默认「三季度」是累计口径，而那个判断没有任何样本支持**。\n\n⚠️ `profiles_test.go:163` 的 `TestProfileAlternationsHaveNoPrefixPairs` **只查前缀对**（`strings.HasPrefix`），而这里是**后缀**关系，两个变体它都放行。\n\n**两条都要做**：① 用真实前三季度正文断言 `flowRE` 捕获组 `m[1]` **逐字等于** `前三季度`（不是「能匹配」）；② 把 `TestProfileAlternationsHaveNoPrefixPairs` 的判据从 prefix 扩到 **substring**（改一行，两个方向都覆盖）。",
      "**外币孪生句必须仍被正确区分** —— reviewer 实测真实一季度正文里**确实存在**（`一季度外币存款增加703亿美元`、`一季度外币贷款增加329亿美元`）⇒ **这条不会落到「未观察到」分支，必须真写用例**。",
      "🔴 **顺手更正 `ingest_test.go` 里一句已过期的注释**（dev-agent-52 发现、dev-agent-53 实测确认，Leader 判定由本任务改因为它与你的代码改动天然同处一次改动）。\n\n**注释声称**：「实测 `Parse` 对这两份直接报 `unrecognized report title`」\n\n**`master 0e2c6fc` 实测现在报的是**：\n```\nhestia: period_type q1 is not supported yet (title \"2026年一季度金融统计数据报告\")：\n  季报抽取侧尚未接线——profiles.go 的 periodAlt 只认「全年|上半年|N月份」、cumulativePeriods\n  只认「全年|上半年」，放行会让期内累计句整条不命中而产出一份看起来正常的空壳。\n  **由 M1b-4b 的 TASK-004 接上后解除本分支**\n```\n\n⇒ **结论仍成立**（季报正文仍进不了 ingest 链路），**但理由整个换了** —— 不再是「`parse.go` 的 `titleRE` 没跟着放宽」（TASK-010 已放宽），而是它加的**显式拒绝分支**。\n\n⚠️ **这是「结论对但理由错」中危害较大的形态**：过期的理由**指向一个已经不存在的机制** —— **照着它去找 `titleRE` 会扑空**，而真正要拆的是那条 `not supported yet` 分支。\n\n⇒ 你拆那条分支时**一并更正这句注释**。那条错误信息里自己写着「由 M1b-4b 的 TASK-004 接上后解除本分支」—— 代码与注释同处一次改动，不会漏。"
    ],
    "error_handling": [
      "**「一季度」与「前三季度」各要一条断言**（reviewer D8 已替我们判断）：两者形态相同（都是 `periodAlt` 的独立分支 + `cumulativePeriods` 的独立键），**但 `前三季度` 有上面 C2 的后缀陷阱而 `一季度` 没有** ⇒ 不能共用一条。"
    ],
    "functional": [
      "**真实样本事实（reviewer 已 curl 核实，直接用，不必重抓）**：一季度正文累计前缀是 **`一季度`**，前三季度是 **`前三季度`**；一季度正文 8 个板块、`sectionRules` 七个 keyword 全命中 ⇒ **`detectExtractor` 判 `rule@v2`，与年报同构**。若你的实测与此不符，**停下来告诉我**。\n\n**两处都要改**：`profiles.go:62` 的 `periodAlt` 加两个分支、`profiles.go:70` 的 `cumulativePeriods` 加两个键。",
      "**端到端跑通**（本任务唯一的真验收）：从真实季报 index 快照出发，`scanPage → parsePeriod → Parse → extractFields → Validate` 全链路产出非空 `ValidationReport`。**贴出实跑输出**。\n\n同时**移除 TASK-010 在 `checkPeriodTypeSupported` 里加的季度拒绝**（那是中间态的自解释，本任务落地即应撤掉）。"
    ],
    "non_functional": [
      "🔴 **消融必须做成 2×2**（reviewer B5：原判据「删 `cumulativePeriods` 确认转红」**四格里三格红，不携带信息** —— 正是 G31 那个形状）：**分别单删两处**，要求两次的失败**信息不同** —— 删 `periodAlt` 时**候选数会掉**（3→1），删 `cumulativePeriods` 时**候选数不变而命中为 0**。**把两次的候选数写进 discovery**，这样消融才区分得开哪一处承重。",
      "覆盖率不低于 93.2%；`gofmt`/`vet` 空；整包 `-count=1` 与 `-race` 全绿。"
    ]
  },
  "TASK-007": {
    "boundary": [
      "🔴 **`--force` 的作用域只到 pending —— 对已在观测表的期次结构上不可达**（reviewer B4 实测推翻计划）。\n\n`--force` 只绕过 `ingestOne` 里的 `HasArticle`，**不绕过 `Discover` 里的 `HasPeriod`**：`discover.go:270-272` 是 `if has { return out, nil }` ⇒ 已在观测表的期次让 `Discover` 当场返回空。\n\n**仓库里有一条现成的绿测试直接证明**：`TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly`，其断言 `discover_test.go:565` 是 `assert.Empty(t, gotB, \"唯一的候选已入库，不该产出任何东西\")`。\n\n⇒ 计划里的 `TestForceOnObservedPeriodIsDuplicate` 会**红在** `assert.Contains(out.String(), \"Duplicate\")` —— 实际输出是 `no new reports`。\n\n🔴 **危险不在它会红，在于它会被怎么修好**：最省事的做法是删掉 `Contains(\"Duplicate\")` 只留 `assert.Equal(t, 1, countRows(...))` —— 那条会绿，**但绿的原因是根本没走到 `Save`**，与「Duplicate 不产生重复行」毫无关系。\n\n**如实写成两半**：\n① 「`--force` 重跑 **pending** 期次 ⇒ `New` 落观测表」**保留**（这条成立，是 `--force` 的主要用途）；\n② 「`--force` 对**已在观测表**的期次**不可达**」，断言输出是 `no new reports`，**并要求写进 CONTRACTS**：\n**`--force` 的作用域只到 pending，「调了阈值想重跑一个已入库的期次」目前没有出路** —— M1c 标定 `MagnitudeRanges` 时会直接撞上，而 CONTRACTS 现在正准备写「`--force` 是为这件事准备的」。"
    ],
    "error_handling": [
      "**单期失败不中断整批**的守卫：一页两条，第一条 Parse 失败，断言第二条**仍被处理且入库**，且 `Ingest` **返回非零错误**（不是 nil）。两个断言缺一不可——只断言「返回错误」不能排除「第二条被跳过」。"
    ],
    "functional": [
      "按计划 Task 4（`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md` 710–991 行）执行。第一步是把 `syntheticIndex` 改成变参（一页两条报告）——**这是全计划唯一的破坏性改动**，TASK-005 的两处调用同步改。",
      "**一级键三种情形各一条守卫**（spec 第 2 节定案）：A 抓取/解析失败→**没写 pending 行**→下次自然重试；B 新 article_id→不命中→抓；C 同一篇没过闸→写了 pending→**命中被挡**。\n\n⚠️ **情形 B 的守卫只能测「一级键不挡」这一层**。Leader 已核实：`Discover` 的判停是 `if has {{ return out, nil }}`，修订版排最前时第一条就命中 `HasPeriod` 立即返回、**它本身进不了候选** ⇒ **端到端的「央行重发能被抓到」是假的**。本任务**不要**写一条声称端到端支持修订的测试；这个限定由 TASK-009 写进 CONTRACTS。"
    ],
    "non_functional": [
      "**每条守卫都要能指出它守的是哪一条 spec 定案**（注释里写明），因为它们是行为守卫而非实现测试。",
      "**消融自证**：任选两条守卫，破坏 TASK-005 的对应实现，确认**是该守卫红**而不是兄弟。贴出失败输出的具体那一行（Sprint 036 G31）。",
      "`gofmt`/`vet` 空、整包全绿、覆盖率不低于 93.2%。"
    ]
  },
  "TASK-008": {
    "boundary": [
      "flag 覆盖：`--hestia-config`、`--force`。配置文件不存在 / 非法时**错误要传播到命令退出码**，不是静默。",
      "`status` 在**空库**上跑通（用 `newDiscardCmd()`，`crisis_test.go:496` 同包可用）。"
    ],
    "error_handling": [
      "配置装载失败的错误信息**含配置文件路径** —— 否则用户不知道它读的是哪一份（`db_path` 与 cwd 的组合已经够绕）。",
      "🔴 **必须设 `SilenceUsage`**（reviewer D4 实测）：`go run ./cmd/atlas crisis status --crisis-config /nonexistent/nope.yaml` 会在 `Error:` 之后打印 **9 行完整 usage**（`main.go:26-30` 没设 `SilenceUsage`/`SilenceErrors`）。\n\n本管线的设计意图是让退出码 + `hestia-ingest.err.log` 成为**唯一报警通道**，而每次失败灌一屏 usage 会把真正那行错误埋掉 —— 且本管线**预期会有连续两个月每天三次的稳定失败态**（见 TASK-001 的 D6 说明）。\n\n⚠️ 在 hestia 子命令层面设即可（**不要动 `main.go` 的 `rootCmd`**，那会波及 crisis 的现有测试；若确需动，先跑一遍 `go test ./cmd/atlas/ -count=1` 确认 crisis 全绿）。"
    ],
    "functional": [
      "按计划 Task 6（`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md` 1311–1548 行）交付 `atlas hestia ingest` 与 `atlas hestia status` 两个子命令，装配 `LoadConfig`/`NewStore`/`NewPBOCFetcher`/`Ingest`/`RecentObservations`/`RecentPending`/`RenderStatus`。",
      "`db_path` 的绝对路径解析**由 TASK-006 的 `RenderStatus` 负责**（本 Sprint 指定，见 TASK-006 `non_functional[0]`）；本任务只把配置里的相对路径原样交给下游，**不要在这一层再解析一次**。"
    ],
    "non_functional": [
      "⚠️ **`cmd/atlas` 的历史覆盖率基线低于门禁阈值**（记忆：sprint-023 实测 75.9% < 80，AD-6）。若 `dev_done` 门禁因整包口径卡住，**不要自行调 config**——停下来告诉我，我按 AD-6 的处置流程走（亲跑 profile 核实文件级覆盖率 → 临时放行 → 立即恢复 → 记 AD + 验证者文件级独立复核）。",
      "`gofmt -l cmd/atlas/` 空、`go vet ./...` 空、`go build ./...` 绿、`go test ./... -count=1` 全绿。"
    ]
  },
  "TASK-009": {
    "boundary": [
      "**`deploy.sh` 检查**（计划自审第 2 节列为计划外新增）：配置没同步到 runtime，plist 起来会因找不到配置而失败，**而 launchd 的失败在日志里不显眼**。确认 `scripts/ops/deploy.sh` 会同步 `configs/hestia.yaml`；若不会，补上并在 discovery 说明。\n\n---\n\n🔴 **`plutil -lint` 必须通过**（reviewer D2）：`install-services.sh:34` 会跑它，而计划的 Go 守卫是**纯字符串匹配** —— XML 写坏了 Go 测试照样绿，**安装时才炸**。把 `plutil -lint` 加进本任务的验收步骤。",
      "🔴 **一条测试：`hestia.LoadConfig(\"../../configs/hestia.yaml\")` 必须成功**（reviewer D3）。有现成先例：`cmd/atlas/crisis_test.go:487` 就是读真实的 `../../configs/crisis-monitor.yaml`。\n\n⚠️ 不加这条的话，「配置文件本身能不能装载」的唯一验证是被我**刻意排除在 DoD 之外**的 Step 9 手工验收 —— 那等于没有自动守卫。"
    ],
    "error_handling": [
      "plist 守卫必须是**精确断言**而非「含某字符串」：断言 `EnvironmentVariables` 里**不存在**任何 `*_proxy` / `*_PROXY` 键。\n\n⚠️ **否定式断言在空集上平凡为真**（Sprint 036 G9）：若 plist 解析失败返回空 map，「不含代理键」照样通过。**必须配一条肯定式锚点**（如断言解析出的键数 > 0 或某个必有键在场），并注明两者互补免得后人当重复删掉。"
    ],
    "functional": [
      "按计划 Task 7（`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md` 1549–1706 行）交付 `configs/hestia.yaml`（写全部阈值，**每个数字带 M0 实测来源**）与 launchd plist（三时点唤起）。\n\n⚠️ Sprint 036 消费者位实测：`configs/hestia.yaml` **此前不存在且不在任何任务的 writes 里** —— 本任务正式认领它。",
      "🔴 **plist 一个代理键都不设**（约束 C6 的正解）。crisis 的 plist 设了代理（Yahoo 直连恒 403）——**别照抄**。\n\n⚠️ **不得照抄计划 1671 行的四名单**（reviewer C3）：计划给的实现只枚举 `http_proxy/https_proxy/HTTP_PROXY/HTTPS_PROXY` 四个，而 `com.newthinker.atlas.crisis-daily.plist` 里**还有一个 `no_proxy`** ⇒ 照抄再删那两对，`no_proxy` 留下、测试全绿，而「一个代理键都不设」这句话是假的。\n\n**断言必须是「不存在任何 `*_proxy` / `*_PROXY` 键」**，不是枚举四个名字。\n\n---\n\n🔴 **plist 必须登记进 `scripts/ops/install-services.sh`**（reviewer D1 实测）：该脚本第 29-31 行是**硬编码的 10 个 label 枚举**，新 plist 不加进去就**永远不会被 `cp` 到 `~/Library/LaunchAgents`** ⇒ 计划 Step 9 的 `launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist` 会直接 `no such file`。\n\n（`deploy.sh` 那边 reviewer 已查过**没问题** —— 它是整目录 `rsync`，`configs/hestia.yaml` 与 `deploy/launchd/*.plist` 都会自动带过去。）"
    ],
    "non_functional": [
      "🔴 **CONTRACTS 必须记下四件，缺一不可**：\n\n① **一级键定案的三种情形**（A/B/C 表）+ `--force` 是情形 C 的直接推论；\n\n② **两处接缝**（ArticleID 由 ingest 补、期次交叉校验）；\n\n③ 🔴 **修订版的限定（人类 2026-08-12 定案）** —— 计划把情形 B 标成 ✅，**而 Leader 核实它在 `Discover` 层不成立**：判停是 `if has {{ return out, nil }}`，修订版排最前时第一条就命中、**它本身进不了候选**（Sprint 036 消费者位 P10 实测：返回 **0 条**）。\n\n**必须原样写明**：「一级键确实不挡修订版，但 `Discover` 的判停规则让它**结构上不可达**；`refreshArticleID` 在正常路径上同样永不触发。Store 侧的双时态设计**当前无生产者**。」\n\n并附上消费者位的诚实限定：**只证明了「若发生则管线看不见」，没有证据说明央行真的重发过同一期** —— 这个前提值得单独确认。\n\n⚠️ 不写这一条，后人会照着计划的 ✅ 以为修订链已经能跑。\n\n④ 🔴 **`--force` 的作用域只到 pending**（reviewer B4 实测）：对**已在观测表**的期次，`Discover` 的 `HasPeriod` 会当场返回空 ⇒ `--force` **够不着它**。「调了阈值想重跑一个已入库的期次」**目前没有出路**，M1c 标定 `MagnitudeRanges` 时会直接撞上。\n\n⚠️ **不要写成「`--force` 是为这件事准备的」** —— 那正是计划原本的说法，而它不成立。",
      "**季报支持一并登记进 CONTRACTS**：TASK-001/004 定的期次类型、期末月、累计前缀，以及「季报每年 2/12 篇，此前静默消失」这个背景。",
      "🔴 **部署闸（Leader 2026-08-12 裁决，dev-agent-52 提醒落盘）**：\n\n**TASK-004 完成之前，不得执行计划 Step 9 的端到端部署**（`bash scripts/ops/deploy.sh` + `launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist`）。\n\n**理由**：独立 reviewer 的 D6 论证的真正对象是**部署之后**的行为 —— 季报若「能被发现但抽取不了」，会在 `MaxPages` 窗口内（约 2 个月）**每天三次持续失败**，把退出码这个唯一报警通道淹掉约 4 个月/年。该代价**只在管线真的跑起来时才付**。\n\n⚠️ Leader 最初把这条写成了「TASK-001 与 TASK-004 必须同一分支、一次合入 master」——**那是把「不要部署中间态」误写成了「不要合入 master」**，两者差得很远（且后者会切断 TASK-010 的依赖链）。合入 master 无害且必需；**部署才是那条线**。\n\n**本任务的动作**：① 在 `CONTRACTS.md` 里记下这条闸；② 交付时**不执行** Step 9 —— 那一步由 Leader 在归档前亲自执行，且必须在 TASK-004 `verified` 之后。\n\n---\n\n`gofmt`/`vet` 空、`go build ./...` 绿、`go test ./... -count=1` 全绿、`-race` 绿。**端到端手工验收（计划 Step 9）由 Leader 在归档前执行，不属本任务 DoD。**"
    ]
  },
  "TASK-011": {
    "boundary": [
      "🔴 **必须正面处理 `discover.go:205-208` 那段既有反论证**（dev-agent-52 指出，Leader 核实属实）：\n\n> 判停用**期次**而不是 article_id：M0 实测 2020 上半年报告的 article_id 是 `2025092212550713215`\n> —— 2025-09-22 的时间戳，央行 **2026-06-26 批量重建过站点**。按 article_id 判停，\n> **一次迁移后全部 id 变新，每次唤起都会翻满上限，且每期都被当成新文章。**\n\n**那是一段有 M0 实测支撑的论证，不是随手写的。TASK-011 要么推翻它、要么说明为什么现在不同了 —— 别让它静默失效。**\n\n---\n\n**Leader 已找到「为什么现在不同」的依据，但要你实测确认，不要照抄**：\n\n`store.go:519-523` 的 `refreshArticleID` 注释**直接回应了同一个担忧**：\n\n> 站点迁移换了 article_id，发布日不变。写新行会造出一个假修订；\n> **什么都不做则一级幂等检查（按 article_id 查）永远 miss，每月重抓一次。**\n> 正确动作是**刷新那一行的 id** —— 它记录「这行数据最后一次在哪个 URL 被看到」。\n\n⇒ 迁移后第一轮：id 全 miss ⇒ 不停 ⇒ 翻满 `MaxPages` ⇒ 候选被抓 ⇒ `Save` 走 `Duplicate` ⇒ **`refreshArticleID` 把 id 刷新** ⇒ **下一轮就命中、恢复正常**。\n\n⇒ **代价从「每次唤起都翻满」降级为「迁移后一轮全量重抓，之后自愈」**，而 `MaxPages: 3` 把那一轮的范围限制在约 45 条候选内。\n\n🔴 **你必须用测试把这个自愈实证出来**（这是本条的核心，不是背景说明）：构造「库里存旧 id、index 上是新 id」⇒ 第一轮翻满且候选非空 ⇒ 走完 `Save` 后 ⇒ **第二轮命中、正常停** 。**贴出两轮的实跑输出。**\n\n⚠️ 若你实测发现自愈**不成立**（例如 `refreshArticleID` 只在 observations 表刷、而 pending 里的旧 id 留着），**停下来告诉我** —— 那意味着这个方案的代价比我估计的大，可能要重新裁决。\n\n---\n\n🔴 **链条里最可能断的那一环（dev-agent-52 指出，先钉它）**：\n\n> `Save` 要走到 `Duplicate` 分支、并触发 `refreshArticleID`，前提是它**按二级键认出这是同一期**。\n> **如果二级键的判定条件比我们以为的窄**（比如还要求某字段相等），那一轮就会变成 `New` 而不是 `Duplicate`，\n> **id 永不刷新，「一轮后自愈」退化成「每轮全量重抓」** —— 也就是 `discover.go` 那段反论证\n> 原本担心的东西，**只是换了个位置发生**。\n\n⇒ **先读 `bitemporal.Lookup`/`Classify` 的实际判定条件，确认「同期次 + 新 article_id」真的判 `Duplicate`**，再去测两轮自愈。这一环不成立的话，后面整条链都不用测了。**把这一环的实测结论单独写进 discovery。**\n\n---\n\n⚠️ **Leader 曾给过一条「参考证据」，dev-agent-52 指出它比结论弱，此处如实更正**：\n\n`bitemporal/classify_test.go:31` 有一条名为「同键同 revision —— **站点迁移换了 URL**」的用例判 `Duplicate`。**但它直接构造 `State`、不经过 `Lookup`** ⇒ 它证明的是「**给定**那个 State，`Classify` 判 `Duplicate`」，**不是**「站点迁移后系统真的会走到 `Duplicate`」。\n\n⇒ **它是待证命题的一个前件，不是那个命题本身。** 证据的重心**完全落在 `Lookup`** 上：`Exists` 与 `LatestRevision` 从哪张表、哪几列取，**是否还要求 article_id 相等**。\n\n**读断言，不读用例名** —— 用例名是作者对场景的描述，断言才是它实际验证的东西（见 H11）。",
      "🔴 **空库首跑必须仍然翻满 `MaxPages`**（spec §4.3 的首跑行为，Sprint 036 有测试钉着）。换判据后这条**极易破**：若实现写成「发现候选就停」会直接违反 spec。**既有那条测试必须仍绿**，且你要在 discovery 里贴出它的实跑输出。",
      "**要不要保留 `HasPeriod` 作为二级键，由你判断并说明理由**（人类未定案）。\n\n考虑点：`HasArticle` 查两表含 pending ⇒ 一个反复未过闸的期次其 article_id 已见过 ⇒ 会停在它那里。**分析这是否是想要的行为**（index 按发布时间倒序，卡住的那期若排在前面，停在它之后的都更旧 —— 但「更旧」正是本任务要质疑的那个假设）。**结论写进 discovery，无论选哪边。**"
    ],
    "error_handling": [
      "**「因命中已见过而停」与「翻满 `MaxPages` 而停」必须在返回值或日志里可区分。**\n\n⚠️ 这条独立于判据本身：**当前最危险的属性是「静默」** —— 返回空 + `nil` error 与「今天真的没有新报告」完全同形。即使判据换了，**调用方仍应能知道 Discover 为何停下**。\n\n（TASK-005 的 dev-54 已实测：`Discover` 返回空时 ingest 打印 `no new reports` —— **那句话在停摆时也会打印**。）"
    ],
    "functional": [
      "**判停从 `HasPeriod(period, periodType)` 换成 `HasArticle(articleID)`**（TASK-003 已交付，查 observations + pending 两张表）。\n\n修订版有**新 article_id** ⇒ 不命中 ⇒ **不停、继续翻、被收进候选**；正常已处理的报告 article_id 已见过 ⇒ **照常停**。\n\n🔴 **必须有一条测试逐字复现上面那个场景**：已入库期次排第 1 页、未入库期次在第 2 页 ⇒ **断言未入库那期被发现**（而不是返回 0 条）。这是本任务存在的唯一理由，缺它等于没做。",
      "**接口调整**：`Discover` 的 `known PeriodChecker` 要改。由你定形态（换成 `ArticleChecker`、或合成一个同时提供两者的接口、或直接收 `*Store`），但**必须在 discovery 里写明选了哪种及理由**。\n\n⚠️ **同步改 TASK-005 的调用点**（`ingest.go` 已在你的 `writes` 里）。改完整包必须绿 —— **若 `ingest_test.go` 因签名变更而红，那不是你能改的文件**（属 TASK-007），**停下来告诉我**。"
    ],
    "non_functional": [
      "🔴 **顺手闭合一个真正的守卫缺口**（test-agent-26 核清，Leader 此前以为已修实为碰巧）：\n\n`TestPackageExposesNoWriteFunctions` 的失败文案里**手写着期望项数**（「恰好是这十七个」），而**没有任何东西比对它与 `len(want)`** —— 同一事实的两个副本，改一处不会让另一处变红。\n\n⚠️ **它此刻正确是副作用**：dev-53 交付时是「16 项 vs 文案十七」（**不一致且无人报警**），Leader 加 `\"Ingest\"` 后列表变 17，**文案碰巧变对了**。⇒ **下次加导出物时同样的不一致会再次发生。**\n\n**闭合方式**：让 message 由 `len(want)` 生成，而不是手写汉字数字。**一行的事。**\n\n（`store_test.go` 已在你的 `writes` 里，因为接口形态变更可能要动守卫。）",
      "**导出面守卫**：若接口形态变更导致导出物增减，按同款规则**登记**（`assert.Equal` 一字不动，只改切片项）。⚠️ `store_test.go` 在你 `writes` 里正是为此 —— **但 TASK-005/006 也在改它**，**你开工前先确认它们都已 `dev_done`**，否则会撞上后写者静默覆盖（见 `plan.md` 的串行裁决）。",
      "**消融自证**：把判停改回 `HasPeriod`，确认上面那条复现测试**转红**，且**红的是它**（贴出失败输出的具体那一行）。\n\n⚠️ 判据是「哪条断言红」不是「测试红不红」。",
      "覆盖率不低于当时的基线；`gofmt`/`vet` 空；整包 `-count=1` 与 `-race` 全绿；`go build ./...` OK。单跑用 `-run '^Top$/^Sub$'` 锚定式。"
    ]
  }
}
```
