package golden

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// ErrNotInGit indicates the path is not inside a git repository.
	ErrNotInGit = errors.New("not a git repository")
	// ErrNotTracked indicates the file is not tracked by git.
	ErrNotTracked = errors.New("file not tracked by git")
)

// PreviousVersion returns the content of path at HEAD.
func PreviousVersion(path string) ([]byte, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	workDir := filepath.Dir(absPath)
	if _, err := os.Stat(workDir); err != nil {
		return nil, err
	}

	topOut, err := exec.Command("git", "-C", workDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, ErrNotInGit
	}
	repoRoot := strings.TrimSpace(string(topOut))
	relPath, err := filepath.Rel(repoRoot, absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return nil, ErrNotTracked
	}
	relPath = filepath.ToSlash(relPath)

	if err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Run(); err != nil {
		return nil, ErrNotTracked
	}

	cmd := exec.Command("git", "-C", repoRoot, "show", "HEAD:"+relPath)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "does not exist") || strings.Contains(stderr, "unknown revision") {
				return nil, ErrNotTracked
			}
		}
		return nil, fmt.Errorf("git show failed: %w", err)
	}
	return out, nil
}
