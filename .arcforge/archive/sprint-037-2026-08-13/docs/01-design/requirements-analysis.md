# Sprint 037 需求分析 — Hestia M1b-4b：CLI 接线与部署 + 季报支持

**需求文档**：`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md`（1882 行，Leader **全文结构 + 关键节全读**，见末尾「阅读实况」）
**基线**：`63ac5b6`（master），`internal/hestia` 覆盖率 93.2%
**目标**：把 M1b-1…4a 的零件接成能跑的管线并交给 launchd；**外加人类定案的季报支持**

---

## 一、本 Sprint 的范围 = 计划的 7 个 task **+ 季报**（人类 2026-08-12 定案）

计划自带 T1–T7 拆分，依赖链：

```
T1 (storage 段) ─────────────┐
T2 (HasArticle) ─→ T3 (Ingest) ─→ T4 (幂等与错误路径) ─┐
T5 (status 查询与渲染) ───────────────────────────────┼→ T6 (cobra) → T7 (配置/plist/验收)
                                                      ┘
```

### 🔴 Leader 在 Step 2 核实出的两个缺口，均已由人类定案

#### 缺口 1：**计划完全没有覆盖季报支持**（人类上个 Sprint 已定案「4b 之前补上」）

**实测**（1882 行全文 grep）：

```
季度 / 一季度 / 前三季度 / quarterly            → 0 次
cumulativePeriods / validPeriodTypes          → 0 次
periodEndMonth / reportTitleRE                → 0 次
discover.go / types.go / profiles.go 出现次数  → 0 / 0 / 0
```

⇒ **人类定案：本 Sprint 并行加进来。** 理由：季报动的文件与 4b **零重叠**，scope 互斥下可同波并行；
且端到端跑通时能报 4/12 而不是 2/12。

**季报的五处改动（Leader 已逐处定位）**：

| # | 位置 | 现状 | 要改成 |
|---|---|---|---|
| 1 | `discover.go:100` `reportTitleRE` | `(\d{4})年(上半年\|\d{1,2}月)?金融统计数据报告` | 期次段加「一季度 / 前三季度」 |
| 2 | `discover.go:104` `parsePeriod` | switch 只有 `""`→annual、`上半年`→h1、default→monthly | 加季度分支 |
| 3 | `types.go:40` `validPeriodTypes` | `{monthly, h1, annual}` | 加季度类型 |
| 4 | `types.go:74` `periodEndMonth` | `{h1:06, annual:12}` | q1→`03`、q3→`09` |
| 5 | `profiles.go:70` `cumulativePeriods` | `{全年, 上半年}` | 加季报的累计前缀 |

⚠️ **第 5 处是唯一的硬卡点**（Sprint 036 消费者位实测）：只改前四处会让季报**被发现却在抽取阶段
响亮失败** —— `人民币存款期内合计 not found among 0 candidate sentence(s)`。

⚠️ **第 5 处需要真实样本才能定**：`cumulativePeriods` 的注释写明「外币孪生句的前缀同样是全年/上半年」
⇒ **期次与口径两个维度都要判**。季报正文里的实际前缀词（「一季度」/「第一季度」/「前三季度」？）
**必须从真实样本读出来，不能推测**。Sprint 036 消费者位抓过 `pboc_q1.html`，但那份在 scratchpad，
**随 session 清理，需重新抓**。

#### 缺口 2：计划把「央行重发 / 站点迁移」标成情形 B ✅，**而它不成立**

计划原文（1719 行）：

| 情形 | 写 pending 行？ | 一级键 | 计划的结论 |
|---|---|---|---|
| B · 央行重发 / 站点迁移 | 无关（**新 article_id**） | 不命中 | 抓 → 双时态修订 ✅ |

**Leader 核实**：`discover.go` 的判停是 `if has { return out, nil }` —— **一旦 `HasPeriod` 命中就立即返回**。
修订版是「同期次、更晚 `published_at`」⇒ 它排在 index 最前 ⇒ **第一条就命中 ⇒ 立即 return，
它本身进不了 `out`**。

