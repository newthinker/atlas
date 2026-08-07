# TASK-001 验证报告 —— policy 包策略表与查表

- 验证者: test-agent-16
- 被验对象: `bac0de19cb1ff6eecfeaa173e3a1c3eb2b5bbde9`（`internal/collector/policy/policy.go` + `policy_test.go`，359 行新增）
- 验证环境: 独立 worktree `../wt-verify-TASK-001`（detached @ bac0de1），`.arcforge/` 读写在主仓库
- assignment_epoch: 1

## 结论：**NEEDS WORK（rejected）**

11 个测试确实全绿、覆盖率确实 95.3%（已独立复跑核实，Dev 自述属实）。但 **`error_handling[0]` 的
断言是空洞的**：变异验证证明把该 DoD 描述的行为改成它明确禁止的行为，套件仍然全绿。按
「每条 done_criteria 必须有真正验证其描述行为的测试」，这一条判 FAIL。

实现代码本身三处发现**全部正确**，需要补的只是测试。预计返工量 ~20 行测试 + 一处
可测性小重构。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 内置主题齐全；yahoo 500ms / tushare 200ms / twelvedata 8s；Domain 正确；登记主题 Coalesce 默认 true | `TestLookupBuiltinTopics`（TTL 由 `TestApplyTTLOnlyLiftsCachingTopics` 间接钉住） | M4(200→300ms) 转红 ✅；M13(yahoo.quote Coalesce→false) 转红 ✅；M14(builtinTTL→0) 转红 ✅；M15(domainOf 取错段) 转红 ✅ / **M11(lixinger.* Coalesce→false) 存活 ❌** | **PASS（带次要缺口）** |
| functional[1] | daily_basic 带 `Quota{5, 24h, Asia/Shanghai}`；daily/index_daily/hk_daily 的 Quota 为 nil | `TestDailyBasicQuota` / `TestOtherTushareTopicsHaveNoQuota` | M5(Limit 5→3) 转红 ✅；M6(Window 24h→1h) 转红 ✅；M7(Loc→UTC) 转红 ✅ | **PASS** |
| functional[2] | 三段查表（精确 → `<域>.*` → 未登记）；lixinger 端点命中通配且 TTL>0 / MinInterval=0 / Quota=nil / Domain=lixinger | 精确段 `TestLookupBuiltinTopics`；通配段 `TestLixingerWildcardTTLOnly`；未登记段 `TestLookupUnregisteredTopicIsZeroPolicy` | M3(删掉通配兜底) 转红 ✅ / **M16(把通配查在精确之前) 存活 ❌** | **PASS（带次要缺口）** |
| functional[3] | Set 时 Domain 为空则取主题名第一段；显式 Domain 保留 | `TestSetOverridesAndDefaultsDomain` | M8(无条件覆盖 Domain) 转红 ✅ | **PASS** |
| boundary[0] | ApplyTTL 只提升本就 TTL>0 的主题（yahoo.quote 保持 0）且 ttl<=0 为 no-op；DisableTTL 归零 TTL 但不动 MinInterval | `TestApplyTTLOnlyLiftsCachingTopics` / `TestApplyTTLNonPositiveIsNoop` / `TestDisableTTLKeepsThrottle` | M2(删掉 `if p.TTL > 0` 守卫) 转红 ✅；M9(DisableTTL 顺带清 MinInterval) 转红 ✅ | **PASS** |
| boundary[1] | eastmoney.kline / akshare.valuation / crypto.ticker / fred.series / edgar.facts / baostock.daily 一律 ok=false（约束 C6） | `TestLookupUnregisteredTopicIsZeroPolicy` | M12(凭空登记 `crypto.*`) 转红 ✅ | **PASS** |
| error_handling[0] | **shanghai() 在 LoadLocation 失败时退回 time.UTC 而非 panic**（tzdata 已嵌入） | `TestShanghaiLoadsEmbeddedTZData` | **M10(把 `return time.UTC` 改成 `panic(err)`) 存活 ❌ —— 套件仍 `ok`** | **FAIL（标准未覆盖）** |
| non_functional[0] | 不得 import internal/collector（约束 C3） | `TestNoImportOfCollectorRoot` | M1(注入 `_ "…/internal/collector"`) 转红 ✅（已独立复核 Dev 的反证） | **PASS** |

## 二、变异验证结果表

共 16 个变异，**13 捕获 / 3 存活**。全部还原后复跑全绿，`git status --porcelain` 与
`git diff --stat` 均为空（还原已核实）。

