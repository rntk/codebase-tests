package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeServer reads requests from in and writes responses to out.
type fakeServer struct {
	in     *bufio.Reader
	out    io.WriteCloser
	handler func(method string, params json.RawMessage) (any, error)
}

func (s *fakeServer) run() {
	defer s.out.Close()
	for {
		data, err := readMessage(s.in)
		if err != nil {
			return
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}
		if req.Method == "initialized" || req.Method == "textDocument/didOpen" || req.Method == "exit" {
			continue // notifications, no response needed
		}
		var result any
		var resErr error
		if s.handler != nil {
			result, resErr = s.handler(req.Method, req.Params)
		}
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
		if resErr != nil {
			resp.Error = &jsonRPCError{Code: -32600, Message: resErr.Error()}
		} else {
			resp.Result = mustMarshal(result)
		}
		b, _ := json.Marshal(resp)
		_ = writeMessage(s.out, b)
	}
}

func writeMessage(w io.Writer, msg []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
	if _, err := w.Write([]byte(header)); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}

func setupClientAndServer(t *testing.T, handler func(method string, params json.RawMessage) (any, error)) (*Client, func()) {
	// clientStdoutR <- server writes, client reads
	clientStdoutR, clientStdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	// clientStdinW -> server reads, client writes
	clientStdinR, clientStdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	server := &fakeServer{
		in:      bufio.NewReader(clientStdinR),
		out:     clientStdoutW,
		handler: handler,
	}
	go server.run()

	client := newClientForTesting("file:///tmp", 5*time.Second)
	if err := client.startWithStreams(clientStdoutR, clientStdinW, nil); err != nil {
		t.Fatalf("client start: %v", err)
	}

	cleanup := func() {
		_ = client.Shutdown(context.Background())
		_ = clientStdoutR.Close()
		_ = clientStdinW.Close()
	}
	return client, cleanup
}

func TestClientInitialize(t *testing.T) {
	called := false
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		if method == "initialize" {
			called = true
			return map[string]any{"capabilities": map[string]any{}}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	if !called {
		t.Fatal("initialize was not called")
	}
	_ = client
}

func TestClientDocumentSymbol(t *testing.T) {
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/documentSymbol":
			return []DocumentSymbol{
				{
					Name: "Add",
					Kind: 12, // Function
					Range: Range{
						Start: Position{Line: 2, Character: 0},
						End:   Position{Line: 4, Character: 1},
					},
					SelectionRange: Range{
						Start: Position{Line: 2, Character: 5},
						End:   Position{Line: 2, Character: 8},
					},
				},
			}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx := context.Background()
	syms, err := client.DocumentSymbol(ctx, "file:///tmp/add.go")
	if err != nil {
		t.Fatalf("DocumentSymbol: %v", err)
	}
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(syms))
	}
	if syms[0].Name != "Add" {
		t.Fatalf("expected Add, got %s", syms[0].Name)
	}
}

func TestClientDocumentSymbolCache(t *testing.T) {
	callCount := 0
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/documentSymbol":
			callCount++
			return []DocumentSymbol{
				{Name: "Foo", Kind: 12, Range: Range{Start: Position{Line: 0}, End: Position{Line: 1}}},
			}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx := context.Background()
	_, _ = client.DocumentSymbol(ctx, "file:///tmp/a.go")
	_, _ = client.DocumentSymbol(ctx, "file:///tmp/a.go")
	if callCount != 1 {
		t.Fatalf("expected 1 server call for cached request, got %d", callCount)
	}
}

func TestClientDefinition(t *testing.T) {
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/definition":
			return []Location{
				{
					URI:   "file:///tmp/add.go",
					Range: Range{Start: Position{Line: 2, Character: 5}, End: Position{Line: 2, Character: 8}},
				},
			}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx := context.Background()
	locs, err := client.Definition(ctx, "file:///tmp/main.go", 5, 10)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) != 1 || locs[0].URI != "file:///tmp/add.go" {
		t.Fatalf("unexpected locations: %+v", locs)
	}
}

