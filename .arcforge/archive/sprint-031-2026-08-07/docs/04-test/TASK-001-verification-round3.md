# TASK-001 验证报告（第三轮 / R4-R5 复核）—— policy 包策略表与查表

- 验证者: test-agent-17
- 被验对象: `92b0be6027915c0a2cf3ec40845ad11f17d93322`（HEAD）
- 验证环境: 独立 worktree `../wt-verify-T001-r3`（detached @ 92b0be6），`.arcforge/` 读写在主仓库
- assignment_epoch: 3 / rework_count: 2
- 前序报告: 首轮 `TASK-001-verification.md`（test-agent-16，rejected）、
  二轮 `TASK-001-verification-round2.md`（test-agent-17，rejected）

## 结论：**PASS（verified）**

R4/R5 两条修复经**独立注入 M21/M26 复现，双双转红**；13 条防回归抽查（覆盖全部 8 条 DoD）
**全部仍捕获**；覆盖率 97.7%、`-race` 绿、12 测试全 PASS 无 SKIP、collector 树 17 包全量回归绿、
C3/gofmt/vet 干净、scope 无越界。

**`policy.go` 的 md5 与二轮基线逐字节相同**（`89da8d213e62570dc8c90dee89d1ddd4`）——
本轮是纯测试加固，实现零改动，不存在「修测试时顺手动了实现」的风险面。

### 本轮范围（Leader 2026-08-06 明确划定，已遵守）

只做两件事：① 复核 R4/R5；② 防回归抽查。**未新造探索性变异。**
理由（Leader 裁量）：DoD 只有 8 条、前两轮已跑 25 个变异，覆盖已充分；若每轮验证者都能造出
新变异则任务永不收敛，而 `rework_count` 已到 2（上限 3），这个结构性矛盾的代价不该由 Dev 承担。
**本轮复核过程中未偶然发现任何缺陷。** 判定的适用边界见 §六。

---

## 一、R4 / R5 修复复核（本轮核心）

### 1.1 修复形态审查（先看断言谓词，再造变异）

二轮我给 Leader 提过一条肉眼可查的判据：**看断言原文的谓词形式**——`!= 期望值` 是等值断言（强），
`== 某个错误值` 是排除断言（弱，只挡住枚举到的那几个值）。先用它审 diff：

| 项 | 二轮的弱写法 | 本轮的修复 | 形态判定 |
|---|---|---|---|
| **R4** | `if p.Quota.Loc == nil \|\| p.Quota.Loc.String() == "UTC"` （排除式） | `if p.Quota.Loc == nil \|\| p.Quota.Loc.String() != "Asia/Shanghai"` | **等值断言 ✅** 不是又一个更宽的排除式 |
| **R5** | `for _, topic := range tbl.Topics()` （全称量词无下界） | `topics := tbl.Topics(); if len(topics) != 9 { t.Fatalf(...) }` 再遍历 | **显式量词下界 ✅** |

**Dev 还多做了一步我没建议的加固**（`policy_test.go:193-199`）：循环内把 `p, _ := tbl.Lookup(topic)`
改成 `p, ok := ...` 并断言 `ok`。理由写在注释里——「`Topics()` 若返回不存在的主题名，`Lookup`
落空拿到零值 `Policy`，其 `TTL` 恰好也是 0，不断言命中的话下面那条 TTL 断言照样空转」。
这是我二轮没想到的**二阶空转路径**（长度对了但内容是垃圾），堵得比我提的方案更深。

### 1.2 变异复现（每次注入后先校验代码确已改动，再跑测试）

