# QA 终审报告 · Sprint 037（M1b-4b ingest + CLI + 部署）

**审查者**：qa-agent-13 ｜ **日期**：2026-08-13
**锚点**：`master @ bb825defffd332cf6886ef32def33d7f05c455a7`（开工时已比对，与 leader 声明一致）
**范围**：`git diff 63ac5b6..HEAD -- internal/hestia/ cmd/atlas/hestia*.go configs/hestia.yaml deploy/launchd/com.newthinker.atlas.hestia-ingest.plist scripts/ops/install-services.sh`（28 文件 / +6458 −125）

---

## 0. 结论

### **CONTESTED —— 但不是因为审查者之间有分歧，而是因为 Sprint 不具备终审条件**

审查过程中（08:35–08:43 本地）**TASK-007 / 009 / 011 经 leader 的 `verified → review_fix` 正规边转出，
dev-agent-52 于 `00:38:10Z` 全部认领为 `in_progress`**，七个交付文件在工作区被改动且**尚未提交**。

⇒ **本报告的全部判定只对已提交的 `bb825de` 成立**，不覆盖在途返工。
终审 verdict 需在 007/009/011 落定并重新提交后补一轮。

**对 `bb825de` 这个committed 状态本身的判定**：**1 条 CRITICAL + 2 条 WARNING ⇒ 不能 PASS。**
CRITICAL 那条（`--force` 绑定无守卫）**不在**任何在途改动内、也不在任何已报 WARNING 内，需单独派回。

---

## 1. 口径与降级声明（先说清楚，免得后面的数字被误读）

| 项 | 状态 |
|---|---|
| 第一轮 常规 review | ✅ 完成（本体 + 消费者位实跑） |
| 第二轮 跨视角对抗 | ⚠️ **降级**：三个 lens 子代理（Skeptic / Architect / Minimalist）spawn 被用户中断，Minimalist 返回空。**改由本体在 context 内做跨视角**，并以 **14 条变异实测**替代「多视角推理」——变异是可证伪的，视角推理不是。 |
| 跨模型复核 | ✅ **未降级**：`codex-cli 0.139.0` 可用，已跑 read-only 独立审查（`gemini` 不可用）。其结论**逐条复核过**，不原样转述。 |

**报数口径**：测试一律 `X PASS / Y FAIL / exit Z`（`-v` 下 `--- PASS:` 行计数）；
所有退出码均以 `cmd >file 2>&1; echo $?` 取得，**不跨管道**。

---

## 2. 机械门禁（全绿，逐条给命令与输出）

```
go build ./...                                   BUILD_EXIT=0
go test ./internal/hestia/... ./cmd/atlas/...    TEST_EXIT=0    948 PASS / 0 FAIL
go vet ./internal/hestia/... ./cmd/atlas/...     VET_EXIT=0     无输出
gofmt -l <本 Sprint 改动的 19 个 .go 文件>        0 个不合规
go test ./internal/hestia/... -coverprofile      coverage: 93.5%   ← 与声明的 93.5% 一致
```

`cmd/atlas/hestia.go` 文件级覆盖（对照 H34 议定的处置）：
`init 100.0% / openHestia 100.0% / runHestiaIngest 50.0% / runHestiaStatus 84.6%`
⇒ `openHestia` 由 0 缺口（此前 85.7%）、`runHestiaIngest` 由 **0.0% → 50.0%**。
残余未覆盖即 H34 议定「真实依赖装配缝」，**处置与议定一致，不作为发现**。

**已知遗留、经我核过 base 后确认不算本 Sprint**：`cmd/atlas/backtest_test.go`、`crisis_test.go` 的 gofmt
——它们**不在**本 Sprint 的改动文件集内（上面 `gofmt -l` 只喂了 19 个改动文件，0 不合规）。

---

## 3. QA-PRE 预登记项复核（判据全部自己量，不引用 leader 结论）

### QA-PRE-2 · 验后漂移复查 —— ✅ 零漂移（我把范围从 9 个扩到全部 11 个）

```bash
for t in 001 … 011; do b=$(jq -r .verify_baseline.head "$f"); w=$(jq -r '(.writes // .packages)[]' "$f")
  git diff --name-only "$b" HEAD -- $w ; done
```
**11 个任务全部输出为空**（leader 原清单漏了 008/009，我补齐后同样为空）。

### QA-PRE-1 / 1b / 1c · 同一句过期结论的三份副本 —— ✅ 三份全部确认成立，**未发现第四份**

| # | 位置 | 原文要害 |
|---|---|---|
| 1 | `internal/hestia/ingest.go:161` | 「Verdict 当前恒为 New」 |
| 2 | `internal/hestia/ingest_test.go:287-289` | 「HasPeriod 过滤（`discover.go:303-318`）… Duplicate/Revision 当前不可达」 |
| 3 | `internal/hestia/store.go:216` | 「HasPeriod … Discover 用它决定翻页何时停」 |

**扫第四份的判据**（三类锚）：`恒为 New|不可达`、把 `HasPeriod` 描述成判停规则、`*.go:NNN` 形式坐标。
**结果：无第四份。** 同批扫出的 `discover.go:143` / `discover_test.go:971`（`Atoi`/`scanPage` 的不可达分支）
与本议题无关且仍成立，`discover.go:205` / `ingest_test.go:659` 已含时态限定 —— 与 leader 登记的「不必改」清单一致。

🔴 **我给这三条加了一条 leader 清单里没有的证据，而且它比测试更强**：
**生产环境实跑直接证否**（见 §5 消费者位）——
```
$ atlas hestia ingest --hestia-config ./hestia.yaml --force
2026-06 Duplicate → hestia_observations        ← Verdict 不是 New，是 Duplicate
```
⇒ 「恒为 New」不只被一条绿测试反证，**在真实央行数据上跑出来就是假的**。

### QA-PRE-1d · 行号锚 —— ✅ 成立，且**同类问题在仓库里不止这一处（但全部早于本 Sprint）**

我把扫描从「那一处」扩到全仓 hestia 范围，另找到 **6 处已失效的行号锚**，逐个 `sed -n` 核过：

