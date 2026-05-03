package api

import (
	"net/http"
	"strings"

	"github.com/review-server/internal/golden"
	"github.com/review-server/internal/plugin"
	"github.com/review-server/internal/project"
	"github.com/review-server/internal/tests"
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
	respondJSON(w, all)
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
	if covered := coveredFunctionSources(p.Path, h.discovery.Registry(), t.CoveredFuncs); len(covered) > 0 {
		resp.CoveredFuncSources = covered
	} else {
		resp.CoveredFuncSources = referencedSymbolSources(p.Path, h.discovery.Registry(), resp.SourceCode, t.ID)
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

func coveredFunctionSources(projectPath string, registry *plugin.Registry, coveredFuncIDs []string) []sourceSnippet {
	if len(coveredFuncIDs) == 0 || registry == nil {
		return nil
	}
	symbols, err := tests.DiscoverSymbols(registry, projectPath)
	if err != nil {
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
		if code, err := readSourceSnippet(projectPath, sym.File, sym.Line); err == nil {
			item.SourceCode = code
		}
		out = append(out, item)
	}
	return out
}

// referencedSymbolSources scans the given test source code for any LSP symbols
// (functions or methods) whose name is referenced lexically. Used as a fallback
// when t.CoveredFuncs is unpopulated by the language plugin.
func referencedSymbolSources(projectPath string, registry *plugin.Registry, sourceCode, testID string) []sourceSnippet {
	if sourceCode == "" || registry == nil {
		return nil
	}
	symbols, err := tests.DiscoverSymbols(registry, projectPath)
	if err != nil {
		return nil
	}
	var out []sourceSnippet
	seen := make(map[string]bool)
	for _, sym := range symbols {
		if sym.ID == testID {
			continue
		}
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		if seen[sym.ID] {
			continue
		}
		if !nameOccursAsIdent(sym.Name, sourceCode) && !nameOccursAsIdent(sym.QualifiedName, sourceCode) {
			continue
		}
		seen[sym.ID] = true
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
		if code, err := readSourceSnippet(projectPath, sym.File, sym.Line); err == nil {
			item.SourceCode = code
		}
		out = append(out, item)
	}
	return out
}

// nameOccursAsIdent returns true if name appears in code at an identifier boundary
// (not preceded or followed by an identifier character). For dotted names like
// "pkg.Foo", the dot is allowed inside the match but boundaries still apply at
// the outer edges.
func nameOccursAsIdent(name, code string) bool {
	if name == "" {
		return false
	}
	idx := 0
	for {
		rel := strings.Index(code[idx:], name)
		if rel < 0 {
			return false
		}
		i := idx + rel
		end := i + len(name)
		var before, after byte = ' ', ' '
		if i > 0 {
			before = code[i-1]
		}
		if end < len(code) {
			after = code[end]
		}
		if !isIdentByte(before) && !isIdentByte(after) {
			return true
		}
		idx = i + 1
	}
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}
