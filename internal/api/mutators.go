package api

import (
	"encoding/json"
	"net/http"

	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
)

// MutatorsHandler exposes mutation-testing endpoints. Mutation execution is
// delegated to language-specific external tools (Gremlins, StrykerJS, ...);
// this handler only orchestrates discovery and dispatch.
type MutatorsHandler struct {
	store    map[string]*project.Project
	registry *plugin.Registry
}

// NewMutatorsHandler creates a new handler.
func NewMutatorsHandler(registry *plugin.Registry) *MutatorsHandler {
	return &MutatorsHandler{
		store:    make(map[string]*project.Project),
		registry: registry,
	}
}

// SetProject registers a project.
func (h *MutatorsHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires mutation routes.
func (h *MutatorsHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/mutators", h.List)
	r.HandleFunc("POST /projects/{id}/mutation-run", h.Run)
}

type mutatorListItem struct {
	plugin.Mutator
	Language string `json:"language"`
}

// List returns the mutation tools available across registered plugins.
func (h *MutatorsHandler) List(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var items []mutatorListItem
	for _, cfg := range p.Plugins {
		pl, err := h.registry.For(cfg.Name)
		if err != nil {
			continue
		}
		provider, ok := pl.(plugin.MutatorProvider)
		if !ok {
			continue
		}
		for _, m := range provider.Mutators() {
			items = append(items, mutatorListItem{Mutator: m, Language: cfg.Name})
		}
	}
	respondJSON(w, items)
}

// mutationRunRequest is the body for POST /projects/{id}/mutation-run.
type mutationRunRequest struct {
	Language string   `json:"language"`           // required: which plugin to invoke
	Files    []string `json:"files,omitempty"`    // optional scope
	Packages []string `json:"packages,omitempty"` // optional scope
}

// Run delegates a full mutation run to the external tool wrapped by the
// language plugin. Tools manage their own sandboxing and parallelism.
func (h *MutatorsHandler) Run(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req mutationRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		http.Error(w, "language is required", http.StatusBadRequest)
		return
	}

	pl, err := h.registry.For(req.Language)
	if err != nil {
		http.Error(w, "plugin not found: "+req.Language, http.StatusBadRequest)
		return
	}
	runner, ok := pl.(plugin.MutationRunner)
	if !ok {
		http.Error(w, "plugin does not support mutation testing: "+req.Language, http.StatusBadRequest)
		return
	}

	scope := plugin.MutationScope{Files: req.Files, Packages: req.Packages}
	report, err := runner.RunMutations(r.Context(), scope, defaultRunOptions(p))
	if err != nil {
		// Still return the partial report (with tool output) for diagnostics.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(struct {
			Error  string                   `json:"error"`
			Report plugin.MutationRunReport `json:"report"`
		}{Error: err.Error(), Report: report})
		return
	}
	respondJSON(w, report)
}
