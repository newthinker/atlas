# Sprint-030 QA Code Review — Prism M3.5a 数据源与降级链

- 审查者:qa-agent-7 | 日期:2026-08-02
- 范围:`dde9639..356d07c`(7 commit,+2091/-79,32 文件)
- 方法:第一轮常规审查(含我本人跑的全仓回归 + 8 项突变矩阵 + 4 个定向探针)
  第二轮对抗审查(Skeptic / Architect / Minimalist 三 lens 独立 context)
- **跨模型轮降级说明**:`capabilities.codex_cli=true` 且 codex CLI 存在,但实际调用返回
  `ERROR: You've hit your usage limit ... try again at Aug 20th, 2026`,**跨模型对抗未能执行**,
  按 CLAUDE.md 原则 4 退回纯 Claude 跨视角三 lens。此为本次审查的已知覆盖削弱项。


> **v2(2026-08-02 收敛版)**:应 Leader 要求补入「跨视角裁决」与「返工项 → TASK 编号映射」
> 两节;v1 全部内容保留未删。

---

## 跨视角裁决(三 lens 判定不一致的显式仲裁)

Leader 转述的三视角结论与我核对 transcript 后的实际结论有一处出入,先更正:

| lens | Leader 转述 | **transcript 实际结论(我已核对)** |
|---|---|---|
| Skeptic(怀疑论) | 1 CRITICAL + 6 MAJOR + 13 MINOR + 7 测试有效性 | 一致 |
| Architect(架构师) | MAJOR 无 CRITICAL,NEEDS WORK | 一致(5 MAJOR + 8 MINOR/建议) |
| Minimalist(极简) | (未提 CRITICAL) | **有 1 CRITICAL** —— twelvedata apikey 泄漏 |

⚠ **这处出入很关键**:被漏掉的那条恰是本次最严重的发现(凭证外泄)。若按转述理解,
会以为唯一的 CRITICAL 争议只有水位那条,从而整条漏掉 C1。

### 裁决:两条 CRITICAL 均**成立**;Architect 的「无 CRITICAL」不构成反对意见

**这不是一场三方分歧,而是三个 lens 的审查面不重叠。** 裁决理由:

1. **Architect 的 brief 本身就没覆盖这两条的问题面**。它被指派审「设计合理性/扩展性/抽象边界/
   依赖方向/接口设计/与既有代码一致性」,产出也确实全部落在这些维度(9 参签名、两套声明机制、
   路由实现两遍、半截开关、桥运维契约)。它**从未检查**错误传播路径与 NaN 写入路径。
   **沉默不是「判定为非 CRITICAL」,而是未曾考察**——故不存在需要在「Skeptic 对 vs Architect 对」
   之间二选一的分歧。
2. **两条 CRITICAL 我都没有采信 lens 的转述,而是自己写探针复现过**(证据见下 C1/C2 两节的
   实际输出)。裁决锚定的是可复现证据,不是 lens 的票数。
3. 反向印证:Architect 在其擅长面上**独立得出了与我一致的 40203 降级结论**
   (「`ErrNoPermission` 唯一用途是拼 Degraded 文案,编排层本来就只调一次」),
   说明它在自己覆盖到的地方判断是可靠的——这恰恰支持「它对 C1/C2 的沉默源于未覆盖而非否定」。

**逐条裁决**:

| 条目 | 提出方 | 我的裁决 | 依据 |
|---|---|---|---|
| 全 NaN 行毒化水位(`refresh.go:475-486`) | Skeptic | **维持 CRITICAL**(本报告 C2) | 我的探针实证 + 仓内 `guardSchemaDrift` 先例 |
| twelvedata apikey 泄漏(`twelvedata/client.go:92-95`) | Minimalist | **维持 CRITICAL**(本报告 C1) | 我的探针实证 + 外泄链路逐跳核实到 `SendText` |
| 「prism.go 不判 enabled」 | Skeptic MAJOR / Architect MAJOR / Minimalist MINOR | **降级为 MINOR** | 采信 Minimalist:`prism.go:191` lixinger 是同款既有先例;且 runtime config 中两者均 `enabled: true`,当前零现实影响 |
| 「40203 语义重载」 | Skeptic 补充分析 / Architect 判「文案准确性非行为正确性」 | **MAJOR,非 CRITICAL** | 采信 Architect,并经我独立核实 `ErrNoPermission` 唯一消费点 |
| 「零行判失败在非交易日产生假 FAILED」 | Architect 质疑已批准决策 | **受理为 MINOR/后续** | TASK-006 演练已实测到该现象,质疑成立但不 gating |

