package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/hestia"
	"github.com/newthinker/atlas/internal/hestia/sheets"
)

// —— sheets push 的五个 flag（M2b 的 TASK-009）——
//
// ⚠️ 一律带 `hestiaSheets` 前缀：`cmd/atlas` 是**一个包**，`period` / `apply` 这种名字
// 在这里迟早撞上别的子命令（`hestiaBackfillFrom` 那条注释记过一次真撞）。
//
// ⚠️ `hestiaSheetsPeriodType` **刻意不给默认值**（`contract emit` 给的是 "monthly"）。
// 给了默认值，「只给 --period」就不再是错误，而那正是要拦的那种输入：同一个 period
// 可能有 monthly 与 h1 两条，猜错写进表里的是另一份报告的数字，静默且难查。
var (
	hestiaSheetsAll        bool
	hestiaSheetsPeriod     string
	hestiaSheetsPeriodType string
	hestiaSheetsApply      bool
	hestiaSheetsCreate     bool
)

// newSheetsClient / sheetsPush 是两个包级测试缝。
//
// 分两个而不是一个：前者造客户端（测试指向 httptest），后者是投影本身（测试注入必 panic
// 的替身）。合成一个的话，「Push 炸了」就只能靠真去打 API 才造得出来。
var (
	newSheetsClient = sheets.NewClient
	sheetsPush      = sheets.Push
)

// sheetsDisabledLine 是能力禁用时的固定一句。**退出码 0**：没配 hestia_sheets 不是故障，
// 与 ingest 侧 C9 同语义；退非零会让巡检脚本把「没配」报成出错。
const sheetsDisabledLine = "hestia_sheets 未配置（credentials_file 留空）"

// sheetsDryRunLine 是 dry-run 的末尾固定一句。开头的 `dry-run` 不是修辞：
// 判据同时要认「明说没写」与「说清怎么才写」，两者缺一，人看完这行仍不知道下一步。
const sheetsDryRunLine = "dry-run：未写入任何内容；确认后加 --apply"

// sheetsCallTimeout 给每次 Sheets 调用一个上限。
//
// 🔴 不设就没有上限：Google API 走代理出网，代理没起时连接会一直挂着，而 ingest 是
// launchd 唤起的批处理——挂住就是这一轮永远不结束、下一个时点的唤起叠上来。
// 取 2 分钟：一次全量 push 对 6 张年度表是 1+2T 次 read 加一次 batchUpdate，
// 实测远小于此；真超了说明网络确实不通，按 C8 打印一行即可，不阻断入库。
const sheetsCallTimeout = 2 * time.Minute

// sheetsEntryFirstRow 是录入区首行（1 月）在表里的 1 基行号。
//
// ⚠️ 与 sheets 包的 entryRowOffset 是**同一事实的两个副本**——那个未导出，这里只为把
// 行号还原成月份给人看。改表结构时两处都要改；漏改这里只会让排版的月份错位，不影响写入
// （写哪一格由 Change.Row 决定，不经过这里）。
const sheetsEntryFirstRow = 4

var hestiaSheetsCmd = &cobra.Command{
	Use:          "sheets",
	Short:        "Project observations into the Google Sheets annual tabs",
	SilenceUsage: true,
}

var hestiaSheetsPushCmd = &cobra.Command{
	Use:   "push",
	Short: "把观测投影到 Google Sheets 年度表录入区",
	Long: `Compare the database against the annual tabs cell by cell and, with --apply, write
only the cells that differ.

Default is dry-run: it prints what would change and touches nothing. --create-sheets
is only meaningful with --apply; scheduled ingest never passes it explicitly.`,
	SilenceUsage: true,
	RunE:         runHestiaSheetsPush,
}

// validateSheetsPushFlags 只看 flag，不碰库也不碰网络——所以无库环境也跑得动。
//
// 放在开库与建客户端之前，照 runHestiaIngest 里 --only-period 那两条校验的先例：
// 参数写错的人要在第一秒知道，不等开库、不等出网。
func validateSheetsPushFlags() error {
	switch {
	case hestiaSheetsAll && hestiaSheetsPeriod != "":
		return fmt.Errorf("--all 与 --period 互斥：要么全推，要么指定一期")
	case !hestiaSheetsAll && hestiaSheetsPeriod == "":
		return fmt.Errorf("必须给 --period YYYY-MM（并配 --period-type）或 --all")
	case hestiaSheetsPeriod != "":
		if !hestiaBackfillFromRE.MatchString(hestiaSheetsPeriod) {
			return fmt.Errorf("--period %q 格式非法：要 YYYY-MM，月份取 01–12", hestiaSheetsPeriod)
		}
		if hestiaSheetsPeriodType == "" {
			return fmt.Errorf("--period 必须配 --period-type：同一期可能有 monthly 与 h1 两条")
		}
		if !hestiaPeriodTypeRE.MatchString(hestiaSheetsPeriodType) {
			return fmt.Errorf("--period-type %q 非法：monthly | q1 | h1 | q1_q3 | annual", hestiaSheetsPeriodType)
		}
	}
	return nil
}

