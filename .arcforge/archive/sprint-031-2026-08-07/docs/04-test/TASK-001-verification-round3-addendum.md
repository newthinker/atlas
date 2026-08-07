# TASK-001 第三轮验证报告 · 增补（M26b 复核与 Topics() 断言链完备性）

- 验证者: test-agent-17
- 被验对象: `92b0be6027915c0a2cf3ec40845ad11f17d93322`（与主报告同）
- 验证环境: 独立 worktree `../wt-verify-T001-r3b`（detached @ 92b0be6）
- 本文件是 `TASK-001-verification-round3.md` 的增补，**不替代**它；主报告的 R4/R5 复核与
  13 条防回归抽查结论**全部不变**。

## 零、为什么有这份增补（时序如实说明）

我的第三轮判定 `verified` 落盘于 `2026-08-06T03:34:36Z`，**早于** Leader 派验单送达。
判定时我**不知道** Dev 自造的 M26b，也**没有**为 Dev 新加的「命中断言」造过变异——主报告
§1.1 只对它做了**代码审查**（认可其堵住了二阶空转路径），未做变异验证。

Leader 派验单要求补做两件事，本文件是其结论：

1. 独立复现 M26b（`Topics()` 返回 9 个不存在的主题名）；
2. 确认「`len == 9` + 每个都能 `Lookup` 命中」两条合起来，**是否等价于**「返回的正是那 9 个
   内置主题」。

**结论：M26b 确实被捕获（Dev 的自查完全正确）；但第 2 问的答案是「不等价，缝仍然存在」，
且缝比预想的深——存在一条能架空 boundary[0] 核心断言的组合逃逸路径。**

---

## 一、M26b 复核：Dev 是对的，Leader 的原方案确实不够

### 1.1 M26b 独立复现

| 变异 | 代码改动量 | 期望 | 实际 | 捕获者与实测输出 |
|---|---|---|---|---|
| **M26b** `Topics()` 返回 `["nope.a" … "nope.i"]`（9 个不存在的主题名） | 1+/5- | 红 | **红 ✅** | `TestDisableTTLKeepsThrottle` — `policy_test.go:198: nope.a: Topics() 返回的主题必须能被 Lookup 命中`（9 条同报） |

### 1.2 对照实验：证明命中断言是 load-bearing 的

把断言退回 Leader 的原方案（只保留 `len` 下界、去掉命中断言），`policy.go` 仍带 M26b：

| 断言状态 | 注入 M26b 的结果 |
|---|---|
| 只有 `len(topics) != 9` 下界（Leader 原方案） | **绿 —— 变异存活 ❌** |
| 加上「每个都能 `Lookup` 命中」（Dev 的加固） | **红 —— 被捕获 ✅** |

**Dev 的自查与闭环证明完全属实，已独立复现。** 它发现的问题是真的：`Lookup` 落空返回零值
`Policy`，其 `TTL` 恰好也是 0，所以纯 `len` 守卫下「9 个假名」能让 TTL 断言逐条通过。

---

## 二、Leader 追问的第 2 问：**不等价，缝仍在**

### 2.1 根因：`Lookup` 命中是一个比「已登记主题」更宽的谓词

`Lookup` 是**三段查表**（精确 → `<域>.*` 通配 → 未登记）。因此
「`topic` 能被 `Lookup` 命中」**不蕴含**「`topic` 是已登记主题」——任何 `lixinger.<随便什么>`
都会经通配段命中 `lixinger.*`。用一个宽谓词逐元素校验，钉不住窄集合。

再加上断言链没有约束**去重**与**覆盖完整性**，`len == 9` 只约束基数、不约束是哪 9 个。

### 2.2 两条已实测的独立逃逸路径

| 变异 | 内容 | 改动量 | 结果 | 说明 |
|---|---|---|---|---|
| **M26c** | `Topics()` 返回 `["lixinger.a" … "lixinger.i"]`（9 个 lixinger 域下的**假名**） | 1+/5- | **绿 ❌ 存活** | 全部经**通配段**命中；连「真实存在」都不需要 |
| **M26d** | `Topics()` 返回同一真实主题 `"yahoo.chart"` **重复 9 次** | 1+/5- | **绿 ❌ 存活** | 这正是 Leader 问的「**真实存在但不完整的子集**」 |

