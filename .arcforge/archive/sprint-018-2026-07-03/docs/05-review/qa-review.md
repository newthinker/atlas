# QA 终审报告 — Sprint 018：atlas watchlist 指标命令

- 审查者：qa-agent-1
- 分支：feature/watchlist-metrics-command（master 之后 4 提交：2e824a5 / 9e9a431 / 203cf8a / e3cdb0a）
- 日期：2026-07-03
- 模式：两轮（第一轮常规 Code Review + 第二轮四视角对抗，Agent Teams 降级纯 Claude）

## VERDICT: PASS（最终 — 修复轮后）

初审 4 个 WARNING 全部修复并聚焦复核通过（见文末「修复复核」节）。对抗轮无 CRITICAL，未达 REJECT 共识。

计数：**CRITICAL 0 / WARNING 4 / SUGGESTION 5**。severity_threshold=warning，四项 WARNING 均附修复建议。

## 实证基础（非纯静态阅读）

- `go build ./...`、`go vet` 干净
- `go test -race ./internal/app ./internal/text` 全绿（含 9 个 TestSnapshotMetrics_*，无 data race）
- `go test ./cmd/atlas`、`go test ./internal/notifier/telegram` 全绿（回归）
- 离线冒烟：空 watchlist→exit 0（提示走 stderr）；坏 config→exit 1；全失败→exit 1（gap 走 stderr）；US happy-path 表格 CJK 对齐、估值三项显示「—」+ gap 摘要
- `go.mod` / `go.sum` diff 为空（零新第三方依赖）
- width.go 迁移：git rename 70% 相似度，isWide 区间表逐字一致（11 个 0x 区间两侧相同）
- serve.go → buildCollectors：装配段逐字迁移，零行为差异
- 新增测试函数 20 个（snapshot 9 / watchlist 6 / collectors 3 / text 2）

---

## 第一轮：常规 Code Review

代码质量优秀：外科式重构且迁移逐字保真已实证；错误处理完善（降级路径齐全）；命名清晰；职责单一；测试充分且真正验证业务逻辑（依赖注入 watchlistDeps / snapFake 隔离网络）。核心逻辑正确。以下为需处置项。

### [WARNING] W1 — eastmoney CHG% 单位不一致（既有缺陷，被新命令显著暴露）

- 文件：`internal/collector/eastmoney/eastmoney.go:239`
- 证据：Price（:233）、Change（:238）均 `/divisor`（stocks=100），但 `ChangePercent: d.F170` 未除。F170 的 json tag 注释为「Change percent」，东财 push2 约定为原始值×100（与 f43 价格同为 2 位小数缩放）。A 股显示 +204.0% 实为 +2.04%。web UI symbol_detail 亦消费该字段。
- 定级理由：非本轮 diff 引入，但 watchlist 命令头牌指标即 A 股行情（含 lixinger 估值集成的旗舰市场），带病发布削弱功能价值。
- 建议：本轮顺手修 `ChangePercent: d.F170 / 100`（一行、隔离；注意百分比恒 2 位小数，不随 ETF divisor=1000 变化）。若坚持范围纪律则 backlog 并开显式 ticket。**推荐：本轮修**。

### [WARNING] W2 — positivePtr 把合法 0/负值静默掩盖为「—」

- 文件：`internal/app/snapshot.go:193`（positivePtr）、`:151-153`（PE/PB/DividendYield 赋值）
- 证据：`positivePtr(v)` 对 v<=0 返回 nil。lixinger.FetchFundamental（`internal/collector/lixinger/fundamental.go:44-53`）无数据时返回 error（已被 snapshot 记 gap），但**有数据时** dyr=0（不分红，A 股成长/亏损股常见）→ DividendYield=0；亏损/ST 股 pe_ttm<0。经 positivePtr 后 0 股息率与负 PE 均渲染成「—」，与「数据不可用」不可区分，且**不记 gap**。「—」暗示未知，但 0% 股息是已知事实——语义混淆。现有测试仅覆盖正值。
- 建议：FetchFundamental 成功后直接取 `&fd.PE / &fd.PB / &fd.DividendYield`，仅 fetch 出错时置 nil；至少 DividendYield 必须如实显示 0.00。负 PE 若产品上不欲显示，也应单独记 gap（如 "pe non-positive: -3.2"）而非静默吞掉。

### [WARNING] W3 — OHLCV 窗口口径分叉 + docstring 过度承诺（重开 AD-8 C3）