func runHestiaSheetsPush(cmd *cobra.Command, _ []string) error {
	if err := validateSheetsPushFlags(); err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	cfg, st, err := openHestia()
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if cfg.HestiaSheets.CredentialsFile == "" {
		fmt.Fprintln(out, sheetsDisabledLine)
		return nil
	}

	rows, err := hestia.BuildSheetRows(cmd.Context(), st)
	if err != nil {
		return err
	}
	return pushSheets(cmd.Context(), out, cfg, rows, hestiaSheetsPeriod,
		sheets.Options{Apply: hestiaSheetsApply, CreateSheets: hestiaSheetsCreate})
}

// pushSheets 建客户端、投影、排版。rows 由调用方给，period 非空时只推那一期。
//
// 单列出来是为了能拿构造好的 rows 直接测：从 CLI 走的话 rows 来自库，而往库里塞一条
// 够 BuildSheetRows 用的观测要先过 ValidationReport 的四道一致性检查。
func pushSheets(ctx context.Context, out io.Writer, cfg hestia.Config, rows []sheets.Row,
	period string, opts sheets.Options) error {
	if period != "" {
		rows = filterRowsByPeriod(rows, period)
	}
	// CLI 路径同样给超时：人手动跑时挂住只是难受，但 `sheets push` 也可能进 cron。
	cctx, ccancel := context.WithTimeout(ctx, sheetsCallTimeout)
	defer ccancel()
	c, err := newSheetsClient(cctx, cfg.HestiaSheets.CredentialsFile, cfg.HestiaSheets.SpreadsheetID)
	if err != nil {
		return err
	}
	pctx, pcancel := context.WithTimeout(ctx, sheetsCallTimeout)
	defer pcancel()
	res, err := sheetsPush(pctx, c, rows, sheetLabels(), opts)
	if err != nil {
		return err
	}
	fmt.Fprint(out, formatResult(res))
	if !opts.Apply {
		fmt.Fprintln(out, sheetsDryRunLine)
	}
	return nil
}

// sheetsProjector 造 ingest 用的投影闭包。凭据留空 ⇒ 返回**字面量 nil**（C9）：
// `IngestDeps.ProjectSheets != nil` 是那条静默降级路径的唯一判据，返回一个「装了 nil
// 的函数值」会让它穿过去。
//
// ingest 自动投影固定 Apply + CreateSheets：新年份的第一期必然缺表，不许建就整批失败。
//
// 🔴 最外层 `defer recover()` 把 panic 转成 error 返回（**不是吞掉**，原始 panic 值留在
// 错误里），让它走 C8 既有的「只打印不返回」路径。理由：这里用的是真实第三方 client，
// 它的 panic 会穿透 Ingest 打断整轮入库——而投影按 ADR-0004 可再生，最不该做的就是让它
// 把数据挡在库外。recover 包住建客户端与 Push 两步：只兜 Push 等于赌它只在那里炸。
func sheetsProjector(cfg hestia.Config) func(context.Context, []sheets.Row) error {
	if cfg.HestiaSheets.CredentialsFile == "" {
		return nil
	}
	// 🔴 客户端**建一次复用**（QA round2 CRITICAL-5）：ingestOne 每期调一次投影，
	// 每次重建都要读凭据文件并做一次 JWT 交换。`--force` 会翻满 max_pages、逐期
	// 重跑，那时重建次数与候选数成正比，白白撞配额。
	//
	// 懒建而不是提前建：凭据非空不代表这一轮真会入库（多数唤起是幂等空跑），
	// 空跑时不该读凭据、更不该出网做 JWT 交换。
	var (
		client *sheets.Client
		build  error
		once   sync.Once
	)
	return func(ctx context.Context, rows []sheets.Row) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("hestia sheets: 投影 panic: %v", r)
			}
		}()
		once.Do(func() {
			// 建客户端也给超时：JWT 交换要出网，代理没起时它会一直挂着，
			// 而 ingest 是 launchd 唤起的，挂住就是这一轮永远不结束。
			cctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
			defer cancel()
			client, build = newSheetsClient(cctx, cfg.HestiaSheets.CredentialsFile, cfg.HestiaSheets.SpreadsheetID)
		})
		if build != nil {
			return build
		}
		pctx, cancel := context.WithTimeout(ctx, sheetsCallTimeout)
		defer cancel()
		_, perr := sheetsPush(pctx, client, rows, sheetLabels(), sheets.Options{Apply: true, CreateSheets: true})
		return perr
	}
}

