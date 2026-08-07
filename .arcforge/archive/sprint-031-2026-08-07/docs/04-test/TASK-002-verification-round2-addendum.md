# TASK-002 第二轮验证 · 增补（闭环对比的独立复现）

- 验证者: test-agent-17
- 被验对象: `487e2ba8e1911729776aee7acab18bd9b0992e89`（与第二轮主报告同）
- 验证环境: 独立 worktree `../wt-v002c`（detached @ 487e2ba）
- 本文件是 `TASK-002-verification-round2.md` 的增补，**不替代**它；
  **`verified` 判定不变**，本文件只补强证据。

## 零、为什么有这份增补

第二轮判定落盘于 `04:14:14Z`，Leader 派验单发于约 `04:05Z` 但送达在判定之后（第三次通知
交错）。派验单要求独立复现**四项**，我在第二轮主报告里覆盖了三项：

| 派验单要求 | 主报告是否覆盖 |
|---|---|
| 正确实现连跑 5 次全绿无假红 | ✅ 覆盖（实跑 **8 次**） |
| var2（全局锁在 early-return **之前**）→ 红 | ✅ 覆盖（定向 + 全量） |
| var1（全局锁在 early-return **之后**）→ 红 | ✅ 覆盖（定向 + 全量） |
| **闭环：var1 下把 `b.x` 退回 `{}` → 绿** | ❌ **未覆盖** |

第四项我在**首轮**做过等价的事（首轮正是在 `b.x: {}` 的旧版上注入 var1 发现它存活，那是判
rejected 的依据），但**跨轮引用不等于同环境对照**——两轮的 worktree、进程、调度状态都不同。
Leader 的要求是对的：这是「修复有效」与「**证明**修复有效」的分界。本文件在**同一个
worktree、同一次会话**里跑完整的 2×2 对照。

---

## 一、闭环 2×2 对照（同一 worktree、同一环境）

每格独立 `git checkout` 复位后重新构造，构造完先用 `git diff --numstat` 断言目标文件确有
改动（无改动即判「变异/改造无效」并中止），再 `-run TestGateThrottleDoesNotHoldGlobalLock`。

| | **无变异**（正确实现） | **var1 变异**（全局锁加在 early-return 之后） |
|---|---|---|
| **旧版 `"b.x": {}`** | 绿（正常） | **绿 ❌ 漏检** ← 闭环项 |
| **新版 `"b.x": {MinInterval: time.Millisecond}`** | 绿（正常） | **红 ✅ 捕获** |

新版 + var1 的实测输出：

```
gate_test.go:207: 第 0 轮: a 域节流期间 b 域被阻塞 479.565166ms —— throttle 持有了跨域的全局锁
```

### 这张表读出三件事

1. **唯一的「该红却绿」格恰是旧版 + var1** —— 修复前确实漏检，**dev 的闭环自报属实**。
2. **新版捕获 var1** —— 修复有效。
3. **两版对正确实现都不产生假红** —— 修复没有把断言收得过紧。

四个格子构成完整的因果隔离：绿/红的差异**只能**归因于 `b.x` 那一行，因为其余一切（HEAD、
worktree、进程、变异内容）都相同。这正是单独跑「新版 + var1 → 红」所不能证明的
——那只说明「现在能抓到」，不说明「以前抓不到」，而后者才是这次返工的全部理由。

---

## 二、Leader 要求的另一项：首轮 7 个变异未失效

已在第二轮主报告 §二 覆盖，且比要求更严——**全部定向 `-run` 到该条 DoD 的专属守护者**
（不以套件转红为准）：G1 / G2a / G2b / G3 / G4 / G5 / G6 / G7 全部仍捕获，另加 G13 / G14，
共 10 条。G1b 的两种形态也已双双复现转红。**无一因这 11 行测试改动而失效。**

---

## 三、判定

**`verified` 不变。** 本增补只是把第四项证据补齐，结论与第二轮主报告一致：
13 项 DoD 全 PASS、覆盖率 97.3%、36 测试 0 SKIP、`-race` 绿、17 包回归绿、scope 干净、
`gate.go` 实现零改动。

## 四、复现命令

```bash
git worktree add --detach ../wt-v002c 487e2ba8e1911729776aee7acab18bd9b0992e89
cd ../wt-v002c/internal/collector/policy

# 2x2 的四格：分别组合
#   b.x 版本：新版 "b.x": {MinInterval: time.Millisecond}  /  旧版 "b.x": {}
#   实现：原样  /  var1 变异（Gate 加 globalMu，在 throttle 的 MinInterval 检查【之后】加锁）
# 每格：git checkout -- gate.go gate_test.go 复位 → 构造 → git diff --numstat 断言非空
#      → go test . -run '^TestGateThrottleDoesNotHoldGlobalLock$'
# 期望：仅「旧版 + var1」为绿（漏检），「新版 + var1」为红，两个无变异格均绿。

# 基线 md5（还原后须复原到）:
#   gate.go      = b350bcaa919e8b9a595e096c83a72216
#   gate_test.go = eaed18ee9e8ef7fb2c5e0f5c0fd3f90b
```

worktree 已拆除；主工作区 `internal/` 零污染
（现存的 `policy_test.go` 未提交改动属 TASK-001 的 review_fix，非本次验证所致）。
