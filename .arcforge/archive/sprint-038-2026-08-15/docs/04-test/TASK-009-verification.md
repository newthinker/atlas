# TASK-009 验证报告 · 把完整性对账接进回填管线 + 补两处无测试背书缺口（两轮）

| | 第 1 轮 | **第 2 轮（返工）** |
|---|---|---|
| 判定锚 | `dd1090c9ff2ee359f38c619ffe7e7467b829dfd2` | **`37388df5223685112f214f0fb19ac371ed96a766`** |
| 判定 | **REJECTED**（`task_defect`） | **VERIFIED** |
| `functional[2](a)` | 6 条写盘路径 **KILLED 5/6** | **KILLED 6/6** ✅ |
| 其余 8 条 DoD | 全部 PASS | 未重验（三道 diff 闸全绿，见 §R1） |
| epoch / rework | 2 / 0 | **3 / 1** |

- **验证者**：test-agent-28（Reality Checker），两轮同一人
- **漂移**：`verify_baseline.head` 与 HEAD 逐字一致、`discovery_sha256` 逐字一致 `4501e4c9…` ⇒ **零漂移**
- **验证工作区**：`/Users/zuowei/workspace/go/src/github.com/newthinker/wt-verify-T009r`（detached @ `37388df`，收尾已 remove）

---

# 第一部分 · 复验（返工轮，锚 `37388df`）

## R1. 三道 diff 闸 —— **我自己重跑了一遍**（Leader 先跑过，但它今天自报判错四次，故独立复核）

| 闸 | 判据 | 我的结果 |
|---|---|---|
| ① 实现是否被改 | `git diff dd1090c..37388df -- backfill_fetch.go` | **空** ⇒ 「补背书」没有变成「改行为」，不必回到 9 条 |
| ② 越界 | 本次改动文件 | **只有 `backfill_fetch_test.go`**（55 增 / 6 删），在声明 `writes` 内 ⇒ **无越界** |
| ③ 承重用例是否被触碰 | 对 8 条 PASS 的 **10 个**承重用例名逐个 `grep` diff | **10 个全部 0 命中** |

⇒ **复验范围 = D1–D6 + 全量回归，8 条不重验。** 依据是：实现字节不变 ∧ 承重用例字节不变
⇒ 那 8 条的消融结论（第 1 轮已逐条取得）在本树上必然仍成立；新增用例只会**增加**覆盖，不会移除。

Leader 说的「6 行删除 = 4 行错误注释 + 2 行表驱动结构」我核对了 diff，与实际一致。

## R2. 🔴 验收标准：`functional[2](a)` 从 5/6 到 **6/6**

我把 TASK-006 报告 §4.1 的**原样 6 个变异**第三次重跑（TASK-006 → TASK-009 第 1 轮 → 本轮）：

| # | 写盘路径 | TASK-006 | 第 1 轮 | **本轮** | 红在哪一格 |
|---|---|---|---|---|---|
| D1 | 文章正文 | KILLED | KILLED | ✅ KILLED | `…/第三步：文章正文落盘失败` |
| D2 | index 快照 | SURVIVED | KILLED | ✅ KILLED | `…/第一步：栏目页快照落盘失败` |
| D3 | 搜索快照 | SURVIVED | KILLED | ✅ KILLED | `…/第二步：检索页快照落盘失败` |
| D4 | `store.Save()` | SURVIVED | KILLED | ✅ KILLED | `…/第四步：清单落盘失败（根目录不可写）` |
| **D5** | **`store.AppendFailed()`** | SURVIVED | **SURVIVED** | ✅ **KILLED** | **`…/第五步：循环内记录失败时落盘失败`** |
| D6 | 搜索路径 `errBackfillDisk` 例外 | SURVIVED | KILLED | ✅ KILLED | `…/第二步：检索页快照落盘失败` |

**1/6 → 5/6 → 6/6。** 且**每条红都落在自己那一格**，没有一条是被兄弟格带红的。

### R2.1 dev 对「A19 与 A20 是同一失败模式」的推翻，我的数据独立证实

