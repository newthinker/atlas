package bank

// Collect 按取数代码去重（A+H 同一主体只拉一次、只计一次，配置中首个条目为主名），
// 按首次出现顺序逐主体拉取并分析；单个主体失败只记在其 Err 上，不影响其他主体。
func Collect(banks []BankCfg, src Source) []BankResult {
	var results []BankResult
	idx := map[string]int{}
	for _, b := range banks {
		sym := b.DataSymbol()
		if i, ok := idx[sym]; ok {
			results[i].Aliases = append(results[i].Aliases, b.DisplayName())
			continue
		}
		idx[sym] = len(results)
		results = append(results, BankResult{Symbol: sym, Name: b.DisplayName()})
	}
	for i := range results {
		series, err := src.Fetch(results[i].Symbol)
		if err != nil {
			series = Series{} // 失败主体一律零时间 + 全 NaN，不采信出错时返回的部分数据
		}
		results[i].Latest, results[i].Ind = Analyze(series.Obs)
		results[i].MissingFields = series.MissingFields
		results[i].Err = err
	}
	return results
}
