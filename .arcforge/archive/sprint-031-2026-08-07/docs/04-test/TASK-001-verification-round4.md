# TASK-001 验证报告（第四轮 / review_fix 复核）—— `Topics()` 断言三层改造

- 验证者: test-agent-17
- 被验对象: `6a2a8df15816e32311b88c32d9f79dfc76c92474`（`test(collector): Topics 断言改集合等值，三层各守一个被测对象`，`policy_test.go` +26/-9）
- 验证环境: 独立 worktree `../wt-v001r4`（detached @ 6a2a8df），`.arcforge/` 读写在主仓库
- assignment_epoch: **3** / rework_count: 2（本次 review_fix 按 Leader 裁定**不消耗**额度）
- 前序：首轮/二轮/三轮报告 + 三轮增补（本轮修的正是三轮增补里的 R4/R5 遗留）

## 结论：**PASS（verified）**

`fix_items` 要求的五条自证（M26/M26b/M26c/M26d/M27）**全部转红**，闭环 2×2 对照成立
（旧形态下 M26c/M26d 确实存活），`want` 的 9 个主题名与实现直读结果**逐字一致**。
覆盖率 97.3%、36 测试 **0 SKIP**、`-race` 绿、collector 树 17 包回归绿、
C3/gofmt/vet 干净、scope 仅 `policy_test.go`。

**`policy.go` 的 md5 仍是 `89da8d213e62570dc8c90dee89d1ddd4`** —— 与前三轮完全相同，
实现自二轮 `ce73488` 后再未改动，本次是纯测试侧加固。

---

## 一、三层改造的形态审查

```go
① want := []string{...9 个主题名...}          // 集合等值：守 Topics()
   got := tbl.Topics(); sort.Strings(got); sort.Strings(want)
   if len(got) != len(want) || !reflect.DeepEqual(got, want) { t.Fatalf(...) }

   for _, topic := range got {
②     p, ok := tbl.Lookup(topic)              // 守 Lookup() 本身
       if !ok { t.Errorf("...必须能被 Lookup 命中"); continue }
③     if p.TTL != 0 { t.Errorf("...TTL 须归零") }   // 守 DisableTTL()
   }
```

**dev 选的是 `sort` + `DeepEqual` 切片比较，不是我建议的 map 方案。这个选择更严格。**
map 会**去重**（`got[topic]=true`），必须靠 `len` 比较兜住重复；排序后的切片比较是
**多重集（multiset）等值**，重复项天然不等。dev 还保留了 `len(got) != len(want)` 前置检查
（`DeepEqual` 已覆盖长度，属冗余但无害）。

`want` 的 9 个主题名要求「从实现直读、不凭记忆写」，已核对：

```
grep -o 't\.Set("[^"]*"' policy.go | sed 's/t.Set("//;s/"//' | sort
→ lixinger.* / tushare.daily / tushare.daily_basic / tushare.hk_daily /
  tushare.index_daily / twelvedata.time_series / yahoo.chart / yahoo.eps / yahoo.quote
```

与测试里的 `want` **逐字一致 ✅**。

---

## 二、`fix_items` 五条自证 + 闭环 2×2（独立复现）

变异有效性双校验：`git diff --numstat` 非空 **且** `go vet` 通过（理由见 §四）；
还原后 `md5` 与基线断言相等。全部定向 `-run TestDisableTTLKeepsThrottle`。

| 变异 | 新形态（集合等值） | 旧形态（基数 + 逐元素命中） |
|---|---|---|
| **M26** `Topics()` 恒返空切片 | **红 ✅** `Topics() = [], want [lixinger.* …]` | — |
| **M26b** 返 9 个不存在的名 | **红 ✅** `Topics() = [n.a n.b …]` | — |
| **M26c** 返 9 个 `lixinger` 域内假名（通配命中） | **红 ✅** `Topics() = [lixinger.a …]` | **绿 ❌ 存活** ← 闭环项 |
| **M26d** 返同一主题重复 9 次 | **红 ✅** `Topics() = [yahoo.chart ×9]` | **绿 ❌ 存活** ← 闭环项 |
| **M27** `DisableTTL` 只清 `lixinger.*` | **红 ✅** `tushare.daily: …TTL 须归零, got 5m0s` | — |
| 无变异（对照） | 绿 | 绿 |

**闭环成立**：唯一的「该红却绿」格恰在旧形态 + M26c/M26d，证明改造前确实漏检；
两形态对正确实现都不产生假红。按第二轮 TASK-002 确立的标准，这四格跑在**同一 worktree、
同一次会话**内，绿/红差异只能归因于第①层的形态改变。

---

## 三、第②层不可省略性：论证边界的据实记录（不影响判定）

dev 在第②层注释里写：

> 若 `Lookup` 恒返 `ok=false`，① 仍通过（`Topics()` 是对的）、③ 也仍通过（`p` 是零值、
> `TTL` 恰好为 0），漏检。

我补造 **M30**（`Lookup` 恒返 `ok=false`）实测：

| 场景 | 结果 | 捕获者 |
|---|---|---|
| 保留第②层 + M30 | **红** | 第②层 — `tushare.daily: Topics() 返回的主题必须能被 Lookup 命中` |
| **删掉第②层 + M30** | **仍红** | **函数末尾的第四条断言** — `限流不受缓存开关影响, got 0s` |
| 删掉第②层 + 无变异 | 绿 | — |

