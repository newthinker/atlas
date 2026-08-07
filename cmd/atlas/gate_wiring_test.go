package main

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/collector/policy"
)

// Context Checkpoint: done_criteria → test mapping (TASK-019)
// functional[0] "crisis 两条路径在构造前完成接线，FRED_API_KEY 非空时也成立；
//                **直接观测接线结果**"
//               → TestCrisisBackfillWiresGateWithFREDEnvSet
//               + TestCrisisEvalWiresGateWithFREDEnvSet（运行期，观测配置里的账本路径落地）
// functional[1] "AST 守护；复合句两分句各需独立变异证据"
//               → TestEntrypointsWireGateBeforeCollectors（阴性）
//               + TestScanGateOrderDetectsMissingCall（分句「存在」）
//               + TestScanGateOrderDetectsWrongOrder（分句「顺序」）
// functional[2] "空真自检：下界守卫 + 改名变异须转红"
//               → TestScanGateOrderReportsFunctionNotFound
// functional[3] "范围封闭：反向追溯全部构造点"
//               → TestEntrypointsWireGateBeforeCollectors 覆盖三个缺口入口；
//                 其余构造点的逐条结论见 discovery（已正确者含理由）
// boundary[0]   "判据限于目标函数体内" → TestScanGateOrderIgnoresOtherFunctions
// boundary[1]   "失败措辞与实际保证对齐（词法位置，非执行顺序）"
//               → 见 TestEntrypointsWireGateBeforeCollectors 的失败信息与下方强度边界说明
// error_handling "配置不可读 + 环境变量有效 ⇒ 仍能运行，Gate 退化而非 panic/退出"
//               → TestEnsurePolicyGateDegradesOnUnreadableConfig

// gateFns 是「装配闸门」的调用名集合。ensurePolicyGate 是本任务新增的显式入口，
// initPolicyGate 是它内部最终调到的那个——两者任一出现都算装配。
var gateFns = map[string]bool{"ensurePolicyGate": true, "initPolicyGate": true}

// collectorCtors 是**会快照 policy.Default() 的** collector 构造函数。
//
// 名单不是「所有 collector」而是「构造函数里取了 policy.Default() 的那些」——
// 实测 akshare / fred / edgar / qlib / qlibpit 不用 Gate，把它们算进来会产生
// 与本约束无关的假红（例如 crisis 里的 fred.New 本就不需要闸门）。
var collectorCtors = map[string]bool{
	"yahoo.New": true, "eastmoney.New": true, "crypto.New": true,
	"tushare.New": true, "lixinger.New": true, "baostock.New": true,
	"twelvedata.New": true,
}

// gateOrder 记录目标函数体内的装配调用与**最早**的 collector 构造调用。
//
// found 不是调试信息而是断言材料（空真自检）：函数改名后扫描会一无所获，
// 而「装配在构造之前」在两个行号都为 0 时**空真地成立**。
type gateOrder struct {
	gateLine int // 装配调用行；0 = 未找到
	ctorLine int // 最早的 collector 构造行；0 = 未找到
	ctorName string
	found    bool // 目标函数是否存在
}

// scanGateOrder 解析 src（nil 时读 filename），在名为 fnName 的**函数体内**定位
// 装配调用与最早的 collector 构造调用。
//
// ⚠ 必须按函数体而非按文件：export_ohlcv.go:297 那处 initPolicyGate 在
// loadConfigOrDefaults 函数体内，按包或按文件扫会把它算进别的函数头上
// （TASK-006 的接线顺序检查首版正是按「文件内首个匹配」比较而误报）。
//
// 行号一律现读，不写死——源文件一改行号就漂移，写死等于埋一个与被测行为无关的红。
func scanGateOrder(filename string, src any, fnName string) (gateOrder, error) {
	var res gateOrder
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return gateOrder{}, err
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fnName || fd.Body == nil {
			continue
		}
		res.found = true
		var ctors []gateOrder
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeQualifiedName(ce)
			line := fset.Position(ce.Pos()).Line
			switch {
			case gateFns[name] && res.gateLine == 0:
				res.gateLine = line
			case collectorCtors[name]:
				ctors = append(ctors, gateOrder{ctorLine: line, ctorName: name})
			}
			return true
		})
		// 取**最早**的构造点：装配必须早于所有构造，故只需与最早那个比。
		sort.Slice(ctors, func(i, j int) bool { return ctors[i].ctorLine < ctors[j].ctorLine })
		if len(ctors) > 0 {
			res.ctorLine, res.ctorName = ctors[0].ctorLine, ctors[0].ctorName
		}
	}
	return res, nil
}

