package plugin

import (
	"context"
	"errors"
	"fmt"
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
	depsErr     error
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
func (f *fakePlugin) CheckDependencies() error       { return f.depsErr }

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
	// No plugins registered -> no dependencies to check.
	if err := r.ReadyCheck(); err != nil {
		t.Fatalf("expected no error with empty registry, got %v", err)
	}

	// Register a plugin without dependency checker -> still no error.
	r.Register(&fakePlugin{name: "noop"})
	if err := r.ReadyCheck(); err != nil {
		t.Fatalf("expected no error without DependencyChecker, got %v", err)
	}

	// Register a plugin that reports missing dependencies.
	r.Register(&fakePlugin{name: "bad", depsErr: fmt.Errorf("missing binary: xyz")})
	err := r.ReadyCheck()
	if err == nil {
		t.Fatal("expected error for plugin with missing dependencies")
	}
	if !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "xyz") {
		t.Errorf("unexpected error format: %v", err)
	}
}

func TestFactoryRegistry(t *testing.T) {
	// Clear any previously registered factories for a clean test.
	factoriesMu.Lock()
	old := factories
	factories = make(map[string]Factory)
	factoriesMu.Unlock()
	defer func() {
		factoriesMu.Lock()
		factories = old
		factoriesMu.Unlock()
	}()

	RegisterFactory("fake", func() Plugin { return &fakePlugin{name: "fake"} })

	f, ok := GetFactory("fake")
	if !ok {
		t.Fatal("expected factory to be registered")
	}
	p := f()
	if p.Name() != "fake" {
		t.Errorf("name = %q, want fake", p.Name())
	}

	names := ListFactories()
	if len(names) != 1 || names[0] != "fake" {
		t.Errorf("factories = %v, want [fake]", names)
	}

	_, ok = GetFactory("missing")
	if ok {
		t.Error("expected missing factory to not exist")
	}
}
