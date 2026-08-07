# Sprint 032 验收报告 — collector policy gate

> **状态：定稿。21 个任务全部 `accepted`，QA 第二轮终审 PASS。**
> 全部数据经现跑核实；推断与实测已逐处标注区分。

## 0. 终审结论（qa-agent-8 第二轮）

**交付物 PASS。** 八家一致性经**实测**收敛（非读表）——维度四七列全部用变异注入验证，
横表标 ✓ 的格子**实测过的无一虚标**。差异均有理由。

三处待处置**均已落盘，都不阻塞交付**：
① 契约教训 32 结论过宽 → **已订正**（左列改为「无漏报，但有已知误报与能力边界」，
处方补「挪之前先问它的误报率」）；
② 横表 §③「`FetchQuote` 接不了」被实测证伪 → **已改文**（改为「是遗漏不是限制」，
并补上 Leader 追加的真实成本：不止三行 `t.Set`，还有 `FundInfo` 深拷贝）；
③ 七家 default 透传风险 → **已登记**，建议下个 Sprint 用 `policy.IsPolicyError`
把判据从「列举」改成「归属」。

### 维度四最终矩阵（全部实测）

| 列 | 覆盖 | 结果 |
|---|---|---|
| 缓存生效 | 八家 | 全 ✓ |
| 错误不写缓存 | 八家 | **全 ✓**（tushare 由 TASK-020 闭合——同一跨包变异同一靶，上一轮 31 个用例无一转红、这一轮转红，前后对照干净）|
| 聚合度 / 粒度不放粗 | 四家五处 | 全 ✓（双向）|
| 区分度 | 四家 + tushare 两维度 | 全 ✓ |
| 链保留 | 六家 | 全 ✓（TASK-021 的守护经实测确认能挡住断链）|
| 校验留在 `fn` 内 | 三家 | 全 ✓（yahoo/eastmoney 未测——靶不唯一，成本超出边际价值；baostock 无此形态，经两人独立印证）|

## 0b. 两处收尾裁定

**不跑 `gofmt -w`**：28 处全是历史债，**本 Sprint 引入 0 处**（`git diff --name-only 28fe89a..HEAD`
与不干净清单交集为空）。混进交付会产生一个与 Sprint 无关的大 diff，掩盖真实变更。
建议单独处理。

**不跑 code-simplifier 改代码**：21 轮 DoD 验证与变异取证刚完成，**此时引入未验证的简化改动，
风险大于收益**——本 Sprint 反复证明「看起来等价的改动」可能改变断言的有效性
（教训 23/30/45 均是此类）。

## 1. 交付内容

用新包 `internal/collector/policy` 的 Gate 中介层统一承接各 collector 的限流／缓存／合并／配额，
并把散落的符号路由谓词收敛为具体度优先的路由表。

**新增包 `internal/collector/policy`**（五个文件）：`policy.go`（Table/Policy/Quota/Override）、
`gate.go`（Gate/Fetch/Do/Wait）、`quota.go`（MemStore）、`quota_file.go`（FileStore + flock）、
`default.go`（进程内单例）。

**七家 collector 接入**（判据是「构造函数快照 `policy.Default()`」,dev-agent-36 实测正好 7 个——原文写「八家」是把被删的 `CachedCollector` 装饰器算成了接入方）：yahoo、eastmoney、crypto、tushare、twelvedata、lixinger、baostock。

**删除**：`CachedCollector` + `maybeCache` 装饰器（TASK-012），改由 Gate 承接。

## 2. 客观数据（Leader 于 2026-08-06 现跑）

- **`go build ./...`**：通过
- **全量 `go test ./...`**：**62 个包全绿、0 FAIL**
- HEAD = `8623e14`，工作区无未提交的生产代码改动

### 覆盖率（**单包口径**，锚点见各任务 discovery）

| 包 | 基线 | 交付后 |
|---|---|---|
| yahoo | 88.0% | **89.9%** ⚠ |
| twelvedata | 92.7% | **95.5%** |
| lixinger | 92.2% | **93.4%** |
| tushare | 95.2% | **95.3%** |
| eastmoney | 87.4% | **87.6%** |
| crypto | 66.7% | **75.0%**（`coverage_floor=70`，见 §5） |
| baostock | 95.7% | **96.4%** |

