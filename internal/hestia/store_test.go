package hestia

// Context Checkpoint: done_criteria → test mapping (store)
// functional[0]      NewStore 在真实 SQLite 建全部表与视图；同一路径重复调用不报错且数据不丢
//                                                        → TestNewStoreCreatesSchemaIdempotently
// functional[1]      Store 含可覆写的 now func() time.Time → TestStoreNowIsInjectable
// functional[2]      DB() 返回可用的 *sql.DB；Close() 正常释放
//                                                        → TestStoreDBIsUsable、TestStoreCloseReleasesDB
// boundary[0]        父目录不存在时自动创建（多级）      → TestNewStoreCreatesParentDir
// boundary[1]        G7 schema 漂移大声失败（老库缺列时 NewStore 报错并给迁移提示）
//                                                        → TestNewStoreRejectsSchemaDriftOnLegacyDB
//                    反向漂移（库里多出本版不认识的列）刻意放行，是登记在案的决定
//                                                        → TestNewStoreToleratesUnknownExtraColumns
// error_handling[0]  开库后 PRAGMA journal_mode 必须返回 wal（验 pragma 生效，不验 DSN 字符串）
//                                                        → TestNewStoreEnablesWAL
// non_functional[0]  本任务不提供任何导出的写方法（Save 在 TASK-005 引入）
//                                                        → TestStoreExposesNoWriteMethods
//
// 上游 TASK-003 明确交接给本任务的一条（见 discoveries/TASK-003.json 的 notes_for_downstream）：
// 「NewStore 实际部署的那个视图从未被验过」——T3 验的是测试自建库（openWithSchema）的结构，
// 用的是测试自己造的 spec。故本文件一律用 NewStore 建库，**不用 openWithSchema / testSpec 代劳**。
//                                                        → TestNewStoreDeploysViewFromItsOwnSpec

import (
	"context"
	"database/sql"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "hestia.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertMetaOnlyRow 只填七个元数据列，业务列留空（它们可空）。
// 用于证明重复 NewStore 之后数据仍在。
func insertMetaOnlyRow(t *testing.T, db *sql.DB, period string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO `+TableObservations+
			` (period, period_type, published_at, article_id, caliber_version, extractor, ingested_at)`+
			` VALUES (?, ?, ?, ?, ?, ?, ?)`,
		period, "h1", "2026-07-15", "2026071512340454869", "2025-01", "rule@v2",
		"2026-07-16T00:00:00Z")
	require.NoError(t, err)
}

func TestNewStoreCreatesSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hestia.db")

	s1, err := NewStore(path)
	require.NoError(t, err)
	insertMetaOnlyRow(t, s1.DB(), "2026-06")
	require.NoError(t, s1.Close())

	// 第二次打开同一个库：CREATE ... IF NOT EXISTS 不应报错
	s2, err := NewStore(path)
	require.NoError(t, err, "对同一路径重复调用 NewStore 必须成功")
	defer s2.Close()

	for _, name := range []string{TableObservations, TablePending, viewCurrent} {
		var got string
		require.NoErrorf(t,
			s2.DB().QueryRow(`SELECT name FROM sqlite_master WHERE name = ?`, name).Scan(&got),
			"%s 不存在", name)
	}

	// 幂等不只是「不报错」，还必须「不丢数据」——建表语句若被写成 DROP+CREATE，
	// 上面那圈存在性断言照样全过，而库已经被清空了。
	var n int
	require.NoError(t, s2.DB().QueryRow(`SELECT COUNT(*) FROM `+TableObservations).Scan(&n))
	assert.Equal(t, 1, n, "第二次 NewStore 之后原有数据必须还在")
}

func TestNewStoreCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "deeper", "hestia.db")
	s, err := NewStore(path)
	require.NoError(t, err, "多级不存在的父目录应被自动创建")
	defer s.Close()
	assert.FileExists(t, path)
}

// TestNewStoreEnablesWAL 直接验 pragma 的**生效结果**，不比对 DSN 字符串。
//
// 字符串对了完全不等于 pragma 生效：`_pragma` 拼错、路径含特殊字符（? & #）
// 都会让参数被当成文件名的一部分而静默失效，而 sql.Open 与建表全都照常成功。
func TestNewStoreEnablesWAL(t *testing.T) {
	s := newTestStore(t)
	var mode string
	require.NoError(t, s.DB().QueryRow(`PRAGMA journal_mode`).Scan(&mode))
	assert.Equal(t, "wal", strings.ToLower(mode))
}

// TestNewStoreDeploysViewFromItsOwnSpec 补上 TASK-003 交接的缺口。
//
// T3 只验到「三段 DDL 产生的结构正确」，建库用的是测试自己的 openWithSchema +
// testSpec。**NewStore 内部自己构造 spec**，它若少一个业务键（例如漏 period_type），
// 部署出来的视图会让「当前行」跨期混算——同一 period 的不同 period_type 会互相
// 覆盖，而这不报错。故这里从 NewStore 建出的库读 sqlite_master，比对实际部署的定义。
//
// 断言用**完整子句**而非 "period" 这种裸词：period 是 period_type 的前缀，
// 子串断言会被另一者蕴含（本包已因此漏过两次缺陷）。
func TestNewStoreDeploysViewFromItsOwnSpec(t *testing.T) {
	s := newTestStore(t)

	var deployed string
	require.NoError(t,
		s.DB().QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'view' AND name = ?`,
			viewCurrent).Scan(&deployed),
		"视图 %s 必须由 NewStore 实际部署到库里", viewCurrent)

	// 独立复述一遍期望：三个参数在这里重新写死，若 NewStore 换了任何一个都会失配。
	// SQLite 存进 sqlite_master 时会去掉 IF NOT EXISTS，其余原样保留。
	want, err := bitemporal.NewSpec(TableObservations,
		[]string{"period", "period_type"}, "published_at")
	require.NoError(t, err)
	assert.Equal(t, "CREATE VIEW "+viewCurrent+" AS "+bitemporal.CurrentQuery(want), deployed,
		"NewStore 部署的视图必须与用正确 spec 生成的一致")

	// 再逐条钉住两个业务键各自的关联子句。上面的全等断言一旦失配，
	// 这两条能直接指出是哪个键出了问题。
	assert.Contains(t, deployed, "period = o.period AND", "缺 period 业务键")
	assert.Contains(t, deployed, "period_type = o.period_type", "缺 period_type 业务键")
	assert.Contains(t, deployed, "MAX(published_at)", "当前行必须由 published_at 的最大值派生")
}

