package golden

import (
	"encoding/json"
	"fmt"
)

const (
	negativeNumberMutation = -1
	largeNumberMutation    = 1_000_000_000
)

// MutationResult is the result of running a single mutation.
type MutationResult struct {
	Mutation string `json:"mutation"`
	Survived bool   `json:"survived"` // true if the test still passed under mutation (bad)
	Output   string `json:"output"`
}

// Mutation represents a single mutation of the JSON data.
type Mutation struct {
	Description string `json:"description"`
	Data        []byte `json:"data"`
}

// MutateJSON generates a list of mutated JSON contents.
func MutateJSON(data []byte) ([]Mutation, error) {
	var val any
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}

	var mutations []Mutation
	walkAndMutate("", val, func(path string, mutatedVal any) {
		mutatedData, err := json.MarshalIndent(mutatedVal, "", "  ")
		if err != nil {
			return
		}
		desc := path
		if desc == "" {
			desc = "<root>"
		}
		mutations = append(mutations, Mutation{
			Description: desc,
			Data:        mutatedData,
		})
	})
	return mutations, nil
}

func walkAndMutate(path string, val any, report func(string, any)) {
	label := func(suffix string) string {
		if path == "" {
			return suffix
		}
		return path + " " + suffix
	}

	switch v := val.(type) {
	case map[string]any:
		report(label("(nulled)"), nil)
		if len(v) > 0 {
			report(label("(emptied)"), map[string]any{})
		}

		for k, child := range v {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}

			// Mutation: remove field
			mutated := copyMap(v)
			delete(mutated, k)
			report(childPath+" (removed)", mutated)

			// Recurse; rebuild parent for each child mutation
			walkAndMutate(childPath, child, func(p string, mv any) {
				m := copyMap(v)
				m[k] = mv
				report(p, m)
			})
		}
	case []any:
		report(label("(nulled)"), nil)
		if len(v) > 0 {
			report(label("(emptied)"), []any{})
		}

		for i, child := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)

			// Mutation: remove element
			mutated := append([]any{}, v[:i]...)
			mutated = append(mutated, v[i+1:]...)
			report(childPath+" (removed)", mutated)

			walkAndMutate(childPath, child, func(p string, mv any) {
				m := append([]any{}, v...)
				m[i] = mv
				report(p, m)
			})
		}
	case string:
		report(label("(nulled)"), nil)
		if v != "" {
			report(label("(emptied)"), "")
		}
		report(label("(mutated)"), "MUTATED_"+v)
	case float64:
		report(label("(nulled)"), nil)
		if v != 0 {
			report(label("(zeroed)"), 0)
		}
		if v != negativeNumberMutation {
			report(label("(negative)"), negativeNumberMutation)
		}
		if v != largeNumberMutation {
			report(label("(large)"), largeNumberMutation)
		}
		report(label("(incremented)"), v+1)
	case bool:
		report(label("(nulled)"), nil)
		report(label("(toggled)"), !v)
	case nil:
		report(label("(unnulled)"), "MUTATED_NULL")
	}
}

// copyMap returns a shallow copy. Safe here because callers serialize the
// result via json.Marshal before any further mutation runs, so shared subtree
// references are never mutated in place. Do not change that invariant.
func copyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
