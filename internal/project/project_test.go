package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndLoadProject(t *testing.T) {
	tmp := t.TempDir()
	p, err := InitProject(tmp, InitOptions{
		Name:               "testproj",
		Plugins:            DefaultPluginSettingsList([]string{"go"}),
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("init project: %v", err)
	}
	if p.Name != "testproj" {
		t.Errorf("name = %q, want %q", p.Name, "testproj")
	}
	if p.Path != tmp {
		t.Errorf("path = %q, want %q", p.Path, tmp)
	}
	if p.ID != filepath.Base(tmp) {
		t.Errorf("id = %q, want %q", p.ID, filepath.Base(tmp))
	}
	if len(p.Plugins) != 1 || p.Plugins[0].Language != "go" || p.Plugins[0].LSPServerAddress != "gopls" {
		t.Errorf("plugins = %#v, want go plugin settings", p.Plugins)
	}

	loaded, err := LoadProject(tmp)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if loaded.Name != p.Name {
		t.Errorf("loaded name = %q, want %q", loaded.Name, p.Name)
	}
}

func TestLoadProjectMissing(t *testing.T) {
	tmp := t.TempDir()
	_, err := LoadProject(tmp)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadProjectBadVersion(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ConfigDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := `{"version": 999, "name": "x", "plugins": [], "goldenRoot": "tests/golden", "toolTimeoutSeconds": 30}`
	if err := os.WriteFile(filepath.Join(tmp, ConfigDir, ConfigFile), []byte(data), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := LoadProject(tmp)
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestLoadProjectLegacyPluginNames(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ConfigDir), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := `{"version": 1, "name": "x", "plugins": ["go"], "goldenRoot": "tests/golden", "toolTimeoutSeconds": 30}`
	if err := os.WriteFile(filepath.Join(tmp, ConfigDir, ConfigFile), []byte(data), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p, err := LoadProject(tmp)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if len(p.Plugins) != 1 {
		t.Fatalf("len plugins = %d, want 1", len(p.Plugins))
	}
	if p.Plugins[0].Name != "go" || p.Plugins[0].Language != "go" || p.Plugins[0].LSPServerAddress != "gopls" {
		t.Errorf("plugin = %#v, want go defaults", p.Plugins[0])
	}
}

func TestInitProjectValidation(t *testing.T) {
	tmp := t.TempDir()
	_, err := InitProject(tmp, InitOptions{
		Name:               "",
		Plugins:            DefaultPluginSettingsList([]string{"go"}),
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 30,
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	_, err = InitProject(tmp, InitOptions{
		Name:               "test",
		Plugins:            DefaultPluginSettingsList([]string{"go"}),
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 0,
	})
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}

	_, err = InitProject(tmp, InitOptions{
		Name:               "test",
		Plugins:            DefaultPluginSettingsList([]string{"go"}),
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: -1,
	})
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestSaveProject(t *testing.T) {
	tmp := t.TempDir()
	p, err := InitProject(tmp, InitOptions{
		Name:               "testproj",
		Plugins:            DefaultPluginSettingsList([]string{"go"}),
		GoldenRoot:         "tests/golden",
		ToolTimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	p.Name = "renamed"
	if err := SaveProject(p); err != nil {
		t.Fatalf("save project: %v", err)
	}

	loaded, err := LoadProject(tmp)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if loaded.Name != "renamed" {
		t.Errorf("name = %q, want %q", loaded.Name, "renamed")
	}
}

func TestSaveProjectValidation(t *testing.T) {
	p := &Project{
		Name:               "",
		Path:               "/tmp",
		ToolTimeoutSeconds: 10,
	}
	if err := SaveProject(p); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateProject(t *testing.T) {
	tests := []struct {
		name    string
		project *Project
		wantErr bool
	}{
		{
			name: "valid",
			project: &Project{
				Name: "test", Path: "/tmp", Plugins: DefaultPluginSettingsList([]string{"go"}), ToolTimeoutSeconds: 10,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			project: &Project{
				Name: "", Path: "/tmp", Plugins: DefaultPluginSettingsList([]string{"go"}), ToolTimeoutSeconds: 10,
			},
			wantErr: true,
		},
		{
			name: "empty path",
			project: &Project{
				Name: "test", Path: "", Plugins: DefaultPluginSettingsList([]string{"go"}), ToolTimeoutSeconds: 10,
			},
			wantErr: true,
		},
		{
			name: "zero timeout",
			project: &Project{
				Name: "test", Path: "/tmp", Plugins: DefaultPluginSettingsList([]string{"go"}), ToolTimeoutSeconds: 0,
			},
			wantErr: true,
		},
		{
			name: "missing plugin address",
			project: &Project{
				Name: "test", Path: "/tmp", Plugins: []PluginSettings{{Name: "go", Language: "go"}}, ToolTimeoutSeconds: 10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProject(tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProject() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
