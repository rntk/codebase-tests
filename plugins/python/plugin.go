package pythonplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rntk/codebase-tests/internal/plugin"
)

// Plugin implements plugin.Plugin for Python using filesystem and source-based discovery.
type Plugin struct {
	mu          sync.RWMutex
	root        string
	timeout     time.Duration
	env         map[string]string
	pythonBin   string
	testsByID   map[string]plugin.TestFunc
	symbolsByID map[string]plugin.Symbol
}

var _ plugin.Plugin = (*Plugin)(nil)

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "python"
}

// Initialize checks the Python command-line tools used by the plugin.
func (p *Plugin) Initialize(ctx context.Context, cfg plugin.PluginConfig) error {
	pythonBin, err := findPython()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("pytest"); err != nil {
		return fmt.Errorf("pytest not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("coverage"); err != nil {
		return fmt.Errorf("coverage not found in PATH: %w", err)
	}

	p.root = cfg.ProjectRoot
	p.pythonBin = pythonBin
	p.timeout = time.Duration(cfg.ToolTimeoutSeconds) * time.Second
	if p.timeout == 0 {
		p.timeout = 30 * time.Second
	}
	p.env = cfg.Env
	return ctx.Err()
}

// Shutdown releases plugin resources.
func (p *Plugin) Shutdown(ctx context.Context) error {
	return ctx.Err()
}

// DiscoverTestFiles finds pytest-style test files.
func (p *Plugin) DiscoverTestFiles(root string) ([]plugin.File, error) {
	p.mu.Lock()
	if p.root == "" {
		p.root = root
	}
	p.mu.Unlock()

	var files []plugin.File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) {
			files = append(files, plugin.File{Path: path, Language: "python", IsTest: true})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

// DiscoverTests parses pytest-style function and class method tests.
func (p *Plugin) DiscoverTests(file plugin.File) ([]plugin.TestFunc, error) {
	if tests, err := p.discoverTestsAST(file.Path); err == nil {
		p.cacheTests(tests)
		return tests, nil
	}

	data, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}

	pkg := p.packageForFile(file.Path)
	tests := parsePythonTests(p.relativePath(file.Path), pkg, data)
	p.cacheTests(tests)
	return tests, nil
}

func (p *Plugin) cacheTests(tests []plugin.TestFunc) {
	p.mu.Lock()
	if p.testsByID == nil {
		p.testsByID = make(map[string]plugin.TestFunc)
	}
	for _, t := range tests {
		p.testsByID[t.ID] = t
	}
	p.mu.Unlock()
}

// DiscoverSymbols walks Python files and extracts classes, functions, and methods.
func (p *Plugin) DiscoverSymbols(root string) ([]plugin.Symbol, error) {
	var symbols []plugin.Symbol
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") {
			return nil
		}
		if astSymbols, serr := p.discoverSymbolsAST(path); serr == nil {
			symbols = append(symbols, astSymbols...)
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		symbols = append(symbols, parsePythonSymbols(p.relativePath(path), p.packageForFile(path), data)...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File == symbols[j].File {
			return symbols[i].Line < symbols[j].Line
		}
		return symbols[i].File < symbols[j].File
	})
	p.mu.Lock()
	p.symbolsByID = make(map[string]plugin.Symbol)
	for _, s := range symbols {
		p.symbolsByID[s.ID] = s
	}
	p.mu.Unlock()
	return symbols, nil
}

// ResolveSymbol returns the cached symbol containing the supplied line.
func (p *Plugin) ResolveSymbol(pos plugin.Position) (plugin.Symbol, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var best plugin.Symbol
	for _, sym := range p.symbolsByID {
		if filepath.Clean(sym.File) != filepath.Clean(pos.File) {
			continue
		}
		if sym.Line <= pos.Line && (best.ID == "" || sym.Line >= best.Line) {
			best = sym
		}
	}
	if best.ID == "" {
		return plugin.Symbol{}, fmt.Errorf("no cached symbol found at %s:%d:%d", pos.File, pos.Line, pos.Column)
	}
	return best, nil
}

// CallGraph returns an empty AST-only graph scaffold rooted at sym.
func (p *Plugin) CallGraph(sym plugin.Symbol) (plugin.Graph, error) {
	return plugin.Graph{Symbol: sym.ID}, nil
}

