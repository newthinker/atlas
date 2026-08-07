# TASK-007 F2 返工验证报告 —— yahoo 两处缓存键的区分度守护

- 验证者: test-agent-16 / `assignment_epoch: 1` / `rework_count: 1`
- 被验对象: **`f6a78af`**（`gate_test.go` +128/-17，**仅测试文件，生产代码零改动**）
- 判定依据: **`fix_items[F2]`**（缺口由我在 F1 验收时发现，变异 W5）
- 验证环境: 独立 worktree `../wt-v007f2 @ f6a78af`（已在主仓库拆除）

## 结论：**PASS（verified）**

F2 完全满足，**7 个维度各自独立有变异证据**，两处 key 交叉独立。
F1 的守护经 helper 重构后**无回归**。另有一处 Dev 自行发现并顺带补上的 F1 残留缺口，我已实证（§四）。

---

## 一、`fix_items[F2]` 逐项对照

| F2 的要求 | 证据 | 判定 |
|---|---|---|
| yahoo **两处 key** 各补区分度守护 | `TestFetchHistoryCacheKeyDistinguishesParams`（4 子测试）+ `TestFetchEPSHistoryCacheKeyDistinguishesParams`（3 子测试）| **PASS** |
| **`eps.go:49` 一并核并补**（我上轮未验、判为很可能同缺口）| EPS 三格全部实证（E1/E2/E3）| **PASS** |
| 至少用两组不同参数验证不互相命中（陷阱 16）| **a→b→a 重放** —— 同时排除「参数没进键」（总数 1）与「压根没缓存」（总数 3）| **PASS** |
| **trap：别只测 symbol 一个维度**，参照 crypto 三组各自独立 | History 四维（symbol/start/end/interval）、EPS 三维，**每维一个 `t.Run` 子测试** | **PASS** |
| 纯测试改动，不碰生产代码 | `git show --name-only` 只有 `gate_test.go` | **PASS** |

## 二、变异验证：7 个维度，每个只红自己那一格

每个变异**注入前先写下预期**，四道门全程强制。

| ID | 变异 | 红在 | 只失败的子测试 | 对照 |
|---|---|---|---|---|
| Y1 | History key 去 `symbol` | `:555` | `/symbol` | **EPS 三格全绿** |
| Y2 | History key 去 `start` | `:555` | `/start` | 同上 |
| Y3 | History key 去 `end` | `:555` + `:436` | `/end` + F1 粒度 | 同上（`:436` 是预期内的连带，见下）|
| Y4 | History key 去 `interval` | `:555` | `/interval` | 同上 |
| E1 | EPS key 去 `symbol` | `:595` | `/symbol` | **History 四格全绿** |
| E2 | EPS key 去 `start` | `:595` | `/start` | 同上 |
| E3 | EPS key 去 `end` | `:595` + `:490` | `/end` + F1 粒度 | 同上 |

**Y3/E3 连带红了 F1 的「分钟粒度不得放粗」是预期内的**：那条 F1 测试正是靠变动 `end` 来验粒度，
`end` 整个从键里消失时它自然也失守。**不是归因不清**——两条断言的失败信息各自指向自己的性质，
且 Y1/Y2/Y4/E1/E2 五格都是干净的单格红。

**交叉独立性**：改 `yahoo.go` 的键从不触及 EPS 三格，改 `eps.go` 的键从不触及 History 四格
⇒ **两处 key 相互独立，F2 要求「各补一条」是必要的**（与 F1 同结论）。

## 三、F1 回归：helper 外提未损伤既有守护

本次 diff 有 **17 行删除**，我核过：全部是把 `TestFetchEPSHistoryCacheKeyAggregatesNearbyTimes`
内联的 EPS server helper **提到包级**（`epsCountingServer`），供新 EPS 测试复用。

**F1 两条聚合度测试的断言行逐字比对一致**（我对 `415f900` 与 `f6a78af` 做了函数体级 diff）。

回归变异：

