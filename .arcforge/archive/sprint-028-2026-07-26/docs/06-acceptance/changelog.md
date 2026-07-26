# Changelog — sprint-028 / Prism M3「财报桥主线」

**2026-07-25 ~ 2026-07-26** · 20 任务 · 31 commits · 全仓 58 包测试通过

## 新增

### 桑基财报桥
- **`/prism/sankey` 页面**：小倍数网格、对比矩阵、单期大图/堆叠柱三视图切换、
  报告期范围选择、中英切换、PNG 导出
- **`/api/prism/sankey`**：多期分析响应，含 `Graph.Notes`（负残差/0 宽节点/统一比例尺近似说明）
  与 `Analysis.Conflicts`（被拒绝聚合的期与原因）
- **桑基模板体系**：`configs/prism/templates/` 下 5 家（MSFT/AAPL/GOOGL/AMZN/NVDA），
  支持公司自定义 member 名映射；manual YAML 兜底路径已实现（**尚未被真实执行**）

### 财务趋势页
- **`/prism/fundamental`**：7 项指标切换、折线/柱状切换、季度/年度切换、dataZoom、
  **股价叠加双轴**（左轴指标 / 右轴价格）
- **`/api/prism/fundamental`**：指标序列 + 股价序列，NaN 与 ±Inf 统一映射为 `null`

### 数据层
- `fundamental_q` 扩 5 列（EPS/shares/equity 等），`segment_revenue`、`price_daily` 两张新表
- **XBRL 分部营收解析**：走 submissions API + 报告实例文档（companyfacts 不含分部维度）
- **EDGAR tag 回退链**：EPS/shares/equity 多 tag 回退 + EPS 推算回填

## 修复

- **`/api/prism/*` 与 `/prism/*` 未启用时返回 200 + dashboard HTML** —— Go `ServeMux` 的
  `"/"` 是 catch-all，且 `/api/` 无自有 catch-all。前端 `r.json()` 会拿到 HTML 直接抛。
  改为**无条件注册路由 + 未启用时返回带原因的 404**（API 侧为 JSON）
- **`fiscal_period` 标签冲突致财年聚合错误** —— 实测 FY2025 productivity 报 392.914B
  而真值 120.810B（**3.25 倍**）。两层处置：`BuildPeriods` 拒绝并记录冲突（防御）+
  `FiscalPeriod` 改由期间自身推导（治本）
- **`renderSingle` 在无报告期时使 `singleChart` 为 null** —— 切「单期大图」抛 TypeError 整页停摆
- **`refresh` 的 Degraded/Failed 明细** 改为始终打印到 stdout

## 测试健壮性

- **`sameQuarter` 不可解析分支**：概率性守护（实测 p≈0.115）改为**结构性保证**（p=1），
  副作用是消除了 sankey 包覆盖率的非确定性（97.5%/97.8% 抖动 → 恒定 97.7%）
- **`anchorYearMonth` 阈值与 `fiscalYearEndMonth` 众数/平票**：补直接锚点，
  三档边界 + 平票稳定性各自有变异守护
- **平票 fixture 顺序**：改为对偶子用例——**两行的书写顺序原本静默决定测试可靠性**
  （p 7/8 vs 1/8），现无论谁调换总有一个方向落在不利档位并被覆盖
- **`fundamentalMetrics` 全部 9 条序列**：补表驱动断言（原仅 `revenue` 有断言）
- **端到端用例**：`/prism/sankey` 页面新增真引擎驱动的贯通用例
  （真 YAML → 真 `LoadTemplates` → 真 sqlite → 真 `Analyze` → 真 mux → 页面 HTML）

## 文档

- `docs/prism/atlas_prism_design.md`：D4 现实注记、§7 模板 schema 同步、§9 M3/M3.5 划分
- `docs/deployment.md`：`configs/prism/` 部署说明（rsync 黑名单结论 +
  **一个坏模板会禁用整个 sankey 服务**的排查提示）
- `docs/superpowers/plans/2026-07-25-prism-m3.md`：M3 验收记录（自动验证项标注实测结果，
  人工项标注「待人工验收，未由 agent 验证」）

## ⚠ 已知未验证

1. **COST/V/CRM/WMT/AVGO 的收益从未 live 实测** —— tag 回退链的立项理由，但验证全部基于
   合成 fixture；live 校验只覆盖 5 家模板公司
2. **manual 兜底路径从未执行** —— `configs/prism/segments/` 磁盘上不存在
3. **三项 JS null 语义无自动化守护** —— 静态守卫强度止于文本级，详见 final-report §4.3

## 破坏性变更

- `TestSankeyRoutesRegistration` 语义反转：从「未启用时不注册路由」改为
  「**始终注册，未启用时返回 JSON 404**」。依赖旧语义的测试需同步更新。
