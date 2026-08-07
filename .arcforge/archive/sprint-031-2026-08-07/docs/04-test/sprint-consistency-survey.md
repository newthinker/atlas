# 八家 Gate 接入一致性核查（Sprint 收口，供 QA 终审与 final-report）

- 核查者：test-agent-17（只读）
- 锚：**`f6a78afeba34b152922da818c58f3c1f04f167a1`**，全程在隔离 worktree 内进行
- ⚠ **必须钉锚的实证**：核查期间主工作区出现 dev-agent-35 的 021 在途改动
  （`M baostock/gate_test.go`、`M eastmoney/gate_test.go`、`M lixinger/errmap_test.go`）。
  若从主工作区读，这张表会是「基线+在途」的混合物。我全程未碰主工作区。
- 标注约定：**现读** = 直接读源码/跑出来的；**实测** = 我注入变异跑出来的；**推断** = 未直接验证。

## 0. 接入范围（现读）

`internal/collector/` 下共 12 个采集包，**引用 `policy` 的恰是那 7 家**。
akshare / edgar / fred / qlib / qlibpit **零引用**——本 Sprint 未纳入，是否该纳入不在本核查范围。

## 1. 维度一：Gate 接入层（现读）

**三种形态，不是一种。**

| 包 | 接入层 | `policy.Fetch` 所在函数 | 导出 `Fetch*` 方法覆盖 |
|---|---|---|---|
| yahoo | **逐方法各自包** | `FetchQuote` / `FetchHistory` / `FetchEPSHistory` | **3 / 3 全覆盖** |
| eastmoney | 单方法 | `FetchHistory` | 1 / 2 —— **`FetchQuote` 未接入** |
| crypto | 单方法 | `FetchHistory` | 1 / 2 —— **`FetchQuote` 未接入** |
| baostock | 单方法 | `FetchHistory` | 1 / 3 —— **`FetchQuote`（转调 `FetchDaily`）与 `FetchDaily` 未接入** |
| twelvedata | 单方法 | `FetchHistory` | 1 / 1 全覆盖（本包只有这一个） |
| tushare | **共享内部层** | `(c *Client) call(...)` | **6 / 6 自动覆盖** |
| lixinger | **共享内部层** | `(l *Lixinger) request(...)` | **9 / 9 自动覆盖** |

**共享内部层（tushare/lixinger）与逐方法（其余五家）是两种不同的架构选择。**
前者「加一个新 API 自动获得闸门」，后者「加一个新方法必须记得包一层」——
后者属「靠记得执行」那一列，是将来漏接的结构性风险。**未见任何一处注释说明为何分两派。**

## 2. 维度二：缓存键（现读）

| 包 | 键表达式 | 时间精度 | 聚合度守护 | 区分度守护 |
|---|---|---|---|---|
| yahoo chart | `symbol\|start\|end\|interval` | `Truncate(Minute).Unix()` | ✓ `TestFetchHistoryCacheKeyAggregatesNearbyTimes` | ✓ `...DistinguishesParams` |
| yahoo eps | `symbol\|start\|end` | `Truncate(Minute).Unix()` | ✓ `TestFetchEPSHistoryCacheKeyAggregates...` | ✓ |
| yahoo quote | **`symbol` 单值** | 无时间维度 | N/A | N/A —— `yahoo.quote` 显式登记 `TTL: 0`，本就不缓存，只节流+合并 |
| eastmoney | `symbol\|start\|end\|interval` | `Truncate(Minute).Unix()` | ✓ | ✓ `TestCacheKeyCoversAllParams` |
| crypto | `symbol\|start\|end\|interval` | `Truncate(Minute).Unix()` | ✓（**016 三轮才修好**，此前是亚秒偏移空断言） | ✓ |
| baostock | `symbol\|start\|end` | `Truncate(Minute).Unix()` | ✓ **双向**（`TestCacheKeyTruncatesToMinute` 含「同分钟聚合」+「跨分钟不串槽」两格） | ✓ + `TestIntervalDoesNotSplitSlot`（interval 不入键，**有理由**：只接受 `""`/`"1d"`） |
| twelvedata | `symbol\|date\|date` | `Format("2006-01-02")` | **N/A**：日粒度天然聚合，墙钟 end 落到同一天即同槽 | ✓ `TestFetchHistoryCacheKeyCoversAllParams` |
| tushare | `callKey(params, fields)`（**排序后拼接**） | 日期以 `Format("20060102")` 入 params | **N/A**：同上 | ✓ `TestCallKeyDistinguishesParams` + `TestCallKeyIsOrderIndependent` |
| lixinger | **整个 JSON payload 字符串** | 日期以 `Format("2006-01-02")` 入 payload | **N/A**：同上 | ✓ `TestLixingerCacheKeyIncludesPayload` + `TestLixingerEndpointsAreSeparateKeys` |