// calleeQualifiedName 返回 "pkg.Fn"（选择器调用）或 "Fn"（普通调用）。
//
// 必须带包名：只取 Sel.Name 的话 yahoo.New / eastmoney.New / 任何 X.New 都塌成 "New"，
// 无法与不相关的构造函数区分。
func calleeQualifiedName(ce *ast.CallExpr) string {
	switch fn := ce.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

// scanGateOrderNoPanic 把 panic 转成断言失败：裸 panic 会中断整个测试二进制，
// 同包其余测试根本跑不到，而汇总只显示「红了」。
func scanGateOrderNoPanic(t *testing.T, filename string, src any, fnName string) (res gateOrder, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("scanGateOrder(%q, %q) panic 了: %v（应返回 error 而非 panic）", filename, fnName, r)
		}
	}()
	return scanGateOrder(filename, src, fnName)
}

// TestEntrypointsWireGateBeforeCollectors 守护三条**不经 loadConfigOrDefaults 就构造
// collector** 的入口（functional[1]/[3]）。
//
// 根因是结构性的：initPolicyGate 全仓只有两个调用点，其中一个藏在
// loadConfigOrDefaults 函数体内 ⇒ 接线与配置加载被隐式耦合，任何跳过配置加载的
// 路径都静默跳过接线。crisis 的两条在 FRED_API_KEY 非空时经 resolveFREDKey 提前
// return 而跳过；backtest 全文不读配置。
//
// ⚠ **强度边界（措辞刻意不写「必须早于」）**：本测试断言的是**源码里的词法位置**
// ——装配调用出现在最早的 collector 构造调用之前。它**不是执行顺序的承诺**：
// 把装配包进 `defer`/`go` 闭包、恒假 `if`、或写在提前 return 之后的死代码里，
// 词法位置照样在前而运行期根本不执行。那几类的守护是本文件的运行期测试
// （TestCrisisBackfillWiresGateWithFREDEnvSet 等直接观测接线结果），不是这条。
func TestEntrypointsWireGateBeforeCollectors(t *testing.T) {
	cases := []struct{ file, fn string }{
		{"crisis.go", "runCrisisBackfill"},
		{"crisis.go", "runCrisisEval"},
		{"backtest.go", "runBacktest"},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			got, err := scanGateOrderNoPanic(t, tc.file, nil, tc.fn)
			if err != nil {
				t.Fatalf("扫描 %s 失败: %v", tc.file, err)
			}
			if !got.found {
				t.Fatalf("%s 里找不到函数 %s —— 「装配在构造之前」在没有函数体时空真地成立",
					tc.file, tc.fn)
			}
			if got.ctorLine == 0 {
				t.Fatalf("%s 的 %s 函数体内找不到任何会快照 policy.Default() 的 collector 构造 —— "+
					"锚点消失了，本测试不再守护任何东西（若确已移走构造，请从 cases 里删掉本项）",
					tc.file, tc.fn)
			}
			if got.gateLine == 0 {
				t.Fatalf("%s 的 %s 函数体内没有装配调用（ensurePolicyGate/initPolicyGate），"+
					"而它在第 %d 行构造了 %s：该 collector 会拿到懒构造的无账本 Gate，"+
					"cache.enabled / cache.ttl / 整个 collector.topics 静默失效",
					tc.file, tc.fn, got.ctorLine, got.ctorName)
			}
			if got.gateLine >= got.ctorLine {
				t.Errorf("%s 的 %s：装配调用的**词法位置**在 collector 构造之后"+
					"（装配 @%d，%s @%d）。collector 在构造函数里快照 policy.Default()，"+
					"晚于构造则快照到的是未接线的闸门",
					tc.file, tc.fn, got.gateLine, got.ctorName, got.ctorLine)
			}
		})
	}
}

