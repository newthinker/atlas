# TASK-001 验证报告（第二轮 / 返工复验）—— policy 包策略表与查表

- 验证者: test-agent-17
- 被验对象: `ce73488072ae3e5f67c7139c7ee2d9eaa6bd8a14` + `ebba14aa8468fc899ff086eddb6db68ff77c3bd8`（HEAD）
- 验证环境: 独立 worktree `../wt-verify-T001-r2`（detached @ ebba14a），`.arcforge/` 读写在主仓库
- assignment_epoch: 2 / rework_count: 1
- 首轮报告: `.arcforge/docs/04-test/TASK-001-verification.md`（test-agent-16，判 rejected）

## 结论：**NEEDS WORK（rejected）**

**Leader 指定的三项返工 R1/R2/R3 全部验证有效**（M10/M11/M16 三个首轮存活变异经我独立注入
后逐个转红），`ebba14a` 的 C3 守卫加固两条分支实测均有效，13 条旧变异抽查无回归，覆盖率
97.7%、`-race` 绿、collector 树 17 包全量回归绿、scope 无越界、可测性重构未改变外部行为。
**这一轮的活干得对。**

判 rejected 的唯一原因是我本轮**新造的 9 个变异中有 2 个存活**，且这 2 个恰好各自触及一条
DoD 明文写出的子句：

- **M21**：`NewTable` 里 `Loc: shanghai()` 改成 `Loc: loadLoc("Asia/Tokyo")` → **套件全绿**。
  functional[1] 明文要求 `Quota{..., Loc:Asia/Shanghai}`，但 `TestDailyBasicQuota` 的断言是
  `Loc == nil || Loc.String() == "UTC"`（「不是 UTC」），**没有钉住 Asia/Shanghai 这个值**。
- **M26**：`Topics()` 删掉填充循环、恒返回空切片 → **套件全绿**。boundary[0] 明文要求
  「DisableTTL 令 **Topics() 全部主题** TTL 归零」，而 `TestDisableTTLKeepsThrottle` 遍历
  `tbl.Topics()` 做断言，**循环体零次执行时断言 vacuously true**，无长度守卫。

