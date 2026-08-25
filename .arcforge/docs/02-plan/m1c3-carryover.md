# M1c-3 结转清单（由 Sprint M1c-2 / sprint-039 移交）

⚠️ **本文件是运行时 `docs/` 下的活文件**，因为归档后 `.arcforge/archive/sprint-039-2026-08-25/`
里的产物**写通道够不到**（`doc` 只接受 `docs/` 下相对路径）。
**完整背景在归档里**，本文件只做索引 + 归档后新增的部分。

## 归档指针

| 内容 | 路径 |
|---|---|
| 交付报告（**唯一人类可读交付物**，556 行） | `.arcforge/archive/sprint-039-2026-08-25/docs/06-acceptance/calibrate-report.md` |
| 最终报告（427 行，含全部 11 条 Review 发现与结转清单） | 同上 `/final-report.md` |
| Changelog | 同上 `/changelog.md` |
| 两轮 Review | 同上 `/docs/05-review/round{1-review,2-adversarial}.md` |
| 五份验证报告 | 同上 `/docs/04-test/` |
| 过程记录（1112 行） | 同上 `/docs/03-progress/plan.md` |
| 方法学沉淀（1274 行，**留在原地不随归档**） | `.arcforge/wisdom/_digest.md` |

归档提交 `7bccab448ecc477dce0b83e16eff98fd3fb430d3`。

---

# 🔴 两条前置（做标定之前必须先做）

**共同点：缺口与 M1c-3 的输入完全重合。**

## 前置一（R2-01, HIGH）分档的载荷前提在本语料上为假

`thresholds.go:39-44` 写「四种年度口径**相邻两期一律相隔 12 个月**」——**无缺口限定**。
而 `store.go:350` 的 `Preceding` 是 `WHERE period < ? AND period_type = ? ORDER BY period DESC`
⇒ **序列有洞时跨洞取**。

实测本语料：`q1` 有 **2023-03 → 2026-03 = 36 个月**；叠加 3 篇 Parse 失败后 `q1_q3` 另有 **24 个月**。
按真跑实测的折年 **7.6920%**：24 月 ⇒ **15.98%**、36 月 ⇒ **24.90%**，
而 **`0.15` 只容得下 22.63 个月** ⇒ **缺口处误拦**。

🔴 **反方向更要紧**：环比表**不带间隔** ⇒ 照 `max + 余量` 定阈值会得到 **≥24.90%** 的上限，
届时一个真实的 12 个月 **20%** 跳变（正常增长的 2.6 倍，**正是这道闸要抓的东西**）**会被判 PASS**。

**最小修法**：`writeStockRateSection` 给环比表加一列**间隔月数分布**（如 `12×5,24×1,36×1`），
**形态照抄已有的 `periodTypeMix`** —— 那个函数解决的是**一模一样的问题**，只是换了个维度。

## 前置二（R2-06, MEDIUM）完整性校验只覆盖 25/218 = 11.5%，而报告不报分母

`articleSHA256` 的生产调用点**恰好 2 个**（写入 `backfill_fetch.go:210` / 读取校验 `calibrate.go:170`，
**后者在 items 循环内**），`loadManifest` 不校验。
218 篇**全都记了 sha256**，但只有 25 篇进循环 ⇒ **193 篇（88.5%）从未被校验，其中 138 篇是社融报告**。
而 `shaUnverified` 只在「进了循环且**缺** sha 字段」时计数 ⇒ 本次恒 0 ⇒ **一条警告都不打**。

⚠️ **期次交叉校验（`calibrate.go:184-191`）在同一个循环里，覆盖率完全一样** ⇒ **两道守卫，同一个 11.5%。**

**修法一行**：`writeCollectSummary` 打出 `完整性校验: 25/218 篇（193 篇本迭代不读，未校验）`
—— 与该函数已有的「四格计数每次都打（含 0）」是同一条理由。

---

# 归档后新增：code-simplifier 只读审查（Leader 复核）

**分母**：15 个改动文件全部看过；逐个过了 22 个新函数 / 5 个改动的既有函数 / 6 个新类型 / 7 个新常量 / 1 个新 cobra 命令。
**命中 6 条 + 5 条「看了、判定不该改」。**

## F1（中）`res.Warnings` 整段打印两遍 —— **R2-02 之外的第四条重复通道**

**Leader 复核属实**：`calibrate.go:358`（`writeCollectSummary` 尾部）与
`calibrate_report.go:148`（「存疑」段）各遍历一次 `res.Warnings`，
而 `Calibrate` 里两者写的是**同一个** `d.Out`。

实测一次运行里三条 warning **逐字重复**，某个 id 出现 **4 次**。
⚠️ 更糟的是措辞不同数字相同（「fetch 阶段未抓到: 1 篇」vs「fetch 阶段有 1 篇没抓到」）
⇒ **读者要停下来判断这是不是两件事**，而读者正是照它填 `MagnitudeRanges` 的人。

🔴 **附带一处「断言钉错层」**：`calibrate_report_test.go:290` 的
`TestRenderCalibrateReportRendersEveryDisposition` 断言「每一类恰好出现一次」——
**这条不变量在 render 层为真、在用户实际看到的 `Calibrate` 层为假**。

