# TASK-012 验证报告 —— 路由表重写（黄金值先行）

- 验证者: test-agent-16
- 被验对象: `f48a8e066b4179d077b19638d6e56fe1159642c9`（route.go 110 行新增 / selector.go 103 改 / route_golden_test.go 62 行新增）
- 黄金值基线: `7528f6d`（旧实现 + 黄金值，其上**无 route.go**）
- 验证环境: 两个独立 worktree —— `../wt-v012-new @ f48a8e0`、`../wt-v012-base @ 7528f6d`
- assignment_epoch: 1

## 结论：**PASS（verified）**

9 条 done_criteria **全部通过**。15 个变异中 12 个被捕获，3 个存活——但三个存活项经逐一
取证**均非测试缺口**（2 个等价变异体 + 1 个本次重构未触碰的既有代码），详见 §三。

**门禁绕过的补偿验证已完成**：Leader 指出首次 `dev_done` 门禁在主工作区测的是旧代码、
`Coverage: 98.2%` 无证明力。本报告的全部数字均为我在被验 commit 上独立复跑所得，
不引用任何门禁回显。

---

## 一、前提独立复核（不采信任何转述）

| 检查 | 命令 | 结果 |
|---|---|---|
| 代码确已合入 | `git log --oneline -5` | `f48a8e0` → `7528f6d` → `bac0de1` ✅ |
| route.go 在 master | `git cat-file -e master:internal/collector/route.go` | 存在 ✅ |
| 工作区无未提交路由改动 | `git status --short internal/collector/` | 仅 `policy/` 两文件（dev-agent-35 的 TASK-001 返工，无关）✅ |
| TASK-012 worktree 已拆 | `git worktree list` | 无 `wt-TASK-012` / `wt-012-cov` 残留 ✅ |

## 二、验收标准 2「新旧实现均全绿」—— 证明是否成立

这是本任务最核心、也最容易被含糊带过的一条。**结论：成立，且我实跑复现了两端。**

### 2.1 黄金值文件跨 commit 的实际差异

```
git diff --numstat 7528f6d f48a8e0 -- internal/collector/route_golden_test.go
62      0       internal/collector/route_golden_test.go
```

**62 行新增，0 行删除。** 进一步逐行比对确认：

- 基线版前 131 行 vs 新版前 131 行 → `diff` 无输出，**完全一致**
- 基线版 132-137 行 vs 新版 194-199 行（尾部 `coveringQlib` 桩）→ **完全一致**
- 新增的 62 行全部是**追加**的两个新测试（`TestHKSuffixBeatsCryptoPrefix` /
  `TestRouteMissTableFallsBackConservatively`），它们引用 `routes`/`specificity`/
  `sortRoutes`/`lookupRoute` 等新实现才有的内部符号，**在旧实现上无法编译**，
  因此只能后加——这是合理的，不构成对基线的篡改。

**判定：黄金值本身（`TestRouteGoldenValues` 的 25 条期望、`TestRouteCaseInsensitive`、
`TestQlibCoversTakesPrecedenceOverTable`、`TestRouteFallsBackWhenCollectorMissing`）
一个字节都没动。** 「同一份黄金值」的说法成立。

### 2.2 在基线 commit 上实跑（旧实现）

```
cd ../wt-v012-base            # detached @ 7528f6d
ls internal/collector/route.go        → No such file or directory   ← 确为旧实现
go test ./internal/collector/ -run 'TestRoute|TestQlibCovers' -v
```

`TestRouteGoldenValues` **25 个子测试全 PASS**，另 3 个测试全 PASS，包回归 `ok`。

### 2.3 在新实现上实跑

同一份黄金值 + 新增 2 测试，全 PASS；`-race` 亦 `ok`。

**验收标准 2 判定：PASS（唯一一次真正的双端取证）。**

## 三、变异验证结果表（15 个，12 捕获 / 3 存活）

