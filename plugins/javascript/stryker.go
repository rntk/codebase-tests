package javascriptplugin

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
)

// strykerCatalog is the static descriptor exposed via Plugin.Mutators().
var strykerCatalog = plugin.Mutator{
	ID:          "stryker",
	Name:        "StrykerJS",
	Description: "Mutation testing for JavaScript/TypeScript (stryker-mutator.io)",
}

// strykerReport models the relevant subset of Stryker's mutation.json schema.
// See https://stryker-mutator.io/docs/mutation-testing-elements/mutation-testing-report-schema/
type strykerReport struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Files         map[string]strykerFile  `json:"files"`
	Thresholds    map[string]float64      `json:"thresholds,omitempty"`
}

type strykerFile struct {
	Language string          `json:"language"`
	Mutants  []strykerMutant `json:"mutants"`
}

type strykerMutant struct {
	ID          string `json:"id"`
	MutatorName string `json:"mutatorName"`
	Status      string `json:"status"`
	Replacement string `json:"replacement,omitempty"`
	Location    struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"location"`
}

// RunMutations runs Stryker over the given scope and returns a normalized report.
func (p *Plugin) RunMutations(ctx context.Context, scope plugin.MutationScope, opts plugin.RunOptions) (plugin.MutationRunReport, error) {
	if _, err := exec.LookPath("npx"); err != nil {
		return plugin.MutationRunReport{}, fmt.Errorf("npx not found in PATH; Node.js is required to run Stryker")
	}

	wd := opts.WorkingDir
	if wd == "" {
		wd = p.root
	}

	args := []string{"--yes", "stryker", "run", "--reporters", "json"}
	if mutate := strykerMutateArg(scope); mutate != "" {
		args = append(args, "--mutate", mutate)
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = wd
	cmd.Env = buildEnv(opts.EnvAllowlist, p.env)

	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	maxOut := opts.MaxOutputBytes
	if maxOut == 0 {
		maxOut = 1 << 20
	}
	truncated := out
	if int64(len(truncated)) > maxOut {
		truncated = append(truncated[:maxOut], []byte("\n...truncated...")...)
	}

	reportPath := filepath.Join(wd, "reports", "mutation", "mutation.json")
	data, readErr := os.ReadFile(reportPath)
	if readErr != nil || len(data) == 0 {
		return plugin.MutationRunReport{
			Tool:     "stryker",
			Output:   string(truncated),
			Duration: elapsed,
		}, fmt.Errorf("stryker did not produce %s: %v", reportPath, runErr)
	}

	report, err := parseStrykerReport(data)
	if err != nil {
		return plugin.MutationRunReport{Tool: "stryker", Output: string(truncated)}, err
	}
	report.Output = string(truncated)
	report.Duration = elapsed
	return report, nil
}

// strykerMutateArg builds Stryker's --mutate glob list from the scope.
func strykerMutateArg(scope plugin.MutationScope) string {
	if len(scope.Files) == 0 {
		return ""
	}
	return strings.Join(scope.Files, ",")
}

func parseStrykerReport(data []byte) (plugin.MutationRunReport, error) {
	var r strykerReport
	if err := json.Unmarshal(data, &r); err != nil {
		return plugin.MutationRunReport{}, fmt.Errorf("parse stryker report: %w", err)
	}

	rep := plugin.MutationRunReport{Tool: "stryker"}
	for path, file := range r.Files {
		for _, m := range file.Mutants {
			status := normalizeStrykerStatus(m.Status)
			rep.Mutants = append(rep.Mutants, plugin.Mutant{
				File:        path,
				Line:        m.Location.Start.Line,
				Column:      m.Location.Start.Column,
				Operator:    m.MutatorName,
				Status:      status,
				Replacement: m.Replacement,
			})
			rep.Total++
			switch status {
			case "killed":
				rep.Killed++
			case "survived":
				rep.Survived++
			case "no-coverage":
				rep.NoCoverage++
			case "timeout":
				rep.TimedOut++
			case "errored":
				rep.Errored++
			}
		}
	}
	if rep.Killed+rep.Survived > 0 {
		rep.Score = float64(rep.Killed) / float64(rep.Killed+rep.Survived)
	}
	return rep, nil
}

func normalizeStrykerStatus(s string) string {
	switch strings.ToLower(s) {
	case "killed":
		return "killed"
	case "survived":
		return "survived"
	case "nocoverage", "no coverage":
		return "no-coverage"
	case "timeout":
		return "timeout"
	case "compileerror", "runtimeerror":
		return "errored"
	case "ignored":
		return "ignored"
	default:
		return strings.ToLower(s)
	}
}