| ID | 变异内容 | 代码改动量（校验非空） | 期望 | 实际 | 捕获者与实测输出 |
|---|---|---|---|---|---|
| **M21** | `NewTable` 的 `Loc: shanghai()` → `Loc: loadLoc("Asia/Tokyo")`（绕开 `shanghai()`） | 1+/1- | 红 | **红 ✅** | `TestDailyBasicQuota` — `policy_test.go:95: Loc = Asia/Tokyo, want Asia/Shanghai` |
| **M26** | `Topics()` 删掉填充循环，恒返空切片 | 0+/3- | 红 | **红 ✅** | `TestDisableTTLKeepsThrottle` — `policy_test.go:191: 内置主题数 = 0, want 9` |

**二轮的两个存活变异现在都被捕获。R4/R5 修复有效。**

---

## 二、防回归抽查（13 条，覆盖全部 8 条 DoD，全部仍捕获）

抽查按「最可能失效的地方优先」编组：R4/R5 改动的正是 `TestDailyBasicQuota` 与
`TestDisableTTLKeepsThrottle`，所以先查这两个测试原本守护的东西。

### 组 A —— R5 改动的 `TestDisableTTLKeepsThrottle` 原有守护

| ID | 变异 | 改动量 | 结果 | 实测输出 |
|---|---|---|---|---|
| M27 | `DisableTTL` 只清 `yahoo.chart` 的 TTL | 3+/4- | **红 ✅** | `tushare.index_daily: cache.enabled=false 时所有 TTL 须归零, got 5m0s`（5 主题同报） |
| M9 | `DisableTTL` 顺带清零 `MinInterval` | 1+/0- | **红 ✅** | `限流不受缓存开关影响, got 0s` |

### 组 B —— R4 改动的 `TestDailyBasicQuota` 原有守护

| ID | 变异 | 改动量 | 结果 | 实测输出 |
|---|---|---|---|---|
| M5 | `Quota.Limit` 5 → 3 | 1+/1- | **红 ✅** | `Limit = 3, want 5` |
| M6 | `Quota.Window` 24h → 1h | 1+/1- | **红 ✅** | `Window = 1h0m0s, want 24h` |
| M7 | `Quota.Loc` `shanghai()` → `time.UTC`（首轮变异，改等值断言后应仍红） | 1+/1- | **红 ✅** | `Loc = UTC, want Asia/Shanghai` |

> M7 是二轮根因分析的原点：首轮它转红只因恰好落在弱断言唯一能覆盖的值上。改成等值断言后，
> 它**依然**转红——修复扩大了守护范围而没有丢掉原有能力。

### 组 C —— 二轮返工目标 R1/R2/R3 防回归

| ID | 变异 | 改动量 | 结果 | 实测输出 |
|---|---|---|---|---|
| M10 | `loadLoc` 的 `return time.UTC` → `panic(err)` | 1+/1- | **红 ✅** | `时区加载失败不得 panic: unknown time zone Not/AZone` |
| M11 | `lixinger.*` 的 `Coalesce` → false | 1+/1- | **红 ✅** | `lixinger.* 也是登记主题，Coalesce 应为 true` |
| M16 | `Lookup` 通配段提到精确段之前 | 2+/2- | **红 ✅** | `精确匹配须优先于 <域>.* 通配, got {…MinInterval:0s…}` |

### 组 D / E —— 其余代表性变异