| ID | 变异 | 结果 |
|---|---|---|
| R1 | 去掉两个 `Truncate`（F1 分句 a）| **红 `:420`** ✅ |
| R2 | 两个都放粗到 `Hour`（F1 分句 b）| **红 `:436`** ✅ |

**F1 无回归。**

## 四、Dev 自行发现并补上的一处 F1 残留缺口 —— 我已实证

Dev 在 `/start` 用例上写了这样一条注释：

> start 这一格顺带堵上聚合度那组的残留缺口：它的「粒度不得放粗」只变动 **end**，
> **start 的 `Truncate` 被放粗到小时原先无人能抓**。故此处取整分钟偏移，而不是随便找个不同的时刻。

**变异 R3 实证该主张成立**：只把 `start.Truncate(time.Minute)` 放粗到 `Hour`（`end` 不动）

```
红：gate_test.go:555  /start
不红：gate_test.go:436  F1 的「分钟粒度不得放粗」
```

⇒ **该缺陷在 F2 之前确实无人能抓**（F1 那条只变动 `end`，对 `start` 的粒度放粗不敏感），
F2 的 `/start` 取**整分钟偏移**后正好补上。

**这不是 F2 要求的**（F2 只要求区分度），是 Dev 自己看出来的。且它对偏移量的选择是**有意的**
——随便取一个不同时刻只能验区分度，取整分钟偏移才能同时压住粒度放粗。

## 五、另一处刻意的判据选择，理由成立（我已核实）

`interval` 变体取 `"1h"` 而非随手写个 `"1wk"`，Dev 的理由：

> `toYahooInterval` 是 switch + default 兜底成 `"1d"`，拿未知值当变体等于要求键区分两个
> **上游 URL 完全相同**的请求 —— 那样断言即使绿，守的也是一个并不存在的区分。

**核实**（`yahoo.go:395-408`）：确有 `default: return "1d"`，`"1wk"` → `"1d"`，与基准的 `"1d"`
产生**完全相同的上游请求**。而 `"1h"` 是 switch 里的真实分支，上游确实不同。

⇒ **理由成立。** 这与本 Sprint 反复出现的「分支可达性」同族：判据落在不可达/无差别的输入上，
断言即使绿也守不住任何东西。

## 六、基线

| 项 | F1 轮 | 本轮 |
|---|---|---|
| 测试规模 | 50 PASS | **52 PASS**（+2，含 7 个子测试）|
| SKIP | 0 | **0** ✅ |
| `-race` | ok | **ok**（23.3s）✅ |
| 覆盖率（单包口径，本任务无 `coverage_floor`）| 89.3% | **89.9%**（+0.6pp）✅ |
| scope | — | 仅 `gate_test.go`，`writes` 内，**生产代码零改动** ✅ |
| 新测试稳定性 | — | `-run CacheKey` 连跑 **8 次 0 失败** ✅ |
| 还原 | — | 每变异 `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

## 七、备录

1. **F2 的根因定性是准确的**：TASK-007 的原始 DoD 只有四条 functional，区分度 criteria 是
   本 Sprint 后期才出现的。按 F1 同口径「规格后到不计」熔断额度——这与我在报告里的定性一致。
2. 我在 F1 验收时只验证了 `yahoo.go` 那处、把 `eps.go` 判为「很可能同缺口但未验证」。
   本轮 E1/E2/E3 三格证实**确实同缺口**，F2 要求「一并核并补」是对的。

## 八、复现命令

```bash
git worktree add --detach ../wt-v007f2 f6a78af && cd ../wt-v007f2
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -race    # 52 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -cover   # 89.9%

# 七个维度（每个只红自己那一格）
#   yahoo.go:312 的键去掉 symbol / start / end / interval → :555 对应子测试
#   eps.go:49   的键去掉 symbol / start / end             → :595 对应子测试
# F1 回归：去两个 Truncate → :420；两个放粗到 Hour → :436
# Dev 补的残留缺口：**只**把 start 放粗到 Hour → 红 :555(/start)，不红 :436

