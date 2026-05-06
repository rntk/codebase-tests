package javascriptplugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rntk/codebase-tests/internal/lsp"
	"github.com/rntk/codebase-tests/internal/plugin"
)

// Plugin implements plugin.Plugin for JavaScript, TypeScript, and web tests.
type Plugin struct {
	mu          sync.RWMutex
	client      *lsp.Client
	name        string
	root        string
	timeout     time.Duration
	env         map[string]string
	testsByID   map[string]plugin.TestFunc
	symbolsByID map[string]plugin.Symbol
}

var _ plugin.Plugin = (*Plugin)(nil)

// Name returns the plugin name.
func (p *Plugin) Name() string {
	if p.name != "" {
		return p.name
	}
	return "javascript"
}

// Initialize checks Node/npm and starts the configured TypeScript LSP server when available.
func (p *Plugin) Initialize(ctx context.Context, cfg plugin.PluginConfig) error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found in PATH: %w", err)
	}

	p.root = cfg.ProjectRoot
	p.timeout = time.Duration(cfg.ToolTimeoutSeconds) * time.Second
	if p.timeout == 0 {
		p.timeout = 30 * time.Second
	}
	p.env = cfg.Env

	lspCommand := strings.Fields(cfg.LSPServerAddress)
	if len(lspCommand) == 0 {
		lspCommand = []string{"typescript-language-server", "--stdio"}
	}
	if _, err := exec.LookPath(lspCommand[0]); err != nil {
		log.Printf("%s plugin: %s not found in PATH; running in degraded mode (source discovery only)", p.Name(), lspCommand[0])
		return nil
	}

	p.client = lsp.NewClient(lspCommand, pathToURI(cfg.ProjectRoot), p.timeout)
	if err := p.client.Start(ctx); err != nil {
		log.Printf("%s plugin: failed to start LSP client (%v); running in degraded mode", p.Name(), err)
		p.client = nil
	}
	return ctx.Err()
}

// Shutdown stops the LSP client.
func (p *Plugin) Shutdown(ctx context.Context) error {
	if p.client != nil {
		return p.client.Shutdown(ctx)
	}
	return ctx.Err()
}

// DiscoverTestFiles finds common Jest/Vitest test files.
func (p *Plugin) DiscoverTestFiles(root string) ([]plugin.File, error) {
	p.setRoot(root)
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
		if isTestFile(path) {
			files = append(files, plugin.File{Path: path, Language: languageForFile(path), IsTest: true})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

// DiscoverTests parses test and it calls from a JavaScript/TypeScript test file.
func (p *Plugin) DiscoverTests(file plugin.File) ([]plugin.TestFunc, error) {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}
	tests := parseTests(p.Name(), p.relativePath(file.Path), p.packageForFile(file.Path), data)
	p.cacheTests(tests)
	return tests, nil
}

// DiscoverSymbols uses LSP document symbols when possible and falls back to source scanning.
func (p *Plugin) DiscoverSymbols(root string) ([]plugin.Symbol, error) {
	p.setRoot(root)
	if p.client != nil {
		syms, err := p.discoverSymbolsFromLSP(root)
		if err == nil && len(syms) > 0 {
			p.cacheSymbols(syms)
			return syms, nil
		}
		log.Printf("%s plugin: LSP symbol discovery returned %d symbols (err=%v); falling back to source scan", p.Name(), len(syms), err)
	}
	syms, err := p.discoverSymbolsFromSource(root)
	if err == nil {
		p.cacheSymbols(syms)
	}
	return syms, err
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

// CallGraph returns an empty source-only graph scaffold rooted at sym.
func (p *Plugin) CallGraph(sym plugin.Symbol) (plugin.Graph, error) {
	return plugin.Graph{Symbol: sym.ID}, nil
}

// RunTests executes the project's npm test script, optimized for Vitest/Jest-style web tests.
func (p *Plugin) RunTests(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.RunResult, error) {
	wd := p.workingDir(opts)
	args := []string{"test", "--"}
	if sel.File != "" {
		args = append(args, relOrAbs(wd, sel.File))
	}
	if len(sel.TestIDs) > 0 {
		args = append(args, "-t", testNamePattern(sel.TestIDs))
	}

	start := time.Now()
	out, err := p.runCommand(ctx, wd, opts, "npm", args...)
	result := parseTestOutput(out)
	result.Duration = time.Since(start).Seconds()
	result.Output = out
	if err != nil {
		result.Passed = false
	}
	p.fillTestIDs(&result)
	return result, nil
}

// Coverage runs npm test with coverage enabled and reads common Istanbul/V8 JSON output.
func (p *Plugin) Coverage(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.CoverageReport, error) {
	wd := p.workingDir(opts)
	args := []string{"test", "--", "--coverage"}
	if sel.File != "" {
		args = append(args, relOrAbs(wd, sel.File))
	}
	if len(sel.TestIDs) > 0 {
		args = append(args, "-t", testNamePattern(sel.TestIDs))
	}
	_, _ = p.runCommand(ctx, wd, opts, "npm", args...)

	report, err := parseCoverageFinal(filepath.Join(wd, "coverage", "coverage-final.json"), wd)
	if err != nil {
		return plugin.CoverageReport{Scope: "run"}, nil
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

// Mutators returns no JavaScript mutators in v1.
func (p *Plugin) Mutators() []plugin.Mutator { return nil }

// Generators returns no JavaScript generators in v1.
func (p *Plugin) Generators() []plugin.Generator { return nil }

func (p *Plugin) setRoot(root string) {
	p.mu.Lock()
	if p.root == "" {
		p.root = root
	}
	p.mu.Unlock()
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

func (p *Plugin) cacheSymbols(syms []plugin.Symbol) {
	p.mu.Lock()
	p.symbolsByID = make(map[string]plugin.Symbol, len(syms))
	for _, s := range syms {
		p.symbolsByID[s.ID] = s
	}
	p.mu.Unlock()
}

func (p *Plugin) discoverSymbolsFromLSP(root string) ([]plugin.Symbol, error) {
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
		if !isSourceFile(path) {
			return nil
		}
		text, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		uri := pathToURI(path)
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()
		_ = p.client.DidOpen(ctx, uri, languageForFile(path), 1, string(text))
		dsyms, derr := p.client.DocumentSymbol(ctx, uri)
		if derr != nil {
			return nil
		}
		symbols = append(symbols, documentSymbolsToPlugin(p.Name(), dsyms, path, p.packageForFile(path), p.root)...)
		return nil
	})
	return symbols, err
}

func (p *Plugin) discoverSymbolsFromSource(root string) ([]plugin.Symbol, error) {
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
		if !isSourceFile(path) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		symbols = append(symbols, parseSymbols(p.Name(), p.relativePath(path), p.packageForFile(path), data)...)
		return nil
	})
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File == symbols[j].File {
			return symbols[i].Line < symbols[j].Line
		}
		return symbols[i].File < symbols[j].File
	})
	return symbols, err
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
	rel = filepath.ToSlash(rel)
	ext := filepath.Ext(rel)
	rel = strings.TrimSuffix(rel, ext)
	rel = strings.TrimSuffix(rel, ".test")
	rel = strings.TrimSuffix(rel, ".spec")
	rel = strings.TrimSuffix(rel, "/index")
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

