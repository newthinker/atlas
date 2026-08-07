# TASK-008 验证报告 —— twelvedata 接入 policy Gate

- 验证者: test-agent-16 / assignment_epoch: 1
- 被验对象: `ec149a0`（client.go +61/-? · client_test.go +25/-? · gate_test.go +259，共 302 增 / 43 删）
- 验证环境: 独立 worktree `../wt-v008 @ ec149a0`；基线对照 `../wt-v008-base @ b197394`（均已拆除）
- 判定依据: 落盘后的 DoD（含 Leader 裁定的豁免范围）+ 覆盖率**绝对下限 92.0%**

## 结论：**PASS（verified）**

7 条 done_criteria 全部通过。**11 个变异全部按预期捕获，无一存活、无一无效**（含 2 个我自加的、
Dev 未做的变异）。B4 的 5 个受保护测试经逐函数体比对**逐字一致**。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | FetchHistory 经 Gate 受 8s 节流，相邻请求间隔符合策略 | `TestFetchHistoryThrottledByGate` + `TestTopicMatchesBuiltinPolicy` | V2 绕过 Gate → 红 :98；V11 使 `throttle` 失效 → 红 :98；V7 主题拼错 → 红 :171；V8 主题改 `yahoo.chart` → 红 **:174**（数值断言本身活着）；V10 改内置表 8s→5s → 红 :174 | **PASS** |
| functional[1] | 经 Gate 走 TTL 缓存，同参数只发一次 HTTP | `TestFetchHistoryCachedByGate` + `TestFetchHistoryCacheKeyCoversAllParams` | V2 → 红 :122/:156；V4/V5/V6 键分别去掉 symbol/start/end → 红 :156；V9 键掺时间戳 → 红 :122+:156 | **PASS** |
| functional[2] | Client 持 `gate *policy.Gate`，构造函数取 `policy.Default()`，包内可注入 | `TestNewSnapshotsDefaultGate` | V3 `gate: nil` → 红 :254/:257 | **PASS** |
| boundary[0] | 缓存命中返回独立切片，调用方修改不污染缓存 | `TestFetchHistoryReturnsIndependentSlice` | V1 不做 `slices.Clone` → 红 **:213 与 :217 同时**（值断言 + 底层数组地址断言两侧都守住） | **PASS** |
| error_handling[0] 分句 1 | 既有错误路径测试断言体与场景**一字不改**且全通过 | client_test.go 既有 5 个测试 | §二 逐函数体比对**逐字一致**；5 个全 PASS | **PASS** |
| error_handling[0] 分句 2 | 错误不写缓存 | `TestFetchHistoryErrorNotCached` | 判据取「两次都真的发了 HTTP」而非「第二次仍返回 error」——后者在「把 error 也缓存」的实现下同样成立（教训 12 的分水岭判据，选对了） | **PASS** |
| non_functional[0]（review） | 源码中不再有 `lastReq`/`minInterval`/`throttle` | 人工 review | `client.go` **零命中**；全包仅 gate_test.go 注释里出现该词 | **PASS** |
| non_functional[1] | `-race` 全绿 + 覆盖率 ≥ 92% | 实测 | `-race ok`（1.585s）；**92.7% ≥ 92.0%** | **PASS** |

## 二、B4 豁免范围逐条核对（Leader 点名的检查）

按「函数数量 / 断言文本 / 只有签名与字段访问方式变了」三个维度核对。

### 2.1 函数增删

| | master `b197394` | 交付 `ec149a0` |
|---|---|---|
| client_test.go 测试函数数 | **6** | **5** |
| 消失的 | — | `TestThrottleMinInterval`（豁免②，被 `TestFetchHistoryThrottledByGate` 取代）|
| 新增的 | — | 无 |

**5 个受保护测试函数一个都没少。**

### 2.2 受保护测试的函数体逐字比对

对每个函数做 `awk` 抽取后 `diff`（滤掉豁免①允许删除的 `c.minInterval = 0` 行）：

| 测试 | 函数体行数 | 比对结果 |
|---|---|---|
| `TestFetchHistoryParsesAndSorts` | 31 | ✅ **逐字一致** |
| `TestFetchHistoryEmptyValues` | 13 | ✅ **逐字一致** |
| `TestFetchHistorySkipsUnparsableClose` | 16 | ✅ **逐字一致** |
| `TestFetchHistoryAPIError` | 7 | ✅ **逐字一致** |
| `TestAPIKeyNeverInURLOrErrors` | 40 | ✅ **逐字一致** |

