package main

import (
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/hestia"
)

// statusLimit 是 status 各列最多显示的行数。管线一个月一期，10 行覆盖近一年。
const statusLimit = 10

var (
	hestiaCfgPath string
	hestiaForce   bool
)

// —— backfill fetch 的四个 flag（M1c-1 的 TASK-007）——
//
// ⚠️ 一律带 `hestia` 前缀：`cmd/atlas` 是**一个包**，而 `crisis.go` 已经有一个
// 包级 `hestiaBackfillFrom`（危机模块的回填起点）。不加前缀会直接 redeclared 编译失败 ——
// 这次是编译器挡住的，但同包里语义不同、名字相同的变量本来就该分开。
//
// ⚠️ `hestiaBackfillExpectPeriods` / `hestiaBackfillExpectArticles` 的**零值就是「未显式传入」**，
// 那时 `reconcileBackfill` 走推算值告警路径（TASK-008 的 notes_for_downstream 定死）。
// **别给它们非零默认值** —— 那会让一个未经实测的推算数取得阻断交付的权力。
var (
	hestiaBackfillFrom string
	hestiaBackfillOut  string
	// —— backfill calibrate 的两个 flag（M1c-2 的 TASK-004）——
	hestiaCalibrateDir             string
	hestiaCalibrateAllowIncomplete bool
	// —— backfill load 的三个 flag（M1c-3b 的 TASK-007）——
	//
	// ⚠️ `hestiaLoadDB` **刻意没有默认值**，注册处也不给：默认值会让人在没意识到的
	// 情况下把回填写进 configs/hestia.yaml 指的那个**生产库**，而回填一旦进了权威表
	// 就没有出路 —— `--force` 对已入权威表的期次是数据层 no-op。
	hestiaLoadDir                string
	hestiaLoadDB                 string
	hestiaLoadAllowIncomplete    bool
	hestiaBackfillExpectPeriods  int
	hestiaBackfillExpectArticles int
)

var hestiaCmd = &cobra.Command{
	Use:   "hestia",
	Short: "PBOC financial statistics pipeline",
	Long: `Discover, parse, validate and store PBOC financial statistics reports.

Scheduled by launchd three times a day; non-publication days are idempotent no-ops.
Thresholds live in configs/hestia.yaml — edit and re-run, no rebuild needed.`,

	// 失败时不再灌一屏 usage。见下面 init() 里那段说明。
	SilenceUsage: true,
}

var hestiaIngestCmd = &cobra.Command{
	Use:          "ingest",
	Short:        "Discover and ingest new reports (launchd entrypoint)",
	RunE:         runHestiaIngest,
	SilenceUsage: true,
}

var hestiaStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Print recent observations and pending rows",
	RunE:         runHestiaStatus,
	SilenceUsage: true,
}

// backfill 是中间层，自己不做事，只挂子命令。
//
// 分两层（`backfill` + `fetch`）而不是一个 `backfill` 叶子：回填还会有别的动作
// （如 M1c-2 可能要的 `--recheck` 重抓比对），留出位置比事后拆命令便宜。
var hestiaBackfillCmd = &cobra.Command{
	Use:          "backfill",
	Short:        "One-off historical backfill of PBOC reports",
	SilenceUsage: true,
}

var hestiaBackfillFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch historical reports into an out-of-repo directory",
	Long: `Scan the PBOC index and site search from --from onward, cross-check both
sides, download every report found, and reconcile the result period by period.

--from is a PUBLICATION DATE, not a period (design addendum A1): --from 2020-01
therefore includes the 2019 annual trio, published 2020-01-16.

--out must be an absolute path OUTSIDE this repository: the artefacts survive
worktree removal, cannot be committed by accident, and back up with a plain cp -r.`,
	RunE:         runHestiaBackfillFetch,
	SilenceUsage: true,
}

var hestiaBackfillCalibrateCmd = &cobra.Command{
	Use:   "calibrate",
	Short: "Summarise field distributions from a fetched artefact directory",
	Long: `Parse every supported report under --dir and print the observed distribution
of each field, so a human can fill MagnitudeRanges from evidence rather than guesswork.

The tool only produces evidence: it never edits configs/hestia.yaml.

--allow-incomplete accepts a manifest that has no completed_at. That marker was
introduced later than some artefact directories, so its absence does NOT by itself
mean the fetch was aborted -- but the two cases are indistinguishable from the
artefact alone, so the report says so out loud.`,
	RunE:         runHestiaBackfillCalibrate,
	SilenceUsage: true,
}