⇒ **计划只检查了「一级键 `HasArticle` 不挡它」这一层，没检查「`Discover` 根本发现不了它」。**
这与 Sprint 036 消费者位的 P10 实测一致（返回 **0 条**）。

⇒ **人类定案：按上次定案改正计划的 ✅** —— 写成「一级键确实不挡，但 `Discover` 判停让它结构上不可达」，
写进 CONTRACTS，**本 Sprint 不动 `Discover` 的判停规则**。

---

## 二、计划自身的强论证（Leader 采纳，不改）

### 一级幂等键查两张表（含 pending）—— 这是 Sprint 036 留的必答题的定案

理由不是「省一次 HTTP」，而是**被挡住的那种重试本来就没有意义**：

| 情形 | 写 pending 行？ | 一级键 | 结果 |
|---|---|---|---|
| A · 抓取或解析失败 | **否**（没调 `Save`） | 不命中 | 下次唤起自然重试 ✅ |
| B · 央行重发 / 站点迁移 | 无关（新 article_id） | 不命中 | ⚠️ 见缺口 2 |
| C · 同一篇文章、没过闸 | 是 | 命中 | 挡住 ✅ |

**情形 A 不写 pending 行这一点，推翻了 Sprint 036 消费者位 W3 的量化**（「每轮一行 pending，
6h 一跑 ≈1460 行/年」）—— 那个量化的前提是「每轮都会走到 `Save`」，而抓取/解析失败**根本到不了 `Save`**。
⇒ **W3 的担忧在本设计下不成立**，`--force` 是情形 C 的逃生阀。

**计划还给了 4a `HasPeriod` 一个自洽的新解释**（Leader 认为这是本计划最好的一段）：

> 不是「为了重抓同一篇」，而是**让同一期出现新文章时还能发现它**（情形 B）。两层由此自洽。

⚠️ 但这个解释**依赖情形 B 成立**，而缺口 2 证明它在 `Discover` 层不成立
⇒ **CONTRACTS 里必须同时记下这个限定**，否则后人会以为两层已经自洽。

---

## 三、人类定案②的落实检查（月报跳过记日志）

Sprint 036 人类定案：**4b 跳过并记日志，不中止本轮**。

计划的 Global Constraints 写着「**单期失败不中断整批，最后汇总返回非零**」（26 行）⇒ **已覆盖**。
配套的「重试记账」按上面的分析**不需要**（情形 A 不写 pending 行）。

---

## 四、约束（照计划，Leader 未改）

- Go 1.24.4，**无新增依赖**（cobra/viper/testify 已有）
- **plist 一个代理键都不设**（约束 C6；crisis 的 plist 设了代理，**别照抄**）
- `db_path` 相对路径按 cwd 解析（C8）；`status` 打印**绝对路径**
- 每个 task 结束 `gofmt -l` / `go vet` / `go test ./...` 必须干净
- 注释引用任务编号**一律带 milestone 前缀**（`M1b-4b 的 TASK-003`）—— Sprint 036 契约

---

## 附：Leader 的阅读实况（Sprint 036 G41 的教训：指令不是载体）

**本 Sprint 起，Leader 自报阅读量，不再假设「我读了」。**

| 材料 | 实际读了多少 |
|---|---|
| 需求文档 1882 行 | **结构全览**（60 个标题）+ **1–102 行全文**（Goal/约束/文件结构/依赖/两条核实的事实/task 列表）+ **1707–1882 行全文**（Sprint 037 契约段 / 悬着的两件事 / Step 7-9 / 自审 / 完成后）。**T1–T7 各自的 Step 明细（103–1706 行）尚未逐行读** —— 拆任务时按需补读，届时如实登记 |
| Sprint 036 归档 | `findings-carryover.md` 的人类定案节、消费者位报告的 W1 原文 |
| 现存代码 | `discover.go` 的 `reportTitleRE`/`parsePeriod`/判停循环、`types.go` 的 `validPeriodTypes`/`periodEndMonth`、`profiles.go` 的 `cumulativePeriods` —— 均为核实上述两个缺口而读 |
