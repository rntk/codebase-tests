package goplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/pluginutil"
)

// gremlinsCatalog is the static descriptor exposed via Plugin.Mutators().
var gremlinsCatalog = plugin.Mutator{
	ID:          "gremlins",
	Name:        "Gremlins",
	Description: "Mutation testing for Go (github.com/go-gremlins/gremlins)",
}

// gremlinsReport mirrors the JSON shape produced by `gremlins unleash --output`.
// Field names are kept lenient so minor version drift in gremlins doesn't
// break the parser.
type gremlinsReport struct {
	GoModule    string  `json:"go_module"`
	Efficacy    float64 `json:"test_efficacy"`
	Total       int     `json:"mutants_total"`
	Killed      int     `json:"killed_count"`
	Lived       int     `json:"lived_count"`
	NotCovered  int     `json:"not_covered_count"`
	TimedOut    int     `json:"timed_out_count"`
	NotViable   int     `json:"not_viable_count"`
	RuntimeErr  int     `json:"runtime_error_count"`
	ElapsedTime float64 `json:"elapsed_time"`
	Files       []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line_number"`
			Column int    `json:"column_number"`
		} `json:"mutations"`
	} `json:"files"`
}

// RunMutations runs gremlins over the given scope and returns a normalized report.
func (p *Plugin) RunMutations(ctx context.Context, scope plugin.MutationScope, opts plugin.RunOptions) (plugin.MutationRunReport, error) {
	if _, err := exec.LookPath("gremlins"); err != nil {
		return plugin.MutationRunReport{}, fmt.Errorf("gremlins not found in PATH: install from https://gremlins.dev")
	}

	wd := opts.WorkingDir
	if wd == "" {
		wd = p.root
	}

	tmp, err := os.CreateTemp("", "gremlins-*.json")
	if err != nil {
		return plugin.MutationRunReport{}, fmt.Errorf("temp file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	args := []string{"unleash", "--output", tmp.Name()}
	args = append(args, gremlinsTargets(scope)...)

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "gremlins", args...)
	cmd.Dir = wd
	cmd.Env = pluginutil.BuildEnv(opts.EnvAllowlist, p.env)

	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	maxOut := opts.MaxOutputBytes
	if maxOut == 0 {
		maxOut = 1 << 20
	}
	if int64(len(out)) > maxOut {
		out = append(out[:maxOut], []byte("\n...truncated...")...)
	}

	data, readErr := os.ReadFile(tmp.Name())
	if readErr != nil || len(data) == 0 {
		// Tool failed before producing the report.
		return plugin.MutationRunReport{
			Tool:     "gremlins",
			Output:   string(out),
			Duration: elapsed,
		}, fmt.Errorf("gremlins did not produce a report: %v", runErr)
	}

	report, err := parseGremlinsReport(data)
	if err != nil {
		return plugin.MutationRunReport{Tool: "gremlins", Output: string(out)}, err
	}
	report.Output = string(out)
	report.Duration = elapsed
	return report, nil
}

func gremlinsTargets(scope plugin.MutationScope) []string {
	if len(scope.Packages) > 0 {
		return scope.Packages
	}
	if len(scope.Files) == 0 {
		return []string{"./..."}
	}
	// Map files to their containing package directories.
	seen := make(map[string]struct{})
	var pkgs []string
	for _, f := range scope.Files {
		dir := filepath.Dir(f)
		if dir == "" || dir == "." {
			dir = "."
		} else {
			dir = "./" + dir
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		pkgs = append(pkgs, dir)
	}
	return pkgs
}

func parseGremlinsReport(data []byte) (plugin.MutationRunReport, error) {
	var r gremlinsReport
	if err := json.Unmarshal(data, &r); err != nil {
		return plugin.MutationRunReport{}, fmt.Errorf("parse gremlins report: %w", err)
	}

	rep := plugin.MutationRunReport{
		Tool:       "gremlins",
		Score:      r.Efficacy / 100.0,
		Total:      r.Total,
		Killed:     r.Killed,
		Survived:   r.Lived,
		NoCoverage: r.NotCovered,
		TimedOut:   r.TimedOut,
		Errored:    r.RuntimeErr + r.NotViable,
	}
	for _, f := range r.Files {
		for _, m := range f.Mutations {
			rep.Mutants = append(rep.Mutants, plugin.Mutant{
				File:     f.FileName,
				Line:     m.Line,
				Column:   m.Column,
				Operator: m.Type,
				Status:   normalizeGremlinsStatus(m.Status),
			})
		}
	}
	return rep, nil
}

func normalizeGremlinsStatus(s string) string {
	switch strings.ToUpper(s) {
	case "KILLED":
		return "killed"
	case "LIVED":
		return "survived"
	case "NOT_COVERED":
		return "no-coverage"
	case "TIMED_OUT":
		return "timeout"
	case "NOT_VIABLE", "RUNTIME_ERROR":
		return "errored"
	default:
		return strings.ToLower(s)
	}
}