| ID | 变异 | 改动量 | 结果 | 实测输出 |
|---|---|---|---|---|
| M2 | `ApplyTTL` 删掉 `if p.TTL > 0` 守卫 | 2+/4- | **红 ✅** | `yahoo.quote 是实时主题，TTL 必须保持 0, got 1m30s` |
| M3 | `Lookup` 删掉 `<域>.*` 通配段 | 0+/3- | **红 ✅** | `lixinger 端点应命中 lixinger.* 通配主题` |
| M4 | tushare 200ms → 300ms | 1+/1- | **红 ✅** | `tushare.daily: MinInterval = 300ms, want 200ms`（4 主题同报） |
| M12 | `NewTable` 凭空登记 `crypto.*` | 1+/0- | **红 ✅** | `crypto.ticker: 未登记主题不应命中策略表` **+ `内置主题数 = 10, want 9`** |
| M29 | `NewTable` 删掉 `yahoo.eps` 登记 | 0+/1- | **红 ✅** | `yahoo.eps: 应为内置主题` **+ `内置主题数 = 8, want 9`** |
| M1 | 注入 `_ ".../internal/collector"`（C3） | 2+/0- | **红 ✅** | `policy 不得依赖 collector 包（约束 C3 循环导入）` |
| M8 | `Set` 无条件覆盖 `Domain`，丢弃显式值 | 1+/3- | **红 ✅** | `显式 Domain 应被保留, got "custom"` |
| M17 | `shanghai()` → `loadLoc("Asia/Tokyo")` | 1+/1- | **红 ✅** | `嵌入 tzdata 后应拿到 Asia/Shanghai, got "Asia/Tokyo"` **+ `Loc = Asia/Tokyo, want Asia/Shanghai`** |

### 修复的正外部性（加粗项）

R4/R5 不只补上了自己那两条，还**顺带强化了三条既有变异的守护**：

- **M12 / M29** 现在多一个捕获者：`len(topics) != 9` 让「内置主题数量变化」有了独立信号，
  不再只依赖 `TestLookupBuiltinTopics` 的逐主题断言。
- **M17** 现在多一个捕获者：等值断言让 `TestDailyBasicQuota` 也能抓到时区错值；
  二轮时它只有 `TestShanghaiLoadsEmbeddedTZData` 一个守护者。

---

## 三、变异有效性纪律（本轮执行方式）

二轮 Leader 采纳的方法论——**注入后先确认代码确已改动，再解释红绿**——本轮以脚本形式机制化：
每个变异 `perl` 注入后先取 `git diff --numstat`，**改动量为空即判「变异无效（perl 表达式静默
失配）」并大声报错、跳过该条**，绝不把「没改代码的绿」记成「存活」；跑完还原后再用 `md5`
与基线比对，不符即中止后续。上表的「改动量」列就是这道校验的留痕，**15 条无一为空**。

这条纪律不是多余的：`shanghai()` 被重构成 `loadLoc(name)` 后注入点已变，照抄首轮报告 §八 的
表达式会静默失配（test-agent-16 首轮 M12/M15 栽过）。另有一个 shell 陷阱是我二轮自己栽的：
`grep -c pattern file && run` 在 0 匹配时 `grep` exit 1、`&&` 短路，`run`（含还原）整段不执行，
变异未还原就叠加了下一个——本轮 runner 改用 `git diff --numstat` 判定并把还原放在无条件路径上。

---

## 四、Done Criteria 逐条覆盖矩阵（8 条 / 9 项全 PASS）

