package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/review-server/internal/plugin"
	"github.com/review-server/internal/project"
	"github.com/review-server/internal/tests"
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
func (h *SymbolsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/projects/{id}/symbols", h.List)
}

// List returns the function tree with coverage annotations.
func (h *SymbolsHandler) List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data, err := h.coverage.Build(r.Context(), p.Path, defaultRunOptions(p))
	if err != nil {
		// Fallback to symbol discovery without coverage
		syms, _ := tests.DiscoverSymbols(h.coverage.Registry(), p.Path)
		if syms == nil {
			syms = []plugin.Symbol{}
		}
		for i := range syms {
			syms[i].Covered = true
		}
		respondJSON(w, syms)
		return
	}

	for i := range data.Symbols {
		if ids, ok := data.StaticMap[data.Symbols[i].ID]; ok && len(ids) > 0 {
			data.Symbols[i].CoveredBy = ids
			data.Symbols[i].Covered = true
		}
	}
	respondJSON(w, data.Symbols)
}
