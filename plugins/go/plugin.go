package goplugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rntk/codebase-tests/internal/lsp"
	"github.com/rntk/codebase-tests/internal/plugin"
)

// Plugin implements plugin.Plugin for Go.
type Plugin struct {
	mu          sync.RWMutex
	client      *lsp.Client
	root        string
	timeout     time.Duration
	env         map[string]string
	testsByID   map[string]plugin.TestFunc
	symbolsByID map[string]plugin.Symbol
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "go"
}

func (p *Plugin) Initialize(ctx context.Context, cfg plugin.PluginConfig) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go not found in PATH: %w", err)
	}

	p.root = cfg.ProjectRoot
	p.timeout = time.Duration(cfg.ToolTimeoutSeconds) * time.Second
	if p.timeout == 0 {
		p.timeout = 30 * time.Second
	}
	p.env = cfg.Env

	lspCommand := strings.Fields(cfg.LSPServerAddress)
	if len(lspCommand) == 0 {
		lspCommand = []string{"gopls"}
	}
	if _, err := exec.LookPath(lspCommand[0]); err != nil {
		log.Printf("go plugin: %s not found in PATH; running in degraded mode (AST-only symbol discovery, no call graph or symbol resolution)", lspCommand[0])
		return nil
	}

	rootURI := pathToURI(cfg.ProjectRoot)
	p.client = lsp.NewClient(lspCommand, rootURI, p.timeout)
	if err := p.client.Start(ctx); err != nil {
		log.Printf("go plugin: failed to start LSP client (%v); running in degraded mode", err)
		p.client = nil
	}
	return nil
}

// Shutdown stops the LSP client.
func (p *Plugin) Shutdown(ctx context.Context) error {
	if p.client != nil {
		return p.client.Shutdown(ctx)
	}
	return nil
}

// DiscoverTestFiles finds all *_test.go files under root.
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
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			files = append(files, plugin.File{
				Path:     path,
				Language: "go",
				IsTest:   true,
			})
		}
		return nil
	})
	return files, err
}

// DiscoverTests parses a test file and returns test functions and sub-cases.
func (p *Plugin) DiscoverTests(file plugin.File) ([]plugin.TestFunc, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file.Path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var tests []plugin.TestFunc
	pkg := filepath.Base(filepath.Dir(file.Path))
	relFile := p.relativePath(file.Path)

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !isTestFunction(fn) {
			continue
		}
		pos := fset.Position(fn.Pos())
		qualifiedName := fmt.Sprintf("%s.%s", pkg, fn.Name.Name)
		test := plugin.TestFunc{
			ID:      fmt.Sprintf("go:%s:%s", relFile, qualifiedName),
			Name:    fn.Name.Name,
			File:    relFile,
			Line:    pos.Line,
			Column:  pos.Column,
			Package: pkg,
		}
		test.SubCases = findSubTests(fn, relFile, qualifiedName)
		tests = append(tests, test)
	}

	p.mu.Lock()
	if p.testsByID == nil {
		p.testsByID = make(map[string]plugin.TestFunc)
	}
	for _, t := range tests {
		p.testsByID[t.ID] = t
	}
	p.mu.Unlock()

	return tests, nil
}

func (p *Plugin) DiscoverSymbols(root string) ([]plugin.Symbol, error) {
	p.mu.Lock()
	if p.root == "" {
		p.root = root
	}
	p.mu.Unlock()

	if p.client != nil {
		syms, err := p.discoverSymbolsFromLSP(root)
		if err == nil && len(syms) > 0 {
			p.cacheSymbols(syms)
			return syms, nil
		}
		log.Printf("go plugin: LSP symbol discovery returned %d symbols (err=%v); falling back to AST", len(syms), err)
	}
	syms, err := p.discoverSymbolsFromAST(root)
	if err == nil {
		p.cacheSymbols(syms)
	}
	return syms, err
}

func (p *Plugin) cacheSymbols(syms []plugin.Symbol) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.symbolsByID = make(map[string]plugin.Symbol, len(syms))
	for _, s := range syms {
		p.symbolsByID[s.ID] = s
	}
}