// RunTests executes pytest and parses a conservative subset of its output.
func (p *Plugin) RunTests(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.RunResult, error) {
	wd := p.workingDir(opts)
	args := []string{"-q", "-vv"}
	if sel.File != "" {
		args = append(args, relOrAbs(wd, sel.File))
	}
	if len(sel.TestIDs) > 0 {
		args = append(args, "-k", testSelectionExpr(sel.TestIDs))
	}

	start := time.Now()
	out, err := p.runCommand(ctx, wd, opts, "pytest", args...)
	result := parsePytestOutput(out)
	result.Duration = time.Since(start).Seconds()
	result.Output = out
	if err != nil {
		result.Passed = false
	}
	p.fillTestIDs(&result)
	return result, nil
}

// Coverage runs coverage.py and maps JSON output to a run-level report.
func (p *Plugin) Coverage(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.CoverageReport, error) {
	wd := p.workingDir(opts)
	tmp, err := os.CreateTemp("", "python-coverage-*.json")
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	dataFile, err := os.CreateTemp("", "python-coverage-*.data")
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	dataPath := dataFile.Name()
	_ = dataFile.Close()
	defer os.Remove(dataPath)

	pytestArgs := []string{"run", "--data-file", dataPath, "-m", "pytest", "-q"}
	if sel.File != "" {
		pytestArgs = append(pytestArgs, relOrAbs(wd, sel.File))
	}
	if len(sel.TestIDs) > 0 {
		pytestArgs = append(pytestArgs, "-k", testSelectionExpr(sel.TestIDs))
	}
	_, _ = p.runCommand(ctx, wd, opts, "coverage", pytestArgs...)

	if _, err := p.runCommand(ctx, wd, opts, "coverage", "json", "--data-file", dataPath, "-q", "-o", tmpPath); err != nil {
		return plugin.CoverageReport{}, err
	}
	report, err := parseCoverageJSON(tmpPath, wd)
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	report.Scope = "run"

	symbols, _ := p.DiscoverSymbols(wd)
	for _, sym := range symbols {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		fc, ok := findFileCoverage(report.Files, sym.File)
		if !ok || !isLineCovered(sym.Line, fc.Lines) {
			report.Uncovered = append(report.Uncovered, sym.ID)
		}
	}
	return report, nil
}

// GoldenLayout returns the default Plan-0 convention.
func (p *Plugin) GoldenLayout() plugin.GoldenConvention {
	return plugin.GoldenConvention{
		Root:        "tests/golden",
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}
}

// Mutators returns no Python mutators in v1.
func (p *Plugin) Mutators() []plugin.Mutator {
	return nil
}

// Generators returns no Python generators in v1.
func (p *Plugin) Generators() []plugin.Generator {
	return nil
}

func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("python not found in PATH")
}

func (p *Plugin) python() (string, error) {
	if p.pythonBin != "" {
		return p.pythonBin, nil
	}
	return findPython()
}

func (p *Plugin) discoverTestsAST(path string) ([]plugin.TestFunc, error) {
	pythonBin, err := p.python()
	if err != nil {
		return nil, err
	}
	root := p.root
	if root == "" {
		root = filepath.Dir(path)
	}
	out, err := exec.Command(pythonBin, "-c", pythonASTScript, "tests", path, root).Output()
	if err != nil {
		return nil, err
	}
	var tests []plugin.TestFunc
	if err := json.Unmarshal(out, &tests); err != nil {
		return nil, err
	}
	return tests, nil
}

