package golden

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available")
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.name", "Test").Run(); err != nil {
		t.Fatal(err)
	}
	// file exists but is not committed
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	_, err := PreviousVersion("file.txt")
	if !errors.Is(err, ErrNotTracked) {
		t.Errorf("expected ErrNotTracked, got %v", err)
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
