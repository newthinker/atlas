# TASK-001 验证报告 — StockContinuityMax 按 period_type 分档

- **验证者**：test-m1c2-v1
- **判定**：**VERIFIED**
- **被验对象**：`0a05c4e43c514efb2c516692d480a5c00999f07c`（merge commit；两个 commit `bfc2ca8` 主改动、`fa7c70f` 补守卫）
- **基线**：`fba0feb1e5ca8ae65277b9957e76b8acc1d7f1bf`
- **验证时主仓库 HEAD**：`6bb60b4689eaecc8cd6979d17cd3c2b4f2164ae7`
- **漂移**：`verify_baseline.head` == 当前 HEAD == `6bb60b4…`；`discovery_sha256` == `8d8d6e23…` 一致
  ⇒ **零漂移**，转 `verified` 不需要 `--ack-drift`
- **测自哪棵树**：全部数字采自隔离 worktree `../wt-verify-TASK-001 @ 0a05c4e4…`，
  基线数字采自 `../wt-verify-T001-base @ fba0feb1…`；主工作区一个字节未碰（收尾指纹核对通过）

---

## 一、Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 证据 | 判定 |
|---|---|---|---|---|
| **F1** | `StockContinuityMax` 改 map，五键齐全且 >0，两档，**断言须含 `monthly < annual`** | `TestDefaultStockContinuityIsTieredByPeriodType` | 消融 D（五档写成同一个数）→ 红在 `thresholds_test.go:61`，正是 `assert.Less(m["monthly"], m["annual"])` | PASS |
| **F2** | `gateStockContinuity` 查表，**缺键 ⇒ `skipped{no_threshold:<pt>}` 而非 `failed`** | `TestStockContinuitySkipsWhenPeriodTypeHasNoThreshold`（2 子测试） | 消融 B（缺档改判 `CheckFailed`）→ **只**红这 2 个子测试 | PASS |
| **B1** | 标量报错在 **Unmarshal 层**，且**不得写 scalar 死分支** | `TestLoadConfigRejectsLegacyScalarStockContinuity` | 见 §三「不可达」直接观测：标量路径进入 `checkStockContinuityComplete` **0 次** | PASS |
| **B2** | 漏档必报错并点名缺的键；**完全不写必须成功**（成对） | `TestLoadConfigRejects/显式写了_map_但漏一档` + `TestLoadConfigKeepsDefaultsForOmittedThresholds` | 消融 A → 只红前者；消融 F（拿掉 `IsSet` 守卫）→ 红后者 ⇒ 两格确实成对 | PASS |
| **B3** | 值非正报错；**三格 want 互不相同**（验收：任一格 want 换成另一格，该格必须红） | `TestLoadConfigRejects` 两格 + 标量独立测试 | 消融 C1–C4 四组互换，**每次只红被换那一格** | PASS |
| **E1** | 装载侧与运行侧**各自单独红** | 同 F2/B2 | 消融 A 只红装载那条、闸门那条不红；消融 B 只红闸门那条、装载那条不红。**两个方向都验了** | PASS |
| **N1** | YAML 改五键块，且 CLI 能装载 | `TestRealHestiaConfigLoads` | `go run ./cmd/atlas hestia status --hestia-config configs/hestia.yaml` **exit 0** | PASS |
| **N2** | 两条会过期的注释都改，带 `M1c-2 的 TASK-001` 前缀 | —（review 性质） | `ingest_test.go:485` 与 `validate.go:343` 两处均已改写并带前缀，见交付 diff | PASS |
| **N3** | gofmt / vet / test / build / 覆盖率不低于基线 / 无新依赖 | — | 见 §二 | PASS |

---

## 二、N3 逐项证据（全部跑出来的，非推出来的）

