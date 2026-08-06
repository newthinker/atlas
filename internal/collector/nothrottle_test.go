package collector

// Context Checkpoint: done_criteria → test mapping
// functional[0] "AST 解析**整棵 internal/collector 树**的生产源码（跳过 _test.go /
//                policy/ / testdata/），断言不存在 lastReq 字段（验收标准 5）；
//                扫描须抽成 scan(root string)"
//                                    → TestNoPrivateThrottleState
//                                      （root 参数化由阳性对照复用同一函数实证）
// functional[1] "变异验证内化为可重放用例：假源码须被检出、生产树须不检出"
//                                    → TestScanDetectsThrottleState（阳性）
//                                      + TestNoPrivateThrottleState（阴性）
// boundary[0]   "只覆盖生产源码，不误报测试文件中的同名标识符"
//                                    → TestScanIgnoresNonFieldOccurrences
// error[0]      "目录不存在或解析失败时清晰失败，不 panic 不静默跳过"
//                                    → TestScanReportsParseFailure / TestScanReportsMissingRoot

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbiddenFields 是「私有节流状态」的字段名黑名单（设计 §7.4 / 验收标准 5）。
//
// DoD 点名的是 lastReq；minInterval 与 throttle 一并禁掉是**免费的加固** ——
// 三者在当前生产源码中的检出数都是 0（接任务时实测），且它们正是被本 Sprint
// 删掉的那套私有节流状态的三个组成部分。
var forbiddenFields = map[string]bool{
	"lastReq":     true,
	"minInterval": true,
	"throttle":    true,
}

// skipDirs 是**生产扫描**要跳过的目录名。
//
//   - policy —— 闸门本体，它持有唯一合法的 lastReq，那里是实现而非回潮。
//   - testdata —— 本文件自己的夹具就住在那儿，含**故意**写成回潮形态的源码
//     和一个**故意**语法错误的文件；不跳过的话生产扫描会把阳性对照当成
//     offender、并被解析失败夹具直接打断。
//
// ⚠ 跳过规则**对 root 自身不生效**：否则把 root 指进 testdata 做阳性对照时，
// WalkDir 第一次回调拿到的就是 root 且名字叫 testdata，SkipDir 掉整棵子树 →
// 对照返回空 → **对照静默退化成一条什么都没测的绿**。
//
// ⚠ testdata 这条规则在本文件出现之前**没有任何测试守护**（全树原本 0 个
// testdata/*.go），删掉它不会有人发现。现在它由 TestNoPrivateThrottleState
// 承重：去掉后该测试会因解析失败夹具而直接报错。
var skipDirs = map[string]bool{"policy": true, "testdata": true}

// scanResult 同时带出「检出了什么」与「扫了些什么」。
//
// scanned 不是调试信息而是**断言材料**：offenders 为空既可能是「真干净」，
// 也可能是「扫描器一个文件都没走」，两者在输出上完全一样（空真，契约陷阱 6）。
// 量词「不存在 lastReq」必须先有下界。
type scanResult struct {
	offenders []string // "相对路径:行号"
	scanned   []string // 实际被解析的生产源码相对路径
}

// scanPrivateThrottleState 从 root 起遍历 .go 生产源码（跳过 _test.go 与
// skipDirs），用 AST 找出名字命中 forbiddenFields 的**结构体字段**。
//
// 为什么是 AST 而不是 grep：注释、局部变量、测试文件里出于验证目的提到这些
// 名字是完全正常的（如 yahoo/gate_test.go 的节流预热注释），grep 会三个全中。
// 回潮的判据是「状态被结构体持有」。
//
// root 是参数而非写死的 "."：DoD functional[1] 要求把变异验证内化成可重放的
// 用例，而那需要把同一个扫描函数分别指向生产树与夹具树。
func scanPrivateThrottleState(root string) (scanResult, error) {
	var res scanResult
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // 目录不存在等：如实上抛，不静默跳过
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("解析 %s 失败: %w", path, perr)
		}
		res.scanned = append(res.scanned, path)
		forEachStructFieldName(f, func(name *ast.Ident) {
			if forbiddenFields[name.Name] {
				res.offenders = append(res.offenders,
					path+":"+strconv.Itoa(fset.Position(name.Pos()).Line))
			}
		})
		return nil
	})
	if err != nil {
		return scanResult{}, err
	}
	return res, nil
}

