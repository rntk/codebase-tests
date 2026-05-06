// Package pluginutil provides shared utilities for language plugins.
package pluginutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rntk/codebase-tests/internal/plugin"
)

// BuildEnv constructs an environment slice from the current process environment,
// optionally filtered by an allowlist, with extra variables appended.
func BuildEnv(allowlist []string, extra map[string]string) []string {
	env := os.Environ()
	if len(allowlist) > 0 {
		allowed := make(map[string]bool)
		for _, k := range allowlist {
			allowed[k] = true
		}
		filtered := make([]string, 0, len(env))
		for _, e := range env {
			key := strings.SplitN(e, "=", 2)[0]
			if allowed[key] {
				filtered = append(filtered, e)
			}
		}
		env = filtered
	}
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// RelativePath returns path relative to root, falling back to a clean slash form.
func RelativePath(root, path string) string {
	if root == "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

// PathToURI converts a filesystem path to a file:// URI.
func PathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + filepath.ToSlash(abs)
}

// URIToPath converts a file:// URI back to a filesystem path.
func URIToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return uri[len("file://"):]
	}
	return uri
}

// FindFileCoverage searches files for a coverage entry matching path.
func FindFileCoverage(files []plugin.FileCoverage, path string) (plugin.FileCoverage, bool) {
	for _, fc := range files {
		if filepath.Clean(fc.Path) == filepath.Clean(path) {
			return fc, true
		}
		if strings.HasSuffix(filepath.Clean(path), filepath.Clean(fc.Path)) {
			return fc, true
		}
		if strings.HasSuffix(filepath.Clean(fc.Path), filepath.Clean(path)) {
			return fc, true
		}
	}
	return plugin.FileCoverage{}, false
}

// IsLineCovered reports whether line falls within a covered range.
func IsLineCovered(line int, ranges []plugin.LineRange) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End && r.Hit {
			return true
		}
	}
	return false
}

// RunCommand executes an external command with context, timeout, working directory,
// and environment controls. Output is truncated to maxOutputBytes (default 1MB).
func RunCommand(ctx context.Context, wd string, opts plugin.RunOptions, extraEnv map[string]string, name string, args ...string) (string, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = wd
	cmd.Env = BuildEnv(opts.EnvAllowlist, extraEnv)
	out, err := cmd.CombinedOutput()
	maxOut := opts.MaxOutputBytes
	if maxOut == 0 {
		maxOut = 1 << 20
	}
	if int64(len(out)) > maxOut {
		out = append(out[:int(maxOut)], []byte("\n...truncated...")...)
	}
	return string(out), err
}
