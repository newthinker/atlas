package crisis

// Context Checkpoint: done_criteria → test mapping (schema)
// functional[0]     macro_observations DDL 含三段主键 + fetched_at TEXT NOT NULL → TestSchemaCreatesBitemporalShape
// functional[1]     v_macro_current 存在且 DDL 含 MAX(fetched_at)（AD-2）        → TestSchemaCreatesCurrentView
// functional[2]     视图取最新修订 [11, 20]                                       → TestCurrentViewPicksLatestRevision
// functional[3]     bitemporal.NewSpec( 在包内非测试源码恰 1 次且在 schema.go     → TestObsSpecIsSingleInstance
// boundary[0]       乱序写入（先大后小 fetched_at）仍取较大那行                   → TestCurrentViewIgnoresInsertOrder
// boundary[1]       schemaDDL() 幂等：连跑两次不报错，表/索引/视图各恰 1 份       → TestSchemaDDLIsIdempotent
// error_handling[0] mustSpec 只在 NewSpec 出错时 panic，文案含包名与原始错误       → verify_by: review（schema.go 的 mustSpec）

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaCreatesBitemporalShape：新库直接是三段主键。
func TestSchemaCreatesBitemporalShape(t *testing.T) {
	s := newTestStore(t)

	var ddl string
	require.NoError(t, s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE name='macro_observations'`).Scan(&ddl))
	// sqlite_master 存的是 DDL 原文（含缩进与对齐空格）；折叠空白后再断言，
	// 断的是列的形状，不是源码的对齐方式。
	flat := strings.Join(strings.Fields(ddl), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator, fetched_at)")

	// fetched_at 必须 NOT NULL：SQLite 的主键**允许 NULL**，空值会同时绕开
	// 唯一性约束与 MAX(fetched_at) 关联。现在库里零空值 ⇒ 加得上。
	assert.Contains(t, flat, "fetched_at TEXT NOT NULL")
}

// TestSchemaCreatesCurrentView：视图存在且由基座生成。
func TestSchemaCreatesCurrentView(t *testing.T) {
	s := newTestStore(t)

	var ddl string
	require.NoError(t, s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='view' AND name='v_macro_current'`).Scan(&ddl))
	require.Contains(t, ddl, "MAX(fetched_at)", "当前行由 revision 列派生，不靠 is_current 列")
}

// TestCurrentViewPicksLatestRevision 是本任务的核心行为。
func TestCurrentViewPicksLatestRevision(t *testing.T) {
	s := newTestStore(t)
	insertObs(t, s.db, "2026-01-02", "vix", 10, "2026-07-14T00:00:00Z")
	insertObs(t, s.db, "2026-01-02", "vix", 11, "2026-08-14T00:00:00Z") // 修订
	insertObs(t, s.db, "2026-01-03", "vix", 20, "2026-07-14T00:00:00Z")

	got := queryFloats(t, s.db,
		`SELECT value FROM v_macro_current WHERE indicator='vix' ORDER BY ts`)
	assert.Equal(t, []float64{11, 20}, got, "同键取 fetched_at 最大的那行")
}

// TestCurrentViewIgnoresInsertOrder：乱序写入不影响结果。
//
// 回填不保证按发布时间顺序落库；当前行从 revision 列派生（而非靠一个写入时
// 维护的 is_current 列）的直接体现就是——插入顺序与结果无关。
func TestCurrentViewIgnoresInsertOrder(t *testing.T) {
	s := newTestStore(t)
	insertObs(t, s.db, "2026-01-02", "vix", 11, "2026-08-14T00:00:00Z") // 先插较大的
	insertObs(t, s.db, "2026-01-02", "vix", 10, "2026-07-14T00:00:00Z") // 后插较小的

	got := queryFloats(t, s.db,
		`SELECT value FROM v_macro_current WHERE indicator='vix' ORDER BY ts`)
	assert.Equal(t, []float64{11}, got, "取 fetched_at 最大的那行，与插入顺序无关")
}

// TestSchemaDDLIsIdempotent：同一库连跑两次 schemaDDL 不报错且不产生第二份对象。
//
// NewStore 每次打开库都会执行它——重复执行是常态，不是异常路径。
func TestSchemaDDLIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	_, err := s.db.Exec(schemaDDL())
	require.NoError(t, err, "第二次执行 schemaDDL 应当无错")

	for _, tc := range []struct{ typ, name string }{
		{"table", "macro_observations"},
		{"table", "crisis_evaluations"},
		{"index", "idx_macro_obs_ind_ts"},
		{"index", "idx_crisis_eval_ind_ts"},
		{"view", "v_macro_current"},
	} {
		var n int
		require.NoError(t, s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?`, tc.typ, tc.name).Scan(&n))
		assert.Equal(t, 1, n, "%s %s 应当恰 1 份", tc.typ, tc.name)
	}
}

// TestObsSpecIsSingleInstance：Spec 只装配一次。
//
// 两份 Spec 意味着「表名/键列/revision 列」有两个副本，改一处不会让另一处变红。
//
// 断言手法是扫源码而非运行时反射：Spec 的字段未导出，运行时看不出「有几份」，
// 而「只装配一次」本来就是一条源码层的性质。
// （mustSpec 的 panic 路径不在此断言——包级变量出错会在 init 阶段就崩，
// 测试体内任何断言都执行不到；那条由 error_handling[0] 的 review 覆盖。）
func TestObsSpecIsSingleInstance(t *testing.T) {
	counts := packageNewSpecCounts(t)
	assert.Equal(t, 1, counts["schema.go"], "obsSpec 的装配点应当在 schema.go")
	total := 0
	for _, n := range counts {
		total += n
	}
	assert.Equal(t, 1, total,
		"bitemporal.NewSpec( 在包内非测试源码只应出现一次；别在 migrate.go / store.go 里再造一个，实际分布 %v", counts)
}

func insertObs(t *testing.T, db *sql.DB, ts, indicator string, value float64, fetchedAt string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?)`,
		ts, indicator, value, "test", fetchedAt)
	require.NoError(t, err)
}

func queryFloats(t *testing.T, db *sql.DB, query string) []float64 {
	t.Helper()
	rows, err := db.Query(query)
	require.NoError(t, err)
	defer rows.Close()
	out := []float64{}
	for rows.Next() {
		var v float64
		require.NoError(t, rows.Scan(&v))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}

// packageNewSpecCounts 统计包内**非测试**源码里 `bitemporal.NewSpec(` 的出现次数，
// 按文件名分组。分组而非只给总数：断言失败时能直接指出多出来的那一份在哪。
func packageNewSpecCounts(t *testing.T) map[string]int {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	counts := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		if n := strings.Count(string(src), "bitemporal.NewSpec("); n > 0 {
			counts[f] = n
		}
	}
	return counts
}