## 总 VERDICT:**REJECT**

**2 CRITICAL + 5 MAJOR**。两条 CRITICAL 均由我本人以可复现探针实证(非 lens 转述),
且三个 lens 与我本体在「兜底跳静默假成功」这一问题类上结论一致(共识,非分歧),
故按 rubric 判 REJECT 而非 CONTESTED。

**但请准确理解 REJECT 的范围**:六个任务的**功能实现主体是正确的**——全仓
`GOTOOLCHAIN=local go test ./... -count=1` 我亲自跑过,**exit 0、零 FAIL**;8 项突变里
6 项被测试杀死(港股路由/TD 闭区间补偿/40203 映射/baostock 符号转换/collectors 副本
ApplyDefaults 时序修复/仅价格跳不写估值行),说明核心契约有真实测试保护;go vet 干净;
密钥哨兵在仓内零命中。REJECT 由两条**新发现的、可导致凭证外泄与永久数据缺口**的缺陷驱动,
两条修法都很小,预计一轮 review_fix 可收敛,**不需要设计返工**。

---

## CRITICAL

### C1. Twelve Data 的 apikey 会经 `Report.Failed` 外发到 Telegram 与日志

**文件**:`internal/collector/twelvedata/client.go:92-95`

```go
resp, err := c.hc.Get(c.baseURL + "/time_series?" + q.Encode())   // q 内含 apikey
if err != nil {
    return nil, fmt.Errorf("twelvedata: %s: %w", symbol, err)      // err 是 *url.Error,含完整 URL
}
```

Go 的 `http.Client` 在传输层失败时返回 `*url.Error`,其 `URL` 字段是**完整请求 URL**
(`stripPassword` 只去 userinfo 密码,**不去 query 参数**)。

**实证**(我在隔离 worktree 跑的探针,已删除):

```
error 文本 = twelvedata: NVDA: Get "http://127.0.0.1:1/time_series?apikey=SUPERSECRETKEY123456
&end_date=2026-08-03&interval=1day&outputsize=5000&start_date=2026-07-28&symbol=NVDA":
dial tcp 127.0.0.1:1: connect: connection refused
```

**外泄链路(逐跳已核实)**:
1. `refresh.go:322` 把该 error 包进 `"price history: %v; twelvedata fallback: %v"`
2. `refresh.go:164` 写入 `rep.Failed`
3. `cmd/atlas/prism.go:127` 打日志;`prism.go:132-133` 拼
   `"⚠️ Prism 刷新部分失败:\n" + strings.Join(rep.Failed, "\n")`
4. `prism.go:144` `d.sender.SendText(msg)` → **Telegram**

触发条件极普通:TD 是**备源**,只在 yahoo 已失败时才被调用,而它面对的正是网络抖动
/DNS 失败/超时/代理故障这类传输层错误(本机 env 有 `http_proxy=127.0.0.1:7897`,
docs/deployment.md:411 自陈踩过代理坑)。一次网络抖动 = 真实 apikey 进 Telegram 聊天
记录与 launchd 日志,且 Telegram 侧不可撤回。

**直接违反本 Sprint 自己的 ADR#7**:「token/key 只入 runtime config;**不入日志**」。

**建议修复**:apikey 改走请求头(TD 支持 `Authorization: apikey <key>`),使其永不进入 URL;
或在 error 包装前用 `strings.ReplaceAll(err.Error(), c.apiKey, "<redacted>")` 兜底。
建议补一条与探针同款的用例把「error 文本不含 apikey」钉死。

> 附:tushare **无**此问题(token 在 POST body,URL 无凭证,`client.go:64-67` 已核实)。

### C2. tushare 估值兜底跳会写下「估值三值全 NaN」的行,永久毒化增量水位

**文件**:`internal/prism/refresh.go:475-486`(`refreshTushareValuation`)

