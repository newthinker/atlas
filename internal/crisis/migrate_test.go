package crisis

// Context Checkpoint: done_criteria → test mapping (migrate)
// functional[0]     老形状 3 行迁移：三计数=3、新表三段主键+NOT NULL、双向 EXCEPT 各 0 行 → TestMigrateConvertsLegacyShape
// functional[0]附   _new 表的 DDL 复用 TASK-001 的生成函数，migrate.go 不另写建表 SQL      → TestMigrateReusesSchemaDDL
// functional[1]     幂等：第二次 AlreadyMigrated=true，行数与 sqlite_master 计数均不变     → TestMigrateIsIdempotent
// functional[2]     旧表保留为 macro_observations_v1 且行数一致（C3 回滚/对照）            → TestMigrateKeepsLegacyTable
// functional[3]     C2 索引重建：idx_macro_obs_ind_ts 的 tbl_name 是新表                   → TestMigrateRebuildsIndex
// boundary[0]       NULL fetched_at ⇒ 报错，且失败后库仍老形状、无 _v1、无 _new（原子性）  → TestMigrateRejectsNullFetchedAtAtomically
// boundary[1]       空库三计数为 0；已迁移库 AlreadyMigrated=true；探形状不看 _v1          → TestMigrateEmptyDB / TestMigrateAlreadyMigrated / TestMigrateShapeProbeIgnoresLegacyTable
// error_handling[0] 库文件不存在 ⇒ 报错、文案含路径、且不创建库                            → TestMigrateMissingDBFile
// non_functional[0] go build + ./internal/crisis ./cmd/atlas 全绿                          → 见 discovery

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacySchema 是 TASK-001 之前的表形状：两段主键、fetched_at 可空。
//
// 刻意在测试里写死而不从别处引用：它描述的是**历史**，源码里已经没有这份
// 形状了。迁移的被测对象正是「从这个形状出发」，所以它必须留在测试中。
const legacySchema = `
CREATE TABLE macro_observations (
	ts          TEXT NOT NULL,
	indicator   TEXT NOT NULL,
	value       REAL,
	source      TEXT,
	fetched_at  TEXT,
	PRIMARY KEY (ts, indicator)
);
CREATE INDEX idx_macro_obs_ind_ts ON macro_observations(indicator, ts);`

type legacyRow struct {
	ts        string
	indicator string
	value     float64
	fetchedAt any // string 或 nil（nil 用来造 NULL fetched_at 的老库）
}

// legacyDB 建一个老形状的库并塞入 rows，返回库路径。
func legacyDB(t *testing.T, rows ...legacyRow) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db := openRaw(t, path)
	_, err := db.Exec(legacySchema)
	require.NoError(t, err)
	for _, r := range rows {
		_, err := db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?)`,
			r.ts, r.indicator, r.value, "test", r.fetchedAt)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	return path
}

// openRaw 直接开库，不经 NewStore —— 测试要观察的正是 NewStore 之外的形状。
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func objectDDL(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var ddl string
	require.NoError(t, db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE name = ?`, name).Scan(&ddl))
	return ddl
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(query, args...).Scan(&n))
	return n
}

// objectExists 报告 sqlite_master 里有没有这个名字的对象（表/索引/视图都算）。
func objectExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	return countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, name) > 0
}

