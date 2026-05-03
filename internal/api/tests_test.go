package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/tests"
)

type fakeTestPlugin struct {
	testFiles []plugin.File
	tests     map[string][]plugin.TestFunc
}

func (f *fakeTestPlugin) Name() string { return "go" }
func (f *fakeTestPlugin) Initialize(context.Context, plugin.PluginConfig) error {
	return nil
}
func (f *fakeTestPlugin) Shutdown(context.Context) error { return nil }
func (f *fakeTestPlugin) DiscoverTestFiles(string) ([]plugin.File, error) {
	return f.testFiles, nil
}
func (f *fakeTestPlugin) DiscoverTests(file plugin.File) ([]plugin.TestFunc, error) {
	return f.tests[file.Path], nil
}
func (f *fakeTestPlugin) DiscoverSymbols(string) ([]plugin.Symbol, error) {
	return nil, nil
}
func (f *fakeTestPlugin) ResolveSymbol(plugin.Position) (plugin.Symbol, error) {
	return plugin.Symbol{}, nil
}
func (f *fakeTestPlugin) CallGraph(plugin.Symbol) (plugin.Graph, error) {
	return plugin.Graph{}, nil
}
func (f *fakeTestPlugin) RunTests(context.Context, plugin.TestSelection, plugin.RunOptions) (plugin.RunResult, error) {
	return plugin.RunResult{}, nil
}
func (f *fakeTestPlugin) Coverage(context.Context, plugin.TestSelection, plugin.RunOptions) (plugin.CoverageReport, error) {
	return plugin.CoverageReport{}, nil
}
func (f *fakeTestPlugin) GoldenLayout() plugin.GoldenConvention { return plugin.GoldenConvention{} }
func (f *fakeTestPlugin) Mutators() []plugin.Mutator            { return nil }
func (f *fakeTestPlugin) Generators() []plugin.Generator        { return nil }

func TestTestsListMarksTestsWithGoldenData(t *testing.T) {
	root := t.TempDir()
	goldenDir := filepath.Join(root, "tests", "golden", "calc", "TestAdd")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goldenDir, "positive.in.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("write golden input: %v", err)
	}

	add := plugin.TestFunc{
		ID:      "go:calc/add_test.go:calc.TestAdd",
		Name:    "TestAdd",
		File:    "calc/add_test.go",
		Line:    1,
		Column:  1,
		Package: "calc",
	}
	sub := plugin.TestFunc{
		ID:      "go:calc/sub_test.go:calc.TestSub",
		Name:    "TestSub",
		File:    "calc/sub_test.go",
		Line:    1,
		Column:  1,
		Package: "calc",
	}

	registry := plugin.NewRegistry()
	registry.Register(&fakeTestPlugin{
		testFiles: []plugin.File{{Path: "calc/add_test.go", Language: "go", IsTest: true}},
		tests: map[string][]plugin.TestFunc{
			"calc/add_test.go": {add, sub},
		},
	})

	handler := NewTestsHandler(tests.NewDiscovery(registry))
	handler.SetProject(&project.Project{
		ID:         "default",
		Path:       root,
		Name:       "sample",
		GoldenRoot: "tests/golden",
	})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/projects/default/tests", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var got []struct {
		ID        string `json:"id"`
		HasGolden bool   `json:"hasGolden,omitempty"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	byID := make(map[string]bool)
	for _, item := range got {
		byID[item.ID] = item.HasGolden
	}
	if !byID[add.ID] {
		t.Fatalf("expected %s to be marked with golden data", add.ID)
	}
	if byID[sub.ID] {
		t.Fatalf("expected %s not to be marked with golden data", sub.ID)
	}
}
