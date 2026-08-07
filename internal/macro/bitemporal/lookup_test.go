package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-005 Lookup)
//
// functional[0]     命中/未命中；多版本时 LatestRevision 取最大者
//                   → TestLookupHitAndMiss（对 bothShapes 并行跑）
// functional[1]     两套 Spec 形状行为一致
//                   → TestLookupHitAndMiss / TestLookupIgnoresInsertOrder 均对 bothShapes 跑
// functional[2]     *sql.Tx 也满足 Querier（真的传 Tx，不只靠编译期接口断言）
//                   → TestLookupAcceptsTx
// boundary[0]       空表 → Exists:false 且不是 error
//                   → TestLookupHitAndMiss 的第一段
// functional[0]     WHERE 用上业务键的每一列（首列相同、次列不同的邻键不得混入 MAX）
//                   → TestLookupUsesEveryKeyColumn（dev-agent-40 补，理由见该测试注释）
// boundary[1]       乱序插入后 LatestRevision 仍是最大者
//                   → TestLookupIgnoresInsertOrder（按 C8 裁定跑两套形状）
// boundary[2]       契约 T5：零值 Spec 必须报错
//                   → TestLookupRejectsZeroSpec
// error_handling[0] Key 键集与 Spec 业务键不匹配 → 报错（复用 checkKey）
// error_handling[2] 契约 T1 对偶：行为侧证明 Lookup 真的调用了 checkKey
//                   → TestLookupRejectsMismatchedKey（变异：跳过 checkKey 后本条须转红）
// error_handling[1] SQL 错误被包装且含表名（断言含表名，不是只断言有 error）
//                   → TestLookupWrapsSQLError
// error_handling[3] 注入防护：Key 的【取值】携带 SQL 元字符绝不能命中
//                   → TestLookupRejectsInjectionInKeyValues（对 allShapes 跑，理由见该测试注释）
// error_handling[5] revision 的 ISO 8601 形态约束——明确判定「本包不管，由建表方与
//                   写入方保证」，并把该判定的可观测后果钉成可执行契约
//                   → TestLookupRevisionFormIsCallerGuaranteed
//
// C8 适用范围（Leader 裁定，按「这条路径是否真的走到 SQL、且键形状是否进入 WHERE」划分）：
//   跑两套/多套 —— TestLookupHitAndMiss、TestLookupIgnoresInsertOrder、
//                  TestLookupRejectsInjectionInKeyValues
//   只跑一套   —— TestLookupRejectsMismatchedKey、TestLookupRejectsZeroSpec（到达 SQL 前就返回）
//                  TestLookupWrapsSQLError（错误来自表/列不存在，与键形状无关）
//                  TestLookupAcceptsTx（验的是 Querier 的实现者，与键形状无关）
//                  TestLookupRevisionFormIsCallerGuaranteed（验的是 revision 列，与键形状无关）
//
// non_functional：本任务显式【不做】 -race——包内无 goroutine，收益为零。这是记录在案
// 的排除，不是遗漏。

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupHitAndMiss(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			ctx := context.Background()

			// 空表：不存在，且不是错误——首次入库是正常路径
			emptyMsg := f.failureContext("空表查询")
			st, err := Lookup(ctx, db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, emptyMsg)
			assert.False(t, st.Exists, emptyMsg+"：不应存在")
			assert.Empty(t, st.LatestRevision, emptyMsg)

			// 一个版本
			singleMsg := f.failureContext("单版本查询")
			f.insert(t, db, "2026-07-15", 1.0, "2026-06", "h1")
			st, err = Lookup(ctx, db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, singleMsg)
			assert.True(t, st.Exists, singleMsg+"：应已存在")
			assert.Equal(t, "2026-07-15", st.LatestRevision, singleMsg)

			// 三个版本：取最大者
			multiMsg := f.failureContext("多版本查询")
			f.insert(t, db, "2026-08-20", 2.0, "2026-06", "h1")
			f.insert(t, db, "2026-09-30", 3.0, "2026-06", "h1")
			st, err = Lookup(ctx, db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, multiMsg)
			assert.Equal(t, "2026-09-30", st.LatestRevision, multiMsg+"：应取最大 revision")

			// 另一个业务键不受影响——证明 WHERE 子句真的按业务键过滤
			otherKeyMsg := f.failureContext("另一业务键查询")
			st, err = Lookup(ctx, db, f.spec, f.key("2026-05", "monthly"))
			require.NoError(t, err, otherKeyMsg)
			assert.False(t, st.Exists, otherKeyMsg+"：不应被其它键的行命中")
		})
	}
}