- 文件：`internal/app/snapshot.go:126-191`（snapshotHistoryStart）↔ `internal/app/app.go:456`（analyzeSymbol）/ `:811`（historyWindowDays）；docstring `cmd/atlas/watchlist.go:29`
- 证据：watchlist.go:29 声称「reuses the analysis loop's exact valuation pipeline」。实际：分析循环按每标的绑定策略的 PriceHistory 定窗（historyWindowDays，无高需求策略时默认 365 日历日），snapshot 全局按 valuation.lookback_years 定窗（默认 5 年≈1826 天，完全不看 item 策略）。对美/港个股 PE 百分位（ReconstructPEPercentile 依赖传入 ohlcv 窗口）与 PricePercentile，当 `pe_percentile/price_percentile.lookback_years ≠ valuation.lookback_years` 时，同一标的同一时刻两条路径产出不同分位值。真正共享的仅 FundamentalSource/EPSSource 取数边界（lixingerLookback/epsFetchStart，同由全局 valuationLookback 驱动）。
- **重开 AD-8 C3**：C3 裁「误报」的证据是「策略侧价格窗口 since-inception 即 ~100年（SinceInceptionBars）」——此前提**事实错误**。核验 `internal/strategy/pe_percentile/strategy.go:40`、`internal/strategy/price_percentile/strategy.go:37`：仅 `lookback_years==0` 才用 SinceInceptionBars，否则用 `lookback*252`（pe_percentile 默认 5×252、price_percentile 默认 3×252，见各自 strategy_test.go）。
- 定级理由（WARNING 非 CRITICAL）：影响有界——A 股完全不受影响（lixinger cvpos 忽略 ohlcv 窗口）；默认配置 PE 窗口恰都 5 年而巧合一致；仅美/港个股在非默认分叉配置下产出「不同但仍有效」的百分位，非崩溃/脏数据。
- 建议二选一：(a) snapshot 对齐 historyWindowDays 语义（真正等价）；或 (b) 弱化 docstring 措辞并注明价格/百分位窗口基准是 valuation.lookback_years。

### [WARNING] W4 — allFailed 判据漏检 PB/DividendYield，可能误吞可展示数据

- 文件：`cmd/atlas/watchlist.go:139-146`（allFailed）
- 证据：allFailed 仅检查 `Price / PricePercentile / PEPercentile / PE`，遗漏 PB 与 DividendYield。snapshotSymbol（snapshot.go:141-160）用同一次 FetchFundamental 独立赋值 PE/PB/DividendYield，亏损但有资产的 A 股完全可能 PE≤0→nil 而 PB>0→非 nil。若此类标的恰好 quote 与 history 也失败（Price==0、PricePercentile==nil）、valuationSrc 缺席（PEPercentile==nil），但 fundamentalSrc 取到 PB/DividendYield，则该标的仅 PB/DividendYield 非 nil，allFailed 仍判其「全失败」；若整个请求子集皆如此，命令误取 allFailed 分支（exit 1、gaps 仅走 stderr），尽管确有 fundamental 数据本可在表格/JSON 展示。与 W2 叠加（修 W2 后 PB 更明显可用）。
- 建议：allFailed 判据补上 `m.PB != nil || m.DividendYield != nil`，与「任一指标可用即非失败」语义对齐。

---

## 第二轮：Adversarial Review（四视角，降级纯 Claude）

变更规模 Large（1117 insertions），启用全部 lens。Leader 指定四视角：正确性/并发、CLI 用户体验、数据一致性、维护者。

### 各视角结论

- **正确性/并发（Skeptic）**：errgroup + results[i] 预分配索引写无 data race（-race 证实，Go 1.24.4 循环变量每迭代独立）；snapshotSymbolSafe 的 defer/recover 完整隔离 panic、命名返回值正确赋值；价格百分位现价缺失退回 closes[len-1] 再自我 PercentileRank 语义合理、边界优雅退化不 panic；无锁读 fundamentalSrc 等在唯一调用方（CLI，装配先于调用、单次顺序写）下安全。独立发现：positivePtr 掩盖（→W2）、窗口分叉（→W3）、gctx 死代码（→S1）、SetFundamentalSource 缺契约文档（→S4）、负 valuationLookback 配置边缘（→S5）。
- **CLI 用户体验 + 数据一致性**：--json 路径 stdout 纯净可被 jq 消费；退出码整体自洽。独立发现：窗口分叉判 CRITICAL（→W3，我裁为 WARNING）、allFailed 漏检 PB/DividendYield（→W4）、table 模式 gap 混入 stdout 且与全失败写 stderr 不一致（→S2/W-adjacent）、SilenceUsage 既有约定（→S3）。核验并排除：--symbols 大小写点非 bug（AddToWatchlistWithDetails 不 ToUpper，known 与存储皆用 config 原值）。
- **维护者**：重构逐字保真、零新依赖、测试覆盖充分，可维护性良好。

### 对抗轮分歧裁定

CLI/一致性 lens 将窗口分叉（W3）判 **CRITICAL**、并发 lens 判 **WARNING**。据代码证据独立裁为 **WARNING**（影响有界：A 股不受影响、默认配置巧合一致、非崩溃/脏数据）。分歧已消解，**不构成 CONTESTED**。

### 对抗轮 verdict：PASS（无 high-severity/CRITICAL 共识，未达 REJECT）