所有变异均以 `cp /tmp/route.orig.go` / `selector.orig.go` 还原；末次核实
`git status --porcelain` 空、`git diff --stat` 空、`HEAD=f48a8e0`。

| ID | 变异内容 | 期望 | 实际 | 捕获者 |
|---|---|---|---|---|
| MU1 | **删掉两处 `IsAShareIndex` 表前置规则**（反审 A9） | 红 | **红** ✅ | TestRouteGoldenValues/930713.CSI+930604.CSI（collector 与 market 双报）、既有 TestCSIIndexRouting |
| MU1a | 只删 `routeCollector` 里的前置 | 红 | **红** ✅ | 同上（仅 collector 维度报错） |
| MU1b | 只删 `MarketForSymbol` 里的前置 | 红 | **红** ✅ | 同上（仅 market 维度报错） |
| MU2 | **`*.HK` 具体度调低**（`*.HK`→`*HK`，3→2，令 `BTC*` 胜出） | 红 | **红** ✅ | TestHKSuffixBeatsCryptoPrefix（`got crypto/CRYPTO, want yahoo/HK`） |
| MU3 | 删掉 qlib `Covers()` 表前置规则 | 红 | **红** ✅ | TestQlibCoversTakesPrecedenceOverTable + 既有 TestSelectForSymbolPrefersQlibWhenCovered |
| MU4 | `sortRoutes` 退化为不排序（保留书写顺序） | 红 | **红** ✅ | TestHKSuffixBeatsCryptoPrefix 的有序性断言 |
| MU5 | `sortRoutes` 改升序（最宽泛优先） | 红 | **红** ✅ | TestRouteGoldenValues 大面积转红 |
| **MU6** | `specificity` 不再扣除通配符（`len(pattern)`） | 红 | **绿（等价变异体，见 §3.1）** | — |
| MU7 | `KnownIndexMarket` 去掉「含通配即 unknown」 | 红 | **红** ✅ | TestRouteGoldenValues（600519.SH 等误报 known=true） |
| MU8 | `MarketForSymbol` 去掉 `ToUpper` | 红 | **红** ✅ | TestRouteCaseInsensitive |
| MU9 | `lookupRoute` 落空返回 `ok=true` | 红 | **红** ✅ | TestRouteMissTableFallsBackConservatively（4 条断言齐报） |
| **MU10** | 删掉内置表末尾 `{"*", "yahoo", MarketUS}` 兜底行 | 红 | **绿（等价变异体，见 §3.1）** | — |
| **MU11** | 删掉「首选缺席回退 yahoo」分支 | 红 | **绿（既有未改动代码，见 §3.2）** | — |
| MU12 | 回退链不再跳过 qlib | 红 | **红** ✅ | 既有 TestSelectExternalForSymbolNeverReturnsQlib / …OnlyQlibReturnsNil |
| MU13 | `matchPattern` 前缀/后缀分支互换 | 红 | **红** ✅ | TestRouteGoldenValues 大面积转红 |

### 3.1 MU6 / MU10 是等价变异体，不是测试缺口

没有停在「测试没抓到」就下结论，而是取证行为是否真的改变。构造 58 个符号的语料
（含 A 股/中证/港股/美股/已登记与未登记指数/商品/两种加密后缀/加密前缀裸符号/
`BTC.HK` 类冲突形态/空串与畸形符号），对每个符号打印
`SelectExternalForSymbol / MarketForSymbol / KnownIndexMarket` 三元组，
原实现与变异实现**逐行 `diff` 无输出**：

- **MU6**：排序分层确实变了（`MATIC*` 升到独占首位等），但**所有可能相撞的 pattern
  对相对次序不变**（`*.HK` 仍在 `BTC*` 之前、`^HSI` 仍在 `^*` 之前、`*` 仍在最后），
  故对任意输入结果相同。
- **MU10**：`*` 兜底行与 `lookupRoute` 落空后的保守回退（`return MarketUS` /
  `return "yahoo"`）**本就产出同一结果**，二者是设计上的双保险（Dev 在 route.go
  注释里已写明这层意图）。删掉任一层，另一层无缝接住。