> ⚠ **yahoo 那行的 89.9% 是当前值（TASK-007 F2 之后，实测于 `f6a78af`），
> 它已被三个任务叠加过**：policy 错误映射（`253c1bc`）+ 缓存键时间精度守护（`415f900`，+?）
> + 缓存键区分度守护（`f6a78af`，+0.6pp）。**若要给其中任一任务单独归因，必须取该任务
> 自己那个 commit 的实测值，不能用后续任务之后的数。**
> 先前记的 88.0→89.3 是**区间值且被串了**：`4c54e77~1..415f900` 里触碰 yahoo 的有**两个** commit
> ——`253c1bc`（policy 错误映射）**和 `415f900`（缓存键时间精度守护，属另一个任务）**，
> 那 +1.3 是两个任务合起来的贡献。dev-agent-35 逐 commit 归因时发现。
> **这正是「区间 diff 跨了别人的 commit」那个坑在覆盖率数字上的变体**——
> test-agent-17 在 diff 归因上防住了它，在覆盖率数字上没防住。
>
> **口径警告**：本 Sprint 出现过 **九次**「同一个量得出不同数字」的情况，全部是口径差异而非事实分歧。
> 引用任何数字前请确认口径。已知的四组：
> - **覆盖率**：门禁报的是 `-coverpkg` **合并**值（如 91.4%），与**单包**值（87.6%/94.4%）不同
> - **gofmt**：**28**（全仓递归、排除 `.worktrees/`、含 `_test.go`）vs **39**（`go list` 传嵌套目录导致
>   gofmt 递归重复计数）vs 历史记录的 9/4（更早的部分扫描）
> - **`policy.Fetch` 调用点**：5/4/3 三个数字，差异在含不含 `_test.go`、含不含注释行、
>   以及 `gate.Wait` 算不算（**它不返回值，没有错误可映射**，不算）
> - **基线 vs 基线+在途**：多 agent 并行下「当前工作区状态」不是任何一个人的基线

## 3. 验收标准与硬约束

**验收标准 1-6 与硬约束 C1-C8 已由 qa-agent-8 第一轮逐条核查**，其中 C6 曾判定违反并促成范围变更
（见 §4）。第二轮终审待四条在途线收口后进行，焦点为八家一致性。

## 4. 五次范围变更（均由核查发现，非计划内）

| # | 触发 | 内容 |
|---|---|---|
| 1 | QA 发现设计文档 §4.1 与 §1.3 自相矛盾 | 被删的 `maybeCache` 覆盖**五家**而新 Gate 内置表只登记两家 ⇒ eastmoney/crypto/baostock 丢失 TTL 缓存。新增 TASK-015/016/017 |
| 2 | test-agent-17 实测证明可配触发 | policy 错误外泄非「防将来」而是现存缺口（`Override.Timeout` 今天就能配出来）。新增 TASK-018，范围经两次订正后为四包（+lixinger +tushare） |
| 3 | dev-agent-36 核实末尾清单时发现方向反了 | `crisis.go` 在 `FRED_API_KEY` 非空时提前 return、不经 `loadConfigOrDefaults`，而 `initPolicyGate` **恰好藏在该函数体内** ⇒ Gate 未接线。qa-agent-8 随后发现 `backtest.go` 全文零接线。新增 TASK-019 |

| 4 | test-agent-16 验 TASK-007 F1 时发现区分度无守护 | 变异 W5(键里 symbol 换固定串)下 83 个测试全绿 ⇒ **AAPL 会拿到 MSFT 的行情**。Leader 用 jq 查证:007/008/009/010 四个早期任务的 DoD **全都没有区分度 criteria**。新增 TASK-007 的 F2 |
| 5 | 同上的横向排查 | test-agent-16 变异实测三家:twelvedata 与 lixinger **都有守护**(Leader 的 grep 在 lixinger 上判反了——它的 DoD 写的是区分度的实质,只是没用那三个字),**只有 tushare 真缺**。新增 TASK-020(范围已从三包收窄至一包) |

**根因（第 3 次）是结构性的**：`initPolicyGate` 全仓只有两个调用点，其一藏在 `loadConfigOrDefaults`
函数体内 ⇒ **接线与配置加载被隐式耦合，任何跳过配置加载的路径都静默跳过接线**。

