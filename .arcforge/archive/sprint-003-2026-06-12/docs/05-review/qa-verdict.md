# QA 终审裁定 — sprint-003 qlib-eval pipeline

**VERDICT: PASS**（无 CRITICAL、无 high-severity 共识发现；含 2 个 WARNING 级非阻塞建议修复）

Reality Checker 立场：默认 NEEDS WORK，仅在逐项有压倒性证据时 PASS。本 Sprint 的核心
风险面（跨语言 CSV 契约、warm-up 边界、事件研究对齐数学）我均以**实测 round-trip + 合成数据
手算用例 + 逐字节 golden** 验证，证据充分，故核心判 PASS。

## 工具门禁（全绿）
| 项 | 结果 |
|---|---|
| go build ./... | ✅ |
| go vet (backtest, ma_crossover, cmd/atlas) | ✅ |
| go test + -race ×3 包 | ✅（race 仅 ld 链接告警，非数据竞争） |
| pytest (.venv) | ✅ 28 passed |
| npx gitnexus analyze | ✅（MCP 工具 QA 侧不可用 → vet+test+race+人工 diff 替代，已记录） |
| CSV 跨语言 round-trip 实测 | ✅ `{"k":1}`/%.2f/日期/转义逐字段一致 |

## 验收对照（plan §6，6 条逐条）
1. 引擎盖戳任意策略 date=bar 时间 — ✅ backtester.go:81 + TestRun_Stamps…
2. `--strategies pe_band` 显式报错列清单 — ✅ TestExportSignals_PEBandViaCLIEngineRejected
3. warm-up：from 后首日即可有信号 + from 过滤 — ✅ golden 边界用例
4. 事件研究口径全有合成单测（入场/顺延丢弃/超额/sell 规避/置信度桶）— ✅ 8 例
5. pytest 零 qlib 依赖 + 缺数据明确指引 — ✅ test_no_qlib_at_module_level / test_main_exits…
6. make signal-eval 数据就位产出报告 — ✅ 链路与 venv python 经 test_makefile 钉死
   （真实数据包未在本环境，端到端 markdown 产出未实跑，依赖链与单测充分）

## 发现汇总
| # | 级别 | 文件:行号 | 描述 | 建议修复 |
|---|---|---|---|---|
| 1 | **WARNING** | evaluate.py:122-123 | 空信号文件(仅表头)真实运行 `NaTType strftime` 崩溃（实测复现） | 入口短路：信号为空时写"无信号"报告并 exit 0 |
| 2 | **WARNING** | evaluate.py:56 | `source.benchmark()` 无 try/except，基准缺失整跑崩溃（与逐 symbol 降级不对称） | 包裹 try/except，给"基准缺失"明确提示 |
| 3 | SUGGESTION | event_study.py:83,90 | 个股 open→close vs 基准 close→close，入场日内偏置 | README 口径注明"基准为收盘到收盘" |
| 4 | SUGGESTION | event_study.py:86-90 | exit_date 超基准末日 `_last_le` 取末行，无 gap 标记 | 终点侧加越界标记（与起点 <0 防御对称） |
| 5 | SUGGESTION | export_signals.go:244 | confidence %.2f 舍入影响桶边界(0.799→0.80) | 记录备查 |
| 6 | SUGGESTION | export_signals.go:198 | backtest.New 在循环内构造 | 上提循环外 |
| 7 | SUGGESTION | report.py:20 | read_signals 未用 utf-8-sig，Excel BOM 失败(响亮报错) | 改 encoding="utf-8-sig" |

## 裁定理由
- 无 CRITICAL；2 个 WARNING 均为**真实运行降级路径在退化态下崩溃**（空信号、基准缺失），
  不污染正常路径的正确结果，三视角对其严重度判断一致（非 high、无分歧）。
- 据 Arcforge 裁定标度：无 high-severity 共识 → 非 REJECT；视角间无分歧 → 非 CONTESTED → **PASS**。
- **建议**（非阻塞）：Leader 酌情安排一次轻量修复 #1、#2（各 1 处入口防护，约 10 行），
  以加固真实运行的运维健壮性，再做最终验收；若优先交付，可作为 fast-follow。

— qa-agent-1
