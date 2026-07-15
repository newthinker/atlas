package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/crisis"
)

var (
	reportFrom string
	reportTo   string
	reportForm string
	reportSend bool
)

// reportSendLimit --send 模式的报告条数硬上限（设计 §1 量控）。
const reportSendLimit = 31

var crisisReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Replay history into daily/monthly reports, a summary and an HTML file",
	Long: `Replays the evaluation pipeline (warm-started from the earliest observation,
zero writes) and renders per-day or per-month reports plus a summary. Always
writes a self-contained HTML report under reports/. With --send, delivers the
texts and the HTML document to telegram (hard cap 31 reports per run).`,
	RunE: runCrisisReport,
}

func init() {
	crisisReportCmd.Flags().StringVar(&reportFrom, "from", "", "start date YYYY-MM-DD (required)")
	crisisReportCmd.Flags().StringVar(&reportTo, "to", "", "end date YYYY-MM-DD (required)")
	crisisReportCmd.Flags().StringVar(&reportForm, "form", "", "daily | monthly (required)")
	crisisReportCmd.Flags().BoolVar(&reportSend, "send", false, "send reports, summary and HTML to telegram")
	crisisCmd.AddCommand(crisisReportCmd)
}

// crisisReportDeps 注入依赖使 report 流程可单测（模式同 crisisEvalDeps）。
type crisisReportDeps struct {
	cfg     *crisis.Config
	store   *crisis.Store
	out     io.Writer
	errOut  io.Writer
	sender  crisis.Sender
	sleep   func(time.Duration)
	htmlDir string
}

func runCrisisReport(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	deps := crisisReportDeps{
		cfg: ccfg, store: st,
		out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		sender: buildCrisisSender(), sleep: time.Sleep, htmlDir: "reports",
	}
	return executeCrisisReport(cmd.Context(), deps, reportFrom, reportTo, reportForm, reportSend)
}

// documentSender 是 --send 附件路径的能力断言目标（Sender 接口不动，
// crisis 包不感知；断言失败降级为总结尾附文件路径提示）。
type documentSender interface {
	SendDocument(path, caption string) error
}

func executeCrisisReport(ctx context.Context, d crisisReportDeps, from, to, form string, send bool) error {
	if from == "" || to == "" {
		return fmt.Errorf("--from and --to are required")
	}
	for _, s := range []string{from, to} {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return fmt.Errorf("bad date %q: want YYYY-MM-DD", s)
		}
	}
	if from > to {
		return fmt.Errorf("--from %s is after --to %s", from, to)
	}
	if form != "daily" && form != "monthly" {
		return fmt.Errorf("--form must be daily or monthly, got %q", form)
	}
	if send && d.sender == nil {
		return fmt.Errorf("--send 需要主配置（-c）notifiers.telegram 凭据")
	}

	// 量控与参数下界都在启动（逐日评估）前用日历完成（设计 §1：启动前直接报错退出）。
	cal, err := d.store.EvalDates(ctx, "", to)
	if err != nil {
		return err
	}
	if len(cal) == 0 {
		return fmt.Errorf("no observations up to %s — run backfill first", to)
	}
	if from < cal[0] {
		return fmt.Errorf("--from %s 早于库内最早观测日，实际可用起点：%s", from, cal[0])
	}
	isReport := reportDates(cal, from, to, form)
	nReports := 0
	for _, ok := range isReport {
		if ok {
			nReports++
		}
	}
	if send && nReports > reportSendLimit {
		return fmt.Errorf("%s 回放共 %d 条报告，超过 --send 上限 %d。请缩短周期，或用 --form monthly，或去掉 --send 输出到 stdout。",
			form, nReports, reportSendLimit)
	}

	days, err := crisis.ReplayRange(d.cfg, d.store.Reader(ctx), from, to)
	if err != nil {
		return err
	}
	if len(days) == 0 {
		return fmt.Errorf("no observations between %s and %s — run backfill first", from, to)
	}

	sr := d.store.Reader(ctx)
	var texts []string
	for i, day := range days {
		if !isReport[day.Date] {
			continue
		}
		var prev *crisis.ReplayDay
		if i > 0 {
			prev = &days[i-1]
		}
		body, err := crisis.ReplayReport(d.cfg, form, day, prev, sr)
		if err != nil {
			return err
		}
		texts = append(texts, replayPrefix(form, day.Date)+"\n"+body)
	}
	summary := crisis.RenderReplaySummary(d.cfg, days)

	for _, txt := range texts {
		fmt.Fprintln(d.out, txt)
		fmt.Fprintln(d.out)
	}
	fmt.Fprintln(d.out, summary)

	html, err := crisis.RenderReplayHTML(d.cfg, days, sr)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.htmlDir, 0o755); err != nil {
		return err
	}
	htmlPath := filepath.Join(d.htmlDir, fmt.Sprintf("crisis-replay-%s-%s.html", from, to))
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(d.out, "HTML 报告已写入 %s\n", htmlPath)

	if !send {
		return nil
	}
	sendText := func(txt string) {
		// 单条失败记 stderr 继续（沿评估链 executeCrisisEvalDaily 的错误语义）
		if err := d.sender.SendText(txt); err != nil {
			fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
		}
		d.sleep(3 * time.Second)
	}
	for _, txt := range texts {
		sendText(txt)
	}
	ds, ok := d.sender.(documentSender)
	if !ok {
		sendText(summary + "\n详细报告（本机）：" + htmlPath)
		return nil
	}
	sendText(summary)
	if err := ds.SendDocument(htmlPath, firstLine(summary)); err != nil {
		fmt.Fprintf(d.errOut, "warning: send document failed: %v\n", err)
	}
	return nil
}

// reportDates 报告日集合：daily = 窗口内全部交易日；monthly = 全库日历中
// 每月首交易日且落在窗口内者（月首判定不受窗口截断影响）。
func reportDates(cal []string, from, to, form string) map[string]bool {
	out := map[string]bool{}
	prevMonth := ""
	for _, d := range cal {
		monthFirst := d[:7] != prevMonth
		prevMonth = d[:7]
		if d < from || d > to {
			continue
		}
		if form == "daily" || monthFirst {
			out[d] = true
		}
	}
	return out
}

// replayPrefix 回放标记前缀行（cmd 层拼接，渲染器不感知回放）。
func replayPrefix(form, date string) string {
	label := date
	if form == "monthly" && len(date) >= 7 {
		label = date[:7]
	}
	return fmt.Sprintf("【历史回放 %s · 非实时告警】", label)
}

// firstLine 取总结首行作 sendDocument caption。
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