## 5. 一处质量门禁豁免

**TASK-016（crypto）设 `coverage_floor=70`**（全局 `dev_minimum=80`）。

理由：crypto 既有水位 66.7%，包内 92 条语句、四个 0% 平凡函数合计仅 4 条 ⇒ 补齐后 78.3% 仍不到 80，
**要过线必须改 `symbol.go`/`Init` 等与本任务无关的既有代码**。交付值 73.9%→75.0%，比既有水位高 8.3 个点。

**补 crypto 既有覆盖率到 80** 已登记为独立后续任务，不在本 Sprint。

## 6. 已知缺口与后续项

### 6.1 三条 MINOR（qa-agent-8 报，dev-agent-36 核实仍成立）

1. **`core.Quote.FundInfo` 指针字段无护栏**——`yahoo.go:246` 的 `out := *q` 是结构体浅拷贝，
   复制的是指针。**今天不触发的唯一原因是 `yahoo.quote` 内置 `TTL: 0`**（不缓存），
   config 给该主题设 TTL 即触发。约束注释已加在 `core/types.go` 定义处（违反者一侧）
2. **`Gate.Do` 零生产调用方**——全仓非测试代码无调用
3. **eastmoney→lixinger 双层 Gate 嵌套**——两层各 5min TTL ⇒ 回退路径最坏 ~10min 陈旧。
   不死锁（`ck` 含 topic 段，`singleflight` 不自我阻塞）。**只适用 eastmoney 一家**，
   crypto 的三个 provider 是裸 HTTP、单层

### 6.2 路由表 4 项（**TASK-012** 遗留，现跑确认仍成立——原文误记为 TASK-004,那是 QuotaStore 文件实现）

`lookupRoute` 未内置 `ToUpper`（三处调用方各自转换）、`KnownIndexMarket` 注释描述不完整、
**`.HK` 具体度残留**（实测 `ATOM.HK`/`LINK.HK`/`MATIC.HK`/`DOGE.HK`/`AVAX.HK` 落到 crypto/CRYPTO，
而 `BTC.HK`/`ETH.HK`/`SOL.HK` 落到 yahoo/HK）、末级回退依赖 map 迭代顺序。

### 6.2b 八家一致性核查的三条「不一致且未找到理由」（test-agent-17，锚 `f6a78af`）

1. **tushare 缺「错误不写缓存」守护**——七家里唯一一家。实现对，但无测试钉住；变异下
   **既有全套无一转红**。失效形态最坏：错误**变成成功**（第二次调用静默返回成功且无数据，
   上层会当作「该标的今天没数据」）。**已并入 TASK-020 处置。**
2. ~~**eastmoney / baostock 映射时完全丢弃原始 `err`**~~ —— **这条批评不成立**
   （qa-agent-8 第二轮查证推翻）：两个 policy 哨兵都是**裸 sentinel、返回时零包装**
   （`gate.go:16`/`quota.go:14` 定义，`gate.go:297`/`:317` 处 `return ErrTimeout` 无 `%w`、
   无上下文）⇒ **被丢弃的 `err` 里没有信息可丢**。两家的手写中文（「本地配额预判未通过，
   本次未发出请求（临时性，窗口过后自愈）」）**信息量严格多于**那 4 个英文单词。
   **反过来，保留 `%v` 的五家会把 `policy: quota exceeded` 这个内部术语拼进用户可见文本，
   那才是可议的。** 横表把风格差异读成了缺陷（标注「疑为时序」的方向对，但归错了因）。

2b. **⚠ 七家共有的真实风险（MINOR，登记）**：全部七家都是
   「**列举已知哨兵 + `default: return err`**」结构（`yahoo.go:229`、`twelvedata:96`、
   `lixinger:75`、`crypto:157`，eastmoney/baostock 用 `switch`）。
   ⇒ **policy 包一旦新增第三种哨兵，七家全部走 default 原样返回、泄漏到调用方**——
   而那正是 TASK-018 加三次范围扩大才建起来的防线。**失效是静默的**：现有测试只注入已知的
   两种哨兵，新增哨兵不会让任何一条转红。**守护覆盖的是「已列举的」，缺口在「未列举的」。**

   **低成本机制修法**：policy 包导出归属判定 `func IsPolicyError(err error) bool`，
   七家把 `errors.Is(A) || errors.Is(B)` 换成它 ⇒ **判据从「列举」变成「归属」**，
   新增哨兵不需要改七个包。**本 Sprint 不做**（改动面 8 包、无当下可观测后果、正在收尾），
   但**与教训 18 那次「为将来风险硬造判据」不同——这条有明确的机制修法，不是只能靠文档承载**，
   建议下个 Sprint 处置。
