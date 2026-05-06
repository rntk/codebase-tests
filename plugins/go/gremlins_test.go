package goplugin

import "testing"

func TestParseGremlinsReport(t *testing.T) {
	data := []byte(`{
		"go_module": "example.com/m",
		"test_efficacy": 75.0,
		"mutants_total": 4,
		"killed_count": 3,
		"lived_count": 1,
		"not_covered_count": 0,
		"timed_out_count": 0,
		"not_viable_count": 0,
		"runtime_error_count": 0,
		"files": [
			{
				"file_name": "foo/bar.go",
				"mutations": [
					{ "type": "CONDITIONALS_BOUNDARY", "status": "KILLED", "line_number": 5, "column_number": 10 },
					{ "type": "ARITHMETIC_BASE", "status": "LIVED", "line_number": 8, "column_number": 4 }
				]
			}
		]
	}`)

	rep, err := parseGremlinsReport(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Tool != "gremlins" {
		t.Errorf("tool=%q", rep.Tool)
	}
	if rep.Total != 4 || rep.Killed != 3 || rep.Survived != 1 {
		t.Errorf("counts: total=%d killed=%d survived=%d", rep.Total, rep.Killed, rep.Survived)
	}
	if rep.Score != 0.75 {
		t.Errorf("score=%v", rep.Score)
	}
	if len(rep.Mutants) != 2 {
		t.Fatalf("mutants=%d", len(rep.Mutants))
	}
	if rep.Mutants[0].Status != "killed" || rep.Mutants[1].Status != "survived" {
		t.Errorf("statuses: %+v", rep.Mutants)
	}
	if rep.Mutants[0].File != "foo/bar.go" || rep.Mutants[0].Line != 5 {
		t.Errorf("location: %+v", rep.Mutants[0])
	}
}

func TestNormalizeGremlinsStatus(t *testing.T) {
	cases := map[string]string{
		"KILLED":        "killed",
		"LIVED":         "survived",
		"NOT_COVERED":   "no-coverage",
		"TIMED_OUT":     "timeout",
		"NOT_VIABLE":    "errored",
		"RUNTIME_ERROR": "errored",
	}
	for in, want := range cases {
		if got := normalizeGremlinsStatus(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}
