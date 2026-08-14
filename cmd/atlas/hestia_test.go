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
	"encoding/xml"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/hestia"
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

// ── T7 / TASK-009：真实配置与 plist 的守卫 ──────────────────────────────────

// 真实的 configs/hestia.yaml 必须能被 LoadConfig 装载成功（reviewer D3）。
//
// 先例：crisis_test.go 也读真实的 ../../configs/crisis-monitor.yaml。
//
// ⚠️ **不加这条的话，「这份配置本身能不能装载」的唯一验证是手工验收** —— 而手工
// 验收被刻意排除在 DoD 之外，等于没有自动守卫。配置写错的后果不是编译失败，是
// launchd 每天三次静默空跑、退出码却可能看起来正常。
//
// 🔴 它**隐含要求这份配置能过 `Config.validate()` 的每一条**，其中两条最容易漏：
//   - `storage.db_path` 必填非空（TASK-002 新加）
//   - `caliber_exemptions` 若出现则每条都必须写 `period_types`（留空不等于「全部」）
//
// 漏了 `storage` 段时本条**会红，但红的理由指向 db_path**，而不是它想测的东西 ——
// 所以下面把 validate 关心的字段逐个断言出来，让「哪一条没写对」一眼可见。
func TestRealHestiaConfigLoads(t *testing.T) {
	cfg, err := hestia.LoadConfig("../../configs/hestia.yaml")
	require.NoError(t, err, "真实配置必须能装载：它是 launchd 每天三次唤起时读的那一份")

	assert.NotEmpty(t, cfg.ConfigVersion, "config_version 要写，否则契约里查不出用的哪版配置")
	assert.NotEmpty(t, cfg.Storage.DBPath, "storage.db_path 必填（validate 的第一条）")
	assert.NotEmpty(t, cfg.Discover.IndexURL)
	assert.GreaterOrEqual(t, cfg.Discover.MaxPages, 1)
	assert.Positive(t, cfg.Discover.Timeout)

	// 五个阈值都必须 > 0：validate 的第二道防线挡的是「显式写 0」，
	// 而 0 容差会让每一期都超差进 pending —— 与漏写的后果完全一样。
	assert.Positive(t, cfg.Thresholds.DepositSumTolerance)
	assert.Positive(t, cfg.Thresholds.DepositSumDriftMax)
	assert.Positive(t, cfg.Thresholds.CorpLoanTolerance)
	assert.Positive(t, cfg.Thresholds.StockContinuityMax)
	assert.Positive(t, cfg.Thresholds.YoYSanityMax)
}

// plistEnvKeys 解析 plist 里 EnvironmentVariables 那个 dict 的**键名**。
//
// 用 encoding/xml 逐 token 扫，**不是子串匹配**：DoD 要求精确断言。子串匹配分不出
// 「键是 http_proxy」与「注释里提到 http_proxy」—— 而本文件的 plist 注释里恰恰
// 大段讨论了代理键，子串匹配会当场误报。
func plistEnvKeys(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	dec := xml.NewDecoder(f)
	var (
		lastKey  string   // 最近一个 <key> 的文本
		inEnv    bool     // 已进入 EnvironmentVariables 的 dict
		depth    int      // 相对该 dict 的嵌套深度
		keys     []string //
		wantDict bool     // 刚读到 EnvironmentVariables 这个键，下一个 <dict> 就是它
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		// 🔴 **解析错误必须响亮失败，不能 break 了事**。原先这里是 `if err != nil { break }`
		// —— 那会在遇到非法 XML 时**静默返回已收集的部分键**，而后面的代理键就此看不见。
		// 这不是假想：`com.newthinker.atlas.aktools.plist` 的注释里含 `--`（XML 注释不允许），
		// Go 的 encoding/xml 与 Python 的 expat 都拒绝它，而 **`plutil -lint` 报它 OK**。
		// ⇒ 同类写法一旦进到本文件，静默截断会让这条守卫瞎掉。
		require.NoErrorf(t, err, "解析 %s 失败：守卫不能在非法 XML 上静默返回部分结果", path)
		switch t2 := tok.(type) {
		case xml.StartElement:
			switch {
			case inEnv && t2.Name.Local == "dict":
				depth++
			case wantDict && t2.Name.Local == "dict":
				inEnv, wantDict, depth = true, false, 0
			}
		case xml.CharData:
			lastKey = string(t2)
		case xml.EndElement:
			switch {
			case t2.Name.Local == "key" && inEnv && depth == 0:
				keys = append(keys, lastKey)
			case t2.Name.Local == "key" && lastKey == "EnvironmentVariables":
				wantDict = true
			case t2.Name.Local == "dict" && inEnv:
				if depth == 0 {
					inEnv = false
				}
				depth--
			}
		}
	}
	return keys
}

