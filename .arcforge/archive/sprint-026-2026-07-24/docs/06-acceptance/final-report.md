# Sprint 026 最终交付报告 — Prism M2「美股深度」

日期:2026-07-24 | 分支:feature/prism-m2(12 commits,e95f6bf..fdb5664) | 需求:docs/superpowers/plans/2026-07-24-prism-m2.md

## 交付内容

| 交付项 | 任务 | 说明 |
|---|---|---|
| EPSPoint.FilingDate + 生效日对齐 | TASK-001 | 防前视核心;零值回退 Date,既有路径零漂移(硬验收) |
| fundamental_q 表 | TASK-002 | EDGAR 季度事实存储,NaN↔NULL,幂等 upsert |
| EDGAR companyfacts 客户端 | TASK-003 | quarterization(duration-first 去重/区间 Q4 推导/tag 回退);经 live 三 bug 修复(rework 1) |
| refreshEdgar 路径 | TASK-004 | EPS_TTM/BVPS/RPS 阶梯+PB/PS+fallback engine;TTM 连续性守卫(rework 2) |
| cmd 接线 + 首批 20 家 | TASK-005 | US-GAAP filer,CIK 官方核验 20/20(XOM 新 CIK 2115436);UA 合规 |
| /prism/compare 页 | TASK-006 | 零新 API,≤8 标的 PE 叠加+横截面表 |
| 文档同步 + M2 验收 | TASK-007 | 数据源矩阵/部署文档;验收 7 条(5✅+2⚠部署后) |
| 拆股归一化(Sprint 中新增,用户批准) | TASK-008 | 时间线归一;NVDA 4:1+10:1 实证;同比例重复拆股支持(rework 1) |

## 质量数据

- 任务:8/8 accepted;rework 共 4 次(003×1 / 004×2 / 008×1),全部闭环,无熔断
- 测试:全仓 go build && go test -count=1 全 PASS(QA 亲跑两次);包覆盖率(门禁口径):valuation 98.5% / storage-prism 81% / edgar 92.2% / prism 94.5% / cmd 74.6%(AD-8 历史基线,新增函数文件级 100%)
- 验证:8 份报告(04-test/),Reality Checker 模式,含变异测试与独立阈值复算
- Code Review:两轮(常规+codex-cli 跨模型)+ iteration-2 定向复核;最终 verdict=PASS,0 CRITICAL / 0 未解决 WARNING
- 审计:transition 全合法、epoch 一致、.claude/ 运行时资产零改动;detect_changes 190 符号全在声明 scope 内

## NVDA 端到端实证(live smoke)

fundamental_q 71 季(Q1-Q4 均匀)/valuation 2513 日(~10Y)/防前视跳变落 filing date/pe_ttm 可复算(隐含 Close 落真实价位)/拆股后 PE 32(修复前污染值 74)/符号矛盾 0/fallback 演练 Degraded 正常。

## 已知限制(如实记录)

1. 两次真实同比例拆股相隔 <365 天会被 clusterEvents 误合并(极罕见,首批不涉及;QA 确认收录)
2. Q4 EPS = FY−ΣQ 为近似口径(股本变动误差通常 <1%,代码注释标注)
3. 拆股比例白名单原子比 + ≥2 财年投票(上市不满 2 年即拆股的公司暂不可检测,首批不涉及)
4. IFRS filer(TSM 等)不支持,ErrNotUSGAAP 明确报错(AD-2,M3+ 扩展)
5. yahoo/qlibpit 路径 FilingDate 零值=报告期近似(AD-1,历史分位轻微前视为已知限制)

## 部署后核对清单

1. A/H(akshare)+指数(lixinger)回归与 pe_percentile 策略数字零漂移(验收第 5 条,本地无凭据未验)
2. compare 页三标的(NVDA/MSFT/沪深300)折线与横截面表目检(验收第 7 条)
3. XOM(新 CIK 2115436,2024 重组实体)fundamental_q 序列起点核对,不足 10Y 应优雅 fallback engine(QA INFO)
4. 其余 19 家首跑逐科目缺漏核对(TASK-003 live 校验点批量版),缺科目按 NaN 落库并看 refresh 日志汇总
5. 生产 config(开发仓 configs/config.yaml 为真相源)增补 edgar_user_agent(真实邮箱)/edgar_lookback_years 与 20 家清单后部署

## 待同步 hooks 清单

无。本 Sprint 未修改 .claude/hooks/、.claude/scripts/、settings(审计确认零改动),无需会话外同步。
