# TASK-006 验证报告 —— 策略闸门装配接线 + config 主题覆盖

- 验证者: test-agent-17
- 被验对象: `b197394e5dcb2ee873728fe91db87cf1499939f1`（6 文件 +515/-4）
- 验证环境: 独立 worktree `../wt-v006`（detached @ b197394）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）/ `coverage_floor: 74`

## 结论：**PASS（verified）**

**最高风险项 A1 已独立实证**，viper 含点 key 缺陷与 `decodeTopics` 修复**均已独立复现**，
约束 C1（不动 `go.mod`/`go.sum`）与「`KeyDelimiter` 未改」均核实通过。
218 测试 **0 SKIP**、全仓 `go build`/`go vet` 通过、**0 个 FAIL 包**、`cmd/atlas` 覆盖率
**74.4%**（floor 74，未跌破）。

**一项报告给 Leader 决定的守护缺口**（不判 FAIL，理由见 §五）：
**serve 的接线无任何测试守护** —— 删掉 `serve.go:85` 的 `initPolicyGate(cfg, log)` 后
`cmd/atlas` 全绿。这与 A1 是**完全相同的失效形态**，只是后果较轻。

---

## 一、A1 实证复现 —— 与预期完全一致

把 `initPolicyGate` 从 `loadConfigOrDefaults` 内部挪到 export_ohlcv 调用点（编译通过）：

| 测试 | **预期**（注入前写下） | 实际 |
|---|---|---|
| `TestLoadConfigOrDefaultsInitsPolicyGate` | 红 | **红** ✓ |
| `TestInitPolicyGateAppliesCacheTTL` | 绿 | **绿** ✓ |
| `TestInitPolicyGateUsesConfiguredQuotaPath` | 绿 | **绿** ✓ |

红的输出精准指向根因：

```
policy_test.go:243: loadConfigOrDefaults 没有在内部初始化 Gate —— prism refresh 走的正是这条入口，
它拿到的会是懒构造的无账本 Gate，配额彻底失效: stat .../from-config-quota.json: no such file or directory
```

**「测试全绿本身就是症状」得到实测**：若没有那条装配测试，A1 变异下 `cmd/atlas` 会全绿。

### 判据选择正确

用「**账本文件是否落在配置路径**」而非「Gate 非 nil」——`Default()` 永不返回 nil，
非 nil 证明不了任何事。这与我验 TASK-005 的 O5 时同源（那次「每次新建实例」的变异
也是靠**单例断言**而非「非 nil」才抓到）。

### 三条入口逐一核实

| 入口 | 接线方式 | 顺序 |
|---|---|---|
| **prism refresh** | `prism.go:171` 调 `loadConfigOrDefaults` | `:191` 才构造 collector（`lixinger.New`）✅ |
| **export_ohlcv** | 经 `loadConfigOrDefaults` | ✅ |
| **serve** | `serve.go:85` **单独接线**（注释写明「serve 不经 loadConfigOrDefaults，故在此单独接线」） | 早于 `buildCollectors` ✅ |

三条入口的接线**都在**，但 serve 那条**无测试守护**（§五）。

---

## 二、viper 含点 key 缺陷：**独立复现属实**，修复有效

写对照探针（探针已删除，不在提交内），同一份 yaml 走两条路径：

```yaml
collector:
  topics:
    yahoo.chart:        {ttl: 30s, coalesce: false}
    tushare.daily_basic: {quota_limit: 3}
```

| 做法 | 结果 |
|---|---|
| **方案原版** `v.Unmarshal` → `map[string]TopicConfig` | `key="yahoo"` 全字段 nil、`key="tushare"` 全字段 nil —— **含点 key 被拆成嵌套层级，配置静默丢弃，装载不报错** ❌ |
| **`decodeTopics(v.Get("collector.topics"))`** | `key="yahoo.chart"` TTL=30s、Coalesce 已设；`key="tushare.daily_basic"` QuotaLimit 已设 ✅ |

**缺陷真实存在，修复确实修好了。**

### 未引入新问题（三项核实）

| 项 | 结果 |
|---|---|
| `KeyDelimiter` 是否被改 | `grep -rn 'KeyDelimiter' internal/config/` → **无匹配**，未改动 ✅ |
| `go.mod` / `go.sum`（约束 C1） | `git diff 3e75dc8 --stat -- go.mod go.sum` → **无输出** ✅ |
| `decodeTopics` 的守护 | C7（`Load` 丢弃其结果）→ **红** `config_test.go:705: yahoo.chart 未装载` ✅ |

### 教训 13 在本轮的实证

C7 在 `./internal/config/` 下**红**、在 `./cmd/atlas/` 下**绿**。这个「不符」是**合理的分层**：
`cmd/atlas` 的测试直接构造 `cfg.Collector.Topics`（不经 yaml 装载），config 层的丢配置它
结构上测不到。**正是这一点让方案的原有测试一直绿** —— 补一条「走真实入口」的测试
（`config_test.go` 经 yaml 装载）才是解药，Dev 做对了。

