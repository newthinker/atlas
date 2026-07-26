# Prism M3 架构决策记录（ADR）

> 记录相对需求文档（`docs/superpowers/plans/2026-07-25-prism-m3.md`）的**偏离**与**待定点定案**。
> 每条含证据与影响任务。Dev 实现时以本文件为准，与 plan 冲突处以本文件优先。

---

## AD-1：YAML 解析用 viper + `mapstructure` tag，不引入 `gopkg.in/yaml.v3`

**背景**：plan §Tech Stack 写「`gopkg.in/yaml.v3`（若 go.mod 已有依赖则复用，否则以仓库现有 YAML 库为准
——crisis config 已解析 YAML，用同一个库）」，且接口草案中 `Template` 用了 `yaml:"..."` tag。

**事实**：仓库**不直接 import 任何 yaml 包**；`gopkg.in/yaml.v3` 与 `go.yaml.in/yaml/v3` 在 go.mod 中
均标记 `// indirect`。crisis config（`internal/crisis/config.go:115`）走 `viper.New()` + `SetConfigFile`
+ `Unmarshal`，结构体 tag 用 `mapstructure:"..."`。主 config 同款。

**决策**：`sankey.LoadTemplates` / `LoadManualSegments` 用 **viper**，结构体 tag 改为 `mapstructure`。
不把 indirect 依赖提升为 direct。

**理由**：plan 自己的判定规则（「用 crisis config 同一个库」）指向 viper；引入第二个 YAML 栈会让同一
仓库出现两种配置解析风格，且徒增依赖面。

**影响**：TASK-003。plan 接口草案里的 `yaml:"key"` 全部读作 `mapstructure:"key"`。
**注意**：viper 默认 key **小写不敏感**，`name_zh` / `xbrl_member` 等下划线 key 可正常映射；
`[]Segment` 列表 Unmarshal 需实测确认（DoD 已含「合法模板解析」断言覆盖）。

---

## AD-2：`FetchSegmentRevenue` 按**期间**聚合返回，一份报告可产出多条 `SegmentPeriod`

**背景**：plan 解析规则第 4 条写「每份报告一个 `SegmentPeriod`」。

**事实**（2026-07-25 live 实测 MSFT）：
- 一份 **10-Q** 的分部 context 覆盖 4 组期间：本季（90d）、**去年同季（90d）**、本年累计（274d）、
  去年累计（274d）。
- 一份 **10-K** 覆盖 **3 个财年** FY 期（365/366d），非仅当期。

plan 的 duration 过滤（70~100d 单季）会**同时命中本季与去年同季**，「一报告一 SegmentPeriod」的
数据结构无法承载。

**决策**：`FetchSegmentRevenue` 返回 `[]SegmentPeriod`，**每个通过 duration 过滤的 (PeriodStart, PeriodEnd)
一条**，`Form`/`FilingDate` 记录其来源报告。同一 (period, member) 跨报告重复出现时，**取 FilingDate 更晚者**
（重述以最新披露为准）。

**理由**：
1. 这是 live 事实，原结构不可实现。
2. 多出的历史期是**免费回填**——首次拉取一份 10-K 即得 3 个财年分部数据，一份 10-Q 附赠去年同季，
   显著加速冷启动，且 upsert 幂等，重复无害。
   > **键名更正（2026-07-25，test-agent-7 指出）**：本条初稿写的幂等键是
   > `(instrument_id, fiscal_period, segment_key)`，那是 **AD-9 之前**的表述。
   > 现行主键为 **`(instrument_id, period_end, segment_key)`**（TASK-002 已按此实现，`sqlite.go:69`）。
   > 结论不变且更强——换 `period_end` 后幂等性更可靠，正是 AD-9 的目的。
   > **TASK-008 注意**：增量与去重一律按 `period_end`，**不得按 `fiscal_period` 去重** ——
   > AD-9 的整个立论就是「fiscal_period 是不可靠展示标签，用它作键会静默覆盖数据」。
3. 对下游 Q4 推导（FY − 3 季）只有增益：FY 期更容易凑齐。

**被拒绝的替代方案**：只收 `endDate == reportDate` 的期间以严格维持「一报告一期」。
拒绝理由：白白丢弃已下载的历史数据，且 10-K 只剩当年 FY，Q4 推导覆盖面反而变窄。

**影响**：TASK-005（解析器 + DoD）、TASK-008（RefreshSegments 的 since 增量语义按期间比较）。

---

## AD-3：实例文档 URL 路径为 `/Archives/`（大写 A）

**事实**：plan 写 `{archives}/edgar/data/...`；实测可用 URL 为
`https://www.sec.gov/Archives/edgar/data/789019/000119312526191507/msft-20260331_htm.xml`（HTTP 200）。
CIK 用**整数形式**（789019，非补零），accession **去连字符**，primaryDocument `.htm` → `_htm.xml`。

**决策**：URL 模板固定为 `%s/Archives/edgar/data/%s/%s/%s`。测试注入 baseURL 时保留该路径前缀，
DoD 含「实例 URL 推导」路径断言。

**影响**：TASK-005。

---

## AD-4：新增 web 页面模板必须同步写入两个目录

**事实**：`internal/api/handler/web/templates/*.html`（`//go:embed`）与 `internal/api/templates/*.html`
（`cmd/atlas/serve.go:226` 设 `TemplatesDir: "internal/api/templates"`，磁盘模式实际读取）当前内容逐字相同；
既有回归测试 `TestNewHandlerDiskModeHasPrismTemplates`（prism_web_test.go:169）以 `NewHandler("../../templates")`
守护该不变量。plan 的 File Structure 只列了 embed 一处。

**决策**：TASK-012 / TASK-013 的 `packages` 与 `done_criteria` 显式包含**两个**模板路径；
另需同步 `handler.go` 中 **两处** pages 列表（`NewHandler:86` 与 `NewHandlerWithFS:124`）。

**理由**：漏同步 → `atlas serve` 磁盘模式启动即失败，且单测（embed 模式）不报错，是静默生产故障。

---

## AD-5：主干流不进模板（采纳 plan，记录理由）

主干流（revenue→cogs/gross→opex/opincome→tax/net）科目固定、全公司同构，由 periods 引擎从
`fundamental_q` 构建；模板只负责左侧分部定义与 XBRL member 映射。相对设计 §7 移除 flow 段，
理由写入模板文件头注释。**影响**：TASK-003（模板 schema）、TASK-007（引擎硬编码主干结构）。

---

## AD-6：任务粒度按「包互斥」组织，接线型任务允许跨包

**背景**：Realistic Scope 要求每任务 ≤1 package；但本计划存在大量「加一条路由 + 一个 Dependencies 字段」
式接线改动，为其单独开任务会让任务图碎片化、关键路径变长。

**决策**：
1. **核心 package ≤ 1**：每个任务只有一个承载主要逻辑与测试的包。
2. 接线型附带改动（`internal/api/server.go` 路由、`cmd/atlas/serve.go` 装配）允许并入消费方任务，
   但必须在 `packages` 字段**显式声明**，由 validator 的 scope 互斥校验保证不与并行任务撞车。
3. 同包任务一律通过 `dependencies` 串行（如 edgar 包 TASK-001 → TASK-005），不依赖 agent 自觉避让。

**影响**：全部任务的 `packages` 与 `dependencies` 编排。

