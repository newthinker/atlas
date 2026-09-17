package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/hestia"
	"github.com/newthinker/atlas/internal/metrics"
)

// buildHestiaHealth 按主配置 hestia.config_path 打开 hestia 库并把健康度 collector
// 挂到 serve 的注册表上（M1.5 的 TASK-006）。
//
// 三种启动语义：
//   - config_path 未设，或 metrics 未启用（reg nil）⇒ 日志一行、跳过，返回 no-op cleanup
//   - config_path 设了但 hestia.yaml 装不上 / 库打不开 ⇒ **返回错误，serve 启动失败**
//     （沿 M1d 挂账 C3「装不上即响亮失败」：静默变成「没有健康度」正是本迭代要消灭的形态）
//   - 成功 ⇒ 注册 collector，cleanup 关 Store
//
// 队列目录缺失**不**让启动失败：那是运行期可告警的事实（hestia_queue_up=0），与「库打不开」
// 刻意不同——队列每轮现读，目录之后被建回就自愈；启动时拒绝反而让健康度整组消失。
//
// NewStore 会执行 CREATE … IF NOT EXISTS，对已建表无操作；serve 与 ingest 跨进程共用
// WAL 库，多读者安全，SQLITE_BUSY 由 sqliteDSN 的 busy_timeout 兜底。
func buildHestiaHealth(cfg *config.Config, reg *metrics.Registry, log *zap.Logger) (func(), error) {
	noop := func() {}
	if reg == nil {
		log.Info("hestia health disabled (metrics disabled)")
		return noop, nil
	}
	if cfg.Hestia.ConfigPath == "" {
		log.Info("hestia health disabled (hestia.config_path not set)")
		return noop, nil
	}
	hcfg, err := hestia.LoadConfig(cfg.Hestia.ConfigPath)
	if err != nil {
		return noop, fmt.Errorf("hestia health: loading %s: %w", cfg.Hestia.ConfigPath, err)
	}
	st, err := hestia.NewStore(hcfg.Storage.DBPath)
	if err != nil {
		return noop, fmt.Errorf("hestia health: opening %s: %w", hcfg.Storage.DBPath, err)
	}
	// 队列目录：hestia 配置里的 queue.dir，与 ingest 写契约用的是同一个值。不判空——
	// LoadConfig 已拒绝空 queue.dir（internal/hestia/config.go 的 validate）。
	// ⚠️ 默认值 "queue/hestia" 是**相对路径**，相对 serve 进程的 cwd 解析：依赖 launchd 给
	// serve 的 WorkingDirectory 与 hestia-ingest 一致。两者不一致时健康度会盯着另一个目录。
	queueDir := hcfg.Queue.Dir
	reg.MustRegister(metrics.NewHestiaCollector(func(ctx context.Context) (hestia.Health, error) {
		return hestia.HealthSummary(ctx, st.DB())
	}, func() (hestia.QueueHealth, error) {
		return hestia.QueueHealthOf(queueDir)
	}, time.Now))
	log.Info("hestia health enabled",
		zap.String("config", cfg.Hestia.ConfigPath), zap.String("db", hcfg.Storage.DBPath),
		zap.String("queue", queueDir))
	return func() { _ = st.Close() }, nil
}
