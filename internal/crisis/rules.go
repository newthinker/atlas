package crisis

// minPercentileObs is the minimum window size for the percentile track and
// the CROWDED check (plan deviation 3: short windows are annotated in
// WindowActualObs, but a rank over a handful of points is meaningless).
const minPercentileObs = 60

// EvaluateIndicator runs the shared pipeline for one indicator (design §3.1):
// data presence → freshness → absolute-threshold track → percentile track,
// either track escalating (maxStatus). NO_DATA and STALE short-circuit and
// leave resonance counting; seasonal suppression and hysteresis are applied
// later by EvalDay.
func EvaluateIndicator(cfg *Config, indicator, date string, sr SeriesReader) (IndicatorResult, error) {
	res := IndicatorResult{Indicator: indicator, RawStatus: StatusGreen, Pct5y: -1}

	latest, err := sr.Window(indicator, date, 1)
	if err != nil {
		return res, err
	}
	if len(latest) == 0 {
		res.Status, res.RawStatus = StatusNoData, StatusNoData
		return res, nil
	}
	res.Value = latest[0].Value
	if staleFor(cfg, indicator, date, latest[0].Date) {
		res.Status, res.RawStatus = StatusStale, StatusStale
		return res, nil
	}

	pctWin, err := sr.WindowSince(indicator, addYears(date, -cfg.Percentile.WindowYears), date)
	if err != nil {
		return res, err
	}
	res.Pct5y, res.WindowActualObs = Percentile(pctWin, res.Value)

	if err := evalAbsolute(cfg, indicator, date, sr, &res); err != nil {
		return res, err
	}

	if percentileTrack(cfg, indicator) && res.WindowActualObs >= minPercentileObs {
		switch {
		case res.Pct5y >= cfg.Percentile.Red:
			res.RawStatus = maxStatus(res.RawStatus, StatusRed)
		case res.Pct5y >= cfg.Percentile.Amber:
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}

	res.Status = res.RawStatus
	return res, nil
}

func percentileTrack(cfg *Config, indicator string) bool {
	switch indicator {
	case IndVIX:
		return cfg.Indicators.VIX.PercentileTrack
	case IndMOVE:
		return cfg.Indicators.MOVE.PercentileTrack
	case IndSOFREFFR:
		return cfg.Indicators.SOFREFFR.PercentileTrack
	case IndHYOAS:
		return cfg.Indicators.HYOAS.PercentileTrack
	case IndT10Y2Y:
		return cfg.Indicators.T10Y2Y.PercentileTrack
	case IndNFCI:
		return cfg.Indicators.NFCI.PercentileTrack
	case IndUSDJPY:
		return cfg.Indicators.USDJPY.PercentileTrack
	}
	return false
}

func evalAbsolute(cfg *Config, indicator, date string, sr SeriesReader, res *IndicatorResult) error {
	switch indicator {
	case IndVIX:
		return evalVIX(cfg, date, sr, res)
	case IndMOVE:
		evalMOVE(cfg, res)
	case IndSOFREFFR:
		return evalSOFREFFR(cfg, date, sr, res)
	case IndHYOAS:
		return evalHYOAS(cfg, date, sr, res)
	case IndT10Y2Y:
		return evalT10Y2Y(cfg, date, sr, res)
	case IndNFCI:
		evalNFCI(cfg, res)
	case IndUSDJPY:
		return evalUSDJPY(cfg, date, sr, res)
	}
	return nil
}

func evalVIX(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.VIX
	switch {
	case res.Value > c.Red:
		res.RawStatus = StatusRed
	case res.Value >= c.Amber:
		res.RawStatus = StatusAmber
	}
	win, err := sr.Window(IndVIX, date, 6)
	if err != nil {
		return err
	}
	if wow, ok := WowPct(win); ok && wow > c.WeeklySpikePct {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
	}
	return nil
}

func evalMOVE(cfg *Config, res *IndicatorResult) {
	c := cfg.Indicators.MOVE
	switch {
	case res.Value > c.Red:
		res.RawStatus = StatusRed
	case res.Value >= c.Amber:
		res.RawStatus = StatusAmber
	}
}

// evalSOFREFFR: persistence conditions are the core noise filter (design
// §3.1 note 3) — red needs red_persist_days consecutive observations above
// red_bp, amber likewise over its own window.
func evalSOFREFFR(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.SOFREFFR
	win, err := sr.Window(IndSOFREFFR, date, c.RedPersistDays)
	if err != nil {
		return err
	}
	if len(win) >= c.RedPersistDays && allAbove(win, c.RedBp) {
		res.RawStatus = StatusRed
		return nil
	}
	if len(win) >= c.AmberPersistDays && allAbove(lastN(win, c.AmberPersistDays), c.AmberBp) {
		res.RawStatus = StatusAmber
	}
	return nil
}

// evalHYOAS: two-sided amber (design §3.1 note 1) — too tight is complacency,
// moderately wide is stress; the momentum condition catches fast widening
// before the level does.
func evalHYOAS(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.HYOAS
	switch {
	case res.Value > c.RedBp:
		res.RawStatus = StatusRed
	case res.Value < c.AmberLowBp:
		res.RawStatus, res.Tag = StatusAmber, TagComplacency
	case res.Value >= c.AmberHighBp:
		res.RawStatus, res.Tag = StatusAmber, TagStress
	}
	win, err := sr.Window(IndHYOAS, date, c.MomentumWindowObs+1)
	if err != nil {
		return err
	}
	if mom, ok := MomChange(win, c.MomentumWindowObs); ok && mom > c.MomentumBp {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		if res.Tag == "" {
			res.Tag = TagStress
		}
	}
	return nil
}

// evalT10Y2Y: inversion is red, but STEEPENING marks the historically most
// dangerous window — fast re-steepening after an inversion (design §3.1
// note 2). The tag never changes the color by itself.
func evalT10Y2Y(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.T10Y2Y
	switch {
	case res.Value < 0:
		res.RawStatus = StatusRed
	case res.Value <= c.AmberBp:
		res.RawStatus = StatusAmber
	}
	win, err := sr.Window(IndT10Y2Y, date, c.SteepeningLookbackObs)
	if err != nil {
		return err
	}
	lowest := res.Value
	for _, o := range win {
		if o.Value < lowest {
			lowest = o.Value
		}
	}
	if lowest < 0 && res.Value-lowest > c.SteepeningBp {
		res.Tag = TagSteepening
	}
	return nil
}

func evalNFCI(cfg *Config, res *IndicatorResult) {
	c := cfg.Indicators.NFCI
	switch {
	case res.Value > c.RedAbove:
		res.RawStatus = StatusRed
	case res.Value >= c.GreenBelow:
		res.RawStatus = StatusAmber
	}
}

// evalUSDJPY: JPY 急升值 = USDJPY 下跌，故 wow 阈值为负、≤ 比较（carry trade
// 急平仓方向）；CROWDED = USDJPY 处 52 周高分位（日元极端弱势 = 空头拥挤）。
func evalUSDJPY(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.USDJPY
	win, err := sr.Window(IndUSDJPY, date, 6)
	if err != nil {
		return err
	}
	if wow, ok := WowPct(win); ok {
		switch {
		case wow <= c.RedWowPct:
			res.RawStatus = StatusRed
		case wow <= c.AmberWowPct:
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}
	yr, err := sr.WindowSince(IndUSDJPY, addYears(date, -1), date)
	if err != nil {
		return err
	}
	if p, n := Percentile(yr, res.Value); n >= minPercentileObs && p >= c.Crowded52wPct {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		res.Tag = TagCrowded
	}
	return nil
}

func allAbove(obs []Observation, threshold float64) bool {
	for _, o := range obs {
		if o.Value <= threshold {
			return false
		}
	}
	return true
}

func lastN(obs []Observation, n int) []Observation {
	if len(obs) <= n {
		return obs
	}
	return obs[len(obs)-n:]
}
