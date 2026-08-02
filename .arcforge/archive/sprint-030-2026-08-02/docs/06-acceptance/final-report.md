# Sprint-030 交付报告 — Prism M3.5a「数据源与降级链」

> 日期: 2026-08-02 | 需求: `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`
> Spec: `docs/superpowers/specs/2026-08-02-prism-m3.5-design.md`
> 提交范围: `dde9639..d012927`(11 commits) | autonomy: dod-gate | scheduling: dag

## 1. 交付内容

接入三个新数据源并把 Prism 降级链按 spec §2 扩展到位:

| 组件 | 交付物 | 覆盖率 |
|------|--------|--------|
| tushare 客户端 | `internal/collector/tushare/`(daily_basic/index_daily/hk_daily/daily + 40203 语义拆分) | 94.2% |
| twelvedata 客户端 | `internal/collector/twelvedata/`(time_series,8s 节流,end+1 闭区间补偿) | 92.7% |
| baostock 桥 | `scripts/baostock/`(venv+bridge.py 侧车) + `internal/collector/baostock/` + launchd plist | 95.7% |
| 配置接线 | `PrismConfig.BaostockBaseURL` + example 空 key 条目 | 95.9% |
| 编排层降级链 | `internal/prism/refresh.go` 四条新跳 + collector registry 二/三跳登记 | 94.0% |
| 部署与文档 | deployment.md 桥部署段、design §9/§10、spec §2、断源演练记录 | docs-only |

25 files changed, +2342/-66。全仓 `go test ./...` 61 包零失败,`go vet` 干净。

## 2. 任务与质量门禁

6 个任务全部 `verified`→`accepted`。返工统计:TASK-001 rw=2、TASK-002 rw=1、TASK-005 rw=1、
TASK-006 rw=2(上限 3,无熔断)。validator 通过;transitions 审计无越权(执行迁移全 `dev-*`、
判定迁移全 `test-*`、调度迁移全 `leader`)。

**QA 两轮 + 定向复审**:首轮 verdict = REJECT(2 CRITICAL + 5 MAJOR),review_fix 一轮收敛,
复审 verdict = **PASS**。

### 两条 CRITICAL(均由 QA 探针实证,已在根因层消除)

**C1 — Twelve Data 的 apikey 会外泄到日志与 Telegram**。`hc.Get()` 的 URL 带 apikey query,
传输层失败时 Go 返回的 `*url.Error` 含完整 URL(标准库只脱 userinfo 密码、不脱 query),该 error
经 `Report.Failed` → `cmd/atlas/prism.go` 打日志并拼进告警文案 → `SendText` 发出。TD 是备源,
只在 yahoo 失败后调用,面对的正是网络抖动这类传输层错误。修法比 QA 建议更彻底:apikey 移到
`Authorization` 请求头(凭证不进 URL,连正向代理 access log 的泄漏面一起堵掉)+ `wrapErr` 作为
包内唯一 error 出口脱敏兜底,且**不保留 error 链**(留链则 `errors.Unwrap` 可取回未脱敏原文)。
QA 复审遍历整条 Unwrap 链确认任一层都取不回明文,并普查全仓携凭证采集器确认无同类残留。

**C2 — tushare 估值兜底跳写「三值全 NaN 的行」永久毒化增量水位**。守卫只查 `len(pts)==0`
不查值;亏损标的的 `pe_ttm: null` 被填 NaN 后照写入库,`LatestDate=MAX(d)` 不过滤 NULL,
于是水位推进、次日 `start=latest+1`、那些日期永不重访,而报的是 "fallback ok"。修:加
「至少一点三值非全 NaN」守卫。**判据经过争议并定案**:QA fix_items 速记为「照抄 akshare
guardSchemaDrift」(PE 全 NaN 即报错),dev 指出那会误伤亏损标的(`pe_ttm` 为 null 而 `pb`/`ps`
有值是有效数据),Leader 采纳 dev 版本;验证者双向夹逼(去守卫→全 NaN 用例红;改 PE-only→
边界用例红)、QA 复审独立探针复现两方向,确认判据成立。