两条均为**一行修复**，实现代码本身正确，与首轮 R2/R3 同性质、同量级。按首轮已确立的
「变异存活 ⇒ 该 DoD 无有效守护」标准，判定必须一致，故仍判 rejected。修复方案见 §六。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 本轮变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 内置主题齐全；yahoo 500ms / tushare 200ms / twelvedata 8s；Domain 正确；**登记主题 Coalesce 默认 true** | `TestLookupBuiltinTopics`（8 个主题）+ `TestLixingerWildcardTTLOnly`（第 9 个 `lixinger.*` 的 Coalesce，**本轮新增**） | M4(tushare 200→300ms) 转红 ✅；**M11(lixinger.* Coalesce→false) 转红 ✅ ← 首轮存活，R3 已修复**；M29(删掉 yahoo.eps 登记) 转红 ✅ | **PASS** |
| functional[1] | daily_basic 带 `Quota{5, 24h, **Asia/Shanghai**}`；daily/index_daily/hk_daily 的 Quota 为 nil | `TestDailyBasicQuota` / `TestOtherTushareTopicsHaveNoQuota` | M28(给 tushare.daily 凭空设配额) 转红 ✅；M17(shanghai() 返 Asia/Tokyo) 转红 ✅（捕获者是 `TestShanghaiLoadsEmbeddedTZData`，**不是** TestDailyBasicQuota） / **M21(绕开 shanghai()，NewTable 直接塞 Asia/Tokyo) 存活 ❌** | **FAIL（Loc 值未被钉住）** |
| functional[2] | 三段查表（精确 → `<域>.*` → 未登记）；lixinger 端点命中通配且 TTL>0 / MinInterval=0 / Quota=nil / Domain=lixinger | `TestLixingerWildcardTTLOnly`（**本轮新增精确遮蔽通配的次序断言**）/ `TestLookupUnregisteredTopicIsZeroPolicy` | M3(删通配兜底) 转红 ✅；**M16(通配段提到精确段之前) 转红 ✅ ← 首轮存活，R2 已修复**；M20(删掉精确段) 转红 ✅ | **PASS** |
| functional[3] | Set 时 Domain 为空则取主题名第一段；显式 Domain 保留 | `TestSetOverridesAndDefaultsDomain` | 沿用首轮 M8 结论（本轮实现未改动该路径） | **PASS** |
| boundary[0] | ApplyTTL 只提升本就 TTL>0 的主题（yahoo.quote 保持 0）且 ttl<=0 为 no-op；**DisableTTL 令 Topics() 全部主题 TTL 归零**而 MinInterval 不受影响 | `TestApplyTTLOnlyLiftsCachingTopics` / `TestApplyTTLNonPositiveIsNoop` / `TestDisableTTLKeepsThrottle` | M2(删 `if p.TTL > 0` 守卫) 转红 ✅；M27(DisableTTL 只清 yahoo.chart) 转红 ✅ / **M26(Topics() 恒返空切片) 存活 ❌** | **FAIL（全量断言可空转）** |
| boundary[1] | 六个未登记主题 Lookup 一律 ok=false（约束 C6） | `TestLookupUnregisteredTopicIsZeroPolicy` | 沿用首轮 M12 结论（本轮实现未改动该路径） | **PASS** |
| error_handling[0] 分句1 | **LoadLocation 失败时退回 time.UTC 而非 panic** | `TestLoadLocFallsBackToUTCWithoutPanic`（**本轮新增**） | **M10(`return time.UTC` → `panic(err)`) 转红 ✅ ← 首轮存活，R1 已修复**；M18(失败返回 nil) 转红 ✅ | **PASS** |
| error_handling[0] 分句2 | tzdata 经 `_ "time/tzdata"` 嵌入，不依赖部署机装 tzdata | `TestShanghaiLoadsEmbeddedTZData`（保留） | M17(shanghai() 返 Asia/Tokyo) 转红 ✅ | **PASS** |
| non_functional[0] | 不得 import internal/collector（约束 C3） | `TestNoImportOfCollectorRoot`（**ebba14a 加固**） | M1(注入 `_ ".../internal/collector"`) 转红 ✅；加固两分支实测有效（§三） | **PASS** |

**6.5 / 8.5 条 PASS**（error_handling[0] 按两个分句分别计）。两条 FAIL 均为断言精度问题，
实现无缺陷。

---

## 二、变异验证结果（本轮 15 个：13 捕获 / 2 存活）

全部在 worktree `../wt-verify-T001-r2` 内注入，逐个还原后 `md5` 与基线比对一致
（`policy.go = 89da8d213e62570dc8c90dee89d1ddd4`、`policy_test.go = 881fc68653f86a95a3551e52bd258477`），
`git status --porcelain` 为空。

### 2.1 三项返工目标的独立复现（Leader 指定的本轮重点）

| ID | 变异内容 | 期望 | 实际 | 捕获者 |
|---|---|---|---|---|
| **M10** | `loadLoc` 的 `return time.UTC` → `panic(err)` | 红 | **红 ✅** | `TestLoadLocFallsBackToUTCWithoutPanic`：`时区加载失败不得 panic: unknown time zone Not/AZone` |
| **M11** | `lixinger.*` 的 `Coalesce: true` → `false` | 红 | **红 ✅** | `TestLixingerWildcardTTLOnly`：`lixinger.* 也是登记主题，Coalesce 应为 true` |
| **M16** | `Lookup` 通配段提到精确段之前 | 红 | **红 ✅** | `TestLixingerWildcardTTLOnly`：`精确匹配须优先于 <域>.* 通配, got {Domain:lixinger TTL:5m0s MinInterval:0s ...}` |

**三个首轮存活变异现在全部转红，Dev 自报属实（已独立复跑核实，非采信）。**

### 2.2 旧变异防回归抽查（返工引入了 loadLoc 重构，验证未动摇原有断言）