第 1 轮的免除理由链条里有一句「A19/A20 是同一失败模式，第四格已覆盖」。本轮实测：

```
D4（A19）→ 只红「第四步」
D5（A20）→ 只红「第五步」
```

**各自唯一命中 ⇒ 是两个可分辨的失效，不是一个。** dev 的订正成立。
（这句原是 dev 提出、Leader 采信并写进裁决理由与 CONTRACTS 的 —— 两个被推翻的判断都在同一条链上。）

### R2.2 dev 落地的构造与我在第 1 轮交出的一致

`chmodOnArticleFetcher`：被问到文章 URL 那一刻 `os.Chmod(out, 0555)` 再报错 ⇒ 首次 `Save()` 已成功、
循环内 `AppendFailed` 那次失败。判据 `建临时文件于` 来自 `manifestStore.Save` 的 `os.CreateTemp`。
⚠️ 它的注释还标了**我第一版探针踩过的那个坑**——`on` 必须匹配文章 URL 而非栏目页
（栏目页路径里也含数字段，匹配宽了会在 index 那一步就触发）。

## R3. 全量回归与自证数字（我自己的口径）

```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok
go test ./... -count=1              → exit 0（全仓库）
```

**847 = 429 顶层 + 406 二级 + 12 三级；FAIL 0；SKIP 0。**
⇒ 与 dev 报的 847、Leader 核的 847 **三方一致**。（dev 本轮**先写口径再写数**，并注明上一轮的 834
漏了三级那层 ——「口径不完整比口径不同更糟」，这句我同意：口径不同能对上，口径缺失对不上。）

**覆盖率**（`dd1090c` 与 `37388df` 两棵树背对背、各采两轮、**各自树内渲染**）：

| 树 | 第 1 轮 | 第 2 轮 |
|---|---|---|
| before `dd1090c` | 94.5% | 94.5% |
| after `37388df` | **94.6%** | **94.6%** |

可复现地**上升**，`non_functional[0]` 的覆盖率子句满足。

## R4. 复验结论

**VERIFIED。** 唯一不通过的那条（`functional[2](a)`）已闭合且达到我定的验收标准 **6/6 KILLED**；
其余 8 条依三道 diff 闸不必重验（实现字节不变 ∧ 承重用例字节不变）；全量回归全绿；
覆盖率上升；无越界；判定对象零漂移。

⚠️ **本轮的边界**：我只重跑了 D1–D6 与全量回归。**新增的第五格用例本身，我没有为它设计"反向"消融**
（即"把第五格改坏、看有没有别的东西红"）——它是新增的**背书**，其正确性由「D5 变异被它杀死」直接证明，
不需要再套一层。如实写明，免得后人以为本轮覆盖面与第 1 轮等同。

---

# 第二部分 · 附录：第 1 轮报告全文（REJECTED，判定锚 `dd1090c`）

> 逐字保留。其中 §2.2「『scope 内结构上不可构造』经我实测不成立」即本次返工的立项理由。

- **验证者**：test-agent-28（Reality Checker）
- **判定**：**REJECTED**，`reason_class=` **`task_defect`**
- **⚠️ 分类与 Leader 预设的不同**：Leader 说「若判字面不满足，请用 `dod_defect`，因为错在我的 DoD 要求了一件 scope 内结构上不可能的事」。
  **我实测证明那件事在 scope 内是可以做到的**（§2），所以 DoD 没有缺陷，`dod_defect` 不成立。