### 2.3 实际改动是否全部落在豁免内

client_test.go 的 diff 只有四处，逐一归类：

| 改动 | 归类 |
|---|---|
| 删 `tdServer()` 里的 `c.minInterval = 0`（第 46 行，**即我上报的那个共享 helper**）| 豁免① ✅ |
| 删 `TestAPIKeyNeverInURLOrErrors` 内两处 `c.minInterval = 0` | 豁免① ✅ |
| 删整个 `TestThrottleMinInterval`，原处留迁出说明注释 | 豁免② ✅ |
| 头部 DoD↔测试映射注释改为指向迁出后的位置 | 注释同步，非断言 ✅ |

**无一处越出豁免范围，无一处触碰断言或场景。**

## 三、变异验证结果表（11 个，11 捕获 / 0 存活 / 0 无效）

**每个变异均在注入前写下预期**（契约：事后补写会被实际结果污染），**实际与预期无一处不符**。
runner 强制四道门：① `md5` 改动量非空 ② `go vet` 通过 ③ `=== RUN` 数 > 0
④ 判红只认 `<file>_test.go:NN:` 断言行，`exit != 0` 但无断言行则报「红但非断言 ⚠不计为捕获」。

| ID | 变异内容 | 预期 | 实际 | 断言行 |
|---|---|---|---|---|
| V1 | `FetchHistory` 不做 `slices.Clone`（用 `if false {}` 保留引用，避免动 import）| 红 | **红** ✅ | `:213` `:217` |
| V2 | 绕过 Gate，直调 `fetchHistory` | 红 | **红** ✅ | `:98` `:122` `:156` |
| V3 | 构造函数 `gate: nil`（不取 `Default()`）| 红 | **红** ✅ | `:254` `:257` |
| V4 | 缓存键去掉 `symbol` | 红 | **红** ✅ | `:156` |
| V5 | 缓存键去掉 `start` | 红 | **红** ✅ | `:156` |
| V6 | 缓存键去掉 `end` | 红 | **红** ✅ | `:156` |
| V7 | 主题常量拼错为 `twelvedata.time_seriesX` | 红 | **红** ✅ | `:171` |
| V8 | 主题常量改为已登记但间隔不同的 `yahoo.chart` | 红 | **红** ✅ | `:174` |
| V9 | 缓存键掺 `time.Now().UnixNano()` | 红 | **红** ✅ | `:122` `:156` |
| **V10** | **（我自加）policy 内置表把 twelvedata 的 8s 改成 5s** | 红 | **红** ✅ | `:174` |
| **V11** | **（我自加）policy 的 `throttle` 整体失效** | 红 | **红** ✅ | `:98` |

### 3.1 我自加的两个变异及其价值

Dev 做了 9 个变异，我全部独立复现（结论一致），另加 2 个它没做的——依据是契约那条
「变异只能证伪我写下的断言，不能证伪我没写的那些」，独立视角要问的是**它没想到测什么**：

- **V10**：Dev 的 `TestTopicMatchesBuiltinPolicy` 断言 `policy.NewTable().Lookup(topic).MinInterval == 8s`。
  但这条断言的**锚在 policy 包的内置表里**，而那是别的任务的产出。我直接去改内置表的 8s，
  确认本包这条断言**真的会转红**——即「8s 数值只平移不调整」这条硬纪律在 twelvedata 侧
  确有守护，不是自说自话。
- **V11**：Dev 用 V2（绕过 Gate）间接证明节流生效，但那同时废掉了缓存，红的原因不唯一。
  我直接让 `policy.throttle` 失效、其余路径不动，**只有 `:98` 转红**——把「节流」这一条
  单独隔离出来归因（对应契约那条「变异捕获 ≠ 你以为的那条断言捕获」）。

### 3.2 闭环左下格（证明「以前抓不到」）

两处在**同一 worktree、同一会话**内取得：

