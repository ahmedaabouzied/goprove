package analysis

import (
	"fmt"
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

type Analyzer struct {
	state    map[ssa.Value]Interval
	findings []Finding
	err      error
}

type Finding struct {
	Pos      token.Pos
	Message  string
	Severity Severity
}

type Severity uint8

const (
	Safe Severity = iota
	Warning
	Bug
)

func (a *Analyzer) Analyze(fn *ssa.Function) []Finding {
	a.state = make(map[ssa.Value]Interval)

	for _, param := range fn.Params {
		a.state[param] = Top()
	}

	blocks, err := ReversePostOrder(fn)
	if err != nil {
		a.err = err
		return nil
	}

	for _, block := range blocks {
		for _, instr := range block.Instrs {
			a.transferInstruction(instr)
		}
	}
	return a.findings
}

func (a *Analyzer) transferInstruction(instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.BinOp:
		a.transferBinOp(v)
	case *ssa.Phi:
		a.transferPhi(v)
	}
}

func (a *Analyzer) transferBinOp(v *ssa.BinOp) {
	x := a.lookupInterval(v.X)
	y := a.lookupInterval(v.Y)

	var result Interval
	switch v.Op {
	case token.ADD:
		result = x.Add(y)
	case token.SUB:
		result = x.Sub(y)
	case token.MUL:
		result = x.Mul(y)
	case token.QUO, token.REM:
		a.flagDivisionByZero(v, y)
		result = x.Div(y)
	default:
		result = Top()
	}
	a.state[v] = result
}

func (a *Analyzer) transferPhi(v *ssa.Phi) {
	result := Bottom()

	for _, edge := range v.Edges {
		result = result.Join(a.lookupInterval(edge))
	}

	a.state[v] = result
}

func (a *Analyzer) flagDivisionByZero(v *ssa.BinOp, divisor Interval) {
	if !divisor.ContainsZero() {
		return
	}

	if divisor.Equals(NewInterval(0, 0)) {
		a.findings = append(a.findings, Finding{
			Pos:      v.Pos(),
			Message:  "division by zero",
			Severity: Bug,
		})
		return
	}

	a.findings = append(a.findings, Finding{
		Pos:      v.Pos(),
		Message:  "possible division by zero",
		Severity: Warning,
	})
}

func (a *Analyzer) lookupInterval(v ssa.Value) Interval {
	if c, ok := v.(*ssa.Const); ok {
		// Extract int64 from the cosnt value
		if c.Value.Kind() != constant.Int {
			a.err = fmt.Errorf("parsing non int const into an interval")
			return Top()
		}
		val := c.Int64() // This will not panic because of the check above
		return NewInterval(val, val)
	}
	if iv, ok := a.state[v]; ok {
		return iv
	}
	return Top() // Value is unknown
}