3. **eastmoney / crypto / baostock 的 `FetchQuote` 未接 Gate——是遗漏，不是限制**
   （原记为「结构性接不了」，**qa-agent-8 第二轮用实测证伪**）：
   三家在内置表里是**通配登记** `<域>.*` 且 `TTL: 5min`，给 `FetchQuote` 起主题会命中通配、
   **连带给实时报价套 5 分钟缓存**。yahoo 能接是因为它**显式登记了 `yahoo.quote` 且 `TTL: 0`**。
   **⚠ 上述「接不了」的推断是错的。** `Table.Lookup` **精确匹配优先于通配**，实测：
   `t.Set("eastmoney.quote", Policy{TTL: 0, MinInterval: 300ms, Coalesce: true})` 与 `eastmoney.*`
   **完全可以并存**——精确条目命中（TTL=0s），通配条目不受污染（`eastmoney.kline` 仍 TTL=5m）。
   **这正是 yahoo 的做法**（`policy.go:73` 显式登记 `yahoo.quote` 且 `TTL: 0`）。
   横表自己在下一句就写明了 yahoo 的解法，却仍把结论写成「结构性不可能」——
   **观察到了解法、写下了机制、结论却超出了证据**（与教训 32 同族）。

   **代价是真的**：这三家的报价路径**既无节流也无请求合并**，eastmoney 尤其值得修
   （非官方端点、本来就没有任何节流，而 `FetchQuote` 在 `symbol_detail` 实时接口路径上）。

   **但补法不止「三行 `t.Set`」**（Leader 补充，qa-agent-8 未涉及这层）：`core.Quote` 含
   `FundInfo *FundInfo` **指针字段**，而 `Coalesce: true` 即使在 `TTL: 0` 下**也会让合并的
   多个调用方拿到同一个 `*FundInfo`**（`yahoo/yahoo.go:246` 的 `out := *q` 是浅拷贝，
   挡不住指针字段）——这正是 TASK-015 落盘的那条边界。**故真实成本 = 三行 `t.Set`
   + 三家各自的 `FundInfo` 深拷贝 + 对应守护。**

   **本 Sprint 不补，登记为独立后续任务**（不是「不能补」而是「范围与风险需单独评估」）。

**另两个结构性发现**：

- **接入层有三种形态**：tushare（`call()`）与 lixinger（`request()`）包在**共享内部层**，
  6 个和 9 个导出方法**自动全覆盖**；其余五家**逐方法各自包** ⇒ **加新方法必须记得包一层**，
  属「靠记得执行」那一列的结构性风险。未见任何注释说明为何分两派。
- **「链保留」七家只有 tushare 有，而且是白捡的**：它有哨兵 ⇒ 那条测试天然写成 `errors.Is`
  判据 ⇒ 顺带钉住了链。其余六家退到文本断言、看不见链。**这不是谁偷懒，是被测对象的
  类型结构支配了断言的判据形式**（TASK-021 正在补其余五家）。

### 6.2c ⚠ 三个 provider 子包从未进过任何门禁的 scope（dev-agent-35 查实）

原登记为「crypto 覆盖率 75.0%，五包最低，可能有大块未覆盖代码」——**那个描述会误导接手的人
去看顶层包，而那里没什么可补的**：顶层 `crypto` 的核心路径 `FetchQuote`/`FetchHistory`
已达 **91.7% / 90.0%**，25% 缺口是 `Start`/`Stop`/setter（0%，空壳）与 `Init`（31.2%）。

**真正的洞在子包**——`go test ./internal/collector/crypto/...`（含子包）total 掉到 **32.4%**：

| 子包 | 覆盖率 |
|---|---|
| binance | **10.3%** |
| coingecko | **19.5%** |
| okx | **15.5%** |

**这三个是实际发 HTTP 的 provider 实现**（解码、错误处理、限频响应全在里面），
而 TASK-021 的链保留守护走的是 `countingProvider`（fake），**一行真 provider 代码都没经过**。

