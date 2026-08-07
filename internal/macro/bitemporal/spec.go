// Package bitemporal 提供双时态表的时态语义：判断一次写入相对库中现状是新增、
// 重复、修订还是乱序，并构造「当前行」与「某时点视图」的查询。
//
// 它只认识键的形状——业务键有哪几列、哪一列是发布时间轴——不知道表里还有什么
// 业务列，也不提供写操作。「当前行」由 revision 列派生（MAX）而非 is_current 列，
// 所以写入是调用方一句普通 INSERT。
//
// 两个使用者的键形状不同，这正是本包做成运行时 Spec 而非泛型的原因：
//
//	hestia_observations: (period, period_type) + published_at
//	macro_observations:  (ts, indicator)       + fetched_at
package bitemporal

import (
	"fmt"
	"regexp"
	"strings"
)

// identRE 限制一切拼进 SQL 的标识符。Spec 的取值来自代码常量而非用户输入，
// 但凡拼接 SQL 就应校验——本包生成的 SQL 中每个标识符都过这一关，
// 业务键的取值则一律走 ? 占位符。
var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Spec 描述一张双时态表的时态形状。
//
// 字段未导出：唯一构造路径是 NewSpec，因此任何非零值 Spec 都已通过校验。
// 这让 CurrentQuery / AsOfQuery 能安全地只返回字符串——否则每个调用点都要
// 处理一段几乎走不到的错误。
type Spec struct {
	table       string
	businessKey []string
	revisionCol string
}

// NewSpec 校验并构造 Spec。
//
// 错误信息里带上被拒的标识符（%q），既方便定位，也让测试能断言「正是这个入口
// 拦下了这个值」，而不是笼统地断言「有 error」。
func NewSpec(table string, businessKey []string, revisionCol string) (Spec, error) {
	if !identRE.MatchString(table) {
		return Spec{}, fmt.Errorf("bitemporal: invalid table name %q", table)
	}
	if !identRE.MatchString(revisionCol) {
		return Spec{}, fmt.Errorf("bitemporal: invalid revision column %q", revisionCol)
	}
	if len(businessKey) == 0 {
		return Spec{}, fmt.Errorf("bitemporal: business key must not be empty")
	}
	seen := make(map[string]bool, len(businessKey))
	for _, c := range businessKey {
		if !identRE.MatchString(c) {
			return Spec{}, fmt.Errorf("bitemporal: invalid business key column %q", c)
		}
		if seen[c] {
			return Spec{}, fmt.Errorf("bitemporal: duplicate business key column %q", c)
		}
		seen[c] = true
	}
	if seen[revisionCol] {
		return Spec{}, fmt.Errorf("bitemporal: revision column %q also appears in business key", revisionCol)
	}
	// 复制切片：调用方之后改自己那份不应影响已构造的 Spec
	return Spec{
		table:       table,
		businessKey: append([]string(nil), businessKey...),
		revisionCol: revisionCol,
	}, nil
}

// zero 报告 s 是否为零值，即未经 NewSpec 构造。
// 表名过了 identRE 就一定非空，所以它足以区分「构造过」与「零值」。
func (s Spec) zero() bool { return s.table == "" }

// Key 是一个业务键的取值，如 {"period": "2026-06", "period_type": "h1"}。
// 键集必须与 Spec 的业务键完全一致。
type Key map[string]string

// checkKey 确认 k 的键集与 s 的业务键一致。
// 键集不符会静默查到错误的行，必须在发出 SQL 之前拦下。
//
// 先比长度再逐列查存在性：两者合起来才能同时挡住缺键、多键与键名不同——
// 长度相等但键名不同的那种最隐蔽，只有逐列检查能发现。
func (s Spec) checkKey(k Key) error {
	if len(k) != len(s.businessKey) {
		return fmt.Errorf("bitemporal: key has %d columns, spec %s expects %d",
			len(k), s.table, len(s.businessKey))
	}
	for _, c := range s.businessKey {
		if _, ok := k[c]; !ok {
			return fmt.Errorf("bitemporal: key missing column %q for table %s", c, s.table)
		}
	}
	return nil
}

// correlate 生成子查询与外层的关联条件，如
// `period = o.period AND period_type = o.period_type`。
//
// 只拼列名与别名，不含任何字面值——取值一律由调用方以 ? 占位符传入。
func (s Spec) correlate(alias string) string {
	parts := make([]string, len(s.businessKey))
	for i, c := range s.businessKey {
		parts[i] = fmt.Sprintf("%s = %s.%s", c, alias, c)
	}
	return strings.Join(parts, " AND ")
}