守卫只有 `len(pts) == 0`,**只数行数、不看值**。

**实证**(探针,tushare 返回 1 行 PE/PB/PS 全 NaN):

```
C1: Refreshed=1 Failed=[] Degraded=[600519.SH: akshare failed (aktools down), tushare fallback ok]
C1: 写入估值行数=1
C1: 落库行 D=2026-07-23 PE=NaN PB=NaN PS=NaN → 该行使 MAX(d) 水位推进到 2026-07-23
```

**危害链**:`sqlite.go:265` `toNull(NaN)→nil` 照样 INSERT → `sqlite.go:255-263`
`LatestDate = SELECT MAX(d) FROM valuation_daily` **无 NULL 过滤**,行存在即推高水位 →
次日主源恢复后 `refresh.go:197-199` 算出 `start = latest+1`,**这些日期永不重访**,
真实估值永久缺失且无任何告警(报的是 `fallback ok`)。

**这不是理论推演,是本项目自己写下的理由**——`refresh.go:491-494` 为「仅价格」跳
逐字论证过同一机制并据此刻意不写估值行;A 股公司估值跳却完全裸奔,**同一文件内自相矛盾**。

**更强的佐证是仓内既有先例**:akshare 主源对**同一 hazard** 已有专门守卫——
`internal/collector/akshare/client.go:85` `guardSchemaDrift`(「拉取 >0 行但全部 PETTM 为 NaN
→ 返回 error,让静默漂移变成 Report.Failed 告警,而非产出一整段空 PE 的序列被下游当正常数据」),
且**两个入口都调用了**(`stock.go:106`、`index.go:42`)。新增的 tushare 估值跳写入**同一张
被水位管理的表**,却漏掉了这层保护。

触发面并不刁钻:`daily_basic` 对亏损标的返回 `pe_ttm: null`(配置内 600519/600036/000423
任一有亏损季即可);`client.go:112-119` 把 null/非数值一律置 NaN。即使只有 PE 为 NaN
(PB/PS 正常)也有害——该日水位推进后,akshare 恢复也永远补不回那天的 PE。

**建议修复**:照搬 `guardSchemaDrift` 的判据,在 `refreshTushareValuation` 建完 `vps` 后加
「至少一个点三值非全 NaN(或至少 PETTM 非全 NaN)」门槛,不满足返回 `errFallbackNoData`;
或在 `upsertWithLocalPercentile` 内跳过三值全 NaN 的新点不建行。

---

## MAJOR

### M1. 「仅价格」跳落价失败仍上报 `fallback ok` 且 `Refreshed++`(零写入假成功)

**文件**:`internal/prism/refresh.go:512` — `return upsertPrices(...), nil`(error 硬编码 nil)

`upsertPrices`(`refresh.go:251-260`)按设计**永不返回 error**,理由(`refresh.go:248-250`)是
「价格对估值链路是附属产物,写失败不应让当日估值刷新整条判负」。该理由在 engine/edgar
路径成立(那里还有估值行要落),**在仅价格跳上不成立——价格是该跳的全部交付物**。

**实证**(注入 `store.priceErr["0700.HK"]`):

```
M2: Refreshed=1 Failed=[]
M2: Degraded[0]=0700.HK: price_daily upsert failed (disk full), valuation unaffected
M2: Degraded[1]=0700.HK: akshare failed (aktools down), tushare fallback ok(仅价格,估值缺失)
M2: 实际落库价格行数=0
```

两条 Degraded 自相矛盾,`Failed` 为空,运维看 `Refreshed` 计数会以为当天没事。

**建议**:`refreshTusharePrices` 把 `upsertPrices` 的非空返回转成 error 上抛。
**测试缺口**:`refresh_test.go:988` `TestRefreshPriceUpsertFailureDegradesOnly` 只覆盖
edgar/engine 两个子用例,仅价格跳 + `priceErr` 的组合无覆盖。

### M2. 兜底跳的「零行判失败」守卫有 3 处,仅 1 处被测试锁定

我跑的 8 项突变矩阵中,**两项存活**:

| 突变 | 位置 | 结果 |
|---|---|---|
| 删除 A 股估值零行守卫 | `refresh.go:479-481` | **SURVIVED**(= Leader 通报的 M8) |
| 删除美股 TD 零行守卫 | `refresh.go:318-320` | **SURVIVED**(**本次新发现,此前无人报告**) |
| 删除港股/指数仅价格零行守卫 | `refresh.go:505-507` | KILLED(`TestRefreshTushareEmptyIsNotSuccess`) |

两处存活守卫都是**载荷性**的,不是可有可无的防御。探针证明去掉后行为翻转:

- TD 侧:`Refreshed=1`、`Degraded=[NVDA: yahoo price failed (yahoo 503), twelvedata fallback ok]`、
  `upserts=0 prices=0` —— 假成功 + 零数据
- A 股估值侧:`Refreshed=1`、`Degraded=[600519.SH: akshare failed (aktools down), tushare fallback ok]`、
  `upserts=0` —— 同款

**建议**:各补一条用例(与我探针同构,约 15 行/条)。这也是 C2 的同类问题,建议与 C2 一并修。

### M3. 港股跳的 symbol 形态未归一,该跳大概率恒返回零行,而仓内已有现成修法

**文件**:`internal/prism/refresh.go:431` 直接把 `inst.Symbol` 透传给 `ts.FetchHKDaily`。

三条互证:配置是 4 位(`config.example.yaml` `symbol: "0700.HK"`);**本仓 tushare 单测自己用 5 位**
(`internal/collector/tushare/client_test.go:73` → `{"hk_daily", "00700.HK", ...}`);
**同一问题仓内已有解**——`internal/collector/lixinger/valuation.go:40`
`c := fmt.Sprintf("%05s", strings.TrimSuffix(symbol, ".HK"))`。

TASK-005 discovery 与 §10 已把此登记为残留风险,我不重复定性;但要点出一个**测试有效性**问题:
`refresh_test.go:1206` `TestRefreshHKFallsBackToTusharePriceOnly` 的 fake 以 `"0700.HK"` 为 map key
返回数据,于是用例恒绿,而生产恒零行。这条用例目前证明的是「fake 的 map 能按 key 取值」,
不是「港股跳能取到数据」。**建议**:要么复用 lixinger 的 `%05s` 归一(一行),
要么在 design/spec 把「港股备源」显式标注为 NOT AVAILABLE,不计入「三跳三故障域」。

### M4. registry 里的「A股行情二跳/三跳」在路由层没有跳序,且不按 SupportedMarkets 过滤

**我本人核实的代码**:`internal/app/app.go:556-571` `orderedCollectors` = `preferred` +
`registry.GetAll()` 的其余项;`internal/collector/registry.go:39` `for _, c := range r.collectors`
是 **map 遍历,顺序不确定**。

后果:(1) eastmoney 失败后的「下一跳」是 crypto/lixinger/fred/qlibpit/tushare/baostock 中
**随机一个**,ADR#10 宣称的「二跳 tushare、三跳 baostock」**无任何机制保证**;
(2) `orderedCollectors` 完全不读 `SupportedMarkets()`,声明 `[MarketCNA]` 的 baostock 会被
拿去跑美股 symbol(叠加 `baostock/client.go:34-40` 的 `bridgeCode("BRK.B") → "b.BRK"` 产生垃圾请求),
tushare 会被拿去跑美股 `daily`(空 items,白烧节流与积分)。

**建议**:要么按 `SupportedMarkets()` 过滤 + 显式跳序表排序,要么把 ADR#10 的「三跳三故障域」
措辞降级为「已登记,跳序未实现」——**当前文档宣称与代码事实不符**。

### M5. tushare 默认端点是明文 `http://`,token 在 body 里明文过网

**文件**:`internal/collector/tushare/client.go:20` `const defaultBaseURL = "http://api.tushare.pro"`

token 虽未进 URL(故无 C1 那类日志泄漏),但整个请求体走明文 HTTP,凭证对路径上任意中间节点
可见。**建议**改 `https://api.tushare.pro`。

---

## MINOR / 建议(不 gating,建议入 final-report 或后续任务)

