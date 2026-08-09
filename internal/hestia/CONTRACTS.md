# `internal/hestia` 的契约与机制清单

**这份表回答一个问题：这个包有哪些地方靠自觉，而不是靠机制。**

来源：Sprint 033 QA 终审（`qa-agent-10`，三视角对抗轮），全部条目附实测依据。
之所以随代码入库而非留在评审报告里——报告会随 Sprint 归档，而这些契约不会随之失效。

## 结论

| 类别 | 条数 | 含义 |
|---|---|---|
| **(a) 有机制** | 12 | 有测试/检查守住，违反会红 |
| **(b) 有契约无机制** | 9 | 注释里写了，没有任何测试能验证调用方是否遵守 |
| **(c) 连契约都没有** | 7 | 既无检查也无声明 |

⇒ **16 处靠自觉，其中 8 处违反后完全静默**（下表加粗的 #1 #2 #3 #7 #10 #14 #19 #21）。

「完全静默」= 违反后不报错、不崩溃、数据看起来正常，只是**错的**。

## 清单

| # | 条目 | 类 | 依据 | 违反后果 |
|---|---|---|---|---|
| **1** | `DB()` 写入绕过 | b | 契约仅 `store.go:186`；两条守卫看不见句柄之后的行为 | **全静默**，五道防线一次全绕过 |
| **2** | 写口守卫对导出 `var`/`const`/`type` 失明 | c | 守卫只看 `*Store` 方法集与 `*ast.FuncDecl` | **全静默** |
| **3** | G9 并发契约（同键 Save 串行化） | b | `fields.go:18-31`；`-race` 明确看不见 SQL 层 TOCTOU | **全静默**，且**登记的后果本身写错了**（见下） |
| 4 | G9 契约文本自身无存在性守卫 | c | `TestPackageDocDeclaresUnits` 只守单位四词 | 整段可删，零测试转红 |
| 5 | `Check.Reason`「Skipped 必填」· `Passed=true` | a | `store.go:357-361` + `TestSaveRejectsSkippedWithoutReason` | 已闭合 |
| 6 | 同上 · `Passed=false` | b | `store.go:343-345` 明文声明刻意不查 | 仅进 pending JSON，消费者是人；论证成立 |
| **7** | `Passed=true` 空 `Checks`（22 字符旁路） | b | `store.go:330-332` 自己点名且不关 | **全静默**进权威表 |
| 8 | `Check.Value` 非有限值 | c | `checkValues` 从不看 `rep.Checks[].Value` | 不静默，但该期**两张表都没有** |
| 9 | 单位约定 · 注释存在性 | a | `TestPackageDocDeclaresUnits` | 已闭合 |
| **10** | 单位约定 · 数值实际符合单位 | c | 无任何量级/区间检查 | **全静默**：亿元当万亿元写 = 10000× 误差，REAL 亲和性正确故无类型信号 |
| 11 | `Meta` 三处同序 | a | reflect ×2 + 落盘验证 ×1 | 已闭合 |
| 12 | pending 八列（第四处同序） | a | `TestSavePendingColumnsMatchTheirValues` | 已闭合 |
| 13 | 字段名字面量只准在 `fields.go` | a | `TestFieldNamesAppearOnlyInFieldsGo` | 捕获上限测试自己写明（变量拼接/`Sprintf`/`[]byte` 往返抓不到） |
| **14** | `ingested_at` 不得 `ORDER BY`/`MAX()` | b | 契约见 H5；测试钉的是**理由**不是禁令 | **全静默**；且包内已有一处违反 |
| 15 | `ingested_at` 用 RFC3339Nano | a | 单点支点 | 已闭合 |
| 16 | pending 表列漂移 | a | `TestSavePendingFailsLoudlyOnDriftedPendingTable` | 确定性响亮失败 |
| 17 | `Values`「键不存在即缺失」 | a | NaN/Inf 闸门 + 白名单 + 落 NULL 测试 | 已闭合。空 `Values` 合法为刻意 |
| 18 | 观测表列**名**漂移 | a | `verifyObservationsSchema` | 已闭合（只覆盖业务列半边） |
| **19** | 观测表列**类型**/主键漂移 | c | 无任何检查 | **全静默**，`m2` 建成 TEXT 时 `MAX{356.71, 9.5} = 9.5` |
| 20 | `period`/`published_at` 形态 | a | 两个正则 + 五条测试（含尾随换行用例） | 已闭合；日历有效性与组合合法性另有登记 |
| **21** | `caliber_version` / `extractor` 取值 | b→c | `types.go:31-32` 注释**枚举了合法值**，却只有非空检查 | **全静默** |
| 22 | `caliber_version` 跨口径同比无效 | c | `fields.go:71-72` 声明，全包无消费者 | 属下游，**未登记任何落点** |
| 23 | `bitemporal` spec 与视图同源 | a | 单一 spec 同喂两侧 + `verifyCurrentView` | 已闭合 |
| 24 | 全等比对依赖驱动**逐字保存**视图文本 | c | `store.go:109-110`「已实测」 | 驱动升级规范化空白 ⇒ 既有库全打不开 |
| 25 | 观测表 PK 与 spec 同源 | a（非派生） | PK 是 `schema.go:55` 手写字面量，第四份拷贝 | 当前有测试兜住 |
| 26 | `Store.now` 注入点 | a | `now` 未导出 + Save 无条件覆写 + 三条测试 | 已闭合 |
| 27 | `insert` 失败不落 pending | b | 「一个入口、两个目的地」只对**过闸失败**成立 | 已就 OutOfOrder 登记 |
| 28 | 错误路径一律返回 `Outcome{}`（`Verdict=New`） | b | 观察级已登记 | — |

