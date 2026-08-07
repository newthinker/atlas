package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-004 query)
// functional[0]     CurrentQuery 在有修订时只返回最新那行                    → TestCurrentQueryReturnsLatestRevision
// functional[0]     AsOfQuery 修订前后结果不同                               → TestAsOfQueryReturnsHistoricalRow
// functional[1]     两套 Spec 形状（hestia + crisis）行为一致                → 上述测试对 bothShapes 并行跑（t.Run 带形状名）
// functional[2]     契约 T1：行为侧证明 Query 真的用了 Spec.correlate 的输出 → TestCurrentQueryReturnsLatestRevision（见其注释）
// boundary[0]       空表 → CurrentQuery 返回零行                             → TestCurrentQueryOnEmptyTable
// boundary[1]       乱序插入后 CurrentQuery 仍返回 revision 最大的行         → TestCurrentQueryIgnoresInsertOrder
// boundary[2]       单列业务键可用（singleKeyShape，不在 bothShapes 内）     → TestQueriesWorkWithSingleColumnKey
// boundary[3]       零值 Spec → 含空表名的 SQL → 执行时报语法错误            → TestZeroSpecProducesSyntaxErrorAtExecution
// boundary[4]①     as-of 恰好等于某个 revision（`<=` 的包含性边界）         → TestAsOfQueryAtExactRevision
// boundary[4]②     as-of 早于全部 revision → 零行                           → TestAsOfQueryBeforeAllRevisions
// boundary[4]③     空表上的 AsOfQuery                                       → TestAsOfQueryOnEmptyTable
// error_handling[0] C7：除 TestQueriesContainNoLiteralValues 外无 SQL 字符串断言 → 本文件唯一的字符串断言在该测试内
// error_handling[1] 进入 SQL 的标识符只能来自 Spec；alias 也必须过 identRE  → TestQueryAliasPassesIdentifierGate
// non_functional[0] 业务键取值一律走 ? 占位符                               → TestQueriesContainNoLiteralValues
// non_functional[1] 断言的 map 以【完整业务键】为键，不用第一列             → currentRows / 各期望值映射

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asOfAtMiddleExpected 是 as-of 恰好等于 revMiddle 时各形状应看到的行。
//
// 硬编码而非由探针数据算出：期望值必须人可读、且不与被测的 SQL 语义共用算法，
// 否则两边可能同时错。键是【完整业务键】——探针里 A 与 B 共享第一列，只用第一列
// 作 map 键会让两者互相覆盖（「补了数据」和「断言能分辨这些数据」是两件事）。
//
// 为什么是这三个值：as-of=revMiddle 时，A 键（有 revEarly/revMiddle 两版）应看到
// revMiddle 那版；B 键唯一的版本是 revLatest，晚于 as-of，整个不该出现；
// C 键只有 revEarly，早于 as-of，照常出现。
var asOfAtMiddleExpected = map[string]map[string]float64{
	"hestia": {"2026-06/h1": 2.0, "2026-05/monthly": 9.0},
	"crisis": {"2026-06-30/cpi": 2.0, "2026-05-31/ppi": 9.0},
}

// TestCurrentQueryReturnsLatestRevision 同时承担契约 T1。
//
// TASK-001 的 TestCorrelate 断言 correlate 的字符串输出——那证明「函数会拼对」，
// 不证明「Query 用了它拼出来的东西」。这条测试跑真 SQL，且 TASK-003 的探针数据
// 刻意包含一对共享恰好一列、且最大 revision 不相等的业务键，使得把 correlate 的
// AND 改成 OR 时结果必然改变。两者合起来才构成 T1 的行为侧证明：
// 改 correlate 的实现 → 本测试转红。
//
// （Sprint 031 的 tushare callKey 缺口同形：纯函数有单测、有一个方向的行为守护，
// 唯独没人验「输出被用上了」。）
func TestCurrentQueryReturnsLatestRevision(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			want := f.seedProbe(t, db)

			assert.Equal(t, want, currentRows(t, db, f, CurrentQuery(f.spec)),
				"形状 %s：应取每个业务键下 revision 最大的那行", f.name)
		})
	}
}

func TestCurrentQueryOnEmptyTable(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			got := currentRows(t, db, f, CurrentQuery(f.spec))
			assert.Empty(t, got, "形状 %s：空表应返回零行而非报错", f.name)
		})
	}
}

