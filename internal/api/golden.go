package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/rntk/codebase-tests/internal/golden"
	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
)

// maxMutations caps the number of mutations executed per request.
const maxMutations = 500

// GoldenHandler implements golden file endpoints.
type GoldenHandler struct {
	store    map[string]*project.Project
	registry *plugin.Registry

	mu        sync.Mutex
	caseLocks map[string]*sync.Mutex
}

// NewGoldenHandler creates a new handler.
func NewGoldenHandler(registry *plugin.Registry) *GoldenHandler {
	return &GoldenHandler{
		store:     make(map[string]*project.Project),
		registry:  registry,
		caseLocks: make(map[string]*sync.Mutex),
	}
}

// lockFor returns a per-path mutex so concurrent mutation runs on the
// same case can't clobber each other's backups.
func (h *GoldenHandler) lockFor(path string) *sync.Mutex {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.caseLocks[path]; ok {
		return m
	}
	m := &sync.Mutex{}
	h.caseLocks[path] = m
	return m
}

// writeAtomic writes data to path via a temp file + rename, preserving mode.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mutate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
	r.HandleFunc("POST /projects/{id}/tests/{testId}/golden/{caseId}/mutate", h.MutateCase)
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

// MutateCase runs mutation tests for a single golden case.
func (h *GoldenHandler) MutateCase(w http.ResponseWriter, r *http.Request) {
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

	if !gc.InExists {
		http.Error(w, "input file not found", http.StatusBadRequest)
		return
	}

	lock := h.lockFor(gc.InPath)
	lock.Lock()
	defer lock.Unlock()

	info, err := os.Stat(gc.InPath)
	if err != nil {
		http.Error(w, "stat input: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mode := info.Mode().Perm()

	data, err := os.ReadFile(gc.InPath)
	if err != nil {
		http.Error(w, "read input: "+err.Error(), http.StatusInternalServerError)
		return
	}

	mutations, err := golden.MutateJSON(data)
	if err != nil {
		http.Error(w, "generate mutations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(mutations) > maxMutations {
		mutations = mutations[:maxMutations]
	}

	pluginName, _, _, _, _ := golden.ParseTestID(testId)
	pl, err := h.registry.For(pluginName)
	if err != nil {
		http.Error(w, "plugin not found: "+pluginName, http.StatusInternalServerError)
		return
	}

	original := make([]byte, len(data))
	copy(original, data)
	defer func() {
		_ = writeAtomic(gc.InPath, original, mode)
	}()

	ctx := r.Context()
	results := make([]golden.MutationResult, 0, len(mutations))
	for _, m := range mutations {
		if err := ctx.Err(); err != nil {
			return
		}

		if err := writeAtomic(gc.InPath, m.Data, mode); err != nil {
			results = append(results, golden.MutationResult{
				Mutation: m.Description,
				Survived: false,
				Output:   "failed to write mutated file: " + err.Error(),
			})
			continue
		}

		runner, ok := pl.(plugin.TestRunner)
		if !ok {
			results = append(results, golden.MutationResult{
				Mutation: m.Description,
				Survived: false,
				Output:   "plugin does not support running tests",
			})
			continue
		}

		res, err := runner.RunTests(ctx, plugin.TestSelection{
			TestIDs: []string{testId},
		}, defaultRunOptions(p))

		survived := false
		output := ""
		if err != nil {
			output = err.Error()
		} else {
			survived = res.Passed
			output = res.Output
		}

		results = append(results, golden.MutationResult{
			Mutation: m.Description,
			Survived: survived,
			Output:   output,
		})
	}

	respondJSON(w, results)
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