// TestNewStoreRejectsSchemaDriftOnLegacyDB 覆盖 G7。
//
// CREATE TABLE IF NOT EXISTS 对已存在的老库**静默无操作**。字段清单刚从 ~20 涨到 54
// 且还会再涨，不检查的话老库上第一次 Save 才炸，错误是驱动层的 no such column，
// 而且只在恰好含新字段的那一期出现——上线后可能几周才复现一次。
//
// 老库的构造刻意是「**七个元数据列齐全 + 只有前三个业务列**」，而不是「只有主键列」。
// 后者会让一个只检查 metaColumns、根本不看 fieldOrder 的实现照样通过——那正是
// DoD 点名要比对 fieldOrder 的地方。判据必须能区分这两种实现，否则等于没验。
func TestNewStoreRejectsSchemaDriftOnLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	var cols []string
	for _, c := range metaColumns {
		cols = append(cols, c+" TEXT NOT NULL")
	}
	for _, f := range fieldOrder[:3] { // 老版本只落地了很少几个业务字段
		cols = append(cols, f+" REAL")
	}

	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE ` + TableObservations + ` (` + strings.Join(cols, ", ") +
		`, PRIMARY KEY (period, period_type, published_at))`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(path)
	if s != nil {
		defer s.Close()
	}
	require.Error(t, err, "老库缺列时 NewStore 必须失败，而不是等到第一次 Save 才炸")
	assert.Contains(t, err.Error(), TableObservations, "错误应指名是哪张表")
	assert.Contains(t, err.Error(), "migrat", "错误应带迁移提示，让运维知道下一步做什么")
	assert.Nil(t, s, "失败时不应返回一个半可用的 Store")
}

// TestNewStoreToleratesUnknownExtraColumns 钉住一个**登记在案的决定**：
// 反向漂移（库里多出本版代码不认识的列）刻意**不**失败。
//
// 两个方向的危害不对称：缺列会让 Save 在运行时炸且只在特定期次出现，必须拦；
// 多列则无害——INSERT 显式列名，多出来的列取 NULL。若这里也失败，一次回滚
// （用旧版二进制打开新版建的库）就会变成硬故障。
func TestNewStoreToleratesUnknownExtraColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")

	s1, err := NewStore(path)
	require.NoError(t, err)
	_, err = s1.DB().Exec(`ALTER TABLE ` + TableObservations + ` ADD COLUMN from_a_newer_version REAL`)
	require.NoError(t, err)
	require.NoError(t, s1.Close())

	s2, err := NewStore(path)
	require.NoError(t, err, "库中多出本版不认识的列时应放行（支持回滚），不应失败")
	defer s2.Close()
}

func TestStoreNowIsInjectable(t *testing.T) {
	s := newTestStore(t)

	require.NotNil(t, s.now, "now 必须有默认实现，否则调用方拿到的 Store 一用就 panic")
	assert.WithinDuration(t, time.Now(), s.now(), time.Minute,
		"默认应为真实时钟")

	// TASK-005 的 IngestedAt 覆写判据与 TASK-006 的 pending 累积判据都依赖这个注入点
	fixed := time.Date(2026, 7, 16, 8, 30, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	assert.Equal(t, fixed, s.now())
}

func TestStoreDBIsUsable(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()
	require.NotNil(t, db)
	// M1b-4 的 article_id 幂等检查要自己发查询，所以句柄必须是真的可用的
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+TableObservations).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestStoreCloseReleasesDB(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "hestia.db"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// database/sql 在 Close 之后返回的是未导出的 errDBClosed（"sql: database is closed"），
	// 没有可用的 sentinel，所以只能比对错误串。注意它不是 sql.ErrConnDone——
	// 后者是「这条连接已还回池」，与「整个 DB 已关闭」是两回事。
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM ` + TableObservations).Scan(new(int))
	assert.ErrorContains(t, err, "database is closed", "Close 之后句柄必须已释放")
}

// TestStoreExposesNoWriteMethods 钉住单一写入口约束。
//
// Save 在 TASK-005 引入，本条随之更新为 [Close, DB, Save]——**仍是精确集合相等，
// 没有弱化成「包含」**：新增任何导出方法都必须在这里显式登记一次，让「又开了一个
// 写口」成为一个需要动手改测试的决定。ADR-0003 在同机场景下唯一的防线就是 Save 的签名。
func TestStoreExposesNoWriteMethods(t *testing.T) {
	typ := reflect.TypeOf(&Store{})
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	assert.Equal(t, []string{"Close", "DB", "Save"}, got,
		"只应导出 Close、DB 与 Save；出现 Insert/Upsert 等写口即违反单一写入口约束")
}

// TestPackageExposesNoWriteFunctions 把写口守卫从「*Store 的方法集」扩到**包导出面**。
//
// 上面那条用 reflect.TypeOf(&Store{})，只看 *Store 的方法，**包级函数不在它的视野内**。
// 实测（T4 的 M10）：包级新增 func InsertRow(db *sql.DB, period string) error 直接
// INSERT、绕过 ValidationReport，整套测试 43/43 全绿无人拦——而判据说的是「任何
// 导出写口」，守卫窄于判据。
//
// 两条测试互补，不是替代：reflect 那条能看见**嵌入类型提升上来**的方法（AST 里看不到
// 那些，它们没有对应的 FuncDecl）；本条能看见包级函数。删掉任一条都会重新开一个缺口。
func TestPackageExposesNoWriteFunctions(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	var got []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoErrorf(t, err, "解析 %s", name)

		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			if fn.Recv == nil {
				got = append(got, fn.Name.Name)
				continue
			}
			// 方法：接收者类型不导出时，包外根本拿不到它，不构成导出面
			recv := recvTypeName(fn.Recv.List[0].Type)
			if ast.IsExported(recv) {
				got = append(got, recv+"."+fn.Name.Name)
			}
		}
	}
	sort.Strings(got)

	assert.Equal(t, []string{"NewStore", "Store.Close", "Store.DB", "Store.Save"}, got,
		"包的导出函数/方法必须恰好是这四个——任何新增的包级写口（如 InsertRow）都会绕过 Save 的签名防线")
}

