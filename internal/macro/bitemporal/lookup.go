package bitemporal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Querier 是 Lookup 需要的最小数据库能力。*sql.DB 与 *sql.Tx 都满足它，
// 由调用方决定在不在事务里查——与 crisis 的 FREDFetcher / HistoryFetcher
// 收窄依赖的手法一致。
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Lookup 查询某业务键在库中的现状，供 Classify 判定这次写入的性质。
//
// 查不到返回 State{Exists: false} 而非 error——首次入库是正常路径，不该走
// 错误分支。
//
// # SQL 构造
//
// 拼进 SQL 的标识符只有三个来源：spec.table、spec.businessKey 各列、
// spec.revisionCol，它们在 NewSpec 里全部过了 identRE。业务键的【取值】一律
// 走 ? 占位符，绝不拼进语句——Lookup 是本包唯一接触调用方提供的值的地方，
// 「注入面为零」这个包级声明在这里被打破的话，别处补不回来。
//
// # revision 的形态：本包不校验，由建表方与写入方保证
//
// 这里用 MAX() 取最大 revision，Classify 用 Go 的字符串比较——两者都是字典序，
// 只有当 revision 是 ISO 8601 日期或 RFC3339 时间戳时字典序才等于时间序。
// 喂进 "2026-7-15"（少补零）或纯数字版本号（"9" > "10"）会静默判错、零告警。
//
// 明确不在这里加形态校验，理由有三：本包是薄机制层，只认识键的形状，不认识列的
// 语义；校验要在每次查询后跑一次正则，成本落在正常路径上；把一个数据质量问题
// 转成查询失败，会让调用方在读路径上遇到本该在写路径上暴露的错误。
// 该判定的可观测后果由 TestLookupRevisionFormIsCallerGuaranteed 钉住——若将来
// 有人推翻它，那条测试会转红，迫使他显式地改。
func Lookup(ctx context.Context, q Querier, spec Spec, key Key) (State, error) {
	if spec.zero() {
		return State{}, fmt.Errorf("bitemporal: zero Spec; construct it with NewSpec")
	}
	if err := spec.checkKey(key); err != nil {
		return State{}, err
	}

	where := make([]string, len(spec.businessKey))
	args := make([]any, len(spec.businessKey))
	for i, c := range spec.businessKey {
		where[i] = c + " = ?" // 列名来自校验过的 Spec，取值走占位符
		args[i] = key[c]
	}
	query := fmt.Sprintf("SELECT MAX(%s) FROM %s WHERE %s",
		spec.revisionCol, spec.table, strings.Join(where, " AND "))

	// MAX() 在无匹配行时返回一行 NULL 而不是 sql.ErrNoRows，所以用 NullString
	// 区分「没有这个业务键」与「有但 revision 为空」。
	var latest sql.NullString
	if err := q.QueryRowContext(ctx, query, args...).Scan(&latest); err != nil {
		return State{}, fmt.Errorf("bitemporal lookup %s: %w", spec.table, err)
	}
	if !latest.Valid {
		return State{}, nil
	}
	return State{Exists: true, LatestRevision: latest.String}, nil
}
