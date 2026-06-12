# 架构决策记录 — sprint-004

> 产品/技术决策已在 spec rev4 定稿（方案 A 同源原则、atlas_cn 独立目录、全量重建+系统 cron、
> CLI/核心分层、--data_path 等钉死口径），此处仅记编排决策。

## ADR-S4-1: 001 与 003 并行、002 跨 scope 居中、004 收口

plan 的 T1/T2 同在 cmd/atlas，T2/T3/T4 都碰 scripts/qlib_eval——唯一可并行对是 T1×T3。
任务图 001∥003 → 002（跨双 scope，等两者 verified）→ 004。团队 dev×2 + test×2 即满配。

## ADR-S4-2: TASK-002 声明双 packages

T2 同时改 cmd/atlas（cobra）与 scripts/qlib_eval/tests/test_makefile.py（泛化 helper + 新断言），
按 sprint-001 TASK-003 多包先例声明双 scope；hook 对 Go 部分跑覆盖率（35 基线沿用）、
对 Python 部分跑 pytest——2e 分流天然支持混合 scope。

## ADR-S4-3: e2e 必跑（升级 sprint-003 的 ADR-S3-4）

数据包、采集链路、venv 在 sprint-003 验收中全部实证可用，「降级为可选」的前提消失；
且本需求的存在理由就是让 signal-eval 默认区间出非空结果——e2e 写进 TASK-004 functional DoD。