| ID | 变异内容 | 期望 | 实际 | 捕获者 |
|---|---|---|---|---|
| M1 | policy.go 注入 `_ ".../internal/collector"` | 红 | **红 ✅** | `TestNoImportOfCollectorRoot` |
| M2 | `ApplyTTL` 删掉 `if p.TTL > 0` 守卫 | 红 | **红 ✅** | `TestApplyTTLOnlyLiftsCachingTopics`（`yahoo.quote ... got 1m30s`） |
| M3 | `Lookup` 删掉 `<域>.*` 通配段 | 红 | **红 ✅** | `TestLixingerWildcardTTLOnly` |
| M4 | tushare 200ms → 300ms | 红 | **红 ✅** | `TestLookupBuiltinTopics`（4 个主题同时报） |

四条全部仍被捕获，**无回归**。

### 2.3 本轮新造的补充变异（针对返工新增断言的强度）

| ID | 变异内容 | 期望 | 实际 | 捕获者 |
|---|---|---|---|---|
| M17 | `shanghai()` → `loadLoc("Asia/Tokyo")` | 红 | **红 ✅** | `TestShanghaiLoadsEmbeddedTZData` |
| M18 | `loadLoc` 失败分支 `return time.UTC` → `return nil` | 红 | **红 ✅** | `TestLoadLocFallsBackToUTCWithoutPanic` |
| M20 | `Lookup` 删掉精确匹配段（只剩通配） | 红 | **红 ✅** | `TestDisableTTLKeepsThrottle` |
| **M21** | **`NewTable` 里 `Loc: shanghai()` → `Loc: loadLoc("Asia/Tokyo")`（绕开 shanghai）** | **红** | **绿 ❌ 存活** | **无** |
| **M26** | **`Topics()` 删掉填充循环，恒返回空切片** | **红** | **绿 ❌ 存活** | **无** |
| M27 | `DisableTTL` 只清 `yahoo.chart` 的 TTL | 红 | **红 ✅** | `TestDisableTTLKeepsThrottle`（5 个主题同时报） |
| M28 | 给 `tushare.daily` 凭空设 `Quota{9, 1h, UTC}` | 红 | **红 ✅** | `TestOtherTushareTopicsHaveNoQuota` |
| M29 | `NewTable` 删掉 `t.Set("yahoo.eps", ...)` | 红 | **红 ✅** | `TestLookupBuiltinTopics`（`yahoo.eps: 应为内置主题`） |

> 过程记录：M26 首次执行时 `grep -c` 返回 0 匹配导致 shell `&&` 短路，变异未还原就叠加了
> M27，结果不可信；两条均已**单独干净重跑**，本表结论以重跑版为准。

---

## 三、`ebba14a` C3 守卫加固的有效性验证（Leader 指定，实测非代码审查）

`ebba14a` 把 `TestNoImportOfCollectorRoot` 的错误处理从「一律 `t.Skipf`」改为
「只在 `errors.Is(err, exec.ErrNotFound)` 时 Skip，其余 `t.Fatalf`」。**两条分支均已构造实测**：

方法：`go test -c` 先编译出测试二进制（脱离工具链状态），再分别制造两种失败。

| 场景 | 构造方式 | 期望 | 实测输出 | 结果 |
|---|---|---|---|---|
| **非 ErrNotFound 错误** | 把仓库根 `go.mod` 临时改成无效内容，在包目录跑测试二进制 | **FAIL** | `policy_test.go:243: go list -deps 失败，C3 约束无法验证: exit status 1` → `--- FAIL` | **转红 ✅** |
| **PATH 无 go** | `env PATH=/nonexistent-bin` 跑测试二进制 | SKIP | `policy_test.go:241: PATH 中没有 go，无法执行 C3 依赖约束检查: exec: "go": executable file not found in $PATH` → `--- SKIP` | **正确跳过 ✅** |

`go.mod` 已还原（`git status --porcelain` 为空）。**加固真实有效，非纸面改动**——它堵上了
首轮 §七备录的「C3 守卫在 go 工具链异常时静默失效」。

---

## 四、可测性重构未改变外部行为（Leader 指定验证点 4）

`shanghai()` 现在是 `loadLoc("Asia/Shanghai")` 的薄封装。用一次性探针测试直读 `NewTable()`
的实际产物：

```
Quota.Loc.String() = "Asia/Shanghai"
UTC offset = 28800 秒   (2026-01-15 参照日，即 UTC+8)
Loc == time.UTC ? false
```

