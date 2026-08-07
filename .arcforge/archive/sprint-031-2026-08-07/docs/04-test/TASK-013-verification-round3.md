# TASK-013 验证报告（第三轮 / 注释修正）—— 事实断言逐条核实

- 验证者: test-agent-17
- 被验对象: `884cb41`（`nothrottle_test.go` +11/-8，**纯注释**）
- 验证环境: 独立 worktree `../wt-v013r3`（detached @ 884cb41）
- assignment_epoch: 1 / `rework_count` 未消耗
- 前序: 首轮 `TASK-013-verification.md`、二轮 `TASK-013-verification-round2.md`

## 结论：**PASS（verified）**

「纯注释改动」独立核实通过；**两处注释的事实断言逐条核对，核心全部成立**。
29 测试 0 SKIP、17 包回归绿、五条核心守护红数未变。

一处措辞我未能完全复现（§三的断言 F「静默假绿」），**不构成缺陷**，仅作精度备录。

---

## 一、「纯注释改动」独立核实

```
git diff -U0 b95d98d 884cb41 | grep '^[+-]' | grep -v '^[+-]//' | grep -v '^[+-]\s*$'
→ 无输出
```

非注释行改动为 **0** ✅（我用的过滤与 Dev 的 `-U0` 机械核实互为独立复现）。

---

## 二、第一处（`skipDirs` 的 `testdata` 项）：**三条事实断言全部对得上**

新注释的断言：

> `testdata` —— 其中有**故意**写成回潮形态的阳性对照；不跳过的话生产扫描会把它**当成 offender 报出来**。
> …… 去掉本项后，生产扫描会走进阳性夹具并**把它检出为 offender**，该测试转红。

逐条核实：

| 断言 | 核实方式 | 结果 |
|---|---|---|
| A 夹具含回潮形态 | `grep` 阳性夹具 | `lastReq` 在 **第 22 行**、`minInterval` 在 **第 23 行** ✅ |
| B/C 去掉该项 → 检出为 offender、转红 | 注入 N2 跑 `TestNoPrivateThrottleState` | **红 `:166 发现私有节流状态字段`**，offenders 恰为 `testdata/throttleback/collector.go:22` 与 `:23` ✅ |

**行号与断言逐字对得上** —— 这正是「新声明必须经过验证」的落点，与 Dev 自报一致。

> 对照二轮：同一变异当时红在 `:159「解析 …bad.go 失败」`。**旧注释描述的机制已随夹具迁移失效，
> 新注释描述的是当下真实机制。** 这条修正是必要的。

---

## 三、第二处（`TestScanDetectsThrottleState` 的 root 规则）：D/E 成立，F 部分成立

新注释把「规则的论据」换成了失效模式本身：

> 共用 root 时这次遍历会同时受多个夹具影响，其中任何一个先出问题都可能让遍历提前结束
> —— 断言失败，而错误信息指向的是**另一个**夹具，方向完全不对；更麻烦的是它随目录名的
> 字典序而变，夹具一改名就从假红翻转成静默假绿。

我在 worktree 里临时构造「共用 root」场景验证（用完即删，`testdata` 复原为 `decoys`/`throttleback`）：

| 情形 | 结果 | 对应断言 |
|---|---|---|
| 共用 root，无额外夹具 | **绿** | — |
| 共用 root + 字典序**在前**的畸形夹具 | **红** `:208 扫描阳性夹具失败: 解析 testdata/aaa-x/bad.go 失败` | **D 遍历提前结束 ✅ / E 错误指向另一个夹具 ✅** |
| 共用 root + 字典序**在后**的畸形夹具 | **红**（错误指向 `zzz-bad/bad.go`） | D/E ✅ |

**D、E 完全成立**：错误信息说的是 `aaa-x/bad.go`，而测试名是 `TestScanDetectsThrottleState`
（验的是 `throttleback`）—— **方向确实不对**。

**F 部分成立**：我复现出了「**结果随夹具集合翻转**」（无额外夹具→绿；加一个→红），
这印证了「随目录名的字典序而变」这半句 —— 绿的那次是「碰巧没有别的夹具干扰」而非「验证有效」。

