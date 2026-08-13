# Changelog — Sprint 037（Hestia M1b-4b：CLI 接线与部署 + 季报支持）

**范围**：`63ac5b6` → `f4d601753ac323ca37d5757da5a547493e3de090`
**QA 终审**：PASS · **11/11 accepted** · `internal/hestia` 92.1% → **93.6%**

## 新增

- **`atlas hestia ingest`** —— launchd 入口，每日 15:30/17:30/21:30 抓央行《金融统计数据报告》
  （`cmd/atlas/hestia.go`，TASK-008）
- **`atlas hestia status`** —— 打印近期观测与 pending 行，含解析后的绝对库路径
  （`internal/hestia/status.go`，TASK-006）
- **`hestia.Ingest`** 编排 —— 发现 → 抓取 → 解析 → 校验 → 入库，**单期失败不中断整批**
  （`internal/hestia/ingest.go`，TASK-005）
- **`Store.HasArticle` / `HasArticleInObservations` / `RecentObservations` / `RecentPending`**
  （`internal/hestia/store.go`，TASK-003 / TASK-006 / TASK-011）
- **`configs/hestia.yaml`** —— 阈值配置实例，每个数字带 M0 实测来源，含 `config_version`
  （TASK-009；**改它不需要重新编译**）
- **`deploy/launchd/com.newthinker.atlas.hestia-ingest.plist`** —— **一个代理键都不设**
  （hestia 直连央行），已登记进 `scripts/ops/install-services.sh` 的安装循环（TASK-009）
- **`internal/hestia/CONTRACTS.md`** 新增「Sprint 037」节（约 200 行）

## 变更

- 🔴 **`Discover` 的判停规则：期次 → article_id**（TASK-011，人类 2026-08-13 定案）
  - **修掉的缺陷**：央行重发同一期时，修订版发布时间最新、期次却是旧的且已入库
    ⇒ 第一条就命中判停、当场返回 ⇒ **其后所有未入库期次静默消失，`err == nil`、退出码 0**
  - 连带：去重键同步从期次改成 article_id（否则「原文与修订版同时在榜」时原文在查库前就被 `continue` 掉）
- 🔴 **`--force` 现在穿透两层幂等**（`Discover` 判停 + `ingestOne` 的 `HasArticle`）
  ⇒ 可重跑**任意**期次，包括已在观测表的。**代价是会翻满 `MaxPages`——那是 `--force` 的语义，不是意外**
  - ⚠️ **但它穿不透第三层**：同一篇 `published_at` 不变 ⇒ 恒判 `Duplicate` ⇒ `refreshArticleID`
    **只刷 `article_id`，重抽的 Values 会被丢弃**。flag help 与 `IngestDeps.Force` 注释已点名这一点
- **`StopReason` 三态**（`seen_article` / `max_pages` / `exhausted`）：候选非空时也打印停止原因；
  **`max_pages` 走 stderr**（窗口外可能还有未发现的期次），**不改退出码**（首跑必然 `max_pages`，改了会产假红）
- 🔴 **季报支持**（人类 2026-08-12 定案，原计划完全没覆盖）：
  - 链接层放宽 `articleLinkRE`（真实前三季度报的 article_id 是 **7 位**，而原正则要求 `\d{14,}`
    ⇒ **第 15 页起一条候选都产不出，完全静默**）（TASK-001）
  - 标题层认季报，`checkPeriodTypeSupported` 把季度类型显式拒绝到 TASK-004（TASK-010）
  - 抽取层 `periodAlt` + `cumulativePeriods` **两处都加**（TASK-004；只改一处是 no-op）
- **三个 cobra 命令均设 `SilenceUsage`** —— 失败时从 13 行降到 1 行。
  **两个叶子命令各自都必须设**：cobra 判的是「被执行的那个命令或根命令」，**不查中间祖先**（实测）

## 修复（QA 返工轮）

- **CRITICAL** `plistIntsUnderKey` 全文档扫描 + 按下标配对 ⇒ **排班键名改一字母、Hour/Minute 跨 dict
  错配都全绿**，而 launchd 会忽略未知键 ⇒ **job 永不唤起**。改为 `plistSchedule`：先认出
  `StartCalendarInterval` 后的 `<array>`，逐 `<dict>` 收成对字段，缺任一即失败（TASK-009）
- **CRITICAL** `TestHestiaFlags` **只断言 flag 存在**，从不断言它绑到哪个变量
  ⇒ `BoolVar` 改成 `Bool` 后 **948 测试全绿而 `--force` 静默失效**。补
  `TestHestiaFlagsBindToVariables`：经 cobra 参数解析、**肯定式**判据、两个 flag 拆两个子测试
  （TASK-008；**代码作者 dev-agent-52，由 dev-agent-53 代为提交，provenance 已注明**）
- `plistEnvKeys` / `plistIntsUnderKey` 的 `if err != nil { break }` ⇒ **解析错误静默返回部分键**。
  改为 `errors.Is(err, io.EOF)` 才 break、其余 `require.NoError`。
  **攻击样本是仓库里现成的 `aktools.plist`**（注释含 `--`，`plutil -lint` 报 OK 而 `encoding/xml` 拒绝）
- **「零候选」路径的守卫改道**：判据换 article_id 后，原先唯一走到 `len(cands)==0` 的用例改走了
  另一条路 ⇒ **一个月 28 天都会走的路径变成未覆盖，而测试数没少、全绿**。补
  `TestIngestReportsNothingNewOnSecondRun`（TASK-011）
- 三份「Verdict 恒为 `New` / `Duplicate` 不可达」的**过期结论副本**（`ingest.go` / `ingest_test.go` /
  `store.go`）—— 保留「决定 + 理由」、删掉「事实陈述 + 过期坐标」，引用改**字串锚**
  （TASK-007 / TASK-011）

## 端到端验收（真实网络 + launchd）

```
真跑     2026-06 New → hestia_observations     ← 抓到的正是本 Sprint 追加的季报/半年报类型
二次跑   no new reports (stopped: seen_article)，status 与首次逐字节一致
launchd  plutil -lint OK · bootstrap EXIT=0 · launchctl list 可见
```

⚠️ **只安装了 hestia 一个服务**，未跑 `install-services.sh`（它会重启全部 11 个服务）。

## 已知遗留（不阻断，建议单开 chore）

- `TestQuarterlyPeriodsAreCumulative` 用**枚举**列表 ⇒ 新增期次类型时漏改一处不会红
- `scanPage` 接受绝对 URL（跨源抓取面）· `status` 会建库
- `cmd/atlas/backtest_test.go` / `crisis_test.go` 未过 gofmt（**base 上就是**）
- 🔴 **`aktools.plist` 是非法 XML** 而 `plutil -lint` 报 OK ——
  **它现在是 plist 解析守卫的真实攻击样本**
- 6 处失效行号锚（`git blame` 确认全部早于 `63ac5b6`）。
  ✅ **本 Sprint 新增契约一处行号锚都没用**
