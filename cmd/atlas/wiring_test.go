package main

// Context Checkpoint: done_criteria → test mapping
// fix_items[S1] "serve.go:85 的 initPolicyGate(cfg, log) 接线零测试守护"
//   → TestServeWiresPolicyGateBeforeCollectors（阴性：扫真实 serve.go）
//     + TestScanWiringDetectsMissingInitCall（阳性：漏调用被检出）
//     + TestScanWiringDetectsWrongOrder（阳性：顺序颠倒被检出）
//     + TestScanWiringReportsMissingFile / TestScanWiringReportsParseFailure（错误路径）
//
// 为什么需要这条：serve 不经 loadConfigOrDefaults（那条路径由 TASK-006 的
// TestLoadConfigOrDefaultsInitsPolicyGate 守护），它在 runServe 里单独接线。
// QA 变异取证：把 serve.go:85 换成 `_ = log` 后 `go test ./cmd/atlas/` 仍然 ok ——
// **serve 下的 cache.enabled / cache.ttl / 整个 topics 覆盖会被静默忽略**，
// 影响面比配额失效更广。
//
// 为什么用 AST 而不是 grep：注释里提到 initPolicyGate 是正常的（serve.go:83-84
// 就有两行说明），grep 会命中它们；判据是「runServe 的函数体内存在这个调用」。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// wiringOrder 记录目标函数体内两个关键调用的行号。0 表示未找到。
//
// 两个字段都要带出来而不是只返回一个 bool：offenders 式的单值无法区分
// 「没调用」与「调用了但顺序错」，而这两种失效的修法不同。
type wiringOrder struct {
	initGateLine int // initPolicyGate(...) 的调用行
	buildLine    int // buildCollectors(...) 的调用行
	found        bool
}

// scanWiring 解析 src（src 为 nil 时读 filename 指向的真实文件），在名为 fnName
// 的**函数体内**定位 initPolicyGate 与 buildCollectors 的调用行号。
//
// src 参数化是为了把变异验证内化成可重放用例：阳性对照用内联源码即可，不必新建
// testdata 夹具文件（那会引入一个未被 writes 声明的子目录路径）。
//
// 行号一律**现读**，不写死 —— serve.go 一改行号就漂移，写死等于给自己埋一个
// 与被测行为无关的红。
func scanWiring(filename string, src any, fnName string) (wiringOrder, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return wiringOrder{}, err
	}

	var out wiringOrder
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != fnName || fn.Body == nil {
			continue
		}
		out.found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			line := fset.Position(call.Lparen).Line
			switch ident.Name {
			case "initPolicyGate":
				if out.initGateLine == 0 {
					out.initGateLine = line
				}
			case "buildCollectors":
				if out.buildLine == 0 {
					out.buildLine = line
				}
			}
			return true
		})
	}
	return out, nil
}

// TestServeWiresPolicyGateBeforeCollectors 是阴性测试：真实的 serve.go 必须通过。
func TestServeWiresPolicyGateBeforeCollectors(t *testing.T) {
	got, err := scanWiring("serve.go", nil, "runServe")
	if err != nil {
		t.Fatalf("解析 serve.go: %v", err)
	}
	// 「找到了 runServe」是下界：函数改名或文件重构后扫描会一无所获，
	// 那时下面两条断言会 vacuously 通过（契约陷阱 6）。
	if !got.found {
		t.Fatal("serve.go 里找不到 runServe —— 扫描没扫到东西，下面的断言无意义")
	}
	if got.initGateLine == 0 {
		t.Fatal("runServe 函数体内没有 initPolicyGate 调用：" +
			"serve 不经 loadConfigOrDefaults，漏掉这一行会让 cache.enabled / cache.ttl / " +
			"collector.topics 全部被静默忽略，而所有既有测试照常绿")
	}
	if got.buildLine == 0 {
		t.Fatal("runServe 函数体内没有 buildCollectors 调用 —— 扫描判据可能已过时")
	}
	if got.initGateLine >= got.buildLine {
		t.Errorf("initPolicyGate 在第 %d 行、buildCollectors 在第 %d 行："+
			"闸门的**词法位置**须在 collector 构造之前（各 collector 在构造函数里取 "+
			"policy.Default()，晚了就拿到懒构造的空表 Gate）", got.initGateLine, got.buildLine)
	}
	// ⚠ 强度边界：本测试断言的是**源码里的词法位置**，不是执行顺序的承诺。
	// 把 initPolicyGate 包进 defer/go 闭包、恒假 if、或写在提前 return 之后的
	// 死代码里，词法位置照样在前而运行期根本不执行——那几类不在本测试的射程内。
	// 运行期证据见 policy_test.go 的 TestLoadConfigOrDefaultsInitsPolicyGate
	// 与 gate_wiring_test.go 里三条入口的运行期观测。
}

