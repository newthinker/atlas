// Package throttleback 是 TestScanDetectsThrottleState 的**阳性对照夹具**：
// 一份故意写成「私有节流回潮」形态的源码，用来证明扫描函数真的会检出。
//
// 为什么需要它：本任务是纯回归任务，被测行为（四个 collector 已清干净）在写
// 测试之前就已成立，`TestNoPrivateThrottleState` **一写就绿**。而「扫描器正确
// 工作、目标确实干净」与「扫描器恒返回空」在输出上完全一样 —— 没有这份夹具，
// 一个永远返回 nil 的扫描函数同样能让本任务全绿（DoD functional[1]）。
//
// 它不参与编译：go 工具链整体忽略 testdata 目录。
package throttleback

import (
	"sync"
	"time"
)

// Client 复刻的是被本 Sprint 删掉的那套私有节流状态
// （mu + lastReq + minInterval），即「照抄旧 collector 重写一遍」的回潮形态。
type Client struct {
	baseURL     string
	mu          sync.Mutex
	lastReq     time.Time
	minInterval time.Duration
}
