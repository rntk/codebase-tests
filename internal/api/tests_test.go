package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

type goldenFileEntry struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type listGoldenInput struct {
	Project     project.Project           `json:"project"`
	TestFiles   []plugin.File             `json:"testFiles"`
	Tests       map[string][]plugin.TestFunc `json:"tests"`
	GoldenFiles []goldenFileEntry         `json:"goldenFiles"`
}

func TestTestsListMarksTestsWithGoldenData(t *testing.T) {
	goldenDir := filepath.Join("..", "..", "tests", "golden", "api", "TestTestsListMarksTestsWithGoldenData")
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".in.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".in.json")
		t.Run(caseName, func(t *testing.T) {
			var in listGoldenInput
			decodeJSON(t, filepath.Join(goldenDir, caseName+".in.json"), &in)

			var want any
			decodeJSON(t, filepath.Join(goldenDir, caseName+".out.json"), &want)

			root := t.TempDir()
			for _, f := range in.GoldenFiles {
				p := filepath.Join(root, filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(f.Content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			registry := plugin.NewRegistry()
			registry.Register(&fakeTestPlugin{
				testFiles: in.TestFiles,
				tests:     in.Tests,
			})

			handler := NewTestsHandler(tests.NewDiscovery(registry))
			handler.SetProject(&project.Project{
				ID:         in.Project.ID,
				Path:       root,
				Name:       in.Project.Name,
				GoldenRoot: in.Project.GoldenRoot,
			})
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			req := httptest.NewRequest(http.MethodGet, "/projects/"+in.Project.ID+"/tests", nil)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}

			var gotJSON any
			roundTrip(t, res.Body.Bytes(), &gotJSON)
			if !reflect.DeepEqual(gotJSON, want) {
				gotBytes, _ := json.MarshalIndent(gotJSON, "", "  ")
				wantBytes, _ := json.MarshalIndent(want, "", "  ")
				t.Fatalf("response =\n%s\nwant\n%s", gotBytes, wantBytes)
			}
		})
	}
}

func decodeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func roundTrip(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
