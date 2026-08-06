package policy

import "sync"

// 进程内单例。各 collector 在**构造函数**里取 Default() 存入私有字段，因此
// SetDefault 必须在构造任何 collector 之前调用（cmd/atlas 的两条装配路径
// 都在配置装载后立即调用，见 cmd/atlas/policy.go）。
//
// 未调用 SetDefault 时 Default() 懒构造一个「内置表 + 无配额账本」的 Gate：
// 限流与合并仍生效，只是拿不到 config 覆盖与跨进程配额。这让 broker 等
// 边缘 CLI 路径无需接线也能安全运行。
var (
	defaultMu   sync.RWMutex
	defaultGate *Gate
)

// Default 返回进程内默认 Gate，**永不为 nil**。
// SetDefault(nil) 之后再次调用会重新懒构造，而不是返回 nil。
func Default() *Gate {
	defaultMu.RLock()
	g := defaultGate
	defaultMu.RUnlock()
	if g != nil {
		return g
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	// 双重检查：RUnlock 与 Lock 之间可能已有别的 goroutine 构造过。
	if defaultGate == nil {
		defaultGate = New(NewTable(), nil)
	}
	return defaultGate
}

// SetDefault 替换进程内默认 Gate。传 nil 表示清空，下次 Default() 会重新懒构造。
func SetDefault(g *Gate) {
	defaultMu.Lock()
	defaultGate = g
	defaultMu.Unlock()
}
