package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/hestia"
	"github.com/newthinker/atlas/internal/hestia/sheets"
)

// Context Checkpoint: done_criteria → test mapping (TASK-009)
//
// functional[0]     sheets push 挂在 hestia 下 / 五个 flag / 默认 dry-run 末尾固定一句 / 缺表提示
//                     → TestHestiaSheetsPushIsRegistered、TestHestiaSheetsPushFlagsBindThroughCobra、
//                       TestSheetsPushDefaultsToDryRun、TestFormatResultAnnouncesMissingTabs
// functional[1]     formatResult 三类计数分开三行各带数字 / push --all 经 newSheetsClient 缝跑通
//                     → TestFormatResultSeparatesThreeCounts、TestSheetsPushDefaultsToDryRun
// functional[2]     credentials_file 非空 ⇒ 填 ProjectSheets（Apply+CreateSheets 都真）；留空 ⇒ nil
//                     → TestSheetsProjectorNilWhenCredentialsEmpty、TestSheetsProjectorPushesWithApplyAndCreate、
//                       TestHestiaIngestDepsCarryProjector
// boundary[0]       flag 校验早于开库与建客户端 / --period 只推该期
//                     → TestSheetsPushRequiresPeriodOrAll、TestSheetsPushPeriodNeedsType、
//                       TestSheetsPushValidatesFlagsBeforeOpeningStore、TestPushSheetsOnlySendsRequestedPeriod
// error_handling[0] credentials_file 留空 ⇒ 打印固定一句并退出 0
//                     → TestSheetsPushPrintsDisabledAndExitsZero
// error_handling[1] 投影 panic ⇒ 转 error 走 C8「只打印不返回」，Ingest 返回 nil、outcome ingested
//                     → TestProjectSheetsPanicDoesNotBreakIngest、TestSheetsProjectorRecoversClientPanic
// non_functional[0] go test ./cmd/atlas/ 全绿、gofmt/vet 空、覆盖率以门禁输出为准（review）

// —— 测试辅助 ——

// sheetsExec 从**根命令**按真实路径跑一次并收输出，照同包 calExec 的写法。
//
// ⚠️ 必须先 Execute 再读 buf：写成 `return buf.String(), rootCmd.Execute()` 时 Go 从左到右
// 求值，buf 在命令跑之前就被读走了 ⇒ 输出恒为空（calExec 的注释记过这次实撞）。
func sheetsExec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	old := os.Args
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	t.Cleanup(func() {
		os.Args = old
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(io.Discard)
		rootCmd.SetErr(io.Discard)
		hestiaSheetsAll, hestiaSheetsPeriod, hestiaSheetsPeriodType = false, "", ""
		hestiaSheetsApply, hestiaSheetsCreate = false, false
		hestiaSheetsPushCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	})
	err := rootCmd.Execute()
	return buf.String(), err
}

// sheetsTestdata 指向 internal/hestia 的语料。跨包读而不复制：复制会得到两份需要同步的
// 真实语料，而它们没有任何机制保证同步（calibrateFixture 立的先例，理由逐字相同）。
func sheetsTestdata(t *testing.T, rel ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{"..", "..", "internal", "hestia"}, rel...)...)
	_, err := os.Stat(p)
	require.NoError(t, err, "跨包语料不在了：%s", p)
	return p
}

