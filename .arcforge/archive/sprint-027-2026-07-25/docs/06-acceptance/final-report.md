# Sprint 027 最终交付报告 — Prism M2.1「yahoo 客户端限流加固」

日期:2026-07-25 | 分支:feature/prism-m2.1(3 commits:cf147dd/fd0dedd/6288a48) | 需求:docs/superpowers/plans/2026-07-24-prism-m2.1-yahoo-throttle.md

## 交付内容

- do() 节流+退避门(TASK-001):同实例 500ms 节流(持锁串行)、429/5xx 指数退避 1s→2s→4s×3、Retry-After 优先(整数秒,**min 60s cap**——QA WARNING 闭环)、实例级重试预算 20 封顶、网络错误不重试、错误语义零变更。
- 三调用点接线(TASK-001/002):FetchQuote/FetchHistory/FetchEPSHistory 全走 do 门,grep 零残留;零调用方改动。
- 测试:throttle_test.go 12 用例(含 reviewer 增补 4 点:Retry-After 非法值回退/网络错误不重试/预算耗尽恰 N 请求/-race);既有测试零修改;覆盖率 86.2%(门禁 80)。

## 质量数据

- 任务:2/2 accepted;rework 1 次(TASK-001,QA WARNING cap);无熔断
- 验证:Reality-Checker 两任务+rework 附录,计时断言均 3 连跑防 flaky,-race 全量 ok
- Code Review:两轮(常规+双视角对抗降级——codex 轮因 API 不稳定获批降级)+iteration-2;最终 PASS,0 CRITICAL/0 未决 WARNING
- 审计:transition 全合法;epoch 机制实战验证(dev-13 连续中断被收回 epoch 1→2,迟到写入零渗漏);detect_changes risk=low(12 符号全在 yahoo 包,0 受影响执行流)

## 运维记录(如实)

2026-07-24 16:23~22:33 API 连接中断 8 次,波及 qa-5(弃,qa-6 接)/dev-13(收回,dev-14 记录员收尾)/dev-14。全程零工作丢失——文件真相源+状态机+epoch 防双写按设计工作。

## 已知限制

- 节流/预算为实例级:常驻多实例进程各自独立(当前生产每日新建实例,无影响)
- Retry-After 仅解析整数秒(http-date 回退指数退避)

## 发布步骤(R5,待执行)

1. tag v1.4.1 + push;2. deploy.sh + kickstart serve;3. kickstart prism-daily(launchd 直连)→ 预期 `25 ok, 0 failed, 1 degraded`(XOM);4. 次日 08:30 定时首跑复核,连续两日 0 failed 即验收,结论补入设计文档 §9 M2.1。

## 待同步 hooks 清单

无。本 Sprint 未修改 .claude/hooks/、.claude/scripts/、settings(审计确认零改动)。