**约束 C5 与 functional[1] 依赖的 Asia/Shanghai 自然日对齐语义未受重构影响 ✅。**
探针文件已删除（不在提交内）。

---

## 五、覆盖率、race、回归、scope 实测

### 5.1 覆盖率（独立复算，Dev 自报 97.7% 属实）

```
ok  github.com/newthinker/atlas/internal/collector/policy  0.483s  coverage: 97.7% of statements
```

| 函数 | 覆盖率 | 备注 |
|---|---|---|
| **loadLoc** | **100.0%** | 首轮 shanghai 的 75% 缺口已补齐 |
| **shanghai** | **100.0%** | |
| NewTable / Set / Lookup / Topics / ApplyTTL / DisableTTL | 100.0% | |
| domainOf | 66.7% | 无 `.` 的主题名分支，非 DoD 要求，**不计入判定**（与首轮一致） |
| **total** | **97.7%** | 95.3% → 97.7% |

### 5.2 `-race`

```
ok  github.com/newthinker/atlas/internal/collector/policy  1.456s
```

12 个测试全 PASS（`TestLookupBuiltinTopics` / `TestLookupUnregisteredTopicIsZeroPolicy` /
`TestDailyBasicQuota` / `TestOtherTushareTopicsHaveNoQuota` / `TestLixingerWildcardTTLOnly` /
`TestSetOverridesAndDefaultsDomain` / `TestApplyTTLOnlyLiftsCachingTopics` /
`TestApplyTTLNonPositiveIsNoop` / `TestDisableTTLKeepsThrottle` /
`TestLoadLocFallsBackToUTCWithoutPanic` / `TestShanghaiLoadsEmbeddedTZData` /
`TestNoImportOfCollectorRoot`），无 SKIP。

### 5.3 约束与回归

| 检查 | 命令 | 结果 |
|---|---|---|
| C3 不循环导入 | `go list -deps ./internal/collector/policy \| grep newthinker/atlas` | **仅输出 policy 自身 ✅** |
| gofmt | `gofmt -l internal/collector/policy/` | 无输出 ✅ |
| vet | `go vet ./internal/collector/...` | exit 0 ✅ |
| 全量回归 | `go test ./internal/collector/... -count=1` | **17 个包全部 ok ✅** |

### 5.4 scope 核查

```
ce73488  internal/collector/policy/policy.go      | 14 ++++---
         internal/collector/policy/policy_test.go | 43 +++++++++++++++---
ebba14a  internal/collector/policy/policy_test.go |  9 ++++-
```

两个提交**严格落在** `packages`/`writes` 声明的 `./internal/collector/policy` 内，未触碰
`route.go` / `selector.go` / `route_golden_test.go`（dev-agent-36 的 TASK-012 scope）。
**无越界申报 ✅**

---

## 六、需要返工的项（两条，各一行）

### R4 —— functional[1]：`Quota.Loc` 的值未被钉住（M21）

`TestDailyBasicQuota` 第 91-93 行：

```go
if p.Quota.Loc == nil || p.Quota.Loc.String() == "UTC" {
    t.Errorf("Loc = %v, want Asia/Shanghai", p.Quota.Loc)
}
```

断言写的是「不是 UTC」，DoD 要的是「**是** Asia/Shanghai」。首轮 M7（`shanghai()` → `time.UTC`）
之所以转红，只是因为它恰好落在这条弱断言唯一能覆盖的值上；换成任何其它非 UTC 时区即静默通过。

**真实影响**（不是纯洁癖）：`Quota` 的语义是「Window >= 24h 时按 `Loc` 的自然日对齐」。
tushare 的 5 次/天是**北京时间**自然日重置；`Loc` 错成 Tokyo 会让配额窗口边界错 1 小时，
TASK-002 的 Gate 据此记账就会在错误时刻重置，进而撞 40203 限频。

修复（一行，改成等值断言）：

```go
if p.Quota.Loc == nil || p.Quota.Loc.String() != "Asia/Shanghai" {
    t.Errorf("Loc = %v, want Asia/Shanghai", p.Quota.Loc)
}
```

### R5 —— boundary[0]：`DisableTTL` 的全量断言可空转（M26）