| 项 | 结果 |
|---|---|
| `go build ./...` | OK |
| `go vet ./...` | 退出码 **0**，输出 **0 字节**（退出码单独取，未跨管道） |
| `go test ./... -count=1` | 退出码 **0**，**64** 个 `ok` 包，零 FAIL |
| `gofmt` (a) 本任务 8 个 `.go` | `gofmt -l` 输出 **0 行** |
| `gofmt` (b) 全仓 base vs post | 各 **28** 个存量未格式化文件，**逐字节一致** ⇒ 无一由本任务引入 |
| G2 无新增依赖 | `git diff --name-only fba0feb1…..0a05c4e4… \| grep -E 'go\.(mod\|sum)'` → 无匹配 |
| scope 越界 | 实际改动 **9** 个文件，与声明 `writes` 的 9 条**逐条相同**，无越界 |

### 覆盖率（背对背、同轮同负载、各自树内渲染）

NumStmt 加权，用**我自写**的脚本计算（未复用被验对象或 dev 的任何脚本）：

| 包 | base `fba0feb1…` | delivered `0a05c4e4…` | 结论 |
|---|---|---|---|
| `internal/hestia` | 94.810% (1370/1445) | **94.874%** (1388/1463) | 上升 |
| `cmd/atlas` | 75.610% (1023/1353) | **75.610%** (1023/1353) | 持平 |

两轮独立采样数字逐字节一致。

**这组数字有一个不由我产生的锚**：我的独立脚本在 base 树上**逐位复现** DoD 里写死的
`94.810% (1370/1445)` 与 `75.610% (1023/1353)`，连原始语句计数都一致 —— 那两个数写在
我介入之前，故它构成对我计算口径的外部校验，而非自证。

**第二口径对账**（`go tool cover -func` 的 total，各自树内渲染）：
`internal/hestia` 94.8% → 94.9%、`cmd/atlas` 75.8% → 75.8%。**两个口径方向一致**。
`cmd/atlas` 的 75.610 vs 75.8 是已知口径差（`-func` 只统计落在已解析函数外延内的 block），
两个口径下都是**持平**，不构成矛盾。

---

## 三、重点复现：dev 自查发现的那个洞，独立确认它真的补住了

### E2 —— 没有守卫时，退化是否完全静默

做法：把 `validate_test.go` 换成 **`bfc2ca8` 的版本**（即 dev 加守卫之前的真实状态，
而不是手工 `if false` 造一个假的对照组；`fa7c70f` 只动了这一个文件，故等价），
再把「`monthly 恰好在阈值上`」那格的 `periodType` 由 `"monthly"` 改回 `"h1"`。

```
802c802
< 		{"monthly 恰好在阈值上", "monthly", 400, 408, CheckPassed, 0.02},
---
> 		{"monthly 恰好在阈值上", "h1", 400, 408, CheckPassed, 0.02},
```

结果：`go test ./internal/hestia/ -count=1` → **退出码 0，零条红**。
⇒ **dev 的说法成立**：`wantStatus` 与 `wantRatio` 都挡不住（`0.02 <= 0.15` 照样 Passed、
比例照样 0.02），这次退化在无守卫时**完全静默**。

### E1 —— 守卫在场时红的是哪一条（本次验证最关键的一格）

同一变异，`validate_test.go` 用交付版（sha256 `c83d79e4…`）：

```
退出码 = 1
Error Trace 条数 = 1
红的行号 = validate_test.go:835
Messages: 这格声称恰好落在阈值上，就必须真的落在上面——改 periodType 或改 prev/cur 常量…
```

**`validate_test.go:835` 正是新守卫本身**（`if tt.atBoundary {` 在 834，`require.Equal(...)` 在 835）。

**关键排除项**——「是不是兄弟断言先红」：

| 行 | 断言 | 类型 | 本次是否报错 |
|---|---|---|---|
| 828 | `assert.Equal(wantStatus, c.Status)` | `assert`（**非致命**） | 否 |
| 829 | `require.NotNil(c.Value)` | require | 否（通过） |
| 830 | `assert.InDelta(wantRatio, *c.Value, 1e-9)` | `assert`（**非致命**） | 否 |
| **835** | **`require.Equal(阈值, *c.Value)`** ← 新守卫 | require | **是，且是唯一一条** |

