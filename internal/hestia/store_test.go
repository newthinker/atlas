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
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
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
// Save 在 TASK-005 引入，届时本条会转红——那是**刻意的**：新增任何导出方法都必须
// 在这里显式登记一次，让「又开了一个写口」成为一个需要动手改测试的决定，而不是
// 顺手加个方法就完成了。ADR-0003 在同机场景下唯一的防线就是 Save 的签名。
func TestStoreExposesNoWriteMethods(t *testing.T) {
	typ := reflect.TypeOf(&Store{})
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	assert.Equal(t, []string{"Close", "DB"}, got,
		"本任务只应导出 Close 与 DB；出现 Insert/Upsert 等写口即违反单一写入口约束")
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
