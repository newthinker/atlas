# TASK-016 二轮验证报告（crypto 接入 Gate）

- 验证者：test-agent-17
- 交付：`c63883356ebc35e9281ca572051d417a2aec5458`（`gate_test.go` +42/-3，生产代码零改动）
- 承接时 `assignment_epoch`：**3**
- 裁决：**rejected** / `reason_class = task_defect`

## 结论一句话

本轮补的 `error_handling` 分句② **确实到位、经独立复现的 2×2 闭环证实**；但复核 DoD 其余
明写变异判据时发现 **`functional[4]` 的 (a) 分句（聚合度）在本包是空断言** ——
DoD 原文写明「变异判据：去掉 `Truncate(time.Minute)` 后该测试须转红」，实测**不转红**。

## 一、基线（交付 commit，单包口径）

| 指标 | 值 |
|---|---|
| `=== RUN` 总数 | 60 |
| PASS / FAIL / SKIP | 60 / 0 / 0 |
| 覆盖率 | 75.0%（`coverage_floor` 70） |
| `-race` | 绿 |
| 文件完整性 | `gate_test.go` 477 行、单个 `TestMain`、`go vet` 通过（Dev 自报的 python 切割事故已还原干净） |

## 二、完成标准覆盖矩阵（逐条变异取证）

每条都独立注入变异并核实**红在哪一行**；断言行均匹配 `^\s+\w+_test\.go:\d+:\s+\S`
（非 panic 栈、非 DATA RACE），且每次注入前后 `crypto.go` 的 md5 均回到基线
`295247761d1f9d8b471a68cbcd078954`。

| # | 完成标准 | 对应测试 | 变异 | 转红位置 | 判定 |
|---|---|---|---|---|---|
| functional[0] | TTL 缓存，同参数只发一次 | `TestFetchHistoryCachedByGate` | D8 绕过 `policy.Fetch` 直取 | `gate_test.go:142`（实得 3 次） | PASS |
| functional[1] | 键覆盖 symbol/区间/interval | `TestCacheKeyCoversAllParams` | D4 键里丢 symbol | `:198`（b 误命中 a） | PASS |
| functional[2] | 构造函数快照 `policy.Default()` | `TestConstructorsSnapshotDefaultGate` | D3 两个构造函数 `gate: nil` | `:272` / `:276` | PASS |
| functional[3] | 主题名域段与内置表一致 | `TestTopicMatchesBuiltinTable` | D9 `crypto.history`→`cryptoo.history` | `:294`（内置表查不到） | PASS |
| **functional[4] (a)** | **聚合度：相邻墙钟调用须同槽** | `TestCacheKeyAggregatesNearbyTimes/相邻时间落进同一槽` | **D1 去掉 `Truncate(time.Minute)`** | **未转红（仍 PASS）** | **FAIL** |
| functional[4] (b) | 粒度不得放粗 | `.../分钟粒度不得放粗` | D2 `Truncate(time.Hour)` | `:252`（取 1 次 want 2） | PASS |
| boundary[0] | 不被节流 | `TestFetchHistoryNotThrottled` | D7 人为注入 300ms sleep | `:359`（902ms vs 对照 905ms） | PASS |
| boundary[1] | 缓存命中返回独立切片 | `TestFetchHistoryReturnsIndependentSlice` | D5 去掉 `slices.Clone` | `:387`（Close=-999） | PASS |
| boundary[2] | 既有测试互不串味 | 包级 `TestMain` 装零策略闸门 | 结构核实 | — | PASS |
| error_handling ① | 错误不写缓存（判据=请求次数） | `TestErrorIsNotCached` | 断言即 `p.count()!=2`；C11 下保持绿，反证与②路径不相交 | — | PASS |
| **error_handling ②** | **校验须留在被缓存的 `fn` 内部** | `TestEmptyResultIsNotCached`（本轮新增） | **C11 长度校验移出 `fn`** | **`:423`（取 1 次 want 2）** | **PASS** |
| error_handling ③ | policy 错误不外泄 | `TestPolicyErrorDoesNotLeak` | D6 `%v`→`%w` 保链 | `:472` | PASS |
| non_functional[0] | 既有用例一字未改 / 0 SKIP / `-race` 绿 / 覆盖率不降 | — | diff 仅含新增函数与注释重排 | — | PASS |

