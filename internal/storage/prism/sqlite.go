// Package prism provides the SQLite-backed store for Prism valuation data
// (design doc: docs/prism/atlas_prism_design.md §6, M1 subset).
package prism

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned by Series when the symbol has no instrument row.
var ErrNotFound = errors.New("prism: symbol not found")

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
CREATE TABLE IF NOT EXISTS fundamental_q (
  instrument_id  INTEGER NOT NULL REFERENCES instrument(id),
  fiscal_period  TEXT NOT NULL,
  period_end     TEXT NOT NULL,
  filing_date    TEXT NOT NULL,
  revenue        REAL,
  net_income     REAL,
  eps_diluted    REAL,
  equity         REAL,
  diluted_shares REAL,
  source         TEXT,
  gross_profit     REAL,
  rnd              REAL,
  sgna             REAL,
  operating_income REAL,
  income_tax       REAL,
  PRIMARY KEY (instrument_id, period_end)
) WITHOUT ROWID;
-- AD-9: PK 用 period_end(与 fundamental_q 同源的真实主键),不用 fiscal_period
-- ——后者只是展示标签,同 (fy,fp) 可能对应不同实际季度,作 PK 会静默覆盖数据。
CREATE TABLE IF NOT EXISTS segment_revenue (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  period_end    TEXT NOT NULL,
  fiscal_period TEXT,
  segment_key   TEXT NOT NULL,
  revenue       REAL,
  source        TEXT,
  PRIMARY KEY (instrument_id, period_end, segment_key)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS price_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d     TEXT NOT NULL,
  close REAL,
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;
`

// m3Columns are the fundamental_q columns introduced in M3. They are listed
// last in schema so a freshly created table and a migrated one share the same
// column order (ALTER TABLE ADD COLUMN always appends).
var m3Columns = []string{"gross_profit", "rnd", "sgna", "operating_income", "income_tax"}

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
	if err := execRetryBusy(db, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("prism: init schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("prism: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// isBusyErr reports whether err is SQLite's SQLITE_BUSY.
func isBusyErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}

// execRetryBusy runs an idempotent DDL batch, retrying while SQLite reports BUSY.
//
// The first statement on a fresh pool also establishes the connection, which is
// where the DSN's journal_mode(WAL) pragma runs. Converting a pre-WAL database to
// WAL needs exclusive access and — unlike ordinary writes — does not honour
// busy_timeout, so serve(常驻) 与 prism-daily(定时) 同时首次打开一个旧库时会有一方
// 直接拿到 SQLITE_BUSY(实测 8 路并发下约 1% Open 失败)。转换窗口极短,重试即可。
func execRetryBusy(db *sql.DB, stmt string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if _, err = db.Exec(stmt); err == nil || !isBusyErr(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return err
}

// isDuplicateColumnErr reports whether err is SQLite's "duplicate column name",
// i.e. the column we were about to add already exists.
func isDuplicateColumnErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}

// migrate adds the M3 columns to a fundamental_q table created before M3.
// Adding columns is the only schema change: existing columns and rows are left
// untouched, and SQLite guarantees an appended column reads as NULL for old rows.
func migrate(db *sql.DB) error {
	for _, col := range m3Columns {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('fundamental_q') WHERE name=?`, col).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		// AD-10: serve(常驻) 与 prism-daily(定时) 会并发 Open 同一库,上面的
		// check-then-act 存在竞态。另一进程抢先加列时 ALTER 报 duplicate column
		// ——视为已迁移继续,否则任一方的 Open 会失败(serve 起不来 / daily 全挂)。
		_, err := db.Exec(`ALTER TABLE fundamental_q ADD COLUMN ` + col + ` REAL`)
		if err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("add column %s: %w", col, err)
		}
	}
	return nil
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

// FundamentalRow is one fiscal quarter of EDGAR facts. NaN fields store as NULL.
type FundamentalRow struct {
	FiscalPeriod                                          string // "2026Q1"
	PeriodEnd                                             string // "YYYY-MM-DD"
	FilingDate                                            string // "YYYY-MM-DD"
	Revenue, NetIncome, EPSDiluted, Equity, DilutedShares float64
	GrossProfit, RnD, SGnA, OperatingIncome, IncomeTax    float64 // M3: 桑基主干流科目
	Source                                                string  // "edgar"
}

// SegmentRow is one reporting segment's revenue for one fiscal quarter.
// PeriodEnd is the real key; FiscalPeriod is a display label and may be empty (AD-9).
type SegmentRow struct {
	PeriodEnd    string // "YYYY-MM-DD"
	FiscalPeriod string // "2026Q1", 展示标签,可为空
	SegmentKey   string // 模板 key
	Revenue      float64
	Source       string // "edgar_segment" | "manual"
}