828 与 830 是 `assert.*`：即便失败也会**照常记录**而不中止，因此「它们先红把守卫遮住」
这条路径在此**结构上不成立**。加上全程只有 **1 条** Error Trace，
⇒ **杀手确实是新守卫，不是兄弟断言的副作用**。上个 Sprint 那次「四格全红、自证判据信息量为零」
的形态在这里没有发生。

### 「不可达」那句注释：构造出来跑了一遍

`config.go` 注释声称「旧格式标量**到不了这里**，所以不为标量写分支」。这是一句
「否则/防的是」型声明，按纪律必须能指出观察。做法：在 `checkStockContinuityComplete`
入口插一行 `println` 探针（`panic` 会让 `go vet` 因 unreachable code 报错，故改用 println），
跑一对正负对照：

| 输入 | 探针命中次数 | 结论 |
|---|---|---|
| 旧格式**标量** YAML | **0** | 确实到不了 ⇒ 那段 scalar 死分支若写了，就是假装守卫存在 |
| **漏一档** map YAML | 命中 | 函数整体并非死代码（阳性对照成立） |

⇒ **注释所述属实**，且 dev「不写那段死分支」的决定正确。

---

## 四、我自己补的消融（dev 的清单里没有）

| ID | 变异 | 结果 | 意义 |
|---|---|---|---|
| **D2** | 默认表 `monthly` 0.02 → 0.03 | 只红 `monthly 恰好在阈值上` | DoD F1 写了「取值两档：monthly 0.02」。tiered 测试只断键集/正负/序关系，**不断具体取值**；我原本怀疑 0.02 没被钉住。实测被 `atBoundary` 守卫**间接钉死** |
| **D3** | `q1/h1/q1_q3/annual` 一起 0.15 → 0.16 | 红 `annual 恰好在阈值上` + `annual 超过阈值` | 同上，0.15 也被钉住。这个变异**绕得过** tiered 测试（`monthly<annual` 与四档相等都仍成立）。🔴 **订正**：本行初稿写了「只有边界格拦得住」，**是错的**——同一格记录的红格里 `annual 超过阈值` 就不是边界格（`atBoundary=false`，它经 `wantStatus` 转红）。**证伪它的数据就在同一行**，我写时跳过去了。补跑确认：换成无 `atBoundary` 的 `bfc2ca8` 测试表后，D3 **仍被 `annual 超过阈值` 抓住**（退出码 1，红 1 格）。⇒ **D3 不构成「不能回退 `fa7c70f`」的证据**；构成该证据的是 E2（零条红）与 §六 的向下畸变（退出码 0，完全静默）|
| **F** | 拿掉 `if !v.IsSet(...)` 守卫 | 红 `TestLoadConfigKeepsDefaultsForOmittedThresholds` 等 8 个 | DoD B2 要求「两格成对」。dev 的 decisions 里论证了这一点但**消融清单里没有对应项**，由我补上 —— 成对关系是真的 |
| **G** | `defaultStockContinuityMax` 改成返回**共用** map | 红 5 个测试 | discovery 把「每次返回新 map」列为接口契约。契约若无守卫就是空声明；实测有守卫，非空声明 |

---

## 五、Reality Checker 的保留意见（不影响本次判定）

以下都**不是** DoD 违反，也不构成 reject 理由，但值得记进 M1c-3 的交接：

1. **`0.02` / `0.15` 仍是占位数。** 代码注释、YAML 注释、discovery 三处都明确说了这一点，
   诚实且一致 —— 我在此只是确认它没有被包装成结论。本任务交付的是**分档机制**，不是标定值。
