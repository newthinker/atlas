# Sprint-030 QA 定向复审(review_fix 第 1 轮)

- 审查者:qa-agent-7 | 日期:2026-08-02 | 范围:`356d07c..d012927`(4 commit)
- 性质:**定向复审**——只验 fix 项是否真修好 + 有无回归,未重跑全量审查
- 纪律:所有结论锚定我本人跑的命令与探针输出,不采信转述

## 总 VERDICT:**PASS** — 可进 Step 7 交付归档

两条 CRITICAL 均在**根因层**消除并经我独立探针确认;M1/M2/M5/40203 全部落实;
你点名要我复核的两个判定(C2 判据、M2 等价突变)**我都判成立**,其中 C2 的判据选择
**比我 v1 报告的字面表述更正确**。无新增 CRITICAL/MAJOR,无回归。

### 我本人的验证基线(非转述)

| 项 | 命令 | 结果 |
|---|---|---|
| 全仓回归 | `GOTOOLCHAIN=local go test ./... -count=1` | **exit 0,FAIL 计数 0** |
| 静态检查 | `go vet ./internal/prism ./internal/collector/tushare ./internal/collector/twelvedata ./cmd/atlas` | 无输出 |
| gofmt | 本轮改动的全部 .go 文件 | 仅 `selector_test.go`(改动前遗留,按裁定不计入) |
| 密钥哨兵 | `git grep -nE "apikey=[A-Za-z0-9]{16,}"` | 零命中 |
| 工作区 | `git status --porcelain` | 干净(仅未跟踪的 `docs/collector/`) |

---

## 一、C1(apikey 外泄)—— **已消除,修法优于我的建议**

**受控复现原漏洞**(与我 v1 用的同一探针,隔离 worktree @ d012927):

```
error 文本 = twelvedata: NVDA: Get "http://127.0.0.1:1/time_series?end_date=2026-08-03
&interval=1day&outputsize=5000&start_date=2026-07-28&symbol=NVDA": dial tcp ... connection refused
Unwrap 链全程无 apikey ✅
```

apikey **已完全不在 URL 中**。我额外做了一项 v1 未做的检查:**遍历整条 `errors.Unwrap` 链**
断言任一层都取不回明文——通过。这正是 `wrapErr` 刻意不用 `%w` 的设计目的,该取舍成立:
本包无 sentinel error,消费方(prism)只格式化文本、不做 `errors.Is/As`,故无损失。

**结构性核验(我自查,非采信)**:`client.go` 5 个 error 返回点(:118/:125/:130/:135/:140)
**全部**经 `wrapErr`;包内 `fmt.Errorf` 仅 `wrapErr` 内部 1 处;空 key 时不做 ReplaceAll
(正确——`strings.ReplaceAll(s,"",x)` 会在每个字符间插入 x)。

**修法比我建议的更彻底**:我建议「改请求头**或**脱敏兜底」,实施是**两者都做**——
凭证不进 URL(根因,连正向代理 access log 的泄漏面一并堵掉)+ 脱敏兜底(防未来 error 来源不可穷举)。

**DoD 修订我认可**:原 functional[0]「query 含 apikey」与 C1 修复在逻辑上不可兼得,
安全优先是唯一正确裁决。

### 附带成果:C1 同类隐患全仓普查(我主动做的,超出你的要求)

既然该 bug 类已被证明可外泄到 Telegram,我普查了所有携凭证的采集器:

| 采集器 | 凭证注入方式 | 判定 |
|---|---|---|
| twelvedata | `Authorization` 头 + wrapErr 脱敏 | ✅ 本轮修复,现为仓内最强形态 |
| tushare | POST body(URL 无凭证) | ✅ 无泄漏面 |
| lixinger | POST + Header | ✅ 无泄漏面 |
| fred | query 含 `api_key`,但有 `stripURLError` 剥离 `*url.Error` | ✅ 已防护(**改动前就有**) |
| baostock | 无凭证(匿名) | ✅ N/A |

**结论:仓内无残留的 C1 同类实例。**

---

## 二、C2(全 NaN 毒化水位)—— **已消除;判据分歧我判 dev 对**

### 双向探针(我本人跑,非采信验证者结论)

```
全 NaN(PE/PB/PS 三值皆 NaN): Refreshed=0 写入行数=0
  Failed=[600519.SH: ... tushare fallback: fallback source returned no data: 1 rows fetched but PE/PB/PS are all NaN]
亏损股形态(PE=NaN, PB=1.8, PS=2.4): Refreshed=1 写入行数=1
  落库 D=2026-07-23 PE=NaN PB=1.8 PS=2.4
```

上行:v1 的 CRITICAL 场景已被挡住,**水位不推进**。下行:亏损标的的有效数据未被误伤。

### 判据分歧的裁决:**dev 对,我的字面表述不够精确**