| 变异 | 既有用例（DoD 原点名的） | Dev 新补的用例 |
|---|---|---|
| V4 缓存键去掉 symbol | `TestFetchHistoryCachedByGate` **仍绿** ❌漏检 | `TestFetchHistoryCacheKeyCoversAllParams` **红** ✅ |
| V7 主题常量拼错 | 其余 **9 个测试全绿** ❌漏检 | `TestTopicMatchesBuiltinPolicy` **红** ✅ |

这两格证明 Dev 那两条 DoD 未点名的补充测试**不是镀金**：缺了它们，两个**静默错行为**
（AAPL 拿到 NVDA 的收盘价；生产完全不节流直接撞 TD 免费层 8 req/min）都会漏网。

## 四、覆盖率实测（判定线 = 绝对下限 92.0%）

| 对象 | 覆盖率 |
|---|---|
| master `b197394` 基线（**在同源 worktree 内重测**）| **92.7%** |
| 交付 `ec149a0` | **92.7%** |
| **判定线（Leader 裁定：绝对下限）** | **92.0%** |

**两种口径都过**（≥92.0% ✅；与基线持平 ✅），本次不卡缝。

> ⚠️ 一处方法坑，记下来备用：我最初在**主仓库**（HEAD 已前移到 `b0bc78a`）对
> `b197394` 生成的 profile 跑 `go tool cover -func`，得到 **91.7%** 这个错数——
> `cover -func` 同时读 profile **和当前源码**，源码变了行号即错配。改在同源 worktree
> 内重测才拿到正确的 92.7%。**这与「验证命令里的锚必须钉全 sha」是同一类失效**：
> 依据在被使用时已不是写下时的那一份。

逐函数：`New` / `NewWithBaseURL` / `wrapErr` / `FetchHistory` 均 **100%**；`fetchHistory` 89.7%。
未覆盖的仅 **3 个语句块**（`client.go:128` build-request 失败、`:140` read-body 失败、
`:145` JSON decode 失败），均为**既有未测的 I/O 错误分支**，master 上同样未覆盖
（master `FetchHistory` 90.0%，函数拆分后数字基本平移），**与本任务 DoD 无关**。

## 五、约束与稳定性核查

| 项 | 结果 |
|---|---|
| scope | `git show --name-only ec149a0` **全部落在 `./internal/collector/twelvedata/`**，与 `writes` 声明一致，无越界 ✅ |
| non_functional[0]（review） | `client.go` 中 `lastReq`/`minInterval`/`throttle`/`sync.` **零命中**；`defaultMinInterval` 常量已删；全包仅 gate_test.go **注释**里出现该词（Dev 刻意把测试常量命名为 `gateInterval` 以免 TASK-013 的 AST 断言误伤——判断正确，AST 不扫注释）✅ |
| 测试规模 | 12 个顶层测试 / **23 个 `=== RUN`** / **0 SKIP**（陷阱 11：与 master 基线的 `0 SKIP` 一致，无守卫被静默吞掉）✅ |
| `-race` | ok（1.585s）✅ |
| build / vet / gofmt | 全 exit 0 / 无输出 ✅ |
| 调用方 | 唯一包外调用点 `cmd/atlas/prism.go:165 twelvedata.New(apiKey)`，签名未变，`go build ./cmd/atlas/` 通过 ✅ |
| 还原 | 每个变异 `cp` 还原 + `md5` 校验；收尾 `git status --porcelain` **为空**、`git diff --stat` 空 ✅ |

### 5.1 陷阱 7（并发/时序测试退化）—— 已量化，风险可忽略

`TestFetchHistoryThrottledByGate` 观测的是**服务端到达时刻**（`at[1].Sub(at[0])`）而非调用方
总耗时——这个观测量选得对（总耗时掺建连/解析开销，是间接现象）。

我量化了它的假绿余量：**注入 V11 使节流失效后，实际到达间隔是 272µs / 313µs / 490µs**，
而判据阈值是 **70ms**。

> **余量约 150–250 倍。** 要产生假绿，本机 httptest 往返需慢到 70ms 以上。
> 该测试的失效方向是**假绿而非假红**，且余量如此之大，实践中可忽略。

稳定性实测：单跑 **10/10 全绿**；全量套件 `-shuffle=on` **8 次全绿**（0.31–0.41s）。