---

## AD-7：手动目检项不进 done_criteria，进人工验收清单

plan Task 12 Step 2 的 7 项验收中，「浏览器目检」「PNG 导出可用」「中英切换可用」「双轴渲染正确」
无法由 agent 客观验证。

**决策**：可自动断言的部分（HTTP 200、DOM id 存在、数值口径可复算、回归全绿）进 done_criteria；
需人眼确认的部分写入 `06-acceptance/final-report.md` 的**人工验收清单**，明确标注「未由 agent 验证」。

**理由**：Reality Checker 原则——agent 不得对未验证的事项声称通过。

---

## AD-8：`prism_compare.html` 的 `data` 包裹层缺陷不在本期修复

`prism_compare.html:48` 直接使用 `r.json()` 结果访问 `d.dates`，未解 `response.JSON` 的 `{data, meta}`
包裹（`prism_detail.html:27` 写法正确）。属既有缺陷，与 M3 无因果关系。

**决策**：本期**不修**（Surgical Changes 原则），仅登记；新增页面 JS 必须用 `resp.data || {}` 正确写法。

---

# 第二轮：独立 reviewer 反审后的追加决策（AD-9 ~ AD-18）

> 来源：独立 reviewer（只读需求文档、未看 DoD）的反审报告。以下为 Leader 对其发现的处置。
> reviewer 独立复现了 AD-1（viper）、AD-2（一份报告多期）、AD-4（pages 双份目录）三项，构成交叉验证。

## AD-9：`segment_revenue` 主键改用 `period_end`，`fiscal_period` 降为展示列

**问题**（reviewer §5.5 / 断言 14）：plan 与设计 §6 用
`PK (instrument_id, fiscal_period, segment_key)`。但 `client.go` 自身注释已警告
「同一 (fy, fp) 可能对应不同实际季度」——`FiscalPeriod` 只是展示标签，不是可靠键。
既有 `fundamental_q` 正确地用 `period_end` 作 PK。用不可靠标签作 PK → 标签碰撞时**静默覆盖**数据。

**决策**：
```sql
CREATE TABLE IF NOT EXISTS segment_revenue (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  period_end    TEXT NOT NULL,      -- 真实主键（与 fundamental_q 同源）
  fiscal_period TEXT,               -- 展示标签，可为空
  segment_key   TEXT NOT NULL,
  revenue       REAL,
  source        TEXT,
  PRIMARY KEY (instrument_id, period_end, segment_key)
) WITHOUT ROWID;
```
`SegmentRow` 相应含 `PeriodEnd` 与 `FiscalPeriod` 两个字段。
`LatestSegmentPeriodEnd` 简化为 `SELECT MAX(period_end) FROM segment_revenue WHERE instrument_id=?`
（**不再 JOIN fundamental_q**，消除 reviewer 断言 15 的锚点回退风险）。

**连带影响**：TASK-008 的 ±3 天容差匹配仍需执行（为填 `fiscal_period` 展示标签），
但**匹配失败不再阻止落库**——落 `period_end` + 空 `fiscal_period`，并记入 Degraded。
数据不丢，可观测。TASK-007 的 `BuildPeriods` 按 `fiscal_period` 关联，空标签行跳过（可观测）。

## AD-10：`migrate` 必须容忍并发竞态

**问题**（reviewer 断言 8）：真实拓扑是 `atlas serve`（常驻）与 `prism-daily`（launchd 定时）
**并发 Open 同一 prism.db**。`migrate` 是 check-then-act：两进程可同时判定列不存在，
第二个 `ALTER TABLE` 报 `duplicate column name` → `Open` 失败 → serve 起不来或 daily 全挂（单点故障）。

**决策**：`ALTER TABLE` 失败时，若错误信息含 `duplicate column`，视为已被并发迁移，**继续不报错**；
其余错误照常上抛。DoD 增加并发 Open 断言。

## AD-11：XBRL 单遍解析 + 双重上限（体积 / 回看期数）

**问题**（reviewer 断言 25/26 / §5.3）：
1. plan 与 design-spec §1.2 写的「流式两遍」**在单个 HTTP 响应上不成立**——body 不可二次读取。
2. `since` 为零值时 `filings.recent` 的 10-Q/10-K 可达 40~60 份/家，× 实测 ~10 MB/份 × N 家
   = 数百个大文件，首跑必然超时或触发 SEC 限频。

**决策**：
1. **单遍解析**：一次 `Decoder.Token()` 遍历，同时收集 contexts 与 pending facts
   （facts 暂存 `(contextRef, tag, value)`），流结束后在内存中关联。不依赖 context 先于 fact 出现的顺序假设。
2. **响应体上限**：`io.LimitReader` 限 64 MB；超限该期跳过并记 Degraded（不 OOM）。
3. **回看期数上限**：默认最多处理最近 **12** 份符合条件的报告；超出部分跳过并记录。
4. **archives host 请求必须携带 UA**（reviewer 断言 29：www.sec.gov 对无 UA 请求同样 403）。
5. **单位过滤**（断言 24）：`unitRef` 非 USD 的 fact 排除。
6. **老式 filing（2019 前非 inline XBRL）的 `_htm.xml` 推导会 404**（断言 27）：
   该期跳过并记 Degraded，不中断整家。

## AD-12：模板迭代必须有全量重拉入口

**问题**（reviewer 断言 31 / §5.6，**阻塞 TASK-014**）：`since = LatestSegmentPeriodEnd` 意味着
第一次跑完锚点即前进。而 TASK-014 的工作方式正是「跑一次看 member 清单 → 改模板 → 再跑」——
第二次跑将拉不到任何数据，模板迭代流程在实现后**走不通**。

**决策**：`RefreshSegments` 增 `force bool` 参数（true 时忽略锚点全量重拉）；
`atlas prism refresh` 增 `--full-segments` flag 透传。TASK-014 用该 flag 迭代模板。

## AD-13：桑基必须有残差节点，否则守恒断言在真实数据上必然失败

**问题**（reviewer 断言 36/37 / §5.2）：`TestBuildSankeyBalance` 要求各节点入流≈出流(±1)，但真实数据中：
- `Σsegments ≠ Revenue`（corporate / other / eliminations 是常态）；
- `RnD + SGnA ≠ GrossProfit − OperatingIncome`（其他营业费用、摊销、减值）。

plan 只给收入侧留了 `tax_other` 残差，分部侧与 opex 侧没有 → 守恒断言只能在人造数据上过，
真实 MSFT/NVDA 必然失衡。

**决策**：桑基增加两个残差节点：
- `other_segment` = `Revenue − Σsegments`（|差| 小于 Revenue 的 0.5% 时省略该节点）
- `other_opex` = `(GrossProfit − OperatingIncome) − RnD − SGnA`（同阈值规则）

**负值处理**（断言 37，`NetIncome > OperatingIncome` 在有大额利息收入的公司是常态，
使 `tax_other` 为负）：残差为负时**不画负流**，改为在图上标注并计入 footnote，
守恒断言相应只对非负流部分成立且残差被显式记账。DoD 必须覆盖负 `tax_other` 场景。

## AD-14：除零必须产出 NaN，且 `jf` 兜底拦截 Inf

**问题**（reviewer 断言 40/42，**硬故障**）：YoY/CAGR/比率的分母为 0 时产生 ±Inf。
`encoding/json` 对 Inf 直接报错 → **整个 API 返回 500**。既有 `jf()` 只把 NaN 映射为 null，挡不住 Inf。

