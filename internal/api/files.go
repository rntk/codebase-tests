package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/review-server/internal/project"
)

// FilesHandler implements the file tree endpoint.
type FilesHandler struct {
	store map[string]*project.Project
}

// NewFilesHandler creates a new handler.
func NewFilesHandler() *FilesHandler {
	return &FilesHandler{
		store: make(map[string]*project.Project),
	}
}

// SetProject registers a project in the store.
func (h *FilesHandler) SetProject(p *project.Project) {
	registerProject(h.store, p)
}

// RegisterRoutes wires file routes.
func (h *FilesHandler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/files", h.List)
}

// FileNode is a node in the file tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Children []FileNode `json:"children,omitempty"`
}

// List walks the project directory and returns a file tree.
func (h *FilesHandler) List(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r.PathValue("id"))
	p, ok := h.store[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	nodes, err := WalkDir(p.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, nodes)
}

// WalkDir walks a directory and builds a tree, honoring .gitignore.
func WalkDir(root string) ([]FileNode, error) {
	patterns := readGitignore(root)
	return walkDir(root, root, patterns)
}

func walkDir(root, dir string, ignore []string) ([]FileNode, error) {
	var nodes []FileNode
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		rel, _ := filepath.Rel(root, path)
		if ignoredByGitignore(filepath.ToSlash(rel), ignore) {
			continue
		}
		node := FileNode{Name: name, Path: path, IsDir: e.IsDir()}
		if e.IsDir() {
			children, err := walkDir(root, path, ignore)
			if err == nil {
				node.Children = children
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func readGitignore(root string) []string {
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, strings.TrimPrefix(line, "/"))
	}
	return patterns
}

func ignoredByGitignore(rel string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if ok, _ := filepath.Match(p, filepath.Base(rel)); ok {
			return true
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
	}
	return false
}
