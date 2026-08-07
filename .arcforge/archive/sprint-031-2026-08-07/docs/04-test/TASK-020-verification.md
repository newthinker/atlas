# TASK-020 验证报告 —— tushare 缓存槽区分度（key/topic 两维）与业务错误不写缓存

- 验证者: test-agent-16 / `assignment_epoch: 1`
- 被验对象: **`792d915`**（新增 `keyguard_test.go`，**生产代码零改动、既有测试一字未改**）
- 验证环境: 独立 worktree `../wt-v020 @ 792d915`（已在主仓库拆除）
- 缺口由我在 TASK-020 判定阶段实测确认（key 维度 + topic 维度）

## 结论：**PASS（verified）**

三条守护全部经变异实证。**K1 拿到闭环左下格**（既有两条在同一变异下保持绿），
**E1'/E2 正反对照成立**（新测试区分「错误被缓存」与「错误类型改变」，不是对任何改动都红）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准 | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] key 维度 | 每个影响结果的参数各自独立验证 | `TestCacheSlotDistinguishesEachParam`（4 子用例）| **K1** 调用点换固定串 → **4 格全红** `:140`；**K2** 只把 `fields` 排出键 → **仅 `/fields` 红** | **PASS** |
| functional[0] topic 维度 | 不同 api 不得共用缓存槽 | `TestCacheSlotDistinguishesTopic` | **T1** topic 塌成常量 → 单独 `-run` 红 `:165`，失败信息来自自己的断言 | **PASS** |
| error_handling[1] | 业务错误不写缓存，校验须留在被缓存的 fn 内 | `TestBusinessErrorIsNotCached` | **E1'** 40203 返回成功值 → **`:195` + `:199` 双红** | **PASS** |
| non_functional | 既有测试一字不改全绿；`-race`；0 SKIP；覆盖率 | 实测 §五 | | **PASS** |

## 二、K1 闭环左下格 —— 一次复现了我的判定，也证明新测试正好补在缺口上

**K1**：把调用点 `policy.Fetch(c.gate, topicPrefix+apiName, callKey(params, fields), ...)`
的第三个实参整个换成固定串。

| | 结果 |
|---|---|
| **新增的 4 个子用例** | **全红** `keyguard_test.go:140` ✅ |
| **既有 `TestCallKeyDistinguishesParams`** | **绿**（不在失败列表）❌ 漏检 |
| **既有 `TestCallKeyIsOrderIndependent`** | **绿**（不在失败列表）❌ 漏检 |

⇒ **左下格成立**：同一变异在旧测试集下漏检、在新测试集下捕获。这正是我在判定阶段
给出的结论（「有聚合度、有纯函数单测，唯独没有区分度」）的独立复现。

### 2.1 Dev 把缺口定位得比我更深一层，我核实成立

我报的是「区分度无守护」。它进一步指出：

> 既有 `TestCallKeyDistinguishesParams` 是 `callKey` 的**纯函数**断言，不走 Fetch 路径。
> ⇒ **缺的不是「`callKey` 会区分」，是「缓存槽真的用了 `callKey` 的输出」。**

**K1 与 K2 的对比正好证成这一点**：

| 变异 | 改的是 | 既有纯函数测试 | 新的行为测试 |
|---|---|---|---|
| K1 | **调用点**（`callKey` 本身没动）| **绿** | 红 |
| K2 | **`callKey` 函数内部**（排除 `fields`）| **红** | 红（仅 `/fields`）|

⇒ 两条测试守的是**两个不同的对象**：纯函数守「函数会不会区分」，行为测试守
「缓存槽有没有用它的输出」。**改调用点时只有后者能看见。**

## 三、E1'/E2 正反对照 —— 新测试确实在区分两件事

Leader 要求独立复现这组对照（它复现了 qa-agent-8 的错误归因形态）。我做了**三格**：

| 变异 | 性质 | `TestBusinessErrorIsNotCached` |
|---|---|---|
| **E1'** 40203 分支改 `return []row{}, nil`（错误被当成功值）| **缓存行为改变** | **红** `:195`「HTTP 请求 1 次, want 2」+ `:199`「静默返回了成功」✅ |
| **E2** 停用 `if env.Code == 40203`（qa-agent-8 首版形态）| 只改错误**类型** | **绿** ✅ |
| **E1(我首版)** 停用 40203 **与** `code != 0` 两个分支 | 同样只改错误类型 | **绿** ✅ |

⇒ **新测试红/绿的分界线正好落在「缓存行为变没变」上**，不是「错误类型变没变」。
qa-agent-8 当时看到 5 条转红就采信——那 5 条全是既有的错误类型测试
（`TestErrRateLimited` / `TestErrNoPermission` / `TestNonPolicyErrorsUnaffected` 等），
**新测试根本不在其中**。我在 E2 下按纪律 4 单独 `-run` 新测试，三条全 PASS。

### 3.1 我首版 E1 绿的诊断 —— 又一个「另一道守卫」实例

我首版 E1 停用了两个错误分支，预期红、**实际绿**。按纪律查归因，诊断探针输出：

