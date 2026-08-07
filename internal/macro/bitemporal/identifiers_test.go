package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-006)
// error_handling[1] 已有 SQL 格式串里不得出现不来自 Spec 的裸标识符（**覆盖边界见
//                   sqlKeywords 的注释，它比这句话窄**） → TestSQLFragmentsIntroduceNoBareIdentifiers
//
// 为什么需要这条：包里原有三处守卫——TASK-001 的 identRE（非法标识符进不了 Spec）、
// TASK-004 的来源核查 + queryAlias 过闸门、TASK-005 的复用 Spec——**守的都是「当前
// 代码正确」**。实证：给 CurrentQuery 的格式串加一个 `ORDER BY rowid`（rowid 不来自
// 任何 Spec），全套 120 条测试无一转红。
//
// 讽刺的是，TASK-004 机制化 alias 时给出的理由原话就是「逐个核对是一次性人工动作，
// 下次有人加个 ORDER BY 列就失效了」——举的那个例子恰恰是当时的新机制不覆盖的。
//
// ⚠ 本测试补的是**那一种形态**，不是「将来加进来的东西」这一整类。三种逃逸形态实测
// 抓不住，列在 sqlKeywords 的注释里——**读那段再判断某个改动是否被覆盖，不要读这段**。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sqlKeywords 是本包的 SQL 片段里【当前实际用到】的全部关键字。
//
// 它同时是这条检查的**触发条件**：只有含其中某个词的字符串字面量才被当作 SQL 片段
// 检查。因此覆盖边界是——**检查只在「同一条字面量内既有已登记关键字、又有裸标识符」
// 时生效**。
//
// ⚠ 以下三种形态【抓不住】。这不是推测，是在 631a9a8 上实测的（各自 137/137 存活，
// 三条自证齐备；对照组 `+ " AND rowid > 0"`（含已登记的 AND）则被抓出 2 红）：
//
//  1. **拼成独立字面量**：`CurrentQuery(...) + " ORDER BY rowid"` —— 新字面量里
//     ORDER / BY / rowid 一个都不在词表，整条被当作「非 SQL 片段」跳过；
//  2. **同形的其它子句**：lookup.go 里 `... + " LIMIT 1"`，同样跳过；
//  3. **经包级常量注入**：新增 `const innerAlias = "[sub]"`（SQLite 合法、过不了
//     identRE）再拼进子查询。它既不在本检查的覆盖内，**也逃过
//     TestQueryAliasPassesIdentifierGate——后者按名字硬编码 queryAlias**
//     （见 discovery TASK-006 的 Q5）。
//
// 所以这条检查堵的是「**在已有的 SQL 格式串里顺手加一段**」这一种形态，
// 不是「任何新增的 SQL 语法」。把闸门重构成遍历登记表以扩大覆盖面是独立的一件事，
// 本 Sprint 未做——**在那之前，新增 SQL 语法仍需人工核对标识符来源**。
var sqlKeywords = map[string]bool{
	"SELECT": true,
	"FROM":   true,
	"WHERE":  true,
	"MAX":    true,
	"AND":    true,
}

// formatVerb 匹配 fmt 的动词（%s / %q / %d / %w…）。它们是参数的占位，
// 参数的来源由 Spec 保证，不是字面量里的标识符。
var formatVerb = regexp.MustCompile(`%[#+\-0-9.]*[a-zA-Z]`)

// TestSQLFragmentsIntroduceNoBareIdentifiers 扫描包内全部非测试源文件的字符串
// 字面量，凡是 SQL 片段（含任一已登记的 SQL 关键字），其中出现的每个标识符样式的
// 词都必须是已登记的关键字——不得有裸标识符。
//
// 扫【整个包目录】而不是写死 query.go / lookup.go：新增的文件会自动纳入，
// 否则「守将来」这个目的会被「将来新建一个文件」绕过。
func TestSQLFragmentsIntroduceNoBareIdentifiers(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, name, nil, 0)
			require.NoError(t, err)

			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				words := identifierWords(s)
				if !hasSQLKeyword(words) {
					return true // 不是 SQL 片段——错误消息、Verdict 名字等
				}
				for _, w := range words {
					assert.True(t, sqlKeywords[strings.ToUpper(w)],
						"%s:%d 的 SQL 片段 %q 里有裸标识符 %q。进入 SQL 的标识符只能来自 "+
							"Spec 的未导出字段，或是过了 identRE 的包级常量（如 queryAlias）；"+
							"若 %q 其实是 SQL 关键字，把它登记进 sqlKeywords",
						name, fset.Position(lit.Pos()).Line, s, w, w)
				}
				return true
			})
		})
	}
	// 扫描目标非空是这条测试的前提：若目录读取方式将来变了导致一个文件都没扫到，
	// 整条测试会静默全绿——那正是它要防的那类失效。
	require.NotZero(t, scanned, "没有扫描到任何非测试源文件，本测试是空的")
}

// identifierWords 返回 s 中所有标识符样式的词（剔除 fmt 动词与纯数字）。
func identifierWords(s string) []string {
	s = formatVerb.ReplaceAllString(s, " ")
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
	}) {
		if strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue // 纯数字不是标识符
		}
		out = append(out, w)
	}
	return out
}

func hasSQLKeyword(words []string) bool {
	for _, w := range words {
		if sqlKeywords[strings.ToUpper(w)] {
			return true
		}
	}
	return false
}
