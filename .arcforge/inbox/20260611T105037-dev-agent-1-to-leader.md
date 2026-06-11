# TASK-002 dev_done — dev-agent-1 → leader

TASK-002（yahoo 指数/期货符号 + FetchEPSHistory = plan Task 2+3）→ status=dev_done（epoch=1，commit 已落）。

## 完成标准↔测试映射（全 PASS）
| 标准 | 测试 | 结果 |
|---|---|---|
| functional[0] 正则接受 ^GSPC/GC=F、拒绝空/^/=F/^GSPC.SH/GC=X=F/注入串 | TestValidateSymbol_IndexAndFutures | PASS |
| functional[1] ^ 编码为 %5E | TestFetchQuote_EscapesIndexSymbol | PASS |
| functional[2] 解析 trailingDilutedEPS 升序 + type=trailingDilutedEPS | TestFetchEPSHistory | PASS |
| functional[3] NewWithBaseURLs 双端点 / NewWithBaseURL 兼容零修改 | TestFetchEPSHistory + 既有测试 | PASS |
| boundary[0] 空/缺字段→空slice+nil；raw<=0 保留 | TestFetchEPSHistory_EmptyAndIndexSymbol / _KeepsNonPositive | PASS |
| error_handling[0] 指数符号不发请求直接 error | TestFetchEPSHistory_EmptyAndIndexSymbol | PASS |
| non_functional 覆盖率≥80、全离线 | go test -cover = 81.7% | PASS |

## ISSUE-1 合规
FetchEPSHistory 在 Do 后 Decode 前加 StatusCode 守卫；HTTP 错误测试(503+合法JSON)与畸形JSON测试(200+非法body)走不同路径分别断言 error。

## 要点落实
- 局部变量 url→reqURL 防遮蔽 net/url；FetchQuote/FetchHistory 用 url.PathEscape。
- 抽 newRequest helper，三条路径(quote/history/eps)共用 UA/Accept 头（真实端点无 UA 会 403）。
- 既有测试零修改通过。

## 修改文件（仅 scope）
yahoo.go / yahoo_test.go / eps.go(新) / eps_test.go(新)；discovery 已写（含 FetchEPSHistory 完整签名）。

本次按全局规范调 code-simplifier 时已加严格 scope 约束（禁改状态/写盘/commit），其本轮无改动、未越权。
我名下暂无其他 assigned 任务，已落 checkpoint 待命。
