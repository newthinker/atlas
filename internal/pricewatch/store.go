package pricewatch

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/newthinker/atlas/internal/core"
)

// Store 是 pricewatch 的 SQLite 存储：K 线、规则状态、kv（spec §6）。
// 时间一律存为 UTC Unix 毫秒，读回时为 UTC。
type Store struct {
	db *sql.DB
}

// CandleStats 是 [from, to] 内 K 线的 max(high)、min(low)、最早一根的时间与根数。
type CandleStats struct {
	High, Low float64
	First     time.Time
	Count     int
}

const storeSchema = `
CREATE TABLE IF NOT EXISTS candles (
	pair  TEXT    NOT NULL,
	bar   TEXT    NOT NULL,
	ts    INTEGER NOT NULL,
	open  REAL    NOT NULL,
	high  REAL    NOT NULL,
	low   REAL    NOT NULL,
	close REAL    NOT NULL,
	PRIMARY KEY (pair, bar, ts)
);
CREATE TABLE IF NOT EXISTS rule_state (
	pair         TEXT    NOT NULL,
	rule_id      TEXT    NOT NULL,
	tier         INTEGER NOT NULL,
	sent_in_tier INTEGER NOT NULL,
	last_sent_ms INTEGER NOT NULL,
	PRIMARY KEY (pair, rule_id)
);
CREATE TABLE IF NOT EXISTS kv (
	key TEXT PRIMARY KEY,
	val TEXT NOT NULL
);`

// NewStore 打开（必要时创建）path 处的库；DSN 与建目录写法同 crisis.NewStore。
func NewStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating pricewatch db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening pricewatch db: %w", err)
	}
	if _, err := db.Exec(storeSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating pricewatch schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func fromMs(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// UpsertCandles 按主键 (pair, bar, ts) 幂等写入；重复的 ts 以新值覆盖。
func (s *Store) UpsertCandles(pair, bar string, cs []core.OHLCV) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("upsert candles: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO candles (pair, bar, ts, open, high, low, close) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("upsert candles: %w", err)
	}
	defer stmt.Close()
	for _, c := range cs {
		if _, err := stmt.Exec(pair, bar, c.Time.UnixMilli(), c.Open, c.High, c.Low, c.Close); err != nil {
			return fmt.Errorf("upsert candle %s/%s@%s: %w", pair, bar, c.Time.UTC().Format(time.RFC3339), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("upsert candles: %w", err)
	}
	return nil
}

// LatestCandle 返回最新一根的时间；没有 K 线时 ok=false。
func (s *Store) LatestCandle(pair, bar string) (time.Time, bool, error) {
	var ms sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(ts) FROM candles WHERE pair = ? AND bar = ?`, pair, bar).Scan(&ms)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("latest candle %s/%s: %w", pair, bar, err)
	}
	if !ms.Valid {
		return time.Time{}, false, nil
	}
	return fromMs(ms.Int64), true, nil
}

// Stats 统计 [from, to]（两端含）内的 K 线；from 为零值表示不设下界。
func (s *Store) Stats(pair, bar string, from, to time.Time) (CandleStats, error) {
	// 零值 time.Time 的 UnixMilli 远早于任何 K 线，天然等于不设下界
	var (
		high, low sql.NullFloat64
		first     sql.NullInt64
		st        CandleStats
	)
	err := s.db.QueryRow(
		`SELECT MAX(high), MIN(low), MIN(ts), COUNT(*) FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts <= ?`,
		pair, bar, from.UnixMilli(), to.UnixMilli(),
	).Scan(&high, &low, &first, &st.Count)
	if err != nil {
		return CandleStats{}, fmt.Errorf("candle stats %s/%s: %w", pair, bar, err)
	}
	if st.Count > 0 {
		st.High, st.Low, st.First = high.Float64, low.Float64, fromMs(first.Int64)
	}
	return st, nil
}

// FirstCloseSince 返回 ts >= from 的最早一根的收盘价；没有时 ok=false。
func (s *Store) FirstCloseSince(pair, bar string, from time.Time) (float64, bool, error) {
	var c float64
	err := s.db.QueryRow(
		`SELECT close FROM candles WHERE pair = ? AND bar = ? AND ts >= ? ORDER BY ts LIMIT 1`,
		pair, bar, from.UnixMilli(),
	).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("first close %s/%s: %w", pair, bar, err)
	}
	return c, true, nil
}

// PruneCandles 删除 ts < before 的 K 线。
func (s *Store) PruneCandles(pair, bar string, before time.Time) error {
	if _, err := s.db.Exec(`DELETE FROM candles WHERE pair = ? AND bar = ? AND ts < ?`, pair, bar, before.UnixMilli()); err != nil {
		return fmt.Errorf("prune candles %s/%s: %w", pair, bar, err)
	}
	return nil
}

// GetRuleState 读取规则状态；不存在时返回零值。
func (s *Store) GetRuleState(pair, ruleID string) (RuleState, error) {
	var (
		st RuleState
		ms int64
	)
	err := s.db.QueryRow(
		`SELECT tier, sent_in_tier, last_sent_ms FROM rule_state WHERE pair = ? AND rule_id = ?`, pair, ruleID,
	).Scan(&st.Tier, &st.SentInTier, &ms)
	if errors.Is(err, sql.ErrNoRows) {
		return RuleState{}, nil
	}
	if err != nil {
		return RuleState{}, fmt.Errorf("get rule state %s/%s: %w", pair, ruleID, err)
	}
	// 零值 LastSentAt 按 UnixMilli 原样往返，读回仍 IsZero
	st.LastSentAt = fromMs(ms)
	return st, nil
}

func (s *Store) PutRuleState(pair, ruleID string, st RuleState) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO rule_state (pair, rule_id, tier, sent_in_tier, last_sent_ms) VALUES (?, ?, ?, ?, ?)`,
		pair, ruleID, st.Tier, st.SentInTier, st.LastSentAt.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("put rule state %s/%s: %w", pair, ruleID, err)
	}
	return nil
}

// GetKV 读取键值；不存在时返回 ""。
func (s *Store) GetKV(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT val FROM kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get kv %q: %w", key, err)
	}
	return v, nil
}

func (s *Store) PutKV(key, val string) error {
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO kv (key, val) VALUES (?, ?)`, key, val); err != nil {
		return fmt.Errorf("put kv %q: %w", key, err)
	}
	return nil
}
