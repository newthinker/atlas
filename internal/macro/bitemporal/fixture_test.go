package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-003 fixture)
// functional[0]     hestiaShape/crisisShape/singleKeyShape 三套形状，bothShapes 供并行跑 → TestFixturesAreUsable
// functional[1]     newDB 用 t.TempDir() + crisis 惯例 DSN，每用例独立的库           → TestEachCallGetsAnIndependentDB
// functional[2]     T1 探针数据固化：共享恰好一列 + 共享方 revision 更大 + 具名常量   → TestProbeKeepsT1MutationObservable
// functional[2]     探针声明的「当前行」与实际插入的数据一致                          → TestProbeCurrentMatchesInsertedData
// functional[2]     探针判别力的直接证据：AND→OR 后查询结果必须改变                  → TestProbeDistinguishesAndFromOr
// boundary[0]       契约 T3：失败信息必须指出是哪套形状                               → TestFailureMessagesNameTheShape
// boundary[1]       insert 支持乱序插入（revision 不按插入顺序递增）                  → TestInsertSupportsOutOfOrderRevisions
// error_handling[0] fixture 自身失败立即中止，不让下游在残缺的库上继续跑              → TestFixtureFailuresAbortImmediately
//
// 本任务只产出 _test.go，不增加包的公开面（non_functional[1]）。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestFixturesAreUsable(t *testing.T) {
	// 形状集合本身是 functional[0] 的承诺：下游靠 bothShapes 并行跑两套，
	// 并靠形状名区分子测试——重名会让 t.Run 加数字后缀，失败信息随之失去指向。
	shapes := allShapes(t)
	require.Len(t, bothShapes(t), 2, "bothShapes 必须恰好两套主形状")
	require.Len(t, shapes, 3, "allShapes 是两套主形状加单列键形状")
	names := map[string]bool{}
	for _, f := range shapes {
		require.False(t, names[f.name], "形状名必须互不相同：%s", f.name)
		names[f.name] = true
	}

	for _, f := range shapes {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			f.insert(t, db, revEarly, 1.5, f.probe[0].keyVals...)

			var n int
			require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+f.spec.table).Scan(&n))
			require.Equal(t, 1, n)
		})
	}
}

// TestEachCallGetsAnIndependentDB 钉住「每个测试用例独立的库」。
// 跨用例共享状态会让测试的通过与否取决于执行顺序——那种失败极难定位。
func TestEachCallGetsAnIndependentDB(t *testing.T) {
	f := hestiaShape(t)

	db1 := newDB(t, f)
	f.insert(t, db1, revEarly, 1.0, "2026-06", "h1")

	db2 := newDB(t, f)
	var n int
	require.NoError(t, db2.QueryRow("SELECT COUNT(*) FROM "+f.spec.table).Scan(&n))
	assert.Equal(t, 0, n, "第二个库不应看到第一个库写入的行")

	// DSN 沿用 crisis 惯例。这里验 pragma 的实际生效值而不是比对 DSN 字符串——
	// DSN 拼对了但 driver 没认（参数名写错、驱动不支持）从字符串上看不出来。
	var journalMode string
	require.NoError(t, db2.QueryRow("PRAGMA journal_mode").Scan(&journalMode))
	assert.Equal(t, "wal", strings.ToLower(journalMode), "DSN 的 journal_mode(WAL) 须实际生效")

	var busyTimeout int
	require.NoError(t, db2.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout))
	assert.Equal(t, 5000, busyTimeout, "DSN 的 busy_timeout(5000) 须实际生效")
}