// 🔴 hestia 的 plist **一个代理键都不设**（约束 C6 的正解）。
//
// hestia 直连央行：`NewPBOCFetcher` 给 http.Client 配了空 Transport 来绕开进程级
// 代理，理由是「代理未必在跑」——launchd 唤起时若 clash 没启动，走代理会连接失败，
// 而直连本来能成。
//
// ⚠️ **断言是「不存在任何 `*_proxy` / `*_PROXY` 键」，不是枚举四个名字**（reviewer C3）。
// 计划给的实现只枚举 `http_proxy/https_proxy/HTTP_PROXY/HTTPS_PROXY`，而 crisis 的
// plist 里还有一个 `no_proxy` ⇒ 照抄再删那两对，`no_proxy` 留下、测试全绿，而
// 「一个代理键都不设」这句话是假的。
//
// 更要紧的是**位置也靠不住**（本任务实测）：`no_proxy` 在 `crisis-daily` /
// `crisis-intraday-jpy` 里被 `PATH` 和一段注释与那两对隔开，在 `refresh-cnhk` /
// `prism-daily` 里却与它们相邻 ⇒ 任何位置性或枚举性的判据都不可靠，只能按**后缀**判。
//
// ⚠️ **否定式断言在空集上平凡为真**（Sprint 036 G9）：解析失败返回空切片时，
// 「不含代理键」照样通过。下面 require.NotEmpty + 断言 PATH 在场，就是那条**肯定式
// 锚点**，与否定式那条**互补，不是重复** —— 删掉任一条都会开一个缺口。
func TestHestiaPlistSetsNoProxyKeys(t *testing.T) {
	const plistPath = "../../deploy/launchd/com.newthinker.atlas.hestia-ingest.plist"
	keys := plistEnvKeys(t, plistPath)

	// 肯定式锚点：解析真的产出了东西，且是我们要的那个 dict。
	require.NotEmpty(t, keys, "前置锚点：解析不出任何环境变量键时，下面的否定式断言平凡为真")
	assert.Contains(t, keys, "PATH", "PATH 必须在 —— 它也证明我们解析的确实是那个 dict")

	// 否定式：按后缀判，不枚举名字。
	for _, k := range keys {
		lower := strings.ToLower(k)
		assert.NotContainsf(t, lower, "proxy",
			"plist 不得设任何代理键，实测到 %q；hestia 直连央行（NewPBOCFetcher 用空 Transport 绕开代理）", k)
	}

	// 🔴 **阳性对照：证明这个解析器真的看得见代理键。**
	//
	// 上面那圈断言在一个「恒返回 []string{"PATH"} 的坏解析器」上**同样全绿** ——
	// 前置锚点挡得住空集，挡不住「解析出了东西但漏掉了代理键」。所以这里拿一份
	// **真的设了代理键**的 plist 跑同一个解析器：它必须报出来。
	//
	// 用 crisis-daily 而不是合成 XML：合成的只证明解析器能处理我写的那种形状，
	// 真实文件才证明它能处理**仓库里实际存在**的那种（含注释、含 PATH 夹在中间）。
	t.Run("阳性对照：解析器在真的有代理键时必须报出来", func(t *testing.T) {
		crisis := plistEnvKeys(t, "../../deploy/launchd/com.newthinker.atlas.crisis-daily.plist")
		require.NotEmpty(t, crisis)

		var proxies []string
		for _, k := range crisis {
			if strings.Contains(strings.ToLower(k), "proxy") {
				proxies = append(proxies, k)
			}
		}
		assert.NotEmpty(t, proxies,
			"crisis-daily 确实设了代理键；这里报不出来说明上面那圈否定式断言是瞎的")
		assert.Contains(t, proxies, "no_proxy",
			"尤其是 no_proxy —— 它被 PATH 和一段注释与另两对隔开，最容易被漏掉")
	})
}

