package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0]     乱序写两次 + 同 ts 更高 high → Count==3、High 取新值        → TestUpsertCandles_Idempotent
// functional[1]     [from,to] 两端含、from 零值、空区间、First、取 high/low       → TestStats_Range
// functional[2]     Put→Close→NewStore→Get 相等（毫秒）；不存在返回零值          → TestRuleState_SurvivesReopen
// functional[3]     Prune / KV / LatestCandle 空表 / FirstCloseSince             → TestPruneCandles、TestKV、TestLatestCandle、TestFirstCloseSince
// boundary[0]       pair、bar、rule_id 互相隔离                                  → TestStore_Isolation
// boundary[1]       非 UTC Location 写入后读回 Equal（毫秒截断）                  → TestStore_NonUTCRoundTrip
// error_handling[0] 自动建父目录；父路径是普通文件时返回 error                    → TestNewStore_CreatesDir、TestNewStore_BadPath
// non_functional[0] 并发 Upsert/Stats/PutRuleState 无 race、无 database is locked → TestStore_Concurrent

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

var storeT0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pw.db")
	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func day(i int) time.Time { return storeT0.AddDate(0, 0, i) }

func candle(ts time.Time, h, l, c float64) core.OHLCV {
	return core.OHLCV{Time: ts, Open: c, High: h, Low: l, Close: c}
}

func mustUpsert(t *testing.T, s *Store, pair, bar string, cs ...core.OHLCV) {
	t.Helper()
	if err := s.UpsertCandles(pair, bar, cs); err != nil {
		t.Fatalf("UpsertCandles: %v", err)
	}
}

func mustStats(t *testing.T, s *Store, pair, bar string, from, to time.Time) CandleStats {
	t.Helper()
	st, err := s.Stats(pair, bar, from, to)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	return st
}

func TestUpsertCandles_Idempotent(t *testing.T) {
	s, _ := newTestStore(t)
	batch := []core.OHLCV{candle(day(2), 120, 90, 100), candle(day(0), 110, 95, 100), candle(day(1), 105, 70, 100)}
	mustUpsert(t, s, "BTCUSDT", "1d", batch...)
	mustUpsert(t, s, "BTCUSDT", "1d", batch...)

	st := mustStats(t, s, "BTCUSDT", "1d", time.Time{}, day(10))
	if st.Count != 3 || st.High != 120 || st.Low != 70 || !st.First.Equal(day(0)) {
		t.Fatalf("after double write: %+v", st)
	}

	mustUpsert(t, s, "BTCUSDT", "1d", candle(day(1), 150, 70, 100))
	st = mustStats(t, s, "BTCUSDT", "1d", time.Time{}, day(10))
	if st.Count != 3 || st.High != 150 {
		t.Fatalf("after replace: want Count=3 High=150, got %+v", st)
	}
}

