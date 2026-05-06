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

// Build computes coverage data for a project.
func (c *Coverage) Build(ctx context.Context, projectPath string, opts plugin.RunOptions) (*CoverageData, error) {
	c.mu.RLock()
	if cached, ok := c.cache[projectPath]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	syms, err := DiscoverSymbols(c.registry, projectPath)
	if err != nil {
		return nil, fmt.Errorf("discover symbols: %w", err)
	}

	disc := NewDiscovery(c.registry)
	tests, err := disc.DiscoverAll(projectPath)
	if err != nil {
		return nil, fmt.Errorf("discover tests: %w", err)
	}

	staticMap := make(map[string][]string)
	for _, p := range c.registry.All() {
		grapher, ok := p.(plugin.CallGrapher)
		if !ok {
			continue
		}
		for _, sym := range syms {
			if sym.Kind != "function" && sym.Kind != "method" {
				continue
			}
			graph, err := grapher.CallGraph(sym)
			if err != nil {
				continue
			}
			for _, test := range tests {
				if graphMentionsTest(graph, test) {
					staticMap[sym.ID] = append(staticMap[sym.ID], test.ID)
				}
			}
		}
	}

	// Run-level coverage
	var report plugin.CoverageReport
	for _, p := range c.registry.All() {
		coverager, ok := p.(plugin.Coverager)
		if !ok {
			continue
		}
		r, err := coverager.Coverage(ctx, plugin.TestSelection{}, opts)
		if err == nil {
			report = r
			break
		}
	}

	uncoveredSet := make(map[string]bool)
	reportUncovered := make(map[string]bool)
	for _, id := range report.Uncovered {
		reportUncovered[id] = true
	}
	for _, sym := range syms {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		runCovered := symbolCoveredByRunReport(sym, report)
		if reportUncovered[sym.ID] {
			runCovered = false
		}
		if len(staticMap[sym.ID]) == 0 && !runCovered {
			uncoveredSet[sym.ID] = true
		}
	}

	var uncovered []string
	for id := range uncoveredSet {
		uncovered = append(uncovered, id)
	}
	report.Uncovered = uncovered

	data := &CoverageData{
		Symbols:   syms,
		Tests:     tests,
		StaticMap: staticMap,
		Uncovered: uncovered,
		Report:    report,
	}

	c.mu.Lock()
	c.cache[projectPath] = data
	c.mu.Unlock()
	return data, nil
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
