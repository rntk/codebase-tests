package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/review-server/internal/golden"
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
func (h *TestsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/projects/{id}/tests", h.List)
	r.Get("/projects/{id}/tests/{testId}", h.Get)
}

// List returns the test tree.
func (h *TestsHandler) List(w http.ResponseWriter, r *http.Request) {
	id := pathParam(chi.URLParam(r, "id"))
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
	id := pathParam(chi.URLParam(r, "id"))
	testID := pathParam(chi.URLParam(r, "testId"))
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
		ID           string              `json:"id"`
		Name         string              `json:"name"`
		File         string              `json:"file"`
		Line         int                 `json:"line"`
		Column       int                 `json:"column"`
		Package      string              `json:"package"`
		SubCases     []goldenTestCaseRef `json:"subCases,omitempty"`
		CoveredFuncs []string            `json:"coveredFuncs,omitempty"`
		GoldenCases  []golden.GoldenCase `json:"goldenCases,omitempty"`
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
