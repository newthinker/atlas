# TASK-010 验证报告（test-pw-a）

- 任务：窗口统计（internal/pricewatch/window.go）
- 被测树：`cea96add97bc682ad8912facb43907e4b74661cc`（feature/pricewatch，= verify_baseline.head；隔离 worktree `../wt-verify-TASK-010`，detached）
- `window.go` / `window_test.go` 与 dev 提交 `5990203af84fc8baab865a220042955945d63a0a` 逐字节一致
- discovery sha256 `a2175000…f287` = verify_baseline.discovery_sha256，无漂移
- 工具链：go1.24.4 darwin/arm64，`GOTOOLCHAIN=local`

## 结论：VERIFIED（附非阻断发现，见 §6）

## 1. 门禁复跑

| 命令 | 结果 |
|---|---|
| `go test -race -count=1 ./internal/pricewatch/` | `ok`，rc=0；`TestWindowStats*` 顶层 8 个 PASS、0 FAIL |
| `go vet ./internal/pricewatch/` | 无输出 |
| `gofmt -l window.go window_test.go` | 无输出 |
| 覆盖率 | 包 93.4%（≥80）；WindowStatsFor 93.2%，其余函数 100% |

## 2. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 | 变异证据 |
|---|---|---|---|---|
| F1 | 上线日 100 天前 + 日线齐全，4y → Short、Days==100、Gap==0 | `TestWindowStats_Short`（两种上线日取法 + 上线日早于起点不判 Short + 按 bar 分键） | PASS | X1（不看 kv、只看本地首根）、X3、X9、X15、X27（秒/毫秒混淆）KILLED |
| F2 | 无上线日、5m 只有最近 5 天、7d → 非 Short、Gap≈2/7 | `TestWindowStats_HeadMissing` | PASS | X1、X10、X28 KILLED |
| F3 | 5m 中间缺 20% → 0.2；日线一段双根、一段缺 30% → 0.3 | `TestWindowStats_Gap`（fixture 自检 Count>21，证明按根数计算会掩盖缺口） | PASS | **X5（按根数算 GapRatio）KILLED**；X24 KILLED |
| F4 | 现价并入 High/Low；否则取 max(high)/min(low) | `TestWindowStats_PriceIncluded`（含窗口外极端值不计入） | PASS | X11–X14、X18 KILLED |
| B1 | all：非 Short、Days 为有效起点到 now；无 K 线 Gap==1 且非 NaN | `TestWindowStats_All`（上线日早于 / 晚于本地首根两种、缺今天那根、无 K 线、只有上线日没有 K 线） | PASS | X2、X6、X8（去掉 max(1,·) 后出 NaN）、X20、X21、X22 KILLED |
| N1 | race/vet/gofmt/覆盖率 | 第 1 节 | PASS | — |
| （附） | 非 UTC now（leader 重点） | `TestWindowStats_NonUTCNow`：UTC+8 的结果与 UTC 结果逐字段相等 | PASS | X28/X29（按 UTC 日界截断）KILLED |

## 3. 独立变异测试

- 手法：`go test -overlay`，变异体写在 scratchpad。worktree 源文件 sha256 前后一致，`git status --porcelain` 为 0 行。每个变异先过 `go vet -overlay`，0 个编译失败。
- 结果：29 个变异，KILLED 23，SURVIVED 6。
- 存活辨析：
  - X19（去掉 ORDER BY）：等价变异，与 dev 结论一致。主键 (pair,bar,ts) 的扫描序和 ORDER BY 相同；反向的 X20（DESC）被 KILLED，说明顺序确实受测试守卫。
  - X23（all 窗口去掉 `pts[0].ts.After(start)` 条件）：**完全等价**。查询已经用 `ts >= start` 过滤，`pts[0]` 恒 ≥ start，相等时赋值不改变结果。
  - X4（`listing.After(start)` 改成含等号）：只在上线日恰好等于 now−Span 的单点上不同，不是 DoD 要求。
  - X16（`ts <= now` 改成 `<`）、X17（去掉上界）：只影响 ts==now 或 ts>now 的 K 线。`TestWindowStats_CandleAtNow` 里 08:00 那根已经覆盖了末槽，所以排除 now 那根不改变结果。不是 DoD 要求，实际中 ts>now 的 K 线也不会出现。
  - **X25（上线日键写死 `:1d`，不按 bar 区分）：真存活。** 现有的「按 bar 分键」用例只测了一个方向：写入 5m 键，断言日线窗口不判 Short。另一个方向没测：5m 窗口（24h/7d）读取 5m 键后应判 Short。这不在 DoD 里，见 §6。

## 4. 范围与提交

- `git diff --stat 5990203^ 5990203` 正好是 writes 声明的两个文件；提交主题 `feat(TASK-010)` / `merge(TASK-010)` 符合约定。

## 5. leader 附问：dev_done 门禁是否认领到了 5990203

方法：门禁整体运行会写 `.arcforge/coverage`，所以没有整体复跑，而是把 `task-completed.sh` 第 173–262 行认领提交用的 git 查询照原样只读重放。门禁本身的完整输出没有重现。

- 时刻：`in_progress` 是 2026-10-20T07:47:12Z，`dev_done` 是 07:54:52Z（取自 transitions.jsonl）。合并 cea96ad 的提交时间是 07:54:06Z，早于 dev_done。下一个提交 bebd2ba（TASK-005）是 07:55:42Z，晚于 dev_done。所以 dev_done 时的 HEAD 是 cea96ad。
- **有界集合 COMMITTED_BOUNDED**（`--since 07:47:12Z`，漂移判定 C5 用它）：重放结果正好是 `internal/pricewatch/window.go`、`internal/pricewatch/window_test.go`，来自 5990203 与合并 cea96ad。**认领成功，门禁不是在空集上判定的。**
- `task/TASK-010` 分支 tip 是 5990203，已在 HEAD 祖先链上，branch-only 文件 0 条。
- 无界集合 COMMITTED_MINE 被**历史上同号 TASK-010 的提交污染**：认领到 2026-06-10 至 2026-09-19 之间往届 sprint 的 47 条提交（hestia/lixinger/crisis/config 等 30 多个文件）。今天的只有 2 条。按脚本注释，这个集合供 docs-only 判定 W1 使用；本任务不是 docs-only，漂移判定用的是有界集合，所以本次结论不受影响。但凡是消费无界集合的地方，这里都是同号碰撞。
- Jev 质量门日志（`.arcforge/jev/log/quality.jsonl`）里本任务的记录是 `"mode":"shadow","n_dod":0,"skipped":"no_judgeable_criteria"`，也就是 Jev 没有对本任务做任何判定。

## 6. 非阻断发现

1. X25：缺少「5m 窗口读取 5m 上线日键后判 Short」的用例。键格式是和 TASK-004（写入方）的接口契约，现在只有一个方向被测试守着。建议补一个用例：写入 `listing:BTCUSDT:5m` 为 3 天前，w=7d → Short==true。
2. 门禁的无界认领集合会跨 sprint 认领同号任务的提交（见 §5），属于机制观察，不归本任务。
3. 实现上的一个细节（不是缺陷）：all 窗口在上线日晚于本地最早一根时，有效起点取的是「上线日之后的第一根」，而不是上线日本身。和 AD-14 的「max(上线日, 本地最早一根)」最多差一个 bar。DoD 的 Days 断言因为向上取整，对此不敏感。
