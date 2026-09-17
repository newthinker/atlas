# Sprint M2b · 第一轮 Code Review 增补（两个只读 lens 的产出，经本体复核后纳入）

- 审查者：qa-m2b
- 对象：master `d42435241b3c7a24c10d2b016db17c364b325631`
- 来源：我 spawn 的两个只读 lens 子代理（lens A = 测试是否真在守；lens B = 错误处理/安全/并发）。
  它们的完整正文经 Leader 转回。**本文件里的每一条定级都是我本体复核后自己下的**，
  与子代理和 Leader 的定级不一致处逐条说明理由。

**为什么另起一份而不是重写 round1**：写通道的 `doc` 是全量覆盖写，重写 277 行有丢内容的风险；
且这批发现的来源（子代理产出 + Leader 复核 + 我复核）与 round1 的自主发现是两条不同的取证链，
分开更可审。两份合起来构成完整的第一轮。

---

## 一、恒真断言：我先怀疑 Leader 的论证，核实后推翻了自己的怀疑

**位置**：`internal/hestia/sheets/push_test.go:330-336`

Leader 给的论证是「`CreateYearTab` 四步全在一个 `BatchUpdateSpreadsheetRequest` 里、只发一次
BatchUpdate ⇒ 写请求恒为 1 ⇒ `require.Len(writeBodies(rec), 1)` 恒真」。

**我起初认为这个论证有漏洞**：它只覆盖了建表那一次写请求，没有排除 `WriteCells` 那一次。
若成功路径下还会发一次 `values:batchUpdate`，写请求就是 2，断言并非恒真。

**核实后我的怀疑不成立。** 读 `push_test.go:330-336` 全文发现：该用例**直接调
`c.CreateYearTab(...)`，不经过 `Push`**。而 `WriteCells` 是 `Push` 第 7 步的事，
`CreateYearTab` 内部根本没有第二次写请求可发。所以在当前实现下，该断言确实恒真。

**记这一笔的理由**：Leader 的结论对、论证也对，是我读得不够。但「论证只覆盖了一半」这个怀疑
本身是该提的——如果那个用例走的是 `Push`，同一条断言就不恒真了。**判断一条断言是否恒真，
必须先确定它调用的是哪一层**，这是我这次差点搞错的地方。

### 我的定级：**WARNING（高位）**，不是 CRITICAL

子代理与 Leader 都判 CRITICAL，我判 WARNING，理由：

- 这是**测试缺陷，不是生产缺陷**。它守不住东西，但它没让任何错误代码通过——
  `push.go:176-188` 的建表失败处理逻辑我逐行读过，是正确的。
- 与 CRITICAL-1（生产上必现的功能失效）、CRITICAL-2（会静默产出错数据）不是一个量级。
  把它和那两条放进同一级，会让 CRITICAL 这个标签失去分辨力。
- 但它在 WARNING 里排最高位，因为子代理指出的那点成立且重要：
  **前几条「声称为真但无测试守卫」是没写断言，这条是写了一条恒真的断言**，更隐蔽——
  它让人以为有守卫。

### 它与零覆盖是同一缺口的两面（这一点 Leader 说得对）

- `push.go:177-179`（`createYearTabs` 失败 ⇒ 必须在 `WriteCells` 之前返回）：**零覆盖**
- `push_test.go:335` 自称守这条性质，实际只守了 `CreateYearTab` 单元的行为，且恒真

⇒ **真正缺的测试是：在 `Push` 层面让建表失败，断言 `WriteCells` 没被调用。**
这条测试一写，`push.go:177-179` 的零覆盖同时消掉。建议合并成一个修复项。

### 那条断言并非绝对恒真，值得写进注释

如果将来有人把四步拆成多个独立 BatchUpdate 请求，失败后不停止就会发出 2–4 个写请求 ⇒ 断言转红。
所以它守的是「四步不被拆成多请求」这个变体。建议把注释改成它真正守的那件事，
或直接按上面那条在 `Push` 层重写。

---

## 二、零覆盖分布：我自己数的，`client.go` 比 `push.go` 还多

子代理报 `push.go` 6 个，Leader 复核为 8 个。**我自己跑 coverprofile 解析，得 8 个，与 Leader 一致**
（子代理漏的是 55-57 与 65-67，都在 `createYearTabs` 内）。

