// Package prompts renders ready-to-paste prompts that instruct an LLM to
// create or convert tests into the table + golden-JSON format.
package prompts

import (
	"bytes"
	"strings"
	"text/template"
)

// Kind identifies which prompt template to render.
type Kind string

const (
	KindCreate    Kind = "create"
	KindTransform Kind = "transform"
)

// Inputs are the values rendered into a prompt template.
type Inputs struct {
	Kind          Kind
	Language      string // "go", "python", ...
	Qualified     string // qualified function name
	File          string // source file (project-relative)
	Line          int
	TestFile      string // existing test file (transform) or suggested path (create)
	GoldenDir     string // project-relative directory for golden files
	LanguageHints string // language-specific notes appended to the prompt
}

// Render returns the rendered prompt text for the given inputs.
func Render(in Inputs) (string, error) {
	tmpl := createTpl
	if in.Kind == KindTransform {
		tmpl = transformTpl
	}
	t, err := template.New(string(in.Kind)).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, in); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}

// HintsFor returns language-specific guidance appended to the prompt.
// Plugins can supply their own via the PromptHinter optional interface;
// this is the fallback used when no plugin hint is available.
func HintsFor(language string) string {
	switch language {
	case "go":
		return goHints
	case "python":
		return pythonHints
	default:
		return ""
	}
}

const createTpl = `# Generate tests

Target function: ` + "`{{.Qualified}}`" + `
Source: {{.File}}:{{.Line}}
Language: {{.Language}}
Golden directory: ` + "`{{.GoldenDir}}`" + `
{{if .TestFile}}Suggested test file: ` + "`{{.TestFile}}`" + `
{{end}}
## Goal

Write a single **table-driven test** for ` + "`{{.Qualified}}`" + ` backed by golden JSON files.
Keep the test code as small as possible — push every case-specific value into the data files.

## Conventions

- For each case, create a pair of files under ` + "`{{.GoldenDir}}/`" + `:
  - ` + "`<case>.in.json`" + `  — the function input (arguments serialized as JSON)
  - ` + "`<case>.out.json`" + ` — the expected return value
- Pick descriptive case names (e.g. ` + "`empty`, `happy_path`, `negative_amount`" + `). Sanitize for the filesystem.
- The test loads each pair, calls ` + "`{{.Qualified}}`" + `, and asserts deep JSON equality against the ` + "`.out.json`" + `.
- Cover happy path + meaningful edge cases (empty input, boundary values, error paths). One case per file pair.

## Steps

1. Read the function at ` + "`{{.File}}:{{.Line}}`" + ` and understand its signature and effects.
2. Create the directory ` + "`{{.GoldenDir}}/`" + ` if it does not exist.
3. Add at least 3 case file pairs (` + "`<case>.in.json` + `<case>.out.json`" + `).
4. Write one table test that iterates the cases by reading the directory, calls ` + "`{{.Qualified}}`" + `, and compares the result to ` + "`<case>.out.json`" + `.
5. Run the tests and make sure they pass. Do not modify the production code.

{{.LanguageHints}}`

const transformTpl = `# Convert existing tests to the table + golden format

Function under test: ` + "`{{.Qualified}}`" + `
Source: {{.File}}:{{.Line}}
Existing test file: ` + "`{{.TestFile}}`" + `
Language: {{.Language}}
Golden directory: ` + "`{{.GoldenDir}}`" + `

## Goal

Rewrite the existing test(s) for ` + "`{{.Qualified}}`" + ` into a single **table-driven test** backed by golden JSON files. Behavior coverage must stay identical — the same inputs and the same expectations as today, just expressed as data.

## Conventions

- Each existing scenario becomes a pair under ` + "`{{.GoldenDir}}/`" + `:
  - ` + "`<case>.in.json`" + `  — the function input
  - ` + "`<case>.out.json`" + ` — the expected return value
- Case names should reflect the original test/sub-test names (e.g. ` + "`empty_input`, `negative_amount`" + `). Sanitize for the filesystem.
- The new test is a loop that reads the directory, calls ` + "`{{.Qualified}}`" + `, and asserts deep JSON equality against the ` + "`.out.json`" + `.

## Steps

1. Read the existing test(s) at ` + "`{{.TestFile}}`" + `.
2. For every case currently asserted, extract the literal input and expected output and write them under ` + "`{{.GoldenDir}}/`" + ` as ` + "`<case>.in.json`" + ` and ` + "`<case>.out.json`" + `.
3. Replace the existing test code with a single table test that iterates those files. Delete the now-obsolete per-case test functions in the same file.
4. Do not change production code. Do not invent new cases and do not drop existing ones — preserve exactly what was tested before.
5. Run the tests and make sure they pass.

{{.LanguageHints}}`

const goHints = `## Go specifics

- Use the standard ` + "`testing`" + ` package; no third-party assertion libs.
- Discover cases at runtime with ` + "`os.ReadDir`" + ` over the golden directory; pair files by the ` + "`<case>`" + ` stem.
- Decode with ` + "`encoding/json`" + `. For the input, decode into the function's parameter type (or a small struct that mirrors it). For the expected output, decode into ` + "`any`" + ` and compare with ` + "`reflect.DeepEqual`" + ` against the function result also re-encoded through ` + "`json.Marshal` + `json.Unmarshal`" + ` so map/number kinds match.
- Wrap each case in ` + "`t.Run(caseName, func(t *testing.T) { ... })`" + ` so failures point at the case.`

const pythonHints = `## Python specifics

- Use ` + "`pytest`" + ` with ` + "`@pytest.mark.parametrize`" + ` over the discovered case names.
- Discover cases at import time with ` + "`pathlib.Path(GOLDEN_DIR).glob('*.in.json')`" + `; the case name is the stem.
- Load JSON with the standard ` + "`json`" + ` module. Compare the function's return value to the expected value with plain ` + "`==`" + ` after round-tripping the actual through ` + "`json.loads(json.dumps(...))`" + ` so types align.
- Keep the test module short: one parametrized function plus the loader helper.`