// TestProbeKeepsT1MutationObservable 是本文件存在的主要理由。
//
// TASK-004 的契约 T1 要靠「改 correlate 的 AND 为 OR，真 SQL 测试须转红」来证明
// Query 真的用了 correlate 的输出。这个变异能否被观测，完全取决于探针数据的形状——
// 需求文档原本的数据（两列都不同）下 AND 与 OR 输出逐字节相同，变异打不中，
// 缺口是独立 reviewer 事后才发现的。所以这里把前提**结构性地钉住**，而不是
// 依赖下游实现者记得摆对数据：
//
//	① 存在一对业务键共享【恰好一列】；
//	② 该对的两个键，其最大 revision 必须【不相等】——OR 变异把彼此拉进对方的
//	   MAX 子查询，较大的一方会淹没较小的一方使其整组落选，从而可观测；
//	   两者相等时谁也淹没不了谁，OR 与 AND 输出相同，变异打不中。
//	   **方向无关**：谁大谁小都可观测，只有相等才不行。（DoD 的表述是「共享方
//	   revision 必须更大」，实测比这更宽——把共享方改小仍可观测，改成相等才转红。
//	   下面用 sharedColumnPair 排序后断言严格大于，等价于断言两者不相等。）
//
// ⚠ singleKeyShape 只有一列业务键，correlate 根本不产出 AND，T1 对它**结构性
// 打不中**。因此本测试只对 bothShapes 跑——「三套形状都跑了」不等于「T1 在三套
// 形状上都有守护」。
func TestProbeKeepsT1MutationObservable(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			require.Greater(t, len(f.keys), 1, "bothShapes 的形状必须是多列键，否则无 AND 可变")

			a, b, ok := f.sharedColumnPair()
			require.True(t, ok, "探针数据必须包含一对共享【恰好一列】的业务键")

			assert.Equal(t, len(f.keys)-1, countSharedColumns(a, b),
				"必须共享恰好一列，不能全同也不能全异")
			// sharedColumnPair 已把较大者排到 b，所以这条等价于「两者不相等」
			assert.Greater(t, f.maxRevisionOf(b), f.maxRevisionOf(a),
				"共享一列的两个键，最大 revision 不得相等，否则 AND→OR 变异不可观测")
		})
	}
}

// TestProbeCurrentMatchesInsertedData 校验 fixture 声明的「当前行」确实对应
// 它插入的数据。期望值是硬编码的（人可读、与被测实现无关），这里用一段与
// CurrentQuery 无关的 Go 逻辑——读全表、按完整业务键分组取 revision 最大者——
// 作为独立参照交叉验证，避免声明与数据悄悄失配。
//
// 本测试不验证 CurrentQuery（那是 TASK-004 的事），只验证基建自身自洽。
func TestProbeCurrentMatchesInsertedData(t *testing.T) {
	for _, f := range allShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			// functional[2]③：revision 一律取自具名常量。下游要用这些常量构造
			// 「as-of 恰好等于某个 revision」的边界，探针里混入裸字面量就会静默失配。
			for _, r := range f.probe {
				assert.Contains(t, []string{revEarly, revMiddle, revLatest}, r.revision,
					"探针 revision 必须是具名常量之一")
			}

			db := newDB(t, f)
			want := f.seedProbe(t, db)

			latest := map[string]string{} // 完整键 → 目前见过的最大 revision
			got := map[string]float64{}
			for _, r := range f.readAll(t, db) {
				k := f.keyString(r.keyVals...)
				if prev, seen := latest[k]; !seen || r.revision > prev {
					latest[k] = r.revision
					got[k] = r.value
				}
			}
			assert.Equal(t, want, got, "声明的当前行与实际插入的数据不一致")
		})
	}
}

// TestFailureMessagesNameTheShape 是契约 T3：两套形状共用断言时，红了必须能
// 一眼看出是哪套的问题。这里不制造真失败（那会让本测试变红），而是检查
// fixture 生成失败消息的那个函数确实带上了形状名。
func TestFailureMessagesNameTheShape(t *testing.T) {
	for _, f := range allShapes(t) {
		assert.Contains(t, f.failureContext("建表"), f.name, "失败消息必须指明形状名")
	}
	// 两套形状的消息必须彼此可区分，否则「带了名字」也没用
	assert.NotEqual(t, hestiaShape(t).failureContext("建表"), crisisShape(t).failureContext("建表"))
}

