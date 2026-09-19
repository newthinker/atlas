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
	// 形状守卫必须在 schemaDDL() **之前**：schemaDDL 会在老库上建出一个指向老表
	// 的 v_macro_current，而这个库随后就会被拒绝 —— 留下一个被改了形状却打不开
	// 的库。检查在前，被拒绝的库一个字节不变。
	if err := verifyBitemporalShape(db, path); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaDDL()); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating crisis schema: %w", err)
	}
	return &Store{db: db}, nil
}

// verifyBitemporalShape 拒绝双时态迁移之前建的库（AD-4/C9）。
//
// 存在的理由：CREATE TABLE IF NOT EXISTS 对**已存在**的表静默无操作，所以老库
// 能一路开到第一次写入。而那次写入不会报错 —— 三段主键的 INSERT OR REPLACE 落
// 在两段主键的表上只会静默覆盖同一天的旧修订，双时态从此形同虚设，且没有任何
// 迹象。等到有人发现修订历史是空的，现场早就不在了。
//
// **刻意不自动迁移**：启动时静默改表结构，会把一个本该被人盯着的动作变成副作用
// —— 迁移要搬全部历史数据、要重建索引与视图、失败时需要人判断怎么回滚。让它在
// 某个 launchd 唤起里悄悄发生，出问题时连「什么时候变的」都查不到。同 hestia 的
// 先例（internal/hestia/store.go 的 verifyObservationsSchema / verifyCurrentView，
// 那里把这条写作 "Automatic migration is an explicit non-goal"）。
//
// 探形状的判据是**主键里有没有 fetched_at**，不是「有没有 macro_observations_v1」：
// 迁移后若有人清理了 _v1，按后者判会拒绝一个完全正常的库。判断本身复用 migrate.go
// 的 isBitemporal —— 两份形状判据意味着改一处不会让另一处变红（同 AD-3）。
func verifyBitemporalShape(db *sql.DB, path string) error {
	ctx := context.Background()

	// 表不存在 = 全新库，schemaDDL() 随后会建出新形状。isBitemporal 对这种库
	// 返回错误，所以要先在这里放行，否则谁都建不了新库。
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='macro_observations'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("crisis: inspecting schema of %s: %w", path, err)
	}
	if n == 0 {
		return nil
	}

	ok, err := isBitemporal(ctx, db)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return fmt.Errorf(
		"crisis: macro_observations in %s still has the legacy two-part primary key "+
			"(ts, indicator); this database predates the bitemporal migration and "+
			"CREATE TABLE IF NOT EXISTS does not change it. Automatic migration is an "+
			"explicit non-goal — run:  atlas crisis migrate-bitemporal --db %s",
		path, path)
}

func (s *Store) Close() error { return s.db.Close() }

