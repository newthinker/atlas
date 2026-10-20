# TASK-004 验证报告（QA 返工复验，test-pw-b）

- 任务：K 线分段补数（internal/pricewatch/sync.go）；`assignment_epoch`=1，`rework_count`=1（review_fix 第 1 轮，reason_class=task_defect）
- **被测树**：`feature/pricewatch` @ `ebc366e43a6deda8dc28ae8974b2e3fa77bb3c1b`（= `verify_baseline.head`；出裁决前主仓库 HEAD 未变），独立 worktree `../wt-verify-TASK-004-b`（detached），主工作区未改动
- 文件 sha256：`sync.go` `a9fa25181b030a39107fba667b130fe95fed24513bccc0cfdef5e5124f58c001`、`sync_test.go` `2b6acaa12521635629137874656d1a7534f5ba508ce01e6360ebef56a5aa4e2c`（= 返工提交 `626a8504a97abf5ab2fda4656c52849a104cd9b7`，合入为 `f8e37c2`，与 discovery `qa_fix.verification` 所记一致；f8e37c2..ebc366e 期间两文件无变化）
- discovery sha256 `39fab782d213d02844e7ac37f940e4e0e06abf0b741f4d552a0fa96575e5b082` = 基线，无漂移
- 范围：`626a850 fix(TASK-004)` 只改 `writes` 的两个文件（+218/−39）
- 派验通知：没有收到这次的派验消息。我是在重扫任务文件时发现被指派的，按文件为准开工，并已告知 Leader
- 工具链：go1.24.4 darwin/arm64，`GOTOOLCHAIN=local`

## 结论：VERIFIED

## 0. 上一轮与本轮对照

| | 上一轮（test-pw-a @ 80a56f3，首验） | QA 第 2 轮（review_fix） | 本轮（test-pw-b @ ebc366e） |
|---|---|---|---|
| 结论 | VERIFIED（附非阻断发现） | 退回 review_fix：QA-8、QA-9/AD-29、R1-5、可选 F2 | **VERIFIED** |
| QA-8 上线日保护 | — | S29/S30 无守卫，另需「跳过的完整段让判定停下」 | 4 个新用例；Q1/Q3/Q4 KILLED（§3） |
| QA-9/AD-29 近期段 100% | — | 人类已裁决，代码未落地 | `requiredCoverage` 落地；A1–A6 全部 KILLED |
| R1-5 主回填源故障上报 | — | binance 挂掉对 `sync:` 健康不可见；okx 替身没有 1440 上限 | fetch 有错就上抛、best 照常入库；替身加上 `okxHorizon=1440`；R1–R5、R3b KILLED |
| F2 复用 readListing | — | 可选 | 已做；F1 KILLED |

## 1. 门禁重跑

| 命令 | 结果 |
|---|---|
| `go test -race -count=1 -coverprofile ./internal/pricewatch/` | `ok`（29.1s），包覆盖率 **94.1%** |
| `go vet ./internal/pricewatch/` | 无输出，rc=0 |
| `gofmt -l sync.go sync_test.go` | 无输出 |
| `-count=3 -run 'TestSync\|TestValidCandle' -v` | 顶层 19 个 × 3 = **57 PASS**、0 FAIL |
| sync.go 函数覆盖 | fetch / requiredCoverage / validCandle / slotsHit 100%；Sync 86.2%；backfillStart 88.9%；localCoverage 75%、slotCount 66.7%、earliest 80%（未覆盖的是 SQL 错误、空区间等防御分支） |

## 2. done_criteria 覆盖矩阵（整份重新验）

| # | 完成标准 | 对应测试 | 守卫证据（本轮变异） | 判定 |
|---|---|---|---|---|
| functional[0] | 本地为空，okx/binance 两种语义，入库根数 = fake 总根数，段边界不缺根 | `TestSync_Chunks`（okx 用例改到 1440 上限内，today−1400 天起） | D3（起点不减 1ms）KILLED | PASS |
| functional[1] | 老段由 B 补、新段用 A；A 报错换 B；全不足取最多 | `TestSync_FallbackPerChunk` | D4（首源即用）、D5（全不足取最少）、D2 KILLED | PASS |
| functional[2] | 最新一根 high 100→120 刷新，两种语义 | `TestSync_Incremental` | D8（不强制重取含最新一根的段）、D3 KILLED；D6（tail=latest）等价存活（与上一轮 S1 相同：请求起点减 1ms 已包含 T） | PASS |
| functional[3] | 稳态只发 1 个请求、只调第一个 provider | `TestSync_SteadyState` | D1（不跳过完整段）、D7 KILLED | PASS |
| functional[4] | 中间缺一整段只重取该段（AD-22） | `TestSync_RepairsGap`、`_RepairsGapUnderMixedAlignment` | D1、D9（覆盖率按根数算）、Q8 KILLED | PASS |
| functional[5] | 上线日写 kv；第二次不请求早于上线日的段（AD-21） | `TestSync_Listing`、`_NoListingWhenNoLeadingEmpty` + 4 个 QA-8 新用例 | Q5b、Q6、Q7、F1 KILLED | PASS |
| boundary[0] | 坏 K 线丢弃，同批其余入库（AD-20b） | `TestValidCandle`、`TestSync_DropsBadCandles` | D12–D16 KILLED | PASS |
| non_functional[0] | race/vet/gofmt/覆盖率；Prune5m；全源报错返回含 pair/bar 的 error，已成功段仍在 | §1；`TestSync_Prune5m`（改用 binance 语义，8 天 5m=2304 根 > okx 上限）、`TestSync_AllSourcesError`、`TestSync_Errors` | D10、D11、R4、R5、D17、D18 KILLED | PASS |

