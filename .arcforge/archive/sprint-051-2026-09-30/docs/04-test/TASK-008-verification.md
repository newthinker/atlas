# TASK-008 验证报告（Tiingo 集成冒烟与 prism 降级演练，integration 标签）

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ b3232d6f3e2b07610bafe31e7381add623fdb442（= verify_baseline.head；主仓库 HEAD 同值）
- 交付提交: f4d21239104d27ed456460f2c4348015d27d9f14，`--stat` 仅 writes 声明的两个 `_integration_test.go`（+68 行），无越界
- 运行环境: 隔离 worktree（detach @ b3232d6f3e2b07610bafe31e7381add623fdb442），GOTOOLCHAIN=local；本会话环境 `ATLAS_TIINGO_TOKEN` 未设置（已检查），且所有命令另加 `env -u ATLAS_TIINGO_TOKEN`。**未尝试获取 token，未发出任何真实 API 请求**（AD-9：真实运行是人类动作）。

## 覆盖矩阵

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | `go vet -tags integration` 两包无输出 | 无输出，rc=0 | PASS |
| functional[1] | 未设 token 时两测试 SKIP，包 ok | `-run 'Integration\|LiveDrill' -v`：`=== RUN` 仅两条（pattern 未误中其他测试），`--- SKIP: TestTiingoIntegration`、`--- SKIP: TestFetchClosesTiingoLiveDrill`，两包 ok | PASS |
| functional[2] | 断言内容覆盖（review） | 见下「断言审查」 | PASS（review） |
| boundary[0] | 不带 tag 时测试列表不含二者 | `go test -list '.*'`：不带 tag 两包命中 0 / 0，带 tag 命中 1 / 1（对照组证明判据会响）；不带 tag 跑 `-v` 两包 ok，输出中两测试名出现 0 次 | PASS |
| non_functional[0] | 源码与提交无 token 字面量（review） | 提交 diff：≥20 位 hex 串 0 处、`Token <字母数字>` 0 处、`os.Getenv("ATLAS_TIINGO_TOKEN")` 2 处；长度 ≥16 的其他字符串字面量只有 `drill: yahoo down` 等测试文本；提交信息 hex≥20 0 处 | PASS（review） |

## 断言审查（functional[2]，按源码逐条）

- `client_integration_test.go`：对 `AAPL`、`BRK.B` 各拉 `now-30d..now`；`require.NoError`、`require.NotEmpty`；逐根 `assert.False(math.IsNaN(b.Close) \|\| b.Close <= 0)`。与 DoD「近 30 天非空、每根 Close 非 NaN 且 >0」一一对应。BRK.B 经 client 内 toTicker 转为 BRK-B（TASK-003/004 已验）。
- `tiingo_integration_test.go`：`fakeUS2{failPrice: NVDA → "drill: yahoo down"}` 强制 yahoo 失败，唯一一跳 `PriceHop{Name:"tiingo", Client: tiingo.New(tok)}` 为真实客户端；`require.NoError`、`NotEmpty`、逐根 `Close > 0`；文案 `assert.Equal("NVDA: yahoo price failed (drill: yahoo down), tiingo fallback ok")`。这句逐字断言蕴含 DoD 要求的子串 `tiingo fallback ok`，且与 TASK-006 已验的 P17 单跳成功格式一致（fetchCloses 源码：`degMsg + ", " + h.Name + " fallback ok"`，单跳无前序失败段）。
- 局限（如实记录）：没有 token，这两个测试的断言路径**本次未执行**。无法做变异测试，因为 `tiingo.New` 固定指向生产端点，没有 token 就无法把请求导向可控的服务器。以上结论是对断言代码的静态审查，DoD 也把此条定为 review。真实运行仍待人类执行：`ATLAS_TIINGO_TOKEN=... go test -tags integration -run 'Integration|LiveDrill' -v ./internal/collector/tiingo/ ./internal/prism/`。

## 额外核对

1. **带 tag 整包与既有测试无冲突**：`env -u ATLAS_TIINGO_TOKEN go test -tags integration -race -count=1 ./internal/collector/tiingo/ ./internal/prism/` 两包 ok（无重名、无编译冲突）。gofmt -l 两个新文件无输出。
2. **prism 演练取的 Gate**：prism 包没有 TestMain，也没有调用 SetDefault 的包级 init。只有 quota_degrade_test.go 里的用例会临时调用 SetDefault，并在结束时还原；包内测试都没有 t.Parallel，因此演练测试开始时不会碰到被换掉的 Gate。`policy.Default()` 在 defaultGate 为 nil 时懒构造 `New(NewTable(), nil)`（policy/default.go:29-30）：
   - `NewTable()` 是纯内存的内置表，不读配置、不读文件；
   - QuotaStore 为 nil，`takeQuota` 在 `g.quota == nil` 时直接 `return nil`（gate.go:308）⇒ **完全不计配额、不落盘**。唯一会写文件的 `NewFileStore` 只在 cmd/atlas/policy.go:66 的生产装配中使用。
   ⇒ dev 的结论「不落盘、不碰生产账本」**成立**。但 discovery 里的措辞「仅内存计数」不准确：配额在这里是**完全跳过**，不是在内存中计数。仍在内存中生效的是 tiingo.daily 的节流与 TTL 缓存。这个措辞偏差不影响本任务的 DoD，只记录，不作为退回理由。
3. tiingo 包的冒烟测试使用既有 TestMain 装好的零策略 Gate（main_test.go），同样不计配额。

## 结论

VERIFIED。三条 test 类 DoD 实跑通过（vet、SKIP、tag 隔离，其中 tag 隔离带对照组）；两条 review 类按源码与提交 diff 核对属实。真实 API 路径未执行，属于 DoD 预期内的人类动作，已在上文注明。
