# QA Code Review — Sprint 017（落地核查优化轮）

> 审查者: qa-agent-1
> 日期: 2026-07-03
> 分支: feature/audit-optimization-wave3-sqlite（master..HEAD 8 commits / 3 stacked PR）
> 方法: 两轮审查（第一轮常规 + 第二轮纯 Claude 跨视角对抗，codex/gemini 不可用降级）
> 实证: go build ./...（0）、go vet ./...（0）、go test ./...（全绿）、go test -race 关键包（clean）
> severity_threshold = warning

## Verdict: NEEDS WORK（无确认 CRITICAL，但 alerting 交付路径存在应修 WARNING）

无我能确认的 CRITICAL。对抗轮提出的 1 个 CRITICAL（telegram SendText parse_mode）
经复核降级为 WARNING（理由见 W1：默认示例可交付、运营者可控、无崩溃/数据丢失/安全破口）。
但该 WARNING 位于告警系统的交付路径，与 evaluator 吞错叠加构成「告警静默丢失」链路，
按 QA 心智模型（WARNING 未解不予 PASS）判定 NEEDS WORK，建议 Leader 至少对 W1 走一次 fix。

---

## 一、已实证通过项（PASS 证据）

| 维度 | 结论 | 证据 |
|---|---|---|
| 构建/静态 | 通过 | `go build ./...`=0；`go vet ./...`=0；`go build -tags integration ./...crypto/...`=0 |
| 测试 | 全绿 | `go test ./...` 无 fail；`go test -race ./cmd/atlas ./internal/{alert,storage/signal,metrics,notifier}/...` clean |
| 契约双实现 | 真钉死 | store_contract_test.go:34-40 `storeFactories` 参数化 memory+sqlite，全部 t.Run 双跑；含 Persistence/OpenError/ConcurrentSaveList |
| metric 名匹配 | 正确 | 指标注册名 `http_requests_total`（无 atlas_ 前缀，metrics.go:41）；runner 查 `http_requests_total_5xx`/`http_requests_total`（alert_runner.go:37-38）一致 |
| status 分类键 | 正确 | statusToString 输出 "2xx".."5xx"（metrics.go:185-197）匹配 snapshot 正则 `^([1-5]xx\|[1-9][0-9]{2})$`（snapshot.go:15），AD-13a 闭环 |
| 时间往返 | 正确 | timeLayout 定宽纳秒写入 vs RFC3339Nano 读取，实证 UTC/亚秒/跨时区 Equal=true，定宽字典序=时序（sqlite.go:20-23） |
| http_error_rate delta | 正确 | counter reset 负 delta clamp 0；dTotal<=0 返回 0 免除零；首快照无基线返 produced=false（alert_runner.go:36-56） |
| 无共享状态泄漏 | 正确 | evaluateOnce 每周期 snapshot() 返回新 map，派生键仅写本地 map，SetMetrics 整体替换引用（alert_runner.go:90-108 / evaluator.go:44-48） |
| 并发安全 | 正确 | Evaluator.mu 保护 metrics/pending/lastFired；runner 是唯一 SetMetrics/EvaluateAll 调用者（单 goroutine）；race clean |
| 老配置兼容 | 通过 | TestLoad_LegacyFutuSection_Ignored（config_test.go:436）验证 futu: 段被 viper 静默忽略；缺 storage.signals 走 sqlite 缺省 |
| paper-only 收敛 | 正确 | Broker.Mode=="live" 直接报错（config.go:442），不破坏 paper 流程；WarnHardcodedSecrets 删 futu 条目无遗漏 |
| 测试隔离无回归 | 通过 | 确定性单测（Name/ToInterval/SymbolToID 等）留原文件默认覆盖；仅 *_Integration 移到 //go:build integration；Makefile test-integration 就位 |
| 范围纪律 | 通过 | 对照 spec §5 明确不做清单，无 retention/迁移/track_record 越界；packages 无越权 |

## 二、第一轮常规审查发现

见下方统一问题清单（第一轮 + 对抗轮合并，标注来源）。

## 三、第二轮跨视角对抗审查

四视角（安全 / API 消费者 / 并发 / 运维）经两个独立 context 的只读 reviewer 复审。

### 推翻第一轮/对抗轮初判的记录
- 对抗轮初判 telegram SendText 为 **CRITICAL** → 复核**降级 WARNING**（W1，理由见下）。
- 对抗轮提出 timeLayout vs RFC3339Nano 潜在 CRITICAL → 实证**判定 NON-ISSUE**（往返 Equal=true）。
- 对抗轮提 memory maxSize vs sqlite 无界「容量语义不一致」WARNING → **降级 SUGGESTION/by-design**
  （spec §5 明确不做 retention，sqlite 无界是设计意图，契约不应强求一致）。
- 并发轮自评 alert goroutine 无 WaitGroup「WARNING」→ 其自身论证 benign，**归入 SUGGESTION**。

### 维持的结论
- 契约测试真参数化双实现（第一轮结论，对抗轮无异议）。
- http_error_rate delta/reset/divzero 正确（两轮一致）。
- SQL 全参数化、无注入（两轮一致，见 W-sec 说明）。

---

## 四、统一问题清单（含修复建议）

### WARNING

