package tiingo

import (
	"regexp"
	"strings"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// reTicker 是美股/ETF 代码形态：1–5 位大写字母基底 + 可选单字母份额类后缀
// （BRK.B / BRK-B）。基底排除数字、后缀限单字母，使数字代码（7203、9988）、
// 外国交易所代码（SAP.DE）与数字形态指数（931151.CSI）在形态层就被拒——
// MarketForSymbol 对它们兜底 US，拦不住（人类裁决 P11，AD-19）。
//
// 后缀字母是 A–C 白名单，其余份额类罕见、宁拒：单字母外国交易所后缀（HSBA.L）
// 与 BRK.B 形态同构，只能靠字母集合区分（Leader 裁决，AD-22）。
var reTicker = regexp.MustCompile(`^[A-Z]{1,5}([.-][A-C])?$`)

// Supported 报告 symbol 是否属于一期覆盖范围（美股/ETF，设计 §3.2）。
// 其余一律拒绝且不发请求：serve 兜底循环会把任意标的交给最后一跳，
// 不拦截就会用 A 股/港股/指数白白消耗小时配额。
//
// 已知限制：路由表的加密前缀（UNI*/LINK*/ADA* 等）会让部分真实美股被拒（AD-12）。
func Supported(symbol string) bool {
	return reTicker.MatchString(symbol) && collector.MarketForSymbol(symbol) == core.MarketUS
}

// toTicker 把 atlas 代码转为 Tiingo ticker：份额类分隔符 '.' 改 '-'（BRK.B → BRK-B）。
func toTicker(symbol string) string { return strings.ReplaceAll(symbol, ".", "-") }