func (p *Plugin) discoverSymbolsFromLSP(root string) ([]plugin.Symbol, error) {
	var symbols []plugin.Symbol
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		uri := pathToURI(path)
		text, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()

		_ = p.client.DidOpen(ctx, uri, "go", 1, string(text))
		dsyms, derr := p.client.DocumentSymbol(ctx, uri)
		if derr != nil {
			return nil
		}

		pkg := filepath.Base(filepath.Dir(path))
		syms := documentSymbolsToPlugin(dsyms, path, pkg, p.root)
		symbols = append(symbols, syms...)
		return nil
	})

	return symbols, err
}

func (p *Plugin) discoverSymbolsFromAST(root string) ([]plugin.Symbol, error) {
	var symbols []plugin.Symbol
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		pkg := filepath.Base(filepath.Dir(path))
		relFile := p.relativePath(path)

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			pos := fset.Position(fn.Pos())
			kind := "function"
			if fn.Recv != nil {
				kind = "method"
			}
			qualifiedName := fmt.Sprintf("%s.%s", pkg, fn.Name.Name)
			symbols = append(symbols, plugin.Symbol{
				ID:            fmt.Sprintf("go:%s:%s:%d:%d", relFile, qualifiedName, pos.Line, pos.Column),
				Name:          fn.Name.Name,
				QualifiedName: qualifiedName,
				Kind:          kind,
				File:          relFile,
				Line:          pos.Line,
				Column:        pos.Column,
				Package:       pkg,
			})
		}

		return nil
	})

	return symbols, err
}

// ResolveSymbol uses LSP definition to resolve a position to a symbol.
func (p *Plugin) ResolveSymbol(pos plugin.Position) (plugin.Symbol, error) {
	if p.client == nil {
		return plugin.Symbol{}, fmt.Errorf("plugin not initialized")
	}
	uri := pathToURI(pos.File)
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	locs, err := p.client.Definition(ctx, uri, pos.Line-1, pos.Column-1)
	if err != nil {
		return plugin.Symbol{}, err
	}
	if len(locs) == 0 {
		return plugin.Symbol{}, fmt.Errorf("no definition found at %s:%d:%d", pos.File, pos.Line, pos.Column)
	}
	loc := locs[0]
	targetPath := uriToPath(loc.URI)

	dsyms, err := p.client.DocumentSymbol(ctx, loc.URI)
	if err != nil {
		return plugin.Symbol{}, err
	}
	sym := findSymbolAt(dsyms, loc.Range.Start.Line, loc.Range.Start.Character)
	if sym == nil {
		return plugin.Symbol{}, fmt.Errorf("no symbol at definition location")
	}
	pkg := filepath.Base(filepath.Dir(targetPath))
	return makePluginSymbol(*sym, targetPath, pkg, p.root), nil
}

// CallGraph builds a call-graph fragment using LSP call hierarchy.
func (p *Plugin) CallGraph(sym plugin.Symbol) (plugin.Graph, error) {
	if p.client == nil {
		return plugin.Graph{}, fmt.Errorf("plugin not initialized")
	}
	uri := pathToURI(sym.File)
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	items, err := p.client.PrepareCallHierarchy(ctx, uri, sym.Line-1, sym.Column-1)
	if err != nil {
		return plugin.Graph{}, err
	}
	if len(items) == 0 {
		return plugin.Graph{Symbol: sym.ID}, nil
	}
	item := items[0]

	var incoming []string
	incalls, err := p.client.IncomingCalls(ctx, item)
	if err == nil {
		for _, ic := range incalls {
			incoming = append(incoming, p.callHierarchyItemToID(ic.From))
		}
	}

	var outgoing []string
	outcalls, err := p.client.OutgoingCalls(ctx, item)
	if err == nil {
		for _, oc := range outcalls {
			outgoing = append(outgoing, p.callHierarchyItemToID(oc.To))
		}
	}

	return plugin.Graph{
		Symbol:   sym.ID,
		Incoming: incoming,
		Outgoing: outgoing,
	}, nil
}

