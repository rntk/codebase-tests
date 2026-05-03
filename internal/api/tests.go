package api

import (
	"go/scanner"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/rntk/codebase-tests/internal/golden"
	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/tests"
)

// TestsHandler implements test endpoints.
type TestsHandler struct {
	store     map[string]*project.Project
	discovery *tests.Discovery
}

// NewTestsHandler creates a handler.
func NewTestsHandler(discovery *tests.Discovery) *TestsHandler {
	return &TestsHandler{
		store:     make(map[string]*project.Project),
		discovery: discovery,
	}
}

// SetProject registers a project.
func (h *TestsHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires test routes.
func (h *TestsHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/tests", h.List)
	r.HandleFunc("GET /projects/{id}/tests/{testId}", h.Get)
}

// List returns the test tree.
func (h *TestsHandler) List(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	all, err := h.discovery.DiscoverAll(p.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	conv := defaultConvention(p)
	resp := make([]testListItem, 0, len(all))
	for _, t := range all {
		item := testListItem{
			ID:           t.ID,
			Name:         t.Name,
			File:         t.File,
			Line:         t.Line,
			Column:       t.Column,
			Package:      t.Package,
			SubCases:     t.SubCases,
			CoveredFuncs: t.CoveredFuncs,
		}
		if cases, err := golden.ResolveCases(p.Path, t.ID, conv); err == nil && len(cases) > 0 {
			item.HasGolden = true
		}
		resp = append(resp, item)
	}
	respondJSON(w, resp)
}

type testListItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	File         string            `json:"file"`
	Line         int               `json:"line"`
	Column       int               `json:"column"`
	Package      string            `json:"package"`
	HasGolden    bool              `json:"hasGolden,omitempty"`
	SubCases     []plugin.TestCase `json:"subCases,omitempty"`
	CoveredFuncs []string          `json:"coveredFuncs,omitempty"`
}

// Get returns a single test with detail.
func (h *TestsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	testID := pathParam(r.PathValue("testId"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	t, found := h.discovery.FindByID(p.Path, testID)
	if !found {
		http.Error(w, "test not found", http.StatusNotFound)
		return
	}
	type detailResponse struct {
		ID                 string              `json:"id"`
		Name               string              `json:"name"`
		File               string              `json:"file"`
		Line               int                 `json:"line"`
		Column             int                 `json:"column"`
		Package            string              `json:"package"`
		SubCases           []goldenTestCaseRef `json:"subCases,omitempty"`
		CoveredFuncs       []string            `json:"coveredFuncs,omitempty"`
		SourceCode         string              `json:"sourceCode,omitempty"`
		CoveredFuncSources []sourceSnippet     `json:"coveredFuncSources,omitempty"`
		GoldenCases        []golden.GoldenCase `json:"goldenCases,omitempty"`
	}
	resp := detailResponse{
		ID:           t.ID,
		Name:         t.Name,
		File:         t.File,
		Line:         t.Line,
		Column:       t.Column,
		Package:      t.Package,
		CoveredFuncs: t.CoveredFuncs,
	}
	if code, err := readSourceSnippet(p.Path, t.File, t.Line); err == nil {
		resp.SourceCode = code
	}

	registry := h.discovery.Registry()
	var symbols []plugin.Symbol
	if registry != nil {
		if syms, err := tests.DiscoverSymbols(registry, p.Path); err == nil {
			symbols = symbolsForLanguage(syms, languageFromID(t.ID))
		}
	}
	reader := newSnippetCache(p.Path)
	if covered := coveredFunctionSources(reader, symbols, t.CoveredFuncs); len(covered) > 0 {
		resp.CoveredFuncSources = covered
	} else {
		resp.CoveredFuncSources = referencedSymbolSources(reader, symbols, resp.SourceCode, t.ID, t.File)
	}
	for _, c := range t.SubCases {
		resp.SubCases = append(resp.SubCases, goldenTestCaseRef{
			ID:       c.ID,
			Name:     c.Name,
			CasePath: c.CasePath,
		})
	}
	if cases, err := golden.ResolveCases(p.Path, testID, defaultConvention(p)); err == nil {
		resp.GoldenCases = cases
	}
	respondJSON(w, resp)
}

type goldenTestCaseRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CasePath string `json:"casePath"`
}

type sourceSnippet struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	Kind          string `json:"kind"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	Package       string `json:"package"`
	SourceCode    string `json:"sourceCode,omitempty"`
}

func coveredFunctionSources(reader *snippetCache, symbols []plugin.Symbol, coveredFuncIDs []string) []sourceSnippet {
	if len(coveredFuncIDs) == 0 || len(symbols) == 0 {
		return nil
	}
	covered := make(map[string]struct{}, len(coveredFuncIDs))
	for _, id := range coveredFuncIDs {
		covered[id] = struct{}{}
	}
	var out []sourceSnippet
	for _, sym := range symbols {
		if _, ok := covered[sym.ID]; !ok {
			continue
		}
		out = append(out, snippetFromSymbol(reader, sym))
	}
	return out
}

// referencedSymbolSources scans the given test source code for any project
// symbols (functions or methods) whose name is used as an identifier. Strings
// and comments are skipped via go/scanner tokenization, so false positives
// from string literals or comments are avoided. Used as a fallback when the
// language plugin doesn't populate t.CoveredFuncs.
func referencedSymbolSources(reader *snippetCache, symbols []plugin.Symbol, sourceCode, testID, testFile string) []sourceSnippet {
	if sourceCode == "" || len(symbols) == 0 {
		return nil
	}
	idents, qualified := goReferencedNames(sourceCode)
	if len(idents) == 0 && len(qualified) == 0 {
		return nil
	}
	testPkg := filepath.ToSlash(filepath.Dir(testFile))
	var out []sourceSnippet
	seen := make(map[string]bool)
	for _, sym := range symbols {
		if sym.ID == testID || seen[sym.ID] {
			continue
		}
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		// Match qualified name (e.g. pkg.Foo) anywhere; match bare name only
		// when the symbol is in the same package as the test (avoiding cross-
		// package collisions for common short names like Close/Read/New).
		matched := qualified[sym.QualifiedName]
		if !matched && idents[sym.Name] {
			symPkg := filepath.ToSlash(filepath.Dir(sym.File))
			if symPkg == testPkg {
				matched = true
			}
		}
		if !matched {
			continue
		}
		seen[sym.ID] = true
		out = append(out, snippetFromSymbol(reader, sym))
	}
	return out
}

// goReferencedNames tokenizes Go source and returns (a) the set of all
// identifier tokens that appear, and (b) the set of "x.y" qualified names
// formed by IDENT '.' IDENT sequences. String and comment contents are
// ignored automatically because go/scanner emits them as STRING/COMMENT
// tokens, not IDENT.
func goReferencedNames(src string) (idents, qualified map[string]bool) {
	idents = make(map[string]bool)
	qualified = make(map[string]bool)

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, []byte(src), nil, 0)

	var prevIdent string
	var prevWasDot bool
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		switch tok {
		case token.IDENT:
			idents[lit] = true
			if prevWasDot && prevIdent != "" {
				qualified[prevIdent+"."+lit] = true
			}
			prevIdent = lit
			prevWasDot = false
		case token.PERIOD:
			prevWasDot = true
		default:
			prevIdent = ""
			prevWasDot = false
		}
	}
	return idents, qualified
}

func snippetFromSymbol(reader *snippetCache, sym plugin.Symbol) sourceSnippet {
	item := sourceSnippet{
		ID:            sym.ID,
		Name:          sym.Name,
		QualifiedName: sym.QualifiedName,
		Kind:          sym.Kind,
		File:          sym.File,
		Line:          sym.Line,
		Column:        sym.Column,
		Package:       sym.Package,
	}
	if code, err := reader.Read(sym.File, sym.Line); err == nil {
		item.SourceCode = code
	}
	return item
}

// snippetCache memoizes file reads and per-(file,line) snippet extraction so
// a single handler call doesn't re-open the same file or re-extract the same
// snippet.
type snippetCache struct {
	projectPath string
	files       map[string]cachedFile
	snippets    map[string]string
}

type cachedFile struct {
	cleanRel string
	lines    []string
	err      error
}

func newSnippetCache(projectPath string) *snippetCache {
	return &snippetCache{
		projectPath: projectPath,
		files:       make(map[string]cachedFile),
		snippets:    make(map[string]string),
	}
}

func (c *snippetCache) Read(file string, line int) (string, error) {
	key := file + "\x00" + strconv.Itoa(line)
	if v, ok := c.snippets[key]; ok {
		return v, nil
	}
	cf, ok := c.files[file]
	if !ok {
		cleanRel, lines, err := readProjectFileLines(c.projectPath, file)
		cf = cachedFile{cleanRel: cleanRel, lines: lines, err: err}
		c.files[file] = cf
	}
	if cf.err != nil {
		return "", cf.err
	}
	code, err := snippetFromLines(cf.lines, filepath.Ext(cf.cleanRel), file, line)
	if err != nil {
		return "", err
	}
	c.snippets[key] = code
	return code, nil
}