---

## 三、crisis 覆盖声明需要精确化（不构成 FAIL）

派验单称「crisis 两条路径也自动覆盖了」。**这个覆盖是有条件的，且条件恰好在生产形态下不成立。**

`resolveFREDKey`（`crisis.go:105`）的第一行是环境变量优先、**提前 return**：

```go
if k := os.Getenv(envName); k != "" {
    return k          // ← 直接返回，根本不调 loadConfigOrDefaults
}
cfg, err := loadConfigOrDefaults()
```

而注释明写「环境变量优先（**launchd/CI 可临时覆盖**）」—— launchd 正是生产形态。
实测探针：

| 情形 | Gate 是否按配置初始化 |
|---|---|
| 环境变量未设置 | ✅ 账本落在配置路径 |
| **环境变量已设置（launchd 形态）** | ❌ **账本未出现在配置路径** |

核实 `runCrisisBackfill` / `runCrisisEval` 全体：`loadConfigOrDefaults` **只经由
`resolveFREDKey`** 一处（`openCrisisStore` 用的是 crisis 专用 `ccfg`）。

**为什么不判 FAIL**：
1. DoD non_functional[0] 点名的是 **serve / export_ohlcv / prism refresh 三条入口**，crisis 不在其中；
2. crisis 走 FRED collector，而 `fred.series` 在策略表里是**未登记主题**（TASK-001 boundary[1]
   明确列出的六个之一）→ 零策略直通，Gate 是否初始化对它**当前无实际影响**。

**但这条声明应当修正**，否则将来 crisis 接入已登记的 collector 时会踩坑。

---

## 四、DoD 逐条覆盖矩阵（变异含**注入前写下的预期**）

| # | 完成标准（摘要） | 变异 | 预期 | 实际 | 判定 |
|---|---|---|---|---|---|
| functional[0] | config 新增指针语义字段；默认账本路径 | 旧 config 探针（§六） | — | 默认路径 `data/collector-quota.json` ✅ | **PASS** |
| functional[1] | `cache.ttl` 经 `ApplyTTL` 施加 | C1 不 `ApplyTTL` | 红 | **红** `policy_test.go:80: 过了配置的 50ms 后应重新调 fn... 仍为 1 说明用的是内置 5 分钟 TTL` ✓ | **PASS** |
| functional[2] | `collector.topics` 经 `Override` 生效（含配额） | C2 不应用覆盖 | 红 | **红** `:131: 配额上限被覆盖为 1，第二次应被拒` ✓ | **PASS** |
| functional[3] | `collector.quota.path` 用作账本路径 | C3 忽略配置路径 | 红 | **红** `:150: 账本未写到配置路径` ✓ | **PASS** |
| boundary[0] | `cache.enabled=false` 时 TTL 归零、限流仍生效 | C4 不 `DisableTTL` | 红 | **红** `:107: cache.enabled=false 时不得缓存` ✓ | **PASS** |
| boundary[1] | **旧 config.yaml 仍能装载且行为不变**（A12） | 探针实测（§六） | — | 既有字段完好、默认值正确 ✅ | **PASS** |
| error_handling[0] | nil logger 不 panic；账本不可写仍完成 `SetDefault` | C5 不兜底 warn | 红 | **红** `:160: initPolicyGate 不得 panic`（`defer recover` 写法正确） ✓ | **PASS** |
| **non_functional[0]（A1）** | `initPolicyGate` 在 `loadConfigOrDefaults` 内部，三条入口全覆盖 | A1 挪出 helper | 红 | **红** `:243` ✓（见 §一） | **PASS** |
| non_functional[1] | build/vet 通过；两包全绿；`cmd/atlas` 覆盖率 ≥ 74% | — | — | build ✅ vet ✅ 218 测试 0 SKIP ✅ **74.4%** ✅ | **PASS** |

**9 项全部 PASS。**

变异五道门全程执行 + 预期列：① 改动量非空且改语义；② `go test -c` 编译通过；
③ 核到断言行（区分 panic 堆栈）；④ `=== RUN` 数 > 0；⑤ 还原后 4 个文件 md5 一致。

---

## 五、报告项：**serve 的接线无测试守护**（不判 FAIL，请 Leader 决定）

**C6**（删掉 `serve.go:85` 的 `initPolicyGate(cfg, log)`）：**预期红，实际绿** ——
`cmd/atlas` 全部测试通过，无一转红。

`policy_test.go` 的 6 条测试没有一条覆盖 serve 这条路径。

### 这与 A1 是完全相同的失效形态

代码在 → 无守护 → 将来被删/被挪不会有任何测试转红。后果：serve 会用 `Default()`
懒构造的**内置表 + 无账本** Gate，`cache.ttl`、`topics` 覆盖、配额账本路径**全部退回默认**。

### 为什么我不判 FAIL

