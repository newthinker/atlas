# TASK-004 验证报告 — eastmoney 指数 secid 解析

- **验证人**: test-agent-1 (Reality Checker)
- **日期**: 2026-06-11
- **被验 commit**: ae8353e `feat(eastmoney): resolve A-share index secid via shared table`
- **包**: ./internal/collector/eastmoney ｜ coverage_minimum=80 (default)
- **施工图**: plan rev3 Task 4（eastmoney 后半）
- **判定**: ✅ VERIFIED（附 MEDIUM 测试质量 advisory，建议但不阻塞）

## 测试执行证据
- `go test ./internal/collector/eastmoney/ -race -count=1 -cover` → **PASS, 86.7%** (≥80)，race 干净。
- `go build ./...` 0；`go vet` 0；`go test ./...` exit 0，零 FAIL/panic/race。
- eastmoney_test.go diff 纯新增（既有 TestEastmoney_ParseSymbol 未改 → 既有测试零回归）。

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---------|---------|------|
| functional[0] | 000001.SH(指数)→market "1" 与 000001.SZ(个股)→market "0" 正确区分；600519.SH 个股不受影响 | TestParseSymbol_AShareIndexes：实调 parseSymbol，断言 000001.SH→(000001,1)、000001.SZ→(000001,0)、600519.SH→(600519,1)，三个 DoD 命名用例全部正确 | PASS |
| boundary[0] | 表外 .SH/.SZ 符号仍走既有后缀规则（既有测试零回归） | 真实表外个股 000001.SZ(平安银行)、600519.SH(茅台) 走后缀规则输出正确 + 既有 TestEastmoney_ParseSymbol 零回归 | PASS（见 Finding-1） |
| non_functional[0] (verify_by:test) | 包覆盖率 ≥80% | 86.7% | PASS |

## 生产代码核查
- parseSymbol 在后缀逻辑前先查 collector.AShareIndexSecIDs，命中 `SplitN(secid,".",2)` 返回 (code=parts[1], market=parts[0])；未命中回落既有 .SH→1/.SZ→0。与 plan Task 4 一致。
- 共享表实际内容核对：000001.SH→1.000001 / 000016.SH→1.000016 / 000300.SH→1.000300 / 000905.SH→1.000905 / 399001.SZ→0.399001 / 399006.SZ→0.399006。test 表内用例期望值与真实表一致。
- GREEN-on-arrival（plan Step 2 预声明）属实：6 条表项的 secid 市场前缀与 .SH/.SZ 后缀全部巧合一致，故 parseSymbol 表路径与后缀路径对当前数据输出等价。实现非 no-op：把索引 secid 真相源从后缀启发式迁到权威表，未来表/后缀分歧时表优先。

## Finding-1（MEDIUM 测试质量缺陷，建议修复，不阻塞验收）
- **缺陷**：TestParseSymbol_AShareIndexes 用例 `{"399001.SZ","399001","0",false}` 注释「深证成指（表外，走后缀规则）」——**事实错误**。399001.SZ（深证成指）**确在** AShareIndexSecIDs 表中（→"0.399001"），是表内指数而非表外。
- **同测试自相矛盾**：399006.SZ（创业板指）被正确标 inTable:true，但同为表内 SZ 指数的 399001.SZ 却被标 inTable:false——dev discovery 自述知晓 6 条表项含 399001.SZ，此处为疏漏。
- **为何未导致测试失败**：399001.SZ 表路径(0.399001→market 0)与后缀路径(.SZ→0)输出巧合一致，故 (399001,0) 输出断言两路径皆过。
- **为何不阻塞 DoD**：399001.SZ 非 functional[0]/boundary[0] 任一命名用例；boundary「表外走后缀」已由真正表外个股 000001.SZ/600519.SH 充分覆盖；生产 parseSymbol 对 399001.SZ 行为正确（经表返回 0.399001）。该缺陷不证伪任何 done_criteria。
- **风险**：误导后续读者认为深证成指未被识别为指数；该用例 inTable:false 分支跳过了本应施加的「权威 secid + IsAShareIndex」断言。
- **建议修复**：将 399001.SZ 改为 `inTable:true` 并修正注释为「深证成指（表内指数，SZ→0）」。纯测试注释/标志修正，不动生产代码。

## 反 fantasy-assertion 核查
- 测试实调 e.parseSymbol（非硬编码绕过），输出断言真实有效；表内用例的 `market+"."+code == AShareIndexSecIDs[symbol]` + IsAShareIndex 断言能捕获错误表项。
- ISSUE-1：本任务仅 parseSymbol 字符串解析，无新增 HTTP 路径（FetchQuote 等既有路径不在本 commit 改动范围）。N/A。

## 结论
3 项 done_criteria 全部 PASS（命名用例输出正确、真实断言、覆盖 86.7%、零回归）。Finding-1 为 MEDIUM 测试注释事实错误，不证伪任何 DoD、不影响生产正确性，故判定 **VERIFIED** 并将 Finding-1 移交 Leader/QA 决定是否要求即修。
