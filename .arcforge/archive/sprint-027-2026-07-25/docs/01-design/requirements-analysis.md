# Sprint 027 需求分析 — Prism M2.1 yahoo 客户端限流加固

> 需求源:`docs/superpowers/plans/2026-07-24-prism-m2.1-yahoo-throttle.md`(writing-plans 产出,含完整代码/测试/自审);
> 设计定稿:`docs/prism/atlas_prism_design.md` §9 M2.1 + §10 风险表(Sprint 026 尾声用户批准)。
> ECC 不可用,走降级路径;需求已高度精炼,直接结构化。

## 核心需求

| ID | 需求 | 来源 |
|---|---|---|
| R1 | 同实例请求节流:相邻请求最小间隔 500ms(持锁 sleep 串行化并发调用方) | 设计 §9 M2.1-1 |
| R2 | 429/5xx 指数退避重试:1s→2s→4s 最多 3 次,优先尊重 Retry-After(整数秒);仅幂等 GET | 设计 §9 M2.1-2 |
| R3 | 实例级重试预算封顶(20 次/run):极端持续限流快速失败,不拖长调度窗口 | 设计 §9 M2.1-3 |
| R4 | 三个调用点(FetchQuote/FetchHistory/FetchEPSHistory)统一走 do() 门;零调用方改动 | 计划 Architecture |
| R5 | 发布验证:launchd 直连环境 prism-daily `0 failed`(XOM degraded 除外) | 计划 Task 3(Step 7 执行) |

## 非功能约束

- N1 常量不进 config(简单优先);N2 错误语义零变更(`unexpected status: %d` 保留;网络错误不重试);
- N3 yahoo 包既有测试**零修改**通过;新测试保持该包标准库 testing 风格(无 testify);
- N4 GOTOOLCHAIN=local;勿动 go.mod;明确不做代理方案(设计已记录)。

## 模糊点

无。背景事故(2026-07-24 v1.4.0 首跑 20 failed/0 degraded)已复盘,根因与对策均见设计文档。