// 🔴 plist 必须**真的排了班**：三个时点，且与 spec 定的一致。
//
// ⚠️ **本条是我自己踩出来的**：改时刻那一步我误把整个 `StartCalendarInterval` 数组
// 连同注释一起删了，而 **`plutil -lint` 照样报 `OK`** —— 一个没有排班键的 plist
// 是**合法的 plist**，它只是**永远不会被唤起**。lint 管的是 XML 合不合法，管不了
// 「这个 job 到底跑不跑」。
//
// 后果是最坏的那一类：`install-services.sh` 装得上、`launchctl list` 看得见、
// 日志目录空着，而**一切看起来都正常**。
//
// 时刻取自方案计划 `2026-08-12-hestia-cli.md:1634-1638`（1632 行的理由：
// 「每日三个时点覆盖发布窗口与延迟发布，非发布期靠幂等空跑」）。**改时刻要先改 spec。**
func TestHestiaPlistSchedulesThreeTimes(t *testing.T) {
	const plistPath = "../../deploy/launchd/com.newthinker.atlas.hestia-ingest.plist"
	raw, err := os.ReadFile(plistPath)
	require.NoError(t, err)

	// 只认 StartCalendarInterval 之下的 dict，且 Hour/Minute 必须同出一个 dict。
	// 用 XML 解析而不是子串：本文件注释里也写着这几个时刻。
	got := plistSchedule(t, raw)

	require.Len(t, got, 3,
		"spec 定的是每日三个时点；解析不出三个说明排班键被改名、改坏或删掉了")
	assert.Equal(t, [][2]int{{15, 30}, {17, 30}, {21, 30}}, got,
		"时刻取自计划 2026-08-12-hestia-cli.md 的 StartCalendarInterval 段；要改先改 spec")
}

// plistSchedule 取出 `StartCalendarInterval` 数组里每个 `<dict>` 的 {Hour, Minute}。
//
// 🔴 **它替换的上一版是本 Sprint 第四个假通过，而且就在为修第三个而写的测试里。**
// 上一版（`plistIntsUnderKey`）在**全文档**范围数 `<key>Hour</key>` 与 `<key>Minute</key>`，
// 再**按下标**把两个独立列表配对 —— 从不断言这些整数位于 `StartCalendarInterval` 之下，
// 也不断言 Hour 与 Minute 出自**同一个** dict。QA 的两条变异实测：
//
//   - 把键名打错一个字母 ⇒ launchd 忽略未知键、job 永不唤起，而断言**全绿**
//   - Hour/Minute 跨 dict 错配（排班从 3 次/天变成约 86 次/天）⇒ **同样全绿**
//
// 两者的后果**与这条测试自述要防的完全相同**：装得上、`launchctl list` 看得见、
// 日志目录空着，而一切看起来都正常。
//
// ⇒ 本版按 `plistEnvKeys` 的做法**限定作用域**：先认出 `StartCalendarInterval` 后面
// 那个 `<array>`，再逐 `<dict>` 收成对的字段，**缺任一字段即失败**。
func plistSchedule(t *testing.T, raw []byte) [][2]int {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(raw))

	var (
		lastTxt   string
		wantArray bool // 刚读到 StartCalendarInterval 这个键，下一个 <array> 就是它
		inArray   bool
		depth     int            // 相对该 array 的嵌套深度
		cur       map[string]int // 当前 <dict> 已收到的字段
		curKey    string         // 当前 <dict> 里最近一个 <key>
		out       [][2]int
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "解析 plist 失败：守卫不能在非法 XML 上静默返回部分结果")

		switch tv := tok.(type) {
		case xml.StartElement:
			switch {
			case wantArray && tv.Name.Local == "array":
				inArray, wantArray, depth = true, false, 0
			case inArray && tv.Name.Local == "dict":
				depth++
				if depth == 1 {
					cur, curKey = map[string]int{}, ""
				}
			}
		case xml.CharData:
			txt := strings.TrimSpace(string(tv))
			if txt == "" {
				break
			}
			// <integer> 的文本紧跟在它自己的 <key> 之后；只在 array 内的第一层 dict 里收。
			if inArray && depth == 1 && curKey != "" {
				if n, convErr := strconv.Atoi(txt); convErr == nil {
					cur[curKey] = n
					curKey = ""
				}
			}
			lastTxt = txt
		case xml.EndElement:
			switch {
			case tv.Name.Local == "key" && !inArray && lastTxt == "StartCalendarInterval":
				wantArray = true
			case tv.Name.Local == "key" && inArray && depth == 1:
				curKey = lastTxt
			case tv.Name.Local == "dict" && inArray:
				if depth == 1 {
					h, okH := cur["Hour"]
					m, okM := cur["Minute"]
					// 缺字段即失败：Hour/Minute 必须出自**同一个** dict。
					require.Truef(t, okH && okM,
						"StartCalendarInterval 的第 %d 个 dict 缺字段（Hour=%v Minute=%v，收到的键 %v）",
						len(out)+1, okH, okM, cur)
					out = append(out, [2]int{h, m})
				}
				depth--
			case tv.Name.Local == "array" && inArray && depth == 0:
				inArray = false
			}
		}
	}
	return out
}