| 锚 | 声称指向 | 实际指向 |
|---|---|---|
| `CONTRACTS.md:31` → `store.go:357-361` | Skipped 必填 Reason 的校验 | `Preceding` 的错误包装 |
| `CONTRACTS.md:32` → `store.go:343-345` | 「明文声明刻意不查」 | `cols := slices.Concat(...)` 建 SQL |
| `CONTRACTS.md:50` → `store.go:109-110` | 逐字保存视图文本 | `var deployed string` |
| `CONTRACTS.md:66` → `store.go:294-299` | 四行论证 NaN 必须入口拦 | `HasArticleInObservations` 的文档注释 |
| `store_test.go:2256` → `store.go:303` | 挡住 `n<=0` | 两层分工的文档注释 |
| `CONTRACTS.md:76/236/340` → `types.go:*` / `profiles.go:*` | （同类） | （同类） |

**`git blame` 逐行定年**：全部出自 `10cb8514`(08-09) / `fbd7332d`(08-11) / `e6395619`(08-11) / `c101d612`(08-12)
—— **全部早于 Sprint 起点 `63ac5b6`** ⇒ **不算本 Sprint 的账**，登记为待单开 chore。
✅ **正面记录**：本 Sprint 新增的 CONTRACTS「Sprint 037」节（556-755 行）**一处行号锚都没用**，
全部改用符号名 + 字串锚 —— QA-PRE-1d 的教训在同一 Sprint 内就被执行了。
（唯一例外是 plist 与 CONTRACTS:700 引用的 `2026-08-12-hestia-cli.md:1634-1638`，指向 **hestia 仓的计划文档**，
本仓无法验证；建议同样改成字串锚。）

---

## 4. 第一轮：常规 code review

### 4.1 契约措辞（leader 点名的两项）—— ✅ 均正确

- **「修订版暂不支持」**（CONTRACTS 665-676）：写成「**没测过，所以不承诺**」，并附一张
  `~~做不到/结构上不可达~~` vs 「没测过所以不承诺」的对照表，明写后人会因此做什么。**措辞正确。**
- **「`--force` 穿透两层」**（CONTRACTS 580-598）：与实现逐条对得上
  （层① `ingest.go:113` `if !d.Force`；层② `ingest.go:58` `known = neverSeen{}`）。**一致。**

### 4.2 前向指针（leader 点名核对）—— ✅ 两条都指得对

| 指针 | 指向 | 核对 |
|---|---|---|
| `CONTRACTS.md:480`「已被 Sprint 037/TASK-011 推翻，见末尾『判停规则』」 | 600-632 行「### 判停规则：article_id，且只查权威表」 | ✅ 存在且对题 |
| `CONTRACTS.md:510-513`「已在 Sprint 037 回答…落点与设想不同」 | 600-622 行 | ✅ 存在且对题，并主动声明「落点不同」 |

⚠️ 唯一措辞瑕疵（SUGGESTION）：510 行「选了『一级键只查 `hestia_observations`』那条路」与紧随的
「**不是**改一级键」在字面上打架，靠下半句救回。建议改成「选了那条路**的效果**，但落点是给 Discover 单独一个判停键」。

### 4.3 安全性

| 面 | 结论 |
|---|---|
| SQL 注入 | ✅ **无**。值全走 `?` 占位符；动态拼接的只有列名/表名，来源是包级常量 `metaColumns`/`fieldOrder`/`TablePending`，非外部输入。 |
| 凭据 | ✅ **无**。plist 与 `configs/hestia.yaml` 均不含任何密钥；plist 刻意一个代理键都不设。 |
| 路径穿越 | ✅ 无可利用面（`db_path` 来自本地可信配置）。 |
| HTTP 客户端 | ✅ 有 `Timeout`、`maxResponseBytes` 上限（`LimitReader` 多读 1 字节区分「恰好等于」与「超过」）、`NewRequestWithContext`。 |
| **跨源抓取** | ⚠️ 见 F3（SUGGESTION）。 |

### 4.4 代码质量

命名、分层、错误包装（每级都带期次 + article_id + stage）、`errors.Join` 保留可判因性 —— 均达标。
注释密度远高于常规，但这是本仓明确约定（记录被实测推翻过的错误理由），**不作为问题**。

---

## 5. 消费者位：真的跑了一遍（隔离 scratchpad 目录 + 真实网络）

二进制由 `bb825de` 构建。**这一位置产出了本报告最强的两条证据。**

| # | 命令 | 退出码 | 输出 |
|---|---|---|---|
| 1 | `hestia status`（空库） | 0 | `hestia status (db: /…/consumer/data/hestia.db)` + `observations: 0` / `pending: 0` |
| 2 | `hestia ingest`（首跑） | 0 | `2026-06 New → hestia_observations` |
| 3 | `hestia ingest`（第二轮） | 0 | `no new reports (stopped: seen_article)` ✅ 幂等 |
| 4 | `hestia ingest --force` | 0 | `2026-06 Duplicate → hestia_observations` ✅ 穿透两层、走到 Save |
| 5 | `hestia ingest`（配置不存在） | **1** | **1 行** `Error: hestia: reading config ./nope.yaml: …` |
| 6 | `hestia ingest`（host 不可达） | **1** | **1 行** `Error: hestia ingest: discover …: connection refused` |

⇒ **报警设计端到端成立**：退出码真传播（`main.go` 的 `os.Exit(1)`）、`SilenceUsage` 真生效
（失败输出 **1 行**，而改动前的同形命令是 13 行）、错误信息带得出「是哪份配置 / 哪个 URL」。

### 5.1 ✅ 独立核对「有没有漏抓」（不引用管线自己的说法）

我自己 `curl` 取回 index 第 1–3 页，用独立的 perl 正则数报告标题：

```
p1/p2/p3 各 ~38KB
三页里含「金融统计数据报告」的链接：仅 1 条 —— 2026年上半年金融统计数据报告
```
⇒ 与 `Discover` 实际交出的候选数**一致**，**无漏抓**。
（这条回答的是「测试语料代表性」那类问题：管线在真实生产语料上看见了它该看见的全部。）

### 5.2 ✅ 部署链路核对

`deploy.sh` 的 rsync **不排除** `configs/` ⇒ `configs/hestia.yaml` 会到达 plist 指向的
`/Users/zuowei/workspace/runtime/atlas/configs/hestia.yaml`；`data/` 被 `--exclude` 保护且由
`mkdir -p` 保证存在 ⇒ `db_path: data/hestia.db` 配合 plist 的 `WorkingDirectory` 解析正确。
`install-services.sh` 已把 `hestia-ingest` 加进**安装循环**（不只是注释）。**链路完整。**