## 三、本轮新增项：分句② 的独立复现（PASS）

Dev 声称新测试走的是与 `TestErrorIsNotCached` **互不相交**的路径。**结构上成立**，
我按源码核对而非采信自报：

- `TestErrorIsNotCached` 夹具 `err: errors.New("upstream down")` ⇒ provider 返回 `(nil, err)`
  ⇒ 走 `crypto.go:197 if err != nil` ⇒ 落 `:203 all providers failed`
- `TestEmptyResultIsNotCached` 夹具 `history`/`err` 均零值 ⇒ provider 返回 `(nil, nil)`
  ⇒ `:190 if err == nil && len(data) > 0` 因 `len==0` 为假，且 `err != nil` 亦为假
  ⇒ 落 `:205 no data available`

两者终点是**不同的两条 return**，Dev 的说法成立。

**C11 的 2×2 同环境对照**（我自己注入，未采信 Dev 的输出）：

| | 新测试 | 既有测试集（59 RUN） |
|---|---|---|
| 无变异 | PASS | PASS |
| C11 | **FAIL** `:423` 取 1 次 want 2 | **全绿（0 FAIL）** |

右下格全绿就是「缺口曾经真实存在」的直接证据，右上格转红是「已被补上」的直接证据。

决定性断言确实是取数次数而非 error：空结果被缓存时第二次照样返回 error
（外层判空仍会报错），断言 error 的写法两种实现都绿。Dev 这点判断正确。

`gate_test.go` 头部映射注释已按三分句拆开，且写明「分句①测不到 C11」——核对属实。

## 四、FAIL 项详述：`functional[4] (a)` 是空断言

### 事实

`crypto.go:180` 的键：

    start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix()

`.Unix()` 是**秒**精度。而 `gate_test.go:231` 的三个时刻是：

    base, base.Add(50 * time.Millisecond), base.Add(900 * time.Millisecond)

三者落在**同一秒**内 ⇒ 即使去掉 `Truncate(time.Minute)`，`.Unix()` 也把它们压成同一个值
⇒ 键不变 ⇒ 仍然只取 1 次 ⇒ 断言 `got != 1` 恒不成立。**断言退化成恒真。**

### 「测试空转」还是「变异等价」——分开证

这两个结论会导向完全不同的处置，故用 DoD 自己规定的跨秒偏移做探针，凑齐 2×2：

| | 既有测试（ms 偏移） | 探针（`+3s` / `+15s`，DoD 规定写法） |
|---|---|---|
| 无变异 | PASS | **PASS** |
| D1 去掉 `Truncate` | **PASS（空转）** | **FAIL** 取 3 次 want 1 |

右列上下不同 ⇒ **变异真实改变了行为，不是等价变异**；左列上下相同 ⇒ **既有断言测不到**。
D1 下**整包 60 个测试无一转红**，无任何其他测试兜底。

生产影响也确凿：`app.go` 传的 `end = time.Now()`，无 `Truncate` 时命中窗口从 60 秒
塌缩到「同一秒内」，即 DoD 描述的「命中率近乎归零」的静默失效。

### 与 DoD 的偏离点

DoD `functional[4]` 明写了测试写法：

> 取 `base := time.Now().Truncate(time.Minute).Add(20*time.Second)`（当前分钟的中点）与
> `base.Add(3*time.Second)`

交付用的是 `base.Add(30*time.Second)` 起点（这部分没问题，同样避开分钟边界）
配 **ms 级**偏移。偏离的是偏移量，而**恰恰是这个偏移量决定了断言是否有效**。