// TestInsertSupportsOutOfOrderRevisions 钉住 insert 不对 revision 顺序做任何
// 假设。回填历史 80 期时不保证按时间顺序写入，TASK-004/005 的乱序边界依赖这一点。
func TestInsertSupportsOutOfOrderRevisions(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			// seedProbe 承诺「插入顺序不按 revision 递增」，好让每个用到它的下游
			// 测试都顺带覆盖乱序。这条承诺只存在于探针数据的排列里，很容易在
			// 后人整理数据时被无声地破坏，所以在这里钉住。
			ascending := true
			for i := 1; i < len(f.probe); i++ {
				if f.probe[i].revision < f.probe[i-1].revision {
					ascending = false
					break
				}
			}
			assert.False(t, ascending, "探针的插入顺序必须是乱序，否则 seedProbe 不覆盖乱序写入")

			db := newDB(t, f)
			k := f.probe[0].keyVals
			// 先写晚的，再写早的
			f.insert(t, db, revLatest, 3.0, k...)
			f.insert(t, db, revEarly, 1.0, k...)
			f.insert(t, db, revMiddle, 2.0, k...)

			var maxRev string
			require.NoError(t, db.QueryRow(
				"SELECT MAX("+f.spec.revisionCol+") FROM "+f.spec.table).Scan(&maxRev))
			assert.Equal(t, revLatest, maxRev, "最大 revision 与插入顺序无关")
		})
	}
}

// TestProbeDistinguishesAndFromOr 是探针数据判别力的【直接证据】。
//
// TestProbeKeepsT1MutationObservable 断言的是两个代理指标（共享恰好一列、
// 共享方 revision 更大）——那是「我认为的必要条件」，不是「变异真的会被观测到」。
// 这里直接把 correlate 的真实输出做 AND→OR 替换，在探针数据上跑两版 SQL 比对
// 结果：两者必须不同。代理指标若哪天推导错了，这条仍然会红。
//
// 本测试不碰 CurrentQuery（那是 TASK-004 的实现，此刻尚不存在），只用与它
// 同构的 SQL 证明「这组数据能区分 AND 与 OR」——这正是 fixture 对下游的承诺。
func TestProbeDistinguishesAndFromOr(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			want := f.seedProbe(t, db)

			correct := f.spec.correlate("o")
			mutated := strings.ReplaceAll(correct, " AND ", " OR ")
			require.NotEqual(t, correct, mutated, "多列键的 correlate 必须含 AND，否则无从变异")

			assert.Equal(t, want, f.currentRowsWith(t, db, correct),
				"探针声明的当前行必须就是 MAX(revision) 语义下的答案——下游拿它当期望值")
			assert.NotEqual(t, want, f.currentRowsWith(t, db, mutated),
				"AND→OR 后结果必须改变，否则 TASK-004 的 T1 变异打不中这组数据")
		})
	}
}

// TestFixtureFailuresAbortImmediately 钉住 error_handling[0]：fixture 内部出错
// 必须立即中止当前测试，而不是返回一个残缺的库让下游测试在上面继续跑——那样
// 报出来的错误会指向别处，排查方向从一开始就是错的。
//
// 两条证据：
//
//	① 类型层面：fixture 的 helper 都不返回 error，调用方没有「忽略错误继续跑」
//	   这个选项；
//	② 源码层面：helper 区段内一律用 require（FailNow）而非 assert（继续执行）。
//	   ② 是真正的判据——①只说明错误不能被忽略，不说明它会中止。
//
// 用静态检查而非制造真失败，是因为 FailNow 无法在同一个测试内被拦截观测。
func TestFixtureFailuresAbortImmediately(t *testing.T) {
	var _ func(*testing.T, *sql.DB, string, float64, ...string) = hestiaShape(t).insert
	var _ func(*testing.T, fixture) *sql.DB = newDB
	var _ func(*testing.T, *sql.DB) map[string]float64 = hestiaShape(t).seedProbe

	src, err := os.ReadFile("fixture_test.go")
	require.NoError(t, err)
	_, helpers, found := strings.Cut(string(src), helperSectionMarker)
	require.True(t, found, "helper 区段标记不存在，静态检查失效")
	assert.NotContains(t, helpers, "assert.",
		"fixture helper 区段内不得使用 assert——失败必须 FailNow 立即中止")
}

// helperSectionMarker 分隔「测试用例」与「fixture helper」两个区段。
// 上面的静态检查依赖它界定范围。
const helperSectionMarker = "// ===== fixture helpers ====="

// ===== fixture helpers =====
//
// 测试基建：两套形状不同的双时态表。
//
// 本包唯一的价值主张是「同一套机制服务两种形状的键」。只用一套测，这个主张
// 就没有证据——而 M4 迁移 crisis 时才发现抽象错了，那时已经迁到一半。
// 因此所有涉及数据库的测试都对 bothShapes 并行跑。

