package typescriptplugin

import (
	"path/filepath"
	"strings"
	"testing"
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
	if p.Name() != "typescript" {
		t.Fatalf("expected name typescript, got %s", p.Name())
	}
}

func TestDiscoverUsesTypeScriptIDs(t *testing.T) {
	p := New()
	files, err := p.DiscoverTestFiles(sampleRoot(t))
	if err != nil {
		t.Fatalf("DiscoverTestFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 test file, got %d", len(files))
	}
	tests, err := p.DiscoverTests(files[0])
	if err != nil {
		t.Fatalf("DiscoverTests: %v", err)
	}
	if len(tests) == 0 {
		t.Fatal("expected discovered tests")
	}
	for _, test := range tests {
		if !strings.HasPrefix(test.ID, "typescript:") {
			t.Fatalf("expected typescript test ID, got %s", test.ID)
		}
	}
}
