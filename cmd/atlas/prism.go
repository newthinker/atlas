package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

var prismCmd = &cobra.Command{
	Use:   "prism",
	Short: "Prism valuation board (估值面板)",
}

var prismRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Incrementally refresh valuation data (launchd entrypoint)",
	RunE:  runPrismRefresh,
}

func init() {
	prismCmd.AddCommand(prismRefreshCmd)
	rootCmd.AddCommand(prismCmd)
}

// textSender matches the telegram client returned by buildCrisisSender (crisis.Sender).
type textSender interface {
	SendText(msg string) error
}

// hasEdgarInstrument reports whether any instrument uses the EDGAR source,
// which requires a configured User-Agent (SEC rejects requests without a
// contact email).
func hasEdgarInstrument(instruments []config.PrismInstrument) bool {
	for _, inst := range instruments {
		if inst.Source == "edgar" {
			return true
		}
	}
	return false
}

type prismRefreshDeps struct {
	refresh func() prism.Report
	sender  textSender // nil → 未配置 telegram,退化为打印
	out     io.Writer
	errOut  io.Writer
}

// runPrismRefreshWith is the testable core: run refresh, report, notify.
func runPrismRefreshWith(d prismRefreshDeps) error {
	rep := d.refresh()
	fmt.Fprintf(d.out, "prism refresh: %d ok, %d failed, %d degraded\n",
		rep.Refreshed, len(rep.Failed), len(rep.Degraded))
	if len(rep.Failed) == 0 && len(rep.Degraded) == 0 {
		return nil
	}
	var parts []string
	if len(rep.Failed) > 0 {
		parts = append(parts, "⚠️ Prism 刷新部分失败:\n"+strings.Join(rep.Failed, "\n"))
	}
	if len(rep.Degraded) > 0 {
		parts = append(parts, "ℹ️ Prism 主源降级(已兜底):\n"+strings.Join(rep.Degraded, "\n"))
	}
	msg := strings.Join(parts, "\n")
	// 明细无条件进日志:配置了 telegram 时也要留下 stdout 痕迹,否则通知一挂就没有观测。
	fmt.Fprintln(d.out, msg)
	if d.sender == nil {
		return nil
	}
	if err := d.sender.SendText(msg); err != nil {
		fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
	}
	return nil
}

func runPrismRefresh(cmd *cobra.Command, args []string) error {
	// 配置装载与 telegram sender 构造复用同包既有 helper(与 crisis eval 同款,
	// 见 crisis.go / export_ohlcv.go),不重写第二套。
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return err
	}
	pcfg := cfg.Prism
	pcfg.ApplyDefaults()
	if !pcfg.Enabled {
		return fmt.Errorf("prism disabled in config (set prism.enabled: true)")
	}

	store, err := prismstore.Open(pcfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if pcfg.EdgarUserAgent == "" && hasEdgarInstrument(pcfg.Instruments) {
		return fmt.Errorf("prism.edgar_user_agent required for source==edgar instruments (SEC requires a contact email in the User-Agent)")
	}

	lix := lixinger.New(cfg.Collectors["lixinger"].APIKey, lixinger.WithRetry(true))
	yh := yahoo.New()
	ak := akshare.New(pcfg.AkshareBaseURL)
	ed := edgar.New(pcfg.EdgarUserAgent)

	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Refresh(pcfg, store, lix, yh, ak, ed, time.Now())
		},
		sender: buildCrisisSender(),
		out:    cmd.OutOrStdout(),
		errOut: cmd.ErrOrStderr(),
	}
	return runPrismRefreshWith(deps)
}
