package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/bank"
	"github.com/newthinker/atlas/internal/notifier/telegram"
)

// 编译期断言：主配置里的 telegram notifier 可直接作为银行月报的推送通道。
var _ bank.Sender = (*telegram.Telegram)(nil)

var (
	bankCfgPath string
	bankDryRun  bool
	// 可注入（AD-7）：测试替换后经 runBankReport 端到端验证 sender 构造与退出码。
	bankSenderFactory = buildBankSender
	bankExit          = os.Exit
)

var bankCmd = &cobra.Command{
	Use:   "bank",
	Short: "银行股关键指标监控（不良率 / 拨备覆盖率 / CET1）",
	Long: `按 configs/bank-monitor.yaml 的银行列表拉取东方财富（经 aktools）三项监管指标，
生成预警 + 同期统计 + 排名报告并推送 Telegram
（设计: docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md）。`,
}

var bankReportCmd = &cobra.Command{
	Use:          "report",
	Short:        "拉取最新指标、生成报告并推送（launchd 每月入口）",
	SilenceUsage: true,
	RunE:         runBankReport,
}

func init() {
	bankCmd.PersistentFlags().StringVar(&bankCfgPath, "bank-config",
		"configs/bank-monitor.yaml", "bank monitor config path")
	bankReportCmd.Flags().BoolVar(&bankDryRun, "dry-run", false, "只打印报告到 stdout，不推送")
	bankCmd.AddCommand(bankReportCmd)
	rootCmd.AddCommand(bankCmd)
}

type bankReportDeps struct {
	source bank.Source
	sender bank.Sender // nil ⇒ 打印到 out
	now    func() time.Time
	out    io.Writer
}

// runBankReport 的退出码：0 全部成功；2 部分主体拉取失败；1（返回 error）配置非法、全部失败、
// 推送失败，或非 dry-run 却拿不到 sender——后者报告照常打印，但未推送的月报不能记成成功。
func runBankReport(cmd *cobra.Command, _ []string) error {
	cfg, err := bank.LoadConfig(bankCfgPath)
	if err != nil {
		return err
	}
	d := bankReportDeps{
		source: bank.NewEMSource(cfg.Source.AktoolsURL),
		now:    time.Now,
		out:    cmd.OutOrStdout(),
	}
	var noSender error
	if !bankDryRun {
		if d.sender, noSender = bankSenderFactory(); noSender != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "无法推送 telegram（%v），报告仅打印到 stdout\n", noSender)
		}
	}
	code, err := executeBankReport(cfg, d)
	if err != nil {
		return err
	}
	if noSender != nil {
		return fmt.Errorf("银行月报未推送：%w", noSender)
	}
	if code != 0 {
		bankExit(code)
	}
	return nil
}

// executeBankReport 返回退出码：0 全部成功；2 部分主体拉取失败（报告照常推送）。
// error 非 nil 对应退出码 1：全部失败（仍推送一条逐家列出错误的摘要）或推送失败。
func executeBankReport(cfg *bank.Config, d bankReportDeps) (int, error) {
	results := bank.Collect(cfg.Banks, d.source)
	now := d.now()
	// nFailed 单独计数：failed 里还有别名附注行，行数不等于主体数。
	var failed []string
	nFailed := 0
	for _, r := range results {
		if r.Err != nil {
			nFailed++
			failed = append(failed, fmt.Sprintf("· %s：%v", r.Name, r.Err))
			// 与 bank.writeAliases 同格式：H 股等共用条目不能在摘要里消失。
			for _, a := range r.Aliases {
				failed = append(failed, fmt.Sprintf("  （%s 同 %s）", a, r.Symbol))
			}
		}
	}
	if nFailed == len(results) {
		msg := fmt.Sprintf("🏦 银行关键指标月报 %s\n❌ 全部 %d 家拉取失败\n%s",
			now.Format("2006-01-02"), nFailed, strings.Join(failed, "\n"))
		if err := deliver(d, msg); err != nil {
			return 1, err
		}
		return 1, fmt.Errorf("全部 %d 家银行拉取失败", nFailed)
	}
	if err := deliver(d, bankReportText(cfg, results, now)); err != nil {
		return 1, err
	}
	if nFailed > 0 {
		return 2, nil
	}
	return 0, nil
}

// bankReportText 由拉取结果生成报告全文；失败主体不参与预警（其 Ind 全为 NaN）。
func bankReportText(cfg *bank.Config, results []bank.BankResult, now time.Time) string {
	var alerts []bank.Alert
	for _, r := range results {
		if r.Err == nil {
			alerts = append(alerts, bank.Alerts(r, cfg.Thresholds)...)
		}
	}
	return bank.Render(bank.Summarize(results, cfg.RankIndicator()), alerts, now)
}

// deliver 按 bank.MaxMessageRunes 切段后逐段推送；无 sender 时打印到 d.out。
func deliver(d bankReportDeps, text string) error {
	chunks := bank.Split(text, bank.MaxMessageRunes)
	if d.sender == nil {
		for _, c := range chunks {
			fmt.Fprintln(d.out, c)
			fmt.Fprintln(d.out)
		}
		return nil
	}
	for i, c := range chunks {
		if err := d.sender.SendText(c); err != nil {
			return fmt.Errorf("telegram 推送第 %d/%d 段失败: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

// buildBankSender 复用主配置 notifiers.telegram 凭据（同 buildCrisisSender）。
// 拿不到时返回 nil 与说明原因的错误，由调用方写到 stderr。
func buildBankSender() (bank.Sender, error) {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		// YAML 解析错误本身不含文件路径，这里补上是哪份主配置。
		return nil, fmt.Errorf("主配置 %q 读取失败: %w", cfgFile, err)
	}
	nc, ok := cfg.Notifiers["telegram"]
	if !ok || !nc.Enabled {
		return nil, errors.New("主配置未启用 notifiers.telegram")
	}
	if nc.BotToken == "" || nc.ChatID == "" {
		return nil, errors.New("notifiers.telegram 缺 bot_token 或 chat_id")
	}
	return telegram.New(nc.BotToken, nc.ChatID, telegram.WithProxy(nc.Proxy)), nil
}
