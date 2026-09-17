# Sprint M2b · 第二轮 Code Review（跨视角对抗）

- 审查者：qa-m2b
- 对象：master `d42435241b3c7a24c10d2b016db17c364b325631`
- 视角：Leader 指定的「三个月后接手的人」与「运维 / SRE」，各由一个独立只读 lens 承担；
  Skeptic 视角由我本体在第一轮承担
- **本轮每一条 CRITICAL 都由我本体独立复核过**，下表给出我自己跑的求证，不引用 lens 的自陈

## 结论：**REJECT**（两轮合计 5 条 CRITICAL，两个 lens 与我本体无分歧）

第一轮找的是「代码写得对不对」，第二轮找的是「这套东西放到真实环境里能不能活」。
答案是：**不能**。自动投影路径在 launchd 下结构性不可用；跨年建表会静默污染新表；
凭据已经被填进了一个没有任何代码读的文件。这三条都不是代码写错，是**没有人从使用者的位置
走过一遍全链路**——而这恰是 TASK-011（docs-only、看似最轻的任务）已经开始暴露、但只走完了
一半的那条链路。

---

## 一、CRITICAL-2：模板表与数据表是同一张，跨年建表会把 2024 年的数字带进新表

**位置**：`internal/hestia/sheets/push.go:30`、`:34`；`internal/hestia/sheets/client.go:178-232`

```go
func tabName(year int) string { return fmt.Sprintf("%d年", year) }   // push.go:30
const templateYearTab = "2024年"                                      // push.go:34
```

### 我的独立求证

| # | 事实 | 求证方式 |
|---|---|---|
| 1 | `templateYearTab` 与 `tabName(2024)` 是**同一个字符串** `"2024年"` | 读 `push.go:30` 与 `:34` |
| 2 | 库里 2024 年有 **10 期**观测 | `sqlite3 data/hestia.db "SELECT COUNT(*) … WHERE period LIKE '2024-%'"` ⇒ 10 |
| 3 | testdata 快照同样有 10 期 2024 | python3 解析 `internal/hestia/testdata/period-keys-2026-09-16.json` ⇒ 10 / 77 |
| 4 | `CreateYearTab` 的四步里**没有清空录入区**的请求 | 读 `client.go:208-232`，四步为 Duplicate / 改名 / 改 A1 标题 / 排位；`grep -c 'StartRowIndex: 3\|RepeatCell\|EndRowIndex: 15'` ⇒ **0** |
| 5 | ingest 自动投影固定 `Apply:true, CreateSheets:true` | `cmd/atlas/hestia_sheets.go:170` |

### 失效链

1. 首次 `--apply` 把库里 2024 年的 10 期写进 `2024年` 表——**它同时是模板**。此刻模板不再是空表。
2. 下一次出现「库里有某年数据、表里没有该年度表」（跨年第一期必然如此，ingest 会自动触发），
   `CreateYearTab` 复制**已填满的** `2024年`。
3. 新表的录入区 4–15 行带着 2024 年的数字。`Push` 随后只对库里存在的月份产出 `Change`
   （新年份通常只有 1 月），**2–12 月那 11 行不会被 `Diff` 触碰**，就地留在新年度表里。
4. 库里缺的格走 `AbsentInDB`「保留表中现值」（`diff.go:70`），同样留下 2024 年的值。
5. AJ–BB 的 19 个公式照常对这些假数据求值。

⇒ 明年 1 月第一次 ingest 就会发生，**没有任何告警**，而表面上看每张年度表都有完整数据。
这是 `client.go:172-173` 自己那句话的实现版本：「最难察觉的那种错」。

`CONTRACTS.md:4024` 声称「`2024年` 录入区全空」——那是 2026-09-16 spike 当天的观测，
**判据五那次 `--apply --create-sheets` 之后即失效**（建表在 `push.go` 第 6 步、写格在第 7 步，
所以只有首次运行是安全的）。这条「事实」写下时为真，会在第一次使用后变假而无人重采。

### 修复建议（择一）

- **①（推荐）** 在 Sheet 里另建一张永久空白的 `模板` 工作表，`templateYearTab` 指向它，
  并加一条断言「`templateYearTab` 不等于任何 `tabName(year)`」，让这条约束长在测试上。
- **②** 在 `CreateYearTab` 的同一批 `batchUpdate` 里追加一个 `UpdateCells` 把 `A4:AI15` 清空。
  成本更低、原子性不变，但没有解决「模板是活表」这个根因。

