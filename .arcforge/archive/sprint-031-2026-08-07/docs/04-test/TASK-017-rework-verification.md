# TASK-017 返工验证报告 —— baostock 缓存键时间精度的双向守护

- 验证者: test-agent-16 / **assignment_epoch: 2** / `rework_count: 1`
- 被验对象: **`4c54e77`**（`gate_test.go` +50 行，**仅测试文件，生产代码零改动**）
- 首轮判定: `rejected`（`functional[4]` 无守护）— 报告见 `TASK-017-verification.md`
- 验证环境: 独立 worktree `../wt-v017r2 @ 4c54e77`（已在主仓库拆除）

## 结论：**PASS（verified）**

首轮唯一的缺口已补齐，**且采纳了我发出的更正：用的是双向形态而非我最初给的单向补丁**。

---

## 一、首轮缺口的闭环

首轮判定依据是 DoD `functional[4]` 自带的变异判据「去掉 Truncate 后该测试须转红」，
而当时**去掉后 28 个测试全绿**。本轮同一变异：

| 变异 | 首轮 | 本轮 |
|---|---|---|
| **U1** 去掉两个 `Truncate(time.Minute)` | **绿（存活）❌** | **红 `:358` ✅**，且**仅** `/同分钟内的相邻调用聚合到同一槽` |

**闭环成立**：同一变异在旧测试集下漏检、在新测试集下捕获。

## 二、双向判据独立有效

| 变异 | 预期 | 实际 | 只红哪个子测试 |
|---|---|---|---|
| **U1** 去掉 `Truncate`（键太细）| 红 | **红** ✅ `:358` | `/同分钟内的相邻调用聚合到同一槽` |
| **U2** `Truncate(time.Minute)` → `Truncate(time.Hour)`（键太粗）| 红 | **红** ✅ `:374` | `/跨分钟的调用不得串槽` |

**两个方向各自独立，互不覆盖**——这正是我在首轮报告后发出更正的原因：

> 我首轮给的候选补丁**只有 (a) 聚合度**这一半。`Truncate(time.Hour)` 能让 `base` 与
> `base+3s` 仍落同一小时 ⇒ 我那条断言照样通过，而小时粒度是错的（相隔几分钟的不同区间
> 查询会串槽、静默返回错区间数据）。

Dev 采纳了更正，实现的是完整双向形态，并在注释里把两类失效的性质写清楚了：
「静默的错」（键太细 ⇒ 命中率恒为零，不报错不出错数据）vs「吵闹的错」（键太粗 ⇒ 串槽返回错数据）。

### 2.1 写法上避开了假红 —— 与我的建议一致

Dev 没有字面用两次 `time.Now()`，而是取**当前分钟的中点**（`Truncate(time.Minute).Add(20*time.Second)`）
作基准，两个偏移（`+3s` / `+2min`）都落在确定位置。注释写明：

> 「约 1e-7 的假红概率可以完全消除，就没理由留着。」

**稳定性实测：`TestCacheKeyTruncatesToMinute` 连跑 10 次，0 次失败。**

## 三、回归确认 —— 首轮已通过的守护全部仍在

本次是**纯测试新增**（`git show --name-only` 只有 `gate_test.go`），生产代码零改动，
故首轮 7 个被捕获的变异原则上不受影响。抽查三条确认：

| 变异 | 结果 | 只红哪条 |
|---|---|---|
| U3 键去掉 symbol | **红** ✅ `:123` | `CacheKeyCoversParams/symbol` |
| U4 构造函数 `gate: nil` | **红** ✅ `:170` | `TestNewUsesDefaultGate`（陷阱 12 唯一守护者）|
| U5 去掉 `slices.Clone` | **红** ✅ `:249` | `TestFetchHistoryReturnsIndependentSlice` |
| U6 `mapPolicyError` 改用 `%w` | **红** ✅ `:322` | `TestPolicyErrorsDoNotLeak/超时` |

**无回归。**

## 四、基线

| 项 | 首轮 | 本轮 |
|---|---|---|
| 测试规模 | 21 PASS | **22 PASS**（+1 新测试，含 2 子测试）|
| SKIP | 0 | **0** ✅ |
| `-race` | ok | **ok**（2.40s）✅ |
| 覆盖率（单包口径，本任务无 `coverage_floor`）| 96.4% | **96.4%** ✅（纯测试新增，实现未动，数值不变符合预期）|
| 既有测试文件 | 一字未改 | **一字未改** ✅ |
| scope | 无越界 | **无越界** ✅（全部落在 `./internal/collector/baostock/`）|
| 还原 | — | 每变异 `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

## 五、备录（不作为 FAIL 依据）

**头部 DoD↔测试映射块未补 `functional[4]` 行。** 该映射写在了测试函数自身的文档注释上
（`gate_test.go:330`「TestCacheKeyTruncatesToMinute 覆盖 functional[4]…」），
**意图已达成、可检索**，但文件头部那个集中的映射块仍只列 `functional[0..3]`。

我首轮请求的是「在 gate_test.go 头部的 DoD↔测试映射注释里补上 functional[4] 一行」——
严格说这一条只做了一半。**不作为返工依据**：映射信息存在且更靠近被守护的代码，
只是没有汇总到头部索引。建议顺手补一行，让头部块保持「一眼看全」的作用。

## 六、复现命令

```bash
git worktree add --detach ../wt-v017r2 4c54e77 && cd ../wt-v017r2
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1 -race     # 22 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1 -cover    # 96.4%

# 双向判据（collector.go:45-46 的缓存键）
#   U1 去掉两个 Truncate(time.Minute)   → 仅 /同分钟内的相邻调用聚合到同一槽 红 :358
#   U2 Truncate(time.Minute)→time.Hour  → 仅 /跨分钟的调用不得串槽 红 :374
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1

# 稳定性（时间基准相关）
for i in $(seq 1 10); do GOTOOLCHAIN=local go test ./internal/collector/baostock/ \
  -count=1 -run TestCacheKeyTruncatesToMinute; done                          # 10/10 ok

cd <主仓库> && git worktree remove ../wt-v017r2
```