// newGateTestCmd 造一个带 context 与缓冲输出的 cobra.Command，供直接驱动
// runCrisisEval / runBacktest（它们从 cmd 取 context 与输出流）。
func newGateTestCmd() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	return c
}

// gateSrc 生成一个含目标函数的最小源码，供阳性对照使用。
func gateSrc(fnName, body string) string {
	return "package main\n\nfunc " + fnName + "() error {\n" + body + "\treturn nil\n}\n"
}

// functional[1] 分句「存在」的独立证据：装配调用整个消失时必须检出。
func TestScanGateOrderDetectsMissingCall(t *testing.T) {
	src := gateSrc("runBacktest", "\treg.Register(yahoo.New())\n")
	got, err := scanGateOrderNoPanic(t, "backtest.go", src, "runBacktest")
	if err != nil {
		t.Fatalf("扫描夹具失败: %v", err)
	}
	if !got.found {
		t.Fatal("夹具里的目标函数没被找到 —— 夹具本身失效了")
	}
	if got.gateLine != 0 {
		t.Errorf("夹具里没有装配调用，扫描器却报出第 %d 行 —— 它可能恒返回命中", got.gateLine)
	}
	if got.ctorLine == 0 {
		t.Error("夹具里有 yahoo.New() 却没被识别为 collector 构造")
	}
}

// functional[1] 分句「顺序」的独立证据。
//
// **这条不能由上一条替代**：一个只检查「装配调用是否存在」的实现，在上一条（删掉调用）
// 下同样转红，却会放过本条。两个分句必须各有独立变异。
func TestScanGateOrderDetectsWrongOrder(t *testing.T) {
	src := gateSrc("runBacktest", "\treg.Register(yahoo.New())\n\tensurePolicyGate()\n")
	got, err := scanGateOrderNoPanic(t, "backtest.go", src, "runBacktest")
	if err != nil {
		t.Fatalf("扫描夹具失败: %v", err)
	}
	if got.gateLine == 0 || got.ctorLine == 0 {
		t.Fatalf("夹具两处调用都应被找到: gate=%d ctor=%d", got.gateLine, got.ctorLine)
	}
	if got.gateLine < got.ctorLine {
		t.Errorf("夹具中装配写在构造**之后**（gate @%d, ctor @%d），扫描器却判成之前 —— "+
			"顺序判据失效，生产那格的绿是假的", got.gateLine, got.ctorLine)
	}
}

// boundary[0]：判据必须限于目标函数体内。
//
// 夹具刻意构造真实存在的形态：**另一个函数**（对应 loadConfigOrDefaults，
// export_ohlcv.go:297 的 initPolicyGate 就住在那里）在文件更靠前处装配，
// 而目标函数自己没装。按文件扫会判成「装配存在且在前」（假绿）。
func TestScanGateOrderIgnoresOtherFunctions(t *testing.T) {
	src := "package main\n\n" +
		"func loadConfigOrDefaults() error {\n\tinitPolicyGate(nil, nil)\n\treturn nil\n}\n\n" +
		"func runBacktest() error {\n\treg.Register(yahoo.New())\n\treturn nil\n}\n"
	got, err := scanGateOrderNoPanic(t, "backtest.go", src, "runBacktest")
	if err != nil {
		t.Fatalf("扫描夹具失败: %v", err)
	}
	if got.gateLine != 0 {
		t.Errorf("把别的函数（loadConfigOrDefaults）里的装配算到了 runBacktest 头上（报第 %d 行）—— "+
			"扫描越出了函数体，会让「跳过配置加载」这类缺口被判成已装配", got.gateLine)
	}
}