> 诚实记录一处**未解释**的观察：`-shuffle=on` 的某一轮曾耗时 2.878s（常态 0.32s）。
> 我用 8 次 shuffle **未能复现**。当时机器上有 4 个 agent 并发跑测试，最可能是机器争用；
> 但我无法确证，故记为「非复现性观察」，不作为缺陷。

### 5.2 Leader 关注点 3（否定断言需正向对照）—— 本任务不适用，已核

TASK-009 那条教训是：断言「计数为 **0**」时必须有正向对照，否则那个 0 可能只是整条链没接上。

我核过了：**twelvedata 的 gate_test.go 里没有任何 `== 0` 型否定断言**。所有计数断言都是
**正值**（1 / 2 / 4），且 `TestFetchHistoryCacheKeyCoversAllParams` 用的是**累计递增序列
1→2→3→4→4**——最后那个「4 而非 5」（缓存命中）本身就被前四步的正向递增锚住了。
这个形态天然自带正向对照，无该风险。

## 六、备录（均不作为 FAIL 依据）

1. **`questions[0].answer` 仍为 `null`**。DoD 正文已按裁定改好（**实质问题已解决**，我据此正常判定，
   未判 `dod_defect`），但那条 question 的记录未闭环。与 TASK-012 的 `discovery` 指针同类记录瑕疵。
   `verifying` 状态下 `questions` 字段的写权在我，如需我代填 `answer` 请示下。
2. **Dev 的 9 个变异我全部独立复现，结论与其自述完全一致**，未发现夸大或误报。其自述的
   「M1 首版因 `slices` 未使用致 vet 不过、被门②判为变异无效后重做为 M1b」是**诚实记录**——
   这正是契约第 500 行那个「三个实例先后栽过」的坑，它自己撞上并正确识别了。
3. Dev 的两处设计判断我认为正确且值得记：① `TestFetchHistoryErrorNotCached` 用 `t.Errorf`
   而非 `t.Fatalf`（陷阱 10：否则「只发了 1 次 HTTP」这条证据会被先到的 Fatal 吞掉）；
   ② `boundary[0]` 把方案原版的排除断言 `Close == -1` 改成等值断言 `Close != 101.5`
   并加了底层数组地址断言（陷阱 6①）。V1 下两条同时红，两侧都活着。

## 七、复现命令

```bash
git worktree add --detach ../wt-v008 ec149a0
cd ../wt-v008
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -count=1 -coverprofile=/tmp/c.out && \
  GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | tail -1        # 92.7%
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -count=1 -v | grep -c '^--- SKIP'   # 0

# B4 逐函数体比对
for fn in TestFetchHistoryParsesAndSorts TestFetchHistoryEmptyValues \
          TestFetchHistorySkipsUnparsableClose TestFetchHistoryAPIError TestAPIKeyNeverInURLOrErrors; do
  diff <(git show b197394:internal/collector/twelvedata/client_test.go | awk "/^func $fn\(/,/^}/" | grep -v 'c\.minInterval = 0') \
       <(git show ec149a0:internal/collector/twelvedata/client_test.go  | awk "/^func $fn\(/,/^}/" | grep -v 'c\.minInterval = 0') \
    && echo "$fn 逐字一致"
done

# 基线覆盖率必须在同源 worktree 内测（否则 cover -func 行号错配 → 91.7% 的错数）
git worktree add --detach ../wt-v008-base b197394 && cd ../wt-v008-base
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -count=1 -cover   # 92.7%

# 收尾（必须在主仓库执行）
cd <主仓库> && git worktree remove ../wt-v008 && git worktree remove ../wt-v008-base
```

---

# 附录：Leader 追加的跨任务核查点（交付后补做）

判定**不变**（`verified`）。以下三项在首轮报告之后应 Leader 要求补做，结论均**支持原判定**。

## A1. 陷阱 14（200-but-error 的缓存陷阱）—— **已守护**

twelvedata **确实存在**该形态：`client.go:150` 的 `if env.Status == "error"`，而
`TestFetchHistoryErrorNotCached` 送的 `{"code":429,...,"status":"error"}` 正是经
httptest 以 **HTTP 200** 返回的。风险成立，需要验证守护。

**变异 W1**：把 fn 内的 `env.Status == "error"` 校验短路掉（`if false && ...`，保留引用不动 import）。

