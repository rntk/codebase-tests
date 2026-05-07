package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rntk/codebase-tests/internal/plugin"
)

// Coverage holds coverage-related data for a project.
type Coverage struct {
	mu       sync.RWMutex
	cache    map[string]*CoverageData // project path -> data
	registry *plugin.Registry
}

// CoverageData is the computed coverage for a project.
type CoverageData struct {
	Symbols   []plugin.Symbol
	Tests     []plugin.TestFunc
	StaticMap map[string][]string // symbol ID -> []test IDs that statically reach it
	Uncovered []string            // symbol IDs with no coverage
	Report    plugin.CoverageReport

	// hasReport tracks whether the run-level coverage report has been
	// computed. BuildSymbols leaves it false; Build sets it true.
	hasReport bool
}

// NewCoverage creates a coverage service.
func NewCoverage(registry *plugin.Registry) *Coverage {
	return &Coverage{
		cache:    make(map[string]*CoverageData),
		registry: registry,
	}
}

// Registry returns the underlying plugin registry.
func (c *Coverage) Registry() *plugin.Registry {
	return c.registry
}

// Invalidate clears the cache for a project.
func (c *Coverage) Invalidate(projectPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, projectPath)
}

// callGraphWorkers caps the number of concurrent CallGrapher requests.
// Each request is a few LSP round-trips against gopls (or equivalent), so
// running them in parallel hides per-request latency without overloading
// the language server.
const callGraphWorkers = 8

// BuildSymbols returns symbols, tests, and static call-graph data for a
// project without running tests for run-level coverage. It is the fast
// path used by the function listing endpoint.
func (c *Coverage) BuildSymbols(ctx context.Context, projectPath string) (*CoverageData, error) {
	c.mu.RLock()
	if cached, ok := c.cache[projectPath]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	data, err := c.buildBase(ctx, projectPath)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if existing, ok := c.cache[projectPath]; ok && existing.hasReport {
		// A full build (with run report) raced ahead of us; prefer it.
		data = existing
	} else {
		c.cache[projectPath] = data
	}
	c.mu.Unlock()
	return data, nil
}

