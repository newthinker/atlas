package crisis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS macro_observations (
	ts          TEXT NOT NULL,
	indicator   TEXT NOT NULL,
	value       REAL,
	source      TEXT,
	fetched_at  TEXT,
	PRIMARY KEY (ts, indicator)
);
CREATE TABLE IF NOT EXISTS crisis_evaluations (
	ts            TEXT NOT NULL,
	eval_at       TEXT NOT NULL,
	indicator     TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT '',
	tag           TEXT NOT NULL DEFAULT '',
	value         REAL NOT NULL DEFAULT 0,
	pct_5y        REAL NOT NULL DEFAULT 0,
	system_state  TEXT NOT NULL DEFAULT '',
	detail        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_macro_obs_ind_ts   ON macro_observations(indicator, ts);
CREATE INDEX IF NOT EXISTS idx_crisis_eval_ind_ts ON crisis_evaluations(indicator, ts);`

// Store is the sqlite-backed source of truth for observations and
// evaluations (WAL + busy_timeout, same conventions as storage/signal).
type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating crisis db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening crisis db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting crisis db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating crisis schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// UpsertObservations writes obs in one transaction; the (ts, indicator)
// primary key makes rewrites overwrite, so backfill and repeated daily
// wakeups are idempotent by construction.
func (s *Store) UpsertObservations(ctx context.Context, obs []Observation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO macro_observations (ts, indicator, value, source, fetched_at)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing upsert: %w", err)
	}
	defer stmt.Close()
	for _, o := range obs {
		if _, err := stmt.ExecContext(ctx, o.Date, o.Indicator, o.Value, o.Source, o.FetchedAt); err != nil {
			return fmt.Errorf("upserting %s/%s: %w", o.Indicator, o.Date, err)
		}
	}
	return tx.Commit()
}

const obsSelect = `SELECT ts, indicator, value, source, fetched_at FROM macro_observations`

func (s *Store) Observation(ctx context.Context, indicator, date string) (*Observation, error) {
	return scanMaybeObservation(s.db.QueryRowContext(ctx,
		obsSelect+` WHERE indicator = ? AND ts = ?`, indicator, date))
}

func (s *Store) LatestObservation(ctx context.Context, indicator string) (*Observation, error) {
	return scanMaybeObservation(s.db.QueryRowContext(ctx,
		obsSelect+` WHERE indicator = ? ORDER BY ts DESC LIMIT 1`, indicator))
}

// scanMaybeObservation scans a single-row query, mapping ErrNoRows to (nil, nil).
func scanMaybeObservation(sc scanner) (*Observation, error) {
	o, err := scanObservation(sc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SeriesWindow returns最近 n 条 ts<=end 的观测（升序）：DESC LIMIT 取窗再反转。
func (s *Store) SeriesWindow(ctx context.Context, indicator, end string, n int) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		obsSelect+` WHERE indicator = ? AND ts <= ? ORDER BY ts DESC LIMIT ?`, indicator, end, n)
	if err != nil {
		return nil, fmt.Errorf("querying window: %w", err)
	}
	out, err := collectObservations(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) SeriesSince(ctx context.Context, indicator, from, end string) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		obsSelect+` WHERE indicator = ? AND ts >= ? AND ts <= ? ORDER BY ts ASC`, indicator, from, end)
	if err != nil {
		return nil, fmt.Errorf("querying range: %w", err)
	}
	return collectObservations(rows)
}

// EvalDates returns vix 的观测日序列（回测的评估日历——vix 覆盖全部验收时段）。
func (s *Store) EvalDates(ctx context.Context, from, to string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts FROM macro_observations WHERE indicator = ? AND ts >= ? AND ts <= ? ORDER BY ts ASC`,
		IndVIX, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying eval dates: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AppendEvaluations(ctx context.Context, evals []Evaluation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO crisis_evaluations (ts, eval_at, indicator, status, tag, value, pct_5y, system_state, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing eval insert: %w", err)
	}
	defer stmt.Close()
	for _, e := range evals {
		if _, err := stmt.ExecContext(ctx, e.TS, e.EvalAt, e.Indicator, string(e.Status),
			string(e.Tag), e.Value, e.Pct5y, string(e.SystemState), e.Detail); err != nil {
			return fmt.Errorf("inserting evaluation %s/%s: %w", e.TS, e.Indicator, err)
		}
	}
	return tx.Commit()
}

const evalSelect = `SELECT ts, eval_at, indicator, status, tag, value, pct_5y, system_state, detail
	FROM crisis_evaluations`

func (s *Store) RecentSystemEvals(ctx context.Context, n int) ([]Evaluation, error) {
	rows, err := s.db.QueryContext(ctx,
		evalSelect+` WHERE indicator = '' ORDER BY ts DESC LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("querying system evals: %w", err)
	}
	return collectEvaluations(rows)
}

func (s *Store) RecentIndicatorEvals(ctx context.Context, indicator string, n int) ([]Evaluation, error) {
	rows, err := s.db.QueryContext(ctx,
		evalSelect+` WHERE indicator = ? ORDER BY ts DESC LIMIT ?`, indicator, n)
	if err != nil {
		return nil, fmt.Errorf("querying indicator evals: %w", err)
	}
	return collectEvaluations(rows)
}

func (s *Store) LatestSystemEval(ctx context.Context) (*Evaluation, error) {
	evals, err := s.RecentSystemEvals(ctx, 1)
	if err != nil || len(evals) == 0 {
		return nil, err
	}
	return &evals[0], nil
}

func (s *Store) HasSystemEvalForDate(ctx context.Context, date string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crisis_evaluations WHERE indicator = '' AND ts = ?`, date).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking eval for %s: %w", date, err)
	}
	return n > 0, nil
}

