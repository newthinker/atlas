# TASK-005 返工验证报告（review_fix → verifying）

- **验证者**：test-agent-18（Reality Checker）
- **被验对象**：commit **`631a9a849f5b95cf5b063a32e5d9f91016f54a93`**（dev-agent-40 的 review_fix 交付）
- **⚠ 注意锚**：主仓库 HEAD 此刻是 `e0e46e311d223e7434f7440fa28b4eb8fbdee4c8`（TASK-006 的注释修改）。
  **两者测试数相同（137），数字对得上不等于测的是对的 commit。**
  本次全部实证在隔离 worktree `../wt-verify-T005r` 中进行，`git rev-parse HEAD` 回显
  `631a9a849f5b95cf5b063a32e5d9f91016f54a93`，**已核实**。worktree 已从主仓库拆除。
- **判定：PASS（verified）** —— 三条 `fix_items` 全部闭合，三条锚全部对上。

> 标注约定：**【实测】**= 本次亲自运行命令并观察输出；**【推断】**= 未单独跑命令验证。

## 一句话交代进度延迟
本任务 `verifying` 落盘于 13:17:50Z，我**未收到派发通知**，上一轮收尾的重扫早于该时刻、
两集合皆空后已按纪律转入空闲（`TeammateIdle` 是转 idle 前的一次性拦截，放行后不再唤醒）。
由 Leader 的催办发现，随即直读文件确认并开工。**延迟的 15 分钟内零工作，不是在深挖。**

---

## 一、基线

```
worktree @ 631a9a849f5b95cf5b063a32e5d9f91016f54a93
$ go test -count=1 -v ./internal/macro/bitemporal/
   PASS=137  SKIP=0  FAIL=0
$ go test -cover   → coverage: 100.0% of statements
$ go vet ./internal/macro/...  → exit 0，无输出
$ gofmt -l internal/macro/bitemporal/  → 无输出
```

**631a9a8 只改了两个 `_test.go`**（`classify_test.go` +47 / `lookup_test.go` +157-21），
生产代码全部未动。逐 commit blob hash 核验【实测】：

| 文件 | blob | 跨 commit 范围 |
|---|---|---|
| `spec.go` | `7aee7f42be28649c3fd9fa7a971f070d65240fd7` | `224c960` → `e0e46e3`（**整个 Sprint 全同**） |
| `classify.go` | `ef25681f3a91306c33cd78f6784ea40b82e6b2a5` | `96641ec` → `631a9a8` 全同 |
| `lookup.go` | `7c8b6745633de81f7372624ccaffd752aa6e6d8a` | `f2205ac` → `631a9a8` 全同 |
| `query.go` | `6cd91756e7d8bcd04bc241a1e6e135e2b300298e` | `89bc09c` → `631a9a8` 全同 |

⇒ **Q3 的 `scope_note`（`classify.go` 生产代码不得改）严格遵守**；
且三条修复全部是**纯补测试**，未触碰任何被测实现——这本身就排除了「改实现让测试变绿」的可能。

---

## 二、三条 fix_items 的原文变异判据 —— 三条锚全部对上【实测】

| | 判据 | dev-40 与 qa-agent-9 的锚 | **我的独立测量** | 一致 |
|---|---|---|---|---|
| **MQ1** | `!latest.Valid` → `latest.String == ""` | KILLED 131/137（红 6） | **KILLED PASS=131，红 6** | ✅ |
| **MQ2** | 对 `i>0` 的列裸拼注入载荷 | KILLED 130/137（红 7） | **KILLED PASS=130，红 7** | ✅ |
| **MQ3** | `Classify` 改时间序 | KILLED 134/137（红 3） | **KILLED PASS=134，红 3** | ✅ |

**三条互不知情的路径量出同一组数，无第三变量。** 红的归属也逐条核对过，是我以为的那个原因：

- **MQ1** 红在 `TestLookupDistinguishesEmptyRevisionFromMissingKey` 与
  `TestLookupFeedsClassifyOnEmptyRevision` ——正是 Q1 要求补的那两条（含端到端接缝那条）。
- **MQ2** 红全部在 `TestLookupRejectsInjectionInKeyValues`，且失败消息逐列指名
  （`放在列 period_type 时绝不能命中任何行`）——正是 Q2 要求的「逐列覆盖」在起作用。
