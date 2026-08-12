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
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
		if err != nil {
			break
		}
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

	// 数 <key>Hour</key> / <key>Minute</key> 的配对值。用 XML 解析而不是子串，
	// 理由同 plistEnvKeys：本文件注释里也写着这几个时刻。
	hours, minutes := plistIntsUnderKey(t, raw, "Hour"), plistIntsUnderKey(t, raw, "Minute")

	require.Len(t, hours, 3, "spec 定的是每日三个时点；解析不出三个说明排班键被改坏或删掉了")
	require.Len(t, minutes, 3)

	got := make([][2]int, 3)
	for i := range hours {
		got[i] = [2]int{hours[i], minutes[i]}
	}
	assert.Equal(t, [][2]int{{15, 30}, {17, 30}, {21, 30}}, got,
		"时刻取自计划 2026-08-12-hestia-cli.md:1634-1638；要改先改 spec")
}

// plistIntsUnderKey 取出所有 `<key>NAME</key><integer>N</integer>` 里的 N，按出现顺序。
func plistIntsUnderKey(t *testing.T, raw []byte, name string) []int {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var (
		out     []int
		lastTxt string
		want    bool
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch tv := tok.(type) {
		case xml.CharData:
			txt := strings.TrimSpace(string(tv))
			if want && txt != "" {
				n, convErr := strconv.Atoi(txt)
				require.NoError(t, convErr, "<%s> 后面应当是整数，得到 %q", name, txt)
				out = append(out, n)
				want = false
			}
			if txt != "" {
				lastTxt = txt
			}
		case xml.EndElement:
			if tv.Name.Local == "key" && lastTxt == name {
				want = true
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
