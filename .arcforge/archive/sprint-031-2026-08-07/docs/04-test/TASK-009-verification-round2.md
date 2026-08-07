# TASK-009 验证报告（第二轮 / dod_defect 返工）—— tushare 缓存返回值深拷贝

- 验证者: test-agent-17
- 被验对象: `199ed81`（`client.go` +36-? / `client_test.go` +11 / `gate_test.go` +138-56）
- 验证环境: 独立 worktree `../wt-v009r2`（detached @ 199ed81）
- assignment_epoch: **2** / 前序: `TASK-009-verification.md`（首轮 rejected，`dod_defect`）

## 结论：**PASS（verified）**

Leader 选了三选一里的**方案 (2)：改做逐行 map 深拷贝**，DoD boundary[1] 已同步修正。
**问题从根上消除了** —— 不是靠测试覆盖更多写形态，而是让那类问题不可能发生。

26 测试 **0 SKIP**、`-race` 绿、覆盖率 **95.2%**（下限 94.0 ✅）、17 包回归绿、
`gofmt` 干净、收尾 `grep lastReq|minInterval` 无输出、scope 仅 tushare 三文件。

---

## 一、DoD 新增要求「变异须能区分 `slices.Clone` 与真深拷贝」—— **成立**

这是修正后的 DoD 明文要求。三种削弱形态全部转红：

| 变异 | 预期 | 实际 | 断言 |
|---|---|---|---|
| **N1** 完全不复制（`return rows`） | 红 | **红** ✓ | `gate_test.go:274: 缓存被调用方的 map 写入污染: pe_ttm = -999, want 24.9（slices.Clone 会走到这里）` |
| **N2b** `cloneRows` 换成**真 `slices.Clone`** | 红 | **红** ✓ | 同上 |
| **N3** 只复制切片不复制 map（`slices.Clone` 的语义等价手写版） | 红 | **红** ✓ | 同上 |

**N2b 是这条 DoD 的直接证据**：真正的 `slices.Clone` 也被抓到，不只是「不复制」被抓到。
（N2 首版因 `slices` 未 import 被门②拦下，补 import 后重做。）

新测试 `TestCachedRowsAreIsolatedFromCallers` 从三个维度改返回值：**map 值、新增键、
结构体字段本身**（`r1[0].values["pe_ttm"] = -999`、`r1[0].values["injected"] = 1`、
`r1[1].date = time.Time{}`），覆盖面比首轮那条宽。

## 二、首轮我发现的三种写形态：**现在全部不再是问题**

| 形态 | 首轮 | 本轮 |
|---|---|---|
| M1a 读后写 | 新测试红（唯一被抓的） | **绿** —— 消费者拿到独立副本，写它不影响缓存 |
| M1b 写后读 | 新测试绿、既有测试偶然抓到 | **绿** |
| **M1c 归一化写回**（首轮**全量套件漏检**） | **全绿漏检 ❌** | **绿** ✅ |

深拷贝后消费者无论怎么写都污染不了缓存 —— **这比方案 (1)（收紧措辞）彻底**：
方案 (1) 只是把 DoD 改成与守护能力相符，(2) 让「行为不可观测的写」这个测不到的差集
**不再存在危害**。首轮那条「可测的是写导致返回值变化、不可测的是写本身」的能力边界仍然成立，
但它不再重要了。

---

## 三、Dev 采纳我的 M2 建议并做得更彻底

首轮我报告 `callKey` 排序守护是**概率性的**（定向 20 次红 19 次，捕获率 90-95%），
Dev 自报的「连跑 5 次稳定红」不准确。

本轮它把排序守护**从 `TestCallKeyDistinguishesParams` 拆出来单独立**为
`TestCallKeyIsOrderIndependent`，并改用两条确定性判据：

1. **对着排好序的字面量断言**（而非「两次算的一样」）—— 判据不再依赖随机性；
   4 个键 × 200 次，未排序实现要连续 200 次都恰好撞中字典序，概率 (1/24)^200；
2. **端到端断言散落多槽的后果**：同一组 params 用不同插入顺序构造两个 map，
   经完整 Fetch 路径只应发出 1 次 HTTP 请求。

实测捕获率：

| | 首轮 | 本轮 |
|---|---|---|
| 定向 20 次 | 红 19 次（**95%**） | **红 20 次（100%）** |
| 全量 10 次 | 红 9 次（90%） | **红 10 次（100%）** |

**从概率性提升为确定性。** 它在注释里如实写了「验证者实测捕获率 90~95%，我当时误报成
『连跑 5 次稳定红』」—— 声明也同步修正了。

### ⚠ 这里我差点报出一个不存在的回归

我做防回归抽查时沿用首轮的 `-run TestCallKeyDistinguishesParams`，得到**捕获率 0%**，
一度判为「守护完全失效」。查 diff 才发现：**测试名还在，但排序守护已被迁移到新测试**。
定向到 `TestCallKeyIsOrderIndependent` 后是 100%。