| ID | 变异内容 | 期望 | 实际 | 捕获者 |
|---|---|---|---|---|
| M1 | policy.go 加 `import _ ".../internal/collector"` | 红 | **红** ✅ | TestNoImportOfCollectorRoot |
| M2 | ApplyTTL 删掉 `if p.TTL > 0` 守卫 | 红 | **红** ✅（`yahoo.quote TTL got 1m30s`） | TestApplyTTLOnlyLiftsCachingTopics |
| M3 | Lookup 删掉 `<域>.*` 通配段 | 红 | **红** ✅ | TestLixingerWildcardTTLOnly |
| M4 | tushare 200ms → 300ms | 红 | **红** ✅（4 个主题同时报） | TestLookupBuiltinTopics |
| M5 | Quota.Limit 5 → 3 | 红 | **红** ✅ | TestDailyBasicQuota |
| M6 | Quota.Window 24h → 1h | 红 | **红** ✅ | TestDailyBasicQuota |
| M7 | Quota.Loc shanghai() → time.UTC | 红 | **红** ✅ | TestDailyBasicQuota |
| M8 | Set 无条件覆盖 Domain（丢弃显式值） | 红 | **红** ✅ | TestSetOverridesAndDefaultsDomain |
| M9 | DisableTTL 顺带清零 MinInterval | 红 | **红** ✅ | TestDisableTTLKeepsThrottle |
| **M10** | **shanghai() 失败分支 `return time.UTC` → `panic(err)`** | **红** | **绿 ❌ 存活** | 无 |
| **M11** | **`lixinger.*` 的 Coalesce true → false** | **红** | **绿 ❌ 存活** | 无 |
| M12 | NewTable 凭空登记 `crypto.*` | 红 | **红** ✅ | TestLookupUnregisteredTopicIsZeroPolicy |
| M13 | yahoo.quote 的 Coalesce true → false | 红 | **红** ✅ | TestLookupBuiltinTopics |
| M14 | `builtinTTL` 5m → 0 | 红 | **红** ✅ | TestLixingerWildcardTTLOnly + TestApplyTTLOnlyLiftsCachingTopics |
| M15 | domainOf 返回 `topic[i+1:]`（取错段） | 红 | **红** ✅ | TestLookupBuiltinTopics 等 3 个 |
| **M16** | **Lookup 把通配查在精确匹配之前（违反 DoD 指定次序）** | **红** | **绿 ❌ 存活** | 无 |

> M15 首版把 `domainOf` 整体改成 `return topic` 导致 `strings` 未使用而编译失败（无效变异），
> 已改为取后缀的等价降级后重做，结论以重做版为准。

## 三、覆盖率实测（独立复算）

```
ok  github.com/newthinker/atlas/internal/collector/policy  0.442s  coverage: 95.3% of statements
```

| 函数 | 覆盖率 |
|---|---|
| shanghai | **75.0%** ← 未覆盖的正是 M10 存活的那个失败分支 |
| NewTable / Set / Lookup / Topics / ApplyTTL / DisableTTL | 100.0% |
| domainOf | 66.7%（无 `.` 的主题名分支未覆盖，非 DoD 要求） |
| **total** | **95.3%** |

覆盖率数字与 Dev 自述一致。但注意：**95.3% 里缺的那 4.7% 恰好就是 error_handling[0]
唯一要求的行为**——覆盖率高不代表该条 DoD 被验证。

## 四、约束核查

| 约束 | 检查方式 | 结果 |
|---|---|---|
| C3 不循环导入 | `go list -deps ./internal/collector/policy \| grep 'internal/collector$'` | **无输出 ✅**；全量 deps 中本仓库包只有 policy 自身 |
| C5 限流数值只平移 | 与设计 §4.2 表格逐格比对 | **全部一致 ✅** yahoo.chart/eps 500ms、tushare.daily/index_daily/hk_daily/daily_basic 200ms、twelvedata.time_series 8s、daily_basic 5/自然日、lixinger 仅 TTL；yahoo.quote 按 §4.2「FetchQuote 类主题 TTL=0」且共享 yahoo 500ms 闸门 ✅ |
| C6 未登记 collector 零变更 | `TestLookupUnregisteredTopicIsZeroPolicy` + M12 反证 | **六个主题全部 ok=false ✅** |
| build / vet | 主仓库 `go build ./...` / `go vet ./internal/collector/...` | **均 exit 0 ✅**（worktree 内 build 报 `error obtaining VCS status: exit status 128`，属 worktree 的 buildvcs 戳记环境问题，加 `-buildvcs=false` 后 exit 0，非代码缺陷） |
| gofmt | `gofmt -l internal/collector/policy/` | 无输出 ✅ |
| 回归 | `go test ./internal/collector/... -count=1` | **17 个包全部 ok ✅**，无既有测试被破坏 |

## 五、scope 核查

`git show --stat bac0de1`：

```
 internal/collector/policy/policy.go      | 145 +++++++++++++++++++++
 internal/collector/policy/policy_test.go | 214 +++++++++++++++++++++++++++++++
 2 files changed, 359 insertions(+)
```

改动**严格落在** `packages`/`writes` 声明的 `./internal/collector/policy` 内，未触碰
`route.go` / `selector.go`（dev-agent-36 的 TASK-012 scope）。**无越界申报 ✅**

## 六、需要返工的项

