# TASK-004 验证报告 — 运维文档对齐与终验（review/manual）

- 验证者：test-agent-1（Reality Checker，默认 NEEDS WORK）
- 提交：cbfbed9（§2/§5 更新，文档首次入库）+ d59bc74（§2 末尾 summaryDue 残留行补修）
- 承接 epoch：1
- 文档：docs/ops/crisis-monitor-notifications.md

## Done Criteria 核对（review/manual，无覆盖率矩阵）

| # | verify_by | 完成标准 | 文档证据 | 判定 |
|---|---|---|---|---|
| F0 | review | §2 频率表 NORMAL 拆两行：周报([P1] 📅 无退出进度行，每周一 1 条) + 月报(21 观测日趋势 sparkline，每月首交易日 1 条；撞日只发月报) | line 38 `NORMAL \| [P1] 📅 周报（无退出进度行） \| 每周一 1 条`；line 39 `NORMAL \| [P1] 📅 月报（含 21 观测日趋势 sparkline） \| 每月首个交易日 1 条；与周一撞日只发月报` | PASS |
| F1 | review | §5 排障速查首行改为「NORMAL/WATCH 态非周一（且 NORMAL 非月初）属正常静默；再查 logs/crisis-daily.out.log 是否 data not ready 或 already evaluated」 | line 75 逐字匹配：`NORMAL/WATCH 态非周一（且 NORMAL 非月初）属正常静默；再查 \`logs/crisis-daily.out.log\` 是否 \`data not ready\` 或 \`already evaluated\`` | PASS |
| N0 | manual | GOTOOLCHAIN=local go test ./... 全绿 | 验证者亲自实跑：全量 ok，grep 无 FAIL/panic/error（含 internal/crisis、cmd/atlas 全部包） | PASS |
| N1 | manual | code-simplifier 对三文件运行，应用或记录无需改动 | Leader 代跑（协议 v2）结论「无需改动」：三文件逐一评估，两处候选简化因防御守卫/字节不变量风险刻意保留（证据由 Leader 提供，此处引用） | PASS |

## 对齐完整性核验
- `grep -n 'summaryDue\|SummaryDue' docs/ops/crisis-monitor-notifications.md` → 无残留。
- §2 末尾「不加第 4 个 plist」行（line 47-48）：`summaryKind 在 daily eval 内判断——NORMAL 当月首个交易日发月报、其余周一发周报（撞日只发月报）；WATCH 周一发周报` —— 与表格 line 38/39 两行语义一致，无自相矛盾（已由 summaryDue 旧措辞改为 summaryKind 语义，d59bc74）。

## 判定：PASS（verified）

两条 review 标准文档措辞与设计 §6/实施计划 Task 4 一致；两条 manual 标准——全量测试验证者亲跑全绿、code-simplifier 结论无需改动（Leader 证据）；对齐完整（无 summaryDue 残留、§2 末尾行与表格无矛盾）。