**这两个变异存活是正确的**，测试无需（也无法）区分语义等价的实现。

### 3.2 MU11 落在本次重构未触碰的既有代码上

`git diff 7528f6d f48a8e0 -- internal/collector/selector.go` 证实：
`SelectExternalForSymbol` 本次**只把那段 `switch` 谓词替换为
`reg.Get(routeCollector(symbol))`**；其后的「Default to Yahoo」与「任一非 qlib
collector」两级回退**逐字未变**。

`TestRouteFallsBackWhenCollectorMissing` 之所以抓不到，是因为它的四个场景里注册表
恒为 ≤1 个 collector（`newRegistryWith("yahoo")` / `newRegistryWith("eastmoney")` /
空 / nil），删掉 yahoo 优先分支后，最后的遍历兜底会返回同一个 collector。
能区分二者的场景是「注册表同时有 yahoo 与 eastmoney、而符号首选 crypto」——此时
原实现确定性返回 yahoo，变异版则落到 `for _, c := range reg.GetAll()`，
而 `Registry.GetAll()` 遍历的是 **map**（`registry.go:34-43`），顺序不确定。

**boundary[2] 判定 PASS**：该条要求的是「重构后回退仍正确」，而这段代码逐字未动、
命名测试在新旧两端均绿，语义确定被保全。MU11 暴露的是**既有**的测试强度不足与
**既有**的潜在非确定性，**不属于 TASK-012 的范围**，作为建议记于 §八。

## 四、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 黄金值先对旧实现跑绿并单独提交为基线，重写后同一份仍全绿 | `TestRouteGoldenValues` 25 子测试；**§二 双端实跑取证**；基线文件 0 删除 | **PASS** |
| functional[1] | 四个公开函数签名完全不变，6 处外部调用点零改动（C4） | 签名逐字比对四条**全一致**；`git diff --stat` 仅 3 个文件、**全在 `internal/collector/` 内**，包外零改动 | **PASS** |
| functional[2] | 具体度优先，排序稳定且与注册顺序无关 | `TestHKSuffixBeatsCryptoPrefix`（有序性断言 + 两种书写顺序同结果）；MU4/MU5 捕获 | **PASS** |
| functional[3] | 大小写不敏感 | `TestRouteCaseInsensitive`；MU8 捕获 | **PASS** |
| boundary[0] | qlib `Covers()` 作为表前置规则，优先级最高 | `TestQlibCoversTakesPrecedenceOverTable`；MU3 捕获 | **PASS** |
| boundary[1] | **`IsAShareIndex()` 同为表前置规则**，930713.CSI / 930604.CSI 路由到 eastmoney / CN_A，黄金值须显式登记（反审 A9） | 两符号**确在** `TestRouteGoldenValues` 第 34-35 行显式登记；`selector.go:79,90` 两处前置；MU1/MU1a/MU1b **三向捕获**；另有既有 `TestCSIIndexRouting` 独立兜底 | **PASS** |
| boundary[2] | 路由表命中但 collector 未注册时正确回退 | `TestRouteFallsBackWhenCollectorMissing`；回退链逐字未改（§3.2）；MU12 捕获 qlib 跳过语义 | **PASS** |
| error_handling[0] | `SelectExternalForSymbol(reg,"BTC.HK").Name()=="yahoo"` 且 `MarketForSymbol("BTC.HK")==MarketHK` | `TestHKSuffixBeatsCryptoPrefix` 前两条断言；MU2 捕获 | **PASS** |
| non_functional[0] | 调用方包全绿；`internal/collector` 覆盖率 ≥98% | 5 个包全 `ok`；覆盖率 **99.1%** | **PASS** |

## 五、覆盖率实测（独立复算，新旧对照）

| commit | 实现 | 覆盖率 |
|---|---|---|
| `7528f6d` | 旧（五谓词） | **99.1%** |
| `f48a8e0` | 新（路由表） | **99.1%** |

