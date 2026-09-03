package hestia

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

// Sender 是通知通道的窄接口，与 crisis.Sender 同形（方案报告 4.8.1，M1d 的 TASK-004）。
//
// 不走 internal/notifier 的 Notifier 接口——那个面向交易信号（Send(core.Signal)），
// 「闸门拦截」「解析失败」塞进 core.Signal 会很别扭。telegram.Telegram 的 SendText
// 直接满足本接口；cmd 层负责构造，本包只管「发什么」。
//
// 本包不负责「什么时候不发」以外的策略：纯文本、不设 parse_mode、单通道无路由，
// 紧急度由 [P0] / [P1] / [P2] 前缀承载。
type Sender interface {
	SendText(text string) error
}

// renderP0：校验闸门拦截、数据落 pending。**这条必须有人看到**——它意味着这期数据缺失。
// 只列 failed 的闸：passed / skipped 混进来会把 P0 稀释成一张全表。
//
// Check.Value 在「无意义时为 nil」（types.go），failed 闸也可能如此（例如期次不一致
// 这类没有单一实测数的闸），故这里写 n/a 而不是解引用。
func renderP0(obs Observation, rep ValidationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[P0] hestia %s/%s 校验未过，已落 pending_review\n", obs.Meta.Period, obs.Meta.PeriodType)
	for _, c := range rep.Checks {
		if c.Status != CheckFailed {
			continue
		}
		val := "n/a"
		if c.Value != nil {
			val = fmtNum(*c.Value)
		}
		fmt.Fprintf(&b, "- %s: 实测 %s · %s\n", c.ID, val, c.Reason)
	}
	fmt.Fprintf(&b, "article %s · extractor %s", obs.Meta.ArticleID, obs.Meta.Extractor)
	return b.String()
}

// renderP1：fetch / parse / 期次不一致 / validate / save 任一阶段失败。
// 只取错误首行：errors.Join 出来的多行错误会把一条通知撑成一屏，而首行已带
// 「期次 (article_id): 阶段」前缀（ingestOne 的 wrap 保证）。
func renderP1(c Candidate, err error) string {
	first, _, _ := strings.Cut(err.Error(), "\n")
	return fmt.Sprintf("[P1] hestia %s/%s 处理失败 (article %s)\n%s",
		c.Period, c.PeriodType, c.ArticleID, first)
}

// renderP2：入权威表。Verdict 原样写进去——Duplicate / Revision / OutOfOrder 不吞，
// 与 ingest.go 「不要为当前的局限写断言」那条理由同源。
//
// Duplicate 的措辞是「已在库（本次抽取值未写入）」而不是「入库」（QA 终审 A7，M1d 的
// TASK-004 返工）：Force 重跑已入库期次时 Save 判 Duplicate、refreshArticleID 只刷
// article_id、本次新抽的 Values 一个都不写（ingest.go 注释原话）。下面那行锚值是**本次
// 重抽**的数，不是库里的；写成「入库」会让运维以为这些数进了库。其它 Verdict 不变。
//
// 四个冷热信号与综合温度尚未实现（等 M2），这里只带四个锚字段：
// 存量三项单位万亿元、存款流量单位亿元并标口径（同族 _ytd / _mom 哪个非空取哪个，
// 两者同在取 ytd）。
func renderP2(obs Observation, out Outcome) string {
	dep, caliber := anchorFlow(obs.Values, FieldDepositFlowYTD, FieldDepositFlowMoM)
	action := "入库"
	if out.Verdict == bitemporal.Duplicate {
		action = "已在库（本次抽取值未写入）"
	}
	return fmt.Sprintf("[P2] hestia %s/%s %s %s · extractor %s\nM2 %s 万亿 · M1 %s 万亿 · 社融存量 %s 万亿 · 人民币存款 %s 亿元 (%s)\narticle %s · 发布 %s",
		obs.Meta.Period, obs.Meta.PeriodType, action, out.Verdict, obs.Meta.Extractor,
		anchorValue(obs.Values, FieldM2), anchorValue(obs.Values, FieldM1),
		anchorValue(obs.Values, FieldTSFStock), dep, caliber,
		obs.Meta.ArticleID, obs.Meta.PublishedAt)
}

// anchorValue 取一个字段的显示值；缺失写 n/a，不写 0——0 是一个合法的数。
func anchorValue(vals map[string]float64, key string) string {
	v, ok := vals[key]
	if !ok {
		return "n/a"
	}
	return fmtNum(v)
}

// fmtNum 用最短精确表示：356.71 → "356.71"，177600 → "177600"。
// 不用 %g：它会把 177600 打成 1.776e+05，运维读数要再换算一次。
func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// anchorFlow 在同族 _ytd / _mom 里取非空的那个，并返回口径标签；两者同在优先 ytd。
// 两者都缺 ⇒ n/a 与口径 -。
func anchorFlow(vals map[string]float64, ytd, mom string) (value, caliber string) {
	if v, ok := vals[ytd]; ok {
		return fmtNum(v), "ytd"
	}
	if v, ok := vals[mom]; ok {
		return fmtNum(v), "mom"
	}
	return "n/a", "-"
}
