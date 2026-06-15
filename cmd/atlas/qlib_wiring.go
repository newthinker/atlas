package main

import (
	"database/sql"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/qlib"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// wireQlibWarehouse opens the qlib SQLite warehouse and registers the collector
// into reg. It returns true when the collector is successfully registered.
//
// The openFn parameter is injected so callers can substitute a real sql.Open
// (production) or a test double (unit tests) without touching the file system.
//
// Degradation behaviour (returns false and skips registration):
//   - cfg.Enabled == false or cfg.DBPath == "" → skip silently.
//   - openFn returns an error → log.Warn + skip.
//   - db.Ping() fails → log.Warn + close DB + skip.
func wireQlibWarehouse(cfg config.QlibConfig, reg *collector.Registry, openFn func(dbPath string) (*sql.DB, error), log *zap.Logger) bool {
	if !cfg.Enabled || cfg.DBPath == "" {
		return false
	}

	db, err := openFn(cfg.DBPath)
	if err != nil {
		log.Warn("qlib warehouse open failed, skipping", zap.Error(err))
		return false
	}

	if err := db.Ping(); err != nil {
		log.Warn("qlib warehouse ping failed, skipping", zap.Error(err))
		db.Close()
		return false
	}

	stale := time.Duration(cfg.MaxStalenessDays) * 24 * time.Hour
	if stale == 0 {
		stale = 7 * 24 * time.Hour
	}

	reg.Register(qlib.New(db,
		qlib.WithMaxStaleness(stale),
		qlib.WithExternal(func(s string) collector.Collector {
			return collector.SelectExternalForSymbol(reg, s)
		}),
	))

	log.Info("qlib warehouse collector registered",
		zap.String("db_path", cfg.DBPath),
		zap.Duration("max_staleness", stale),
	)
	return true
}
