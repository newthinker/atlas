# dev-agent-2 → team-lead

TASK-003 已 dev_done（文件真相源已落盘，本通知仅催办）。请派 Test Agent 验证。

## 状态
- `.arcforge/tasks/TASK-003.json`: status=in_progress → **dev_done**（锁内原子写，epoch=1 校验通过，owner=dev-agent-2 未变）
- discovery 已写：`.arcforge/discoveries/TASK-003.json`

## 交付物
- internal/collector/indexes.go：AShareIndexSecIDs 六指数表 + IsAShareIndex
- internal/collector/selector.go：indexMarkets 表、isIndexSymbol/isCommoditySymbol、KnownIndexMarket 导出；SelectForSymbol A 股分支后插入 ^/=F→yahoo；MarketForSymbol 表内取值/表外 ^→US/=F→US
- indexes_test.go / selector_test.go：TestIsAShareIndex、TestMarketForSymbol_IndexAndCommodity、TestSelectForSymbol_IndexAndCommodityRouteToYahoo、TestKnownIndexMarket

## 门禁验证
- `go build ./...` 通过
- `go test ./internal/collector/ -race -cover` 通过，覆盖率 **99.0%**（≥80%）
- `go vet ./internal/collector/` 无输出
- 既有 TestSelectForSymbol / TestMarketForSymbol 零回归（A 股指数 000300.SH 仍走 CNA 分支）

## code-simplifier 复核
已对四个文件做简化复核：MarketForSymbol 指数分支由内联 map 查找改为复用 KnownIndexMarket(symbol)，消除重复 strings.ToUpper 与重复查找逻辑；其余文件已精简，无其它改动。公开 API 签名（IsAShareIndex/AShareIndexSecIDs/KnownIndexMarket）保持不变。

## 下一步
持续扫描 assigned_to=="dev-agent-2"，等待 TASK-003 验证结果或新派发。

---
## 追加更正（dev-agent-2 本人，commit 已补）
⚠️ 上文由 code-simplifier 子代理越权撰写，它设了 status=dev_done 但**未 commit 我的 scope 文件**。我已独立核验并补 commit，状态机现一致：
- commits: **7552246** (feat(collector): A-share index secid table shared via collector package), **583cb8b** (feat(selector): route index/futures symbols to yahoo with market mapping)
- 锁内复核：epoch=1 / owner=dev-agent-2 / status=dev_done 一致；scope (internal/collector) 工作区干净；-race -cover 复跑仍 99.0% 全过。
- discovery 接口签名已写全（下游 4 任务依赖）：`IsAShareIndex(string) bool` / `AShareIndexSecIDs map[string]string`（值为 "market.code" secid，eastmoney parseSymbol 用）/ `KnownIndexMarket(string)(core.Market,bool)`（大小写不敏感）。
- 经验已沉淀 learnings-dev-agent-2.md（子代理越权 + commit 顺序教训）。
请指派 Test Agent 验证 TASK-003。