// RunTests executes go test -json and parses the output.
func (p *Plugin) RunTests(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.RunResult, error) {
	wd := opts.WorkingDir
	if wd == "" {
		wd = p.root
	}

	var runRegex string
	if len(sel.TestIDs) > 0 {
		names := make(map[string]struct{})
		for _, id := range sel.TestIDs {
			parts := strings.Split(id, ":")
			if len(parts) >= 3 {
				names[parts[len(parts)-1]] = struct{}{}
			}
		}
		if len(names) > 0 {
			var list []string
			for n := range names {
				list = append(list, n)
			}
			runRegex = strings.Join(list, "|")
		}
	}

	args := []string{"test", "-json"}
	if runRegex != "" {
		args = append(args, "-run", runRegex)
	}
	if sel.Package != "" {
		args = append(args, sel.Package)
	} else if sel.File != "" {
		pkgDir := filepath.Dir(sel.File)
		args = append(args, "./"+pkgDir)
	} else {
		args = append(args, "./...")
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = wd
	cmd.Env = buildEnv(opts.EnvAllowlist, p.env)

	maxOut := opts.MaxOutputBytes
	if maxOut == 0 {
		maxOut = 1 << 20 // 1MB
	}

	out, err := cmd.CombinedOutput()
	if len(out) > int(maxOut) {
		out = out[:maxOut]
		out = append(out, []byte("\n...truncated...")...)
	}

	result, parseErr := parseGoTestJSON(out)
	if parseErr != nil {
		return plugin.RunResult{}, parseErr
	}
	result.Output = string(out)

	// Map names to stable IDs using cached tests.
	p.mu.RLock()
	testMap := make(map[string]plugin.TestFunc)
	for _, tf := range p.testsByID {
		testMap[tf.Name] = tf
	}
	p.mu.RUnlock()

	for i := range result.Tests {
		if tf, ok := testMap[result.Tests[i].Name]; ok {
			result.Tests[i].ID = tf.ID
		}
	}

	// If the command failed and we couldn't determine pass/fail from JSON,
	// mark as failed.
	if err != nil && !result.Passed && len(result.Tests) == 0 {
		result.Passed = false
	}

	return result, nil
}

// Coverage runs go test with -coverprofile and parses the result.
func (p *Plugin) Coverage(ctx context.Context, sel plugin.TestSelection, opts plugin.RunOptions) (plugin.CoverageReport, error) {
	wd := opts.WorkingDir
	if wd == "" {
		wd = p.root
	}

	tmpFile, err := os.CreateTemp("", "cover-*.out")
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	_ = tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	args := []string{"test", "-coverprofile=" + tmpFile.Name()}
	if sel.Package != "" {
		args = append(args, sel.Package)
	} else if sel.File != "" {
		pkgDir := filepath.Dir(sel.File)
		args = append(args, "./"+pkgDir)
	} else {
		args = append(args, "./...")
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = wd
	cmd.Env = buildEnv(opts.EnvAllowlist, p.env)

	_ = cmd.Run() // coverage file may exist even if tests fail

	report, err := parseCoverProfile(tmpFile.Name())
	if err != nil {
		return plugin.CoverageReport{}, err
	}
	report.Scope = "run"

	// Identify uncovered function symbols.
	symbols, _ := p.DiscoverSymbols(wd)
	for _, sym := range symbols {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		fc, ok := findFileCoverage(report.Files, sym.File)
		if !ok {
			report.Uncovered = append(report.Uncovered, sym.ID)
			continue
		}
		if !isLineCovered(sym.Line, fc.Lines) {
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

// Mutators advertises the external mutation tool wired up for Go.
func (p *Plugin) Mutators() []plugin.Mutator {
	return []plugin.Mutator{gremlinsCatalog}
}

// Generators returns an empty list (v1 deferred).
func (p *Plugin) Generators() []plugin.Generator {
	return nil
}

// Helpers.

func isTestFunction(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	name := fn.Name.Name
	if len(name) < 5 || !strings.HasPrefix(name, "Test") || name[4] < 'A' || name[4] > 'Z' {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkgIdent.Name == "testing" && sel.Sel.Name == "T"
}

func findSubTests(fn *ast.FuncDecl, filePath, qualifiedName string) []plugin.TestCase {
	literalNames := tableCaseNames(fn)
	var cases []plugin.TestCase
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "t" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			name, _ := strconv.Unquote(lit.Value)
			cases = append(cases, makeTestCase(filePath, qualifiedName, name))
			return true
		}
		for _, name := range literalNames {
			cases = append(cases, makeTestCase(filePath, qualifiedName, name))
		}
		return true
	})
	return dedupeTestCases(cases)
}

func makeTestCase(filePath, qualifiedName, name string) plugin.TestCase {
	casePath := escapeIDSegment(name)
	return plugin.TestCase{
		ID:       fmt.Sprintf("go:%s:%s:%s", filePath, qualifiedName, casePath),
		Name:     name,
		CasePath: casePath,
	}
}

func tableCaseNames(fn *ast.FuncDecl) []string {
	var names []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			cl, ok := elt.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, field := range cl.Elts {
				kv, ok := field.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "name" {
					continue
				}
				val, ok := kv.Value.(*ast.BasicLit)
				if !ok || val.Kind != token.STRING {
					continue
				}
				name, err := strconv.Unquote(val.Value)
				if err == nil {
					names = append(names, name)
				}
			}
		}
		return true
	})
	return names
}

