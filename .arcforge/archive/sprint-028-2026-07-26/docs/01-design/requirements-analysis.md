# Prism M3「财报桥主线」需求分析

- **需求文档**：`docs/superpowers/plans/2026-07-25-prism-m3.md`（superpowers plan，已含 12 个 TDD 任务与接口草案）
- **上游设计**：`docs/prism/atlas_prism_design.md` §5.5 / §5.6 / §7 / §9 / §10
- **分析日期**：2026-07-25
- **降级说明**：`arcforge.config.json` 的 `capabilities.ecc = false` → 不走 `/multi-plan`。需求文档本身即
  superpowers 流程产出（`docs/superpowers/plans/`），brainstorming 阶段已在上游完成，本阶段直接进入
  结构化分析 + live 事实校验，不重复精炼设计。

---

## 1. 目标

上线 Prism 财报桥能力，三条并列主线：

1. **桑基财报桥**（设计 §5.5/§7）：单期「分部营收 → 收入 → 毛利 → 营业利润 → 净利」资金流图。
2. **多期分析引擎**（设计 §5.6）：期聚合（季/财年）、智能对比上下文、对比矩阵（YoY/CAGR）。
3. **`/prism/fundamental` 财务趋势页**：季度指标序列 + 股价叠加。

前置修复：**EDGAR tag 回退链扩展**，救回 COST/V/CRM/WMT/AVGO 五家当前 degraded 的数据。

## 2. 范围边界（用户决策）

| 纳入 M3 主线 | 移出至 M3.5 |
|---|---|
| EDGAR tag 回退 + 主干流科目 | Stooq 备源 |
| XBRL filings 分部解析 + manual 兜底 | etfholdings / D3 成分聚合 |
| 多期引擎、Sankey/Fundamental API 与页面 | |
| 首批模板（5~10 家）+ live 校验 | |

## 3. 关键约束（继承 M1/M2 + 本期新增）

- `modernc.org/sqlite` 固定 **v1.38.2**；所有 go 命令加 `GOTOOLCHAIN=local` 前缀（Go 1.24.4）。
- NaN ↔ NULL 双向往返；testify（assert + require）；测试文件头「Context Checkpoint: done_criteria → test mapping」注释块。
- commit 前缀 `feat(prism):` / `docs(prism):`；提交前跑 `gitnexus detect_changes`。
- **EDGAR 限频**：串行请求天然 < 10 req/s；UA 必填。
- **既有 A/H（akshare/lixinger）与美股估值链路零行为变更**；`fundamental_q` 加列必须带存量库迁移
  （runtime prism.db 已有生产数据）。
- 主干科目缺失 → 该节点显示「—」+ footnote，不整图失败；某公司分部解析失败 → 走 manual 兜底，
  **不阻塞其余交付**。
- 新增 `configs/prism/` 目录，launchd WorkingDirectory 下相对路径可达。

## 4. 代码现状（调研结论，含 plan 未记载的事实）

| 事项 | 现状 | 影响 |
|---|---|---|
| `edgar.Client` | `New(ua)` / `NewWithBaseURL(ua, baseURL)`，单 `baseURL` 字段 | 分部解析需第二 host（archives），要加构造器 |
| tag 辅助 | `unitsOf`/`hasQuarterly`/`firstQuarterlyTag`/`durationMetric` 全是 `FetchCompanyFacts` **内部闭包** | tag 链扩展在函数内改，不需重构 |
| 包级 tag 常量 | 只有 `revenueTags`，其余（EPS/shares/equity）为内联字面量 | 需提为包级变量供 segments.go 复用 |
| 测试 helper | 实际名为 **`factsFileServer(t, fixture, wantPath)`**（plan 写的 `factsServerFile` 不存在） | 直接复用，无需新增 helper |
| `storage/prism` | 单 `const schema` 三张表；**无任何 migration 机制**（无 user_version / ALTER） | 迁移逻辑为全新建设 |
| 无 `price_daily` / `segment_revenue` | 确认不存在 | 新表追加进 schema 常量即可 |
| `prism.Store` 接口 | 5 个方法（refresh.go:21） | 扩 segment/price 方法需同步 fake |
| closes 产生点 | `refreshEdgar` refresh.go:406、`refreshEngine` refresh.go:190 | `UpsertPrices` 两处都要接 |
| **YAML 解析** | 仓库**不直接 import yaml 包**；crisis config 走 **viper + `mapstructure` tag** | 见 AD-1 |
| **web 模板双份目录** | `internal/api/handler/web/templates/`（embed）与 `internal/api/templates/`（serve.go 磁盘实际使用），内容必须逐字一致，已有回归测试 `TestNewHandlerDiskModeHasPrismTemplates` 守护 | 新增页面模板**必须同步两处**，plan 的 File Structure 只写了一处 |
| `PrismDetail` 路径参数 | 由 server.go 侧 `TrimPrefix` 剥离后作为第三形参传入 | 新页面沿用 |
| 已知既有缺陷 | `prism_compare.html` 直接用 `r.json()` 结果，未解 `response.JSON` 的 `data` 包裹层（`prism_detail.html` 写法正确） | **本期范围外，不修**，仅记录；新页面必须用 `resp.data` 正确写法 |