// PriceRow is one trading day's close. NaN stores as NULL.
type PriceRow struct {
	D     string // "YYYY-MM-DD"
	Close float64
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

// UpsertFundamentals writes rows in one transaction (idempotent per (id,period_end)).
func (s *Store) UpsertFundamentals(instrumentID int64, rows []FundamentalRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO fundamental_q
		(instrument_id,fiscal_period,period_end,filing_date,revenue,net_income,
		 eps_diluted,equity,diluted_shares,source,
		 gross_profit,rnd,sgna,operating_income,income_tax)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(instrument_id,period_end) DO UPDATE SET
		  fiscal_period=excluded.fiscal_period, filing_date=excluded.filing_date,
		  revenue=excluded.revenue, net_income=excluded.net_income,
		  eps_diluted=excluded.eps_diluted, equity=excluded.equity,
		  diluted_shares=excluded.diluted_shares, source=excluded.source,
		  gross_profit=excluded.gross_profit, rnd=excluded.rnd, sgna=excluded.sgna,
		  operating_income=excluded.operating_income, income_tax=excluded.income_tax`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(instrumentID, r.FiscalPeriod, r.PeriodEnd, r.FilingDate,
			toNull(r.Revenue), toNull(r.NetIncome), toNull(r.EPSDiluted),
			toNull(r.Equity), toNull(r.DilutedShares), r.Source,
			toNull(r.GrossProfit), toNull(r.RnD), toNull(r.SGnA),
			toNull(r.OperatingIncome), toNull(r.IncomeTax)); err != nil {
			return fmt.Errorf("prism: upsert fundamental %s: %w", r.PeriodEnd, err)
		}
	}
	return tx.Commit()
}

// QuarterlyFundamentals returns all fundamental rows of the instrument, period_end ascending.
func (s *Store) QuarterlyFundamentals(instrumentID int64) ([]FundamentalRow, error) {
	rows, err := s.db.Query(`SELECT fiscal_period,period_end,filing_date,revenue,
		net_income,eps_diluted,equity,diluted_shares,source,
		gross_profit,rnd,sgna,operating_income,income_tax
		FROM fundamental_q WHERE instrument_id=? ORDER BY period_end`, instrumentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FundamentalRow
	for rows.Next() {
		var r FundamentalRow
		var rev, ni, eps, eq, sh sql.NullFloat64
		var gp, rnd, sgna, opinc, tax sql.NullFloat64
		if err := rows.Scan(&r.FiscalPeriod, &r.PeriodEnd, &r.FilingDate,
			&rev, &ni, &eps, &eq, &sh, &r.Source,
			&gp, &rnd, &sgna, &opinc, &tax); err != nil {
			return nil, err
		}
		r.Revenue, r.NetIncome, r.EPSDiluted = fromNull(rev), fromNull(ni), fromNull(eps)
		r.Equity, r.DilutedShares = fromNull(eq), fromNull(sh)
		r.GrossProfit, r.RnD, r.SGnA = fromNull(gp), fromNull(rnd), fromNull(sgna)
		r.OperatingIncome, r.IncomeTax = fromNull(opinc), fromNull(tax)
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertSegments writes rows in one transaction (idempotent per
// (id,period_end,segment_key)); a manual row overwrites the fetched one.
func (s *Store) UpsertSegments(instrumentID int64, rows []SegmentRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO segment_revenue
		(instrument_id,period_end,fiscal_period,segment_key,revenue,source)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(instrument_id,period_end,segment_key) DO UPDATE SET
		  fiscal_period=excluded.fiscal_period, revenue=excluded.revenue,
		  source=excluded.source`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(instrumentID, r.PeriodEnd, r.FiscalPeriod, r.SegmentKey,
			toNull(r.Revenue), r.Source); err != nil {
			return fmt.Errorf("prism: upsert segment %s/%s: %w", r.PeriodEnd, r.SegmentKey, err)
		}
	}
	return tx.Commit()
}

// SegmentRows returns all segment rows of the instrument, (period_end, segment_key) ascending.
func (s *Store) SegmentRows(instrumentID int64) ([]SegmentRow, error) {
	rows, err := s.db.Query(`SELECT period_end,fiscal_period,segment_key,revenue,source
		FROM segment_revenue WHERE instrument_id=? ORDER BY period_end, segment_key`, instrumentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SegmentRow
	for rows.Next() {
		var r SegmentRow
		var fp sql.NullString
		var rev sql.NullFloat64
		if err := rows.Scan(&r.PeriodEnd, &fp, &r.SegmentKey, &rev, &r.Source); err != nil {
			return nil, err
		}
		r.FiscalPeriod, r.Revenue = fp.String, fromNull(rev)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestSegmentPeriodEnd returns the max stored segment period_end, "" when empty.
// This is the segment-refresh anchor; it deliberately does not join fundamental_q
// (AD-9), so a quarter without fundamentals cannot make the anchor regress.
func (s *Store) LatestSegmentPeriodEnd(instrumentID int64) (string, error) {
	var d sql.NullString
	err := s.db.QueryRow(`SELECT MAX(period_end) FROM segment_revenue WHERE instrument_id=?`,
		instrumentID).Scan(&d)
	if err != nil {
		return "", err
	}
	return d.String, nil
}

// UpsertPrices writes closes in one transaction (idempotent per (id,d)).
func (s *Store) UpsertPrices(instrumentID int64, closes []PriceRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO price_daily (instrument_id,d,close) VALUES(?,?,?)
		ON CONFLICT(instrument_id,d) DO UPDATE SET close=excluded.close`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range closes {
		if _, err := stmt.Exec(instrumentID, r.D, toNull(r.Close)); err != nil {
			return fmt.Errorf("prism: upsert price %s: %w", r.D, err)
		}
	}
	return tx.Commit()
}

// PriceSeries returns the close series of symbol with d >= from (from=""=all),
// dates ascending. Unknown symbols yield ErrNotFound, as in Series.
func (s *Store) PriceSeries(symbol, from string) ([]string, []float64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM instrument WHERE symbol=?`, symbol).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("prism: query symbol %s: %w", symbol, err)
	}
	rows, err := s.db.Query(`SELECT d, close FROM price_daily
		WHERE instrument_id=? AND (?='' OR d>=?) ORDER BY d`, id, from, from)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var dates []string
	var closes []float64
	for rows.Next() {
		var d string
		var c sql.NullFloat64
		if err := rows.Scan(&d, &c); err != nil {
			return nil, nil, err
		}
		dates = append(dates, d)
		closes = append(closes, fromNull(c))
	}
	return dates, closes, rows.Err()
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if err != nil {
		return nil, fmt.Errorf("prism: query symbol %s: %w", symbol, err)
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