### DoD 修订(需记录在案)

TASK-002 `functional[0]` 原文要求「query 含 …/apikey」,与 C1 修复不可兼得(key 入 query 正是
外泄根因),Leader 裁决**安全优先**,修订为「apikey 经 Authorization 请求头传递,不得出现在
query」。因写通道不允许在 `dev_done` 后改 `done_criteria`,该修订以派验消息与本报告为准;
dev 在 discovery 的 `dod_deviation` 与测试映射注释两处显式标注,未静默偏离。

## 3. 已知覆盖削弱与遗留

**⚠ 跨模型对抗轮两轮均未执行**。`capabilities.codex_cli=true` 且二进制存在,但调用返回
usage limit(窗口称 2026-08-20 恢复)。按降级原则退回纯 Claude 三 lens(Skeptic/Architect/
Minimalist)。这是本次审查**已知的独立性覆盖削弱**,首轮影响大于定向复审轮。

**未实证项(均为外部依赖限制,非代码缺陷,已在 design §9/§10 如实标注)**:
1. **baostock 真桥取数** — 上游 `www.baostock.com:10030` 本机持续不可达(DNS 正常、80 通、
   10030 超时)。桥的启动/404/500 路径已验,真实行情解析路径只有假桥与替身覆盖。
2. **港股 hk_daily 取数** — 配置 4 位 `0700.HK` 与 tushare 所需 5 位 `00700.HK` 形态不匹配,
   客户端**刻意未做归一**(5 位能否取到数从未实证,两次探针均撞 1 次/小时限频;先归一等于
   拿假设换假绿)。现状是「被测试锁住的已知缺口」而非「被假测试掩盖的未知缺口」。
3. **A 股经 tushare 恢复数据** — 断源演练接线正确(跳被调用、Degraded 上报),但受限于
   `daily_basic` 1 次/分钟 + refresh 串行遍历,单次 refresh 只有第一个 A 股标的能兜底
   (实测 600519 通过、600036/000423 全 40203),窗口还会自升级到 1 次/小时。
   **该跳当前只能救个别标的,不构成批量断源的完整兜底**(当前 3 标的覆盖率约 1/3)。