### 修复方向（一处改动）

`gate_test.go:231` 的偏移量改为跨秒即可，实现零改动：

    for i, end := range []time.Time{base, base.Add(3 * time.Second), base.Add(15 * time.Second)} {

改后需自行复跑 D1（去掉 `Truncate(time.Minute)`）确认**转红**，再还原。

## 五、跨任务观察（不计入本任务判定）

### 1. crypto 是这个坑的**源头**，不是受害者

TASK-015 复盘时把 eastmoney 的同类失效归因为「eastmoney 的 key 用 `.Unix()`（秒级），
与照搬来的 crypto ms 偏移不匹配」。**这个归因不完整**：crypto 的键**同样**用 `.Unix()`，
所以 crypto 自己的 (a) 断言从写下那天起就是空转的。eastmoney 不是「抄了对的写法但本包精度不同」，
是**抄了一份本就无效的断言**，只是抄过去后有人做了变异才暴露。

baostock 那条当时用 `+3s`「是运气不是判断」的自评，反过来同样适用：真正该问的是
「**这三个时刻经过键函数之后还是不是三个不同的值**」，而不是「偏移量看起来够不够小」。

### 2. eastmoney 里两条「行为保证式注释」无对应测试（实证，非推断）

按 TASK-015 派验时布置的额外视角扫了 eastmoney，用 2×2 实证了三条（每条都先建探针
证明变异有效，再确认既有套件全绿）：

| 注释 | 承诺 | 变异 | 既有套件 | 探针 |
|---|---|---|---|---|
| `eastmoney.go:432-433` | Gate 包在**整个** `FetchHistory` 外层，「缓存的正是这个方法的最终结果（**含 fund 分支与 lixinger 回退**）」 | fund 分支绕过 gate | 42 RUN **全绿** | 红：3 次请求 want 1 |
| `eastmoney.go:236` | 「ETF prices are returned in 厘」⇒ 除数 1000 | ETF 除数改回 100 | **全绿** | 红：Price=15 want 1.5 |
| `eastmoney.go:400` | 「Skip dates outside range」 | 删掉区间外剔除 | **全绿** | 红：bars=2 want 1 |

补充：lixinger 回退分支在测试中**零覆盖**——全包仅 `eastmoney_test.go:346` 有一处
`SetLixingerFallback(nil)`，nil 时该分支根本进不去，所以 `:432-433` 承诺的另一半
（「含 lixinger 回退」）同样无守护。

`:236` 与 `:400` 属**既有历史代码**、不在 TASK-015 范围内；`:432-433` 是 TASK-015
本轮做出的设计决定，其注释所承诺的两个分支都没有测试。三条均**不影响** TASK-015 的
verified 判定（都不在其 done_criteria 内），仅作观察项登记。

### 3. 残留 worktree

`../wt-v007r`（415f900，无未提交改动）仍在 worktree 列表里，非本轮所建，请确认归属后回收。
本轮我建的 `wt-obs015` / `wt-v016r2` 已拆除，主工作区 `internal/collector/crypto/` 与
`internal/collector/eastmoney/` 零污染。

## 六、原始产物指针

- 任务文件：`.arcforge/tasks/TASK-016.json`
- 上游 discovery：`.arcforge/discoveries/TASK-010.json`、`.arcforge/discoveries/TASK-001.json`
- 前轮报告：`.arcforge/docs/04-test/TASK-016-verification.md`
- git ref：`c63883356ebc35e9281ca572051d417a2aec5458`（验证用 worktree 已拆）
- 复现命令（锚全部钉全 sha）：

      git worktree add --detach ../wt-v016r2 c63883356ebc35e9281ca572051d417a2aec5458
      # D1：把 crypto.go:180 的两处 Truncate(time.Minute) 删掉
      GOTOOLCHAIN=local go test ./internal/collector/crypto/ -count=1 -v   # 预期:60 全绿(空转)