`TestDisableTTLKeepsThrottle` 遍历 `tbl.Topics()` 断言「所有 TTL 归零」。`Topics()` 一旦返回
空切片，循环体零次执行，断言 **vacuously true**——M26 证明整个套件照样绿。DoD boundary[0]
明文写的是「令 **Topics() 全部主题** TTL 归零」，「全部」这个量词今天没有下界。

修复（一行，加长度守卫）：

```go
topics := tbl.Topics()
if len(topics) != 9 {   // 内置 9 个登记主题；数量变了应当有意识地改这里
    t.Fatalf("Topics() 应返回全部 9 个登记主题, got %d: %v", len(topics), topics)
}
for _, topic := range topics {
    ...
}
```

> 顺带：`Topics()` 是 TASK-002 的 Gate 可能用来枚举主题的公开方法，它目前**没有任何直接
> 断言**——上面的长度守卫同时补上了这个空白。

---

## 七、给 Leader 的判定说明（重要）

1. **本轮返工本身没有失误。** R1/R2/R3 三项 Leader 指定的修复全部经我独立注入变异复现证实
   有效，`ebba14a` 加固实测有效，无回归。R4/R5 是我**本轮新造的变异**发现的、**首轮 16 个
   变异未曾探测到**的断言精度缺口，不属于 dev-agent-35 返工执行不到位。

2. **`rework_count` 将升至 2**（`max_rework` 默认 3）。若 Leader 认为为两行断言消耗一次返工
   额度不划算，这是 Leader 的调度权衡，我据实提供判定依据；从质量角度，R4 触及一条会影响
   TASK-002 配额记账正确性的 DoD 明文值，我不建议放行。

3. **给 TASK-002 (Gate) 的 Dev 的事项**（无论本任务如何放行）：
   - `Table` **非并发安全**（裸 map 无锁，构造后只读是设计意图）。Gate 若需运行期改表必须自行加锁。
   - `Lookup` 的次序契约现已被测试钉死：**精确 → `<域>.*` 通配 → 未登记**。给 lixinger 追加
     精确主题时精确条目会遮蔽通配，这是有意行为。
   - `Quota.Loc` 语义：`Window >= 24h` 按 `Loc` 自然日对齐（今天 00:00 起算），否则按 UTC
     截断到 Window 整数倍。daily_basic 的 `Loc` 实测为 `Asia/Shanghai`（UTC+8）。
   - `loadLoc(name)` 是包内私有函数，失败退回 `time.UTC` **不 panic**；Gate 若也要加载时区
     应复用它而非直接 `time.LoadLocation`。
   - 未登记主题 `Lookup` 返回 `(Policy{}, false)`，Gate 必须按「零策略直通」处理以保约束 C6。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-verify-T001-r2 ebba14aa8468fc899ff086eddb6db68ff77c3bd8
cd ../wt-verify-T001-r2

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -v
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t1.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t1.out
GOTOOLCHAIN=local go list -deps ./internal/collector/policy | grep 'newthinker/atlas'   # 应只有 policy 自身
GOTOOLCHAIN=local go test ./internal/collector/... -count=1

# M21 复现（存活变异）：把 policy.go 中 NewTable 的 `Loc: shanghai()` 改成
#   `Loc: loadLoc("Asia/Tokyo")`，重跑第一条命令 —— 仍然全绿。
# M26 复现（存活变异）：把 policy.go 中 Topics() 的 for 循环整段删掉（恒返回空切片），
#   重跑第一条命令 —— 仍然全绿。

# ebba14a 加固复现：
GOTOOLCHAIN=local go test -c -o /tmp/policy.test ./internal/collector/policy/
cp go.mod /tmp/go.mod.bak && printf 'this is not a valid go.mod\n' > go.mod
(cd internal/collector/policy && /tmp/policy.test -test.run TestNoImportOfCollectorRoot -test.v)  # 期望 FAIL
cp /tmp/go.mod.bak go.mod
(cd internal/collector/policy && env PATH=/nonexistent-bin /tmp/policy.test -test.run TestNoImportOfCollectorRoot -test.v)  # 期望 SKIP
```

worktree 已于验证结束后 `git worktree remove ../wt-verify-T001-r2` 清理。
