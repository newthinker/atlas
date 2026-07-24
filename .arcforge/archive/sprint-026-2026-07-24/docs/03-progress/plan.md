# Prism M2 进度计划(Sprint 026)

> 真相源:`.arcforge/tasks/*.json`;分支 feature/prism-m2 | dag | max_rework:3

## QA 终审结果(sprint-026-code-review.md)

**verdict=PASS**,0 CRITICAL / 3 WARNING / 2 INFO。两轮完成(常规全量亲跑 + codex-cli 0.139.0 跨模型)。codex 报的 3 条"CRITICAL"经 QA 逐条对照实码评估:均不在首批触发或属刻意设计。

按 severity_threshold=warning 处置:
- W1(effByRatio 同比例拆股合并丢失)→ TASK-008 review_fix(dev-11,rework 1/3)
- W2(ttmPoints 不校验季度连续性)→ TASK-004 review_fix(dev-9,rework 2/3)
- W3(派生值口径)→ 刻意设计,discovery 记录 QA 结论,不修
- INFO(XOM 新 CIK 历史深度)→ 进 final-report 部署后核对清单

## 任务看板

6/8 verified;TASK-004/008 review_fix 修复中(并行,包互斥)。修复 verified 后:QA 定向复核(iteration 2/3)→ 全任务 accepted → final-report → /arcforge-archive。

## 部署后核对清单(final-report 固定段落草稿)
1. A/H(akshare)+指数(lixinger)回归与 pe_percentile 零漂移(验收第 5 条,本地无凭据)
2. compare 页三标的(NVDA/MSFT/沪深300)目检(验收第 7 条)
3. XOM(新 CIK 2115436)fundamental_q 序列起点核对(QA INFO)
4. 其余 19 家逐科目缺漏首跑核对(TASK-003 live 校验点批量版)
