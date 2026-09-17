# TASK-001 验证报告 — AST 写口守卫改为递归子目录

- 验证者: test-m2b-a　　时间: 2026-09-16T10:18:15Z
- 判定对象: master @ `28fca4c67ce4e303a983ddd13c4de4921849a428`（= verify_baseline.head，merge commit）；dev commit `5a2c1a264b97989b01d65aa57dcf0ef4384f20d0`；base `6297fee16e1906032fd219dd0fe0f9329fd9a06e`
- discovery sha256: `a110144ca91896f2ce064f16ce9dafcdd4a570f93e4d41cdfa081ffc5bd22953`（= verify_baseline.discovery_sha256，判定前后各现读一次均一致）
- 隔离 worktree: `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-m2b-v001`（detach @ 上述全 sha）
- 范围核对: `git diff --stat 6297fee..28fca4c` 仅 `internal/hestia/exported_funcs_test.go`(186/0) 与 `internal/hestia/store_test.go`(2/33)，与 `writes` 声明完全一致，无越界；`go.mod`/`go.sum` 无 diff。

## 结论：**VERIFIED**（7/7 PASS）

## Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | verify_by | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 夹具 top.go + sub/deep.go ⇒ 恰好 `[Parse, sub.InsertRow]` | test | dev: `TestExportedFuncsDescendsIntoSubpackages`（夹具与 DoD 逐字一致、`require.Equal` 精确相等）；验证者自构 `TestV_ThreeLevelNestingAndRootMethod`：根 `Alpha`/`S.M`/`Zeta`/未导出 `zed`、`sub/x.go` 同名 `Alpha` + 泛型指针接收者 `(*T[P]).Do`、`a/b/c.go` `Deep`+`(*Y).Hidden`、`a/b/c_test.go` 一个导出函数 ⇒ 得 `[Alpha S.M Zeta a/b.Deep a/b.Y.Hidden sub.Alpha sub.T.Do]`，`_test.go` 不计、`StringsAreSorted` 真 | PASS |
| functional[1] | 收集循环换成一次 `exportedFuncs(".")`；守卫仍绿；want 零改动；import 只删不增 | test | `git diff 6297fee..28fca4c -- store_test.go`：import 块 `-go/parser` `-go/token` `-sort`，**+ 行 0**；`grep -E '^[-+]\s+"'` 仅这 3 个 `-` 行，无任何 want 字符串行；函数体 33 行 → 2 行（`got, err := exportedFuncs(".")` + `require.NoError`）；`TestPackageExposesNoWriteFunctions` PASS；验证者 `TestV_OldLoopEqualsNewOnRealPackage` 把 6297fee 的旧循环原样内联在真实包上跑，与 `exportedFuncs(".")` **38 项逐项相等** ⇒ 递归未改变现状认定 | PASS |
| boundary[0] | `testdata/`、`_`、`.` 前缀目录跳过，非法源码不报错、结果为空 | test | dev: `TestExportedFuncsSkipsTestdata`、`TestExportedFuncsSkipsUnderscoreAndDotDirs`；验证者 `TestV_SkipDirsEvenWhenValid`（根 `testdata/ok.go` 合法源码不出现、嵌套 `sub/testdata`、`sub/_gen`、`sub/.git` 里非法源码不报错，只剩 `sub.Keep`）、`TestV_RootNamedWithPrefixIsNotSkipped`（root 本身叫 `_root` 或 `"."` 时不被整棵跳过）；变异 M2（去掉 `name == "testdata"`）⇒ dev 与验证者测试均红 | PASS |
| boundary[1] | 接收者类型未导出的方法不算导出面 | test | dev: `TestExportedFuncsIgnoresMethodsOnUnexportedReceiver`（`hidden.Exported` 不出现、`Shown.Exported` 出现）；验证者 `TestV_UnexportedReceiverVariants`（`*hidden`、泛型 `gen[P]`、子包 `priv` ⇒ 结果为空） | PASS |
| boundary[2] | 两次 RED 原始输出贴进 discovery `verification.red_phase` | review | `red_phase.step2_undefined` 含 `exported_funcs_test.go:28:14: undefined: exportedFuncs` + `[build failed]`；`red_phase.step4_non_recursive_misses_subpackage` 是 `TestExportedFuncsDescendsIntoSubpackages` 的 FAIL，`expected [Parse sub.InsertRow]` / `actual [Parse]`——正是非递归看不见 `sub.InsertRow` 的形状。另做变异 M1（非 root 目录一律 `SkipDir`）⇒ 该测试当场红，证实它守着递归本身。注：step4 里行号 82 与最终文件 125 不同，系后续 code-simplifier 重排，不影响判定 | PASS |
| error_handling[0] | 非 testdata 下解析失败 ⇒ `fmt.Errorf("解析 %s: %w", name, perr)`，不 panic 不静默 | test | dev: `TestExportedFuncsReportsParseErrorWithFileName`（含「解析 」、含文件名、`errors.As` 到 `scanner.ErrorList`）；验证者 `TestV_ParseErrorInSubdirIsWrappedWithPath`：错误以「解析 」开头、含 `sub/broken.go`、`errors.As` 得到的 ErrorList 文本与直接 `parser.ParseFile` 同文件的错误**完全相等**（证明 `%w` 包的是原始错误）、`got == nil`（未返回部分结果）、不存在的 root 返回 err 不 panic | PASS |
| non_functional[0] | 全绿 / gofmt / vet / 无新依赖 / code-simplifier | manual | `GOTOOLCHAIN=local go test ./internal/hestia/ -count=1` exit 0，`-v` 计数 **820 PASS / 0 FAIL**（`ok github.com/newthinker/atlas/internal/hestia 1.785s`）；`gofmt -l internal/hestia/` 空；`go vet ./internal/hestia/` exit 0；`go.mod`/`go.sum` 在 base..head 无 diff；code-simplifier 改动在 discovery `decisions[3]` 申报（提取 `writeGoFile`/`invalidGo`、`path == root` 注释），与文件实际内容相符 | PASS |

