package calc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAdd(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "positive"},
		{name: "negative"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			inPath := filepath.Join("..", "tests", "golden", "calc", "TestAdd", tt.name+".in.json")
			outPath := filepath.Join("..", "tests", "golden", "calc", "TestAdd", tt.name+".out.json")
			inData, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			var in struct{ A, B int }
			if err := json.Unmarshal(inData, &in); err != nil {
				t.Fatalf("parse input: %v", err)
			}
			got := Add(in.A, in.B)
			outData, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			var out struct{ Want int }
			if err := json.Unmarshal(outData, &out); err != nil {
				t.Fatalf("parse output: %v", err)
			}
			if got != out.Want {
				t.Errorf("Add(%d,%d) = %d, want %d", in.A, in.B, got, out.Want)
			}
		})
	}
}
