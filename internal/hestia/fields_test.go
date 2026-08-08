package hestia

// Context Checkpoint: done_criteria → test mapping (fields)
// functional[0]      分组计数符合附录 A（tsf_ 27 / 货币 6 / deposit_ 7 / loan_ 10 / rate_+fx_ 4）
//                                                        → TestFieldGroupCounts
// functional[1]      allFields 与 fieldOrder 元素完全一致 → TestAllFieldsMatchesFieldOrder
// functional[2]      golden list：逐字写出的 54 元素、顺序敏感 → TestFieldOrderGoldenList
// boundary[0]        fieldOrder 内无重复                  → TestFieldOrderHasNoDuplicates
// error_handling[0]  每个字段名匹配 ^[a-z][a-z0-9_]*$     → TestFieldNamesAreValidIdentifiers
// non_functional[0]  fields.go 之外的非 _test.go 文件不得出现业务字段名字面量
//                                                        → TestFieldNamesAppearOnlyInFieldsGo
// non_functional[1]  包注释写明单位约定与 breaking change 声明
//                                                        → TestPackageDocDeclaresUnits

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goldenFieldOrder 是 54 个字段名的逐字副本，顺序与 fieldOrder 必须完全一致。
//
// 刻意用字符串字面量而不是 Field* 常量：用常量写这份清单等于把同一个值抄给自己看，
// 常量值里的拼写错误（deposit_corp_ytd 写成 deposit_corp_yt）会同时出现在两边而全绿通过。
// 那种错仍然唯一、合法、前缀正确、分组计数不变，包内又完全自洽（DDL 与 INSERT 都从
// 同一常量派生），要等下游编译或 Grafana 列名对不上才暴露，那时改名已经需要数据迁移。
var goldenFieldOrder = []string{
	"tsf_stock",
	"tsf_stock_yoy",
	"tsf_flow_ytd",
	"tsf_stock_rmb_loan",
	"tsf_stock_rmb_loan_yoy",
	"tsf_stock_fx_loan",
	"tsf_stock_fx_loan_yoy",
	"tsf_stock_entrust",
	"tsf_stock_entrust_yoy",
	"tsf_stock_trust",
	"tsf_stock_trust_yoy",
	"tsf_stock_bankaccept",
	"tsf_stock_bankaccept_yoy",
	"tsf_stock_corp_bond",
	"tsf_stock_corp_bond_yoy",
	"tsf_stock_govt_bond",
	"tsf_stock_govt_bond_yoy",
	"tsf_stock_equity",
	"tsf_stock_equity_yoy",
	"tsf_flow_rmb_loan_ytd",
	"tsf_flow_govt_bond_ytd",
	"tsf_flow_corp_bond_ytd",
	"tsf_flow_fx_loan_ytd",
	"tsf_flow_entrust_ytd",
	"tsf_flow_trust_ytd",
	"tsf_flow_bankaccept_ytd",
	"tsf_flow_equity_ytd",
	"m2",
	"m2_yoy",
	"m1",
	"m1_yoy",
	"m0",
	"m0_yoy",
	"deposit_balance",
	"deposit_balance_yoy",
	"deposit_flow_ytd",
	"deposit_household_ytd",
	"deposit_corp_ytd",
	"deposit_fiscal_ytd",
	"deposit_nbfi_ytd",
	"loan_balance",
	"loan_balance_yoy",
	"loan_flow_ytd",
	"loan_hh_short_ytd",
	"loan_hh_mlt_ytd",
	"loan_corp_total_ytd",
	"loan_corp_short_ytd",
	"loan_corp_mlt_ytd",
	"loan_bill_ytd",
	"loan_nbfi_ytd",
	"rate_ibo",
	"rate_repo",
	"fx_reserve",
	"fx_rate",
}

func TestFieldOrderGoldenList(t *testing.T) {
	require.Len(t, goldenFieldOrder, 54, "golden list 自身必须是 54 个——它写错了后面的断言全无意义")
	assert.Equal(t, goldenFieldOrder, fieldOrder, "fieldOrder 与 golden list 必须逐字逐序相等")
}