func TestClientReferences(t *testing.T) {
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/references":
			return []Location{
				{URI: "file:///tmp/a.go", Range: Range{Start: Position{Line: 1}}},
				{URI: "file:///tmp/b.go", Range: Range{Start: Position{Line: 2}}},
			}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx := context.Background()
	locs, err := client.References(ctx, "file:///tmp/a.go", 1, 1)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 references, got %d", len(locs))
	}
}

func TestClientCallHierarchy(t *testing.T) {
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/prepareCallHierarchy":
			return []CallHierarchyItem{
				{Name: "Add", Kind: 12, URI: "file:///tmp/add.go", Range: Range{Start: Position{Line: 2}}},
			}, nil
		case "callHierarchy/incomingCalls":
			return []CallHierarchyIncomingCall{
				{From: CallHierarchyItem{Name: "TestAdd", Kind: 12, URI: "file:///tmp/add_test.go"}},
			}, nil
		case "callHierarchy/outgoingCalls":
			return []CallHierarchyOutgoingCall{
				{To: CallHierarchyItem{Name: "Sub", Kind: 12, URI: "file:///tmp/sub.go"}},
			}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx := context.Background()
	items, err := client.PrepareCallHierarchy(ctx, "file:///tmp/add.go", 2, 5)
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Add" {
		t.Fatalf("unexpected items: %+v", items)
	}

	inc, err := client.IncomingCalls(ctx, items[0])
	if err != nil {
		t.Fatalf("IncomingCalls: %v", err)
	}
	if len(inc) != 1 || inc[0].From.Name != "TestAdd" {
		t.Fatalf("unexpected incoming: %+v", inc)
	}

	out, err := client.OutgoingCalls(ctx, items[0])
	if err != nil {
		t.Fatalf("OutgoingCalls: %v", err)
	}
	if len(out) != 1 || out[0].To.Name != "Sub" {
		t.Fatalf("unexpected outgoing: %+v", out)
	}
}

func TestClientContextCancellation(t *testing.T) {
	client, cleanup := setupClientAndServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "textDocument/documentSymbol":
			time.Sleep(500 * time.Millisecond)
			return []DocumentSymbol{}, nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.DocumentSymbol(ctx, "file:///tmp/x.go")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestClientStderrCapture(t *testing.T) {
	// clientStdoutR <- server writes, client reads
	clientStdoutR, clientStdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	// clientStdinW -> server reads, client writes
	clientStdinR, clientStdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	// stderr pipe
	clientStderrR, clientStderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	server := &fakeServer{
		in:  bufio.NewReader(clientStdinR),
		out: clientStdoutW,
		handler: func(method string, params json.RawMessage) (any, error) {
			if method == "initialize" {
				return map[string]any{"capabilities": map[string]any{}}, nil
			}
			return nil, fmt.Errorf("unexpected method %s", method)
		},
	}
	go server.run()

	// Write something to stderr
	go func() {
		_, _ = clientStderrW.Write([]byte("some warning\n"))
		_ = clientStderrW.Close()
	}()

	client := newClientForTesting("file:///tmp", 5*time.Second)
	if err := client.startWithStreams(clientStdoutR, clientStdinW, clientStderrR); err != nil {
		t.Fatalf("client start: %v", err)
	}
	defer func() {
		_ = client.Shutdown(context.Background())
	}()

	// Give the stderr goroutine time to copy data.
	time.Sleep(50 * time.Millisecond)

	if !strings.Contains(client.Stderr(), "some warning") {
		t.Fatalf("expected stderr to contain warning, got: %q", client.Stderr())
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &limitedBuffer{max: 10}
	n, err := b.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 11 {
		t.Fatalf("expected write to report 11, got %d", n)
	}
	s := b.String()
	if !strings.Contains(s, "hello worl") || !strings.Contains(s, "truncated") {
		t.Fatalf("unexpected buffer content: %q", s)
	}
}