// objectNames 返回库内全部对象的 `type:name`（有序）。
//
// 用它而不是逐个 objectExists：后者只挡得住**你想得到的**那个残留名字，而
// 「迁移失败必须原子」这个不变量说的是**任何**中间产物都不该留下。逐个点名的
// 写法在实现换一个临时表名时就失效了，而且因为那个名字从没出现在实现里，
// 断言恒真、永远不会告诉你它已经失效（V-3 就是这么来的）。
func objectNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT type || ':' || name FROM sqlite_master ORDER BY type, name`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestMigrateConvertsLegacyShape：老形状 → 新形状，数据一行不差。
func TestMigrateConvertsLegacyShape(t *testing.T) {
	path := legacyDB(t,
		legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"},
		legacyRow{"2026-01-03", "vix", 20, "2026-07-14T00:00:00Z"},
		legacyRow{"2026-01-02", "move", 30, "2026-07-14T00:00:00Z"},
	)

	res, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)
	assert.False(t, res.AlreadyMigrated)
	assert.Equal(t, 3, res.RowsBefore)
	assert.Equal(t, 3, res.RowsAfter)
	assert.Equal(t, 3, res.ViewRows, "无重复键时视图行数 == 表行数")

	db := openRaw(t, path)
	// C1 在迁移路径上也要成立，不只对新建库。折叠空白后再断言形状（同 schema_test.go）。
	flat := strings.Join(strings.Fields(objectDDL(t, db, "macro_observations")), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator, fetched_at)")
	assert.Contains(t, flat, "fetched_at TEXT NOT NULL")

	// 逐值对照：视图与旧表互为子集 ⇒ 完全相同。单向为 0 只能说明一边没多出来。
	assert.Equal(t, 0, countRows(t, db,
		`SELECT COUNT(*) FROM (SELECT * FROM v_macro_current EXCEPT SELECT * FROM macro_observations_v1)`),
		"视图里不应有旧表没有的行")
	assert.Equal(t, 0, countRows(t, db,
		`SELECT COUNT(*) FROM (SELECT * FROM macro_observations_v1 EXCEPT SELECT * FROM v_macro_current)`),
		"旧表里不应有视图看不到的行")
}

// TestMigrateReusesSchemaDDL 钉住「表形状只有一个副本」（同 AD-3 的risk源）。
//
// 断言手法是扫源码：migrate.go 里不得出现 CREATE TABLE —— 新表必须由
// TASK-001 的 schemaDDL()/tablesDDL 生成。运行时看不出「SQL 是从哪来的」，
// 这本来就是一条源码层的性质。
func TestMigrateReusesSchemaDDL(t *testing.T) {
	src, err := os.ReadFile("migrate.go")
	require.NoError(t, err)
	body := string(src)

	assert.NotContains(t, strings.ToUpper(body), "CREATE TABLE",
		"migrate.go 不得另写建表 SQL —— 那是表形状的第二个副本，C1 会在迁移路径上悄悄漂移")
	assert.Contains(t, body, "schemaDDL()",
		"新表必须由 TASK-001 的 schemaDDL() 生成")
}

// TestMigrateIsIdempotent：跑第二次什么都不做。
func TestMigrateIsIdempotent(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	db := openRaw(t, path)
	before := sqliteMasterCounts(t, db)
	rowsBefore := countRows(t, db, `SELECT COUNT(*) FROM macro_observations`)

	res, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)
	assert.True(t, res.AlreadyMigrated, "第二次必须是 no-op，不能再搬一遍")

	assert.Equal(t, before, sqliteMasterCounts(t, db), "sqlite_master 中表/索引/视图数量不得变化")
	assert.Equal(t, rowsBefore, countRows(t, db, `SELECT COUNT(*) FROM macro_observations`),
		"不得再搬一遍数据")
}

// sqliteMasterCounts 按 type 统计对象数，用于「什么都没变」这类断言。
func sqliteMasterCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.Query(`SELECT type, COUNT(*) FROM sqlite_master GROUP BY type`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var typ string
		var n int
		require.NoError(t, rows.Scan(&typ, &n))
		out[typ] = n
	}
	require.NoError(t, rows.Err())
	return out
}

// TestMigrateKeepsLegacyTable：旧表留着，可逐值对照（C3）。
func TestMigrateKeepsLegacyTable(t *testing.T) {
	path := legacyDB(t,
		legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"},
		legacyRow{"2026-01-03", "vix", 20, "2026-07-14T00:00:00Z"},
	)
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	db := openRaw(t, path)
	assert.Equal(t, 2, countRows(t, db, `SELECT COUNT(*) FROM macro_observations_v1`),
		"旧表必须保留且行数一致 —— 回滚要把它 RENAME 回去（一次），逐值对照也靠它")
}

// TestMigrateRebuildsIndex 是 C2，最容易漏的一条。
//
// RENAME 后索引跟着旧表走（实测：tbl_name 变成 macro_observations_v1），而
// 索引名在 sqlite 里是全局唯一的 —— 此时 CREATE INDEX IF NOT EXISTS 会因
// 同名已存在而静默跳过，新表永远没有索引。不报错，只变慢。
func TestMigrateRebuildsIndex(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	_, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)

	db := openRaw(t, path)
	var tbl string
	require.NoError(t, db.QueryRow(
		`SELECT tbl_name FROM sqlite_master WHERE type='index' AND name='idx_macro_obs_ind_ts'`).Scan(&tbl))
	assert.Equal(t, "macro_observations", tbl, "索引必须挂在新表上，不是 _v1")

	// 老库的索引本来就叫这个名、也挂在 macro_observations 上 ⇒ 上面那条断言
	// 在「压根没迁移」时同样成立。补一条确认真的迁了，否则它挡不住退化。
	flat := strings.Join(strings.Fields(objectDDL(t, db, "macro_observations")), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator, fetched_at)")
}

// TestMigrateRejectsNullFetchedAtAtomically：响亮失败好过静默塞空值。
//
// 生产库实测零空值，但 NOT NULL 拦下这种行是迁移的正确行为；本测试同时钉住
// 失败后库仍是老形状 —— 半迁移的库比没迁移危险得多。
func TestMigrateRejectsNullFetchedAtAtomically(t *testing.T) {
	path := legacyDB(t,
		legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"},
		legacyRow{"2026-01-03", "vix", 20, nil}, // NULL fetched_at
	)

	before := objectNames(t, openRaw(t, path))

	_, err := MigrateBitemporal(context.Background(), path)
	require.Error(t, err, "含 NULL fetched_at 的老库必须迁移失败")

	db := openRaw(t, path)
	flat := strings.Join(strings.Fields(objectDDL(t, db, "macro_observations")), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator)", "失败后必须仍是老形状")
	assert.NotContains(t, flat, "fetched_at TEXT NOT NULL")
	assert.False(t, objectExists(t, db, "macro_observations_v1"), "不得留下 _v1")
	// 全集比对：任何中间产物（不论叫什么名字）都会让这一条变红。
	// 这里曾写的是 `assert.False(objectExists(db, "macro_observations_new"))` —— 而
	// `macro_observations_new` 全仓只出现在那一行里（实现从不走 _new 路线），
	// 于是它恒真。恒真的断言与「被删掉」的唯一区别是它看起来还在守着。
	assert.Equal(t, before, objectNames(t, db), "迁移失败必须原子：不得留下任何中间产物")
	assert.Equal(t, 2, countRows(t, db, `SELECT COUNT(*) FROM macro_observations`), "数据一行不少")
}

// TestMigrateRejectsUnmigratedColumns：老库多一列时必须响亮失败，而不是静默丢掉它。
//
// 这一条钉的是 V-9：三个计数（RowsBefore/RowsAfter/ViewRows）全是 COUNT(*)，**行数口径**，
// 而搬运语句的列清单是硬编码的。老库多一列时那一列整列搬不过来，**三个计数一个都不会变**
// —— 它们照样相等、照样「全绿」，而数据已经丢了。
//
// ⇒ 判据在行维度、丢失在列维度：**检查的维度比它声称守住的集合窄一维，必然漏。**
// 所以这里不测计数，测的是「迁移拒绝执行」这个性质。
func TestMigrateRejectsUnmigratedColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra-col.db")
	db := openRaw(t, path)
	_, err := db.Exec(legacySchema)
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE macro_observations ADD COLUMN note TEXT`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?,?)`,
		"2026-01-02", "vix", 10.0, "test", "2026-07-14T00:00:00Z", "有值")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = MigrateBitemporal(context.Background(), path)
	require.Error(t, err, "老库多出的列搬不过来，必须失败而不是静默丢列")
	assert.Contains(t, err.Error(), "note", "错误必须点名是哪一列，否则运维不知道改哪儿")

	// 失败后库原样：note 列与它的值都还在，等人去修搬运语句。
	db = openRaw(t, path)
	assert.Equal(t, 1, countRows(t, db,
		`SELECT COUNT(*) FROM macro_observations WHERE note IS NOT NULL`),
		"失败必须原子，note 的值不能丢")
	assert.False(t, objectExists(t, db, legacyTableName), "不得留下 _v1")
}

// TestMigrateEmptyDB：空库迁移成功，三个计数均为 0。
func TestMigrateEmptyDB(t *testing.T) {
	path := legacyDB(t)

	res, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)
	assert.False(t, res.AlreadyMigrated)
	assert.Equal(t, 0, res.RowsBefore)
	assert.Equal(t, 0, res.RowsAfter)
	assert.Equal(t, 0, res.ViewRows)

	// 三个 0 单独看不区分「迁了一个空库」与「什么都没做」——必须同时钉住形状真的变了。
	db := openRaw(t, path)
	flat := strings.Join(strings.Fields(objectDDL(t, db, "macro_observations")), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator, fetched_at)")
	assert.True(t, objectExists(t, db, "v_macro_current"), "空库也要建出视图")
}

// TestMigrateAlreadyMigrated：已是新形状的库直接 no-op。
func TestMigrateAlreadyMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	s, err := NewStore(path) // NewStore 建出来的就是新形状
	require.NoError(t, err)
	require.NoError(t, s.Close())

	res, err := MigrateBitemporal(context.Background(), path)
	require.NoError(t, err)
	assert.True(t, res.AlreadyMigrated)
}

// TestMigrateShapeProbeIgnoresLegacyTable 钉住探形状的判据。
//
// 探形状必须看主键是否含 fetched_at，**不能看有没有 _v1 表** —— 回滚之后
// 库里可能既是老形状、又留着一张 _v1，按后者判会误判成「已迁移」而拒绝干活，
// 那正是最需要迁移的时候。
func TestMigrateShapeProbeIgnoresLegacyTable(t *testing.T) {
	path := legacyDB(t, legacyRow{"2026-01-02", "vix", 10, "2026-07-14T00:00:00Z"})
	db := openRaw(t, path)
	_, err := db.Exec(`CREATE TABLE macro_observations_v1 (ts TEXT)`) // 上一次回滚的残留
	require.NoError(t, err)
	require.NoError(t, db.Close())

	res, err := MigrateBitemporal(context.Background(), path)

	// 判据本身：不得因为看见 _v1 就当作「已迁移」。那是最需要迁移的时候。
	assert.False(t, res.AlreadyMigrated,
		"库仍是老形状就不能报『已迁移』—— 有没有 _v1 表与此无关")

	// 但也不能硬迁：_v1 占着名字，而它可能是上一次迁移留下的唯一历史副本，
	// 删掉它就把 C3 的回滚依据和逐值对照一起丢了。响亮失败、交给人处理。
	require.Error(t, err, "_v1 已占名时必须报错，不得自作主张删它")
	assert.Contains(t, err.Error(), legacyTableName, "文案要点名是哪张表挡路")

	// 失败后库必须原样：老形状还在、数据一行不少。
	db2 := openRaw(t, path)
	flat := strings.Join(strings.Fields(objectDDL(t, db2, "macro_observations")), " ")
	assert.Contains(t, flat, "PRIMARY KEY (ts, indicator)")
	assert.Equal(t, 1, countRows(t, db2, `SELECT COUNT(*) FROM macro_observations`))
}

// TestMigrateMissingDBFile：不存在的库要响亮失败，且不得顺手建一个。
//
// sql.Open("sqlite", "file:"+path) 默认会建库，所以这条必须显式实现；
// 不写这一步它恒不失败，而「迁移了一个空库并报成功」是最糟的结果。
func TestMigrateMissingDBFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.db")

	_, err := MigrateBitemporal(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path, "错误文案要带上路径，否则运维不知道打错了哪个")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "不得顺手建出一个空库")
}