// plist 必须能过 `plutil -lint`（reviewer D2）。
//
// ⚠️ **纯字符串/XML 守卫挡不住这件事**：`install-services.sh:34` 会跑 `plutil -lint`，
// 而 XML 写坏时上面那些 Go 断言可能照样绿（encoding/xml 比 plutil 宽容），
// **安装时才炸**。把它拉进测试，失败就落在这里而不是运维手上。
//
// macOS 专属工具；找不到就跳过，不把「环境没有 plutil」记成交付缺陷。
func TestHestiaPlistPassesPlutilLint(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil 不可用（非 macOS），跳过；install-services.sh 仍会在安装时跑它")
	}
	const plistPath = "../../deploy/launchd/com.newthinker.atlas.hestia-ingest.plist"
	out, err := exec.Command("plutil", "-lint", plistPath).CombinedOutput()
	require.NoErrorf(t, err, "plutil -lint 未通过，安装时会失败：%s", out)
}

// install-services.sh 必须真的**装**这个 plist。
//
// 没有这条的话，plist 写得再对也只是一个没人加载的文件 —— 而那种失败是**静默**的：
// 部署脚本照常成功，服务从来没起来过。
//
// 🔴 **断言的是「它在 `for L in …` 那个安装列表里」，不是「文件里出现过这个串」**。
// 消融实测（本任务）：把标签从安装循环里删掉、只留文件头注释里那一行，
// **子串版断言照样全绿** —— 而 plist 从此不再被安装。
// 与 plist 那条守卫是同一条纪律：**注释里提到 ≠ 生效**。
func TestInstallServicesInstallsHestiaPlist(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/ops/install-services.sh")
	require.NoError(t, err)

	// 取 `for L in ... ; do` 之间那一段（可跨行续行），只在它里面找。
	m := regexp.MustCompile(`(?s)for L in\s(.*?);\s*do`).FindStringSubmatch(string(raw))
	require.Len(t, m, 2, "前置锚点：没找到安装循环，下面的断言会在错的文本上做")

	loop := m[1]
	assert.Contains(t, loop, "com.newthinker.atlas.hestia-ingest",
		"hestia-ingest 必须在安装循环的标签列表里，否则 plist 不会被加载")
	// 阴性对照：确认我们截到的确实是那段列表，而不是整个文件。
	assert.NotContains(t, loop, "每天 15:30", "截取范围不该包含文件头的注释段")
}