var hestiaBackfillLoadCmd = &cobra.Command{
	Use:   "load",
	Short: "Parse, merge and load a fetched artefact directory into a NEW database",
	Long: `Parse every supported report under --dir, merge the articles that share a
business key, run every gate, and write the result into the database at --db.

--db must NOT already exist. Backfill is a one-off: appending would make the
report's identities meaningless, and re-running is meant to be "delete the file
and start over" -- which is also the only way around the fact that --force is a
data-level no-op for periods already in the authoritative table.

The tool never touches the production database and never edits configs/hestia.yaml:
switching over is a human action, taken after reading the report.`,
	RunE:         runHestiaBackfillLoad,
	SilenceUsage: true,
}

// runHestiaBackfillLoad 装配 BackfillLoad 的依赖。
//
// 🔴 **Out 必须显式填**：BackfillLoad 对 nil 直接报错，不退化成 io.Discard
// （M1c-3b 的 TASK-006 刻意背离需求文档，沿用同包 Calibrate 的相反契约）。
// 理由正是本层最容易犯的错 —— 漏填 Out 会让命令把观测正确写进库、退出码 0、
// 而 stdout 一片空白，且「子命令注册了吗」「flag 解析对吗」那类测试**全部通过**。
// TestHestiaBackfillLoadWritesReport 钉住这一点。
//
// ⚠️ **--db 的存在性检查不在这一层**：它在 BackfillLoad 里，且必须先于 NewStore
// （NewStore 会把文件建出来）。这一层若自己 os.Stat 就要 import path/filepath，
// 而 TestHestiaCmdDoesNotResolveDBPath 明令 hestia.go 不得 import 它 —— 路径语义归
// internal/hestia。
//
// ⚠️ 只读配置里的 Thresholds，**不碰 cfg.Storage.DBPath**：回填写的是 --db 指的那个
// 新库，与生产库无关。Long 里那句「never touches the production database」就是这个意思。
func runHestiaBackfillLoad(cmd *cobra.Command, _ []string) error {
	cfg, err := hestia.LoadConfig(hestiaCfgPath)
	if err != nil {
		return err
	}
	// 失败时 res 也非 nil（调用方要拿它看差在哪），故判成功看 err 而不是 res
	// —— M1c-3b 的 TASK-006 的 interfaces_exposed 明确了这条。
	_, err = hestia.BackfillLoad(cmd.Context(), hestia.BackfillLoadDeps{
		Dir:             hestiaLoadDir,
		DBPath:          hestiaLoadDB,
		Cfg:             cfg.Thresholds,
		Out:             cmd.OutOrStdout(),
		AllowIncomplete: hestiaLoadAllowIncomplete,
	})
	return err
}

