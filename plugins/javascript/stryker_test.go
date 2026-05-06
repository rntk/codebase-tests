package javascriptplugin

import "testing"

func TestParseStrykerReport(t *testing.T) {
	data := []byte(`{
		"schemaVersion": "1.0",
		"files": {
			"src/foo.ts": {
				"language": "typescript",
				"mutants": [
					{ "id": "1", "mutatorName": "ConditionalExpression", "status": "Killed", "location": {"start":{"line":5,"column":10}}, "replacement": "false" },
					{ "id": "2", "mutatorName": "BooleanLiteral", "status": "Survived", "location": {"start":{"line":7,"column":2}} },
					{ "id": "3", "mutatorName": "ArithmeticOperator", "status": "NoCoverage", "location": {"start":{"line":9,"column":4}} }
				]
			}
		}
	}`)

	rep, err := parseStrykerReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Tool != "stryker" {
		t.Errorf("tool=%q", rep.Tool)
	}
	if rep.Total != 3 || rep.Killed != 1 || rep.Survived != 1 || rep.NoCoverage != 1 {
		t.Errorf("counts: %+v", rep)
	}
	if rep.Score != 0.5 {
		t.Errorf("score=%v want 0.5", rep.Score)
	}
	if len(rep.Mutants) != 3 {
		t.Fatalf("mutants=%d", len(rep.Mutants))
	}
	// Find the killed mutant and verify location.
	var killed *struct{ line, col int }
	for _, m := range rep.Mutants {
		if m.Status == "killed" {
			killed = &struct{ line, col int }{m.Line, m.Column}
		}
	}
	if killed == nil || killed.line != 5 {
		t.Errorf("killed mutant location: %+v", killed)
	}
}

func TestNormalizeStrykerStatus(t *testing.T) {
	cases := map[string]string{
		"Killed":       "killed",
		"Survived":     "survived",
		"NoCoverage":   "no-coverage",
		"Timeout":      "timeout",
		"CompileError": "errored",
		"RuntimeError": "errored",
	}
	for in, want := range cases {
		if got := normalizeStrykerStatus(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}
