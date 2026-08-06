// 本文件**故意语法错误**，是 TestScanReportsParseFailure 的夹具：
// DoD error_handling[0] 要求解析失败时给出清晰信息、不 panic 不静默跳过。
//
// ⚠ go build / go vet / go test 都整体忽略 testdata，不受影响；但**裸跑
// `gofmt -l internal/collector/` 会对本文件报解析错误**。这是预期的，不是仓库坏了。
package parsefail

func broken(
