# 架构决策记录 — 落地核查优化轮

> 决策 AD-1~AD-7 继承自权威设计文档 §1（已确认），AD-8+ 为本轮执行期新增。

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| AD-1 | 本轮范围 | 5 项全包（FutuBroker 清理、测试隔离、文档治理、alert 接线、sqlite 持久化） | 一轮补齐"已建未接线/遗留"缺口 |
| AD-2 | ML sidecar | 不在本轮 | 独立子系统，体量远超其余项 |
| AD-3 | 交付组织 | 单 spec、3 波次、每波一 PR | 独立合并/回滚 |
| AD-4 | 告警去向 | 复用已注册 notifier | M3 原意，零新增基础设施 |
| AD-5 | 指标喂数 | Registry.Snapshot 桥接 + 2 个派生指标 | 一次投入，规则可引用任意指标 |
| AD-6 | 信号存储 | sqlite 新实现 + 配置可选 backend（默认 sqlite） | 接口不变，API/Web/router 零改动 |
| AD-7 | 测试隔离 | `//go:build integration` build tag | 保留活 API 探测，改动最小 |

## 执行期新增决策

### AD-8 三波串行执行（单工作树）
Wave2/3 逻辑互不依赖，理论可并行；但 serve.go / config.go / config.example.yaml 在两波均被触碰，且单工作树下分支切换成本高于并行收益。**串行执行三波**，波内任务并行（dag 就绪派发 + scope 互斥）。

### AD-9 validator / arcforge-write.sh 缺失 → 降级
仓库无 `validator/`、`.claude/hooks/` 无 `arcforge-write.sh`。降级：
- 任务图校验由 Leader 手工执行（DAG 无环、wave 序、scope 互斥/非空、context_from 闭合）；
- 状态写入直接原子写（临时文件 + mv），跨 agent 竞争用 `with-task-lock.sh` 临界区；
- teammate 边界声明改为"task JSON 写入必须走 with-task-lock.sh 临界区"。

### AD-10 wave 字段做调度批次，PR 归组用 `pr` 标签
CLAUDE.md 约束 `本任务.wave > max(依赖.wave)`，波内依赖（如 serve 装配依赖 snapshot）需拆子波。task JSON 中 `wave` 为调度批次（1~5），另加 `pr` 字段（1~3）标注 PR 归组：wave1=PR1{101,102,103}，wave2=PR2{201,202}，wave3=PR2{203}，wave4=PR3{301}，wave5=PR3{302}。

### AD-11 TASK-101 跨 2 个 Go 包（Realistic Scope 例外）
删 FutuConfig（internal/config）与删 futu case（cmd/atlas）互相耦合——broker.go 引用 `cfg.Broker.Futu`，拆开必现编译中断的中间态。合并为一个任务（4 文件、7 条 DoD，仍在阈值内），packages 声明两包以保证 scope 互斥。TASK-102 同理跨 3 个 crypto 子包 + Makefile：机械改动（测试搬家加 build tag），拆三份纯增开销。

### AD-13 Snapshot 双层聚合（裁决 reviewer C.1 矛盾）
独立 reviewer 发现：`httpRequestsTotal` 带 `method/path/status` label，纯"聚合求和"丢失 5xx 维度，`http_error_rate` 无法实现。裁决：Snapshot 基名仍聚合求和；**对带 `status` label 且值为 3 位数字的序列，额外产出 `<name>_2xx/_4xx/_5xx` 状态类聚合键**（按百位归类，跨其余 label 求和）；非数字 status 值（如 ok/error）不产额外键（YAGNI）。http_error_rate 由 runner 用 `atlas_http_requests_total_5xx` 与基名两键的增量计算。prometheus 仍封闭在 metrics 包内。

**AD-13a 修订（2026-07-03，TASK-201 实施发现）**：现有 `RecordRequest` 经 `statusToString` 把 status label 存为 `"2xx"~"5xx"` 类字符串（metrics.go:185），非 3 位数字——按原规则 `_5xx` 键永不产出。修订：状态类键识别扩展为 **3 位数字或 `^[1-5]xx$` 类字符串**，两者均归入对应 `<name>_<N>xx` 键；`RecordRequest` 行为不动（避免指标基数与既有观测语义变化）。

### AD-14 List 排序键与边界语义（裁决 reviewer C.2/B.4/B.7）
memory.List 现为插入序、无显式排序，"以内存为基准"无确定基准可循。裁决：排序语义定为 **`generated_at ASC, id ASC`**；memory.List 补显式稳定排序（ID 单调递增，与现插入序兼容，非破坏性）；`limit=0` = 不限制（不得直译 `LIMIT 0`）；offset 越界返回空；from/to 沿用 memory 现状**闭区间（含端点，include iff >=From && <=To）**
——AD-14a 更正（2026-07-03，TASK-301 实施发现）：原文"严格开区间"系 Leader 笔误，memory.matches 的 Before/After 排除的是端点之外，端点包含；GetByID 未命中统一返回 `core.ErrSymbolNotFound`（errors.Is 可判）。全部由契约测试钉死。NewSQLiteStore 对 path 父目录 MkdirAll（默认 data/ 不存在时首启可用），失败返回 error。

### AD-15 paper-only 校验收敛（裁决 reviewer B.1 编译阻断）
config.go:419-421 live 校验与 :445 WarnHardcodedSecrets 均引用 `Broker.Futu`，删 FutuConfig 会编译阻断（设计文档只点名 :355）。裁决：live 校验改为 **`Broker.Mode=="live"` 时直接报错 `live trading not supported (paper-only)`**（与"实盘链路定格 paper-only"决策一致）；WarnHardcodedSecrets 删除 `broker.futu.trade_password` 条目。

### AD-12 适配器位置约束
alert.Notifier ← notifier.Notifier 适配器不得让 internal/alert import internal/notifier（保持 alert 只见 map[string]float64 与自身接口的边界）。放 internal/notifier 侧或 cmd/atlas 由 Dev 实施时定，决策记入 TASK-202 discovery，TASK-203 经 context_from 读取。
