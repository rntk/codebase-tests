package golden

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rntk/codebase-tests/internal/plugin"
)

func TestParseTestID(t *testing.T) {
	tests := []struct {
		input      string
		wantPlugin string
		wantPath   string
		wantQual   string
		wantCase   string
		wantErr    bool
	}{
		{"go:calc/add_test.go:calc.TestAdd", "go", "calc/add_test.go", "calc.TestAdd", "", false},
		{"go:calc/add_test.go:calc.TestAdd:positive", "go", "calc/add_test.go", "calc.TestAdd", "positive", false},
		{"go:path/to/file.go:pkg.Func:case%3Aname", "go", "path/to/file.go", "pkg.Func", "case%3Aname", false},
		{"invalid", "", "", "", "", true},
		{"a:b:c:d:e", "", "", "", "", true},
	}

	for _, tt := range tests {
		p, path, qual, c, err := ParseTestID(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseTestID(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTestID(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if p != tt.wantPlugin || path != tt.wantPath || qual != tt.wantQual || c != tt.wantCase {
			t.Errorf("ParseTestID(%q) = (%q, %q, %q, %q), want (%q, %q, %q, %q)",
				tt.input, p, path, qual, c, tt.wantPlugin, tt.wantPath, tt.wantQual, tt.wantCase)
		}
	}
}

func TestEscapeUnescapeSegment(t *testing.T) {
	tests := []struct {
		plain   string
		escaped string
	}{
		{"hello", "hello"},
		{"a/b", "a/b"},
		{"a:b", "a%3Ab"},
		{"a%b", "a%25b"},
		{"a/b:c", "a/b%3Ac"},
		{"a%3Ab", "a%253Ab"},
	}

	for _, tt := range tests {
		got := EscapeSegment(tt.plain)
		if got != tt.escaped {
			t.Errorf("EscapeSegment(%q) = %q, want %q", tt.plain, got, tt.escaped)
		}
		got = UnescapeSegment(tt.escaped)
		if got != tt.plain {
			t.Errorf("UnescapeSegment(%q) = %q, want %q", tt.escaped, got, tt.plain)
		}
	}
}

func TestResolveCasesSampleGo(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "sample-go")
	conv := plugin.GoldenConvention{
		Root:        "tests/golden",
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}

	cases, err := ResolveCases(root, "go:calc/add_test.go:calc.TestAdd", conv)
	if err != nil {
		t.Fatalf("ResolveCases error: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}

	if cases[0].Name != "negative" {
		t.Errorf("expected first case negative, got %s", cases[0].Name)
	}
	if cases[1].Name != "positive" {
		t.Errorf("expected second case positive, got %s", cases[1].Name)
	}

	for _, c := range cases {
		if !c.InExists {
			t.Errorf("case %s: expected in to exist", c.Name)
		}
		if !c.OutExists {
			t.Errorf("case %s: expected out to exist", c.Name)
		}
		if c.Meta != nil {
			t.Errorf("case %s: expected no meta", c.Name)
		}
	}
}

func TestResolveCasesMissingSide(t *testing.T) {
	dir := t.TempDir()
	conv := plugin.GoldenConvention{
		Root:        "golden",
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}

	funcDir := filepath.Join(dir, "golden", "pkg", "TestFunc")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "onlyIn.in.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "onlyOut.out.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	cases, err := ResolveCases(dir, "go:pkg/foo.go:pkg.TestFunc", conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}

	for _, c := range cases {
		switch c.Name {
		case "onlyIn":
			if !c.InExists || c.OutExists {
				t.Errorf("onlyIn: inExists=%v outExists=%v", c.InExists, c.OutExists)
			}
		case "onlyOut":
			if c.InExists || !c.OutExists {
				t.Errorf("onlyOut: inExists=%v outExists=%v", c.InExists, c.OutExists)
			}
		default:
			t.Errorf("unexpected case %s", c.Name)
		}
	}
}

func TestResolveCasesEscapedNames(t *testing.T) {
	dir := t.TempDir()
	conv := plugin.GoldenConvention{
		Root:        "golden",
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}

	funcDir := filepath.Join(dir, "golden", "pkg", "Test%3AFunc")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "case%2Fname.in.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "case%2Fname.out.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	cases, err := ResolveCases(dir, "go:pkg/foo.go:pkg.Test%3AFunc", conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].Name != "case/name" {
		t.Errorf("expected case/name, got %s", cases[0].Name)
	}
	if !cases[0].InExists || !cases[0].OutExists {
		t.Errorf("expected both sides to exist")
	}
}

func TestResolveCasesDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	conv := plugin.GoldenConvention{
		Root:        "golden",
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}

	funcDir := filepath.Join(dir, "golden", "pkg", "TestFunc")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "dup.in.json"), []byte(`1`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "dup.out.json"), []byte(`1`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "dup_0.in.json"), []byte(`2`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(funcDir, "dup_0.out.json"), []byte(`2`), 0644); err != nil {
		t.Fatal(err)
	}

	cases, err := ResolveCases(dir, "go:pkg/foo.go:pkg.TestFunc", conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}

	names := map[string]bool{}
	for _, c := range cases {
		names[c.Name] = true
	}
	if !names["dup"] || !names["dup_0"] {
		t.Errorf("expected dup and dup_0, got %v", names)
	}
}

func TestResolveCasesMeta(t *testing.T) {
	type fileInput struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	type input struct {
		Convention plugin.GoldenConvention `json:"convention"`
		TestID     string                  `json:"testId"`
		Dirs       []string                `json:"dirs"`
		Files      []fileInput             `json:"files"`
	}

	goldenDir := filepath.Join("..", "..", "tests", "golden", "golden", "TestResolveCasesMeta")
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
			var in input
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".in.json"), &in)

			var want any
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".out.json"), &want)

			dir := t.TempDir()
			for _, d := range in.Dirs {
				if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range in.Files {
				p := filepath.Join(dir, filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(f.Content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cases, err := ResolveCases(dir, in.TestID, in.Convention)
			if err != nil {
				t.Fatalf("ResolveCases error: %v", err)
			}

			prefix := dir + string(filepath.Separator)
			for i := range cases {
				cases[i].InPath = strings.TrimPrefix(cases[i].InPath, prefix)
				cases[i].OutPath = strings.TrimPrefix(cases[i].OutPath, prefix)
				cases[i].MetaPath = strings.TrimPrefix(cases[i].MetaPath, prefix)
			}

			var gotJSON any
			roundTripJSON(t, cases, &gotJSON)
			if !reflect.DeepEqual(gotJSON, want) {
				t.Errorf("ResolveCases =\n%v\nwant\n%v", gotJSON, want)
			}
		})
	}
}

