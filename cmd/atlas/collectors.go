package main

import (
	"database/sql"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/eastmoney"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/qlibpit"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// buildCollectors wires every configured collector plus the valuation/EPS/
// fundamental sources into application — the single shared assembly for
// `atlas serve` and offline commands (`atlas watchlist`). cleanup closes the
// qlib warehouse handle (nil-safe); err is reserved by the spec and currently
// always nil (collector wiring degrades instead of failing).
func buildCollectors(cfg *config.Config, application *app.App, log *zap.Logger) (cleanup func(), err error) {
	// OHLCV cache settings applied when registering collectors below.
	cacheEnabled := cfg.Collector.Cache.Enabled
	cacheTTL := cfg.Collector.Cache.TTL
	if cacheEnabled {
		log.Info("OHLCV collector cache enabled", zap.Duration("ttl", cacheTTL))
	}

	// Register collectors if configured. yahooCollector is declared at function
	// scope so it can later be injected as the app's EPS source for PE-percentile
	// reconstruction (it stays nil when yahoo is not configured).
	var yahooCollector *yahoo.Yahoo
	if collectorCfg, ok := cfg.Collectors["yahoo"]; ok && collectorCfg.Enabled {
		yahooCollector = yahoo.New()
		application.RegisterCollector(maybeCache(yahooCollector, cacheEnabled, cacheTTL))
	}

	// Create Lixinger collector if configured (used as fallback for Eastmoney).
	// retry 开关默认开启（遵循 SKILL.md 退避策略），可在配置中关闭。
	var lixingerCollector *lixinger.Lixinger
	if collectorCfg, ok := cfg.Collectors["lixinger"]; ok && collectorCfg.Enabled && collectorCfg.APIKey != "" {
		retry := true
		if v, ok := collectorCfg.Extra["retry"].(bool); ok {
			retry = v
		}
		lixingerCollector = lixinger.New(collectorCfg.APIKey, lixinger.WithRetry(retry))
		log.Info("lixinger collector initialized as fallback for eastmoney")
	}

	// Register Eastmoney collector for A-shares
	if collectorCfg, ok := cfg.Collectors["eastmoney"]; ok && collectorCfg.Enabled {
		eastmoneyCollector := eastmoney.New()
		// Set Lixinger as fallback if available
		if lixingerCollector != nil {
			eastmoneyCollector.SetLixingerFallback(lixingerCollector)
			log.Info("lixinger fallback configured for eastmoney collector")
		}
		application.RegisterCollector(maybeCache(eastmoneyCollector, cacheEnabled, cacheTTL))
	}

	// Register Crypto collector for digital assets
	if collectorCfg, ok := cfg.Collectors["crypto"]; ok && collectorCfg.Enabled {
		cryptoCollector := crypto.New()
		// Configure from config if available
		if collectorCfg.Extra != nil {
			cryptoCollector.Init(collector.Config{
				Enabled: true,
				Extra:   collectorCfg.Extra,
			})
		}
		application.RegisterCollector(maybeCache(cryptoCollector, cacheEnabled, cacheTTL))
		log.Info("crypto collector registered")
	}

	// Wire qlib warehouse collector after all external collectors are registered
	// so that SelectExternalForSymbol can resolve to them at runtime.
	// The returned db handle is shared with qlibpit below (single open, no double-open).
	qlibWarehouseDB, _ := wireQlibWarehouse(cfg.Qlib, application.CollectorRegistry(), func(p string) (*sql.DB, error) {
		return sql.Open("sqlite", "file:"+p+"?mode=ro")
	}, log)

	// Inject valuation/EPS sources used to assemble PE-percentile fundamentals.
	// Pass through the typed-nil guards so an unconfigured collector stays an
	// untyped-nil interface (see valuationSourceOrNil).
	// When the qlib warehouse is available, wrap the yahoo EPS source with the
	// PIT-correct qlibpit source (yahoo becomes the fallback, no recursive wrapping).
	var epsSrc app.EPSSource = epsSourceOrNil(yahooCollector)
	if qlibWarehouseDB != nil {
		epsSrc = qlibpit.New(qlibWarehouseDB, epsSrc)
		log.Info("qlib PIT EPS source enabled (yahoo fallback)")
	}
	application.SetValuationSources(valuationSourceOrNil(lixingerCollector), epsSrc)
	application.SetValuationLookback(cfg.Valuation.LookbackYears)

	// Valuation fields (PE/PB/dividend yield) currently come from lixinger
	// (A-shares). Guarded by the same typed-nil helper as valuationSourceOrNil so
	// an unconfigured collector stays an untyped-nil interface.
	application.SetFundamentalSource(fundamentalSourceOrNil(lixingerCollector))

	// cleanup closes the qlib warehouse handle shared with the qlibpit EPS
	// source; nil-safe so callers can defer it unconditionally.
	cleanup = func() {
		if qlibWarehouseDB != nil {
			_ = qlibWarehouseDB.Close()
		}
	}
	return cleanup, nil
}

// fundamentalSourceOrNil mirrors valuationSourceOrNil for the fundamental
// (PE/PB/dividend-yield) source. Returning a nil *lixinger.Lixinger directly
// would yield a non-nil interface wrapping a nil pointer (the typed-nil trap),
// defeating snapshotSymbol's `fundamentalSrc != nil` guard.
func fundamentalSourceOrNil(c *lixinger.Lixinger) app.FundamentalSource {
	if c == nil {
		return nil
	}
	return c
}
