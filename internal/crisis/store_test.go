package crisis

// Context Checkpoint: done_criteria → test mapping (store)
// functional[1] NewStore WAL+建表 / UpsertObservations 事务 / SeriesWindow / SeriesSince → TestStoreUpsertIdempotentAndWindows
// functional[2] AppendEvaluations / RecentSystemEvals(新→旧) / RecentIndicatorEvals / LatestSystemEval / HasSystemEvalForDate / Reader/History 适配器 → TestStoreEvaluations
// boundary[0]   同三段主键 (ts,indicator,fetched_at) 且 value 相同的重复 upsert 被吞掉；**异值则报错**
//               （TASK-005/C6 契约变更：此前是 INSERT OR REPLACE 覆盖）→ TestStoreUpsertIdempotentAndWindows
// boundary[1]   Observation/LatestObservation/LatestSystemEval 无数据返回 (nil,nil) → TestStoreUpsertIdempotentAndWindows / TestStoreEvaluations
// error_handling[0] NewStore 不可创建路径返回包装错误 → TestNewStoreBadPath

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

	// 整批重写同一批（三段全同、value 也全同）⇒ 吞掉，不报错。
	// 这是多时点唤起的幂等基础：采集器重跑同一批没有信息差。
	require.NoError(t, s.UpsertObservations(ctx, obs))

	// 🔴 契约变更（TASK-005/C6）：三段全同而 value 不同，是「同一次取回给出了
	// 两个值」—— 那是数据源或采集器的问题，**必须响亮失败**，不能像以前
	// （INSERT OR REPLACE）那样静默覆盖掉前一个值。
	obs[2].Value = 18
	err := s.UpsertObservations(ctx, obs)
	require.Error(t, err, "同三段主键异值必须报错，不得覆盖")
	assert.Contains(t, err.Error(), IndVIX)
	assert.Contains(t, err.Error(), "2026-07-03")

	// 失败的批次整批回滚 ⇒ 库里仍是第一次写入的 17
	win, err := s.SeriesWindow(ctx, IndVIX, "2026-07-03", 2)
	require.NoError(t, err)
	require.Len(t, win, 2) // 截断到 n，升序
	assert.Equal(t, 16.0, win[0].Value)
	assert.Equal(t, 17.0, win[1].Value, "冲突批次整批回滚，旧值不变")

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

// ---------------------------------------------------------------------------
// TASK-004 读路径改走 v_macro_current
//
// Context Checkpoint: done_criteria → test mapping (read-through-view)
// functional[0]     四个读方法只看到最新修订（Observation/LatestObservation/SeriesWindow/SeriesSince） → TestReadsSeeOnlyLatestRevision
// functional[1]     EvalDates 不返回重复日期（裁决 R1）                                                  → TestEvalDatesDeduplicatesRevisions
// functional[2]     包内裸表守卫：带词边界正则，豁免逐行列出且条数恰为 N                                 → TestNoBareTableReadsOutsideMigration
// boundary[0]       C5 四个调用点 WHERE/ORDER/LIMIT 逐字未变                                            → verify_by: review（git diff）
// boundary[1]       单版本数据下与改动前逐值相同                                                         → TestSingleRevisionReadsUnchanged + 既有测试零改动
// error_handling[0] 视图缺失 ⇒ 响亮失败且文案含 v_macro_current                                          → TestReadsFailLoudlyWhenViewMissing
// non_functional[1] obsSelect 与 EvalDates 各留注释说明「为什么换 FROM 而非每处加 WHERE」                → verify_by: review（store.go）