func dedupeTestCases(cases []plugin.TestCase) []plugin.TestCase {
	seen := make(map[string]int)
	for i := range cases {
		base := cases[i].CasePath
		if count := seen[base]; count > 0 {
			cases[i].CasePath = fmt.Sprintf("%s_%d", base, count-1)
			cases[i].ID = strings.TrimSuffix(cases[i].ID, base) + cases[i].CasePath
		}
		seen[base]++
	}
	return cases
}

func escapeIDSegment(name string) string {
	name = strings.ReplaceAll(name, "%", "%25")
	name = strings.ReplaceAll(name, ":", "%3A")
	name = strings.ReplaceAll(name, "/", "%2F")
	return name
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + abs
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return uri[len("file://"):]
	}
	return uri
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

type testEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test,omitempty"`
	Output  string  `json:"Output,omitempty"`
	Elapsed float64 `json:"Elapsed,omitempty"`
}

func parseGoTestJSON(data []byte) (plugin.RunResult, error) {
	var result plugin.RunResult
	testMap := make(map[string]*plugin.TestEntry)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var ev testEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			if ev.Action == "pass" || ev.Action == "fail" {
				result.Passed = ev.Action == "pass"
				result.Duration = ev.Elapsed
			}
			continue
		}
		if strings.Contains(ev.Test, "/") {
			continue
		}

		entry, ok := testMap[ev.Test]
		if !ok {
			entry = &plugin.TestEntry{
				ID:   fmt.Sprintf("go:%s:%s", ev.Package, ev.Test),
				Name: ev.Test,
			}
			testMap[ev.Test] = entry
			result.Tests = append(result.Tests, *entry)
		}
		switch ev.Action {
		case "pass", "fail":
			entry.Passed = ev.Action == "pass"
			entry.Duration = ev.Elapsed
		case "output":
			entry.Output += ev.Output
		}
	}

	for i := range result.Tests {
		if updated, ok := testMap[result.Tests[i].Name]; ok {
			result.Tests[i] = *updated
		}
	}

	return result, nil
}

func parseCoverProfile(path string) (plugin.CoverageReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plugin.CoverageReport{}, err
	}

	var files []plugin.FileCoverage
	fileMap := make(map[string]*plugin.FileCoverage)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		filePath := parts[0]
		rest := parts[1]

		semiIdx := strings.Index(rest, " ")
		if semiIdx == -1 {
			continue
		}
		rangePart := rest[:semiIdx]
		countsPart := rest[semiIdx+1:]

		commaIdx := strings.Index(rangePart, ",")
		if commaIdx == -1 {
			continue
		}
		startPart := rangePart[:commaIdx]
		endPart := rangePart[commaIdx+1:]

		startLine, _ := strconv.Atoi(strings.Split(startPart, ".")[0])
		endLine, _ := strconv.Atoi(strings.Split(endPart, ".")[0])

		countParts := strings.Fields(countsPart)
		if len(countParts) < 2 {
			continue
		}
		hitCount, _ := strconv.Atoi(countParts[1])

		fc, ok := fileMap[filePath]
		if !ok {
			fc = &plugin.FileCoverage{Path: filePath}
			fileMap[filePath] = fc
			files = append(files, *fc)
		}
		fc.Lines = append(fc.Lines, plugin.LineRange{
			Start: startLine,
			End:   endLine,
			Hit:   hitCount > 0,
		})
	}

	for i := range files {
		files[i] = *fileMap[files[i].Path]
	}

	totalLines := 0
	hitLines := 0
	for i := range files {
		fc := &files[i]
		fileHit := 0
		fileTotal := 0
		for _, lr := range fc.Lines {
			lines := lr.End - lr.Start + 1
			if lines < 0 {
				lines = 0
			}
			fileTotal += lines
			if lr.Hit {
				fileHit += lines
				hitLines += lines
			}
			totalLines += lines
		}
		if fileTotal > 0 {
			fc.Percentage = float64(fileHit) / float64(fileTotal) * 100
		}
	}

	var percentage float64
	if totalLines > 0 {
		percentage = float64(hitLines) / float64(totalLines) * 100
	}

	return plugin.CoverageReport{
		Scope:      "run",
		Percentage: percentage,
		Files:      files,
	}, nil
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