// TestLookupUsesEveryKeyColumn 守「WHERE 子句用上了业务键的每一列」。
//
// 为什么 TestLookupHitAndMiss 挡不住这个变异：它的「另一个业务键不受影响」那段
// 用的是 ("2026-05","monthly")，**两列都与库中数据不同**——WHERE 只用首列也照样
// 查不到，变异存活。必须构造【首列相同、次列不同】的一对键才能打中。
//
// 探针数据正好是这个形状：probe[0] 与 probe[1] 共享首列，且 probe[0] 的 revision
// （revLatest）比 probe[1] 这个键的最大 revision（revMiddle）更大——所以 WHERE
// 漏列时 MAX 会把邻键的行算进来，返回 revLatest。
//
// ⚠ singleKeyShape 只有一列，这个变异对它**结构性打不中**，故只跑 bothShapes。
func TestLookupUsesEveryKeyColumn(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			f.seedProbe(t, db)

			// probe[1] 与 probe[0] 共享首列；查它应只算它自己的行
			msg := f.failureContext("共享首列的键查询")
			shared := f.probe[1].keyVals
			st, err := Lookup(context.Background(), db, f.spec, f.key(shared...))
			require.NoError(t, err, msg)
			require.True(t, st.Exists, msg)
			assert.Equal(t, revMiddle, st.LatestRevision,
				msg+"：应只算本键的行，不得把共享首列的邻键算进 MAX")
		})
	}
}

// TestLookupDistinguishesEmptyRevisionFromMissingKey 守 sql.NullString 的第二种情形。
//
// Lookup 用 NullString 是为了分辨两件事：「没有这个业务键」（MAX 返回 NULL）与
// 「有这个键、但 revision 是空串」。**第二种此前零覆盖**——三套 fixture 都没插过
// revision 为空串的行，而 DDL 只是 TEXT NOT NULL，**空串完全合法**。QA 实证：把
// `!latest.Valid` 换成 `latest.String == ""`，全包 125 条无一转红。
//
// 这不是形式问题。把 revision 为空串的已存在键判成 Exists:false，Classify 会给出
// New 而不是 Revision，于是重复入库——正是本包存在的理由被静默绕过。
// 端到端的那一环由 TestLookupFeedsClassifyOnEmptyRevision 守。
func TestLookupDistinguishesEmptyRevisionFromMissingKey(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			ctx := context.Background()

			// 键存在，但 revision 是空串
			f.insert(t, db, "", 1.0, "2026-06", "h1")

			msg := f.failureContext("空串 revision 查询")
			st, err := Lookup(ctx, db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, msg)
			assert.True(t, st.Exists, msg+
				"：键存在、revision 为空串，必须判 Exists:true——判成 false 会让 Classify 给出 New 而重复入库")
			assert.Equal(t, "", st.LatestRevision, msg+"：revision 就是空串")

			// 对照组不可省：否则「一律返回 Exists:true」也能让上面全绿。真的不存在
			// 的键仍须是 false——两条合起来才说明分辨的是「NULL」而不是「空串」。
			msgMiss := f.failureContext("确实不存在的键查询")
			st, err = Lookup(ctx, db, f.spec, f.key("2026-05", "monthly"))
			require.NoError(t, err, msgMiss)
			assert.False(t, st.Exists, msgMiss+"：这个键真的不存在，必须是 false")
		})
	}
}

