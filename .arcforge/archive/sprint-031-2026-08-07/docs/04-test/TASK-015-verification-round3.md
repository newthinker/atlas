# TASK-015 验证报告（第三轮 / 聚合度补救）—— eastmoney 缓存键时间精度双向守护

- 验证者: test-agent-17
- 被验对象: `de72c0c`（**本次提交**仅 `gate_test.go` **+57**，纯测试）
- 验证环境: 独立 worktree `../wt-v015r3`（detached @ de72c0c）
- assignment_epoch: **3** / rework_count: 1
- 前序: 首轮 `TASK-015-verification.md`、二轮 `-round2.md`

## 结论：**PASS（verified）**

上轮唯一的 FAIL（`functional[4]` 缓存键聚合度无守护）已补上，且**补成了双向** ——
Dev 多写了一个方向（「粒度不得放粗」），那是 DoD 没要求的另一半。

30 测试 **0 SKIP**、`-race` 绿、覆盖率 **87.6%**（保持）、17 包回归绿、
本次提交**仅测试文件**（实现与既有测试均未动）。

---

## 一、`functional[4]` 已有守护，且是**双向**的

新增 `TestCacheKeyAggregatesNearbyTimes`，含两个子测试：

| 子测试 | 断言 | 防的是 |
|---|---|---|
| 「相邻时间落进同一槽」 | 同一分钟内相隔数秒的**三次**调用 → HTTP **1** 次 | 键**太细**（生产以 `time.Now()` 为 end ⇒ 命中率恒为零） |
| 「**分钟粒度不得放粗**」 | 相隔 1 分钟的两次调用 → HTTP **2** 次 | 键**太粗**（不同区间串槽，静默返回错区间数据） |

**第二个方向是 DoD 没要求的** —— `functional[4]` 只写了聚合度。Dev 自己补了它的反面，
理由写在断言消息里：「为 1 说明截断粒度被放粗到小时/天——不同区间的查询会串槽，**静默返回错区间数据**」。

> 这与 DoD 自身对「区分度 / 聚合度」的诊断是同一个结构：**只写一半会得到「键太细导致缓存失效」
> 或「键太粗导致串味」之一**。Dev 把这个结构又往下推了一层 —— 聚合度本身也有过头的一侧。

### 四条变异，两个方向各自独立

| 变异 | 预期 | 实际 | 命中的子测试 |
|---|---|---|---|
| **G1** 去掉 `Truncate`（纳秒精度） | 红 | **红** ✓ | `:387 三次调用应命中同一缓存槽: HTTP 请求 3 次, want 1` |
| **G2** 秒级精度（`Unix()` 不截断） | 红 | **红** ✓ | `:387` 同上 |
| **G3** 粒度放粗到小时 | 红 | **红** ✓ | `:403 相隔 1 分钟的两次调用是不同区间，必须分槽: HTTP 请求 1 次, want 2` |
| **G4** 粒度放粗到 24 小时 | 红 | **红** ✓ | `:403` 同上 |

**两个方向互不重叠**：G1/G2 只打第一个子测试，G3/G4 只打第二个 —— 说明不是一条断言凑合覆盖两面。

---

## 二、变异有效性的旁证（沿用上轮探针）

上轮我判 FAIL 时用探针证明过「变异确实改变行为」（避免把「全绿」误读为无守护）。
本轮 G1 的行为与上轮探针一致：

| 实现 | 生产路径形态（以 `time.Now()` 为 end 连调） |
|---|---|
| 变异（`UnixNano`） | 每次新键 ⇒ 上轮探针实测 **2 次 HTTP**；本轮测试实测 **3 次**（三次调用） |
| 交付态（`Truncate` 到分钟） | **1 次** |

⇒ 上轮「变异有效但无守护」与本轮「同一变异转红」形成完整对照：**缺口确实被补上了**，
不是变异失效导致的假绿转假红。

---

## 三、flaky 检查（该测试依赖 `time.Now()`）

`base := time.Now().Truncate(time.Minute).Add(20 * time.Second)` —— 取**当前分钟的中点**，
子测试最远用到 `base.Add(15*time.Second)`（即 :35），不会跨分钟边界。

实测**连跑 10 次，10 次通过**。设计上规避了跨分钟 flaky，实测也稳定。

---

## 四、防回归

| 变异 | 目标 | 结果 |
|---|---|---|
| **G5** key 去掉 `symbol` | `TestCacheKeyCoversAllParams`（区分度） | **红** `:139 缓存键未区分 symbol` |

上轮验过的区分度守护未被这 57 行削弱。首轮/二轮的其余守护（映射层四方向、
`TestErrorResponseIsNotCached`、`TestNotThrottled` 等）本次提交未触及实现，不重复抽查。

---

## 五、Done Criteria 终态

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| functional[0..3] | 缓存/区分度/构造函数/主题域 | 首轮已验；G5 防回归 | **PASS** |
| **functional[4]** | **缓存键聚合度** | **G1/G2 红 `:387`，G3/G4 红 `:403`（双向）** | **PASS（上轮 FAIL 已修）** |
| boundary[0..3] | 不节流/独立切片/不串味/FetchQuote 不经 Gate | 首轮已验 | **PASS** |
| error_handling[0] | 错误不写缓存 | 首轮已验（E9 三条断言） | **PASS** |
| error_handling[1] | policy 错误不外泄 | 二轮已验（R1-R4 四方向） | **PASS（二轮 FAIL 已修）** |
| non_functional[0] | 既有测试一字不改、`-race`、0 SKIP、覆盖率 | 本次仅 `gate_test.go`；30 测试 0 SKIP；`-race` 绿；**87.6%** 保持 | **PASS** |

**全部 PASS。**

---

## 六、复现命令

```bash
git worktree add --detach ../wt-v015r3 de72c0c
cd ../wt-v015r3

git show --stat de72c0c        # 本次 scope：仅 gate_test.go +57
GOTOOLCHAIN=local go test ./internal/collector/eastmoney/ -count=1 -race -cover

# functional[4] 双向（改 eastmoney.go:434-435 的 key 构造）：
#   G1 start/end.UnixNano()                    → :387 红（三次调用 3 次 HTTP）
#   G2 start/end.Unix()（不截断）                → :387 红
#   G3 Truncate(time.Hour)                     → :403 红（相隔 1 分钟只发 1 次）
#   G4 Truncate(24*time.Hour)                  → :403 红
#   两个方向互不重叠：G1/G2 只打子测试①，G3/G4 只打子测试②
# 防回归：key 去掉 symbol → :139 红（区分度）
# flaky：TestCacheKeyAggregatesNearbyTimes 连跑 10 次，10 次通过
```

worktree 已于验证结束后清理；主工作区零污染。