// functional[2] 空真自检：目标函数不存在时 found 必须为 false。
//
// 「静默跳过」是守卫失效的第三种形态——函数改名后若扫描只返回空结果，
// 「装配在构造之前」会空真地通过，而接线可能早已被删。
func TestScanGateOrderReportsFunctionNotFound(t *testing.T) {
	src := gateSrc("somethingElse", "\t_ = 1\n")
	got, err := scanGateOrderNoPanic(t, "backtest.go", src, "runBacktest")
	if err != nil {
		t.Fatalf("扫描夹具失败: %v", err)
	}
	if got.found {
		t.Error("夹具里没有 runBacktest，found 却为 true")
	}
	if got.gateLine != 0 || got.ctorLine != 0 {
		t.Errorf("没找到目标函数时不该有任何命中: gate=%d ctor=%d", got.gateLine, got.ctorLine)
	}
}

func TestScanGateOrderReportsParseFailure(t *testing.T) {
	if _, err := scanGateOrderNoPanic(t, "x.go", "package main\n\nfunc broken(\n", "runBacktest"); err == nil {
		t.Fatal("语法错误的源码必须报错，而不是当作「扫完了、没问题」")
	}
}

func TestScanGateOrderReportsMissingFile(t *testing.T) {
	if _, err := scanGateOrderNoPanic(t, filepath.Join(t.TempDir(), "no-such.go"), nil, "runBacktest"); err == nil {
		t.Fatal("不存在的文件必须报错")
	}
}

// wiringProbeConfig 写一份带**独特配额账本路径**的配置，返回 (配置路径, 账本路径)。
//
// 账本路径是本文件运行期测试的观测量：Gate 装配时会按 config 的 quota.path 建
// FileStore，此后对带配额的主题做一次 Fetch 就会落盘。**账本出现在这个路径**
// 唯一可能的来源就是「本次装配读到了这份配置」——比断言「没报错」强得多（陷阱 8）。
func wiringProbeConfig(t *testing.T) (cfgPath, quotaPath string) {
	t.Helper()
	dir := t.TempDir()
	quotaPath = filepath.Join(dir, "wiring-probe-quota.json")
	cfgPath = filepath.Join(dir, "config.yaml")
	yaml := "collector:\n" +
		"  quota:\n" +
		"    path: " + quotaPath + "\n" +
		"  topics:\n" +
		"    tushare.daily_basic:\n" +
		"      min_interval: 0\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("写探针配置: %v", err)
	}
	return cfgPath, quotaPath
}

// assertGateCarriesConfig 断言当前单例闸门确实带着 wiringProbeConfig 那份配置。
func assertGateCarriesConfig(t *testing.T, quotaPath, pathDesc string) {
	t.Helper()
	if _, err := policy.Fetch(policy.Default(), "tushare.daily_basic", "probe",
		func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("%s: Fetch 失败: %v", pathDesc, err)
	}
	if _, err := os.Stat(quotaPath); err != nil {
		t.Errorf("%s **没有装配闸门**：配额账本未出现在配置指定的 %s。"+
			"该路径下的 collector 会拿到懒构造的无账本 Gate，"+
			"cache.enabled / cache.ttl / 整个 collector.topics 静默失效（%v）",
			pathDesc, quotaPath, err)
	}
}

// withFREDEnvAndConfig 布置「FRED_API_KEY 非空 + 指定配置」的现场并清空闸门单例。
//
// FRED_API_KEY 非空是关键前提：resolveFREDKey 会因此在 crisis.go 提前 return、
// 不经 loadConfigOrDefaults —— 那正是本任务要修的缺口路径。
func withFREDEnvAndConfig(t *testing.T, cfgPath string) {
	t.Helper()
	origGate, origFile := policy.Default(), cfgFile
	t.Cleanup(func() { policy.SetDefault(origGate); cfgFile = origFile })
	t.Setenv("FRED_API_KEY", "probe-key-not-used-for-network")
	cfgFile = cfgPath
	policy.SetDefault(nil) // 清空：账本落盘只可能来自本次装配
}