**决策**：双保险 ——
1. `internal/prism/sankey` 内所有比率/YoY/CAGR 计算：分母为 0 或 NaN 时返回 NaN（不产生 Inf）；
2. `jf()` 扩展为同时把 `math.IsInf` 映射为 null（防御后续新增计算路径）。

## AD-15：期数上限统一为 10

**问题**（reviewer 断言 43 / §5.4）：三处不一致 —— `DefaultSelection` 年度分支返回 ≤10 个 FY；
plan 的 API 写 `≤8 期截断`；plan Task 12 验收第 3 条要求「库中年报数全部并列」。

**决策**：**统一为 10**。`DefaultSelection` ≤10、API 上限 10、超出置 `Truncated=true`。
（design-spec §3.3 中的「8」全部读作 10。）

## AD-16：`LoadTemplates` 的 error 不得丢弃

**问题**（reviewer 断言 19/20/44 / §5.7）：plan Task 6 Step5 明文写 `templates, _ := sankey.LoadTemplates(...)`。
模板目录被 rsync `--delete` 清掉或 YAML 写坏时，表现为 sankey 全站 404 且日志无痕，
与全局约束「外部依赖必有降级路径 + 可观测」相悖。

**决策**：
1. `LoadTemplates` 遇非法 YAML 返回 error 且**错误信息含出错文件名**；
2. `serve.go` 与 `cmd/atlas/prism.go` 的调用点**不得丢弃 error**：记录到日志/Degraded；
   模板加载失败时 serve 仍启动但不注册 sankey 路由（降级可观测），不静默。
3. 校验 `company` 非空；`cik` 与 config 中该 symbol 的 CIK 不一致时报错（防模板串号拉错公司数据）。

## AD-17：Q4 推导用的 FY 期分部行不落库

**问题**（reviewer 断言 32）：若 FY 期分部值同时以 `FY20xx` 行落进 `segment_revenue`，
TASK-007 的 `fy` 聚合（4 季求和）会与 FY 行并存 → 重复计算。

**决策**：FY 期数据**只用于 Q4 推导，不落库**。`segment_revenue` 只存季度粒度行。
年度值一律由引擎从 4 个季度聚合得到（单一真相源）。

## AD-18：tag 链改动必须有 golden 回归（拆股检测受影响）

**问题**（reviewer 断言 2 / §3.1，**高风险**）：`detectSplits(epsFacts, sharesFacts)`
当前喂的是 `unitsOf("EarningsPerShareDiluted")` / `unitsOf("WeightedAverage…Diluted…")` 单 tag 结果。
改喂 tag 链后，拆股比例投票与生效日聚簇可能变化 → 历史 EPS/股本归一化改变 → `ttmPoints`
→ **PE/PB/PS 全序列漂移**。现有 9 个 fixture 测试只断言各自关注的少数字段，捕获不到这种全局漂移。

**决策**：TASK-001 增加 golden 回归断言 —— 对既有 fixture（至少含 `companyfacts_split`、
`companyfacts_double_split`、`companyfacts_nonsplit_jump`），改动前后 `FetchCompanyFacts`
的输出**逐季逐字段完全一致**（值 + 期数 + FiscalPeriod 标签）。
另断言：首选 tag 存在且可用时，即使回退 tag 也存在，结果与改动前一致（回退只在缺失时生效）。

## 未采纳 / 降级处理的 reviewer 建议

| reviewer 项 | 处置 | 理由 |
|---|---|---|
| 断言 11「旧二进制读迁移后的库」 | 降为 TASK-002 的 review 条目（论证加列不破坏显式列 SELECT），不做自动测试 | 需要旧版二进制产物，超出本 sprint 工程投入；`ALTER TABLE ADD COLUMN` 对显式列名 SELECT 的兼容性是 SQLite 契约 |
| 断言 9「对真实 runtime prism.db 副本执行迁移」 | 降为 TASK-015 的 review 条目（附命令输出） | 涉及用户生产数据，Leader 已在 TASK-014 明确禁止 agent 触碰生产库；改为在**副本**上验证并记录 |
| 断言 52「valuation_daily 逐行 diff」 | 同上，进 TASK-015 review 条目 | 同上 |
| 断言 17「UpsertPrices 单次调用时长」 | 不做时长断言，只做幂等与行数断言 | 时长断言在 CI 上不稳定，易成 flaky |
| 断言 51「Task 10 记录可复算证据」 | 已在 TASK-014 的 discovery 要求中 | 已覆盖 |
| §5.8「测试同包访问 s2.db」 | 无需处置 | 已确认 `sqlite_test.go` 是同包测试（`package prism`） |

---

## AD-10 修正（TASK-002 实测，2026-07-25）：真正的并发故障点是 schema 初始化的 SQLITE_BUSY

AD-10 原文判断「serve 常驻 + prism-daily 定时并发 Open 会出问题」**方向正确，但定位的故障点不对**。

**dev-agent-16 的定位实验**（240 次并发 Open × 两种库）：
- 非 WAL 存量库 + DSN `journal_mode(WAL)`：**3/240 次 Open 直接拿到 SQLITE_BUSY**
- 已是 WAL 的库：0/240

**根因**：DSN 里的 `journal_mode(WAL)` **转换需要独占访问，且不走 busy_timeout**
（对调 pragma 顺序无效，已实测）。连接是 `db.Exec(schema)` 时才惰性建立的，
所以 BUSY 从 **schema 初始化**处冒出来，而非 AD-10 预判的 `ALTER TABLE` duplicate column。

**修复**：schema 初始化改走 `execRetryBusy`（10 次、20ms 递增退避，累计约 1.1s）。
schema 全是 `CREATE TABLE IF NOT EXISTS`，重试幂等安全；重试耗尽如实返回原始 BUSY 不吞错。
修复前 `-count=20` 失败 2 次，修复后 `-count=40` 连续全绿。

**AD-10 原要求的 duplicate-column 容错仍然必要**，只是**不充分** —— 两者都要。

**生产影响**：runtime `prism.db` 由现有代码创建，早已是 WAL 模式，该路径只在
「首次以并发方式打开一个非 WAL 旧库」时触发，现网大概率碰不到，但已修掉。

**风险等级复核（gitnexus CLI 不可用，grep 手工评估）**：`prism.Open` 上游只有 2 处 ——
`cmd/atlas/serve.go:201`（失败则 serve 整体启动失败）与 `cmd/atlas/prism.go:99`
（失败则所有 `atlas prism` 子命令中止），两处均 fail-fast → **HIGH**。
`Open` 里任何新增的可失败步骤，一次失败就同时打掉常驻服务与定时任务。

**给后续任务的约束**：新列必须写在 `const schema` 的 `source` **之后**。
`ALTER TABLE ADD COLUMN` 只能追加，写到中间会让「新建库」与「迁移库」列序漂移；
`TestMigratedSchemaMatchesFreshSchema` 逐 cid 比对列名+类型会当场抓住。**勿动这个顺序。**

## AD-19：**非 docs-only 任务的** `packages` 只能声明 Go 包路径（框架口径缺陷）