func (p *Plugin) fillTestIDs(result *plugin.RunResult) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i := range result.Tests {
		for _, test := range p.testsByID {
			if result.Tests[i].Name == test.Name || strings.HasSuffix(result.Tests[i].Name, " > "+test.Name) {
				result.Tests[i].ID = test.ID
				break
			}
		}
	}
}

var testCallRE = regexp.MustCompile("\\b(?:test|it)(?:\\.(?:only|skip|todo|concurrent))?\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]")
var describeRE = regexp.MustCompile("\\bdescribe(?:\\.(?:only|skip|concurrent))?\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]")

func parseTests(pluginName, path, pkg string, data []byte) []plugin.TestFunc {
	var tests []plugin.TestFunc
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var describes []describeFrame
	depth := 0
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := stripLineComment(scanner.Text())
		describes = activeDescribes(describes, depth)
		if m := describeRE.FindStringSubmatch(line); m != nil {
			describes = append(describes, describeFrame{name: m[1], depth: depth + 1})
		}
		matches := testCallRE.FindAllStringSubmatchIndex(line, -1)
		for _, m := range matches {
			name := line[m[2]:m[3]]
			display := name
			if len(describes) > 0 {
				display = strings.Join(append(describeNames(describes), name), " > ")
			}
			qualified := qualify(pkg, display)
			tests = append(tests, plugin.TestFunc{
				ID:      fmt.Sprintf("%s:%s:%s", pluginName, path, qualified),
				Name:    display,
				File:    path,
				Line:    lineNo,
				Column:  m[0] + 1,
				Package: pkg,
			})
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth < 0 {
			depth = 0
		}
	}
	return tests
}

type describeFrame struct {
	name  string
	depth int
}

func activeDescribes(frames []describeFrame, depth int) []describeFrame {
	for len(frames) > 0 && depth < frames[len(frames)-1].depth {
		frames = frames[:len(frames)-1]
	}
	return frames
}

func describeNames(frames []describeFrame) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		names = append(names, frame.name)
	}
	return names
}

var symbolPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"class", regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?class\s+([A-Za-z_$][\w$]*)`)},
	{"function", regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)},
	{"function", regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)},
	{"function", regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?function\b`)},
}

func parseSymbols(pluginName, path, pkg string, data []byte) []plugin.Symbol {
	var symbols []plugin.Symbol
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		for _, pattern := range symbolPatterns {
			m := pattern.re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}
			name := line[m[2]:m[3]]
			qualified := qualify(pkg, name)
			symbols = append(symbols, plugin.Symbol{
				ID:            fmt.Sprintf("%s:%s:%s:%d:%d", pluginName, path, qualified, lineNo, m[2]+1),
				Name:          name,
				QualifiedName: qualified,
				Kind:          pattern.kind,
				File:          path,
				Line:          lineNo,
				Column:        m[2] + 1,
				Package:       pkg,
			})
			break
		}
	}
	return symbols
}

