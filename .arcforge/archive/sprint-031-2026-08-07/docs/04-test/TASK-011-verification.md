# TASK-011 验证报告 —— 删除 CachedCollector，缓存全面移交 policy Gate

- 验证者: test-agent-16 / assignment_epoch: 1
- 被验对象: `2a77803`（8 文件，**+90 / −532**）
- 验证环境: 独立 worktree `../wt-v011 @ 2a77803`（已在主仓库拆除）
- 判定线: 三包合并 `-coverpkg` total ≥ **74%**（新口径；cmd/atlas 单包为**观测项**）

## 结论：**PASS（verified）**

全部 done_criteria 通过。**A8 的正向断言经变异实证有效**（D1：装饰器回潮 → 唯一守护者转红）。
全量 `go test ./... -race` **62 包全 ok / 0 FAIL**。

删除类任务的核心问题「删掉的东西所承载的知识有没有去处」——**有，且转移后比原文更完整**。

一处 Dev 自述需**精度订正**（非缺陷，见 §四），一处清单有**假阳性**（非缺陷，见 §五）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 三符号在仓库中完全消失 | `grep -rE '\bNewCached\(\|\bmaybeCache\(\|\bCachedCollector\b'` **零命中** | **PASS** |
| functional[1] | `collectors.go` 不再包装，直接返回原实例 | 5 处 `maybeCache(...)` 全改为直接 `RegisterCollector(x)` | **PASS** |
| functional[2] | `cache.go`/`cache_test.go` 删除，`TestMaybeCache_*` 及其桩一并删除，不留孤儿 | 两文件整删（−137/−262）；`serve_test.go` −114；`go build`/`go vet` **exit 0，无未使用 import/变量** | **PASS** |
| **functional[3]** | **反审 A8：必须补正向断言，证明扩展接口不再被遮蔽** | `TestBuildCollectorsRegistersUnwrappedCollectors` + 两条编译期 `var` 断言；**D1 变异实证**（见 §三） | **PASS** |
| boundary[0] | `cache.enabled=false` 语义由 `Table.DisableTTL` 承接 | `policy.go` 仅改注释文字，`DisableTTL` 分支未动；TASK-006 的 `TestInitPolicyGateDisabledCacheStillThrottles` 在全量中通过 | **PASS** |
| boundary[1] | 删 `cloneOHLCV` 时知识必须转移；约束贴在会被违反的一侧；契约 §T2 限定仍在 | `types.go` +15 行（§五）；§T2 `grep` 命中 **1** 处，表述完整 | **PASS** |
| error_handling[0] | `go build ./...` && `go vet ./...` 通过，无删除产生的孤儿 | 均 **exit 0** | **PASS** |
| non_functional[0] | **全量 `go test ./... -race` 全绿**（反审 A11）| **实跑：62 包 ok / 0 FAIL** | **PASS** |
| non_functional[1] | `git diff master -- go.mod go.sum` 无输出（反审 A4）| **无输出** | **PASS** |
| non_functional[2] | 三包合并覆盖率 ≥ 74%（新口径）| **75.8%** | **PASS** |

## 二、覆盖率（新口径首次适用）

| 口径 | 值 | 角色 |
|---|---|---|
| **三包合并 `-coverpkg`** | **75.8%** | **判定线 74 → 过线，余量 1.8pp** |
| `cmd/atlas` 单包 | **74.3%** | **观测项**（DoD 要求 Dev 在 discovery 报告，**已核实其确实报了**）|

Dev 在 discovery 中的报告：「cmd/atlas 单包（观测项）：**74.4% → 74.3%**，仅降 0.1pp
——低于 Leader 粗估的 73.1%，原因是删掉的 `maybeCache`（原 100% 覆盖）与其 4 个测试对
分子分母的影响被新增的 `TestBuildCollectorsRegistersUnwrappedCollectors`（走完整
`buildCollectors` 装配路径）补回了大部分。」

**我实测 74.3%，与其自述一致。** 并且要更正我自己：**我派发前给出的 73.1% 粗估过于悲观**
——我按「删掉约 5% 已覆盖语句」线性外推，没有计入新增测试会走完整装配路径把覆盖补回来。
**方向判断（会往下走、余量薄）是对的，数值是错的。**

