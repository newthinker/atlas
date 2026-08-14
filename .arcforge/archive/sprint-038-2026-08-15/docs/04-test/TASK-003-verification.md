# TASK-003 验证报告

- **验证者**：test-agent-27
- **任务**：manifest 的类型、逐篇追加落盘与「已抓」判据
- **被验树**：`master @ 62b924300415fb109e7355aea0785b4c1ab4903b`（= `verify_baseline.head`，无漂移）
- **discovery sha256**：`8172a39e…49486`，与 `verify_baseline.discovery_sha256` 逐字节一致（无判定期漂移）
- **验证方式**：独立隔离 worktree `../wt-verify-T1T3`（detached @ 62b9243）+ 6 组消融实验
- **结论**：**VERIFIED（8/8 条 done_criteria 全部通过）**，附 1 条低severity 风险提示

---

## 一、验证方法学

判据不是「有没有测试」，是「**改坏它会不会有东西变红**」。做了 6 组消融（B1–B6），每组记录
**哪些测试红、红在哪条断言**，而不只是红不红。harness 四道闸（变异生效 / `gofmt -e` 语法 /
`go vet` 编译 / 主工作区 sha256 指纹 + 还原核实）全程通过，`backfill_manifest.go` 主工作区
sha256 恒为 `45046fad6157…`。

---

## 二、done_criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 我跑出的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 三类型按 §6 定义，`json` tag **逐字一致**（含 4 个本 sprint 新增字段），写出再读回字段全等 | `TestBackfillManifestRoundTrip` + `TestBackfillManifestJSONKeysAreVerbatim` + `TestBackfillManifestSaveWritesEmptyArraysNotNull` | **我逐字比对了 design-spec §6 与实现**：Manifest 8 key（`from` / `scanned_at` / `pages_scanned` / `search_pages_scanned` / `articles` / `failed` / `only_in_index` / `only_in_search`）、Article 8 key（含 `sha256` / `fetched_at` / `source`）、Failed 3 key —— **全部逐字一致**。往返测试用的 `fullManifest()` **每个字段都非零**（零值会让往返平凡为真）；`JSONKeysAreVerbatim` 做的是**精确 key 集合**比对（多一个少一个都红），与往返互补而非重复。 | ✅ PASS |
| functional[1] | **逐篇追加立刻落盘**；断言方式：连续追加两篇，**中途直接读文件**（不调 Flush/Close） | `TestBackfillManifestAppendPersistsImmediately` + `…AppendFailedPersistsImmediately` | 测试用 `readManifestFile()` 直接 `os.ReadFile`，**绕开被测的 `loadManifest` 读路径** —— 这一点很关键：走被测读路径的话「缓存在内存」与「已落盘」看起来完全一样。断言了两篇的**全部字段**与**落盘顺序**。**消融 B5**（`AppendArticle`/`AppendFailed` 改成只改内存不落盘）：两个测试均红。 | ✅ PASS |
| functional[2] | `sha256` 是内容的十六进制**小写**，断言**具体期望值**（不许写「长度是 64」） | `TestBackfillArticleSHA256KnownContent`（2 子用例） | 两个期望值我用 **`shasum -a 256` 与 `python3 hashlib` 双工具独立核对**，均一致：`c066d572…818d60`（HTML 片段）、`e3b0c442…7852b855`（空内容）。是具体值断言，不是长度断言。 | ✅ PASS |
| functional[3] | 「已抓」= id 在 `articles` **且** 文件存在，**两个条件各写一条断言** | `TestBackfillManifestHasArticleNeedsBothManifestAndFile`（4 子用例）+ `…RejectsEmptyFile` + `…ScansPastOtherIDs` | **两条件各自被独立消融验证**（这是 Leader 特别点名要验的一条）：<br>· **消融 B1**（去掉 `os.Stat`，只查 manifest）→ 只有「manifest 有记录但**文件被删**」子用例红（+ `ScansPastOtherIDs`）<br>· **消融 B2**（去掉 manifest 遍历，只查文件）→ 只有「**文件在但 manifest 没记**」子用例红<br>⇒ 两个条件**各有一条真正承重的断言**，不是只验了一个。 | ✅ PASS |
| boundary[0] | manifest **不存在** ⇒ 空 `Manifest` + nil error | `TestBackfillManifestLoadMissingFile` | 断言 nil error + store 非 nil + `Manifest` 为零值 + `HasArticle` 返回 false。另有 `LoadUnreadableErrors` / `LoadRejectsBrokenOutPath` 守住这个豁免口的边界（判据是 `os.IsNotExist` 而非「读失败就当没有」）。 | ✅ PASS |
| boundary[1] | 内容**不是合法 JSON ⇒ 报错**，不静默当空 | `TestBackfillManifestLoadInvalidJSONErrors`（3 子用例） | 三个子用例：非 JSON / 截断的 JSON / 合法 JSON 但非对象。同时断言「报错」**且**「不返回可用的 store」**且**「错误信息指出文件名」。**消融 B3**（非法 JSON 静默返回空 store + nil）：三个子用例**全部**红。 | ✅ PASS（见 §三风险提示） |
| error_handling[0] | 落盘失败 ⇒ 返回错误**且不破坏既有 manifest**（原子写）；断言磁盘上那份**仍是之前的完整内容** | `TestBackfillManifestSaveFailureKeepsPreviousFile` | 实现确为「同目录临时文件 + `rename`」。测试用 `chmod 0555` 制造落盘失败，断言 ①返回错误 ②磁盘内容**逐字节**等于失败前 ③解析回来 `Articles` 完整 ④无临时文件残留。**两条断言我分别验了它们各自承重**：<br>· **消融 B4**（改成 `os.WriteFile` 直写）→ 红在**①「必须返回错误」** —— 因为只读目录挡不住对已存在文件的 `O_TRUNC` 写，非原子实现会「悄悄成功」<br>· **消融 B6**（先 `os.Truncate` 原文件再走原子写，**错误仍正确返回**）→ 红在**②「写失败破坏了既有 manifest」** ⇒ **内容完整性那条断言本身是承重的，不是摆设**（这正是「守卫在场 ≠ 守卫有效」要查的东西）。<br>非 root 环境下未 skip，实测执行。 | ✅ PASS |
| non_functional[0] | `gofmt -l` 空、`go vet` 空、`go test -count=1` 全绿、**不降低既有覆盖率** | — | 前三项全部满足（见 §四）。覆盖率见 §四的专门说明。 | ✅ PASS |

