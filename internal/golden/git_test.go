package golden

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreviousVersionTracked(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available")
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.name", "Test").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "add", "file.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	data, err := PreviousVersion("file.txt")
	if err != nil {
		t.Fatalf("PreviousVersion error: %v", err)
	}
	if string(data) != "old content" {
		t.Errorf("expected old content, got %q", string(data))
	}
}

func TestPreviousVersionNotInGit(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	_, err := PreviousVersion("file.txt")
	if !errors.Is(err, ErrNotInGit) {
		t.Errorf("expected ErrNotInGit, got %v", err)
	}
}

func TestPreviousVersionNotTracked(t *testing.T) {
	type gitConfig struct {
		UserEmail string `json:"userEmail"`
		UserName  string `json:"userName"`
	}
	type fileInput struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	type input struct {
		Git   gitConfig   `json:"git"`
		Files []fileInput `json:"files"`
		Path  string      `json:"path"`
	}

	goldenDir := filepath.Join("..", "..", "tests", "golden", "golden", "TestPreviousVersionNotTracked")
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".in.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".in.json")
		t.Run(caseName, func(t *testing.T) {
			var in input
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".in.json"), &in)

			var want any
			decodeJSONFile(t, filepath.Join(goldenDir, caseName+".out.json"), &want)

			dir := t.TempDir()
			if err := exec.Command("git", "init", dir).Run(); err != nil {
				t.Skip("git not available")
			}
			for _, file := range in.Files {
				path := filepath.Join(dir, filepath.FromSlash(file.Path))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if in.Git.UserEmail != "" {
				if err := exec.Command("git", "-C", dir, "config", "user.email", in.Git.UserEmail).Run(); err != nil {
					t.Fatal(err)
				}
			}
			if in.Git.UserName != "" {
				if err := exec.Command("git", "-C", dir, "config", "user.name", in.Git.UserName).Run(); err != nil {
					t.Fatal(err)
				}
			}

			oldWd, _ := os.Getwd()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(oldWd)

			data, err := PreviousVersion(in.Path)
			got := outputForPreviousVersion(data, err)
			var gotJSON any
			roundTripJSON(t, got, &gotJSON)
			if !reflect.DeepEqual(gotJSON, want) {
				t.Errorf("PreviousVersion(%q) = %#v, want %#v", in.Path, gotJSON, want)
			}
		})
	}
}

func decodeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func roundTripJSON(t *testing.T, in any, out any) {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}

func outputForPreviousVersion(data []byte, err error) any {
	var dataString *string
	if data != nil {
		s := string(data)
		dataString = &s
	}
	var errString *string
	if err != nil {
		s := err.Error()
		if errors.Is(err, ErrNotTracked) {
			s = "ErrNotTracked"
		} else if errors.Is(err, ErrNotInGit) {
			s = "ErrNotInGit"
		}
		errString = &s
	}
	return struct {
		Data  *string `json:"data"`
		Error *string `json:"error"`
	}{
		Data:  dataString,
		Error: errString,
	}
}

func TestPreviousVersionNoCommits(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available")
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "add", "file.txt").Run(); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	_, err := PreviousVersion("file.txt")
	if !errors.Is(err, ErrNotTracked) {
		t.Errorf("expected ErrNotTracked for no-commits repo, got %v", err)
	}
}