- **判定锚**：`dd1090c9ff2ee359f38c619ffe7e7467b829dfd2`（= `verify_baseline.head`，零漂移；`discovery_sha256` 逐字一致 `1051614f…`）
- **9 条 DoD：8 条 PASS，1 条 FAIL。** 失败的只有 `functional[2](a)`，**其余 8 条与全部实现行为均无问题，返工时不必重做**。
- **消融**：**27 个有效变异，KILLED 25 / SURVIVED 1 / 等价变异 1**

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 我的消融 | 判定 |
|---|---|---|---|
| functional[0] | 对账被**执行**且产出到得了输出；cutover/expect 由调用方传入 | F0a 整个不调用 / **F0b 调用了但结果被丢弃** / F0c cutover 写死 / F0d expect 写死 / F0e 入参换成空 —— **5 全 KILLED** | **PASS** |
| functional[1] | 四个可见面全部输出，断言**具体内容** | F1a/F1b/F1c/F1d **逐个面各删一次，4 全 KILLED**，且 F1c/F1d 精确红在 `ReportsAllFourFaces` | **PASS** |
| **functional[2](a)** | **6 条写盘路径逐条钉住** | 我在 TASK-006 §4.1 的原 6 个变异重跑：**KILLED 5/6**，`AppendFailed` 那条 **SURVIVED** | **🔴 FAIL** |
| functional[2](b) | 当前 `base` 不造成两侧 URL 分叉 | B1 生产侧 URL 多一个末尾斜杠 —— **KILLED** | **PASS** |
| boundary[0] | 零期次 vs 全部齐全**各自钉住特征文本** | BD0′ 两标签改成同一句 / BD0″ 零期次不输出 / BD0‴ 零期次输出「齐全」那句 / BD0⁗ 齐全不输出 —— **4 全 KILLED**，红全在 `EmptyDiffersFromComplete` | **PASS** |
| boundary[1] | 缺篇非空但无违规 ⇒ 退出码 0 | EH0b 把缺篇也算失败 —— **KILLED** | **PASS** |
| error_handling[0] | 违规 ⇒ 非零，且错误文本含**具体 violation** | EH0a 违规也返回 nil / EH0c 错误文本笼统化 —— **2 全 KILLED** | **PASS** |
| error_handling[1] | failed 与 violations **互不吞没** | EH1a 只报 failed / EH1b 只报 violations —— **2 全 KILLED** | **PASS** |
| non_functional[0] | gofmt/vet/test 干净 + 导出守卫绿 + 不降覆盖率 | 见 §3；另 RW 变异（报告默认改 `io.Discard`）**KILLED** | **PASS** |

---

## 2. 🔴 唯一不通过的一条：`functional[2](a)` 要求 6 条，实得 5 条

### 2.1 事实

DoD 原文：「**(a) 6 条写盘路径逐条钉住**……最低成本做法（验证者给的）：改成表驱动，用一个「第 N 次写盘失败」的 fake **跑满 6 格**。」
所指清单是我在 TASK-006 报告 §4.1 列的那 6 条。我把**原样的 6 个变异**在本树重跑：

| # | 写盘路径 | TASK-006 时 | **本次** | 红在哪一格 |
|---|---|---|---|---|
| D1 | 文章正文 | KILLED | ✅ KILLED | `AbortsOnEveryDiskFailurePath/第三步：文章正文落盘失败` |
| D2 | index 快照 | SURVIVED | ✅ KILLED | `…/第一步：栏目页快照落盘失败` |
| D3 | 搜索快照 | SURVIVED | ✅ KILLED | `…/第二步：检索页快照落盘失败` |
| D4 | `store.Save()` | SURVIVED | ✅ KILLED | `…/第四步：清单落盘失败（根目录不可写）` |
| **D5** | **`store.AppendFailed()`** | SURVIVED | **❌ SURVIVED** | —— |
| D6 | 搜索路径 `errBackfillDisk` 例外 | SURVIVED | ✅ KILLED | `…/第二步：检索页快照落盘失败` |

**5/6**（TASK-006 时为 1/6）。**这是很大的推进**，但 DoD 写的是 6。

**顺带答 Leader 让我实测的那个推测**：D6 是否被第 2 格间接覆盖 —— **是，实测 KILLED**。
⚠️ 但它是**顺带覆盖**不是**逐条钉住**：第 2 格若将来改动，D6 会**静默失去背书**，而没有任何用例的名字提到它。
（不判为缺陷，DoD 只要求「被钉住」；如实记录。）

### 2.2 🔴 「scope 内结构上不可构造」这个理由 —— **我实测证明不成立**

dev 的探针只试了**静态预置条件**（`manifest.json` 预置为只读文件 / 预置为目录），据此得出
「必须让 `Save` 可注入 ⇒ 改 `backfill_manifest.go` ⇒ 不在 writes 里」。