// forEachStructFieldName 对 f 里每个**结构体字段**的名字调用 fn。
//
// 「回潮的判据是状态被结构体持有」这条判据在两处要用（生产扫描、夹具行号现读），
// 收在一处可保证二者永远看的是同一批节点。
func forEachStructFieldName(f *ast.File, fn func(name *ast.Ident)) {
	ast.Inspect(f, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, fld := range st.Fields.List {
			for _, name := range fld.Names {
				fn(name)
			}
		}
		return true
	})
}

// scanNoPanic 把 panic 转成断言失败。
//
// 「不该 panic」型的 DoD（error_handling[0]）不能让 panic 裸奔：裸 panic 会
// **中断整个测试二进制**，同包其余测试根本跑不到，而汇总只显示「红了」——
// 红的方式决定了别的测试还能不能跑（教训 10）。
func scanNoPanic(t *testing.T, root string) (res scanResult, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("scanPrivateThrottleState(%q) panic 了: %v（应返回 error 而非 panic）", root, r)
		}
	}()
	return scanPrivateThrottleState(root)
}

// gateMigratedPkgs 是 DoD functional[0] 点名必须覆盖的四个子包。
var gateMigratedPkgs = []string{"yahoo", "twelvedata", "tushare", "lixinger"}

// TestNoPrivateThrottleState 是验收标准 5 的守护者：整棵 internal/collector 树
// 的生产源码里不得再有私有节流状态字段，限流一律走 policy 的 Gate。
//
// 范围是**全树**而非 DoD 字面点名的四个子包 —— 全树是四包的超集（四包各自被
// 扫到这一点由下面的下界断言显式钉住），且防回潮的价值恰恰在于挡住**将来新写
// 的** collector 把 mu+lastReq+sleep 抄回来；限定四包等于放过所有新增者。
//
// ⚠ **强度边界，勿高估**（设计 §7.4）：它只能挡住「结构体里有个叫 lastReq /
// minInterval / throttle 的字段」这一种形态。挡不住：改名字段（lastCall、last）、
// 包级 var、局部变量、裸 time.Sleep、以及「有 gate 字段但压根没调 Fetch」。
// 后两类的真实守护在各 collector 自己包里（构造函数取 Default() 的测试、
// 主题常量对内置表的断言），不在这个文件。
func TestNoPrivateThrottleState(t *testing.T) {
	res, err := scanNoPanic(t, ".")
	if err != nil {
		t.Fatalf("扫描 internal/collector 失败: %v", err)
	}

	if len(res.offenders) > 0 {
		t.Errorf("发现私有节流状态字段（限流须走 internal/collector/policy 的 Gate）:\n  %s",
			strings.Join(res.offenders, "\n  "))
	}

	// 下界断言：上面那条「没有 offender」在扫描器一个文件都没走时同样成立。
	// DoD 点名的四个子包必须**每个**都确实贡献了被解析的文件。
	for _, pkg := range gateMigratedPkgs {
		if !anyScannedIn(res.scanned, pkg) {
			t.Errorf("子包 %s 一个生产源码文件都没被扫到 —— 「不存在 lastReq」是空真的（共扫 %d 个文件）",
				pkg, len(res.scanned))
		}
	}
}

