# 架构决策记录 — sprint-003

> 产品/技术决策已在 design rev3 定稿（CSV 唯一契约、引擎机制性盖戳、动态白名单、价格源可注入、事件研究口径），此处仅记 arcforge 编排决策。

## ADR-S3-1: Python 任务单 scope 强制串行

scripts/qlib_eval 作为单一 packages scope，T5-T9 派生的 4 个任务依赖链串行（004→005→006→007→008）。不按文件拆细 scope——plan 依赖本就递进（symbols→prices→event_study→report→e2e），细拆无并行收益且破坏 scope 模型。Go 侧 3 任务与 Python 链并行补偿吞吐。

## ADR-S3-2: hook 跨语言分流而非双门禁体系

task-completed.sh 增加 2e 分流：scope 含 scripts/qlib_eval → pytest 门禁（失败阻断）；无 .go 的 scope 不进 Go 覆盖率门禁。Python 覆盖率不设机器阈值（pytest-cov 未引入，避免新依赖），由 Test Agent 按 DoD 逐条核对补偿——DoD 每条都对应具体测试，矩阵核对比百分比更严。

## ADR-S3-3: Leader 预置统一 venv

默认 python3（pyenv 3.10.12）dyld 损坏。Leader 在 Sprint 启动时创建 `scripts/qlib_eval/.venv`（Python 3.11.2 + pandas 3.0.3 + pytest 9.0.3）并写入设计文档，所有角色统一使用——避免 4 个 Python 任务各自踩同一环境坑（澄清风暴）。

## ADR-S3-4: qlib 数据包真实运行降级为可选验收

`make signal-eval` 端到端需 ~/.qlib/qlib_data/cn_data。数据包未确认存在且下载受网络限制；plan §5 本就设计了缺数据指引（exit 1 + 下载命令）。验收口径：pytest 全绿 + 指引路径有测试 = PASS；数据就位时的真实报告作为加分项人工验证（写入 final-report 遗留区）。