---

## 三、发现的问题（不构成 DoD 违反，作为下游风险登记）

### 🟡 低：manifest.json 内容为字面量 `null` 时静默当空

我用探针实测 `loadManifest` 对各种边界内容的行为：

| 文件内容 | 结果 |
|---|---|
| `null` | **err=nil，返回空 store** ⚠ |
| `{}` | err=nil，返回空 store（合理：这是一份合法的空 manifest） |
| `""`（0 字节） | err = `unexpected end of JSON input` ✅ |
| `true` / `123` / `"abc"` / `[]` | err = `cannot unmarshal … into Manifest` ✅ |

`null` 是**合法 JSON**，而 DoD boundary[1] 的字面要求是「内容**不是合法 JSON** ⇒ 报错」
⇒ **不违反 DoD，不作为退回理由**。

但它落在 DoD 想防的那类失效里（静默当空 ⇒ 断点续抓退化成全量重抓 400 次请求且不报错）。
测试注释把第三个子用例标为「合法 JSON 但不是对象」，而 `null` 恰是这一类里唯一漏过去的成员
—— 注释的类别声称比实际守住的范围略宽。触发条件很窄（截断写不会产出 `null`；`jq` 管道
误操作可能），故只登记不阻断。**建议 M1c-2/-3 的消费侧或 TASK-008 reconcile 顺手加一条
`raw` 为 `null` 的判空**。

### ℹ️ 信息：包总覆盖率 −0.3pp，但「既有覆盖率」未降

**背对背对照**（三棵树同为 62b9243，`pre3` 去掉本任务两个 .go 文件，同一时刻并排跑）：

| 树 | 包总覆盖率 |
|---|---|
| `wt-t27-pre3`（无本任务） | 94.0% |
| `wt-verify-T1T3`（交付树） | **93.7%**（−0.3pp） |