// TestLookupIgnoresInsertOrder 验乱序写入后仍取最大 revision。
//
// 按 C8 裁定跑两套形状：这条路径真的执行 SQL，键形状会进入 WHERE 子句。
// （需求文档的样例只跑 hestiaShape，裁定要求改为两套。）
func TestLookupIgnoresInsertOrder(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			msg := f.failureContext("乱序插入后查询")
			// 先写晚的、后写早的：插入顺序与 revision 顺序相反
			f.insert(t, db, "2026-07-15", 3.0, "2026-06", "h1")
			f.insert(t, db, "2020-07-10", 1.0, "2026-06", "h1")

			st, err := Lookup(context.Background(), db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, msg)
			assert.Equal(t, "2026-07-15", st.LatestRevision,
				msg+"：应取最大 revision 而非最后写入的")
		})
	}
}

// TestLookupRejectsMismatchedKey 是契约 T1 对偶的行为侧证明：Lookup 必须真的
// 调用 checkKey。
//
// TASK-001 的 TestCheckKey 只证明 checkKey 本身正确，证明不了 Lookup 用上了它——
// 若 Lookup 跳过 checkKey 直接查询，缺键会被当成空字符串取值查出「不存在」，
// 静默返回 Exists:false 而非报错。本条转红是该变异的唯一守护。
//
// 只跑一套形状：这三种输入在到达 SQL 之前就返回了，与键形状无关（C8 裁定）。
func TestLookupRejectsMismatchedKey(t *testing.T) {
	f := hestiaShape(t)
	db := newDB(t, f)
	ctx := context.Background()

	t.Run("缺键", func(t *testing.T) {
		_, err := Lookup(ctx, db, f.spec, Key{"period": "2026-06"})
		require.Error(t, err)
	})
	t.Run("多键", func(t *testing.T) {
		_, err := Lookup(ctx, db, f.spec, Key{"period": "2026-06", "period_type": "h1", "extra": "x"})
		require.Error(t, err)
	})
	t.Run("键名不符", func(t *testing.T) {
		// 个数相同但键名不同——最隐蔽的一种，只有逐列检查能发现
		_, err := Lookup(ctx, db, f.spec, Key{"period": "2026-06", "wrong": "h1"})
		require.Error(t, err)
	})
}