先澄清一处事实:我 v1 报告的 CRITICAL 正文写的是
「至少一个点**三值非全 NaN**(**或**至少 PETTM 非全 NaN)」——**两个选项都给了,且三值版在前**;
是 fix_items 那行的速记「照 guardSchemaDrift 的判据」把它压缩得过窄。dev 选的正是我的首选项。

**更重要的是,dev 的论证在实质上就是对的**,与我 v1 的措辞无关:
- `daily_basic` 对亏损标的的真实形态是 `pe_ttm` 为 null 而 `pb/ps_ttm` 有值——**那是有效估值数据**。
- 按 PE-only 判据会把整段真实 PB/PS 判成「无数据」而拒绝落库,是**假阳性**,
  且后果不轻:亏损标的在 akshare 断源期间将**完全拿不到兜底**。
- 验证者的双向夹逼(去守卫→全 NaN 用例转红;改 PE-only→`KeepsRowsWhenOnlyPEIsNaN` 转红)
  是证明判据**边界**的正确方法,不是只证「能挡住」。我的独立探针复现了同样的两个方向。

**结论:判据选择成立,无需改动。**

### 残留(MINOR,登记后续,**不 gating**)

我 v1 曾指出「即使只有 PE 为 NaN(PB/PS 正常)也有害——该日水位推进后,akshare 恢复
也永远补不回那天的 PE」。在现判据下这一条**依然成立**:partial-NaN 行会推进水位。

但它**不应 gating**,理由:
1. 触发面窄:真亏损股 akshare 侧同样无有效 PE(无可回填之物);盈利股 tushare 的 `pe_ttm` 正常有值。
   两者背离的情形罕见。
2. **这是仓内既有属性而非本次引入**:akshare 主源侧 `guardSchemaDrift` 同样只在
   「**全部**行 PE 为 NaN」时报错,单行 NaN PE 照写照推水位。现判据让兜底跳与主源**行为一致**,
   反而消除了不对称。
3. 真正的根治是给 `valuation_daily` 加 `source` 列 + 让水位按「有效值」而非「有行」计算,
   那是独立的数据模型改动,超出本 Sprint。

---

## 三、M1 / M2 / M5 / 40203

**M1 已修**(探针):`Refreshed=0 Failed=[0700.HK: ... tushare fallback: price upsert: disk full] Degraded=[]`
——不再有「两条自相矛盾的 Degraded + Refreshed++」。且实现顺手去掉了 `upsertPrices` 中转,
直接构造 `PriceRow` 后判错,链路更短。

**M2 的「等价突变」判定:成立,我复核通过。** 我做了行为对拍:

```
现状(两道守卫):  零行 → Refreshed=0 写入=0 Failed=[... fallback source returned no data]
去掉 len(pts)==0: 零行 → Refreshed=0 写入=0 Failed=[... fallback source returned no data: 0 rows fetched but PE/PB/PS are all NaN]
该突变下 internal/prism 全包测试:ok(仍绿)
```

两者在**可观测行为上完全一致**(Refreshed / 写入行数 / `errors.Is(errFallbackNoData)` 均同),
差异仅是错误文本多一个后缀。根因是 `hasAnyValuation([])` 对空切片返回 false,
C2 守卫**在语义上包含**了零行分支——故 `len(pts)==0` 是冗余分支,**删除它是真等价突变,
突变存活是正确结果而非覆盖缺口**。验证者用「同时废掉两道守卫」证明用例有判别力,方法正确。

> 补充建议(非问题):既然 `len(pts)==0` 已被 `hasAnyValuation` 语义覆盖,保留它是**有意的显式性**
> (错误文本更精确地区分「零行」与「有行但全空」)。我**不建议**删——保留即可,只是别再把
> 它的突变存活当缺口反复追查。建议在该行加一句注释说明「与下方 hasAnyValuation 语义重叠,
> 保留是为错误文本可区分」,免得下一轮 QA 重走这条路。

**M5 已修**:`defaultBaseURL = "https://api.tushare.pro"`,注释说明了 token 走 body 故必须 https。

**40203 拆分已修**(探针,两种 msg 分别注入):
```
限频   → 600519.SH: tushare 限频,本次跳过,下次自动重试: tushare: rate limited: daily_basic (…频率超限(1次/分钟))
无权限 → 600519.SH: tushare 跳不可用(权限不足,配置性问题,不重试): tushare: no api permission: daily_basic (…没有接口访问权限)
```
文案已正确分叉,运维动作不再被误导。判据 `rateLimitMarker = "频率超限"` 只认四字、
不匹配具体窗口口径——与「窗口会自升级到 1 次/小时」的实测一致,**这个细节处理对了**。

---

## 四、你问的两点评估

### Q1. ADR 是否还有与现行代码矛盾的条目 → **没有**

我逐条核对了 10 条 ADR 与现行代码:

| ADR | 状态 |
|---|---|
| #2 TD / #3 40203 / #4 bridge / #8 symbol / #10 三跳 | ✅ 已修订,**逐条与代码相符**(我按代码核过,非只读文字) |
| #1 Stooq | 无代码面,N/A |
| #5 相邻两跳正交 | 成立(Prism 编排的三条链均跨数据商);行情链的跳序问题已由 #10 承载,不重复 |
| #6 baostock 只登记不消费 | 成立 |
| #7 密钥卫生 | 成立(C1 已修,普查无残留实例) |
| #9 未配置不提示 | 成立,`TestRefreshNilClientsSkipHops` 仍在 |

**一条建议(SUGGESTION,非矛盾、不 gating)**:ADR#7 目前只声明**结果**(「不入日志」),
未声明**机制**。而本仓已在同一 hazard 上撞了两次——fred 的 `stripURLError`(改动前)与
本轮的 C1。一个新采集器作者读 ADR#7 完全可能把 key 放进 query 并自认合规。
建议 ADR#7 补一句机制+两个参照实现:「**凭证不得进 URL query**——`*url.Error` 携带完整 URL
且 query 不脱敏;首选 twelvedata 的请求头方案,最低限度参照 fred 的 `stripURLError`」。

> 顺带一个值得记进 wisdom 的观察:**C1 与 C2 有同一个根因模式——新代码未沿用仓内既已存在的
> 保护**(C1 对应 fred 的 `stripURLError`,C2 对应 akshare 的 `guardSchemaDrift`)。
> 两次都不是想不到,是**没发现旁边就有**。这比两个缺陷本身更值得沉淀。

### Q2. 未修项(两条港股用例的同步改写提示未进 design §9)的取舍 → **取舍成立,我不反对**

理由(按证据):
1. **信息实际落在了更有效的位置**。我核实过:两条用例的**注释就在函数本体旁**
   (`refresh_test.go:1214` 与 `:1445`,后者明写两条用例的分工)。改这两条用例的人
   **不可能看不到**——在地注释的触达率高于任何外部文档。
2. **ADR#8 是比 design §9 更正确的归宿**。ADR 是「下游施工依据」,而 design §9 是里程碑状态表。
   ADR#8 已逐字点名两个测试函数名,且写明「顺序不能颠倒」。
3. **链路闭合**:design §9 第 447 行(后续任务第 4 面)已写「届时同步修订 ADR#8」→ ADR#8 → 两条用例名。
   从任一入口出发都能到达,最多一跳。
4. **代价评估正确**:`rework_count` 2/3,再开一轮即触顶 `max_rework` 转 `blocked_human`。
   把最后一格安全阀消耗在「文档搬家」上是明显的坏交易——那个阀是留给真死锁的。

**唯一补强建议(零成本,不需现在返工)**:等后续任务「限频感知退避」真正立项时,
把两条用例名直接抄进该任务的 description,省掉那一跳。

---

## 五、仍存在的问题分级(全部 **不 gating**,建议入 final-report)

**MINOR**
1. partial-NaN 行仍推进水位(见 C2 残留);根治需 `valuation_daily` 加 `source` 列 + 水位按有效值算。
2. `tushare/client.go` 仍无 `resp.StatusCode` 守卫(`eastmoney`/`lixinger` 有守卫+专门用例)。
   twelvedata 侧已用注释给出**有实测依据**的不做理由(401 仍回同一 JSON 错误信封),该理由我接受;
   tushare 侧无对应说明。
3. `bridge.py` 仍无重登录/健康端点,`bs.login()` 返回值未检查;单线程 `HTTPServer` 无超时。
4. `refreshTusharePrices` 仍全量拉 `LookbackYears`(默认 10 年)、无增量。
5. 非交易日窗口零行 → 假 FAILED 告警(长假每日推送);TASK-006 演练已实测。
6. `FallbackSource` 无白名单校验;第三跳仍被第二跳的 `fallback_source == "akshare"` 门控。
7. tushare「symbol → api」路由仍在 collector 层与 prism 层各实现一份(判据不同)。
8. `Refresh` 仍是 9 位置参数;`TestRefreshCNPathsNeverUpsertPrices` 名称仍失真。

**SUGGESTION**
9. ADR#7 补机制表述(见 Q1)。
10. 在 `len(pts)==0` 处加注释说明与 `hasAnyValuation` 的语义重叠(见 M2 补充建议)。

**已登记的后续任务**(我核实过登记到位):「限频感知退避」**一个**任务、四面
(退避消费分类 / 容量排队 / 路由层跳序与市场过滤 / 港股 symbol 归一+取数实证),
并在 spec §2、design §9/§10、plan、ADR 四处交叉引用。**与我 v1 的建议(合并为一个任务而非两个)一致。**

## 过程说明
- **跨模型对抗轮本轮同样未执行**(codex usage limit 未恢复,窗口称 Aug 20)。本轮为定向复审,
  影响小于首轮,但仍建议在 final-report 保留这条覆盖削弱说明。
- 复审探针在隔离 git worktree 内进行,**已 `git worktree remove` + `prune`**,未触碰任何仓内源文件。
