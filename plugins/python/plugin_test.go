package pythonplugin

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/review-server/internal/plugin"
)

func sampleRoot(t *testing.T) string {
	root, err := filepath.Abs("../../testdata/sample-python")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}

func TestPluginName(t *testing.T) {
	p := New()
	if p.Name() != "python" {
		t.Fatalf("expected name python, got %s", p.Name())
	}
}

func TestDiscoverTestFiles(t *testing.T) {
	p := New()
	files, err := p.DiscoverTestFiles(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 test files, got %d", len(files))
	}
}

func TestDiscoverTests(t *testing.T) {
	p := &Plugin{root: sampleRoot(t)}
	files, err := p.DiscoverTestFiles(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	var tests []plugin.TestFunc
	for _, file := range files {
		found, err := p.DiscoverTests(file)
		if err != nil {
			t.Fatalf("DiscoverTests: %v", err)
		}
		tests = append(tests, found...)
	}
	var hasAdd, hasSub, hasMethod bool
	for _, test := range tests {
		switch test.Name {
		case "test_add":
			hasAdd = true
			if len(test.SubCases) != 2 {
				t.Fatalf("expected two parametrized add cases, got %d", len(test.SubCases))
			}
		case "test_sub":
			hasSub = true
		case "TestCalculator.test_class_add":
			hasMethod = true
		}
	}
	if !hasAdd || !hasSub || !hasMethod {
		t.Fatalf("missing expected tests: add=%v sub=%v method=%v all=%+v", hasAdd, hasSub, hasMethod, tests)
	}
}

func TestDiscoverSymbols(t *testing.T) {
	p := &Plugin{root: sampleRoot(t)}
	syms, err := p.DiscoverSymbols(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSymbols: %v", err)
	}
	var hasAdd, hasMul bool
	for _, sym := range syms {
		if sym.QualifiedName == "calc.add.add" {
			hasAdd = true
		}
		if sym.QualifiedName == "calc.mul.mul" {
			hasMul = true
		}
	}
	if !hasAdd || !hasMul {
		t.Fatalf("missing expected symbols: add=%v mul=%v symbols=%+v", hasAdd, hasMul, syms)
	}
}

func TestRunTestsAndCoverage(t *testing.T) {
	if _, err := findPython(); err != nil {
		t.Skip(err)
	}
	if !toolExists("pytest") {
		t.Skip("pytest not installed")
	}
	if !toolExists("coverage") {
		t.Skip("coverage not installed")
	}

	p := &Plugin{root: sampleRoot(t)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := p.RunTests(ctx, plugin.TestSelection{}, plugin.RunOptions{WorkingDir: sampleRoot(t)})
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected pytest to pass, output:\n%s", result.Output)
	}

	report, err := p.Coverage(ctx, plugin.TestSelection{}, plugin.RunOptions{WorkingDir: sampleRoot(t)})
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if report.Scope != "run" {
		t.Fatalf("expected scope run, got %s", report.Scope)
	}
	if report.Percentage <= 0 {
		t.Fatalf("expected positive coverage, got %f", report.Percentage)
	}
	var hasMul bool
	for _, id := range report.Uncovered {
		if strings.Contains(id, "mul") {
			hasMul = true
		}
	}
	if !hasMul {
		t.Logf("uncovered: %v", report.Uncovered)
		t.Log("warning: mul not found in uncovered list")
	}
}

func TestGoldenLayout(t *testing.T) {
	layout := New().GoldenLayout()
	if layout.Root != "tests/golden" {
		t.Fatalf("unexpected root %s", layout.Root)
	}
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
