package bitemporal

// Context Checkpoint: done_criteria → test mapping (TASK-006)
// error_handling[1] 进入 SQL 的标识符只能来自 Spec 的未导出字段或过了 identRE 的
//                   包级常量——包括【将来加进来的】 → TestSQLFragmentsIntroduceNoBareIdentifiers
//
// 为什么需要这条：包里原有三处守卫——TASK-001 的 identRE（非法标识符进不了 Spec）、
// TASK-004 的来源核查 + queryAlias 过闸门、TASK-005 的复用 Spec——**守的都是「当前
// 代码正确」，没有一处守「将来加进来的东西也得正确」**。实证：给 CurrentQuery 的格式串
// 加一个 `ORDER BY rowid`（rowid 不来自任何 Spec），全套 120 条测试无一转红。
//
// 讽刺的是，TASK-004 机制化 alias 时给出的理由原话就是「逐个核对是一次性人工动作，
// 下次有人加个 ORDER BY 列就失效了」——举的那个例子恰恰是当时的新机制不覆盖的。
// 这条测试补上那一半。

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
// 刻意只列用到的这几个，而不是抄一份完整的 SQL 关键字表：任何新增的 SQL 语法都会让
// 下面的测试把它报成「裸标识符」，迫使加它的人来这里显式登记——**那一刻正是审视
// 「这个标识符来自哪里」的时机**。白名单越短，这道摩擦越有效。
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
