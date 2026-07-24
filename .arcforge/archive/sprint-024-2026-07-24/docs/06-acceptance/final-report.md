# Prism M1 交付报告(final-report)

> Sprint: prism-m1 | 需求: docs/superpowers/plans/2026-07-23-prism-m1.md | 分支: feature/prism-m1
> 日期: 2026-07-24 | QA verdict: **PASS**(05-review/qa-review-prism-m1.md)

## 交付范围(10/10 任务 verified,QA PASS)

| Task | 内容 | commits | 覆盖率(changed scope) |
|---|---|---|---|
| 001 | PrismConfig 配置结构 | 2f3cb76 | 95.5% |
| 002 | Prism SQLite store(WAL,NaN↔NULL) | e80de76 | 80.6% |
| 003 | 理杏仁 FetchValuationSeries(.mcw/cvpos) | a0c55b2 | 92.1% |
| 004 | valuation 序列函数(alignPE 提取) | bb02624 | 98.5% |
| 005 | 刷新编排·理杏仁增量路径 | eb13663 | 88.6% |
| 006 | 刷新编排·美股引擎路径 | edec2e7+adab4cb+6973e44 | 95.0% |
| 007 | atlas prism refresh CLI+告警 | 38f7f70 | 核心 100%(AD-6 文件级) |
| 008 | JSON API board/series | 4f7f57c+5e920de+53dc55a | handler 六函数 100% |
| 009 | Web 页面(HTMX+ECharts vendor) | 42a20ff+c065e5e | prism 函数 96.4-100% |
| 010 | 配置示例/launchd/部署文档 | 4b91c3b+45d4020 | review/manual |
| - | 聚合简化(sortedEPSWithGate) | 02b2d88 | - |
| - | 过期返工 revert | 4fcbac8 | - |

全仓 go test ./... -count=1: 55 包全绿;go build/vet/fmt 干净。

## 质量过程

- 返工 2 例(T6/T8 断言缺口)一轮补齐;QA review_fix 1 轮(CRITICAL 磁盘模板/时区守卫/EPS 熔断/plist 路径/500 脱敏)全部复验闭环(含反向亲证与独立时区硬证)。
- AD-6 整包口径临时放行 5 例,均三查亲核+立即恢复+文件级复核补偿(详见 plan.md)。
- transition 审计干净;detect_changes 完整回归确认既有流程仅三类增量触点。
- 事件: dev-agent-3/5 会话卡壳(收回重派+token 轮换)、多次 API 连接中断(文件断点恢复,零工作丢失)。

## 遗留技术债/tickets(QA 共识处置,不阻塞 M1)

1. [M2 ticket·横切] /prism/detail 与 symbol_detail 同源: 启用 ATLAS_API_KEY 时客户端 fetch 无鉴权头 → 图表 401(deployment.md 已文档化已知限制;建议 same-origin 免鉴权统一整改)
2. [M2] status 分级 api/web 双实现收敛为共享分类器
3. [M2 follow-up] ReconstructPESeries 空序列 len==0 守卫;NaN 进度条 width 外观;ps_ttm 只写不读(注释意图或去列)
4. [框架侧] task-completed.sh 支持 changed-files 覆盖率口径(根治 AD-6);归档流程补 tokens 清理

## M1 人工验收清单(设计 §9,verify_by:manual,待人类执行)

1. `bin/atlas prism refresh -c configs/config.yaml`(runtime 目录)首跑成功,`sqlite3 data/prism.db 'SELECT COUNT(*) FROM valuation_daily'` > 0
2. /prism/board 上茅台与沪深300 的 PE(TTM)/10Y 分位对齐理杏仁官网同日数字(±0.1)——⚠ 同时校验 Task 3 的指数指标名(pe_ttm.mcw)外推,若报 metric missing 按计划修正常量
3. /prism/detail/NVDA 的 5Y PE 曲线形态(阶梯+价格波动)与参考图一致
4. 停跑一天,board 卡片显示「数据至 <昨日>」而非报错
5. 第二次 refresh,日志确认理杏仁只拉增量(区间 latest+1 起)

### 待同步 hooks 清单(人类执行)

本 Sprint 未改动 `project-template/hooks/`、`project-template/scripts/` 或 `templates/CLAUDE.md.template`——无待同步项。
(运行时根 CLAUDE.md 顶部 gitnexus 区块因索引重建被工具自动更新,属 gitnexus 自管理区块,非 Arcforge 模板漂移。)
