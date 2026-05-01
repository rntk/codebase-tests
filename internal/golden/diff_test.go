package golden

import (
	"encoding/json"
	"testing"
)

func parseJSON(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

func TestDiffJSONObject(t *testing.T) {
	old := parseJSON(`{"a": 1, "b": 2}`)
	new := parseJSON(`{"a": 1, "b": 3, "c": 4}`)

	root := DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Fatalf("expected root kind changed, got %s", root.Kind)
	}
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}

	byKey := map[string]DiffNode{}
	for _, c := range root.Children {
		byKey[c.Key] = c
	}

	if byKey["a"].Kind != "unchanged" {
		t.Errorf("a: expected unchanged, got %s", byKey["a"].Kind)
	}
	if byKey["b"].Kind != "changed" {
		t.Errorf("b: expected changed, got %s", byKey["b"].Kind)
	}
	if byKey["c"].Kind != "added" {
		t.Errorf("c: expected added, got %s", byKey["c"].Kind)
	}
}

func TestDiffJSONArray(t *testing.T) {
	old := parseJSON(`[1, 2, 3]`)
	new := parseJSON(`[1, 4]`)

	root := DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Fatalf("expected root kind changed, got %s", root.Kind)
	}
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}

	if root.Children[0].Kind != "unchanged" {
		t.Errorf("index 0: expected unchanged, got %s", root.Children[0].Kind)
	}
	if root.Children[1].Kind != "changed" {
		t.Errorf("index 1: expected changed, got %s", root.Children[1].Kind)
	}
	if root.Children[2].Kind != "removed" {
		t.Errorf("index 2: expected removed, got %s", root.Children[2].Kind)
	}
}

func TestDiffJSONScalar(t *testing.T) {
	old := parseJSON(`42`)
	new := parseJSON(`42`)
	root := DiffJSON(old, new)
	if root.Kind != "unchanged" {
		t.Errorf("expected unchanged, got %s", root.Kind)
	}

	old = parseJSON(`42`)
	new = parseJSON(`43`)
	root = DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Errorf("expected changed, got %s", root.Kind)
	}
	if root.OldValue != float64(42) {
		t.Errorf("oldValue = %v", root.OldValue)
	}
	if root.NewValue != float64(43) {
		t.Errorf("newValue = %v", root.NewValue)
	}
}

func TestDiffJSONTypeChange(t *testing.T) {
	old := parseJSON(`1`)
	new := parseJSON(`"hello"`)
	root := DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Errorf("expected changed, got %s", root.Kind)
	}

	old = parseJSON(`[1, 2]`)
	new = parseJSON(`{"a": 1}`)
	root = DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Errorf("expected changed for array->object, got %s", root.Kind)
	}
}

func TestDiffJSONNested(t *testing.T) {
	old := parseJSON(`{"outer": {"inner": 1, "keep": 2}}`)
	new := parseJSON(`{"outer": {"inner": 2, "keep": 2}}`)

	root := DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Fatalf("expected root changed, got %s", root.Kind)
	}
	outer := root.Children[0]
	if outer.Key != "outer" || outer.Kind != "changed" {
		t.Fatalf("expected outer changed, got %s", outer.Kind)
	}
	if len(outer.Children) != 2 {
		t.Fatalf("expected 2 inner children, got %d", len(outer.Children))
	}
	byKey := map[string]DiffNode{}
	for _, c := range outer.Children {
		byKey[c.Key] = c
	}
	if byKey["inner"].Kind != "changed" {
		t.Errorf("inner: expected changed, got %s", byKey["inner"].Kind)
	}
	if byKey["keep"].Kind != "unchanged" {
		t.Errorf("keep: expected unchanged, got %s", byKey["keep"].Kind)
	}
}

func TestDiffJSONAddedRemovedRoot(t *testing.T) {
	old := parseJSON(`{"a": 1}`)
	root := DiffJSON(nil, old)
	if root.Kind != "added" {
		t.Errorf("expected added, got %s", root.Kind)
	}

	root = DiffJSON(old, nil)
	if root.Kind != "removed" {
		t.Errorf("expected removed, got %s", root.Kind)
	}
}

func TestDiffJSONArrayAppend(t *testing.T) {
	old := parseJSON(`[1]`)
	new := parseJSON(`[1, 2]`)
	root := DiffJSON(old, new)
	if root.Kind != "changed" {
		t.Fatalf("expected changed, got %s", root.Kind)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}
	if root.Children[1].Kind != "added" {
		t.Errorf("index 1: expected added, got %s", root.Children[1].Kind)
	}
}
