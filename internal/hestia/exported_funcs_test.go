package hestia

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping (TASK-001, M2b)
// functional[0]     夹具 top.go + sub/deep.go ⇒ 恰好 ["Parse","sub.InsertRow"]
//                                                     → TestExportedFuncsDescendsIntoSubpackages
// functional[1]     store_test.go 守卫改为一次 exportedFuncs(".")，want 零改动
//                                                     → store_test.go TestPackageExposesNoWriteFunctions
// boundary[0]       testdata/ 被跳过（非法源码不报错、结果为空）→ TestExportedFuncsSkipsTestdata
//                   _ 与 . 前缀目录同样跳过              → TestExportedFuncsSkipsUnderscoreAndDotDirs
// boundary[1]       接收者类型未导出的方法不算导出面      → TestExportedFuncsIgnoresMethodsOnUnexportedReceiver
// error_handling[0] 非 testdata 下解析失败 ⇒ 含文件名、可 errors.Is 的错误
//                                                     → TestExportedFuncsReportsParseErrorWithFileName

// exportedFuncs 收集 root 下所有非测试 .go 文件的导出函数与方法。
//
// 2026-09-16（M2b TASK-001）改为**递归**。原实现 os.ReadDir(root) 遇目录直接 continue，
// 子包完全在视野之外；M2b 新开 internal/hestia/sheets，不改就等于给守卫留一个盲区。
//
// 命名：根目录裸名（Parse、Store.Save），子包按相对目录限定（sheets.Push）。
// 跳过 testdata 与 _ / . 前缀目录——Go 工具链自己就忽略它们，里面是夹具不是代码。
func exportedFuncs(root string) ([]string, error) {
	fset := token.NewFileSet()
	var got []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// root 自身不参与跳过判定：它可能就叫 "." 或以 _ / . 开头，
			// 那样会一进门就 SkipDir，整棵树一个文件都扫不到。
			if path == root {
				return nil
			}
			if name == "testdata" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("解析 %s: %w", path, perr)
		}
		prefix := ""
		if rel, rerr := filepath.Rel(root, filepath.Dir(path)); rerr == nil && rel != "." {
			prefix = filepath.ToSlash(rel) + "."
		}
		got = append(got, declNames(f, prefix)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(got)
	return got, nil
}

// declNames 抽出一份文件里的导出函数与方法，prefix 非空时按包限定。
func declNames(f *ast.File, prefix string) []string {
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || !fn.Name.IsExported() {
			continue
		}
		if fn.Recv == nil {
			out = append(out, prefix+fn.Name.Name)
			continue
		}
		// 方法：接收者类型不导出时，包外根本拿不到它，不构成导出面
		if recv := recvTypeName(fn.Recv.List[0].Type); ast.IsExported(recv) {
			out = append(out, prefix+recv+"."+fn.Name.Name)
		}
	}
	return out
}

// writeGoFile 在 root 下按相对路径写一个夹具源文件，中间目录自动创建。
func writeGoFile(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(src), 0o644))
}

// invalidGo 是一段**不合法**的 Go 源码，用来触发 parser.ParseFile 报错。
const invalidGo = "这不是合法的 Go 源码"

// TestExportedFuncsDescendsIntoSubpackages 钉住本次改动的全部理由。
//
// 原实现是 TestPackageExposesNoWriteFunctions 里的 os.ReadDir(".")，**遇目录直接 continue**。
// M2b 要加 internal/hestia/sheets 子包；不改的话，子包里写一个
// func InsertRow(db *sql.DB, ...) 直接 INSERT，守卫一声不吭——那正是守卫自己注释里
// 记着的那起事故（「包级新增 InsertRow、43/43 全绿无人拦」）的形状，只是换了个目录。
func TestExportedFuncsDescendsIntoSubpackages(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "top.go", "package hestia\n\nfunc Parse() {}\nfunc unexported() {}\n")
	writeGoFile(t, root, "sub/deep.go", "package sub\n\nfunc InsertRow() {}\n")

	got, err := exportedFuncs(root)
	require.NoError(t, err)

	// 子包的符号必须出现，且**按包限定**——不限定的话两个包里的同名函数会在扁平
	// 列表里撞成两个 "New"，排序后位置还会互换，看不出哪个是哪个。
	require.Equal(t, []string{"Parse", "sub.InsertRow"}, got)
}

// TestExportedFuncsSkipsTestdata：testdata 里放的是夹具不是代码。
//
// 现在 internal/hestia/testdata 里没有 .go，所以不跳也能过——但那是运气不是设计。
// 将来谁放一个进去，parser.ParseFile 会报一个与写口守卫毫无关系的错，排查的人要绕很远。
func TestExportedFuncsSkipsTestdata(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "testdata/broken.go", invalidGo)

	got, err := exportedFuncs(root)
	require.NoError(t, err, "testdata 必须被跳过，否则解析夹具会报一个牛头不对马嘴的错")
	require.Empty(t, got)
}

// TestExportedFuncsSkipsUnderscoreAndDotDirs：_ 与 . 前缀目录与 go 工具链约定一致，一并跳过。
func TestExportedFuncsSkipsUnderscoreAndDotDirs(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"_scratch", ".hidden"} {
		writeGoFile(t, root, filepath.Join(dir, "broken.go"), invalidGo)
	}

	got, err := exportedFuncs(root)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestExportedFuncsIgnoresMethodsOnUnexportedReceiver：接收者类型不导出，包外拿不到它，
// 它的导出方法不构成导出面（沿用既有 ast.IsExported(recv) 判定）。
func TestExportedFuncsIgnoresMethodsOnUnexportedReceiver(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "recv.go", `package hestia

type hidden struct{}

func (h hidden) Exported() {}

type Shown struct{}

func (s *Shown) Exported() {}
`)

	got, err := exportedFuncs(root)
	require.NoError(t, err)
	require.Equal(t, []string{"Shown.Exported"}, got)
}

// TestExportedFuncsReportsParseErrorWithFileName：非 testdata 目录下的 .go 解析失败必须报错，
// 错误带文件名、且包住原始 *scanner.ErrorList（可 errors.As/Is），不 panic、不静默跳过。
func TestExportedFuncsReportsParseErrorWithFileName(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "broken.go", invalidGo)

	got, err := exportedFuncs(root)
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "解析 ")
	require.Contains(t, err.Error(), "broken.go")
	var list scanner.ErrorList
	require.True(t, errors.As(err, &list), "必须用 %%w 包住原始解析错误，实际: %v", err)
}