**但逐函数比对：两侧共有的 120 个既有函数百分比完全相同，无一下降。**

包总下降是**纯算术稀释** —— 新增文件自身的 `Save()` 只有 66.7%（7 条 os 层错误返回分支：
`Marshal` / `tmp.Write` / `tmp.Close` / `Chmod` / `Rename` / `MkdirAll` 失败，无可移植的构造
手段）。`backfill_manifest.go` 其余 8 个函数全部 100%。

DoD 的措辞是「不降低**既有**覆盖率」，指的是既有代码的覆盖率不因本任务而下降 —— 逐函数
口径下严格成立。项目机制门禁是**下限**（`coverage.dev_minimum = 80`），93.7% 远超。
dev-agent-56 在 discovery 里主动披露了这一点，未为凑数注入 hook 变量，处理得当。
**判为通过。**

---

## 四、跑出的原始证据

```
$ cd ../wt-verify-T1T3 && GOTOOLCHAIN=local
$ gofmt -l internal/hestia/          → 空
$ go vet ./internal/hestia/          → 空
$ go test ./internal/hestia/ -count=1
ok  github.com/newthinker/atlas/internal/hestia  1.025s

$ go test ./internal/hestia/ -count=1 -run '^TestBackfill(Manifest|Article)' -v
24 条 RUN = 15 个顶层测试函数 + 9 个子测试；24 PASS / 0 FAIL / 0 SKIP
（`SaveFailureKeepsPreviousFile` 实测执行，未走 root skip 分支）
```

`backfill_manifest.go` 逐函数覆盖率：

```
AppendArticle 100.0%   AppendFailed 100.0%   HasArticle 100.0%   Path 100.0%
Save 66.7%             articleFile 100.0%    articleSHA256 100.0%
loadManifest 100.0%    withEmptySlices 100.0%
```

**6 组消融汇总**（记的是哪些测试红）：

| 消融 | 变异内容 | 红的测试 |
|---|---|---|
| B1 | `HasArticle` 去掉文件存在性检查（只查 manifest） | `HasArticleNeedsBothManifestAndFile/文件被删` + `HasArticleScansPastOtherIDs` |
| B2 | `HasArticle` 去掉 manifest 遍历（只查文件） | `HasArticleNeedsBothManifestAndFile/manifest 没记` |
| B3 | 非法 JSON 静默返回空 store | `LoadInvalidJSONErrors`（3 子用例全红） |
| B4 | 原子写改成 `os.WriteFile` 直写 | `SaveFailureKeepsPreviousFile`（红在「必须返回错误」） |
| B5 | `AppendArticle`/`AppendFailed` 不落盘 | `AppendPersistsImmediately` + `AppendFailedPersistsImmediately` + `SaveFailureKeepsPreviousFile` |
| B6 | 先 `os.Truncate` 原文件再原子写（错误仍返回） | `SaveFailureKeepsPreviousFile`（红在「**写失败破坏了既有 manifest**」） |

每组变异后主工作区 `backfill_manifest.go` sha256 均未变，worktree `git diff` 均为空。

---

## 五、声明范围核对

`git show --numstat 3a7889e` 实际改动 2 个文件：

```
203  0  internal/hestia/backfill_manifest.go
504  0  internal/hestia/backfill_manifest_test.go
```

与 `writes` 声明**逐条一致，零越界、零删除**（纯新增，未触碰任何既有文件 —— 包括
`store_test.go` 那份导出函数白名单，dev 全部用非导出名规避，处理正确）。

---

## 六、结论

**VERIFIED。** 8 条 done_criteria 全部通过。DoD 明写「这样写会假绿」的三处
（`functional[3]` 两条件各一断言、`boundary[1]` 不许静默当空、`error_handling[0]` 原子写内容
完整性）经消融验证均为**真守卫**，其中 `error_handling[0]` 的两半（返回错误 / 内容完整）
我分别用 B4 与 B6 证实各自承重。

登记 1 条低 severity 风险（`null` 静默当空）与 1 条信息项（包总覆盖率 −0.3pp 但既有覆盖率
逐函数未降），**均不构成退回理由**。
