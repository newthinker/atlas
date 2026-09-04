package hestia

// Context Checkpoint: done_criteria → test mapping (health，M1.5 的 TASK-003)
// functional[0]     Health 两个 map 非 nil；LastRun=MAX(run_at)；LastIngest 只看 ingested/pending；
//                   RunsByOutcome / BlockedByCheck / NotifyFailures / PendingReview 各一条 SQL
//                                                   → TestHealthSummaryEmpty、TestHealthSummaryTimestamps、
//                                                     TestHealthSummaryCounts、TestHealthSummaryPendingReview
// functional[1]     四条需求测试通过（reviewer S6：一行两语句已 gofmt 成两行）
// functional[2]     AST 守卫登记 HealthSummary（25 项）；health.go 无业务字段名字面量
//                                                   → TestPackageExposesNoWriteFunctions、TestFieldNamesAppearOnlyInFieldsGo
// boundary[0]       parseNullTime：NULL ⇒ 零值；非法串 ⇒ hestia health: bad run_at
//                                                   → TestHealthSummaryRejectsCorruptRunAt
//                   五条 SQL 各自的错误前缀            → TestHealthSummaryPropagatesQueryErrors
//                   BlockedByCheck 只数 pending 行：failed 夹具带 BlockedCheck="x"，去掉 outcome 过滤即红
//                                                   → TestHealthSummaryCounts
// error_handling[0] 红阶段 undefined: HealthSummary → discovery verification

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAt(day int, outcome RunOutcome) Run {
	at := time.Date(2026, 9, day, 15, 30, 0, 0, time.UTC)
	return Run{RunAt: at, FinishedAt: at, Outcome: outcome, Period: "2026-08", PeriodType: "monthly"}
}

// 空库：两个时间戳零值、计数全 0、map 非 nil。
func TestHealthSummaryEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	h, err := HealthSummary(ctx, s.DB())
	require.NoError(t, err)
	assert.True(t, h.LastRun.IsZero())
	assert.True(t, h.LastIngest.IsZero())
	assert.NotNil(t, h.RunsByOutcome)
	assert.NotNil(t, h.BlockedByCheck)
	assert.Zero(t, h.PendingReview)
	assert.Zero(t, h.NotifyFailures)
}

// LastRun 看任何 outcome；LastIngest 只看 ingested 与 pending——duplicate 与 no_new 不推进。
func TestHealthSummaryTimestamps(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.RecordRun(ctx, runAt(10, RunIngested)))
	require.NoError(t, s.RecordRun(ctx, runAt(12, RunPending)))
	require.NoError(t, s.RecordRun(ctx, runAt(14, RunDuplicate))) // --force 重跑
	require.NoError(t, s.RecordRun(ctx, runAt(16, RunNoNew)))     // 心跳

	h, err := HealthSummary(ctx, s.DB())
	require.NoError(t, err)
	assert.True(t, h.LastRun.Equal(time.Date(2026, 9, 16, 15, 30, 0, 0, time.UTC)), "心跳推进 LastRun")
	assert.True(t, h.LastIngest.Equal(time.Date(2026, 9, 12, 15, 30, 0, 0, time.UTC)),
		"duplicate 不推进 LastIngest——否则 --force 重跑会掩盖真实停摆")
}

// 计数：按 outcome 分组；blocked 只数 pending 行的 blocked_check；notify 失败只数 notify_error 非空。
func TestHealthSummaryCounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p1 := runAt(1, RunPending)
	p1.BlockedCheck = "deposit_sum"
	p2 := runAt(2, RunPending)
	p2.BlockedCheck = "deposit_sum"
	p3 := runAt(3, RunPending)
	p3.BlockedCheck = "stock_continuity"
	f := runAt(4, RunFailed)
	f.Stage, f.Error, f.NotifyError = "parse", "boom", "telegram down"
	// failed 行也带一个 blocked_check：BlockedByCheck 只数 pending 行——去掉 outcome 过滤这条会红。
	f.BlockedCheck = "x"
	i := runAt(5, RunIngested)
	i.NotifyError = "telegram down"
	for _, r := range []Run{p1, p2, p3, f, i, runAt(6, RunNoNew), runAt(7, RunNoNew)} {
		require.NoError(t, s.RecordRun(ctx, r))
	}

	h, err := HealthSummary(ctx, s.DB())
	require.NoError(t, err)
	assert.Equal(t, map[RunOutcome]int{RunPending: 3, RunFailed: 1, RunIngested: 1, RunNoNew: 2}, h.RunsByOutcome)
	assert.Equal(t, map[string]int{"deposit_sum": 2, "stock_continuity": 1}, h.BlockedByCheck)
	assert.Equal(t, 2, h.NotifyFailures)
}

