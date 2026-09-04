package hestia

import (
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// StatusRow 是 status 命令展示的一行观测摘要。
type StatusRow struct {
	Period, PeriodType, PublishedAt, Extractor string
}

// PendingRow 是一次未过闸的尝试。
//
// Failed 从 report 列的 JSON 解出 —— 不解就等于让人去 sqlite 里读 JSON，
// 而 pending 表自述「它的消费者只有人」（schema.go:13）。
type PendingRow struct {
	Period, PeriodType, PublishedAt, IngestedAt string
	Failed                                      []Check
}

// RenderStatus 把库状态写成人读的形式。
//
// dbPath 会被解析成绝对路径再打印：它是相对路径（约束 C8），在错误的 cwd 下
// 会静默指向另一个不存在的库，然后如实报告「0 期」—— 看起来像数据没进来，
// 实际是查错了地方。
//
// 解析发生在这里而不是 cmd 层（M1b-4b reviewer D7 裁定）：两处都做会解析两次，
// 行为无害，但下一个人读到 cmd 层那句「这是唯一被解析的地方」会来把这里删掉。
func RenderStatus(w io.Writer, dbPath string, obs []StatusRow, pending []PendingRow, runs []Run) error {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		abs = dbPath // 拿不到绝对路径不该让 status 整个失败
	}
	if _, err := fmt.Fprintf(w, "hestia status  (db: %s)\n\n", abs); err != nil {
		return err
	}

	fmt.Fprintf(w, "observations: %d\n", len(obs))
	for _, r := range obs {
		fmt.Fprintf(w, "  %-8s %-8s published %s  %s\n",
			r.Period, r.PeriodType, r.PublishedAt, r.Extractor)
	}

	fmt.Fprintf(w, "\npending: %d\n", len(pending))
	for _, p := range pending {
		fmt.Fprintf(w, "  %-8s %-8s published %s  ingested %s\n",
			p.Period, p.PeriodType, p.PublishedAt, p.IngestedAt)
		for _, c := range p.Failed {
			fmt.Fprintf(w, "    %-20s failed  %s\n", c.ID, c.Reason)
		}
	}

	// runs 段（M1.5 的 TASK-007）：最近几次运行。销 M1d 挂账 C2 的第二半——通知失败
	// 此前只在 err.log 里，这里让它在 status 里可见。
	//
	// RunAt 先 UTC() 再格式化：库里存的就是 UTC，但读回的 Location 可能是 +00:00
	// 而非 UTC，不转会打成 `+00:00` 后缀而非 `Z`。`%-9s` 对齐到最长的 outcome
	// `duplicate`（9 字符），恰好不截断。
	fmt.Fprintf(w, "\nruns: %d\n", len(runs))
	for _, r := range runs {
		fmt.Fprintf(w, "  %s  %-9s", r.RunAt.UTC().Format(time.RFC3339), r.Outcome)
		if r.Period != "" {
			fmt.Fprintf(w, "  %s/%s", r.Period, r.PeriodType)
		}
		if r.Stage != "" {
			fmt.Fprintf(w, "  stage=%s", r.Stage)
		}
		if r.Error != "" {
			fmt.Fprintf(w, "  %s", r.Error)
		}
		if r.NotifyError != "" {
			fmt.Fprintf(w, "  notify_error=%s", r.NotifyError)
		}
		fmt.Fprintln(w)
	}
	return nil
}