// 🔴 flag 必须**真的绑到变量上** —— 经 cobra 的参数解析验，不是查 flag 存不存在。
//
// ⚠️ **`TestHestiaFlags` 只断言 flag 存在，而那正是失效的那一条**：把
// `BoolVar(&hestiaForce, …)` 换成 `Bool(…)`，flag 仍在、仍叫 `force`、`--help` 一模一样，
// **只是不再写回 `hestiaForce`** ⇒ `--force` 静默失效。实测（QA M13/M14，我独立复现）：
// 两处一起换掉之后 `go test ./cmd/atlas/ ./internal/hestia/ -count=1` **EXIT=0、零 FAIL**。
//
// 后果落在设计上唯一的报警通道之外：`atlas hestia ingest --force` 打出
// `no new reports (stopped: seen_article)` 然后 **exit 0** —— 与「今天真的没有新报告」
// **逐字同形**。而 `--force` 是 CONTRACTS 写明的**唯一恢复出口**
// （搜 `否则调了阈值之后没有任何办法重跑`）。
//
// 🔴 **判据是肯定式的**：「解析之后变量等于我传进去的值」。
// 「flag 存在」是否定式判据的近亲 —— 它在**绑定断裂**这个失效上恒真。
//
// ⚠️ 本用例改全局变量，故**不能 t.Parallel**，并用 t.Cleanup 还原
// （同包其它用例直接读写这两个变量）。
func TestHestiaFlagsBindToVariables(t *testing.T) {
	oldForce, oldCfg := hestiaForce, hestiaCfgPath
	t.Cleanup(func() { hestiaForce, hestiaCfgPath = oldForce, oldCfg })

	t.Run("--force 绑到 hestiaForce", func(t *testing.T) {
		hestiaForce = false
		require.NoError(t, hestiaIngestCmd.Flags().Parse([]string{"--force"}))
		assert.True(t, hestiaForce,
			"--force 必须写回 hestiaForce；绑定断裂时 flag 照样存在、--help 一模一样，而 --force 静默失效")
	})

	t.Run("--hestia-config 绑到 hestiaCfgPath", func(t *testing.T) {
		const want = "/tmp/some-explicit-hestia.yaml"
		hestiaCfgPath = ""
		require.NoError(t, hestiaCmd.PersistentFlags().Parse([]string{"--hestia-config", want}))
		assert.Equal(t, want, hestiaCfgPath,
			"绑定断裂时会静默读默认路径的配置 —— 生产上因 plist 的 WorkingDirectory 碰巧无害，"+
				"手工在别处跑就会读错配置而不报错")
	})
}

// ============================================================================
// M1c-1 的 TASK-007：backfill fetch 子命令装配
//
// Context Checkpoint: done_criteria → test mapping
//   functional[0] 两层子命令注册   → TestHestiaBackfillFetchIsRegistered
//   functional[1] 四个 flag 真解析 → TestHestiaBackfillFlagsBindThroughCobra
//   boundary[0]   --out 必填       → TestHestiaBackfillFetchRequiresOut
//   boundary[1]   --from 格式校验  → TestHestiaBackfillFetchRejectsBadFrom
// ============================================================================

// bfResetFlags 把四个绑定变量清零、**并清掉 flag 自己的 Changed 状态**。
//
// ⚠️ 两样都要清，少一样就会跨用例污染 —— 而这是我实测撞出来的：
// 只清变量时，`TestHestiaBackfillFetchRequiresOut` **单跑 PASS、与其它用例同跑 FAIL**。
// 成因是 `hestiaBackfillFetchCmd` 是**包级全局**，pflag 把「这个 flag 被显式传过」
// 记在 `flag.Changed` 上并**跨 Execute 保留**；前面的用例传过 `--out`，
// 于是 cobra 的 required 校验认为它「已提供」，那条用例就静默失去了鉴别力。
//
// 🔴 **单跑绿、同跑红**这种形态最容易被读成「测试不稳定」而加 `-run` 绕过去 ——
// 它其实是在报告一个真实的共享状态。
func bfResetFlags(t *testing.T) {
	t.Helper()
	oF, oO, oP, oA := hestiaBackfillFrom, hestiaBackfillOut, hestiaBackfillExpectPeriods, hestiaBackfillExpectArticles
	t.Cleanup(func() {
		hestiaBackfillFrom, hestiaBackfillOut = oF, oO
		hestiaBackfillExpectPeriods, hestiaBackfillExpectArticles = oP, oA
	})
	hestiaBackfillFrom, hestiaBackfillOut = "", ""
	hestiaBackfillExpectPeriods, hestiaBackfillExpectArticles = 0, 0
	hestiaBackfillFetchCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

// bfExec 从**根命令**按真实路径跑一次，输出丢弃。
//
// ⚠️ 用 `SetArgs` + `Execute()` 而不是 `Flags().Parse()`：后者绕过了命令树查找与
// required-flag 校验，而本任务要验的恰恰是「装配对不对」——两层嵌套走不走得通、
// `--out` 缺失会不会被 cobra 拦下。
func bfExec(t *testing.T, args ...string) error {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	rootCmd.SetArgs(args)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	return rootCmd.Execute()
}

// 🔴 functional[0]：`atlas hestia backfill fetch` **两层嵌套**都要装上。
//
// 判据是**从根命令按路径找得到**，而不是「那个变量非 nil」——后者对一个建了命令
// 却忘了 AddCommand 的实现同样为真，而那种实现在 CLI 上根本调不出来。
func TestHestiaBackfillFetchIsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"hestia", "backfill", "fetch"})
	require.NoError(t, err, "从根命令按 hestia→backfill→fetch 必须找得到")
	require.NotNil(t, cmd)

	assert.Equal(t, "fetch", cmd.Name())
	assert.NotNil(t, cmd.RunE, "叶子命令必须有 RunE，否则它只会打印 usage")

	// 中间层也要能单独找到 —— 少了它，`atlas hestia backfill` 会被当成未知命令。
	mid, _, err := rootCmd.Find([]string{"hestia", "backfill"})
	require.NoError(t, err)
	assert.Equal(t, "backfill", mid.Name())
	assert.True(t, mid.HasSubCommands(), "backfill 是中间层，必须挂着子命令")
}