**无回退，且远高于 98% 门槛。** 逐函数：

```
route.go:    sortRoutes 100%  specificity 100%  matchPattern 100%  lookupRoute 100%
selector.go: KnownIndexMarket 100%  SelectForSymbol 100%
             SelectExternalForSymbol 100%  MarketForSymbol 100%  routeCollector 100%
total: 99.1%
```

> 注：Dev 首次门禁回显的 `98.2%` 确系改动前水位，与本次 99.1% 不是同一对象，
> 已按 Leader 提示不予采信；上表两个数字均为我在对应 commit 上亲自跑出。

## 六、约束与回归核查

| 项 | 结果 |
|---|---|
| C4 四个公开函数签名 | `KnownIndexMarket` / `SelectForSymbol` / `SelectExternalForSymbol` / `MarketForSymbol` **逐字一致** ✅ |
| C4 外部调用点零改动 | `git diff --stat` 只有 3 个文件，均在 `internal/collector/` 内；包外 6 个非测试调用点（app.go、export_signals.go、backtest.go、qlib_wiring.go、tushare/collector.go、symbol_detail.go）**零改动** ✅ |
| 五个旧谓词 + 两张旧表被取代 | `isAShareSymbol` / `isIndexSymbol` / `isCommoditySymbol` / `isCryptoSymbol` / `indexMarkets` / `cryptoTickers` 在新代码中**仅作为出处注释残留，无任何代码引用** ✅ |
| 调用方包全绿 | `internal/collector` / `internal/app` / `internal/api/handler/api` / `internal/collector/tushare` / `cmd/atlas` **5 个包全 ok** ✅ |
| `-race` | `ok`（1.48s）✅ |
| build / vet / gofmt | 均 exit 0 / 无输出 ✅ |

## 七、Leader 点名的两处 Dev 增补 —— 判断

### 7.1 `TestHKSuffixBeatsCryptoPrefix` 的有序性与顺序无关性断言：**合理，且已证明其必要性**

Dev 的理由（原测试只验 BTC.HK 一点、证明不了 functional[2] 的「排序稳定且与注册顺序无关」）
经变异验证**成立**：**MU4**（把 `sortRoutes` 退化为不排序）——如果只有原来的 BTC.HK
单点断言，这个变异会**存活**，因为内置表的书写顺序恰好已使所有黄金值通过；正是新增的
`routes` 有序性断言把它抓成红（`"*.HK"(3) 排在 "^GSPC"(5) 之前`）。

**这条增补不是镀金，它堵住了 functional[2] 的一个真实盲区。**

### 7.2 `TestRouteMissTableFallsBackConservatively` 的 defer 还原：**可靠**

```go
saved := routes
defer func() { routes = saved }()
routes = sortRoutes([]Route{{"*.SH", "eastmoney", core.MarketCNA}})
```

四项取证：

1. **无并发风险**：全包 6 个测试文件中 `t.Parallel()` **出现 0 次**，Go 默认串行执行子测试，
   包级变量替换期间不存在其它测试并发读 `routes`。
2. **panic / `t.Fatal` 下不泄漏**：`defer` 在 panic 展开与 `runtime.Goexit`（`t.Fatal` 的实现）
   两条路径上均会执行。该测试内部用的是 `t.Error`（不中断），更无风险。
3. **乱序实测**：`-shuffle=on` 连跑 **5 轮全部 `ok`**，未出现任何顺序相关失败。
4. **同进程重入实测**：`-count=2` `ok`；显式指定
   `-run 'TestRouteMissTable…|TestRouteGoldenValues|TestHKSuffix…'` 亦全 PASS，
   即先污染后校验的场景下 `routes` 确已还原。

**判定：还原可靠，不构成缺陷。**

## 八、备录与建议（均不作为 FAIL 依据）

