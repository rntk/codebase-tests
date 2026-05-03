package golden

import (
	"encoding/json"
	"fmt"
)

// MutationResult is the result of running a single mutation.
type MutationResult struct {
	Mutation string `json:"mutation"` // Description of the mutation
	Passed   bool   `json:"passed"`   // Whether the test passed (usually bad for a mutation)
	Output   string `json:"output"`   // Test output
}

// MutateJSON generates a list of mutated JSON contents.
func MutateJSON(data []byte) ([]Mutation, error) {
	var val any
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}

	var mutations []Mutation
	walkAndMutate("", val, func(path string, mutatedVal any) {
		mutatedData, _ := json.MarshalIndent(mutatedVal, "", "  ")
		mutations = append(mutations, Mutation{
			Description: fmt.Sprintf("mutate %s", path),
			Data:        mutatedData,
		})
	})
	return mutations, nil
}

// Mutation represents a single mutation of the JSON data.
type Mutation struct {
	Description string `json:"description"`
	Data        []byte `json:"data"`
}

func walkAndMutate(path string, val any, report func(string, any)) {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}

			// Mutation: Remove field
			mutated := copyMap(v)
			delete(mutated, k)
			report(childPath+" (removed)", mutated)

			// Recurse
			walkAndMutate(childPath, child, func(p string, mv any) {
				m := copyMap(v)
				m[k] = mv
				report(p, m)
			})
		}
	case []any:
		for i, child := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)

			// Mutation: Remove element
			mutated := make([]any, 0, len(v)-1)
			mutated = append(mutated, v[:i]...)
			mutated = append(mutated, v[i+1:]...)
			report(childPath+" (removed)", mutated)

			// Recurse
			walkAndMutate(childPath, child, func(p string, mv any) {
				m := make([]any, len(v))
				copy(m, v)
				m[i] = mv
				report(p, m)
			})
		}
	case string:
		if val != "" {
			report(path+" (empty string)", "")
		}
		report(path+" (changed string)", "MUTATED_"+v)
	case float64:
		if v != 0 {
			report(path+" (zero)", 0)
		}
		report(path+" (increment)", v+1)
	case bool:
		report(path+" (toggle)", !v)
	}
}

func copyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