// revision 取值以具名常量暴露，供下游构造「as-of 恰好等于某个 revision」这类
// 包含性边界（TASK-004 的 `<=` vs `<`）。下游若靠字面量重复书写这些值，
// 探针数据一改就会静默失配。
//
// 三个值的字典序即时间序：revEarly < revMiddle < revLatest。
const (
	revEarly  = "2026-07-15"
	revMiddle = "2026-08-20"
	revLatest = "2026-12-31"
)

// probeRow 是探针数据里的一行。
type probeRow struct {
	keyVals  []string
	revision string
	value    float64
}

type fixture struct {
	name     string
	spec     Spec
	ddl      string
	ins      string   // 占位符顺序：业务键各列…, revision, value
	keys     []string // 业务键列名，与 spec 一致
	valueCol string   // 占位业务列，用于验证查询取到了正确的行

	// probe 是固化的探针数据，probeCurrent 是它对应的「当前行」（完整键 → value）。
	// probeCurrent 硬编码而非由 probe 算出：期望值应当人可读、且不与任何被测
	// 逻辑共用算法，否则两边可能同时错。TestProbeCurrentMatchesInsertedData
	// 用一段独立的 Go 逻辑交叉验证两者自洽。
	probe        []probeRow
	probeCurrent map[string]float64
}

// key 按 f.keys 的顺序把取值组装成 Key。
func (f fixture) key(vals ...string) Key {
	k := make(Key, len(f.keys))
	for i, c := range f.keys {
		k[c] = vals[i]
	}
	return k
}

// keyString 把一组业务键取值拼成稳定的 map 键。
//
// 下游断言「每个业务键的当前行」时必须以【完整】业务键作 map 键：探针里
// A 与 B 共享第一列，只用第一列作键会让两者互相覆盖——数据补对了，断言却
// 分辨不出来。「补了数据」和「断言能分辨这些数据」是两件事。
func (f fixture) keyString(vals ...string) string { return strings.Join(vals, "/") }

// failureContext 生成带形状名的失败上下文（契约 T3）。
// 两套形状共用同一组断言，失败信息不带形状名就无法定位是哪套出的问题。
func (f fixture) failureContext(op string) string {
	return fmt.Sprintf("形状 %s（表 %s）%s失败", f.name, f.spec.table, op)
}

// insert 写一行。不返回 error：失败即 FailNow，调用方没有忽略它继续跑的选项。
func (f fixture) insert(t *testing.T, db *sql.DB, revision string, value float64, keyVals ...string) {
	t.Helper()
	require.Len(t, keyVals, len(f.keys), f.failureContext("插入")+"：keyVals 数量与业务键列数不符")
	args := make([]any, 0, len(keyVals)+2)
	for _, v := range keyVals {
		args = append(args, v)
	}
	args = append(args, revision, value)
	_, err := db.Exec(f.ins, args...)
	require.NoError(t, err, f.failureContext("插入"))
}

// seedProbe 插入固化的探针数据，返回期望的「完整键 → 当前 value」映射。
//
// 插入顺序刻意不按 revision 递增（探针第一行就是全表最大的 revision），
// 这样每个用到 seedProbe 的下游测试都顺带覆盖了乱序写入。
func (f fixture) seedProbe(t *testing.T, db *sql.DB) map[string]float64 {
	t.Helper()
	for _, r := range f.probe {
		f.insert(t, db, r.revision, r.value, r.keyVals...)
	}
	want := make(map[string]float64, len(f.probeCurrent))
	for k, v := range f.probeCurrent {
		want[k] = v
	}
	return want
}

// readAll 读回全表，供基建自检做交叉验证。
func (f fixture) readAll(t *testing.T, db *sql.DB) []probeRow {
	t.Helper()
	cols := append(append([]string{}, f.keys...), f.spec.revisionCol, f.valueCol)
	rows, err := db.Query("SELECT " + strings.Join(cols, ", ") + " FROM " + f.spec.table)
	require.NoError(t, err, f.failureContext("读取"))
	defer rows.Close()

	var out []probeRow
	for rows.Next() {
		r := probeRow{keyVals: make([]string, len(f.keys))}
		dest := append(scanTargets(r.keyVals), &r.revision, &r.value)
		require.NoError(t, rows.Scan(dest...), f.failureContext("读取"))
		out = append(out, r)
	}
	require.NoError(t, rows.Err(), f.failureContext("读取"))
	return out
}