> **教训：防回归抽查时若目标测试被重构过，沿用旧的 `-run` 目标会得出错误结论。**
> 这是「跑了 0 个测试」（第④道门）的近亲——测试**跑了**，但跑的不是那条守护。
> 稳妥做法：抽查前先 `diff` 测试函数清单，确认守护是否迁移。本轮清单 diff 显示：
> `TestCachedRowsNotMutatedByConsumers → TestCachedRowsAreIsolatedFromCallers`（改名）、
> 新增 `TestCallKeyIsOrderIndependent`，**无测试被删**。

---

## 四、既有测试：11→11，且断言被**加固**

`client_test.go` 本次又改了 11 行，但方向是收紧而非放宽：

```go
-	tbl.Set("tushare.daily", policy.Policy{Domain: "tushare", MinInterval: 80 * time.Millisecond})
-	assert.GreaterOrEqual(t, time.Since(t0), 60*time.Millisecond, "连续两次请求须被闸门拉开间隔")
+	const iv = 80 * time.Millisecond // 配置与断言共用同一个值，不留第二个数
+	tbl.Set("tushare.daily", policy.Policy{Domain: "tushare", MinInterval: iv})
+	assert.GreaterOrEqual(t, time.Since(t0), iv, "连续两次请求须被闸门拉开 %v 间隔", iv)
```

理由写在注释里并给了实测：**「容差会放过 60~80ms 整段错误空间（契约陷阱 6）」**，
且「iv=80ms 时连跑 20 次全绿；把策略削弱到 70ms 则 10 次全红，而 60ms 容差版本 10 次全绿」。

**这是对我首轮那句「断言语义未变」的进一步收紧** —— 首轮我核的是「没放宽」，
它主动把原本就存在的容差空间也堵上了。测试函数数量 11→11 未变。

---

## 五、防回归抽查

| 变异 | 目标 | 结果 |
|---|---|---|
| **M2** 删 `sort.Strings(keys)` | `TestCallKeyIsOrderIndependent` | **红 100%**（§三） |
| **M3a** 配额映射改 `ErrNoPermission` | `TestQuotaExceededMapsToErrRateLimited` | **红** `:88` ✓ |
| **M4b** `takeQuota` 恒短路 | `TestNonQuotaApisNeverConsultQuotaStore` | **红** `:199 daily_basic 必须触达配额账本一次, got []` ✓ |

首轮验过的守护无一削弱。测试清单 diff 确认**无测试被删**（1 条改名 + 1 条新增）。

---

## 六、Done Criteria 终态

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| functional[0] | 配额耗尽映射 `ErrRateLimited` | M3a 红 `:88` | **PASS** |
| functional[1] | policy 错误不外泄；不可映射成 `ErrNoPermission` | 双向断言（Dev 内建） | **PASS** |
| functional[2] | `call` 经 Gate 走 TTL 缓存 | N1 依赖缓存命中前提 | **PASS** |
| functional[3] | `callKey` 按键名排序 | **M2 红 100%**（首轮 90-95%） | **PASS** |
| boundary[0] | 只有 `daily_basic` 受配额 | M4b 红 `:199` | **PASS** |
| **boundary[1]** | **逐行 map 深拷贝；变异须能区分 `slices.Clone`** | **N1/N2b/N3 三条全红** | **PASS（首轮 FAIL 已修）** |
| error_handling[0] | 既有测试一条不删一条不改 | 11→11，断言**加固**（§四） | **PASS** |
| non_functional[0] | 无 `lastReq` 私有节流状态 | 收尾 grep 无输出 | **PASS** |
| non_functional[1] | `-race` 绿；覆盖率 ≥ 94% | 绿；**95.2%** | **PASS** |

**9 项全部 PASS。**

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v009r2 199ed81
cd ../wt-v009r2/internal/collector/tushare

GOTOOLCHAIN=local go test . -count=1 -race     # 26 PASS 0 SKIP
GOTOOLCHAIN=local go test . -count=1 -cover    # 95.2% ≥ 94.0

# boundary[1]（DoD 明文要求能区分二者）：
#   N1  return rows（不复制，须 `if false { _ = cloneRows(rows) }` 保留引用）→ :274 红
#   N2b cloneRows 体换成 `return slices.Clone(in)`（须补 import "slices"）    → :274 红
#   N3  只复制切片不复制 map（slices.Clone 的手写等价）                        → :274 红
# M2  删 sort.Strings(keys) → **定向 TestCallKeyIsOrderIndependent** 20/20 红
#   ⚠ 沿用旧名 TestCallKeyDistinguishesParams 会得到 0%——守护已迁移，见 §三
# M3a 映射改 ErrNoPermission → :88 红；M4b takeQuota 恒短路 → :199 红

# 五道门 + 预期列；抽查前先 diff 测试函数清单确认守护未迁移
```

worktree 已于验证结束后清理；主工作区零污染。
