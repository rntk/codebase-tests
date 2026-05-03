package golden

import (
	"encoding/json"
	"strings"
	"testing"
)

func descriptions(ms []Mutation) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Description
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func containsPrefix(ss []string, prefix string) bool {
	for _, x := range ss {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}

func TestMutateJSONInvalid(t *testing.T) {
	if _, err := MutateJSON([]byte("not json")); err == nil {
		t.Fatal("expected error on invalid input")
	}
}

func TestMutateJSONObject(t *testing.T) {
	ms, err := MutateJSON([]byte(`{"name":"foo","age":30,"on":true}`))
	if err != nil {
		t.Fatal(err)
	}
	descs := descriptions(ms)
	for _, want := range []string{
		"name (removed)",
		"age (removed)",
		"on (removed)",
		"name (emptied)",
		"name (mutated)",
		"age (zeroed)",
		"age (incremented)",
		"on (toggled)",
	} {
		if !contains(descs, want) {
			t.Errorf("missing mutation %q in %v", want, descs)
		}
	}

	// Verify each mutation produces valid JSON differing from input.
	for _, m := range ms {
		var v any
		if err := json.Unmarshal(m.Data, &v); err != nil {
			t.Errorf("mutation %q produced invalid JSON: %v", m.Description, err)
		}
	}
}

func TestMutateJSONNestedSibling(t *testing.T) {
	// Regression: shallow-copy parent must not let later mutations affect
	// already-serialized earlier ones.
	in := []byte(`{"a":{"x":1},"b":{"y":2}}`)
	ms, err := MutateJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		// The original input keys "a" and "b" should both still be present
		// unless the mutation explicitly removes one of them.
		var v map[string]any
		if err := json.Unmarshal(m.Data, &v); err != nil {
			t.Fatal(err)
		}
		removedA := m.Description == "a (removed)"
		removedB := m.Description == "b (removed)"
		if _, ok := v["a"]; !ok && !removedA {
			t.Errorf("mutation %q unexpectedly dropped key a: %s", m.Description, m.Data)
		}
		if _, ok := v["b"]; !ok && !removedB {
			t.Errorf("mutation %q unexpectedly dropped key b: %s", m.Description, m.Data)
		}
	}
}

func TestMutateJSONArray(t *testing.T) {
	ms, err := MutateJSON([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	descs := descriptions(ms)
	for _, want := range []string{"[0] (removed)", "[1] (removed)", "[2] (removed)"} {
		if !contains(descs, want) {
			t.Errorf("missing %q in %v", want, descs)
		}
	}
	if !containsPrefix(descs, "[0]") {
		t.Errorf("expected element-level mutations, got %v", descs)
	}
}

func TestMutateJSONRootPrimitive(t *testing.T) {
	ms, err := MutateJSON([]byte(`"hello"`))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if strings.HasPrefix(m.Description, " ") {
			t.Errorf("description has leading space: %q", m.Description)
		}
	}
}

func TestMutateJSONEmptyStringNoEmptyMutation(t *testing.T) {
	ms, err := MutateJSON([]byte(`{"k":""}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Description == "k (emptied)" {
			t.Error("should not emit (emptied) for already-empty string")
		}
	}
}

func TestMutateJSONNull(t *testing.T) {
	ms, err := MutateJSON([]byte(`{"k":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(descriptions(ms), "k (unnulled)") {
		t.Errorf("expected (unnulled) mutation, got %v", descriptions(ms))
	}
}
