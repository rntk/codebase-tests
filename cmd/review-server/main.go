package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net/http"

	"github.com/rntk/codebase-tests/internal/api"
	"github.com/rntk/codebase-tests/internal/plugin"
	"github.com/rntk/codebase-tests/internal/project"
	"github.com/rntk/codebase-tests/internal/tests"
	goplugin "github.com/rntk/codebase-tests/plugins/go"
	javascriptplugin "github.com/rntk/codebase-tests/plugins/javascript"
	pythonplugin "github.com/rntk/codebase-tests/plugins/python"
	typescriptplugin "github.com/rntk/codebase-tests/plugins/typescript"
)

func main() {
	var projectPath string
	var pluginsFlag string
	flag.StringVar(&projectPath, "project", ".", "path to project directory")
	flag.StringVar(&pluginsFlag, "plugins", "go", "comma-separated plugins to enable when initializing a project")
	flag.Parse()

	var p *project.Project
	var err error

	if _, statErr := os.Stat(projectPath); os.IsNotExist(statErr) {
		log.Fatalf("project path does not exist: %s", projectPath)
	}

	configPath := project.ConfigDir + "/" + project.ConfigFile
	if _, statErr := os.Stat(projectPath + "/" + configPath); os.IsNotExist(statErr) {
		log.Printf("no %s found; initializing project at %s", configPath, projectPath)
		p, err = project.InitProject(projectPath, project.InitOptions{
			Name:               projectNameFromPath(projectPath),
			Plugins:            parsePlugins(pluginsFlag),
			GoldenRoot:         "tests/golden",
			ToolTimeoutSeconds: 30,
		})
		if err != nil {
			log.Fatalf("init project: %v", err)
		}
	} else {
		p, err = project.LoadProject(projectPath)
		if err != nil {
			log.Fatalf("load project: %v", err)
		}
	}

	// Create plugin registry and register built-in plugins.
	registry := plugin.NewRegistry()
	for _, cfg := range p.Plugins {
		switch cfg.Name {
		case "go":
			registry.Register(goplugin.New())
		case "javascript":
			registry.Register(javascriptplugin.New())
		case "typescript":
			registry.Register(typescriptplugin.New())
		case "python":
			registry.Register(pythonplugin.New())
		default:
			log.Printf("warning: plugin %q is configured but not available", cfg.Name)
		}
	}

	ctx := context.Background()
	basePluginConfig := plugin.PluginConfig{
		ProjectRoot:        p.Path,
		GoldenRoot:         p.GoldenRoot,
		ToolTimeoutSeconds: p.ToolTimeoutSeconds,
	}
	pluginConfigs := make([]plugin.PluginConfig, 0, len(p.Plugins))
	for _, cfg := range p.Plugins {
		pluginConfigs = append(pluginConfigs, plugin.PluginConfig{
			Name:               cfg.Name,
			Language:           cfg.Language,
			LSPServerAddress:   cfg.LSPServerAddress,
			ProjectRoot:        p.Path,
			GoldenRoot:         p.GoldenRoot,
			ToolTimeoutSeconds: p.ToolTimeoutSeconds,
		})
	}
	if err := registry.InitializeConfigured(ctx, basePluginConfig, pluginConfigs); err != nil {
		log.Printf("warning: plugin initialization failed: %v", err)
		log.Println("server will continue with limited functionality")
	}

	configureDefaultStaticDir()

	r := http.NewServeMux()
	ph := api.NewProjectsHandler()
	ph.RegisterRoutes(r)
	ph.SetProject(p)

	fh := api.NewFilesHandler()
	fh.RegisterRoutes(r)
	fh.SetProject(p)

	gh := api.NewGoldenHandler(registry)
	gh.RegisterRoutes(r)
	gh.SetProject(p)

	discovery := tests.NewDiscovery(registry)
	coverage := tests.NewCoverage(registry)
	if discoveredTests, err := discovery.DiscoverAll(p.Path); err != nil {
		log.Printf("warning: startup test discovery failed: %v", err)
	} else {
		log.Printf("startup discovered %d tests", len(discoveredTests))
	}
	if discoveredSymbols, err := tests.DiscoverSymbols(registry, p.Path); err != nil {
		log.Printf("warning: startup symbol discovery failed: %v", err)
	} else {
		log.Printf("startup discovered %d symbols", len(discoveredSymbols))
	}

	th := api.NewTestsHandler(discovery)
	th.RegisterRoutes(r)
	th.SetProject(p)

	sh := api.NewSymbolsHandler(coverage)
	sh.RegisterRoutes(r)
	sh.SetProject(p)

	ch := api.NewCoverageHandler(coverage)
	ch.RegisterRoutes(r)
	ch.SetProject(p)

	prh := api.NewPromptsHandler(registry, discovery)
	prh.RegisterRoutes(r)
	prh.SetProject(p)

	server := api.NewServer(r)
	addr, err := server.Start()
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	fmt.Printf("review-server running at http://%s (project: %s)\n", addr, p.Name)

	api.WaitForInterrupt()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := registry.Shutdown(shutdownCtx); err != nil {
		log.Printf("registry shutdown error: %v", err)
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func parsePlugins(value string) []project.PluginSettings {
	var names []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			names = append(names, part)
		}
	}
	if len(names) == 0 {
		names = []string{"go"}
	}
	return project.DefaultPluginSettingsList(names)
}

func projectNameFromPath(path string) string {
	if path == "." || path == "/" {
		return "project"
	}
	base := filepath.Base(path)
	if base == "." || base == "/" {
		return "project"
	}
	return base
}

func configureDefaultStaticDir() {
	if os.Getenv("REVIEW_SERVER_STATIC_DIR") != "" {
		return
	}

	for _, dir := range staticDirCandidates() {
		if hasIndexHTML(dir) {
			if err := os.Setenv("REVIEW_SERVER_STATIC_DIR", dir); err != nil {
				log.Printf("warning: unable to set static UI directory: %v", err)
				return
			}
			log.Printf("serving UI from %s", dir)
			return
		}
	}

	log.Println("warning: no built UI found; set REVIEW_SERVER_STATIC_DIR or build web/dist to serve /")
}

func staticDirCandidates() []string {
	candidates := []string{
		filepath.Join(".", "web", "dist"),
	}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "web", "dist"))
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "web", "dist"),
			filepath.Join(exeDir, "..", "web", "dist"),
		)
	}

	return candidates
}

func hasIndexHTML(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}