// —— 为什么三个命令都显式设 SilenceUsage（reviewer D4）——
//
// 改动前实测：`atlas crisis status --crisis-config /nonexistent/nope.yaml` 输出 **13 行**，
// 其中只有第 1 行是真正的错误，其余 12 行是 usage —— 而全仓当时**没有任何命令**设过它
// （main.go 的 rootCmd 也没设）。
//
// 本管线对此格外敏感：设计意图是让**退出码 + hestia-ingest.err.log 成为唯一报警通道**，
// 而它**预期会有连续两个月每天三次的稳定失败态**（TASK-001 的 D6）。按每天 3 次算，
// 两个月就是 ~180 次失败 × 12 行样板 —— 真正那行错误会被埋进两千多行 usage 里。
//
// ⚠️ **只在 hestia 这一层设，不动 rootCmd**：动 rootCmd 会波及 crisis 与其余全部子命令
// 的现有测试。
//
// 🔴 **两个叶子命令各自都必须设，这不是防御性冗余 —— 是必需**（实测，非推断）：
// cobra 判的是「**被执行的那个命令** 或 **根命令**」，**不查中间祖先**。实测把
// hestiaStatusCmd 的那行去掉、只留 hestiaCmd 上的，跑 `hestia status --hestia-config
// /nonexistent/nope.yaml` 仍打出完整 13 行 usage。
//
// ⇒ 只写父命令**没有任何作用**。（我最初在这段注释里写的是「沿命令链查找，写三个是
// 为了不依赖继承」—— **那句话是错的**，是跑了上面那次对照才发现。结论没变、理由整个
// 换了，正是本 Sprint 反复记的那一族。）
//
// hestiaCmd 那行则是**真正的防御性冗余**：它自己没有 RunE，走不到出错路径；留着是为了
// 将来有人给它加 RunE 时不必重新发现这件事。
//
// **不设 SilenceErrors**：错误本身仍要打给用户（err.log 就靠它），被消掉的只有 usage。
func init() {
	hestiaCmd.PersistentFlags().StringVar(&hestiaCfgPath, "hestia-config",
		"configs/hestia.yaml", "hestia config path")
	hestiaIngestCmd.Flags().BoolVar(&hestiaForce, "force", false,
		"bypass both idempotency layers (Discover stop-key and ingest's article_id); "+
			"NOTE: re-extracted values are DISCARDED for periods already in the observations "+
			"table -- same published_at means Duplicate, which only refreshes article_id")
	bf := hestiaBackfillFetchCmd.Flags()
	bf.StringVar(&hestiaBackfillFrom, "from", "",
		"earliest PUBLICATION month to fetch, YYYY-MM (not a period; see addendum A1)")
	bf.StringVar(&hestiaBackfillOut, "out", "",
		"artefact directory; MUST be an absolute path outside this repository")
	bf.IntVar(&hestiaBackfillExpectPeriods, "expect-periods", 0,
		"expected period count; omit to compare against the estimate as a WARNING only")
	bf.IntVar(&hestiaBackfillExpectArticles, "expect-articles", 0,
		"expected article count; omit to compare against the estimate as a WARNING only")
	// --out 无默认值且必填：让「误落进仓库」需要显式打出来才会发生。
	if err := hestiaBackfillFetchCmd.MarkFlagRequired("out"); err != nil {
		panic(err) // 只会在 flag 名写错时触发，属编程错误
	}
	cal := hestiaBackfillCalibrateCmd.Flags()
	cal.StringVar(&hestiaCalibrateDir, "dir", "",
		"artefact directory produced by `backfill fetch` (the --out of that run)")
	cal.BoolVar(&hestiaCalibrateAllowIncomplete, "allow-incomplete", false,
		"accept a manifest without completed_at; the report will say why it was accepted")
	// --dir 必填，与 fetch --out 对称：没有默认值可言，猜一个只会让人标定错目录。
	if err := hestiaBackfillCalibrateCmd.MarkFlagRequired("dir"); err != nil {
		panic(err) // 只会在 flag 名写错时触发，属编程错误
	}

	load := hestiaBackfillLoadCmd.Flags()
	load.StringVar(&hestiaLoadDir, "dir", "",
		"artefact directory produced by `backfill fetch` (the --out of that run)")
	load.StringVar(&hestiaLoadDB, "db", "",
		"path to the NEW database to create; must not already exist")
	load.BoolVar(&hestiaLoadAllowIncomplete, "allow-incomplete", false,
		"accept a manifest without completed_at; the report will say why it was accepted")
	// --dir 与 --db 都必填。--db **不给默认值**：见 hestiaLoadDB 的声明处。
	for _, name := range []string{"dir", "db"} {
		if err := hestiaBackfillLoadCmd.MarkFlagRequired(name); err != nil {
			panic(err) // 只会在 flag 名写错时触发，属编程错误
		}
	}

	hestiaBackfillCmd.AddCommand(hestiaBackfillFetchCmd, hestiaBackfillCalibrateCmd,
		hestiaBackfillLoadCmd)

	hestiaCmd.AddCommand(hestiaIngestCmd, hestiaStatusCmd, hestiaBackfillCmd)
	rootCmd.AddCommand(hestiaCmd)
}

// openHestia 装载配置并打开库。两个子命令共用。
//
// 调用方负责 Close —— 与 openCrisisStore 同形。
//
// ⚠️ `cfg.Storage.DBPath` **原样**交给下游，这一层不解析成绝对路径：解析归
// hestia.RenderStatus（本 Sprint 的 reviewer D7 裁决）。两处都解析行为无害，但读到
// RenderStatus 那句「解析发生在这里」的人会来找第二处并删掉它，而删错一处就静默改变了
// cwd 语义。TestHestiaCmdDoesNotResolveDBPath 用「不 import path/filepath」钉住这一点。
func openHestia() (hestia.Config, *hestia.Store, error) {
	cfg, err := hestia.LoadConfig(hestiaCfgPath)
	if err != nil {
		return hestia.Config{}, nil, err
	}
	st, err := hestia.NewStore(cfg.Storage.DBPath)
	if err != nil {
		return hestia.Config{}, nil, err
	}
	return cfg, st, nil
}

func runHestiaIngest(cmd *cobra.Command, _ []string) error {
	cfg, st, err := openHestia()
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	return hestia.Ingest(cmd.Context(), hestia.IngestDeps{
		Store: st,
		Fetch: hestia.NewPBOCFetcher(cfg.Discover.Timeout),
		Cfg:   cfg,
		Out:   cmd.OutOrStdout(),
		Force: hestiaForce,
	})
}

func runHestiaStatus(cmd *cobra.Command, _ []string) error {
	cfg, st, err := openHestia()
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ctx := cmd.Context()
	obs, err := st.RecentObservations(ctx, statusLimit)
	if err != nil {
		return err
	}
	pending, err := st.RecentPending(ctx, statusLimit)
	if err != nil {
		return err
	}
	return hestia.RenderStatus(cmd.OutOrStdout(), cfg.Storage.DBPath, obs, pending)
}

