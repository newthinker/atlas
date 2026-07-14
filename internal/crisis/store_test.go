package crisis

// Context Checkpoint: done_criteria → test mapping (store)
// functional[1] NewStore WAL+建表 / UpsertObservations 事务 / SeriesWindow / SeriesSince → TestStoreUpsertIdempotentAndWindows
// functional[2] AppendEvaluations / RecentSystemEvals(新→旧) / RecentIndicatorEvals / LatestSystemEval / HasSystemEvalForDate / Reader/History 适配器 → TestStoreEvaluations
// boundary[0]   同 (ts,indicator) 重复 upsert 覆盖而非报错 → TestStoreUpsertIdempotentAndWindows
// boundary[1]   Observation/LatestObservation/LatestSystemEval 无数据返回 (nil,nil) → TestStoreUpsertIdempotentAndWindows / TestStoreEvaluations
// error_handling[0] NewStore 不可创建路径返回包装错误 → TestNewStoreBadPath

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "crisis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreUpsertIdempotentAndWindows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	obs := []Observation{
		{Date: "2026-07-01", Indicator: IndVIX, Value: 15, Source: "fred", FetchedAt: "2026-07-02T00:00:00.000000000Z"},
		{Date: "2026-07-02", Indicator: IndVIX, Value: 16, Source: "fred", FetchedAt: "2026-07-03T00:00:00.000000000Z"},
		{Date: "2026-07-03", Indicator: IndVIX, Value: 17, Source: "fred", FetchedAt: "2026-07-04T00:00:00.000000000Z"},
		{Date: "2026-07-03", Indicator: IndHYOAS, Value: 267, Source: "fred", FetchedAt: "2026-07-04T00:00:00.000000000Z"},
	}
	require.NoError(t, s.UpsertObservations(ctx, obs))

	// 同 (ts, indicator) 重写为覆盖而非报错（多时点唤起的幂等基础）
	obs[2].Value = 18
	require.NoError(t, s.UpsertObservations(ctx, obs))

	win, err := s.SeriesWindow(ctx, IndVIX, "2026-07-03", 2)
	require.NoError(t, err)
	require.Len(t, win, 2) // 截断到 n，升序
	assert.Equal(t, 16.0, win[0].Value)
	assert.Equal(t, 18.0, win[1].Value)

	since, err := s.SeriesSince(ctx, IndVIX, "2026-07-02", "2026-07-03")
	require.NoError(t, err)
	require.Len(t, since, 2)
	assert.Equal(t, "2026-07-02", since[0].Date)

	got, err := s.Observation(ctx, IndVIX, "2026-07-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 16.0, got.Value)
	missing, err := s.Observation(ctx, IndVIX, "2026-06-30")
	require.NoError(t, err)
	assert.Nil(t, missing)

	latest, err := s.LatestObservation(ctx, IndVIX)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "2026-07-03", latest.Date)

	// LatestObservation 无数据返回 (nil, nil)
	noLatest, err := s.LatestObservation(ctx, IndMOVE)
	require.NoError(t, err)
	assert.Nil(t, noLatest)

	dates, err := s.EvalDates(ctx, "2026-07-01", "2026-07-03")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-07-01", "2026-07-02", "2026-07-03"}, dates)
}

func TestStoreEvaluations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, empty)

	evals := []Evaluation{
		{TS: "2026-07-01", EvalAt: "2026-07-02T01:00:00.000000000Z", Indicator: IndVIX,
			Status: StatusGreen, Value: 15, Pct5y: 0.12, Detail: `{"raw":"GREEN"}`},
		{TS: "2026-07-01", EvalAt: "2026-07-02T01:00:00.000000000Z", Indicator: "",
			SystemState: StateNormal, Detail: `{"any_trigger":false}`},
		{TS: "2026-07-02", EvalAt: "2026-07-03T01:00:00.000000000Z", Indicator: IndVIX,
			Status: StatusAmber, Tag: TagStress, Value: 26, Pct5y: 0.91, Detail: `{"raw":"AMBER"}`},
		{TS: "2026-07-02", EvalAt: "2026-07-03T01:00:00.000000000Z", Indicator: "",
			SystemState: StateWatch, Detail: `{"any_trigger":true}`},
	}
	require.NoError(t, s.AppendEvaluations(ctx, evals))

	sys, err := s.RecentSystemEvals(ctx, 5)
	require.NoError(t, err)
	require.Len(t, sys, 2)
	assert.Equal(t, "2026-07-02", sys[0].TS) // 新→旧
	assert.Equal(t, StateWatch, sys[0].SystemState)

	ind, err := s.RecentIndicatorEvals(ctx, IndVIX, 1)
	require.NoError(t, err)
	require.Len(t, ind, 1)
	assert.Equal(t, StatusAmber, ind[0].Status)
	assert.Equal(t, TagStress, ind[0].Tag)

	latest, err := s.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, StateWatch, latest.SystemState)

	has, err := s.HasSystemEvalForDate(ctx, "2026-07-02")
	require.NoError(t, err)
	assert.True(t, has)
	has, err = s.HasSystemEvalForDate(ctx, "2026-07-03")
	require.NoError(t, err)
	assert.False(t, has)

	// HasIndicatorEvalForDate：按 (indicator, ts) 判在（intraday 每日去重用）
	hasVix, err := s.HasIndicatorEvalForDate(ctx, IndVIX, "2026-07-02")
	require.NoError(t, err)
	assert.True(t, hasVix)
	hasVix, err = s.HasIndicatorEvalForDate(ctx, IndVIX, "2026-07-05")
	require.NoError(t, err)
	assert.False(t, hasVix)

	// Reader / History 适配器冒烟
	w, err := s.Reader(ctx).Window(IndVIX, "2026-07-03", 1)
	require.NoError(t, err)
	assert.Len(t, w, 0) // 本测试未写观测
	h, err := s.History(ctx).RecentSystem(1)
	require.NoError(t, err)
	assert.Len(t, h, 1)
}

// TestNewStoreBadPath 覆盖 error_handling[0]：db 路径不可创建（以已存在的普通文件作为父目录）
// 时，NewStore 返回包装错误而非 panic。
func TestNewStoreBadPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	s, err := NewStore(filepath.Join(file, "crisis.db"))
	require.Error(t, err)
	assert.Nil(t, s)
}