**[WARNING] W1 — telegram.go:99-104 / 287-294（TASK-202）SendText 声称逐字发送却带 parse_mode=Markdown**
- 证据: `SendText` 注释「plain-text, no Markdown escaping」，但 `sendRaw` 硬编码
  `parse_mode:"Markdown"`（telegram.go:293）。告警文本 `[SEVERITY] Name: Message`
  （rules.go:63，Name/Message 为运营者 config 自由填写）含不成对 `_ * [ \`` 元字符时
  Telegram 返回 400，告警送不达。示例规则名 `high_error_rate` 含成对下划线侥幸可发，
  但奇数下划线/方括号/反引号即失败。
- 注入面评估: 文本来源是运营者 config（非外部用户输入），无远程注入；风险是**静默丢告警**，非越权。
- 降级 CRITICAL→WARNING 理由: 默认示例可交付、运营者可控、无崩溃/数据丢失/安全破口；但位于
  告警交付路径，对告警系统而言「特定规则文本悄悄丢失」不可接受。
- 建议: 告警路径 `parse_mode` 置空（纯文本发送），或为 SendText 单开无 parse_mode 的 sendRaw 变体。

**[WARNING] W2 — internal/alert/evaluator.go:95-100 Notify 返回值被吞且失败仍进冷却（pre-existing，被本轮接线激活）**
- 证据: `for _, n := range e.notifiers { n.Notify(msg) }` 丢弃 error，随后 `e.lastFired[rule.Name]=now`
  无条件进 5min 冷却。与 W1 叠加 → 400 告警彻底静默丢失，且冷却期内不再重试。
- 范围说明: evaluator.go **不在本轮 diff**（本轮只做装配），但本轮首次实例化该评估器使此休眠缺陷上线。
- 建议: Notify 失败记 Warn 并**不写 lastFired**（失败不进冷却，下周期重试）。属 pre-existing，Leader 可决定本轮修或转 backlog。

**[WARNING] W3 — sqlite.go:84-93 vs memory.go:38 metadata 经 JSON 往返类型强制（int→float64），契约未钉死**
- 证据: sqlite Save 走 `json.Marshal`（sqlite.go:84）、scan 走 `json.Unmarshal`（sqlite.go:209），
  数字统一变 float64、嵌套 map 类型改变；memory 保留原生 Go 类型。契约
  TestContract_MetadataRoundTrip 仅用 float64+string，未覆盖 int/bool/嵌套，分歧未被钉死。
- 现状影响: 当前消费者（telegram renderTable 读 `["name"].(string)`/`["pe_percentile_display"].(float64)`）
  只用 string/float64，**不受影响**；分歧为**潜在**回归。
- 建议: 契约补 int/bool/嵌套用例明确 JSON 值语义，或让 memory 也做一次 JSON 归一化使两实现真等价。

### SUGGESTION

- **[SUGGESTION] S1 — sqlite.go:57-58 未设 SetMaxOpenConns**：WAL 单写下并发写正确性押在
  busy_timeout(5000)。低频信号写场景 5s 足够（race 测试通过），但突发批量写可能耗尽超时返
  SQLITE_BUSY 使 Save 冒泡 error。建议 `db.SetMaxOpenConns(1)` 序列化写者，机制上消除 BUSY。
- **[SUGGESTION] S2 — serve.go 关闭无 WaitGroup 排空 alert goroutine**：appCancel 与 closeSignalStore
  的 defer 顺序正确（LIFO：appCancel 先于 closeSignalStore），且 count 闭包捕获 appCtx 会先取消在途查询，
  database/sql 对已关闭池返回 error 而非 panic，evaluateOnce 记 Warn 后继续 → **benign**。建议加 sync.WaitGroup 让 shutdown 确定性排空。
- **[SUGGESTION] S3 — 运维文档**：默认 `data/signals.db` 为相对 CWD 路径，容器/只读 FS 易踩坑；
  WAL 附带 `-wal`/`-shm` 文件，备份/卷挂载需覆盖三文件；Close() 未做 WAL checkpoint。建议 runbook 注明。
- **[SUGGESTION] S4 — sqlite.go:57 DSN 字符串拼接 path 未做 URI 转义**（config 可控，低危）；错误信息含 path（非敏感，会入日志）。
- **[SUGGESTION] S5 — snapshot.go:15 正则 `[1-9][0-9]{2}` 宽至 600-999**：越界状态码会产 `_6xx`.._9xx 死键，无害（不被引用）。test-agent 定性正确。
- **[SUGGESTION] S6 — serve.go buildSignalStore 冗余 path 缺省兜底**：config.Load 已归一化 path，此处二次兜底属防御式，无害。test-agent 定性正确。
- **[SUGGESTION] S7 — 契约缺跨时区/亚秒/单调性用例**：实现已实证正确，仅未钉死。test-agent 定性正确（已独立实证）。

---

## 五、给 Leader 的建议 fix_items

最小修复集（建议至少 W1）：
1. W1（TASK-202）: telegram 告警路径去掉 parse_mode=Markdown（纯文本发送）。
2. W2（pre-existing / TASK-203 接线激活）: evaluator Notify 失败记日志且不进冷却——Leader 决定本轮修或 backlog。
3. W3（TASK-301）: 契约补 metadata 非 float64 用例，或 memory 侧 JSON 归一化。

S1-S7 为技术债/加固/文档，建议转 backlog，不阻断合并。