// currentRowsWith 用调用方给定的关联条件跑「当前行」查询，返回完整键 → value。
//
// 关联条件做成参数，是为了能在同一组数据上跑正确版与变异版（AND→OR）作对比——
// 这是探针判别力的直接证据。本 helper 与 TASK-004 的 CurrentQuery 无依赖关系，
// 只是同构：它属于基建的自证，不替下游验实现。
func (f fixture) currentRowsWith(t *testing.T, db *sql.DB, correlation string) map[string]float64 {
	t.Helper()
	cols := append(append([]string{}, f.keys...), f.valueCol)
	rows, err := db.Query(fmt.Sprintf(
		"SELECT %s FROM %s o WHERE o.%s = (SELECT MAX(%s) FROM %s WHERE %s)",
		strings.Join(cols, ", "), f.spec.table,
		f.spec.revisionCol, f.spec.revisionCol, f.spec.table, correlation))
	require.NoError(t, err, f.failureContext("查询当前行"))
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		keyVals := make([]string, len(f.keys))
		var val float64
		dest := append(scanTargets(keyVals), &val)
		require.NoError(t, rows.Scan(dest...), f.failureContext("查询当前行"))
		out[f.keyString(keyVals...)] = val
	}
	require.NoError(t, rows.Err(), f.failureContext("查询当前行"))
	return out
}

// scanTargets 返回 vals 各元素的地址，供 rows.Scan 填充业务键列。
// 业务键列数随形状而变，两处扫描都得先把切片元素逐个取址。
func scanTargets(vals []string) []any {
	dest := make([]any, len(vals))
	for i := range vals {
		dest[i] = &vals[i]
	}
	return dest
}

// maxRevisionOf 返回探针中该业务键的最大 revision。
func (f fixture) maxRevisionOf(keyVals []string) string {
	want := f.keyString(keyVals...)
	maxRev := ""
	for _, r := range f.probe {
		if f.keyString(r.keyVals...) == want && r.revision > maxRev {
			maxRev = r.revision
		}
	}
	return maxRev
}

// sharedColumnPair 从探针里找出一对共享【恰好一列】的业务键，
// 返回时 b 是最大 revision 较大的那个（T1 变异的可观测性取决于这个方向）。
func (f fixture) sharedColumnPair() (a, b []string, ok bool) {
	var distinct [][]string
	seen := map[string]bool{}
	for _, r := range f.probe {
		if k := f.keyString(r.keyVals...); !seen[k] {
			seen[k] = true
			distinct = append(distinct, r.keyVals)
		}
	}
	for i := range distinct {
		for j := i + 1; j < len(distinct); j++ {
			if countSharedColumns(distinct[i], distinct[j]) != len(f.keys)-1 {
				continue
			}
			a, b = distinct[i], distinct[j]
			if f.maxRevisionOf(a) > f.maxRevisionOf(b) {
				a, b = b, a
			}
			return a, b, true
		}
	}
	return nil, nil, false
}

// countSharedColumns 返回两个业务键逐列相同的列数。
func countSharedColumns(a, b []string) int {
	n := 0
	for i := range a {
		if i < len(b) && a[i] == b[i] {
			n++
		}
	}
	return n
}

// newDB 建一个临时库并建好 f 的表。DSN 沿用 crisis 的约定。
// 每次调用都是全新的 t.TempDir()，因此用例之间不共享任何状态。
func newDB(t *testing.T, f fixture) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err, f.failureContext("打开数据库"))
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(f.ddl)
	require.NoError(t, err, f.failureContext("建表"))
	return db
}

