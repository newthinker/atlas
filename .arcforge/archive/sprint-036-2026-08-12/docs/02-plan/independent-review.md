# 独立 reviewer 反审结论 — Sprint 036

**方法**：spawn 一个只读上游计划 + spec + 既有代码、**禁读 `.arcforge/`** 的独立 agent。
它在 `git worktree add --detach /tmp/reviewer-dod f5a17d5` 隔离副本里**按计划正文逐字落盘**
T1–T7 全部代码与测试，实际编译、实际跑、**实际联网**。

**Leader 复核**：`git status` 确认主仓库零改动、无残留 worktree；F6 已亲自复验（见下）。

---

## 一定会发生（reviewer 已在隔离副本实际复现）

### 🔴 R1 导出面守卫必红，且 **8 处「预期整包绿」中有 7 处为假**

逐个实测（每次只加一个导出物后跑两条守卫）：

| 新增导出物 | 任务 | reflect 版 `:361` | AST 版 `:406` |
|---|---|---|---|
| `NewPBOCFetcher` | T1 | 绿 | 🔴 **红** |
| `Store.HasPeriod` | T4 | 🔴 **红** | 🔴 **红** |
| `Discover` | T5 | 绿 | 🔴 **红** |
| `LoadConfig` | T7 | 绿 | 🔴 **红** |
| `Fetcher`/`PeriodChecker`（接口） | — | 绿 | 绿 |
| `Candidate`/`DiscoverCfg`/`Config`（结构体） | — | 绿 | 绿 |
| `pbocFetcher.Get`（未导出接收者） | — | 绿 | 绿 |

⇒ **Leader 在 AD-036-2 的推断全部被实测确认**（守卫只看 `FuncDecl`；未导出接收者被 `ast.IsExported(recv)` 排除）。

**触发时点比预估更早**：不是最后整包跑才发现，而是 **T1 Step 7** 就红（该 Step 明文写「预期：全绿」）。
计划全文**零处**提及导出面守卫，而 T1–T7 共 8 处写着「整包绿」，**T1 Step 7 之后每一处都是假的**。

**reviewer 建议把守卫同步单列成一个任务排在末尾。Leader 不采纳**，理由：

> `dev_done` 门禁**内建执行 `go test $PKGS`**，守卫红就 DENY ⇒ 每个任务必须自己让整包绿，
> 单列成任务会让 T1/T4/T5/T7 全部卡在门禁外。
>
> 而 reviewer 担心的「四个任务竞写同一个断言列表」**在本 Sprint 的 wave 划分下不成立**：
> T1(w1)/T4(w3)/T5(w4)/T7(w5) **分处四个不同 wave、严格串行**，每个在前一个基础上**追加**。
> Sprint 035 实证过同一做法（T1/T3/T4 各自追加，验证者逐条核实「是追加而非放松」）。
>
> reviewer 禁读 `.arcforge/`，看不到 wave 划分，其担忧基于「按计划的并行声明」——那个声明本身是错的（见 R4）。

### 🔴 R2 T5 的两条测试与实现矛盾（**与 Leader 独立推演一致，reviewer 实测确认**）

`TestDiscoverFindsReportOnSecondPage` 与 `TestDiscoverNeverFetchesArticlePages` 都用 `MaxPages: 3`，
而 `twoPageFetcher` 只备 2 页。`Discover` **只在命中已入库期次时提前返回**，空库下恒 false ⇒ 翻到 page=3 ⇒ fake 报错。

```
--- FAIL: TestDiscoverFindsReportOnSecondPage
        fake: 没有为 .../11040-3.html 准备页面
--- PASS: TestDiscoverStopsAtKnownPeriod
--- PASS: TestDiscoverRespectsMaxPages
--- PASS: TestDiscoverDoesNotExceedTotalPages
--- PASS: TestDiscoverDeduplicatesAcrossPages
--- PASS: TestDiscoverFailsOnCheckerError
--- FAIL: TestDiscoverNeverFetchesArticlePages
```

**修法（reviewer 实测有效）**：两条的 `MaxPages` 改成 **2** ⇒ 七条全绿。
**是测试写错，不是实现写错** —— 「空库翻满 `MaxPages`」正是 spec §4.3 定义的首跑行为。

> ⚠️ reviewer 的风险判断值得原样记：**危害不止「测试红」** ——
> RED→GREEN 循环里 dev 看到红会**先怀疑自己的实现**，而计划的实现是对的。
> 有人可能去改 `Discover`（例如「发现候选就停」），**那会直接违反 spec §4.3**。

**两条独立路径撞上同一处**：Leader 在写追溯矩阵时推演出此条（标注为推断），reviewer 独立实跑证实。

### 🔴 R3 三处 `import` 清单错误，各让当步的「预期 PASS」变成 build failed