但两边都只展开了 `push.go`。我把三个文件都数了，附自洽校验：

| 文件 | 零覆盖块 | 总块 | 自洽（零+已覆盖==总） |
|---|---|---|---|
| `client.go` | **13** | 64 | 13+51==64 ✅ |
| `push.go` | 8 | 58 | 8+50==58 ✅ |
| `diff.go` | 4 | 28 | 4+24==28 ✅ |
| `header.go` | 0 | 15 | 0+15==15 ✅ |

`client.go` 的 13 个零覆盖块里，有三条我认为比 `push.go` 那 8 条更要紧：

### ① `client.go:152` —— `wrapErr` 的非 googleapi 分支零覆盖，而生产上走的正是这条

```go
var ge *googleapi.Error
if errors.As(err, &ge) {
    return fmt.Errorf("… HTTP %d %s: %w", …)   // ← 测过
}
return fmt.Errorf("hestia sheets: %s: %w", action, err)   // ← 零覆盖
```

测过的是 HTTP 错误分支（替身能返 4xx）。零覆盖的是**网络层失败**分支：连接超时、DNS 失败、
代理不可用。而 round2 的 CRITICAL-3（launchd 无代理键）说的生产形态，走的恰恰是这条。

⇒ **我们测过的是不会发生的那个分支，没测过的是会发生的那个。** httptest 替身永远建立得了连接，
这个形状差异没有任何断言记录过。

### ② `client.go:129-131` —— `len(changes)==0` 零覆盖 ⇒ 幂等的 apply 侧无守卫

`push_test.go` 有 6 个 `Apply: true` 用例，但**没有一个是 `WillWrite == 0` 的**。
⇒ 判据六（幂等：全一致时再跑一次 `--apply` 应当零写请求）的 apply 侧**结构上没有测试**。
这条与 CRITICAL-1 直接相关：修好渲染选项之后，正是这条测试来证明幂等真的达成了。

### ③ `client.go:204-206` —— 模板标题不带年份的分支零覆盖

`client.go:199-206` 的注释专门说明了两个分支（标题带年份 ⇒ 只换年份；不带 ⇒ 拼
「`<year>` 年 · `<原标题>`」），只测了一个。空标题也走这条。

其余 10 个零覆盖块（60-62、69-71、82-84、97-99、100-102、119-121、180-182、196-198、238-240、241-243）
全部是「读路径失败」与「空响应」，与下一节的替身粒度问题同源。

---

## 三、回答 Leader 的问题：还有哪些地方，替身与真实系统的形状不同而无人断言过

这是 CRITICAL-1 暴露出的那一类问题。我系统排查了一遍，**六条**，按现网风险排序：

| # | 替身的形状 | 真实系统的形状 | 有无断言 | 对应零覆盖 |
|---|---|---|---|---|
| 1 | `values.get` 返 JSON 数字（`412.5`） | 默认 `FORMATTED_VALUE` 返格式化字符串 | 无 | 走不到 `diff.go` 的数值路径 |
| 2 | httptest 永远连得上 | 连接超时 / DNS / 代理不可用 | 无 | `client.go:152` |
| 3 | 4xx 只有「全路径失败」与「全 POST 失败」两种粒度 | 读请求单独 403 / 400 Unable to parse range / 404 | 无 | `push.go:76,84,130`；`client.go:82,97,119,238` |
| 4 | `values:batchUpdate` 响应返 `{}` | 返 `{totalUpdatedCells, totalUpdatedRows, …}` | 无。**且产品代码也丢弃**（`client.go:140` 用 `_`，全仓 `TotalUpdatedCells` 命中 **0**） | — |
| 5 | `/token` 恒成功 | 凭据过期 / 被撤销 / 时钟偏移 | 无 | `client.go:60-62` |
| 6 | `ReadHeader` 总能返回非空表头 | 表头行可能全空（新表未初始化） | 无 | `client.go:100-102` |

**第 4 条值得单独说**：替身返 `{}`、产品代码用 `_` 丢弃返回值 ⇒
**「API 返回 200 但一格都没写」这件事在结构上不可能被任何测试或运行时检查发现**。
这不是漏了一条用例，是一条反馈回路整个不存在。建议 `WriteCells` 断言
`resp.TotalUpdatedCells == len(changes)`，不等就返回错误。