> **修正（2026-07-25，dev-agent-15 实测）**：本条初稿写的是「`packages` 只能声明 Go 包路径」，
> **缺了「非 docs-only」这个前提**。`task-completed.sh` 的判定顺序是
> **先判 docs-only（`:36-39`）→ 是则整个跳过 Go 门禁（`:86-105`），压根走不到 `go test`**。
> 实证对比：
> - **TASK-003** 含 `verify_by: test` 条目 → 非 docs-only → 走 `go test` → `configs/` 路径
>   `no Go files [setup failed]` → DENY（本条初稿的依据，在此前提下成立）；
> - **TASK-014** 全部 `review|manual` → docs-only → **跳过 Go 门禁** →
>   `packages = ["configs/prism", "./internal/collector/edgar"]` **门禁通过**。
>
> **按未修正的版本去无谓收窄 packages 有两个副作用**（dev-agent-15 实测踩到）：
> ① 产物路径落到声明范围外 → **docs-only 门禁反而找不到范围内变更而 DENY**；
> ② review 项容易被漏掉（产物不在声明范围里，验收时没有指引）。
>
> **正确做法**：docs-only 任务**应当**在 `packages` 里声明产物路径（文档/YAML/配置目录）；
> 非 docs-only 任务才必须只声明 Go 包。

---

### 原始记录（前提已由上方修正框定）


**问题**（TASK-003 实测）：`task-completed.sh` 把 `packages` 的**全部条目**直接喂给
`go test` / `-coverpkg`。声明非 Go 路径（如 `configs/prism/templates`）必然
`no Go files ... [setup failed]` → `dev_done` 被 DENY（`./` 前缀与否均失败）。

但 `packages` 同时被 validator 用作 **scope 互斥校验**的依据，两个用途冲突：
互斥校验希望声明全部触碰路径，门禁却只接受 Go 包。

**处置**：在途任务的 `packages` **只声明 Go 包**；非 Go 产物（YAML/模板/文档）
写进 `progress_note` 说明，并由 Leader 在验收时人工核对。
TASK-003 已按此收窄（`configs/prism/templates/msft.yaml` 仍在交付范围内，
由 `TestLoadTemplatesFromRepoDir` 仓库冒烟测试覆盖）。

**注意**：这削弱了 scope 互斥校验对非 Go 产物的保护 —— 两个并行任务同时改同一 YAML 目录
不会被 validator 拦住。本 sprint 的任务图中 `configs/prism` 只有 TASK-003（wave1）与
TASK-014（wave5）触碰且串行，风险为零；后续排任务图时需人工注意。

**已向上游登记**：期望 `packages` 支持区分「测试范围」与「scope 声明范围」，
或门禁自动过滤非 Go 路径。

## AD-20：并发 agent 的未提交 WIP 会污染他人的 dev_done 门禁

**问题**（TASK-004 实测）：门禁按**整包编译**跑测试。dev-agent-18 的 `transition dev_done`
第一次 DENY 报的是 `internal/storage/prism/sqlite.go:114:23: undefined: strings` ——
那是 dev-agent-16 的 AD-10 实现中间态（已写 `strings.Contains` 但尚未补 import），
与 dev-agent-18 的改动毫无关系，却让它的门禁失败。

**影响**：任何并发 agent 的半成品都会误伤同包或**下游依赖包**任务的 `dev_done`，
产生与自身无关的 DENY，可能诱导 dev 去"修"不属于自己的文件（scope 漂移）。

**处置（已写入 wisdom）**：门禁 DENY 时**先区分「本任务问题」与「他人 WIP 污染」**。
隔离验证手法（dev-agent-18 首创）：
```bash
git worktree add --detach /tmp/verify-<task> HEAD
cd /tmp/verify-<task> && GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./<pkg>/ -count=1
```
在只含已提交状态的隔离副本里验证，干净排除他人未提交改动的干扰。
**确认是他人污染时：不要去改别人的文件**，等对方提交后重试，或向 Leader 报告。

## AD-21：`verifying` 状态是单点死锁风险（verifier 实例死亡则无解）

**问题**（sprint-028 实测）：test-agent-7 在验证 TASK-002 途中因 API 错误
（`Connection closed mid-response`）失败，且**未留下任何 checkpoint 或报告**。此时：

- `verifying` 在 write-matrix 里**只有两条出边**，且都是 `test-*` 专属：
  `verifying → verified`、`verifying → rejected`；
- `owner_table["verifying"] = ["test-*"]` → **leader 无 `update` 权**，改不了 `verifier` 字段；
- `bindings` 规定 `test-*` 绑定 `verifier` 字段 → **其他 test 实例也写不了**该任务。

结论：**该任务在原 verifier 恢复之前是死锁的**，且会级联阻塞其下游 wave。
这是状态机里唯一没有 leader 逃生舱的状态（`in_progress` 有 `in_progress → assigned` 收回边，
`assigned` 有 `assigned → assigned` 重派边，唯独 `verifying` 没有）。

**本次缓解手段**：agent 名在实例结束后仍可路由，`SendMessage` 会从其 transcript 恢复 ——
已用此法唤回 test-agent-7 续做。

**流程改进（本 sprint 起执行）**：
1. **test agent 开工第一件事是写 checkpoint**，而非等干完再写。中断代价从「全丢」降为「丢增量」。
   （已写入给 test-agent-7 的恢复指令；后续 spawn 的 test agent prompt 都要带这条。）
2. Leader 派验后应留意 verifier 实例的存活；收到 `idleReason: failed` 类通知立即核查其名下
   `verifying` 任务。

**向上游登记的需求**：为 `verifying` 增加 leader 逃生边（如 `verifying → dev_done`），
或允许 leader 在 `verifying` 状态改写 `verifier` 字段以便转派。当前设计对「验证者进程死亡」
这一现实故障没有恢复路径。

**相关**：终态任务的 `verifier` 字段残留（TASK-003 已 `rejected`/`in_progress` 但 `verifier`
仍指向 test-agent-6）会让 idle hook 误判该 test agent 仍有活干 —— 同属 verifier 字段的生命周期
管理缺失。

### AD-21 补充：本 sprint 的 API 中断频率与 dev/test 两侧的非对称后果

**实测频率**：wave1~wave2 期间发生 **3 次** agent 因 `API Error: Connection closed mid-response` 中断：

| 实例 | 任务 | 中断时进度 | 后果 |
|---|---|---|---|
| test-agent-7 | TASK-002 验证 | 干到一半，**无 checkpoint** | 产出全丢，且任务**死锁**（`verifying` 无 leader 出边） |
| dev-agent-17 | TASK-007 | 认领后 4 分钟，未建文件 | 无损失 |
| dev-agent-15 | TASK-005 | 认领后 15 分钟，未建文件 | 无损失 |

**dev 侧与 test 侧的后果非对称**：
- **dev 侧不死锁** —— 状态机有 `in_progress → assigned`（leader 收回）与 `assigned → assigned`（重派）
  两条逃生边，最坏情况是 Leader 收回重派，丢失的只是未提交的工作。
- **test 侧死锁** —— `verifying` 只有 `verified`/`rejected` 两条 `test-*` 专属出边，
  leader 既无 transition 边也无 `update` 权改 `verifier`。**这是状态机唯一没有逃生舱的状态**。

**恢复手段（三次均有效）**：agent 名在实例结束后仍可路由，`SendMessage` 会从其 transcript 恢复。
这是当前唯一的补救途径，但它依赖 transcript 尚在——不能替代 checkpoint。