// fakeSheetsCredentials 把 sheets 包的假服务账号复制一份并把 token_uri 指向本地服务，
// 这样鉴权不出网。与 sheets 包的 fakeCredentials 同形。
func fakeSheetsCredentials(t *testing.T, srvURL string) string {
	t.Helper()
	raw, err := os.ReadFile(sheetsTestdata(t, "sheets", "testdata", "fake-sa.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	doc["token_uri"] = srvURL + "/token"
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(p, out, 0o600))
	return p
}

// stubSheetsServer 起一个只会发假 token、其余一律回 {} 的服务：够 Client 建起来并让
// Tabs 返回空表。它记下收到的每个 API 路径，供「一个写请求都没发」这类断言用。
func stubSheetsServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`)
			return
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// stubNewSheetsClient / stubSheetsPush 换掉两个包级测试缝，测试结束复原。
//
// 单列出来是因为「存旧值 → 注册复原 → 赋新值」这三行在本文件出现了八次；漏掉复原那行
// 不会让当前用例变红，而是让后面的用例拿到上一条留下的替身。
func stubNewSheetsClient(t *testing.T, fn func(context.Context, string, string, ...sheets.Option) (*sheets.Client, error)) {
	t.Helper()
	old := newSheetsClient
	t.Cleanup(func() { newSheetsClient = old })
	newSheetsClient = fn
}

func stubSheetsPush(t *testing.T, fn func(context.Context, *sheets.Client, []sheets.Row, []string, sheets.Options) (sheets.Result, error)) {
	t.Helper()
	old := sheetsPush
	t.Cleanup(func() { sheetsPush = old })
	sheetsPush = fn
}

// withStubSheetsClient 把 newSheetsClient 换成指向本地服务的真 Client，测试结束复原。
func withStubSheetsClient(t *testing.T) (string, *[]string) {
	t.Helper()
	srv, seen := stubSheetsServer(t)
	creds := fakeSheetsCredentials(t, srv.URL)
	stubNewSheetsClient(t, func(ctx context.Context, credentialsFile, spreadsheetID string, opts ...sheets.Option) (*sheets.Client, error) {
		return sheets.NewClient(ctx, credentialsFile, spreadsheetID, append(opts, sheets.WithEndpoint(srv.URL))...)
	})
	return creds, seen
}

// withCapturedPush 换掉 sheetsPush，记下它收到的 rows 与 opts，并回一个空 Result。
func withCapturedPush(t *testing.T) (*[]sheets.Row, *sheets.Options) {
	t.Helper()
	var gotRows []sheets.Row
	var gotOpts sheets.Options
	stubSheetsPush(t, func(_ context.Context, _ *sheets.Client, rows []sheets.Row, _ []string, opts sheets.Options) (sheets.Result, error) {
		gotRows, gotOpts = rows, opts
		return sheets.Result{}, nil
	})
	return &gotRows, &gotOpts
}

// sheetsCfg 造一份**够 openHestia 用**的配置，并把 hestia_sheets.credentials_file 指过去。
func sheetsCfg(t *testing.T, credentialsFile string) {
	t.Helper()
	withConfig(t, fmt.Sprintf(`
storage:
  db_path: %s
discover:
  index_url: https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html
  max_pages: 3
  timeout: 30s
hestia_sheets:
  credentials_file: %s
  spreadsheet_id: sheet-id
`, filepath.Join(t.TempDir(), "hestia.db"), credentialsFile))
}

// sampleSheetRows：三个不同年月的行，用来验「只推该期」确实按年月过滤。
func sampleSheetRows() []sheets.Row {
	return []sheets.Row{
		{Year: 2025, Month: 6, Cells: []sheets.Cell{{Label: "社融存量", Value: 462.06}}},
		{Year: 2025, Month: 12, Cells: []sheets.Cell{{Label: "社融存量", Value: 470.0}}},
		{Year: 2026, Month: 6, Cells: []sheets.Cell{{Label: "社融存量", Value: 480.0}}},
	}
}

// —— functional[0]：命令树与 flag ——

// 判据是**从根命令按路径找得到**，而不是「那个变量非 nil」——后者对一个建了命令却忘了
// AddCommand 的实现同样为真，而那种实现在 CLI 上根本调不出来（bfExec 那组立的先例）。
func TestHestiaSheetsPushIsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"hestia", "sheets", "push"})
	require.NoError(t, err, "从根命令按 hestia→sheets→push 必须找得到")
	require.Equal(t, "push", cmd.Name())
	require.True(t, cmd.SilenceUsage, "失败时不灌一屏 usage，与同包其余 hestia 子命令一致")
}

// 五个 flag 都要经 cobra 真解析绑到变量上——查 Lookup 非 nil 只能证明「注册了」，
// 证明不了「绑对了变量」。
func TestHestiaSheetsPushFlagsBindThroughCobra(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	sheetsCfg(t, creds)
	_, _ = withCapturedPush(t)

	_, err := sheetsExec(t, "hestia", "sheets", "push",
		"--period", "2025-06", "--period-type", "monthly", "--apply", "--create-sheets")
	require.NoError(t, err)
	require.Equal(t, "2025-06", hestiaSheetsPeriod)
	require.Equal(t, "monthly", hestiaSheetsPeriodType)
	require.True(t, hestiaSheetsApply)
	require.True(t, hestiaSheetsCreate)
	require.False(t, hestiaSheetsAll, "没给 --all 就不该被置真")
}

// —— boundary[0]：flag 校验 ——

// TestSheetsPushRequiresPeriodOrAll：必须给其一，且互斥。
func TestSheetsPushRequiresPeriodOrAll(t *testing.T) {
	_, err := sheetsExec(t, "hestia", "sheets", "push")
	require.ErrorContains(t, err, "--period")

	_, err = sheetsExec(t, "hestia", "sheets", "push", "--all", "--period", "2025-06")
	require.ErrorContains(t, err, "互斥")
}

// TestSheetsPushPeriodNeedsType：--period 必须配 --period-type。
//
// 同一个 period 可能有 monthly 与 h1 两条，不给类型就得猜——而这里猜错写进去的是
// 另一份报告的数字，静默且难查。
func TestSheetsPushPeriodNeedsType(t *testing.T) {
	_, err := sheetsExec(t, "hestia", "sheets", "push", "--period", "2025-06")
	require.ErrorContains(t, err, "--period-type")
}

// flag 校验必须早于开库与建客户端：配置指向一个根本不存在的文件，报出来的仍该是
// flag 的错。反过来（先开库）的实现会先报配置错，参数写错的人就得等开库才知道。
func TestSheetsPushValidatesFlagsBeforeOpeningStore(t *testing.T) {
	old := hestiaCfgPath
	hestiaCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
	t.Cleanup(func() { hestiaCfgPath = old })

	_, err := sheetsExec(t, "hestia", "sheets", "push", "--period", "2025-06")
	require.ErrorContains(t, err, "--period-type", "该先报 flag 错，而不是配置读不到")
}

// —— functional[1] / [0]：dry-run 与排版 ——

// TestSheetsPushDefaultsToDryRun：不给 --apply 时输出要明说没有写。
func TestSheetsPushDefaultsToDryRun(t *testing.T) {
	creds, seen := withStubSheetsClient(t)
	sheetsCfg(t, creds)

	out, err := sheetsExec(t, "hestia", "sheets", "push", "--all")
	require.NoError(t, err)
	require.Contains(t, out, "dry-run")
	require.Contains(t, out, "--apply")
	for _, m := range *seen {
		require.True(t, strings.HasPrefix(m, "GET "), "dry-run 一个写请求都不许发，收到 %s", m)
	}
}

// TestFormatResultSeparatesThreeCounts：三类计数分开三行、各带自己的数字。
//
// 合并成一类会让人看不出「这格没动」是因为已经对了，还是因为库里没数——这两种情况
// 的后续动作完全不同（spec §7.2）。
func TestFormatResultSeparatesThreeCounts(t *testing.T) {
	got := formatResult(sheets.Result{WillWrite: 1282, Same: 34, AbsentInDB: 819})
	require.Contains(t, got, "将写 1282 格")
	require.Contains(t, got, "一致跳过 34 格")
	require.Contains(t, got, "库缺跳过 819 格")

	lines := strings.Split(strings.TrimSpace(got), "\n")
	idx := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	a, b, c := idx("将写 1282 格"), idx("一致跳过 34 格"), idx("库缺跳过 819 格")
	require.NotEqual(t, -1, a)
	require.NotEqual(t, -1, b)
	require.NotEqual(t, -1, c)
	require.Equal(t, 3, len(map[int]bool{a: true, b: true, c: true}), "三类计数必须在三**行**上，不能挤在一行")
}

// 明细行要自报类别：将写 / 一致跳过 / 库缺各有自己的说法。
func TestFormatResultLabelsEachChangeKind(t *testing.T) {
	got := formatResult(sheets.Result{
		Changes: []sheets.Change{
			{Sheet: "2025年", Row: 9, Col: 2, Label: "社融存量", Current: nil, Want: 462.06, Kind: sheets.WillWrite},
			{Sheet: "2026年", Row: 9, Col: 1, Label: "发布日期", Current: "2026-07-15", Want: "2026-07-15", Kind: sheets.Same},
			{Sheet: "2020年", Row: 6, Col: 31, Label: "同业拆借月加权利率", Current: 1.5, Kind: sheets.AbsentInDB},
		},
		WillWrite: 1, Same: 1, AbsentInDB: 1,
	})
	require.Contains(t, got, "将写 462.06")
	require.Contains(t, got, "一致，跳过")
	require.Contains(t, got, "库缺，保留表中现值")
	require.Contains(t, got, "6月", "行号要还原成月份（录入区首行是 1 月）")
	require.Contains(t, got, "AF", "列号要还原成表格列字母")
}

// 缺表时提示「将新建工作表」并点名要哪个 flag。
func TestFormatResultAnnouncesMissingTabs(t *testing.T) {
	got := formatResult(sheets.Result{MissingTabs: []string{"2019年", "2020年"}})
	require.Contains(t, got, "将新建工作表")
	require.Contains(t, got, "2019年")
	require.Contains(t, got, "2020年")
	require.Contains(t, got, "--create-sheets")

	require.NotContains(t, formatResult(sheets.Result{}), "将新建工作表", "不缺表就别提这一句")
}

// —— boundary[0]：--period 只推该期 ——

func TestPushSheetsOnlySendsRequestedPeriod(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	gotRows, _ := withCapturedPush(t)
	cfg := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"}}

	require.NoError(t, pushSheets(context.Background(), io.Discard, cfg, sampleSheetRows(),
		"2025-06", sheets.Options{}))
	require.Len(t, *gotRows, 1, "给了 --period 就只推该期")
	require.Equal(t, 2025, (*gotRows)[0].Year)
	require.Equal(t, 6, (*gotRows)[0].Month)
}

func TestPushSheetsWithoutPeriodSendsEverything(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	gotRows, gotOpts := withCapturedPush(t)
	cfg := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"}}

	require.NoError(t, pushSheets(context.Background(), io.Discard, cfg, sampleSheetRows(),
		"", sheets.Options{Apply: true, CreateSheets: true}))
	require.Len(t, *gotRows, 3)
	require.Equal(t, sheets.Options{Apply: true, CreateSheets: true}, *gotOpts, "两个 flag 要原样传到 Push")
}

// —— error_handling[0]：能力禁用 ——

// credentials_file 留空 ⇒ 打印固定一句并**退出 0**：能力禁用不是错误，与 ingest 侧 C9
// 同语义。退非零会让巡检脚本把「没配」报成故障。
func TestSheetsPushPrintsDisabledAndExitsZero(t *testing.T) {
	sheetsCfg(t, "")
	stubNewSheetsClient(t, func(context.Context, string, string, ...sheets.Option) (*sheets.Client, error) {
		t.Fatal("凭据留空时不该建客户端")
		return nil, nil
	})

	out, err := sheetsExec(t, "hestia", "sheets", "push", "--all")
	require.NoError(t, err, "能力禁用不是错误")
	require.Contains(t, out, "hestia_sheets 未配置（credentials_file 留空）")
}

// —— functional[2]：ingest 侧装配 ——

// 凭据留空 ⇒ ProjectSheets 保持 nil（C9）。返回**字面量 nil** 而不是「装了 nil 的函数值」：
// ingest 里 `d.ProjectSheets != nil` 是那条静默降级路径的唯一判据。
func TestSheetsProjectorNilWhenCredentialsEmpty(t *testing.T) {
	require.Nil(t, sheetsProjector(hestia.Config{}))
}

// 凭据非空 ⇒ 闭包真去 Push，且 ingest 自动投影固定用 Apply+CreateSheets
// （新年份第一期必缺表，不建就整批失败）。
func TestSheetsProjectorPushesWithApplyAndCreate(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	gotRows, gotOpts := withCapturedPush(t)
	cfg := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"}}

	project := sheetsProjector(cfg)
	require.NotNil(t, project)
	require.NoError(t, project(context.Background(), sampleSheetRows()))
	require.Len(t, *gotRows, 3, "ingest 侧不过滤，收到什么推什么")
	require.Equal(t, sheets.Options{Apply: true, CreateSheets: true}, *gotOpts)
}

// runHestiaIngest 装出来的 deps 必须真把投影挂上去——只测 sheetsProjector 证明不了
// 「它被接到 IngestDeps 上了」，而漏接那一行的实现在单测里完全为绿。
func TestHestiaIngestDepsCarryProjector(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	cfgWith := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds}}
	require.NotNil(t, hestiaIngestDeps(cfgWith, nil, nil, io.Discard).ProjectSheets)
	require.Nil(t, hestiaIngestDeps(hestia.Config{}, nil, nil, io.Discard).ProjectSheets)
}

// —— error_handling[1]：投影 panic 不得打断整轮 ingest ——

// 闭包最外层的 recover 也兜得住建客户端那一步的 panic：真实第三方 client 的 panic
// 可能发生在任一步，只兜 Push 那一段等于赌它只在那里炸。
func TestSheetsProjectorRecoversClientPanic(t *testing.T) {
	stubNewSheetsClient(t, func(context.Context, string, string, ...sheets.Option) (*sheets.Client, error) {
		panic("boom in client")
	})

	err := sheetsProjector(hestia.Config{
		HestiaSheets: hestia.HestiaSheets{CredentialsFile: "/nonexistent/sa.json"},
	})(context.Background(), nil)
	require.Error(t, err, "panic 要转成 error 返回，不是吞掉")
	require.Contains(t, err.Error(), "boom in client", "原始 panic 值要留在错误里，否则查不出是什么炸了")
}

// TestProjectSheetsPanicDoesNotBreakIngest 是 error_handling[1] 的端到端形态。
//
// 装配方是本任务，用的是真实第三方 client；它的 panic 会穿透 Ingest 打断整轮入库，
// 与 C8「投影失败不阻断入库」的意图冲突。转成 error 之后走的是 010 已有的「只打印
// 不返回」那条路径，所以断言落在 Ingest 的返回值、run 的 outcome 与那行固定文案上。
func TestProjectSheetsPanicDoesNotBreakIngest(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	stubSheetsPush(t, func(context.Context, *sheets.Client, []sheets.Row, []string, sheets.Options) (sheets.Result, error) {
		panic("boom in push")
	})

	cfg := hestia.Config{
		ConfigVersion: "test",
		Storage:       hestia.StorageCfg{SnapshotDir: t.TempDir()},
		Queue:         hestia.QueueCfg{Dir: t.TempDir()},
		Discover:      hestia.DiscoverCfg{IndexURL: sheetsIndexURL, MaxPages: 3},
		Thresholds:    hestia.DefaultThresholds(),
		Signals:       hestia.DefaultSignals(),
		HestiaSheets:  hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"},
	}
	st, err := hestia.NewStore(filepath.Join(t.TempDir(), "hestia.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	var out bytes.Buffer
	deps := hestiaIngestDeps(cfg, st, nil, &out)
	deps.Fetch = newSheetsFakeFetcher(t)
	require.NotNil(t, deps.ProjectSheets)

	require.NoError(t, hestia.Ingest(context.Background(), deps), "投影 panic 不该让 ingest 返回错误")

	runs, err := st.RecentRuns(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, hestia.RunIngested, runs[0].Outcome, "数据照样进了权威表")
	require.Contains(t, out.String(), "sheets: 投影失败（不影响入库）:", "走的是 C8 既有的只打印路径")
}

// —— ingest 端到端用的最小语料 ——

const (
	sheetsIndexURL   = "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html"
	sheetsAnnualID   = "2026011509294440745"
	sheetsAnnualName = "2025年金融统计数据报告"
	sheetsAnnualFile = "pboc-2025-12-annual.html"
)

// sheetsFakeFetcher 按 URL 查表回快照，查不到就是 404——照 internal/hestia 的 fakeFetcher。
type sheetsFakeFetcher struct{ pages map[string][]byte }

func (f *sheetsFakeFetcher) Get(_ context.Context, url string) ([]byte, error) {
	body, ok := f.pages[url]
	if !ok {
		return nil, fmt.Errorf("fake fetcher: 没有 %s", url)
	}
	return body, nil
}

// newSheetsFakeFetcher：index 页只挂 2025 年报这一条，外加它那份能被 Parse 接受的正文快照。
func newSheetsFakeFetcher(t *testing.T) *sheetsFakeFetcher {
	t.Helper()
	body, err := os.ReadFile(sheetsTestdata(t, "testdata", sheetsAnnualFile))
	require.NoError(t, err)
	index := `<html><body>` + "\n" +
		`<a onclick="jumpTo(this,'1','1','/goutongjiaoliu/113456/113469/11040-%1.html')">尾页</a>` + "\n" +
		fmt.Sprintf(`<a href="/goutongjiaoliu/113456/113469/%s/index.html" title="true">%s</a>`+"\n",
			sheetsAnnualID, sheetsAnnualName) +
		`</body></html>`
	return &sheetsFakeFetcher{pages: map[string][]byte{
		sheetsIndexURL: []byte(index),
		"https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/" + sheetsAnnualID + "/index.html": body,
	}}
}

// —— 错误路径：每条 return err 都要有人走过 ——

// flag 的两条格式校验：写错了要当场说清楚要什么，而不是等开库或等 Sheets 报一个
// 看不懂的 400。
func TestSheetsPushRejectsMalformedPeriodFlags(t *testing.T) {
	_, err := sheetsExec(t, "hestia", "sheets", "push", "--period", "2025-6", "--period-type", "monthly")
	require.ErrorContains(t, err, "YYYY-MM")

	_, err = sheetsExec(t, "hestia", "sheets", "push", "--period", "2025-06", "--period-type", "quarterly")
	require.ErrorContains(t, err, "monthly | q1 | h1 | q1_q3 | annual")
}

// flag 合法之后配置才轮到被读——这条与 TestSheetsPushValidatesFlagsBeforeOpeningStore
// 是一对：那条证明「flag 错时不开库」，这条证明「flag 对时真的开库并把错带出来」。
// 少了这一条，一个「永远不开库」的实现也能让那条绿。
func TestSheetsPushPropagatesConfigError(t *testing.T) {
	old := hestiaCfgPath
	hestiaCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
	t.Cleanup(func() { hestiaCfgPath = old })

	_, err := sheetsExec(t, "hestia", "sheets", "push", "--all")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "--period", "flag 都给对了，不该再报 flag 的错")
}

// 建客户端失败要原样带出去：凭据文件读不到是运维最常撞的一种，吞掉它会让
// 「推了但没生效」看起来和「推成功了」一样。
func TestPushSheetsPropagatesClientError(t *testing.T) {
	stubNewSheetsClient(t, func(context.Context, string, string, ...sheets.Option) (*sheets.Client, error) {
		return nil, fmt.Errorf("read sa.json: permission denied")
	})

	err := pushSheets(context.Background(), io.Discard, hestia.Config{}, nil, "", sheets.Options{})
	require.ErrorContains(t, err, "permission denied")
}

// Push 报错时不排版：印一份「将写 0 格」的汇总会让人以为这次跑完了。
func TestPushSheetsPropagatesPushError(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	stubSheetsPush(t, func(context.Context, *sheets.Client, []sheets.Row, []string, sheets.Options) (sheets.Result, error) {
		return sheets.Result{}, fmt.Errorf("hestia sheets: 缺年度表 2019年；加 --create-sheets 允许建表")
	})

	var out bytes.Buffer
	cfg := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"}}
	err := pushSheets(context.Background(), &out, cfg, sampleSheetRows(), "", sheets.Options{})
	require.ErrorContains(t, err, "缺年度表 2019年")
	require.Empty(t, out.String(), "报错就别再印一份看着像跑完了的汇总")
}

// 闭包里建客户端失败 ⇒ 返回 error（走 C8 的只打印路径），而不是 panic 也不是吞掉。
func TestSheetsProjectorPropagatesClientError(t *testing.T) {
	stubNewSheetsClient(t, func(context.Context, string, string, ...sheets.Option) (*sheets.Client, error) {
		return nil, fmt.Errorf("read sa.json: no such file")
	})

	err := sheetsProjector(hestia.Config{
		HestiaSheets: hestia.HestiaSheets{CredentialsFile: "/nonexistent/sa.json"},
	})(context.Background(), nil)
	require.ErrorContains(t, err, "no such file")
}

// --apply 时不打 dry-run 那一句：真写了还说「未写入任何内容」是最坏的一种假话。
func TestPushSheetsApplyDropsDryRunLine(t *testing.T) {
	creds, _ := withStubSheetsClient(t)
	_, _ = withCapturedPush(t)

	var out bytes.Buffer
	cfg := hestia.Config{HestiaSheets: hestia.HestiaSheets{CredentialsFile: creds, SpreadsheetID: "sheet-id"}}
	require.NoError(t, pushSheets(context.Background(), &out, cfg, nil, "", sheets.Options{Apply: true}))
	require.NotContains(t, out.String(), "未写入任何内容")
	require.NotContains(t, out.String(), "dry-run")
}

// 表里已有值但与库不同 ⇒ 现值要打出来，别只说「将写」：人要靠这一列判断
// 「这格本来是什么」才敢确认 --apply。
func TestFormatResultShowsCurrentValueWhenOverwriting(t *testing.T) {
	got := formatResult(sheets.Result{
		Changes: []sheets.Change{
			{Sheet: "2025年", Row: 9, Col: 2, Label: "社融存量", Current: 400.0, Want: 462.06, Kind: sheets.WillWrite},
		},
		WillWrite: 1,
	})
	require.Contains(t, got, "表中 400")
	require.Contains(t, got, "将写 462.06")
	require.NotContains(t, got, "(空)", "表里有值就别显示成空")
}

// 按 period 过滤是**精确**匹配到月：2025-06 不能把 2025-12 或 2026-06 带出来，
// 也不能因为「2025-6 与 2025-06 长得像」就放行。
func TestFilterRowsByPeriodMatchesExactMonth(t *testing.T) {
	require.Len(t, filterRowsByPeriod(sampleSheetRows(), "2025-12"), 1)
	require.Empty(t, filterRowsByPeriod(sampleSheetRows(), "2025-07"), "库里没有这一期就该是空")
	require.Empty(t, filterRowsByPeriod(sampleSheetRows(), "2025-6"), "补零形态不匹配，别猜")
}