---

## 6. 第二轮：对抗式验证 —— 14 条变异实测

隔离 detached worktree @ `bb825de`；**每轮收尾核对主工作区 sha256 未变**（变异期间主工作区逐字节不变，
已用指纹比对而非「工作区干净」判据）。基线 **948 PASS / 0 FAIL**。

| # | 变异 | 结果 | 被哪条断言杀死（下钻到因果） |
|---|---|---|---|
| M1 | 撤 `--force` 层②（`neverSeen`） | **KILLED** | `TestForceOnObservedPeriodIsDuplicate` |
| M2 | 撤 `--force` 层①（`if !d.Force`） | **KILLED** (4) | `TestIngestSkipsSeenArticleUnlessForce/Force_绕过一级幂等键` 等 |
| M3 | 撤升序排序 | **KILLED** (2) | `TestIngestProcessesOldestFirst` |
| M4 | 撤期次交叉校验 | **KILLED** | `TestIngestRejectsPeriodMismatch` |
| M5b | 吞掉单期失败（返回 nil） | **KILLED** (5) | `TestIngestWrapsStageErrors` 等 |
| **M6b** | **删掉 `no new reports` 的 `(stopped: %s)`** | 🔴 **SURVIVED** | **948 PASS / 0 FAIL** |
| M7 | `status` 不解析绝对路径 | **KILLED** | `TestRenderStatusPrintsAbsoluteDBPath` |
| M8 | `status` 不打 pending 失败明细 | **KILLED** | `TestRenderStatus` |
| M9 | `errors.Join` → `errors.New`（丢可判因性） | **KILLED** | `TestIngestWrapsStageErrors` |
| M10 | 不打 `%s FAILED:` 行 | **KILLED** (2) | `TestIngestContinuesAfterOneFailure` |
| M11 | 不打 `already ingested` 行 | **KILLED** | `TestIngestSkipsSeenArticleUnlessForce/默认跳过` |
| **M13** | **`--force` 解绑（`BoolVar`→`Bool`）** | 🔴 **SURVIVED** | **948 PASS / 0 FAIL** |
| **M14** | **`--hestia-config` 解绑（`StringVar`→`String`）** | 🔴 **SURVIVED** | **0 FAIL** |
| M5/M6/M12 | （编译失败） | **作废** | 有效性闸拦下，**不计入 KILLED** |

✅ **M1/M2 独立复核了 CONTRACTS 596-598 的消融声明**：该文声称「撤层②红在『走到 Save』那半、
撤层①红在『结果是 Duplicate』那半」——**实测与之逐条吻合**。那条声明**不是自述，是可复现的**。

✅ **M10/M11 KILLED 证明这不是「输出普遍没测」**：其余每一条运维可见输出都有守卫，
**缺口精确地只在 stop reason 那一半** —— 这让 F2 成为一条精确发现而非笼统抱怨。

---

## 7. 跨模型复核（codex-cli 0.139.0，read-only）

codex 独立给出 4 类判断。**我逐条复核，不原样转述**：

| codex 的说法 | 我的复核 | 采纳 |
|---|---|---|
| `Ingest` 编排无 bug；`--force` 两层自洽；按 `Period` 升序正确（因 `Preceding` 按 `period_type` 隔离，同 `YYYY-12` 的 monthly/annual 互不为历史） | ✅ 我独立验过：`Preceding` 的 SQL 确为 `WHERE period < ? AND period_type = ?`；M3 变异也佐证排序承重 | ✅ 采纳为**正面结论** |
| **SQL 注入 / 凭据 / 路径穿越均无** | ✅ 与我 §4.3 独立得出的结论一致 | ✅ 采纳 |
| **`TestHestiaFlags` 只断言 flag 存在、不断言绑定** | 🔴 **实测坐实**（M13/M14 两条变异 + 行为自证） | ✅ **采纳，升级为 CRITICAL** |
| `TestHestiaCommandIsRegistered` 不查 `RunE` | ✅ 属实（`hestia_test.go:36-50` 只比对子命令名） | ⚠️ 降为 SUGGESTION（测试直接调 `runHestia*`，真实风险低于 flag 绑定） |
| `ingest_test.go:723` 用 `Duplicate` 证明「走到 Save」可被伪实现骗过 | ❌ **不采纳**：其构造的伪实现（只打印不落库）在现实变异空间外，且 **M1/M2 实测该测试确实杀得死两层撤除** | ❌ |
| `ingest_test.go:313` 未数 pending 表行数 | ❌ **不采纳**：该性质由 Store 层测试与 `TestIngestContinuesAfterOneFailure`（已改为真数 pending 行）覆盖 | ❌ |
| 跨源 URL（SSRF 面） | ✅ **实测坐实**（见 F3） | ✅ 采纳为 SUGGESTION |

---

## 8. 发现清单

### 🔴 F1 [CRITICAL] `--force` / `--hestia-config` 的 flag 绑定完全无守卫，失效静默

**位置**：`cmd/atlas/hestia.go:70-73`（缺陷在守卫侧：`cmd/atlas/hestia_test.go:53-58`）

**观察**
```
M13: Flags().BoolVar(&hestiaForce,"force",…) → Flags().Bool("force",…)
     go test ./cmd/atlas/... ./internal/hestia/... -count=1  → TEST_EXIT=0  FAIL=0（948 PASS，与基线相同）
行为自证（同库同命令）：
     变异版 $ atlas hestia ingest … --force  → no new reports (stopped: seen_article)   ← 静默失效，exit 0
     未变异 $ 同上                            → 2026-06 Duplicate → hestia_observations
M14: StringVar→String 同样 0 FAIL
grep -rn 'SetArgs|ParseFlags|"--force"' --include='*_test.go' cmd/atlas/  → grep_exit=1（零命中）
```

**为什么是 CRITICAL**
1. `--force` 是契约写明的**唯一**恢复出口（CONTRACTS 577-578、591）。
2. 失效形态与「今天没有新报告」逐字同形且 exit 0 ⇒ 落在设计上唯一报警通道之外。
3. **TASK-008 的 DoD `boundary[0]` 把这条记为已覆盖**（「两个 flag → TestHestiaFlags」），
   而该测试只断言 `Lookup(...) != nil`。⇒ **H46 的形状：占着「已覆盖」那个位置**，
   且这是「DoD 声称的性质实际未被覆盖」，不是额外加固。