// TestLookupRejectsZeroSpec 守契约 T5。
//
// 零值 Spec 是唯一绕过 NewSpec 校验的途径（字段未导出 ⇒ 包外造不出别的未校验 Spec），
// 而 Lookup 是它能造成 SQL 注入的唯一入口——零值 Spec 的 table/revisionCol 是空串，
// 拼出来的 SQL 结构本身就是坏的。这是安全边界，不是形式条款。
func TestLookupRejectsZeroSpec(t *testing.T) {
	f := hestiaShape(t)
	db := newDB(t, f)

	_, err := Lookup(context.Background(), db, Spec{}, Key{"period": "2026-06"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NewSpec", "错误信息应指出正确的构造方式")
}

// TestLookupWrapsSQLError 断言错误信息【含表名】。
//
// 只断言 require.Error 是不够的：任何原因的 error 都能让它成立（契约 T2 同族），
// 包括本该被 checkKey 拦下的输入。含表名才证明错误确实来自这次查询。
func TestLookupWrapsSQLError(t *testing.T) {
	f := hestiaShape(t)
	db := newDB(t, f)
	ctx := context.Background()

	t.Run("表不存在", func(t *testing.T) {
		missing, err := NewSpec("no_such_table", []string{"period", "period_type"}, "published_at")
		require.NoError(t, err)

		_, err = Lookup(ctx, db, missing, Key{"period": "2026-06", "period_type": "h1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no_such_table", "错误信息应带表名，便于定位")
	})

	// ⚠ 上面那条【证明不了】包装本身加了表名——sqlite 的原始信息就是
	// "no such table: no_such_table"，表名是驱动给的，包装删掉表名它照样绿。
	// 这不是推测：变异 MT4（从包装里删掉 spec.table）后，只有上面那条时全套 27
	// 条无一转红。需求文档的样例测试到此为止，继承它就等于 error_handling[1]
	// 无人守护。
	//
	// 「列不存在」把包装的贡献隔离出来：驱动信息是 "no such column: no_such_col"，
	// 不含表名，所以表名只可能来自包装。先用 errors.Unwrap 断言前提成立，
	// 再断言包装后的信息含表名——两条合起来才构成守护。
	t.Run("列不存在——表名只可能来自包装", func(t *testing.T) {
		badCol, err := NewSpec("hestia_observations", []string{"period", "period_type"}, "no_such_col")
		require.NoError(t, err)

		_, err = Lookup(ctx, db, badCol, Key{"period": "2026-06", "period_type": "h1"})
		require.Error(t, err)

		inner := errors.Unwrap(err)
		require.NotNil(t, inner, "应以 %w 包装，调用方才能 errors.Is/As 到驱动错误")
		require.NotContains(t, inner.Error(), "hestia_observations",
			"前提：驱动的原始信息不含表名——前提不成立则下一条断言什么也证明不了")
		assert.Contains(t, err.Error(), "hestia_observations",
			"表名不在驱动信息里，只能由 Lookup 的包装提供")
	})
}

// TestLookupAcceptsTx 真的传一个 *sql.Tx 进去。
//
// 编译期接口断言（var _ Querier = (*sql.Tx)(nil)）只证明方法集匹配，证明不了
// 在事务里查得到数据——本条走真事务、真查询。
func TestLookupAcceptsTx(t *testing.T) {
	f := hestiaShape(t)
	db := newDB(t, f)
	f.insert(t, db, "2026-07-15", 1.0, "2026-06", "h1")

	tx, err := db.Begin()
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck // 只读事务，回滚失败无可处置

	st, err := Lookup(context.Background(), tx, f.spec, f.key("2026-06", "h1"))
	require.NoError(t, err)
	assert.True(t, st.Exists, "*sql.Tx 应满足 Querier 且能查到数据")
}

// TestLookupRejectsInjectionInKeyValues 是包的架构声明「注入面为零」在 lookup.go
// 这一侧的守护。
//
// 为什么必须单列一条：Lookup 是包内唯一把【调用方提供的值】混进查询构造的函数。
// 若把 `c + " = ?"` 改写成 fmt.Sprintf("%s = '%s'", c, key[c]) 并去掉 args，
// 本任务其余全部用例仍然全绿——因为它们的取值都是良性字符串。「取值走 ? 占位符」
// 若只写在注释里，那是一句未经验证的断言，不是守护。
//
// 跑 allShapes（含单列键）而非 bothShapes：键列数会改变注入串与 WHERE 子句其余
// 部分的相互作用——单列键没有尾随的 ` AND ...` 可被 `--` 注释掉。这是对 C8 裁定的
// 一处加强，理由已写进 discovery。
func TestLookupRejectsInjectionInKeyValues(t *testing.T) {
	for _, f := range allShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			// 库里必须有数据：否则「注入没命中」与「表是空的」无法区分，
			// 这条测试在变异下也就不会转红。
			f.seedProbe(t, db)
			ctx := context.Background()

			// 受试取值要【逐列】出现，不能固定在首列。
			//
			// 理由是结构性的：lookup.go 里 where[i] 是**逐列循环**，每一列都是一个
			// 独立的拼接点，判据要覆盖 N 个位置。此前只填 vals[0]、其余列恒为 "h1"，
			// 于是「只对 i>0 的列裸拼」这个变异完全存活（QA 实证：125 PASS 无一转红；
			// 同一变异下仅把载荷移到末列立即 7 FAIL——缺口确由**位置**造成）。
			//
			// ⚠ 这与载荷的**字符类**（单引号系 / 双引号系）是**正交**的两个维度：
			// 字符类决定哪种拼法会被打中，位置决定哪个拼接点会被打中。两者都要遍历。
			keyValsAt := func(pos int, payload string) []string {
				vals := make([]string, len(f.keys))
				for i := range vals {
					vals[i] = "h1"
				}
				vals[pos] = payload
				return vals
			}

			t.Run("注入载荷不得命中", func(t *testing.T) {
				msg := f.failureContext("注入取值查询")
				// 经典布尔注入 + 行尾注释：拼进 SQL 会让 WHERE 恒真而匹配全表；
				// 走占位符则只是一个不存在的普通取值。
				// ⚠ 单引号系与双引号系都要有。只测单引号会漏掉一整类得手的注入：
				// 若有人把取值改成 fmt.Sprintf("%s = %q", c, key[c])——那看起来像
				// 在转义，正是「被告知不要裸拼值」的人最可能选的写法——Go 的 %q 用
				// 反斜杠转义双引号，而 SQLite 不认反斜杠转义，token 在 `\` 后的 `"`
				// 处就结束，剩下的 ` OR 1=1 --` 被当 SQL 解析，`--` 注释掉尾部条件，
				// WHERE 恒真。此时单引号载荷仍被 %q 的双引号包裹挡住，全套测试无一
				// 转红，而双引号载荷会返回全表最大 revision（不属于被查的业务键）。
				payloads := []string{
					"x' OR 1=1 --",
					`x" OR 1=1 --`,
					"' OR '1'='1",
					"2026-06'; DROP TABLE " + f.spec.table + "; --",
				}
				for _, p := range payloads {
					// 逐列：每一列都是独立的拼接点，都要单独承受一次载荷
					for pos, col := range f.keys {
						vals := keyValsAt(pos, p)
						// 前提检查：载荷必须真的进入业务键取值。keyValsAt 若被改坏
						// （不再放入指定列），本条就退化成「用良性取值查不到」的空
						// 测试，却依然全绿——非空性必须自己守，不能靠 helper 一直正确。
						require.Contains(t, vals, p, msg+"：载荷未进入 Key，本条测试是空的")

						st, err := Lookup(ctx, db, f.spec, f.key(vals...))
						require.NoError(t, err,
							msg+"：应是普通的查不到，不是 SQL 错误（列 %s）", col)
						assert.False(t, st.Exists,
							msg+"：取值 %q 放在列 %s 时绝不能命中任何行", p, col)
						assert.Empty(t, st.LatestRevision, msg+"（列 %s）", col)
					}
				}
			})

			t.Run("含单引号的合法取值仍能被正确命中", func(t *testing.T) {
				// 反向证明：不是「一律查不到」。占位符既要挡住注入，也要让含引号的
				// 正常取值照常工作——只有这两条同时成立才说明走的是参数化查询。
				msg := f.failureContext("含引号取值查询")
				// 同样逐列：每个拼接点既要挡住注入，也要让合法的含引号取值照常工作
				for pos, col := range f.keys {
					vals := keyValsAt(pos, "O'Brien")
					f.insert(t, db, "2026-07-15", 42.0, vals...)

					st, err := Lookup(ctx, db, f.spec, f.key(vals...))
					require.NoError(t, err, msg+"（列 %s）", col)
					assert.True(t, st.Exists, msg+"：含单引号的合法取值应能命中（列 %s）", col)
					assert.Equal(t, "2026-07-15", st.LatestRevision, msg+"（列 %s）", col)
				}
			})
		})
	}
}