// recvTypeName 取接收者的类型名，剥掉指针与泛型实参。
func recvTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(x.X)
	case *ast.IndexExpr: // 泛型接收者 T[P]
		return recvTypeName(x.X)
	case *ast.IndexListExpr: // 泛型接收者 T[P, Q]
		return recvTypeName(x.X)
	case *ast.Ident:
		return x.Name
	}
	return ""
}

// TestNewStoreFailsOnUnopenablePath 证明失败路径不泄漏句柄也不返回半成品。
func TestNewStoreFailsOnUnopenablePath(t *testing.T) {
	dir := t.TempDir()
	// 用一个普通文件占住本该是目录的位置，MkdirAll 必然失败
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	s, err := NewStore(filepath.Join(blocker, "sub", "hestia.db"))
	require.Error(t, err)
	assert.Nil(t, s)
}

// ---------------------------------------------------------------------------
// TASK-005：Save 的输入校验与 INSERT 路径
//
// Context Checkpoint: done_criteria → test mapping (save)
// functional[0]     New/Revision/OutOfOrder 三种 Verdict 各有明确行为，Revision 两行都保留
//                                                       → TestSaveNewObservation、TestSaveRevisionKeepsBothRows、
//                                                         TestSaveOutOfOrder
// functional[1]     insert 取值顺序与 metaColumns、Meta 字段声明顺序三处同序
//                                                       → TestSaveMetaValuesLandInMatchingColumns
// functional[2]     Values 按 fieldOrder 拼列，不按 map 迭代顺序（抽出 insertSQL 以可机械判定）
//                                                       → TestInsertSQLColumnOrderIsDeterministic
// boundary[0]       部分字段 → 其余列为 NULL 而非 0     → TestSavePartialFieldsLeavesNulls
// boundary[1]       空 Values 与全 54 字段两种极端      → TestSaveEmptyValues、TestSaveAllFields
// error_handling[0] 白名单外键 → 报错且两张表零行      → TestSaveRejectsUnknownField
// error_handling[1] Meta 非法 → 报错且零行             → TestSaveRejectsBadMeta
// error_handling[2] IngestedAt 由 s.now() 覆写         → TestSaveOverwritesIngestedAt
// error_handling[3] G10 自相矛盾的 ValidationReport → 报错且零行
//                                                       → TestSaveRejectsSelfContradictoryReport
// error_handling[4] G6 非有限值（NaN/±Inf）→ 报错且零行 → TestSaveRejectsNonFiniteValues
// non_functional[1] 写口守卫扩到包导出面               → TestPackageExposesNoWriteFunctions

func passing() ValidationReport {
	return ValidationReport{Passed: true, Checks: []Check{
		{ID: "monetary_hierarchy", Status: CheckPassed},
	}}
}

func obsWith(values map[string]float64) Observation {
	return Observation{Meta: validMeta(), Values: values}
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

// assertNoRowsAnywhere 是全部错误路径的共同判据：只断言 error 非 nil 排除不掉
// 「已经写了脏数据再报错」——那种实现在只看 error 的测试下完全隐形。
func assertNoRowsAnywhere(t *testing.T, s *Store) {
	t.Helper()
	assert.Equal(t, 0, countRows(t, s, TableObservations), "报错时不得写观测表")
	assert.Equal(t, 0, countRows(t, s, TablePending), "报错时也不得写 pending")
}

func TestSaveNewObservation(t *testing.T) {
	s := newTestStore(t)
	out, err := s.Save(context.Background(),
		obsWith(map[string]float64{FieldM2: 356.71, FieldM2YoY: 8.0}), passing())
	require.NoError(t, err)
	assert.Equal(t, bitemporal.New, out.Verdict)
	assert.Equal(t, TableObservations, out.Table)
	assert.Equal(t, 1, countRows(t, s, TableObservations))
	assert.Equal(t, 0, countRows(t, s, TablePending))

	var m2 float64
	require.NoError(t, s.DB().QueryRow(`SELECT m2 FROM `+TableObservations).Scan(&m2))
	assert.Equal(t, 356.71, m2)
}

func TestSaveRevisionKeepsBothRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 356.71}), passing())
	require.NoError(t, err)

	// 同一期，更晚的发布日 = 央行修订重发
	second := obsWith(map[string]float64{FieldM2: 357.00})
	second.Meta.PublishedAt = "2026-08-20"
	out, err := s.Save(ctx, second, passing())
	require.NoError(t, err)
	assert.Equal(t, bitemporal.Revision, out.Verdict)
	assert.Equal(t, TableObservations, out.Table)

	assert.Equal(t, 2, countRows(t, s, TableObservations), "修订产生新行而非覆盖")

	// 视图只返回最新那行
	var m2 float64
	require.NoError(t, s.DB().QueryRow(`SELECT m2 FROM `+viewCurrent).Scan(&m2))
	assert.Equal(t, 357.00, m2)
}

func TestSaveOutOfOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	late := obsWith(map[string]float64{FieldM2: 357.00})
	late.Meta.PublishedAt = "2026-08-20"
	_, err := s.Save(ctx, late, passing())
	require.NoError(t, err)

	// 回填不保证按时间顺序：后写入一个更早的版本
	early := obsWith(map[string]float64{FieldM2: 356.71})
	early.Meta.PublishedAt = "2026-07-15"
	out, err := s.Save(ctx, early, passing())
	require.NoError(t, err)
	assert.Equal(t, bitemporal.OutOfOrder, out.Verdict)
	assert.Equal(t, TableObservations, out.Table, "乱序仍是权威数据，只是不是当前行")
	assert.Equal(t, 2, countRows(t, s, TableObservations))

	var m2 float64
	require.NoError(t, s.DB().QueryRow(`SELECT m2 FROM `+viewCurrent).Scan(&m2))
	assert.Equal(t, 357.00, m2, "当前行由 MAX(published_at) 决定，与写入顺序无关")
}