func TestStats_Range(t *testing.T) {
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "1d",
		candle(day(0), 300, 1, 100),  // 区间外（早）
		candle(day(1), 120, 80, 100), // 区间左端
		candle(day(2), 110, 90, 100),
		candle(day(3), 115, 85, 100), // 区间右端
		candle(day(4), 500, 2, 100),  // 区间外（晚）
	)

	cases := []struct {
		name     string
		from, to time.Time
		want     CandleStats
	}{
		{"两端含", day(1), day(3), CandleStats{High: 120, Low: 80, First: day(1), Count: 3}},
		{"单根取 high/low 非 close", day(1), day(1), CandleStats{High: 120, Low: 80, First: day(1), Count: 1}},
		{"from 零值统计全部", time.Time{}, day(4), CandleStats{High: 500, Low: 1, First: day(0), Count: 5}},
		{"from 零值仍受 to 约束", time.Time{}, day(2), CandleStats{High: 300, Low: 1, First: day(0), Count: 3}},
		{"空区间", day(5), day(9), CandleStats{}},
		{"区间夹在两根之间", day(1).Add(time.Hour), day(2).Add(-time.Hour), CandleStats{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustStats(t, s, "BTCUSDT", "1d", c.from, c.to)
			if got.High != c.want.High || got.Low != c.want.Low || got.Count != c.want.Count || !got.First.Equal(c.want.First) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestRuleState_SurvivesReopen(t *testing.T) {
	s, path := newTestStore(t)
	T0 := time.Date(2026, 10, 20, 8, 30, 15, 123_456_789, time.UTC)
	if err := s.PutRuleState("BTCUSDT", "dd", RuleState{Tier: 2, SentInTier: 1, LastSentAt: T0}); err != nil {
		t.Fatalf("PutRuleState: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetRuleState("BTCUSDT", "dd")
	if err != nil {
		t.Fatalf("GetRuleState: %v", err)
	}
	want := RuleState{Tier: 2, SentInTier: 1, LastSentAt: T0.Truncate(time.Millisecond)}
	if got.Tier != want.Tier || got.SentInTier != want.SentInTier || !got.LastSentAt.Equal(want.LastSentAt) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	missing, err := s2.GetRuleState("BTCUSDT", "nope")
	if err != nil {
		t.Fatalf("missing: err=%v", err)
	}
	if missing.Tier != 0 || missing.SentInTier != 0 || !missing.LastSentAt.IsZero() {
		t.Fatalf("missing: want zero value, got %+v", missing)
	}
}

func TestRuleState_Overwrite(t *testing.T) {
	s, _ := newTestStore(t)
	T0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	for _, st := range []RuleState{{1, 1, T0}, {3, 2, T0.Add(time.Hour)}} {
		if err := s.PutRuleState("BTCUSDT", "dd", st); err != nil {
			t.Fatalf("PutRuleState: %v", err)
		}
	}
	// 清零后 LastSentAt 仍保留（Decide case 1），零值 LastSentAt 也要能往返
	if err := s.PutRuleState("BTCUSDT", "rise", RuleState{}); err != nil {
		t.Fatalf("PutRuleState zero: %v", err)
	}
	got, _ := s.GetRuleState("BTCUSDT", "dd")
	if got.Tier != 3 || got.SentInTier != 2 || !got.LastSentAt.Equal(T0.Add(time.Hour)) {
		t.Fatalf("overwrite: got %+v", got)
	}
	zero, _ := s.GetRuleState("BTCUSDT", "rise")
	if zero.Tier != 0 || zero.SentInTier != 0 || !zero.LastSentAt.IsZero() {
		t.Fatalf("zero round trip: got %+v", zero)
	}
}

func TestPruneCandles(t *testing.T) {
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "5m", candle(day(0), 1, 1, 1), candle(day(1), 2, 2, 2), candle(day(2), 3, 3, 3))
	if err := s.PruneCandles("BTCUSDT", "5m", day(1)); err != nil {
		t.Fatalf("PruneCandles: %v", err)
	}
	st := mustStats(t, s, "BTCUSDT", "5m", time.Time{}, day(10))
	if st.Count != 2 || !st.First.Equal(day(1)) {
		t.Fatalf("want 2 left starting at day(1) (before 不含), got %+v", st)
	}
}

func TestKV(t *testing.T) {
	s, _ := newTestStore(t)
	v, err := s.GetKV("heartbeat")
	if err != nil || v != "" {
		t.Fatalf("missing key: v=%q err=%v", v, err)
	}
	for _, val := range []string{"2026-10-20", "2026-10-21"} {
		if err := s.PutKV("heartbeat", val); err != nil {
			t.Fatalf("PutKV: %v", err)
		}
		if v, err = s.GetKV("heartbeat"); err != nil || v != val {
			t.Fatalf("GetKV: want %q, got %q err=%v", val, v, err)
		}
	}
	if v, _ = s.GetKV("listing:BTCUSDT:1d"); v != "" {
		t.Fatalf("other key leaked: %q", v)
	}
}

func TestLatestCandle(t *testing.T) {
	s, _ := newTestStore(t)
	ts, ok, err := s.LatestCandle("BTCUSDT", "1d")
	if err != nil || ok || !ts.IsZero() {
		t.Fatalf("empty: ts=%v ok=%v err=%v", ts, ok, err)
	}
	mustUpsert(t, s, "BTCUSDT", "1d", candle(day(1), 1, 1, 1), candle(day(3), 1, 1, 1), candle(day(2), 1, 1, 1))
	ts, ok, err = s.LatestCandle("BTCUSDT", "1d")
	if err != nil || !ok || !ts.Equal(day(3)) {
		t.Fatalf("want day(3), got ts=%v ok=%v err=%v", ts, ok, err)
	}
}

func TestFirstCloseSince(t *testing.T) {
	s, _ := newTestStore(t)
	if _, ok, err := s.FirstCloseSince("BTCUSDT", "1d", day(0)); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	mustUpsert(t, s, "BTCUSDT", "1d", candle(day(3), 1, 1, 30), candle(day(1), 1, 1, 10), candle(day(2), 1, 1, 20))

	cases := []struct {
		name   string
		from   time.Time
		want   float64
		wantOK bool
	}{
		{"恰在某根（含）", day(2), 20, true},
		{"两根之间取后一根", day(1).Add(time.Hour), 20, true},
		{"早于全部取最早", day(0), 10, true},
		{"晚于全部无数据", day(4), 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := s.FirstCloseSince("BTCUSDT", "1d", c.from)
			if err != nil || ok != c.wantOK || got != c.want {
				t.Fatalf("got %v ok=%v err=%v, want %v ok=%v", got, ok, err, c.want, c.wantOK)
			}
		})
	}
}

func TestStore_Isolation(t *testing.T) {
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "1d", candle(day(0), 100, 50, 70), candle(day(1), 110, 60, 80))
	mustUpsert(t, s, "ETHUSDT", "1d", candle(day(0), 10, 5, 7))
	mustUpsert(t, s, "BTCUSDT", "5m", candle(day(0), 1000, 900, 950)) // 与 1d 同 ts

	if st := mustStats(t, s, "BTCUSDT", "1d", time.Time{}, day(9)); st.Count != 2 || st.High != 110 || st.Low != 50 {
		t.Fatalf("BTC 1d: %+v", st)
	}
	if st := mustStats(t, s, "ETHUSDT", "1d", time.Time{}, day(9)); st.Count != 1 || st.High != 10 || st.Low != 5 {
		t.Fatalf("ETH 1d: %+v", st)
	}
	if st := mustStats(t, s, "BTCUSDT", "5m", time.Time{}, day(9)); st.Count != 1 || st.High != 1000 {
		t.Fatalf("BTC 5m: %+v", st)
	}
	if ts, ok, _ := s.LatestCandle("ETHUSDT", "1d"); !ok || !ts.Equal(day(0)) {
		t.Fatalf("ETH latest: %v %v", ts, ok)
	}
	if ts, ok, _ := s.LatestCandle("BTCUSDT", "5m"); !ok || !ts.Equal(day(0)) {
		t.Fatalf("BTC 5m latest: %v %v", ts, ok)
	}
	if c, ok, _ := s.FirstCloseSince("ETHUSDT", "1d", time.Time{}); !ok || c != 7 {
		t.Fatalf("ETH first close: %v %v", c, ok)
	}

	if err := s.PruneCandles("BTCUSDT", "1d", day(5)); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if st := mustStats(t, s, "BTCUSDT", "1d", time.Time{}, day(9)); st.Count != 0 {
		t.Fatalf("BTC 1d after prune: %+v", st)
	}
	if st := mustStats(t, s, "ETHUSDT", "1d", time.Time{}, day(9)); st.Count != 1 {
		t.Fatalf("ETH 1d affected by BTC prune: %+v", st)
	}
	if st := mustStats(t, s, "BTCUSDT", "5m", time.Time{}, day(9)); st.Count != 1 {
		t.Fatalf("BTC 5m affected by 1d prune: %+v", st)
	}

	T0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	_ = s.PutRuleState("BTCUSDT", "a", RuleState{1, 1, T0})
	_ = s.PutRuleState("BTCUSDT", "b", RuleState{2, 2, T0})
	_ = s.PutRuleState("ETHUSDT", "a", RuleState{3, 3, T0})
	for _, c := range []struct {
		pair, rule string
		tier       int
	}{{"BTCUSDT", "a", 1}, {"BTCUSDT", "b", 2}, {"ETHUSDT", "a", 3}, {"ETHUSDT", "b", 0}} {
		if got, _ := s.GetRuleState(c.pair, c.rule); got.Tier != c.tier {
			t.Fatalf("%s/%s: want tier %d, got %+v", c.pair, c.rule, c.tier, got)
		}
	}
}

func TestStore_NonUTCRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	loc := time.FixedZone("UTC+8", 8*3600)
	ts := time.Date(2026, 10, 20, 7, 59, 59, 987_654_321, loc) // 本地日期与 UTC 日期不同
	want := ts.Truncate(time.Millisecond)

	mustUpsert(t, s, "BTCUSDT", "5m", candle(ts, 1, 1, 1))
	if got, ok, err := s.LatestCandle("BTCUSDT", "5m"); err != nil || !ok || !got.Equal(want) {
		t.Fatalf("LatestCandle: got %v ok=%v err=%v, want %v", got, ok, err, want)
	}
	if st := mustStats(t, s, "BTCUSDT", "5m", want, want); st.Count != 1 || !st.First.Equal(want) {
		t.Fatalf("Stats at exact ts: %+v", st)
	}

	if err := s.PutRuleState("BTCUSDT", "dd", RuleState{1, 1, ts}); err != nil {
		t.Fatalf("PutRuleState: %v", err)
	}
	if got, _ := s.GetRuleState("BTCUSDT", "dd"); !got.LastSentAt.Equal(want) {
		t.Fatalf("LastSentAt: got %v, want %v", got.LastSentAt, want)
	}
}

func TestNewStore_CreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "pw.db")
	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}

func TestNewStore_BadPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(filepath.Join(file, "sub", "pw.db"))
	if err == nil {
		s.Close()
		t.Fatal("want error when parent path is a regular file")
	}
}

func TestStore_Concurrent(t *testing.T) {
	s, _ := newTestStore(t)
	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds*3)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			pair := fmt.Sprintf("P%d", w%3)
			for i := 0; i < rounds; i++ {
				if err := s.UpsertCandles(pair, "5m", []core.OHLCV{candle(day(i), float64(i), 1, 1), candle(day(i+1), 1, 1, 1)}); err != nil {
					errs <- err
				}
				if _, err := s.Stats(pair, "5m", time.Time{}, day(100)); err != nil {
					errs <- err
				}
				if err := s.PutRuleState(pair, fmt.Sprintf("r%d", w), RuleState{Tier: i, SentInTier: 1, LastSentAt: day(i)}); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op: %v", err)
	}
	if st := mustStats(t, s, "P0", "5m", time.Time{}, day(100)); st.Count != rounds+1 || st.High != rounds-1 {
		t.Fatalf("P0 after concurrent writes: %+v", st)
	}
}
