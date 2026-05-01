package tests

import (
	"path/filepath"
	"sort"
	"sync"

	"github.com/review-server/internal/plugin"
)

// Discovery caches discovered tests for a project.
type Discovery struct {
	mu     sync.RWMutex
	cache  map[string][]plugin.TestFunc // project path -> tests
	registry *plugin.Registry
}

// NewDiscovery creates a discovery service.
func NewDiscovery(registry *plugin.Registry) *Discovery {
	return &Discovery{
		cache:    make(map[string][]plugin.TestFunc),
		registry: registry,
	}
}

// Invalidate clears the cache for a project.
func (d *Discovery) Invalidate(projectPath string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cache, projectPath)
}

// DiscoverAll returns all tests across all plugins for a project.
func (d *Discovery) DiscoverAll(projectPath string) ([]plugin.TestFunc, error) {
	d.mu.RLock()
	if cached, ok := d.cache[projectPath]; ok {
		d.mu.RUnlock()
		return cached, nil
	}
	d.mu.RUnlock()

	all := []plugin.TestFunc{}
	for _, p := range d.registry.All() {
		files, err := p.DiscoverTestFiles(projectPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			tests, err := p.DiscoverTests(f)
			if err != nil {
				continue
			}
			all = append(all, tests...)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	d.mu.Lock()
	d.cache[projectPath] = all
	d.mu.Unlock()
	return all, nil
}

// FindByID returns a single test by its stable ID.
func (d *Discovery) FindByID(projectPath, testID string) (plugin.TestFunc, bool) {
	all, _ := d.DiscoverAll(projectPath)
	for _, t := range all {
		if t.ID == testID {
			return t, true
		}
	}
	return plugin.TestFunc{}, false
}

// DiscoverSymbols returns all production symbols across all plugins.
func DiscoverSymbols(registry *plugin.Registry, projectPath string) ([]plugin.Symbol, error) {
	all := []plugin.Symbol{}
	for _, p := range registry.All() {
		syms, err := p.DiscoverSymbols(projectPath)
		if err != nil {
			continue
		}
		all = append(all, syms...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})
	return all, nil
}

// GroupByFile groups tests by their file path.
func GroupByFile(tests []plugin.TestFunc) map[string][]plugin.TestFunc {
	m := make(map[string][]plugin.TestFunc)
	for _, t := range tests {
		m[t.File] = append(m[t.File], t)
	}
	return m
}

// GroupSymbolsByFile groups symbols by their file path.
func GroupSymbolsByFile(symbols []plugin.Symbol) map[string][]plugin.Symbol {
	m := make(map[string][]plugin.Symbol)
	for _, s := range symbols {
		m[s.File] = append(m[s.File], s)
	}
	return m
}

// TreeNode is a generic tree node used for file -> item grouping.
type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*TreeNode `json:"children,omitempty"`
}

// BuildFileTree builds a tree from a flat list of file paths.
func BuildFileTree(paths []string, root string) []*TreeNode {
	nodeMap := make(map[string]*TreeNode)
	var roots []*TreeNode

	for _, p := range paths {
		rel, _ := filepath.Rel(root, p)
		if rel == "" || rel == "." {
			rel = p
		}
		dir := filepath.Dir(rel)
		name := filepath.Base(rel)

		if dir == "." || dir == "" {
			nodeMap[p] = &TreeNode{Name: name, Path: p, IsDir: false}
			roots = append(roots, nodeMap[p])
			continue
		}

		parts := filepath.SplitList(dir)
		if len(parts) == 0 {
			parts = []string{dir}
		}
		// Simple approach: just create a node per file under a folder node
		folderPath := filepath.Join(root, dir)
		if _, ok := nodeMap[folderPath]; !ok {
			nodeMap[folderPath] = &TreeNode{
				Name:  filepath.Base(folderPath),
				Path:  folderPath,
				IsDir: true,
			}
			roots = append(roots, nodeMap[folderPath])
		}
		nodeMap[p] = &TreeNode{Name: name, Path: p, IsDir: false}
		nodeMap[folderPath].Children = append(nodeMap[folderPath].Children, nodeMap[p])
	}
	return roots
}