// PendingReview 数的是 hestia_pending 的行数，不是 runs 里的 pending。
func TestHealthSummaryPendingReview(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cfg := ingestCfg(t)
	cfg.Thresholds.DepositSumTolerance = 1e-9
	require.NoError(t, Ingest(ctx, IngestDeps{Store: s, Fetch: annualFetcher(t), Out: nil, Cfg: cfg}))

	h, err := HealthSummary(ctx, s.DB())
	require.NoError(t, err)
	assert.Equal(t, 1, h.PendingReview)
	assert.Equal(t, 1, h.RunsByOutcome[RunPending])
}

// 库里 run_at 不是 RFC3339 时要报错，而不是把零值当「表里没有」交给 collector。
// 行经裸连接写入：RecordRun 自己永远写不出这种行。两个子例分别打在 LastRun 与 LastIngest
// 的解析上：坏串取 "1999"（字典序小于任何合法 RFC3339），让 MAX(run_at) 仍取到合法行、
// 只有 ingested 那一支的 MAX 拿到坏串。
func TestHealthSummaryRejectsCorruptRunAt(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, badRunAt string
		badOutcome     RunOutcome
	}{
		{"LastRun 坏", "yesterday", RunNoNew},
		{"LastIngest 坏", "1999", RunIngested},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, path := newTestStoreAt(t)
			require.NoError(t, s.RecordRun(ctx, runAt(16, RunNoNew)))
			_, err := rawDB(t, path).ExecContext(ctx, `INSERT INTO `+TableRuns+
				` (run_at, finished_at, outcome, duration_ms) VALUES (?, ?, ?, 0)`,
				tc.badRunAt, tc.badRunAt, string(tc.badOutcome))
			require.NoError(t, err)

			_, err = HealthSummary(ctx, s.DB())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "hestia health: bad run_at")
		})
	}
}

// failingQuerier 把第 failAt 次查询换成 replace 这条 SQL，其余透传真实库：
// HealthSummary 发五条查询，每条的错误都要带自己的步骤前缀。
type failingQuerier struct {
	Querier
	calls, failAt int
	replace       string
}

func (f *failingQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	f.calls++
	if f.calls == f.failAt {
		query, args = f.replace, nil
	}
	return f.Querier.QueryContext(ctx, query, args...)
}

func (f *failingQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	f.calls++
	if f.calls == f.failAt {
		query, args = f.replace, nil
	}
	return f.Querier.QueryRowContext(ctx, query, args...)
}

func TestHealthSummaryPropagatesQueryErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	// 查询本身出错：表不存在。
	for i, step := range []string{"last runs", "runs by outcome", "blocked by check", "notify failures", "pending review"} {
		t.Run("query "+step, func(t *testing.T) {
			_, err := HealthSummary(ctx, &failingQuerier{Querier: s.DB(), failAt: i + 1, replace: `SELECT * FROM no_such_table`})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "hestia health: "+step+":")
		})
	}
	// 查询成功但行形状不对（列数少一）：两条 GROUP BY 的 rows.Scan 出错也要带前缀，且不泄漏 rows。
	for i, step := range []string{"runs by outcome", "blocked by check"} {
		t.Run("scan "+step, func(t *testing.T) {
			_, err := HealthSummary(ctx, &failingQuerier{Querier: s.DB(), failAt: i + 2, replace: `SELECT 'only-one-column'`})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "hestia health: "+step+":")
		})
	}
}
