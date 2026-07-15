package crisis

import "fmt"

// ReplayReport 渲染一个回放日的指定形式报告（忽略消息矩阵门控，回放前缀由
// cmd 层拼接，渲染器不感知回放）。prev 为前一回放日，窗口首日传 nil——PrevDay
// 空 map 使差异行输出"无变化"（设计 §3，可接受）。
func ReplayReport(cfg *Config, form string, day ReplayDay, prev *ReplayDay, sr SeriesReader) (string, error) {
	nc := NotifyContext{
		Res:       day.Res,
		StateDays: day.StateDays,
		PrevDay:   map[string]Evaluation{},
	}
	if prev != nil {
		for _, ind := range AllIndicators {
			r := prev.Res.Results[ind]
			nc.PrevDay[ind] = Evaluation{Indicator: ind, Status: r.Status, Value: r.Value}
		}
	}
	switch form {
	case "daily":
		return renderDaily(cfg, nc), nil
	case "monthly":
		nc.Trends = map[string]Trend{}
		for _, ind := range AllIndicators {
			win, err := sr.Window(ind, day.Date, 21)
			if err != nil {
				return "", err
			}
			if len(win) == 0 {
				continue
			}
			nc.Trends[ind] = Trend{Window: win, Delta: win[len(win)-1].Value - win[0].Value}
		}
		return renderMonthly(cfg, nc), nil
	}
	return "", fmt.Errorf("unknown report form %q (want daily or monthly)", form)
}
