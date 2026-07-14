package crisis

// MemHistory is the in-memory EvalHistory used by replay and tests; live
// evaluation uses Store.History instead. Entries are kept newest-first.
type MemHistory struct {
	sys []Evaluation
	ind map[string][]Evaluation
}

func NewMemHistory() *MemHistory {
	return &MemHistory{ind: map[string][]Evaluation{}}
}

// Append prepends one evaluation day's rows.
func (m *MemHistory) Append(evals []Evaluation) {
	for _, e := range evals {
		if e.Indicator == "" {
			m.sys = append([]Evaluation{e}, m.sys...)
		} else {
			m.ind[e.Indicator] = append([]Evaluation{e}, m.ind[e.Indicator]...)
		}
	}
}

func (m *MemHistory) RecentSystem(n int) ([]Evaluation, error) {
	return headN(m.sys, n), nil
}

func (m *MemHistory) RecentIndicator(indicator string, n int) ([]Evaluation, error) {
	return headN(m.ind[indicator], n), nil
}

func headN(evals []Evaluation, n int) []Evaluation {
	if len(evals) > n {
		return evals[:n]
	}
	return evals
}