4. 全仓**没有任何测试走过 cobra 参数解析**；`hestiaForce` 在测试里一次都没被设过。

**建议修复**：补一条经 `SetArgs`/`ParseFlags` 的**肯定式**绑定守卫
（解析 `--force` 后断言 `hestiaForce == true`；解析 `--hestia-config X` 后断言 `hestiaCfgPath == X`）。

### ⚠️ F2 [WARNING] `no new reports` 分支的 stop reason 无任何断言

**位置**：`internal/hestia/ingest.go:70`

**观察**：M6b 删掉 `(stopped: %s)` ⇒ **948 PASS / 0 FAIL**；
`grep -rn stopped --include='*_test.go' .` → **零命中**。
「已覆盖」的错觉来源：`StopReason` 在 **Discover 层有 9 条断言**（`discover_test.go:746/755/1174/1206/1246/1294`…），
但**把它送到运维眼前的那一半**没有。而据 `discover.go` 自述，`StopReason` 存在的全部理由就是
「当前最危险的属性是**静默**」——`max_pages` 意味着窗口外还有发现不了的期次。

**归属订正**：本条与**已存在的 QA WARNING-1 重合**，是我独立复现，**不是新发现**。
dev-52 在途的 `TestIngestReportsStopReasonEvenWithCandidates` 已覆盖**有候选**那条路径，
但断言的是 `discover stopped:`；**`len(cands)==0` 的 `no new reports (stopped: %s)` 仍无断言**，建议顺手补。

### 💡 F3 [SUGGESTION] index 页里的**绝对 URL** 会被原样当作抓取目标

**位置**：`internal/hestia/discover.go:70`(`resolveURL`) → `:278`(`scanPage`) → `ingest.go:124`(`Fetch.Get`)

**观察**（我写了独立最小复现，不依赖 codex 的说法）：
```
输入: <a href="https://attacker.example/9999999/index.html">2026年上半年金融统计数据报告</a>
articleLinkRE 命中: id=9999999
标题匹配 reportTitleRE: true
解析出的抓取目标 URL: https://attacker.example/9999999/index.html   ← 跨源
```
`url.ResolveReference` 对绝对 ref 原样返回；`http.Client` 默认跟随重定向。
**威胁模型限定**：需要央行页面本身被污染 —— 那已是更大的问题，故只列 SUGGESTION。
**建议**：`scanPage` 加一条同源约束（`host` 必须等于 `IndexURL` 的 host），成本一行，
且**符合本仓「静默降级要出声」的既有纪律**。

### 💡 F4 [SUGGESTION] `status` 是只读诊断，却会创建库与父目录

**位置**：`cmd/atlas/hestia.go:114` → `openHestia` → `NewStore`（`MkdirAll` + 建库）

**观察**：在两个全新空目录里各跑一次 `hestia status`，两次都 `EXIT=0`，且各自留下 `./data/hestia.db`（20480 字节）。
**后果**：在错的 cwd 下诊断会**创建**一个空库并如实报告「0 期」；此后「库存不存在」这类判据永远为真。
现有缓解（打印解析后的绝对路径）**有效但只对看输出的人有效**。
**建议**：`status` 用只读方式打开，库不存在时明确说「库不存在」而不是造一个。

### 💡 F5 [SUGGESTION] `CONTRACTS.md:510` 措辞自相矛盾（见 §4.2）；plist/CONTRACTS:700 对 hestia 仓计划文档仍用行号锚

---

## 9. 正面记录（这些经我实测确认成立，不是客套）

1. **CONTRACTS 596-598 的消融声明可复现**（M1/M2 逐条吻合）—— 声明不是自述。
2. **本 Sprint 新增契约一处行号锚都没用** —— QA-PRE-1d 的教训同 Sprint 内已执行。
3. **plist 守卫的阳性对照是真的**：`TestHestiaPlistSetsNoProxyKeys` 用真实的 `crisis-daily` 跑同一解析器并要求报出 `no_proxy`
   ⇒ 一个恒返回 `["PATH"]` 的坏解析器骗不过它。
4. **`TestFlowRECapturesQuarterlyPeriodVerbatim` 是语料锚定的正向断言**（捕获组逐字等于期望词），
   配 `require.NotEmpty` 前置锚点 —— 与 H25 的结论一致，是本 Sprint 守卫设计的正面样本。
5. **`TestInstallServicesInstallsHestiaPlist` 有阴性对照**（断言截取范围不含文件头注释）。
6. **无漏抓**（§5.1 独立数索引页）。
7. **报警链路端到端成立**（§5 的 6 条实跑）。

---

## 10. 我自己的判断失误（记下来免得后人以为已复核）

`cmd/atlas/hestia_test.go` 上一版的 `plistIntsUnderKey` **按下标跨 dict 配对** Hour/Minute
——**我读到了这个形状，但错误地判断它「语义上无害」而放过**。
实际后果是排班从 3 次/天变成约 86 次/天，且断言全绿。
**该假通过是别的 QA 轮次找到的，不是我。** 我的错误在于：用「推演」判定它无害，而没有花一次变异去量。
（这正是结转发现里反复记的「用推演代替测量」，第 N 次。）

---

## 11. 建议的返工项

| 任务 | 当前状态 | fix_items | reason_class |
|---|---|---|---|
| **TASK-008** | `verified` | 补经 cobra 参数解析的 flag 绑定守卫（`--force` → `hestiaForce==true`；`--hestia-config X` → `hestiaCfgPath==X`），判据用肯定式 | **`task_defect`**（DoD `boundary[0]` 声称覆盖的性质实际未覆盖） |
| TASK-011（**已在途**） | `in_progress` | 顺手补 `len(cands)==0` 分支的 `stopped:` 断言（F2 后半） | —（并入在途返工） |
| （可选）TASK-005/011 | — | `scanPage` 同源约束（F3）；`status` 只读开库（F4） | 建议留作 chore，不阻断本 Sprint |

**不建议**为 §3 那 6 处历史行号锚开返工 —— 全部早于 `63ac5b6`，应单开 chore。