// hestiaBackfillFromRE 校验 `--from` 的形态。
//
// ⚠️ 月份两位且必须落在 01–12：宽松的 `\d{4}-\d{2}` **认得 `2020-13` 与 `2020-00`**，
// 放过去会变成一个语义非法的日期，而回填按**发布日期**判停 —— 错一个月就少抓或多抓一整批。
var hestiaBackfillFromRE = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// parseHestiaBackfillFrom 把 `YYYY-MM` 解析成该月**第一天**。
//
// 取当月 1 号而不是最后一天：`--from` 的语义是「这个月**起**的发布日期都要」，
// 取月末会把该月前 30 天静默漏掉。
func parseHestiaBackfillFrom(s string) (time.Time, error) {
	if !hestiaBackfillFromRE.MatchString(s) {
		return time.Time{}, fmt.Errorf("--from %q 格式非法：要 YYYY-MM，月份取 01–12", s)
	}
	return time.Parse("2006-01", s)
}

// runHestiaBackfillFetch 跑一次历史回填。
//
// 顺序刻意是「先校验 --from、再读配置、最后才触网」：前两步都是本地判断，
// 让参数写错的人在**第一秒**就知道，而不是等约 9 分钟的抓取跑到一半。
func runHestiaBackfillFetch(cmd *cobra.Command, _ []string) error {
	from, err := parseHestiaBackfillFrom(hestiaBackfillFrom)
	if err != nil {
		return err
	}
	cfg, err := hestia.LoadConfig(hestiaCfgPath)
	if err != nil {
		return err
	}
	return hestia.BackfillFetch(cmd.Context(), hestiaBackfillConfig(cfg, cmd.OutOrStdout(), from))
}

// runHestiaBackfillCalibrate 装配并跑一次标定。
//
// 🔴 **Out 必须显式传 cmd.OutOrStdout()**。漏掉它是一个既真实又完全隐形的错误：
// 若 Calibrate 把 nil 当成 io.Discard，这个命令就会**静默打印零字节、退出码 0**，
// 而「子命令注册了吗」「flag 解析对吗」这类测试全部通过。
// Calibrate 因此对 nil Out **报错**（见它的注释），本行是它的另一半。
//
// 不读 hestia 配置：标定只读产物目录，不碰库、不碰阈值。
func runHestiaBackfillCalibrate(cmd *cobra.Command, _ []string) error {
	return hestia.Calibrate(hestia.CalibrateDeps{
		Dir:             hestiaCalibrateDir,
		Out:             cmd.OutOrStdout(),
		AllowIncomplete: hestiaCalibrateAllowIncomplete,
	})
}

// hestiaBackfillConfig 把 hestia 配置 + flag 拼成一次回填的参数。
//
// 抽成独立函数**只为了能被测试直接检查** —— 组装本身很短，但它里面有三个
// 「不这么写就会静默出错」的决定（见下），而 `runHestiaBackfillFetch` 一路走到触网，
// 没法在单测里断言这三个决定。
func hestiaBackfillConfig(cfg hestia.Config, report io.Writer, from time.Time) hestia.BackfillConfig {
	return hestia.BackfillConfig{
		IndexURL: cfg.Discover.IndexURL,
		From:     from,
		Out:      hestiaBackfillOut,
		// 🔴 **MaxPages 刻意留零值**，走 hestia 包的 backfillMaxPages(200)，
		// **不传 `cfg.Discover.MaxPages`**。
		//
		// `discover.max_pages` 是**日常增量**那条链路的上界（configs 里是 **3** ——
		// 每页 15 条约覆盖 20 天，3 页 ≈ 60 天窗口足以接住新发的报告）；
		// 而回填要从 2020-01 翻到今天，约 **150 页**。
		//
		// 复用它的后果我实测过：真跑第一次就被 TASK-002 那条守卫当场拦下
		// （`reached max_pages=3 without any page falling before from=2020-01-01`）。
		// ⚠️ 那条守卫**报错而不是静默返回 3 页** —— 若当初写成「翻满就返回已收集的部分」，
		// 这里会静默只抓最近三页的报告，而回填看起来跑完了。
		Timeout: cfg.Discover.Timeout,
		// 对账报告走 cobra 的输出通道：命令的输出该由命令层决定去向。
		Report: report,
		// Cutover 留空 ⇒ 用 hestia 包里的 backfillCutover。
		// **别在命令层再写一个 "2025-09"**：那会造出第二个定义处，两处迟早不一致。
		ExpectPeriods:  hestiaBackfillExpectPeriods,
		ExpectArticles: hestiaBackfillExpectArticles,
	}
}
