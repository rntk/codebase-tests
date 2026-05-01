package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakePlugin struct {
	name        string
	initErr     error
	shutdownErr error
	initialized bool
	shutdown    bool
	cfg         PluginConfig
}

func (f *fakePlugin) Name() string { return f.name }
func (f *fakePlugin) Initialize(ctx context.Context, cfg PluginConfig) error {
	f.initialized = true
	f.cfg = cfg
	return f.initErr
}
func (f *fakePlugin) Shutdown(ctx context.Context) error {
	f.shutdown = true
	return f.shutdownErr
}
func (f *fakePlugin) DiscoverTestFiles(root string) ([]File, error) { return nil, nil }
func (f *fakePlugin) DiscoverTests(file File) ([]TestFunc, error)   { return nil, nil }
func (f *fakePlugin) DiscoverSymbols(root string) ([]Symbol, error) { return nil, nil }
func (f *fakePlugin) ResolveSymbol(pos Position) (Symbol, error)    { return Symbol{}, nil }
func (f *fakePlugin) CallGraph(sym Symbol) (Graph, error)           { return Graph{}, nil }
func (f *fakePlugin) RunTests(ctx context.Context, sel TestSelection, opts RunOptions) (RunResult, error) {
	return RunResult{}, nil
}
func (f *fakePlugin) Coverage(ctx context.Context, sel TestSelection, opts RunOptions) (CoverageReport, error) {
	return CoverageReport{}, nil
}
func (f *fakePlugin) GoldenLayout() GoldenConvention { return GoldenConvention{} }
func (f *fakePlugin) Mutators() []Mutator            { return nil }
func (f *fakePlugin) Generators() []Generator        { return nil }

func TestRegistryRegisterAndFor(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go"}
	r.Register(fp)

	got, err := r.For("go")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.Name() != "go" {
		t.Errorf("name = %q, want %q", got.Name(), "go")
	}

	_, err = r.For("python")
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakePlugin{name: "go"})
	r.Register(&fakePlugin{name: "python"})

	all := r.All()
	if len(all) != 2 {
		t.Errorf("len = %d, want 2", len(all))
	}
}

func TestRegistryInitialize(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go"}
	r.Register(fp)

	ctx := context.Background()
	if err := r.Initialize(ctx, PluginConfig{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !fp.initialized {
		t.Error("expected plugin to be initialized")
	}
}

func TestRegistryInitializeError(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go", initErr: errors.New("boom")}
	r.Register(fp)

	ctx := context.Background()
	err := r.Initialize(ctx, PluginConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegistryInitializeConfigured(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go"}
	r.Register(fp)

	err := r.InitializeConfigured(context.Background(), PluginConfig{
		ProjectRoot:        "/project",
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 30,
	}, []PluginConfig{{
		Name:               "go",
		Language:           "go",
		LSPServerAddress:   "gopls -remote=auto",
		ToolTimeoutSeconds: 10,
	}})
	if err != nil {
		t.Fatalf("InitializeConfigured: %v", err)
	}
	if !fp.initialized {
		t.Fatal("expected plugin to be initialized")
	}
	if fp.cfg.Language != "go" {
		t.Errorf("language = %q, want go", fp.cfg.Language)
	}
	if fp.cfg.LSPServerAddress != "gopls -remote=auto" {
		t.Errorf("lsp = %q, want configured address", fp.cfg.LSPServerAddress)
	}
	if fp.cfg.ProjectRoot != "/project" {
		t.Errorf("project root = %q, want base root", fp.cfg.ProjectRoot)
	}
	if fp.cfg.ToolTimeoutSeconds != 10 {
		t.Errorf("timeout = %d, want plugin override", fp.cfg.ToolTimeoutSeconds)
	}
}

func TestRegistryShutdown(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go"}
	r.Register(fp)

	ctx := context.Background()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !fp.shutdown {
		t.Error("expected plugin to be shutdown")
	}
}

func TestRegistryShutdownError(t *testing.T) {
	r := NewRegistry()
	fp := &fakePlugin{name: "go", shutdownErr: errors.New("boom")}
	r.Register(fp)

	ctx := context.Background()
	err := r.Shutdown(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadyCheck(t *testing.T) {
	r := NewRegistry()
	err := r.ReadyCheck()
	// We cannot assume all binaries exist in the test environment.
	// Verify that when it fails, the error mentions the missing binaries.
	if err != nil {
		if !strings.Contains(err.Error(), "missing required binaries") {
			t.Errorf("unexpected error format: %v", err)
		}
	}
}