1. **DoD 的可执行断言要求已满足**：non_functional[0] 明确写「必须有可执行断言：cmd/atlas
   测试调用 `loadConfigOrDefaults()` 后断言 `policy.Default()` 使用的账本路径等于 cfg 中配置值」
   —— 这条断言存在且有效（§一）。DoD 未要求为 serve 单独立测试。
2. **补测试需要重构实现**：`runServe` 是单体函数（装载 → 校验 → 接线 → **启动 HTTP 服务**），
   直接调用会起服务器。要测它得先提取一个 helper —— 那是实现改动，不是「补一条测试」。
3. **后果比 A1 轻**：prism refresh 是「配额唯一真正生效的进程」（DoD 原话），那条已有守护；
   serve 是长驻进程，退回内置默认后缓存/限流仍在，只是配置不生效。

### 我的分类输入

属**守护缺口**（不是实现缺陷 —— 接线代码正确且在正确位置）。成本高于前几次（需重构
`runServe` 提取 helper），收益是消除一处与 A1 同类的静默失效面。**是否值得一次返工由 Leader 定**；
若不做，建议登记到 Sprint 末尾那批（与 `aDone` 增强、`Topics()` 注释措辞、跨进程探针一起）。

---

## 六、旧 config.yaml 兼容实测（反审 A12，「生产在用的就是那份」）

用不含 `collector.topics` / `collector.quota` 的旧配置装载：

```
Topics=map[] (未设置)  Quota.Path="data/collector-quota.json"  Cache.Enabled=true  Cache.TTL=5m0s
collectors.fred.api_key = "legacy-key"（既有字段完好）
```

装载不报错、既有字段未被破坏、新增字段取到默认值。**兼容性成立 ✅**

---

## 七、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| `cmd/atlas` 覆盖率 | **74.4%**（floor **74**，未跌破 ✅） |
| `internal/config` 覆盖率 | 83.3% |
| 测试 | `cmd/atlas` + `internal/config` 共 **218 个全 PASS，0 SKIP，0 FAIL** |
| `go build ./...` | 通过 ✅ |
| `go vet ./...` | 通过 ✅ |
| 全仓回归 | **0 个 FAIL 包 ✅** |
| scope | 6 文件全部落在 `./internal/config` + `./cmd/atlas` 声明内 ✅ |
| 约束 C1 | `go.mod` / `go.sum` **无变更** ✅ |

> **一处数据差异备录**：门禁报 `Coverage: 75.4%`，我实测 `go test ./cmd/atlas/ -cover` 为
> **74.4%**，差 1 个百分点。两者都高于 floor 74，不影响判定；差异可能来自门禁的统计口径
> （如是否计入其他包）。若 floor 将来上调到 75，这 1 个点的口径差需要先对齐。

---

## 八、方法论：预期列在本轮的两次不符

| 不符 | 诊断 | 结论 |
|---|---|---|
| **C6** 预期红实际绿 | serve 接线确实无测试守护 | **真缺口**（§五）—— 预期列直接定位到它 |
| **C7b** 预期红实际绿 | `cmd/atlas` 测试不经 yaml 装载，config 层缺陷它结构上测不到 | **我的预期错了**，分层是合理的 |

这轮两个方向各出现一次，正好印证了 TASK-005 我提的那条：**不符时两个方向都要诊断** ——
「预期红实际绿」既可能是真缺口（C6），也可能是我对系统的理解有偏差（C7b）。

关于「检验工具本身没有门可以检验」：本轮我的 A1 变异脚本、crisis 探针、legacy 探针都是
一次性写的代码，它们的缺陷同样不会被任何门捕获。**唯一的信号仍是「预期与实际不符」**
—— 本轮 C7b 正是靠它才没被记成缺陷。

---

## 九、复现命令

```bash
git worktree add --detach ../wt-v006 b197394e5dcb2ee873728fe91db87cf1499939f1
cd ../wt-v006
# 注意 worktree 内 go build 需加 -buildvcs=false

GOTOOLCHAIN=local go test ./cmd/atlas/ ./internal/config/ -count=1 -v | grep -c '^--- SKIP'  # 0
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -cover                                        # 74.4% ≥ 74

# A1 复现：把 export_ohlcv.go 的 `initPolicyGate(cfg, nil)` 从 loadConfigOrDefaults 内
#   挪到其调用点 → TestLoadConfigOrDefaultsInitsPolicyGate 红，另两条直调测试绿
# viper 缺陷复现：同一份含 `yahoo.chart:` 的 yaml，
#   v.Unmarshal 到 map[string]TopicConfig → key 变成 "yahoo"，全字段 nil
#   decodeTopics(v.Get("collector.topics")) → key 保持 "yahoo.chart"，字段正确
# crisis 条件性覆盖：设置 FRED key 环境变量后调 resolveFREDKey → 不触达 loadConfigOrDefaults
# C6（报告项）：删掉 serve.go:85 的 initPolicyGate → cmd/atlas 全绿

# 五道门 + 预期列：每个变异注入前先写下预期，跑完比对；不符必须诊断到根因再下结论。
```

worktree 已于验证结束后清理；主工作区零污染。