cd <主仓库> && git worktree remove ../wt-v007f2
```

---

# 附录：补核 Dev 自曝的取证缺口（判定不变）

dev-agent-35 在交付后主动申报：它的 harness 只 grep `--- FAIL` 行拿到**子测试名**，
**没读断言消息**；而每个子测试有两条失败路径且显示为同一行——
`t.Fatal(err)`（请求出错）与 `t.Errorf("缓存键未区分 ...")`（计数断言）。
它有一个论证认为必然是后者，但明确声明「这是论证不是观察」。

**它的申报是对的，而且这一格我原先也没显式核过。补核如下，结论不变。**

## A1. 行号本身已完成归因（我的证据比 Dev 的强一层，但当时未点明）

| 行 | 语句 |
|---|---|
| `gate_test.go:551` | `t.Fatal(err)` —— 请求出错路径 |
| **`gate_test.go:555`** | **`t.Errorf("缓存键未区分 %s: HTTP 请求 %d 次, want 2...")`** —— 计数断言 |
| `gate_test.go:591` | `t.Fatal(err)`（EPS 侧）|
| **`gate_test.go:595`** | **`t.Errorf("EPS 缓存键未区分 ...")`**（EPS 侧计数断言）|

我的 runner 判红的条件是「输出中存在 `<file>_test.go:NN:` 断言行」并记录该行号。
正文 §二 表格里记的 **全部是 `:555` / `:595`**，**无一落在 `:551` / `:591`**。

⇒ **两条失败路径的行号不同，行号即归因。** Dev 的 harness 抓子测试名做不到这一点，
我的抓行号做得到——但我当时**没有把这层含义写出来**，所以它申报时我无法立刻确认。
**记此为我自己的报告缺陷：证据是充分的，但结论的支撑关系没写明。**

## A2. 按 Dev 给的判据做的直接确认（M8 = 我的 R3）

注入「只把 `start.Truncate(time.Minute)` 放粗到 `Hour`，`end` 不动」，完整输出：

```
gate_test.go:555: 缓存键未区分 start: HTTP 请求 1 次, want 2
                  （为 1 说明该参数没进键 —— 不同起点共用一槽，会返回别的区间；
                    为 3 说明缓存压根没生效）

--- PASS: .../symbol      --- FAIL: .../start      --- PASS: .../end      --- PASS: .../interval
--- PASS: F1「相邻时间落进同一槽」   --- PASS: F1「分钟粒度不得放粗」
```

**三项同时确认**：
1. 红来自**计数断言**（`HTTP 请求 1 次, want 2`），**不是** `t.Fatal` 的请求错误
2. **只红 `/start` 一格**，同组另三格保持绿
3. **F1 两格保持绿** —— 这正是 Dev 预先写死的判据，绿才证明「`start` 放粗原先无人能抓」

## A3. 全格复查：无一格的红落在 `t.Fatal` 行

抽 History/symbol、History/interval、EPS/symbol 三格重跑并只提取行号：

```
History/symbol   → gate_test.go:555
History/interval → gate_test.go:555
EPS/symbol       → gate_test.go:595
```

**全部为断言行。** 加上正文 §二 已记录的其余各格，**7 格无一例外**。

## A4. 关于 `fmt.Sprintf` 类变异的 vet 陷阱（Dev 的反面对照 X1）

Dev 报告：只删 `%d` verb 而保留实参 → `go test` 默认跑的 vet 报
`format %s has arg ... of wrong type int64` ⇒ **build failed**，那一格显示为红但**红的是构建**。

**我的变异未踩此坑**：我的注入一律保持 verb 与实参配对（如去掉 `start` 时写成
`symbol, int64(0), end..., interval`，`%d` 与 `int64(0)` 匹配），故门②全部通过、
无一格是构建红。

> 这是「编译失败的红」的第七次，但**载体是 `vet` 而非编译器**——前六次都是删符号/删导入。
> `fmt.Sprintf` 类变异必须 **verb 与实参一并处理**。

## A5. 判定

**维持 `verified`。** 补核未推翻任何一格，反而把 Dev 论证的那一步变成了观察。