1. `tushare/client.go:67-92` 与 `twelvedata/client.go:92-108` **均不检查 `resp.StatusCode`**,
   而 `baostock/client.go:54` 检查了——同一批改动内三个客户端两种标准。仓内惯例是检查的:
   `eastmoney` 与 `lixinger` 都有专门用例断言「HTTP 500 即使 body 是合法 JSON 也必须报错」
   (`eastmoney_test.go:244-282`、`lixinger/valuation_test.go:142`)。
2. `scripts/baostock/bridge.py:11` `bs.login()` 仅模块加载时执行一次且**未检查返回值**
   (而同文件 :30 对查询结果检查了 `error_code`,标准不一);无重登录、无 `/health` 端点。
   会话中途失效时每个 `/daily` 恒 500,而 `KeepAlive` 只发现「进程死了」,发现不了「活着但已废」。
   docs/deployment.md:419-425 覆盖了「启动时 login 失败」,未覆盖「运行中失效」。
   另:用 `HTTPServer` 而非 `ThreadingHTTPServer` 且无 server 端超时,一个卡住的查询堵死整座桥。
3. `refresh.go:501` `refreshTusharePrices` 每次全量拉 `LookbackYears`(默认 10 年 ≈2450 行)、
   无增量,与本文件其余所有路径的增量口径相反;叠加 hk_daily 最严限频,且逼近 tushare 单次
   返回行数上限——上限截断是静默的。
4. **非交易日窗口稳定产生假 FAILED**:周末/长假运行且 akshare 断源时,`incrementalStart` 不 skip
   → `daily_basic` 返回 0 行 → `errFallbackNoData` → 进 `Failed` → 每天推 Telegram 告警。
   TASK-006 演练已实测到这一现象(「周末窗口必然零行」)。建议给零行判定加「窗口内是否存在
   可能交易日」的前置守卫。
5. `refresh.go:118` 的第三跳嵌套在 `inst.FallbackSource == "akshare"` 分支内,
   **第三跳被第二跳的配置门控**:`source: lixinger` 但未写 `fallback_source` 的标的即使配了
   tushare key 也永远拿不到三跳。另 `FallbackSource` 无白名单校验,写 `fallback_source: "tushare"`
   会被静默当作「无兜底」。
6. tushare 的「symbol → api」路由**实现了两遍**且判据不同:`tushare/collector.go:48-55`
   按 symbol 形态,`refresh.go:429-436` 按 config 元数据。作者自己写明走错 api 是静默零行
   (`collector.go:37-39`),两份路由漂移的症状因此不可见。
7. `refresh.go:132-144` 的 akshare 分支触发 tushare 跳只看 `ts != nil`,不看 `inst.FallbackSource`,
   而 lixinger(:109)与 edgar(:149)分支都看——无法对单个标的关掉该跳。
8. `cmd/atlas/prism.go:197-198` 只判 `APIKey` 不判 `Enabled`,与 `collectors.go:82` 的
   `Enabled && APIKey != ""` 两套语义。**我把 lens 给的 MAJOR 降级为 MINOR**,证据:
   (a) `prism.go:191` 的 lixinger 是同款既有先例,非本 Sprint 引入;
   (b) 我查了 runtime `configs/config.yaml`,tushare 与 twelvedata **均为 `enabled: true`**,
   当前无现实影响。但方向是「静默开启」而非「静默关闭」,建议抽 `collectorEnabled(cfg,name)` 共用。
9. `collectors.go:91` 的 `prismCfg.BaostockBaseURL != ""` 在 `ApplyDefaults()` 之后**恒为真**
   (`config.go:509-511` 保证非空),是死条件;实际语义是「prism.enabled 即注册 baostock」,
   且没有 `collectors.baostock.enabled` 开关。
10. `refresh.go:118` 传给三跳的 `mainErr` 是 `fbErr`(akshare 的错误),Degraded 文案写
    `"lixinger+akshare failed"` 却只带 akshare 的原因,**lixinger 的原始错误在 Degraded 里丢失**。
11. `valuation_daily` 无 `source` 列,akshare(百度口径)与 tushare(官方口径)的行混在同一序列,
    `upsertWithLocalPercentile` 又跨口径合并算滚动分位,口径接缝会污染分位且事后不可区分/不可定向回填。
