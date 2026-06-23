# 架构决策记录 — IC/IR 评估管线

## ADR-1：时序 IC（逐标的）而非横截面 IC
watchlist 仅十来个标的，横截面 IC 每日只有十来个点、噪声极大。时序 IC 在每个标的时间轴上累积
(score_t, 前向收益_t) 配对算相关，样本量足够。**理由**：设计文档 rev2 §1.2 钉死。

## ADR-2：next-open 前向收益对齐
score(t) 由 close(t) 算 → open_{t+1} 入场 → h 交易日后收盘出场。复用既有 `prices.align_entry`，
规避前视偏差。**理由**：与方向①事件研究腿口径一致，可比。

## ADR-3：重叠前向收益 → t-stat 虚高，必须并列非重叠旁证
horizon 5/20/60 的前向收益窗口重叠，相邻样本强相关，t-stat 偏乐观。每个 t_stat 并列一个
非重叠采样 t_stat（每 h 天取一点）作审慎旁证，报告显式告诫。**理由**：设计文档 §2.3。

## ADR-4：t_stat = ic*sqrt(n_periods)（小 IC 近似，钉死）
不用更复杂的 Newey-West，避免实现漂移与口径争议。**理由**：设计文档钉死。

## ADR-5：顶层禁止 import qlib，函数体内惰性导入
pytest 零 qlib 依赖。守门测试 `test_no_qlib_at_module_level` 机制锁死。**理由**：CI/测试可在无 qlib 环境跑。

## ADR-6：CSV 长格式为唯一跨语言契约
`date,symbol,score` 等于方向② ML sidecar 的输出格式 → IC 管线既验证 baseline，也直接验收未来 ML 信号。

## 降级决策（本 Sprint 特有）
- validator 缺失：Leader 手工校验 DAG/scope 不变量，在 plan.md 记录校验结论。
- write-hook 缺失：以 `.claude/hooks/with-task-lock.sh` 原子写 task JSON。
- config.language=go vs 实际 Python：放弃 Go coverage hook，验证统一用计划指定 pytest 命令；
  覆盖率以「done_criteria 逐条 test 覆盖」替代数值门槛（计划已内建 RED→GREEN 测试）。