2. **消融 G 红的那 5 个测试带顺序依赖。** 共用 map 被 `delete` 污染后波及哪些测试，
   取决于执行顺序。它**会**红（非静默），但「红哪几个」不稳定。不必改，记一笔。
3. **`no_threshold` 优先于 `absent_field` 是 dev 自选的**（DoD 未锁），
   已在 `validate.go` 注释写明理由并由测试钉住，改回去是一行 + 一条断言。合规。

---

## 六、追加检查：`InDelta(1e-9)` 容差本身有没有人守（Leader 追加 / dev 自己点名的未验项）

dev 在交付报告里主动划界：E1/E2 只证明了**边界格**的精确相等，同表其余各格的
`InDelta(…, 1e-9)` **容差没有被消融过**，「很可能同样守不住，但这是推出来的」。

### 先答「存不存在一个消融能让它变假」

**只放宽容差、不注入缺陷**这个实验是**零信息量的**：`InDelta` 放宽容差是**单调削弱**，
数学上不可能让任何原本通过的断言转红。SURVIVED 是必然结果，不是证据。实跑确认：

```
[无缺陷 tol=1e-9] 退出码=0    [无缺陷 tol=1e-2] 退出码=0
```

⇒ 有意义的实验必须是**两因子**：注入真实缺陷 × 对照两种容差。

### 两因子消融（缺陷注入在 `validate.go:393` 的比例计算上）

| 注入的缺陷 | tol=1e-9 红几格 | tol=1e-2 红几格 |
|---|---|---|
| P1 分母 `prev`→`cur` | 7 | 3 |
| P2 比例 ×1.001 | 7 | 2 |
| P3 比例 ×1.000001 | 7 | 2 |
| P4 分子丢 `math.Abs` | 1 | 1 |

**四个缺陷在两种容差下都被抓住（退出码均为 1）**。但红的格数从 7 掉到 2 ——
tol=1e-2 时那 **4 个非边界格不再贡献任何信息**。

### 决定性的一格：**向下**畸变

上表 P2/P3 在 1e-2 下仍红，原因**不是** `atBoundary`（我先假设是，实测证伪），而是
**向上**畸变会把恰好落在阈值上的格**推过阈值**，`wantStatus` 因此翻转。
所以真正的判别式是**向下**畸变（不翻转任何 status）：

`r := math.Abs(cur-prev) / math.Abs(prev) * 0.999`

| | tol=1e-9 | tol=1e-2 |
|---|---|---|
| `atBoundary` 在场（**交付态**） | 红 7 格 | 红 2 格（边界格的精确相等） |
| `atBoundary` 不在场（`bfc2ca8`） | 红 7 格 | **退出码 0 — 完全静默** |

### 结论（区分「跑出来的」与「推出来的」——以下全是跑出来的）

1. **按交付态，dev 的猜测不成立**：我构造的四类缺陷**没有一个**能在 tol=1e-2 下逃脱。
   容差即便被放宽 7 个数量级，套件整体仍抓得住。**故这不是缺陷，不构成 reject。**
2. **但容差确实是承重的**：那 4 个非边界格的比例值，**只**由 `InDelta` 的容差钉住；
   放宽到 1e-2 后它们对任何比例畸变都不再转红。
3. 🔴 **`fa7c70f` 有一个 dev 自己没意识到的第二重作用**：唯一「完全静默」的那一格，
   需要**同时**放宽容差**且**没有 `atBoundary`。也就是说 `atBoundary` 不只补了 dev 描述的
   那次退化，它**顺带把容差参数也兜住了**。

### 一条经自测的加固建议（给 M1c-3，非本任务欠账）

那 7 格的比例全是**精确整数相除**（`8/400`、`4/400`、`20/400`、`60/400`、`64/400`），
与字面量舍入到同一个 double，所以 `InDelta` 可以直接换成精确相等。**我自己跑过才提**：

