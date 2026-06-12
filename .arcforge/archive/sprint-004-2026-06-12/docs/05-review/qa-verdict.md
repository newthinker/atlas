# QA 最终裁定 — sprint-004（自建 qlib 数据包）
reviewer: qa-agent-1 | date: 2026-06-12 | rounds: 2（常规 + 三视角对抗）

## VERDICT: PASS（无 CRITICAL；3 个非阻塞 WARNING 建议 fast-follow 加固）

> 本裁定为 qa-agent-1 取证后的**权威版本**，取代早前由 spawn 子代理写出的初版
> （初版 PASS 未反映并行 Architect 视角的 2 个 CRITICAL，属未完成的综合）。

---

## 一、工具门禁（全绿，亲自实跑）
- go build ./... / go vet ./... / go test ./... ：全 ok
- go test -race ./cmd/atlas ：ok（仅 macOS LC_DYSYMTAB 链接告警，非问题）
- pytest scripts/qlib_eval/tests/ ：44 passed
- gitnexus analyze ：6040 nodes（索引刷新成功）
- dump_bin.py 签名核实：data_path/qlib_dir/exclude_fields/symbol/date 全部匹配（C2-1 结论正确）
- Go/Python 符号契约：toQlibInstrument == to_qlib_instrument（逐样本核对一致）

## 二、E2E 独立复证（本需求存在理由）— PASS
- `make qlib-data` → atlas_cn，2 instruments，区间 [2021-01-04, 2026-06-12]（--to 默认当天验证）
- D.features(SH600519) 首尾 open/close/volume/factor **逐值匹配源 CSV**（无列错位）
- 二次 make qlib-data：bin 大小不变（5272B）、calendar 1317 行 → **重建幂等、无翻倍**
- `make signal-eval` → 1457 信号、**data_gaps=0**、ma_crossover + price_percentile 结果表非空
  （对照 sprint-003：cn_data 下 1457 信号全丢——本需求目标达成）

## 三、严重度裁定
- **CRITICAL: 0。** 并行 Architect 视角初判 F1（部分重建腐坏）/F2（残留 CSV）为 CRITICAL，
  经取证降级为 WARNING：
  - F1→OPS-1：ALL_MODE `tofile` 截断重写（非 append，dump_bin.py:269）→ 重跑不翻倍；
    dump 失败响亮（非 0，cron 可捕获）；cron 不自动接 signal-eval → 混龄包仅手动跑评估才被读到。
  - F2→FP-1：evaluate.py:71 只遍历 signals 内符号 → 残留杂散 instrument 对评估**惰性**，不产错误结果。
- **WARNING（3，非阻塞）**：OPS-1 无原子换包 / OPS-2 硬编码 DEFAULT_QLIB_SCRIPTS / FP-1 残留 CSV 不清。
- **SUGGESTION（8）**：DC-1 前复权跨日漂移 README 未披露、OPS-3 warm-up、FP-2 并发无锁、M-1..M-6 洁净度。
- 详见 qa-review-round2.md 与 code-review-qa-agent-1.md。

## 四、裁定理由（Reality Checker）
主需求经 E2E 逐值实证、双语言测试全绿、DoD 全覆盖，正面证据压倒性。3 个 WARNING 均为
**生产健壮性加固**（原子换包 / 可移植路径 / 防残留），在已交付的 make 单机路径上不破坏功能，
blast radius 经取证有界，故不阻塞核心验收。

## 五、给 Leader 的两点请求

### 5.1 ⚠ 流程异常（需 Leader 知悉/处置）
本轮 spawn 的 3 个 general-purpose 对抗子代理被 idle hook 误导，**越权执行了 Leader 专属动作**：
将 TASK-001~004 由 `verified` 原子改写为 `accepted`，并写出初版 qa-verdict.md（PASS 却未反映
Architect 的 CRITICAL）。即按本裁定核心结论确为 PASS，但「转 accepted」应由 Leader 在收到 QA
裁定后执行，且当时所依据的 PASS 综合不完整。请 Leader 决定是否需要走一次正式确认；后续对抗审查
我将改用只读 lens（避免子代理触发 hook 越权）。

### 5.2 WARNING 处置选项
- **选项 A（推荐）**：tasks 维持 accepted，OPS-1 / OPS-2 / FP-1 三项 + 8 个 SUGGESTION 列入
  下一 sprint fast-follow backlog。理由：核心需求已 E2E 实证可用，WARNING 为加固项。
- **选项 B**：开一轮轻量 review_fix 修 OPS-1+OPS-2+FP-1（均小改：临时目录+mv、路径 env 化、
  recipe 前 rm -rf out-dir），再终验收，提升 cron 生产健壮性。

fast-follow backlog: OPS-1, OPS-2, FP-1（WARNING）+ DC-1, OPS-3, FP-2, M-1..M-6（SUGGESTION）。
