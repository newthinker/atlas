package hestia

// Context Checkpoint: done_criteria → test mapping (fields)
// functional[0]      分组计数符合附录 A（tsf_ 36 / 货币 6 / deposit_ 12 / loan_ 18 / rate_+fx_ 4）
//                                                        → TestFieldGroupCounts
// functional[1]      allFields 与 fieldOrder 元素完全一致 → TestAllFieldsMatchesFieldOrder
// functional[2]      golden list：逐字写出的 76 元素、顺序敏感 → TestFieldOrderGoldenList
//                    + 常量名↔值的绑定（返工 P1，变异 M11）  → TestFieldConstantBindings
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

// goldenFields 是规格侧的 76 条 golden 记录：**左侧是常量、右侧是字面量**，
// 顺序与 fieldOrder 必须完全一致。它同时钉住两件独立的事：
//
//	want 之间的顺序 → fieldOrder 的值序列（TestFieldOrderGoldenList）
//	got ↔ want 的配对 → 哪个常量名对应哪个值（TestFieldConstantBindings）
//
// want 侧刻意是字符串字面量而不是 Field* 常量：整份清单都用常量写等于把同一个值
// 抄给自己看，常量值里的拼写错误（deposit_corp_ytd 写成 deposit_corp_yt）会同时
// 出现在断言两边而全绿通过。那种错仍然唯一、合法、前缀正确、分组计数不变，包内
// 又完全自洽（DDL 与 INSERT 都从同一常量派生），要等下游编译或 Grafana 列名对不上
// 才暴露，那时改名已经需要数据迁移。
//
// 一侧常量一侧字面量则**不是自指**：got 来自实现，want 来自规格，两者独立可比。
var goldenFields = []struct {
	got  string // 实现侧：Field* 常量
	want string // 规格侧：逐字写出的字面量
}{
	{FieldTSFStock, "tsf_stock"},
	{FieldTSFStockYoY, "tsf_stock_yoy"},
	{FieldTSFFlowYTD, "tsf_flow_ytd"},
	{FieldTSFStockRMBLoan, "tsf_stock_rmb_loan"},
	{FieldTSFStockRMBLoanYoY, "tsf_stock_rmb_loan_yoy"},
	{FieldTSFStockFXLoan, "tsf_stock_fx_loan"},
	{FieldTSFStockFXLoanYoY, "tsf_stock_fx_loan_yoy"},
	{FieldTSFStockEntrust, "tsf_stock_entrust"},
	{FieldTSFStockEntrustYoY, "tsf_stock_entrust_yoy"},
	{FieldTSFStockTrust, "tsf_stock_trust"},
	{FieldTSFStockTrustYoY, "tsf_stock_trust_yoy"},
	{FieldTSFStockBankAccept, "tsf_stock_bankaccept"},
	{FieldTSFStockBankAcceptYoY, "tsf_stock_bankaccept_yoy"},
	{FieldTSFStockCorpBond, "tsf_stock_corp_bond"},
	{FieldTSFStockCorpBondYoY, "tsf_stock_corp_bond_yoy"},
	{FieldTSFStockGovtBond, "tsf_stock_govt_bond"},
	{FieldTSFStockGovtBondYoY, "tsf_stock_govt_bond_yoy"},
	{FieldTSFStockEquity, "tsf_stock_equity"},
	{FieldTSFStockEquityYoY, "tsf_stock_equity_yoy"},
	{FieldTSFFlowRMBLoanYTD, "tsf_flow_rmb_loan_ytd"},
	{FieldTSFFlowGovtBondYTD, "tsf_flow_govt_bond_ytd"},
	{FieldTSFFlowCorpBondYTD, "tsf_flow_corp_bond_ytd"},
	{FieldTSFFlowFXLoanYTD, "tsf_flow_fx_loan_ytd"},
	{FieldTSFFlowEntrustYTD, "tsf_flow_entrust_ytd"},
	{FieldTSFFlowTrustYTD, "tsf_flow_trust_ytd"},
	{FieldTSFFlowBankAcceptYTD, "tsf_flow_bankaccept_ytd"},
	{FieldTSFFlowEquityYTD, "tsf_flow_equity_ytd"},
	{FieldTSFFlowMoM, "tsf_flow_mom"},
	{FieldTSFFlowRMBLoanMoM, "tsf_flow_rmb_loan_mom"},
	{FieldTSFFlowGovtBondMoM, "tsf_flow_govt_bond_mom"},
	{FieldTSFFlowCorpBondMoM, "tsf_flow_corp_bond_mom"},
	{FieldTSFFlowFXLoanMoM, "tsf_flow_fx_loan_mom"},
	{FieldTSFFlowEntrustMoM, "tsf_flow_entrust_mom"},
	{FieldTSFFlowTrustMoM, "tsf_flow_trust_mom"},
	{FieldTSFFlowBankAcceptMoM, "tsf_flow_bankaccept_mom"},
	{FieldTSFFlowEquityMoM, "tsf_flow_equity_mom"},
	{FieldM2, "m2"},
	{FieldM2YoY, "m2_yoy"},
	{FieldM1, "m1"},
	{FieldM1YoY, "m1_yoy"},
	{FieldM0, "m0"},
	{FieldM0YoY, "m0_yoy"},
	{FieldDepositBalance, "deposit_balance"},
	{FieldDepositBalanceYoY, "deposit_balance_yoy"},
	{FieldDepositFlowYTD, "deposit_flow_ytd"},
	{FieldDepositHouseholdYTD, "deposit_household_ytd"},
	{FieldDepositCorpYTD, "deposit_corp_ytd"},
	{FieldDepositFiscalYTD, "deposit_fiscal_ytd"},
	{FieldDepositNBFIYTD, "deposit_nbfi_ytd"},
	{FieldDepositFlowMoM, "deposit_flow_mom"},
	{FieldDepositHouseholdMoM, "deposit_household_mom"},
	{FieldDepositCorpMoM, "deposit_corp_mom"},
	{FieldDepositFiscalMoM, "deposit_fiscal_mom"},
	{FieldDepositNBFIMoM, "deposit_nbfi_mom"},
	{FieldLoanBalance, "loan_balance"},
	{FieldLoanBalanceYoY, "loan_balance_yoy"},
	{FieldLoanFlowYTD, "loan_flow_ytd"},
	{FieldLoanHHShortYTD, "loan_hh_short_ytd"},
	{FieldLoanHHMLTYTD, "loan_hh_mlt_ytd"},
	{FieldLoanCorpTotalYTD, "loan_corp_total_ytd"},
	{FieldLoanCorpShortYTD, "loan_corp_short_ytd"},
	{FieldLoanCorpMLTYTD, "loan_corp_mlt_ytd"},
	{FieldLoanBillYTD, "loan_bill_ytd"},
	{FieldLoanNBFIYTD, "loan_nbfi_ytd"},
	{FieldLoanFlowMoM, "loan_flow_mom"},
	{FieldLoanHHShortMoM, "loan_hh_short_mom"},
	{FieldLoanHHMLTMoM, "loan_hh_mlt_mom"},
	{FieldLoanCorpTotalMoM, "loan_corp_total_mom"},
	{FieldLoanCorpShortMoM, "loan_corp_short_mom"},
	{FieldLoanCorpMLTMoM, "loan_corp_mlt_mom"},
	{FieldLoanBillMoM, "loan_bill_mom"},
	{FieldLoanNBFIMoM, "loan_nbfi_mom"},
	{FieldRateIBO, "rate_ibo"},
	{FieldRateRepo, "rate_repo"},
	{FieldFXReserve, "fx_reserve"},
	{FieldFXRate, "fx_rate"},
}