// functional[0]：crisis backfill 路径在 FRED_API_KEY 非空时也完成装配。
func TestCrisisBackfillWiresGateWithFREDEnvSet(t *testing.T) {
	cfgPath, quotaPath := wiringProbeConfig(t)
	snapshotCrisisFlags(t)
	withFREDEnvAndConfig(t, cfgPath)
	crisisCfgPath = writeTempCrisisConfig(t)
	backfillCSV, backfillIndicator, backfillFrom = "", "", "" // 走到 --from 缺失即返回

	_, _, err := runBackfill(t)
	if err == nil {
		t.Fatal("本探针预期在参数校验处返回错误（只需路径进入函数体即可）")
	}
	assertGateCarriesConfig(t, quotaPath, "crisis backfill")
}

// functional[0]：crisis eval 路径同上。该函数体内有三处 collector 构造
// （daily/nfci 的 yahoo.New() 与 intraday 的 yahoo.New().FetchQuote）。
func TestCrisisEvalWiresGateWithFREDEnvSet(t *testing.T) {
	cfgPath, quotaPath := wiringProbeConfig(t)
	snapshotCrisisFlags(t)
	withFREDEnvAndConfig(t, cfgPath)
	crisisCfgPath = filepath.Join(t.TempDir(), "no-such-crisis.yaml") // 让 openCrisisStore 早返回

	if err := runCrisisEval(newGateTestCmd(), nil); err == nil {
		t.Fatal("本探针预期在打开 crisis store 处返回错误")
	}
	assertGateCarriesConfig(t, quotaPath, "crisis eval")
}

// functional[0]/[3]：backtest 路径（qa-agent-8 发现的第四条，原本全文零接线）。
func TestBacktestWiresGate(t *testing.T) {
	cfgPath, quotaPath := wiringProbeConfig(t)
	origGate, origFile := policy.Default(), cfgFile
	origSym := backtestSymbol
	t.Cleanup(func() { policy.SetDefault(origGate); cfgFile = origFile; backtestSymbol = origSym })
	cfgFile = cfgPath
	policy.SetDefault(nil)
	backtestSymbol = "" // SelectForSymbol 对空注册表外的符号返回 provider 后仍会失败，足够早返回

	_ = runBacktest(newGateTestCmd(), []string{"ma_crossover"})
	assertGateCarriesConfig(t, quotaPath, "backtest")
}

// error_handling[0]：配置不可读 + 环境变量有效 ⇒ 路径仍能运行，闸门退化而非 panic/退出。
//
// 补接线带来的**新**风险：crisis 在 FRED_API_KEY 非空时原本根本不读配置，
// 现在会读。若配置损坏，原先能跑的路径不能因此挂掉。
// 退化目标是内置策略表（限流/缓存按内置值、无 config 覆盖、无跨进程账本）。
func TestEnsurePolicyGateDegradesOnUnreadableConfig(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(bad, []byte("collector:\n  quota:\n   path: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshotCrisisFlags(t)
	origGate, origFile := policy.Default(), cfgFile
	t.Cleanup(func() { policy.SetDefault(origGate); cfgFile = origFile })
	t.Setenv("FRED_API_KEY", "probe-key")
	cfgFile = bad
	policy.SetDefault(nil)
	crisisCfgPath = writeTempCrisisConfig(t)
	backfillCSV, backfillIndicator, backfillFrom = "", "", ""

	// 不得 panic：panic 会中断整个测试二进制，同包其余测试根本跑不到。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("配置损坏时 panic 了: %v（应退化而非阻断）", r)
		}
	}()

	_, _, err := runBackfill(t)
	if err == nil {
		t.Fatal("本探针预期在参数校验处返回错误")
	}
	if strings.Contains(err.Error(), "loading config") {
		t.Errorf("配置损坏把原本能跑的路径挂掉了：err = %v。"+
			"crisis 在 FRED_API_KEY 有效时不依赖配置，补接线不应引入这个硬依赖", err)
	}

	// 闸门必须可用（退化为内置策略），而不是 nil 或不可用。
	if got := policy.Default(); got == nil {
		t.Fatal("配置损坏后 policy.Default() 为 nil —— 应退化为内置策略表")
	}
	if _, err := policy.Fetch(policy.Default(), "yahoo.chart", "probe",
		func() (int, error) { return 1, nil }); err != nil {
		t.Errorf("退化后的闸门不可用: %v", err)
	}
}