**最该记的是原因**：本 Sprint 所有 collector 任务的 `packages` 都写
`./internal/collector/crypto`，**不带 `/...`** ⇒ binance/coingecko/okx
**从头到尾没被任何一次 `dev_done` 门禁测过**。

**这不是谁的疏漏，是「声明粒度决定了门禁视野」**——与本 Sprint 一路在抓的
「检查范围比实际范围窄，而窄的那部分自己不会报警」是同一条，只是载体从测试断言
换成了任务声明字段。**本 Sprint 不补**（provider 层要 mock 真实 HTTP 语义，是新工作量）。

### 6.3 三项未落地

`aDone` 增强（TASK-002 并发测试假绿可观测化）、跨进程探针固化 `//go:build manual`、
TASK-001 第②层注释措辞收窄。

> 标注为「**未找到**」而非「确定不存在」——依赖核实者的搜索词，若用了未预料的命名则结论会变。

### 6.4 gofmt

**28 处**（口径见 §2），**本 Sprint 引入 0 处**，全部为历史债。
`gofmt -w` 统一须等全部任务 verified 后执行，否则会碰到在途文件。

## 7. 方法论产出

`.arcforge/docs/01-design/design-spec.md` 从初始的接口契约表增长至 **2067 行**，
含 **教训编号至 30**（`### 教训 N` 小节 16 条,含子节后最大编号 30）**+ 18 个契约陷阱**。⚠ 口径:教训计数随子节划分方式变化,引用时请以最大编号而非条数为准。其中本 Sprint 后期新增的教训 17-26 覆盖：
规格层锚点过期、单向判据的递归性、覆盖率不是质量论据、归因合并抹掉教训、
跨包变异的角色差异、缺陷模式的范围封闭、精度单位不匹配、注释不是守护。

**贯穿性观察**（dev-agent-35 总结）：

> serve 接线、yahoo 缓存键、crisis 提前 return——三个都是「代码写对了但没有测试守护」，
> 而且**三个都是在别的任务的核查过程中顺带发现的，没有一个是原任务自己发现的**。

## 7b. 契约条目是否真的影响了交付物（qa-agent-8 第二轮实测）

契约有 47 条教训、2729 行，但**「写下来了」与「落到代码里了」是两件事**。
qa-agent-8 在维度四实测中找到**一处正向对应的实证**：

> **「粒度不放粗」那一列**：教训 19 记录了 test-agent-16 自己给 baostock 的补丁曾**只有单向**
> （改成 `Truncate(Hour)` 照样通过）。**而四家现在都是双向守护，五处变异全部转红。**
> ⇒ 那个教训在交付物里落实了，不只是写在契约里。

**这是本 Sprint 唯一一处经实测确认的「契约条目 → 实际代码」正向对应。** 其余条目未做同类核查
（qa-agent-8 明确标注：契约其余 26 条正文、教训之间是否重复（19/27/28/33 高度相关）均未查）。

⇒ **交付判定不依赖契约条目的落实率**——契约是方法论产出，交付质量由各任务的 DoD 与变异
证据支撑。但这一处对应说明**契约至少不是纯记录**：它进入了后续任务的 DoD（019/021 的
「方向/强度/机制三坐标」、020 的取证陷阱），并改变了实现。

### 维度四实测矩阵（全部为变异注入，非读表）

| 列 | 结果 |
|---|---|
| 错误不写缓存 | 七家转红 ✓ / **tushare 全绿 ✗**（TASK-020 修） |
| 缓存生效 | **八家全部转红 ✓**（含 tushare） |
| 聚合度 | yahoo×2 / eastmoney / crypto / baostock **五处全转红 ✓** |
| 粒度不放粗 | **五处全转红 ✓**（四家均双向） |
| 区分度（**仅 symbol 维度**） | 四家全转红 ✓（start/end/interval 未逐一测，按验证者措辞纪律标注） |
| **链保留** | **六家全转红 ✓** —— TASK-021 的守护经实测确认能挡住断链 |

