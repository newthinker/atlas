package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism"
	"github.com/newthinker/atlas/internal/prism/sankey"
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

// prismFullSegments 忽略分部增量锚点、全量重拉(AD-12)。模板迭代的工作方式是
// 「跑一次看 member 清单 → 改模板 → 再跑」,纯增量下第二次拉不到任何数据,该流程
// 走不通,故必须留这个入口。
var prismFullSegments bool

// 桑基模板与人工兜底分部数据的目录(相对仓库根,与 launchd 工作目录一致)。
const (
	sankeyTemplatesDir = "configs/prism/templates"
	sankeySegmentsDir  = "configs/prism/segments"
)

func init() {
	prismRefreshCmd.Flags().BoolVar(&prismFullSegments, "full-segments", false,
		"忽略分部增量锚点,全量重拉分部数据(改模板后需要)")
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

// mergeReports 合并估值刷新与分部刷新的结果,使两者共用同一条打印与通知路径。
//
// Refreshed **不相加**:两次刷新跑的是同一批标的,相加会让汇总行的 "N ok" 超过标的
// 总数,误导 launchd 日志与告警。汇总行 ok 的口径固定为「估值刷新成功的标的数」,
// 分部刷新的结果通过带 segmentPrefix 的 Failed/Degraded 条目体现(design-spec §4.2
// 要求两者分开表述)。
func mergeReports(val, seg prism.Report) prism.Report {
	// slices.Concat 而非 append:总是分配新底层数组,不会就地改写入参的切片。
	return prism.Report{
		Refreshed: val.Refreshed,
		Failed:    slices.Concat(val.Failed, prefixAll(seg.Failed)),
		Degraded:  slices.Concat(val.Degraded, prefixAll(seg.Degraded)),
	}
}

// segmentPrefix 标出条目来自分部刷新 —— 两条链路的条目都以 "SYMBOL: " 开头,
// 同一标的两处都失败时无前缀就分不清是哪一条挂了。
const segmentPrefix = "segments: "

func prefixAll(msgs []string) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = segmentPrefix + m
	}
	return out
}

// segmentReport 加载桑基模板并跑分部刷新;无模板时返回零值 Report(不是错误——
// 没配模板是合法状态)。
//
// AD-16:模板加载失败**不得丢弃 error**。模板目录被 rsync --delete 清掉或 YAML 写坏时,
// 丢弃 error 的表现是 sankey 全站 404 且日志无痕;此处降级为 Degraded 条目,使其同时
// 出现在 stdout 与告警里,且此轮不发起任何分部拉取。
func segmentReport(pcfg config.PrismConfig, store prism.Store, seg prism.SegmentClient,
	ak prism.AkshareFinancialsClient, templatesDir, manualDir string, force bool) prism.Report {
	templates, err := sankey.LoadTemplates(templatesDir)
	if err != nil {
		msg := fmt.Sprintf("sankey templates: %v (分部刷新已跳过)", err)
		return prism.Report{Degraded: []string{msg}}
	}
	if len(templates) == 0 {
		return prism.Report{}
	}
	return prism.RefreshSegments(pcfg, store, seg, ak, templates, manualDir, force)
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
			valuation := prism.Refresh(pcfg, store, lix, yh, ak, ed, time.Now())
			segments := segmentReport(pcfg, store, ed, ak, sankeyTemplatesDir, sankeySegmentsDir, prismFullSegments)
			return mergeReports(valuation, segments)
		},
		sender: buildCrisisSender(),
		out:    cmd.OutOrStdout(),
		errOut: cmd.ErrOrStderr(),
	}
	return runPrismRefreshWith(deps)
}