## 待 M1b-2 处置（Sprint 033 QA 判定 REJECT，2 CRITICAL + 7 WARNING）

Sprint 033 的 7 个任务在 QA 本体裁决落盘**之前**就已 accepted 并归档——**质量门禁顺序被绕过了一次**，这些发现因此没有走返工流程。全部留给 M1b-2。

### CRITICAL

**C-1 `rep.Checks[].Value` 的非有限值让该期数据两张表同时消失**（#8）

`checkValues` 只看 `Observation.Values`，而 `savePending` 第一件事是 `json.Marshal(rep)`。`store.go:294-299` 花四行论证「NaN 必须在入口拦」，**理由逐字就是这个后果——隔壁一个字段没覆盖**。

且必然触发：那段注释自己点名来源是「比率型 0/0」，而 `Check.Value` 恰是比率闸门的实测值。

修法 ~6 行：`checkReportValues(rep)`。

**C-2 `caliber_version`/`extractor` 枚举了合法值却只查非空**（#21）

`types.go:31-32` 用与 `period_type` **完全相同的文体**列出合法值，实测 `"garbage"` / `"2099-99"` 均 `err=nil` 落盘。

而 `fields.go:71-72` 写着 M1 口径断裂「只能靠 `caliber_version` 标注」——**它是那个断裂点的唯一防线**。

与 Sprint 033 的 C2 返工（G10 白名单化）是同一条推理，只是没走到 `Meta` 上。

### WARNING

1. 未知键 + `Passed=false` ⇒ 两表皆空
2. 漂移检查只比列名，类型漂移静默放行（#19）
3. 写口守卫对导出 `var` 失明，M10 的洞换个语法重开（#2）—— 修法 ~15 行，**只改测试**（补扫 `*ast.GenDecl`）
4. `INSERT OR IGNORE` 变异存活（insert 路径主键碰撞行为无测试）
5. `Passed=true` 空 `Checks` 直进权威表（#7）—— 修法 3 行
6. **G9 契约登记的后果与实测不符**（#3）：24 goroutine 全 `err=nil`、零 UNIQUE 冲突，**实际是静默退化成重复写入**，而契约写的是「一方拿 UNIQUE 错误」
7. `verifyCurrentView` 对纯书写差异假阳性，且错误文案是误诊（#24）

### 零成本时间窗

**`DB()` 收窄为只读接口**（#1）——实测**包外零调用方**，且与 M1b-4 的 `article_id` 幂等查询不冲突（只读足够）。一旦有了外部调用方，这个窗口就关了。

## 相关文档

- **`.arcforge/docs/05-review/qa-review-sprint033.md`** —— QA 终审报告（403 行，含每条的实测取证）

  ⚠️ **它不在 sprint-033 的归档目录里**——落盘时间（`Aug 9 20:04`）晚于归档提交 `6fd9107`，
  所以留在了运行时目录，**会随下一个 Sprint（M1b-2）一起归档到那个 Sprint 的目录下**。
  按归档目录找 sprint-033 的评审报告会找不到；这正是「本 Sprint 在无 code review 报告的
  情况下关闭」这一流程错误的物理痕迹。

- `.arcforge/archive/sprint-033-2026-08-09/docs/01-design/handoff-*.md` —— H1–H11 交接缺口
  （`handoff-to-later-iterations.md` / `handoff-additions-h8-h10.md` / `handoff-h11-cross-package.md`）

  其中 **H8 需在有生产数据前定案**（同期两篇报告的数据模型：54 个字段横跨社融与金融统计
  两篇报告，同期同日发布但 `article_id` 不同 ⇒ 第二篇判 Duplicate、字段静默丢弃，
  且与 M1b-4 的幂等检查构成永不收敛的重抓循环）。