// revisedStore 建一个**新形状**的库并用裸 INSERT 造出重复修订。
//
// 📌 这里用裸 INSERT 的理由是**解耦**，不是「别的方式造不出来」：本函数服务的是
// **读路径**测试，不该依赖 UpsertObservations 的当时行为——那个方法正在被逐个
// sprint 改（TASK-005 把它从 INSERT OR REPLACE 改成了追加），夹具若走它，读路径
// 测试的输入会随写路径的改动而静默变形。
//
// ⚠️ 本段此前写的是「必须裸 INSERT——走公开 API 造不出同一 (ts, indicator) 的
// 两行」，**该句为假**，已于 TASK-006 订正（test-m4c-a 在 46c401c 实测证伪）：
// 三段主键下不同 fetched_at 即不同主键，走公开 API 先写 10@07-14 再写 11@08-14
// 就得到裸表 2 行。
//
// 订正时容易换上的另一个半真说法也记在这里，免得再绕回去：同 (ts, indicator) 的
// 多行**从 TASK-001 三段主键落地起一直造得出**；TASK-005 之后造不出的只是
// 「**同三段主键、异值**」——那会响亮失败（C6）。两者不是一回事。
//
// 对照：下方 asOfStore 刻意**走公开 API**，因为 as-of 测试验的正是「经正常写入
// 路径落库的东西能被正确读出」，那里耦合是要测的性质本身。
//
// 夹具在 DoD 给定的那组之上加了一行，**两处细节都是语义的一部分，别随手改**：
//
//  1. 最新那天（01-03）也有修订。DoD 原夹具把修订只放在较旧的 01-02，于是
//     SeriesWindow(end=01-03, n=2) 的 LIMIT 2 正好跨到两个不同日期，**改动前
//     就返回 [11 20]**、与期望一字不差 —— 那条断言测不出任何东西。加上这行后，
//     裸表上 LIMIT 2 全落在 01-03（dates=[01-03 01-03]），确定性变红。
//
//  2. 01-03 那天**先插新修订(21)、后插旧修订(20)**。裸表上 LatestObservation 的
//     `ORDER BY ts DESC LIMIT 1` 在同一天的多行里取的是**扫描顺序**的某一行，
//     不是语义上的当前行 —— 实测（改动前的树）：旧在前得 21（碰巧对），新在前
//     得 20（错）。顺序写反，这条断言就悄悄失去牙而看不出来。
//
// 走视图后两种顺序都得 21：视图按 MAX(fetched_at) 选，与插入顺序无关（这正是
// TASK-001 boundary[0]「乱序写入」钉住的性质），所以这个夹具不依赖未定义行为
// 来产生**正确**结果，只用它来产生一个**能区分正确与错误实现**的输入。
func revisedStore(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	for _, r := range []struct {
		ts        string
		value     float64
		fetchedAt string
	}{
		{"2026-01-02", 10, "2026-07-14T00:00:00Z"},
		{"2026-01-02", 11, "2026-08-14T00:00:00Z"},
		{"2026-01-03", 21, "2026-08-14T00:00:00Z"}, // 新修订先落库
		{"2026-01-03", 20, "2026-07-14T00:00:00Z"}, // 旧修订后落库（回填乱序是真实场景）
	} {
		_, err := s.db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?)`,
			r.ts, IndVIX, r.value, "test", r.fetchedAt)
		require.NoError(t, err)
	}
	return s
}

// TestReadsSeeOnlyLatestRevision 覆盖 functional[0]：四个读方法都只看当前行。
func TestReadsSeeOnlyLatestRevision(t *testing.T) {
	s := revisedStore(t)
	ctx := context.Background()

	obs, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.Equal(t, 11.0, obs.Value, "Observation 必须取最新修订，不是任取一行")

	latest, err := s.LatestObservation(ctx, IndVIX)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "2026-01-03", latest.Date)
	// 断 value 而不只断 ts：最大 ts 与「那天有没有修订」无关，只断 ts 的话裸表
	// 实现也照样通过。裸表在本夹具的插入顺序下返回 20（旧修订），视图返回 21。
	assert.Equal(t, 21.0, latest.Value, "最新那天有修订时，要取当前行不是任取一行")

	// LIMIT 不被重复行吃掉：裸表上 DESC LIMIT 2 全落在 01-03 的两个修订上
	// （实测 dates=[01-03 01-03]）—— 要最近 2 个交易日，去重后只拿到 1 个。
	// 走视图后每个业务键只剩一行，LIMIT 2 才真的是 2 天。
	win, err := s.SeriesWindow(ctx, IndVIX, "2026-01-03", 2)
	require.NoError(t, err)
	require.Len(t, win, 2)
	assert.Equal(t, []string{"2026-01-02", "2026-01-03"},
		[]string{win[0].Date, win[1].Date}, "LIMIT 2 必须是 2 个不同交易日")
	assert.Equal(t, []float64{11, 21}, []float64{win[0].Value, win[1].Value})

	since, err := s.SeriesSince(ctx, IndVIX, "2026-01-02", "2026-01-03")
	require.NoError(t, err)
	require.Len(t, since, 2, "不得混进旧修订")
	assert.Equal(t, []float64{11, 21}, []float64{since[0].Value, since[1].Value})
}

// TestEvalDatesDeduplicatesRevisions 覆盖 functional[1]（裁决 R1）。
//
// EvalDates 不经 obsSelect —— 它是第五个读取点，内联的裸表查询。迁移后只要有
// 修订，它就返回重复日期，回测会把同一天评估两次。而库里现全是单版本 ⇒ 缺陷
// 要到第一次真实修订才发作。
func TestEvalDatesDeduplicatesRevisions(t *testing.T) {
	s := revisedStore(t)

	dates, err := s.EvalDates(context.Background(), "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-02", "2026-01-03"}, dates,
		"同一天的两个修订只能产出一个评估日")
}

// TestSingleRevisionReadsUnchanged 覆盖 boundary[1]：无修订时行为不变。
//
// 单版本是当前生产库的形态，这条是回归护栏——改 FROM 不该让既有语义漂移。
func TestSingleRevisionReadsUnchanged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertObservations(ctx, []Observation{
		{Date: "2026-02-01", Indicator: IndVIX, Value: 15, Source: "fred", FetchedAt: "2026-02-02T00:00:00Z"},
		{Date: "2026-02-02", Indicator: IndVIX, Value: 16, Source: "fred", FetchedAt: "2026-02-03T00:00:00Z"},
	}))

	obs, err := s.Observation(ctx, IndVIX, "2026-02-01")
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.Equal(t, 15.0, obs.Value)
	assert.Equal(t, "fred", obs.Source, "视图要保留全部业务列，不只是 ts/value")

	latest, err := s.LatestObservation(ctx, IndVIX)
	require.NoError(t, err)
	assert.Equal(t, "2026-02-02", latest.Date)

	since, err := s.SeriesSince(ctx, IndVIX, "2026-02-01", "2026-02-02")
	require.NoError(t, err)
	assert.Len(t, since, 2)

	dates, err := s.EvalDates(ctx, "2026-02-01", "2026-02-02")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-02-01", "2026-02-02"}, dates)
}

// TestReadsFailLoudlyWhenViewMissing 覆盖 error_handling[0]。
//
// 钉的是「不得把错误吞成空集」：视图没了要报错，而不是安静地返回 0 行——后者
// 会让回测得出「这段时间没有数据」的结论，和真的没数据完全同形。
func TestReadsFailLoudlyWhenViewMissing(t *testing.T) {
	s := revisedStore(t)
	_, err := s.db.Exec(`DROP VIEW v_macro_current`)
	require.NoError(t, err)
	ctx := context.Background()

	_, err = s.SeriesSince(ctx, IndVIX, "2026-01-01", "2026-01-31")
	require.Error(t, err, "视图缺失必须报错，不能吞成空集")
	assert.Contains(t, err.Error(), "v_macro_current", "错误要能定位到视图")

	_, err = s.EvalDates(ctx, "2026-01-01", "2026-01-31")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v_macro_current")
}

// TestNoBareTableReadsOutsideMigration 覆盖 functional[2]：包内裸表读取守卫。
//
// 判据用**带词边界**的正则：朴素子串会被 `FROM macro_observations_v1` 假阳命中
// （实测：同一行文本，带词边界 0 次、朴素子串 1 次），而迁移正需要读 _v1。
//
// 豁免逐文件逐行列出并断言条数**恰为** len(allowed)：新增一处裸读即变红，删掉
// 一处豁免也变红——两个方向都钉住，否则豁免清单会慢慢变成整文件豁免。
//
// ⚠️ schemaDDL() 里的视图 SQL 由 bitemporal.CurrentQuery 在**运行时**生成，源码
// 里没有 `FROM macro_observations` 这个字面量 ⇒ 本判据不会命中它，**不需要**为它
// 加豁免。（写在这里免得后人看见视图 SQL 就去加一条多余的豁免。）
func TestNoBareTableReadsOutsideMigration(t *testing.T) {
	// 逐条列出而非整文件豁免 —— 这些文件将来新增的裸读同样要被看见。
	//
	// 两类合法裸读，理由不同：
	//   - migrate.go：迁移的工作就是在视图还不存在、或正要重建它的时候搬数据。
	//     🔴 它的两处 COUNT(*) **必须留在基表**：数的是**全部修订**，改走视图就
	//     只数当前行，迁移的三计数会当场失去意义。
	//   - store.go 的 UpsertObservations：写冲突检测要回答「这个确切的
	//     (ts, indicator, fetched_at) 是否已存在」，而视图按定义只有当前行
	//     （实测：同键两个修订时，基表查旧修订得 1 行、视图得 0 行），据视图判
	//     「不存在」而插入会直接撞主键。这是**写路径**的裸读，与「读路径必须走
	//     视图」不冲突。
	// 每条带 SQL 特征片段 ⇒ 匹配是**逐处**的，不是整文件放行：同一文件里新增
	// 一处不同的裸读会因匹配不上而红，新增一处相同的会因总数超标而红。
	type bareRead struct{ file, sqlFragment, why string }
	allowedBare := []bareRead{
		{"migrate.go", "SELECT COUNT(*) FROM macro_observations", "迁移前行数 RowsBefore"},
		{"migrate.go", "SELECT COUNT(*) FROM macro_observations", "迁移后行数 RowsAfter"},
		{"store.go", "SELECT value FROM macro_observations", "UpsertObservations 的写冲突检测"},
	}

	bare := regexp.MustCompile(`FROM\s+macro_observations\b`)
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	var found []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			// 行注释里提到表名不是读取。不跳过的话，任何人在注释里解释视图
			// 是怎么回事都会触发假阳，守卫很快会被当成噪音整文件豁免掉 ——
			// 而那正是本判据要防的。
			// ⚠️ 已知边界：只跳行注释，块注释 /* */ 不处理（本包没有，且把
			// SQL 藏在块注释里也不构成读取）。
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if bare.MatchString(line) {
				found = append(found, fmt.Sprintf("%s:%d: %s", f, i+1, trimmed))
			}
		}
	}

	for _, hit := range found {
		matched := slices.ContainsFunc(allowedBare, func(a bareRead) bool {
			return strings.HasPrefix(hit, a.file+":") && strings.Contains(hit, a.sqlFragment)
		})
		assert.True(t, matched,
			"非测试源码只许经 v_macro_current 读；这处裸读不在豁免清单里：%s", hit)
	}
	assert.Len(t, found, len(allowedBare),
		"豁免条数必须恰为 %d —— 多了是新增裸读，少了是豁免清单没跟上实现。\n  清单：%v\n  实际：%v",
		len(allowedBare), allowedBare, found)
}

// ---------------------------------------------------------------------------
// TASK-005 写路径改追加：裸 INSERT + 冲突按 value 分流（C6/AD-7）
//
// Context Checkpoint: done_criteria → test mapping (append-on-write)
// functional[0]     追加而非覆盖：新 fetched_at ⇒ 裸表 2 行，Observation 看到新值      → TestUpsertAppendsNewRevision
// functional[1]     同三段同值幂等：写两次不报错，裸表仍 1 行                            → TestUpsertSameTripleSameValueIsNoop
// functional[2]     同三段异值响亮失败 + 整批回滚，文案含 indicator 与 ts               → TestUpsertSameTripleDifferentValueFails
// functional[3]     源码钉死：不得出现 INSERT OR REPLACE / OR IGNORE INTO macro_observations → TestWritePathHasNoReplaceOrIgnore
// boundary[0](a)    裸 INSERT 造 NULL 前置行，API 写同三段非 NULL ⇒ 报错                → TestUpsertNullVersusNonNullConflicts
// boundary[0](b)    两边都 NULL 判「相同」——直接调比对函数                              → TestSameObservationValueNullSemantics
// boundary[1]       批量中途冲突 ⇒ 整批回滚，前面已成功的行也不落盘                      → TestUpsertRollsBackWholeBatch
// error_handling[0] 空切片是 no-op，不报错                                              → TestUpsertEmptySliceIsNoop
// non_functional[0] 既有契约改写方向（同三段异值期望 error）                             → TestStoreUpsertIdempotentAndWindows（上方，已改写）

// baseRowCount 直接数**基表**行数。
//
// 刻意不走视图：本任务全部断言的对象是「旧修订有没有被保留」，而视图按定义只
// 显示当前行 —— 用它数行永远得不到 2，测不出覆盖与追加的区别。
func baseRowCount(t *testing.T, s *Store, ts, indicator string) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow(
		`SELECT COUNT(*) FROM macro_observations WHERE ts = ? AND indicator = ?`,
		ts, indicator).Scan(&n))
	return n
}

// TestUpsertAppendsNewRevision 是本任务的核心：新的 fetched_at 是新增行，不是覆盖。
func TestUpsertAppendsNewRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	}))
	require.NoError(t, s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 11, Source: "fred", FetchedAt: "2026-08-14T00:00:00Z"},
	}))

	assert.Equal(t, 2, baseRowCount(t, s, "2026-01-02", IndVIX),
		"旧修订必须还在 —— 保留它正是这次迁移的全部目的")

	got, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 11.0, got.Value, "读路径仍只看到当前行")
}

// TestUpsertSameTripleSameValueIsNoop：采集器重跑同一批没有信息差，吞掉即可。
func TestUpsertSameTripleSameValueIsNoop(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	batch := []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	}

	require.NoError(t, s.UpsertObservations(ctx, batch))
	require.NoError(t, s.UpsertObservations(ctx, batch), "同三段同值重写不得报错")
	assert.Equal(t, 1, baseRowCount(t, s, "2026-01-02", IndVIX), "也不得插出第二行")
}

// TestUpsertSameTripleDifferentValueFails：同一次取回给出两个值 ⇒ 响亮失败。
//
// 这是 OR IGNORE 会掩盖掉的那一种：它把「重复」和「矛盾」揉成一种，而后者是
// 数据源或采集器出了问题，静默吞掉等于把一个真实故障变成看不见的数据损坏。
func TestUpsertSameTripleDifferentValueFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	}))

	err := s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 99, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), IndVIX, "文案要指名是哪个指标")
	assert.Contains(t, err.Error(), "2026-01-02", "文案要指名是哪天")

	assert.Equal(t, 1, baseRowCount(t, s, "2026-01-02", IndVIX), "失败批次不留半个状态")
	got, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	assert.Equal(t, 10.0, got.Value, "原值不得被改动")
}

// TestUpsertRollsBackWholeBatch 覆盖 boundary[1]：批内前面已成功的行也要回滚。
func TestUpsertRollsBackWholeBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	}))

	// 批内第一行是全新的（本可成功），第二行与已有行冲突
	err := s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-05", Indicator: IndVIX, Value: 50, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
		{Date: "2026-01-02", Indicator: IndVIX, Value: 99, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	})
	require.Error(t, err)

	assert.Equal(t, 0, baseRowCount(t, s, "2026-01-05", IndVIX),
		"批内前面已成功的行也必须回滚 —— 半批落盘比整批失败更难排查")
	assert.Equal(t, 1, baseRowCount(t, s, "2026-01-02", IndVIX))
}

// TestUpsertEmptySliceIsNoop 覆盖 error_handling[0]。
func TestUpsertEmptySliceIsNoop(t *testing.T) {
	s := newTestStore(t)
	assert.NoError(t, s.UpsertObservations(context.Background(), nil))
	assert.NoError(t, s.UpsertObservations(context.Background(), []Observation{}))
}

// TestUpsertNullVersusNonNullConflicts 覆盖 boundary[0](a)。
//
// ⚠️ Observation.Value 是 float64，经公开 API **写不出 NULL**，所以前置行只能用
// 裸 INSERT 造。一边 NULL 一边非 NULL 是**异值**，必须报错。
func TestUpsertNullVersusNonNullConflicts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.db.Exec(
		`INSERT INTO macro_observations (ts, indicator, value, source, fetched_at) VALUES (?,?,NULL,?,?)`,
		"2026-01-02", IndVIX, "fred", "2026-07-14T00:00:00Z")
	require.NoError(t, err)

	err = s.UpsertObservations(ctx, []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	})
	require.Error(t, err, "已有行 value 为 NULL 而新值非 NULL，是异值")
	assert.Equal(t, 1, baseRowCount(t, s, "2026-01-02", IndVIX))
}

// TestSameObservationValueNullSemantics 覆盖 boundary[0](b)：直接调比对函数。
//
// 「两边都 NULL 判相同」走不到 UpsertObservations（API 写不出 NULL），只能直接
// 测比对函数本身 —— 否则这条语义没有任何测试覆盖。
func TestSameObservationValueNullSemantics(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }

	assert.True(t, sameObservationValue(nil, nil), "两边都 NULL 判相同（SQL 的 NULL != NULL 在这里是错的语义）")
	assert.False(t, sameObservationValue(nil, ptr(0)), "一边 NULL 一边 0 是异值，0 不是「没有值」")
	assert.False(t, sameObservationValue(ptr(0), nil))
	assert.True(t, sameObservationValue(ptr(10), ptr(10)))
	assert.False(t, sameObservationValue(ptr(10), ptr(11)))
}

// TestWritePathHasNoReplaceOrIgnore 覆盖 functional[3]：源码钉死两种写法。
//
// OR REPLACE 是本次要修的缺陷本身；OR IGNORE 把「重复」和「矛盾」揉成一种，
// 会让「同一次取回给出两个值」被静默吞掉 —— 那正是最该响的一种。
func TestWritePathHasNoReplaceOrIgnore(t *testing.T) {
	src, err := os.ReadFile("store.go")
	require.NoError(t, err)
	body := strings.ToUpper(string(src))

	assert.NotContains(t, body, "INSERT OR REPLACE INTO MACRO_OBSERVATIONS")
	assert.NotContains(t, body, "INSERT OR IGNORE INTO MACRO_OBSERVATIONS")
}

// ---------------------------------------------------------------------------
// TASK-006 as-of 只读副本：obsFrom() 按 asOf 选形态（C4/AD-8/AD-9）
//
// Context Checkpoint: done_criteria → test mapping (as-of)
// functional[0]     as-of 看历史版本；AsOf(远未来) 与不带 as-of 逐值相同（O7）      → TestAsOfSeesHistoricalRevision / TestAsOfFarFutureMatchesCurrent
// functional[1]     边界含端点（<= 而非 <）；生产 Nano 形态下整秒 as-of 含同秒小数行 → TestAsOfIncludesEndpoint / TestAsOfNanoPrecisionBoundary
// functional[2]     五个读方法在 as-of 形态下参数顺序正确，逐值断言                  → TestAsOfAllReadersParameterOrder
// boundary[0]       C8：t 之前无任何行 ⇒ (nil, nil)，不回退到最早那行                → TestAsOfExcludesKeysWithNoRowsBefore
// boundary[1]       AsOf 不改原 Store；副本与原 Store 共用同一个 *sql.DB             → TestAsOfIsReadOnlyCopySharingDB
// boundary[2]       写路径裸读的**行为层**守卫（R3 覆盖缺口）                        → TestUpsertOlderRevisionAfterNewerIsSwallowed
// error_handling[0] AsOf("") 等价于当前形态                                          → TestAsOfEmptyEqualsCurrent
// non_functional[1] C5 在本任务的尺是「WHERE/ORDER/LIMIT 文本逐字不变」              → verify_by: review（git diff）

// asOfStore 造 DoD 指定的 as-of 夹具：同键两个修订 + 另一个单版本键。
//
// 走公开 API：三段主键下不同 fetched_at 即不同主键，UpsertObservations 会追加
// 而非覆盖（TASK-005 起「同三段异值」才报错）。
func asOfStore(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	ctx := context.Background()
	for _, o := range []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
		{Date: "2026-01-02", Indicator: IndVIX, Value: 11, Source: "fred", FetchedAt: "2026-08-14T00:00:00Z"},
		{Date: "2026-01-03", Indicator: IndVIX, Value: 20, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	} {
		require.NoError(t, s.UpsertObservations(ctx, []Observation{o}))
	}
	return s
}

func TestAsOfSeesHistoricalRevision(t *testing.T) {
	s := asOfStore(t)
	ctx := context.Background()

	now, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, now)
	assert.Equal(t, 11.0, now.Value, "不带 as-of 看当前行")

	// 07-20 时点上，08-14 那次修订还没发生
	past, err := s.AsOf("2026-07-20T00:00:00Z").Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, past)
	assert.Equal(t, 10.0, past.Value, "as-of 要看到当时那个值，而不是后来修订的")
}

// TestAsOfFarFutureMatchesCurrent 是 TASK-008 判据三唯一的事前保险（O7）。
func TestAsOfFarFutureMatchesCurrent(t *testing.T) {
	s := asOfStore(t)
	ctx := context.Background()
	future := s.AsOf("2099-01-01T00:00:00Z")

	cur, err := s.SeriesSince(ctx, IndVIX, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	fut, err := future.SeriesSince(ctx, IndVIX, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	assert.Equal(t, cur, fut, "远未来 as-of 必须与不带 as-of 逐值相同")

	curEval, err := s.EvalDates(ctx, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	futEval, err := future.EvalDates(ctx, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	assert.Equal(t, curEval, futEval)
}

// TestAsOfIncludesEndpoint：<= 而非 <。差这一位会让每个历史视图静默偏移一个修订。
func TestAsOfIncludesEndpoint(t *testing.T) {
	s := asOfStore(t)
	got, err := s.AsOf("2026-08-14T00:00:00Z").Observation(context.Background(), IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 11.0, got.Value, "as-of 恰好等于某个 fetched_at 时，那个修订应当被看见")
}

// TestAsOfNanoPrecisionBoundary 钉住生产形态下的实际行为。
//
// 生产 fetched_at 形如 2026-07-14T05:42:08.150777000Z，而比较是**字符串字典序**：
// '.'(0x2E) < 'Z'(0x5A)，所以 --as-of 2026-07-14T05:42:08Z 会**包含**同秒带小数
// 的那行。方向无害，但整秒夹具把这点完全掩盖，必须显式钉住而不是留给人去推。
func TestAsOfNanoPrecisionBoundary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, o := range []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-13T00:00:00.000000000Z"},
		{Date: "2026-01-02", Indicator: IndVIX, Value: 11, Source: "fred", FetchedAt: "2026-07-14T05:42:08.150777000Z"},
	} {
		require.NoError(t, s.UpsertObservations(ctx, []Observation{o}))
	}

	got, err := s.AsOf("2026-07-14T05:42:08Z").Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 11.0, got.Value,
		"'.'(0x2E) < 'Z'(0x5A) ⇒ 整秒 as-of 字典序上大于同秒带小数的 fetched_at，故包含它")
}

// TestAsOfAllReadersParameterOrder 覆盖 functional[2]：五个读方法各一例。
//
// 🔴 逐值断言而非「非空」：as-of 的 ? 在子查询里、位置先于外层 WHERE 的参数，
// 弄反**不报错**——SeriesWindow 有 indicator/end/LIMIT 三个外层参数，插错位置
// 可能返回**非空但错**的结果，只断非空等于没断。
func TestAsOfAllReadersParameterOrder(t *testing.T) {
	s := asOfStore(t)
	ctx := context.Background()
	past := s.AsOf("2026-07-20T00:00:00Z") // 08-14 那次修订尚未发生

	obs, err := past.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.Equal(t, 10.0, obs.Value)

	latest, err := past.LatestObservation(ctx, IndVIX)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "2026-01-03", latest.Date)
	assert.Equal(t, 20.0, latest.Value)

	win, err := past.SeriesWindow(ctx, IndVIX, "2026-01-03", 2)
	require.NoError(t, err)
	require.Len(t, win, 2)
	assert.Equal(t, []float64{10, 20}, []float64{win[0].Value, win[1].Value})

	// n=1 恰 1 行：证明 LIMIT 的参数没被前置的 as-of 挤走
	win1, err := past.SeriesWindow(ctx, IndVIX, "2026-01-03", 1)
	require.NoError(t, err)
	require.Len(t, win1, 1, "LIMIT 参数插错位置时这里会返回别的行数")
	assert.Equal(t, 20.0, win1[0].Value)

	since, err := past.SeriesSince(ctx, IndVIX, "2026-01-02", "2026-01-03")
	require.NoError(t, err)
	require.Len(t, since, 2)
	assert.Equal(t, []float64{10, 20}, []float64{since[0].Value, since[1].Value})

	dates, err := past.EvalDates(ctx, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-02", "2026-01-03"}, dates)
}

// TestAsOfExcludesKeysWithNoRowsBefore 覆盖 C8。
func TestAsOfExcludesKeysWithNoRowsBefore(t *testing.T) {
	s := asOfStore(t)
	// 全部 fetched_at 都在 07-14 之后 ⇒ 07-01 时点上这个键还不存在
	got, err := s.AsOf("2026-07-01T00:00:00Z").Observation(context.Background(), IndVIX, "2026-01-02")
	require.NoError(t, err, "空结果不是错误")
	assert.Nil(t, got, "t 之前没有任何行的键要被排除，不是回退到最早那行")
}

// TestAsOfIsReadOnlyCopySharingDB 覆盖 boundary[1]（AD-8）。
func TestAsOfIsReadOnlyCopySharingDB(t *testing.T) {
	s := asOfStore(t)
	ctx := context.Background()
	past := s.AsOf("2026-07-20T00:00:00Z")

	require.Same(t, s.db, past.db, "副本必须共用同一个 *sql.DB —— Close() 一次即可")

	// 取过副本之后，原 Store 仍看当前值
	now, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, now)
	assert.Equal(t, 11.0, now.Value, "AsOf 不得改动原 Store")
}

// TestAsOfEmptyEqualsCurrent 覆盖 error_handling[0]：不留未定义语义。
func TestAsOfEmptyEqualsCurrent(t *testing.T) {
	s := asOfStore(t)
	ctx := context.Background()

	got, err := s.AsOf("").Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 11.0, got.Value, `AsOf("") 等价于当前形态`)
}

// TestUpsertOlderRevisionAfterNewerIsSwallowed 覆盖 boundary[2]。
//
// 🔴 这是「写路径的冲突检测必须裸读基表」的**行为层**守卫。源码守卫（豁免清单）
// 只报「清单对不上」这个记账理由，读到的人未必明白为什么不能改；把检测改走
// v_macro_current 时**只有它红、零行为测试红**（test-m4c-a 在 TASK-005 的变异 R3）。
//
// 机制：视图只有当前行。更新的修订存在时，视图里看不到更旧那行 ⇒ 冲突检测判
// 「不存在」⇒ 直接 INSERT ⇒ UNIQUE constraint failed (1555)。
//
// ⚠️ 这是真实场景不是构造的：补跑历史（manual_backfill 路径）就是这个形状 ——
// 先有当前值，再回填更早的抓取。
func TestUpsertOlderRevisionAfterNewerIsSwallowed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	older := []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 10, Source: "fred", FetchedAt: "2026-07-14T00:00:00Z"},
	}
	newer := []Observation{
		{Date: "2026-01-02", Indicator: IndVIX, Value: 11, Source: "fred", FetchedAt: "2026-08-14T00:00:00Z"},
	}

	require.NoError(t, s.UpsertObservations(ctx, older))
	require.NoError(t, s.UpsertObservations(ctx, newer))

	// 关键一步：更新的修订已在库里，此时重写**更旧**的那个修订
	require.NoError(t, s.UpsertObservations(ctx, older),
		"重写已存在的旧修订必须被静默吞掉；冲突检测若走视图会看不到它而撞主键")

	assert.Equal(t, 2, baseRowCount(t, s, "2026-01-02", IndVIX), "不得插出第三行")
	got, err := s.Observation(ctx, IndVIX, "2026-01-02")
	require.NoError(t, err)
	assert.Equal(t, 11.0, got.Value, "当前行仍是更新的那个修订")
}