| # | 位置 | 错误 | 实测输出 |
|---|---|---|---|
| F3 | T2 Step 6 的 `discover.go` | 多了 `"net/url"`（`pageURL` 到 Step 9 才出现） | `discover.go:6:2: "net/url" imported and not used` |
| F4 | T1 Step 6 的 import 清单 | **漏了 `io`**（「超大响应被拒」用例调 `io.Copy`） | `fetch_test.go:81:11: undefined: io` |
| F5 | T5 Step 1 的 import 清单 | 多了 `"time"`（`discover_test.go` 全文无 `time.` 引用） | `discover_test.go:9:2: "time" imported and not used` |

**同一族**：计划的 Global Constraints **逐字写了这条纪律**，但只覆盖测试文件，而 F3 的违反发生在**实现文件**里。

⇒ 危害是**侵蚀 dev 对「RED 阶段预期失败信息」的信任**：计划说「预期 `undefined: X`」，
dev 实际看到 `imported and not used` —— **两者是不同的失败信号**（AD-036-6 第 1 条硬要求正是为此）。

### 🔴 R4 并行冲突比 Leader 描述的**更宽**，且计划**自相矛盾**

| 文件 | 被哪些任务写 |
|---|---|
| `discover.go` | **T2、T3、T4、T5**（四个） |
| `discover_test.go` | **T2、T3、T5**（三个 —— Leader 原先只点了 `discover.go`） |
| `store_test.go` | T4 ＋（守卫修复后）T1、T5、T7 |

**计划内部的自相矛盾**（reviewer 指出，非外部推断）：
T3 的 `Interfaces` 段自己写着 `Consumes: T2 的 pageURL`，
T3 Step 8 明文「先把 T2 的 `pageURL` 里那段 URL 解析抽出来复用」，
**而依赖顺序一节把 T2 与 T3 列为可并行**。

⇒ Leader 的 wave 划分（`w1{T1,T2,T6}` / `w2{T3}` / `w3{T4}` / `w4{T5}` / `w5{T7}`）**已覆盖此冲突**，
reviewer 建议的 `{T1}/{T2→T3→T5}/{T4}/{T6→T7}` 与之等价（T4 也写 `discover.go`，故仍须串行）。

---

## 可能发生

### 🟡 R5 `boundary[1]` 的守卫**在空集上平凡为真**（消融实证）—— 最危险的一类

`TestDiscoverDeduplicatesAcrossPages` 断言「遍历 `got`，每个期次计数为 1」，**没有 `require.NotEmpty`**。

**消融实验（实测）**：把 `scanPage` 改成恒返回 `nil, nil`：

```
--- FAIL: TestScanPage/第_2_页提取出报告条目
--- FAIL: TestDiscoverNeverFetchesArticlePages
（TestDiscoverDeduplicatesAcrossPages 未出现在 FAIL 列表 —— 它仍然绿）
```

⇒ **条目提取彻底失效时，这条「去重」用例照样 PASS。**

整包跑会被兄弟用例接住，但**验证者按 DoD 逐条单跑取证时会拿到假绿**，
且会作为「已验证」写进验收矩阵。修法一行：开头加 `require.NotEmpty(t, got)`。

**这是 Sprint 035 反复出现的「否定式/遍历型断言在空集上平凡为真」的又一例**（F14 同族）。

### 🟡 R6 快照时效性 —— **窗口正在关闭**

报告发布于 2026-07-15，**已 28 天**，位于第 2 页 15 条中的**第 5 条**；第 2 页最旧一条是 2026-06-28。
按每页约覆盖 20 天算，**约 1–2 周内会掉到第 3 页**。

⇒ **抓快照必须是 TASK-002 的第一个动作。** 计划的「若 p2 没有则改抓 p3」应急分支**今天用不上**，但窗口有限。

（第 3 页也含 `jumpTo`，总页数一致 408 —— 万一需要往后翻，机制成立。）

### 🟡 R7 `pageURL` 注释的事实错误（**Leader 已亲自复验**）

计划注释：「模板生成的 `11040-1.html` **未必存在**（实测 `index_1.html` 是 404）」。

Leader 复验（2026-08-12）：

```
11040-1.html: HTTP 200, 38147 bytes
index.html:   HTTP 200, 38147 bytes
index_1.html: HTTP 404
diff 11040-1.html index.html  ⇒ 逐字节相同
```

⇒ **`11040-1.html` 存在、返回 200、与 `index.html` 逐字节相同**。
计划用关于**另一个 URL**（`index_1.html`）的观察，去论证 `11040-1.html` 的存在性。

