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
	"errors"
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
	s, _ := newTestStoreAt(t)
	return s
}

// newTestStoreAt 与 newTestStore 相同，另外返回库文件路径，供需要裸连接的测试。
func newTestStoreAt(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hestia.db")
	s, err := NewStore(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

// rawDB 另开一个连接到同一个库文件，供测试制造「Store 管不到」的库状态——
// 加列、直接写 NaN、把表改成结构不符的样子。
//
// 生产代码拿不到这样的句柄：DB() 收窄成只读接口后，测试要造脏数据得自己开连接，
// 而不是从 Store 借一个写口。这正是收窄的目的——「唯一写入通道」不再靠自觉。
//
// DSN 走 sqliteDSN 而不是另抄一份：裸连接与 Store 的连接会同时开着（例如
// TestSaveRejectsNonFiniteValues），busy_timeout 不一致只会表现为偶发 flake。
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(path))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// queryRow / queryRows 是 s.DB().Query…Context(context.Background(), …) 的简写。
//
// DB() 收窄成只读接口后只剩这两个带 ctx 的方法，而下面的断言没有一条关心 ctx——
// 每处重复一遍 context.Background() 会把真正要看的 SQL 挤到行尾甚至折行。
// 需要断言 DB() 本身的用例（TestStoreDBIsUsable、TestStoreCloseReleasesDB）
// 仍然直接调，因为那两条的主语就是句柄。
func queryRow(s *Store, query string, args ...any) *sql.Row {
	return s.DB().QueryRowContext(context.Background(), query, args...)
}

func queryRows(s *Store, query string, args ...any) (*sql.Rows, error) {
	return s.DB().QueryContext(context.Background(), query, args...)
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
	// insertMetaOnlyRow 是写操作，DB() 收窄后拿不到写句柄——测试自己开一个
	insertMetaOnlyRow(t, rawDB(t, path), "2026-06")
	require.NoError(t, s1.Close())

	// 第二次打开同一个库：CREATE ... IF NOT EXISTS 不应报错
	s2, err := NewStore(path)
	require.NoError(t, err, "对同一路径重复调用 NewStore 必须成功")
	defer s2.Close()

	for _, name := range []string{TableObservations, TablePending, viewCurrent} {
		var got string
		require.NoErrorf(t,
			queryRow(s2, `SELECT name FROM sqlite_master WHERE name = ?`, name).Scan(&got),
			"%s 不存在", name)
	}

	// 幂等不只是「不报错」，还必须「不丢数据」——建表语句若被写成 DROP+CREATE，
	// 上面那圈存在性断言照样全过，而库已经被清空了。
	var n int
	require.NoError(t, queryRow(s2, `SELECT COUNT(*) FROM `+TableObservations).Scan(&n))
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
	require.NoError(t, queryRow(s, `PRAGMA journal_mode`).Scan(&mode))
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
		queryRow(s, `SELECT sql FROM sqlite_master WHERE type = 'view' AND name = ?`,
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
// TestNewStoreRejectsDriftedCurrentView 修 QA 的 C1（CRITICAL）。
//
// 我在 T4 只把**表的列**纳入了一致性检查。但 `CREATE VIEW IF NOT EXISTS` 对已存在的
// 视图同样**空转**，而 verifyObservationsSchema 只读 pragma_table_info——**视图完全不在
// 视野内**。于是老库里一个业务键写错的旧视图会原样保留，NewStore 返回 nil，
// 而那个视图是**全部下游读取的入口**。
//
// 这与 TASK-003 交接给 T4 的那条是同一位置的两个不同问题：
// TestNewStoreDeploysViewFromItsOwnSpec 验的是「新建库时部署的视图对不对」，
// 它每次都在空库上跑，**结构上碰不到「老库里已存在的错视图」**。
//
// 本条先证明该漂移**确有危害**（不只是「定义不一样」），再要求 NewStore 拦下它。
func TestNewStoreRejectsDriftedCurrentView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-view.db")

	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	for _, ddl := range []string{observationsDDL(), pendingDDL()} {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}
	// 老版本建的视图：业务键漏了 period_type，只按 period 关联
	_, err = db.Exec(`CREATE VIEW ` + viewCurrent + ` AS SELECT * FROM ` + TableObservations +
		` o WHERE o.published_at = (SELECT MAX(published_at) FROM ` + TableObservations +
		` WHERE period = o.period)`)
	require.NoError(t, err)

	// 先坐实危害：同一 period 的两个 period_type 应各出一行，坏视图只给一行。
	//
	// 两行的 published_at 必须**不同**——MAX() 是在漏掉 period_type 的分组上取的，
	// 只有当两个 period_type 的 published_at 不同时，较早的那个才会被较晚的吞掉。
	// （初版 fixture 给两行用了同一个 published_at，坏视图照样返回两行，
	//   等于没复现出危害。这条前提自证就是为了逼出那种情况。）
	rows := []struct{ periodType, publishedAt string }{
		{"monthly", "2026-07-15"},
		{"h1", "2026-08-20"},
	}
	for _, r := range rows {
		_, err = db.Exec(`INSERT INTO `+TableObservations+
			` (period, period_type, published_at, article_id, caliber_version, extractor, ingested_at)`+
			` VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"2026-06", r.periodType, r.publishedAt, "art-"+r.periodType, "2025-01", "rule@v2",
			"2026-07-16T00:00:00Z")
		require.NoError(t, err)
	}
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+viewCurrent).Scan(&n))
	require.Equal(t, 1, n,
		"前提自证：漏 period_type 的视图会让同一 period 的两个 period_type 互相吞掉")

	var survivor string
	require.NoError(t, db.QueryRow(`SELECT period_type FROM `+viewCurrent).Scan(&survivor))
	require.Equal(t, "h1", survivor, "被吞掉的是 published_at 较早的 monthly")
	require.NoError(t, db.Close())

	s, err := NewStore(path)
	if s != nil {
		defer s.Close()
	}
	require.Error(t, err, "老库里业务键错误的视图必须被拦下——它是全部下游读取的入口")
	assert.Contains(t, err.Error(), viewCurrent, "错误应指名是哪个视图")
	assert.Contains(t, err.Error(), "migrat", "错误应带迁移提示")
	assert.Nil(t, s, "失败时不应返回一个半可用的 Store")
}

func TestNewStoreToleratesUnknownExtraColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")

	s1, err := NewStore(path)
	require.NoError(t, err)
	_, err = rawDB(t, path).Exec(`ALTER TABLE ` + TableObservations + ` ADD COLUMN from_a_newer_version REAL`)
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
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM `+TableObservations).Scan(&n))
	assert.Equal(t, 0, n)
}

func TestStoreCloseReleasesDB(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "hestia.db"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// database/sql 在 Close 之后返回的是未导出的 errDBClosed（"sql: database is closed"），
	// 没有可用的 sentinel，所以只能比对错误串。注意它不是 sql.ErrConnDone——
	// 后者是「这条连接已还回池」，与「整个 DB 已关闭」是两回事。
	err = s.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM `+TableObservations).Scan(new(int))
	assert.ErrorContains(t, err, "database is closed", "Close 之后句柄必须已释放")
}

// TestStoreExposesNoWriteMethods 钉住单一写入口约束。
//
// Save 在 TASK-005 引入，本条随之更新为 [Close, DB, Save]——**仍是精确集合相等，
// 没有弱化成「包含」**：新增任何导出方法都必须在这里显式登记一次，让「又开了一个
// 写口」成为一个需要动手改测试的决定。ADR-0003 在同机场景下唯一的防线就是 Save 的签名。
//
// Preceding 在 M1b-3 / TASK-003 追加，本条随之更新为 [Close, DB, Preceding, Save]，
// **仍是精确集合相等**。它是 Store 的第一个读方法：只发 SELECT、走 v_hestia_current
// 视图，不碰任何写路径，因此扩大的是读面而不是写面。登记而不放松的理由同 Save——
// 断言的形状会把任何新增导出方法都判成违规（无论读写），这正是它逼人留下这段说明的方式。
//
// HasPeriod 在 M1b-4a / TASK-004 追加，本条随之更新为
// [Close, DB, HasPeriod, Preceding, Save]，**仍是精确集合相等**。它是 Store 的第二个
// 读方法：只发一条 SELECT ... LIMIT 1、走 v_hestia_current 视图，不碰任何写路径。
// Discover 经 PeriodChecker 窄接口消费它，用来决定翻页何时停。
//
// HasPeriod 是 *Store 的方法，所以它**同时**打红本条（reflect 版）与
// TestPackageExposesNoWriteFunctions（AST 版），两条都登记过才算数——这与
// Store.Preceding 同形，也再次演示了那两条测试为什么互补而不能互替。
//
// HasArticle 在 M1b-4b / TASK-003 追加，本条随之更新为
// [Close, DB, HasArticle, HasPeriod, Preceding, Save]，**仍是精确集合相等**。它是
// Store 的第三个读方法：一条 SELECT ... UNION ALL ... LIMIT 1，不碰任何写路径。
// 与 HasPeriod 同形（*Store 的方法 ⇒ 同时打红两条），登记理由见 AST 版下面那段。
func TestStoreExposesNoWriteMethods(t *testing.T) {
	typ := reflect.TypeOf(&Store{})
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	assert.Equal(t, []string{"Close", "DB", "HasArticle", "HasPeriod", "Preceding", "RecentObservations", "RecentPending", "Save"}, got,
		"只应导出 Close、DB、HasArticle、HasPeriod、Preceding、RecentObservations、RecentPending 与 Save；出现 Insert/Upsert 等写口即违反单一写入口约束")
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

	assert.Equal(t, []string{"DefaultThresholds", "Discover", "LoadConfig", "NewPBOCFetcher", "NewStore", "Parse", "RenderStatus", "Store.Close", "Store.DB", "Store.HasArticle", "Store.HasPeriod", "Store.Preceding", "Store.RecentObservations", "Store.RecentPending", "Store.Save", "Validate"}, got,
		"包的导出函数/方法必须恰好是这十七个——任何新增的包级写口（如 InsertRow）都会绕过 Save 的签名防线")
}

// —— 为什么名单里多了 Parse（M1b-2 / TASK-006 追加）——
//
// 本条守的是**写口**，而 Parse 是纯函数：输入 []byte、输出 Observation，不碰数据库。
// 它被列进名单不是对守卫的放宽，而是这条断言用的是**全导出面精确相等**——那个形状
// 会把任何新增导出物都判成违规，无论它是不是写口。精确相等是有意为之（它顺带
// 逼着每次扩大导出面都要在这里留一行说明），所以正确的处置是登记而不是放松断言。
//
// Parse 不碰库这一点由 parse_test.go 的 TestParseDoesNotTouchStorage 独立守着：
// 它用 go/parser 断言 parse.go 既不 import database/sql、也没有任何 .Save( 调用。
// 两条测试的分工是「本条管导出面的**形状**，那条管 parse.go 的**行为**」——
// 只有本条时，一个叫 Parse 却偷偷写库的函数照样在名单里；只有那条时，
// 新增一个 InsertRow 无人拦。
//
// —— 为什么名单里多了 DefaultThresholds（M1b-3 / TASK-001 追加）——
//
// 同 Parse，是登记而不是放宽。DefaultThresholds 返回一个 Thresholds 值，
// 既不碰库也不带副作用；它必须导出，是因为 M1b-4 的 cobra 命令要拿它当
// Validate 的默认入参——校验阈值属于调用方能覆盖的策略，而不是包的内部常量。
// 它排在名单最前是 sort.Strings 的字节序结果（"D" < "N"），不是优先级。
//
// —— 为什么名单里多了 Store.Preceding（M1b-3 / TASK-003 追加）——
//
// 同样是登记。它是 Store 的第一个**读**方法（SELECT + v_hestia_current 视图），
// 闸门层经 History 窄接口消费它。与上面 DefaultThresholds 的区别在于：Preceding 是
// *Store 的方法，所以它**同时**打红本条（AST 版）与 TestStoreExposesNoWriteMethods
// （reflect 版）；DefaultThresholds 是包级函数，只打红本条。两条都登记过才算数——
// 这恰好也演示了那两条测试为什么互补而不能互替。
//
// —— 为什么名单里多了 Validate（M1b-3 / TASK-004 追加）——
//
// 同样是登记。Validate 是纯函数：吃 Observation + History + Thresholds，吐
// ValidationReport，**不碰数据库**——它产出的正是 Save 要求的那份报告，属于
// Save 签名防线的上游而不是绕过它。它是包级函数，所以只打红本条（同
// DefaultThresholds），排在末位是字节序结果（"V" > "S"）。
//
// History（接口类型）与 NoHistory（变量）都不是 FuncDecl，不进本条视野，无需登记。
//
// —— 为什么名单里多了 NewPBOCFetcher（M1b-4a / TASK-001 追加）——
//
// 同样是登记而不是放宽。NewPBOCFetcher 是构造器：返回一个只发 GET、把响应体
// 原样吐出来的 Fetcher，**不碰数据库**，也没有任何通向 Save 的路径——它开的是
// 一个「读外部 HTTP」的口，不是写口。它必须导出，是因为 M1b-4b 的 ingest 与
// cmd 层要拿它当 Discover 的入参（discover 的测试则喂快照 fake，不碰网络）。
// 它是包级函数，所以只打红本条（同 DefaultThresholds/Validate），排在
// DefaultThresholds 之后是字节序结果（"D" < "N"，且 "NewP" < "NewS"）。
//
// —— 为什么名单里多了 Store.HasPeriod（M1b-4a / TASK-004 追加）——
//
// 同样是登记而不是放宽。HasPeriod 是 Store 的第二个**读**方法：一条
// SELECT 1 FROM v_hestia_current ... LIMIT 1，不碰任何写路径。Discover 经
// PeriodChecker 窄接口消费它，用来决定翻页何时停。
//
// 它与 Store.Preceding 同形：**是 *Store 的方法，所以同时打红本条（AST 版）与
// TestStoreExposesNoWriteMethods（reflect 版）**，两条都登记过才算数。与之相对，
// 同一迭代里 TASK-001 的 NewPBOCFetcher 是包级函数，只打红本条。**同一个 Sprint
// 里两种形态各出现一次**，这比任何说明都更直接地演示了两条守卫为什么不能互替。
//
// 本迭代新增的另外两个导出物**实测确认不进本条视野**（跑一次只加它们的版本核对过，
// 不是照抄推断）：Fetcher 是**接口类型**——本条只收 *ast.FuncDecl，类型声明是
// GenDecl；pbocFetcher.Get 的**接收者未导出**，被上面 ast.IsExported(recv) 那道
// 分支排除，包外根本拿不到它。两者也都不打红 reflect 版（它只看 *Store 的方法集）。
//
// —— 为什么名单里多了 Discover（M1b-4a / TASK-005 追加）——
//
// 同样是登记而不是放宽：断言仍是 assert.Equal 的**全导出面精确集合相等**，只是切片里
// 多了一项（十→十一），没有换成 Subset/Contains。Discover 是包级函数，读 index 页、
// 产出候选清单，**不碰数据库**——它连 Store 都不持有，只经 PeriodChecker 窄接口问
// 「这一期入库了没有」，那是个只读判定。它必须导出，是因为 M1b-4b 的 ingest 与 cmd 层
// 要调它。排在 DefaultThresholds 之后是字节序结果（"De" < "Di" < "N"）。
//
// 它与 NewPBOCFetcher 同形：包级函数 ⇒ **只打红本条（AST 版）**，reflect 版全程绿
// （实测确认，非推断）。同一迭代里 DiscoverCfg 与 Candidate 都是**结构体类型**，
// 与 Fetcher 同理不进本条视野——本条只收 FuncDecl。
//
// ⚠️ 这一条的正确性**只能靠人逐字核对交付 diff**：把新增项从期望列表里删掉会让本条
// 变红（那证明守卫确实在按精确集合相等工作），但**把 assert.Equal 换成 assert.Subset
// 不会让任何东西变红**——「守卫的守卫」本身无人守。所以审查这一行时要看的不是
// 「测试绿不绿」，而是「是不是追加一项、断言形状有没有被动过」。
//
// —— 为什么名单里多了 LoadConfig（M1b-4a / TASK-007 追加）——
//
// 同样是登记而不是放宽：切片十一→十二，assert.Equal 的全导出面精确集合相等一字未动。
// LoadConfig 是包级函数：读一个 YAML 文件、返回 Config 值，**不碰数据库**，也没有任何
// 通向 Save 的路径——它开的是「读本地配置文件」的口。它必须导出，是因为 M1b-4b 的
// ingest 与 cmd 层要拿它装载阈值与 discover 参数。排在 Discover 之后是字节序结果
// （"Di" < "Lo" < "Ne"）。
//
// 它与 Discover/NewPBOCFetcher 同形：包级函数 ⇒ **只打红本条（AST 版）**，reflect 版
// 全程绿（实测确认）。同一任务新增的 Config 是**结构体类型**，与 DiscoverCfg/Candidate
// 同理不进本条视野——本条只收 FuncDecl。
//
// —— 为什么名单里多了 Store.HasArticle（M1b-4b / TASK-003 追加）——
//
// 同样是登记而不是放宽：切片十二→十三，assert.Equal 的全导出面精确集合相等一字未动。
// HasArticle 是 Store 的第三个**读**方法：一条 SELECT 1 ... UNION ALL ... LIMIT 1，
// 只查存在性，不碰任何写路径。它是方案报告 4.1 的一级幂等键，M1b-4b 的 ingest 用它
// 决定这篇文章要不要抓。
//
// 它与 Store.HasPeriod/Store.Preceding 同形：**是 *Store 的方法，所以同时打红本条
// （AST 版）与 TestStoreExposesNoWriteMethods（reflect 版）**，两条都登记过才算数。
//
// 排在 Store.DB 之后、Store.HasPeriod 之前是 sort.Strings 的字节序结果
// （"Store.D" < "Store.HasA" < "Store.HasP"），不是优先级。
//
// 本次登记的**正向自证**（实测，非推断）：把 "Store.HasArticle" 从上面的期望切片里
// 删掉，本条立刻红在 assert.Equal 那一行——这证明守卫确实在按精确集合相等工作，
// 而不是被悄悄放宽成了包含关系。反过来的「换成 assert.Subset」那种变异证明不了
// 本次登记（它必然存活），所以没跑。
//
// —— 为什么名单里多了 RenderStatus / Store.RecentObservations / Store.RecentPending
//    （M1b-4b / TASK-006 追加）——
//
// 同样是登记而不是放宽：切片十三→十六，assert.Equal 的全导出面精确集合相等一字未动。
// 三者都是 status 命令的读路径：两个 *Store 方法各发一条 SELECT（分别打 viewCurrent
// 与 TablePending），RenderStatus 是纯函数——吃两个切片吐文本，连 Store 都不持有，
// **不碰数据库**。没有任何一个通向 Save 的路径。
//
// 三者恰好把两条守卫的分工又演示了一遍，且这次**同一个任务里两种形态都出现**：
//   - Store.RecentObservations / Store.RecentPending 是 *Store 的方法 ⇒ **同时**打红
//     本条（AST 版）与 TestStoreExposesNoWriteMethods（reflect 版），两条都登记过才算数
//   - RenderStatus 是包级函数 ⇒ **只**打红本条，reflect 版全程绿
//
// 同任务新增的 StatusRow / PendingRow 是**结构体类型**，与 DiscoverCfg/Candidate/Config
// 同理不进本条视野——本条只收 *ast.FuncDecl，类型声明是 GenDecl。
//
// **这一条不需要单独跑实验，本条自己就是证据**：期望切片里没有 StatusRow / PendingRow，
// 而断言是全集精确相等——它们若进了视野，本条此刻就是红的。（先前几段写的是「实测确认」
// 那种形式，那对**当时**的作者是必要的；这里换成直接引用本条的绿，是因为它更强：
// 不依赖另跑一次，也不依赖我记得跑过。）
//
// 排序是 sort.Strings 的字节序结果，不是优先级："Parse" < "RenderStatus" < "Store."，
// 而三个 Store 方法之间 "Store.Preceding" < "Store.RecentObservations" <
// "Store.RecentPending" < "Store.Save"（"Rec" < "S"，"RecentO" < "RecentP"）。
//
// —— 为什么名单里多了 Ingest（M1b-4b / TASK-005 追加）——
//
// 同样是登记而不是放宽：切片十六→十七，assert.Equal 的全导出面精确集合相等一字未动。
// Ingest 是包级函数：跑一轮「发现→抓→解析→校验→入库」的编排。它**自己不写库** ——
// 唯一的写动作是调 Store.Save，也就是说它走的正是 Save 那道签名防线，而不是绕过它。
// 它必须导出，是因为 cmd 层要调它（cmd 只做装配）。
//
// 它与 Discover/LoadConfig/RenderStatus 同形：包级函数 ⇒ **只打红本条（AST 版）**，
// reflect 版全程绿。同任务新增的 IngestDeps 是**结构体类型**，与 StatusRow/DiscoverCfg
// 同理不进本条视野；它的方法 ingestOne 名字不导出，被上面 fn.Name.IsExported() 那道
// 分支排除。**这两条同样不需要另跑实验**：期望切片里没有它们，而断言是全集精确相等
// ——它们若进了视野，本条此刻就是红的（沿用上一段那个更强的论证形式）。
//
// 排在 Discover 之后、LoadConfig 之前是字节序结果（"Di" < "In" < "Lo"）。
//
// ⚠️ **本次登记撞上了一次真实的并发冲突，记在这里免得后人重蹈**：TASK-005 与 TASK-006
// 同在 wave2、同一棵工作树，而两者都必须改这同一行。本条的作用域是**全包**（go/parser
// 扫全部非 _test.go 文件），所以任一方的新导出物一落盘，另一方的包测试立刻红 —— 双向、
// 必然、与代码对错无关。⇒ 同 wave 内有多人新增导出物时，**登记必须串行**，且后到的那个
// 只能**追加**、不能把先到者的条目覆盖掉（这里 TASK-006 先落，TASK-005 追加 "Ingest"）。

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
// boundary[1]       全 54 字段这一极端                  → TestSaveAllFields
//                   另一极端「空 Values」在 M1b-1.5 被改判：TestSaveEmptyValues 已删除，
//                   现由 TestSaveRejectsPassedWithNoValues（Passed=true 时拒绝）与
//                   TestSavePendingAcceptsEmptyValues（Passed=false 时落 pending）覆盖
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
	require.NoError(t, queryRow(s, `SELECT COUNT(*) FROM `+table).Scan(&n))
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
	require.NoError(t, queryRow(s, `SELECT m2 FROM `+TableObservations).Scan(&m2))
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
	require.NoError(t, queryRow(s, `SELECT m2 FROM `+viewCurrent).Scan(&m2))
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
	require.NoError(t, queryRow(s, `SELECT m2 FROM `+viewCurrent).Scan(&m2))
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
	// 除 article_id 外每项都受 Meta.validate 约束——前三个是形态，后两个是白名单
	// （C-2 之后哨兵串不再被接受）——所以取的是「合法且彼此不同」的值。
	obs.Meta.Period = "2026-06"
	obs.Meta.PeriodType = "h1"
	obs.Meta.PublishedAt = "2026-07-15"
	obs.Meta.ArticleID = "article-id-sentinel"
	obs.Meta.CaliberVersion = "2025-01"
	obs.Meta.Extractor = "rule@v1"
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
		require.NoError(t, queryRow(s, `SELECT `+col+` FROM `+TableObservations).Scan(&got))
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
	require.NoError(t, queryRow(s, `SELECT `+FieldTSFStock+` FROM `+TableObservations).Scan(&tsf))
	assert.False(t, tsf.Valid, "未提供的字段应为 NULL，而不是 0")

	// 0 与 NULL 必须可区分：显式写入的 0 要留下一个非 NULL 的 0
	obs2 := obsWith(map[string]float64{FieldTSFFlowYTD: 0})
	obs2.Meta.PublishedAt = "2026-08-20"
	_, err = s.Save(context.Background(), obs2, passing())
	require.NoError(t, err)

	var flow sql.NullFloat64
	require.NoError(t, queryRow(s,
		`SELECT `+FieldTSFFlowYTD+` FROM `+TableObservations+` WHERE published_at = '2026-08-20'`).
		Scan(&flow))
	assert.True(t, flow.Valid, "显式写入的 0 是「增量为零」，不是「未披露」")
	assert.Equal(t, 0.0, flow.Float64)
}

// TestSaveEmptyValues 已删除（M1b-1.5）。它当初钉住「空 Values + Passed=true
// 可写入权威表」，而那个行为现被判定为缺陷：一次解析全失败先占住业务键后，
// 之后任何正确重跑都判 Duplicate，字段永远补不回来。
// 取代它的是 TestSaveRejectsPassedWithNoValues 与 TestSavePendingAcceptsEmptyValues。

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
		require.NoErrorf(t, queryRow(s, `SELECT `+f+` FROM `+TableObservations).Scan(&got),
			"读 %s", f)
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
	require.NoError(t, queryRow(s, `SELECT ingested_at FROM `+TableObservations).Scan(&got))
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
		ValidationReport{Passed: true, Checks: []Check{
			{ID: "tsf_sum", Status: CheckSkipped, Reason: "absent_field:tsf_stock"},
		}})
	require.NoError(t, err, "CheckSkipped 不是失败")
	assert.Equal(t, 1, countRows(t, s2, TableObservations))
}

// TestSaveRejectsUnknownCheckStatus 堵上 G10 守护的绕过口（返工 C2）。
//
// CheckStatus 是 string 具名类型，任何字符串都能构造出一个 Check。原实现只做
// `c.Status == CheckFailed` 单值比较——M1b-3 写成 "FAILED" / "fail" / 任何错拼，
// Passed=true 的自相矛盾报告就照常入库、Save 返回 nil。**而观测表不存校验痕迹**，
// 那种行落库后无从审计它是否真跑过闸门，正是 G10 要防的后果。
//
// 同一文件的 checkValues 对 Values 的键用的是白名单（未知键拒绝且零行）；同一个包、
// 同一条 Save 路径、同一类外部输入，把关强度必须一致。故此处也改白名单：
// Passed=true 时每个 Status 必须 ∈ {CheckPassed, CheckSkipped}，其余一律拒绝。
func TestSaveRejectsUnknownCheckStatus(t *testing.T) {
	for _, bad := range []CheckStatus{
		"FAILED",  // 大小写错拼——最可能的一种
		"fail",    // 词形错拼
		"",        // 零值：结构体字面量漏填 Status
		"PASSED",  // 看起来像通过，实则不在白名单
		"pending", // 合理但未定义的第四种状态
		"passed ", // 尾随空格
	} {
		t.Run(string("status="+bad), func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Save(context.Background(),
				obsWith(map[string]float64{FieldM2: 1}),
				ValidationReport{Passed: true, Checks: []Check{
					{ID: "monetary_hierarchy", Status: CheckPassed},
					{ID: "deposit_sum", Status: bad},
				}})
			require.Error(t, err, "未知状态必须拒绝，不能默认「不等于 failed 就是好的」")
			assert.Contains(t, err.Error(), string(bad),
				"错误须回显原串，M1b-3 才知道是哪个拼写出了问题")
			assert.Contains(t, err.Error(), "deposit_sum", "错误须指明是哪个检查")
			assertNoRowsAnywhere(t, s)
		})
	}

	// 对照组：白名单内的两个状态在 Passed=true 下必须放行，
	// 否则「一律拒绝」同样是全绿测试发现不了的过度拦截。
	s := newTestStore(t)
	_, err := s.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}),
		ValidationReport{Passed: true, Checks: []Check{
			{ID: "monetary_hierarchy", Status: CheckPassed},
			{ID: "mom_jump", Status: CheckSkipped, Reason: "no_prior_period"},
		}})
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, s, TableObservations))
}

// TestSaveRejectsSkippedWithoutReason 让 types.go 里「Skipped 必填 Reason」的注释
// 真正成立（返工 C2 的顺带项）。
//
// 在此之前那句注释宣称了一个没有任何机制强制的约束——与 G9 曾经的「宣称并发、
// 机制不支持」是同一种形状。跳过而不说为什么，事后无法区分「字段本就缺失」与
// 「闸门自己出错跳过了」。
//
// 只在 Passed=true 时校验，与 checkReportConsistency 的既有结构一致。
func TestSaveRejectsSkippedWithoutReason(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}),
		ValidationReport{Passed: true, Checks: []Check{
			{ID: "stock_continuity", Status: CheckSkipped}, // Reason 为空
		}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stock_continuity")
	assert.Contains(t, err.Error(), "reason")
	assertNoRowsAnywhere(t, s)

	// 只要求非空，不校验格式——absent_field:<name> | no_prior_period 的形态归 M1b-3
	s2 := newTestStore(t)
	_, err = s2.Save(context.Background(), obsWith(map[string]float64{FieldM2: 1}),
		ValidationReport{Passed: true, Checks: []Check{
			{ID: "stock_continuity", Status: CheckSkipped, Reason: "whatever"},
		}})
	require.NoError(t, err, "本层只要求 Reason 非空，格式约定归闸门层")
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
	s, path := newTestStoreAt(t)
	_, err := rawDB(t, path).Exec(
		`INSERT INTO `+TableObservations+
			` (period, period_type, published_at, article_id, caliber_version, extractor, ingested_at, `+
			FieldM2+`) VALUES (?,?,?,?,?,?,?,?)`,
		"2026-06", "h1", "2026-07-15", "a", "2025-01", "rule@v2", "2026-07-16T00:00:00Z",
		math.NaN())
	require.NoError(t, err, "SQLite 本身并不拒绝 NaN——所以拦截必须发生在 Save 里")

	var typ string
	var isNull int
	require.NoError(t, queryRow(s,
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
	rows, err := queryRows(s,
		`SELECT article_id, extractor, ingested_at, report, values_json FROM `+
			TablePending+` ORDER BY ingested_at`)
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
	require.NoError(t, queryRow(s, `SELECT article_id FROM `+TableObservations).Scan(&id))
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
	require.NoError(t, queryRow(s,
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
	s, path := newTestStoreAt(t)
	ctx := context.Background()

	// 造一个「结构与代码预期不符」的 pending 表
	_, err := rawDB(t, path).Exec(
		`ALTER TABLE ` + TablePending + ` RENAME COLUMN values_json TO values_json_legacy`)
	require.NoError(t, err)

	out, err := s.Save(ctx, obsWith(map[string]float64{FieldM2: 1}), failing())

	require.Error(t, err, "pending 表结构不符时必须当场失败")
	assert.Contains(t, err.Error(), "pending", "错误必须指明是写 pending 这一步")
	assert.Contains(t, err.Error(), "2026-06", "错误必须带上是哪一期，否则无法定位")
	assert.Equal(t, Outcome{}, out, "失败时不得返回一个看起来成功的 Outcome")
	assert.Equal(t, 0, countRows(t, s, TableObservations), "未过闸的数据不得漏进观测表")
}

// TestSavePendingColumnsMatchTheirValues 修 QA 的 C3。
//
// pending 的八列是 savePending 里**手写**的位置参数，与观测表不同——观测表的列序由
// metaColumns/fieldOrder 派生且有三条同序测试钉住，pending 这份没有任何东西钉。
// 原有的 pending 测试虽然 SELECT 了 article_id / extractor，却**从不断言它们的值**，
// 于是同时交换 Period↔PeriodType 与 ArticleID↔Extractor 的变异完全存活。
//
// 「三处同序危害已闭合」这个说法当时只对观测表成立，pending 是漏掉的第四处。
// 判据手法与 TestSaveMetaValuesLandInMatchingColumns 相同：取互不相同的可辨识值，
// 逐列读回比对；先自证 fixture 够强，否则「互换必红」不成立。
func TestSavePendingColumnsMatchTheirValues(t *testing.T) {
	s := newTestStore(t)
	fixed := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }

	obs := obsWith(map[string]float64{FieldM2: 356.71})
	// 六个值互不相同：任意两列互换都会让下面的逐列比对红。
	// article_id 无取值域约束故用哨兵串；extractor 有白名单，取一个与其余不同的合法值
	obs.Meta.Period = "2026-06"
	obs.Meta.PeriodType = "h1"
	obs.Meta.PublishedAt = "2026-07-15"
	obs.Meta.ArticleID = "article-id-sentinel"
	obs.Meta.Extractor = "rule@v1"

	_, err := s.Save(context.Background(), obs, failing())
	require.NoError(t, err)

	want := []struct{ col, val string }{
		{"period", "2026-06"},
		{"period_type", "h1"},
		{"published_at", "2026-07-15"},
		{"article_id", "article-id-sentinel"},
		{"extractor", "rule@v1"},
		{"ingested_at", fixed.Format(time.RFC3339Nano)},
	}

	// 先自证 fixture 够强：六个值必须互不相同，否则任意两列互换都发现不了
	seen := map[string]bool{}
	for _, w := range want {
		require.Falsef(t, seen[w.val],
			"fixture 太弱：值 %q 重复出现，列互换将无法被发现", w.val)
		seen[w.val] = true
	}

	for _, w := range want {
		var got string
		require.NoError(t, queryRow(s, `SELECT `+w.col+` FROM `+TablePending).Scan(&got))
		assert.Equalf(t, w.val, got,
			"pending 列 %s 的取值错位——八列是手写位置参数，错位不会有任何报错", w.col)
	}

	// report 与 values_json 是内容型而非位置型，单独确认没被互换
	rows := pendingRows(t, s)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].report, `"Passed"`, "report 列必须是序列化后的校验报告")
	assert.Contains(t, rows[0].valuesJSON, FieldM2, "values_json 列必须是序列化后的 Values")
}

// TestSaveDuplicateOnlyTouchesTheMatchingRevision 修 QA 的 C4。
//
// refreshArticleID 的 WHERE 里带 published_at，但**现有两条 Duplicate 测试的库里都只有
// 一行**——删掉 `AND published_at = ?` 之后行为完全不变，结构上打不中这个缺陷。
//
// 两行库下才看得出危害：同一业务键会有多个修订行（这正是双时态的常态），少了
// published_at 约束，一次站点迁移的新 article_id 会被**盖到全部历史行上**，
// 静默毁掉逐行溯源，并让 M1b-4 的 article_id 幂等检查命中错误的修订行。
func TestSaveDuplicateOnlyTouchesTheMatchingRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	original := obsWith(map[string]float64{FieldM2: 1})
	original.Meta.PublishedAt = "2026-07-15"
	original.Meta.ArticleID = "art-v1"
	_, err := s.Save(ctx, original, passing())
	require.NoError(t, err)

	revised := obsWith(map[string]float64{FieldM2: 2})
	revised.Meta.PublishedAt = "2026-08-20"
	revised.Meta.ArticleID = "art-v2"
	out, err := s.Save(ctx, revised, passing())
	require.NoError(t, err)
	require.Equal(t, bitemporal.Revision, out.Verdict)
	require.Equal(t, 2, countRows(t, s, TableObservations), "前提自证：库里必须有两行修订")

	// 站点迁移：最新那次修订换了 URL，发布日不变 ⇒ Duplicate
	migrated := obsWith(map[string]float64{FieldM2: 2})
	migrated.Meta.PublishedAt = "2026-08-20"
	migrated.Meta.ArticleID = "art-v2-NEWURL"
	out, err = s.Save(ctx, migrated, passing())
	require.NoError(t, err)
	require.Equal(t, bitemporal.Duplicate, out.Verdict)
	assert.Equal(t, 2, countRows(t, s, TableObservations), "不得新增行")

	got := map[string]string{}
	rows, err := queryRows(s, `SELECT published_at, article_id FROM `+TableObservations)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var pub, id string
		require.NoError(t, rows.Scan(&pub, &id))
		got[pub] = id
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, "art-v2-NEWURL", got["2026-08-20"], "目标修订行的 article_id 必须刷新")
	assert.Equal(t, "art-v1", got["2026-07-15"],
		"历史修订行必须原值保留——盖掉它会毁掉逐行溯源，且让 article_id 幂等检查命中错误的行")
}

// ── M1b-1.5 · C-1：ValidationReport 里的非有限值 ───────────────────────────
// error_handling[C-1] rep.Checks[].Value 为 NaN/Inf → 入口拒绝且信息指名 check
//                                                    → TestSaveRejectsNonFiniteCheckValue
// error_handling[C-1] 有限值与 nil 不受影响          → TestSaveAcceptsFiniteAndNilCheckValues

func TestSaveRejectsNonFiniteCheckValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			v := tc.val
			rep := ValidationReport{Passed: false, Checks: []Check{
				{ID: "deposit_sum", Status: CheckFailed, Value: &v},
			}}

			_, err := s.Save(context.Background(),
				obsWith(map[string]float64{FieldM2: 356.71}), rep)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "deposit_sum", "错误应指名是哪道闸门")

			// 连 pending 都没有——这是有意的，不是遗留缺陷。
			// Check.Value 是 NaN 说明闸门实现有 bug（比率型 0/0 未处理），那是
			// M1b-3 的代码错误，应当响亮失败让开发者立刻看见。若在此静默净化成
			// null 写进 pending，闸门的 bug 会被掩盖，而那一期数据本来就可以在
			// 修完闸门后重跑补回。
			assertNoRowsAnywhere(t, s)
		})
	}
}

func TestSaveAcceptsFiniteAndNilCheckValues(t *testing.T) {
	s := newTestStore(t)
	finite := 0.0857
	rep := ValidationReport{Passed: false, Checks: []Check{
		{ID: "deposit_sum", Status: CheckFailed, Value: &finite},
		{ID: "monetary_hierarchy", Status: CheckFailed, Value: nil},
		{ID: "stock_continuity", Status: CheckSkipped, Reason: "absent_field:tsf_stock"},
	}}
	_, err := s.Save(context.Background(),
		obsWith(map[string]float64{FieldM2: 356.71}), rep)
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, s, TablePending))
}

// ── M1b-1.5 · Passed=true 的空内容 ─────────────────────────────────────────
// error_handling[#7]  Passed=true 但 Checks 为空 → 拒绝  → TestSaveRejectsPassedWithNoChecks
// error_handling[obs] Passed=true 但 Values 为空 → 拒绝  → TestSaveRejectsPassedWithNoValues
// boundary[obs]       Passed=false 时空 Values 仍落 pending → TestSavePendingAcceptsEmptyValues

func TestSaveRejectsPassedWithNoChecks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		checks []Check
	}{
		{"nil", nil},
		{"empty", []Check{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Save(context.Background(),
				obsWith(map[string]float64{FieldM2: 356.71}),
				ValidationReport{Passed: true, Checks: tc.checks})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "zero checks")
			assertNoRowsAnywhere(t, s)
		})
	}
}

func TestSaveRejectsPassedWithNoValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]float64
	}{
		{"empty map", map[string]float64{}},
		{"nil map", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Save(context.Background(), obsWith(tc.values), passing())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "empty")
			assert.Equal(t, 0, countRows(t, s, TableObservations),
				"空 Values 一旦占住业务键，之后任何正确重跑都判 Duplicate，字段永远补不回来")
		})
	}
}

func TestSavePendingAcceptsEmptyValues(t *testing.T) {
	// 解析全失败时 Values 就是空的，而这正是 pending 要接住的情况——那条记录
	// 本身是诊断信息。拒绝它会让「什么都没解析出来」这件事无处留痕。
	s := newTestStore(t)
	rep := ValidationReport{Passed: false, Checks: []Check{
		{ID: "completeness", Status: CheckFailed},
	}}
	out, err := s.Save(context.Background(), obsWith(map[string]float64{}), rep)
	require.NoError(t, err)
	assert.Equal(t, TablePending, out.Table)
	assert.Equal(t, 1, countRows(t, s, TablePending))
	assert.Equal(t, 0, countRows(t, s, TableObservations))
}

// ── M1b-1.5 · #1：DB() 收窄为只读 ──────────────────────────────────────────
// non_functional[#1] DB() 返回的类型不得具备写能力 → TestDBReturnsReadOnlyHandle

func TestDBReturnsReadOnlyHandle(t *testing.T) {
	// 查的是 DB() 的**静态**返回类型，不是动态类型。
	//
	// 直觉写法 `var h any = s.DB(); _, ok := h.(interface{ Exec(...) })` 是错的：
	// Go 的类型断言看动态类型，而 Querier 接口里装的仍是 *sql.DB，断言必然成功——
	// 那个测试即便在收窄之后也照样红，测不出任何东西。
	//
	// 真正要保证的是「调用方在编译期拿到的类型没有写方法」，所以查方法签名。
	m, ok := reflect.TypeOf((*Store)(nil)).MethodByName("DB")
	require.True(t, ok, "Store 应有 DB 方法")
	ret := m.Type.Out(0)

	require.Equal(t, reflect.Interface, ret.Kind(),
		"DB() 必须返回接口而非具体类型——返回 *sql.DB 等于把写口一并交出去")

	for _, bad := range []string{"Exec", "ExecContext", "Begin", "BeginTx", "Prepare"} {
		_, has := ret.MethodByName(bad)
		assert.Falsef(t, has, "DB() 的返回接口不得含 %s", bad)
	}
	// 只读能力必须真的可用，否则 M1b-4 的 article_id 幂等检查无从下手
	for _, want := range []string{"QueryContext", "QueryRowContext"} {
		_, has := ret.MethodByName(want)
		assert.Truef(t, has, "DB() 的返回接口应提供 %s", want)
	}
}

// —— TASK-003: History 窄接口与 Store.Preceding ——

// saveMonthly 存一期已过闸的月度观测，供需要历史序列的测试造数据。
//
// 用 monthly 而不是 validMeta() 的 h1：h1 期次的月份必须是 06
// （types.go 的 periodEndMonth 校验），造六期连续历史就要跨六年。
func saveMonthly(t *testing.T, s *Store, period string, values map[string]float64) {
	t.Helper()
	_, err := s.Save(context.Background(), Observation{
		Meta: Meta{
			Period:         period,
			PeriodType:     "monthly",
			PublishedAt:    period + "-15",
			ArticleID:      "art-" + period,
			CaliberVersion: "2025-01",
			Extractor:      extractorV2,
		},
		Values: values,
	}, passing())
	require.NoError(t, err)
}

func TestPrecedingReturnsRecentPeriodsInDescendingOrder(t *testing.T) {
	s := newTestStore(t)
	for _, p := range []string{"2025-10", "2025-11", "2025-12", "2026-01"} {
		saveMonthly(t, s, p, map[string]float64{FieldM2: 300})
	}

	got, err := s.Preceding(context.Background(), "2026-01", "monthly", 2)
	require.NoError(t, err)
	require.Len(t, got, 2, "LIMIT 必须生效，且不含 period 自身")
	assert.Equal(t, "2025-12", got[0].Meta.Period, "最近的一期排最前")
	assert.Equal(t, "2025-11", got[1].Meta.Period)
}

// 库里的 NULL 必须还原成「键不存在」，显式写入的 0 必须保留。
//
// 两个方向缺一不可：只查前者会放过「把 0 也读丢了」，只查后者会放过
// 「把 NULL 读成 0」。后者更危险——stock_continuity 会拿这个 0 算环比。
func TestPrecedingRestoresAbsenceNotZero(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{
		FieldM2:             300,
		FieldDepositCorpYTD: 0, // 显式的零：真的「增量为零」
	})

	got, err := s.Preceding(context.Background(), "2026-01", "monthly", 1)
	require.NoError(t, err)
	require.Len(t, got, 1)

	_, hasTSF := got[0].Values[FieldTSFStock]
	assert.False(t, hasTSF, "未写入的字段读回来不该出现在 Values 里")

	v, hasZero := got[0].Values[FieldDepositCorpYTD]
	require.True(t, hasZero, "显式写入的 0 必须保留，不能被当成缺失")
	assert.Equal(t, 0.0, v)
}

func TestPrecedingIsolatesPeriodType(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	// 同一个 period 字符串上再存一条 annual：期次相同，序列不同。
	_, err := s.Save(context.Background(), Observation{
		Meta: Meta{
			Period: "2025-12", PeriodType: "annual", PublishedAt: "2026-01-15",
			ArticleID: "art-annual", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 999},
	}, passing())
	require.NoError(t, err)

	got, err := s.Preceding(context.Background(), "2026-01", "monthly", 6)
	require.NoError(t, err)
	require.Len(t, got, 1, "annual 那条不该混进 monthly 序列")
	assert.Equal(t, 300.0, got[0].Values[FieldM2])
}

// 修订产生新行而非覆盖，Preceding 走 v_hestia_current 视图，必须看到修订后的值。
func TestPrecedingSeesRevisions(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	_, err := s.Save(context.Background(), Observation{
		Meta: Meta{
			Period: "2025-12", PeriodType: "monthly", PublishedAt: "2026-02-20",
			ArticleID: "art-2025-12-rev", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 305},
	}, passing())
	require.NoError(t, err)

	got, err := s.Preceding(context.Background(), "2026-03", "monthly", 6)
	require.NoError(t, err)
	require.Len(t, got, 1, "修订不产生第二期，只是同一期的新行")
	assert.Equal(t, 305.0, got[0].Values[FieldM2], "必须取修订后的值")
}

func TestPrecedingOnEmptyHistory(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Preceding(context.Background(), "2026-01", "monthly", 6)
	require.NoError(t, err, "首期入库是正常路径，不是错误")
	assert.Empty(t, got)
}

// SQLite 的 LIMIT -1 表示不限制。不挡住非正数，n=0 会把整个序列拉回来。
func TestPrecedingRejectsNonPositiveN(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	for _, n := range []int{0, -1} {
		got, err := s.Preceding(context.Background(), "2026-01", "monthly", n)
		require.NoError(t, err)
		assert.Empty(t, got, "n=%d 应返回空而不是全部", n)
	}
}

// 查库真失败时必须返回 error，且**包住底层 err**（errors.Is 找得到）并带上
// period/periodType 上下文。
//
// 计划的 Step 1-12 没有覆盖这条（DoD error_handling[0] 要求），故本条为 TASK-003
// 自行补写。用「已取消的 context」而不是「关掉的库」来触发：database/sql 在 Close
// 之后返回的是未导出的 errDBClosed，没有可用的 sentinel，只能比对错误串；而
// context.Canceled 是标准 sentinel，能真的把 errors.Is 这条判据测出来。
//
// 上下文之所以必测：三处 %w 的包裹都写着同一个前缀，漏掉任何一处都会让运维
// 拿到一条不知道是哪个期次、哪条序列出的错。
func TestPrecedingWrapsQueryError(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消，QueryContext 必然失败

	got, err := s.Preceding(ctx, "2026-01", "monthly", 6)
	require.Error(t, err, "查库失败必须返回 error")
	assert.Nil(t, got, "出错时不得返回半截结果")
	assert.ErrorIs(t, err, context.Canceled, "必须用 %w 包住底层 err，否则调用方无法分辨是取消还是真故障")
	assert.ErrorContains(t, err, "2026-01", "错误信息要带 period")
	assert.ErrorContains(t, err, "monthly", "错误信息要带 periodType")

	// errors.Is 为真也可能是「err 本身就是 context.Canceled」（即根本没包）。
	// 要的是「包住」，所以额外断言 Unwrap 后还剩东西、且外层不等于内层。
	//
	// ⚠️ 这里原是 `require.NotErrorIs(t, errors.Unwrap(err), err)`（Sprint 035 的 F8），
	// **它在自己本该抓住的场景里平凡为真**：不包裹时 Unwrap 返回 nil，而
	// errors.Is(nil, x) 恒 false ⇒ NotErrorIs 恒成立。换成 NotNil 才真的在守
	// 「包住了」这件事。M1b-4a / TASK-007 修正。
	require.NotNil(t, errors.Unwrap(err), "必须包住底层错误：Unwrap 后应当还剩东西")
	assert.NotEqual(t, context.Canceled, err, "应是包裹后的错误而不是裸 sentinel")
}

// Store 必须满足 History。签名一旦漂移，这行在编译期就红。
var _ History = (*Store)(nil)

func TestHasPeriod(t *testing.T) {
	ctx := context.Background()

	t.Run("已入库返回 true", func(t *testing.T) {
		s := newTestStore(t)
		saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

		has, err := s.HasPeriod(ctx, "2025-12", "monthly")
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("未入库返回 false", func(t *testing.T) {
		s := newTestStore(t)
		has, err := s.HasPeriod(ctx, "2025-12", "monthly")
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("period_type 不同不算命中", func(t *testing.T) {
		s := newTestStore(t)
		saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

		has, err := s.HasPeriod(ctx, "2025-12", "annual")
		require.NoError(t, err)
		assert.False(t, has, "同一个 period 字符串下 monthly 与 annual 是两条独立序列")
	})

	t.Run("修订后仍然命中", func(t *testing.T) {
		s := newTestStore(t)
		saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

		_, err := s.Save(ctx, Observation{
			Meta: Meta{
				Period: "2025-12", PeriodType: "monthly", PublishedAt: "2026-02-20",
				ArticleID: "art-rev", CaliberVersion: "2025-01", Extractor: extractorV2,
			},
			Values: map[string]float64{FieldM2: 305},
		}, passing())
		require.NoError(t, err)

		has, err := s.HasPeriod(ctx, "2025-12", "monthly")
		require.NoError(t, err)
		assert.True(t, has)
	})
}

// pending 里的期次**不算已入库**。
//
// 这是刻意的：没过闸的一期该被重新发现、重新尝试。若 HasPeriod 也认 pending，
// 一次解析失败就会让那期**永远不再被抓** —— 而 pending 的设计意图恰恰是
// 「人看一眼再决定」，不是「就此丢弃」。
//
// ⚠️ 与 CONTRACTS.md 里「落 pending 的期次对依赖历史的闸门永久不可见」是同一张
// 表的两面：那里 pending 不可见是**缺陷**（基线冻结），这里不可见是**正确的**
// （允许重试）。区别在消费者要的东西不同 —— 闸门要「历史事实」，discover 要
// 「还需不需要抓」。
func TestHasPeriodIgnoresPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	out, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-11", PeriodType: "monthly", PublishedAt: "2025-12-15",
			ArticleID: "art-pending", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 300},
	}, failing())
	require.NoError(t, err)
	require.Equal(t, TablePending, out.Table, "前置条件：这一期必须落在 pending")

	has, err := s.HasPeriod(ctx, "2025-11", "monthly")
	require.NoError(t, err)
	assert.False(t, has, "pending 里的期次不算已入库，否则一次解析失败会让那期永远不再被抓")
}

// 查库真失败时必须返回 error，且**包住底层 err**并带 period/periodType 上下文。
//
// 计划的 Task 4 Step 1-7 没有覆盖这条（DoD error_handling[0] 要求），故本条自行补写。
// 触发方式与邻居 TestPrecedingWrapsQueryError 一致（已取消的 context 而不是关掉的库）：
// database/sql 在 Close 之后返回未导出的 errDBClosed，没有可用 sentinel，只能比错误串；
// context.Canceled 是标准 sentinel，能真的把 errors.Is 这条判据测出来。
//
// ⚠️ 「包住了」这一条**刻意不照抄邻居当时的写法**：邻居原本用的
// require.NotErrorIs(t, errors.Unwrap(err), err) 在不包裹时 Unwrap 返回 nil、
// errors.Is(nil, err) 恒 false ⇒ **平凡为真**（Sprint 035 的 F8 实测）。
// 这里用 require.NotNil(t, errors.Unwrap(err))，它在不包裹时会真的红。
//
// **邻居那处已于 M1b-4a / TASK-007 改成同一写法**，两处现在一致；这段说明保留，
// 因为它记录的是「为什么是这个写法」——F8 那种平凡为真不会在代码里留下痕迹，
// 删掉这段，下一个人很可能照着直觉又写回 NotErrorIs。
func TestHasPeriodWrapsQueryError(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消，QueryRowContext 必然失败

	has, err := s.HasPeriod(ctx, "2025-12", "monthly")
	require.Error(t, err, "查库失败必须返回 error")
	assert.False(t, has, "出错时不得报告「已入库」——否则一次查库故障会让那期被永久跳过")
	assert.ErrorIs(t, err, context.Canceled, "必须用 %w 包住底层 err，否则调用方无法分辨是取消还是真故障")
	assert.ErrorContains(t, err, "2025-12", "错误信息要带 period")
	assert.ErrorContains(t, err, "monthly", "错误信息要带 periodType")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}

// Store 必须满足 PeriodChecker。签名漂移在编译期就红。
var _ PeriodChecker = (*Store)(nil)

// —— M1b-4b / TASK-003：一级幂等键 Store.HasArticle ——
//
// Context Checkpoint: done_criteria → test mapping (has-article)
// functional[0]     HasArticle 同时查 TableObservations 与 TablePending，任一命中即 true（真库）
//                                          → TestHasArticle/观测表里的命中、TestHasArticle/pending_里的也命中
// functional[1]     三种情形各一条测试      → TestHasArticle 的三个子测试
//                   另加一条计划要求的：查全表而非当前行视图 → TestHasArticleSeesSupersededRows
// boundary[0]       空 articleID 的行为被钉住（false, nil，理由见该测试与 discovery）
//                                          → TestHasArticleOnEmptyID
// error_handling[0] 查库失败必须用 %w 包住底层 err 并带上下文
//                                          → TestHasArticleWrapsQueryError
// non_functional[0] 新增导出方法登记进两条导出面守卫
//                                          → TestStoreExposesNoWriteMethods（reflect 版）、
//                                            TestPackageExposesNoWriteFunctions（AST 版）

// 一级幂等键：这篇文章处理过没有？两张表都算。
//
// 「含 pending」是刻意的：落 pending 说明数据已抓到并跑过闸门，重抓必然得到
// 同样的结果，一天三次唤起是纯浪费。而真正需要重试的两种情形都不被它挡 ——
// 抓取/解析失败根本不写 pending 行；央行重发是**新的 article_id**。
//
// ⚠️ 与 TestHasPeriodIgnoresPending 恰好相反，而两条都对：HasPeriod 问的是
// 「这一期入库了没有」（pending 不算，否则一次解析失败让那期永远不再被抓），
// HasArticle 问的是「这篇文章处理过没有」（pending 算，重抓必然同样结果）。
// 两个问题的答案对同一行数据不同，是因为**消费者要的东西不同**，不是不一致。
func TestHasArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("观测表里的命中", func(t *testing.T) {
		s := newTestStore(t)
		saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

		has, err := s.HasArticle(ctx, "art-2025-12") // saveMonthly 用 "art-"+period
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("pending 里的也命中", func(t *testing.T) {
		s := newTestStore(t)
		out, err := s.Save(ctx, Observation{
			Meta: Meta{
				Period: "2025-11", PeriodType: "monthly", PublishedAt: "2025-12-15",
				ArticleID: "art-pending", CaliberVersion: "2025-01", Extractor: extractorV2,
			},
			Values: map[string]float64{FieldM2: 300},
		}, failing())
		require.NoError(t, err)
		require.Equal(t, TablePending, out.Table, "前置条件：这一期必须落 pending")

		has, err := s.HasArticle(ctx, "art-pending")
		require.NoError(t, err)
		assert.True(t, has, "落 pending 的文章也算处理过：重抓必然得到同样的结果")
	})

	t.Run("没见过的返回 false", func(t *testing.T) {
		s := newTestStore(t)
		saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

		has, err := s.HasArticle(ctx, "art-never-seen")
		require.NoError(t, err)
		assert.False(t, has)
	})
}

// 修订产生新行，但被取代的旧行同样是「处理过的文章」。
//
// 所以查 hestia_observations 全表而不是 v_hestia_current —— 视图只保留当前行，
// 用它会让旧 article_id 变成「没见过」，站点若把旧链接再挂出来就会重抓一遍。
//
// 这条是本任务里唯一能把「全表 vs 视图」区分开的断言：另外三个子测试在两种
// 实现下都绿（它们查的都是当前行）。
func TestHasArticleSeesSupersededRows(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	// 修订：同期次、更晚的 published_at、新的 article_id
	_, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-12", PeriodType: "monthly", PublishedAt: "2026-02-20",
			ArticleID: "art-2025-12-rev", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 305},
	}, passing())
	require.NoError(t, err)

	for _, id := range []string{"art-2025-12", "art-2025-12-rev"} {
		has, err := s.HasArticle(ctx, id)
		require.NoError(t, err)
		assert.True(t, has, "%s 应当算处理过", id)
	}
}

// 空 articleID 返回 (false, nil) —— 这是登记在案的决定，不是遗漏。
//
// 计划的实现没有空值分支，本条把那个行为钉住，理由有三：
//
//  1. **这个 false 是真话，不是平凡为真**：两张表的 article_id 都经 Meta.validate()
//     把关（Save 的第一步，pending 路径也走它），空串写不进任何一张表 —— 所以
//     「这个 ID 没处理过」确实是正确答案，不是「问题不合法所以随便答一个」。
//  2. **幂等门唯一危险的答案是错误的 true**：错 true 让那篇文章被永久跳过；
//     错 false 只多花一次抓取，而那正是幂等门 miss 的固有代价。守住 true 就够了。
//  3. **校验是 Save 的职责，不该在读方法里再来一遍**：validate() 刻意不导出
//     （见 types.go 那段注释），空 ArticleID 会在 Save 处以
//     "meta.article_id must not be empty" 精确报出来，不会被吞掉。在这里加一道
//     guard 会把同一份责任分到第二处，也与不校验入参的 HasPeriod/Preceding 不一致。
//
// 本条**不是**空断言：它先把两张表都填上真数据再问空串，所以实现若漏了
// `WHERE article_id = ?`（或写成 LIKE、写成恒真谓词），这里会立刻红。
func TestHasArticleOnEmptyID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})
	out, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-11", PeriodType: "monthly", PublishedAt: "2025-12-15",
			ArticleID: "art-pending", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 300},
	}, failing())
	require.NoError(t, err)
	require.Equal(t, TablePending, out.Table, "前置条件：两张表都得有行，空串才问得出东西")

	has, err := s.HasArticle(ctx, "")
	require.NoError(t, err, "空 ID 不是错误：它只是一个从未处理过的 ID")
	assert.False(t, has, "空串不得命中任何一张表里的任何一行")
}

// 查库真失败时必须返回 error，且**包住底层 err**并带 articleID 上下文。
//
// 触发方式与邻居 TestHasPeriodWrapsQueryError / TestPrecedingWrapsQueryError 一致
// （已取消的 context 而不是关掉的库）：database/sql 在 Close 之后返回未导出的
// errDBClosed，没有可用 sentinel，只能比错误串；context.Canceled 是标准 sentinel，
// 能真的把 errors.Is 这条判据测出来。
//
// ⚠️ 「包住了」这一条用 require.NotNil(t, errors.Unwrap(err))，**不是**
// require.NotErrorIs(t, errors.Unwrap(err), err) —— 后者在不包裹时 Unwrap 返回 nil、
// errors.Is(nil, err) 恒 false ⇒ **平凡为真**（Sprint 035 的 F8，跨 Sprint 存活了
// 一整轮才被修）。也不能只靠上面那条 assert.ErrorIs：实现写成 `return false, err`
// （完全不包）时它同样为真，NotNil 才把那个差集测掉。
func TestHasArticleWrapsQueryError(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消，QueryRowContext 必然失败

	has, err := s.HasArticle(ctx, "art-2025-12")
	require.Error(t, err, "查库失败必须返回 error")
	assert.False(t, has, "出错时不得报告「处理过」——否则一次查库故障会让那篇文章被永久跳过")
	assert.ErrorIs(t, err, context.Canceled, "必须用 %w 包住底层 err，否则调用方无法分辨是取消还是真故障")
	assert.ErrorContains(t, err, "art-2025-12", "错误信息要带 articleID")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}

// —— M1b-4b / TASK-006：status 的两个只读查询 ——
//
// Context Checkpoint: done_criteria → test mapping (status queries)
// functional[0]     RecentObservations / RecentPending 真库测试
//                                          → TestRecentObservations、TestRecentPendingExtractsFailedChecks
// functional[1]     RecentObservations 查 viewCurrent（不是 TableObservations），与 HasPeriod 同口径
//                                          → TestRecentObservationsShowsCurrentOnly
//                   RecentPending 查 TablePending 且带上失败的 []Check
//                                          → TestRecentPendingExtractsFailedChecks
// boundary[0]       空库上两个查询都跑通、返回空且不报错
//                                          → TestRecentQueriesOnEmptyStore
// boundary[1]       n 为 0 与负数的行为各一条，且**判据不对称**
//                                          → TestRecentObservationsLimitZeroVsNegative、
//                                            TestRecentPendingLimitZeroVsNegative
// error_handling[0] 查库失败 %w 包住并带上下文
//                                          → TestRecentObservationsWrapsQueryError、
//                                            TestRecentPendingWrapsQueryError、
//                                            TestRecentPendingWrapsBadReportJSON
// non_functional[1] 新增导出方法登记进两条导出面守卫
//                                          → TestStoreExposesNoWriteMethods（reflect 版）、
//                                            TestPackageExposesNoWriteFunctions（AST 版）

func TestRecentObservations(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, p := range []string{"2025-10", "2025-11", "2025-12"} {
		saveMonthly(t, s, p, map[string]float64{FieldM2: 300})
	}

	got, err := s.RecentObservations(ctx, 2)
	require.NoError(t, err)
	require.Len(t, got, 2, "LIMIT 要生效")
	assert.Equal(t, "2025-12", got[0].Period, "最近的排最前")
	assert.Equal(t, "2025-11", got[1].Period)
	assert.Equal(t, "monthly", got[0].PeriodType)
	assert.Equal(t, extractorV2, got[0].Extractor)
}

// 只列当前行：修订后应看到新值那一行，不是两行。
//
// 这条是本任务里唯一能把「viewCurrent vs TableObservations」区分开的断言 ——
// 其余用例在两种实现下都绿（没有修订就没有被取代的旧行）。查错了表不会报错，
// 只会让 status 把同一期显示两遍，而运维读到的是「数据重复了」这个错误结论。
func TestRecentObservationsShowsCurrentOnly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})
	_, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-12", PeriodType: "monthly", PublishedAt: "2026-02-20",
			ArticleID: "art-rev", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 305},
	}, passing())
	require.NoError(t, err)

	got, err := s.RecentObservations(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1, "修订不产生第二期")
	assert.Equal(t, "2026-02-20", got[0].PublishedAt, "应是修订后的那一行")
}

// pending 的失败闸门要解出来 —— 不解就等于让人去 sqlite 里读 JSON，
// 而那正是 status 要消灭的事（pending 表自述「它的消费者只有人」，schema.go:13）。
func TestRecentPendingExtractsFailedChecks(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	out, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-11", PeriodType: "monthly", PublishedAt: "2025-12-15",
			ArticleID: "art-bad", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 300},
	}, failing())
	require.NoError(t, err)
	require.Equal(t, TablePending, out.Table)

	got, err := s.RecentPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "2025-11", got[0].Period)
	assert.NotEmpty(t, got[0].IngestedAt)
	require.NotEmpty(t, got[0].Failed, "至少要解出一条 failed 检查")
	assert.Equal(t, CheckFailed, got[0].Failed[0].Status)
}

// 只解出 failed 的那些，passed/skipped 不进 Failed。
//
// 与上一条互补：上一条只断言「至少解出一条」，在「把 Checks 原样抄进 Failed」
// 的实现下同样为真。本条喂一份混合报告，把「筛选确实发生了」钉死。
func TestRecentPendingKeepsOnlyFailedChecks(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mixed := ValidationReport{Passed: false, Checks: []Check{
		{ID: "monetary_hierarchy", Status: CheckPassed},
		{ID: "completeness", Status: CheckFailed, Reason: "absent_field:m2"},
		{ID: "magnitude_sanity", Status: CheckSkipped, Reason: "not_calibrated"},
	}}
	_, err := s.Save(ctx, Observation{
		Meta: Meta{
			Period: "2025-11", PeriodType: "monthly", PublishedAt: "2025-12-15",
			ArticleID: "art-mixed", CaliberVersion: "2025-01", Extractor: extractorV2,
		},
		Values: map[string]float64{FieldM2: 300},
	}, mixed)
	require.NoError(t, err)

	got, err := s.RecentPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Failed, 1, "三条里只有一条 failed，passed/skipped 不得混进来")
	assert.Equal(t, "completeness", got[0].Failed[0].ID)
	assert.Equal(t, "absent_field:m2", got[0].Failed[0].Reason, "理由要带出来，否则还是得开 sqlite")
}

// 空库两个查询都要跑通：返回空、不报错、不 panic。
//
// DoD boundary[0]：**配置装载错与库路径错的表现都是「0 期」**，空库 status 是
// 唯一能把它们分开的东西 —— 它必须真的跑到底，而不是在空结果集上炸掉。
func TestRecentQueriesOnEmptyStore(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	obs, err := s.RecentObservations(ctx, 10)
	require.NoError(t, err, "空库不是错误")
	assert.Empty(t, obs)

	pending, err := s.RecentPending(ctx, 10)
	require.NoError(t, err, "空库不是错误")
	assert.Empty(t, pending)
}

// 🔴 n ≤ 0 一律返回空 —— 0 与 -1 **都**要测，因为底层不对称。
//
// SQLite 的 `LIMIT 0` 是零行、**`LIMIT -1` 是不限行数**。把 n 直接透传给 LIMIT
// 时两者语义相反，一个「防御性地传 -1」的调用方会拿到**整张表**（reviewer D5）。
// 只测 0 的话，未加守卫的实现照样全绿 —— 这正是「别只测 0」的原因。
//
// 本方法挡住 n <= 0，与 Preceding 同约定（store.go:303 已经这么做）。同一个 Store
// 上两个读方法对 n <= 0 给出相反语义本身就是缺陷，所以这里不是加新规矩，是跟上
// 既有的那条。
//
// ⚠️ 「两者都返回空」这个断言**在守卫被删掉时不足以全部转红**：n=0 在无守卫下
// 也是空。所以下面第三段用**裸 SQL 走同一个库**跑一次未加守卫的查询，证明
// 「若不挡，-1 确实会拿到整张表」—— 守卫因此是**承重的**，而不是一句空话。
func TestRecentObservationsGuardsNonPositiveN(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, p := range []string{"2025-10", "2025-11", "2025-12"} {
		saveMonthly(t, s, p, map[string]float64{FieldM2: 300})
	}

	zero, err := s.RecentObservations(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, zero, "n=0 返回空")

	neg, err := s.RecentObservations(ctx, -1)
	require.NoError(t, err)
	assert.Empty(t, neg, "n=-1 也返回空 —— 不得漏给 LIMIT 变成「整张表」")

	// 守卫承重性自证：同一个库上直接跑未加守卫的那条 SQL。
	assert.Equal(t, 3, rawLimitCount(t, s, viewCurrent, -1),
		"前提：裸 LIMIT -1 确实返回整张表(3 行)，故上面 n=-1 的空结果是守卫挡出来的，不是库本来就空")
	assert.Equal(t, 0, rawLimitCount(t, s, viewCurrent, 0),
		"对照：裸 LIMIT 0 本就是零行 —— 这说明只测 n=0 分辨不出守卫在不在")
}

// 同上，pending 侧独立钉一遍：两个查询各写各的 SQL，一处改对不代表另一处也对。
func TestRecentPendingGuardsNonPositiveN(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, p := range []string{"2025-10", "2025-11"} {
		_, err := s.Save(ctx, Observation{
			Meta: Meta{
				Period: p, PeriodType: "monthly", PublishedAt: p + "-15",
				ArticleID: "art-p-" + p, CaliberVersion: "2025-01", Extractor: extractorV2,
			},
			Values: map[string]float64{FieldM2: 300},
		}, failing())
		require.NoError(t, err)
	}

	zero, err := s.RecentPending(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, zero, "n=0 返回空")

	neg, err := s.RecentPending(ctx, -1)
	require.NoError(t, err)
	assert.Empty(t, neg, "n=-1 也返回空 —— 不得漏给 LIMIT 变成「整张表」")

	assert.Equal(t, 2, rawLimitCount(t, s, TablePending, -1),
		"前提：裸 LIMIT -1 确实返回整张表(2 行)，故上面 n=-1 的空结果是守卫挡出来的")
	assert.Equal(t, 0, rawLimitCount(t, s, TablePending, 0),
		"对照：裸 LIMIT 0 本就是零行")
}

// rawLimitCount 绕开 Store 的守卫，直接对 table 跑 `LIMIT ?` 数行数。
//
// 用 DB()（只读 Querier）而不是 rawDB：这里要的正是「Store 自己那条连接上、
// 同一份数据、唯一区别是没有守卫」，换一条连接会多引入一个变量。
func rawLimitCount(t *testing.T, s *Store, table string, n int) int {
	t.Helper()
	rows, err := s.DB().QueryContext(context.Background(),
		`SELECT period FROM `+table+` ORDER BY period DESC LIMIT ?`, n)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var count int
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	return count
}

// 触发方式与邻居 TestPrecedingWrapsQueryError / TestHasArticleWrapsQueryError 一致
// （已取消的 context 而不是关掉的库）：database/sql 在 Close 之后返回未导出的
// errDBClosed，没有可用 sentinel，只能比错误串；context.Canceled 是标准 sentinel。
//
// ⚠️ 「包住了」用 require.NotNil(t, errors.Unwrap(err))，**不是** assert.ErrorIs ——
// 实现写成 `return nil, err`（完全不包）时 ErrorIs 同样为真，NotNil 才把那个差集测掉。
func TestRecentObservationsWrapsQueryError(t *testing.T) {
	s := newTestStore(t)
	saveMonthly(t, s, "2025-12", map[string]float64{FieldM2: 300})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消，QueryContext 必然失败

	got, err := s.RecentObservations(ctx, 10)
	require.Error(t, err, "查库失败必须返回 error")
	assert.Nil(t, got, "出错时不得返回半截结果")
	assert.ErrorIs(t, err, context.Canceled, "必须用 %w 包住底层 err")
	assert.ErrorContains(t, err, "recent observations", "错误要带上下文，否则运维不知是哪条查询")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}

func TestRecentPendingWrapsQueryError(t *testing.T) {
	s := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := s.RecentPending(ctx, 10)
	require.Error(t, err, "查库失败必须返回 error")
	assert.Nil(t, got, "出错时不得返回半截结果")
	assert.ErrorIs(t, err, context.Canceled, "必须用 %w 包住底层 err")
	assert.ErrorContains(t, err, "recent pending", "错误要带上下文，否则运维不知是哪条查询")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}

// report 列存的是坏 JSON 时要报错并带上是**哪一期**，而不是静默给一个空的 Failed。
//
// 这是 RecentPending 的第二条错误出口（另一条是查库本身失败），走的是 Unmarshal。
// 静默吞掉它的后果特别隐蔽：status 会显示「这期 pending，但没有任何失败检查」——
// 看起来像闸门没记录理由，实际是报告存坏了。
//
// 用裸连接直接写行：Save 走的是 json.Marshal，产不出坏 JSON。
func TestRecentPendingWrapsBadReportJSON(t *testing.T) {
	ctx := context.Background()
	s, path := newTestStoreAt(t)

	db := rawDB(t, path)
	_, err := db.ExecContext(ctx,
		`INSERT INTO `+TablePending+
			` (period, period_type, published_at, article_id, extractor, ingested_at, report, values_json)
		  VALUES ('2025-11','monthly','2025-12-15','art-x','rule@v2','2025-12-15T15:30:04Z','{ 不是 json','{}')`)
	require.NoError(t, err, "前置条件：坏 JSON 必须真的写进去了")

	got, err := s.RecentPending(ctx, 10)
	require.Error(t, err, "坏 report JSON 必须报错，不得静默当成「没有失败检查」")
	assert.Nil(t, got, "出错时不得返回半截结果")
	assert.ErrorContains(t, err, "2025-11", "错误要带上是哪一期，否则无从下手")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}

// Scan 失败也要被 %w 包住并带上下文 —— 这是 RecentPending 的第三条错误出口。
//
// 触发方式：用裸连接把 pending 表换成没有 NOT NULL 约束的同名表，再写一行 NULL
// period。database/sql 把 NULL 扫进 string 会失败（converting NULL to string is
// unsupported），走的正是 rows.Scan 那条分支。
//
// 这正是 rawDB 注释里说的用途——「制造 Store 管不到的库状态：加列、直接写 NaN、
// 把表改成结构不符的样子」。生产路径造不出这种行（Save 的七个元数据列都 NOT NULL），
// 但库文件是外部对象，Grafana、手工 sqlite3、旧版本代码都可能留下这种行，
// 而 status 是运维在库出问题时第一个会跑的命令：它必须报错，不能崩也不能装没事。
func TestRecentPendingWrapsScanError(t *testing.T) {
	ctx := context.Background()
	s, path := newTestStoreAt(t)

	db := rawDB(t, path)
	_, err := db.ExecContext(ctx, `DROP TABLE `+TablePending)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE `+TablePending+` (
		period TEXT, period_type TEXT, published_at TEXT, article_id TEXT,
		extractor TEXT, ingested_at TEXT, report TEXT, values_json TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO `+TablePending+
		` (period, period_type, published_at, ingested_at, report)
		  VALUES (NULL, 'monthly', '2025-12-15', '2025-12-15T15:30:04Z', '{}')`)
	require.NoError(t, err, "前置条件：NULL period 那一行必须真的写进去了")

	got, err := s.RecentPending(ctx, 10)
	require.Error(t, err, "Scan 失败必须报错，不得静默跳过那一行")
	assert.Nil(t, got, "出错时不得返回半截结果")
	assert.ErrorContains(t, err, "recent pending", "错误要带上下文")
	require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")
}
