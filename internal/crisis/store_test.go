package crisis

// Context Checkpoint: done_criteria → test mapping (store)
// functional[1] NewStore WAL+建表 / UpsertObservations 事务 / SeriesWindow / SeriesSince → TestStoreUpsertIdempotentAndWindows
// functional[2] AppendEvaluations / RecentSystemEvals(新→旧) / RecentIndicatorEvals / LatestSystemEval / HasSystemEvalForDate / Reader/History 适配器 → TestStoreEvaluations
// boundary[0]   同 (ts,indicator) 重复 upsert 覆盖而非报错 → TestStoreUpsertIdempotentAndWindows
// boundary[1]   Observation/LatestObservation/LatestSystemEval 无数据返回 (nil,nil) → TestStoreUpsertIdempotentAndWindows / TestStoreEvaluations
// error_handling[0] NewStore 不可创建路径返回包装错误 → TestNewStoreBadPath

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
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

// ---------------------------------------------------------------------------
// TASK-003 NewStore 漂移守卫
//
// Context Checkpoint: done_criteria → test mapping (guard)
// functional[0]     老形状库 ⇒ NewStore 报错，文案含 migrate-bitemporal / macro_observations / 库路径 → TestNewStoreRejectsLegacyShape
// functional[1]     迁移后的库正常打开；空库正常；迁移后两条不变量（索引 tbl_name、视图指向）      → TestNewStoreAcceptsMigratedDB / TestNewStoreAcceptsFreshDB / TestMigratedDBKeepsIndexAndViewOnNewTable
// functional[2]     守卫不自动迁移（NewStore 路径无改形语句）                                      → TestGuardDoesNotAutoMigrate（源码扫描）+ verify_by: review
// boundary[0]       探形状看主键、不看 _v1；AD-5 老库上 NewStore 报错而 MigrateBitemporal 仍成功  → TestNewStoreShapeProbeIgnoresLegacyTable / TestMigrateWorksOnDBRejectedByNewStore
// boundary[1]       每条 error 返回路径前已 db.Close()                                             → verify_by: review（store.go 的 NewStore）
// error_handling[0] 既有错误路径不变                                                               → TestNewStoreBadPath（既有，零改动）

// legacyDBWithView 建一个**带 v_macro_current 视图**的老形状库。
//
// 这个夹具是 functional[1]② 的一半（另一半是行为断言），缺了它缺陷根本不会被
// 触发：RENAME 那一刻若没有视图可被改写，schemaDDL() 会照常建出指向新表的视图，
// 变异（删掉迁移里的 DROP VIEW）照样存活。TASK-002 实测过三种存活形态。
//
// 视图 SQL 由 bitemporal.CurrentQuery(obsSpec) 生成而非手抄——手抄就是视图定义的
// 第二个副本（同 AD-3）。它模拟的是「TASK-001 之后、守卫之前被 NewStore 打开过」
// 的生产库：那时每次打开老库都会执行 schemaDDL()，于是视图建在了老表上。
func legacyDBWithView(t *testing.T, rows ...legacyRow) string {
	t.Helper()
	path := legacyDB(t, rows...)
	db := openRaw(t, path)
	_, err := db.Exec(`CREATE VIEW v_macro_current AS ` + bitemporal.CurrentQuery(obsSpec))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return path
}

// TestNewStoreRejectsLegacyShape 是本任务的核心：老库要在打开时就被拦下。
//
// 不拦的话，老库能一路开到第一次写入才炸——而三段主键的写入在两段主键的表上
// 不会报错，只会静默覆盖同一天的旧修订，双时态从此形同虚设。
func TestNewStoreRejectsLegacyShape(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})

	s, err := NewStore(path)
	require.Error(t, err, "老形状库必须在 NewStore 就失败，而不是等到第一次写入")
	assert.Nil(t, s, "失败时不应返回一个半可用的 Store")
	assert.Contains(t, err.Error(), "macro_observations", "错误应指名是哪张表")
	assert.Contains(t, err.Error(), "migrate-bitemporal", "错误应给出该跑的命令")
	assert.Contains(t, err.Error(), path, "错误应指名是哪个库——运维常同时有好几个")
}

// TestNewStoreAcceptsMigratedDB：迁过的库正常打开。
func TestNewStoreAcceptsMigratedDB(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	s, err := NewStore(path)
	require.NoError(t, err, "迁移之后必须能正常打开")
	require.NotNil(t, s)
	require.NoError(t, s.Close())
}

// TestNewStoreAcceptsFreshDB：全新路径直接是新形状。
//
// 守卫必须能区分「老形状」与「表还不存在」——后者是全新库，schemaDDL() 会建出
// 新形状，拦下它等于谁都别想建新库。
func TestNewStoreAcceptsFreshDB(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "fresh.db"))
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NoError(t, s.Close())
}

