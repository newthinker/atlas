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
	// 重点行业指数
	"000932.SH": "1.000932", // 中证主要消费
	"000933.SH": "1.000933", // 中证医药卫生
	"399808.SZ": "0.399808", // 中证新能源
	"399967.SZ": "0.399967", // 中证军工
	"399975.SZ": "0.399975", // 证券公司
	"399986.SZ": "0.399986", // 中证银行
	"399997.SZ": "0.399997", // 中证白酒
}

// IsAShareIndex reports whether symbol is a known A-share index.
func IsAShareIndex(symbol string) bool {
	_, ok := AShareIndexSecIDs[symbol]
	return ok
}