func TestResolveCasesSubfolder(t *testing.T) {
	type fileInput struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	type input struct {
		Convention plugin.GoldenConvention `json:"convention"`
		TestID     string                  `json:"testId"`
		Dirs       []string                `json:"dirs"`
		Files      []fileInput             `json:"files"`
	}

	goldenDir := filepath.Join("..", "..", "tests", "golden", "golden", "TestResolveCasesSubfolder")
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
			var in input
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".in.json"), &in)

			var want any
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".out.json"), &want)

			dir := t.TempDir()
			for _, d := range in.Dirs {
				if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range in.Files {
				p := filepath.Join(dir, filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(f.Content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cases, err := ResolveCases(dir, in.TestID, in.Convention)
			if err != nil {
				t.Fatalf("ResolveCases error: %v", err)
			}

			prefix := dir + string(filepath.Separator)
			for i := range cases {
				cases[i].InPath = strings.TrimPrefix(cases[i].InPath, prefix)
				cases[i].OutPath = strings.TrimPrefix(cases[i].OutPath, prefix)
				cases[i].MetaPath = strings.TrimPrefix(cases[i].MetaPath, prefix)
			}

			var gotJSON any
			roundTripJSON(t, cases, &gotJSON)
			if !reflect.DeepEqual(gotJSON, want) {
				t.Errorf("ResolveCases =\n%v\nwant\n%v", gotJSON, want)
			}
		})
	}
}