**行为仍正确**（第 1 页复用已取回的字节，避免重复请求，也让 `f.calls` 断言干净），**但理由是错的**。
⇒ 又一例「结论对、理由错」—— 结论正确会让判断永不被复查，而理由是后人复现时的唯一入口。
**要求 T2 把注释改成真实理由**（规范化 URL + 不重复请求），别留一个会被后人当实测事实引用的错句。

### 🟡 R8 「9 处构造点」实际 **7 处**，且 `PeriodTypes` 检查的插入位置会改变既有用例的错误串

计划列了 7 个行号却说 9 处。reviewer 实测真正因此转红的是 **7 处**
（`thresholds_test.go:64/107/145` + `validate_test.go:834/856/879/1016`）。

`validate_test.go:212` **不需要改**：`Version` 的枚举校验排在新增的 `PeriodTypes` 检查之前，
该用例断言的正是版本非法，先行返回。

**更值得 dev 知道的**：实测 `TestExemptionRejectsUnknownCheckID`（`:879`）在补 `PeriodTypes` 之前是红的，
**红的原因是错误串变成了「PeriodTypes 为空」而不是 `deposit_summ`** ——
**测试红了，但红的理由与它要守的东西无关**。

### 🟢 R9 两条覆盖缺口（非阻断，DoD 可补）

- `parsePaging` 的「总页数非法（0/负数/非数字）」分支**有实现、无用例**
- `fakeFetcher.err` 字段**定义了却从未被独立测过**（抓页失败路径）

---

## reviewer 实测确认**正确**的部分（避免 dev 白花时间复核）

| 计划的声称 | 实测 |
|---|---|
| 构造点 10 处 / 调用点 4 处 / 生产侧仅 `validate.go:93` | ✅ 行号逐个吻合（与 Leader 独立实测一致） |
| `validMeta().PeriodType` 是 `"h1"` | ✅ `types_test.go:32` |
| viper 预填 `DefaultThresholds()` 再 `Unmarshal`，未写的键保持默认 | ✅ **一次通过**；Step 5 的 `v.SetDefault` 退路**用不上** |
| `timeout: 30s` 解成 `30*time.Second` | ✅ viper 默认带 `StringToTimeDurationHookFunc` |
| T6 新增 4 组 10 条子测试、T7 全部 9 条 | ✅ **各自一次全绿** |
| T1/T2/T3/T4 全部用例（补齐 import 后） | ✅ 全绿 |
| **计划把 spec 的 `non_functional[0]` 测试推翻重写** | ✅ **计划最有价值的一处修正**：spec 原版（httptest + Setenv + `require.NoError`）对代理行为零鉴别力 |
| 修完 R1/R2 后全量 | ✅ `-count=1` 绿、`-race` 绿、`gofmt`/`vet` 空、`go build ./...` 绿 |

### `articleLinkRE` 对真实 HTML 的匹配（reviewer 实跑，最关键的一条）

两页各**恰好**命中 15 条链接 = 每页条目数，无漏无重。正则能穿过 `onclick`/`target`/`title`/`istitle`
四个属性，`(?s)(.*?)</a>` 正确取到链接文本而非 `title=` 属性值。

**⭐ 干扰项断言不是平凡为真**：`国新办…金融统计数据情况` **确实被 `articleLinkRE` 提取到了**
（它就在第 2 页命中列表里），是被 `parsePeriod` 过滤掉的 —— **过滤器在真实做功**。

**额外收获**：`TestParsePeriodRejects` 用的 `2026年二季度金融机构贷款投向统计报告` 与
`2026年6月金融市场运行情况` **是第 1 页上真实存在的条目**，不是编的。
这两条同时解释了「第 1 页产出 0 个候选」**不是因为提取失败** —— 第 1 页有 15 条被提取、**全被正确拒绝**。

### 代理绕过（reviewer 用生产实现联网实测 + 阴性对照）

```
NewPBOCFetcher 直连 OK, 38147 bytes
对照组（走 ProxyFromEnvironment）: proxyconnect tcp: dial tcp 127.0.0.1:1: connect: connection refused
```

⇒ **阴性对照成立：代理绕过是承重的，不是装饰。**

**一处措辞修正**：计划说「不带 User-Agent —— 与生产实现一致」。实际 `curl` 默认发 `curl/8.x`、
Go 默认发 `Go-http-client/1.1`，**两者都不是「不带 UA」**。真正被验证的是「用 Go 的默认 UA 能拿到 200」。
⇒ 抓快照若想与生产完全同构，**应当用 Go 而非 curl**。

**一处非阻断观察（推断）**：`&http.Transport{}` 是裸零值，相比 `http.DefaultTransport` 缺
`DialContext` 超时、`TLSHandshakeTimeout`、`ForceAttemptHTTP2`、`IdleConnTimeout`。
实测能正常工作（`Client.Timeout` 兜住整体时长），**本迭代不处理，仅记录**。