// TestSaveMetaValuesLandInMatchingColumns 是三处同序契约的**第三端**。
//
// Meta 字段声明顺序（types.go）→ metaColumns（schema.go）→ insert 取值顺序（本文件），
// 三处必须同序。前两端已分别被 TestMetaFieldOrderIsCrossTaskContract 与
// TestMetaColumnsMatchMetaStructByReflect 钉住，但那两条在 Meta 与 metaColumns 都
// 没变时**都不会红**——insert 单方面把 args 写错序只有这里能发现。
//
// 判据不是「能写进去」：七列都是 TEXT，错位写入不触发任何数据库错误。所以让七个字段
// 取**互不相同且可辨识**的值，再逐列读回比对；期望值用 reflect 按 Meta 的声明顺序取，
// 读取列用 metaColumns 的顺序——任意两列互换，两边就对不上。
func TestSaveMetaValuesLandInMatchingColumns(t *testing.T) {
	s := newTestStore(t)
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }

	obs := obsWith(map[string]float64{FieldM2: 1})
	// 七个值互不相同：任意两列互换都会让下面的逐列比对红。
	// 前三个受 Meta.validate 的形态约束，取合法但彼此不同的值。
	obs.Meta.Period = "2026-06"
	obs.Meta.PeriodType = "h1"
	obs.Meta.PublishedAt = "2026-07-15"
	obs.Meta.ArticleID = "article-id-sentinel"
	obs.Meta.CaliberVersion = "2025-01"
	obs.Meta.Extractor = "extractor-sentinel"
	obs.Meta.IngestedAt = "1999-01-01T00:00:00Z" // 会被 s.now() 覆写

	_, err := s.Save(context.Background(), obs, passing())
	require.NoError(t, err)

	// 期望值 = Meta 的声明顺序（reflect），且 IngestedAt 换成 Store 实际写的值
	want := obs.Meta
	want.IngestedAt = fixed.Format(time.RFC3339)
	wantByDeclOrder := reflect.ValueOf(want)
	require.Equal(t, wantByDeclOrder.NumField(), len(metaColumns))

	// 先自证 fixture 够强：七个值必须互不相同，否则「互换必红」不成立
	seen := map[string]bool{}
	for i := 0; i < wantByDeclOrder.NumField(); i++ {
		v := wantByDeclOrder.Field(i).String()
		require.Falsef(t, seen[v], "fixture 太弱：值 %q 重复出现，列互换将无法被发现", v)
		seen[v] = true
	}

	for i, col := range metaColumns {
		var got string
		require.NoError(t, s.DB().QueryRow(
			`SELECT `+col+` FROM `+TableObservations).Scan(&got))
		assert.Equalf(t, wantByDeclOrder.Field(i).String(), got,
			"列 %s 里应是 Meta 第 %d 个字段的值——不同序会让写入静默错位", col, i)
	}
}

// TestInsertSQLColumnOrderIsDeterministic 让「按 fieldOrder 遍历而非按 map」成为
// 可机械判定的断言。
//
// SQL 构造抽在 insertSQL 里正是为此：若只有 insert 一个方法、它不返回生成的 SQL，
// 包内就没有观测点，验证者只能读代码确认 range 的是 fieldOrder 不是 map——那是
// review 不是 test。
func TestInsertSQLColumnOrderIsDeterministic(t *testing.T) {
	values := make(map[string]float64, len(fieldOrder))
	for i, f := range fieldOrder {
		values[f] = float64(i) + 0.5
	}
	obs := obsWith(values)
	obs.Meta.IngestedAt = "2026-08-08T10:00:00Z"

	wantCols := append(append([]string{}, metaColumns...), fieldOrder...)
	q, args := insertSQL(obs)

	assert.Equal(t, strings.Join(wantCols, ", "), sqlColumnList(t, q),
		"列序必须是 metaColumns 后接 fieldOrder，逐项一致")
	require.Len(t, args, len(wantCols))
	for i, f := range fieldOrder {
		assert.Equalf(t, values[f], args[len(metaColumns)+i], "第 %d 个业务参数应对应 %s", i, f)
	}

	// map 迭代顺序随机：同一 Observation 反复构造必须得到逐字相同的 SQL。
	// 单次比对可能碰巧相同，所以多跑几轮。
	for i := 0; i < 20; i++ {
		got, _ := insertSQL(obs)
		require.Equalf(t, q, got, "第 %d 轮生成的 SQL 与首轮不同——说明遍历的是 map", i)
	}
}

// sqlColumnList 从 INSERT 语句里取出括号内的列清单。
func sqlColumnList(t *testing.T, q string) string {
	t.Helper()
	open := strings.Index(q, "(")
	closing := strings.Index(q, ")")
	require.Greaterf(t, closing, open, "不是可解析的 INSERT 语句：%s", q)
	return q[open+1 : closing]
}

// TestInsertSQLOmitsAbsentFields 钉住「键不存在即字段缺失」在 SQL 层的表示：
// 缺失的字段根本不出现在列清单里，而不是补一个 0 或 NULL 参数。
func TestInsertSQLOmitsAbsentFields(t *testing.T) {
	obs := obsWith(map[string]float64{FieldM2: 1, FieldLoanBillYTD: 2})
	q, args := insertSQL(obs)

	cols := strings.Split(sqlColumnList(t, q), ", ")
	assert.Equal(t, append(append([]string{}, metaColumns...), FieldM2, FieldLoanBillYTD), cols,
		"只写实际存在的键，且仍按 fieldOrder 的相对顺序")
	assert.Len(t, args, len(metaColumns)+2)
}

func TestSavePartialFieldsLeavesNulls(t *testing.T) {
	s := newTestStore(t)
	// 模拟 rule@v1：六板块报告没有任何社融字段
	obs := obsWith(map[string]float64{FieldM2: 213.49, FieldM1: 60.43, FieldM0: 7.95})
	obs.Meta.Extractor = "rule@v1"
	obs.Meta.CaliberVersion = "2015-01"
	_, err := s.Save(context.Background(), obs, passing())
	require.NoError(t, err)

	var tsf sql.NullFloat64
	require.NoError(t, s.DB().QueryRow(
		`SELECT `+FieldTSFStock+` FROM `+TableObservations).Scan(&tsf))
	assert.False(t, tsf.Valid, "未提供的字段应为 NULL，而不是 0")

	// 0 与 NULL 必须可区分：显式写入的 0 要留下一个非 NULL 的 0
	obs2 := obsWith(map[string]float64{FieldTSFFlowYTD: 0})
	obs2.Meta.PublishedAt = "2026-08-20"
	_, err = s.Save(context.Background(), obs2, passing())
	require.NoError(t, err)

	var flow sql.NullFloat64
	require.NoError(t, s.DB().QueryRow(
		`SELECT `+FieldTSFFlowYTD+` FROM `+TableObservations+` WHERE published_at = '2026-08-20'`).
		Scan(&flow))
	assert.True(t, flow.Valid, "显式写入的 0 是「增量为零」，不是「未披露」")
	assert.Equal(t, 0.0, flow.Float64)
}

