package main

import (
	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/hestia"
)

// statusLimit 是 status 各列最多显示的行数。管线一个月一期，10 行覆盖近一年。
const statusLimit = 10

var (
	hestiaCfgPath string
	hestiaForce   bool
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
		"bypass the article_id idempotency key; use after changing thresholds")
	hestiaCmd.AddCommand(hestiaIngestCmd, hestiaStatusCmd)
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
