package collector

import (
	"sort"
	"strings"

	"github.com/newthinker/atlas/internal/core"
)

// Route 把一类符号形态绑定到一个 collector 与一个市场。
//
// Pattern 是简化 glob：只认一个 '*'，位于开头（后缀匹配）、结尾（前缀匹配）
// 或独占全串（兜底）。不用 path.Match：'.' 与 '=' 在符号里是普通字符，
// 而 path.Match 的 '*' 不跨 '/'，语义对不上还更慢。
type Route struct {
	Pattern   string
	Collector string
	Market    core.Market
}

// routes 是内置路由表（设计 §6.3）。**具体度优先，而非注册顺序**：查表前按
// 非通配字符数降序稳定排序，故 config 追加规则时不存在顺序陷阱——'^HSI'
// 永远胜过 '^*'，与它写在文件哪一行无关。
//
// 具体度相同时按本切片的书写顺序决胜（稳定排序）。唯一真正相撞的形态是
// '*.HK' 与加密前缀（同为 3，如 "BTC.HK"）：旧实现在这里是**自相矛盾**的
// ——SelectExternalForSymbol 判它 crypto（isCryptoSymbol 前缀命中），
// MarketForSymbol 判它 HK（'.HK' 分支排在 crypto 之前）。统一到一张表后
// 必须二选一，取 '.HK' 优先（后缀是显式的交易所标识，比裸前缀更强的信号），
// 故 '*.HK' 写在加密前缀之前。
//
// 注意该决胜只在具体度相等时生效：'DOGE*'/'AVAX*'/'MATIC*'/'LINK*'/'ATOM*'
// 具体度 ≥4，仍会盖过 '*.HK'（3）。这些形态在 watchlist 中都不存在，故不额外
// 加机制去拉平；若将来真出现此类符号，应显式加一条精确路由，而不是调整具体度语义。
var routes = sortRoutes([]Route{
	// A 股（原 isAShareSymbol）
	{"*.SH", "eastmoney", core.MarketCNA},
	{"*.SZ", "eastmoney", core.MarketCNA},
	// 港股（原 MarketForSymbol 的 .HK 分支）—— 见上方与加密前缀的决胜说明
	{"*.HK", "yahoo", core.MarketHK},
	// 已登记指数（原 indexMarkets）
	{"^GSPC", "yahoo", core.MarketUS},
	{"^IXIC", "yahoo", core.MarketUS},
	{"^DJI", "yahoo", core.MarketUS},
	{"^HSI", "yahoo", core.MarketHK},
	{"^HSCE", "yahoo", core.MarketHK},
	// 未登记指数兜底（原 isIndexSymbol）——命中它即 KnownIndexMarket 的 unknown
	{"^*", "yahoo", core.MarketUS},
	// 商品（原 isCommoditySymbol）
	{"*=F", "yahoo", core.MarketUS},
	// 加密后缀（原 isCryptoSymbol 的后缀分支）
	{"*-USD", "crypto", core.MarketCrypto},
	{"*USDT", "crypto", core.MarketCrypto},
	// 加密前缀（原 cryptoTickers 白名单，逐条平移；加币种改这里或走 config）
	{"BTC*", "crypto", core.MarketCrypto},
	{"ETH*", "crypto", core.MarketCrypto},
	{"SOL*", "crypto", core.MarketCrypto},
	{"XRP*", "crypto", core.MarketCrypto},
	{"DOGE*", "crypto", core.MarketCrypto},
	{"ADA*", "crypto", core.MarketCrypto},
	{"DOT*", "crypto", core.MarketCrypto},
	{"AVAX*", "crypto", core.MarketCrypto},
	{"MATIC*", "crypto", core.MarketCrypto},
	{"LINK*", "crypto", core.MarketCrypto},
	{"UNI*", "crypto", core.MarketCrypto},
	{"ATOM*", "crypto", core.MarketCrypto},
	{"LTC*", "crypto", core.MarketCrypto},
	// 默认兜底：美股
	{"*", "yahoo", core.MarketUS},
})

// sortRoutes 按具体度降序稳定排序。稳定性是语义的一部分：同具体度时
// 书写顺序决胜（见 routes 上的 '*.HK' 说明）。
func sortRoutes(in []Route) []Route {
	out := append([]Route(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		return specificity(out[i].Pattern) > specificity(out[j].Pattern)
	})
	return out
}

// specificity 是 pattern 中的非通配字符数。
func specificity(pattern string) int {
	return len(pattern) - strings.Count(pattern, "*")
}

// matchPattern 匹配简化 glob。s 必须已大写。
func matchPattern(pattern, s string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*"):
		return strings.HasSuffix(s, pattern[1:])
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	default:
		return pattern == s
	}
}

// lookupRoute 返回第一个命中的路由。表末尾有 "*" 兜底，故实际总能命中；
// 返回 ok 是为了在表被改坏时让调用方保守回退而非 panic。
func lookupRoute(upper string) (Route, bool) {
	for _, r := range routes {
		if matchPattern(r.Pattern, upper) {
			return r, true
		}
	}
	return Route{}, false
}