// TestFieldConstantBindings 钉住「哪个常量名对应哪个值」。
//
// 值序列的 golden list 挡不住这一类错：同时交换两个常量的值与它们在 fieldOrder 里的
// 位置，值序列逐字不变、计数不变、去重与标识符合法性都不变，整套测试全绿，而此时
// FieldDepositCorpYTD 已经等于 "deposit_fiscal_ytd"。下游拿它当 Values 的键，
// 企业存款就静默写进财政存款列——无编译错、无运行错、无测试红。
func TestFieldConstantBindings(t *testing.T) {
	require.Len(t, goldenFields, 76, "绑定表自身必须是 76 条——它写错了后面的断言全无意义")
	for i, g := range goldenFields {
		assert.Equalf(t, g.want, g.got,
			"goldenFields[%d]：常量与值的绑定错位，该位置的常量应当等于 %q", i, g.want)
	}
}

func TestFieldOrderGoldenList(t *testing.T) {
	// 期望序列只由 want 构造。用 got 构造会让这条断言退化成自指：
	// 常量值怎么变，期望值就跟着怎么变，恒真。
	wantOrder := make([]string, len(goldenFields))
	for i, g := range goldenFields {
		wantOrder[i] = g.want
	}
	assert.Equal(t, wantOrder, fieldOrder, "fieldOrder 与 golden list 必须逐字逐序相等")
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
	assert.Equal(t, 36, count(func(f string) bool { return strings.HasPrefix(f, "tsf_") }), "A.1 社融")
	assert.Equal(t, 6, count(func(f string) bool {
		return f == "m0" || f == "m1" || f == "m2" ||
			f == "m0_yoy" || f == "m1_yoy" || f == "m2_yoy"
	}), "A.2 货币供应量")
	assert.Equal(t, 12, count(func(f string) bool { return strings.HasPrefix(f, "deposit_") }), "A.3 存款")
	assert.Equal(t, 18, count(func(f string) bool { return strings.HasPrefix(f, "loan_") }), "A.4 贷款")
	assert.Equal(t, 4, count(func(f string) bool {
		return strings.HasPrefix(f, "rate_") || strings.HasPrefix(f, "fx_")
	}), "A.5 利率与外部")

	// 分组计数之和必须正好是全部字段，否则「某组多一个、另一组少一个」之外
	// 还有第三种漏网：一个既不属于任何分组、也不影响上面五条断言的野字段。
	//
	// M1c-4 的 TASK-004：27+7+10 → 36+12+18，三族各自加上当月口径列（9/5/8）。
	// **仍写成分族求和而不是裸 76**——上面那句才是这个写法的理由。
	require.Len(t, fieldOrder, 36+6+12+18+4)
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
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			// 只看字面量与**字面量之间的拼接**："deposit_" + "corp_ytd" 拼出的
			// 也是写死的字段名，按 AST 逐个 BasicLit 比对会漏掉。
			v, ok := foldStringLiteral(expr)
			if !ok {
				return true
			}
			assert.Falsef(t, allFields[v],
				"%s:%d 出现业务字段名字面量 %q——字段清单只写一次，请遍历 fieldOrder",
				name, fset.Position(expr.Pos()).Line, v)
			return true
		})
	}
}

