// Package decoys 是 TestScanIgnoresNonFieldOccurrences 的**假阳性夹具**：
// 三种「文本里出现禁用标识符但不构成回潮」的形态，扫描器一个都不该检出。
//
// 它同时量化了 AST 相对 grep 的价值 —— 同一份夹具 `grep lastReq` 会三个全中。
package decoys

import "time"

// 这段注释里出现 lastReq / minInterval / throttle，AST 扫描不该看见它们：
// parser 以 mode 0 解析，注释不进语法树。

// Fine 是已正确迁移的形态：限流交给 Gate，不持有任何私有节流状态。
type Fine struct {
	baseURL string
	gate    *int // 占位：真实代码里是 *policy.Gate
}

// throttleLocal 里的 lastReq / minInterval 是**局部变量**，不是结构体字段。
// 回潮的判据是「状态被持有」，局部变量随调用结束即消失，不构成节流状态。
func throttleLocal() time.Duration {
	lastReq := time.Now()
	minInterval := 500 * time.Millisecond
	return minInterval - time.Since(lastReq)
}