---

## 12. 复现本报告的命令锚

全部结论锚定 **`bb825defffd332cf6886ef32def33d7f05c455a7`**（全 sha，非 `HEAD`/分支名 —— 工作区此刻已有在途返工，
用符号引用会取到另一棵树）。变异复现：`git worktree add --detach <tmp> bb825def…`，在该副本内施变异，
主工作区 sha256 全程比对不变。

---
---

# 附录 A · 终审裁决与合并（qa-agent-13 追加，2026-08-13 第二次落盘）

> 本节是在 leader 转回三个 lens 结论、并追加三条审查手法之后写的。
> **上文（§0–§12）一字未改**，全部结论仍锚定 `bb825de`。

## A0. 🔴 三处必须先更正的事实 —— 它们会改变 leader 的处置

### A0-1 · `sprint-037-code-review.md` **已经存在，且不是我写的** ⇒ 我没有覆盖它

```
.arcforge/docs/05-review/sprint-037-code-review.md   282 行   mtime 08:33:11
  <!-- 落盘者: qa-agent-14 -->
  <!-- 审查执行者: qa-agent-13（三个 Claude lens）；本实例只做合并落盘，未重做审查 -->
```

leader 让我 `verdict sprint-037-code-review.md` 落盘。**写通道是全量覆盖写、且没有删除子命令**
⇒ 那条命令会**销毁 qa-agent-14 已合并的 282 行 lens 裁决**，而**我复现不出它**
（三个 lens 的结论发给了 `main`，从未发给我）。

⇒ **我没有执行那条命令**，改为把裁决**追加进我自己的这份报告**。
两份并存、互不覆盖，且 `qa-review-sprint037.md` 的 mtime 晚于全部 `verified` 任务 ⇒ idle 逃生口照样闭合。

### A0-2 · 🔴 那条 CRITICAL **已经被在途返工修掉了**，`verified → review_fix` 对 TASK-009 **走不通**

| | 状态 |
|---|---|
| TASK-007 / 009 / 011 | **`in_progress`**（`00:38:10Z` 由 dev-agent-52 认领） |
| TASK-008 | `verified` |

⇒ **TASK-009 不在 `verified`**，`verified → review_fix` 这条边**没有合法源状态**，写通道会 DENY。

**而且缺陷已修**。leader 的复核结论「全仓只命中 `hestia_test.go:386` 一行注释，**没有任何断言**」
——**那是 `bb825de`（已提交）上的事实，在当前工作区已不成立**。我重跑同一条 grep：

```
grep -rn 'StartCalendarInterval' --include='*.go' .     → 9 处命中（不是 1 处）
  :411  func plistSchedule(...)          ← 新的限定作用域解析器
  :472  case ... lastTxt == "StartCalendarInterval":   ← 真正的作用域断言
  :482  "StartCalendarInterval 的第 %d 个 dict 缺字段…" ← 断言失败信息
```

dev-52 已把 `plistIntsUnderKey`（全文档扫 + 按下标配对）替换为 **`plistSchedule`**
（先认出 `StartCalendarInterval` 后的 `<array>`，再逐 `<dict>` 收成对字段，**缺任一字段即失败**），
并在注释里写明「**它替换的上一版是本 Sprint 第四个假通过，而且就在为修第三个而写的测试里**」。

⇒ **处置建议改为：不要为它开 `review_fix`，改为在 TASK-009 的验证环节确认这一版守卫有效。**
（这正是 §12 那条纪律的实例：`HEAD`／工作区是会漂的锚，leader 的 grep 与我的 grep 差 9 倍，
差别只在跑的时刻 —— 而**两次都没错**。）

### A0-3 · 验证报告是 **11 份** —— 我复核了 leader 的更正

```
ls .arcforge/docs/04-test/TASK-*-verification.md | wc -l   →  11
```
✅ 更正成立。我上文 §3 用的就是 11 份口径（`TASK-001…011`），不受影响。

---

## A1. 🔴 如实回答：差分守卫判据（H48，`状态：待证伪，n=1`）

**leader 要的是如实回答，不是好消息。**

### A1-1 · 我的两轮审查里，**没有任何一处用过这条判据**

我的 14 条发现全部来自**逐行读 + 变异实测 + 消费者位实跑**。
Skeptic 那条 CRITICAL 同样是逐行读 + 变异（leader 已声明不算）。⇒ **原本的答案是「没用过」。**

### A1-2 · 收到 leader 的消息后我**补跑了两个域**，结果如下

**域① · 11 份 plist × 3 个解析器**（`plutil -lint` / `plutil -convert json` / Python `plistlib`）

```
差集①（lint 放行、严格 XML 拒绝）: ['com.newthinker.atlas.aktools.plist']   ← 就是已知的那一个
差集②（Apple 语义视图 vs plistlib 的 env 键集合不同）: （空）
差集③（有 env 但无任何唤起机制）: （空）
```

**域② · `configs/hestia.yaml` 的 YAML 键 vs loader 的 `mapstructure` tag**

```
差集①（YAML 写了但没有 tag 接 —— 写了等于没写）: （空）
差集②（loader 期望但 YAML 没写 —— 走零值）: 10 个键
   ['caliber_exemptions','magnitude_ranges','max','min','period','period_types','reason','skip_checks','unit','version']
```

### A1-3 · 结论：**没有捞到第二个样本**

- 域① **只重现了已知的 `aktools.plist`**，零新增。
- 域② 那 10 个「差异」**全部是合法分歧** —— `magnitude_ranges` / `caliber_exemptions` 是
  `configs/hestia.yaml` 里**明文写着「有意不写」**的两块（连理由都写了），其余 8 个是它们的嵌套子键。**零新增。**

⇒ 按 leader 定下的约定：**在 final-report 里写成「一个案例的归纳」，不得写成「已验证的方法」。**

### A1-4 · 但这两次空跑**给 leader 的更正提供了第二个数据点**（这一条是正收益）

leader 更正后的版本说：「**差集是攻击样本的候选集**，一般情况下差集里混着**合法的分歧**」。
域② 恰好是这句话的干净实例：**差集非空（10 个）、而其中真缺陷数为 0**。

⇒ **原版判据（「差集就是攻击样本」「不一致处必然有一道是错的」）在域② 上会直接产出 10 条假发现。**
更正后的版本在这个新数据点上**成立**。⇒ 更正本身 n 从 1 变 2，**而被更正掉的那版 n=1 的反例现在有了**。