// foldStringLiteral 把纯字面量表达式折叠成它的值：单个字符串字面量，或全部
// 操作数都是字面量的 + 拼接。任一操作数不是字面量（变量、函数调用、fmt.Sprintf、
// []byte 往返）就返回 false——那些形态本检查抓不到，是已知的捕获上限，不是遗漏。
func foldStringLiteral(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(x.Value)
		return v, err == nil
	case *ast.ParenExpr:
		return foldStringLiteral(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, lok := foldStringLiteral(x.X)
		r, rok := foldStringLiteral(x.Y)
		return l + r, lok && rok
	}
	return "", false
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

// —— M1c-4 的 TASK-004：当月口径（*_mom）列 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-4 的 TASK-004）
// functional[0] 22 个常量在 fieldOrder 里、以 _mom 结尾、总数 76
//                                                → TestMomFieldsExistForEveryFlowFamily
// functional[0] 每个 _mom 都有 _ytd 孪生列，计数 22
//                                                → TestEveryMomFieldHasAYTDTwin
// boundary[0]   四条既有计数断言同步更新（36/6/12/18/4）
//                                                → TestFieldGroupCounts（上方）

// TestMomFieldsExistForEveryFlowFamily：三个流量族都必须有当月口径的对应列。
//
// 存在的理由：真语料 57 篇「处理不了」的报告里 54 篇是同一个缺口 —— 央行在
// 2020–2023 的相当多期次里，合计给累计、分部门给当月，而 schema 只有 *_ytd。
// 解析器为此**拒绝整篇**（拒绝是对的：混口径的值量级完全合理，下游拦不住）。
func TestMomFieldsExistForEveryFlowFamily(t *testing.T) {
	want := []string{
		FieldTSFFlowMoM, FieldTSFFlowRMBLoanMoM, FieldTSFFlowFXLoanMoM,
		FieldTSFFlowEntrustMoM, FieldTSFFlowTrustMoM, FieldTSFFlowBankAcceptMoM,
		FieldTSFFlowCorpBondMoM, FieldTSFFlowGovtBondMoM, FieldTSFFlowEquityMoM,

		FieldDepositFlowMoM, FieldDepositHouseholdMoM, FieldDepositCorpMoM,
		FieldDepositFiscalMoM, FieldDepositNBFIMoM,

		FieldLoanFlowMoM, FieldLoanHHShortMoM, FieldLoanHHMLTMoM,
		FieldLoanCorpTotalMoM, FieldLoanCorpShortMoM, FieldLoanCorpMLTMoM,
		FieldLoanBillMoM, FieldLoanNBFIMoM,
	}
	require.Len(t, want, 22)
	for _, f := range want {
		require.Contains(t, fieldOrder, f, "%s 必须在 fieldOrder 里（DDL 由它派生）", f)
		require.True(t, strings.HasSuffix(f, "_mom"), "%s 必须以 _mom 结尾", f)
	}
	require.Len(t, fieldOrder, 76, "54 个既有字段 + 22 个当月口径字段")
}

// TestEveryMomFieldHasAYTDTwin：每个 _mom 字段都要有同名的 _ytd 孪生列。
//
// 这条守的是 spec §5.2 那条路由断言的前提：|ytd| >= |mom| 只有在两列成对时才算得
// 出来。单方面加一个 _mom 而没有对应 _ytd，会让那条断言对它永远 skip 且无人察觉。
func TestEveryMomFieldHasAYTDTwin(t *testing.T) {
	have := make(map[string]bool, len(fieldOrder))
	for _, f := range fieldOrder {
		have[f] = true
	}
	var n int
	for _, f := range fieldOrder {
		if !strings.HasSuffix(f, "_mom") {
			continue
		}
		n++
		require.True(t, have[strings.TrimSuffix(f, "_mom")+"_ytd"],
			"%s 缺少 _ytd 孪生列", f)
	}
	require.Equal(t, 22, n)
}
