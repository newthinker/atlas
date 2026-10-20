package main

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/crypto/binance"
	"github.com/newthinker/atlas/internal/collector/crypto/okx"
	"github.com/newthinker/atlas/internal/llm"
	"github.com/newthinker/atlas/internal/llm/factory"
	"github.com/newthinker/atlas/internal/notifier/telegram"
	"github.com/newthinker/atlas/internal/pricewatch"
)

var (
	pricewatchCfgPath string
	pricewatchDryRun  bool
	pricewatchSymbol  string
)

// pricewatchTick 是 run 检查各资产是否到期的间隔；资产的 check_interval 最小按分钟计。
const pricewatchTick = 30 * time.Second

// 以下三个是测试替身的注入点。
var (
	// pricewatchProviderCtors 按 sources 里的名字构造数据源；目前只支持 okx 与 binance。
	pricewatchProviderCtors = map[string]func() crypto.Provider{
		"okx":     func() crypto.Provider { return okx.New() },
		"binance": func() crypto.Provider { return binance.New() },
	}
	pricewatchNewSender = func(c *pricewatch.Config) pricewatch.Sender {
		return telegram.New(c.Telegram.BotToken, c.Telegram.ChatID, telegram.WithProxy(c.Telegram.Proxy))
	}
	// pricewatchNewLLM 沿用主配置（--config）的 llm 段。
	pricewatchNewLLM = func() (llm.Provider, error) {
		cfg, err := loadConfigOrDefaults()
		if err != nil {
			return nil, err
		}
		return factory.New(cfg.LLM)
	}
)

// pricewatchHistoryRank 是 K 线回填的优先级：历史越长越靠前（AD-20）。
// binance 日线可回溯到 2017 年；okx /market/candles 只有约 1440 根。
var pricewatchHistoryRank = map[string]int{"binance": 0, "okx": 1}

var pricewatchCmd = &cobra.Command{
	Use:   "pricewatch",
	Short: "Crypto price-swing alerts for dual-investment orders",
	Long: `Watches crypto prices against drop-from-high / rise-from-low rules and
sends Telegram alerts with optional AI advice (see
docs/superpowers/specs/2026-10-20-pricewatch-design.md).`,
}

var pricewatchRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run continuously (launchd entrypoint); stops on SIGTERM/SIGINT",
	RunE:  runPricewatchRun,
}

var pricewatchOnceCmd = &cobra.Command{
	Use:   "once",
	Short: "Check every asset once",
	RunE:  runPricewatchOnce,
}

var pricewatchBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Backfill 1d and 5m candles",
	RunE:  runPricewatchBackfill,
}

func init() {
	pricewatchCmd.PersistentFlags().StringVar(&pricewatchCfgPath, "pricewatch-config",
		"configs/pricewatch.yaml", "pricewatch config file")
	pricewatchOnceCmd.Flags().BoolVar(&pricewatchDryRun, "dry-run", false,
		"print messages instead of sending them; rule state is not saved")
	pricewatchBackfillCmd.Flags().StringVar(&pricewatchSymbol, "symbol", "", "only this asset (default: all)")
	pricewatchCmd.AddCommand(pricewatchRunCmd, pricewatchOnceCmd, pricewatchBackfillCmd)
	rootCmd.AddCommand(pricewatchCmd)
}

// pricewatchProviders 按 sources 构造数据源，返回同一组实例的两种顺序：
// 取价按 sources 原序（okx 在前，用户在 okx 下单），K 线回填按历史长度（AD-20）。
// 构造之前先装好 collector 策略闸门：provider 在构造时快照 policy.Default()。
func pricewatchProviders(sources []string) (price, backfill []crypto.Provider, err error) {
	ensurePolicyGate()
	for _, name := range sources {
		ctor, ok := pricewatchProviderCtors[name]
		if !ok {
			return nil, nil, fmt.Errorf("pricewatch: unsupported source %q (supported: okx, binance)", name)
		}
		price = append(price, ctor())
	}
	backfill = append([]crypto.Provider(nil), price...)
	sort.SliceStable(backfill, func(i, j int) bool {
		return pricewatchHistoryRank[backfill[i].Name()] < pricewatchHistoryRank[backfill[j].Name()]
	})
	return price, backfill, nil
}

// pricewatchAdvisor：ai.enabled 为 false 返回 nil；LLM 构造失败时记 warn 并返回 nil（不带 AI 运行）。
func pricewatchAdvisor(cfg *pricewatch.Config, log *zap.Logger) *pricewatch.Advisor {
	if !cfg.AI.Enabled {
		return nil
	}
	p, err := pricewatchNewLLM()
	if err != nil {
		log.Warn("pricewatch: AI disabled, LLM unavailable", zap.Error(err))
		return nil
	}
	return &pricewatch.Advisor{LLM: p, Timeout: cfg.AI.Timeout}
}

