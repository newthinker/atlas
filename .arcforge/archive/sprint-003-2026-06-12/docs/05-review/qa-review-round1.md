# QA Round 1 — 常规 Code Review (sprint-003 qlib-eval pipeline)

审查范围：`git log 945ced8..HEAD`（剔除 48965b7 文档）。心智模型 Reality Checker。
工具门禁：`go build ./...` ✅ / `go vet` ✅ / `go test -race`（backtest, ma_crossover, cmd/atlas）✅ /
pytest `28 passed`（`scripts/qlib_eval/.venv/bin/python`）✅ / `npx gitnexus analyze` ✅。
> gitnexus MCP 工具在本 QA session 不可用 → 按规范用 `vet + test + -race + 人工 diff` 替代并记录。

---

## 1. 跨语言 CSV 契约（本 Sprint 最大风险面）— PASS（实测）

证据（不是"测试通过了"，是我手工拉通两端）：
- Go 写出端 `export_signals.go:230-248 signalRow`：七列 = `csvHeader`(:77)；`confidence/price` 用 `%.2f`；
  `metadata` 用 `json.Marshal`（nil→空串，:231-238）；日期 `GeneratedAt.Format(dateLayout)`，
  `dateLayout = "2006-01-02"`（backtest.go:23）。
- Python 读取端 `report.py:12-47 read_signals`：`csv.reader` 标准解析，表头严格等值校验(:26)，
  列数=7 校验(:34)，`date→pd.Timestamp`、`confidence/price→float`，metadata **保留原串**(:39-45)。
- **实测 round-trip**：喂入 Go `encoding/csv` 风格行
  `600519.SH,2024-01-03,flat,buy,0.70,101.00,"{""k"":1}"` →
  解析得 `metadata='{"k":1}'`、`date=Timestamp('2024-01-03')`、`conf=0.7`。逐字段一致。
- **键序确定性**：Go `encoding/json` 对 map 键按字母序输出，多键 metadata 亦确定，golden 可复现。
- **金标用例**`TestExportSignals_GoldenCSV`(export_signals_test.go:115) 逐字节钉死含转义的整段 CSV。

裁定 PASS：跨语言契约两端格式、转义、日期、空值口径一致，且有逐字节 golden + 实测佐证。

## 2. warm-up 边界与 from 过滤（off-by-one）— PASS

- 拉数起点前移：`warmupStart = from.AddDate(0,0,-(maxBars*365/252+30))`（export_signals.go:188），
  与 `app.historyWindowDays` 同口径。
- 过滤：`if sig.GeneratedAt.Before(from) { continue }`（:204）→ 保留 `>= from`，无 off-by-one。
- golden 用例 `From:"2024-01-03"` 而首 bar 在 01-02，断言 01-02 信号被滤、01-03 起保留，
  且周末 01-06/07 自然缺席（makeBars 仅工作日）。边界被测试夹死。

## 3. 引擎盖戳 + SkippedBars — PASS

- `backtester.go:81` 统一 `sig.GeneratedAt = ohlcv[i].Time`（覆写策略自报值，机制性保证）。
- `backtester.go:72-74` Analyze 出错 `skipped++; continue`；`:100` 写入 `Result.SkippedBars`。
- `ma_crossover/strategy.go` 两处 `time.Now()→ctx.Now`（diff 已核），去掉 `time` import。
- 用例 `TestRun_StampsGeneratedAtAndCountsSkips` 用故意写 1999 的 stub + failOn 断言盖戳与计数。

## 4. 白名单拒绝（基本面策略）— PASS

- `executeExport` 先全量校验后拉数(:170-186)：未知策略→`unknown strategy`(:175)，
  `RequiredData().Fundamentals`→`requires fundamentals`(:179)，可用清单动态来自 `offlineNames`。
- CLI 引擎 `newExportEngine` 注册全部 5 策略(:85-95)，使 `pe_band` 落到"fundamentals 拒绝"而非
  "unknown"分支——回归守门 `TestExportSignals_PEBandViaCLIEngineRejected`(test:189) 钉死。

## 5. 事件研究数学 — PASS

- 基准对齐 `_last_le`(event_study.py:43-49)：`searchsorted(side="right")-1`，**负索引显式 <0 防御**
  (:69,:87)——杜绝 `iloc[-1]` 静默取末行（事件研究最易错处）。
- horizon 用 positional `entry.index+h`，越界→None(:79-82)，不污染聚合。
- sell 规避口径 `-(ret-bench_ret)`(:92)；累积置信度桶 `>=bucket`(:118-119)；
  胜率 = `excess>0` 占比(:147)。全部有合成数据手算用例（test_event_study.py 8 例全绿）。

## 6. 与 plan rev4 逐 Task 偏差 — 无实质偏差

Task1-9 实现与施工图一致；文末 6 条验收对照逐条满足（见 qa-verdict.md 附表）。
唯一良性增量：测试比计划多覆盖了 nil-metadata、SkippedBars 摘要、unknown-strategy 等边界。

---

## Round-1 发现（分级）

- **[WARNING] evaluate.py:122-123** — 空信号文件（仅表头）真实运行崩溃。
  `signals["date"].min()` 在空 DataFrame 为 `NaT`，`.strftime("%Y-%m-%d")` 抛
  `ValueError: NaTType does not support strftime`（已实测复现）。header-only CSV 是真实运维态
  （某区间无任何信号），应优雅处理（写"无信号"报告并 exit 0，或给明确提示）。
- **[SUGGESTION] event_study.py:83,90** — 个股收益 `close[entry+h]/open[entry]`（open→close）与
  基准 `close/close` 不对称，含入场日日内收益偏置。符合设计"次日开盘入场"取舍，建议在 README
  口径中明示"基准为收盘到收盘"。
- **[SUGGESTION] export_signals.go:244** — confidence `%.2f` 舍入可使 0.799 写成 0.80 落入 ≥0.8 桶。
  桶边界舍入伪影，影响可忽略，记录备查。
- **[SUGGESTION] export_signals.go:198** — `backtest.New(deps.provider)` 在 symbol×strategy 循环内
  反复构造；无害（廉价 struct），可上提。

Round-1 结论：核心路径 PASS（证据充分）；1 个 WARNING（空信号崩溃）建议修复。
