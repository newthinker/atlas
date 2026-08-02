# M3.5a 架构决策记录

均为 spec 定案决策(2026-08-02 brainstorming + 实测),此处登记 rationale 供下游引用。

> **⚠ 本文件已按 Sprint-030 实施结果修订(2026-08-02 QA 后)**:第 3、8、10 条的原始前提
> 已被实施与实测推翻或收窄,修订内容直接写在各条内并标注「实施修订」。ADR 是下游施工依据,
> 停在旧前提上会让人得到与现行代码相反的假设,故就地修订而非另立勘误。

1. **Stooq 出局** — 全站 SHA-256 PoW 反爬墙,非 IP/配额问题,代理无效;headless 跑 PoW
   不值得。M3.5 原立项的 Stooq 备源改为 Twelve Data 转正。
2. **Twelve Data 只服务 Prism 标的池美股备源** — 免费层 800 次/天,不适用 ETF 成分股
   (~500 只)场景;8s 最小间隔节流内置在客户端;缺 key 该跳静默跳过。
   **实施修订(TASK-002 C1)**:apikey 经 `Authorization` 请求头传递,**不入 query**——
   Go 的 `*url.Error` 携带完整 URL 且 query 不脱敏,会经 `Report.Failed` → 日志与 Telegram
   告警外泄。另设 `wrapErr` 为包内唯一 error 出口做脱敏兜底,且**不保留 error 链**
   (留链则 `errors.Unwrap` 可取回未脱敏原文)。
3. **Tushare 角色 = A/H 估值+行情备源** — 实测基础积分档:daily/index_daily/daily_basic/
   hk_daily ✅;us_daily/income ❌(40203)。`index_dailybasic` 经 TASK-001 live 探针实测
   **40203 无权限**,指数链尾定为**仅价格**。
   **实施修订(TASK-001 S1)**:原文「40203 定义为永久性错误(ErrNoPermission),不触发降级链
   重试」**已不准确**。实测 40203 是**语义重载码**——限频与无权限同码,msg 含「频率超限」者
   为**临时**错误(窗口口径自身会变:同 token 先报 1 次/分钟、75 秒后报 1 次/小时)。现拆分为:
   `ErrRateLimited`(临时,判据只认「频率超限」四字、与窗口口径无关)与 `ErrNoPermission`
   (永久,仅非限频的 40203)。二者互斥、均 `%w` 包装、`errors.Is` 可判。
4. **Baostock 走侧车桥** — 纯 Python 库无 HTTP API,仿 aktools 成熟模式(独立 venv/
   launchd KeepAlive/仅绑 127.0.0.1:8181);bridge 用 stdlib http.server 不引 FastAPI;
   复权口径 adjustflag=3 不复权(与 PE 计算一致)。失败面仅影响 A 股行情第三跳。
   **实施补充(TASK-003)**:bridge 对 baostock `error_code != "0"` 显式 raise 转 500——
   参考实现的静默 `200 []` 会把上游故障伪装成「无数据」,下游不触发降级下一跳。
5. **降级链按「市场×能力」组织** — 相邻两跳必须在数据商/网络路径/封禁面至少一层正交
   (v1.4.0 双失败教训);标的级 fallback_source 仅作例外覆盖。
6. **baostock 本批只注册 collector registry,Prism 编排不直接消费** — Prism 编排不
   消费 A 股行情,行情三跳属 collector 侧;避免 Refresh 签名进一步膨胀。
7. **密钥卫生** — token/key 只入 runtime config(gitignored);commit 前哨兵 grep;
   不入 plist、不入日志、不入任何被 git add 的文件。见第 2 条的 C1 修订(日志外泄面)。
   **机制(QA SUGGESTION 补充,防第三次重犯)**:只声明结果不足以防重犯,故明确做法——
   **凭证不得进 URL query**。`*url.Error` 携带完整 URL 且 query 不脱敏(标准库只脱 userinfo
   密码),传输层失败时会经 `Report.Failed` 流向日志与告警通道。首选 twelvedata 的
   **请求头方案**(凭证根本不进 URL,连正向代理 access log 的泄漏面一起堵掉);最低限度参照
   `collector/fred` 既有的 `stripURLError`。本仓已在同一 hazard 撞过两次(fred 与本轮 C1),
   新增采集器前请先读最相似的既有采集器做了哪些防护。
8. **symbol 形态** — tushare ts_code 与 Atlas 形态天然一致("600519.SH"),无需映射;
   baostock 需桥内转换("600519.SH"→"sh.600519")。
   **⚠ 待修订(TASK-005/006 实测)**:本条**对 A 股成立、对港股不成立**。配置为 4 位
   "0700.HK",tushare hk_daily 需 5 位 "00700.HK",客户端**不做归一**,生产形态调用返回
   `code=0` 但 items 为空数组(静默空)。归一**刻意未做**:5 位形态能否真取到数据从未实证
   (两次探针均撞 hk_daily 限频 1 次/小时),先归一等于拿假设换假绿。修订随后续任务
   「限频感知退避」第 4 面落地——**顺序不能颠倒**:先解限频拿正向证据,再一次做对归一。
   届时 `internal/prism/refresh_test.go` 的 `TestRefreshHKProductionSymbolHitsKnownGap`
   与 `TestRefreshHKPriceOnlyHopWiring` **两条用例必须同步改写**(前者锁定的正是「4 位取不到数」
   这个缺口,归一修好后它会反过来把修复判成失败)。
9. **spec §3「备源未配置」Degraded 提示不实现(reviewer Fix-2 定案)** — nil 客户端时
   Refresh 行为与改动前完全一致(计划 TestRefreshNilClientsSkipHops 为准)。理由:主源
   失败已有 failed 上报;未配置属永久状态,天天提示违反 spec §2「不得天天降级」原则。
10. **A 股行情二跳补齐(reviewer Fix-1)** — tushare(FetchDaily)与 baostock 客户端均
   登记 collector registry,凑齐 spec §2「三跳三故障域」;避免 FetchDaily 成死代码。
   **⚠ 实施修订(QA M4)**:「三跳三故障域」**在路由层未实现**——三源确已登记,但
   `orderedCollectors` 走 `registry.GetAll()` 的 map 遍历(顺序不确定)且**从不调用
   `SupportedMarkets()`**(声明 CN_A 的 baostock 可能被拿去跑美股 symbol)。故本条当前
   实为「三源已登记为备源,跳序与市场过滤未实现」,真正的跳序并入后续任务第 3 面。