func (p *Plugin) discoverSymbolsAST(path string) ([]plugin.Symbol, error) {
	pythonBin, err := p.python()
	if err != nil {
		return nil, err
	}
	root := p.root
	if root == "" {
		root = filepath.Dir(path)
	}
	out, err := exec.Command(pythonBin, "-c", pythonASTScript, "symbols", path, root).Output()
	if err != nil {
		return nil, err
	}
	var symbols []plugin.Symbol
	if err := json.Unmarshal(out, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

const pythonASTScript = `
import ast
import json
import os
import sys

mode, path, root = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, "r", encoding="utf-8") as f:
    tree = ast.parse(f.read(), filename=path)

def package_for(path, root):
    rel = os.path.relpath(path, root)
    if rel.endswith(".py"):
        rel = rel[:-3]
    if rel.endswith(os.sep + "__init__"):
        rel = rel[: -len(os.sep + "__init__")]
    return rel.replace(os.sep, ".")

def rel_file(path, root):
    return os.path.relpath(path, root).replace(os.sep, "/")

def qualify(pkg, name):
    return f"{pkg}.{name}" if pkg else name

def case_path(name):
    return str(name).replace("%", "%25").replace(":", "%3A").replace("/", "%2F")

def parametrize_cases(node, base_id):
    cases = []
    for dec in node.decorator_list:
        call = dec if isinstance(dec, ast.Call) else None
        if call is None:
            continue
        func = call.func
        is_parametrize = (
            isinstance(func, ast.Attribute)
            and func.attr == "parametrize"
        )
        if not is_parametrize:
            continue
        ids = None
        for kw in call.keywords:
            if kw.arg == "ids" and isinstance(kw.value, (ast.List, ast.Tuple)):
                ids = [elt.value for elt in kw.value.elts if isinstance(elt, ast.Constant)]
        if ids is None and len(call.args) >= 2 and isinstance(call.args[1], (ast.List, ast.Tuple)):
            ids = [str(i) for i, _ in enumerate(call.args[1].elts)]
        for i, name in enumerate(ids or []):
            escaped = case_path(name if name else i)
            cases.append({"id": f"{base_id}:{escaped}", "name": str(name), "casePath": escaped})
    return cases

pkg = package_for(path, root)
rel = rel_file(path, root)
if mode == "tests":
    out = []
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith("test_"):
            qualified = qualify(pkg, node.name)
            test_id = f"python:{rel}:{qualified}"
            out.append({
                "id": test_id,
                "name": node.name,
                "file": rel,
                "line": node.lineno,
                "column": node.col_offset + 1,
                "package": pkg,
                "subCases": parametrize_cases(node, test_id),
            })
        if isinstance(node, ast.ClassDef) and node.name.startswith("Test"):
            for child in node.body:
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)) and child.name.startswith("test_"):
                    name = f"{node.name}.{child.name}"
                    qualified = qualify(pkg, name)
                    test_id = f"python:{rel}:{qualified}"
                    out.append({
                        "id": test_id,
                        "name": name,
                        "file": rel,
                        "line": child.lineno,
                        "column": child.col_offset + 1,
                        "package": pkg,
                        "subCases": parametrize_cases(child, test_id),
                    })
    print(json.dumps(out))
else:
    out = []
    for node in tree.body:
        if isinstance(node, ast.ClassDef):
            out.append({
                "id": f"python:{rel}:{qualify(pkg, node.name)}:{node.lineno}:{node.col_offset + 1}",
                "name": node.name,
                "qualifiedName": qualify(pkg, node.name),
                "kind": "class",
                "file": rel,
                "line": node.lineno,
                "column": node.col_offset + 1,
                "package": pkg,
            })
            for child in node.body:
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    out.append({
                        "id": f"python:{rel}:{qualify(pkg, f'{node.name}.{child.name}')}:{child.lineno}:{child.col_offset + 1}",
                        "name": child.name,
                        "qualifiedName": qualify(pkg, f"{node.name}.{child.name}"),
                        "kind": "method",
                        "file": rel,
                        "line": child.lineno,
                        "column": child.col_offset + 1,
                        "package": pkg,
                    })
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            out.append({
                "id": f"python:{rel}:{qualify(pkg, node.name)}:{node.lineno}:{node.col_offset + 1}",
                "name": node.name,
                "qualifiedName": qualify(pkg, node.name),
                "kind": "function",
                "file": rel,
                "line": node.lineno,
                "column": node.col_offset + 1,
                "package": pkg,
            })
    print(json.dumps(out))
`

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".mypy_cache", ".pytest_cache", ".ruff_cache", "__pycache__", ".venv", "venv", "env":
		return true
	default:
		return false
	}
}

func (p *Plugin) packageForFile(path string) string {
	root := p.root
	if root == "" {
		root = filepath.Dir(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), ".py")
	rel = strings.TrimSuffix(rel, "/__init__")
	return strings.ReplaceAll(rel, "/", ".")
}

