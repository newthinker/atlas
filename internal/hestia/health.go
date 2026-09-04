package hestia

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Health 是健康度汇总（M1.5 的 TASK-003）。它只报事实：时间戳零值 = 表里没有；
// 「距今多久」由 collector 用自己的 now 算——本函数不接收 now。
type Health struct {
	LastRun, LastIngest time.Time
	RunsByOutcome       map[RunOutcome]int
	BlockedByCheck      map[string]int
	PendingReview       int
	NotifyFailures      int
}

// HealthSummary 从 hestia_runs 与 hestia_pending 各查一次，返回汇总。
//
// 只读：接收 Querier 而不是 *Store，serve 侧拿 Store.DB() 就能调。
// LastIngest 只看 ingested / pending：duplicate 是 --force 重跑，不代表新数据到了。
func HealthSummary(ctx context.Context, q Querier) (Health, error) {
	h := Health{RunsByOutcome: map[RunOutcome]int{}, BlockedByCheck: map[string]int{}}

	var lastRun, lastIngest sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT MAX(run_at), MAX(CASE WHEN outcome IN (?, ?) THEN run_at END) FROM `+TableRuns,
		string(RunIngested), string(RunPending)).Scan(&lastRun, &lastIngest)
	if err != nil {
		return Health{}, fmt.Errorf("hestia health: last runs: %w", err)
	}
	if h.LastRun, err = parseNullTime(lastRun); err != nil {
		return Health{}, fmt.Errorf("hestia health: bad run_at: %w", err)
	}
	if h.LastIngest, err = parseNullTime(lastIngest); err != nil {
		return Health{}, fmt.Errorf("hestia health: bad run_at: %w", err)
	}

	rows, err := q.QueryContext(ctx, `SELECT outcome, count(*) FROM `+TableRuns+` GROUP BY outcome`)
	if err != nil {
		return Health{}, fmt.Errorf("hestia health: runs by outcome: %w", err)
	}
	for rows.Next() {
		var o string
		var n int
		if err := rows.Scan(&o, &n); err != nil {
			rows.Close()
			return Health{}, fmt.Errorf("hestia health: runs by outcome: %w", err)
		}
		h.RunsByOutcome[RunOutcome(o)] = n
	}
	rows.Close()

	rows, err = q.QueryContext(ctx,
		`SELECT blocked_check, count(*) FROM `+TableRuns+`
		 WHERE outcome = ? AND blocked_check IS NOT NULL GROUP BY blocked_check`, string(RunPending))
	if err != nil {
		return Health{}, fmt.Errorf("hestia health: blocked by check: %w", err)
	}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			rows.Close()
			return Health{}, fmt.Errorf("hestia health: blocked by check: %w", err)
		}
		h.BlockedByCheck[c] = n
	}
	rows.Close()

	if err := q.QueryRowContext(ctx,
		`SELECT count(*) FROM `+TableRuns+` WHERE notify_error IS NOT NULL`).Scan(&h.NotifyFailures); err != nil {
		return Health{}, fmt.Errorf("hestia health: notify failures: %w", err)
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM `+TablePending).Scan(&h.PendingReview); err != nil {
		return Health{}, fmt.Errorf("hestia health: pending review: %w", err)
	}
	return h, nil
}

func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s.String)
}
