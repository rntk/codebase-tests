package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/review-server/internal/project"
)

// ProjectsHandler implements project endpoints.
type ProjectsHandler struct {
	store map[string]*project.Project
}

// NewProjectsHandler creates a handler with an empty store.
func NewProjectsHandler() *ProjectsHandler {
	return &ProjectsHandler{store: make(map[string]*project.Project)}
}

// SetProject registers a project in the store.
func (h *ProjectsHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires project routes.
func (h *ProjectsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/projects", h.List)
	r.Post("/projects", h.Create)
	r.Get("/projects/{id}", h.Get)
}

// List returns registered projects.
func (h *ProjectsHandler) List(w http.ResponseWriter, r *http.Request) {
	var out []*project.Project
	seen := make(map[*project.Project]bool)
	for _, p := range h.store {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	respondJSON(w, out)
}

// Get returns a single project.
func (h *ProjectsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(chi.URLParam(r, "id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	respondJSON(w, p)
}

// Create registers a new project.
func (h *ProjectsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string                   `json:"path"`
		Name    string                   `json:"name"`
		Plugins []project.PluginSettings `json:"plugins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	opts := project.InitOptions{
		Name:               req.Name,
		Plugins:            req.Plugins,
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 30,
	}
	p, err := project.InitProject(req.Path, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registerProject(h.store, p)
	respondJSON(w, p)
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