**因此本 sprint 起的硬纪律（对全体 agent，不分 dev/test）**：
**认领任务后的第一个动作是写 checkpoint，而不是等干完再写。**
checkpoint 里必须包含**外部世界的事实**（live 实测数据、对账基准、外部 API 的结构性观察）——
这类信息在仓库里推导不出来，丢失后只能重新联网获取，代价远高于代码。

**给上游的需求（重申并升级）**：为 `verifying` 增加 leader 逃生边（如 `verifying → dev_done`），
或允许 leader 在该状态改写 `verifier`。在 API 中断达到本 sprint 这种频率（3 次/2 wave）时，
「验证者进程死亡无恢复路径」不是理论风险而是必然会发生的运行事故。

---

# 第三轮：wave2/wave3 实施中产生的决策（AD-22 ~ AD-25）

> **补登记说明（2026-07-25，test-agent-6 发现）**：以下四条此前**只写在 `design-spec.md` 就地位置**，
> 未登记进本文件。而本文件头部声明「Dev 实现时以本文件为准」，导致 dev 在 task JSON 里读到 AD-22
> 却在本表查不到。这是「派生位置未同步」的**反方向**——权威登记表落后于派生位置，
> 且比单点残留更隐蔽：**单点残留让人读到错的，这个让人读不到**，
> 而「查不到」通常被归因为「记错编号了」而非「登记表漏了」。

## AD-22：`Graph` 增 `Notes []string` 字段

**问题**（dev-agent-17 在 TASK-007 提出）：design-spec §3.2 一边要求「负残差被**显式记录**、footnote 记账」，
一边给的 `Graph` 只有 `Nodes`/`Links` —— **没有任何字段能承载该记录**。要求与结构自相矛盾（Leader 设计疏漏）。

**决策**：`Graph` 加 `Notes []string`（纯增字段，Node/Link 契约不变，当时 TASK-009/012 尚未开工，向后兼容）。
不加的话负 `tax_other` 只会被画成 0 宽，**图上凭空少一块且无人知情** —— 恰是 AD-13 要消灭的静默失真。

**被拒方案**：把说明塞进节点名 —— 污染前端渲染文本，且把结构化信息降级成字符串。

**透传链（三任务，任一环断掉前功尽弃）**：TASK-007 产生 → TASK-009 装配层原样透传进 `PeriodView`
→ TASK-011 透传进 API 响应 → TASK-012 渲染为可见 footnote。**三处 DoD 均已写入对应要求。**

## AD-23：XBRL 单位判定与干扰类型分布（live 实测校正 Leader 的两处失真）

**① 单位判定必须解析 `<unit>` 定义，不能只看 `unitRef` 字符串。**
计划示例写的 `unitRef="usd"` 是失真的；真实文档用引用 id（`U_USD`/`U_EUR`/`U_shares`），
且含 `U_UnitedStatesOfAmericaDollarsShare -> <divide>` 即 **USD/share（EPS 的单位）**。
按「measure 含 USD 就算营收单位」的天真判定，**每股值会被当成分部营收落库**
（合法数值、语义完全错误）。正确规则：只认**恰好一个直接子 `<measure>`** 且 local name 为 USD；
`<divide>` 里的 measure 是孙元素，Go 的直接子元素匹配天然排除。
`<unit>` 定义可能出现在引用它的 fact **之后**，故单位关联不能依赖文档顺序。

**② 「MSFT 没出现交叉维」只对分部轴成立**（Leader 早前基于局部 grep 的结论是取样偏差）。
全文档统计：409 个 context，explicitMember 个数分布 `{0:16, 1:276, 2:73, 3:44}` ——
**多维 context 真实存在 117 个**；而 276 个单维 context 里只有 **18 个**落在
`StatementBusinessSegmentsAxis`，其余是 DebtInstrumentAxis(73) / StatementEquityComponentsAxis(68) /
ProductOrServiceAxis(48)。→ **「单维但轴不匹配」才是压倒性主要干扰源，不是交叉维。两条排除规则缺一不可。**

另注：真实根元素用**默认命名空间**，`context`/`unit`/`period` 全无前缀，只有 `xbrldi:explicitMember` 带前缀；
`dimension` 属性值与 member 文本是 QName **字符串**，`encoding/xml` 不解析其中前缀，**须自行按最后一个 `:` 截断**。

## AD-24：`prismstore` 增只读 `InstrumentID(symbol)`

**问题**（dev-agent-17 在 TASK-009 提出）：design-spec §3.3 写「由 symbol 解析 instrumentID 的**既有途径**
（实现者按 store 现状选取）」—— **该「既有途径」根本不存在**（Leader 未核实 Store 现状就留下的洞）。
实测 Store 的 13 个方法：需 id 的四个用不了；`PriceSeries`/`Series` 收 symbol 但**不返回 id**；
`Board()` 的 `BoardRow` 无 id 且 INNER JOIN valuation_daily；唯一返回 id 的 `UpsertInstrument` 是**写**操作，
其 `ON CONFLICT DO UPDATE SET market=…, name=…, grp=…, source=…` 会把该标的字段**静默刷成空串**，绝不可用于读路径。

**决策**：加纯增只读方法 `func (s *Store) InstrumentID(symbol string) (int64, error)`，
`sql.ErrNoRows` → 包装 `ErrNotFound`，实现同 `PriceSeries` 的解析逻辑。
理由：symbol→id 本就是 store 层职责；纯增只读、对既有调用者零影响；
顺带为 Analyze/Fundamental 的「未知 symbol」提供统一错误来源。

**被拒方案**：(B) 窄接口改 symbol 口径 + TASK-011 写适配器 —— 适配器仍需同一个只读方法，
洞没堵、只是挪到 cmd/atlas 绕过 store 层；(C) `NewService` 多收 resolve 函数 ——
解析逻辑仍要有人写，只是换个地方写同样的 SQL 且偏离 §3.3 签名。

**连带**：TASK-009 的 `packages` 扩含 `./internal/storage/prism`；code-simplifier 顺势让
`PriceSeries` 复用该方法（Leader 已批准，test-agent-6 三层核实语义等价：SQL 逐字、错误串逐字、
运行时四种输入 diff 为空）。

## AD-25：`RefreshSegments` 第 5 参改为 `manualDir string`（去掉 `now time.Time`）

**问题**（dev-agent-16 在 TASK-008 实现时偏离，Leader 核实后认可）：§4.2 初稿第 5 参写 `now time.Time`，
**实测用不到** —— 分部刷新的 `since` 来自 `LatestSegmentPeriodEnd` 锚点（或 force 时的零值），
全函数无任何 `time.Now()` 依赖（与需要算 lookback 范围的 `Refresh` 不同，初稿是照其形状套的）。
反之第 5 步「manual 数据最后 upsert 覆盖同键自动行」需要 `LoadManualSegments(dir, symbol)` 的目录，
而初稿**描述了该步骤却没在签名里给出获取目录的途径**。

**与 AD-24 同一类洞**：**描述了要做什么，但没给做这件事所需的参数。**

**最终签名**：
```go
func RefreshSegments(cfg config.PrismConfig, store Store, seg SegmentClient,
	templates map[string]*sankey.Template, manualDir string, force bool) Report
```
TASK-010 的 cmd 接线按此调用，`manualDir` 生产值 `configs/prism/segments`。