---

## A2. 「互补 / 各管一半 / 不可互替」句式扫描（leader 追加项 ②）

验证者主动交出了这个薄弱点。我扫了全部 11 份验证报告，**命中 2 处，逐条补差集**：

| # | 位置 | 声称 | 差集跑了没有 | 结论 |
|---|---|---|---|---|
| 1 | `TASK-003-verification.md:94-95` | reflect 版 与 AST 版「互补不可互替」 | ✅ **验证者自己跑了**：「A1 时 reflect PASS / AST FAIL，**A2 恰好反过来**」 | **两个方向差集都非空 ⇒ 互补性有实证，成立** |
| 2 | `TASK-009-verification.md:65` | `plutil -lint` 与 Go 守卫「各管一半、不可互替」 | ❌ 当时没跑 → ✅ **我补跑了**（域①） | 差集 = `{aktools.plist}`，非空 ⇒ **不可互替成立** |

⇒ **两处都站得住，没有发现「写了互补但其实是同一道守卫」的情形。**

⚠️ 但第 2 处要按 leader 更正后的读法收尾：差集非空**不代表有一道是错的**。
`plutil -lint` 答「Apple 收不收」、`encoding/xml` 答「是不是良构 XML」，**两个答案都对**；
错的是**消费者代码假定两者等价**（把 XML 解析失败当成不会发生）。**缺陷不在任何一道守卫里。**

---

## A3. 合并裁决（我的两轮 + 三个 lens，交叉命中已标注）

三个 lens 的结论我**未直接收到**（发给了 `main`），此处依 leader 转述 + qa-agent-14 的合并件，
**凡我自己复核过的标 ✅亲验，未复核的标 📋转述**。

| # | 级别 | 发现 | 来源 | 我的复核 |
|---|---|---|---|---|
| C1 | ~~CRITICAL~~ → **已修** | 排班守卫不限定 `StartCalendarInterval` 作用域 | Skeptic | ✅亲验：**工作区已换成 `plistSchedule`**，见 A0-2 |
| **C2** | **CRITICAL** | **`--force`/`--hestia-config` flag 绑定无守卫** | **本体（M13/M14）** | ✅亲验 + 行为自证 |
| W1 | WARNING | `StopReason` 有候选时被丢掉 | Skeptic + Minimalist **交叉** + 本体 M6b | ✅亲验；**在途已加 `TestIngestReportsStopReasonEvenWithCandidates`**；⚠️ `len(cands)==0` 分支仍无断言 |
| W2 | WARNING | `HasPeriod` 无生产消费者、文档仍说它判停 | Minimalist + Architect **交叉** | ✅亲验 = 我的 QA-PRE-1c；**在途已修** |
| W3 | WARNING | `--force` 漏第三层 + flag help 承诺过头 | Skeptic + Architect **交叉** | ✅亲验：**在途已改 help 文案**，明写 re-extracted values are DISCARDED |
| W4 | WARNING | `ingest` 无 cwd 守卫 | Architect | ✅亲验：**在途已加** `db: <abs>` 打印 |
| W5 | WARNING | `periodAlt`/`cumulativePeriods` 守卫用硬编码列表 | Minimalist | ✅亲验：枚举式 `for _, p := range []string{"一季度","前三季度"}`，新增期次类型时不会红 |
| S1 | SUGGESTION | 跨源 URL（SSRF 面） | 本体 + codex | ✅亲验（最小复现） |
| S2 | SUGGESTION | `status` 会创建库与父目录 | 本体 | ✅亲验 |
| S3 | SUGGESTION | `exhausted` 差一、名字对不上等 | Architect/Minimalist | 📋转述 |

**交叉命中三处（W1/W2/W3）** —— 按 leader 要求标出：独立命中比单点更有分量。

---

## A4. 🔴 最终 `review_fix` 清单（**只剩一条**，且源状态合法）

| 任务 | 当前状态 | fix_items | reason_class |
|---|---|---|---|
| **TASK-008** | **`verified`** ✅ 边合法 | 补**经 cobra 参数解析**的 flag 绑定守卫：`SetArgs`/`ParseFlags` 后断言 `hestiaForce == true`、`hestiaCfgPath == 传入值`。判据用**肯定式**（解析后变量等于传入值），不得用「flag 存在」 | **`task_defect`** |

**其余全部不建议开 `review_fix`**：

- C1 / W1 / W2 / W3 / W4 —— **已在 TASK-007/009/011 的在途返工里修掉或部分修掉**，
  它们是 `in_progress`，**没有 `verified → review_fix` 这条边**。建议在**验证环节**逐条确认。
- W5 / S1 / S2 / S3 —— 建议进 final-report 遗留清单，**不阻断本 Sprint**。
- §3 那 6 处历史行号锚 —— 全部早于 `63ac5b6`，单开 chore。

⚠️ **给 TASK-011 验证者的一条补充**：在途新增的 `TestIngestReportsStopReasonEvenWithCandidates`
断言的是 `discover stopped:`（**有候选**分支）；**`len(cands)==0` 的 `no new reports (stopped: %s)`
仍然没有任何断言** —— 我的 M6b 变异（删掉那半）在 `bb825de` 上是 **948 PASS / 0 FAIL 全绿**。
建议顺手补一条，成本一行。

---

## A5. 待同步 hooks 清单（`.claude/` 只读，须人类会话外执行）

**`teammate-idle.sh` 的 `qa-*` 分支不按归属过滤**（Architect 诊断，📋转述 + 我复核了后果）：

```bash
# teammate-idle.sh:76-86
MINE=$(query_tasks 'select(.status == "verified") | .id')     # ← 不按归属过滤
```

`dev-*` 用 `.assigned_to == $me`、`test-*` 用 `.verifier == $me`，**唯独 `qa-*` 取全体 verified**
⇒ 只要还有任何 `verified` 任务，`MINE` 恒非空 ⇒ 只读 lens **结构上无法停机**，
而唯一逃生口（`05-review/` 下 mtime 更新的 `*.md`）**恰恰是 lens 被禁止做的事**。