**结论**：在**整个测试函数**范围内，第②层不是 M30 的唯一守护者——函数末尾还有一条
`tbl.Lookup("yahoo.chart").MinInterval != 500ms`，它同样依赖 `Lookup`。dev 的论证在
「①②③ 这三层内部」是成立的，只是没把第四条断言算进来。

**这不构成缺陷，第②层也不该删**：
- 在①成立的前提下，②守的确实是「`Topics()` 与 `Lookup` 的一致性」这个独立性质；
- 它的错误信息精确得多（直指不一致），而末尾那条报的是「限流不受缓存开关影响」——
  对「`Lookup` 坏了」这个根因是**误导性**的。

建议（可选，措辞级）：把注释改为「**在①②③ 这三层内部**，②是 `Lookup` 的唯一守护者」。

---

## 四、我自己的一次无效变异（方法论修正，建议进契约）

跑闭环第①层时，第一版旧形态构造**跑出了红**，与三轮增补的实测（M26c/M26d 在旧形态下存活）
矛盾。按「先证明变异有效，再解释红绿」的纪律先怀疑构造——查实际输出：

```
./policy_test.go:7:2: "reflect" imported and not used
./policy_test.go:8:2: "sort" imported and not used
FAIL	github.com/newthinker/atlas/internal/collector/policy [build failed]
```

**删掉集合等值块后 `reflect`/`sort` 成了未使用 import，编译失败**，而我的 runner 把
输出里出现 `FAIL` 一律判为「捕获」。这是**假红**。

> **一直在防「没改代码的绿」，却栽在「改坏代码的红」上。**
> `git diff --numstat` 非空只证明文件被改了，**不证明改对了**。
>
> **变异有效性校验应是两条：改动量非空 + 编译/vet 通过。**
> 编译失败的红是构建的红，不是断言的红。

值得注意的是 test-agent-16 首轮的 M15 栽过同类问题（`domainOf` 改成 `return topic` 导致
`strings` 未使用而编译失败，它标注为「无效变异」），**我读过那份报告仍然重犯**——
说明只在报告里记一句不够，得进操作纪律的检查清单。修正 runner（加 `go vet` 门）后，
结果与三轮增补完全一致。

---

## 五、覆盖率、race、回归、约束、scope

| 项 | 结果 |
|---|---|
| 覆盖率 | **97.3%**（`Topics`/`DisableTTL`/`Lookup` 均 100%；`domainOf` 66.7% 属非 DoD，历轮一致不计入） |
| 测试 | **36 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿（worktree 与主工作区各跑一次，均 ok） |
| C3 不循环导入 | `go list -deps \| grep newthinker/atlas` → **仅 policy 自身 ✅** |
| gofmt | 无输出 ✅ |
| vet（`./internal/collector/...`） | exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | 仅 `internal/collector/policy/policy_test.go`（26+/9-），落在声明内 ✅ |
| 实现零改动 | `policy.go` md5 = `89da8d213e62570dc8c90dee89d1ddd4`，与前三轮**逐字节相同** ✅ |

`gate.go` / `gate_test.go` 未被触碰，TASK-002 的验收结论不受影响。

---

## 六、三轮增补的三条逃逸路径现状

| 路径 | 三轮增补时 | 本轮 |
|---|---|---|
| M26c（`lixinger` 域内假名，通配命中） | 存活 ❌ | **捕获 ✅** |
| M26d（同一主题重复 9 次） | 存活 ❌ | **捕获 ✅** |
| M26c + M27 组合（架空 boundary[0]） | 全绿逃逸 ❌ | **捕获 ✅**（M26c 单独即被第①层拦下，组合无从形成） |

**三条全部封堵。** boundary[0]「DisableTTL 令 `Topics()` 全部主题 TTL 归零」现在有了
不可绕过的守护。

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v001r4 6a2a8df15816e32311b88c32d9f79dfc76c92474
cd ../wt-v001r4

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race        # 36 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t.out                             # 97.3%
GOTOOLCHAIN=local go test ./internal/collector/... -count=1                  # 17 包全绿

# want 与实现一致性核对：
grep -o 't\.Set("[^"]*"' internal/collector/policy/policy.go | sed 's/t.Set("//;s/"//' | sort

# 闭环 2x2（第①层）：把 Topics() 实现整段替换为各变异体；旧形态 = 把集合等值块换回
#   `if len(got) != 9 { t.Fatalf(...) }`，**并一并删掉 reflect/sort 两个 import**
#   （否则编译失败，红是假红——见 §四）
# 每格：git checkout 复位 → 构造 → git diff --numstat 断言非空 → **go vet 断言通过**
#      → go test . -run '^TestDisableTTLKeepsThrottle$'
# 期望：仅「旧形态 + M26c」与「旧形态 + M26d」为绿（漏检），新形态五条全红。

# 基线 md5（还原后须复原到）:
#   policy.go      = 89da8d213e62570dc8c90dee89d1ddd4   （与前三轮相同 → 实现零改动）
#   policy_test.go = 3e49623f33f8090d2eaddb3a38c67867
```

worktree 已于验证结束后 `git worktree remove ../wt-v001r4` 清理；主工作区 `internal/` 零污染。