## AD-26：`BuildPeriods` 返回 `[]PeriodConflict`，`PeriodMetrics` 增 `PeriodEnd`

> **补登记说明（2026-07-25，test-agent-7 发现）**：本条此前**只写在 `design-spec.md` 的内联注释里**
> （§3.2 三处），未登记进本权威表——而 TASK-011 的 DoD 两处引用了「AD-26」，dev 按 DoD 来此查证会扑空。
> **这是本 sprint 第二次犯同一个错误**（第一次是 AD-22~25 未登记，同样由 test agent 发现）。
> 根因诊断见文末。

**问题**：真实 EDGAR 数据中 FiscalPeriod 标签冲突（实测 MSFT **4 组 / 12.7%**，影响 71 行中的 9 行），
下游 `BuildPeriods` 把标签当聚合分组键用，导致：
- `buildQuarters` 的 `+=` 让不同季度的分部值**静默相加**（实测 `intelligent_cloud` 报 61.432B = 两季之和）；
- `aggregateFiscalYears` 只挡 `< 4` 不挡 `> 4`，**6 个季度被聚合成"完整财年"**
  （实测 FY2025 productivity 报 **392.914B**，真值 120.810B，**3.25 倍**）。

**决策**（TASK-016，两层防御各治一半、不可互相替代）：
1. `buildQuarters` 分部桶改 `(label, period_end)` 二级键，主表行只取与自身 `period_end` 相符
   （±15 天，`periodEndSlackDays`）的桶；对不上则 `Segments` 为 nil（**而非取最近的桶**）——
   「本期没有分部数据」会被 `other_segment` 残差吸收、可见；「错记了别的季度的值」不可见。
2. `aggregateFiscalYears` 增 `len(qs) > 4` 上界拒绝。
   **第 1 层拦不住第 2 层**：主表行本身就是 6 行 `period_end` 各不相同的合法数据。
3. 同比反查校验对照期 `period_end` 相差 **340~390 天**，否则退化 `ChangeKind: none`——
   实测标签冲突会让 `findPeriod` 拿到相差**两年**的对照期，输出 `change=0.75/kind=yoy`，
   **标签对了、期间错了**，且 0.75 是完全合理的增速、页面上无任何异常。
4. 签名改为 `([]PeriodMetrics, []PeriodConflict)`；`PeriodMetrics` 增 `PeriodEnd string`
   （财年取**最后一季**的 period_end——取首季会把年份标早 9 个月）。

**被拒方案**：保留原签名 + 另加带 report 的变体做向后兼容——**那等于留下一个丢弃冲突的便捷入口**，
AD-16 的教训正是「可忽略的错误一定会被忽略」。全仓生产调用点只有 `service.go` 两处，代价可控。

**透传链（三任务，任一环断掉前功尽弃）**：
`BuildPeriods` → `Analysis.Conflicts`（TASK-009 装配层）→ **TASK-011 API 响应** → TASK-012 降级提示。
**§5.1 的 `Analysis` 结构体块已同步补入该字段**（此前漏了，test-agent-7 发现）。

**与 AD-17（治本）的关系**：治本只覆盖「有年度条目」的路径；**无年度条目时回退原 fy/fp，
冲突风险原样保留**（test-agent-7 构造样本实证）。**故本条的防御不是冗余，而是降级路径的唯一防线。**

---

### 根因诊断：为什么"AD 定案必须登记进权威表"这条纪律没生效

AD-22~25 漏登记后，我立的纪律是「**AD 定案 = 权威表登记 + 全部派生位置更新**」。AD-26 仍然漏了。

**诊断**：AD-26 是我在**同步 dev 的签名变更**时顺手命名的编号，我把这件事归类为
「更新 design-spec」而不是「定案一条 AD」——**命名了一个 AD 编号 ≠ 走了 AD 定案流程**。
纪律绑定在"定案"这个动作上，而我当时的心智动作是"同步"。

**修正后的可执行判据**：**只要在任何地方写下 `AD-N` 这个记号，就必须立刻在权威表建条目**——
把触发条件从"我认为这是一次定案"（主观）改成"出现了 AD-N 字样"（客观、可 grep）。
自查命令（**作用域必须含代码与配置**，见下）：
```bash
grep -ohE 'AD-[0-9]+' -r .arcforge/docs .arcforge/tasks internal/ cmd/ configs/ | sort -uV
# 与权威表 `grep -oE '^## AD-[0-9]+' ADR | sed 's/## //'` 求差集
```

> **作用域加固（test-agent-7 实测指出）**：初版命令只扫 `docs` + `tasks`。它实测发现
> **代码与配置里实际引用了 16 个 AD**（`AD-2/3/5/8/9/10/11/12/13/14/15/16/17/18/22/24`，
> 散在 `sqlite.go` / `segments.go` / `periods.go` / 模板 yaml 的注释里）——
> 它们之所以今天没被漏掉，是因为**每一个恰好也在 docs 或 tasks 里被引用过**，
> **而不是因为命令扫到了它们**。
>
> 若将来有人**只在代码注释里**写下一个新编号（例如 dev 在实现注释里标一条决策编号），初版命令会扫不到。
>
> **注意本段不要写出真实的 `AD-` + 数字字面量**：加固后的命令会扫描本文件，
> 举例用的编号会被当成「被引用但权威表没有」而误报。**我加固后第一次运行就踩了这个**——
> 原文写了一个具体编号作示例，自查立刻把它报为缺失条目。
> 误报会磨钝检查（几次「查了发现是假的」之后，真报也会被当成噪声跳过），
> 所以**扩大作用域的同时必须清理作用域内的示例文本**，两者是同一次修改的两半。
>
> **教训**：我把判据的**触发条件**客观化了（"出现 AD-N 字样"而非"我认为这是定案"），
> 却在**判据的作用域**里留了一个主观假设——「AD 只会写在文档里」。
> **客观化必须覆盖判据的每一个组成部分，遗漏任一部分都会让整条判据退回主观。**

## AD-27：覆盖率读数非确定，比较与门禁判定须多次采样

**背景**：本 sprint 中 dev / Leader / test 三方报出的 sankey 包覆盖率长期对不上——
首轮 97.7 / 98.0 / 97.7，返工轮 97.4 / 97.5 / 97.8→97.5。我们一直当成「谁读错了口径」
（`go test -cover` 与 `go tool cover -func` 确实差 0.2pp，这个真实差异掩盖了真正的原因）。

**test-agent-6 实测定案**：连测 12 次，**10 次 97.5%、2 次 97.8%，同一份代码、同一条命令**。
不是谁读错了——**这个包的覆盖率本身就是非确定的**。

**机制**：`periods.go` 的 `segmentsAt` **命中即返回**，而 `TestBuildPeriodsUnparsablePeriodEnd`
的 fixture 含两个桶（合法的 `2026-03-31` 与不可解析的 `n/a`）。合法桶先被遍历到就直接返回，
`n/a` 桶根本不会进入 `sameQuarter`，于是「`time.Parse` 失败 → return false」这一分支时执行时不执行。
遍历起点随机（Go map），命中概率即上一轮坐实的 `(9−n)/8`。

**决定**：
1. **覆盖率的跨方比较、以及任何「是否达标」的边界判定，以多次采样为准**，单次读数不足以支撑结论。
   仅当读数远离门禁阈值（如 93.6% vs 阈值 80）时可用单次读数。
