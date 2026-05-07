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
	"github.com/rntk/codebase-tests/internal/pluginutil"
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
	SchemaVersion string                 `json:"schemaVersion"`
	Files         map[string]strykerFile `json:"files"`
	Thresholds    map[string]float64     `json:"thresholds,omitempty"`
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

	var err error
	wd, scope.Files, err = findPackageRoot(wd, scope.Files)
	if err != nil {
		return plugin.MutationRunReport{}, err
	}

	env := pluginutil.BuildEnv(opts.EnvAllowlist, p.env)
	if err := ensureStrykerInstalled(wd, env); err != nil {
		return plugin.MutationRunReport{}, err
	}

	// Remove stale Stryker sandboxes and create a symlink for tests/golden so
	// that test files using __dirname relative paths can find fixture files
	// from inside Stryker's sandbox.
	_ = os.RemoveAll(filepath.Join(wd, ".stryker-tmp"))
	if goldenDir := findTestsGoldenDir(wd); goldenDir != "" {
		symlinkPath := filepath.Join(wd, ".stryker-tmp", "tests", "golden")
		_ = os.MkdirAll(filepath.Dir(symlinkPath), 0755)
		_ = os.Symlink(goldenDir, symlinkPath)
	}

	args := []string{"--yes", "@stryker-mutator/core@8.7.0", "run", "--testRunner", "vitest", "--reporters", "json", "--ignorePatterns", "*.timestamp-*.mjs,*.timestamp-*.cjs"}
	if mutate := strykerMutateArg(scope); mutate != "" {
		args = append(args, "--mutate", mutate)
	}

	// Mutation testing is inherently slow; enforce a minimum timeout of 2 minutes.
	timeout := opts.Timeout
	if timeout < 120 {
		timeout = 120
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = wd
	cmd.Env = env

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

// findPackageRoot finds the nearest directory containing package.json for the given files.
// If wd already contains package.json, it returns wd and the files unchanged.
// Otherwise it walks up from each file's directory to find package.json and returns
// that directory with file paths adjusted to be relative to it.
func findPackageRoot(wd string, files []string) (string, []string, error) {
	if len(files) == 0 {
		return wd, files, nil
	}
	if _, err := os.Stat(filepath.Join(wd, "package.json")); err == nil {
		return wd, files, nil
	}

	var pkgRoot string
	out := make([]string, len(files))
	for i, f := range files {
		dir := filepath.Dir(f)
		if !filepath.IsAbs(f) {
			dir = filepath.Join(wd, dir)
		}

		root := dir
		for {
			if _, err := os.Stat(filepath.Join(root, "package.json")); err == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				return "", nil, fmt.Errorf("no package.json found for %s", f)
			}
			root = parent
		}

		if pkgRoot == "" {
			pkgRoot = root
		} else if filepath.Clean(pkgRoot) != filepath.Clean(root) {
			return "", nil, fmt.Errorf("mutation files span multiple package roots (%s and %s)", pkgRoot, root)
		}

		absFile := f
		if !filepath.IsAbs(f) {
			absFile = filepath.Join(wd, f)
		}
		rel, err := filepath.Rel(pkgRoot, absFile)
		if err != nil {
			rel = f
		}
		out[i] = filepath.ToSlash(rel)
	}
	return pkgRoot, out, nil
}

// findTestsGoldenDir searches upward from wd for a tests/golden directory.
func findTestsGoldenDir(wd string) string {
	dir := wd
	for {
		candidate := filepath.Join(dir, "tests", "golden")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ensureStrykerInstalled checks for a local @stryker-mutator/core installation
// and installs it if missing. A local installation is required so that Stryker
// can resolve project dependencies (e.g. typescript) via Node's module resolution.
func ensureStrykerInstalled(wd string, env []string) error {
	corePkg := filepath.Join(wd, "node_modules", "@stryker-mutator", "core", "package.json")
	runnerPkg := filepath.Join(wd, "node_modules", "@stryker-mutator", "vitest-runner", "package.json")
	if pkgExists(corePkg) && pkgExists(runnerPkg) {
		return nil
	}
	cmd := exec.Command("npm", "install", "--no-save", "--no-package-lock", "--force", "@stryker-mutator/core@8.7.0", "@stryker-mutator/vitest-runner@8.7.0")
	cmd.Dir = wd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install @stryker-mutator/core in %s: %w\n%s", wd, err, string(out))
	}
	return nil
}

func pkgExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