---

## 二、CRITICAL-3：launchd 环境下自动投影大概率恒失败，而现有测试会拦住修复

**位置**：`deploy/launchd/com.newthinker.atlas.hestia-ingest.plist`；`cmd/atlas/hestia_test.go:477-490`

### 我的独立求证

| # | 事实 | 求证方式 |
|---|---|---|
| 1 | sheets 包走**默认 transport**（`ProxyFromEnvironment`） | `client.go:44-48` 自述，C7 要求 |
| 2 | hestia-ingest plist 的 `EnvironmentVariables` **只有 PATH** | 直读该 plist 的 dict 段 |
| 3 | `TestHestiaPlistSetsNoProxyKeys` 按「键名含 proxy」**全面禁止**，且带阳性对照 | `hestia_test.go:486-490` |
| 4 | 同仓 crisis-daily plist **设了** `http_proxy` / `https_proxy`，注释写「从本网络直连恒 403，需经本地代理」 | 直读该 plist `:24-36` |
| 5 | Atlas 全局设了 `http_proxy=127.0.0.1:7897` | `internal/hestia/fetch.go:26` 自述 |

### 那条守卫的理由已经过期，而它的反证就写在被它引用的那个文件里

守卫的失败文案是：「hestia 直连央行（`NewPBOCFetcher` 用空 Transport 绕开代理）」。
而 `fetch.go:33-34` 自己写着：

> 这一层必须在 client 里做，**不能只靠 plist 的 no_proxy** —— 那是进程级的，
> 会把 Telegram 和 **Sheets** 一起放行

⇒ PBOC 的直连由 client 层的空 `Transport{}` 保证，**与 plist 设不设代理键无关**
（空 Transport 的 `Proxy` 是零值，不读环境变量）。所以 plist 加代理键
**不会**破坏这条守卫想保护的东西，只会打红那条测试本身。

M2b 之前 hestia 只有一条出网路径（直连央行），「plist 不设代理」与「client 层直连」是同向的，
守卫写得没问题。M2b 之后 hestia 有了**第二条方向相反的出网路径**，C7 明确要求「同进程内三种策略」，
而那条守卫仍按 M2b 之前的单一假设在执行。

⇒ launchd 唤起的 ingest 没有代理环境变量 ⇒ sheets 的默认 transport 无代理可用 ⇒ 连不上
`sheets.googleapis.com`。而按 C8，失败只在 stdout 留一行、退出码 0、outcome 不变、不发通知。
**运维在自己终端（profile 里有代理）跑 `sheets push` 一切正常，生产一直在坏。**

### 修复建议

把守卫从「禁止任何 proxy 键」收窄成它真正要断言的那件事：
`NewPBOCFetcher` 返回的 client 不读环境代理（对 `Transport.Proxy == nil` 断言，
或用一个设了 `http_proxy` 的环境跑一次直连）。然后给 hestia-ingest plist 补
`http_proxy` / `https_proxy` / `no_proxy`，与 crisis-daily 对齐。

**这条必须在首次上线前解决**，否则自动投影这条路径交付即失效。

---

## 三、CRITICAL-4：凭据已经被填进了没有任何代码读的文件——未决项的风险已经兑现

**位置**：`configs/config.yaml:332-334`（gitignored，未进仓库）

我直读该文件确认：`hestia_sheets:` 段已填入**真实的** `credentials_file` 路径与
`spreadsheet_id`（具体值不在此复述）。而：

- `openHestia`（`cmd/atlas/hestia.go`）走 `hestia.LoadConfig(configs/hestia.yaml)`；
- `configs/hestia.yaml` 里 `hestia_sheets` 命中 **0**；
- 主配置 `internal/config/config.go` 没有 `HestiaSheets` 字段，且 `config.Load` 对未知顶层键不报错。

⇒ `CredentialsFile == ""` ⇒ 能力**当前处于静默禁用**，而配置文件看起来已经配好了。

**这不再是一个假设风险。** TASK-011 的 dev 自加了「⚠️ 生效位置不是本文件」的警告
（`config.example.yaml:272-278`），CONTRACTS 的未决项也写了三条出路——**警告没能拦住**，
填错这件事已经发生了。这正面印证了未决项里那句「填错位置没有任何反馈，这正是它危险的地方」。

### 建议

