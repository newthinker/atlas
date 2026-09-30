# TASK-002 验证报告（test-tg-b）

- 任务：policy 内置表登记 tiingo.daily（internal/collector/policy）
- 判定对象：`b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90`（= verify_baseline.head，= 验证时主仓库 HEAD），即 dev 提交 `720997cb0d53f429fd14f7ec40847d86dfa2bdad` 的 merge
- `git diff --stat 720997c b6c6a25 -- internal/collector/policy/` 为空；dev 提交只改 `policy.go`、`policy_test.go` 两个文件，都在 `writes` 内，无越界
- discovery sha256 = `094be471…f8af`，与 verify_baseline 一致
- 环境：隔离 detached worktree（测试一个、变异另一个，另外建了一个父提交 `8d1c6cc7edf573c9879b53125f486eb6d515f01f` 的基线树），`GOTOOLCHAIN=local`

## 结论：VERIFIED

## done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 守卫证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | Lookup 命中；Domain tiingo、Quota 40/1h、MinInterval 0、Coalesce、TTL=builtinTTL | TestBuiltinTiingoDailyQuota | Limit 41/39、Window 24h、无 Quota、Coalesce false、TTL 0、MinInterval 1ms、Domain 固定为 twelvedata：全部被这条杀 | PASS |
| functional[1] | Topics 集合等值的 want 含 tiingo.daily；计数注释与实际一致 | TestDisableTTLKeepsThrottle | 从 want 删掉 tiingo（M13）、主题名拼错 / 删行（M8/M9）被杀。计数注释人工核对：`t.Set("` 共 13 处（policy.go:70–107），tiingo.daily 排第 9、lixinger.* 排第 10，TestLookupBuiltinTopics 覆盖的是前 8 个；与注释「13 个」「第 9 个」「第 10 个」「前 8 个」一致 | PASS |
| boundary[0] | 内置策略 + NewMemStore：第 41 次（不同 key）返回 ErrQuotaExceeded，fn 计数 = 40 | TestTiingoDailyQuotaBlocksFortyFirst | Limit 41、39、无 Quota 都被这条独立杀掉（不只靠 functional[0]）；跨整点有重跑处理 | PASS |
| boundary[1] | ApplyTTL 提升 TTL，Quota 保持 40/1h | TestTiingoDailyApplyTTLKeepsQuota | M10 ApplyTTL 丢 Quota、M12 ApplyTTL 不提升 被杀 | PASS |
| boundary[2] | ApplyTTL(5m)→Override(TTL 6h)：TTL 6h 且 Quota 40/1h | TestTiingoDailyOverrideTTLKeepsQuota | M15 Override 给 TTL 时丢 Quota：**只被这条杀** | PASS |
| non_functional[0] | test ok；vet 无输出；覆盖率 ≥ 94.4%（-func） | 实跑 | — | PASS |

## 证据

```
@ b6c6a25  go test -count=1 -cover ./internal/collector/policy/  → ok  coverage 94.4%
           go tool cover -func total                             → 94.4%
@ 8d1c6cc（父提交基线）go tool cover -func total                → 94.4%
go vet → rc=0 无输出；gofmt -l → 空；go test -race → ok
-v：TestDisableTTLKeepsThrottle 与 4 个 Tiingo 测试都 PASS
```

### 独立变异（隔离 worktree；脚本 scratchpad/test-tg-b-TASK-002-mut.py；每个变异先 vet 确认可编译，匹配次数 ==1，收尾源文件还原，porcelain 为空）

**14/15 KILLED**。dev 自报的是 5/5，我额外加了 Limit 39、无 Quota、主题名拼错、删行、Domain、ApplyTTL/Override 丢 Quota、ApplyTTL 不提升、want 删项这几个。

存活 1 个，是等价变异：
- **M11 把 Override 的 `if o.QuotaLimit != nil || o.QuotaWindow != nil` 改成 `if true`**：tiingo.daily 的 Quota 非 nil，`q = *p.Quota` 复制之后什么都不改，结果仍是 40/1h，所以对本任务是等价的。只有 Quota 为 nil 的主题才会表现出差别（会被凭空塞进 0/24h 配额），但那是既有代码、在本任务 DoD 之外。本任务真正要守的行为「只给 TTL 时保留配额」由 M15 验证：被 TestTiingoDailyOverrideTTLKeepsQuota 独家杀掉。

## 对 dev 决定的评议
- boundary[1] 用 30m 而不是 DoD 举例的 5m：DoD 写的是「如 5m」，只是举例；builtinTTL 本身就是 5m，用 5m 观察不到「被提升」。改用 30m 是对的，M12 被杀就是证据。
- 覆盖率 94.4% 等于基线（dev 自报 block 口径 255/270 对 254/269，我这边只核了 -func 的 total，两个都是 94.4%），满足「不低于」。

## 补充：Leader 追加派验的三个关注点
1. **ApplyTTL 用 30m 替代 5m**：boundary[1] 的本意是「全局 TTL 作用后 TTL 被提升，同时 Quota 不变」。builtinTTL 本身就是 5m，用 5m 无法观察提升，用 30m 才满足本意。证据：M12（ApplyTTL 不提升）被这条测试杀，M10（ApplyTTL 丢 Quota）也被它杀。同时 boundary[2] 仍按 DoD 字面用 ApplyTTL(5m)→Override(6h)。
2. **跨整点「重跑一轮」会不会掩盖真失败**：不会。检查过这段实现：
   - 前 40 次里任何一次被拒，都会立即 Fatal，与是否跨窗口无关。
   - 跳过一轮的唯一条件是这 41 次调用前后 `windowStart(now)` 变了，而这只由时钟决定，被测代码影响不了。
   - 只要某一轮没跨窗口，就会完整断言 ErrQuotaExceeded 和 fn==40。两轮都跨窗口时直接 Fatal，不会静默通过。
   - 实证：Limit 41/39 和无 Quota 三个变异都被这条测试独立杀掉。
3. **code-simplifier 内联 helper**：我验证的是 b6c6a25 上的最终测试代码，与 720997c 逐字节一致。被内联的 `mustTiingoQuota` 已不在代码里，现存的 `assertTiingoQuota` 由 M10/M15 证明确实在守卫。判定依据是最终代码加变异结果，与简化过程无关。
- 门禁输出里的往届同名 TASK-002 提交：本判定没有使用门禁输出，全部证据来自在基线树上的实跑。