// runServeSrc 生成一段最小的 runServe 源码，body 由调用方给定。
func runServeSrc(body string) string {
	return "package main\n\nfunc runServe() error {\n" + body + "\n\treturn nil\n}\n"
}

// TestScanWiringDetectsMissingInitCall 是阳性对照：漏掉 initPolicyGate 必须被检出。
//
// 这一格证明的是「扫描器真的会报」，没有它，阴性测试的绿既可能是「代码正确」，
// 也可能是「扫描器根本不工作」。
func TestScanWiringDetectsMissingInitCall(t *testing.T) {
	src := runServeSrc("\tcleanup, err := buildCollectors(cfg, application, log)\n\t_ = cleanup\n\t_ = err")

	got, err := scanWiring("fake_serve.go", src, "runServe")
	if err != nil {
		t.Fatalf("解析夹具: %v", err)
	}
	if !got.found {
		t.Fatal("夹具里应能找到 runServe")
	}
	if got.initGateLine != 0 {
		t.Errorf("夹具没有 initPolicyGate，却报出第 %d 行", got.initGateLine)
	}
	if got.buildLine == 0 {
		t.Error("夹具里的 buildCollectors 应被找到（否则这格对照本身就没生效）")
	}
}

// TestScanWiringDetectsWrongOrder 是阳性对照：顺序颠倒必须能被区分出来。
func TestScanWiringDetectsWrongOrder(t *testing.T) {
	src := runServeSrc("\tcleanup, err := buildCollectors(cfg, application, log)\n\tinitPolicyGate(cfg, log)\n\t_ = cleanup\n\t_ = err")

	got, err := scanWiring("fake_serve.go", src, "runServe")
	if err != nil {
		t.Fatalf("解析夹具: %v", err)
	}
	if got.initGateLine == 0 || got.buildLine == 0 {
		t.Fatalf("两个调用都应被找到, got %+v", got)
	}
	if got.initGateLine <= got.buildLine {
		t.Errorf("夹具是颠倒顺序，扫描结果却是 init(%d) <= build(%d) —— "+
			"顺序判据失效，真实文件里顺序写错也测不出来", got.initGateLine, got.buildLine)
	}
}

// TestScanWiringIgnoresOtherFunctions 守护「判据是 runServe 的函数体内」：
// 别的函数里调用 initPolicyGate 不算数（loadConfigOrDefaults 就有一处）。
func TestScanWiringIgnoresOtherFunctions(t *testing.T) {
	src := "package main\n\n" +
		"func other() { initPolicyGate(cfg, nil) }\n\n" +
		"func runServe() error {\n\tcleanup, err := buildCollectors(cfg, application, log)\n\t_ = cleanup\n\t_ = err\n\treturn nil\n}\n"

	got, err := scanWiring("fake_serve.go", src, "runServe")
	if err != nil {
		t.Fatalf("解析夹具: %v", err)
	}
	if got.initGateLine != 0 {
		t.Errorf("别的函数里的 initPolicyGate 不应计入 runServe, got 第 %d 行", got.initGateLine)
	}
}

func TestScanWiringReportsMissingFile(t *testing.T) {
	if _, err := scanWiring("no_such_file.go", nil, "runServe"); err == nil {
		t.Error("文件不存在时必须报错，不得静默返回空结果")
	}
}

func TestScanWiringReportsParseFailure(t *testing.T) {
	_, err := scanWiring("fake_serve.go", "package main\nfunc runServe( {", "runServe")
	if err == nil {
		t.Fatal("语法错误时必须报错")
	}
	if !strings.Contains(err.Error(), "fake_serve.go") {
		t.Errorf("错误信息应指出出错的文件, got %v", err)
	}
}
