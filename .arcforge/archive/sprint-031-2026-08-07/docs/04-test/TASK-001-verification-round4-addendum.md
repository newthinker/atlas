# TASK-001 第四轮验证 · 增补（防回归抽查补做）

- 验证者: test-agent-17
- 被验对象: `6a2a8df15816e32311b88c32d9f79dfc76c92474`（与第四轮主报告同）
- 验证环境: 独立 worktree `../wt-v001r4b`（detached @ 6a2a8df）
- 本文件是 `TASK-001-verification-round4.md` 的增补，**不替代**它；**`verified` 判定不变**。

## 零、为什么有这份增补（时序如实说明）

第四轮判定落盘于 `04:27:10Z`，Leader 的派验单发于 `04:19`（转 `verifying`）但送达在判定之后
——本 Sprint 第四次通知交错。派验单的复核要求里有**一项我判定时未做**：

> **防回归抽查**：确认 TASK-001 原有 8 条 DoD 的守护未被这 26 行改动削弱。

主报告覆盖了其余各项（三层逐层验证、五条自证、2×2 闭环、`want` 与实现一致性、
覆盖率/回归/scope）。本文件补做防回归抽查。**结论：无任何守护被削弱，判定不变。**

另核对：派验单中的 **M28**（`Lookup` 恒返 `ok=false`）与我主报告 §三 里的 **M30** 是同一个
变异，只是编号不同。实测捕获者为 `policy_test.go:214`，与派验单要求的第②层行号一致。

---

## 一、防回归抽查（9 条，覆盖全部 8 条 DoD，全部仍捕获）

按 Leader 新增的第 3 条纪律执行：**不只看 exit code，核到 `file:line` 的断言错误**。
每条变异均通过三道门——① `git diff --numstat` 非空；② `go vet` 通过（防编译失败的假红）；
③ 输出中存在 `policy_test.go:NNN` 断言错误行。还原后 `md5` 与基线断言相等。

| 变异 | 覆盖的 DoD | 结果 | 断言位置与实测输出 |
|---|---|---|---|
| M4 tushare 200→300ms | functional[0] 数值平移 | **红 ✅** | `policy_test.go:62: tushare.daily: MinInterval = 300ms, want 200ms` |
| M21 `Loc: shanghai()` → `loadLoc("Asia/Tokyo")` | functional[1] Loc 等值断言 | **红 ✅** | `policy_test.go:97: Loc = Asia/Tokyo, want Asia/Shanghai` |
| M16 `Lookup` 通配段提到精确段之前 | functional[2] 三段查表次序 | **红 ✅** | `policy_test.go:137: 精确匹配须优先于 <域>.* 通配` |
| M8 `Set` 无条件覆盖 `Domain` | functional[3] 显式 Domain 保留 | **红 ✅** | `policy_test.go:154: 显式 Domain 应被保留, got "custom"` |
| M2 `ApplyTTL` 删掉 `if p.TTL > 0` 守卫 | boundary[0] 只提升缓存主题 | **红 ✅** | `policy_test.go:166: yahoo.quote 是实时主题，TTL 必须保持 0, got 1m30s` |
| M12 `NewTable` 凭空登记 `crypto.*` | boundary[1] 未登记零策略 | **红 ✅** | `policy_test.go:74: crypto.ticker: 未登记主题不应命中策略表` |
| M10 `loadLoc` 失败分支 → `panic(err)` | error_handling[0] 分句1 | **红 ✅** | `policy_test.go:235: 时区加载失败不得 panic: unknown time zone Not/AZone` |
| M17 `shanghai()` → `loadLoc("Asia/Tokyo")` | error_handling[0] 分句2 | **红 ✅** | `policy_test.go:255: 嵌入 tzdata 后应拿到 Asia/Shanghai, got "Asia/Tokyo"` |
| M1 注入 `_ ".../internal/collector"` | non_functional[0] 约束 C3 | **红 ✅** | `policy_test.go:287: policy 不得依赖 collector 包（约束 C3 循环导入）` |

**8 条 DoD 各自的守护全部完好。** 这 26 行改动集中在 `TestDisableTTLKeepsThrottle` 一个函数内，
未触及其他测试，实测结果与预期一致——但按纪律这必须实测而非推断。

---

## 二、第 3 条纪律的执行留痕

Leader 本轮新增的纪律：

> | 3 | **红的不是断言**（编译/vet/panic/超时）| **看报错内容，核到 `file:line`** |

本次抽查的 runner 已把它固化：判定「红」的条件是**输出中存在 `policy_test.go:NNN` 行**，
而非 `'FAIL' in out`。若出现「exit 非 0 但无断言行」，脚本报 `红但非断言 ⚠` 而不计为捕获；
若 `go vet` 不过，直接报「变异无效」并跳过。上表 9 条**全部**给出了具体断言行号，
说明红的确实是断言。

这一条与我第四轮主报告 §四 自曝的那次假红是同一件事的两面：我当时的 runner 只看
`'FAIL' in out`，把编译失败当成了捕获。**Dev 在做 R5b 闭环时独立撞上同一个坑**
（删掉 `reflect.DeepEqual` 致 `import "reflect"` 未使用），且它的结论方向与我相反
（会得出「旧形态也能捕获、这次修复没必要」）。两人独立撞同一坑并给出同一诊断，
正是这条纪律该被机制化的理由。

Dev 的变通写法也值得记：`(false && !reflect.DeepEqual(...))` —— 让表达式失效但**保留引用**，
避免 import 变成未使用。比我采用的「一并删掉 import」更省事，且不改变 import 列表这一
无关变量。

---

## 三、判定

**`verified` 不变。** 本增补只补齐派验单要求的防回归一项，结论与第四轮主报告一致：
三层各守一个对象且各自有变异证据（① :205 / ② :214 / ③ :219）、五条自证全红、
2×2 闭环成立、`want` 与实现直读一致、覆盖率 97.3%、36 测试 0 SKIP、17 包回归绿、
`policy.go` 实现零改动。

## 四、复现命令

```bash
git worktree add --detach ../wt-v001r4b 6a2a8df15816e32311b88c32d9f79dfc76c92474
cd ../wt-v001r4b/internal/collector/policy

# 每条变异三道门：
#   ① git diff --numstat -- policy.go   非空
#   ② GOTOOLCHAIN=local go vet .        exit 0（否则红是构建的红，变异无效）
#   ③ go test . -run '^<该 DoD 的守护者>$'，输出须含 policy_test.go:NNN 断言行
# 9 条变异与对应 DoD 见 §一表格。

# 基线 md5（还原后须复原到）:
#   policy.go      = 89da8d213e62570dc8c90dee89d1ddd4
#   policy_test.go = 3e49623f33f8090d2eaddb3a38c67867
```

worktree 已拆除；主工作区 `internal/` 零污染。
