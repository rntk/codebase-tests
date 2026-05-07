package api

import (
	"net/http"

	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/tests"
)

// SymbolsHandler implements the symbols endpoint.
type SymbolsHandler struct {
	store    map[string]*project.Project
	coverage *tests.Coverage
}

// NewSymbolsHandler creates a handler.
func NewSymbolsHandler(coverage *tests.Coverage) *SymbolsHandler {
	return &SymbolsHandler{
		store:    make(map[string]*project.Project),
		coverage: coverage,
	}
}

// SetProject registers a project.
func (h *SymbolsHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires symbol routes.
func (h *SymbolsHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/symbols", h.List)
}

// symbolWithSource extends plugin.Symbol with optional inline source code.
type symbolWithSource struct {
	plugin.Symbol
	SourceCode string `json:"sourceCode,omitempty"`
}

// List returns the function tree with coverage annotations.
func (h *SymbolsHandler) List(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	withSource := r.URL.Query().Get("withSource") == "true"
	language := r.URL.Query().Get("language")

	// Use the fast path: symbols + static call-graph only, no test runs.
	// Run-level coverage refinement happens via /coverage when needed.
	data, err := h.coverage.BuildSymbols(r.Context(), p.Path)
	if err != nil {
		// Fallback to symbol discovery without coverage
		syms, _ := tests.DiscoverSymbols(h.coverage.Registry(), p.Path)
		syms = symbolsForLanguage(syms, language)
		if syms == nil {
			syms = []plugin.Symbol{}
		}
		for i := range syms {
			syms[i].Covered = true
		}
		respondJSON(w, attachSource(syms, p.Path, withSource))
		return
	}

	for i := range data.Symbols {
		if ids, ok := data.StaticMap[data.Symbols[i].ID]; ok && len(ids) > 0 {
			data.Symbols[i].CoveredBy = ids
			data.Symbols[i].Covered = true
		}
	}
	respondJSON(w, attachSource(symbolsForLanguage(data.Symbols, language), p.Path, withSource))
}

func symbolsForLanguage(syms []plugin.Symbol, language string) []plugin.Symbol {
	if language == "" {
		return syms
	}
	out := make([]plugin.Symbol, 0, len(syms))
	for _, sym := range syms {
		if languageFromID(sym.ID) == language {
			out = append(out, sym)
		}
	}
	return out
}

// maxInlineSourceBytes caps the cumulative size of inline source attached to
// a single /symbols response so that large projects don't ship megabytes of
// code. Symbols past the cap omit SourceCode; the client can fetch them
// individually if needed.
const maxInlineSourceBytes = 512 * 1024

func attachSource(syms []plugin.Symbol, projectPath string, withSource bool) []symbolWithSource {
	out := make([]symbolWithSource, len(syms))
	if !withSource {
		for i, s := range syms {
			out[i] = symbolWithSource{Symbol: s}
		}
		return out
	}
	reader := newSnippetCache(projectPath)
	budget := maxInlineSourceBytes
	for i, s := range syms {
		out[i] = symbolWithSource{Symbol: s}
		if s.Kind != "function" && s.Kind != "method" {
			continue
		}
		if budget <= 0 {
			continue
		}
		code, err := reader.Read(s.File, s.Line)
		if err != nil {
			continue
		}
		out[i].SourceCode = code
		budget -= len(code)
	}
	return out
}