**「亚秒偏移空断言」排查已封闭**：五处 `Truncate(Minute).Unix()` 键中，只有 crypto 曾用
亚秒偏移做断言（已由 016 三轮修掉）；其余四处（baostock / eastmoney / yahoo×2）均为跨秒偏移。
tushare / twelvedata / lixinger 因键里是**日期串而非墙钟**，结构上不存在该风险面。

## 3. 维度三：错误映射口径（现读）

| 包 | 哨兵数 | 口径 | 是否保留原始原因 |
|---|---|---|---|
| tushare | **2**（`ErrNoPermission` / `ErrRateLimited`） | **`%w` 映射成本包哨兵** | ✓（含 apiName + `%w` 链） |
| yahoo | 0 | `%v` 断链 + 本包前缀 | ✓ `...: %v", err` |
| twelvedata | 0 | 经 `wrapErr`（本包唯一出口，天然 `%v`） | ✓ |
| lixinger | 0 | `%v` 断链 + 本包前缀 | ✓ |
| crypto | 0 | `%v` 断链 + 本包前缀 | ✓ |
| **eastmoney** | 0 | 断链 + 本包前缀 | **✗ 完全丢弃 `err`**，只写手写中文（「请求超时（临时性，可重试）」） |
| **baostock** | 0 | 同 eastmoney | **✗ 同上** |

两种口径本身**执行一致**（有哨兵→`%w`，无哨兵→断链），分派规则无例外。
但「**映射是换类型不是丢信息**」（TASK-018 `functional[1]`）这条，**eastmoney/baostock 没做到**。

## 4. 维度四：守护完整性

| 包 | 聚合度 | 区分度 | **链保留** | 错误不写缓存 | 校验留在 `fn` 内 |
|---|---|---|---|---|---|
| yahoo | ✓×2 | ✓×2 | **缺**（021 覆盖） | ✓ `TestErrorEnvelopeIsNotCached` | ✓ |
| eastmoney | ✓ | ✓ | **缺**（021 覆盖） | ✓ `TestErrorResponseIsNotCached` | ✓ |
| crypto | ✓ | ✓ | **缺**（021 覆盖） | ✓ `TestErrorIsNotCached` | ✓ `TestEmptyResultIsNotCached`（016 分句②） |
| baostock | ✓双向 | ✓ | **缺**（021 覆盖） | ✓ `TestErrorIsNotCached` | ✓ |
| lixinger | N/A | ✓ | **缺**（021 覆盖） | ✓ `TestLixingerErrorIsNotCached` | ✓ |
| twelvedata | N/A | ✓ | **N/A**（`wrapErr` 按设计对一切断链，无链可保） | ✓ `TestFetchHistoryErrorNotCached` | ✓ |
| **tushare** | N/A | ✓ | **✓（七家中唯一有）** | **✗ 缺（我实测确认）** | **✗ 缺（同一条）** |

**「链保留」七家只有 tushare 有，而且是白捡的**——它有哨兵，于是
`TestNonPolicyErrorsUnaffected` 天然写成 `errors.Is(wantIs/wantNot)` 判据，
顺带把链也钉住了。其余六家退到文本断言，看不见链。**这不是谁偷懒，是被测对象的类型结构
支配了断言的判据形式。**

## 5. 「不一致且未找到理由」——终审要盯的三条

### ① tushare 缺「错误不写缓存 / 校验留在 fn 内」守护 —— **我实测确认，且失效形态最坏**

实现是对的（40203 校验在 `client.go:182`，位于被缓存的 `callHTTP` 内）。但**没有测试钉住**。

变异 TS-C11b（业务错误在 `fn` 内被藏起当成功返回、错误在 `Fetch` 之外重新抛出）：

| | 既有全套 | 我的探针（40203 连调两次，断言 HTTP 次数==2） |
|---|---|---|
| 无变异 | 绿 | **绿** |
| TS-C11b | **绿（无一转红）** | **红**：`第 2 次:40203 必须报错,却成功返回` + `两次调用只发了 1 次请求, want 2` |

