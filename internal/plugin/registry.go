package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Registry holds loaded plugins for a project.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name()] = p
}

// For returns a plugin by name, or an error if not found.
func (r *Registry) For(name string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", name)
	}
	return p, nil
}

// All returns all registered plugins.
func (r *Registry) All() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

// Initialize calls Initialize on every registered plugin.
func (r *Registry) Initialize(ctx context.Context, cfg PluginConfig) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, p := range r.plugins {
		pluginCfg := cfg
		if pluginCfg.Name == "" {
			pluginCfg.Name = name
		}
		if err := p.Initialize(ctx, pluginCfg); err != nil {
			return fmt.Errorf("initialize plugin %q: %w", name, err)
		}
	}
	return nil
}

// InitializeConfigured calls Initialize on every registered plugin with its own
// language/LSP settings merged into the shared project settings.
func (r *Registry) InitializeConfigured(ctx context.Context, base PluginConfig, configs []PluginConfig) error {
	byName := make(map[string]PluginConfig, len(configs))
	for _, cfg := range configs {
		if cfg.Name != "" {
			byName[cfg.Name] = cfg
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, p := range r.plugins {
		cfg := base
		if pluginCfg, ok := byName[name]; ok {
			cfg.Name = pluginCfg.Name
			cfg.Language = pluginCfg.Language
			cfg.LSPServerAddress = pluginCfg.LSPServerAddress
			if pluginCfg.ProjectRoot != "" {
				cfg.ProjectRoot = pluginCfg.ProjectRoot
			}
			if pluginCfg.GoldenRoot != "" {
				cfg.GoldenRoot = pluginCfg.GoldenRoot
			}
			if pluginCfg.ToolTimeoutSeconds != 0 {
				cfg.ToolTimeoutSeconds = pluginCfg.ToolTimeoutSeconds
			}
			if pluginCfg.Env != nil {
				cfg.Env = pluginCfg.Env
			}
		} else if cfg.Name == "" {
			cfg.Name = name
		}
		if err := p.Initialize(ctx, cfg); err != nil {
			return fmt.Errorf("initialize plugin %q: %w", name, err)
		}
	}
	return nil
}

// Shutdown calls Shutdown on every registered plugin.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var errs []error
	for name, p := range r.plugins {
		if err := p.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown plugin %q: %w", name, err))
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ReadyCheck verifies that required external binaries are available in PATH.
func (r *Registry) ReadyCheck() error {
	required := []string{"go", "gopls", "git", "pytest", "coverage"}
	var missing []string
	for _, bin := range required {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required binaries: %s", strings.Join(missing, ", "))
	}
	return nil
}
