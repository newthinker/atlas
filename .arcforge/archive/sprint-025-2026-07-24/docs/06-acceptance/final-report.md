# M1.5 AKShare 数据源交付报告(final-report)

> Sprint: m15-akshare | 需求: docs/superpowers/plans/2026-07-24-prism-akshare-source.md | 分支: feature/prism-m1
> 日期: 2026-07-24 | QA: 三视角审查+Leader 聚合裁决(见 qa-adjudication-m15.md),review_fix 1 轮复验通过

## 交付范围(7/7 verified)

| Task | 内容 | commits | 覆盖率 |
|---|---|---|---|
| 001 | 配置(FallbackSource/AkshareBaseURL) | e98483d | 95.7% |
| 002 | akshare collector A股 + fnum 健壮性/schema 漂移守卫 | 3d46f71+1f98924 | 86.6%(rework=1) |
| 003 | HK 双指标合并 + A 股指数 | 1e1bfa3 | 87.8% |
| 004 | refreshAkshare(增量+本地 5Y/10Y 分位)+Refresh 六参 | 1e2a5a6 | prism 93.9%(AD-6#6) |
| 005 | 指数自动降级链 + Report.Degraded | 12904ae | 94.2% |
| 006 | CLI Degraded 告警 | d22f324 | 核心 100%(AD-6#7) |
| 007 | 部署产物 + setup 可复现/文档硬化 | 3cfb2eb+0c99ae5 | review(rework=1) |

全仓 go test ./... -count=1: 56 包零失败;聚合 code-simplifier 零 diff;detect_changes 无新增既有流程触点;transition 审计干净。

## 质量过程

- **零开发侧返工**(对比 M1 的 2 reject+1 review_fix 轮)——M1 经验前置到 DoD/spawn prompt 的直接效果。
- QA 三视角(Minimalist/Skeptic/Architect)审查;qa-agent-3 本体因 API 中断卡壳(与 M1 同型),Leader 按降级路径聚合裁决;review_fix 1 轮(T2 fnum 健壮性 MAJOR、T7 部署可复现 Medium×2)全部复验闭环。
- AD-6 两例(#6/#7),均三查亲核+临时放行+立即恢复+文件级复核补偿。

## 遗留 tickets/记录(不阻塞)

1. [M2] 兜底 provenance 库内不可追溯(行级 provenance 属 spec 明确不做)
2. [记录] fdate 单坏行 fail-fast(可辩护语义保留);符号映射无边界校验;Minimalist S1-S3 polish(维持现状)
3. [NIT·待办] internal/config/config.go Source 字段注释未纳入 "akshare" 取值(一行,下次触碰该文件时顺手)
4. [框架侧] QA 子代理 dispatch 模式连续两 Sprint 卡壳,建议改为独立 reviewer 自聚合或修 TeammateIdle 误触发;task-completed.sh 支持 changed-files 口径(AD-6 根治)
5. [部署约定] requirements.lock 待部署机首次真实 setup 生成后提交

## M1.5 人工验收清单(verify_by:manual,待人类/授权执行)

1. `bash scripts/akshare/setup.sh` 成功(runtime 侧);launchd load aktools;`curl 127.0.0.1:8180` 可达
2. runtime 配置同步池变更后 `atlas prism refresh`: 茅台/腾讯上墙,detail 曲线连续;⚠ live 校验点(lg/baidu/index 接口名与字段)首跑核验
3. 指数兜底演练: 临时坏 lixinger key → 指数当日行仍写入 + 告警含「主源降级(已兜底)」;恢复后次日主源续跑
4. 二跑增量: akshare 标的零重复拉取(行数不变)
5. 分位 sanity: 茅台 5Y/10Y 分位 0-100 且方向与理杏仁官网一致(本地口径,允许数值差异)

### 待同步 hooks 清单(人类执行)

本 Sprint 未改动 `project-template/hooks/`、`project-template/scripts/`、`templates/CLAUDE.md.template`——无待同步项。