// Reader / History bind a context so the pure engine interfaces stay
// context-free (replay and tests use in-memory implementations instead).
func (s *Store) Reader(ctx context.Context) SeriesReader { return storeReader{ctx, s} }
func (s *Store) History(ctx context.Context) EvalHistory { return storeHistory{ctx, s} }

type storeReader struct {
	ctx context.Context
	s   *Store
}

func (r storeReader) Window(indicator, end string, n int) ([]Observation, error) {
	return r.s.SeriesWindow(r.ctx, indicator, end, n)
}

func (r storeReader) WindowSince(indicator, from, end string) ([]Observation, error) {
	return r.s.SeriesSince(r.ctx, indicator, from, end)
}

type storeHistory struct {
	ctx context.Context
	s   *Store
}

func (h storeHistory) RecentSystem(n int) ([]Evaluation, error) {
	return h.s.RecentSystemEvals(h.ctx, n)
}

func (h storeHistory) RecentIndicator(indicator string, n int) ([]Evaluation, error) {
	return h.s.RecentIndicatorEvals(h.ctx, indicator, n)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanObservation(sc scanner) (Observation, error) {
	var o Observation
	if err := sc.Scan(&o.Date, &o.Indicator, &o.Value, &o.Source, &o.FetchedAt); err != nil {
		return Observation{}, err
	}
	return o, nil
}

func collectObservations(rows *sql.Rows) ([]Observation, error) {
	defer rows.Close()
	out := []Observation{}
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func collectEvaluations(rows *sql.Rows) ([]Evaluation, error) {
	defer rows.Close()
	out := []Evaluation{}
	for rows.Next() {
		var (
			e      Evaluation
			status string
			tag    string
			state  string
		)
		if err := rows.Scan(&e.TS, &e.EvalAt, &e.Indicator, &status, &tag,
			&e.Value, &e.Pct5y, &state, &e.Detail); err != nil {
			return nil, err
		}
		e.Status, e.Tag, e.SystemState = Status(status), Tag(tag), SystemState(state)
		out = append(out, e)
	}
	return out, rows.Err()
}