// TestCurrentQueryIgnoresInsertOrder 用显式乱序插入覆盖 boundary[1]。
// 回填 80 期历史时不保证按时间顺序写入，当前行必须由 MAX(revision) 决定。
func TestCurrentQueryIgnoresInsertOrder(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			k := f.probe[0].keyVals
			// 先写晚的，再写早的，最后写中间的
			f.insert(t, db, revLatest, 3.0, k...)
			f.insert(t, db, revEarly, 1.0, k...)
			f.insert(t, db, revMiddle, 2.0, k...)

			got := currentRows(t, db, f, CurrentQuery(f.spec))
			assert.Equal(t, map[string]float64{f.keyString(k...): 3.0}, got,
				"形状 %s：当前行由 MAX(revision) 决定，与插入顺序无关", f.name)
		})
	}
}

// TestQueriesWorkWithSingleColumnKey 覆盖 boundary[2]。
//
// ⚠ singleKeyShape **不在 bothShapes 里**——bothShapes 是 hestia 与 crisis，
// 两者都是两列业务键。单列键只有这一处守护，不能指望上面那些测试顺带覆盖。
// 它的 Scan 目标数也与主形状不同（三列而非四列），currentRows 按 f.keys 长度
// 动态处理这一点。
func TestQueriesWorkWithSingleColumnKey(t *testing.T) {
	f := singleKeyShape(t)
	db := newDB(t, f)
	want := f.seedProbe(t, db)

	assert.Equal(t, want, currentRows(t, db, f, CurrentQuery(f.spec)),
		"单列业务键的 CurrentQuery")

	// AsOfQuery 在单列键上同样可用：correlate 只产出一个等式、没有 AND
	got := currentRows(t, db, f, AsOfQuery(f.spec), revMiddle)
	assert.Equal(t, map[string]float64{"2026-06": 2.0}, got,
		"单列业务键的 AsOfQuery：as-of=revMiddle 时 2026-05 的 revLatest 版本还看不到")
}

// TestZeroSpecProducesSyntaxErrorAtExecution 钉住一个【既有决定】，不重新裁量。
//
// 设计文档 2026-08-07-macro-bitemporal-design.md:231 明确：
//
//	零值 Spec 传给 SQL 构造器 → 生成表名为空的 SQL，执行时立刻报语法错误。
//	不额外防御——这是构造不出合法 Spec 才会走到的编程错误。
//
// 所以这里断言的是「执行时报错」，而不是「构造器 panic」或「返回空串」——
// 后两者是别的设计，选它们等于把一个已经决定的事重新开放。
// （Lookup 那条路径不同：它显式检查零值并返回错误，见同文档:230。）
func TestZeroSpecProducesSyntaxErrorAtExecution(t *testing.T) {
	db := newDB(t, hestiaShape(t))

	_, err := db.Query(CurrentQuery(Spec{}))
	require.Error(t, err, "零值 Spec 生成的 SQL 必须在执行时失败")
	assert.ErrorContains(t, err, "syntax error",
		"必须是 SQL 语法错误——这正是「不额外防御」意味着的暴露方式")

	_, err = db.Query(AsOfQuery(Spec{}), revMiddle)
	require.Error(t, err, "AsOfQuery 同理")
	assert.ErrorContains(t, err, "syntax error")
}

// TestAsOfQueryReturnsHistoricalRow 覆盖「修订前后结果不同」。
// 没有这个函数就无法区分「我判断错了」和「数据后来被修订了」。
func TestAsOfQueryReturnsHistoricalRow(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			current := f.seedProbe(t, db)
			k := f.keyString(f.probe[1].keyVals...) // A 键：有 revEarly 与 revMiddle 两版

			// 「我在 revEarly 那天看到的是多少」——初值
			before := currentRows(t, db, f, AsOfQuery(f.spec), revEarly)
			assert.Equal(t, 1.0, before[k], "形状 %s：as-of=revEarly 应看到初值", f.name)

			// 「现在看到的是多少」——修订值，与 CurrentQuery 一致
			after := currentRows(t, db, f, AsOfQuery(f.spec), revLatest)
			assert.Equal(t, current[k], after[k],
				"形状 %s：as-of 覆盖全部 revision 时应与 CurrentQuery 一致", f.name)
			assert.NotEqual(t, before[k], after[k], "形状 %s：修订前后必须不同", f.name)
		})
	}
}

// TestAsOfQueryAtExactRevision 是 boundary[4]① —— AsOfQuery 最要紧的一条边界。
//
// hestia 的 published_at 是**日期**，「我在发布当天知道什么」是常用查询。
// 包含性差一位（`<=` 写成 `<`）会让每个历史视图静默偏移一个修订——而区分
// 「我判断错了」与「数据后来被修订了」正是本函数存在的理由。
//
// 需求文档原有的测试用 as-of 2026-08-01/2026-09-01 对 revision 2026-07-15/2026-08-20，
// **两个 as-of 都不等于任何 revision，所以把 `<=` 改成 `<` 两条断言都保持绿**。
// 这里刻意让 as-of 恰好落在 revMiddle 上，使包含性成为可观测的。
func TestAsOfQueryAtExactRevision(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			f.seedProbe(t, db)

			got := currentRows(t, db, f, AsOfQuery(f.spec), revMiddle)
			assert.Equal(t, asOfAtMiddleExpected[f.name], got,
				"形状 %s：as-of 恰好等于 revMiddle 时，该 revision 必须【被包含】", f.name)
		})
	}
}