## 3. fix_items 逐项验证

### 3.1 QA-8：上线日保护

- 实现：Sync 对尾段之前的每一段先算本地覆盖率，`cov > 0 ⇒ leadingEmpty = false`，这一处同时覆盖「完整段被跳过」和「含最新一根的段被强制重取后得 0 根」两条路径。fetchSeg 的报错分支 `leadingEmpty = false` 保留。
- 用例：`TestSync_ListingNotRecordedOnError` 与 `_AllError` 原样收录了 qa-8-probe.md 的两个探针（我逐字比对过）。新增 `_OnPartialWithError`（A 只有部分数据、B 报错时不记上线日，A 的数据照常入库）、`_StoppedBySkippedSegment`（两个子例，外加「本地为空时会记」的对照，排除「本来就不会记」这种空断言）。
- 变异：
  - Q1 = S29（删掉报错分支的 `leadingEmpty=false`）**KILLED**，被 4 个用例抓到
  - Q4 = S36（`cov>0` 改为恒假）**KILLED**，被 `_StoppedBySkippedSegment` 抓到
  - Q3 = S29+S30 组合 **KILLED**
  - **Q2 = S30（有错时 `allZero = bestSlots==0`）单独存活**。我同意 dev 的等价判断：R1-5 之后，有错就一定先走报错分支把 `leadingEmpty` 置 false，allZero 只影响「空切片要不要入库」。唯一的差别出在 cs 非空但全部落在段外（slotsHit=0）：此时变异体不入库，原实现入库。这些 K 线不在本段，下一段会再取到，实际等价。

### 3.2 QA-9 / AD-29：与最短窗口相交的段要求 100%

- 实现：`requiredCoverage(bar, to, now)`：`to.After(now − shortest.Span())`（1d→Window30d，5m→Window24h）时返回 1，否则返回 0.9。判定用的是段的 `to`，正确表达了「相交」：只要段的终点晚于窗口起点，两者就有交集。与 AD-29 人类裁决的文字一致。
- 用例 `TestSync_RecentSegmentsNeedFullCoverage`：1d 子例把段边界构造在 now−15d，删掉 now−20d（段与 30d 窗口相交）与 now−400d 各一根，前者补回、后者不补；5m 子例删掉 now−2h 与 now−3d 各一根，结果同理。
- 变异：A1（近期仍 90%）、A2（所有段 100%）、A3（1d 窗口改 7d）、A4（5m 窗口改 30d）、A5（相交判定改用 from，即整段在窗口内才 100%）、A6（5m 不区分）**全部 KILLED**。

### 3.3 R1-5：主回填源故障上报

- 实现：`fetch` 不再用 `anyOK` 吞错，只要有源报错就返回 `best, false, errors.Join(errs...)`。Sync 的报错分支不再 return，非空的 best 照常入库，最后汇总成 `sync <pair>/<bar>: …` 返回。替身 `okxSem` 只取 data 最后 `okxHorizon=1440` 根。
- 用例 `TestSync_BinanceDownReportsError`：okx（3000 天数据、1440 上限）+ binance 一律报错。断言错误含 BTCUSDT、1d、binance；入库根数 == 1440；不记上线日。
- 变异：R1（旧 anyOK 语义：有部分结果就不上抛）、R2（报错时不入库 best）、R3b（报错即中止整个 Sync）、R4（最终不返回错误）、R5（错误不含 pair/bar）**全部 KILLED**。
- **R6（错误里去掉源名）存活**：用例断言 `strings.Contains(err, "binance")`，但替身自己的错误文本就是 `"binance boom"`，所以即使 fetch 不带 `p.Name()`，源名仍然会出现。这是 QA-4 那种形态：断言被另一条路径顺带满足。DoD 只要求 pair 和 bar，不构成缺陷。真实 provider 的错误文本未必带源名，建议把替身的错误改成不含名字（例如 `errors.New("boom")`）。

### 3.4 F2（可选）：backfillStart 复用 readListing

已落地（sync.go:155）。F1（不读上线日）KILLED。

## 4. 独立变异汇总（39 个，`go test -overlay`，隔离 worktree）