**建议改法**（simplifier 给的）：把 `writeCollectSummary` 定位成**失败路径诊断**，
让 `collectSamples` 在「可用样本为 0」时连 `res` 一起返回，把调用移到 `Calibrate` 的错误分支。

⚠️ **不是零成本**：会红 3 处测试（`calibrate_test.go:212` / `:267` / `:328`），
其中两处断 `assert.Nil(t, res)` **与改法直接冲突**（它们的理由是「报错时不产出结果」）。
⇒ **交换是「删掉约 30 行重复输出 vs 重写 3 处测试断言」**，由 M1c-3 判断值不值。

## F2（低）4 处列宽格式串两两成副本

`calibrate_report.go:157` vs `:201`；`:343` vs `:194`。每对**只差 `%4s` 与 `%4d`**。
⚠️ **改一处不改另一处 ⇒ 表头与数据行错位，而测试用 `strings.Fields` 解析、列宽变了不会红。**
⇒ 与同文件 `fieldCells` 被抽出来的理由（「同一事实的两个副本」）是同一条 —— **只抽了值，没抽格式**。

## F3（低）4 个类型导出了但包外无人用

`CalibrateResult` / `ParseFailure` / `SampleRecord` / `FieldStats` ——
包外唯一引用是 `hestia.Calibrate` + `hestia.CalibrateDeps`。
⚠️ 不导出它们，`SampleRecord` 那条「不要给它加导出方法」的注释脚就不存在了。
**simplifier 判定「改了不会红」（`store_test.go:399` 只收 `*ast.FuncDecl` 且跳过未导出接收者），
但它明说这是 grep + 读断言实现推出来的，没真改一遍验证。**

## F5（nit）死初始化

`calibrate.go:142` 的 `Samples: map[string][]float64{}` 在 `:202` 被无条件覆盖，中间无读取。
**Leader 复核属实。** 改成 `res := &CalibrateResult{}`，不会变红。

## F4（nit）/ F6（观察）

F4：calibrate 的两个变量插进了 `cmd/atlas/hestia.go:31-38` 别人的 var 块中间，
而上方 doc 注释通篇在讲「backfill fetch 的**四个** flag」。
F6：`calibrate.go:454` 说「放行说明排在报告**之前**」，实测落在第 11 行而前 10 行已是带数字的采集摘要
—— **若采纳 F1 则自动消失**，不必单列。

## 5 条「看了、判定不该改」（记下来，免得下次重查）

1. `writeFieldRow` / `writeFieldRowWithMix` —— 刻意分开，服务不同的表
2. `collectSamples` 的 items 循环 47 行但**平坦**，抽出来反而更难读
3. `defaultStockContinuityMax()` 硬编码 5 档 —— **已有绊线**（`thresholds_test.go:55` 断言与 `periodTypeList()` 相等）
4. `bfExec` vs `calExec` —— 形状接近但差别实质（`io.Discard` vs buffer；flag 重置逻辑不同）
5. CONTRACTS.md 的自证判据 `⇒ 8` —— 跑过，确实是 8

**没有找到「真正只是复制粘贴」的近似函数；没有找到死代码。**

---

# 其余结转（详见归档的 final-report §五）

| # | 项 |
|---|---|
| 1 | ④a `calibrate_report_test.go` 中间三列（p5/median/p95）无人守 ⇒ 补 `annual[3]`/`annual[5]` |
| 2 | ④b `:469` 断言文案单向（多加一列也红，文案只说「少一列」） |
| 3 | V6 验收报告**跳读者路径**无保护（表格首行 298，最近保护 272 ⇒ 26 行；Ctrl-F 到 325 ⇒ 53 行）⇒ 修法是**在数据表内部或紧邻处**加保护，不是再加一层开头 |
| 4 | V7 表头 CJK 列宽错位 ⇒ **消除成因**（表头改 ASCII / 按显示宽度补齐），**不要守阈值** |
| 5 | 54 个建议区间里 24 个带浮点长尾 ⇒ 加定点格式化 |
| 6 | R1-01…04 / R2-02…05 / R2-07 共 9 条 Review 发现 |
| 7 | 全仓 28 个存量未格式化文件（本 sprint 未新增也未清理） |
| 8 | `writeFieldRow`（环比一节）**本次真跑零执行** ⇒ 列对齐只被夹具测过 |
| 9 | `discoveries/TASK-002.json` 的 `n=25`（正确 22）与 `Failures=0`（正确 3）**在 `interfaces_exposed` 里**，而 discovery 冻结 ⇒ **指针只能放消费侧**：M1c-3 相关 task 的 `context_from` 说明里要加一句订正 |

## 🔴 给 M1c-3 验证者的三条（QA 提）

1. **预登记判据逐条写出可执行命令**（grep/awk/sed），不是意译描述
   —— 本 sprint 实测：两个人按字面重跑同一条判据都得到与报告不同的数，**而失效是静默的**。
2. **否定式判据必须同时报分母**（「窗口内候选 N 个，命中 0」）。
3. **每条判据附一句「什么情况下我会 FAIL」——写不出来的那条，就还不是判据。**
   ⚠️ 本 sprint 实证：M1–M7 里**只有 M7 带数值阈值，而 M7 是唯一抓到新缺陷的**。
   **能失败正是判据有信息量的前提。**
