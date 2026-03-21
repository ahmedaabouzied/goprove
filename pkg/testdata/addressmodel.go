package testdata

// ---------------------------------------------------------------------------
// Address model test fixtures
// These test patterns that require tracking nil state per memory address
// rather than per SSA register.
// ---------------------------------------------------------------------------

// AddrFieldReload: if o.In != nil { o.In.Val } — two loads from same field.
func AddrFieldReload(o *Outer) int {
	if o.In != nil {
		return o.In.Val // safe — o.In was just checked
	}
	return 0
}

// AddrFieldReloadMultiple: multiple accesses after nil check.
func AddrFieldReloadMultiple(o *Outer) int {
	if o.In != nil {
		a := o.In.Val  // safe
		b := o.In.Val  // safe
		return a + b
	}
	return 0
}

// AddrNestedFieldCheck: if o.In != nil { if o.In.Val > 0 { use(o.In.Val) } }
func AddrNestedFieldCheck(o *Outer) int {
	if o.In != nil {
		if o.In.Val > 0 {
			return o.In.Val // safe — o.In checked above
		}
	}
	return 0
}

// AddrGlobalNilCheck: if globalOuter.In != nil { globalOuter.In.Val }
var globalOuter *Outer

func AddrGlobalFieldReload() int {
	if globalOuter != nil {
		if globalOuter.In != nil {
			return globalOuter.In.Val // safe — both checked
		}
	}
	return 0
}

// AddrFieldNotChecked: o.In used without nil check — should warn.
func AddrFieldNotChecked(o *Outer) int {
	return o.In.Val // want "possible nil dereference"
}

// AddrDeepNested: mirrors the go-redis FTAggregateQuery pattern.
// options is nil-checked at the top, then multiple fields are accessed
// across many nested if blocks deep inside the guarded scope.
type DeepConfig struct {
	Enabled   bool
	Name      string
	SubConfig *SubConfig
}

type SubConfig struct {
	Count   int
	MaxIdle int
}

func AddrDeepNested(cfg *DeepConfig) int {
	result := 0
	if cfg != nil {
		if cfg.Enabled {
			result += 1
		}
		if cfg.Name != "" {
			result += 2
		}
		if cfg.SubConfig != nil {
			if cfg.SubConfig.Count > 0 {
				result += cfg.SubConfig.Count
			}
			if cfg.SubConfig.MaxIdle > 0 {
				result += cfg.SubConfig.MaxIdle
			}
		}
	}
	return result
}

// AddrGoRedisPattern mirrors the exact go-redis FTAggregateQuery pattern.
// options is nil-checked at the top, many fields accessed in between,
// then a nested pointer field is nil-checked and accessed.
func AddrGoRedisPattern(options *DeepConfig) []interface{} {
	queryArgs := []interface{}{"query"}
	if options != nil {
		if options.Enabled {
			queryArgs = append(queryArgs, "ENABLED")
		}
		if options.Name != "" {
			queryArgs = append(queryArgs, "NAME", options.Name)
		}
		// Many intermediate accesses of options fields...
		if options.Enabled && options.Name != "" {
			queryArgs = append(queryArgs, "BOTH")
		}
		// Now the nested pointer field check:
		if options.SubConfig != nil {
			if options.SubConfig.Count > 0 {
				queryArgs = append(queryArgs, "COUNT", options.SubConfig.Count)
			}
			if options.SubConfig.MaxIdle > 0 {
				queryArgs = append(queryArgs, "MAXIDLE", options.SubConfig.MaxIdle)
			}
		}
	}
	return queryArgs
}
