package lsp

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestWithGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	client := NewClient([]string{"gopls"}, "file:///app/testdata/sample-go", 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("start: %v\nstderr: %s", err, client.Stderr())
	}

	_ = client.Shutdown(context.Background())
}
