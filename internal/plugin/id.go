package plugin

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const idSeparator = ":"

// ID is the structured representation of a plugin-scoped identifier.
type ID struct {
	Plugin        string
	Path          string
	QualifiedName string
	CasePath      string
	Line          int
	Column        int
}

// NewTestID builds a stable test identifier.
func NewTestID(pluginName, file, qualifiedName string) string {
	return joinID(pluginName, file, qualifiedName)
}

// NewTestCaseID builds a stable sub-test or parameterized-case identifier.
func NewTestCaseID(pluginName, file, qualifiedName, casePath string) string {
	return joinID(pluginName, file, qualifiedName, casePath)
}

// AppendTestCaseID appends a raw case path to an existing test ID.
func AppendTestCaseID(testID, casePath string) string {
	return testID + idSeparator + EscapeSegment(casePath)
}

// NewSymbolID builds a stable symbol identifier.
func NewSymbolID(pluginName, file, qualifiedName string, line, column int) string {
	return joinID(pluginName, file, qualifiedName, strconv.Itoa(line), strconv.Itoa(column))
}

// ParseID splits any supported plugin ID into its components.
func ParseID(id string) (ID, error) {
	parts := splitEscaped(id, ':')
	switch len(parts) {
	case 3:
		return ID{Plugin: parts[0], Path: parts[1], QualifiedName: parts[2]}, nil
	case 4:
		return ID{Plugin: parts[0], Path: parts[1], QualifiedName: parts[2], CasePath: parts[3]}, nil
	case 5:
		line, err := strconv.Atoi(parts[3])
		if err != nil {
			return ID{}, fmt.Errorf("invalid symbol ID line: %q", id)
		}
		col, err := strconv.Atoi(parts[4])
		if err != nil {
			return ID{}, fmt.Errorf("invalid symbol ID column: %q", id)
		}
		return ID{Plugin: parts[0], Path: parts[1], QualifiedName: parts[2], Line: line, Column: col}, nil
	default:
		return ID{}, fmt.Errorf("invalid plugin ID format: %q", id)
	}
}

// ParseTestID splits a test or test-case ID into its components.
func ParseTestID(testID string) (pluginName string, path string, qualifiedName string, casePath string, err error) {
	parsed, err := ParseID(testID)
	if err != nil {
		return "", "", "", "", err
	}
	if parsed.Line != 0 || parsed.Column != 0 {
		return "", "", "", "", fmt.Errorf("invalid test ID format: %q", testID)
	}
	return parsed.Plugin, parsed.Path, parsed.QualifiedName, parsed.CasePath, nil
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

func joinID(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, EscapeSegment(part))
	}
	return strings.Join(escaped, idSeparator)
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
			continue
		}
		current.WriteByte(s[i])
		i++
	}
	parts = append(parts, current.String())
	return parts
}