12. `tushare/client.go:103-119`:`di >= len(item)` 与 `time.Parse` 失败**静默丢行**,
    `item[i].(float64)` 断言失败**静默置 NaN**。上游把 `close` 改成字符串编码时会得到
    「N 行、全 NaN」——正是 C2 的输入形态。建议至少统计丢弃行数,超比例报错。
13. 节流不跨进程共享:`collectors.go:82` 与 `prism.go:197` 各自 `New` 出独立的 `mu/lastReq`,
    且 `serve` 与 `prism-daily` 是两个 launchd 进程,同一 token 实际速率可达设计值 2 倍,
    抬高撞 40203-限频的概率。
14. `refresh.go` 的 `Refresh` 已达 9 个位置参数(5→6→7→9 的演进趋势已成立);
    仓内有现成先例 `cmd/atlas/prism.go:116` 的 `prismRefreshDeps` struct 可循。
15. `refresh_test.go:1021` `TestRefreshCNPathsNeverUpsertPrices` 名称在本次改动后已失真——
    CN 路径(HK/指数仅价格跳)现在确实会写 `price_daily`。
16. tushare 日期参数(`client.go:127-130`)用宿主本地时区格式化,而 lixinger 路径有专门的
    时区边界用例(`refresh_test.go:445,460`),新增的两条 tushare 路径无对应用例。
17. `config.example.yaml` 新增的 `markets:` 字段全仓无读者(惰性字段);
    `baostock/client.go:34-40` `bridgeCode` 的「已是桥形态」透传分支属 YAGNI。
18. `install-services.sh` 只校验 plist 存在,不校验 venv 已建;venv 缺失时 launchd 无法 spawn,
    `KeepAlive` + `ThrottleInterval 10` 每 10 秒刷一次错误日志(与 aktools 同款既有模式,非本 Sprint 引入)。

---

## 三个专项评审结论

### 专项 1:40203 语义重载 —— 定级 **MAJOR / 建议纳入本轮 review_fix 顺带修,但其本身不构成 CRITICAL**

**关键事实(我独立核实,Architect lens 独立得出同一结论)**:
`ErrNoPermission` 在全仓的**唯一消费点**是 `refresh.go:450`,作用**仅是拼一条 Degraded 文案**。
编排层对 tushare 跳**本来就只调用一次、永不重试**(`refresh.go:448-449` 注释自陈),
且这与错误类型无关——任何错误都是调一次就返回。ADR 也明确「不做跨标的记忆化跳过」。

⇒ **误分类不改变任何执行路径**:不产生错误数据、不推进水位、不抑制次日重试
(次日 refresh 会照常再试该跳)。后果**纯粹是运维文案误导**——TASK-006 演练已实锤:
Degraded 写「权限不足,配置性问题,不重试」,同一行 tushare msg 却写「频率超限(1次/分钟)」,
运维会去查积分档而实际只需等窗口。

**触发面(按当前 configs/config.yaml 实测标的构成)**:A 股公司 3 个(600519/600036/000423)、
港股 1 个(0700.HK)、A 股指数 2 个。akshare 断源时 3 个 A 股标的**串行** + `daily_basic`
1 次/分钟 ⇒ 第 2、3 个必撞限频被误报(TASK-006 实测:600519 通过,600036/000423 全 40203)。
港股每日仅 1 次调用,1 次/分钟窗口本身不撞,但 TASK-005 观测到窗口会自升级到 1 次/小时。
即断源时约 2/4 条 A/H 兜底会被误报。

**修复风险低,且不破坏 TASK-001 已验收契约**:判别依据现成(msg 含「频率超限」即临时)。
`TestErrNoPermission`(`tushare/client_test.go:99-109`)用的 msg 是「抱歉,您没有接口(income)
访问权限」,**不含「频率超限」**,故加一条优先级更高的限频分支后该用例仍绿,
TASK-001 的 functional[2] 只被**收窄**而非推翻。成本约 5 行 + 1 用例。

**结论**:**不是 CRITICAL**(无数据正确性后果)。但既然已经必须开一轮 review_fix 修 C1/C2,
建议把这 5 行一并带上——`docs/prism/atlas_prism_design.md` §10 第 6 条目前写的
「…而**不重试**」属**描述夸大**,应同步改为「文案误导」而非「行为错误」,否则会误导后续任务定优先级。

