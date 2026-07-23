// Package prism provides the SQLite-backed store for Prism valuation data
// (design doc: docs/prism/atlas_prism_design.md §6, M1 subset).
package prism

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS instrument (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  symbol  TEXT NOT NULL,
  type    TEXT NOT NULL,
  market  TEXT NOT NULL,
  name    TEXT,
  grp     TEXT,
  source  TEXT NOT NULL,
  UNIQUE(symbol, type)
);
CREATE TABLE IF NOT EXISTS valuation_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d        TEXT NOT NULL,
  pe_ttm   REAL,
  pb       REAL,
  ps_ttm   REAL,
  pctl_5y  REAL,
  pctl_10y REAL,
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;
`

// Store is a SQLite-backed Prism store. Safe for concurrent use via WAL.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the Prism database at path, creating any
// missing parent directories along the way.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("prism: create db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("prism: open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("prism: init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Instrument is one row of the instrument table.
type Instrument struct {
	Symbol, Type, Market, Name, Group, Source string
}

// ValuationRow is one day of valuation metrics. NaN fields are stored as NULL.
type ValuationRow struct {
	D                                 string
	PETTM, PB, PSTTM, Pctl5Y, Pctl10Y float64
}

// BoardRow is the latest valuation row of one instrument, for the board page.
type BoardRow struct {
	Instrument
	AsOf                       string
	PETTM, PB, Pctl5Y, Pctl10Y float64
}

// SeriesData is the full (or from-filtered) valuation series of one symbol.
type SeriesData struct {
	Symbol, Name, Source   string
	Dates                  []string
	PETTM, Pctl5Y, Pctl10Y []float64
}

// UpsertInstrument inserts or updates by (symbol,type) and returns the row id.
func (s *Store) UpsertInstrument(inst Instrument) (int64, error) {
	_, err := s.db.Exec(`INSERT INTO instrument(symbol,type,market,name,grp,source)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(symbol,type) DO UPDATE SET market=excluded.market,
		  name=excluded.name, grp=excluded.grp, source=excluded.source`,
		inst.Symbol, inst.Type, inst.Market, inst.Name, inst.Group, inst.Source)
	if err != nil {
		return 0, fmt.Errorf("prism: upsert instrument %s: %w", inst.Symbol, err)
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM instrument WHERE symbol=? AND type=?`,
		inst.Symbol, inst.Type).Scan(&id)
	return id, err
}

// LatestDate returns the max stored date for the instrument, "" when empty.
// This is the incremental-fetch anchor (理杏豆计费:只拉增量).
func (s *Store) LatestDate(instrumentID int64) (string, error) {
	var d sql.NullString
	err := s.db.QueryRow(`SELECT MAX(d) FROM valuation_daily WHERE instrument_id=?`,
		instrumentID).Scan(&d)
	if err != nil {
		return "", err
	}
	return d.String, nil
}

func toNull(v float64) any {
	if math.IsNaN(v) {
		return nil
	}
	return v
}

func fromNull(v sql.NullFloat64) float64 {
	if !v.Valid {
		return math.NaN()
	}
	return v.Float64
}

// UpsertValuations writes rows in one transaction (idempotent per (id,d)).
func (s *Store) UpsertValuations(instrumentID int64, rows []ValuationRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO valuation_daily
		(instrument_id,d,pe_ttm,pb,ps_ttm,pctl_5y,pctl_10y) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(instrument_id,d) DO UPDATE SET pe_ttm=excluded.pe_ttm,
		  pb=excluded.pb, ps_ttm=excluded.ps_ttm,
		  pctl_5y=excluded.pctl_5y, pctl_10y=excluded.pctl_10y`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(instrumentID, r.D, toNull(r.PETTM), toNull(r.PB),
			toNull(r.PSTTM), toNull(r.Pctl5Y), toNull(r.Pctl10Y)); err != nil {
			return fmt.Errorf("prism: upsert valuation %s: %w", r.D, err)
		}
	}
	return tx.Commit()
}

// Board returns each instrument joined with its latest valuation row.
func (s *Store) Board() ([]BoardRow, error) {
	rows, err := s.db.Query(`
		SELECT i.symbol, i.type, i.market, i.name, i.grp, i.source,
		       v.d, v.pe_ttm, v.pb, v.pctl_5y, v.pctl_10y
		FROM instrument i
		JOIN valuation_daily v ON v.instrument_id = i.id
		 AND v.d = (SELECT MAX(d) FROM valuation_daily WHERE instrument_id = i.id)
		ORDER BY i.grp, i.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BoardRow
	for rows.Next() {
		var r BoardRow
		var pe, pb, p5, p10 sql.NullFloat64
		if err := rows.Scan(&r.Symbol, &r.Type, &r.Market, &r.Name, &r.Group,
			&r.Source, &r.AsOf, &pe, &pb, &p5, &p10); err != nil {
			return nil, err
		}
		r.PETTM, r.PB, r.Pctl5Y, r.Pctl10Y = fromNull(pe), fromNull(pb), fromNull(p5), fromNull(p10)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Series returns the valuation series of symbol with d >= from (from=""=all).
func (s *Store) Series(symbol, from string) (*SeriesData, error) {
	sd := &SeriesData{Symbol: symbol}
	err := s.db.QueryRow(`SELECT name, source FROM instrument WHERE symbol=?`,
		symbol).Scan(&sd.Name, &sd.Source)
	if err != nil {
		return nil, fmt.Errorf("prism: unknown symbol %s: %w", symbol, err)
	}
	rows, err := s.db.Query(`
		SELECT v.d, v.pe_ttm, v.pctl_5y, v.pctl_10y
		FROM valuation_daily v JOIN instrument i ON i.id = v.instrument_id
		WHERE i.symbol=? AND (?='' OR v.d>=?) ORDER BY v.d`, symbol, from, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var pe, p5, p10 sql.NullFloat64
		if err := rows.Scan(&d, &pe, &p5, &p10); err != nil {
			return nil, err
		}
		sd.Dates = append(sd.Dates, d)
		sd.PETTM = append(sd.PETTM, fromNull(pe))
		sd.Pctl5Y = append(sd.Pctl5Y, fromNull(p5))
		sd.Pctl10Y = append(sd.Pctl10Y, fromNull(p10))
	}
	return sd, rows.Err()
}