**但那不是唯一的构造方式。** `runBackfill(ctx, f Fetcher, sleep, cfg)` 的 **`Fetcher` 本来就是注入点**，
而且本文件每个用例都在用它。让 fake 在**被问到文章 URL 那一刻**把产物目录改成不可写，
即可让第一次 `store.Save()` 成功、而循环内 `AppendFailed` 的那次失败 —— **正是 dev 说「没有中间态」的那个中间态。**

我在一次性副本里写了这个探针（约 15 行，**只用 `backfill_fetch_test.go`，没碰 `backfill_manifest.go`、没给生产代码加任何注入点**）：

```go
type d5Fetcher struct{ inner *fakeFetcher; out string }

func (f *d5Fetcher) Get(ctx context.Context, url string) ([]byte, error) {
    if strings.Contains(url, "/8001/") {           // 只在文章那次生效，别匹配到 index 栏目页
        _ = os.Chmod(f.out, 0o555)                 // 跑到一半让磁盘“满”
        return nil, errors.New("probe: 单篇抓取失败")
    }
    return f.inner.Get(ctx, url)
}
// …bfConfig + bfSite 造一篇；t.Cleanup 里 chmod 回 0755，否则 TempDir 清理失败
// 断言：err != nil 且 err.Error() 含 "建临时文件于"
```

**实测两格**：

| | 结果 |
|---|---|
| 未变异 | **PASS** — `runBackfill` 返回 `建临时文件于 …/manifest.json.tmp-…: permission denied` ⇒ 确实中止了 |
| 打上 D5 变异（`_ = store.AppendFailed(...)`） | **FAIL** — 返回 `回填完成但有 1 篇抓取失败（…）`，跑完了没中止 ⇒ **该变异被杀** |

⇒ **构造存在、有鉴别力、在 scope 内。**

**而且它与交付物已经在用的手法是同一个**：既有第四格用的就是
`os.Chmod(out, 0o555)` + `t.Cleanup(chmod 回 0755)`。差别只在**施加时点**——
dev 把它当作**跑前的静态预置**，而 D5 需要的是**跑到一半**。

⇒ **不是「scope 不够」，是穷举只覆盖了静态预置这一类构造。** dev 的探针结论「没有中间态」
在**静态构造**的范围内成立，写成一般结论就错了。

### 2.3 为什么是 `task_defect` 而不是 `dod_defect`

`dod_defect` 的定义是「done_criteria 自相矛盾、不可测试或无法实现」。
**这条 DoD 三者皆非**：它可测试、可实现、我已实现给你看。⇒ 缺的是那条用例本身。

⚠️ **这不是说 dev 敷衍**。相反：它跑了穷举探针而不是靠推理、把残留风险明明白白写出来
（「若将来有人把循环内那两处改成记下错误继续跑，第四格不会红」）——**那句话是准确的，
而且正是我实测到的 D5**。它诚实地标出了自己没盖住的地方，只是把「我没找到构造」
写成了「构造不存在」。

### 2.4 返工范围（很小，请照此派）

- **只需补一格**：把上面那 15 行做成 `AbortsOnEveryDiskFailurePath` 的第五格（或独立用例）。
- **实现代码一行都不用改。** 本次 8 条 PASS 的结论对返工后的树仍然有效，
  我复验时只重跑 D1–D6 六个变异 + 全量回归，**不重验那 8 条**。

---

## 3. 我自己跑出来的证据

### 3.1 套件与计数口径（三方数字对不上，这里给出统一解释）
```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok
```
**846 条 RUN = 429 顶层 + 405 二级子测试 + 12 三级子测试**，真 FAIL **0**。

| 谁 | 数字 | 口径 |
|---|---|---|
| dev | 834 | 429 + 405 —— **漏了 12 条三级子测试** |
| Leader | 846 = 429 + 417 | 把二级与三级**合并**成「子测试」 |
| 我 | 846 = 429 + 405 + 12 | 分三层 |

⇒ **三个数都能对上，没有人错，只是口径不同**；总数 846 三方一致。

