package main

import (
	"bufio"
	"bytes"
	"go/token"
	"strings"
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"golang.org/x/tools/go/ssa"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestProver(t *testing.T, path string) *Prover {
	t.Helper()
	p, err := NewProver(path)
	if err != nil {
		t.Fatalf("NewProver(%s): %v", path, err)
	}
	return p
}

func findFunction(pkg *ssa.Package, name string) *ssa.Function {
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == name {
			return fn
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// printFinding tests
// ---------------------------------------------------------------------------

func TestPrintFinding_Bug(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/project/foo.go", -1, 1000)
	f.AddLine(0)   // line 1
	f.AddLine(100) // line 2

	finding := analysis.Finding{
		Pos:      f.Pos(100), // line 2
		Message:  "division by zero",
		Severity: analysis.Bug,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	err := printFinding("/work/project", w, fset, finding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "Error:") {
		t.Errorf("expected 'Error:' in output, got: %s", output)
	}
	if !strings.Contains(output, "division by zero") {
		t.Errorf("expected 'division by zero' in output, got: %s", output)
	}
	if !strings.Contains(output, "foo.go") {
		t.Errorf("expected 'foo.go' in output, got: %s", output)
	}
	// Check ANSI red color code
	if !strings.Contains(output, "\033[31m") {
		t.Errorf("expected ANSI red code in output, got: %s", output)
	}
	// Check ANSI reset code
	if !strings.Contains(output, "\033[0m") {
		t.Errorf("expected ANSI reset code in output, got: %s", output)
	}
}

func TestPrintFinding_Warning(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/project/bar.go", -1, 1000)
	f.AddLine(0)

	finding := analysis.Finding{
		Pos:      f.Pos(0), // line 1
		Message:  "possible division by zero",
		Severity: analysis.Warning,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	err := printFinding("/work/project", w, fset, finding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "Warning:") {
		t.Errorf("expected 'Warning:' in output, got: %s", output)
	}
	if !strings.Contains(output, "possible division by zero") {
		t.Errorf("expected message in output, got: %s", output)
	}
	// Check ANSI yellow color code
	if !strings.Contains(output, "\033[33m") {
		t.Errorf("expected ANSI yellow code in output, got: %s", output)
	}
	if !strings.Contains(output, "\033[0m") {
		t.Errorf("expected ANSI reset code in output, got: %s", output)
	}
}

func TestPrintFinding_SafeSeverity_PrintsNothing(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/project/safe.go", -1, 1000)
	f.AddLine(0)

	finding := analysis.Finding{
		Pos:      f.Pos(0),
		Message:  "all good",
		Severity: analysis.Safe,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = printFinding("/work/project", w, fset, finding)
	w.Flush()

	if buf.Len() != 0 {
		t.Errorf("expected no output for Safe severity, got: %s", buf.String())
	}
}

func TestPrintFinding_RelativePath(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/home/user/code/myproject/pkg/foo.go", -1, 1000)
	f.AddLine(0)
	f.AddLine(50)
	f.AddLine(100)

	finding := analysis.Finding{
		Pos:      f.Pos(100), // line 3
		Message:  "division by zero",
		Severity: analysis.Bug,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	err := printFinding("/home/user/code/myproject", w, fset, finding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w.Flush()

	output := buf.String()
	// Should show relative path, not absolute.
	if strings.Contains(output, "/home/user/code/myproject/") {
		t.Errorf("expected relative path, got absolute: %s", output)
	}
	if !strings.Contains(output, "pkg/foo.go") {
		t.Errorf("expected 'pkg/foo.go' in output, got: %s", output)
	}
}

func TestPrintFinding_UnknownSeverity_PrintsNothing(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/src/unknown.go", -1, 1000)
	f.AddLine(0)

	finding := analysis.Finding{
		Pos:      f.Pos(0),
		Message:  "mystery",
		Severity: analysis.Severity(99),
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = printFinding("/work/src", w, fset, finding)
	w.Flush()

	if buf.Len() != 0 {
		t.Errorf("expected no output for unknown severity, got: %s", buf.String())
	}
}

func TestPrintFinding_LongMessage(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/src/long.go", -1, 1000)
	f.AddLine(0)

	longMsg := strings.Repeat("overflow detected ", 50)
	finding := analysis.Finding{
		Pos:      f.Pos(0),
		Message:  longMsg,
		Severity: analysis.Bug,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	err := printFinding("/work/src", w, fset, finding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w.Flush()

	if !strings.Contains(buf.String(), longMsg) {
		t.Error("long message was truncated in output")
	}
}

func TestPrintFinding_MultipleFindings_EachOnOwnLine(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f := fset.AddFile("/work/src/multi.go", -1, 1000)
	f.AddLine(0)
	f.AddLine(100)
	f.AddLine(200)

	findings := []analysis.Finding{
		{Pos: f.Pos(0), Message: "first issue", Severity: analysis.Bug},
		{Pos: f.Pos(100), Message: "second issue", Severity: analysis.Warning},
		{Pos: f.Pos(200), Message: "third issue", Severity: analysis.Bug},
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, finding := range findings {
		_ = printFinding("/work/src", w, fset, finding)
	}
	w.Flush()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

// ---------------------------------------------------------------------------
// analyzeFunction tests
// ---------------------------------------------------------------------------

func TestAnalyzeFunction_DivByZeroLiteral(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "DivByZeroLiteral")
	if fn == nil {
		t.Fatal("function DivByZeroLiteral not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != analysis.Bug {
		t.Errorf("expected Bug severity, got %v", findings[0].Severity)
	}
	if !strings.Contains(findings[0].Message, "division by zero") {
		t.Errorf("expected 'division by zero' in message, got %s", findings[0].Message)
	}
}

func TestAnalyzeFunction_DivSafe_NoFindings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "DivSafe")
	if fn == nil {
		t.Fatal("function DivSafe not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for safe function, got %d: %+v", len(findings), findings)
	}
}

func TestAnalyzeFunction_DivByParam_Warning(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "DivByParam")
	if fn == nil {
		t.Fatal("function DivByParam not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != analysis.Warning {
		t.Errorf("expected Warning severity, got %v", findings[0].Severity)
	}
	if !strings.Contains(findings[0].Message, "possible division by zero") {
		t.Errorf("expected 'possible division by zero' message, got %s", findings[0].Message)
	}
}

func TestAnalyzeFunction_DivByConstant_Safe(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "DivByConstant")
	if fn == nil {
		t.Fatal("function DivByConstant not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for constant divisor, got %d: %+v", len(findings), findings)
	}
}

func TestAnalyzeFunction_ModByZero_Bug(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "ModByZero")
	if fn == nil {
		t.Fatal("function ModByZero not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != analysis.Bug {
		t.Errorf("expected Bug severity, got %v", findings[0].Severity)
	}
}

func TestAnalyzeFunction_DivInLoop_Warning(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	fn := findFunction(p.pkgs[0], "DivInLoop")
	if fn == nil {
		t.Fatal("function DivInLoop not found")
	}

	findings := p.analyzeFunction(fn)
	if len(findings) < 1 {
		t.Fatal("expected at least 1 finding for division in loop")
	}
	hasWarningOrBug := false
	for _, f := range findings {
		if f.Severity == analysis.Warning || f.Severity == analysis.Bug {
			hasWarningOrBug = true
			break
		}
	}
	if !hasWarningOrBug {
		t.Errorf("expected Warning or Bug finding, got: %+v", findings)
	}
}

func TestAnalyzeFunction_NoDivision_NoFindings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	for _, name := range []string{"Add", "Multiply", "Constant", "LocalVar"} {
		fn := findFunction(p.pkgs[0], name)
		if fn == nil {
			t.Fatalf("function %s not found", name)
		}
		findings := p.analyzeFunction(fn)
		if len(findings) != 0 {
			t.Errorf("%s: expected 0 findings, got %d: %+v", name, len(findings), findings)
		}
	}
}

func TestAnalyzeFunction_BranchFunctions_NoFindings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	for _, name := range []string{"Abs", "Clamp", "Max", "Sign"} {
		fn := findFunction(p.pkgs[0], name)
		if fn == nil {
			t.Fatalf("function %s not found", name)
		}
		findings := p.analyzeFunction(fn)
		if len(findings) != 0 {
			t.Errorf("%s: expected 0 findings, got %d: %+v", name, len(findings), findings)
		}
	}
}

func TestAnalyzeFunction_LoopFunctions_NoFindings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	for _, name := range []string{"Sum", "Countdown", "Nested"} {
		fn := findFunction(p.pkgs[0], name)
		if fn == nil {
			t.Fatalf("function %s not found", name)
		}
		findings := p.analyzeFunction(fn)
		if len(findings) != 0 {
			t.Errorf("%s: expected 0 findings, got %d: %+v", name, len(findings), findings)
		}
	}
}

// ---------------------------------------------------------------------------
// analyzePkg tests — sort order verification
// ---------------------------------------------------------------------------

func TestAnalyzePkg_SortOrder_BugsBeforeWarnings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	findings := []analysis.Finding{}
	for _, member := range p.pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, p.analyzeFunction(fn)...)
	}

	sortFindingsForTest(findings)

	// Verify: all Bugs come before all Warnings.
	seenWarning := false
	for _, f := range findings {
		if f.Severity == analysis.Warning {
			seenWarning = true
		}
		if f.Severity == analysis.Bug && seenWarning {
			t.Error("found Bug after Warning — sort order is wrong")
		}
	}
}

func TestAnalyzePkg_SortOrder_SourceOrderWithinSeverity(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	findings := []analysis.Finding{}
	for _, member := range p.pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, p.analyzeFunction(fn)...)
	}

	sortFindingsForTest(findings)

	// Verify: within each severity, positions are ascending.
	var lastBugPos, lastWarnPos token.Pos
	for _, f := range findings {
		switch f.Severity {
		case analysis.Bug:
			if f.Pos < lastBugPos {
				t.Errorf("bug findings not in source order: %v came after %v", f.Pos, lastBugPos)
			}
			lastBugPos = f.Pos
		case analysis.Warning:
			if f.Pos < lastWarnPos {
				t.Errorf("warning findings not in source order: %v came after %v", f.Pos, lastWarnPos)
			}
			lastWarnPos = f.Pos
		}
	}
}

func TestAnalyzePkg_CollectsAllFindings(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")

	findings := []analysis.Finding{}
	for _, member := range p.pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, p.analyzeFunction(fn)...)
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, finding := range findings {
		_ = printFinding("/tmp", w, p.prog.Fset, finding)
	}
	w.Flush()

	output := buf.String()
	if !strings.Contains(output, "Error:") {
		t.Error("expected at least one Error in output")
	}
	if !strings.Contains(output, "Warning:") {
		t.Error("expected at least one Warning in output")
	}
}

// sortFindingsForTest replicates the sort logic from analyzePkg.
func sortFindingsForTest(findings []analysis.Finding) {
	for i := 1; i < len(findings); i++ {
		for j := i; j > 0; j-- {
			swap := false
			if findings[j].Severity > findings[j-1].Severity {
				swap = true
			} else if findings[j].Severity == findings[j-1].Severity && findings[j].Pos < findings[j-1].Pos {
				swap = true
			}
			if swap {
				findings[j], findings[j-1] = findings[j-1], findings[j]
			} else {
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Prove integration tests
// ---------------------------------------------------------------------------

func TestProve_ValidTestdata(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/testdata")
	err := p.Prove()
	if err != nil {
		t.Fatalf("Prove returned error: %v", err)
	}
}

func TestProve_NonexistentPath(t *testing.T) {
	t.Parallel()
	_, err := NewProver("./nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestProve_SafePackage_NoError(t *testing.T) {
	t.Parallel()
	p := newTestProver(t, "../../pkg/loader")
	err := p.Prove()
	if err != nil {
		t.Fatalf("Prove on safe package returned error: %v", err)
	}
}