## 5. Live 事实校验（2026-07-25 实测，plan 标注的 ⚠ 不确定点）

网络：`data.sec.gov` 与 `www.sec.gov` 经本地 proxy 均 200 可达，live 校验路径成立。

| plan 假设 | 实测结果 | 结论 |
|---|---|---|
| submissions `filings.recent` 平行数组含 form/accessionNumber/primaryDocument/reportDate/filingDate | 完全一致 | ✅ 采纳 |
| 实例 URL = `{archives}/edgar/data/{cik}/{acc}/{doc _htm.xml}` | 实际路径为 **`/Archives/`（大写 A）** | ⚠ 修正，见 AD-3 |
| 轴 `StatementBusinessSegmentsAxis` | 存在，18 个 context | ✅ |
| member 名 | `msft:{ProductivityAndBusinessProcesses,IntelligentCloud,MorePersonalComputing}Member` — 与 plan 的 msft.yaml **逐字一致** | ✅ 模板可直接落地 |
| revenue tag | 全部 `RevenueFromContractWithCustomerExcludingAssessedTax`，**无同 (period,member) 多 tag 冲突** | ✅ 优先级规则保留但实测不触发 |
| 交叉维度 context 需排除 | MSFT 分部 context **全为单维度**，未出现交叉维 | 防御规则保留（他司/他轴可能有），但 fixture 需自造该场景 |
| 「每份报告一个 SegmentPeriod」 | **不成立**：一份 10-Q 含 4 组期间（本季 90d、去年同季 90d、本年累计 274d、去年累计 274d）；一份 10-K 含 **3 个财年** FY（365/366d） | ⚠ 重大修正，见 AD-2 |
| duration 过滤 70~100d / 350~380d | 有效：274d 累计期被正确排除；366d 闰年 FY 在 350~380 内 | ✅ |
| 文档体积 | **9.7 MB（10-Q）/ 10.5 MB（10-K）** | 流式 `Decoder.Token()` 解析为硬性要求，不可建全树 |

实测数值样本（MSFT FY26Q3，2026-01-01~2026-03-31）：
Productivity $35.013B / IntelligentCloud $34.681B / MorePersonalComputing $13.192B。
10-K（FY2025，2024-07-01~2025-06-30）：$120.810B / $106.265B / $54.649B。
→ 这两组数字即 TASK-014 live 校验的对账基准。

## 6. 验收标准来源

- plan Task 12 Step 2 的 7 项手动验收 → 拆为「可自动断言部分」（进各任务 done_criteria）
  与「需人工目检部分」（浏览器渲染、PNG 导出、中英切换）。
  **后者写入 `06-acceptance/final-report.md` 的人工验收清单，agent 不声称已完成。**
- 各任务 `done_criteria` 由 plan 的 Step 1 失败测试清单逐条转化，是 Dev 写测试与 Test 验证的唯一依据。

## 7. 风险登记

| 风险 | 等级 | 缓解 |
|---|---|---|
| 他司 XBRL 结构与 MSFT 不同（AAPL 走 `ProductOrServiceAxis`、TSM 为 IFRS/20-F） | 中 | 模板 `segment_axis` 可覆盖；解析失败 → manual 兜底；TSM 明确记录跳过 |
| 存量 prism.db 迁移失败导致生产数据不可读 | **高** | 迁移只做 `ADD COLUMN`（不改既有列）；DoD 强制「旧 schema 库 Open 成功且数据可读」双断言 |
| web 模板双份目录漏同步 → serve 启动即失败 | 中 | 已有磁盘模式回归测试；写入两个页面任务的 done_criteria |
| 实例文档 ~10 MB × N 家的解析耗时/内存 | 中 | 流式解析 + 增量（稳态每家每季 1 次） |
| 人工目检项无法由 agent 完成 | 确定 | 显式列入 final-report 人工验收清单，不伪称完成 |