func documentSymbolsToPlugin(pluginName string, dsyms []lsp.DocumentSymbol, path, pkg, root string) []plugin.Symbol {
	var out []plugin.Symbol
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	var walk func([]lsp.DocumentSymbol)
	walk = func(symbols []lsp.DocumentSymbol) {
		for _, s := range symbols {
			kind := symbolKind(s.Kind)
			if kind != "" {
				line := s.Range.Start.Line + 1
				col := s.Range.Start.Character + 1
				qualified := qualify(pkg, s.Name)
				out = append(out, plugin.Symbol{
					ID:            fmt.Sprintf("%s:%s:%s:%d:%d", pluginName, rel, qualified, line, col),
					Name:          s.Name,
					QualifiedName: qualified,
					Kind:          kind,
					File:          rel,
					Line:          line,
					Column:        col,
					Package:       pkg,
				})
			}
			walk(s.Children)
		}
	}
	walk(dsyms)
	return out
}

func symbolKind(kind int) string {
	switch kind {
	case 5:
		return "class"
	case 6:
		return "method"
	case 12:
		return "function"
	default:
		return ""
	}
}

type istanbulFile struct {
	Path string              `json:"path"`
	S    map[string]int      `json:"s"`
	Stmt map[string]location `json:"statementMap"`
}

type location struct {
	Start locPoint `json:"start"`
	End   locPoint `json:"end"`
}

type locPoint struct {
	Line int `json:"line"`
}

func parseCoverageFinal(path, root string) (plugin.CoverageReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	var raw map[string]istanbulFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return plugin.CoverageReport{}, err
	}
	var report plugin.CoverageReport
	var covered, total int
	for key, file := range raw {
		if file.Path == "" {
			file.Path = key
		}
		fc := plugin.FileCoverage{Path: relPath(root, file.Path)}
		for id, loc := range file.Stmt {
			hit := file.S[id] > 0
			if hit {
				covered++
			}
			total++
			end := loc.End.Line
			if end == 0 {
				end = loc.Start.Line
			}
			fc.Lines = append(fc.Lines, plugin.LineRange{Start: loc.Start.Line, End: end, Hit: hit})
		}
		if len(file.Stmt) > 0 {
			var fileCovered int
			for id := range file.Stmt {
				if file.S[id] > 0 {
					fileCovered++
				}
			}
			fc.Percentage = float64(fileCovered) / float64(len(file.Stmt)) * 100
		}
		report.Files = append(report.Files, fc)
	}
	if total > 0 {
		report.Percentage = float64(covered) / float64(total) * 100
	}
	return report, nil
}

func parseTestOutput(out string) plugin.RunResult {
	result := plugin.RunResult{Passed: true}
	scanner := bufio.NewScanner(strings.NewReader(out))
	statusRE := regexp.MustCompile(`^\s*(?:PASS|FAIL)\s+(.+?)\s*$`)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "FAIL ") || strings.Contains(trimmed, "failed") {
			result.Passed = false
		}
		m := statusRE.FindStringSubmatch(trimmed)
		if m != nil {
			result.Tests = append(result.Tests, plugin.TestEntry{Name: strings.TrimSpace(m[1]), Passed: strings.HasPrefix(trimmed, "PASS")})
		}
	}
	return result
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

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "dist", "build", "coverage", ".next", ".nuxt", ".vite", ".turbo":
		return true
	default:
		return false
	}
}

func isSourceFile(path string) bool {
	if isTestFile(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func isTestFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".js" && ext != ".jsx" && ext != ".ts" && ext != ".tsx" && ext != ".mjs" && ext != ".cjs" {
		return false
	}
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}

func languageForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".jsx":
		return "javascriptreact"
	default:
		return "javascript"
	}
}

func qualify(pkg, name string) string {
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}

func stripLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func testNamePattern(ids []string) string {
	var names []string
	for _, id := range ids {
		parts := strings.Split(id, ":")
		if len(parts) > 0 {
			names = append(names, regexp.QuoteMeta(parts[len(parts)-1]))
		}
	}
	return strings.Join(names, "|")
}

func relOrAbs(wd, path string) string {
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return path
	}
	return filepath.ToSlash(path)
}

func relPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func findFileCoverage(files []plugin.FileCoverage, path string) (plugin.FileCoverage, bool) {
	for _, fc := range files {
		if sameCoveragePath(fc.Path, path) {
			return fc, true
		}
	}
	return plugin.FileCoverage{}, false
}

func isLineCovered(line int, ranges []plugin.LineRange) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End {
			return r.Hit
		}
	}
	return false
}

func sameCoveragePath(a, b string) bool {
	a = filepath.ToSlash(filepath.Clean(a))
	b = filepath.ToSlash(filepath.Clean(b))
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + filepath.ToSlash(abs)
}