⚠️ **这是第三次**：hook 自己的注释记着 sprint-033 的 `qa-agent-10` 被唤醒约 1500 次，
**上次补救只改了文案没动控制流** —— 而「交回父实例」不改变 `MINE`。

⇒ 建议：`qa-*` 分支按归属过滤，或给只读 lens 显式标记走 `*)` 分支。

---

## A6. 终审 verdict（更新）

### **CONTESTED（维持）** —— 理由与上文 §0 相同，但清单已收敛

- **唯一需要你动的**：TASK-008 走 `verified → review_fix`（C2，`task_defect`）。
- **TASK-007/009/011 在途**，其中包含对 C1/W1/W2/W3/W4 的修复 ⇒ **Sprint 仍不具备终审条件**，
  待三者提交后**补一轮**（重点：确认 `plistSchedule` 有效、`len(cands)==0` 分支的 stop reason）。
- **不建议把返工拆多轮**：C2 与在途三个任务**互不重叠**（`cmd/atlas/hestia.go` 的 flag 绑定 vs
  `hestia_test.go` 的 plist 守卫），可并行，不额外消耗 `rework_count`。

---
---

# 附录 B · 最终轮与终审 verdict（qa-agent-13，第三次落盘）

**新锚点**：`f4d601753ac323ca37d5757da5a547493e3de090`（上文 §0–§12 锚定 `bb825de`、附录 A 锚定在途工作区，**均不改**）

## B1. 终审前置条件 —— 我自己量的，不引用 leader 结论

```
HEAD                    f4d601753ac323ca37d5757da5a547493e3de090   ✅ 与 leader 声明一致
工作树                   只剩 .arcforge/（write-matrix.json + 未跟踪产物）✅ 代码零未提交
11 个任务                 001…011 全部 verified                      ✅
VALIDATOR_EXIT=0         ✓ 11 个任务、19 条规则，**告警 0 条**        ✅（先前 12 条假阳已随任务离开在途而消失）
C2 守卫已进 commit        f4d6017 「38  0  cmd/atlas/hestia_test.go」纯新增、在 TASK-008 的 writes 内 ✅
```

## B2. 机械门禁（`f4d6017`）

```
go build ./...                                    BUILD_EXIT=0
go vet ./internal/hestia/... ./cmd/atlas/...      VET_EXIT=0（无输出）
gofmt -l <本 Sprint 改动的 .go 文件>               0 个不合规
go test ./internal/hestia/... ./cmd/atlas/... -v  TEST_EXIT=0   954 PASS / 0 FAIL
internal/hestia 覆盖率                             93.6%（Sprint 起点 92.1%，上一轮 93.5%）
cmd/atlas 覆盖率                                   75.4%；hestia.go: init 100 / openHestia 100 /
                                                   runHestiaIngest 50.0 / runHestiaStatus 84.6
```

## B3. 🔴 最终变异复核 —— **五条全部 KILLED**（我自己跑的，不引用 Skeptic / test-agent-26）

隔离 detached worktree @ `f4d6017`；基线 `EXIT=0 / FAIL=0`。

| # | 变异 | 上一轮 | 本轮 | 杀死它的具体断言 |
|---|---|---|---|---|
| **M6b** | 零候选分支丢掉 `(stopped: %s)` | 🔴 **SURVIVED**（948 PASS 全绿） | ✅ **KILLED** | `TestIngestReportsNothingNewOnSecondRun` |
| **M13** | `--force` 解绑（`BoolVar`→`Bool`） | 🔴 **SURVIVED** | ✅ **KILLED** | `TestHestiaFlagsBindToVariables/--force_绑到_hestiaForce` |
| **M14** | `--hestia-config` 解绑 | 🔴 **SURVIVED** | ✅ **KILLED** | `TestHestiaFlagsBindToVariables/--hestia-config_绑到_hestiaCfgPath` |
| **M_A** | `StartCalendarInterval` 键名打错一字母 | （C1 缺陷态：全绿） | ✅ **KILLED** | `TestHestiaPlistSchedulesThreeTimes` |
| **M_B** | Hour/Minute 跨 dict 错配 | （C1 缺陷态：全绿） | ✅ **KILLED** | `TestHestiaPlistSchedulesThreeTimes` |

⇒ **M_A/M_B 是我对 C1 修复的独立复核** —— 此前我只看到 `plistSchedule` 的源码，没有量过它是否真的守得住。
⇒ **M13/M14 各自只杀死自己那个子断言、不误伤兄弟** ⇒ 因果干净，两个 flag 是**两个独立缺陷**（旧树上两条都全绿）。

## B4. 消费者位复跑（`f4d6017` 构建，真实网络）—— 四条修复全部可见

```
首跑  EXIT=0
  db: /…/final-consumer/data/hestia.db                                    ← W4 cwd 守卫已生效
  WARNING: discover stopped at max_pages (3) with 1 candidate(s): …       ← W1，走 stderr，且**不改退出码**
  discover stopped: max_pages (1 candidate(s))
  2026-06 New → hestia_observations
二轮  EXIT=0   no new reports (stopped: seen_article)                      ← 幂等 + 停止原因
force EXIT=0   2026-06 Duplicate → hestia_observations                    ← 穿透两层、走到 Save
status EXIT=0  observations: 1 / 2026-06 h1 published 2026-07-15 rule@v2
```

⚠️ **`max_pages` 那条 WARNING 刻意不改退出码是对的**：空库首跑必然 `max_pages`，改退出码会产出**假红**，而假红会被训练成忽略 —— 比不报更糟。

## B5. 三份过期副本的最终核实（含一次我自己的判据失误）

| # | 位置 | 结果 |
|---|---|---|
| ① | `ingest.go` 「恒为 New」 | ✅ 已清（grep 零命中） |
| ② | `ingest_test.go` | ✅ **事实陈述已删、决定与理由保留**，并把 `discover.go:303-318` 换成字串锚（「搜 `判据是 article_id`」）⇒ 同时兑现了 QA-PRE-1d |
| ③ | `store.go` 「Discover 用它决定翻页何时停」 | ✅ 已清 |

🔴 **我在核 ② 时踩了 H32**：我的否定式 grep（`当前不可达`）**命中了 `:296`**，
而读原文才发现那是**「保留并驳回」**——原句被引在一段写着「**三重过期 … 事实陈述已删**」的说明里。
⇒ **H32 预言的假阳，我照着踩了一次**：判据应当问「新说法在不在」，而我问了「旧说法没了没」。
**写得越好的订正，否定式判据的假阳率越高** —— 这次的订正正是写得好的那种。

