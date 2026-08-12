package main

// Context Checkpoint: done_criteria → test mapping (TASK-008 cobra 装配)
// functional[0]     两个子命令注册 + 装配链路              → TestHestiaCommandIsRegistered、TestHestiaStatusOnEmptyStore
// functional[1]     db_path 的绝对路径解析**不在这一层**   → TestHestiaCmdDoesNotResolveDBPath
// boundary[0]       --hestia-config / --force 两个 flag    → TestHestiaFlags
// boundary[0]       配置不存在 / 非法时错误传播，不静默    → TestHestiaStatusFailsOnBadConfig
// boundary[1]       status 在空库上跑通（newDiscardCmd）   → TestHestiaStatusOnEmptyStore
// error_handling[0] 配置装载失败的错误信息含配置文件路径   → TestHestiaConfigErrorNamesThePath
// error_handling[1] 🔴 必须设 SilenceUsage                 → TestHestiaCommandsSilenceUsage

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 命令挂在 rootCmd 下，两个子命令挂在 hestia 下。
func TestHestiaCommandIsRegistered(t *testing.T) {
	var found bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "hestia" {
			found = true
			var subs []string
			for _, sc := range c.Commands() {
				subs = append(subs, sc.Name())
			}
			assert.Contains(t, subs, "ingest")
			assert.Contains(t, subs, "status")
		}
	}
	assert.True(t, found, "hestia 命令没有注册到 rootCmd")
}

// --hestia-config 是持久化 flag（两个子命令都要用），--force 只在 ingest 上。
func TestHestiaFlags(t *testing.T) {
	assert.NotNil(t, hestiaCmd.PersistentFlags().Lookup("hestia-config"))
	assert.NotNil(t, hestiaIngestCmd.Flags().Lookup("force"))
	assert.Nil(t, hestiaStatusCmd.Flags().Lookup("force"),
		"--force 是 ingest 的事，status 不该有")
}

// 🔴 失败时不得再灌一屏 usage（reviewer D4）。
//
// 实测（改动前，crisis 那条同形）：`atlas crisis status --crisis-config /nonexistent/nope.yaml`
// 输出 **13 行**，其中只有第 1 行是真正的错误，其余 12 行是 usage —— 全仓当时**没有任何
// 命令**设过 SilenceUsage。
//
// 为什么这条对本管线尤其要紧：设计意图是让退出码 + hestia-ingest.err.log 成为**唯一报警
// 通道**，而本管线**预期会有连续两个月每天三次的稳定失败态**（TASK-001 的 D6）。每次失败
// 灌 12 行 usage ⇒ err.log 里真正那行错误被埋在几千行样板里。
//
// ⚠️ 只在 hestia 这一层设，**不动 main.go 的 rootCmd** —— 那会波及 crisis 的现有测试。
//
// 🔴 **两个叶子命令各自都必须设**（实测）：cobra 判的是「被执行的那个命令」或「根命令」，
// **不查中间祖先**。实测把 hestiaStatusCmd 那行去掉、只留父命令 hestiaCmd 上的，
// `hestia status --hestia-config /nonexistent/nope.yaml` 仍打出完整 13 行 usage。
// ⇒ 循环里逐个断言不是啰嗦，**少任何一个叶子都会真的漏**。
func TestHestiaCommandsSilenceUsage(t *testing.T) {
	for _, c := range []*cobra.Command{hestiaCmd, hestiaIngestCmd, hestiaStatusCmd} {
		assert.Truef(t, c.SilenceUsage, "%s 必须设 SilenceUsage，否则每次失败灌一屏 usage 把错误埋掉", c.Name())
	}

	// 行为侧自证：真的跑一次失败路径，输出里不得出现 usage 的特征串。
	// 只断言字段为 true 不够 —— 字段对而行为错是可能的（上面那次继承实测就是一例：
	// 父命令字段为 true，而实际执行的子命令照样打 usage）。
	withConfig(t, "storage:\n  db_path: /nonexistent/x.db\ndiscover:\n  index_url: \"\"\n")
	cmd, out := newCapturingCmd()
	err := runHestiaStatus(cmd, nil)
	require.Error(t, err, "非法配置必须报错")
	assert.NotContains(t, out.String(), "Usage:", "失败输出里不得有 usage")
	assert.NotContains(t, out.String(), "Global Flags:", "失败输出里不得有 flag 清单")
}

// 🔴 db_path 的绝对路径解析归 TASK-006 的 RenderStatus，**本层不得再解析一次**。
//
// 两处都解析行为无害（幂等），但下一个人读到 RenderStatus 那句「解析发生在这里」会来找
// 第二处并删掉它 —— 而删错一处就静默改变了 cwd 语义。本 Sprint 已就此裁决过一次
// （TASK-006 non_functional[0] 的 reviewer D7）。
//
// 判据用「源码不 import path/filepath」而不是比对输出：RenderStatus 自己就会解析，
// 所以从输出上**看不出**这一层有没有多解析一次 —— 那正是它容易被悄悄加进来的原因。
func TestHestiaCmdDoesNotResolveDBPath(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "hestia.go", nil, parser.ImportsOnly)
	require.NoError(t, err)

	for _, imp := range f.Imports {
		assert.NotEqualf(t, `"path/filepath"`, imp.Path.Value,
			"hestia.go 不该 import path/filepath —— db_path 的解析归 RenderStatus，"+
				"这一层只把配置里的相对路径原样传下去")
	}
}

