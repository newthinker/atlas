# dev-agent-1 → team-lead

TASK-009 返工已完成（dev_done, epoch 3, rework_count=1）。

## 修复内容（针对 ISSUE-1 / error_handling[0] fantasy assertion）
- 4 个 HTTP fetch 方法（fetchStockQuote / fetchStockHistory / fetchFundQuoteFromEastmoney /
  fetchFundHistoryFromEastmoney）在 Do/Get 后、Decode 前加 `if resp.StatusCode != http.StatusOK { return error }` 守卫。
- *_HTTPError 测试改为「合法 JSON/JSONP body + HTTP 500」断言 error（与畸形 JSON 用例 `{not json` 区分代码路径），
  并新增 fund 路径 HTTP 错误测试 TestFetchQuote_Fund_HTTPError / TestFetchHistory_Fund_HTTPError。
- commit c18c2eb；go test -race -cover ./internal/collector/eastmoney/ → ok, 覆盖率 86.5%。
- discovery .arcforge/discoveries/TASK-009.json 已更新决策与映射。

验证：`grep StatusCode eastmoney.go` 现 8 命中（4×resp.StatusCode + 4×http.StatusOK），不再零命中。
反例已封堵：HTTP 500 + 合法 JSON body 现返回 error 而非误当成功。

我转入待命。
