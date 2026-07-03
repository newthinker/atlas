package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/newthinker/atlas/internal/core"

	_ "modernc.org/sqlite"
)

// timeLayout stores generated_at as fixed-width UTC RFC3339 with nanosecond
// precision so lexicographic ORDER BY / range comparisons on the TEXT column
// equal chronological order (variable-width RFC3339Nano would not sort right).
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

const schema = `
CREATE TABLE IF NOT EXISTS signals (
	id           TEXT PRIMARY KEY,
	symbol       TEXT NOT NULL,
	action       TEXT NOT NULL,
	confidence   REAL,
	price        REAL,
	reason       TEXT,
	strategy     TEXT,
	metadata     TEXT,
	generated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_signals_symbol_time ON signals(symbol, generated_at);
CREATE INDEX IF NOT EXISTS idx_signals_time ON signals(generated_at);`

// SQLiteStore is a persistent signal Store backed by modernc.org/sqlite.
type SQLiteStore struct {
	db      *sql.DB
	counter atomic.Int64
}

// NewSQLiteStore opens (creating parent dirs and the schema if needed) a sqlite
// signal store at path. The connection runs in WAL mode with a busy_timeout so
// concurrent readers/writers coordinate via the database/sql pool. It returns an
// error (never panics) when the path is unusable, so callers can fail fast.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating signal db dir: %w", err)
		}
	}

	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening signal db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting signal db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating signals schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close releases the underlying database handle.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Save persists a signal, assigning a sig_<unixnano>_<counter> ID (same scheme
// as the in-memory store).
func (s *SQLiteStore) Save(ctx context.Context, signal core.Signal) error {
	signal.ID = fmt.Sprintf("sig_%d_%d", time.Now().UnixNano(), s.counter.Add(1))

	meta, err := json.Marshal(signal.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling signal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO signals (id, symbol, action, confidence, price, reason, strategy, metadata, generated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		signal.ID, signal.Symbol, string(signal.Action), signal.Confidence, signal.Price,
		signal.Reason, signal.Strategy, string(meta), signal.GeneratedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("inserting signal: %w", err)
	}
	return nil
}

// GetByID retrieves a signal by ID, returning core.ErrSymbolNotFound when absent.
func (s *SQLiteStore) GetByID(ctx context.Context, id string) (*core.Signal, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, symbol, action, confidence, price, reason, strategy, metadata, generated_at
		 FROM signals WHERE id = ?`, id)

	sig, err := scanSignal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrSymbolNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sig, nil
}

// List retrieves signals matching filter, ordered generated_at ASC, id ASC.
func (s *SQLiteStore) List(ctx context.Context, filter ListFilter) ([]core.Signal, error) {
	where, args := buildWhere(filter)
	query := `SELECT id, symbol, action, confidence, price, reason, strategy, metadata, generated_at
		 FROM signals` + where + ` ORDER BY generated_at ASC, id ASC`

	// limit=0 means unlimited; sqlite needs LIMIT -1 to apply a bare OFFSET.
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	} else if filter.Offset > 0 {
		query += " LIMIT -1 OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing signals: %w", err)
	}
	defer rows.Close()

	result := []core.Signal{}
	for rows.Next() {
		sig, err := scanSignal(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sig)
	}
	return result, rows.Err()
}

// Count returns the number of signals matching filter (ignoring limit/offset).
func (s *SQLiteStore) Count(ctx context.Context, filter ListFilter) (int, error) {
	where, args := buildWhere(filter)
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM signals`+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting signals: %w", err)
	}
	return n, nil
}

// buildWhere renders the shared filter. From/To are inclusive endpoints to match
// MemoryStore.matches (Before(From)/After(To)); see discovery for the DoD wording
// note ("开区间") that this deliberately mirrors memory instead.
func buildWhere(filter ListFilter) (string, []any) {
	var conds []string
	var args []any
	if filter.Symbol != "" {
		conds = append(conds, "symbol = ?")
		args = append(args, filter.Symbol)
	}
	if filter.Strategy != "" {
		conds = append(conds, "strategy = ?")
		args = append(args, filter.Strategy)
	}
	if filter.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, string(filter.Action))
	}
	if !filter.From.IsZero() {
		conds = append(conds, "generated_at >= ?")
		args = append(args, filter.From.UTC().Format(timeLayout))
	}
	if !filter.To.IsZero() {
		conds = append(conds, "generated_at <= ?")
		args = append(args, filter.To.UTC().Format(timeLayout))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// scanner abstracts *sql.Row and *sql.Rows so one scan routine serves both.
type scanner interface {
	Scan(dest ...any) error
}

func scanSignal(sc scanner) (core.Signal, error) {
	var (
		sig    core.Signal
		action string
		meta   string
		genAt  string
	)
	if err := sc.Scan(&sig.ID, &sig.Symbol, &action, &sig.Confidence, &sig.Price,
		&sig.Reason, &sig.Strategy, &meta, &genAt); err != nil {
		return core.Signal{}, err
	}

	sig.Action = core.Action(action)
	if err := json.Unmarshal([]byte(meta), &sig.Metadata); err != nil {
		return core.Signal{}, fmt.Errorf("unmarshaling signal metadata: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, genAt)
	if err != nil {
		return core.Signal{}, fmt.Errorf("parsing generated_at: %w", err)
	}
	sig.GeneratedAt = t
	return sig, nil
}
