# QA 终审（Round 3 修复复审）— sprint-003 qlib-eval pipeline

**VERDICT: PASS** ✅（所有此前 WARNING 已闭合，无新增问题；无 CRITICAL/CONTESTED/REJECT）

修复对象：commit `35c18c9` "fix(TASK-007): harden degraded paths"。
复审方法：亲自构造与我原始报告同场景的反例跑通，而非仅看测试绿（Reality Checker）。

---

## ① 两个崩溃反例是否真实闭合 — 是（亲自构造同场景验证）

### QA-W1 空信号文件崩溃 → 闭合 ✅
- 原始缺陷：header-only CSV → `signals["date"].min()`=NaT → `.strftime` 抛
  `ValueError: NaTType does not support strftime`（我此前已实测复现）。
- 修复（evaluate.py:146-153）：`main` 在 `read_signals` 后 `if signals.empty:` 短路，
  用 `_empty_stats()` 写一份"无信号"报告并 `return 0`，绝不触及 NaT.strftime。
- **我亲自复验**：构造仅表头的 signals.csv + 存在的 dummy qlib 目录跑 `main(...)` →
  `exit code: 0`，`reports/signal-eval-*.md` 已写出，**无 traceback**。RESULT A: PASS。

### QA-W2 基准缺失整跑崩溃 → 闭合 ✅
- 原始缺陷：`bench = source.benchmark()` 无 try/except，CSI300 缺失抛裸栈中断整跑，
  与逐 symbol 的 data_gaps 降级不对称。
- 修复（evaluate.py:55-58 + report.py:99-103）：`try/except` 包裹 benchmark()，
  失败降级为 `([], {**_empty_stats(), "benchmark_error": str(e)})`；render_report
  增「⚠ 基准 … 数据缺失，全部超额收益无法计算」节。
- **我亲自复验**：注入 benchmark() 抛 RuntimeError 的 fake source 跑 collect_outcomes →
  返回 `([], stats)` 含 `benchmark_error`，**未抛异常**；render_report 正确呈现告警。RESULT B: PASS。

## ② 修复未引入新问题 — 确认 ✅
- pytest（hook 同款 `.venv/bin/python -m pytest tests/ -q`）：**32 passed**（原 28 + 4 个
  精准回归：empty-signals/benchmark-failure/render-benchmark-error/utf8-bom，与我报告一一对应）。
- Go 全量回归：`go build ./...` ✅、`go vet ./...` ✅、`go test ./...` **FAIL/panic = 0**。
- 重构无副作用：抽出的 `_empty_stats/_meta/_write_report` 消除 drift，正常路径行为不变
  （金标/事件研究用例仍全绿）。

## ③ 顺手项落地确认
- **S7（Excel BOM）✅**：report.py:21 `open(..., encoding="utf-8-sig")`；我用 BOM 文件复验
  → 正确解析 1 行、metadata `{"k":1}` 完好。RESULT S7: PASS。
- **S3（基准口径文档化）✅**：README.md:85-86 明确"基准为收盘到收盘 vs 个股开盘到收盘，
  入场日存在日内偏置"，与 event_study 实现一致。

## 遗留（已与 Leader 达成、非本轮范围）
- S4（exit_date 超基准末日 last_le 取末行无 gap 标记）、S5（confidence %.2f 桶边界）、
  S6（backtest.New 循环内构造）→ 记录留下一 Sprint，均非阻塞、不污染正常结果。

---

## 裁定
所有 WARNING 已**亲验闭合**，回归全绿且新增针对性守门测试，顺手项落地。
无 high-severity 残留、无视角分歧 → **PASS**。建议进入最终验收（TASK-007 → accepted）。

— qa-agent-1 / round 3