type pricewatchApp struct {
	cfg    *pricewatch.Config
	store  *pricewatch.Store
	syncer *pricewatch.Syncer
	engine *pricewatch.Engine
}

// newPricewatchApp 按 spec §9 装配：读配置 → 构造数据源 → 打开库 → AI → Engine。
// 配置错误在构造任何 sender 之前返回，因此不会发出任何消息。
func newPricewatchApp(out io.Writer, dryRun bool, log *zap.Logger) (*pricewatchApp, error) {
	cfg, err := pricewatch.LoadConfig(pricewatchCfgPath)
	if err != nil {
		return nil, err
	}
	for _, item := range cfg.UnsupportedItems() {
		log.Warn("pricewatch: asset type not supported in this version, skipped", zap.String("item", item))
	}
	price, backfill, err := pricewatchProviders(cfg.Assets.Crypto.Sources)
	if err != nil {
		return nil, err
	}
	store, err := pricewatch.NewStore(cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	syncer := &pricewatch.Syncer{Providers: backfill, Store: store, Now: time.Now}
	eng := &pricewatch.Engine{
		Cfg: cfg, Store: store, Syncer: syncer, Providers: price,
		Advisor: pricewatchAdvisor(cfg, log),
		Sender:  pricewatchNewSender(cfg),
		Now:     time.Now, Log: log, DryRun: dryRun, Out: out,
	}
	return &pricewatchApp{cfg: cfg, store: store, syncer: syncer, engine: eng}, nil
}

func (a *pricewatchApp) runner() *pricewatch.Runner {
	return &pricewatch.Runner{Engine: a.engine, Assets: a.cfg.CryptoAssets(),
		HeartbeatAt: a.cfg.Health.HeartbeatAt, Tick: pricewatchTick}
}

func newPricewatchLogger() *zap.Logger {
	if log, err := zap.NewProduction(); err == nil {
		return log
	}
	return zap.NewNop()
}

func runPricewatchRun(cmd *cobra.Command, _ []string) error {
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	// SIGTERM（launchd 停止）/ SIGINT 取消 ctx；Runner 等正在执行的检测返回后以 nil 退出。
	ctx, stop := signal.NotifyContext(parent, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log := newPricewatchLogger()
	defer log.Sync() //nolint:errcheck
	app, err := newPricewatchApp(cmd.OutOrStdout(), false, log)
	if err != nil {
		return err
	}
	defer app.store.Close()
	return app.runner().Run(ctx)
}

// runPricewatchOnce 每个资产检测一次；任一资产失败（如所有源取价失败）都以 error 返回，退出码非 0。
func runPricewatchOnce(cmd *cobra.Command, _ []string) error {
	log := newPricewatchLogger()
	defer log.Sync() //nolint:errcheck
	out := cmd.OutOrStdout()
	app, err := newPricewatchApp(out, pricewatchDryRun, log)
	if err != nil {
		return err
	}
	defer app.store.Close()
	runErr := app.runner().RunOnce(context.Background())
	if pricewatchDryRun {
		for _, r := range app.engine.Snapshot() {
			active := "无"
			if len(r.Active) > 0 {
				active = strings.Join(r.Active, "、")
			}
			fmt.Fprintf(out, "%s %s %s · 触发中：%s\n", r.Asset.Symbol,
				strconv.FormatFloat(r.Price, 'f', -1, 64), r.Asset.Quote, active)
		}
	}
	return runErr
}

// runPricewatchBackfill 对指定（不区分大小写）或全部资产补 1d 与 5m K 线，并打印库中根数。
func runPricewatchBackfill(cmd *cobra.Command, _ []string) error {
	log := newPricewatchLogger()
	defer log.Sync() //nolint:errcheck
	app, err := newPricewatchApp(cmd.OutOrStdout(), false, log)
	if err != nil {
		return err
	}
	defer app.store.Close()

	assets := app.cfg.CryptoAssets()
	if pricewatchSymbol != "" {
		want := strings.ToUpper(pricewatchSymbol)
		var picked []pricewatch.Asset
		for _, a := range assets {
			if a.Symbol == want {
				picked = append(picked, a)
			}
		}
		if len(picked) == 0 {
			return fmt.Errorf("pricewatch backfill: asset %q not in config", pricewatchSymbol)
		}
		assets = picked
	}

	ctx := context.Background()
	for _, a := range assets {
		for _, bar := range []string{"1d", "5m"} {
			if err := app.syncer.Sync(ctx, a.Pair, bar); err != nil {
				return fmt.Errorf("pricewatch backfill %s %s: %w", a.Symbol, bar, err)
			}
			st, err := app.store.Stats(a.Pair, bar, time.Time{}, time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %d 根\n", a.Symbol, bar, st.Count)
		}
	}
	return nil
}