### R1（阻塞项）error_handling[0] 无有效断言 —— 必须修

DoD 原文两个分句：
1. 「shanghai() 在 `time.LoadLocation` 失败时**退回 time.UTC 而非 panic**」
2. 「tzdata 经 `_ "time/tzdata"` 嵌入，不依赖部署机装 tzdata」

`TestShanghaiLoadsEmbeddedTZData` 把 `ZONEINFO` 指向不存在路径再断言拿到
`Asia/Shanghai`——这**很好地覆盖了分句 2**，但它证明的是「加载成功」，与分句 1 描述的
「加载失败时的行为」是互补而非重叠的两件事。M10 证明：把 `return time.UTC` 换成
`panic(err)`，这条测试照样绿。

失败分支之所以测不到，是因为 `shanghai()` 把时区名写死成字面量、错误路径由构造决定不可达。
建议做**最小可测性重构**（不改任何外部行为）：

```go
// 把时区名抽成参数，shanghai() 变成薄封装
func loadLoc(name string) *time.Location {
    loc, err := time.LoadLocation(name)
    if err != nil {
        return time.UTC
    }
    return loc
}

func shanghai() *time.Location { return loadLoc("Asia/Shanghai") }
```

补一条测试直接钉住 DoD 原句（顺带把 shanghai 的覆盖率补到 100%）：

```go
func TestLoadLocFallsBackToUTCWithoutPanic(t *testing.T) {
    defer func() {
        if r := recover(); r != nil {
            t.Fatalf("时区加载失败不得 panic: %v", r)
        }
    }()
    if got := loadLoc("Not/AZone"); got != time.UTC {
        t.Errorf("加载失败应退回 time.UTC, got %v", got)
    }
}
```

现有的 `TestShanghaiLoadsEmbeddedTZData` **请保留**，它覆盖分句 2，是有价值的。

### R2（次要）Lookup 的「精确优先于通配」次序未被钉住

M16 把两段查找对调后套件仍全绿。今天没有任何域同时存在精确条目与通配条目，所以行为等价；
但 DoD functional[2] 明确写了次序，且 TASK-002 的 Gate 与后续 config 覆盖一旦给某个已有
通配的域（如 lixinger）追加精确主题，次序错误会静默生效错策略。建议在
`TestLixingerWildcardTTLOnly` 里补三行：

```go
// 精确条目必须遮蔽同域通配条目（DoD 指定的查表次序）
tbl.Set("lixinger.exact", Policy{TTL: time.Minute, MinInterval: 3 * time.Second})
if p, _ := tbl.Lookup("lixinger.exact"); p.MinInterval != 3*time.Second {
    t.Errorf("精确匹配须优先于 <域>.* 通配, got %+v", p)
}
```

### R3（次要）`lixinger.*` 的 Coalesce 未断言

functional[0] 要求「**登记主题** Coalesce 默认 true」。表里共 9 个登记主题，
`TestLookupBuiltinTopics` 覆盖了 8 个，第 9 个 `lixinger.*` 的 Coalesce 无人断言
（M11 存活）。实现是 `true`，正确。在 `TestLixingerWildcardTTLOnly` 里加一行
`if !p.Coalesce { t.Error(...) }` 即可。

## 七、不作为 FAIL 依据的观察（备录）

- **task JSON 的 `discovery` 字段未回填**：`.arcforge/discoveries/TASK-001.json` 本身已正确
  落盘且内容充实（7 条 key_findings、6 条 decisions、10 条 interfaces_exposed，质量很高）。
  仅记录瑕疵，已按 Leader 指示不计入判定。
- **未跑 code-simplifier**：方案在 Task 12 Step 6 统一安排，Leader 已认可，不计入判定。
- **`Table` 非并发安全**（裸 map 无锁）：Dev 在 discovery 里已显式声明为设计意图（构造后只读）。
  本任务 DoD 无并发要求，不计入判定，但 **TASK-002 的 Gate 若需运行期改表必须自行加锁**。
- **`TestNoImportOfCollectorRoot` 在 `go list` 失败时走 `t.Skipf`**：本次执行确为 PASS 非 SKIP
  （已核对输出）。但该降级意味着在 go 工具链不可用的环境里 C3 守卫会静默失效，属可接受的
  防御性写法，仅备录。

## 八、复现命令

```bash
git worktree add --detach ../wt-verify-TASK-001 bac0de19cb1ff6eecfeaa173e3a1c3eb2b5bbde9
cd ../wt-verify-TASK-001
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -v
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t1.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t1.out
GOTOOLCHAIN=local go list -deps ./internal/collector/policy | grep 'internal/collector$'   # 应无输出
GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./internal/collector/...
# M10 复现（存活变异）：把 policy.go 中 shanghai() 的 `return time.UTC` 改成 `panic(err)`，
# 重跑上面第一条命令 —— 仍然全绿，即为 error_handling[0] 无有效断言的证据。
```
