# Sprint 2026-09-27 — 银行股关键指标月度监控

> 状态：**QA 第 2 轮审查中**（7/7 verified @ 7234cd3）；QA 第 1 轮 REJECT → TASK-006 review_fix 已修复（FIX-1 bankExit(2) 生产路径无守卫；FIX-2 全失败摘要缺 H 股别名）。原：QA 审查中（7/7 verified；validator exit 0，transition-audit 0，unregistered-writer 0）。reviewer 19 条已处理；人类裁决 R14–R17（D1/D2/D15/D16 全取推荐）
> 需求：docs/superpowers/plans/2026-09-27-bank-indicator-monitor.md ；spec：docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md
> 起点：feature/bank-indicator-monitor @ b2f85462aa32f7766a07dba5938ea9c8d4440830 ；scheduling=dag

## 任务图
```
TASK-001 (w1) ──┬── TASK-002 (w2) ─────────────────────┐
                └── TASK-003 (w2) ── TASK-004 (w3) ── TASK-005 (w4) ──┴── TASK-006 (w5) ── TASK-007 (w6)
```

| 任务 | 标题 | wave | 依赖 | 状态 | owner | verifier |
|---|---|---|---|---|---|---|
| TASK-001 | 类型与配置加载 | 1 | — | **verified**（merge d647428，24/25 KILLED） | dev-bk-a | test-bk-a |
| TASK-002 | EMSource 数据源 | 2 | 001 | **verified**（merge 7f51cd4，26/27 KILLED） | dev-bk-a | test-bk-a |
| TASK-003 | 按指标回退的环比同比与预警 | 2 | 001 | **verified**（返工 1 次，merge 6d79247，29/29 KILLED） | dev-bk-b | test-bk-a |
| TASK-004 | A+H 去重与同期统计（众数统计期） | 3 | 003 | **verified**（merge d4d1d05；观察项 Q27 转入 TASK-005） | dev-bk-b | test-bk-a |
| TASK-005 | 渲染与分段（+golden，+Q27 守卫） | 4 | 004 | **verified**（返工 1 次，merge 6d83a0d，40/41 KILLED） | dev-bk-b | test-bk-a |
| TASK-006 | atlas bank report 命令（floor 78） | 5 | 002,005 | **verified**（QA r1 返工 merge 7234cd3；rework 2；26/27 KILLED） | dev-bk-b | test-bk-a |
| TASK-007 | 集成冒烟 + plist + 真实 dry-run | 6 | 006 | **verified**（merge 8fdd59b，沙箱内 dry-run，推送 0） | dev-bk-b | test-bk-a |

## 团队规划
dev × 2（仅 w2 有并行：TASK-002 ∥ TASK-003），test × 1。

## 人类执行项（不在自动流程内）
部署 runtime、真实推送一次 Telegram、`launchctl load` bank-monthly.plist。

## ~~QA 阶段待挂 fix_items~~ → **已于 13:55 并入 TASK-005 返工 r1 的 done_criteria（functional 末条），不再待挂**
- **TASK-005-F1**：头部计数不闭合——「覆盖 7 家（…3 家；未更新 2；失败 1）」漏计 Ahead。裁决：Ahead>0 时在「未更新」前插入「；领先 N」，N=0 不出现；用例断言分项和 = 覆盖家数。
- **TASK-005-F2**：统计标题 n=当期主体数，但某指标因回退值被排除后实际 N 更小，读者看不出。裁决：`Stat.N ≠ 当期主体数` 时该行指标名后加「（n=2）」，相等不加。
- 来源：dev-bk-b 在 5dcf6fd 交付时自报的两处观察（DoD 未覆盖）。原计划在本任务内修；**更正（13:41）：并非 dev 侧互等或消息未达——Leader 会话在 11:51 之后挂起约 1h50m（merge commit 提交时间 13:40 为证），dev 三次催办均属实**。改为先 merge 5dcf6fd（满足全部 7 条 DoD），修正经 QA 的 `verified → review_fix` 挂 fix_items 执行。

## 范围外既有问题（final-report 登记，本 sprint 不修）
- **cmd/atlas 整包 `-shuffle` 顺序依赖**：`TestBackfillLoadRequiresDBFlag` 在 717f14d 上 seed 1/2/3/5/9 失败，背对背基线 6d83a0d 上 seed 3/9 同样失败；`-skip Bank` 后失败 seed 集合不变 ⇒ 非本 sprint 引入。疑似 `hestia_test.go:1139` 传 `--db` 后未复位 cobra flag。任何用 `-shuffle` 的门禁都会随机假红。（test-bk-a，TASK-006 验证报告）
- **出处订正（TASK-006 返工时 dev-bk-b 指出）**：单 seed 的「go test -shuffle=on ok」写法出自 **TASK-004/005** 的 discovery（各 1 处，已 grep 核实），不是 TASK-006 首轮（该版本未入 git，「0 处」一说**未独立核实**，采信 dev）。验证报告与 Leader 转述均误指了出处。dev 已补跑 internal/bank seed 1–10 × count=3 全部 rc=0，原结论成立、原措辞不严谨；TASK-004/005 已 verified，discovery 不可改，以本条为准。
