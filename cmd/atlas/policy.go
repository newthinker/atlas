package main

import (
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

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