// Build computes full coverage data including the run-level report. It
// shells out to per-language coverage tools (e.g. `go test -coverprofile`)
// and is therefore expensive — callers that only need the symbol list
// should use BuildSymbols instead.
func (c *Coverage) Build(ctx context.Context, projectPath string, opts plugin.RunOptions) (*CoverageData, error) {
	c.mu.RLock()
	if cached, ok := c.cache[projectPath]; ok && cached.hasReport {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	base, err := c.buildBase(ctx, projectPath)
	if err != nil {
		return nil, err
	}

	var reports []plugin.CoverageReport
	for _, p := range c.registry.All() {
		coverager, ok := p.(plugin.Coverager)
		if !ok {
			continue
		}
		r, err := coverager.Coverage(ctx, plugin.TestSelection{}, opts)
		if err == nil {
			reports = append(reports, r)
		}
	}
	report := mergeCoverageReports(reports)

	reportUncovered := make(map[string]bool)
	for _, id := range report.Uncovered {
		reportUncovered[id] = true
	}
	uncoveredSet := make(map[string]bool)
	for _, sym := range base.Symbols {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		runCovered := symbolCoveredByRunReport(sym, report)
		if reportUncovered[sym.ID] {
			runCovered = false
		}
		if len(base.StaticMap[sym.ID]) == 0 && !runCovered {
			uncoveredSet[sym.ID] = true
		}
	}
	var uncovered []string
	for id := range uncoveredSet {
		uncovered = append(uncovered, id)
	}
	report.Uncovered = uncovered

	base.Report = report
	base.Uncovered = uncovered
	base.hasReport = true

	c.mu.Lock()
	c.cache[projectPath] = base
	c.mu.Unlock()
	return base, nil
}

// buildBase computes everything that does not require running tests:
// symbol discovery, test discovery, and the static call-graph map. The
// call-graph step issues one CallGrapher request per function/method
// against each language plugin; those requests are run concurrently with
// a bounded worker pool to hide per-request LSP latency.
func (c *Coverage) buildBase(_ context.Context, projectPath string) (*CoverageData, error) {
	syms, err := DiscoverSymbols(c.registry, projectPath)
	if err != nil {
		return nil, fmt.Errorf("discover symbols: %w", err)
	}

	disc := NewDiscovery(c.registry)
	tests, err := disc.DiscoverAll(projectPath)
	if err != nil {
		return nil, fmt.Errorf("discover tests: %w", err)
	}

	staticMap := buildStaticCallMap(c.registry, syms, tests)

	uncoveredSet := make(map[string]bool)
	for _, sym := range syms {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		if len(staticMap[sym.ID]) == 0 {
			uncoveredSet[sym.ID] = true
		}
	}
	var uncovered []string
	for id := range uncoveredSet {
		uncovered = append(uncovered, id)
	}

	return &CoverageData{
		Symbols:   syms,
		Tests:     tests,
		StaticMap: staticMap,
		Uncovered: uncovered,
	}, nil
}

type callGraphJob struct {
	plug plugin.CallGrapher
	sym  plugin.Symbol
}

type callGraphResult struct {
	symID string
	ids   []string
}

func buildStaticCallMap(registry *plugin.Registry, syms []plugin.Symbol, tests []plugin.TestFunc) map[string][]string {
	out := make(map[string][]string)
	if len(syms) == 0 || len(tests) == 0 {
		return out
	}

	var jobs []callGraphJob
	for _, p := range registry.All() {
		grapher, ok := p.(plugin.CallGrapher)
		if !ok {
			continue
		}
		for _, sym := range syms {
			if sym.Kind != "function" && sym.Kind != "method" {
				continue
			}
			jobs = append(jobs, callGraphJob{plug: grapher, sym: sym})
		}
	}
	if len(jobs) == 0 {
		return out
	}

	workers := callGraphWorkers
	if len(jobs) < workers {
		workers = len(jobs)
	}
	jobCh := make(chan callGraphJob)
	resCh := make(chan callGraphResult, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				graph, err := j.plug.CallGraph(j.sym)
				if err != nil {
					continue
				}
				var ids []string
				for _, t := range tests {
					if graphMentionsTest(graph, t) {
						ids = append(ids, t.ID)
					}
				}
				if len(ids) > 0 {
					resCh <- callGraphResult{symID: j.sym.ID, ids: ids}
				}
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
	}()
	wg.Wait()
	close(resCh)

	for r := range resCh {
		out[r.symID] = append(out[r.symID], r.ids...)
	}
	return out
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func graphMentionsTest(graph plugin.Graph, test plugin.TestFunc) bool {
	qualified := test.Package + "." + test.Name
	for _, id := range append(graph.Incoming, graph.Outgoing...) {
		if id == test.ID {
			return true
		}
		if strings.Contains(id, ":"+qualified+":") || strings.HasSuffix(id, ":"+qualified) {
			return true
		}
	}
	return false
}

func symbolCoveredByRunReport(sym plugin.Symbol, report plugin.CoverageReport) bool {
	if len(report.Files) == 0 {
		return false
	}
	for _, fc := range report.Files {
		if !sameCoveragePath(fc.Path, sym.File) {
			continue
		}
		for _, lr := range fc.Lines {
			if sym.Line >= lr.Start && sym.Line <= lr.End && lr.Hit {
				return true
			}
		}
	}
	return false
}

func sameCoveragePath(a, b string) bool {
	a = filepath.ToSlash(filepath.Clean(a))
	b = filepath.ToSlash(filepath.Clean(b))
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func mergeCoverageReports(reports []plugin.CoverageReport) plugin.CoverageReport {
	if len(reports) == 0 {
		return plugin.CoverageReport{Scope: "run"}
	}
	if len(reports) == 1 {
		return reports[0]
	}

	fileMap := make(map[string]plugin.FileCoverage)
	for _, r := range reports {
		for _, fc := range r.Files {
			key := filepath.ToSlash(filepath.Clean(fc.Path))
			if existing, ok := fileMap[key]; ok {
				// Merge line ranges. For simplicity we append and let downstream
				// consumers handle overlapping ranges; the isLineCovered helper
				// checks any matching range.
				existing.Lines = append(existing.Lines, fc.Lines...)
				fileMap[key] = existing
			} else {
				fileMap[key] = fc
			}
		}
	}

	var files []plugin.FileCoverage
	var totalLines, hitLines int
	for path := range fileMap {
		fc := fileMap[path]
		fileHit := 0
		fileTotal := 0
		seen := make(map[plugin.LineRange]bool)
		for _, lr := range fc.Lines {
			if seen[lr] {
				continue
			}
			seen[lr] = true
			lines := lr.End - lr.Start + 1
			if lines < 0 {
				lines = 0
			}
			fileTotal += lines
			if lr.Hit {
				fileHit += lines
				hitLines += lines
			}
			totalLines += lines
		}
		if fileTotal > 0 {
			fc.Percentage = float64(fileHit) / float64(fileTotal) * 100
		}
		files = append(files, fc)
	}

	var percentage float64
	if totalLines > 0 {
		percentage = float64(hitLines) / float64(totalLines) * 100
	}

	var languages []plugin.LanguageCoverage
	seenLang := make(map[string]bool)
	for _, r := range reports {
		for _, lc := range r.Languages {
			if seenLang[lc.Language] {
				continue
			}
			seenLang[lc.Language] = true
			languages = append(languages, lc)
		}
	}

	return plugin.CoverageReport{
		Scope:      "run",
		Percentage: percentage,
		Files:      files,
		Languages:  languages,
	}
}
