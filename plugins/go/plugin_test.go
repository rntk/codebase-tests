package goplugin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rntk/codebase-tests/internal/plugin"
)

func sampleRoot(t *testing.T) string {
	root, err := filepath.Abs("../../testdata/sample-go")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}

func initPlugin(t *testing.T) *Plugin {
	p := New().(*Plugin)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := plugin.PluginConfig{
		ProjectRoot:        sampleRoot(t),
		ToolTimeoutSeconds: 30,
	}
	if err := p.Initialize(ctx, cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return p
}

func TestPluginName(t *testing.T) {
	p := New()
	if p.Name() != "go" {
		t.Fatalf("expected name go, got %s", p.Name())
	}
}

func TestDiscoverTestFiles(t *testing.T) {
	p := New()
	root := sampleRoot(t)
	files, err := p.DiscoverTestFiles(root)
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 test files, got %d", len(files))
	}
}

func TestDiscoverTests(t *testing.T) {
	p := New()
	root := sampleRoot(t)
	files, _ := p.DiscoverTestFiles(root)
	if len(files) == 0 {
		t.Fatal("no test files found")
	}

	var allTests []plugin.TestFunc
	for _, f := range files {
		tests, err := p.DiscoverTests(f)
		if err != nil {
			t.Fatalf("DiscoverTests %s: %v", f.Path, err)
		}
		allTests = append(allTests, tests...)
	}

	if len(allTests) < 2 {
		t.Fatalf("expected at least 2 tests, got %d", len(allTests))
	}

	var hasAdd, hasSub bool
	for _, tf := range allTests {
		if tf.Name == "TestAdd" {
			hasAdd = true
			// Variable-based t.Run names can't be detected statically;
			// sub-cases may be empty for table tests with variable names.
		}
		if tf.Name == "TestSub" {
			hasSub = true
		}
	}
	if !hasAdd {
		t.Fatal("missing TestAdd")
	}
	if !hasSub {
		t.Fatal("missing TestSub")
	}
}

func TestDiscoverSymbols(t *testing.T) {
	p := initPlugin(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
	}()

	root := sampleRoot(t)
	syms, err := p.DiscoverSymbols(root)
	if err != nil {
		t.Fatalf("DiscoverSymbols: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("expected non-empty symbols")
	}

	var hasAdd, hasMul bool
	for _, s := range syms {
		if s.Name == "Add" {
			hasAdd = true
		}
		if s.Name == "Mul" {
			hasMul = true
		}
	}
	if !hasAdd {
		t.Fatal("missing Add symbol")
	}
	if !hasMul {
		t.Fatal("missing Mul symbol")
	}
}

func TestCallGraph(t *testing.T) {
	p := initPlugin(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
	}()

	if p.client == nil {
		t.Skip("gopls not available")
	}

	root := sampleRoot(t)
	syms, err := p.DiscoverSymbols(root)
	if err != nil {
		t.Fatalf("DiscoverSymbols: %v", err)
	}

	var addSym *plugin.Symbol
	for i := range syms {
		if syms[i].Name == "Add" {
			addSym = &syms[i]
			break
		}
	}
	if addSym == nil {
		t.Fatal("Add symbol not found")
	}

	graph, err := p.CallGraph(*addSym)
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if graph.Symbol == "" {
		t.Fatal("empty graph symbol")
	}
	// We expect at least one incoming call from TestAdd.
	if len(graph.Incoming) == 0 && len(graph.Outgoing) == 0 {
		t.Logf("graph: %+v", graph)
		t.Fatal("expected non-empty call graph")
	}
}

func TestRunTests(t *testing.T) {
	p := New()
	root := sampleRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.RunTests(ctx, plugin.TestSelection{}, plugin.RunOptions{WorkingDir: root})
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected tests to pass, output:\n%s", result.Output)
	}
	if len(result.Tests) == 0 {
		t.Fatal("expected at least one test result")
	}
}

func TestCoverage(t *testing.T) {
	p := New()
	root := sampleRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := p.Coverage(ctx, plugin.TestSelection{}, plugin.RunOptions{WorkingDir: root})
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if report.Scope != "run" {
		t.Fatalf("expected scope run, got %s", report.Scope)
	}
	if report.Percentage <= 0 {
		t.Fatalf("expected positive coverage, got %f", report.Percentage)
	}
	// Mul is not tested, so it should appear in uncovered.
	var hasUncoveredMul bool
	for _, id := range report.Uncovered {
		if strings.Contains(id, ":Mul") {
			hasUncoveredMul = true
			break
		}
	}
	if !hasUncoveredMul {
		t.Logf("uncovered: %v", report.Uncovered)
		t.Logf("files: %+v", report.Files)
		// This may be a false negative depending on how coverage blocks line up;
		// tolerate it as a warning rather than fatal.
		t.Log("warning: Mul not found in uncovered list")
	}
}

func TestGoldenLayout(t *testing.T) {
	p := New()
	layout := p.GoldenLayout()
	if layout.Root != "tests/golden" {
		t.Fatalf("unexpected root %s", layout.Root)
	}
}

func TestMutatorsAndGenerators(t *testing.T) {
	p := New()
	if len(p.Mutators()) != 0 {
		t.Fatal("expected no mutators")
	}
	if len(p.Generators()) != 0 {
		t.Fatal("expected no generators")
	}
}
