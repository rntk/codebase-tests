package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/rntk/codebase-tests/internal/golden"
	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
)

// GoldenHandler implements golden file endpoints.
type GoldenHandler struct {
	store map[string]*project.Project
}

// NewGoldenHandler creates a new handler with an empty store.
func NewGoldenHandler() *GoldenHandler {
	return &GoldenHandler{store: make(map[string]*project.Project)}
}

// SetProject registers a project in the store.
func (h *GoldenHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires golden file routes.
func (h *GoldenHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden", h.ListCases)
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden/{caseId}", h.GetCase)
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden/{caseId}/diff", h.DiffCase)
}

func (h *GoldenHandler) getProject(w http.ResponseWriter, r *http.Request) *project.Project {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return nil
	}
	return p
}

// ListCases returns all golden cases for a test.
func (h *GoldenHandler) ListCases(w http.ResponseWriter, r *http.Request) {
	p := h.getProject(w, r)
	if p == nil {
		return
	}
	testId := pathParam(r.PathValue("testId"))

	conv := defaultConvention(p)
	cases, err := golden.ResolveCases(p.Path, testId, conv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondJSON(w, cases)
}

// GetCase returns the current in + out content for a golden case.
func (h *GoldenHandler) GetCase(w http.ResponseWriter, r *http.Request) {
	p := h.getProject(w, r)
	if p == nil {
		return
	}
	testId := pathParam(r.PathValue("testId"))
	caseId := pathParam(r.PathValue("caseId"))

	conv := defaultConvention(p)
	cases, err := golden.ResolveCases(p.Path, testId, conv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var gc *golden.GoldenCase
	for i := range cases {
		if cases[i].ID == caseId || cases[i].Name == caseId {
			gc = &cases[i]
			break
		}
	}
	if gc == nil {
		http.Error(w, "case not found", http.StatusNotFound)
		return
	}

	type response struct {
		ID        string             `json:"id"`
		Name      string             `json:"name"`
		In        json.RawMessage    `json:"in"`
		Out       json.RawMessage    `json:"out"`
		Meta      *golden.GoldenMeta `json:"meta,omitempty"`
		InExists  bool               `json:"inExists"`
		OutExists bool               `json:"outExists"`
	}

	resp := response{
		ID:        gc.ID,
		Name:      gc.Name,
		In:        json.RawMessage("null"),
		Out:       json.RawMessage("null"),
		Meta:      gc.Meta,
		InExists:  gc.InExists,
		OutExists: gc.OutExists,
	}

	if gc.InExists {
		data, err := os.ReadFile(gc.InPath)
		if err == nil {
			resp.In = data
		}
	}
	if gc.OutExists {
		data, err := os.ReadFile(gc.OutPath)
		if err == nil {
			resp.Out = data
		}
	}

	respondJSON(w, resp)
}

// DiffCase returns a structural diff of in + out against git HEAD.
func (h *GoldenHandler) DiffCase(w http.ResponseWriter, r *http.Request) {
	p := h.getProject(w, r)
	if p == nil {
		return
	}
	testId := pathParam(r.PathValue("testId"))
	caseId := pathParam(r.PathValue("caseId"))

	conv := defaultConvention(p)
	cases, err := golden.ResolveCases(p.Path, testId, conv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var gc *golden.GoldenCase
	for i := range cases {
		if cases[i].ID == caseId || cases[i].Name == caseId {
			gc = &cases[i]
			break
		}
	}
	if gc == nil {
		http.Error(w, "case not found", http.StatusNotFound)
		return
	}

	type diffResponse struct {
		ID      string           `json:"id"`
		Name    string           `json:"name"`
		InDiff  *golden.DiffNode `json:"inDiff,omitempty"`
		OutDiff *golden.DiffNode `json:"outDiff,omitempty"`
	}

	resp := diffResponse{ID: gc.ID, Name: gc.Name}

	if gc.InExists {
		current, err := os.ReadFile(gc.InPath)
		if err == nil {
			prev, err := golden.PreviousVersion(gc.InPath)
			if err == nil {
				var oldVal, newVal any
				_ = json.Unmarshal(prev, &oldVal)
				_ = json.Unmarshal(current, &newVal)
				resp.InDiff = golden.DiffJSON(oldVal, newVal)
			} else if errors.Is(err, golden.ErrNotTracked) {
				var newVal any
				_ = json.Unmarshal(current, &newVal)
				resp.InDiff = &golden.DiffNode{Kind: "added", NewValue: newVal}
			}
		}
	}

	if gc.OutExists {
		current, err := os.ReadFile(gc.OutPath)
		if err == nil {
			prev, err := golden.PreviousVersion(gc.OutPath)
			if err == nil {
				var oldVal, newVal any
				_ = json.Unmarshal(prev, &oldVal)
				_ = json.Unmarshal(current, &newVal)
				resp.OutDiff = golden.DiffJSON(oldVal, newVal)
			} else if errors.Is(err, golden.ErrNotTracked) {
				var newVal any
				_ = json.Unmarshal(current, &newVal)
				resp.OutDiff = &golden.DiffNode{Kind: "added", NewValue: newVal}
			}
		}
	}

	respondJSON(w, resp)
}

func defaultConvention(p *project.Project) plugin.GoldenConvention {
	return plugin.GoldenConvention{
		Root:        p.GoldenRoot,
		SegmentFunc: "qualifiedName",
		CaseSegment: "file",
	}
}