> 这次口径切换是必要的：即便实际只降到 74.3%，按旧的「cmd/atlas 单包 ≥74%」判也只剩
> **0.3pp** 余量，一次无关的小改动就可能让正确实现被判不过。

## 三、反审 A8 —— 本任务核心，变异实证有效

**D1（预期 RED）**：在装配路径重新引入装饰器包装，模拟回潮：

```go
application.RegisterCollector(mutWrap{yahooCollector})
type mutWrap struct{ collector.Collector }   // 仅嵌入 Collector —— 正是从前遮蔽扩展接口的形态
```

四道门：① md5 已变 ✅ ② `go vet` 通过 ✅ ③ `=== RUN` = 230 ✅ ④ 红来自**断言行** ✅

```
collectors_test.go:182: yahoo 注册的是 main.mutWrap 而非原始具体类型——装饰器会遮蔽扩展接口
--- FAIL: TestBuildCollectorsRegistersUnwrappedCollectors      ← 全量中唯一的 FAIL
```

**纪律 4**：单独 `-run` 该守护者 → **自身 FAIL**，非被别的测试先炸带出。

⇒ **「只删证据不算修复」这条要求被真正满足了。** 新测试不是摆设：它刻意把
`cache.Enabled` 设为 `true`（那正是从前触发包装的开关），并有 `len(got)==0 → Fatal`
的量词下界守卫（陷阱 6②：避免遍历空集合的空真）。

### 3.1 Dev 主动废弃了一个「形式与内容不符」的写法，判断正确

它在注释里记录：最初写的是 `var c collector.Collector = lixinger.New(...)` 再 type assert
的运行期测试，注入变异时发现**在「代码能编译」的前提下恒真**——lixinger 直接实现该接口，
经 `collector.Collector` 传递后动态类型不变，断言必然成功。

**我认同它的处置与措辞**：「形式与内容不符的测试比没有更糟——它看起来在守护运行期行为，
实际什么都不防，还会让人以为这块已经有覆盖。」这与本 Sprint 的空转断言家族同源。

### 3.2 一处覆盖面观察（不构成缺陷）

`TestBuildCollectorsRegistersUnwrappedCollectors` 的 `default` 分支**只对
`yahoo`/`eastmoney` 报错**，`crypto`/`tushare`/`baostock` 若被包装抓不到。
测试名（`RegistersUnwrappedCollectors`）读起来像覆盖全部。

**但对 A8 的诉求而言足够**：我核过这三家**均无扩展接口方法**
（`FetchFundamental`/`FetchValuationPercentile` 命中文件数皆为 0），
包装它们不会触发「扩展接口被遮蔽」这一风险。**建议**（非返工项）：把测试名或注释
限定为「持有扩展接口的 collector」，避免后人误以为覆盖全表。

## 四、Dev 一处自述需要精度订正（非缺陷，结论正确、理由略宽）

Dev 在 discovery 中称：

> `_ collector.FundamentalCollector = (*lixinger.Lixinger)(nil)` **非冗余**——
> `SetLixingerFallback(l *lixinger.Lixinger)` 用的是具体类型，**生产代码里没有任何
> lixinger→FundamentalCollector 的编译期约束**。

**我用 2 格实验实测**（破坏 lixinger 的接口符合性，同时修好其包内调用点以免混淆）：

| 格 | 操作 | 结果 |
|---|---|---|
| D2a | 破坏符合性，**保留** `var` 断言 | `collectors_test.go:203` 编译失败（`does not implement`）✅ |
| D2b | 破坏符合性，**并删掉** `var` 断言 | **仍编译失败** ——但错误移到了 `collectors.go:140`：`fundamentalSourceOrNil` 返回 `c` 作 `app.FundamentalSource` |

⇒ **生产代码里确实存在一处编译期约束**：`fundamentalSourceOrNil(c *lixinger.Lixinger) app.FundamentalSource { return c }`。

方法集对照：

