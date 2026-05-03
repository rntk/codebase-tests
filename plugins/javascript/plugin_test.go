package javascriptplugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rntk/codebase-tests/internal/plugin"
)

func sampleRoot(t *testing.T) string {
	root, err := filepath.Abs("../../testdata/sample-javascript")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}

func TestPluginName(t *testing.T) {
	p := New()
	if p.Name() != "javascript" {
		t.Fatalf("expected name javascript, got %s", p.Name())
	}
}

func TestDiscoverTestFiles(t *testing.T) {
	p := New()
	files, err := p.DiscoverTestFiles(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 test file, got %d", len(files))
	}
}

func TestDiscoverTests(t *testing.T) {
	p := &Plugin{root: sampleRoot(t)}
	files, err := p.DiscoverTestFiles(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	tests, err := p.DiscoverTests(files[0])
	if err != nil {
		t.Fatalf("DiscoverTests: %v", err)
	}
	var hasAdd, hasSub bool
	for _, test := range tests {
		if test.Name == "calculator > adds values" {
			hasAdd = true
		}
		if test.Name == "calculator > subtracts values" {
			hasSub = true
		}
	}
	if !hasAdd || !hasSub {
		t.Fatalf("missing expected tests: add=%v sub=%v tests=%+v", hasAdd, hasSub, tests)
	}
}

func TestDiscoverSymbols(t *testing.T) {
	p := &Plugin{root: sampleRoot(t)}
	syms, err := p.DiscoverSymbols(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSymbols: %v", err)
	}
	var hasAdd, hasCalculator bool
	for _, sym := range syms {
		if sym.QualifiedName == "src.calc.add" {
			hasAdd = true
		}
		if sym.QualifiedName == "src.calc.Calculator" && sym.Kind == "class" {
			hasCalculator = true
		}
	}
	if !hasAdd || !hasCalculator {
		t.Fatalf("missing expected symbols: add=%v calculator=%v symbols=%+v", hasAdd, hasCalculator, syms)
	}
}

func TestRunTests(t *testing.T) {
	if !toolExists("node") || !toolExists("npm") {
		t.Skip("node/npm not installed")
	}
	if _, err := os.Stat(filepath.Join(sampleRoot(t), "node_modules", ".bin", "vitest")); err != nil {
		t.Skip("sample JavaScript dependencies not installed")
	}
	p := &Plugin{root: sampleRoot(t)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := p.RunTests(ctx, plugin.TestSelection{}, plugin.RunOptions{WorkingDir: sampleRoot(t)})
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected npm test to pass, output:\n%s", result.Output)
	}
}

func TestGoldenLayout(t *testing.T) {
	layout := New().GoldenLayout()
	if layout.Root != "tests/golden" {
		t.Fatalf("unexpected root %s", layout.Root)
	}
}

func TestParseCoverageFinal(t *testing.T) {
	report, err := parseCoverageFinal(filepath.Join(sampleRoot(t), "coverage", "coverage-final.json"), sampleRoot(t))
	if err != nil {
		t.Fatalf("parseCoverageFinal: %v", err)
	}
	if report.Percentage <= 0 {
		t.Fatalf("expected positive coverage, got %f", report.Percentage)
	}
	var hasCalc bool
	for _, file := range report.Files {
		if strings.HasSuffix(file.Path, "src/calc.ts") {
			hasCalc = true
		}
	}
	if !hasCalc {
		t.Fatalf("expected calc coverage file, got %+v", report.Files)
	}
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
