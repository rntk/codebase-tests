package plugin

import "context"

// Plugin is the language-agnostic interface every language plugin must implement.
type Plugin interface {
	Name() string
	Initialize(ctx context.Context, cfg PluginConfig) error
	Shutdown(ctx context.Context) error

	DiscoverTestFiles(root string) ([]File, error)
	DiscoverTests(file File) ([]TestFunc, error)
	DiscoverSymbols(root string) ([]Symbol, error)
	ResolveSymbol(pos Position) (Symbol, error)
	CallGraph(sym Symbol) (Graph, error)

	RunTests(ctx context.Context, sel TestSelection, opts RunOptions) (RunResult, error)
	Coverage(ctx context.Context, sel TestSelection, opts RunOptions) (CoverageReport, error)

	GoldenLayout() GoldenConvention
	Mutators() []Mutator
	Generators() []Generator
}

// PluginConfig is passed to Plugin.Initialize.
type PluginConfig struct {
	Name               string            `json:"name,omitempty"`
	Language           string            `json:"language,omitempty"`
	LSPServerAddress   string            `json:"lspServerAddress,omitempty"`
	ProjectRoot        string            `json:"projectRoot"`
	GoldenRoot         string            `json:"goldenRoot"`
	ToolTimeoutSeconds int               `json:"toolTimeoutSeconds"`
	Env                map[string]string `json:"env,omitempty"`
}

// RunOptions controls external tool execution.
type RunOptions struct {
	Timeout        int      `json:"timeout,omitempty"`        // seconds
	WorkingDir     string   `json:"workingDir,omitempty"`
	EnvAllowlist   []string `json:"envAllowlist,omitempty"`   // allowed env var names
	MaxOutputBytes int64    `json:"maxOutputBytes,omitempty"` // default 1MB
}

// File represents a source or test file discovered by a plugin.
type File struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	IsTest   bool   `json:"isTest"`
}

// TestFunc represents a discovered test function.
type TestFunc struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	File         string     `json:"file"`
	Line         int        `json:"line"`
	Column       int        `json:"column"`
	Package      string     `json:"package"`
	SubCases     []TestCase `json:"subCases,omitempty"`
	CoveredFuncs []string   `json:"coveredFuncs,omitempty"` // symbol IDs
}

// TestCase represents a sub-test or parameterized case.
type TestCase struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CasePath string `json:"casePath"` // escaped subtest / parametrized case path
}

// Symbol represents a function, method, or other discoverable symbol.
type Symbol struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	QualifiedName string  `json:"qualifiedName"`
	Kind         string   `json:"kind"` // "function", "method", "class", etc.
	File         string   `json:"file"`
	Line         int      `json:"line"`
	Column       int      `json:"column"`
	Package      string   `json:"package"`
	Covered      bool     `json:"covered"`
	CoveredBy    []string `json:"coveredBy,omitempty"` // test IDs
}

// Position identifies a location in a file.
type Position struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Graph is a call-graph fragment rooted at a symbol.
type Graph struct {
	Symbol   string   `json:"symbol"`   // symbol ID
	Incoming []string `json:"incoming,omitempty"` // symbol IDs
	Outgoing []string `json:"outgoing,omitempty"` // symbol IDs
}

// TestSelection identifies which tests to run.
type TestSelection struct {
	TestIDs []string `json:"testIds,omitempty"`
	File    string   `json:"file,omitempty"`
	Package string   `json:"package,omitempty"`
}

// RunResult is the outcome of a test run.
type RunResult struct {
	Passed   bool        `json:"passed"`
	Duration float64     `json:"duration"` // seconds
	Tests    []TestEntry `json:"tests"`
	Output   string      `json:"output,omitempty"`
}

// TestEntry is a single test outcome.
type TestEntry struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Passed   bool    `json:"passed"`
	Duration float64 `json:"duration"` // seconds
	Output   string  `json:"output,omitempty"`
}

// CoverageReport holds coverage data.
type CoverageReport struct {
	Scope      string  `json:"scope"` // "run" or "test"
	Percentage float64 `json:"percentage"`
	Files      []FileCoverage `json:"files,omitempty"`
	Uncovered  []string       `json:"uncovered,omitempty"` // symbol IDs with no coverage
}

// FileCoverage is per-file coverage details.
type FileCoverage struct {
	Path       string  `json:"path"`
	Percentage float64 `json:"percentage"`
	Lines      []LineRange `json:"lines,omitempty"`
}

// LineRange is a covered or uncovered line range.
type LineRange struct {
	Start int  `json:"start"`
	End   int  `json:"end"`
	Hit   bool `json:"hit"`
}

// GoldenConvention describes how a plugin lays out golden files on disk.
type GoldenConvention struct {
	Root        string `json:"root"`        // e.g. "tests/golden"
	SegmentFunc string `json:"segmentFunc"` // "qualifiedName" or "packagePath"
	CaseSegment string `json:"caseSegment"` // "file" or "subfolder"
}

// Mutator describes a mutation operator (v1 extension point).
type Mutator struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Generator describes a code generation operator (v1 extension point).
type Generator struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PromptHinter is an optional interface a Plugin may implement to supply
// language-specific guidance appended to test-generation prompts. When a
// plugin does not implement it, prompts.HintsFor(language) is used instead.
type PromptHinter interface {
	TestPromptHints() string
}