| 接口 | 方法 |
|---|---|
| `app.FundamentalSource`（snapshot.go:17）| `FetchFundamental` —— **1 个** |
| `collector.FundamentalCollector` | `Name` / `SupportedMarkets` / `Init` / `Start` / `Stop` / `FetchFundamental` / `FetchFundamentalHistory` —— **7 个** |

**订正后的准确表述**：该 `var` 断言**不是冗余的，但它的独有贡献是另外 6 个方法**
（生命周期 + `FetchFundamentalHistory`），`FetchFundamental` 这一个已被
`fundamentalSourceOrNil` 在生产代码里编译期钉住。

**Dev 的结论（保留该断言）正确，只是理由说宽了。** 另一条
`_ app.ValuationSource = (*lixinger.Lixinger)(nil)` 它自评为「与 `valuationSourceOrNil` 冗余、
保留以防 helper 改动」——**我核实其判断准确**（`ValuationSource` 只有
`FetchValuationPercentile` 一个方法，与 helper 完全重叠）。

> 顺带：D2 我第一次跑砸了——忘了 `-buildvcs=false`（worktree 里 `go build` 报 VCS 错、
> 退出码无意义，**门②未过**），且改名连带破坏了 lixinger 包内调用点，编译错来自它自己
> 而非 `var` 断言。**两个混淆因子叠加，首版结论不可信**，重做时同时修内部调用点并改用
> `go vet` 才拿到干净结果。记此以自证四道门不是形式。

## 五、boundary[1] 知识转移 —— 语义完整，清单有一处假阳性

### 5.1 语义转移：完整，且比原文更强

原文（`cache.go` 被删的 `cloneOHLCV` 上，2 行）：

> `OHLCV is a flat value type, so a shallow element copy is a deep copy.`

转移后（`internal/core/types.go` 的 `OHLCV` 定义处，+15 行）**覆盖了原文的全部语义并补足三层**：

| 层 | 原文 | types.go |
|---|---|---|
| **结论** | 浅元素拷贝 = 深拷贝 | ✅「当且仅当元素是 flat value type」——把**充要条件**显式化 |
| **失效机制** | 无 | ✅ 加 map/slice/指针字段 → `slices.Clone` **静默退化**，且**不会有任何测试变红** |
| **下游动作** | 无 | ✅ 需同步把各 collector 的 `slices.Clone` 改逐元素深拷贝 |
| **为何在此** | 无 | ✅「移到**会被违反的这一侧**——改本文件的人看不到写在 collector 侧的注释」 |

⇒ **不是文字搬运，是教训 6 的完整落实**（连「为什么放这里」都写进去了）。

### 5.2 下游清单：一处假阳性，无假阴性

types.go 列的是「yahoo / twelvedata / **lixinger** 的 `FetchHistory`、以及任何缓存
`[]OHLCV` 的新接入点」。我逐一核实：

| collector | 实际 | 清单是否准确 |
|---|---|---|
| yahoo | `yahoo.go:302 return slices.Clone(data)` —— `[]core.OHLCV` | ✅ 准确 |
| twelvedata | `client.go:108 return slices.Clone(out)` —— `[]core.OHLCV` | ✅ 准确 |
| **lixinger** | 只有 `client.go:55 bytes.Clone(raw)`，缓存在 **`[]byte` 层**；`FetchHistory` 每次从字节重新解析出 OHLCV，**不缓存也不 Clone `[]OHLCV`** | ❌ **假阳性** |
| tushare | 缓存 `[]row`（含 map），用 `cloneRows` 而非 `slices.Clone`；`row` 不是 `core.OHLCV` | ✅ 正确地未列入 |

⇒ **给 OHLCV 加引用型字段时，lixinger 实际不需要改动**（它每次调用都新建 OHLCV，无共享）。

**危害评估：低。** 假阳性只会让后人去看一眼 lixinger 然后发现无事可做；**没有假阴性**
（真正需要改的 yahoo/twelvedata 都在列），且有「任何缓存 `[]OHLCV` 的新接入点」这条兜底。
不构成返工项，建议后续顺手删掉 `lixinger` 三字。