// 🔴 functional[1]：四个 flag **经 cobra 真实解析**后写进了正确的变量。
//
// ⚠️ 不查 `Lookup("from") != nil`——那只证明 flag 存在，证不了它绑到了哪个变量。
// 绑定断裂时 `--help` 一模一样、flag 也照样存在，而值静默丢失。
//
// 🔴 `--expect-*` **不传即零值**（TASK-008 的 notes_for_downstream 定死）：零值走
// 推算值告警路径，**不是**「默认某个数」。所以下面第二个子用例是必须的——
// 少了它，一个「不传就填 79/217」的错误实现在第一个子用例上照样绿。
func TestHestiaBackfillFlagsBindThroughCobra(t *testing.T) {
	t.Run("四个都传 ⇒ 四个变量都拿到值", func(t *testing.T) {
		bfResetFlags(t)
		withConfig(t, "discover:\n  index_url: \"\"\n") // 让 RunE 在装配之后、触网之前失败

		err := bfExec(t, "hestia", "backfill", "fetch",
			"--from", "2020-01", "--out", "/tmp/bf-probe",
			"--expect-periods", "79", "--expect-articles", "217")
		require.Error(t, err, "前置锚点：配置无效 ⇒ RunE 报错；若这里 nil 说明它真去抓了网")

		assert.Equal(t, "2020-01", hestiaBackfillFrom)
		assert.Equal(t, "/tmp/bf-probe", hestiaBackfillOut)
		assert.Equal(t, 79, hestiaBackfillExpectPeriods)
		assert.Equal(t, 217, hestiaBackfillExpectArticles)
	})

	t.Run("--expect-* 不传 ⇒ 保持零值（不是默认 79/217）", func(t *testing.T) {
		bfResetFlags(t)
		withConfig(t, "discover:\n  index_url: \"\"\n")

		err := bfExec(t, "hestia", "backfill", "fetch", "--from", "2020-01", "--out", "/tmp/bf-probe")
		require.Error(t, err)

		// 🔴 下面两条查的是 **flag 注册时声明的默认值**，不是变量的当前值。
		//
		// 这不是重复，两条查的是**不同的东西**，缺一条就有一个错误实现钻得过去：
		//
		//	Lookup(...).DefValue  → 「注册时声明的默认值是 0」
		//	assert.Zero(变量)      → 「解析路径没有往变量里塞东西」
		//
		// ⚠️ 只有下面那对 assert.Zero 时，**本子用例对它声称要防的实现没有鉴别力** ——
		// 实测：把 `IntVar(..., 0, ...)` 改成 `79` / `217`（DoD 原文点名的那个错误实现），
		// 全包 282 条**无一变红**。成因是 cobra 的默认值在 **init() 注册时**就写进了变量，
		// `Execute()` 对**未传**的 flag 不会重新应用它，而 bfResetFlags 在子用例开头把变量
		// 显式清零了 ⇒ **重置动作把默认值抹掉，assert.Zero 恒真**。
		//
		// 🔴 讽刺的是 bfResetFlags 本身是**正确**的测试隔离（上面那条注释记了它为什么必须存在），
		// 它恰好摧毁了本子用例的鉴别力。⇒ **测试隔离与鉴别力可以互相拆台，而且都不出声。**
		//
		// 危害是实的：默认值若真是 79/217，生产路径（无重置）下不传 `--expect-*` 就会走
		// **硬失败**分支 ⇒ 让一个**推算值**取得阻断交付的权力，而那正是 TASK-008 的
		// notes_for_downstream 反复声明不许发生的事。
		//
		// DefValue 是字符串（pflag 存的是注册时的字面量），所以断言 "0" 而不是 0。
		assert.Equal(t, "0", hestiaBackfillFetchCmd.Flags().Lookup("expect-periods").DefValue,
			"注册时的默认值必须是 0 —— 非零默认值会让推算值有权阻断交付")
		assert.Equal(t, "0", hestiaBackfillFetchCmd.Flags().Lookup("expect-articles").DefValue)

		assert.Zero(t, hestiaBackfillExpectPeriods, "零值 = 未显式传入 ⇒ 走推算值告警路径")
		assert.Zero(t, hestiaBackfillExpectArticles)
	})
}