### 专项 2:M8 突变存活 —— **需本轮加固,且范围应从 1 处扩到 3 处**

Leader 通报的是 1 处(A 股估值零行)。我的突变矩阵 + 探针查出这是一个**问题类**而非孤例:

| # | 位置 | 状态 | 失败模式(已探针实证) |
|---|---|---|---|
| 1 | `refresh.go:479-481` A 股估值零行 | 突变存活,无用例 | `Refreshed++` + `fallback ok` + 零写入 |
| 2 | `refresh.go:318-320` 美股 TD 零行 | 突变存活,无用例(**新发现**) | 同上 |
| 3 | `refresh.go:475-486` A 股估值**全 NaN 行** | **无守卫**(= C2) | `Refreshed++` + `fallback ok` + 写毒化水位的空行 |

第 3 项比前两项严重(前两项是零写入,第 3 项是**写入劣质行并永久阻断回填**),
且它恰好落在 #1 同一个函数里——说明「只补 #1 的用例」会给出虚假的安全感。
**建议 review_fix 一次性覆盖三处**:#3 加守卫 + 用例,#1/#2 各补一条锁定用例。

### 专项 3:tushare 估值兜底容量 —— **spec 文案必须修正,并立后续任务**

spec §2 当前表述为:

> | A股公司·估值 | akshare 百度 | **tushare daily_basic**(新) | — | tushare 为官方计算口径 pe_ttm/pb/ps_ttm |

读起来是「一条对整个 A 股公司类目可用的备源」。**实测不成立**:`daily_basic` 限频 1 次/分钟,
而 prism refresh **串行**遍历标的 ⇒ 单次 refresh 中**只有第一个 A 股标的**能用 tushare 兜底
(TASK-006 实测:600519 通过,紧随的 600036/000423 全部 40203 限频),且窗口连续触发后
**自升级到 1 次/小时**。当前 3 个 A 股公司标的 ⇒ 批量断源时兜底覆盖率约 1/3。

**建议**:
- **(a) 文案修正(建议纳入本轮)**:spec §2 该行备注与 `docs/prism/atlas_prism_design.md` §9/§10
  补注「该跳容量 ≈ 每次 refresh 1 个标的,批量断源场景下不成立,仅救个别标的」。
  不改文案的话,下一个读者会按「A 股公司估值已有备源」做容量假设。
- **(b) 立后续任务**:限频感知退避——识别 msg 含「频率超限」→ 退避重试(而非当永久错误直接
  放弃),配合标的间隔调度。**这与专项 1 的分类修复天然同源**(都要先把限频从 `ErrNoPermission`
  里分出来),建议合并为**一个**后续任务,而不是两个。

---

## 建议的 fix_items(供 Leader 生成 `review_fix` 用)

**gating(必须修)**:
1. **[C1]** `twelvedata/client.go`:apikey 改走请求头或在 error 包装前脱敏,
   补用例断言 error 文本不含 apikey。
2. **[C2]** `refresh.go` `refreshTushareValuation`:补「全 NaN 行」守卫
   (照 `akshare/client.go:85` `guardSchemaDrift` 的判据),补用例。
3. **[M1]** `refresh.go:512` `refreshTusharePrices`:`upsertPrices` 写失败转 error 上抛,补用例。
4. **[M2]** 补两条零行守卫锁定用例(`refresh.go:479-481` 与 `refresh.go:318-320`)。

**建议同批(成本极低)**:
5. **[M5]** `tushare/client.go:20` 改 `https://`。
6. **[专项1]** 40203 拆分限频分支(msg 含「频率超限」→ 临时错误)+ 同步修正 §10 第 6 条措辞。
7. **[专项3a]** spec §2 / design §9§10 补注 tushare 估值跳的容量边界。