## B6. 🔴 我这一轮的两次工具误导（如实记，与验证者的 7 次同族）

| # | 误导 | 真相 | 排除方式 |
|---|---|---|---|
| 1 | `grep -n 'stopped' --include='*_test.go'` **只 1 处命中** ⇒ 我以为零候选分支仍无断言 | 该测试用的是 **`string(StopSeen)` 这个类型常量**，不是字面量 `"stopped:"` | **跑变异**，M6b 当场红 |
| 2 | 我先在 `ingest_test.go` 里搜那两个测试函数名 ⇒ **零命中**，一度以为 leader 说错了 | 它们在 **`discover_test.go:1328/1356`**；是我把搜索范围钉错了文件 | 扩到全仓重搜 |

⇒ 两次都是**「grep 判字面、而我要判的是语义」**。
⇒ 与 test-agent-26 那句可以并列，**它的表述比我准，原样引**：

> **验证者的假阳/假阴不是偶发，是常态。判定之所以没错，不是因为我少犯错，是因为每次都回到原文/换个位置再查一遍。**

⚠️ 而我这两次的排除手段里，**真正决定性的是变异（第 1 条），不是再读一遍** ——
再读一遍能纠正「我搜错了文件」，纠正不了「我不知道它用的是常量」。
⇒ **补一句**：当判据是「某个性质有没有被守住」时，**grep 回答不了它，只有变异能**。

## B7. 降级申明（按事实，不粉饰）

| 项 | 事实 |
|---|---|
| **跨模型** | **未降级**。`codex-cli 0.139.0` 可用并已跑 read-only 独立审查；`gemini` 不可用。codex 的 7 条结论逐条复核：**采纳 3 / 降级 1 / 不采纳 2**。 |
| **视角轮** | **部分降级，且降级是有代价的**。三个 lens 的结论**发给了 `main` 而非我**，我全程未直接收到；**其中两个被 leader `TaskStop`**（为止住 `teammate-idle.sh` 的无限保活），Minimalist 在补位实例里返回空。⇒ **止血本身有代价：我失去了对三份 lens 原始报告的直接访问**，合并件里凡我未复核的均标 📋转述。我用 **19 条变异实测**（两轮合计）顶上 —— 变异可证伪，视角推理不可。 |

## B8. H48 待证伪判据 —— 按约定的结论

**写成「一个案例的归纳」，不得写成「已验证的方法」。**
我补跑两个域（11 份 plist × 3 解析器；YAML 键 vs `mapstructure` tag），**真缺陷 0，没有第二个样本。**

**保留的那一点**：域②（差集非空 10、真缺陷 0）是 leader **更正后**那版判据的干净实例
⇒ **原版（「差集就是攻击样本」「不一致处必然有一道是错的」）在域② 上会直接产出 10 条假发现**，
**被更正掉的那版现在有了反例**，更正版的 n 从 1 变 2。

**另需记明**（leader 已认，我同意）：这条判据**不是本 Sprint 的方法创新** ——
`TASK-003` 的验证者当时就跑了双向差集（「A1 时 reflect PASS/AST FAIL，A2 恰好反过来」），
比我们把它总结成判据早得多。**它是把一个已有的好实践显式化。**

---

## B9. 🔴 终审 verdict：**PASS**

**全部 CRITICAL 与 WARNING 均已闭合，且闭合结论由我自己的变异实测坐实，不依赖任何转述。**

| # | 级别 | 发现 | 终态 |
|---|---|---|---|
| C1 | CRITICAL | 排班守卫不限定作用域 | ✅ **闭合**（M_A/M_B 双杀） |
| C2 | CRITICAL | flag 绑定无守卫 | ✅ **闭合**（M13/M14 双杀，已提交 `f4d6017`） |
| W1 | WARNING | `StopReason` 丢失 | ✅ **闭合**（M6b 已 KILLED + 消费者位可见） |
| W2 | WARNING | `HasPeriod` 文档过期 | ✅ **闭合** |
| W3 | WARNING | `--force` help 承诺过头 | ✅ **闭合** |
| W4 | WARNING | ingest 无 cwd 守卫 | ✅ **闭合**（消费者位可见 `db: <abs>`） |

### 遗留清单（**不阻断**，建议进 final-report）

| # | 级别 | 项 |
|---|---|---|
| W5 | SUGGESTION | `TestQuarterlyPeriodsAreCumulative` 用**枚举**列表（`[]string{"一季度","前三季度"}`）⇒ 将来新增期次类型时，`periodAlt`/`cumulativePeriods` 漏改一处**不会红**。CONTRACTS 已登记「两处都要改」，但那是文档不是守卫。 |
| S1 | SUGGESTION | `scanPage` 接受 index 页里的**绝对 URL** ⇒ 跨源抓取面（需央行页面被污染才可利用）。一行同源约束可闭合。 |
| S2 | SUGGESTION | `status` 是只读诊断却会 `MkdirAll` + 建库；错误 cwd 下会造一个空库并如实报「0 期」。 |
| S3 | SUGGESTION | 6 处失效行号锚（`CONTRACTS.md` / `store_test.go`），`git blame` 确认**全部早于 `63ac5b6`** ⇒ 单开 chore。 |
| S4 | 待同步 | `teammate-idle.sh:76-86` 的 `qa-*` 分支不按归属过滤 ⇒ 只读 lens 结构上无法停机（**第三次复发**）。`.claude/` 只读 ⇒ 人类会话外执行。 |

### 交付质量的一句话总结

**本 Sprint 的守卫质量在 QA 过程中被显著抬高，而抬高的部分全部经过变异坐实**：
进入 QA 时有 **3 条关键守卫是假通过**（plist 排班作用域、两个 flag 绑定），
其中 **2 条是 QA 轮次自己找出来的**，出去时**五条变异全部 KILLED**。
⚠️ 但这句话的另一半必须一起说：**那 3 条假通过在进入 QA 前，都躺在「已 verified」里**
——`948 PASS / 0 FAIL` 与今天的 `954 PASS / 0 FAIL` 在外观上完全一样。
**测试全绿从来不是守卫有效的证据；只有变异是。**
