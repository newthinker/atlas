# Prism M1 需求分析

> Sprint 输入: `docs/superpowers/plans/2026-07-23-prism-m1.md`(git 83f7765)
> 上游设计: `docs/prism/atlas_prism_design.md` v2(git dd2b7df,D8/D9/D5')

## 降级说明(capabilities)

- ECC 不可用(`ecc: false`)→ 未走 `/multi-plan`。
- 输入文档本身已是完成 Self-Review 的实现计划(10 个 TDD task,接口签名在定义/消费
  task 间逐一核对),设计精炼在计划编写阶段已完成,brainstorming 判定为已完成,
  不重复展开。本文档为需求结构化摘要。

## 目标(M1「现成数据上墙」)

用理杏仁现成数据(A/H 公司+指数、标普500)与 yahoo 重建路径(美股公司近 5Y),
把 `/prism/board` 估值卡片页和 `/prism/detail` 时序页跑起来,每日由 launchd 增量刷新。

## 功能模块与复杂度

| # | 模块 | 复杂度 | 依赖 |
|---|------|--------|------|
| R1 | PrismConfig 配置结构 + 默认值 | 简单 | - |
| R2 | SQLite store(instrument/valuation_daily,NaN↔NULL,WAL) | 中等 | - |
| R3 | 理杏仁时序方法 FetchValuationSeries(公司/指数 .mcw 口径,cvpos×100) | 中等 | - |
| R4 | valuation 序列函数(ReconstructPESeries + RollingPercentile,提取 alignPE) | 中等 | - |
| R5 | 刷新编排理杏仁路径(增量:LatestDate+1,理杏豆计费约束) | 中等 | R1,R2,R3 |
| R6 | 刷新编排美股 yahoo 引擎路径(每日全量重算近 5Y) | 中等 | R4,R5 |
| R7 | `atlas prism refresh` 子命令 + telegram 失败告警(复用 crisis helper) | 中等 | R5,R6 |
| R8 | JSON API /api/prism/board、/api/prism/series(NaN→null,status 标签) | 中等 | R2(+ErrNotFound) |
| R9 | Web 页面 board/detail(HTMX 卡片 + vendored ECharts 双图 + /static) | 复杂 | R8 |
| R10 | 配置示例、launchd plist、部署文档、M1 五条验收 | 简单 | R7,R9 |

## 全局约束(必须传导到 DoD)

1. module `github.com/newthinker/atlas`,go 1.24.4;**勿升级 modernc.org/sqlite(固定 v1.38.2)**;toolchain 问题加 `GOTOOLCHAIN=local`。
2. 测试风格 testify + httptest;测试文件顶部 Context Checkpoint 注释映射验收条目。
3. 缺失值约定:内存 `math.NaN()`,store 写 NULL,读回 NaN。
4. 估值阈值:百分位 <15 低估、>85 高估(配置可覆盖)。
5. 理杏仁按理杏豆计费:时序请求前先查 LatestDate 只拉增量;首次回填 lookback_years(默认 10)。
6. 理杏仁美股指数仅标普500(^GSPC→.INX);其余留 M2。
7. 前端不引新外部 CDN:ECharts embed 进二进制。
8. 每 Task 单独 commit(`feat(prism):` / `refactor(valuation):`),提交前跑 gitnexus detect_changes。
9. 中文注释风格一致;不动无关代码;docs/deployment.md 有未提交改动,只追加不重排。

## 风险点

- **Task 3 ⚠ 实测校验点**:理杏仁指数端点原始指标名(`pe_ttm.mcw`)按 cvpos 命名规则外推
  未 live 验证——Task 7 首跑若 metric missing,以真实响应修正常量并同步测试。
- Task 7 的 `loadConfigForCLI`/`buildCrisisSender` 是占位名,实现时必须读 `cmd/atlas/crisis.go`
  确认同包 helper 实名后复用,禁止自建第二套。
- Task 8/9 同时改 `internal/api/server.go`,Task 7/8 同时改 `cmd/atlas` 包 → 调度上强制串行
  (scope 互斥)。
