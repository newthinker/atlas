# 终验收报告 — Telegram 信号汇总表格

> 分支 `feature/telegram-digest-table`（相对 master 5 提交）
> autonomy=dod-gate / scheduling=dag / 降级：ECC·codex·gemini·Go validator·write-hook 缺失

## 结论：✅ 全部 accepted，可交付

## 完成任务

| 任务 | 标题 | commit | 验收 |
|------|------|--------|------|
| TASK-001 | CJK 显示宽度工具 | 619faaf | width.go 100% |
| TASK-003 | router 缓冲 + FlushNotifications | 3f0bb11 | 87.6%，-race 无竞争 |
| TASK-002 | 分组表格 formatBatch/SendBatch | 6629618 → 611d8e4 | 整包 90.3% |
| TASK-004 | config 默认 + app defer flush 接线 | 39fb228 | 8 包零回归，串行 flush 回归 PASS |

零返工进入验证（rework：仅 TASK-002 经 1 轮 QA review_fix，rework_count=1，未触 max_rework=3）。

## 覆盖率
- 变更/新增函数：displayWidth/padRight/isWide 100%、formatBatch 91.7%、renderTable 100%、SendBatch 100%、sendRaw 64.3%（HTTP error 分支范围外）、FlushNotifications（router 整包 87.6%）
- telegram 包整包 90.3%（修复后较 83.6% 提升）
- 全仓：`go build ./...` 通过，50 包测试全绿，`-race` 无数据竞争

## 质量门
- **独立评审**：发现 1 个真实 bug（串行 workers≤1 路径不 flush → 信号永不发送+跨轮泄漏），已用 `defer FlushNotifications` 修复并加回归测试 `TestRunAnalysisCycle_SerialPath_FlushCalled`。
- **QA 两轮**（常规 + 跨视角对抗，cross-model 降级纯 Claude）：verdict=PASS，无 CRITICAL。2 条 WARNING（W1 escapeMarkdown 破代码块、W2 零值时间）经人类裁定做 1 轮 review_fix 全部修复：
  - W1：拆分 `sendMessage=sendRaw(escapeMarkdown)`，SendBatch 走 sendRaw 跳过转义（逐条 Send 仍转义，行为保留）
  - W2：`latest.IsZero()` 省略时间段
  - I1：slices.Contains；I3：hold 图标统一 ⏸️

## DoD 达成（对照 plan「完成标准」）
- ✅ 一轮多条放行信号汇成一条 telegram 消息，按买/卖/持分组、组内置信度降序、含中文名列对齐
- ✅ batch_notify:false 回退逐条即时发（有测试）
- ✅ 执行/冷却/信号存储语义不变（仍逐信号）
- ✅ 空轮不发消息
- ✅ go build ./... 通过；router/telegram/config/app 包测试全绿

## 关键决策
- D1 覆盖率基准：整包→变更文件（既有未测 HTTP 代码范围外）
- D2 串行 flush：defer 覆盖全部出口
（详见 wisdom/decisions-leader.md、02-plan/independent-review.md）

## 遗留（下迭代 backlog，非阻断）
- 含反引号的 symbol/name 会破坏 ``` 围栏（现网数据无；escapeMarkdown 本就不处理反引号）
- digest 消息 >4096 字符的 Telegram 上限未做分页（信号极多时）

## 部署验证（按 plan）
```
bash scripts/ops/deploy.sh && bash scripts/ops/services.sh restart
bash scripts/ops/services.sh analysis-now
# 预期：Telegram 收到一条按动作分组的等宽表格，而非数十条单条消息
```