2. **变异测试的「杀死/存活」判定同样受此影响**，且后果更重：单次跑动看到「没变红」会直接
   得出「变异存活」的错误结论。实测例证——`sameQuarter` 不可解析分支变异成 `return true`
   后，**25 次只变红 6 次，存活率 76%**。今后变异记录一律写 **「N 次中红了几次」**，
   不写「杀死/存活」二元结论。

   > **报的是「用例层面」的 N/M，不是「单次函数调用」层面**（TASK-018 实测澄清）：
   > `n > bestN → n >= bestN` 在单次调用层面只有 **87.5%** 检出（平票时依 map 遍历序抖动），
   > 但用例内连跑 50 次后，**用例层面**漏检降到约 `10⁻⁴⁵`，故按本条报出 **20/20**。
   >
   > **⚠ 由此推翻本条原来的「≥20 次」建议**（dev-agent-15 实测）：N 该取多少**取决于实测的 p**，
   > 而 p 既不是直觉的 50%，也不能从代码直接推出。**先测 p，再由 `(1−p)^N` 定 N。**
   >
   > 决定性证据——**同一个测试，仅调换 fixture 里两行的先后顺序**：
   >
   > | fixture 顺序 | 单次检出 p | N=20 用例层漏检 | N=50 用例层漏检 |
   > |---|---|---|---|
   > | 较小月份在前（现状） | 0.876 | 7.5e-19 | 4.8e-46 |
   > | 较大月份在前 | **0.126** | **6.8%** ← 真·flaky | 0.12% |
   >
   > 有人为了「按月份排序好看」把两行挪个位置，测试就从 7e-19 漏检变成 **6.8%** 漏检，
   > **没有任何信号，它照样绿**。
   >
   > **⚠ 本条初稿说「若要保留默认下限，50 比 20 稳健得多」——这个说法要收回**
   > （TASK-020 实测反证）。50 确实比 20 好，但**它同样不达标**：
   >
   > ```
   > 不利方向 p=0.12416：
   >   N= 20 → 漏检 7.055e-02   ✗
   >   N= 50 → 漏检 1.322e-03   ✗ 仍超 1e-3 门槛
   >   N=209 → 漏检 9.264e-13   ✓
   >   N=250 → 漏检 4.038e-15   ✓
   > ```
   >
   > TASK-018 里那个「成本可忽略就加裕度」选出的 50，**恰好卡在 1e-3 门槛的外侧**——
   > 不是错得离谱，而是**差一点点、且从没人核算过**。dev-agent-15 的原话：
   > 「上次说那是运气好不是判断准，现在有了具体的量：运气差了 20%。」
   >
   > **更根本的教训：「默认下限」这个概念本身就是问题的一部分。** 任何固定的 N
   > 都会在某个 p 下失效，而 p 可以因为一次无害的排版调整而变化 7 倍。
   > **正解只有一条：`N = ceil(ln(target)/ln(1−p))`，p 实测、target 显式声明。**
   >
   > **⚠ 但在算 N 之前，先问能不能不算**（TASK-019 DoD 修正时想明白的）：
   > `(1−p)^N` 是**降级方案**，只在无法消除随机性时才用。
   > 先问「能不能让它**必然**发生」（结构性保证），问不出来再问「跑多少次才够」。
   > **结构性保证与概率性压制在测试报告上产出同样的绿色，但强度差一个量级。**
   > 反例——我在 TASK-019 的 DoD 初稿里写「连跑 ≥25 次」，
   > 而该任务的**全部目的**就是把 p 从 0.24 提到 1；一旦达成，N 取多少都无所谓。
   > 停在「压概率」的思路上时，挑数字成了唯一动作，**而挑得准不准都改变不了这是个降级方案**。

   > **「满分的 N/M」不足以说明健壮性，还要问它是在哪个档位上测出来的**
   > （test-agent-6，TASK-020 复盘）：TASK-018 报的 20/20 之所以那么漂亮，
   > 是因为它的 fixture **只有一个顺序**（p=0.875 的有利方向），不利方向根本不存在。
   > **20/20 反映的是有利档的表现，不是该用例的最坏情况。**
   >
   > **「端到端用例」是两条独立的链，必须两环都断言才算贯通**（dev-agent-23 实测，TASK-012）：
   > 变异 M6「页面 handler 不收集 Notes」使**页面环 20/20 红，而 API 环 20/20 全绿、完全无感**。
   > 名字叫「端到端」不代表它端到端——它可能只是若干条各自独立的短链并排放着。
   > 这精确刻画了 G4 那类残留的一般形态：**每一环在自己的边界都绿，而链路从未被整体验证过**。
   >
   > 由此得到一条更基本的：**p 不是代码的属性，是 fixture 的属性**——
   > 所以「这个测试有多强」脱离 fixture 无法回答。
   > 这也是为什么 TASK-020 必须**分方向统计**：只看「整个用例红了没有」，
   > 有利方向几乎必红，会把不利方向的表现完全掩盖——
   > **「红了好几个」不等于「红对地方」**。
   >
   > **机理**（20 万次实测，8 组不同月份对全部落在 0.874~0.877）：小 map 只有 1 个 bucket，
   > 键按**插入顺序**占据槽位 0、1；迭代起点是 0~7 的随机偏移，只有偏移恰好为 1 时
   > 后插入的键才先被访问，故 `P = 7/8`。**与哈希无关，由插入顺序决定**——
   > 这也解释了为何调换 fixture 顺序会把 p 翻到 1/8。
   >
   > **`fixture 里两行的列出顺序，静默决定了这个测试的可靠性`** ——与「交叉维 fact 前置」
   > 「干扰项同 tag」同族：**fixture 的不可见属性承载着测试强度**。
   > 处置：已安排在该 fixture 上方加注释说明「这两行的先后决定单次检出率是 7/8 还是 1/8，
   > 不要为排版调换」。

   > **那个 87.5% 本身是三轮修正的产物，过程比数字更值得记**：
   > dev 最初按直觉估「约 50%」→ 自测 40 次得 **32/40 = 80%** 并注明「并非直觉上的 50%，
   > 这个数字是测出来的不是推出来的」→ test-agent-6 用 **20 万次**采样得 **87.57%**，
   > **（后续更正：test-agent-6 那个 76% 存活率是 n=25 的小样本涨落。**
   > test-agent-9 用 600 次合并样本测得 **p=0.115，95%CI≈[0.090, 0.145]**，含 1/8=0.125 而不含 0.24；
   > 真值 0.115 下 `P(X≥6|n=25)≈0.06`。**本 sprint 唯一一个「来源于小样本、被更大样本推翻」的数字。**
   > 教训与本条正文一致：**概率必须实测，而实测的样本量本身也要够。**）
   >
   > 与它在 TASK-010 坐实的 `(9−n)/8` 模型在 n=2 的预测值 **87.5%** 吻合
   > （dev-agent-15 独立测得 175047/200000 = **0.8752 = 7/8**，两条路径殊途同归）
   > （80% 是小样本估计，p=0.875 时 sd≈5.2%，落在 1.4 个标准差内，统计上相容）。
   > **Leader 一度把最初那个「约 50%」写进本条**——一个未经验证的直觉数字，
   > 经转述后取得了与实测数字相同的书面地位。结论不受影响（无论 80% 还是 87.5%，
   > 单次调用都会放过一到两成，循环是必须的），但**数字本身错了**。
   > **循环正是把 p 提到 1 的手段**——看到概率性变异报满分不是矛盾，是循环生效了。
   > 若不加循环而直接报「20 次中红了 10 次」，那才是守护未加固的信号。

