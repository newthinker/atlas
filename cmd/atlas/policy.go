package main

import (
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// ensurePolicyGate 供**不经 loadConfigOrDefaults 就构造 collector** 的入口显式装配闸门。
//
// 为什么需要它：initPolicyGate 全仓只有两个调用点，其中一个藏在 loadConfigOrDefaults
// 函数体内 ⇒ **接线与配置加载被隐式耦合，任何跳过配置加载的路径都会静默跳过接线**。
// 已知三条这样的路径：crisis backfill / crisis eval（两者的 resolveFREDKey 在
// FRED_API_KEY 非空时提前 return，根本不读配置）、backtest（全文不读配置）。
// 它们随后构造的 collector 会按构造时快照拿到懒构造的无账本 Gate，
// cache.enabled / cache.ttl / 整个 collector.topics 全部静默失效。
//
// 配置不可读时**退化而不阻断**：这些入口原本在配置无效时也能跑（crisis 靠环境变量拿
// FRED key，backtest 压根不需要配置），补接线不应该把它们变成必须有合法配置才能启动。
// 退化目标是内置策略表，即「限流/缓存按内置值、无 config 覆盖、无跨进程配额账本」。
func ensurePolicyGate() {
	if _, err := loadConfigOrDefaults(); err != nil {
		initPolicyGate(config.Defaults(), nil)
	}
}

// initPolicyGate 从配置构建 collector 策略闸门并装成进程内单例。
//
// 必须在**构造任何 collector 之前**调用：各 collector 在构造函数里取
// policy.Default() 存入私有字段（设计 §3.4）。log 可为 nil（离线 CLI 路径）。
//
// ⚠ 调用点只有两处，且**其中一处必须是 loadConfigOrDefaults 内部**（而不是它的
// 某个调用点）：prism refresh / crisis / export_signals / watchlist 都经由该 helper
// 装载配置，放在外面会让这些入口拿到懒构造的无账本 Gate——配额彻底失效，而所有
// 直接调用 initPolicyGate 的测试仍然全绿。设计 §1.5 的立论是「launchd 短命进程下
// 内存计数无效」，prism refresh 正是跨进程配额唯一真正生效的进程。
func initPolicyGate(cfg *config.Config, log *zap.Logger) {
	tbl := policy.NewTable()
	if cfg.Collector.Cache.Enabled {
		tbl.ApplyTTL(cfg.Collector.Cache.TTL)
	} else {
		// 等价于被本 Sprint 删除的 maybeCache 直接返回原 collector（设计 §4.2）：
		// 只清 TTL，限流与配额不受影响。
		tbl.DisableTTL()
	}
	for topic, tc := range cfg.Collector.Topics {
		tbl.Override(topic, policy.Override{
			TTL:         tc.TTL,
			MinInterval: tc.MinInterval,
			Timeout:     tc.Timeout,
			Coalesce:    tc.Coalesce,
			QuotaLimit:  tc.QuotaLimit,
			QuotaWindow: tc.QuotaWindow,
		})
	}

	path := cfg.Collector.Quota.Path
	if path == "" {
		path = "data/collector-quota.json"
	}

	warn := func(string, error) {}
	if log != nil {
		warn = func(msg string, err error) { log.Warn(msg, zap.Error(err)) }
	}
	policy.SetDefault(policy.New(tbl, policy.NewFileStore(path), policy.WithWarn(warn)))
}
