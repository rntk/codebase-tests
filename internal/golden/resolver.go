package golden

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/review-server/internal/plugin"
)

// GoldenCase represents a single golden test case.
type GoldenCase struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	InPath    string      `json:"inPath"`
	OutPath   string      `json:"outPath"`
	MetaPath  string      `json:"metaPath"`
	InExists  bool        `json:"inExists"`
	OutExists bool        `json:"outExists"`
	Meta      *GoldenMeta `json:"meta,omitempty"`
}

// GoldenMeta is the optional metadata for a golden case.
type GoldenMeta struct {
	Version int            `json:"version"`
	Labels  []string       `json:"labels,omitempty"`
	SLOs    map[string]any `json:"slos,omitempty"`
}

// ResolveCases enumerates golden cases for a test.
func ResolveCases(projectRoot string, testId string, conv plugin.GoldenConvention) ([]GoldenCase, error) {
	_, _, qualifiedName, _, err := ParseTestID(testId)
	if err != nil {
		return nil, err
	}
	qualifiedName = UnescapeSegment(qualifiedName)

	if conv.Root == "" {
		conv.Root = "tests/golden"
	}
	if conv.SegmentFunc == "" {
		conv.SegmentFunc = "qualifiedName"
	}
	if conv.CaseSegment == "" {
		conv.CaseSegment = "file"
	}

	root := filepath.Join(projectRoot, conv.Root)

	var segments []string
	switch conv.SegmentFunc {
	case "qualifiedName":
		parts := strings.Split(qualifiedName, ".")
		for _, p := range parts {
			segments = append(segments, EscapeSegment(p))
		}
	case "packagePath":
		segments = append(segments, EscapeSegment(qualifiedName))
	default:
		segments = append(segments, EscapeSegment(qualifiedName))
	}

	funcDir := filepath.Join(root, filepath.Join(segments...))

	info, err := os.Stat(funcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []GoldenCase{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []GoldenCase{}, nil
	}

	var cases []GoldenCase
	if conv.CaseSegment == "subfolder" {
		entries, err := os.ReadDir(funcDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			caseName := UnescapeSegment(e.Name())
			c := buildCase(funcDir, e.Name(), caseName, testId, true)
			cases = append(cases, c)
		}
	} else {
		entries, err := os.ReadDir(funcDir)
		if err != nil {
			return nil, err
		}

		caseMap := make(map[string]struct{})
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".in.json") {
				caseMap[strings.TrimSuffix(name, ".in.json")] = struct{}{}
			} else if strings.HasSuffix(name, ".out.json") {
				caseMap[strings.TrimSuffix(name, ".out.json")] = struct{}{}
			}
		}

		for caseName := range caseMap {
			c := buildCase(funcDir, caseName, UnescapeSegment(caseName), testId, false)
			cases = append(cases, c)
		}
	}

	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Name < cases[j].Name
	})

	return cases, nil
}

func buildCase(funcDir string, caseFileName string, caseName string, testId string, isSubfolder bool) GoldenCase {
	var inPath, outPath, metaPath string
	if isSubfolder {
		base := filepath.Join(funcDir, caseFileName)
		inPath = filepath.Join(base, "in.json")
		outPath = filepath.Join(base, "out.json")
		metaPath = filepath.Join(base, "meta.json")
	} else {
		base := filepath.Join(funcDir, caseFileName)
		inPath = base + ".in.json"
		outPath = base + ".out.json"
		metaPath = base + ".meta.json"
	}

	inExists := false
	if _, err := os.Stat(inPath); err == nil {
		inExists = true
	}
	outExists := false
	if _, err := os.Stat(outPath); err == nil {
		outExists = true
	}

	var meta *GoldenMeta
	if data, err := os.ReadFile(metaPath); err == nil {
		var m GoldenMeta
		if err := json.Unmarshal(data, &m); err == nil {
			meta = &m
		}
	}

	return GoldenCase{
		ID:        testId + ":" + EscapeSegment(caseName),
		Name:      caseName,
		InPath:    inPath,
		OutPath:   outPath,
		MetaPath:  metaPath,
		InExists:  inExists,
		OutExists: outExists,
		Meta:      meta,
	}
}

// ParseTestID splits a test ID into its components.
// TestFunc ID: plugin:path:qualifiedName
// TestCase ID: plugin:path:qualifiedName:casePath
func ParseTestID(testId string) (pluginName string, path string, qualifiedName string, casePath string, err error) {
	parts := splitEscaped(testId, ':')
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], "", nil
	case 4:
		return parts[0], parts[1], parts[2], parts[3], nil
	default:
		return "", "", "", "", fmt.Errorf("invalid test ID format: %q", testId)
	}
}

func splitEscaped(s string, delim byte) []string {
	var parts []string
	var current strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' && i+3 <= len(s) {
			esc := strings.ToLower(s[i : i+3])
			if (delim == ':' && esc == "%3a") || (delim == '/' && esc == "%2f") {
				current.WriteString(s[i : i+3])
				i += 3
				continue
			}
		}
		if s[i] == delim {
			parts = append(parts, current.String())
			current.Reset()
			i++
		} else {
			current.WriteByte(s[i])
			i++
		}
	}
	parts = append(parts, current.String())
	return parts
}

// EscapeSegment applies ID escaping rules to a path segment.
func EscapeSegment(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, "/", "%2F")
	return s
}

// UnescapeSegment reverses ID escaping rules.
func UnescapeSegment(s string) string {
	out, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return out
}