3. **变异必须先确认「跑起来了」再计数**（test-agent-6 与 dev-agent-15 独立踩到同一坑，各两次）：
   注入变异后若代码**编译失败**（典型：改成 `if false { ... }` 使某变量未使用），
   `go test` 输出里同样**没有任何 `--- FAIL:` 行**——与「变异存活」的输出形态**完全相同**。
   按「有没有 FAIL 行」计数会把编译失败记成存活，是**假阴性**。
   处置：每个变异先 `go vet`（或 `go build`）确认可编译，再跑测试计数。
   **推广**：任何「以某信号缺席为判据」的检查，都必须先确认「信号有机会出现」——
   缺席既可能是「没发生」，也可能是「根本没跑到」，两者在输出上不可区分。
4. **变异脚本「跑完了」不等于「跑对了」——已知四种伪装成合法结果的失败**
   （dev-agent-17 归纳，本 sprint 四种都实际发生过）：

   | 失败 | 表现 | 与「真结果」的区别 |
   |---|---|---|
   | a. 还原失败 | 备份路径不一致 → `cp` 静默失败 → **变异累积** | 8 次全报 KILLED，实为前面积累的变异所致；跑完源文件仍处污染态 |
   | b. 编译失败 | `if false {}` 使变量未使用 | 无 `--- FAIL:` 行，**与「变异存活」输出形态完全相同** |
   | c. `sed` 未命中 | 模式不匹配，源码根本没被改 | 测试全绿，**与「变异被杀」相反地表现为「无变异」** |
   | d. 变异体语义等价 | 如 `>= 15` 与 `> 14` 对整数天 | 报告为两个独立变异，实为一个 |

   **第五类（test-agent-9 实撞，TASK-019）：执行环境本身错了，导致全部结果无效。**
   它的 harness v1 在 worktree 根目录跑测试二进制，而 `go test` 真实 cwd 是 package 目录 →
   9 个走相对路径的用例**恒红** → 「全包」档的 20/20 是**假 KILLED**。

   > **前四类守卫没有一个失效，它们只是测的不是这件事**——三重防护全程报告正常、`TREE CLEAN`、
   > 每个变异的施加与还原都正确。所以第五类的本质不是「多加一道守卫」，而是
   > **守卫的作用域必须覆盖到执行环境本身**。
   >
   > 它只能靠**差值**（变异组 vs 对照组）发现，不能靠任何单组的自检。判据要写足：
   > **对照组必须与变异组跑在完全相同的执行环境下，且必须实测为 0 红**——
   > 光说「要有对照组」不够，若对照组也只跑单个用例档就同样看不出来。
   > **一个只在特定档位才现形的守卫，等于在其他档位不存在。**

   **第六类（dev-agent-23 实撞，TASK-013）：fixture 与生产不同，测试验证的是生产中不存在的场景。**
   `TestSankeyRoutesRegistration` 断言「prism 未启用时 `/api/prism/*` 返回 404」并**长期绿**。
   但它的 `newTestServer` 传的是 `Config{Host, Port}`——**没有 `TemplatesDir`**，
   而 `mux.HandleFunc("/", Dashboard)` 在 `if cfg.TemplatesDir != ""` 块内。

   | fixture | status | ctype | len |
   |---|---|---|---|
   | 无 `TemplatesDir`（既有用例） | 404 | text/plain | 19 |
   | **有 `TemplatesDir`（生产）** | **200** | **text/html** | **2896** |

   > **它看到的 404 是生产根本不会出现的。** 缺陷完整存在于生产，守护它的测试从头到尾是绿的。
   >
   > **本类与前五类的根本区别：变异测试对它无效。** 变异改的是**被测代码**，
   > 而问题在 **fixture 里不存在的那一行**——无论怎么变异被测代码，
   > 那个错误的 fixture 都会一致地给出「正确」结果。
   >
   > 它是 ME12（fixture 值与断言值相同致断言恒真，TASK-012）的孪生，但**更隐蔽**：
   > ME12 的两个值都写在用例里、肉眼可比；**本类的关键信息是缺席的**，
   > 读用例时看到的每一行都对，错的是没写的那行。
   >
   > **处置：反盲守卫**——把「fixture 必须贴近生产」本身变成一条可被变异杀死的断言。
   > dev-agent-23 加的 `requireDashboardCatchAllPresent` 即此：
   > 变异 A4「去掉 fixture 的 `TemplatesDir`」实测 **10/10 红**，
   > 谁再把它拿掉会立刻炸，而不是悄悄退化成假绿。
   >
   > **一般化的检查手段**（因变异无效，只能靠这两条）：
   > ① fixture 与生产配置逐项对照；② 跨 fixture 对照——同一份被测代码换一种 fixture 跑，
   > 结果应当一致；不一致就说明某个 fixture 在测别的东西。

   **共同点：变异测试的输出信号只有红/绿两位，任何环节出错都会伪装成合法结果。**
   这是少数几种「必须验证测量工具本身」的场合。处置：施加后 `go vet` 确认可编译（堵 b）、
   `diff` 确认变异真的写进去了（堵 c）、还原后 `diff` 确认恢复干净（堵 a）。

   > **(d) 没有机械堵法**（dev-agent-17 订正，本条初稿写「跨变异比对行为矩阵」是错的）：
   > 等价变异体的**定义**就是在所有输入下行为相同——行为比对无论跑多少输入都只得到「无差异」，
   > 而这恰恰**与「测试有洞」的表现完全一致**。判定两段程序是否等价一般情况下**不可判定**
   > （停机问题的推论）。
   >
   > 实际可执行的只有：**存活变异体逐个人工判定是「测试有洞」还是「语义等价」，
   > 判定结论与推理一并写进 discovery。禁止默认归类为等价。**
   >
   > 最后半句是要害——**等价是个方便的借口，它和「测试有洞」在数据上完全一样，只有推理能区分。
   > 所以真正的风险不是漏判，是图省事全判成等价**：那样报告好看，且没有任何机制能反驳。
   >
   > 正面样例（TASK-007）：「空 `FiscalPeriod` 的 `continue`」变异体存活，
   > dev-agent-17 推理出它落进的 `byPeriod[""]` 桶永远无人读取（空标签的主表行同样被跳过），
   > 据此判定等价并**如实写进 discovery，而不是凑一个 20/20 的漂亮数字**。

5. **概率性守护要当缺陷治**：76% 存活率意味着「不可解析的期末被并进本期」这个真实错误
   大约只有四分之一的跑动能抓住，在 CI 里可以连续多次不被发现。
   修法优先「构造无短路路径的专用子用例」（确定性最强），其次「循环 N 轮」。

**更一般的教训**：一个真实存在的小差异（`-cover` 与 `-func` 的 0.2pp）会**吸收掉**对更大异常的
怀疑——三方数字对不上时，我们每次都用这个已知差异解释掉了，于是没人去查为什么同一个人
两次读数也不一样。**已知的部分解释，是继续追查的阻力，不是终点。**