func TestSaveEmptyValues(t *testing.T) {
	s := newTestStore(t)
	out, err := s.Save(context.Background(), obsWith(map[string]float64{}), passing())
	require.NoError(t, err)
	assert.Equal(t, TableObservations, out.Table)
	assert.Equal(t, 1, countRows(t, s, TableObservations))

	// nil map 与空 map 走同一条路
	obs := obsWith(nil)
	obs.Meta.PublishedAt = "2026-08-20"
	_, err = s.Save(context.Background(), obs, passing())
	require.NoError(t, err)
	assert.Equal(t, 2, countRows(t, s, TableObservations))
}

func TestSaveAllFields(t *testing.T) {
	s := newTestStore(t)
	values := make(map[string]float64, len(fieldOrder))
	for i, f := range fieldOrder {
		values[f] = float64(i) + 0.5
	}
	_, err := s.Save(context.Background(), obsWith(values), passing())
	require.NoError(t, err)

	// 逐列读回：只查最后一列会让「中间某列错位」完全隐形
	for i, f := range fieldOrder {
		var got float64
		require.NoErrorf(t, s.DB().QueryRow(
			`SELECT `+f+` FROM `+TableObservations).Scan(&got), "读 %s", f)
		assert.Equalf(t, float64(i)+0.5, got, "列 %s 的值错位", f)
	}
}

func TestSaveRejectsUnknownField(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Save(context.Background(),
		obsWith(map[string]float64{FieldM2: 1, "m2_typo": 2}), passing())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "m2_typo")
	assertNoRowsAnywhere(t, s)
}

func TestSaveRejectsBadMeta(t *testing.T) {
	s := newTestStore(t)
	obs := obsWith(map[string]float64{FieldM2: 1})
	obs.Meta.Extractor = "" // 强制点 2
	_, err := s.Save(context.Background(), obs, passing())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor")
	assertNoRowsAnywhere(t, s)
}

func TestSaveOverwritesIngestedAt(t *testing.T) {
	s := newTestStore(t)
	s.now = func() time.Time { return time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC) }

	obs := obsWith(map[string]float64{FieldM2: 1})
	obs.Meta.IngestedAt = "1999-01-01T00:00:00Z" // 调用方撒谎
	_, err := s.Save(context.Background(), obs, passing())
	require.NoError(t, err)

	var got string
	require.NoError(t, s.DB().QueryRow(
		`SELECT ingested_at FROM `+TableObservations).Scan(&got))
	assert.Equal(t, "2026-08-08T10:00:00Z", got, "入库时刻只能由 Store 决定")

	// 调用方的 Observation 不应被就地改写——它是值传递的，但字段赋值容易写成指针语义
	assert.Equal(t, "1999-01-01T00:00:00Z", obs.Meta.IngestedAt)
}

// TestSaveRejectsSelfContradictoryReport 落实 G10。
//
// 签名要求 ValidationReport 是 ADR-0003 在同机场景下的唯一防线，但类型只要求报告
// **存在**。Passed=true 却带着 CheckFailed，正是「加了失败检查却忘了把 Passed 置
// false」这一类 bug 的形状；而观测表不存任何校验痕迹（pending 才存 report），
// 事后无从审计某行是否真跑过闸门——所以必须当场拒绝。
func TestSaveRejectsSelfContradictoryReport(t *testing.T) {
	cases := []struct {
		name string
		rep  ValidationReport
	}{
		{"failed check among passed", ValidationReport{Passed: true, Checks: []Check{
			{ID: "monetary_hierarchy", Status: CheckPassed},
			{ID: "deposit_sum", Status: CheckFailed, Reason: "residual 14%"},
		}}},
		{"single failed check", ValidationReport{Passed: true, Checks: []Check{
			{ID: "deposit_sum", Status: CheckFailed},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}), tc.rep)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "deposit_sum", "错误须指明是哪个检查自相矛盾")
			assertNoRowsAnywhere(t, s)
		})
	}

	// 对照组：Passed=false 且有 CheckFailed 并不矛盾，不应被本条拦下
	// （它该走 pending 路径——TASK-006 的桩会让它以另一个错误失败，
	//   所以这里只断言错误不是「自相矛盾」那一个）。
	s := newTestStore(t)
	_, err := s.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}),
		ValidationReport{Passed: false, Checks: []Check{{ID: "deposit_sum", Status: CheckFailed}}})
	if err != nil {
		assert.NotContains(t, err.Error(), "contradict",
			"Passed=false 带失败检查是正常的未过闸，不是自相矛盾")
	}

	// 跳过的检查不构成矛盾：CheckSkipped 在 Passed=true 下合法
	s2 := newTestStore(t)
	_, err = s2.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}),
		ValidationReport{Passed: true, Checks: []Check{{ID: "tsf_sum", Status: CheckSkipped}}})
	require.NoError(t, err, "CheckSkipped 不是失败")
	assert.Equal(t, 1, countRows(t, s2, TableObservations))
}

// TestSaveRejectsNonFiniteValues 落实 G6。
//
// NaN 写进 SQLite 后 typeof 是 null、isNull=1——**与「字段缺失」完全不可区分**，
// 而「用 map 的键是否存在表示缺失」正是本设计区分「本就没有」与「解析漏了」的核心。
// 比率型字段的 0/0 是最常见来源。另：json.Marshal(NaN) 直接报错，NaN 若流到 pending
// 路径会让 savePending 失败 → Save 返回 error → 两张表都没有这条数据，而 pending
// 存在的理由正是「不让那期数据彻底消失」。
func TestSaveRejectsNonFiniteValues(t *testing.T) {
	for name, v := range map[string]float64{
		"NaN":       math.NaN(),
		"+Inf":      math.Inf(1),
		"-Inf":      math.Inf(-1),
		"0/0 → NaN": 0.0 / func() float64 { return 0 }(),
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Save(context.Background(),
				obsWith(map[string]float64{FieldM2: 1, FieldM2YoY: v}), passing())
			require.Error(t, err)
			assert.Contains(t, err.Error(), FieldM2YoY, "错误须指明是哪个字段")
			assertNoRowsAnywhere(t, s)
		})
	}

	// 先证明 NaN 一旦写进去确实与缺失不可区分——这是上面为什么必须拦的依据，
	// 而不是凭空的谨慎。
	s := newTestStore(t)
	_, err := s.DB().Exec(
		`INSERT INTO `+TableObservations+
			` (period, period_type, published_at, article_id, caliber_version, extractor, ingested_at, `+
			FieldM2+`) VALUES (?,?,?,?,?,?,?,?)`,
		"2026-06", "h1", "2026-07-15", "a", "2025-01", "rule@v2", "2026-07-16T00:00:00Z",
		math.NaN())
	require.NoError(t, err, "SQLite 本身并不拒绝 NaN——所以拦截必须发生在 Save 里")

	var typ string
	var isNull int
	require.NoError(t, s.DB().QueryRow(
		`SELECT typeof(`+FieldM2+`), `+FieldM2+` IS NULL FROM `+TableObservations).Scan(&typ, &isNull))
	assert.Equal(t, "null", typ, "NaN 存进去读出来是 null")
	assert.Equal(t, 1, isNull, "与「字段缺失」完全不可区分")
}