---

## SUGGESTION（不阻断）

- **S1** `internal/app/snapshot.go:59-67` — errgroup.WithContext 的 gctx 从不被观察（snapshotSymbol 签名 `_ context.Context` 已坐实丢弃；worker 恒返回 nil；collector 方法不收 context）。caller 传带 deadline 的 ctx 并取消 → 零效果。当前仅 CLI 无实害，但制造「可取消」假象，日后接 HTTP handler 会踩坑。镜像既有 analyzeSymbol。建议真正检查 gctx.Err() 或去掉 gctx 传递。
- **S2** `cmd/atlas/watchlist.go:99 ↔ :106` — renderGaps 同一诊断内容全失败时写 stderr、部分失败时写 stdout（紧跟表格），流选择不一致，把自由文本混入 stdout 破坏表格 schema。建议 table 模式 renderGaps 也统一写 deps.errOut（数据只走 stdout、诊断只走 stderr）。注：table 模式 gap 走 stdout 属 AD 原设计取舍，但与全失败分支不一致值得对齐。
- **S3** `cmd/atlas/watchlist.go:24` — watchlistCmd 未设 SilenceUsage/SilenceErrors，运行时错误 dump 完整 Long usage；全仓零命中，属既有约定，非本轮退步。若改应在 rootCmd 统一设（仓库级决策），否则维持现状。
- **S4** `internal/app/snapshot.go:23`（SetFundamentalSource） — 缺姊妹 setter（SetValuationSources app.go:166-169 / SetValuationLookback app.go:176）显式的「set-once-before-Start」契约注释。当前安全（buildCollectors 顺序 set-once 先于 goroutine 启动），但文档缺口意味未来热重载/重配特性若在 workers 运行中二次调用即成真·data race。建议补齐契约注释。
- **S5** `internal/app/snapshot.go:187`（snapshotHistoryStart <=0）— 与自称 mirror 的 epsFetchStart（app.go:218）/ lixingerLookback（app.go:183，均只特判 ==0）判定不一致，且 config.Validate() 无 LookbackYears>=0 校验。负值时 snapshotHistoryStart 防御正确（-100年），但 epsFetchStart 会算出未来起始日→静默退化 EPS 窗口。低概率配置边缘。建议 config.Validate() 加 LookbackYears>=0 校验，或三处统一用 <=0。

---

## 交付建议

核心逻辑正确、质量优秀，4 个 WARNING 均属「保真度/承诺准确性/边界一致性」范畴，无功能性崩溃或数据损坏。建议 Leader：
- 就 **W2 + W4** 走 review_fix（同为 fundamental 数据保真/一致性、改动小、用户可见）；
- **W1** 可本轮顺手修（一行）；
- **W3** 可用 (b) 文档软化 + 重开 AD-8 C3 处理。

状态机迁移（review_fix / accepted）为 Leader 专属职责，QA 无 tasks/*.json 写权限。

---

## 修复复核（终审，qa-agent-1，2026-07-03）

修复轮 4 提交聚焦复核通过——仅确认 4 个 WARNING 修复到位与无新引入问题，未重跑全量审查。build/vet 干净，各新测试与相关包全量离线全绿。

| WARNING | 提交 | 核验结论 |
|---|---|---|
| W1 eastmoney CHG% | ddc543c | PASS。`ChangePercent: d.F170 / 100`，固定 100 非 divisor；`TestFetchQuote_ChangePercentScale` 四子用例（stock/ETF/0/负）全绿，ETF 子用例证实 divisor=1000 不触 percent。 |
| W2 positivePtr 掩盖 | b6f59a4 | PASS。成功路径直取 `&fd.PE/&fd.PB/&fd.DividendYield`（fd 每次调用独立分配，无别名），positivePtr 删除，仅 fetch 出错置 nil+gap；`TestSnapshotMetrics_LegitimateZeroNegative` 断言负 PE=-8.2 如实、0 股息率显示 0、无 fundamental gap。 |
| W3b docstring | 8ef8835 | PASS。watchlist.go Long 由「reuses exact pipeline」改为「mirrors…；percentile windows use global valuation.lookback_years，非默认配置下可能不同」；snapshot.go 注释同步。AD-8a 已回写。 |
| W4 allFailed | df769a4 | PASS。补 `m.PB != nil \|\| m.DividendYield != nil`；`TestExecuteWatchlist_FundamentalOnlyNotAllFailed` 断言仅 PB/DYR 的标的不误判全失败、照常渲染。与 W2 协同修复边界。 |

无新引入问题：W2 的 `&fd.PE` 无逃逸/别名风险；W2+W4 协同后 fundamental-only 标的正确展示；既有 TestSnapshotMetrics_AllCollectorsFail 等零回归。S1~S4/S5 转 backlog（Leader 已记入交付报告）。

**最终 verdict：PASS。** 批准进入交付（PR）。