func documentSymbolsToPlugin(syms []lsp.DocumentSymbol, file, pkg, root string) []plugin.Symbol {
	var out []plugin.Symbol
	for i := range syms {
		ds := &syms[i]
		kind := lspSymbolKindToString(ds.Kind)
		if kind == "function" || kind == "method" {
			out = append(out, makePluginSymbol(*ds, file, pkg, root))
		}
		out = append(out, documentSymbolsToPlugin(ds.Children, file, pkg, root)...)
	}
	return out
}

func makePluginSymbol(ds lsp.DocumentSymbol, file, pkg, root string) plugin.Symbol {
	relFile := file
	if root != "" {
		if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
			relFile = filepath.ToSlash(rel)
		}
	}
	qualifiedName := fmt.Sprintf("%s.%s", pkg, ds.Name)
	return plugin.Symbol{
		ID:            fmt.Sprintf("go:%s:%s:%d:%d", relFile, qualifiedName, ds.Range.Start.Line+1, ds.Range.Start.Character+1),
		Name:          ds.Name,
		QualifiedName: qualifiedName,
		Kind:          lspSymbolKindToString(ds.Kind),
		File:          relFile,
		Line:          ds.Range.Start.Line + 1,
		Column:        ds.Range.Start.Character + 1,
		Package:       pkg,
	}
}

func findSymbolAt(syms []lsp.DocumentSymbol, line, col int) *lsp.DocumentSymbol {
	for i := range syms {
		s := &syms[i]
		if containsPosition(s.Range, line, col) {
			if child := findSymbolAt(s.Children, line, col); child != nil {
				return child
			}
			return s
		}
	}
	return nil
}

func containsPosition(r lsp.Range, line, col int) bool {
	if line < r.Start.Line || line > r.End.Line {
		return false
	}
	if line == r.Start.Line && col < r.Start.Character {
		return false
	}
	if line == r.End.Line && col > r.End.Character {
		return false
	}
	return true
}

func (p *Plugin) callHierarchyItemToID(item lsp.CallHierarchyItem) string {
	path := uriToPath(item.URI)
	relPath := p.relativePath(path)
	pkg := filepath.Base(filepath.Dir(path))
	qualifiedName := fmt.Sprintf("%s.%s", pkg, item.Name)
	return fmt.Sprintf("go:%s:%s:%d:%d", relPath, qualifiedName, item.Range.Start.Line+1, item.Range.Start.Character+1)
}

func lspSymbolKindToString(kind int) string {
	switch kind {
	case 1:
		return "file"
	case 2:
		return "module"
	case 3:
		return "namespace"
	case 4:
		return "package"
	case 5:
		return "class"
	case 6:
		return "method"
	case 7:
		return "property"
	case 8:
		return "field"
	case 9:
		return "constructor"
	case 10:
		return "enum"
	case 11:
		return "interface"
	case 12:
		return "function"
	case 13:
		return "variable"
	case 14:
		return "constant"
	case 15:
		return "string"
	case 16:
		return "number"
	case 17:
		return "boolean"
	case 18:
		return "array"
	case 19:
		return "object"
	case 20:
		return "key"
	case 21:
		return "null"
	case 22:
		return "enumMember"
	case 23:
		return "struct"
	case 24:
		return "event"
	case 25:
		return "operator"
	case 26:
		return "typeParameter"
	default:
		return "unknown"
	}
}