**所以：`len == 9` + `逐元素 Lookup 命中` ≠ 「返回的正是那 9 个内置主题」。**

### 2.3 严重性量化：组合逃逸可架空 boundary[0] 的核心断言

M26c 的危害不止「`Topics()` 坏了没人发现」。因为循环检查的是 `Topics()` 返回的那些名字，
一旦它们全是 lixinger 假名，**9 个真实内置主题一个都没被检查**——宿主断言（DisableTTL 令
全部主题 TTL 归零）就空转了。实测这个组合：

| 组合变异 | 内容 | 结果 |
|---|---|---|
| **M26c + M27** | `Topics()` 返 lixinger 假名 **且** `DisableTTL` 只清 `lixinger.*` | **绿 ❌ 全绿逃逸** |

此时 `yahoo.*` / `tushare.*` / `twelvedata.*` 的 TTL 全部保持 `5m0s`（`DisableTTL` 实际只清了
一个主题），测试完全没发现。**boundary[0] 的「DisableTTL 令全部主题 TTL 归零」这半条被完全架空。**

### 2.4 但要公允：单点故障仍然被守护

逃逸需要**两处协同改坏**。单独注入任一处，现行断言都能抓到：

| 单点变异 | 结果 |
|---|---|
| 只改 `DisableTTL`（M27，只清 `lixinger.*`） | **红 ✅** `yahoo.eps: cache.enabled=false 时所有 TTL 须归零, got 5m0s` |
| 只改 `Topics()`（M26 空切片 / M26b 假名） | **红 ✅**（见 §一） |

现实中「两处同时坏且恰好互补」的概率低。这是把它归类为**断言精度**而非**阻塞正确性**的
主要依据（另一个依据是：实现今天完全正确，`Topics()` 与 `DisableTTL` 都对）。

---

## 三、已验证的修复方案（不是纸上建议，已在 worktree 实测封堵全部路径）

问题的形态是：用「基数 + 逐元素宽谓词」去逼近「集合等值」。**直接做集合等值断言**即可
一步到位覆盖基数 / 重复 / 成员有效性 / 完整性四件事：

```go
// 对集合本身做等值断言，而不是「基数 + 逐元素谓词」的组合逼近。
want := map[string]bool{
    "yahoo.chart": true, "yahoo.eps": true, "yahoo.quote": true,
    "tushare.daily": true, "tushare.index_daily": true, "tushare.hk_daily": true,
    "tushare.daily_basic": true, "twelvedata.time_series": true, "lixinger.*": true,
}
topics := tbl.Topics()
got := map[string]bool{}
for _, topic := range topics {
    got[topic] = true
}
if len(topics) != len(want) || !reflect.DeepEqual(got, want) {   // len 比较兼管去重
    t.Fatalf("Topics() = %v (n=%d), want %v", topics, len(topics), want)
}
for _, topic := range topics {
    p, _ := tbl.Lookup(topic)      // 集合已等值，无需再断言命中
    ...
}
```

（需 `import "reflect"`；净效果是**替换**掉现有的 `len` 守卫 + 命中断言两段，代码量基本持平。）

实测封堵情况（在 worktree 里试装该方案后逐个注入，每次注入均先校验 `git diff --numstat` 非空）：

| 变异 | 现行断言 | 候选方案 |
|---|---|---|
| M26（空切片） | 红 ✅ | **红 ✅** |
| M26b（9 个不存在的名） | 红 ✅ | **红 ✅** |
| **M26c（9 个 lixinger 假名）** | **绿 ❌** | **红 ✅** |
| **M26d（同一主题重复 9 次）** | **绿 ❌** | **红 ✅** |
| **M26c + M27 组合** | **绿 ❌** | **红 ✅** |
| M27（单点，防回归） | 红 ✅ | **红 ✅** |

**候选方案封堵全部 5 条逃逸路径，且不削弱任何既有断言。** 该方案只在 worktree 内试装验证，
**未提交、未落入任何分支**；worktree 已还原至基线 md5 并拆除。