// TestLookupRevisionFormIsCallerGuaranteed 钉住一个【已知且被接受】的限制。
//
// Lookup 用 SQL 的 MAX() 取最大 revision，Classify 用 Go 的字符串比较——两者都是
// 字典序。只有当 revision 是 ISO 8601 日期或 RFC3339 时间戳时，字典序才等于时间序。
//
// 本包【不】校验这个形态，明确判定：由建表方与写入方保证（理由见 Lookup 的文档注释）。
// 这条测试把「不校验」的可观测后果钉成可执行契约：若将来有人给 Lookup 加了形态校验、
// 或改了取最大值的方式，本条会转红，迫使他显式地推翻这个判定，而不是悄悄改掉它。
//
// 只跑一套形状：验的是 revision 列的取值形态，与业务键形状无关（C8 裁定）。
func TestLookupRevisionFormIsCallerGuaranteed(t *testing.T) {
	f := hestiaShape(t)
	db := newDB(t, f)

	// 纯数字版本号是非 ISO 形态的典型：字典序 "9" > "10"，而时间序是 10 更新。
	f.insert(t, db, "9", 1.0, "2026-06", "h1")
	f.insert(t, db, "10", 2.0, "2026-06", "h1")

	st, err := Lookup(context.Background(), db, f.spec, f.key("2026-06", "h1"))
	require.NoError(t, err, "非 ISO 形态不报错——本包不校验 revision 形态")
	require.True(t, st.Exists)
	assert.Equal(t, "9", st.LatestRevision,
		"字典序的结果，这【不是】语义上最新的版本；喂非 ISO 形态会静默判错，由调用方保证形态")
}