// TestSaveDuplicateIsLoudNotSilent（TASK-005）已被 TASK-006 的
// TestSaveDuplicateIsDefinedNotSilent 取代——它断言「Duplicate 必须报错」，而本任务
// 按 DoD functional[0] 给 Duplicate 加了专门处置（refreshArticleID），那个前提被
// 有意推翻。它真正要防的「静默丢弃 / 静默多一行」换了形式继续被防住，
// 说明见取代它的那条测试的注释。

// ---------------------------------------------------------------------------
// TASK-006: Duplicate 的 UPDATE 与 pending 分流
//
// functional[0] Duplicate 只刷 article_id、不新增行 → TestSaveDuplicateRefreshesArticleID
// functional[1] Passed=false 走 savePending，Outcome.Table 如实反映去向
//                                                    → TestSaveFailedValidationGoesToPending
// functional[2] savePending 桩确已被替换（无 not implemented，且 pending 有行）
//                                                    → TestSaveFailedValidationGoesToPending
// functional[3] G2/M7' pending 路径的 Verdict 必须有意义（与零值可区分）
//                                                    → TestSavePendingVerdictSurvivesClassify
// functional[4] G3 Duplicate 携带更丰富 Values 时的行为已钉死（保持丢弃）
//                                                    → TestSaveDuplicateDiscardsRicherValues
// boundary[0]   同一期反复失败在 pending 逐次累积    → TestPendingAccumulatesPerAttempt
// boundary[1]   G8 同秒两次失败必须可区分（RFC3339Nano）
//                                                    → TestPendingDistinguishesSameSecondAttempts
//                RFC3339Nano 去尾零导致字典序≠时间序，登记在案
//                                                    → TestIngestedAtLexicalOrderIsNotTimeOrder
// boundary[2]   pending 表漂移时必须明确失败且错误可定位（选②的直接证据）
//                                                    → TestSavePendingFailsLoudlyOnDriftedPendingTable
// ---------------------------------------------------------------------------

// failing 是 passing 的镜像：一份最小的未过闸报告，用于只关心「走 pending 分流」
// 而不关心报告内容的用例。需要断言报告字段如实落盘的用例（如
// TestSaveFailedValidationGoesToPending）自己构造更丰富的报告。
func failing() ValidationReport {
	return ValidationReport{Passed: false, Checks: []Check{
		{ID: "completeness", Status: CheckFailed},
	}}
}

// pendingRow 是 pending 表里一行的读回形态。
type pendingRow struct {
	articleID  string
	extractor  string
	ingestedAt string
	report     string
	valuesJSON string
}

func pendingRows(t *testing.T, s *Store) []pendingRow {
	t.Helper()
	rows, err := s.DB().Query(
		`SELECT article_id, extractor, ingested_at, report, values_json FROM ` +
			TablePending + ` ORDER BY ingested_at`)
	require.NoError(t, err)
	defer rows.Close()

	var out []pendingRow
	for rows.Next() {
		var p pendingRow
		require.NoError(t, rows.Scan(&p.articleID, &p.extractor, &p.ingestedAt,
			&p.report, &p.valuesJSON))
		out = append(out, p)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestSaveDuplicateRefreshesArticleID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first := obsWith(map[string]float64{FieldM2: 356.71})
	_, err := s.Save(ctx, first, passing())
	require.NoError(t, err)

	// 站点迁移：同一篇报告换了新 URL，发布日不变
	migrated := obsWith(map[string]float64{FieldM2: 356.71})
	migrated.Meta.ArticleID = "2026999999999999999"
	out, err := s.Save(ctx, migrated, passing())
	require.NoError(t, err)

	assert.Equal(t, bitemporal.Duplicate, out.Verdict)
	assert.Equal(t, TableObservations, out.Table)
	assert.Equal(t, 1, countRows(t, s, TableObservations), "不得写新行——那会造出一个假修订")

	var id string
	require.NoError(t, s.DB().QueryRow(
		`SELECT article_id FROM `+TableObservations).Scan(&id))
	assert.Equal(t, "2026999999999999999", id,
		"必须刷新 article_id，否则一级幂等检查永远 miss，每月重抓一次")
}

// TestSaveDuplicateIsDefinedNotSilent **取代** TASK-005 的 TestSaveDuplicateIsLoudNotSilent。
//
// 那条钉住的是「Duplicate 撞主键约束而响亮失败」，理由是「此前重复写入报错好过静默
// 多一行」，并防止有人「顺手加个 INSERT OR IGNORE 把它变成静默丢弃」。
//
// 本任务按 DoD functional[0] 给 Duplicate 加了专门处置（refreshArticleID），
// 「响亮失败」这个前提**被有意推翻**：它当时是「尚无处置」的副产品，不是目标行为。
// 但那条钉子真正要防的东西必须继续被防住，所以它换了一种形式活下来——
//
//	原来的防线：Duplicate 必须报错（任何静默成功都是退化）
//	现在的防线：Duplicate 必须产生**可观测的确定动作**——行数恰好不变 **且** article_id
//	           已被刷新。INSERT OR IGNORE 会让行数不变但 article_id 不变，
//	           因此仍然被 TestSaveDuplicateRefreshesArticleID 杀死。
//
// 本条只补一件那条测试原本覆盖、而上面那条没覆盖的事：Duplicate 不得**多出一行**，
// 无论 Values 是否相同。
func TestSaveDuplicateIsDefinedNotSilent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	obs := obsWith(map[string]float64{FieldM2: 356.71})

	_, err := s.Save(ctx, obs, passing())
	require.NoError(t, err)

	out, err := s.Save(ctx, obs, passing())
	require.NoError(t, err, "Duplicate 现在有专门处置，不再是错误路径")
	assert.Equal(t, bitemporal.Duplicate, out.Verdict)
	assert.Equal(t, 1, countRows(t, s, TableObservations),
		"同键同 published_at 不得多出一行——那是假修订")
	assert.Equal(t, 0, countRows(t, s, TablePending),
		"过闸的 Duplicate 不该落 pending")
}