三条出路的选择仍归人拍板，但**有一条与出路选择无关、可立即做的修复**：
让 `hestia.LoadConfig` 在 `credentials_file` 非空时校验 `spreadsheet_id` 也非空，
并（更重要的）让配置装载在检测到主配置 `config.yaml` 有 `hestia_sheets` 顶层键时**报错并指明正确位置**。
把「零反馈」变成「装载期就红」，这是这个缺陷的全部危害所在。

附带：`internal/hestia/config.go` 的 `LoadConfig` 对 `HestiaSheets` **一条校验都没有**
（`cfg.validate()` 里 `Sheets` 命中 0），`sheets.NewClient` 也只 `os.Stat` 凭据文件、
不检查 `spreadsheetID` 非空 ⇒ 空 ID 会走到 `GET /v4/spreadsheets//values/...` 得 404，
而在 ingest 路径这只是 stdout 一行。

---

## 四、CRITICAL-5：无重试、无退避、无超时，而调用量随期次线性放大

这是第一轮 WARNING-2 在运维视角下的升级。我的独立求证：

| # | 事实 | 求证方式 |
|---|---|---|
| 1 | sheets 包非测试文件里 `retry\|backoff\|429\|ratelimit\|Sleep` 命中 **0** | `grep -rniE` 求证 |
| 2 | 生成的客户端用 `gensupport.SendRequest`，**不是** `SendRequestWithRetry` | 读 `sheets-gen.go` |
| 3 | sheets 包与 CLI 侧 `WithTimeout\|http.Client{` 命中 **0** | `grep` 求证 |
| 4 | `ingestOne` 每期调一次投影，每次读**全库**、新建 client、推全量 | `ingest.go:482-490` 在 `for _, c := range cands` 循环内 |

稳态每期约 `1 + 2T` 次 read（T = 年度表数）加 1 次 write；首跑带建表更高（每张缺表额外
2 read + 1 write，建完还要补 diff 的 2 read）。Sheets API 的读配额是分钟级的，
**一分钟内投影 4 期就会撞线**；`--force` 翻满 `max_pages` 时必撞 429。
429 不重试、不退避，直接变成 stdout 一行，**那几期的投影永久丢失**——没有补投队列。

无超时这条同样要紧：`cmd.Context()` 是 `context.Background()`（无 deadline），
`sheets.NewService` 也没设 `http.Client.Timeout`。境外端点被黑洞时 ingest 进程可能无限期挂起，
launchd 同 label 不并发 ⇒ 后续唤起全部不执行，最终由 `hestia_stalled`（>30h）兜住，
但告警文案会把人指向「launchd 没跑」这个错误方向。

### 建议

- ingest 路径把 rows 收敛到本期所在年份（`filterRowsByPeriod` 已有，只用在 CLI 侧），
  或把投影移出 `ingestOne`、在候选循环之后做一次。
- `sheetsProjector` 复用一个 client，别每期重读凭据加一次 JWT 交换。
- 对 429 / 5xx 加指数退避；给投影加独立 `context.WithTimeout`（30–60s 量级）。

---

## 五、WARNING（第二轮新增）

1. **投影失败没有任何告警通路，连续失败 N 天无人会知道。** 逐条求证：outcome 不变、
   退出码 0、不发通知（C8）；`hestia_stalled` / `hestia_no_ingest` 两条告警规则只看库、与投影无关；
   `hestia_sheets_write_total` 指标 CONTRACTS:3409 明写本 sprint 不注册；
   `internal/hestia/health.go`、`status.go` 里 `sheet` 命中 0；`docs/ops/` 里 `sheets` 命中 0。
   ⇒ 唯一发现路径是人主动跑 dry-run 巡检，而这条只写在一份**待人粘贴进 vault 的 md** 里，
   没进 runbook、没进任何周期任务。
   建议：给 `atlas hestia health` 加一格「最近一次投影结果」，或把 dry-run 巡检登记成周期任务。

2. **投影失败打 stdout，而同文件的 `max_pages` 警告打 stderr。** 求证：`ingest.go:215` 是
   `fmt.Fprintf(os.Stderr, ...)`，而 `:485`/`:487` 是 `fmt.Fprintf(d.Out, ...)` ⇒ 落到
   `StandardOutPath` 那个噪音更大的日志。同一个程序里两类「不改退出码的警告」分流到两个文件。
   建议：投影失败改走 `os.Stderr`，与 `max_pages` 同待遇（先例已在同文件里）。