- 首轮 39 个，其中 2 个编译失败（Q5 变量未使用、R3 出现 unreachable code），修正为 Q5b、R3b 后重跑，均 KILLED。每个变异先过 `go vet -overlay`。源文件 sha256 跑前跑后一致；worktree `git status --porcelain` 前后均为 0 行。首轮计数自洽：34+3+2=39。
- **最终 36 KILLED / 3 SURVIVED**：Q2（S30，等价）、D6（tail=latest，等价）、R6（非 DoD，见 §3.3）。
- dev 自报 23 个里 22 KILLED（S30 等价）：结论一致。

## 5. 非 DoD 发现

1. R6：`BinanceDownReportsError` 对源名的断言被替身的错误文本顺带满足（§3.3）。
2. AD-29 的代价：最近 30 天（1d）/ 24 小时（5m）内，如果交易所本身就缺 K 线，每轮都会重取那一段，直到它移出窗口。这是人类裁决接受的取舍（AD-29 只为老缺口保留 90%），按 Syncer 的切段方式通常每轮只多 1 个请求。
3. 对调用方的行为变化（discovery 已如实记录）：binance 不可用时，`pricewatch backfill`（cmd/atlas/pricewatch.go:245）会在第一个出错的 bar 报错退出，已取到的数据已经入库，后面的资产或 bar 不再补。该文件属于 TASK-009，本任务没有改，Engine 侧只记日志并 track `sync:` 键，正是 R1-5 的目的。
4. QA 第 2 轮列为低优先级、不在 fix_items 里的 R2-1（只有 okx 一个源时上线日会误记）、S33/S11：本轮没有处理，也不要求处理。

## 6. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-004-b ebc366e43a6deda8dc28ae8974b2e3fa77bb3c1b
cd ../wt-verify-TASK-004-b
GOTOOLCHAIN=local go test -race -count=1 -coverprofile=c.out ./internal/pricewatch/ && go tool cover -func=c.out | grep sync.go
GOTOOLCHAIN=local go vet ./internal/pricewatch/; gofmt -l internal/pricewatch/sync.go internal/pricewatch/sync_test.go
python3 <scratchpad>/test-pw-b-T004-mut/run.py "$PWD" <输出目录>     # 39 个变异（Q5/R3 的修正版 Q5b/R3b 另跑）
```

## 7. 补核（13:1xZ，按 Leader 派验消息的追加要求；派验消息晚于任务文件到达，补核时结论已落盘）

| 追加要求 | 证据 | 结果 |
|---|---|---|
| 首轮摘要 | §0 对照表第一列：test-pw-a @ 80a56f3 判 VERIFIED（附非阻断发现）；首轮报告所列 S1 等价，本轮 D6 结论相同 | ✓ |
| 每条 fix_item 都有测试守卫 | QA-8：Q1、Q3、Q4、Q5b 由 4 个新用例抓到；AD-29：A1–A6；R1-5：R1–R5、R3b；F2：F1（§3–§4） | ✓ |
| S30 等价的独立判断 | 见 §3.1：R1-5 之后，任何源报错都会先走报错分支置 `leadingEmpty=false`，allZero 只决定「空切片是否入库」。唯一差别在「cs 非空但全部落在段外」，此时这些 K 线由所属段再取，所以等价。S29+S30 组合（Q3）KILLED | 同意等价 |
| AD-29 的 1d 用例（段边界放在 now−15d）能否区分 30d 与 7d | 变异 A3（1d 最短窗口从 30d 改成 7d）**KILLED**。now−20d 那一根所在的段终点是 now−15d，晚于 now−30d、早于 now−7d：按 30d 判要求 100% 而重取，按 7d 判只要 90% 而跳过。断言「now−20d 补回」能分开这两种实现，A5（判定改用段起点 from）也被同一用例抓到 | ✓ 能区分 |
| R1-5 的错误能被 Engine 的 `sync:` 健康键消费 | 读码 engine.go:78–85：`err := e.Syncer.Sync(ctx, a.Pair, bar)` → `e.track("sync:"+a.Symbol+"/"+bar, …, err, now)`，Sync 的任何非 nil 错误都会计入 `sync:<SYM>/<bar>` 失败计时。TASK-007 的 `TestCheckAsset_HealthAlerts/sync` 已证明这条路径满 30m 发 1 条「监控异常」（我复验 TASK-007 时 m24 KILLED）。R1-5 之前，binance 挂掉时 fetch 返回 nil，这条路径收不到错误；之后 `TestSync_BinanceDownReportsError` 断言 Sync 返回错误，R1 变异（旧 anyOK 语义）被抓到 | ✓ |
| 行为变化记录 | §5-3：`pricewatch backfill`（cmd/atlas/pricewatch.go:245）遇到 Sync 错误直接 return，后面的 bar 和资产不再补，已取到的数据已入库；非 DoD，属 TASK-009 文件 | 已记录 |
| R6（Leader 记为 QA-17） | §3.3：源名断言被替身错误文本 `"binance boom"` 顺带满足 | 已记录 |