但**「翻转成静默假绿」这个措辞我未能复现**：当前实现里 `scan` 遇错即上抛、`t.Fatalf` 先触发，
所以另一个夹具出问题时**总是红**，没有出现「该红却绿」。

⇒ **不构成缺陷**（规则保留的理由由 D/E 充分支撑，且注释已诚实标注「当下无法复现」）。
若要更精确，那半句可收敛为「**结果取决于其他夹具的存在与命名这类无关因素**」——
我实测到的正是这个形态。

> 括注「规则保留 —— 它防的是往 testdata 里再放夹具的下一个人，不是已经消失的那一个」
> **是准确的**：我加一个畸形夹具就复现了 D/E，说明这条规则守的是真实的未来风险。

---

## 四、抽验：五条核心守护红数未变

| 变异 | 定向到 | 结果 |
|---|---|---|
| V1 `scan` 恒返空 | 阳性 `TestScanDetectsThrottleState` | **红** `:211` |
| V2 `scan` 恒记 offender | 阴性 `TestNoPrivateThrottleState` | **红** `:166` |
| V3 去掉 root 豁免 | `TestScanRootExemptFromSkipRule` | **红** `:275` |
| V6 不跳过 `_test.go` | `TestScanIgnoresNonFieldOccurrences` | **红** `:294` |
| V9 行号恒记为 1 | 阳性 | **红** `:216` |

（行号相对二轮有位移，因注释增删；**断言内容与红的原因均未变**。）

29 测试 0 SKIP、17 包回归绿、worktree 复原干净。

---

## 五、我这轮自己的两次构造缺陷（据实记录）

**① 改 root 时没同步 `fixture`** —— 第一版「共用 root」实验里我只改了扫描 root，
`fixture := filepath.Join(root, "collector.go")` 随之变成 `testdata/collector.go`（不存在），
于是红在 `:215 解析夹具 …: no such file`，**红的原因与我要验的失效模式无关**。
修正为「root 改、fixture 保持指向真实夹具」后才拿到有效结果。

**② 第一版实验两种字典序都红，我一度以为 F 不成立** —— 实为①的连带后果。

⇒ 又两次「验证工具自身有缺陷」。这轮我是靠**核到断言行**发现的：
`:215 解析夹具 …` 与我预期的 `:208 扫描阳性夹具失败` 不是同一条断言。
**若只看红/绿，我会得出「F 不成立」的错误结论。** ——
与本轮被验对象的主题（红的原因换了）恰好是同一件事。

---

## 六、Done Criteria

本轮为纯注释修正，不改变任何 DoD 的覆盖结论。五项判定沿二轮，均 **PASS**。
`non_functional[0]`（不引入生产代码改动）：`git diff --name-only` 仅 `nothrottle_test.go` ✅

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v013r3 884cb41
cd ../wt-v013r3

# 纯注释核实
git diff -U0 b95d98d 884cb41 | grep '^[+-]' | grep -v '^[+-]//' | grep -v '^[+-][[:space:]]*$'   # 须无输出

# 第一处断言：skipDirs 去掉 testdata → TestNoPrivateThrottleState 红 :166
#   offenders 须恰为 testdata/throttleback/collector.go:22 与 :23
#   （与 grep lastReq/minInterval 的实际行号逐字比对）

# 第二处断言（D/E）：把 TestScanDetectsThrottleState 的扫描 root 改成 "testdata"
#   **同时保持 fixture 指向 testdata/throttleback/collector.go**（否则红在 :215，与失效模式无关）
#   再在 testdata/ 下放一个畸形目录（aaa-x/bad.go 内容 "package bad\n\nfunc broken(\n"）
#   → 红 :208「扫描阳性夹具失败: 解析 testdata/aaa-x/bad.go 失败」= 错误指向另一个夹具
#   不放畸形夹具时 → 绿（结果随夹具集合翻转）
#   ⚠ 实验后删除临时目录，testdata 须复原为 decoys/ 与 throttleback/
```

worktree 已于验证结束后清理；主工作区零污染。
