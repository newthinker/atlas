# Changelog · Sprint M2a（Hestia 契约队列与信号快照）

锚 `d27791c` → `589835a`（master）

## Added
- `internal/hestia/config.go`：`Queue{Dir}`、`Signals`（8 键，`json` tag）、`DefaultSignals`、四条校验（001）
- `configs/hestia.yaml`：`queue` 与 `signals` 段，`config_version` → `2026-09-05`（001）
- `internal/hestia/signals.go`：`Signal`/`Temperature`/`Evaluate`，月均 `_mom` 优先 → monthly ÷ MM → 3/6/9/12（002）
- `internal/hestia/contract.go`：`Contract`（固定键序）、`BuildContract`、`JSON()`、`FileName()`（带 period_type）、`pbocArticleURL`（003）
- `internal/hestia/store.go`：只读 `Current`、`PriorPublishedAt`（003）
- `internal/hestia/queue.go`：`EnsureQueueDirs`（四目录）、`WriteContract`（只写 `pending/`、原子覆盖）（004）
- `internal/hestia/ingest.go`：入口建齐队列 + 空 `queue.dir` 报错；`Save` 后仅 New/Revision 写契约再发 P2；`contractError`（写失败 ⇒ ingested + stage=contract + P1 照发、P2 不发）（005）
- `internal/hestia/notify.go`：P2 信号行 `信号 活化🔴 楼市🟢 消费⚪ 信贷🟡 · 温度 n/Known`（005）
- `cmd/atlas/hestia.go`：`atlas hestia contract emit --period --period-type --stdout`（006）
- `internal/hestia/ingest.go`：契约前经 `Store.Current` 回读 `IngestedAt`，实时契约 `extracted_at` 不再为空（005 返工 2，QA C1）
- `internal/hestia/CONTRACTS.md`：`## Sprint M2a` §A（A1–A10）/ §B / §C（007 + 005 返工 2）
- 守卫登记：AST 25→34、reflect 12→14；新增测试 48

## Changed
- `cmd/atlas/hestia_test.go` 两处内联 yaml 补 `queue.dir`（AD-5）
- `ingest_test.go` 的 `outPeriods` helper 跳过契约打印行（005 申报）

## Not changed
- `parse.go` / `extract.go` / `validate.go` / `fields.go`、`Save` 函数体、`go.mod`/`go.sum`