func TestFieldGroupCounts(t *testing.T) {
	count := func(pred func(string) bool) int {
		n := 0
		for _, f := range fieldOrder {
			if pred(f) {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 27, count(func(f string) bool { return strings.HasPrefix(f, "tsf_") }), "A.1 社融")
	assert.Equal(t, 6, count(func(f string) bool {
		return f == "m0" || f == "m1" || f == "m2" ||
			f == "m0_yoy" || f == "m1_yoy" || f == "m2_yoy"
	}), "A.2 货币供应量")
	assert.Equal(t, 7, count(func(f string) bool { return strings.HasPrefix(f, "deposit_") }), "A.3 存款")
	assert.Equal(t, 10, count(func(f string) bool { return strings.HasPrefix(f, "loan_") }), "A.4 贷款")
	assert.Equal(t, 4, count(func(f string) bool {
		return strings.HasPrefix(f, "rate_") || strings.HasPrefix(f, "fx_")
	}), "A.5 利率与外部")

	// 分组计数之和必须正好是全部字段，否则「某组多一个、另一组少一个」之外
	// 还有第三种漏网：一个既不属于任何分组、也不影响上面五条断言的野字段。
	require.Len(t, fieldOrder, 27+6+7+10+4)
}

func TestAllFieldsMatchesFieldOrder(t *testing.T) {
	assert.Equal(t, len(fieldOrder), len(allFields), "allFields 与 fieldOrder 数量必须一致")
	for _, f := range fieldOrder {
		assert.True(t, allFields[f], "allFields 缺少 %s", f)
	}
}

func TestFieldOrderHasNoDuplicates(t *testing.T) {
	// 重复字段会让 DDL 生成重复列而建表失败，那是运行期才暴露的错，必须在此拦住。
	seen := map[string]bool{}
	for _, f := range fieldOrder {
		require.False(t, seen[f], "字段重复：%s", f)
		seen[f] = true
	}
}

func TestFieldNamesAreValidIdentifiers(t *testing.T) {
	// 字段名会拼进 DDL 与 INSERT 的列名，与 bitemporal 的 identRE 同一把关态度：
	// 这是本包注入面的第一道闸门。
	re := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, f := range fieldOrder {
		assert.Regexp(t, re, f, "字段名必须是小写下划线形式的合法标识符")
	}
}

// TestFieldNamesAppearOnlyInFieldsGo 机械核验「唯一真相源」：本包除 fields.go 外的
// 非测试文件不得出现业务字段名的字符串字面量——schema/store 必须遍历 fieldOrder。
//
// 用 AST 取字符串字面量而不是 grep 文本：grep 会把注释里的 m0/m2 也算进去而误报，
// 也会因 "m2" 是 "m2_yoy" 的子串而错判。测试文件豁免——golden list 与分组计数
// 本身就必须写字面量，否则这条判据自相矛盾。
func TestFieldNamesAppearOnlyInFieldsGo(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" ||
			strings.HasSuffix(name, "_test.go") || name == "fields.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err, "解析 %s", name)

		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			assert.Falsef(t, allFields[v],
				"%s:%d 出现业务字段名字面量 %q——字段清单只写一次，请遍历 fieldOrder",
				name, fset.Position(lit.Pos()).Line, v)
			return true
		})
	}
}

// TestPackageDocDeclaresUnits 核验包注释声明了单位约定。单位不入库、也没有
// units_version 列，唯一记录它的地方就是这段注释——注释掉了等于口径丢失。
func TestPackageDocDeclaresUnits(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fields.go", nil, parser.ParseComments)
	require.NoError(t, err)
	require.NotNil(t, f.Doc, "fields.go 必须有包注释")

	doc := f.Doc.Text()
	for _, want := range []string{"万亿元", "亿元", "百分数", "breaking change"} {
		assert.Contains(t, doc, want, "包注释缺少单位/变更约定：%s", want)
	}
}
