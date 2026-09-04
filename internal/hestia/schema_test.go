package hestia

// Context Checkpoint: done_criteria → test mapping (schema)
// functional[0]      业务列逐列由 fieldOrder 派生并按其顺序排列，列数恰好 83
//                                                        → TestObservationsColumnsDeriveFromFieldOrder
// functional[1]      metaColumns 与 reflect 取到的 Meta 字段名(snake_case)逐一相等
//                                                        → TestMetaColumnsMatchMetaStructByReflect
// functional[2]      视图基于 bitemporal 的当前行语义(published_at 派生)
//                                                        → TestCurrentViewDDLUsesBitemporal
// boundary[0]        三段 DDL 在真实 SQLite 上连续执行两次不报错且结果一致
//                                                        → TestDDLIsIdempotent
// error_handling[0]  DDL 不得出现 is_current 与 absent_fields 列
//                                                        → TestDDLHasNoDerivableStateColumns
// non_functional[0]  字段名不在 schema.go 出现字面量 → TestFieldNamesAppearOnlyInFieldsGo(fields_test.go)
// non_functional[1]  DDL 在真实 SQLite 上实际执行 → 下列各测试一律经 openWithSchema
// non_functional[2]  从真实库读结构逐项断言(PK/REAL 亲和性/NOT NULL/pending PK/视图定义)
//                                                        → TestObservationsStructureFromLiveDB、TestPendingStructureFromLiveDB、
//                                                          TestCurrentViewStructureFromLiveDB
//
// M1.5 的 TASK-001（hestia_runs）：
// functional[0]      runsDDL 建表 + 索引均 IF NOT EXISTS；列集精确 13 列且不含 fieldOrder 名字；
//                    run_at/finished_at/outcome/notified/duration_ms 五列 NOT NULL
//                                                        → TestRunsStructureFromLiveDB
//                    openWithSchema / TestDDLIsIdempotent 的 DDL 列表加 runsDDL()，幂等测试对 runs 表前后比对
//                                                        → TestDDLIsIdempotent

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func testSpec(t *testing.T) bitemporal.Spec {
	t.Helper()
	spec, err := bitemporal.NewSpec(TableObservations,
		[]string{"period", "period_type"}, "published_at")
	require.NoError(t, err)
	return spec
}

// openWithSchema 建一个真实 SQLite 库并执行三段 DDL。
//
// 全部结构断言都走真实库而不是比对 DDL 字符串：字符串对了而 SQLite 拒绝等于没验，
// 更要命的是字符串「看起来对」时结构可能已经错了——列建成 TEXT 时 Scan 进 float64
// 照样成功，写回读回全绿，而 MAX() 与 WHERE 按字典序静默算错。
func openWithSchema(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "t.db") +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, s := range append([]string{observationsDDL(), pendingDDL(), currentViewDDL(testSpec(t))}, runsDDL()...) {
		_, err := db.Exec(s)
		require.NoErrorf(t, err, "DDL 执行失败：%s", s)
	}
	return db
}

// columnInfo 是 PRAGMA table_info 的一行。
type columnInfo struct {
	name    string
	typ     string
	notNull int
	pkOrder int // 0 表示不在主键中；否则是主键内的 1-based 次序
}