- **MQ3** 红**排他性地**只有 `TestClassifyComparesLexicographically`（父 + 两个子用例）。
  ⇒ 原有的全部 classify 测试对这个变异**贡献为零**，与 QA「125 条无一转红」的诊断吻合。

### MQ3 靶点的一处自我更正（记录取证过程）
我第一次构造 MQ3 时把 `classify.go` 的 import 靶写成了 `import (`，而实际是
`import "fmt"` 单行形态 ⇒ harness 报**「靶不存在/不唯一(0) —— 变异失败」并拒绝替换**。
这正是「靶不存在即失败、禁止静默 sed」这条纪律的作用：
**若 harness 静默跳过，我会拿到一个「未施加变异」的全绿,并可能读成「MQ3 存活」。**
修正靶点后重跑得到上表结果。

---

## 三、两处「设计选择是否承重」的对照实证

dev 在两条新测试里做了刻意的设计选择并写了理由。**理由写得对不等于选择承重**——我各造了一个对照。

### 3.1 Q3①：取值刻意选「能被 `time.Parse` 解析的 RFC3339」——**承重**【实测】

dev 的理由：若用 `"2026-7-15"` 这类解析不了的形态，按时间序的实现会**退回字典序兜底**，
变异反而存活，这条测试就白写了。

| 对照 | 结果 |
|---|---|
| A：测试保持 RFC3339 取值 + MQ3 | **KILLED**，PASS=134，红 3（全在该测试） |
| B：把取值弱化为 date-only（非 RFC3339）+ **同一个** MQ3 | **存活，PASS=137，无一转红** |

⇒ **理由实证成立**。若取值选错，这条测试会「写了等于没写」——而且它仍然全绿，
没有任何信号提示它是空的。

补充：我另跑了 **MQ3b（严格时间序，解析失败退 `New`、不退字典序）** → KILLED，PASS=115、红 22。
它红得更广，因为 date-only 的 revision 解析不了。⇒ **MQ3（带兜底）才是更难的那个变异**，
而它已被守住；dev 挑的正是难的那个。

### 3.2 Q1：新测试里的「对照组」——**承重**【实测】

dev 的理由：只断言「空串 revision 判 `true`」的话，「一律返回 `Exists:true`」这种实现也能全绿。
它加了对照组（真的不存在的键仍须 `false`）。

我造了那个实现（删掉 `!latest.Valid` 分支），**只跑 Q1 那条测试**（这才是它那句话的适用范围）：

```
--- FAIL: TestLookupDistinguishesEmptyRevisionFromMissingKey/{hestia,crisis}
    Messages: 形状 hestia（表 hestia_observations）确实不存在的键查询失败：这个键真的不存在，必须是 false
```

⇒ **红的正是对照组那条断言**，理由成立。

---

## 四、我另加的 8 个变异

共 **11 个变异（含 MQ1/MQ2/MQ3），10 KILLED / 1 存活**。

| ID | 变异 | 结果 |
|---|---|---|
| MX1 | 只对**末列**裸拼（Q2 的反证方向） | KILLED PASS=127，红 10，全在注入测试 |
| **MX2** | **逐列裸拼但用 `%q`（双引号系，即我上轮报的 F1 缺口）** | **KILLED PASS=130，红 7** —— 见 4.1 |
| MX3 | Lookup 一律返回 `Exists:true` | KILLED PASS=121，红 16 |
| MX4 | `Classify` 的 `>` 改 `>=` | **存活** —— 见 4.2 |
| MX5 | `Classify` 忽略 `Exists`（直接比 revision） | KILLED PASS=125，红 12（含 `TestLookupFeedsClassify`） |
| MX6 | 空串 revision 返 `Exists:true` 但 `LatestRevision` 填残值 `"0000-00-00"` | KILLED PASS=134，红 3 |
| MQ3b | 严格时间序（不退字典序） | KILLED PASS=115，红 22 |

### 4.1 我上轮报的 F1（双引号载荷缺口）—— **已实证闭合**【实测】

上一轮验 `f2205ac` 时，`%q` 形态的拼接**存活于全套 120 条**，我用探针证明载荷
`x" OR 1=1 --` 能实际命中（返回全表 MAX `2026-12-31`）。

现在 `631a9a8` 的载荷清单已含 `` `x" OR 1=1 --` ``（`lookup_test.go:328`）。
**同一个 `%q` 变异现在 KILLED**，且我核了归因——红**排他性地**由双引号载荷造成：