| 门 | 结果 |
|---|---|
| ① md5 改动量 | 已变 ✅ |
| ② `go vet` | 通过 ✅ |
| ③ `=== RUN` | 23 ✅ |
| ④ 红的性质 | **断言行**，非 panic/构建 ✅ |

```
gate_test.go:238: call 0: want error, got nil
gate_test.go:238: call 1: want error, got nil
gate_test.go:242: 失败不得写缓存,两次调用应各发一次 HTTP, got 1     ← 关键
--- FAIL: TestFetchHistoryErrorNotCached
--- FAIL: TestFetchHistoryAPIError
```

**`:242` 转红是决定性证据**：校验一旦离开被缓存的 fn，错误响应即被当成功 body 缓存
（HTTP 请求数从 2 塌成 **1**），而这条断言抓住了它。**「不被缓存」这一面有独立守护，
不只是「返回 error」那一面。**

纪律 4（单独 `-run` 确认 DoD 守护者自己转红）：单跑 `TestFetchHistoryErrorNotCached`
→ 自身 FAIL，`:238` + `:242` 同时输出。**不是被别的测试先炸带出来的。**

> 结构性补充：twelvedata 比 lixinger **更难**犯这个错。lixinger 的 `request()` 返回
> `[]byte`，`parseEnvelope` 天然可以放到 `Fetch` 外面；twelvedata 的 fn 返回
> `[]core.OHLCV`，错误信号（`env.Status`）只存在于解析层内部，**结构上就挪不出去**。
> 所以这里的现实风险形态是「忘了写这个校验」而非「把它挪到外面」——W1 测的正是前者。

## A2. 陷阱 13（接入缓存导致既有用例互相串味）—— **同类耦合存在，挡法有效**

**变异 W2**：把 `TestMain` 的零策略闸门换成 `{TTL: 5m, Coalesce: true}`
（刻意不掺 8s 节流，隔离「缓存串味」这一问）。

结果：**2 个既有测试转红** —— `TestFetchHistorySkipsUnparsableClose`、
`TestAPIKeyNeverInURLOrErrors`。

耦合面实测：`client_test.go` 的 6 次调用中 **5 次是 `FetchHistory("NVDA")`**，
日期取自 `time.Now()` 并格式化到 `2006-01-02` ⇒ 同日同 key ⇒ **同一个缓存槽**。

| 包 | TTL 闸门下转红的既有测试 |
|---|---|
| lixinger | **18 条** |
| twelvedata | **2 条** |

⇒ **同一类耦合，twelvedata 程度轻得多**（它的既有用例本来就少、且 `tdServer` 每个用例
起独立 server）。两包用的是同一种挡法（包级 `TestMain` 装零策略闸门），**实测均有效**。

## A3. 陷阱 12（构造函数取 `Default()` 是全绿缺陷）—— 首轮已实证，此处复述证据

首轮 **V3**（`gate: policy.Default()` → `gate: nil`）的失败测试列表是：

```
失败测试=[TestNewSnapshotsDefaultGate,]
```

**只有这一条**，其余 11 个测试全绿。与 Leader 描述的形态一致：其余用例各自注入了测试闸门，
而 `nil *Gate` 是透明直通的，**缺陷在它们眼里不可见**。
⇒ `TestNewSnapshotsDefaultGate` **确是唯一守护者**，独立复现确认。

## A4. 陷阱 15（主题常量 ↔ 内置表）—— 首轮已实证

首轮 **V7**（常量拼错为 `twelvedata.time_seriesX`）：

```
失败测试=[TestTopicMatchesBuiltinPolicy,]      ← 只有它，其余 11 个全绿
```

**V8**（改成已登记但间隔不同的 `yahoo.chart`）→ 红在 **`:174`**（数值断言），
证明「8s」那条断言本身也活着、没被 `!ok` 分支掩盖。
**V10**（改 policy 内置表 8s→5s）→ 红在 `:174`，证明该断言的锚确实钉在内置表上。

Leader 指出的风险成立且已被守护：**该域没有 `twelvedata.*` 通配兜底**（不同于 lixinger），
常量拼错 ⇒ `Lookup` 落空 ⇒ Gate 直通不节流 ⇒ 直撞免费层 8 req/min。