3. **改年度表的任意一个表头字，整个投影会永久停止且无人知晓。** `ResolveHeader`
   （`header.go:39-45`）缺标签**整批拒绝**，`diffTab` 的错误经 `push.go:161-167` 的
   `return res, err` 让整个 `Push` 返回 ⇒ **一格都不写**。在 ingest 路径这个 error 走 C8，
   只留 stdout 一行。而 `SheetColumns` 里是硬编码中文标签（含「·」前缀、「汇率 USD/CNY」里的空格），
   在表格里手改一个字就能触发。这类错误不是瞬时故障、不会自愈。
   建议：ingest 侧对「表头解析失败」单独处理（提升为告警），或给 `ResolveHeader` 加别名表。

4. **录入区首行这个事实有四个副本，而注释只点名了两个。**
   `cmd/atlas/hestia_sheets.go:50` 写「与 sheets 包的 `entryRowOffset` 是**同一事实的两个副本**」，
   实际承载同一事实的有四处：`diff.go:30`（`entryRowOffset = 3`，决定写哪一格）、
   `client.go:19`（`entryFirstRow = 4`，决定读哪些行）、`row.go:19`（注释「行 = Month + 3」）、
   `hestia_sheets.go:53`（`sheetsEntryFirstRow = 4`，排版）。
   照注释只改两处 ⇒ 读回的录入区与写入行号错位一行 ⇒ `Diff` 把越界当「表中无值」⇒
   **每格都判 WillWrite、每次 apply 重写全表，而表面上在正常工作**。
   建议：把 `entryFirstRow` 提成 `sheets` 包的导出常量、其余三处引用它；或至少把注释改成列全四处。

5. **`--apply` 无备份、无审计、不可回滚。** 基线一次写 1282 格；`Change.Current` 持有旧值
   但只在内存里，CLI 靠 `formatResult` 打到终端，**ingest 自动路径连打都不打**
   （`hestia_sheets.go:170` 是 `_, perr := sheetsPush(...)`，`Result` 被丢弃）。
   代码里无删表能力（`sheets/*.go` 里 `Delete` 命中 0），建错的表只能人去 UI 里删。
   建议：`--apply` 前把 `Result.Changes`（含 `Current`）落一份 JSON 到 `logs/`，ingest 路径同样落盘。
   这是唯一能事后回答「那格原来是什么」的东西。成功时也打一行「写入 N 格」。

6. **spreadsheet_id 配错时，CLI dry-run 安全但 ingest 自动路径不安全。**
   指向别人的表且对方没有 `20xx年` 工作表时，CLI 路径（`CreateSheets:false`）报「缺年度表」，
   而 ingest 路径（固定 `CreateSheets:true`）会**直接在别人的文件里建 5 张表**。
   没有任何一处校验「这张表是不是我的表」。
   建议：首次投影前校验一个指纹（模板表存在且 A1 标题匹配 `\d{4}\s*年`），不匹配就拒绝。

---

## 六、SUGGESTION（第二轮新增）

1. **`ingest.go:63` 的注释引用了一个不存在的符号。** 原文「同 `sheets.createTabs` 的手法」，
   全仓 `createTabs` 命中 0（sheets 包里是 `createYearTabs`，且它是普通函数、**不是**测试缝 var，
   `var createYearTabs` 命中 0 处）。接手者会去找一个不存在的先例。
   建议：删掉括号里那半句，或改指 `cmd/atlas/hestia_sheets.go:36-40` 的两个真测试缝。

2. **spec 会随 sprint 归档而消失，而代码注释里到处引用它。** 仓库代码注释里有大量
   `spec §N` 引用（M2b 用到 §3.1 C2、§4、§5.5、§7.2、§8.3、§10），而 spec 与
   `architecture-decisions.md` 都只在 `.arcforge/docs/01-design/` 下；`.arcforge/archive/` 里
   已有多份归档，说明这个目录会被整体搬走。CONTRACTS 通篇用「见 spec §X」当解释落点。
   建议：把 spec 的约束表、判据表、35 列映射复制进 `docs/hestia-m2b/`，
   或在 CONTRACTS 顶部写明归档后的路径。

3. **`docs/hestia-m2b/TASK-011-vault-content.md` 的状态不可判。** 它是一份「供人粘贴进 vault」
   的一次性稿件，末尾五条「粘贴后自查」没有任何一条被勾选 ⇒ 三个月后读它的人无法知道
   vault 里到底是不是这个内容。建议粘贴完成后在顶部加一行「已于 YYYY-MM-DD 粘贴」。