func TestAsOfQueryBeforeAllRevisions(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			f.seedProbe(t, db)

			// 早于探针里最早的 revision（revEarly = 2026-07-15）
			got := currentRows(t, db, f, AsOfQuery(f.spec), "2020-01-01")
			assert.Empty(t, got, "形状 %s：as-of 早于全部 revision 应返回零行", f.name)
		})
	}
}

func TestAsOfQueryOnEmptyTable(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			got := currentRows(t, db, f, AsOfQuery(f.spec), revMiddle)
			assert.Empty(t, got, "形状 %s：空表应返回零行", f.name)
		})
	}
}

// TestQueriesContainNoLiteralValues 是本文件【唯一】断言 SQL 字符串本身的测试
// （约束 C7）。其余测试一律跑真 SQL 断言行为——断言字符串会把测试钉死在某种
// 写法上，而 SQL 的写法可以变、行为不能变。
func TestQueriesContainNoLiteralValues(t *testing.T) {
	s, err := NewSpec("t", []string{"period"}, "published_at")
	require.NoError(t, err)

	// 单引号与双引号都要否定。只否定单引号时这条对双引号字面量完全无守护：
	// 实测在 SQL 里混入恒真的 `AND "x" = "x"`（不改变查询行为，故行为测试也不红）
	// 后全套 120 条无一转红。SQLite 里双引号本是标识符引用、不匹配列名时才回退为
	// 字符串，正因为这层歧义，双引号字面量比单引号更容易被当成「没问题」而写进来。
	for _, q := range []string{CurrentQuery(s), AsOfQuery(s)} {
		assert.NotContains(t, q, "'", "SQL 中不应出现单引号字面量")
		assert.NotContains(t, q, `"`, "SQL 中不应出现双引号字面量")
	}
	assert.Equal(t, 0, strings.Count(CurrentQuery(s), "?"), "CurrentQuery 无参数")
	assert.Equal(t, 1, strings.Count(AsOfQuery(s), "?"), "AsOfQuery 恰有一个时点参数")
}

// TestQueryAliasPassesIdentifierGate 是 error_handling[1] 的机制化守护。
//
// TASK-001 守住了「非法标识符进不了 Spec」，本条守另一半：「SQL 里的标识符
// 只能来自 Spec」。表名、业务键列、revision 列都取自 Spec 的未导出字段，天然
// 已过校验；**唯一的例外是 correlate 的 alias 参数——它是调用方传入的字符串，
// 不过 identRE**。本包把它固定为包级常量 queryAlias，这里让它也过同一道闸门，
// 把「逐个人工核对」变成机制。
//
// 注：这断言的是一个标识符常量，不是生成的 SQL 字符串，不属于 C7 禁止的范围。
func TestQueryAliasPassesIdentifierGate(t *testing.T) {
	assert.True(t, identRE.MatchString(queryAlias),
		"进入 SQL 的 alias 必须过与 Spec 标识符相同的校验，当前值 %q", queryAlias)
}

// currentRows 跑 query 并按【完整业务键】收集 value。
//
// CurrentQuery/AsOfQuery 用的是 SELECT *，列顺序即 DDL 顺序：业务键各列、
// revision 列、value 列。Scan 目标数随形状而变（单列键形状少一列），
// 所以按 f.keys 的长度动态构造。
//
// map 键用 f.keyString(完整业务键) 而非第一列：探针里 A 与 B 共享第一列，
// 只用第一列会让两者互相覆盖，断言随行返回顺序 flaky。
func currentRows(t *testing.T, db *sql.DB, f fixture, query string, args ...any) map[string]float64 {
	t.Helper()
	rows, err := db.Query(query, args...)
	require.NoError(t, err, "形状 %s：查询失败", f.name)
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		keyVals := make([]string, len(f.keys))
		var rev string
		var val float64
		dest := append(scanTargets(keyVals), &rev, &val)
		require.NoError(t, rows.Scan(dest...), "形状 %s：扫描失败", f.name)
		out[f.keyString(keyVals...)] = val
	}
	require.NoError(t, rows.Err(), "形状 %s：遍历失败", f.name)
	return out
}