**入 final-report,不 gating**:M3(港股形态,二选一:归一 or 文档标 NOT AVAILABLE)、
M4(ADR#10 跳序宣称与代码不符,二选一:实现跳序 or 降级措辞)、上列 MINOR 1-18。

**reason_class**:`task_defect`
(依判定表:实现与 done_criteria 的质量意图不符 + 测试覆盖不足;非 DoD 自相矛盾,非环境故障,非无进展)

---

## 返工项 → TASK 编号映射(Leader 开 review_fix 用)

各 TASK 的 `packages` 已核对 `.arcforge/tasks/TASK-00X.json`,按「修复落点所在包」归属。

### 必须返工(gating)

| # | 级别 | 修复要点 | 落点文件 | **TASK** | 当前 status | owner |
|---|---|---|---|---|---|---|
| C1 | CRITICAL | apikey 改走请求头(TD 支持 `Authorization: apikey <key>`),或 error 包装前 `strings.ReplaceAll(err.Error(), c.apiKey, "<redacted>")`;**补用例断言 error 文本不含 apikey** | `internal/collector/twelvedata/client.go:92-95` | **TASK-002** | review_fix ✅ | dev-agent-33 |
| C2 | CRITICAL | `refreshTushareValuation` 建完 `vps` 后加「至少一点三值非全 NaN」门槛,不满足返 `errFallbackNoData`(判据照抄 `akshare/client.go:85` `guardSchemaDrift`);补用例 | `internal/prism/refresh.go:475-486` | **TASK-005** | **verified — 需转 review_fix** | dev-agent-32 |
| M1 | MAJOR | `refreshTusharePrices` 把 `upsertPrices` 的非空返回转 error 上抛(该跳价格是全部交付物,不适用「附属产物」豁免);补用例 | `internal/prism/refresh.go:512` | **TASK-005** | 同上 | dev-agent-32 |
| M2 | MAJOR | 补两条零行守卫锁定用例(A 股估值 `:479-481`、美股 TD `:318-320`) | `internal/prism/refresh_test.go` | **TASK-005** | 同上 | dev-agent-32 |

> **TASK-005 目前仍是 `verified`,但承载 3 条 gating 项(C2/M1/M2),必须一并转 review_fix。**
> TASK-001/002 你已转好。TASK-003/004/006 无 gating 项。

### 建议同批(成本极低,不单独 gating)

| # | 修复要点 | 落点文件 | **TASK** |
|---|---|---|---|
| M5 | `defaultBaseURL` 改 `https://api.tushare.pro`(现为明文 http,token 在 body 明文过网) | `internal/collector/tushare/client.go:20` | **TASK-001**(已 review_fix) |
| 专项1 | 40203 拆限频分支:`code==40203 && strings.Contains(msg,"频率超限")` → 新的临时错误(优先级高于 `ErrNoPermission`)。**不破坏 TASK-001 契约**:`TestErrNoPermission` 的 msg 不含「频率超限」,加分支后仍绿 | `internal/collector/tushare/client.go:87-89` | **TASK-001** |
| 专项1-doc | §10 第 6 条「…而**不重试**」改为「文案误导」而非「行为错误」(描述夸大) | `docs/prism/atlas_prism_design.md` | **TASK-006** |
| 专项3a | spec §2 该行 + design §9/§10 补注「该跳容量 ≈ 每次 refresh 1 个标的,批量断源不成立」 | spec `2026-08-02-prism-m3.5-design.md`、`docs/prism/atlas_prism_design.md` | **TASK-006** |

### 二选一,需 Leader 定夺(文档宣称与代码事实不符)

| # | 选项 A(改代码) | 选项 B(改文档) | **TASK** |
|---|---|---|---|
| M3 港股形态 | 复用 `lixinger/valuation.go:40` 的 `%05s` 归一(一行) | design/spec 把「港股备源」标 NOT AVAILABLE、不计入三跳 | A→**TASK-001** / B→**TASK-006** |
| M4 跳序 | `orderedCollectors` 按 `SupportedMarkets()` 过滤 + 显式跳序表 | ADR#10「三跳三故障域」措辞降级为「已登记,跳序未实现」 | A→**TASK-005** / B→**TASK-006** |

> M3 若选 A,建议落在 **TASK-001**(`FetchHKDaily` 内归一,路由真相源收敛到 tushare 包一处),
> 而非 TASK-005 的 `tushareFallback`——后者会让形态知识散落两处,与 MINOR-6「路由实现两遍」同源恶化。

### 不返工,入 final-report

MINOR 1-18(见上节),以及**跨模型对抗轮未执行**这一覆盖削弱项(codex usage limit)。