| # | 完成标准（摘要） | 对应测试 | 本轮变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 内置主题齐全；yahoo 500ms / tushare 200ms / twelvedata 8s；Domain 正确；登记主题 Coalesce 默认 true | `TestLookupBuiltinTopics` + `TestLixingerWildcardTTLOnly`（第 9 个主题）+ `TestDisableTTLKeepsThrottle` 的数量下界（**本轮新增守护**） | M4 ✅ / M11 ✅ / M12 ✅ / M29 ✅ | **PASS** |
| functional[1] | daily_basic 带 `Quota{5, 24h, **Asia/Shanghai**}`；daily/index_daily/hk_daily 的 Quota 为 nil | `TestDailyBasicQuota`（**R4 改为等值断言**）/ `TestOtherTushareTopicsHaveNoQuota` | **M21 ✅（二轮存活，已修）** / M5 ✅ / M6 ✅ / M7 ✅ / M17 ✅ | **PASS** |
| functional[2] | 三段查表（精确 → `<域>.*` → 未登记）；lixinger 端点命中通配且 TTL>0 / MinInterval=0 / Quota=nil / Domain=lixinger | `TestLixingerWildcardTTLOnly` / `TestLookupUnregisteredTopicIsZeroPolicy` | M3 ✅ / M16 ✅ | **PASS** |
| functional[3] | Set 时 Domain 为空则取主题名第一段；显式 Domain 保留 | `TestSetOverridesAndDefaultsDomain` | M8 ✅ | **PASS** |
| boundary[0] | ApplyTTL 只提升本就 TTL>0 的主题且 ttl<=0 为 no-op；**DisableTTL 令 Topics() 全部主题 TTL 归零**而 MinInterval 不受影响 | `TestApplyTTLOnlyLiftsCachingTopics` / `TestApplyTTLNonPositiveIsNoop` / `TestDisableTTLKeepsThrottle`（**R5 补量词下界 + 命中断言**） | **M26 ✅（二轮存活，已修）** / M2 ✅ / M9 ✅ / M27 ✅ | **PASS** |
| boundary[1] | 六个未登记主题 Lookup 一律 ok=false（约束 C6） | `TestLookupUnregisteredTopicIsZeroPolicy` | M12 ✅ | **PASS** |
| error_handling[0] 分句1 | LoadLocation 失败时退回 `time.UTC` 而非 panic | `TestLoadLocFallsBackToUTCWithoutPanic` | M10 ✅ | **PASS** |
| error_handling[0] 分句2 | tzdata 经 `_ "time/tzdata"` 嵌入，不依赖部署机装 tzdata | `TestShanghaiLoadsEmbeddedTZData` | M17 ✅ | **PASS** |
| non_functional[0] | 不得 import internal/collector（约束 C3） | `TestNoImportOfCollectorRoot`（`ebba14a` 加固，二轮已实测两分支有效） | M1 ✅ | **PASS** |

**8 条 DoD / 9 项（error_handling[0] 按两分句计）全部 PASS，每条都有转红的变异作为守护证据。**

---

## 五、覆盖率、race、回归、约束、scope

### 5.1 覆盖率（独立复算）

```
ok  github.com/newthinker/atlas/internal/collector/policy  0.442s  coverage: 97.7% of statements
```

| 函数 | 覆盖率 |
|---|---|
| loadLoc / shanghai / NewTable / Set / Lookup / **Topics** / ApplyTTL / DisableTTL | **100.0%** |
| domainOf | 66.7%（无 `.` 的主题名分支，非 DoD 要求，**不计入判定**，与前两轮一致） |
| **total** | **97.7%**（与二轮持平；本轮是断言强度提升，不产生新语句） |

### 5.2 `-race` 与测试清单

```
ok  github.com/newthinker/atlas/internal/collector/policy  1.497s
```

12 个测试全 PASS、**无 SKIP**（`TestNoImportOfCollectorRoot` 实为 PASS 非 SKIP，已核对 `-v` 输出）。

### 5.3 约束与回归

| 检查 | 命令 | 结果 |
|---|---|---|
| C3 不循环导入 | `go list -deps ./internal/collector/policy \| grep newthinker/atlas` | **仅输出 policy 自身 ✅** |
| gofmt | `gofmt -l internal/collector/policy/` | 无输出 ✅ |
| vet | `go vet ./internal/collector/...` | exit 0 ✅ |
| 全量回归 | `go test ./internal/collector/... -count=1` | **17 个包全部 ok ✅** |

### 5.4 scope 核查

```
92b0be6  internal/collector/policy/policy_test.go | 23 ++++++++++++++++++++---
         1 file changed, 20 insertions(+), 3 deletions(-)
```

只动 `policy_test.go`，严格落在 `packages`/`writes` 声明的 `./internal/collector/policy` 内，
未触碰 `route.go` / `selector.go` / `route_golden_test.go`（TASK-012 scope）。**无越界申报 ✅**
`policy.go` md5 与二轮基线相同 → **实现零改动**，纯测试加固。

---

## 六、判定的适用边界（如实声明）