1. **`.HK` 与加密前缀的口径不一致（Leader 点名第 4 项）—— 我核实后认可 Dev 的取舍。**
   实测（临时探针，已删除）确认 Dev 的风险陈述**准确**：

   ```
   BTC.HK / ETH.HK / SOL.HK / XRP.HK / ADA.HK / DOT.HK / UNI.HK / LTC.HK  → yahoo | HK
   ATOM.HK / AVAX.HK / LINK.HK / MATIC.HK / DOGE.HK                        → crypto | CRYPTO
   ```

   - **(a) 这些形态确不存在于 watchlist/配置**：全仓 grep（`*.go/*.yaml/*.yml/*.json/*.md`）
     除 route.go 自身注释外**零命中**；`configs/percentile-watchlist.yaml` 与
     `config.example.yaml` 中的港股符号全部是**数字代码**形态（0700.HK / 2800.HK /
     9988.HK / 3033.HK …）。港交所代码本就是数字，「以加密货币代号命名的 .HK 股票」
     在真实命名空间中不可能出现。
   - **(b) 不违反任何一条 DoD**：error_handling[0] 只写死了 `BTC.HK`（已 PASS）；
     functional[2] 要求「具体度优先」，而 `ATOM*`(4) > `*.HK`(3) 判 crypto **恰恰是
     该规则的正确结果**，不是例外。

   **结论：Dev 没有擅自扩大范围，处理正确，且在 route.go 注释里主动写明了取舍与将来的
   修法（「应显式加一条精确路由，而不是调整具体度语义」）。** 附一处订正：Dev 的代码注释
   列了 5 个形态（`DOGE*`/`AVAX*`/`MATIC*`/`LINK*`/`ATOM*`），比派验单里列的 4 个多一个
   `DOGE*`——**代码注释是完整且准确的那一份**。

2. **`f48a8e0` 的 commit message 把基线 sha 写成 `482442c`**，实际基线是 `7528f6d`。
   系 rebase 前的旧 sha 未同步，属提交信息瑕疵，不影响任何代码或验证结论。

3. **建议（属未来任务，非本任务缺陷）**：`SelectExternalForSymbol` 最后一级
   「任一非 qlib collector」回退遍历的是 map（`GetAll()`），**顺序不确定**。当 yahoo
   未注册且有 2 个以上其它 collector 时，选中结果不可预测。这是既有行为，本次重构
   逐字保留。若后续要收紧，可给 `GetAll()` 定序或给该回退加一条确定性优先级，
   并补一个「yahoo+eastmoney 同时在册、符号首选 crypto」的用例。

4. **task JSON 的 `discovery` 字段未回填**（`.arcforge/discoveries/TASK-012.json` 本身
   已落盘，6 findings / 5 decisions，质量良好）。与 TASK-001 同类记录瑕疵，已按 Leader
   指示不计入判定。

5. **3 条 `scope-writes-outside-packages` validator 告警**已按 Leader 说明确认为
   **有意设置**（文件级 `writes` 精确声明以避开子树互斥误判），未按 ISSUE-4 处置。

## 九、复现命令

```bash
git worktree add --detach ../wt-v012-base 7528f6d      # 旧实现 + 黄金值
git worktree add --detach ../wt-v012-new  f48a8e066b4179d077b19638d6e56fe1159642c9

# 验收标准 2 双端取证
cd ../wt-v012-base && GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -run 'TestRoute|TestQlibCovers' -v
cd ../wt-v012-new  && GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -v

# 黄金值基线未被篡改
git diff --numstat 7528f6d f48a8e0 -- internal/collector/route_golden_test.go   # 应为 62  0

# 覆盖率 / race / 调用方 / defer 还原
GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -coverprofile=/tmp/c.out && GOTOOLCHAIN=local go tool cover -func=/tmp/c.out | tail -1
GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/collector/ ./internal/app/ ./internal/api/handler/api/ ./internal/collector/tushare/ ./cmd/atlas/ -count=1
for i in 1 2 3 4 5; do GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -shuffle=on; done

git worktree remove ../wt-v012-base && git worktree remove ../wt-v012-new
```
