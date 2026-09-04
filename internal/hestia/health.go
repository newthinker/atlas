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

	byOutcome, err := groupCount(ctx, q, "runs by outcome",
		`SELECT outcome, count(*) FROM `+TableRuns+` GROUP BY outcome`)
	if err != nil {
		return Health{}, err
	}
	for o, n := range byOutcome {
		h.RunsByOutcome[RunOutcome(o)] = n
	}

	h.BlockedByCheck, err = groupCount(ctx, q, "blocked by check",
		`SELECT blocked_check, count(*) FROM `+TableRuns+`
		 WHERE outcome = ? AND blocked_check IS NOT NULL GROUP BY blocked_check`, string(RunPending))
	if err != nil {
		return Health{}, err
	}

	if err := q.QueryRowContext(ctx,
		`SELECT count(*) FROM `+TableRuns+` WHERE notify_error IS NOT NULL`).Scan(&h.NotifyFailures); err != nil {
		return Health{}, fmt.Errorf("hestia health: notify failures: %w", err)
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM `+TablePending).Scan(&h.PendingReview); err != nil {
		return Health{}, fmt.Errorf("hestia health: pending review: %w", err)
	}
	return h, nil
}

// groupCount 跑一条「键, count(*) … GROUP BY 键」查询并收成 map；错误一律带 hestia health: <step>: 前缀。
//
// 迭代结束后必须查 rows.Err()（review_fix W1）：ctx 中断或连接断掉时 Next() 只是返回 false，
// 不查就会把**部分计数**当成完整结果交给 collector——collect_errors 不加一，健康度自己不健康时
// 反而不响。返回的 map 永远非 nil，空结果也是空 map。
func groupCount(ctx context.Context, q Querier, step, query string, args ...any) (map[string]int, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hestia health: %s: %w", step, err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, fmt.Errorf("hestia health: %s: %w", step, err)
		}
		out[k] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hestia health: %s: %w", step, err)
	}
	return out, nil
}

func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s.String)
}