func (p *Plugin) relativePath(path string) string {
	root := p.root
	if root == "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

func (p *Plugin) workingDir(opts plugin.RunOptions) string {
	if opts.WorkingDir != "" {
		return opts.WorkingDir
	}
	if p.root != "" {
		return p.root
	}
	return "."
}

func (p *Plugin) runCommand(ctx context.Context, wd string, opts plugin.RunOptions, name string, args ...string) (string, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = wd
	cmd.Env = buildEnv(opts.EnvAllowlist, p.env)
	out, err := cmd.CombinedOutput()
	maxOut := opts.MaxOutputBytes
	if maxOut == 0 {
		maxOut = 1 << 20
	}
	if int64(len(out)) > maxOut {
		out = append(out[:int(maxOut)], []byte("\n...truncated...")...)
	}
	return string(out), err
}

func buildEnv(allowlist []string, extra map[string]string) []string {
	env := os.Environ()
	if len(allowlist) > 0 {
		allowed := make(map[string]bool)
		for _, k := range allowlist {
			allowed[k] = true
		}
		filtered := make([]string, 0, len(env))
		for _, e := range env {
			key := strings.SplitN(e, "=", 2)[0]
			if allowed[key] {
				filtered = append(filtered, e)
			}
		}
		env = filtered
	}
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

var (
	classRE = regexp.MustCompile(`^\s*class\s+(Test\w*|\w+)\b`)
	defRE   = regexp.MustCompile(`^\s*def\s+([A-Za-z_]\w*)\s*\(`)
)

type classFrame struct {
	name   string
	indent int
}

func parsePythonTests(path, pkg string, data []byte) []plugin.TestFunc {
	var tests []plugin.TestFunc
	var class *classFrame
	var pending []plugin.TestCase
	lines := bytes.Split(data, []byte("\n"))
	for i, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := countIndent(line)
		if class != nil && indent <= class.indent && !strings.HasPrefix(trimmed, "@") {
			class = nil
		}
		if strings.HasPrefix(trimmed, "@") {
			if cases := parseParametrizeCases(trimmed); len(cases) > 0 {
				pending = cases
			}
			continue
		}
		if m := classRE.FindStringSubmatch(line); m != nil {
			if strings.HasPrefix(m[1], "Test") {
				class = &classFrame{name: m[1], indent: indent}
			} else {
				class = nil
			}
			pending = nil
			continue
		}
		m := defRE.FindStringSubmatch(line)
		if m == nil {
			if !strings.HasPrefix(trimmed, "@") {
				pending = nil
			}
			continue
		}
		name := m[1]
		if !strings.HasPrefix(name, "test_") {
			pending = nil
			continue
		}
		display := name
		if class != nil && indent > class.indent {
			display = class.name + "." + name
		} else if indent != 0 {
			pending = nil
			continue
		}
		id := plugin.NewTestID("python", path, display)
		for j := range pending {
			pending[j].ID = plugin.NewTestCaseID("python", path, display, plugin.UnescapeSegment(pending[j].CasePath))
		}
		tests = append(tests, plugin.TestFunc{
			ID:       id,
			Name:     display,
			File:     path,
			Line:     i + 1,
			Column:   indent + 1,
			Package:  pkg,
			SubCases: pending,
		})
		pending = nil
	}
	return tests
}

func parsePythonSymbols(path, pkg string, data []byte) []plugin.Symbol {
	var symbols []plugin.Symbol
	var class *classFrame
	lines := bytes.Split(data, []byte("\n"))
	for i, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "@") {
			continue
		}
		indent := countIndent(line)
		if class != nil && indent <= class.indent {
			class = nil
		}
		if m := classRE.FindStringSubmatch(line); m != nil {
			name := m[1]
			qualified := qualify(pkg, name)
			id := plugin.NewSymbolID("python", path, qualified, i+1, indent+1)
			symbols = append(symbols, plugin.Symbol{
				ID:            id,
				Name:          name,
				QualifiedName: qualified,
				Kind:          "class",
				File:          path,
				Line:          i + 1,
				Column:        indent + 1,
				Package:       pkg,
			})
			class = &classFrame{name: name, indent: indent}
			continue
		}
		m := defRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		kind := "function"
		qualified := qualify(pkg, name)
		if class != nil && indent > class.indent {
			kind = "method"
			qualified = qualify(pkg, class.name+"."+name)
		} else if indent != 0 {
			continue
		}
		symbols = append(symbols, plugin.Symbol{
			ID:            plugin.NewSymbolID("python", path, qualified, i+1, indent+1),
			Name:          name,
			QualifiedName: qualified,
			Kind:          kind,
			File:          path,
			Line:          i + 1,
			Column:        indent + 1,
			Package:       pkg,
		})
	}
	return symbols
}

func countIndent(line string) int {
	count := 0
	for _, r := range line {
		switch r {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count
		}
	}
	return count
}

func qualify(pkg, name string) string {
	if pkg == "" || pkg == "." {
		return name
	}
	return pkg + "." + name
}