### 3.2 覆盖率（`559393b^` 与 `dd1090c` 两棵树背对背，各采两轮，**各自树内渲染**）
| 树 | 第 1 轮 | 第 2 轮 |
|---|---|---|
| before | 94.0% | 94.0% |
| after | **94.5%** | **94.5%** |

可复现地**上升**。（与 dev 报的 94.003% → 94.535% 一致。）
⚠️ 渲染必须在各自的树里做——我在 TASK-006 上撞过跨树混渲产假差异。

### 3.3 越界申报
`git show --numstat dd1090c`：`backfill_fetch.go` 130/2、`backfill_fetch_test.go` 373/0。
恰好 **2 个声明的 `writes`**，**无越界**。（`backfill_crosscheck_test.go` 已于 13:55 被移出 writes，
dev 也确实没碰它——与 `functional[2](b)` 改到 `runBackfill` 层的裁决一致。）

### 3.4 消融（27 个有效变异）
harness：独占 detached worktree；有效性闸用 `go vet`；`-v` 抓 `--- FAIL` 行确认**是哪一格红**；
跑完还原并校验 `backfill_fetch.go` 与 `backfill_scan.go` 两文件 sha256（均一致）；探针文件已删除，工作区干净。

**KILLED 25 / SURVIVED 1 / 等价变异 1。**

**⚠️ 那 1 个「等价变异」是我自己设计错的，如实记录**：我最初打 `boundary[0]` 时，
把 `backfillReconcileEmptyLabel` 改成**另一句仍然不同**的话 ⇒ SURVIVED。
但**「可区分」这个性质并没有被破坏** ⇒ 它是等价变异，**SURVIVED 不携带任何信息**。
测试用常量做断言、期望随之移动，是**正确**写法。改打「使其不可区分」的四个变异后**全部 KILLED**。
⇒ 与我在 TASK-005 记的「KILLED 来自副作用则什么都没证明」是同一句话的另一半：
**SURVIVED 来自等价变异，同样什么都没证明。**

---

## 4. 其它观察（均不影响判定）

### 4.1 ℹ️ D6 是**顺带**被覆盖的，不是逐条钉住
它由「第二步：检索页快照落盘失败」那一格连带杀掉。第 2 格若将来改动，D6 会**静默失去背书**，
而没有任何用例名提到它。DoD 只要求「被钉住」，故不判缺陷；建议返工时顺手在第 2 格的注释里
写明「本格同时覆盖 `errBackfillDisk` 例外那条路径」。

### 4.2 ✅ 我先前两处前置告警均已落实，且落实得比我建议的更彻底
- `functional[0]` 的自相矛盾已消除，且**采纳了我那句「『只调一次』防的是什么」的追问**——
  DoD 里直接写明「重复调用纯函数无可观测后果 ⇒ 那条断言钉的是实现细节而非行为，故去掉」。
- `functional[2](b)` 已从 `crossCheckBackfill` 层（恒真）改到 `runBackfill` 层（可证伪），
  `backfill_crosscheck_test.go` 也相应移出 writes。B1 变异证明它现在**确实**可证伪。

### 4.3 ℹ️ dev 自己揪出的那个假阳，我独立复核成立
`t.TempDir()` 的路径含子测试名 ⇒ `assert.Contains(err, "manifest")` 会匹配到临时目录路径。
我复核了现在的写法：子测试名与 `want` 字面量互不包含，D1–D4 四格的红**各自落在自己那一格**
（见 §2.1 表格右列），互补性成立。

---

## 5. 结论

**REJECTED，`reason_class=task_defect`。**

9 条 DoD 中 **8 条 PASS**，逐条有承重消融；覆盖率上升；无越界；判定对象零漂移。
唯一不通过的是 `functional[2](a)`：**要求 6 条写盘路径逐条钉住，实得 5 条**，
缺 `store.AppendFailed()` 那条；而「scope 内不可构造」这个免除理由**经我实测不成立**
（§2.2 给出了可直接照抄的 15 行构造，与交付物既有第四格是同一手法）。

**返工只需补一格用例，实现代码一行不用改。** 复验时我只重跑 D1–D6 与全量回归。
