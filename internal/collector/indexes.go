package collector

// AShareIndexSecIDs maps A-share index symbols (watchlist form) to Eastmoney
// secids. Membership in this map is also how the rest of the codebase tells
// an A-share index apart from an equity with the same numeric code.
var AShareIndexSecIDs = map[string]string{
	"000001.SH": "1.000001", // 上证指数
	"000016.SH": "1.000016", // 上证50
	"000300.SH": "1.000300", // 沪深300
	"000905.SH": "1.000905", // 中证500
	"399001.SZ": "0.399001", // 深证成指
	"399006.SZ": "0.399006", // 创业板指
}

// IsAShareIndex reports whether symbol is a known A-share index.
func IsAShareIndex(symbol string) bool {
	_, ok := AShareIndexSecIDs[symbol]
	return ok
}
