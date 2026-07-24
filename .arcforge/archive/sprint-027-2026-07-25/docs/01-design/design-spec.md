# Sprint 027 设计规格(摘要)

完整规格(含全部代码)见计划文档。接口契约:

- `func (y *Yahoo) do(req *http.Request) (*http.Response, error)`:节流+退避门;重试耗尽/预算耗尽返回最后响应(非 error)。
- Yahoo struct 新增:`mu sync.Mutex; lastReq time.Time; minInterval, backoffBase time.Duration; maxRetries, retryBudget int`(测试可覆盖)。
- 常量:`minRequestInterval=500ms / retryBackoffBase=1s / maxRetryAttempts=3 / retryBudgetPerRun=20`。
- 辅助:`throttle()`(持锁 sleep)、`takeRetryToken()`、`retryableStatus(429||>=500)`、`retryAfterWait(整数秒,解析失败回退退避)`。
- 调用点:yahoo.go:131(Quote)/yahoo.go:192(History)/eps.go:56(EPS)→ `y.do(req)`。
- 重试前排干并关闭 body(连接复用);GET body=nil 可安全重发同一 *http.Request。
