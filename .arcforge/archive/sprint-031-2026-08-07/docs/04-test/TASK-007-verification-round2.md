# TASK-007 验证报告（第二轮 / review_fix 返工）—— yahoo 补两条守护

- 验证者: test-agent-17
- 被验对象: `04bf776`（仅 `internal/collector/yahoo/gate_test.go` **+77**）
- 验证环境: 独立 worktree `../wt-v007r2`（detached @ 04bf776）
- assignment_epoch: 1 / rework_count: 1（`reason_class=task_defect`）
- 前序: `TASK-007-verification.md`（首轮 verified，两条缺口后续经我补核确认）

## 结论：**PASS（verified）**

两条缺口（我首轮漏核的 200-but-error、以及首轮报告项 EPS Clone）**均已补上守护**，
经独立复现全部有效。**生产代码零改动** —— 定性「实现对、缺守护」得到确认。

42 测试 **0 SKIP**、`-race` 绿、覆盖率 **88.0%**（原 87.3%）、17 包回归绿、
既有测试与实现文件**一行未动**。

---

## 一、生产代码零改动（定性确认）

```
git diff 1c08953 04bf776 --name-only -- internal/collector/yahoo/
→ internal/collector/yahoo/gate_test.go        ← 仅此一个文件
```

**若它改了实现，就说明我们对缺口的定性有误**（实现本来就是对的，缺的只是守护）。
实测确认定性正确。

---

## 二、F1（200-but-error）：三条断言全部命中，含**决定性的那条**

W1（短路 `fetchHistory` 内的 `Chart.Error` 校验）下完整输出：

```
gate_test.go:346: 第 0 次: 200-but-error 信封必须识别为错误, got nil
gate_test.go:346: 第 1 次: 200-but-error 信封必须识别为错误, got nil
gate_test.go:353: 错误不得写缓存，两次调用应各发一次 HTTP, got 1
                  （为 1 说明错误响应被当成功值缓存了——一次瞬时故障会变成整个 TTL 期的持续故障）
```

**`:353` 的 `got 1` 是决定性证据** —— 第二次命中了缓存。`:346` 只证明「返回了 error」，
`:353` 才证明「不被缓存」。**这两面是分开的**：一个实现完全可以正确返回 error 却仍然缓存它
（若错误判定发生在 Gate 之外）。

Dev 的复盘与此吻合：

> 我原本的方案里两条断言都有，**但没意识到它们守的是两件事**。

---

## 三、W1b 对照格：**构造陷阱真实存在**（Leader 点名复核）

| 场景 | 结果 |
|---|---|
| W1（短路校验）+ `errorEnvelopeBody` 的 `result` **非空** | **红**（三条断言，§二） |
| **W1b**：同一变异 + `result` 改成**空数组** | **绿 —— 测不出** |
| 对照：无变异（基线） | 绿 |

**W1b 实证了我给的构造陷阱**：响应的 `result` 必须非空，否则删掉 `Chart.Error` 校验后
会被 `len(result.Chart.Result) == 0` 这条**更早的兜底分支**拦截，`fn` 仍返回 error ⇒ 不缓存 ⇒ 变异测不出。

Dev 的 `errorEnvelopeBody` 注释明写「**result 非空**且 error 非空」，绕开了这个拦截点。✅

> 这是「变异没打中」的又一形态：**被另一个更早的兜底分支拦截**。与 TASK-005 的 O5
> （改对地方但那地方不可达）同族，只是拦截者不同 —— O5 是快速路径，这次是另一个校验分支。

---

## 四、F2（EPS Clone）

| 变异 | 结果 |
|---|---|
| Y8：`FetchEPSHistory` 不 `Clone`（`if false { _ = slices.Clone(pts) }` 保留 import） | **红** `gate_test.go:381: 缓存命中必须返回独立副本，否则调用方能污染缓存` |

首轮报告项已闭合。

---

## 五、防回归抽查（+77 行未削弱任何既有守护）

| 变异 | 目标 | 结果 |
|---|---|---|
| Y1 `do()` 首发也 `Wait` | `TestFirstRequestDoesNotWaitTwice` | **红** `:303 首次请求等了 199.9ms（约 100ms 的两倍）` |
| Y2 `FetchQuote` 误用 chart 主题 | `TestEachMethodUsesItsOwnTopic` | **红** `:232` |
| Y3b `FetchHistory` 不 `Clone` | `TestFetchHistoryReturnsIndependentSlice` | **红** `:129` |

既有测试文件（`yahoo_test.go` / `eps_test.go` / `throttle_test.go`）与实现文件**一行未动**。

---

## 六、我这轮的一次变异打偏（方法论留痕）

W1 首版**预期红实际绿**。诊断：`if result.Chart.Error != nil` 在 `yahoo.go` 里有**两处**
（`fetchQuote` `:254` 与 `fetchHistory` `:332`），我的 `replace(..., 1)` 只替换了**第一处**，
而测试走的是 `FetchHistory` ⇒ **删的是 quote 路径的校验，测的是 history 路径**。

改用 `rindex`（最后一处）后立即转红。

> 这是「变异没打中」的第 N 种形态：**同一文本在文件中出现多次，替换了错误的那一处**。
> 落盘校验（0+/3-）通过了、编译通过了、测试也跑了 —— 前四道门全部放行，
> **只有预期列能发现它**。

---

## 七、Done Criteria 相关条目

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| error_handling[0] | 既有错误路径行为不变；**错误不写缓存** | **W1 三条断言全红**（含决定性的 `:353`）；既有错误路径测试一行未动 | **PASS（缺口已补）** |
| boundary[0] | 缓存命中返回独立切片 | Y3b 红 `:129`（FetchHistory）+ **Y8 红 `:381`（EPS，扩围）** | **PASS** |
| functional[0..3] | 缓存/主题/同域/重试 | Y1 `:303`、Y2 `:232` 防回归全红 | **PASS** |
| non_functional[1] | `-race` 全绿；覆盖率 ≥ 86% | 绿；**88.0%**（原 87.3%） | **PASS** |

---

## 八、复现命令

```bash
git worktree add --detach ../wt-v007r2 04bf776
cd ../wt-v007r2

git diff 1c08953 04bf776 --name-only -- internal/collector/yahoo/   # 须仅 gate_test.go
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -race   # 42 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -cover  # 88.0%

# W1：删掉 yahoo.go 中**最后一处**（fetchHistory 的）`if result.Chart.Error != nil {...}`
#     ⚠ 该文本有两处，删 fetchQuote 那处（第一处）测不出 —— 见 §六
#     → :346 ×2 + :353（决定性，got 1）
# W1b：W1 + 把 errorEnvelopeBody 的 result 改成 [] → **绿**（被 len(Result)==0 更早拦截）
# Y8：eps.go 去掉 slices.Clone（`if false { _ = slices.Clone(pts) }` 保留 import）→ :381 红
```

worktree 已于验证结束后清理；主工作区零污染
（`D internal/collector/cache.go` 等属 TASK-011/013 在途改动，非本次验证所致）。
