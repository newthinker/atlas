# 设计规格 · Sprint M2a（契约队列与信号快照）

设计本体在 spec `2026-09-05-hestia-m2a-contract-queue-design.md` 与需求文档逐步代码；此处只写 Arcforge 拆分层需要的东西：数据流、接口交接、调度形态、门禁差异。

## 1. 数据流（改动后）

```
Ingest ─入口─▶ EnsureQueueDirs(cfg.Queue.Dir)  [pending/ processing/ done/ failed/]
ingestOne ─Save─▶ out
   ├─ Table==observations && Verdict!=Duplicate
   │     ├─ Revision ⇒ Store.PriorPublishedAt(period, type, published_at) → Supersedes
   │     ├─ BuildContract{Obs, Report, SourceURL=c.URL, IsRevision} → WriteContract(dir) → pending/<period>-<type>.json
   │     │     失败 ⇒ contractError：stage=contract，P2 不发，P1 照发，runs 记 ingested + Error 列（AD-4）
   │     └─ Evaluate(obs, cfg.Signals) → temp
   └─ renderP2(obs, out, temp)  ← 锚字段行之后加「信号 活化🔴 楼市🟢 消费⚪ 信贷🟡 · 温度 n/Known」
hestia contract emit ─▶ Store.Current(period, type) → PriorPublishedAt → BuildContract{Replay:true, Report{Passed:true}} → --stdout | WriteContract
```

## 2. 接口交接（写进各自 discovery `interfaces_exposed`）

| 生产者 | 接口 | 消费者 |
|---|---|---|
| 001 | `QueueCfg{Dir}`、`Signals`（8 字段，`mapstructure`+`json` tag，`TempScale` `json:"-"`）、`DefaultSignals()`、`Config.Queue`/`Config.Signals` | 002（`Signals`）、003（`Config`）、005/006（`cfg.Queue.Dir`） |
| 002 | `Signal` 四常量、`Temperature{Activation,Housing,Consumption,Credit,Score,Known}`、`Evaluate`、`monthlyAverage`/`monthsInPeriod`（非导出） | 005 |
| 003 | `Contract`（键序见需求）、`ContractInput{Obs,Report,IsRevision,Supersedes,SourceURL,Replay}`、`BuildContract(in, cfg Config)`、`(Contract).JSON()`/`FileName()`、`Store.Current`、`Store.PriorPublishedAt`、`articleURL` | 004（`Contract`）、005、006 |
| 004 | `EnsureQueueDirs(dir)`、`WriteContract(dir, c) (path, error)` | 005、006 |
| 005 | `renderP2(obs, out, temp)` 三参；`contractError`/`isContractError`；`Ingest` 空 `Queue.Dir` 报错 | 007（§A5/§A6 措辞） |
| 006 | `atlas hestia contract emit --period YYYY-MM [--period-type monthly] [--stdout]` | 007（回放样本） |

## 3. 调度形态（`scheduling: dag`）

```
001 ──▶ 002 ──▶ 003 ──▶ 004 ──▶ 005 ──▶ 007
                  └────────┴──▶ 006 ──┘
```
wave：001=1、002=2、003=3、004=4、005=5、006=5、007=6。`writes` 按文件声明；005（`internal/hestia` 四文件）与 006（`cmd/atlas` 两文件）互不相交可并行。

## 4. 门禁差异

- 001 与 006 含 `cmd/atlas` ⇒ `coverage_floor: 75`（门禁对 packages 合并取 total，`cmd/atlas` 基线 76.4 < dev_minimum 80）；DoD 仍要求各包不低于基线。
- 007 docs-only：`packages`/`writes` = `internal/hestia/CONTRACTS.md`，全部 DoD 条目 `verify_by: review|manual`。
- 每任务 `dev_done` 前：merge 进 master → 在 master 重采数字（AD-9）。

## 5. 不做

- 不给 `Signals` 加第五信号；`corp_mlt_short_expand` 只快照。
- 不改 `.gitignore`：launchd `WorkingDirectory` 是 `/Users/zuowei/workspace/runtime/atlas`，`queue/hestia` 不会落进仓库。
- 不 deploy。
