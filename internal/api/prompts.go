package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rntk/codebase-tests/internal/golden"
	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/prompts"
	"github.com/rntk/codebase-tests/internal/tests"
)

// PromptsHandler returns ready-to-paste test-generation prompts.
type PromptsHandler struct {
	store     map[string]*project.Project
	registry  *plugin.Registry
	discovery *tests.Discovery
}

// NewPromptsHandler creates a handler.
func NewPromptsHandler(reg *plugin.Registry, d *tests.Discovery) *PromptsHandler {
	return &PromptsHandler{
		store:     make(map[string]*project.Project),
		registry:  reg,
		discovery: d,
	}
}

// SetProject registers a project.
func (h *PromptsHandler) SetProject(p *project.Project) { registerProject(h.store, p) }

// RegisterRoutes wires prompt routes.
func (h *PromptsHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/symbols/{symbolId}/test-prompt", h.ForSymbol)
	r.HandleFunc("GET /projects/{id}/tests/{testId}/test-prompt", h.ForTest)
}

// PromptResponse is the JSON body returned to the UI.
type PromptResponse struct {
	Kind      string `json:"kind"` // "create" | "transform" | "supported"
	Language  string `json:"language"`
	Qualified string `json:"qualified"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	TestFile  string `json:"testFile,omitempty"`
	GoldenDir string `json:"goldenDir"`
	Prompt    string `json:"prompt"`
	Note      string `json:"note,omitempty"`
}

// ForSymbol returns the right prompt for a function symbol.
func (h *PromptsHandler) ForSymbol(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r.PathValue("id"))
	symbolID := pathParam(r.PathValue("symbolId"))
	p, ok := h.store[projectID]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	syms, err := tests.DiscoverSymbols(h.registry, p.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var sym *plugin.Symbol
	for i := range syms {
		if syms[i].ID == symbolID {
			sym = &syms[i]
			break
		}
	}
	if sym == nil {
		http.Error(w, "symbol not found", http.StatusNotFound)
		return
	}

	language := languageFromID(sym.ID)
	pl, _ := h.registry.For(language)
	conv := conventionFor(p, pl)
	goldenDir := goldenDirForQualified(p, conv, sym.QualifiedName)
	relSrc := relPath(p.Path, sym.File)

	covering := h.testsCovering(p.Path, sym.ID)
	supported := h.anyTestHasGolden(p.Path, covering, conv)

	in := prompts.Inputs{
		Language:      language,
		Qualified:     sym.QualifiedName,
		File:          relSrc,
		Line:          sym.Line,
		GoldenDir:     goldenDir,
		LanguageHints: hintsFor(pl, language),
	}

	resp := PromptResponse{
		Language:  language,
		Qualified: sym.QualifiedName,
		File:      relSrc,
		Line:      sym.Line,
		GoldenDir: goldenDir,
	}

	switch {
	case supported:
		resp.Kind = "supported"
		resp.Note = "This function already has tests with golden files in the supported format."
		if len(covering) > 0 {
			resp.TestFile = relPath(p.Path, covering[0].File)
		}
		respondJSON(w, resp)
		return
	case len(covering) > 0:
		in.Kind = prompts.KindTransform
		in.TestFile = relPath(p.Path, covering[0].File)
		resp.Kind = "transform"
		resp.TestFile = in.TestFile
	default:
		in.Kind = prompts.KindCreate
		in.TestFile = suggestedTestFile(language, relSrc)
		resp.Kind = "create"
		resp.TestFile = in.TestFile
	}

	text, err := prompts.Render(in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp.Prompt = text
	respondJSON(w, resp)
}

// ForTest returns a transform prompt for an existing test function.
func (h *PromptsHandler) ForTest(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r.PathValue("id"))
	testID := pathParam(r.PathValue("testId"))
	p, ok := h.store[projectID]
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	t, found := h.discovery.FindByID(p.Path, testID)
	if !found {
		http.Error(w, "test not found", http.StatusNotFound)
		return
	}

	language := languageFromID(t.ID)
	pl, _ := h.registry.For(language)
	conv := conventionFor(p, pl)

	_, _, qualified, _, perr := golden.ParseTestID(t.ID)
	if perr != nil {
		http.Error(w, perr.Error(), http.StatusInternalServerError)
		return
	}
	qualified = golden.UnescapeSegment(qualified)
	goldenDir := goldenDirForQualified(p, conv, qualified)
	relTest := relPath(p.Path, t.File)

	cases, _ := golden.ResolveCases(p.Path, t.ID, conv)
	resp := PromptResponse{
		Language:  language,
		Qualified: qualified,
		File:      relTest,
		Line:      t.Line,
		TestFile:  relTest,
		GoldenDir: goldenDir,
	}
	if len(cases) > 0 {
		resp.Kind = "supported"
		resp.Note = "This test already has golden cases in the supported format."
		respondJSON(w, resp)
		return
	}

	in := prompts.Inputs{
		Kind:          prompts.KindTransform,
		Language:      language,
		Qualified:     qualified,
		File:          relTest,
		Line:          t.Line,
		TestFile:      relTest,
		GoldenDir:     goldenDir,
		LanguageHints: hintsFor(pl, language),
	}
	text, err := prompts.Render(in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp.Kind = "transform"
	resp.Prompt = text
	respondJSON(w, resp)
}

func (h *PromptsHandler) testsCovering(projectPath, symbolID string) []plugin.TestFunc {
	all, _ := h.discovery.DiscoverAll(projectPath)
	var out []plugin.TestFunc
	for _, t := range all {
		for _, c := range t.CoveredFuncs {
			if c == symbolID {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func (h *PromptsHandler) anyTestHasGolden(projectPath string, ts []plugin.TestFunc, conv plugin.GoldenConvention) bool {
	for _, t := range ts {
		cases, err := golden.ResolveCases(projectPath, t.ID, conv)
		if err == nil && len(cases) > 0 {
			return true
		}
	}
	return false
}

func languageFromID(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i]
	}
	return ""
}

func relPath(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

func conventionFor(p *project.Project, pl plugin.Plugin) plugin.GoldenConvention {
	conv := defaultConvention(p)
	if pl != nil {
		if layouter, ok := pl.(plugin.GoldenLayouter); ok {
			c := layouter.GoldenLayout()
			if c.Root != "" {
				conv.Root = c.Root
			}
			if c.SegmentFunc != "" {
				conv.SegmentFunc = c.SegmentFunc
			}
			if c.CaseSegment != "" {
				conv.CaseSegment = c.CaseSegment
			}
		}
	}
	return conv
}

func goldenDirForQualified(p *project.Project, conv plugin.GoldenConvention, qualified string) string {
	root := conv.Root
	if root == "" {
		root = p.GoldenRoot
	}
	if conv.SegmentFunc == "packagePath" {
		return filepath.Join(root, golden.EscapeSegment(qualified))
	}
	parts := strings.Split(qualified, ".")
	segs := make([]string, 0, len(parts))
	for _, part := range parts {
		segs = append(segs, golden.EscapeSegment(part))
	}
	return filepath.Join(append([]string{root}, segs...)...)
}

func hintsFor(pl plugin.Plugin, language string) string {
	if h, ok := pl.(plugin.PromptHinter); ok {
		if s := h.TestPromptHints(); s != "" {
			return s
		}
	}
	return prompts.HintsFor(language)
}

// suggestedTestFile is a hint for the LLM when no existing test file is known.
func suggestedTestFile(language, relSrc string) string {
	dir := filepath.Dir(relSrc)
	base := strings.TrimSuffix(filepath.Base(relSrc), filepath.Ext(relSrc))
	switch language {
	case "go":
		return filepath.Join(dir, base+"_test.go")
	case "python":
		return filepath.Join(dir, "test_"+base+".py")
	default:
		return ""
	}
}
