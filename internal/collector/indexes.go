package collector

// AShareIndexSecIDs maps A-share index symbols (watchlist form) to Eastmoney
// secids. Membership in this map is also how the rest of the codebase tells
// an A-share index apart from an equity with the same numeric code.
var AShareIndexSecIDs = map[string]string{
	// 宽基指数
	"000001.SH": "1.000001", // 上证指数
	"000016.SH": "1.000016", // 上证50
	"000300.SH": "1.000300", // 沪深300
	"000688.SH": "1.000688", // 科创50
	"000852.SH": "1.000852", // 中证1000
	"000905.SH": "1.000905", // 中证500
	"399001.SZ": "0.399001", // 深证成指
	"399006.SZ": "0.399006", // 创业板指
	// 重点行业/主题指数。中证发布的跨市场指数（93xxxx，.CSI 后缀）在东财的
	// secid 市场前缀为 2；下列 secid 均经 push2 接口实测验证（2026-06-12）。
	"000922.SH":  "1.000922", // 中证红利
	"000932.SH":  "1.000932", // 中证主要消费
	"000933.SH":  "1.000933", // 中证医药卫生
	"399975.SZ":  "0.399975", // 证券公司
	"399986.SZ":  "0.399986", // 中证银行
	"399997.SZ":  "0.399997", // 中证白酒
	"930604.CSI": "2.930604", // 中证海外中国互联网30
	"930713.CSI": "2.930713", // 中证人工智能主题（CS人工智）
}

// IsAShareIndex reports whether symbol is a known A-share index.
func IsAShareIndex(symbol string) bool {
	_, ok := AShareIndexSecIDs[symbol]
	return ok
}