// newCapturingCmd 与同包的 newDiscardCmd 同形，只是把 out/err 收进一个 buffer，
// 供需要**看输出**的用例用（newDiscardCmd 丢弃输出，断言不了内容）。
func newCapturingCmd() (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetContext(context.Background())
	c.SetOut(&buf)
	c.SetErr(&buf)
	return c, &buf
}

// withConfig 写一个临时配置并把全局 flag 指过去，测试结束后复原。
func withConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hestia.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

	old := hestiaCfgPath
	hestiaCfgPath = p
	t.Cleanup(func() { hestiaCfgPath = old })
	return p
}

// 配置错误要在开库之前就报出来。
func TestHestiaStatusFailsOnBadConfig(t *testing.T) {
	withConfig(t, "discover:\n  index_url: \"\"\n")

	err := runHestiaStatus(newDiscardCmd(), nil)
	require.Error(t, err)
}

// ingest 侧同样要传播配置错误 —— **不能只测 status**。
//
// 两条 RunE 各自调一次 openHestia，是两条独立的返回路径；只测 status 时
// runHestiaIngest 的错误分支**一行都没被执行过**（实测：改动前它整函数 0% 覆盖）。
// 而 ingest 才是 launchd 的入口 —— 配置错却退 0，会让「每天三次静默不干活」看起来一切正常。
//
// ⚠️ 只走**配置失败**这一条：配置正确的路径会真的发 HTTP（NewPBOCFetcher），
// cmd 层按计划 1321 行「只测装配对不对」，不在这一层碰网络。
func TestHestiaIngestPropagatesConfigError(t *testing.T) {
	old := hestiaCfgPath
	hestiaCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
	t.Cleanup(func() { hestiaCfgPath = old })

	err := runHestiaIngest(newDiscardCmd(), nil)
	require.Error(t, err, "配置读不到必须报错，否则 launchd 每天三次静默空跑而退出码是 0")
	assert.Contains(t, err.Error(), hestiaCfgPath, "错误要带出是哪份配置文件")
}

// 配置合法但**库打不开**时同样要报错 —— 这是启动期第三种失败，且最容易被误读。
//
// 前两种（配置不存在 / 配置非法）用户一看就懂；这一种的现场是「配置明明是对的」，
// 而真正的原因是 db_path 相对路径 + cwd 不对（launchd 的 WorkingDirectory 没设对是
// 首次部署最常见的错）。**它必须响亮失败**，否则 openHestia 那条 NewStore 错误分支
// 一行都不会被执行过 —— 实测：补这条之前 hestia.go 的 92 行未覆盖。
//
// 用一个普通文件占住本该是目录的位置，MkdirAll 必然失败（与 internal/hestia 的
// TestNewStoreFailsOnUnopenablePath 同一手法）。
func TestHestiaStatusFailsWhenStoreCannotOpen(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	withConfig(t, `
storage:
  db_path: `+filepath.Join(blocker, "sub", "hestia.db")+`
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)

	err := runHestiaStatus(newDiscardCmd(), nil)
	require.Error(t, err, "库打不开必须报错，不能当成空库跑出一份「0 期」的正常报告")
}

// 配置装载失败的错误必须带上**是哪一份配置** —— db_path 与 cwd 的组合已经够绕，
// 不说清读的是哪个文件，用户会去改另一份而百思不解。
func TestHestiaConfigErrorNamesThePath(t *testing.T) {
	t.Run("文件不存在", func(t *testing.T) {
		old := hestiaCfgPath
		hestiaCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
		t.Cleanup(func() { hestiaCfgPath = old })

		err := runHestiaStatus(newDiscardCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), hestiaCfgPath, "错误要带出是哪份配置文件")
	})

	t.Run("配置非法", func(t *testing.T) {
		p := withConfig(t, "discover:\n  index_url: \"\"\n")

		err := runHestiaStatus(newDiscardCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), p, "错误要带出是哪份配置文件")
	})
}

// 空库上 status 能跑通 —— 这是首次部署后的第一条命令。
//
// 配置里用**绝对路径**：db_path 的相对路径按进程 cwd 解析，而 go test 的 cwd 是包目录
// —— 写相对路径会在 cmd/atlas/ 下建库文件。
func TestHestiaStatusOnEmptyStore(t *testing.T) {
	dir := t.TempDir()
	withConfig(t, `
storage:
  db_path: `+filepath.Join(dir, "hestia.db")+`
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
`)

	cmd, out := newCapturingCmd()
	require.NoError(t, runHestiaStatus(cmd, nil))

	// 跑通不等于说得出话：空库 status 是「配置装载错」与「库路径错」唯一的区分手段，
	// 它必须真的打出计数（两者都表现为「0 期」，而那正是要被区分开的东西）。
	assert.Contains(t, out.String(), "observations: 0")
	assert.Contains(t, out.String(), "pending: 0")
}