// 🔴 boundary[0]：`--out` **必填**，缺失即报错。
//
// 不给默认值是刻意的：产物目录必须是仓库外的绝对路径（人类裁决），
// **让「误落进仓库」需要显式打出来才会发生**。
func TestHestiaBackfillFetchRequiresOut(t *testing.T) {
	bfResetFlags(t)
	withConfig(t, "discover:\n  index_url: \"\"\n")

	err := bfExec(t, "hestia", "backfill", "fetch", "--from", "2020-01")

	require.Error(t, err, "--out 缺失必须报错，不能落到某个默认目录")
	assert.Contains(t, err.Error(), "out", "错误要点名是哪个 flag 缺了")
}

// 🔴 boundary[1]：`--from` 格式非法即报错。至少两条：格式不对的、月份越界的。
//
// ⚠️ 月份越界那条单独列，是因为 `\d{4}-\d{2}` 这类宽松校验**认得 2020-13 与 2020-00**——
// 放过去之后它会变成一个语义非法的 time.Time，而回填按发布日期判停，
// 错一个月就少抓/多抓一整批。
func TestHestiaBackfillFetchRejectsBadFrom(t *testing.T) {
	for _, tt := range []struct{ name, from string }{
		{"格式不对：缺月份", "2020"},
		{"格式不对：带日", "2020-01-15"},
		{"格式不对：单位数月", "2020-1"},
		{"月份越界：13 月", "2020-13"},
		{"月份越界：0 月", "2020-00"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bfResetFlags(t)
			withConfig(t, "discover:\n  index_url: \"\"\n")

			err := bfExec(t, "hestia", "backfill", "fetch", "--from", tt.from, "--out", "/tmp/bf-probe")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "from", "错误要点名是 --from，否则读的人分不清是哪个参数错了")
		})
	}
}

// 🔴 回填**不继承** `discover.max_pages`，而是留零值走 hestia 包的兜底上限。
//
// 实撞：我第一版传了 `cfg.Discover.MaxPages`，真跑**第一秒**就被拦下 ——
// configs 里那个值是 **3**（日常增量每天翻三页），而回填要翻约 150 页。
//
// ⚠️ 拦下它的是 TASK-002 那条「翻满 MaxPages ⇒ 报错」守卫。**若当初把它写成
// 「翻满就返回已收集的部分」，这里会静默只抓最近三页，而回填看起来跑完了。**
// ⇒ 这条用例钉住的是**装配**，不是那条守卫：下一个人「顺手」把 MaxPages 补回去时，
// 它会红，而不是等到下一次真跑才发现。
func TestHestiaBackfillConfigDoesNotInheritDiscoverMaxPages(t *testing.T) {
	bfResetFlags(t)
	hestiaBackfillOut = "/tmp/bf-probe"

	cfg := hestia.Config{}
	cfg.Discover.IndexURL = "https://example.invalid/index.html"
	cfg.Discover.MaxPages = 3 // 日常增量的窗口
	cfg.Discover.Timeout = 30 * time.Second

	got := hestiaBackfillConfig(cfg, io.Discard, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))

	assert.Zero(t, got.MaxPages,
		"回填必须留零值走自己的兜底上限；继承 discover.max_pages=3 会让它只翻三页")
	assert.Equal(t, cfg.Discover.IndexURL, got.IndexURL, "入口 URL 仍来自配置")
	assert.Equal(t, cfg.Discover.Timeout, got.Timeout, "单次请求超时可以共用，它与翻多少页无关")
	assert.Empty(t, got.Cutover,
		"Cutover 留空 ⇒ 用 hestia 包里的唯一定义；命令层再写一个 2025-09 会造出第二个定义处")
	assert.Equal(t, "/tmp/bf-probe", got.Out)
	assert.NotNil(t, got.Report, "报告要有去向")
}
