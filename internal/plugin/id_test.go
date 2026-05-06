package plugin

import "testing"

func TestNewAndParseTestID(t *testing.T) {
	id := NewTestID("go", "calc/add_test.go", "calc.TestAdd")

	pluginName, path, qualified, casePath, err := ParseTestID(id)
	if err != nil {
		t.Fatalf("ParseTestID(%q) unexpected error: %v", id, err)
	}
	if pluginName != "go" || path != "calc/add_test.go" || qualified != "calc.TestAdd" || casePath != "" {
		t.Fatalf("ParseTestID(%q) = (%q, %q, %q, %q)", id, pluginName, path, qualified, casePath)
	}
}

func TestNewAndParseCaseIDEscapesSegments(t *testing.T) {
	id := NewTestCaseID("go", "calc/add_test.go", "calc.TestAdd", "a/b:c%")

	parsed, err := ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q) unexpected error: %v", id, err)
	}
	if parsed.Plugin != "go" || parsed.Path != "calc/add_test.go" || parsed.QualifiedName != "calc.TestAdd" || parsed.CasePath != "a/b%3Ac%25" {
		t.Fatalf("ParseID(%q) = %#v", id, parsed)
	}
	if got := UnescapeSegment(parsed.CasePath); got != "a/b:c%" {
		t.Fatalf("UnescapeSegment(%q) = %q", parsed.CasePath, got)
	}
}

func TestAppendTestCaseID(t *testing.T) {
	id := AppendTestCaseID("go:calc/add_test.go:calc.TestAdd", "a/b:c%")
	if id != "go:calc/add_test.go:calc.TestAdd:a/b%3Ac%25" {
		t.Fatalf("AppendTestCaseID() = %q", id)
	}
}

func TestNewAndParseSymbolID(t *testing.T) {
	id := NewSymbolID("javascript", "src/calc.ts", "src.add", 12, 3)

	parsed, err := ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q) unexpected error: %v", id, err)
	}
	if parsed.Plugin != "javascript" || parsed.Path != "src/calc.ts" || parsed.QualifiedName != "src.add" || parsed.Line != 12 || parsed.Column != 3 {
		t.Fatalf("ParseID(%q) = %#v", id, parsed)
	}
}