---

## 四、给 Leader 的分类输入（分类权在你，我只给依据）

**我的判断：这属于「断言精度」，不属于「阻塞正确性」。** 依据：

1. **实现今天完全正确**——`Topics()` 遍历 map 返回全部 key，无假名、无重复、无遗漏；
   `DisableTTL` 清全部主题。三轮下来实现只在二轮 `ce73488` 动过一次。
2. **所有单点故障仍被守护**：M26/M26b/M27 单独注入都转红。逃逸需两处协同改坏且恰好互补。
3. **不影响任何运行期行为**——纯测试侧断言强度问题，`policy.go` 一行不用改。

按 Leader 给的框架，这是「可放行、登记到 Sprint 末尾统一处理」的那一类。`rework_count=2`
不必为它用掉最后一次额度。

### 但有一件事我建议**不要**推迟：契约里那条新规则需要修正

Leader 刚把「集合类断言要同时约束**基数**与**成员有效性**」写进契约。本轮实测证明
**这两条加起来仍然不够**——M26c/M26d 正是同时满足「基数 = 9」与「每个成员都能被 `Lookup`
命中」却完全错误的集合。缺的是**去重**与**覆盖完整性**，而把这四件事分开列容易再漏第五件。

建议改写成一句可执行的：

> **集合类断言应当对集合本身做等值断言**（比对期望集合），而不是用「基数 + 逐元素谓词」
> 组合逼近。逐元素谓词尤其危险，当该谓词比目标集合更宽时（如本例 `Lookup` 因通配段而
> 宽于「已登记主题」），基数与成员校验可同时满足而集合完全错误。

这其实是我那条谓词判据的**集合版**，同一个道理：
`集合 == 期望集合` 是等值断言（强），`基数 + 逐元素宽谓词` 是排除式的组合逼近（弱）。
**这条同样肉眼可查**：看到「先 `len(x) != N` 再 `for range x` 逐项校验」的形状，就该问
「这个逐项谓词是否比目标集合更宽」。

后面还有 12 个任务会读契约，带着一条已被证伪的规则去做，会把这个缺陷复制到每个有集合
断言的地方——这个成本远大于 TASK-001 本身。

---

## 五、任务状态与处置权说明

TASK-001 当前为 **`verified`**（我于 `03:34:36Z` 落盘，早于派验单送达）。
写权限矩阵中 `verified` **没有面向 `test-*` 的出边**（`verifying→verified` / `verifying→rejected`
才是我的边），**我无法自行改判**。若 Leader 认为本文件的发现需要返工，合法路径是
Leader 执行 `verified → review_fix` 并附 `fix_items`。

主报告的所有结论（R4/R5 复核转红、13 条防回归全捕获、覆盖率 97.7%、17 包回归绿、
scope 无越界）**均不受本增补影响**，无需重验。

---

## 六、复现命令

```bash
git worktree add --detach ../wt-verify-T001-r3b 92b0be6027915c0a2cf3ec40845ad11f17d93322
cd ../wt-verify-T001-r3b/internal/collector/policy

# 把 Topics() 的实现整段替换为下列各行之一，每次注入后先 `git diff --numstat -- policy.go`
# 确认改动量非空（为空说明 perl 表达式静默失配，此时的绿是假绿），再跑 go test .
#   M26b: return []string{"nope.a","nope.b",…,"nope.i"}                      → 红（被捕获）
#   M26c: return []string{"lixinger.a","lixinger.b",…,"lixinger.i"}          → 绿（存活）
#   M26d: return []string{"yahoo.chart" ×9}                                  → 绿（存活）
# 组合逃逸：M26c 同时把 DisableTTL 改成只清 t.policies["lixinger.*"]          → 绿（全绿逃逸）
# 单点对照：只改 DisableTTL（不动 Topics）                                    → 红（仍被守护）

# 基线 md5（还原后须复原到）:
#   policy.go      = 89da8d213e62570dc8c90dee89d1ddd4
#   policy_test.go = 67d68a6c946d18f39ee2557c5503cb03
```

worktree `../wt-verify-T001-r3b` 已拆除；主工作区 `internal/` 全程零污染。
