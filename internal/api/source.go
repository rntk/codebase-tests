package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readSourceSnippet(projectPath, relFile string, line int) (string, error) {
	cleanRel, lines, err := readProjectFileLines(projectPath, relFile)
	if err != nil {
		return "", err
	}
	return snippetFromLines(lines, filepath.Ext(cleanRel), relFile, line)
}

func readProjectFileLines(projectPath, relFile string) (string, []string, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(relFile))
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("invalid source path %q", relFile)
	}
	data, err := os.ReadFile(filepath.Join(projectPath, cleanRel))
	if err != nil {
		return "", nil, err
	}
	return cleanRel, strings.SplitAfter(string(data), "\n"), nil
}

func snippetFromLines(lines []string, ext, relFile string, line int) (string, error) {
	if line <= 0 {
		return "", fmt.Errorf("invalid source line %d", line)
	}
	if len(lines) == 0 || line > len(lines) {
		return "", fmt.Errorf("source line %d outside %s", line, relFile)
	}
	start := line - 1
	end := snippetEnd(lines, start, ext)
	return strings.TrimRight(strings.Join(lines[start:end], ""), "\n"), nil
}

func snippetEnd(lines []string, start int, ext string) int {
	switch ext {
	case ".go", ".js", ".jsx", ".ts", ".tsx":
		return braceSnippetEnd(lines, start)
	case ".py":
		return pythonSnippetEnd(lines, start)
	default:
		return paragraphSnippetEnd(lines, start)
	}
}

func braceSnippetEnd(lines []string, start int) int {
	balance := 0
	sawOpen := false
	limit := min(len(lines), start+160)
	for i := start; i < limit; i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				balance++
				sawOpen = true
			case '}':
				balance--
			}
		}
		if sawOpen && balance <= 0 {
			return i + 1
		}
	}
	return limit
}

func pythonSnippetEnd(lines []string, start int) int {
	baseIndent := leadingWhitespace(lines[start])
	limit := min(len(lines), start+160)
	for i := start + 1; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if leadingWhitespace(lines[i]) <= baseIndent {
			return i
		}
	}
	return limit
}

func paragraphSnippetEnd(lines []string, start int) int {
	limit := min(len(lines), start+80)
	for i := start + 1; i < limit; i++ {
		if strings.TrimSpace(lines[i]) == "" {
			return i
		}
	}
	return limit
}

func leadingWhitespace(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			return count
		}
		count++
	}
	return count
}