**失效形态**：第二次调用**静默返回成功且无数据**——不是「错误持续一个 TTL」，
是「错误变成了成功」。上层拿到空结果会当作「该标的今天没数据」。
其余六家都有这条守护，只有 tushare 没有，**未找到任何理由**。

### ② eastmoney / baostock 映射时**完全丢弃**原始错误 —— 未找到理由

其余五家都写 `: %v", err`。这两家只留手写中文。文本对人可读，但
**丢掉了机器可 grep 的原文**，也与 018 `functional[1]` 的要求相反。
**疑为时序原因**（两家的映射早于 018 把「保留原始原因」写成判据），但这属于推断，
源码与注释里没有任何说明。

### ③ eastmoney / crypto / baostock 的 `FetchQuote` 未接 Gate —— **有结构性理由，但没写下来**

三处 `FetchQuote` 的注释只说功能，**没有一处解释为何不接闸门**。

但我读表后认为**存在一个真实的结构性障碍**（标注：这一条是**推断**，非实测）：
这三家在内置表里是**通配登记** `<域>.*` 且 `TTL: builtinTTL`（= **5 分钟**）。
若给 `FetchQuote` 起一个 `eastmoney.quote` 之类的主题，它会**命中通配**、
连带给**实时报价**套上 5 分钟缓存——那是错的。
yahoo 之所以能接，是因为它**显式登记了** `yahoo.quote` 且 `TTL: 0`（只节流+合并、不缓存）。

⇒ **不是「忘了接」，是「通配登记下接不了」。** 但代价是这三家的报价路径
**既无节流也无请求合并**。建议终审要么按 yahoo 的形态显式登记 `<域>.quote` + `TTL: 0`，
要么把这个取舍写进注释——**现在它既没做也没记，下一个人只会看到「不一致」**。

## 6. 已确认的合理差异（有理由，不必收敛）

| 差异 | 理由 | 出处 |
|---|---|---|
| tushare 走 `%w` 映射成 `ErrRateLimited` | `refresh.go:450/:453` 的降级链依赖 `errors.Is` 分叉，断链会让两个分支都匹配不上 | `client.go:106-108` 注释 + 我在 018 实测（T9 转红） |
| twelvedata 的 `wrapErr` 对一切断链 | 凭证脱敏：留链则 `errors.Unwrap` 可取回未脱敏原文 | `client.go:65-71` 注释 |
| baostock `interval` 不入键 | 只接受 `""`/`"1d"`，取同一份日线 | `TestIntervalDoesNotSplitSlot` |
| crypto 无解码路径 | provider 是裸 HTTP，闸门包在 fallback 链外层 | `crypto.go` FetchHistory 注释 |
| yahoo `quote` 键只含 symbol | `TTL: 0` 不缓存，键只用于节流/合并 | 内置表 `policy.go:73` |
| tushare/lixinger 键为日期串/payload | 日粒度天然聚合，无墙钟精度问题 | 现读 |

## 7. 核查方法与自检

- 全部作业在隔离 worktree（锚 `f6a78af`）；唯一一次变异（TS-C11b）注入前后
  `git status --porcelain` 均为 0，七包收尾复跑全绿；**主工作区零改动**。
- TS-C11b **首版变异是错的**：我直接把 `if env.Code == 40203` 停用，导致 5 条测试转红——
  但那些红是「错误不再产生」而非「错误被缓存」，**归因方向不对**。重造了保留错误语义、
  只改缓存行为的版本才拿到有效证据。**变异改错了变量，红也是假的。**
- 探针先在无变异基线上确认为真（格A）再注入（格B），沿用 021 `boundary[0]` 那条纪律。

## 8. 原始产物指针

- git ref：`f6a78afeba34b152922da818c58f3c1f04f167a1`
- 相关报告：`TASK-018-verification.md`、`TASK-018-verification-addendum.md`、
  `TASK-021-premeasure.md`、`TASK-016-verification-round2.md`
- 复现（锚钉全 sha）：

      git worktree add --detach ../wt-survey f6a78afeba34b152922da818c58f3c1f04f167a1
      # TS-C11b：把 tushare call() 的 fn 改成「业务错误藏起来返回 (nil,nil)、
      # 错误在 Fetch 之外重新抛出」，既有全套应全绿（缺口证据）。