// TestMigratedDBKeepsIndexAndViewOnNewTable 核对 functional[1] 的两条迁移后不变量。
//
// 夹具刻意带视图（见 legacyDBWithView 的注释），且第②条用**行为断言**而非
// DDL 文本比对：迁移后 _v1 与新表内容完全相同，视图即使指向 _v1，行数与双向
// EXCEPT 也全对（那是 _v1 减 _v1）。只有写入一个新修订再读，才区分得出来。
func TestMigratedDBKeepsIndexAndViewOnNewTable(t *testing.T) {
	path := legacyDBWithView(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	db := openRaw(t, path)

	// ① 索引挂在新表上（C2：RENAME 后索引跟着旧表走，同名索引让 CREATE INDEX
	//    IF NOT EXISTS 静默跳过——不报错，只变慢）
	var tbl string
	require.NoError(t, db.QueryRow(
		`SELECT tbl_name FROM sqlite_master WHERE type='index' AND name='idx_macro_obs_ind_ts'`).Scan(&tbl))
	assert.Equal(t, "macro_observations", tbl, "索引必须挂在新表上，不是 _v1")

	// ② 视图跟着新表走 —— 行为断言：写一个新修订，必须经视图读得到
	_, err = db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?)`,
		"2026-01-02", "vix", 11, "test", "2026-08-14T00:00:00Z")
	require.NoError(t, err)

	var got float64
	require.NoError(t, db.QueryRow(
		`SELECT value FROM v_macro_current WHERE ts='2026-01-02' AND indicator='vix'`).Scan(&got))
	assert.Equal(t, 11.0, got, "视图必须跟着新表走；指向 _v1 时读到的是迁移前那份")

	// 打开也要正常（守卫认这个库）
	s, err := NewStore(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

// TestNewStoreShapeProbeIgnoresLegacyTable：判据是主键，不是 _v1 是否存在。
//
// 按 _v1 存在与否判会在「迁移后有人清理了 _v1」时误判成老形状而拒绝打开一个
// 完全正常的库。
func TestNewStoreShapeProbeIgnoresLegacyTable(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	db := openRaw(t, path)
	_, err = db.Exec(`DROP TABLE macro_observations_v1`) // 迁完之后有人清理了它
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(path)
	require.NoError(t, err, "已是新形状就该放行 —— 有没有 _v1 表与此无关")
	require.NoError(t, s.Close())
}

// TestMigrateWorksOnDBRejectedByNewStore 钉住 AD-5（鸡生蛋防线）。
//
// 守卫拒绝老形状的库，而迁移恰恰要在那种库上跑。若 MigrateBitemporal 改走
// NewStore，本测试必红。⚠️ TASK-002 阶段守卫尚不存在，那时这个缺陷零测试能
// 发现——这条断言是那个缺口的接管者。
func TestMigrateWorksOnDBRejectedByNewStore(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})

	_, err := NewStore(path)
	require.Error(t, err, "前提：这个库确实被守卫拒绝")

	res, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err, "迁移必须能在守卫拒绝的库上跑 —— 否则鸡生蛋")
	assert.False(t, res.AlreadyMigrated)
	assert.Equal(t, 1, res.RowsAfter)
}

// TestGuardDoesNotAutoMigrate：守卫只报错，不改结构（functional[2]）。
//
// 启动时静默改结构会把一个本该被人盯着的动作变成副作用。用源码扫描把这条钉住：
// store.go 里不得出现改形语句。
func TestGuardDoesNotAutoMigrate(t *testing.T) {
	src, err := os.ReadFile("store.go")
	require.NoError(t, err)
	body := strings.ToUpper(string(src))

	for _, forbidden := range []string{"ALTER TABLE", "RENAME TO", "DROP TABLE", "DROP VIEW", "DROP INDEX"} {
		assert.NotContains(t, body, forbidden,
			"store.go 不得出现改形语句 %q —— 迁移是 migrate.go 的事，启动路径只许报错", forbidden)
	}

	// 行为侧：被拒绝的库必须**整个 sqlite_master 原样留着**，等人来处理。
	//
	// 🔴 快照比的是全部对象，不是只比 macro_observations 的 DDL。只比表 DDL 时，
	// 「守卫写在 schemaDDL() 之后」这个缺陷会存活（变异实测过）：那样老库会先被
	// 建出一个指向老表的 v_macro_current 再被拒绝，表 DDL 一个字没变、而库已经
	// 被改了。被拒绝却已被改动的库是最难排查的一种——下一个人看到视图存在，会
	// 以为迁移跑过一半。
	path := legacyDBWithView(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	before := schemaSnapshot(t, openRaw(t, path))

	_, err = NewStore(path)
	require.Error(t, err)

	after := openRaw(t, path)
	assert.Equal(t, before, schemaSnapshot(t, after),
		"被拒绝的库必须一个字节不变 —— 含视图与索引，不只是表")
	assert.False(t, objectExists(t, after, "macro_observations_v1"), "守卫不得顺手迁移")
}

// schemaSnapshot 取 sqlite_master 的全量快照（type/name/tbl_name/sql 逐行）。
//
// 用它而非逐个对象断言：要证明的是「什么都没变」，而逐个断言只能证明「我想到的
// 那几个没变」——漏掉的那个恰好就是缺陷所在。
func schemaSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master ORDER BY type, name`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ, name, tbl, ddl string
		require.NoError(t, rows.Scan(&typ, &name, &tbl, &ddl))
		out = append(out, typ+"|"+name+"|"+tbl+"|"+ddl)
	}
	require.NoError(t, rows.Err())
	return out
}