- 换成 `assert.Equal(t, tt.wantRatio, *c.Value)` ⇒ 当前 7 格**全绿**（可行）
- 再注入 ×0.999 向下畸变 ⇒ **7 格全红**（严格强于 tol=1e-2 时的 2 格）

⇒ 建议但不强制；这是 dev 拿回写权后的一行改动，**验证者不代写**。

---

## 七、`fa7c70f` 超出 DoD 字面 —— 我的验收判断：**保留**

dev 提出「若认为超出 scope，可只回退 `fa7c70f`，`bfc2ca8` 独立自洽」。**我判定保留**，两条理由：

1. 它堵的洞是真的：E2 实测**零条红**，那次退化在无守卫时完全静默。
2. §六 新发现的第二重作用：它同时兜住了 `InDelta` 容差参数。回退它就等于**同时**打开两个洞，
   而其中第二个连 dev 自己都没意识到。

它只增加断言、不改变生产代码行为（`fa7c70f` 的 diff 仅 `validate_test.go` 20 增 7 删），
**不引入 DoD 未要求的运行时风险**。

---

## 八、⚠️ 一条给后来者的记录：绝对覆盖率值不构成基线

本报告 §二 的覆盖率对是 `fba0feb1…（base）→ 0a05c4e4…（交付）`，**两棵树都不含 TASK-002**。

派验时 `verify_baseline.head` 记的是 `6bb60b4…`（当时的 master），而 TASK-002 已在
`0a05c4e4…..6bb60b4…` 之间落进 `internal/hestia`（新增 `calibrate.go` / `calibrate_test.go`）。
**若在 `6bb60b4` 上量 `internal/hestia`，会得到 95.241% (1501/1576)（我实测过，非转述），
分母由 1463 涨到 1576 —— 多出的 113 条语句属于 TASK-002，不属于 TASK-001。** 拿它去对 discovery 记的 94.874% 会看到「对不上的数」，
而最省事的解释恰好是「dev 报错了」——那会变成一次**假 reject**。（本次未发生：我从一开始
就在 `0a05c4e4…` 上量，独立复现出 1388/1463，与 discovery 逐位一致。）

dev 对这件事的定性，原样引用：

> **钉住 sha 只防住「引用时指错树」，防不住「我引用的那棵树被后来者挤到旁边去」。**
> 绝对覆盖率值天生依赖整棵树的语句总数，而那个总数**不属于我的改动**，任何并行任务落地
> 都会改写它。⇒ **跨时间点保存的绝对覆盖率值，在演进中的仓库里不构成基线；只有同一时刻
> 并排产生的一对才构成。**

dev **刻意没去改 discovery**（它被 `verify_baseline.discovery_sha256 = 8d8d6e23…` 锚着，
改它正是 AD-29 要防的「判定之中的漂移」）。这个克制是对的，但代价是下一个读 discovery 的人
读不到这条澄清 —— **故记在此处，本报告是当下唯一无漂移的落点。**

复核方式（锚为全 sha，**不要**用 `HEAD` 或分支名）：

```bash
git worktree add --detach ../wt-v1-base fba0feb1e5ca8ae65277b9957e76b8acc1d7f1bf
git worktree add --detach ../wt-v1-post 0a05c4e43c514efb2c516692d480a5c00999f07c
# 两棵树同一时刻背对背跑，各自在自己的源码树里渲染 profile
```

---

## 九、结论

9 条 done_criteria **逐条有对应测试且断言非空洞**；每条关键断言都经消融证明「拿掉就会红、
且红的是它本身」。全量套件 exit 0、vet 空、build OK、两包覆盖率不低于基线（一升一平）、
无新增依赖、无 scope 越界、零验证对象漂移。

Leader 追加的 `InDelta` 容差检查（§六）**未发现缺陷**：按交付态，无一构造能在容差放宽 7 个
数量级后逃脱。`fa7c70f` 判定**保留**（§七）。

**判定：VERIFIED**