func tableInfo(t *testing.T, db *sql.DB, table string) []columnInfo {
	t.Helper()
	rows, err := db.Query(`SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer rows.Close()

	var out []columnInfo
	for rows.Next() {
		var c columnInfo
		require.NoError(t, rows.Scan(&c.name, &c.typ, &c.notNull, &c.pkOrder))
		out = append(out, c)
	}
	require.NoError(t, rows.Err())
	require.NotEmptyf(t, out, "%s 不存在或没有列", table)
	return out
}

// toSnakeCase 把 Go 字段名转成列名。ArticleID → article_id（不是 article_i_d）。
func toSnakeCase(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) {
			// 连续大写内部不断词，只在「小写→大写」与「大写→小写」的交界处断。
			if i > 0 && (unicode.IsLower(rs[i-1]) ||
				(i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestMetaColumnsMatchMetaStructByReflect 补上三处同序契约的第二端。
//
// TASK-002 的 TestMetaFieldOrderIsCrossTaskContract 只钉住 Meta 那一端：谁重排
// Meta 字段它会红，但 metaColumns **单方面**写错序不会让任何测试变红。七列都是
// TEXT，把 extractor 的值塞进 caliber_version 列不触发任何数据库错误，只让下游
// 读到胡话。所以这里用 reflect 从 Meta 取字段名转 snake_case 逐一比对，而不是
// 人眼核对两份清单。
func TestMetaColumnsMatchMetaStructByReflect(t *testing.T) {
	// 先钉住转换函数自身：ArticleID 是唯一有歧义的形态，helper 错了整条断言就
	// 变成两个错误互相印证。
	assert.Equal(t, "article_id", toSnakeCase("ArticleID"))
	assert.Equal(t, "period_type", toSnakeCase("PeriodType"))
	assert.Equal(t, "ingested_at", toSnakeCase("IngestedAt"))
	assert.Equal(t, "period", toSnakeCase("Period"))

	typ := reflect.TypeOf(Meta{})
	want := make([]string, typ.NumField())
	for i := range want {
		want[i] = toSnakeCase(typ.Field(i).Name)
	}

	require.Equal(t, len(want), len(metaColumns),
		"metaColumns 的数量必须等于 Meta 的字段数")
	assert.Equal(t, want, metaColumns,
		"metaColumns 必须与 Meta 字段声明顺序逐一同序——不同序会让写入静默错位")
}

func TestObservationsColumnsDeriveFromFieldOrder(t *testing.T) {
	cols := tableInfo(t, openWithSchema(t), TableObservations)

	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}
	// 83 = 7 元数据 + 76 业务。PRIMARY KEY 是约束不是列，不计入。
	//
	// M1c-4 的 TASK-004 由 61 改成 83：observationsDDL 逐列从 fieldOrder 派生，
	// 加那 22 个 *_mom 列**没有手写一行 DDL**，本条转红正是它在如实报告扩表。
	require.Len(t, names, 83, "7 个 metaColumns + 76 个业务字段")
	assert.Equal(t, metaColumns, names[:len(metaColumns)], "前七列是元数据列，按 metaColumns 顺序")
	assert.Equal(t, fieldOrder, names[len(metaColumns):],
		"业务列必须逐项、按 fieldOrder 的顺序排列——列序与 INSERT 列序同源")
}

// TestObservationsStructureFromLiveDB 从真实库读结构，而不是比对 DDL 字符串。
//
// 两条实测教训：①把 PRIMARY KEY 整行删掉时，按行数列的字符串断言依然全绿（它
// 本来就跳过 PRIMARY KEY 行），而无 PK 时重复写入不再是响亮的 UNIQUE 错误，变成
// QueryRow 静默取第一行、UPDATE 同时改两行；②业务列建成 TEXT 时 Scan 进 float64
// 照样成功，写进去读回来全绿，但 SQLite 按字典序比较，MAX() 与 WHERE m2 > 300
// 全部静默算错。这两种错都只有读结构才能发现。
func TestObservationsStructureFromLiveDB(t *testing.T) {
	cols := tableInfo(t, openWithSchema(t), TableObservations)

	byName := make(map[string]columnInfo, len(cols))
	for _, c := range cols {
		byName[c.name] = c
	}

	// ① 双时态主键：period, period_type, published_at，且次序固定
	for order, name := range []string{"period", "period_type", "published_at"} {
		assert.Equalf(t, order+1, byName[name].pkOrder,
			"%s 必须是主键第 %d 列——无 PK 时重复写入不报 UNIQUE 错，退化成静默取第一行", name, order+1)
	}
	// 主键之外的列不得进主键
	for _, c := range cols {
		if c.name == "period" || c.name == "period_type" || c.name == "published_at" {
			continue
		}
		assert.Equalf(t, 0, c.pkOrder, "%s 不应在主键中", c.name)
	}

	// ② 业务列的类型亲和性必须是 REAL：建成 TEXT 时读写全绿而比较按字典序错
	for _, f := range fieldOrder {
		assert.Equalf(t, "REAL", byName[f].typ, "业务列 %s 必须是 REAL 亲和性", f)
		assert.Equalf(t, 0, byName[f].notNull, "业务列 %s 必须可空——键不存在即字段缺失", f)
	}

	// ③ 元数据列 NOT NULL 且为 TEXT
	for _, c := range metaColumns {
		assert.Equalf(t, 1, byName[c].notNull, "元数据列 %s 必须 NOT NULL", c)
		assert.Equalf(t, "TEXT", byName[c].typ, "元数据列 %s 必须是 TEXT", c)
	}
}

// TestPendingStructureFromLiveDB 断言 pending 表的主键带 ingested_at——
// 同一期反复失败要留下多条记录，那本身是诊断信息，不该被主键冲突吃掉。
func TestPendingStructureFromLiveDB(t *testing.T) {
	cols := tableInfo(t, openWithSchema(t), TablePending)

	byName := make(map[string]columnInfo, len(cols))
	for _, c := range cols {
		byName[c.name] = c
	}
	for order, name := range []string{"period", "period_type", "published_at", "ingested_at"} {
		assert.Equalf(t, order+1, byName[name].pkOrder, "%s 必须是 pending 主键第 %d 列", name, order+1)
	}
	// pending 存 JSON 而不是 54 列：结构上就不该像观测表
	for _, f := range fieldOrder {
		assert.NotContainsf(t, byName, f, "pending 表不应有业务列 %s——它存 JSON", f)
	}
	assert.Contains(t, byName, "values_json")
	assert.Contains(t, byName, "report")
}

// TestCurrentViewStructureFromLiveDB 读 sqlite_master 里**实际部署**的视图定义。
//
// 只断言 currentViewDDL 的返回值等于没验：那是测试自己造的 spec 生成的字符串。
// 这里验的是库里真正存在的那个视图。
func TestCurrentViewStructureFromLiveDB(t *testing.T) {
	db := openWithSchema(t)

	var ddl string
	err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'view' AND name = ?`, viewCurrent).Scan(&ddl)
	require.NoError(t, err, "视图 %s 必须存在于 sqlite_master", viewCurrent)

	// 当前行由 published_at 派生（MAX），不靠 is_current 列
	assert.Contains(t, ddl, "MAX(published_at)", "当前行必须由 published_at 的最大值派生")
	// 且按业务键关联——少一个键会让「当前行」跨期混算
	assert.Contains(t, ddl, "period")
	assert.Contains(t, ddl, "period_type")
	assert.Contains(t, ddl, TableObservations)

	// 视图可查询：定义写得出不等于 SQLite 能执行它
	rows, err := db.Query(`SELECT * FROM ` + viewCurrent + ` LIMIT 0`)
	require.NoError(t, err)
	defer rows.Close()
	names, err := rows.Columns()
	require.NoError(t, err)
	assert.Len(t, names, 83, "视图是 SELECT *，列数应与观测表一致")
}