// TestSaveDuplicateDiscardsRicherValues 落实 G3：把 Duplicate 携带不同 Values 时的
// 行为**从意外变成登记在案的决定**。
//
// 选择的是「保持丢弃」而非「覆盖式更新」，因为 DoD functional[0] 明文要求
// refreshArticleID **只更新 article_id、不新增行**——覆盖式更新会直接违反它。
//
// 于是这里钉住丢弃的**全部可观测后果**，让它可被审计：
// 上线 rule@v2 后回填重跑历史时，每期 published_at 都没变 → 全判 Duplicate →
// 只刷 article_id → v2 新抽的字段一个都没写进去，extractor 列还写着旧值，
// 而 Save 返回 nil，运维看到「N 期处理完毕、零错误」。
//
// **这是本任务已知且未修复的数据静默丢失面**，不是本条测试的疏漏——它正是用来
// 让这件事在下次有人动这段代码时立刻可见。若将来决定改成覆盖式更新，本条会转红，
// 迫使他显式推翻这个决定而不是悄悄改掉。
func TestSaveDuplicateDiscardsRicherValues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first := obsWith(map[string]float64{FieldM2: 356.71})
	first.Meta.Extractor = "rule@v1"
	_, err := s.Save(ctx, first, passing())
	require.NoError(t, err)

	// 同一篇报告用更强的抽取器重跑：值更全、extractor 也变了，但发布日没变
	richer := obsWith(map[string]float64{FieldM2: 999.99, FieldM1: 123.45})
	richer.Meta.Extractor = "rule@v2"
	richer.Meta.ArticleID = "2026999999999999999"
	out, err := s.Save(ctx, richer, passing())
	require.NoError(t, err)
	require.Equal(t, bitemporal.Duplicate, out.Verdict)
	require.Equal(t, 1, countRows(t, s, TableObservations))

	var m2 float64
	var m1 sql.NullFloat64
	var extractor, articleID string
	require.NoError(t, s.DB().QueryRow(
		`SELECT `+FieldM2+`, `+FieldM1+`, extractor, article_id FROM `+TableObservations).
		Scan(&m2, &m1, &extractor, &articleID))

	assert.Equal(t, 356.71, m2, "既有列不被覆盖——重跑的新值被丢弃")
	assert.False(t, m1.Valid, "v2 新抽出来的字段一个都没写进去")
	assert.Equal(t, "rule@v1", extractor, "extractor 列仍写着旧抽取器，与实际不符")
	assert.Equal(t, "2026999999999999999", articleID, "唯一被更新的就是 article_id")
}

func TestSaveFailedValidationGoesToPending(t *testing.T) {
	s := newTestStore(t)
	val := 0.0857
	rep := ValidationReport{Passed: false, Checks: []Check{
		{ID: "deposit_sum", Status: CheckFailed, Value: &val},
		{ID: "stock_continuity", Status: CheckSkipped, Reason: "absent_field:tsf_stock"},
	}}

	out, err := s.Save(context.Background(),
		obsWith(map[string]float64{FieldM2: 356.71}), rep)
	require.NoError(t, err, "未过闸不是错误——它是一条正常的分流路径")
	// 桩确已被替换：桩会让上一行的 require.NoError 直接失败，这里再钉一次措辞，
	// 让「桩还在」与「真实现有 bug」在失败信息上可区分。
	assert.Equal(t, TablePending, out.Table)

	assert.Equal(t, 0, countRows(t, s, TableObservations))
	assert.Equal(t, 1, countRows(t, s, TablePending))

	rows := pendingRows(t, s)
	require.Len(t, rows, 1)

	var gotRep ValidationReport
	require.NoError(t, json.Unmarshal([]byte(rows[0].report), &gotRep))
	assert.False(t, gotRep.Passed)
	require.Len(t, gotRep.Checks, 2)
	assert.Equal(t, "deposit_sum", gotRep.Checks[0].ID)
	assert.Equal(t, CheckFailed, gotRep.Checks[0].Status)
	require.NotNil(t, gotRep.Checks[0].Value)
	assert.Equal(t, 0.0857, *gotRep.Checks[0].Value)
	assert.Equal(t, CheckSkipped, gotRep.Checks[1].Status)
	assert.Equal(t, "absent_field:tsf_stock", gotRep.Checks[1].Reason)

	var gotValues map[string]float64
	require.NoError(t, json.Unmarshal([]byte(rows[0].valuesJSON), &gotValues))
	assert.Equal(t, 356.71, gotValues[FieldM2])
}

// TestSavePendingVerdictSurvivesClassify 闭合 TASK-005 交接的存活变异 M7'（G2）。
//
// M7' 是「把 `if !rep.Passed` 整块移到 Classify 之前」——少一次查询，很有诱惑力。
// 在 T5 范围内它**完全存活**（53/53 全绿、vet exit 0、三条自证齐全），根因是结构性
// 不可测：T5 的 savePending 是永远返回 error 的桩 ⇒ Passed=false 必然走
// `return Outcome{}, err` ⇒ 调用方拿到的永远是零值 Outcome ⇒ 判据没有观测点。
// 本任务实现 savePending 之后它才第一次可测。
//
// **只测 New 场景等于没测，因为 bitemporal.New == 0**：早早 return `Outcome{}` 的
// 实现同样能通过 `assert.Equal(bitemporal.New, out.Verdict)`。所以这里一律构造
// Verdict != New 的场景。
func TestSavePendingVerdictSurvivesClassify(t *testing.T) {
	// 先证明用来区分的两个取值确实非零——否则整条判据退化成上面批评的那种
	require.NotEqual(t, bitemporal.Verdict(0), bitemporal.Revision)
	require.NotEqual(t, bitemporal.Verdict(0), bitemporal.OutOfOrder)

	for _, tc := range []struct {
		name        string
		publishedAt string
		want        bitemporal.Verdict
	}{
		{"Revision", "2026-08-20", bitemporal.Revision},     // 比库中更晚
		{"OutOfOrder", "2026-06-01", bitemporal.OutOfOrder}, // 比库中更早
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()

			// 库里先有一行过闸的数据（published_at = validMeta 的 2026-07-15）
			_, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 1}), passing())
			require.NoError(t, err)

			later := obsWith(map[string]float64{FieldM2: 2})
			later.Meta.PublishedAt = tc.publishedAt
			out, err := s.Save(ctx, later, failing())
			require.NoError(t, err)

			assert.Equal(t, TablePending, out.Table)
			assert.Equal(t, tc.want, out.Verdict,
				"未过闸的数据也必须带真实 Verdict——告警要能说清是新一期被拦还是一次修订被拦")
			assert.Equal(t, 1, countRows(t, s, TableObservations), "未过闸不得进观测表")
			assert.Equal(t, 1, countRows(t, s, TablePending))
		})
	}
}

