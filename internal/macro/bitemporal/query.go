package bitemporal

import "fmt"

// queryAlias 是两个构造器给外层表用的别名。
//
// 它是本包拼进 SQL 的标识符里**唯一不来自 Spec 的那个**：表名、业务键列、
// revision 列都取自 Spec 的未导出字段，已在 NewSpec 里过了 identRE；而
// correlate 的 alias 参数是普通字符串，不经任何校验。做成包级常量而非各处
// 写字面量，是为了让「注入面为零」这个声明有一个可检查的落点——
// TestQueryAliasPassesIdentifierGate 让它也过同一道闸门。
const queryAlias = "o"

// CurrentQuery 返回「每个业务键的当前行」的查询。
//
// 当前行由 revision 列派生——同一业务键下 revision 最大的那行——而不是靠一个
// is_current 列。派生消除了冗余状态：不可能出现列值与事实不符，且乱序写入
// （回填不保证按时间顺序）自动正确。
//
// 建视图由调用方完成，本包不接受视图名参数：
//
//	ddl := "CREATE VIEW v_hestia_current AS " + bitemporal.CurrentQuery(spec)
//
// 这样本包生成的 SQL 中每个标识符都来自校验过的 Spec，没有未校验的拼接输入。
//
// 传入零值 Spec 会生成表名为空的 SQL，在第一次执行时立刻报语法错误。
// 这是刻意不额外防御的——零值 Spec 是构造不出合法 Spec 才会走到的编程错误，
// 而语法错误暴露得足够早（设计文档 2026-08-07-macro-bitemporal-design.md:231）。
func CurrentQuery(spec Spec) string {
	return fmt.Sprintf(
		"SELECT * FROM %s %s WHERE %s.%s = (SELECT MAX(%s) FROM %s WHERE %s)",
		spec.table, queryAlias, queryAlias, spec.revisionCol,
		spec.revisionCol, spec.table, spec.correlate(queryAlias),
	)
}

// AsOfQuery 返回「在某个时点应当看到的行」的查询，带一个 ? 占位符接收时点。
//
// 这是双时态的另一半：CurrentQuery 回答「现在最好的估计」，AsOfQuery 回答
// 「我当时看到的」。没有后者就无法区分「我判断错了」和「数据后来被修订了」。
//
// 时点比较用 <= 而非 <：as-of 恰好等于某个 revision 时，那个 revision 应当
// 【被看见】——「我在发布当天知道什么」的答案包含当天发布的内容。差这一位
// 会让每个历史视图静默偏移一个修订。
func AsOfQuery(spec Spec) string {
	return fmt.Sprintf(
		"SELECT * FROM %s %s WHERE %s.%s = (SELECT MAX(%s) FROM %s WHERE %s AND %s <= ?)",
		spec.table, queryAlias, queryAlias, spec.revisionCol,
		spec.revisionCol, spec.table, spec.correlate(queryAlias), spec.revisionCol,
	)
}