4. **`configs/hestia.yaml` 不在 deploy.sh 的 rsync 排除表里。** 这是未决项的一部分，
   但有一条独立于出路选择的后果值得记：运维即使把开关关掉（把 `credentials_file` 改空），
   **下一次部署会用源树覆盖它，把关掉的开关悄悄打开**。
   关停方式本身也没写进任何 runbook（`docs/ops/` 里 `sheets` 命中 0）。

5. **CONTRACTS ⑤ 的 `push.go:169-172`**：实际 `if !opts.Apply` 在 171，169-170 是注释行。
   范围包含了目标行，**不构成误导**，可不改。（与 Leader 清单第 7 条的 `sheets_project_test.go:113-121`
   性质相同，那条的范围也包含目标函数，我在第一轮判为「略偏、可顺手改」。）

---

## 七、两轮合并：给 Leader 的处置建议

### 最终 verdict：**REJECT**

两个 lens 与我本体在全部 5 条 CRITICAL 上**无分歧**（不是 CONTESTED）。
全部 5 条都由我本体独立复核，求证方式逐条写在上文表格里。

### 建议以 `review_fix` 派回，`reason_class=task_defect`，分三批

**第一批（必须修，阻断首次上线）**

| # | 修什么 | 落在哪个文件 | 出自 |
|---|---|---|---|
| 1 | `read()` / `readTitle()` 加 `.ValueRenderOption("UNFORMATTED_VALUE")`，补「替身返回字符串形态数值 ⇒ 判 Same」的测试 | `internal/hestia/sheets/client.go` | 第一轮 CRITICAL-1 |
| 2 | 模板表与数据表解耦（另建空白模板表，或建表时清空 A4:AI15），加「`templateYearTab` 不等于任何 `tabName(year)`」的断言 | `internal/hestia/sheets/push.go`、`client.go` | CRITICAL-2 |
| 3 | plist 守卫收窄为「PBOC fetcher 不读环境代理」，plist 补三个代理键 | `cmd/atlas/hestia_test.go:477`、`deploy/launchd/…hestia-ingest.plist` | CRITICAL-3 |
| 4 | `LoadConfig` 校验「`credentials_file` 非空 ⇒ `spreadsheet_id` 非空」，并对主配置里的 `hestia_sheets` 顶层键报错指路 | `internal/hestia/config.go` | CRITICAL-4 |
| 5 | 投影加超时；429/5xx 加退避；client 复用；ingest 路径收敛到本期年份 | `sheets/client.go`、`cmd/atlas/hestia_sheets.go`、`internal/hestia/ingest.go` | CRITICAL-5 |

**第二批（同批顺手做，成本低）**

6. C8 recover 上移覆盖 `buildSheetRows`（第一轮 WARNING-1）。
7. 移植三条夹具断言 N7 / N10 / P16（清单在第一轮报告第五节①）。
8. 投影失败改走 `os.Stderr`；成功时打一行「写入 N 格」；`Result.Changes` 落盘供事后追溯。
9. 改陈旧注释 `push_test.go:23`、glob 侧车 `ingest_test.go:1756`、
   不存在的符号引用 `ingest.go:63`、四副本注释 `hestia_sheets.go:50`。

**第三批（进 final-report 的已知项，不必本轮修）**

10. 告警通路缺失、表头改字致投影永久停止、无备份回滚、spreadsheet_id 指错表、
    spec 归档后不可达、依赖增量偏重、dry-run 缺表场景低估计数。

### 给人拍板的两件事（不属于 dev 可决）

- 凭据落位三条出路（`CONTRACTS.md:4194`）——**且请注意风险已兑现**：
  `configs/config.yaml:332` 已经填进了真实值而无任何代码读它。
- 是否接受「自动投影暂不可用、只用手动 `sheets push`」作为过渡态。
  若接受，CRITICAL-3 可降级；若不接受，它是上线前置。

### 一句话给复盘

本 sprint 的验证质量确实很高——11 份报告、53 个变异、验证者多次订正 Leader 的事实错误，
**在「代码写得对不对」这个维度上几乎无懈可击**。第二轮找到的 4 条 CRITICAL 全部落在
另一个维度：**没有人从使用者的位置走过一遍全链路**。TASK-011 是唯一一次尝试（它因此抓出了
配置落位问题），但它是 docs-only、`writes` 够不到代码，只能立未决项。
⇒ 这正面印证了 Leader 在 plan.md:224 记下的那条 PENDING 候选，并且把它加强一档：
**「以使用者视角重走全链路」不该由一个 docs-only 任务承担，它需要能改代码的 writes。**