```
call 0: rows=0 err=tushare: daily: no trade_date field
call 1: rows=0 err=tushare: daily: no trade_date field
hits=2
```

⇒ **下游还有一道守卫**（`client.go:198` 的 `no trade_date field`）：40203 的响应体没有
`data`，即便跳过信封校验，解析层仍会报错 ⇒ fn 仍返回 error ⇒ Gate 仍不缓存 ⇒ **缓存行为一字未变**。

**所以我首版 E1 的绿是正确的**——它和 E2 同类，都只改了错误类型。这是契约教训 9 那条
「删掉守卫 X 后行为不变 ≠ X 无用，可能只是存在另一道守卫」的新实例，
而且它顺带说明：要让业务错误真正进缓存槽，必须让 fn 返回**成功值**（E1' 的形态），
只是「不报那个错」还不够。

> **若我停在「预期红、实际绿 ⇒ 无守护」，会报出一个不存在的缺口并触发一轮返工。**

## 四、topic 维度：连带不废掉证据（纪律 4）

topic 塌成常量的本地变异**必然有连带**（内置四个 tushare 主题策略不等价，`daily_basic`
多一个 Quota）——这是我在判定阶段给出的结论，本次复现：**全量 6 条红**。

按纪律 4 单独 `-run` 新测试：

```
keyguard_test.go:165: HTTP 请求 1 次, want 2 —— 不同 api 共用缓存槽 ⇒ 拿到另一个接口的数据
--- FAIL: TestCacheSlotDistinguishesTopic
```

⇒ **失败信息来自它自己的断言**，归因成立。连带影响的是别的测试，不影响对这一条的判定。

> 这一格正是我先前标为「存疑、未判定」的那个——现在判了：**有守护**。

## 五、基线

| 项 | 结果 |
|---|---|
| 测试规模 | **34 个测试全 PASS / 0 FAIL / 0 SKIP** ✅ |
| `-race` | ok（1.65s）✅ |
| 覆盖率（单包口径，本任务无 `coverage_floor`）| **95.3%** ✅ |
| scope | 只新增 `keyguard_test.go`，**生产代码零改动、既有测试文件一字未改** ✅ |
| 新测试稳定性 | 连跑 **5 次全 ok** ✅ |
| 还原 | 每变异 `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

## 六、判据选择的核实（两处刻意选择，理由均成立）

1. **日期对照相差 ≥1 天**：`callKey` 经 `dateParams` 用 `Format("20060102")` ⇒ **日粒度**。
   基准 `20260101/20260131` 与变体 `20260201/20260228` 相差以月计。
   照搬别包的毫秒/秒级偏移会落进同一个键 ⇒ **假的「变异无效」**（教训 23）。**选择正确。**
2. **topic 对照取 `daily`/`index_daily`**：二者在内置表里策略等价（同一份 `tusharePolicy`），
   避开 `daily_basic` 的 `Quota` 成为混淆因子。**与我判定阶段的分析一致。**

另：测试用 `ttlTable` 手搓等价策略而非内置表，理由是「本文件要隔离的是缓存槽，不是策略解析」
——这与我给出的干净注入点（`gate.go` 的 `ck := topic + "|" + key`）是同一思路的两侧。

## 七、备录

1. **陷阱 10 的判定顺序**：Dev 自报首版把「每次调用都应报错」写在 `hits()` 断言之前，
   E1 下消息是「本轮未构成检验」——缺陷伪装成测试构造失败。调整后消息才指向真因。
   **我核了交付版：`:195` 的行为断言在前、`:197-202` 的错误断言其后、有效性对照组在最后**，顺序正确。
2. `TestBusinessErrorIsNotCached` 末尾有一个**TTL 有效性对照组**（正常 body 两次调用须只发 1 次请求）
   ——没有它，「2 次请求」在 TTL 根本没生效时也恒成立，本测试与「缓存压根没开」无法区分。**这一格是必要的。**
3. `TestCacheSlotDistinguishesEachParam` 有 `len(cases) == 0` 下界守卫（防空真）。

## 八、复现命令

```bash
git worktree add --detach ../wt-v020 792d915 && cd ../wt-v020
GOTOOLCHAIN=local go test ./internal/collector/tushare/ -count=1 -race   # 34 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/tushare/ -count=1 -cover  # 95.3%

# K1 闭环：调用点 callKey(params,fields) → "FIXEDKEY"
#   → 新测试 4 格红 :140；既有 TestCallKeyDistinguishesParams / IsOrderIndependent **保持绿**
# K2：callKey 内部排除 fields → 仅 /fields 红
# E1'：client.go:182-188 的 40203 分支改 `return []row{}, nil` → :195 + :199 双红
# E2：`if env.Code == 40203` → `if false && ...` → 新测试三条全 PASS（只改错误类型）
# T1：topic 塌成常量 → 全量 6 红（连带），单独 -run 新测试红 :165（自己的断言）

cd <主仓库> && git worktree remove ../wt-v020
```