**共同形状**：这六条没有一条是「写错了代码」，全部是「替身比真实系统仁慈」。
而仁慈的方向是一致的——**替身从不失败、从不返回意外形状**。
⇒ 建议把「替身与真 API 的形状差异」在 `push_test.go` 顶部列成一段注释，
让下一个写测试的人知道哪些形状是被假设掉的。这比逐条补测试更持久。

---

## 四、两个 lens 其余条目的定级（本体复核后）

### 纳入、我认同其定级

| 条目 | 位置 | 级别 | 我的复核 |
|---|---|---|---|
| `Period` 定长切片 panic 在 recover 之外 | `sheets_project.go:127-128` | WARNING | 与我 round1 的 WARNING-1 **是同一条**，lens B 独立发现，证据链更全（它补了 `schema.go:73` 的 period 列无 CHECK 约束这一点） |
| 每候选建新 client + 全表重推 | `hestia_sheets.go:160-172` | WARNING→CRITICAL | 与我 round1 WARNING-2 同条，round2 已升 CRITICAL-5 |
| `TestClientDoesNotUseBareTransport` 三重窄射程 | `client_test.go:202-209` | WARNING | 属实。它是对**源码文本** grep `"http.Transport{"`，新增 `transport.go` 即逃逸、带空格即不匹配，且「没有裸 Transport」不蕴含「用的是默认 transport」。与 round2 的 CRITICAL-3 相关：C7 的守卫方式本身就不够 |
| `TestSheetsPushDefaultsToDryRun` 空库致平凡为真 | `hestia_sheets_test.go:240-251` | WARNING | 属实。`t.TempDir()` 空库 ⇒ 0 行 ⇒ `WillWrite==0` ⇒ 即便 `--apply` 也不发写请求。同作者在 `push_test.go:122` 写了 `Greater(WillWrite,0)` 的防空跑护栏，这条缺 |
| `values:batchUpdate` 响应双双忽略 | `client.go:140` | WARNING | 见第三节第 4 条 |
| GET 路径 4xx 从未走过 | `newTestClient` | WARNING | 见第三节第 3 条。它建议的改法（加 `failPath map[string]int`）对症 |
| `.gitignore` 无 `*-sa.json` 规则 | `.gitignore:29-31` | SUGGESTION | 属实。`config.example.yaml` 建议把密钥放仓库外（做法对），但没有机制拦住「顺手放进 `configs/` 再 `git add .`」 |
| `fake-sa.json` 是真生成的 2048 位 RSA 私钥 | `sheets/testdata/` | SUGGESTION | 属实且**确系假凭据**（`token_uri` 指 `127.0.0.1:1`，出不了网）。风险只在 secret scanner 误报 |
| `WriteCells` 批量不分片 | `client.go:128` | SUGGESTION | 属实，最坏 2520 个 ValueRange 一个请求 |
| `rangeFormat` 不做 A1 单引号转义 | `client.go:23` | SUGGESTION | 属实且**今天不可达**——它的不可达追证做得好：远端表名只用于 `have[t]` 判存在与排序，**从不进 range 字符串**。建议仍加那一行替换（对现有表名是恒等的） |

### 纳入但我调整了定级

| 条目 | 子代理定级 | 我的定级 | 理由 |
|---|---|---|---|
| 恒真断言 `push_test.go:335` | CRITICAL | **WARNING（高位）** | 见第一节 |
| `push.go` 六条 error 传播零覆盖 | CRITICAL | **WARNING** | 零覆盖本身不是缺陷，是缺守卫。其中 177-179 因与恒真断言构成同一缺口，**并入那一条一起修** |
| `diff.go:62` 行号无钳制 | SUGGESTION | **WARNING** | **我升级了它。** lens B 指出 `month=0 ⇒ Row=3`，而 `headerRow` 正是 3 ⇒ 会把「0月」写进**表头行**，破坏 C3 依赖的表头本身。这比「写错一格」严重一档——它会让后续所有投影都因表头解析失败而停摆。与 round2 的 WARNING「改一个表头字致投影永久停止」是同一后果 |

### 不纳入