本轮按 Leader 划定的范围执行，**未新造探索性变异**。因此这份 PASS 的准确表述是：

> 8 条 DoD 各自都有**至少一个已验证能转红**的变异作为守护证据；三轮累计 28 个变异中，
> 目前**没有已知的存活变异**。

它**不**等价于「不存在任何未被探测的断言精度缺口」——变异测试没有终点，这一点在二轮 M21
（首轮 16 个变异「全绿通过」的条目上仍能造出存活变异）已经实证。停止搜索是 Leader 基于
「验证深度递增 vs 有限返工额度」的调度决定，不是「已证明无缺陷」的结论。记录于此以免
后续读者误读这份 PASS 的强度。

若后续 TASK-002 集成阶段暴露 policy 的行为问题，建议优先怀疑**断言精度**而非实现——
三轮下来实现代码只在二轮 `ce73488` 动过一次（`shanghai()` 抽出 `loadLoc`），其余全是测试侧加固。

---

## 七、给 TASK-002（Gate）Dev 的事项（沿二轮，无变化）

- `Table` **非并发安全**（裸 map 无锁，构造后只读是设计意图）。Gate 若需运行期改表必须自行加锁。
- `Lookup` 次序契约已被测试钉死：**精确 → `<域>.*` 通配 → 未登记**。给 lixinger 追加精确主题
  会遮蔽通配，这是有意行为。
- `Quota.Loc` 语义：`Window >= 24h` 按 `Loc` 自然日对齐（当地 00:00 起算），否则按 UTC 截断到
  Window 整数倍。daily_basic 的 `Loc` 现已被**等值断言**钉死为 `Asia/Shanghai`。
- `loadLoc(name)` 包内私有，失败退回 `time.UTC` 不 panic；Gate 若也要加载时区应复用它。
- 未登记主题 `Lookup` 返回 `(Policy{}, false)`，Gate 必须按「零策略直通」处理以保约束 C6。
- **新增**：`Topics()` 现在有了直接断言（数量 9 + 每项可被 `Lookup` 命中）。Gate 若依赖它枚举
  主题，注意内置主题数变化会触发 `TestDisableTTLKeepsThrottle` 的 `内置主题数 = N, want 9`
  ——这是有意的量词下界，新增内置主题时需**有意识地**同步改这个数字。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-verify-T001-r3 92b0be6027915c0a2cf3ec40845ad11f17d93322
cd ../wt-verify-T001-r3

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -v      # 12 PASS 无 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t3.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t3.out                        # 97.7%
GOTOOLCHAIN=local go list -deps ./internal/collector/policy | grep 'newthinker/atlas'  # 只有 policy 自身
GOTOOLCHAIN=local go test ./internal/collector/... -count=1              # 17 包全绿

# R4 复核（M21）：把 policy.go 中 NewTable 的 `Loc: shanghai()` 改成 `Loc: loadLoc("Asia/Tokyo")`
#   → 期望 TestDailyBasicQuota 转红 `Loc = Asia/Tokyo, want Asia/Shanghai`
# R5 复核（M26）：把 policy.go 中 Topics() 的 for 循环整段删掉（恒返空切片）
#   → 期望 TestDisableTTLKeepsThrottle 转红 `内置主题数 = 0, want 9`
# 注意：每次注入后先 `git diff --numstat -- <file>` 确认改动量非空，为空说明 perl 表达式
#       静默失配，此时的「绿」是假绿，不得记为存活。

# 基线 md5（还原后必须复原到这两个值）:
#   policy.go      = 89da8d213e62570dc8c90dee89d1ddd4   （与二轮相同 → 实现零改动）
#   policy_test.go = 67d68a6c946d18f39ee2557c5503cb03
```

worktree 已于验证结束后 `git worktree remove ../wt-verify-T001-r3` 清理；
主工作区 `internal/` 全程零污染（`git status --porcelain internal/` 为空）。
