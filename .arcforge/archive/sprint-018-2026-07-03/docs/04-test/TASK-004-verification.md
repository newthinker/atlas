# TASK-004 验证报告 — atlas watchlist 命令与渲染（Sprint 收口）

- 验证者: test-agent-1
- 判定: **VERIFIED (PASS)**
- commit: e3cdb0a `feat(watchlist-cmd): add atlas watchlist metrics command`
- 验证时间: 2026-07-03

## Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| F1 | 表格 CJK 对齐(NAME 列起点宽度一致)+缺失显示 —+表后缺口摘要 | test | TestExecuteWatchlist_TableAligned PASS。对齐断言真实: 用 text.DisplayWidth(ln[:cut]) 比较各行 NAME 列起点显示宽度==headWidth(非字节偏移); fixture 含中英混排(贵州茅台/Apple); 断言 out 含 — 与 gap 摘要 | PASS |
| F2 | --json 合法数组+缺失为 null+可 roundtrip(AD-5 内嵌 gaps) | test | TestExecuteWatchlist_JSON PASS: json.Unmarshal 成功, len==2, got[1].PE==nil, 输出含 `"pe": null` | PASS |
| F3 | --symbols 未知 stderr warn 跳过; 全未知返回 error | test | _UnknownSymbolWarns(errOut 含 NOPE 警告, err nil) + _AllUnknownSymbolsErrors(全未知→err!=nil) 均 PASS | PASS |
| B1 | 空 watchlist: stderr 提示+返回 nil 不报错 | test+smoke | _EmptyWatchlist PASS; **离线冒烟进程级实证**: `atlas watchlist` 空配置→stdout 空/stderr "watchlist is empty…"/退出码 0 | PASS |
| E1 | 全标的失败: 打 gaps 后返回 error→非零退出 | test | _AllFailedErrors PASS(两标的仅 Gaps→err!=nil)。退出码为 proxy 断言: executeWatchlist 返 error→runWatchlist(RunE)→cobra 映射非零退出(措辞如实) | PASS |
| E2 | config 语义: --config 不可解析→非零退出; 无 --config 走默认继续 | review | loadConfigOrDefaults(export_ohlcv.go:283 共用): cfgFile!="" 时 config.Load 失败→返回 error→runWatchlist 返 error→非零退出; 无 --config→config.Defaults() 继续。与 export 命令逐字同源 | PASS |
| N1 | stdout 仅表格/JSON; 注册进 rootCmd; 零新增第三方依赖 | review+smoke | 注册: init() rootCmd.AddCommand, `atlas --help` 含 watchlist; 零新增依赖: go.mod/sum 未变; stdout/stderr 分离(构造保证): data→deps.out, 操作警告(未知标的/空提示)→deps.errOut, logger→newStderrLogger(os.Stderr,WarnLevel)。冒烟实证空配置 stdout 纯净、提示走 stderr | PASS |
| N2 | go build ./... && go test ./... 离线全绿 | test | 见下"完整门禁" | PASS |

## 完整门禁（收口验证，Leader 要求）
- `go build ./...`: PASS(干净)
- `go vet ./...`: PASS(无告警)
- `go test ./...`: 全仓离线全绿(无一失败)
- 离线端到端冒烟: `atlas watchlist` / `--json` 空 watchlist 均退出码 0，stdout/stderr 分离正确

## 测试运行证据
- TestExecuteWatchlist_{TableAligned,JSON,UnknownSymbolWarns,AllUnknownSymbolsErrors,AllFailedErrors,EmptyWatchlist}: 6/6 PASS

## 覆盖率
- watchlist.go: executeWatchlist/renderTable/renderGaps/allFailed/fmtPtr 100%; resolveSymbolFilter 93.8%; init 100%
- runWatchlist/newStderrLogger 0%: 薄 CLI 装配层(loadConfig→app.New→buildCollectors→executeWatchlist),逻辑刻意下沉到可测的 executeWatchlist(镜像 exportDeps 模式),由离线冒烟兜底,非 DoD 单测场景

## 关键实现说明（透明记录，供 QA 判读）
- table 模式 gap 摘要写 deps.out(stdout): 与计划 line 1006-1007 逐字一致——`renderTable(deps.out)` + `renderGaps(deps.out)` 是设计意图(人读报告一部分)。stdout 纯净在 JSON 模式为严格(纯数组, gaps 内嵌 per AD-5); table 模式 stdout=表格+缺口摘要(均属报告), 操作警告/logger 走 stderr。allFailed 路径 gaps 改走 errOut(错误诊断)。
- code-simplifier 两处简化: 简化后 6 个用例全 PASS 且行为与计划(executeWatchlist/renderTable/renderGaps 逐字比对)一致,无语义偏离。

## 反向验收
- 改动仅 watchlist.go/watchlist_test.go 两新文件(name-status: A/A),与 estimated_files 一致,无越界。
- 无 YAGNI/误实现,复用 TASK-001(text)/002(SnapshotMetrics)/003(buildCollectors)接口。

## 结论
8/8 done_criteria PASS，完整门禁全绿，离线冒烟进程级实证退出码与 stdout/stderr 分离。判定 VERIFIED。
