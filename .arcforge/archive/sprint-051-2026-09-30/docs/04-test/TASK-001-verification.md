# TASK-001 验证报告（Registry.GetAll 按注册顺序）

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90（= verify_baseline.head；主仓库 HEAD 同值，无漂移）
- 交付提交: e11918a9484dec4adf3f4654e1a6c198a6d1a9a9（`git show --stat`：仅 registry.go / registry_test.go，与 writes 声明一致，无越界）
- 运行环境: 隔离 worktree `git worktree add --detach <scratchpad>/test-tg-a-wt b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90`，GOTOOLCHAIN=local

## 覆盖矩阵

| # | done_criteria | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 七个 collector 注册 × 100 次 GetAll 逐元素等于注册序 | `TestRegistry_GetAllKeepsRegistrationOrder`（名字表与 DoD 七个逐字一致，reflect.DeepEqual 全序比较，循环 100 次） | PASS |
| functional[1] | 同名 a 重注册：长度 2、[0] 与新实例指针相等、[1]=="b"、Get("a") 返回新实例 | `TestRegistry_ReRegisterKeepsPosition`（`all[0] != replacement` 为指针比较；Get 亦比指针） | PASS |
| boundary[0] | 空 Registry GetAll 长度 0 不 panic | `TestRegistry_GetAllEmpty`（额外断言非 nil，严于 DoD） | PASS |
| non_functional[0] | 并发 Register（同名+不同名）与 GetAll，-race 无竞争，终态无重复且长度=不同名数 | `TestRegistry_ConcurrentRegisterGetAll`（8×2 goroutine，10 个名字各被注册 8 次）`go test -race` | PASS |
| non_functional[1] | 三包 go test ok；go vet 无输出 | `go test -count=1 ./internal/collector/ ./internal/app/ ./cmd/atlas/` → 三行 ok；`go vet ./internal/collector/` 空输出 rc=0 | PASS |
| non_functional[2] | discovery 记 impact 风险 + 5 个调用方改后行为（review） | 见下「调用方核对」 | PASS |

`go test -race -count=1 -run TestRegistry_ -v ./internal/collector/`：6 PASS / 0 FAIL；包覆盖率 100.0%。gofmt -l 空。

## 变异测试（隔离 worktree，脚本 scratchpad/test-tg-a-TASK-001-mut.py，每个变异 vet 均 rc=0 即语法合法；还原后 sha256 一致、git status 空）

| 变异 | 结果 | 致红测试 |
|---|---|---|
| M1 同名重注册也追加 order | KILLED | ReRegisterKeepsPosition, ConcurrentRegisterGetAll |
| M3 GetAll 回退为 map 迭代（原缺陷） | KILLED | GetAllKeepsRegistrationOrder |
| M4 空时返回 nil 切片 | KILLED | GetAllEmpty |
| M5 GetAll 去掉 RLock | KILLED（DATA RACE） | ConcurrentRegisterGetAll |
| M6 同名不覆盖实例 | KILLED | ReRegisterKeepsPosition |
| M7 GetAll 倒序 | KILLED | GetAllKeepsRegistrationOrder, ReRegisterKeepsPosition |

6/6 KILLED，每个 DoD 行为条目至少有一个独家或共同致红的变异；无存活变异。

## 调用方核对（逐一读基线树代码）

impact 风险：discovery 记 GetAll / Register 均 CRITICAL（含索引落后说明与 grep 补查），满足 AD-10 的记录要求。

1. `internal/app/app.go:556 orderedCollectors` — preferred 在前，其余按 `GetAll()` 顺序追加；preferred==nil 时直接返回 all。discovery 描述属实。
2. `internal/collector/selector.go:50 SelectExternalForSymbol` — 路由 collector、yahoo 都未注册时才走 `for _, c := range reg.GetAll()` 跳过 qlib 取第一个。描述属实。
3. `cmd/atlas/serve.go:324 buildArbitrator` — `if _, ok := marketCollectors[m]; !ok` 首个占位，即每市场取注册序中第一个支持者。描述属实。
4. `cmd/atlas/serve.go:171-173 backtest.New(collectors[0])` — collectors 来自 `GetCollectors()` = `GetAll()`，改后固定首个注册者。描述属实。
5. `internal/api/server.go:182-185` — 按 `GetCollectors()` 顺序 Register 进新 Registry，新 Registry 的 order 与原一致。描述属实。

补查 `.GetAll()` 全部非测试调用点（grep）：另有 app.go:649（只取 len，顺序无关）与 notifier/strategy 的同名方法（不同类型），discovery 已说明，无遗漏的依序调用方。

## 结论

VERIFIED。所有 test 类条目有对应测试且断言非空洞（变异 6/6 KILLED），回归三包 ok，review 条目逐一核对属实。
