package main

import (
	"database/sql"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/qlib"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// wireQlibWarehouse opens the qlib SQLite warehouse, registers the collector
// into reg, and returns the opened *sql.DB so the caller can share the same
// read-only handle for additional purposes (e.g. qlibpit EPS source).
//
// The openFn parameter is injected so callers can substitute a real sql.Open
// (production) or a test double (unit tests) without touching the file system.
//
// Degradation behaviour (returns (nil, false) and skips registration):
//   - cfg.Enabled == false or cfg.DBPath == "" → skip silently.
//   - openFn returns an error → log.Warn + skip.
//   - db.Ping() fails → log.Warn + close DB + skip.
//
// On success the same *sql.DB is both registered inside qlib.New and returned
// to the caller; the DB is opened exactly once (no double-open).
func wireQlibWarehouse(cfg config.QlibConfig, reg *collector.Registry, openFn func(dbPath string) (*sql.DB, error), log *zap.Logger) (*sql.DB, bool) {
	if !cfg.Enabled || cfg.DBPath == "" {
		return nil, false
	}

	db, err := openFn(cfg.DBPath)
	if err != nil {
		log.Warn("qlib warehouse open failed, skipping", zap.Error(err))
		return nil, false
	}

	if err := db.Ping(); err != nil {
		log.Warn("qlib warehouse ping failed, skipping", zap.Error(err))
		db.Close()
		return nil, false
	}

	stale := time.Duration(cfg.MaxStalenessDays) * 24 * time.Hour
	if stale == 0 {
		stale = 7 * 24 * time.Hour
	}

	// Recycle pooled connections so a rebuilt warehouse is picked up without
	// restarting atlas; <=0 falls back to the collector default.
	connLifetime := cfg.ConnMaxLifetime
	if connLifetime <= 0 {
		connLifetime = qlib.DefaultConnMaxLifetime
	}

	reg.Register(qlib.New(db,
		qlib.WithMaxStaleness(stale),
		qlib.WithConnMaxLifetime(connLifetime),
		qlib.WithExternal(func(s string) collector.Collector {
			return collector.SelectExternalForSymbol(reg, s)
		}),
	))

	log.Info("qlib warehouse collector registered",
		zap.String("db_path", cfg.DBPath),
		zap.Duration("max_staleness", stale),
		zap.Duration("conn_max_lifetime", connLifetime),
	)
	return db, true
}