// sameObservationValue 判断两个观测值是否相同，把「两边都没有值」算作相同。
//
// 不能直接用 SQL 比较：那里 NULL != NULL，两行都没有值会被判成异值、于是一次
// 无害的重跑变成报错。value 列可空（REAL 无 NOT NULL），所以这条不是假想情形。
//
// 一边 NULL 一边有值则是**异值** —— 0 不是「没有值」，把它们混为一谈会让一个
// 真实的数据缺口悄悄变成 0。
func sameObservationValue(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// UpsertObservations 在一个事务里写入 obs，**追加而非覆盖**（C6/AD-7）。
//
// 三段主键 (ts, indicator, fetched_at) 下，一次写入相对库中现状只有三种可能，
// 三种的正确处置各不相同 —— 这正是不能用 INSERT OR REPLACE 或 OR IGNORE 的原因：
//
//   - 新的 fetched_at：这是一个新修订，追加一行。旧值留着，那是本次迁移的全部目的。
//   - 三段全同且 value 也相同：采集器重跑了同一批，没有信息差，吞掉。
//   - 三段全同而 value 不同：**同一次取回给出了两个值** —— 数据源或采集器出了
//     问题。整批回滚并报错，不能静默选一个。
//
// OR REPLACE 会把第三种静默覆盖（正是本次要修的缺陷）；OR IGNORE 把第二、三种
// 揉成一种，于是最该响的那一种被吞掉。两者都让错误变成看不见的数据损坏。
//
// ⚠️ 冲突检测查的是**基表**而非 v_macro_current：视图按定义只有当前行，查不到
// 已存在的旧修订（实测：同键两个修订时，基表查旧修订得 1 行、视图得 0 行），
// 据视图判「不存在」而插入会直接撞主键。这条裸读在 TestNoBareTableReadsOutsideMigration
// 的豁免清单里，理由同此。
func (s *Store) UpsertObservations(ctx context.Context, obs []Observation) error {
	if len(obs) == 0 {
		return nil // 没有要写的东西，连事务都不必开
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	conflictCheck, err := tx.PrepareContext(ctx,
		`SELECT value FROM macro_observations WHERE ts = ? AND indicator = ? AND fetched_at = ?`)
	if err != nil {
		return fmt.Errorf("preparing conflict check: %w", err)
	}
	defer conflictCheck.Close()

	insert, err := tx.PrepareContext(ctx,
		`INSERT INTO macro_observations (ts, indicator, value, source, fetched_at)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer insert.Close()

	for _, o := range obs {
		var prev *float64
		err := conflictCheck.QueryRowContext(ctx, o.Date, o.Indicator, o.FetchedAt).Scan(&prev)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := insert.ExecContext(ctx,
				o.Date, o.Indicator, o.Value, o.Source, o.FetchedAt); err != nil {
				return fmt.Errorf("inserting %s/%s: %w", o.Indicator, o.Date, err)
			}
		case err != nil:
			return fmt.Errorf("checking %s/%s: %w", o.Indicator, o.Date, err)
		case sameObservationValue(prev, &o.Value):
			// 同一批被重跑了，无信息差
		default:
			return fmt.Errorf(
				"crisis: %s/%s already has a different value for fetched_at %s (have %v, got %v): "+
					"the same fetch reported two values; refusing to overwrite",
				o.Indicator, o.Date, o.FetchedAt, derefValue(prev), o.Value)
		}
	}
	return tx.Commit()
}

// derefValue 把可空的 value 渲染成错误文案里能读的东西。
func derefValue(v *float64) any {
	if v == nil {
		return "NULL"
	}
	return *v
}

// obsSelect 从 v_macro_current 读，不是从 macro_observations。
//
// 换 FROM 而不是给每个调用点加一句 "取 fetched_at 最大的那行"：后者是同一条规则的
// N 份副本，加第 N+1 个读点时必漏 —— EvalDates 就是这么漏掉的（它不经本常量，
// 计划原文因此把读取点数错成「只改 obsSelect 即可」）。视图把规则收进一个地方，
// 新读点只要 FROM 对了就自动正确。
//
// 保持显式列名而非 SELECT *：视图是 SELECT * FROM macro_observations o WHERE...，
// 用 * 会把将来新增的列一并带出来，而 scanObservation 按固定列序扫描。
const obsSelect = `SELECT ts, indicator, value, source, fetched_at FROM v_macro_current`

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
//
// 🔴 这是第五个读取点，且**不经 obsSelect** —— 它自己内联了一条查询，所以「改了
// obsSelect 就都改完了」是错的（计划原文正是这么数的）。读裸表时，同一天的每个
// 修订都会产出一个日期，回测于是把那天评估两遍。走视图后每个业务键只剩一行。
//
// 同样是换 FROM 而非加 DISTINCT：DISTINCT 能去掉重复日期，但它是「就地补一条规则」
// ——下一个读点还得再补一次。规则收在视图里只需对一次。
func (s *Store) EvalDates(ctx context.Context, from, to string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts FROM v_macro_current WHERE indicator = ? AND ts >= ? AND ts <= ? ORDER BY ts ASC`,
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

// HasIndicatorEvalForDate dedupes per-day one-shot alerts (intraday JPY).
func (s *Store) HasIndicatorEvalForDate(ctx context.Context, indicator, date string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crisis_evaluations WHERE indicator = ? AND ts = ?`, indicator, date).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking eval for %s/%s: %w", indicator, date, err)
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
