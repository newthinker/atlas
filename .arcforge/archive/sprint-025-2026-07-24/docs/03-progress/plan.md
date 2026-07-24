# M1.5 AKShare Sprint 进度(plan.md)

> 真相源: `.arcforge/tasks/*.json`;分支 feature/prism-m1 | 阶段: QA 审查中

## 状态: 7/7 verified ✅

| Task | commit | 覆盖率/备注 |
|---|---|---|
| 001 配置 | e98483d | 95.7% |
| 002 akshare A股 | 3d46f71 | 85.2% |
| 003 HK+指数 | 1e1bfa3 | 87.8% |
| 004 refreshAkshare | 1e2a5a6 | prism 93.9%(AD-6#6 放行,文件级复核达成) |
| 005 降级链 | 12904ae | 94.2% |
| 006 CLI 告警 | d22f324 | 核心 100%(AD-6#7 放行,复核达成) |
| 007 部署产物 | 3cfb2eb | review 四条全过;manual×4 留人工 |

## QA 前置(全部完成)

- arcforge-validate 全绿;transition 审计干净(35 条迁移,执行者全在册,leader 无执行类迁移)
- 全仓 go test -count=1: 56 包零失败;detect_changes: 无新增既有流程触点
- 聚合 code-simplifier(simplifier-m15): 零 diff

## 当前: qa-agent-3 两轮审查中(对抗重点: 分位合并算法/降级链元数据/HK 双调用/切片别名/产物路径)

## 零返工记录: 本 Sprint 至今 0 reject 0 review_fix(M1 经验前置的直接效果)

## 待人工验收(M1.5 五条 manual): ①setup+launchd+curl ②茅台腾讯上墙+live 校验 ③兜底演练 ④二跑增量 ⑤分位 sanity