### 5.3 那 6 处扩散点未因删除而失效 —— 但留下 3 处指向已删文件的历史指针

首轮我统计过该知识已扩散到 6 处。本次删除后复查：**全部仍有效**，因为它们都把原文
**内联引用**了，不依赖 `cache.go` 存在。

但有 3 处保留了指向**已删除文件**的出处标注：

| 位置 | 文本 |
|---|---|
| `internal/core/types.go:80` | 「原本写在 internal/collector/cache.go 的 cloneOHLCV 上」|
| `internal/collector/twelvedata/gate_test.go:187` | 「这是 **cache.go:127-128** 原有的限定」|
| 契约 `design-spec.md:53`（§T2）| 「（`internal/collector/cache.go:128-129`：「OHLCV is a flat value type…」）」|

三处**均为历史沿革叙述且已内联原文**，知识零丢失；DoD 要求的「§T2 限定表述仍在」
**已满足**（`grep` 命中 1 处，表述完整）。仅**指针悬空**（那个文件与行号不复存在），
属记录瑕疵，**不作为 FAIL 依据**。建议契约 §T2 择机把 `cache.go:128-129` 改成
「已删除，见 `internal/core/types.go` 的 OHLCV 定义处」。

## 六、约束与回归

| 项 | 结果 |
|---|---|
| scope | 改动全部落在 `./internal/collector` + `./cmd/atlas` + `./internal/core`，与 `writes` 一致，**无越界** ✅ |
| **C1 / 反审 A4** | `git diff b197394 2a77803 -- go.mod go.sum` **无输出** ✅ |
| **反审 A11** | **全量 `go test ./... -race`：62 包 ok / 0 FAIL** ✅（输出中的 `ld: warning: malformed LC_DYSYMTAB` 是 macOS 链接器噪音，非测试失败——我逐行确认无 `--- FAIL`）|
| build / vet | `go build ./...` / `go vet ./...` 均 **exit 0**，删除未留孤儿 ✅ |
| 三符号消失 | 符号引用模式零命中；朴素 `grep NewCached` 会命中 `internal/context` 的 `NewCachedNewsProvider`（**无关同前缀符号**，非残留）✅ |
| 还原 | 每个变异 `cp` + `md5` 校验；收尾 `git status --porcelain` 空、`git diff --stat` 空 ✅ |
| worktree | 在**主仓库**执行 `remove` ✅ |

## 七、备录（均不作为 FAIL 依据）

1. §四 的自述精度订正：`var` 断言非冗余的**结论正确**，独有贡献是 6 个方法而非 7 个。
2. §5.2 的清单假阳性：`lixinger` 不需要列入。
3. §5.3 的 3 处悬空历史指针（含契约 §T2）。
4. §3.2 的测试名/覆盖面观察：断言只覆盖 yahoo/eastmoney，对 A8 足够但名字读起来更宽。

## 八、复现命令

```bash
git worktree add --detach ../wt-v011 2a77803 && cd ../wt-v011

# 判定线（新口径）
GOTOOLCHAIN=local go test ./internal/collector/ ./cmd/atlas/ ./internal/core/ -count=1 \
  -coverpkg=./internal/collector,./cmd/atlas,./internal/core -coverprofile=/tmp/c.out
GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | tail -1          # 75.8%

# 反审 A11 / A4
GOTOOLCHAIN=local go test ./... -count=1 -race                      # 62 ok / 0 FAIL
git diff b197394 2a77803 -- go.mod go.sum                           # 无输出

# 三符号消失（必须用符号引用模式，朴素 grep 会误命中 NewCachedNewsProvider）
grep -rnE '\bNewCached\(|\bmaybeCache\(|\bCachedCollector\b' --include='*.go' .

# A8 核心变异 D1：装配路径重新包装 → 唯一守护者转红
#   RegisterCollector(mutWrap{yahooCollector}) + type mutWrap struct{ collector.Collector }
#   → collectors_test.go:182 红，且单独 -run 亦红

# 收尾（必须在主仓库执行）
cd <主仓库> && git worktree remove ../wt-v011
```