func parseParametrizeCases(line string) []plugin.TestCase {
	if !strings.Contains(line, "parametrize") {
		return nil
	}
	var names []string
	if idx := strings.Index(line, "ids="); idx >= 0 {
		names = quotedStrings(line[idx:])
	}
	if len(names) == 0 {
		names = quotedStrings(line)
		if len(names) > 0 {
			names = names[1:]
		}
	}
	var cases []plugin.TestCase
	for i, name := range names {
		if name == "" {
			name = strconv.Itoa(i)
		}
		escaped := plugin.EscapeSegment(name)
		cases = append(cases, plugin.TestCase{Name: name, CasePath: escaped})
	}
	return cases
}

func quotedStrings(s string) []string {
	re := regexp.MustCompile(`["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func testSelectionExpr(ids []string) string {
	var names []string
	seen := make(map[string]bool)
	for _, id := range ids {
		parsed, err := plugin.ParseID(id)
		if err != nil {
			continue
		}
		name := parsed.QualifiedName
		if strings.Contains(name, ".") {
			nameParts := strings.Split(name, ".")
			name = nameParts[len(nameParts)-1]
		}
		name = plugin.UnescapeSegment(name)
		if name != "" && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	sort.Strings(names)
	return strings.Join(names, " or ")
}

func relOrAbs(wd, path string) string {
	if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func parsePytestOutput(out string) plugin.RunResult {
	result := plugin.RunResult{Passed: strings.Contains(out, " passed")}
	lineRE := regexp.MustCompile(`(?m)^(.+?)\s+(PASSED|FAILED|ERROR|SKIPPED)\s*(.*)$`)
	matches := lineRE.FindAllStringSubmatch(out, -1)
	for _, m := range matches {
		name := filepath.Base(m[1])
		entry := plugin.TestEntry{
			Name:   name,
			Passed: m[2] == "PASSED",
			Output: strings.TrimSpace(m[3]),
		}
		result.Tests = append(result.Tests, entry)
		if !entry.Passed {
			result.Passed = false
		}
	}
	return result
}

func (p *Plugin) fillTestIDs(result *plugin.RunResult) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i := range result.Tests {
		for _, tf := range p.testsByID {
			if result.Tests[i].Name == tf.Name || strings.Contains(result.Tests[i].Name, tf.Name) {
				result.Tests[i].ID = tf.ID
				break
			}
		}
	}
}

type coverageJSON struct {
	Totals struct {
		PercentCovered float64 `json:"percent_covered"`
	} `json:"totals"`
	Files map[string]struct {
		Summary struct {
			PercentCovered float64 `json:"percent_covered"`
		} `json:"summary"`
		ExecutedLines []int `json:"executed_lines"`
		MissingLines  []int `json:"missing_lines"`
	} `json:"files"`
}

func parseCoverageJSON(path, wd string) (plugin.CoverageReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	var raw coverageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return plugin.CoverageReport{}, err
	}
	report := plugin.CoverageReport{
		Scope:      "run",
		Percentage: raw.Totals.PercentCovered,
	}
	for file, fc := range raw.Files {
		fullPath := file
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(wd, file)
		}
		report.Files = append(report.Files, plugin.FileCoverage{
			Path:       fullPath,
			Percentage: fc.Summary.PercentCovered,
			Lines:      coverageLines(fc.ExecutedLines, fc.MissingLines),
		})
	}
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Path < report.Files[j].Path })
	return report, nil
}

func coverageLines(executed, missing []int) []plugin.LineRange {
	hit := make(map[int]bool)
	for _, line := range executed {
		hit[line] = true
	}
	for _, line := range missing {
		hit[line] = false
	}
	lines := make([]int, 0, len(hit))
	for line := range hit {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	out := make([]plugin.LineRange, 0, len(lines))
	for _, line := range lines {
		out = append(out, plugin.LineRange{Start: line, End: line, Hit: hit[line]})
	}
	return out
}

func findFileCoverage(files []plugin.FileCoverage, path string) (plugin.FileCoverage, bool) {
	for _, fc := range files {
		if filepath.Clean(fc.Path) == filepath.Clean(path) {
			return fc, true
		}
		if strings.HasSuffix(filepath.Clean(path), filepath.Clean(fc.Path)) {
			return fc, true
		}
		if strings.HasSuffix(filepath.Clean(fc.Path), filepath.Clean(path)) {
			return fc, true
		}
	}
	return plugin.FileCoverage{}, false
}

func isLineCovered(line int, ranges []plugin.LineRange) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End && r.Hit {
			return true
		}
	}
	return false
}
