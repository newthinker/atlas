# 需求 ↔ DoD 双向追溯矩阵

> 需求来源: `docs/superpowers/specs/2026-07-02-audit-optimization-round-design.md`
> 生成: 2026-07-02，Leader 机器检查结论见文末。

## 正向：需求 → 任务/DoD

| 需求条目（设计文档章节） | 任务 | 覆盖 DoD |
|---|---|---|
| §2a broker.go 删 futu case，语义转"不支持" | TASK-101 | functional#1, error_handling#1 |
| §2a 删 FutuConfig / BrokerConfig.Futu | TASK-101 | functional#2 |
| §2a Provider 缺省 futu→mock | TASK-101 | functional#3 |
| §2a 老配置 futu: 段静默忽略 | TASK-101 | boundary#1 |
| §2a config.example.yaml 删 futu 段；m4 文档撤回标注 | TASK-101 | non_functional#1 |
| §2b 三包 Integration 测试拆 build tag 文件 | TASK-102 | functional#1, boundary#1, boundary#2 |
| §2b Makefile test-integration target | TASK-102 | functional#2 |
| §2b go test ./... 不联网确定性全绿 | TASK-102 | boundary#1, non_functional#2 |
| §2c runbook 补 analysis LaunchAgent | TASK-103 | functional#1 |
| §2c 架构文档 superseded 注记（六项） | TASK-103 | functional#2 |
| §2c crypto 设计三处"未实现"标注（不补实现） | TASK-103 | functional#3, non_functional#1 |
| §3a Registry.Snapshot()（gauge/counter/histogram/多 label） | TASK-201 | functional#1-3, boundary#1 |
| §3a prometheus 封闭在 metrics 包内 | TASK-201 | non_functional#1 |
| §3b http_error_rate（delta 增量） | TASK-203 | functional#2, boundary#2 |
| §3b signals_24h（Count from=now-24h） | TASK-203 | functional#3 |
| §3b 裁剪 up / analysis_failures_1h | TASK-203 | description 边界（不做项） |
| §3c 适配器 SendText 直发 / 系统信号回退 | TASK-202 | functional#1-2 |
| §3c telegram 公开 SendText（走 sendRaw） | TASK-202 | functional#3 |
| §3c email/webhook 零改动 | TASK-202 | non_functional#1 |
| §3c serve 装配：Enabled 时循环评估、随 ctx 优雅退出 | TASK-203 | functional#1, boundary#1, error_handling#1 |
| §3c config.AlertRule ↔ alert.Rule 映射 | TASK-203 | functional#4 |
| §3c config.example.yaml 示例规则 | TASK-203 | non_functional#1 |
| §3d 四类测试（snapshot/delta/适配器/装配 ObservedLogs） | TASK-201/202/203 | 各自测试类 DoD |
| §4a NewSQLiteStore（v1.38.2/WAL/busy_timeout） | TASK-301 | functional#1, non_functional#1 |
| §4a 单表 schema + 两索引，无迁移框架 | TASK-301 | functional#1 |
| §4a ID 方案沿用；ListFilter 直译；语义以内存为基准 | TASK-301 | functional#2 |
| §4b storage.signals 节（backend 默认 sqlite / path 默认 data/signals.db） | TASK-302 | functional#1, boundary#1 |
| §4b serve 按配置构造（替换固定 memory） | TASK-302 | functional#2 |
| §4b sqlite 打开失败快速失败不降级 | TASK-302 | error_handling#1 |
| §4b config.example.yaml 说明两 backend + 行为变化 | TASK-302 | non_functional#1 |
| §4c 契约测试（双实现同套用例，t.TempDir） | TASK-301 | functional#4, boundary#1 |
| §6 每 PR go test 全绿 | 全部任务 | 各 non_functional |
| §6 提交前 gitnexus_detect_changes | Leader 流程 | 波次提交门禁（plan.md） |
| §5 明确不做清单 | 全部任务 | description 中显式声明边界 |

## 反向：DoD → 需求（凭空 DoD 检查）

逐条核对 8 个任务全部 DoD，均可回溯到 §2a/2b/2c/3a/3b/3c/3d/4a/4b/4c/§6/§7 条目。
补充性 DoD 说明（非凭空，属需求的可测试化引申）：
- TASK-101 error_handling#1（错误信息含 provider 名）← §2a "unknown broker provider: futu" 语义。
- TASK-201 error_handling#1（Gather 出错不 panic）← §3a 桥接健壮性引申（Dev 定策略记 discovery）。
- TASK-202 boundary#1（不吞错）← §3c 适配器语义引申。
- TASK-301 boundary#1（重开同 path 数据在）← §6 "重启仍能查到信号" 的存储层投影。
- TASK-302 error_handling#2（backend 非法值报错）← §4b backend 枚举的封闭性引申。

## 独立 reviewer 反审后的增补（2026-07-02，见 dod-review-independent.md）

| 增补条目 | 来源 | 任务/DoD |
|---|---|---|
| config.go:419/445 Futu 引用收敛（live→paper-only 报错、WarnHardcodedSecrets 清理） | reviewer B.1 + AD-15 | TASK-101 functional#2, error_handling#2 |
| Snapshot 状态类键 `_2xx/_4xx/_5xx` | reviewer C.1 + AD-13 | TASK-201 functional#4 |
| http_error_rate 取数改 `_5xx`+基名、负增量 clamp、Notify 失败不中断、停止断言 | reviewer B.2/B.5 + AD-13 | TASK-203 functional#2, error_handling#1 |
| 排序键 generated_at ASC,id ASC；limit=0/offset 越界/端点开闭/sentinel；父目录 MkdirAll | reviewer C.2/B.4/B.7 + AD-14 | TASK-301 functional#1-2, boundary#2 |

## 机器检查结论

- **孤儿需求**：0。§2~§4 全部交付条目均有任务与 DoD 覆盖；§5"明确不做"以任务边界声明覆盖。
- **凭空 DoD**：0。5 条引申型 DoD 均标注来源。
- 总 DoD 条数：任务最多 7 条（TASK-101/203），全部 ≤8 ✓。
