# TASK-003 验证报告 — export-signals CLI（白名单+warm-up+golden CSV+cobra+Makefile）

- **Verifier**: test-agent-2 (Reality Checker)
- **判定**: ✅ VERIFIED
- **Commit**: 024c195 `feat(cli): export-signals with whitelist, warm-up, golden CSV and Makefile target`
- **Package**: `./cmd/atlas`（coverage_minimum=35）
- **依赖**: TASK-001 已 verified（engine 盖戳 GeneratedAt==bar.Time 是 golden 复现前提，已满足）
- **验证时间**: 2026-06-12 sprint-003

## 实际运行证据
```
$ go test ./cmd/atlas/ -race -cover -count=1 -run 'Export|NewExportEngine' -v
PASS (9 个测试 + 3 子测试全 PASS)
$ go test ./cmd/atlas/ -cover -count=1
ok  github.com/newthinker/atlas/cmd/atlas  coverage: 56.1% of statements   # ≥ 35 门禁
$ go build -o /tmp/atlas-smoke ./cmd/atlas   # exit 0
$ /tmp/atlas-smoke export-signals --help     # 列出 --from/--out/--strategies/--symbols/--to 全部 flags
```
注：`-race` 链接期出现 `malformed LC_DYSYMTAB` 警告 = 已知 macOS/Go 工具链噪声，非测试失败，测试在 -race 下全部 PASS。

## 验证要点逐条（lead 指定 ①–⑥）
1. **golden CSV 逐字节真实**：`TestExportSignals_GoldenCSV` 的 want 串与 **plan T3 want 串逐字节一致**
   （plan L185-189 ≡ test L131-135）：7 列 header、`%.2f`（0.70/101.00）、metadata JSON 引号转义
   `"{""k"":1}"`、warm-up 信号被 from 过滤（2024-01-02 bar 落在 from=01-03 前被剔除，输出 4 行）。
   **非 fantasy**：flatStub 故意发 `GeneratedAt=1999-01-01`（test L68），引擎必须覆写为 bar.Time，
   否则全部信号 Before(from) 被过滤→输出仅 header→断言失败。该 golden 真正穿透 引擎盖戳 + warm-up 过滤 + CSV 编码。
2. **engine 注册全 5 策略 + 真实 CLI 路径 pe_band 走 requires fundamentals（H1 反审硬断言）**：
   - 源码 `newExportEngine()`（export_signals.go L85-95）注册 ma_crossover/price_percentile/pe_band/dividend_yield/pe_percentile（默认构造器）。
   - cobra 路径 `runExportSignals` L130 `strategies: newExportEngine()` —— **真实 CLI 用真引擎，非测试 stub 替身**（已读 cobra 代码确认）。
   - `TestExportSignals_PEBandViaCLIEngineRejected`（L189-207）用 **真实 `newExportEngine()`** 驱动 pe_band，断言 err 含 `requires fundamentals` 且**不含** `unknown strategy`。
   - `pe_band` 源码 `RequiredData().Fundamentals: true`（internal/strategy/pe_band/strategy.go:30）确认——拒绝路径真实可达。
3. **白名单动态判定（无硬编码策略名清单）**：`grep '"(ma_crossover|...)"' export_signals.go` → **零命中**；
   拒绝路径走 `offlineNames()`（动态 GetStrategyNames + 过滤 Fundamentals + 排序），错误消息含可用清单。
4. **未知策略 / 日期错误路径**：`TestExportSignals_UnknownStrategy`（unknown strategy + 列清单）；
   `TestExportSignals_DateErrors`（bad_from / bad_to / from_after_to 三子测试全 err）。
5. **SkippedBars 摘要**：`TestExportSignals_SkippedBarsSummary`（skipStub 在 close==102 返回错误，断言 errOut 含 "skip"）。
6. **go build + --help 冒烟**：亲自 build（exit 0）并运行 `--help`，5 个 flags 全列出；
   单测 `TestExportCommand_UsageListsAllFlags` 另从 UsageString 守护。

## Done Criteria 覆盖矩阵
| # | 完成标准 | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | golden CSV 逐字节（7列/%.2f/metadata 转义/warm-up 过滤） | test | TestExportSignals_GoldenCSV（与 plan T3 want 逐字节一致） | ✅ PASS |
| functional[1] | Fundamentals 动态拒绝（requires fundamentals + 清单，不硬编码） | test | TestExportSignals_RejectsFundamentalStrategies（断言 "requires fundamentals"+"flat_stub"） | ✅ PASS |
| functional[2] | 未知策略名报错 + 可用清单 | test | TestExportSignals_UnknownStrategy | ✅ PASS |
| functional[3] | engine 注册全部 5 策略（默认构造器） | test | TestNewExportEngine_RegistersAllFive（真实 newExportEngine） | ✅ PASS |
| functional[4] | 真实 CLI engine pe_band → requires fundamentals（非 unknown）[H1] | test | TestExportSignals_PEBandViaCLIEngineRejected（真引擎）+ cobra L130 + pe_band.Fundamentals=true | ✅ PASS |
| boundary[0a] | SkippedBars>0 → errOut 一行摘要 | test | TestExportSignals_SkippedBarsSummary | ✅ PASS |
| boundary[0b] | metadata 为 nil → 该列空串 | test | TestExportSignals_NilMetadataEmptyColumn（断言行尾逗号） | ✅ PASS |
| error_handling[0] | from/to 解析失败、from>to 明确错误 | test | TestExportSignals_DateErrors（3 子测试） | ✅ PASS |
| non_functional[0] | go build 后 --help 输出全部 flags（冒烟） | test | 亲自 build+--help（5 flags）+ TestExportCommand_UsageListsAllFlags | ✅ PASS |
| — | coverage_minimum ≥ 35 | test | 全包 56.1% | ✅ PASS |

## Fantasy Assertion 排查
- golden（functional[0]）穿透引擎盖戳：故意污染 GeneratedAt=1999 强制依赖真实覆写，非硬编码绕过。
- H1（functional[4]）用真实 `newExportEngine()` 而非 stub，且 cobra 真实路径同款引擎；pe_band.Fundamentals 源码确认。
- 错误测试（unknown vs requires fundamentals）走**不同分支**且各自断言专属错误子串，未共用代码路径掩盖。

## 非阻断观察（不影响判定）
- `TestExportSignals_DateErrors` 仅断言 `err != nil`，未断言具体错误文案（如 "end date must be after start date"）。
  三条路径均覆盖、DoD 措辞为「返回明确错误」，故 PASS；建议后续补错误子串断言以防回归到错误文案。

## 结论
压倒性证据齐备：源码忠实 plan T3+T4、golden 逐字节匹配 plan、H1 硬断言走真实引擎、白名单动态无硬编码、
覆盖率 56.1%≥35、build+--help 冒烟通过、既有测试零修改（commit 仅新增 2 文件 + Makefile target）。判定 **VERIFIED**。