// hestiaShape 是 Hestia 的形状：两列业务键 + published_at。
//
// 探针中 ("2026-06","h1") 与 ("2026-06","monthly") 共享 period 一列，
// 且后者的 revision（revLatest）大于前者的最大 revision（revMiddle）——
// 这是 TASK-004 的 T1 变异可被观测的必要条件，见
// TestProbeKeepsT1MutationObservable。
func hestiaShape(t *testing.T) fixture {
	t.Helper()
	s, err := NewSpec("hestia_observations", []string{"period", "period_type"}, "published_at")
	require.NoError(t, err)
	return fixture{
		name:     "hestia",
		spec:     s,
		keys:     []string{"period", "period_type"},
		valueCol: "m2",
		ddl: `CREATE TABLE hestia_observations (
			period       TEXT NOT NULL,
			period_type  TEXT NOT NULL,
			published_at TEXT NOT NULL,
			m2           REAL,
			PRIMARY KEY (period, period_type, published_at))`,
		ins: `INSERT INTO hestia_observations (period, period_type, published_at, m2) VALUES (?,?,?,?)`,
		probe: []probeRow{
			{[]string{"2026-06", "monthly"}, revLatest, 7.0}, // 与下一个键共享 period，revision 最大
			{[]string{"2026-06", "h1"}, revMiddle, 2.0},
			{[]string{"2026-05", "monthly"}, revEarly, 9.0},
			{[]string{"2026-06", "h1"}, revEarly, 1.0}, // 乱序：同键的更早版本后写
		},
		probeCurrent: map[string]float64{
			"2026-06/monthly": 7.0,
			"2026-06/h1":      2.0,
			"2026-05/monthly": 9.0,
		},
	}
}

// crisisShape 是 crisis 的形状：列名与表名都不同，revision 列也不同名。
// 它证明本包不是围着 Hestia 的字段名写的。
func crisisShape(t *testing.T) fixture {
	t.Helper()
	s, err := NewSpec("macro_observations", []string{"ts", "indicator"}, "fetched_at")
	require.NoError(t, err)
	return fixture{
		name:     "crisis",
		spec:     s,
		keys:     []string{"ts", "indicator"},
		valueCol: "value",
		ddl: `CREATE TABLE macro_observations (
			ts         TEXT NOT NULL,
			indicator  TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			value      REAL,
			PRIMARY KEY (ts, indicator, fetched_at))`,
		ins: `INSERT INTO macro_observations (ts, indicator, fetched_at, value) VALUES (?,?,?,?)`,
		probe: []probeRow{
			{[]string{"2026-06-30", "ppi"}, revLatest, 7.0}, // 与下一个键共享 ts，revision 最大
			{[]string{"2026-06-30", "cpi"}, revMiddle, 2.0},
			{[]string{"2026-05-31", "ppi"}, revEarly, 9.0},
			{[]string{"2026-06-30", "cpi"}, revEarly, 1.0}, // 乱序
		},
		probeCurrent: map[string]float64{
			"2026-06-30/ppi": 7.0,
			"2026-06-30/cpi": 2.0,
			"2026-05-31/ppi": 9.0,
		},
	}
}

// singleKeyShape 覆盖单列业务键的边界。
//
// ⚠ 单列键的 correlate 不产出 AND，所以 TASK-004 的 T1 变异对本形状
// **结构性打不中**。它的探针数据只覆盖「单列键可用」与「乱序」，
// 不承担 T1 的守护——不要把它计入 T1 的形状覆盖数。
func singleKeyShape(t *testing.T) fixture {
	t.Helper()
	s, err := NewSpec("single_key_obs", []string{"period"}, "published_at")
	require.NoError(t, err)
	return fixture{
		name:     "single-key",
		spec:     s,
		keys:     []string{"period"},
		valueCol: "m2",
		ddl: `CREATE TABLE single_key_obs (
			period       TEXT NOT NULL,
			published_at TEXT NOT NULL,
			m2           REAL,
			PRIMARY KEY (period, published_at))`,
		ins: `INSERT INTO single_key_obs (period, published_at, m2) VALUES (?,?,?)`,
		probe: []probeRow{
			{[]string{"2026-05"}, revLatest, 7.0},
			{[]string{"2026-06"}, revMiddle, 2.0},
			{[]string{"2026-06"}, revEarly, 1.0}, // 乱序
		},
		probeCurrent: map[string]float64{
			"2026-05": 7.0,
			"2026-06": 2.0,
		},
	}
}

// bothShapes 返回两套主形状，供 t.Run 并行跑同一组用例。
func bothShapes(t *testing.T) []fixture {
	t.Helper()
	return []fixture{hestiaShape(t), crisisShape(t)}
}

// allShapes 是 bothShapes 再加上单列键形状，供不区分键宽的用例使用。
func allShapes(t *testing.T) []fixture {
	t.Helper()
	return append(bothShapes(t), singleKeyShape(t))
}