## 变异测试（隔离 worktree 内就地改副本，跑完 `git checkout` 还原，前后 sha256 `ee1ca054…` 一致）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 非递归：非 root 目录 `return fs.SkipDir` | exportedFuncs L55 | KILLED：`TestExportedFuncsDescendsIntoSubpackages` + 3 条 TestV_ 红 |
| M2 不跳 testdata | L52 | KILLED：`TestExportedFuncsSkipsTestdata` + `TestV_SkipDirsEvenWhenValid` 红 |
| M3 不加包前缀（`prefix = ""`） | L66 | KILLED：`TestExportedFuncsDescendsIntoSubpackages` + 2 条 TestV_ 红 |

## 测试质量评审
- 断言均为精确相等（`require.Equal` 整切片）或带原因的 `require.Error/Empty/Nil`，无空洞断言；无 mock。
- 夹具经 `t.TempDir()` 隔离，不碰真实 `internal/hestia` 目录；解析错误测试用 `errors.As` 而非字符串匹配证明 `%w`。
- 新逻辑全在 `_test.go`，不进生产导出面；`recvTypeName` 复用 store_test.go 既有实现（含泛型接收者剥离）。

## 备注（非缺陷）
- DoD functional[1] 与 `questions[0].answer` 写「`go/parser`、`go/token` 两行」，实际无使用者的 import 是 3 行（多一个 `sort`），plan.md「TASK-001 验证者注」已说明；判据「只有 - 行没有 + 行」不受影响。
- 验证者自构测试文件 `zz_verify_m2b_a_test.go` 只存在于验证 worktree，不进交付。