func TestCurrentViewDDLUsesBitemporal(t *testing.T) {
	spec := testSpec(t)
	ddl := currentViewDDL(spec)
	assert.True(t, strings.HasPrefix(ddl, "CREATE VIEW IF NOT EXISTS "+viewCurrent+" AS "))
	// 视图主体必须原样来自 bitemporal，不能手写——手写的话它与 Lookup 可能用
	// 不同的键，而那种错不报错，只给出错误的数据。
	assert.Contains(t, ddl, bitemporal.CurrentQuery(spec))
}

func TestDDLIsIdempotent(t *testing.T) {
	db := openWithSchema(t) // 第一轮已在 helper 里执行

	before := tableInfo(t, db, TableObservations)
	// runs 表也要前后比对：列表里没有 runsDDL 时这条测试对它恒真（M1.5 的 TASK-001，reviewer B1）
	beforeRuns := tableInfo(t, db, TableRuns)
	for _, s := range append([]string{observationsDDL(), pendingDDL(), currentViewDDL(testSpec(t))}, runsDDL()...) {
		_, err := db.Exec(s)
		require.NoErrorf(t, err, "第二轮执行失败：%s", s)
	}
	assert.Equal(t, before, tableInfo(t, db, TableObservations), "重复执行不得改变结构")
	assert.Equal(t, beforeRuns, tableInfo(t, db, TableRuns), "重复执行不得改变 runs 表结构")
}

// TestDDLHasNoDerivableStateColumns 否定断言：is_current 与 absent_fields 都是
// 可派生状态，存一份只会多一个与事实不符的机会（spec §2 D5）。
func TestDDLHasNoDerivableStateColumns(t *testing.T) {
	all := observationsDDL() + "\n" + pendingDDL() + "\n" + currentViewDDL(testSpec(t))
	for _, banned := range []string{"is_current", "absent_fields"} {
		assert.NotContainsf(t, all, banned, "DDL 不得出现可派生状态列 %s", banned)
	}

	// 字符串里没有，库里也不能有——两者都查，因为 DDL 可能经由别处拼接
	db := openWithSchema(t)
	for _, table := range []string{TableObservations, TablePending} {
		for _, c := range tableInfo(t, db, table) {
			assert.NotEqual(t, "is_current", c.name)
			assert.NotEqual(t, "absent_fields", c.name)
		}
	}
}

// hestia_runs 的列集精确相等；它与 pending 一样不该长得像观测表（M1.5 的 TASK-001）。
func TestRunsStructureFromLiveDB(t *testing.T) {
	cols := tableInfo(t, openWithSchema(t), TableRuns)
	var names []string
	notNull := map[string]int{}
	for _, c := range cols {
		names = append(names, c.name)
		notNull[c.name] = c.notNull
	}
	assert.ElementsMatch(t, []string{
		"run_at", "finished_at", "outcome", "period", "period_type", "article_id", "extractor",
		"blocked_check", "stage", "error", "notified", "notify_error", "duration_ms",
	}, names, "列集必须精确等于 spec §2.1；多一列少一列都是 schema 漂移")
	for _, f := range fieldOrder {
		assert.NotContainsf(t, names, f, "runs 表不应有业务列 %s——它记的是闸门的结论，不是数据", f)
	}
	// 只比列名钉不住 spec §2.1 的约束：NOT NULL 丢了 Scan 照样成功，只在读回时炸
	for _, c := range []string{"run_at", "finished_at", "outcome", "notified", "duration_ms"} {
		assert.Equalf(t, 1, notNull[c], "%s 必须 NOT NULL", c)
	}
	for _, c := range []string{"period", "period_type", "article_id", "extractor", "blocked_check", "stage", "error", "notify_error"} {
		assert.Equalf(t, 0, notNull[c], "%s 是可空列，空串要能存 NULL", c)
	}
}
