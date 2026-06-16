// Package qlibpit serves point-in-time EPS(TTM) history from the local SQLite
// warehouse (fundamentals_pit). It implements app.EPSSource. Each row's
// observe_date is the date the value became publicly known, so querying
// observe_date <= window-end eliminates the look-ahead bias present in the
// report-period-dated Yahoo path. Falls back to an inner EPSSource when the
// warehouse has no fundamentals for a symbol.
package qlibpit

import (
	"database/sql"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

const dateFmt = "2006-01-02"

// EPSSource is the fallback shape (satisfied by *yahoo.Yahoo); mirrors
// app.EPSSource without importing the app package.
type EPSSource interface {
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}

// Source reads PIT EPS history from the warehouse, delegating to a fallback
// when a symbol has no fundamentals stored.
type Source struct {
	db       *sql.DB
	fallback EPSSource
}

// New builds a PIT EPS source. fallback may be nil.
func New(db *sql.DB, fallback EPSSource) *Source {
	return &Source{db: db, fallback: fallback}
}

func (s *Source) hasFundamentals(symbol string) bool {
	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM fundamentals_pit WHERE symbol=?", symbol,
	).Scan(&n)
	return err == nil && n > 0
}

// FetchEPSHistory returns the PIT EPS(TTM) series for symbol, ascending by
// observe_date, including only points knowable on or before end.
func (s *Source) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	symbol = strings.ToUpper(symbol)
	if !s.hasFundamentals(symbol) {
		if s.fallback != nil {
			return s.fallback.FetchEPSHistory(symbol, start, end)
		}
		return []core.EPSPoint{}, nil
	}
	rows, err := s.db.Query(
		"SELECT observe_date,eps_ttm FROM fundamentals_pit "+
			"WHERE symbol=? AND observe_date<=? AND eps_ttm IS NOT NULL ORDER BY observe_date",
		symbol, end.Format(dateFmt),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.EPSPoint
	for rows.Next() {
		var od string
		var eps float64
		if err := rows.Scan(&od, &eps); err != nil {
			return nil, err
		}
		date, err := time.Parse(dateFmt, od)
		if err != nil {
			continue
		}
		out = append(out, core.EPSPoint{Date: date, EPS: eps})
	}
	return out, rows.Err()
}