## 附录：验证者夹具测试（zz_verify_m2b_a_test.go）
```go
package hestia

// 验证者 test-m2b-a 自构夹具（不进交付；只在 worktree wt-m2b-v001 存在）
import (
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// V1 functional[0] 加强：三层嵌套 + 根目录方法 + 同名函数跨包限定 + 指针/泛型接收者
func TestV_ThreeLevelNestingAndRootMethod(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "s.go", "package hestia\n\ntype S struct{}\nfunc (S) M() {}\nfunc Zeta() {}\nfunc Alpha() {}\nfunc zed() {}\n")
	writeGoFile(t, root, "sub/x.go", "package sub\n\nfunc Alpha() {}\ntype T[P any] struct{}\nfunc (*T[P]) Do() {}\n")
	writeGoFile(t, root, "a/b/c.go", "package b\n\nfunc Deep() {}\nfunc (x *Y) Hidden() {}\ntype Y struct{}\n")
	writeGoFile(t, root, "a/b/c_test.go", "package b\n\nfunc FromTestFile() {}\n")
	got, err := exportedFuncs(root)
	require.NoError(t, err)
	want := []string{"Alpha", "S.M", "Zeta", "a/b.Deep", "a/b.Y.Hidden", "sub.Alpha", "sub.T.Do"}
	require.Equal(t, want, got)
	require.True(t, sort.StringsAreSorted(got))
}

// V2 boundary[0] 加强：嵌套 testdata、_/. 目录里的合法源码也不出现；testdata 里合法 .go 同样不计
func TestV_SkipDirsEvenWhenValid(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "testdata/ok.go", "package x\n\nfunc FromTestdata() {}\n")
	writeGoFile(t, root, "sub/testdata/broken.go", invalidGo)
	writeGoFile(t, root, "sub/_gen/broken.go", invalidGo)
	writeGoFile(t, root, "sub/.git/broken.go", invalidGo)
	writeGoFile(t, root, "sub/ok.go", "package sub\n\nfunc Keep() {}\n")
	got, err := exportedFuncs(root)
	require.NoError(t, err)
	require.Equal(t, []string{"sub.Keep"}, got)
}

// V2b root 自身以 _ 或 . 开头 / root == "." 时不被整棵跳过
func TestV_RootNamedWithPrefixIsNotSkipped(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "_root")
	writeGoFile(t, root, "p.go", "package p\n\nfunc P() {}\n")
	got, err := exportedFuncs(root)
	require.NoError(t, err)
	require.Equal(t, []string{"P"}, got)

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))
	got, err = exportedFuncs(".")
	require.NoError(t, err)
	require.Equal(t, []string{"P"}, got)
}

// V3 boundary[1] 加强：指针接收者 + 未导出泛型接收者 + 子包内的未导出接收者
func TestV_UnexportedReceiverVariants(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "r.go", "package hestia\n\ntype hidden struct{}\nfunc (h *hidden) Exported() {}\ntype gen[P any] struct{}\nfunc (g gen[P]) Exported2() {}\nfunc (h hidden) unexported() {}\n")
	writeGoFile(t, root, "sub/r.go", "package sub\n\ntype priv struct{}\nfunc (priv) Exported() {}\n")
	got, err := exportedFuncs(root)
	require.NoError(t, err)
	require.Empty(t, got)
}

// V4 error_handling[0] 加强：子目录里的坏文件 ⇒ 错误含相对路径文件名、errors.Is 对原始错误成立、返回 nil 切片
func TestV_ParseErrorInSubdirIsWrappedWithPath(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "ok.go", "package hestia\n\nfunc Fine() {}\n")
	writeGoFile(t, root, "sub/broken.go", invalidGo)
	got, err := exportedFuncs(root)
	require.Error(t, err)
	require.Nil(t, got, "出错时不得静默返回部分结果")
	require.True(t, strings.HasPrefix(err.Error(), "解析 "), err.Error())
	require.Contains(t, err.Error(), filepath.Join("sub", "broken.go"))
	// errors.Is：直接解析同一文件拿到原始错误做对照
	_, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "sub", "broken.go"), nil, 0)
	require.Error(t, perr)
	var list scanner.ErrorList
	require.True(t, errors.As(err, &list))
	require.Equal(t, perr.Error(), list.Error())
	// 不存在的 root ⇒ 报错不 panic
	_, err = exportedFuncs(filepath.Join(root, "nope"))
	require.Error(t, err)
}

// V5 functional[1] 加强：在真实包上，旧实现（6297fee 的非递归循环）与新实现结果逐项相等
func TestV_OldLoopEqualsNewOnRealPackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var old []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			if fn.Recv == nil {
				old = append(old, fn.Name.Name)
				continue
			}
			recv := recvTypeName(fn.Recv.List[0].Type)
			if ast.IsExported(recv) {
				old = append(old, recv+"."+fn.Name.Name)
			}
		}
	}
	sort.Strings(old)
	neu, err := exportedFuncs(".")
	require.NoError(t, err)
	require.Equal(t, old, neu)
	t.Logf("真实包导出面 %d 项: %v", len(neu), neu)
}
```
