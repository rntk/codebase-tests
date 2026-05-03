package api

import (
	"net/http"

	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/tests"
)

// CoverageHandler implements the coverage endpoint.
type CoverageHandler struct {
	store    map[string]*project.Project
	coverage *tests.Coverage
}

// NewCoverageHandler creates a handler.
func NewCoverageHandler(coverage *tests.Coverage) *CoverageHandler {
	return &CoverageHandler{
		store:    make(map[string]*project.Project),
		coverage: coverage,
	}
}

// SetProject registers a project.
func (h *CoverageHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires coverage routes.
func (h *CoverageHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/coverage", h.Get)
}

// Get returns coverage summary and uncovered functions.
func (h *CoverageHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data, err := h.coverage.Build(r.Context(), p.Path, defaultRunOptions(p))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, data.Report)
}

func defaultRunOptions(p *project.Project) plugin.RunOptions {
	return plugin.RunOptions{
		Timeout:        p.ToolTimeoutSeconds,
		WorkingDir:     p.Path,
		MaxOutputBytes: 1 << 20,
	}
}
