package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigDir = ".review"
const ConfigFile = "config.json"

// Project is the runtime representation of a loaded project.
type Project struct {
	ID                 string           `json:"id"`
	Path               string           `json:"path"`
	Name               string           `json:"name"`
	Plugins            []PluginSettings `json:"plugins"`
	GoldenRoot         string           `json:"goldenRoot"`
	ToolTimeoutSeconds int              `json:"toolTimeoutSeconds"`
}

// Config is the on-disk metadata format.
type Config struct {
	Version            int              `json:"version"`
	Name               string           `json:"name"`
	Plugins            []PluginSettings `json:"plugins"`
	GoldenRoot         string           `json:"goldenRoot"`
	ToolTimeoutSeconds int              `json:"toolTimeoutSeconds"`
}

// PluginSettings describes one language plugin configured for a project.
type PluginSettings struct {
	Name             string `json:"name"`
	Language         string `json:"language"`
	LSPServerAddress string `json:"lspServerAddress"`
}

// UnmarshalJSON accepts both the v1 string shape ("go") and the richer object
// shape used by current configs.
func (p *PluginSettings) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		*p = DefaultPluginSettings(name)
		return nil
	}
	type pluginSettings PluginSettings
	var cfg pluginSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	*p = PluginSettings(cfg)
	p.ApplyDefaults()
	return nil
}

// ApplyDefaults fills omitted fields from the plugin name.
func (p *PluginSettings) ApplyDefaults() {
	defaults := DefaultPluginSettings(p.Name)
	if p.Language == "" {
		p.Language = defaults.Language
	}
	if p.LSPServerAddress == "" {
		p.LSPServerAddress = defaults.LSPServerAddress
	}
}

// DefaultPluginSettings returns conventional settings for a built-in plugin.
func DefaultPluginSettings(name string) PluginSettings {
	switch name {
	case "go":
		return PluginSettings{Name: "go", Language: "go", LSPServerAddress: "gopls"}
	case "python":
		return PluginSettings{Name: "python", Language: "python", LSPServerAddress: "pylsp"}
	default:
		return PluginSettings{Name: name, Language: name, LSPServerAddress: name}
	}
}

// DefaultPluginSettingsList maps plugin names to full settings.
func DefaultPluginSettingsList(names []string) []PluginSettings {
	out := make([]PluginSettings, 0, len(names))
	for _, name := range names {
		if name != "" {
			out = append(out, DefaultPluginSettings(name))
		}
	}
	return out
}

// PluginNames returns configured plugin identifiers.
func (p *Project) PluginNames() []string {
	out := make([]string, 0, len(p.Plugins))
	for _, cfg := range p.Plugins {
		out = append(out, cfg.Name)
	}
	return out
}

// LoadProject reads .review/config.json and returns a hydrated Project.
func LoadProject(path string) (*Project, error) {
	cfgPath := filepath.Join(path, ConfigDir, ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	p := &Project{
		ID:                 filepath.Base(path),
		Path:               path,
		Name:               cfg.Name,
		Plugins:            cfg.Plugins,
		GoldenRoot:         cfg.GoldenRoot,
		ToolTimeoutSeconds: cfg.ToolTimeoutSeconds,
	}
	if err := ValidateProject(p); err != nil {
		return nil, fmt.Errorf("validate project: %w", err)
	}
	return p, nil
}

// InitProject performs first-time setup for a project path.
func InitProject(path string, opts InitOptions) (*Project, error) {
	if err := validateInitOptions(opts); err != nil {
		return nil, err
	}
	for i := range opts.Plugins {
		opts.Plugins[i].ApplyDefaults()
	}
	if err := os.MkdirAll(filepath.Join(path, ConfigDir), 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	cfg := Config{
		Version:            1,
		Name:               opts.Name,
		Plugins:            opts.Plugins,
		GoldenRoot:         opts.GoldenRoot,
		ToolTimeoutSeconds: opts.ToolTimeoutSeconds,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(path, ConfigDir, ConfigFile)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	return LoadProject(path)
}

// InitOptions controls first-time project initialization.
type InitOptions struct {
	Name               string
	Plugins            []PluginSettings
	GoldenRoot         string
	ToolTimeoutSeconds int
}

func validateInitOptions(opts InitOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if len(opts.Plugins) == 0 {
		return fmt.Errorf("at least one plugin is required")
	}
	for _, cfg := range opts.Plugins {
		cfg.ApplyDefaults()
		if cfg.Name == "" {
			return fmt.Errorf("plugin name is required")
		}
		if cfg.Language == "" {
			return fmt.Errorf("plugin %q language is required", cfg.Name)
		}
		if cfg.LSPServerAddress == "" {
			return fmt.Errorf("plugin %q LSP server address is required", cfg.Name)
		}
	}
	if opts.ToolTimeoutSeconds <= 0 {
		return fmt.Errorf("tool timeout must be positive")
	}
	return nil
}

// ValidateProject checks that a project has all required fields.
func ValidateProject(p *Project) error {
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if p.Path == "" {
		return fmt.Errorf("project path is required")
	}
	if len(p.Plugins) == 0 {
		return fmt.Errorf("at least one plugin is required")
	}
	for _, cfg := range p.Plugins {
		if cfg.Name == "" {
			return fmt.Errorf("plugin name is required")
		}
		if cfg.Language == "" {
			return fmt.Errorf("plugin %q language is required", cfg.Name)
		}
		if cfg.LSPServerAddress == "" {
			return fmt.Errorf("plugin %q LSP server address is required", cfg.Name)
		}
	}
	if p.ToolTimeoutSeconds <= 0 {
		return fmt.Errorf("tool timeout must be positive")
	}
	return nil
}

// SaveProject persists a project back to disk.
func SaveProject(p *Project) error {
	for i := range p.Plugins {
		p.Plugins[i].ApplyDefaults()
	}
	if err := ValidateProject(p); err != nil {
		return fmt.Errorf("validate project: %w", err)
	}
	cfg := Config{
		Version:            1,
		Name:               p.Name,
		Plugins:            p.Plugins,
		GoldenRoot:         p.GoldenRoot,
		ToolTimeoutSeconds: p.ToolTimeoutSeconds,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(p.Path, ConfigDir, ConfigFile)
	return os.WriteFile(cfgPath, data, 0644)
}