| 条目 | 理由 |
|---|---|
| `toFloat` 的 `float32/int/int64` 零覆盖 | lens B 自己判定「不可达，是死代码而非漏测」，我认同。`ValueRange.Values` 经 JSON 解码只会产出 `float64` 与 `string`，那三个 case 永不执行。**但这也说明 `string` 才是真正该有的 case**，与 CRITICAL-1 的修法 b 一致 |
| `header.go:24` 重复标签取最左零测试 | 属实，且是「覆盖率绿 ≠ 性质有人守」的干净一例（跳过分支无语句，覆盖率不可见）。但该性质本身是防御性的、不影响正确路径，留 SUGGESTION 记录即可 |
| `client.go:53` 凭据**路径**进错误消息 | 只泄路径不泄内容，诊断价值高于风险。lens B 自己也说「不外送则保留现状即可」 |
| 并发、凭据泄漏、资源泄漏三项 clean | 我认可其方法：`go test -race` 三包全绿、零 `t.Parallel()`、生产路径无 goroutine；19 个调用点无 ID 或密钥进日志/错误/契约；`AllPeriods` 的 `defer Close` + `rows.Err()` 三要素齐全 |

---

## 五、对 Leader 的 `review_fix` 落点规划的回应

Leader 的规划我**基本同意**，三处建议调整：

### 同意

- **CRITICAL-1 挂 TASK-006**，`update` 其 `writes` 纳入 `diff.go` / `diff_test.go` / `push_test.go`，
  走越界申报的正规路径。
- **WARNING-1（recover 上移）挂 TASK-010**，把 `sheets_project.go:127-128` 的读侧防御一并纳入。
- 两者 `writes` 不重叠，scope-mutex 可过。

### 建议调整

**① 把「超时」从 CRITICAL-5 里拆出来，现在就做。**

Leader 倾向 WARNING-2（API 读放大）整条进 final-report，理由是两种改法都改变执行语义。
**这条我同意一半**：投影移出 `ingestOne` 或 client 提到循环外，确实改变执行语义，该单独立项。
但 **`context.WithTimeout` 不改任何语义**，是一行，且它防的是一个独立的真实风险：
`cmd.Context()` 无 deadline、`sheets.NewService` 无 `http.Client.Timeout` ⇒
境外端点被黑洞时 ingest 进程无限期挂起 ⇒ launchd 同 label 不并发 ⇒ 后续唤起全部不执行，
最终由 `hestia_stalled`（>30h）兜住，而告警文案会把人指向「launchd 没跑」这个错误方向。
⇒ **建议超时进本批**（可挂 TASK-010 或 006），放大与重试进 final-report 单独立项。

**② round2 新增的 4 条 CRITICAL 的落点建议**

| 条目 | 建议落点 | 理由 |
|---|---|---|
| CRITICAL-2 模板自污染 | **TASK-008** | `CreateYearTab` 与 `templateYearTab` 都是 008 的核心交付 |
| CRITICAL-3 plist 无代理键 | **新建任务** | 它跨 `deploy/launchd/` 与 `cmd/atlas/hestia_test.go:477`，**不在任何 M2b 任务的 `writes` 里**，且要改的那条守卫是 M2b 之前就存在的。塞进现有任务会让 scope 声明失真 |
| CRITICAL-4 配置校验 | **TASK-010** | `config.go` 的 `HestiaSheets` 是 010 的交付。可与 recover 上移同批 |
| CRITICAL-5 超时部分 | **TASK-006 或 010** | 见①；放大与重试部分不进本批 |

**③ 恒真断言与 `push.go:177-179` 零覆盖合并成一个修复项**

它们是同一缺口的两面（见第一节）。修法是在 `Push` 层写一条「建表失败 ⇒ `WriteCells` 未被调用」
的测试，一条测试同时消掉两个问题。挂 TASK-008（与 CRITICAL-2 同一任务、同一文件）。

### 关于「能力当前禁用」的边界，我接受 Leader 的修正

我在 round1 写「能力当前在生产上禁用，所以没有正在发生的损害」。Leader 指出**它缓和的是紧迫性、
不是必要性**——人执行清单第 1 条正是拍板凭据落位，一旦落位能力就开。**这个修正我接受**，
并且 round2 把它变得更尖锐：`configs/config.yaml:332-334` 已经填了真实凭据，
只是填在了没有代码读的位置。⇒ **启用动作已经被尝试过一次了**，紧迫性比我 round1 判断的高。