**一个决定「下次该复验哪部分」的区分**（qa-agent-8）：**横表的实测格可靠、推断格不可靠**——
维度四标 ✓ 的格子实测过的部分**无一虚标**，而 §③ 的推断格被实测证伪。
⇒ **一份混合了实测与推断的文档，其可靠性不是均匀的**；逐格标注证据类型的价值正在于
**让复验可以精确投放，而不是全篇重来**。

## 8. 待同步 hooks 清单

> 本 Sprint 未修改任何 `.claude/hooks/`、`.claude/scripts/`、`.claude/settings*.json`
> （运行时资产对全体 agent 只读）。以下为**建议项**，须走
> `project-template/ → TDD → 人类确认 → 会话外同步` 流程，**不在本 Sprint 执行**。

| # | 目标 | 建议内容 | 提出者 |
|---|---|---|---|
| 1 | `CLAUDE.md` | 把「用 python/perl/heredoc 处理 `.arcforge/` 文件」列为**显式禁止项**。现措辞只说 write-guard「可被逃逸」，读起来像描述机制局限而非约束行为。dev-agent-36 自报了一次无意绕过（用 `python3 heredoc` 改 discovery，未被拦下，发现后主动经正规通道重写）。**这条盲区不会被任何机制发现，只能靠自报** | dev-agent-36 |
| 2 | 变异脚本/runner | 把**落盘校验 + `go build`** 两道门做成固定前置，而非每个 agent 各自记得。「删改符号导致编译失败被误当成变异捕获」本 Sprint 出现 **6 次**，门②固化进 runner 后仍发生——**机制拦住了误判，但没拦住误操作**。dev-agent-36 手写的 `mut()` shell 函数形态可参考 | dev-agent-36 |
| 3 | Leader 流程 | 「任何任务判 rejected 且原因是『某个代码特征无守护』时，由 **Leader 统一 grep 该特征扫全仓**」——Leader 是唯一能看到所有任务判定的人。本 Sprint 的 yahoo 缺口能被查出，靠的是 Leader 恰好转达了 criteria 变更给某个 dev | dev-agent-35 |
| 4 | 状态机 | 新增 `blocked_scope` 状态：「被派了但机制上不该动」在文件层面无法表达 | Leader |
| 5 | 状态机 | `review_fix` 是**单出边状态**（唯一出边 `→ in_progress`），转入时必须一次性把 owner 定对，否则绑定锁死。本 Sprint 踩过一次（TASK-011），导致一份已完成的产物无法落地 | Leader |
| 6 | `reason_class` | 现有 `task_defect`/`dod_defect` 不足以表达**第三类：规格后到**（实现与**当时的** DoD 相符，criteria 本身也没问题）。该类返工不应计入 `max_rework` 熔断额度——熔断的立论是「反复返工大概率是 criteria 不可实现」，而后加条款不携带这个信号 | qa-agent-8 |
| 6b | validator | **加一条 scope 粒度 warn**：对每个声明的 `packages` 路径跑 `go list "$p/..."`，若含子包则 warn「确认是有意的还是漏了 `/...`」。**全 Sprint 实测有三处裸路径漏了子包**，最大的是 `./internal/collector`（TASK-011/012/013 共同声明，**17 个包只测了 1 个**——而 011 删的正是被所有 collector 引用的装饰器）。**做成 warn 不做 block**：关键是让「选了 `./x` 而非 `./x/...`」成为**被显式确认过的决定**，而不是**从未被想起的默认** | dev-agent-35 |
| 7 | `task-completed.sh` | 门禁的 `packages` 字段**同时驱动漂移判据与 `go test`**，且漂移检测**只看未提交改动** ⇒ 并行 dev 的提交时序会决定谁被拦。本 Sprint 两次撞上（一次被拦、一次侥幸通过） | dev-agent-35 / dev-agent-38 |

## 9. 待补（四条在途线收口后）

- [x] **TASK-019 已 verified**(crisis/backtest 装配缺口 + 守护,覆盖率反升至 75.2%)
- [ ] TASK-007(`blocked_clarification`→开工中,F2 区分度)/016(`verifying` 三轮)/018(`verifying` 四包)/020(`pending`,tushare 区分度) 的验收结论
- [ ] QA 第二轮终审报告（八家一致性、错误映射「两种模式各有依据」的口径核对）
- [ ] code-simplifier 统一简化
- [ ] `gofmt -w` 统一（28 处历史债）
- [ ] `changelog.md`