// sheetLabels 是录入区 35 列的表头标签，顺序即列序。运行时按表头解析（C3），
// 这份只是「要哪些列」的清单。
func sheetLabels() []string {
	labels := make([]string, len(hestia.SheetColumns))
	for i, c := range hestia.SheetColumns {
		labels[i] = c.Label
	}
	return labels
}

// filterRowsByPeriod 只留下 period（YYYY-MM）那一期的行。
//
// ⚠️ 只按年月过滤，**period_type 选不了行**：BuildSheetRows 经 selectRows 已经把每个
// period 收敛成一条（有 monthly 用 monthly，否则取 published_at 最新的累计期次），而
// sheets.Row 只带 Year/Month。--period-type 仍必填，兑现的是「不许猜」，不是「选哪条」。
func filterRowsByPeriod(rows []sheets.Row, period string) []sheets.Row {
	var out []sheets.Row
	for _, r := range rows {
		// 把行拼成 period 再比，而不是把 period 拆成两个数：拆就得处理「拆不出来」，
		// 而那条分支在 validateSheetsPushFlags 之后不可达——无法触发的错误处理是负债。
		if fmt.Sprintf("%04d-%02d", r.Year, r.Month) == period {
			out = append(out, r)
		}
	}
	return out
}

// formatResult 排版一次投影的结果。纯函数：给同样的 Result 恒得同样的字符串。
//
// 三类**明细行**各自报出类别（spec §7.2），三类**计数**分开三行。合并成一类会让人看不出
// 「这格没动」是因为已经对了，还是因为库里没数——这两种情况的后续动作完全不同。
//
// 不做列宽对齐：CJK 在等宽终端占两格而 Go 的字符串长度按字节/rune 算，补空格只会把
// 中文列排得更歪。分隔用两个空格，让终端自己断。
func formatResult(res sheets.Result) string {
	var b strings.Builder
	seen := make(map[string]bool, len(res.Changes))
	for _, ch := range res.Changes {
		seen[fmt.Sprintf("%s/%d", ch.Sheet, ch.Row)] = true
		fmt.Fprintf(&b, "%s  %d月  %s %s  %s\n",
			ch.Sheet, ch.Row-sheetsEntryFirstRow+1, sheets.ColumnLetter(ch.Col), ch.Label, changeVerdict(ch))
	}
	if len(res.MissingTabs) > 0 {
		fmt.Fprintf(&b, "\n将新建工作表: %s   （需 --create-sheets）\n",
			strings.Join(res.MissingTabs, " "))
	}
	fmt.Fprintf(&b, "\n%d 行\n将写 %d 格\n一致跳过 %d 格\n库缺跳过 %d 格\n",
		len(seen), res.WillWrite, res.Same, res.AbsentInDB)
	// 🔴 丢格必须打印（006 返工第 3 轮）：`Result` 里有字段还不够——dry-run 不打印它，
	// 人就永远看不到。「有数据**根本没被比对**」与「比对完发现一致」是完全不同的两件事，
	// 而前者此前在输出里没有任何痕迹。
	//
	// 只在非零时打印：恒在的那一行会变成人人忽略的固定噪声，等于没打。
	if res.DroppedCells > 0 {
		fmt.Fprintf(&b, "⚠️ %d 格**未比对**（有行的月份越界，整行被跳过；上面三类之和因此不等于格数）\n",
			res.DroppedCells)
	}
	return b.String()
}

// changeVerdict 是一条明细行的后半截：这一格要怎么处置，以及凭什么。
func changeVerdict(ch sheets.Change) string {
	switch ch.Kind {
	case sheets.WillWrite:
		return fmt.Sprintf("表中 %s  将写 %v", cellDisplay(ch.Current), ch.Want)
	case sheets.Same:
		return fmt.Sprintf("表中 %v  == 一致，跳过", ch.Current)
	default: // AbsentInDB
		return "库缺，保留表中现值"
	}
}

// cellDisplay 把「表里没有值」显示成 (空)，而不是 Go 的 <nil>。
func cellDisplay(v any) string {
	if v == nil {
		return "(空)"
	}
	return fmt.Sprint(v)
}