```
Messages: 取值 "x\" OR 1=1 --" 放在列 period       时绝不能命中任何行
Messages: 取值 "x\" OR 1=1 --" 放在列 period_type  时绝不能命中任何行
Messages: 取值 "x\" OR 1=1 --" 放在列 ts / indicator 时绝不能命中任何行
Error:    Should be empty, but was 2026-12-31          ← 正是我上轮观测到的那个值
```

⇒ **缺口在「每套形状 × 每个列位置」上全部闭合**。
这也印证了 Q2 与我那条 F1 的**正交性**：F1 补的是载荷**字符类**，Q2 补的是载荷**位置**；
`%q` 变异需要**两者同时到位**才打得中——只有双引号载荷但固定首列、或只有逐列但全是单引号载荷，
都杀不掉它。**两条修复合起来才构成守护，任何一条单独都不够。**

### 4.2 唯一存活的 MX4 是**可证明的等价变异**（比三条自证更强的理由）

三条自证齐备：`diff=5 行(非空)` / `go vet exit=0` / `PASS=137 == 基线 137`。

但存活的理由不必停在自证——**它可以被证明**：

```go
switch {
case incoming == s.LatestRevision:   return Duplicate   // ← 先于下一条
case incoming >  s.LatestRevision:   return Revision
default:                             return OutOfOrder
}
```

第一个 case 已捕获全部 `==` 的情形，**求值到第二个 case 时 `incoming != s.LatestRevision`
恒成立**，故 `>=` 与 `>` 在该位置行为完全相同。⇒ **真等价变异，不是缺口。**

（这与本 Sprint 早前登记的「TASK-002 `boundary[1]` 指定的 `<` 在 `Classify` 里不存在」同源——
`Classify` 的分支结构使若干比较符变异天然等价。）

---

## 五、Q4 / Q5 的登记核验【实测】

两条均为 `SUGGESTION` 且明示本 Sprint 不修，我只核「是否已登记进 discovery」——**已登记，且带不修理由**：

- **Q4**：`spec.go` 键形状 3 个错误出口只有 `require.Error`；风险是忽略 error 的调用方拿到
  `zero()==false` 的 Spec **穿过 `Lookup` 的零值闸门**（即契约 T5 有绕行路径）。
  不修理由：需改 `spec.go`（属已 verified 的 TASK-001）。
- **Q5**：`query.go` 的标识符闸门按名字硬编码 `queryAlias`；建议改为遍历登记表。
  不修理由：重构超出返工范围。

**未作为返工项处理。**

---

## 六、一处不据以判 REJECT 的说明

`non_functional[0]` 写着「本任务自身无代码提交」，而三条 `fix_items` 都要求改（测试）代码。
Leader 已声明这是 DoD 撰写时的错、状态迁走后无权修改、将在 `accepted` 时一并更正。
**本报告采纳该说明，不据此判 REJECT。**【记录采信】

（附一句事实：三条修复**确实没有提交任何非测试代码**——四个生产文件的 blob hash 全程未变，
见第一节。所以若把该条读作「不得改生产代码」，交付是满足的；冲突只在字面。）

---

## 七、判定

**PASS（verified）。**

依据（全部为实际运行命令的输出）：
1. 三条 `fix_items` 的**原文变异判据**逐条 KILLED，且 **131 / 130 / 134 三个数与两条独立路径完全一致**；
2. 红的归属逐条核对，均为预期的那条测试（MQ3 更是排他性只红新增的那一条）；
3. 基线 137 PASS / 0 SKIP / 0 FAIL，覆盖 100%，vet exit 0，gofmt 干净；
4. 11 个变异 10 KILLED / 1 存活，存活那条经**逻辑证明**为真等价变异；
5. 两处设计选择（Q3 的 RFC3339 取值选型、Q1 的对照组）各用对照实证为**承重**；
6. 生产代码四个文件 blob hash 全程未变，`classify.go` 的 `scope_note` 严格遵守；
7. **我上轮遗留的 F1（双引号载荷）已实证闭合**，且与 Q2 的位置维度**正交互补**；
8. Q4/Q5 已登记进 discovery 并附不修理由。

**无新增阻断项。**

## 收尾
验证 worktree `../wt-verify-T005r` 已从主仓库以绝对路径 `git worktree remove` 拆除，
拆除前 `git status` 为空（无残留改动）；全部变异注入后均逐个还原并以 md5 核实；
临时探针文件已删，未进入任何提交；主工作区无污染。