func anyScannedIn(scanned []string, pkg string) bool {
	for _, p := range scanned {
		if strings.HasPrefix(p, pkg+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// TestScanDetectsThrottleState 是 DoD functional[1] 的**阳性对照**：把同一个
// 扫描函数指向故意含回潮形态的夹具，必须检出。
//
// 它与 TestNoPrivateThrottleState 是 2×2 的两格，缺一不可 —— 只有阴性那格时，
// 一个恒返回空的扫描函数照样全绿；只有阳性那格时，一个恒返回非空的照样全绿。
//
// root 精确指向 throttleback 而非 testdata：夹具之间**不能共用 root**。
// WalkDir 按字典序走，parsefail 排在 throttleback 前面，指向 testdata 会在解析
// 失败处提前返回、根本走不到阳性夹具 —— 断言失败，而错误信息说的是「解析失败」，
// 指向的方向完全不对。
func TestScanDetectsThrottleState(t *testing.T) {
	root := filepath.Join("testdata", "throttleback")
	fixture := filepath.Join(root, "collector.go")
	res, err := scanNoPanic(t, root)
	if err != nil {
		t.Fatalf("扫描阳性夹具失败: %v", err)
	}
	if len(res.offenders) == 0 {
		t.Fatal("阳性夹具未被检出 —— 扫描函数没有守护任何东西（它可能恒返回空）")
	}
	// 逐个禁用字段都要被点出来，且带上可定位的 file:line。
	for _, want := range []string{"lastReq", "minInterval"} {
		if !hasOffenderFor(t, res.offenders, fixture, want) {
			t.Errorf("字段 %s 未被检出，offenders=%v", want, res.offenders)
		}
	}
}

// hasOffenderFor 确认 offenders 里存在一条指向 file 的记录，且其行号处的源码
// 确实是 field 那一行 —— 只比对文件名会让行号错位静默通过。
func hasOffenderFor(t *testing.T, offenders []string, file, field string) bool {
	t.Helper()
	wantLine := lineOfField(t, file, field)
	want := filepath.ToSlash(file) + ":" + strconv.Itoa(wantLine)
	for _, o := range offenders {
		if filepath.ToSlash(o) == want {
			return true
		}
	}
	return false
}

// lineOfField 从夹具源码里现读 field 所在行号，不写死字面量 —— 夹具一改行，
// 写死的期望值就会变成假红（锚点文本要现读现取）。
func lineOfField(t *testing.T, file, field string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("解析夹具 %s: %v", file, err)
	}
	line := 0
	forEachStructFieldName(f, func(name *ast.Ident) {
		if name.Name == field {
			line = fset.Position(name.Pos()).Line
		}
	})
	if line == 0 {
		t.Fatalf("夹具 %s 里找不到字段 %s —— 夹具本身失效了", file, field)
	}
	return line
}

// TestScanRootExemptFromSkipRule 守护「跳过规则对 root 自身不生效」这一条。
//
// 它把扫描器指向**真实的 policy 包** —— 那里有本仓库唯一合法的 lastReq
// （gate.go 的 domainState）。两件事一次钉住：
//
//  1. root 豁免确实生效。去掉 `path != root` 后，root 名叫 policy 即被 SkipDir，
//     返回空 —— 而「返回空」和「真的没有」在输出上完全一样，正是那种假绿。
//  2. 扫描器在**真实生产代码**上确有检出能力，不只是在我自己写的夹具上。
//     夹具是我按扫描器的行为造的，二者可能共享同一个盲区（教训 13）；
//     policy/gate.go 不是我写的，它是独立语料。
//
// 反过来它也解释了 skipDirs 里为什么必须有 policy：那个 lastReq 是闸门实现
// 本身，不跳过的话 TestNoPrivateThrottleState 会把它当成回潮报出来。
func TestScanRootExemptFromSkipRule(t *testing.T) {
	res, err := scanNoPanic(t, "policy")
	if err != nil {
		t.Fatalf("扫描 policy 包失败: %v", err)
	}
	if len(res.offenders) == 0 {
		t.Fatal("直接以 policy 为 root 时必须检出它的 lastReq —— " +
			"返回空说明跳过规则误伤了 root 自身（这种假绿与「真的没有」无法区分）")
	}
	if !hasOffenderFor(t, res.offenders, filepath.Join("policy", "gate.go"), "lastReq") {
		t.Errorf("应检出 policy/gate.go 的 lastReq，offenders=%v", res.offenders)
	}
}

// TestScanIgnoresNonFieldOccurrences 守护 boundary[0]：注释、局部变量、
// 测试文件里的同名标识符都不得误报。
//
// 有效性判定放在行为断言之后（陷阱 10）：夹具若一个文件都没被扫到，
// 「没有误报」是空真的。
func TestScanIgnoresNonFieldOccurrences(t *testing.T) {
	res, err := scanNoPanic(t, filepath.Join("testdata", "decoys"))
	if err != nil {
		t.Fatalf("扫描假阳性夹具失败: %v", err)
	}
	if len(res.offenders) > 0 {
		t.Errorf("误报了非结构体字段的出现: %v", res.offenders)
	}
	if len(res.scanned) == 0 {
		t.Fatal("假阳性夹具一个文件都没被扫到 —— 本测试空转")
	}
	// decoy_test.go 必须是被**跳过**的，而不是被扫了但恰好没命中。
	for _, p := range res.scanned {
		if strings.HasSuffix(p, "_test.go") {
			t.Errorf("测试文件 %s 不该进入扫描范围", p)
		}
	}
}

// TestScanReportsParseFailure 守护 error_handling[0] 的解析失败分支：
// 必须返回带文件名的错误，不 panic、不静默跳过。
//
// 「静默跳过」是守卫失效的第三种形态（陷阱 11）—— 它既不红也不绿，
// 一个 `if perr != nil { return nil }` 会让整个扫描对畸形源码视而不见。
//
// 畸形源码**运行时现写进 t.TempDir()**，不作为夹具存在于仓库。原先它是
// testdata/parsefail/bad.go，那形态有一个不可兼得的约束冲突：
//
//	packages 字段同时驱动漂移判据（task-completed.sh:58）与 go test（:112-119）。
//	把畸形夹具的目录写进 packages ⇒ go test 对它 build failed ⇒ 此后任何
//	dev_done 迁移被永久 BLOCKED，且报错显示「测试失败」，看不出根因在声明里；
//	不写进 packages ⇒ 它一旦处于未提交状态就会被算成**别人**的 scope 漂移，
//	拦掉那个恰好在 transition 的 agent（本 Sprint 实测发生过一次）。
//
// 根因是「仓库里存在一个故意不可编译的 .go 文件」，故消除它而非二选一。
// 顺带也消掉了裸跑 gofmt -l internal/collector/ 对它的解析报错。
//
// 反过来，decoys/ 与 throttleback/ 两个夹具**刻意留在仓库**：它们可编译、
// 对 go test 无害（[no tests to run] / [no test files]），且作为语料需要被人读。
func TestScanReportsParseFailure(t *testing.T) {
	root := t.TempDir()
	const badFile = "bad.go"
	// 缺右括号：parser 必然失败，且失败点在包子句之后，确保走的是
	// ParseFile 的语法错误分支而不是「根本不是 Go 文件」。
	if err := os.WriteFile(filepath.Join(root, badFile),
		[]byte("package parsefail\n\nfunc broken(\n"), 0o644); err != nil {
		t.Fatalf("写畸形源码失败: %v", err)
	}

	res, err := scanNoPanic(t, root)
	if err == nil {
		t.Fatalf("语法错误的源码必须报错，却返回了 offenders=%v scanned=%v（静默跳过 = 没有守护）",
			res.offenders, res.scanned)
	}
	if !strings.Contains(err.Error(), badFile) {
		t.Errorf("错误信息必须指出是哪个文件，got: %v", err)
	}
}

// TestScanReportsMissingRoot 守护 error_handling[0] 的目录不存在分支。
func TestScanReportsMissingRoot(t *testing.T) {
	_, err := scanNoPanic(t, filepath.Join("testdata", "no-such-dir"))
	if err == nil {
		t.Fatal("不存在的扫描根必须报错，而不是当作「扫完了、没问题」")
	}
}
