package golden

import (
	"fmt"
	"reflect"
	"sort"
)

// DiffNode represents a structural difference between two JSON values.
type DiffNode struct {
	Kind     string     `json:"kind"`          // added, removed, changed, unchanged
	Key      string     `json:"key,omitempty"` // object key or array index
	OldValue any        `json:"oldValue,omitempty"`
	NewValue any        `json:"newValue,omitempty"`
	Children []DiffNode `json:"children,omitempty"`
}

// DiffJSON computes a structural diff between two JSON values.
func DiffJSON(old, new any) *DiffNode {
	return diffValue(old, new)
}

func diffValue(old, new any) *DiffNode {
	if old == nil && new == nil {
		return &DiffNode{Kind: "unchanged"}
	}
	if old == nil {
		return &DiffNode{Kind: "added", NewValue: new}
	}
	if new == nil {
		return &DiffNode{Kind: "removed", OldValue: old}
	}

	oldMap, oldIsMap := old.(map[string]any)
	newMap, newIsMap := new.(map[string]any)
	oldArr, oldIsArr := old.([]any)
	newArr, newIsArr := new.([]any)

	if oldIsMap && newIsMap {
		return diffObject(oldMap, newMap)
	}
	if oldIsArr && newIsArr {
		return diffArray(oldArr, newArr)
	}

	if oldIsMap || oldIsArr || newIsMap || newIsArr || !reflect.DeepEqual(old, new) {
		return &DiffNode{Kind: "changed", OldValue: old, NewValue: new}
	}

	return &DiffNode{Kind: "unchanged", OldValue: old}
}

func diffObject(old, new map[string]any) *DiffNode {
	node := &DiffNode{Kind: "unchanged", Children: []DiffNode{}}
	allKeys := make(map[string]bool)
	for k := range old {
		allKeys[k] = true
	}
	for k := range new {
		allKeys[k] = true
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	hasChanges := false
	for _, k := range keys {
		ov, oOk := old[k]
		nv, nOk := new[k]
		var child DiffNode
		if !oOk {
			child = DiffNode{Kind: "added", Key: k, NewValue: nv}
			hasChanges = true
		} else if !nOk {
			child = DiffNode{Kind: "removed", Key: k, OldValue: ov}
			hasChanges = true
		} else {
			child = *diffValue(ov, nv)
			child.Key = k
			if child.Kind != "unchanged" {
				hasChanges = true
			}
		}
		node.Children = append(node.Children, child)
	}

	if hasChanges {
		node.Kind = "changed"
	}
	return node
}

func diffArray(old, new []any) *DiffNode {
	node := &DiffNode{Kind: "unchanged", Children: []DiffNode{}}
	maxLen := len(old)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	hasChanges := false
	for i := 0; i < maxLen; i++ {
		var child DiffNode
		if i >= len(old) {
			child = DiffNode{Kind: "added", Key: fmt.Sprintf("%d", i), NewValue: new[i]}
			hasChanges = true
		} else if i >= len(new) {
			child = DiffNode{Kind: "removed", Key: fmt.Sprintf("%d", i), OldValue: old[i]}
			hasChanges = true
		} else {
			child = *diffValue(old[i], new[i])
			child.Key = fmt.Sprintf("%d", i)
			if child.Kind != "unchanged" {
				hasChanges = true
			}
		}
		node.Children = append(node.Children, child)
	}

	if hasChanges {
		node.Kind = "changed"
	}
	return node
}
