# TASK-003 验证报告 — 提取 buildCollectors（serve 装配段迁出）

- 验证者: test-agent-1
- 判定: **VERIFIED (PASS)** — 含一处偏离，经等价性判定接受
- commit: 203cf8a `refactor(watchlist-cmd): extract buildCollectors shared wiring from serve`
- 验证时间: 2026-07-03

## Done Criteria 覆盖矩阵

| # | 完成标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| F1 | 空配置成功+cleanup nil-safe; Defaults 断言注册 collector 名称集合(AD-8/B10) | test | TestBuildCollectors_EmptyConfig(0 采集器+cleanup 不 panic) + TestBuildCollectors_Defaults(yahoo/eastmoney/crypto 启用→断言 got=={crypto,eastmoney,yahoo} 精确集合,非只判非空) 均 PASS | PASS |
| F2 | lixinger 启用注入 FundamentalSource; 未配置不注入(typed-nil 不逃逸) | test | TestFundamentalSourceOrNil: fundamentalSourceOrNil(nil)→untyped-nil 接口(!=nil 为假), live collector→非 nil。防 typed-nil 陷阱实证 PASS | PASS |
| B1 | 装配段逐字迁移(diff 对照 serve.go:99-170 除包装+两处新增零差异); serve 行为零变化 | review | 归一化 diff: 老 serve.go 被删装配段(cache→SetValuationLookback) vs collectors.go 函数体逐字相同(唯一 diff 是 git 归为 context 行的 `}`,实际存在)。差异仅: ①函数包装(package/import/签名/return) ②新增 SetFundamentalSource ③新增 cleanup 闭包 ④新增 fundamentalSourceOrNil helper | PASS |
| N1 | go build ./... 干净(serve.go 无未使用 import); cmd/atlas 全绿零回归 | test | go build ./... 干净; go vet ./cmd/atlas ok; serve.go 移除 database/sql,crypto,eastmoney,qlibpit,保留 lixinger(line 374/377 valuationSourceOrNil 引用)/yahoo(backtester); cmd/atlas 全量 ok(serve/executor/export/alert_runner/signalstore 零回归) | PASS |
| N2 | cleanup 确实关 qlib 仓库句柄(nil-safe) | review | cleanup 闭包 `if qlibWarehouseDB != nil { qlibWarehouseDB.Close() }`,可无条件 defer; EmptyConfig 用例走 nil 分支不 panic 实证 | PASS |

## 偏离判定（Leader 要求）
**偏离**: 计划的内联 `if lixingerCollector != nil { SetFundamentalSource(lixingerCollector) }` 改为
`SetFundamentalSource(fundamentalSourceOrNil(lixingerCollector))` 无条件调用 + 新增 helper。

**判定: 等价，接受**
- a) helper 语义与既有 valuationSourceOrNil 一致: `if c == nil { return nil }; return c` — nil 具体类型→字面 nil 接口，非 typed-nil。
- b) 行为等价: lixinger!=nil → helper 返回 c → fundamentalSrc=c(同内联); lixinger==nil → helper 返回 nil 接口 → SetFundamentalSource(nil) 令 fundamentalSrc=nil(=零值默认,与内联"不调用"相同)。snapshot.go:149 守卫 `if a.fundamentalSrc != nil` 对两路径行为一致(nil→记 gap,不调 FetchFundamental)。
- 理由正当: fundamentalSrc 为 app 不导出字段,cmd/atlas 无法观测,抽 helper 才能单测 typed-nil 防护(F2)。

## 测试运行证据
- TestBuildCollectors_EmptyConfig / _Defaults / TestFundamentalSourceOrNil: 全 PASS
- go build ./...: 干净; go vet ./cmd/atlas: ok
- go test ./cmd/atlas/: ok(既有测试零回归)
- go test ./...: 全仓离线全绿

## 覆盖率
- cmd/atlas: 68.6%（超先例 floor 35）
- collectors.go: fundamentalSourceOrNil 100%; buildCollectors 70.0%

## 非阻断观察
- buildCollectors 70%: 未覆盖 30% 为 lixinger/eastmoney-fallback/crypto-Extra/qlib-enabled 分支(需配置+网络/DB 的集成路径),非 5 条 DoD 要求场景,离线不可及,不阻断。

## 结论
5/5 done_criteria PASS，逐字迁移经归一化 diff 实证，偏离经等价性判定接受。判定 VERIFIED。