// TestLookupFeedsClassify 覆盖包的【两半之间的接缝】。
//
// QA 用 grep 确认过：此前全测试集**没有任何一处**把 Lookup 返回的 State 喂给
// Classify。两半各自被测得很细，接缝本身零覆盖——而接缝正是两个排序假设必须
// 一致的地方：Lookup 侧走 SQL 的 MAX()，Classify 侧走 Go 的字符串比较。两者
// 一旦错位，任何**单侧**测试都发现不了。
//
// 本条按 Lookup → Classify 的真实调用顺序跑完四种 Verdict，端到端断言判定结果。
func TestLookupFeedsClassify(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			ctx := context.Background()
			msg := f.failureContext("Lookup→Classify 接缝")

			lookup := func(t *testing.T) State {
				t.Helper()
				st, err := Lookup(ctx, db, f.spec, f.key("2026-06", "h1"))
				require.NoError(t, err, msg)
				return st
			}

			// 空库 → New：首次入库走的就是这条路径
			assert.Equal(t, New, Classify(lookup(t), "2026-07-15"), msg+"：空库应判 New")

			f.insert(t, db, "2026-07-15", 1.0, "2026-06", "h1")
			st := lookup(t)
			// 同 revision 重来——央行迁移站点换了 URL，发布日不变
			assert.Equal(t, Duplicate, Classify(st, "2026-07-15"), msg+"：同 revision 应判 Duplicate")
			assert.Equal(t, Revision, Classify(st, "2026-08-20"), msg+"：更新的 revision 应判 Revision")
			assert.Equal(t, OutOfOrder, Classify(st, "2020-07-10"), msg+"：更旧的 revision 应判 OutOfOrder")

			// 多版本后，判定必须以库中最大者为基准——这一步同时验了
			// 「Lookup 取的最大者」与「Classify 拿它作比较基准」是同一个值
			f.insert(t, db, "2026-09-30", 2.0, "2026-06", "h1")
			st = lookup(t)
			assert.Equal(t, Duplicate, Classify(st, "2026-09-30"), msg+"：应以最大 revision 为基准")
			assert.Equal(t, OutOfOrder, Classify(st, "2026-08-20"),
				msg+"：比最大者旧的应判 OutOfOrder，即使它确实在库里")
		})
	}
}

// TestLookupFeedsClassifyOnEmptyRevision 是 Q1 后果链在接缝上的直接体现。
//
// revision 为空串的行是已存在的键。若 Lookup 把它判成 Exists:false，Classify 就会
// 给出 New 而不是 Revision——调用方随即重复入库。这条把「NullString 分辨的是 NULL
// 而不是空串」从 Lookup 的内部细节，变成一条端到端的行为断言。
func TestLookupFeedsClassifyOnEmptyRevision(t *testing.T) {
	for _, f := range bothShapes(t) {
		t.Run(f.name, func(t *testing.T) {
			db := newDB(t, f)
			msg := f.failureContext("空串 revision 的接缝")

			f.insert(t, db, "", 1.0, "2026-06", "h1")

			st, err := Lookup(context.Background(), db, f.spec, f.key("2026-06", "h1"))
			require.NoError(t, err, msg)
			assert.Equal(t, Revision, Classify(st, "2026-07-15"),
				msg+"：键已存在（revision 为空串），必须判 Revision；判 New 就会重复入库")
		})
	}
}
