package analysis

type FunctionSummary struct {
	Params  []Interval
	Returns []Interval
}

func (s *FunctionSummary) ArgsMatch(args []Interval) bool {
	if len(s.Params) != len(args) {
		return false
	}

	for i, _ := range s.Params {
		if !s.Params[i].Equals(args[i]) {
			return false
		}
	}
	return true
}
