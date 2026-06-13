# 进度看板 — lixinger collector rewrite

> 真相源：`.arcforge/tasks/*.json` 的 status 字段。本文件由 Leader 维护。
> 执行模式：**Arcforge 单 dev 串行**（依赖链强制顺序）。

## 阻塞高亮
（无）

## 任务状态

| 任务 | 标题 | wave | 依赖 | status | owner | rework |
|---|---|---|---|---|---|---|
| TASK-001 | 传输层 client.go（信封+重试+选项） | 1 | — | verified | dev-agent-1 | 0 |
| TASK-002 | valuation 改 request()/扁平key/指数.mcw（+删遗留测试） | 2 | 001 | verified | dev-agent-1 | 0 |
| TASK-003 | stock.go candlestick（History/Quote） | 3 | 002 | verified | dev-agent-1 | 1 |
| TASK-004 | fundamental.go non_financial（metricsList） | 4 | 003 | verified | dev-agent-1 | 0 |
| TASK-005 | fund.go 净值+多接口聚合 | 5 | 004 | verified | dev-agent-1 | 0 |
| TASK-006 | serve.go retry 开关接线+config | 6 | 005 | verified | dev-agent-1 | 0 |
| TASK-007 | 清理 _probe + 全量验证 | 7 | 006 | verified | leader | 0 |

## 阶段
- [x] Step 2 需求分析（01-design）
- [x] Step 3 任务拆分 + DoD + 追溯矩阵 + 手动任务图校验
- [x] Step 4 dod-gate 人类确认（已放行）
- [x] Step 5 dev/test 串行推进（7/7 verified）
- [x] Step 6 QA 两轮（r1 REJECT 5 CRITICAL → Leader live 取证裁决 → 修 4 真 bug + 单位归一 → r2 PASS）
- [x] Step 7 交付验收：7/7 accepted；final-report + changelog 已出
- [ ] 归档 /arcforge-archive（待人工确认）+ git 提交（待人工确认，提交前跑 code-simplifier）

## 全部 7 任务 accepted。rework：TASK-002/003/005 各 1 次。
## 全量 go test ./... 零 FAIL；lixinger 覆盖率 90.9%。

## 验证摘要
- 全量 `go test ./...` 全 PASS，零 FAIL；lixinger 包覆盖率 90.8%（≥80%）。
- `go build ./...` 通过；`go vet ./internal/collector/lixinger/` 无告警。
- baseline RED（history_test）已在 TASK-003 转绿。

## 降级备注
ecc/codex/gemini 不可用；validator 缺失（手动校验，无越权写入）；arcforge-write.sh 缺失（with-task-lock.sh）。