**QA 复审列出的 MINOR 8 条 + SUGGESTION 2 条**(均不 gating,详见
`docs/05-review/sprint-030-qa-review-followup.md`):partial-NaN 行仍推进水位(仓内既有属性,
主源同款,根治需 `valuation_daily` 加 `source` 列)、tushare 无 StatusCode 守卫、bridge.py 无
重登录/健康端点、`refreshTusharePrices` 全量拉 10 年、非交易日假 FAILED、FallbackSource 无
白名单、tushare 路由两处实现、`TestRefreshCNPathsNeverUpsertPrices` 名称失真;
SUGGESTION 两条已当场落实(ADR#7 补机制、`len(pts)==0` 加注释建议)。

## 4. 后续任务(仅一个,已在 design §9 登记四面)

**「限频感知退避」**——四面合为一个任务,顺序不可颠倒:
1. `ErrRateLimited` 触发退避重试(现仅用于 Degraded 文案分叉,**尚无退避逻辑**)
2. A 股批量断源容量改善(解除 1 次/分钟 × 串行遍历的结构性限制)
3. 路由层跳序与市场过滤(`orderedCollectors` 走 map 遍历、从不调 `SupportedMarkets()`,
   故 ADR#10「三跳三故障域」当前实为「已登记,跳序未实现」)
4. 港股 symbol 归一 + 取数实证 —— **必须先靠第 1 面拿到正向证据再做归一**。
   立项时请把这两条用例名抄进任务 description:归一落地时
   `TestRefreshHKProductionSymbolHitsKnownGap`(`refresh_test.go:1450`)与
   `TestRefreshHKPriceOnlyHopWiring`(`:1219`)**必须同步改写**——前者锁定的正是
   「4 位取不到数」这个缺口,归一修好后它会反过来把修复判成失败,下一个人会误以为自己改坏了。
   届时同步修订 **ADR#8**(「symbol 形态天然一致」对 A 股成立、对港股不成立)。

## 5. 流程改进建议(本 Sprint 实测所得,已全部记入 wisdom)

1. **门禁 OTHERS 白名单缺 `verified` 与 `review_fix`** — 这两类任务的未提交改动会被算成当前
   transition 者的 scope 漂移,且随 verified 累积而恶化(最后一个转 `dev_done` 的人承担全部
   存量)。本 Sprint 撞了三次。Leader 侧对策已执行(verified 后尽快提交);机制侧需人类会话外
   把两个状态补进 `task-completed.sh` 的白名单。
2. **追加需求必须写进 `fix_items`,消息只作通知** — D5 只经 inbox 传达,dev 转 `dev_done` 后
   文档 mtime 再无变动即证明未落盘,而验证者验完在册项判 verified 完全成立(「验证者只能对
   在册项负责」)。这是「文件是唯一真相源」在需求层的应用。
3. **转 `verifying` 前先确认无追加项** — 交棒后追加需求会强制多一轮完整往返;机制侧可考虑
   增加 `verifying → review_fix` 边(leader 持有)。
4. **变异/派生副本一律用 `git worktree`** — 仓内派生目录(`*_mut`)会误触发他人门禁。注意
   **scratchpad 独立 module 对 `internal/` 包不可行**(Go 可见性规则,编译器直接拒绝),
   这是 worktree 成为唯一可行方案的原因。
5. **文档中指向代码的引用一律用函数名,不用行号** — 实测行号引用不是「可能过期」而是
   **静默指向语义完全无关的代码**(计划文档引的 `refresh.go:80-90` 现指向 `Report` 结构体与
   `degrade` 方法,与所称的 edgar→engine 结构毫无关系,照此施工直接走错)。
6. **突变测试必须能区分「突变生效但没杀掉」与「突变根本没生效」** — shell perl 替换把含
   `!`/`|` 的 Go 代码静默写坏,差点得出方向相反的结论;改用 Python 精确替换 + 找不到目标即
   报 MUTATION-MISS。另:**存活突变要先判是不是等价突变**再下「测试缺口」结论。
7. **两条 CRITICAL 同一根因:新代码没沿用仓内既有的保护** — C1 对应 fred 的 `stripURLError`,
   C2 对应 akshare 的 `guardSchemaDrift`,两次都不是想不到而是没发现旁边就有。对策:新增同类
   组件前先读最相似的既有组件,逐条问「它做的防护我要不要」。ADR#7 已据此补上机制而非只声明结果。
8. **idle hook 路由缺陷** — 发给 dev 的 idle 提醒被路由到其 spawn 的子代理会话(子代理连收 7 次
   并正确拒绝),真正该被唤醒的实例收不到,会造成「看起来在催、实际无人推进」的静默停滞。

### 待同步 hooks 清单(人类执行)

| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 Sprint 未改动 `project-template/hooks/` 或 `project-template/scripts/` 下任何文件 | — |

本 Sprint 全部改动落在目标仓库业务代码与 `docs/`,未触碰运行时资产(`.claude/hooks/`、
`.claude/scripts/`、`settings*.json`)——上述流程改进建议 1/3/8 涉及运行时 hook 的修改,
**均未自行实施**,列在此处供人类决定是否走 project-template → TDD → 人类确认的正式路径。
复制后如有新增/改动脚本请补可执行位,再运行
`bash project-template/scripts/check-runtime-sync.sh` 核对运行时副本与模板一致
(agent 只呈现清单,不执行这些命令)。