func TestPendingAccumulatesPerAttempt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stamps := []time.Time{
		time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 14, 0, 0, 0, time.UTC),
	}
	for _, ts := range stamps {
		s.now = func() time.Time { return ts }
		_, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 1}), failing())
		require.NoError(t, err)
	}
	assert.Equal(t, 2, countRows(t, s, TablePending),
		"主键含 ingested_at：同一期反复失败留下多条，那本身是诊断信息")
}

// TestPendingDistinguishesSameSecondAttempts 落实 G8。
//
// pending 的主键含 ingested_at。TASK-005 用 RFC3339 格式化它，而 RFC3339 只有秒精度
// ——即时重试循环里同一期两次过闸失败会撞 UNIQUE constraint，第二次 Save 直接报错，
// **那次尝试的诊断信息全部丢失**，而 pending 表存在的唯一理由就是保留诊断信息。
//
// 上面的 TestPendingAccumulatesPerAttempt 用相隔四小时的两个时刻，**恰好绕开了这个
// 边界**——它在秒精度下也会通过。本条专门盯住同一秒内的两次尝试：两个时刻只在纳秒
// 位不同，并先断言它们在 RFC3339 下渲染成**完全相同的串**，以此证明本条确实踩在
// 边界上，而不是又一次绕开它。
func TestPendingDistinguishesSameSecondAttempts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	first, second := base.Add(1), base.Add(2) // 相差 1 纳秒，同一秒内

	require.Equal(t, first.Format(time.RFC3339), second.Format(time.RFC3339),
		"前提自证：这两个时刻在 RFC3339 下不可区分，否则本条又绕开了边界")

	for _, ts := range []time.Time{first, second} {
		s.now = func() time.Time { return ts }
		_, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 1}), failing())
		require.NoError(t, err,
			"同一秒内的两次过闸失败都必须留下记录，第二次不得撞 UNIQUE constraint")
	}

	rows := pendingRows(t, s)
	require.Len(t, rows, 2, "同秒两次尝试必须是两行")
	assert.NotEqual(t, rows[0].ingestedAt, rows[1].ingestedAt,
		"两行的 ingested_at 必须真的不同，否则主键根本没区分开")
}

// TestIngestedAtLexicalOrderIsNotTimeOrder 钉住一个**已知且被接受**的限制。
//
// RFC3339Nano 会去掉小数部分的尾零，于是整秒时刻渲染成 "…:00Z"、半秒时刻渲染成
// "…:00.5Z"，而 '.'(0x2E) < 'Z'(0x5A) —— **字典序与时间序相反**。
//
// 当前无害：ingested_at 不是 revision 列（published_at 才是），bitemporal 的 MAX()
// 与 Classify 都不碰它；pending 主键只要求唯一，不要求有序。
// 但凡有人将来对 ingested_at 做 ORDER BY 或 MAX()（例如「取最近一次失败尝试」），
// 结果会静默出错。本条把这个限制钉成可执行契约：若哪天改成定宽补零格式，本条会
// 转红，迫使他显式推翻它——顺带提醒那次改动需要迁移既有数据。
func TestIngestedAtLexicalOrderIsNotTimeOrder(t *testing.T) {
	whole := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	frac := time.Date(2026, 8, 8, 10, 0, 0, 500000000, time.UTC)

	require.True(t, frac.After(whole), "前提：frac 在时间上更晚")
	assert.False(t, frac.Format(time.RFC3339Nano) > whole.Format(time.RFC3339Nano),
		"RFC3339Nano 去尾零导致字典序与时间序相反——不要对 ingested_at 做 ORDER BY/MAX()")
}

// TestSavePendingFailsLoudlyOnDriftedPendingTable 是 boundary[2] 选②的**直接证据**。
//
// TASK-004 只把观测表纳入了 NewStore 的漂移检查，pending 表没有。本任务复核后仍选择
// 不加列存在性校验，但那必须是有论证的决定而不是声明，所以这里把论证的关键一环变成
// 可执行的：**pending 表结构与代码预期不符时，失败是确定的、当场的、且错误可定位。**
//
// 与观测表的危害对比（这正是两者处置不同的理由）：
//
//	观测表：INSERT 的列由 Values 里实际存在的字段决定 ⇒ 列清单**随数据变化** ⇒
//	        缺某一列只在恰好含该字段的那一期才炸，可能上线数周后才复现。
//	pending：INSERT 的八列**固定不变**，每次写 pending 都用同一份列清单 ⇒ 任何不符
//	        在**第一次**写 pending 时就炸，不存在「特定期次才暴露」这个失效模式。
//
// 所以 G7 那条「静默到某一期才炸」的危害在 pending 上结构性地不成立，剩下的要求
// 只是「失败要可定位」——由 savePending 的错误包装满足，本条断言它。
func TestSavePendingFailsLoudlyOnDriftedPendingTable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 造一个「结构与代码预期不符」的 pending 表
	_, err := s.DB().Exec(
		`ALTER TABLE ` + TablePending + ` RENAME COLUMN values_json TO values_json_legacy`)
	require.NoError(t, err)

	out, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 1}), failing())

	require.Error(t, err, "pending 表结构不符时必须当场失败")
	assert.Contains(t, err.Error(), "pending", "错误必须指明是写 pending 这一步")
	assert.Contains(t, err.Error(), "2026-06", "错误必须带上是哪一期，否则无法定位")
	assert.Equal(t, Outcome{}, out, "失败时不得返回一个看起来成功的 Outcome")
	assert.Equal(t, 0, countRows(t, s, TableObservations), "未过闸的数据不得漏进观测表")
}
