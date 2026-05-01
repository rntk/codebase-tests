package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client manages an LSP subprocess over JSON-RPC.
type Client struct {
	command []string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  *limitedBuffer
	rootURI string
	timeout time.Duration

	mu      sync.Mutex
	nextID  int
	pending map[int]chan *jsonRPCResponse
	cache   *responseCache

	done chan struct{}
	wg   sync.WaitGroup
}

// NewClient creates an LSP client that will run the given command.
func NewClient(command []string, rootURI string, timeout time.Duration) *Client {
	return &Client{
		command: command,
		rootURI: rootURI,
		timeout: timeout,
		pending: make(map[int]chan *jsonRPCResponse),
		cache:   newResponseCache(),
	}
}

// newClientForTesting creates a client without a subprocess (streams injected later).
func newClientForTesting(rootURI string, timeout time.Duration) *Client {
	return &Client{
		rootURI: rootURI,
		timeout: timeout,
		pending: make(map[int]chan *jsonRPCResponse),
		cache:   newResponseCache(),
	}
}

// Start launches the subprocess and performs the LSP Initialize handshake.
func (c *Client) Start(ctx context.Context) error {
	if len(c.command) == 0 {
		return fmt.Errorf("no command configured")
	}
	cmd := exec.Command(c.command[0], c.command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	c.stdin = stdin
	c.stdout = stdout
	c.stderr = &limitedBuffer{max: 65536}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}
	c.cmd = cmd

	go func() {
		_, _ = io.Copy(c.stderr, stderr)
	}()

	// Give the server a moment to start up.
	time.Sleep(100 * time.Millisecond)

	c.done = make(chan struct{})
	go func() {
		<-c.done
		if c.stdout != nil {
			_ = c.stdout.Close()
		}
	}()
	c.wg.Add(1)
	go c.readLoop()

	return c.doInitialize(ctx)
}

// startWithStreams is used by tests to wire custom readers/writers.
func (c *Client) startWithStreams(stdout io.ReadCloser, stdin io.WriteCloser, stderr io.Reader) error {
	c.stdin = stdin
	c.stdout = stdout
	c.stderr = &limitedBuffer{max: 65536}
	if stderr != nil {
		go func() {
			_, _ = io.Copy(c.stderr, stderr)
		}()
	}
	c.done = make(chan struct{})
	go func() {
		<-c.done
		if c.stdout != nil {
			_ = c.stdout.Close()
		}
	}()
	c.wg.Add(1)
	go c.readLoop()
	return c.doInitialize(context.Background())
}

func (c *Client) doInitialize(ctx context.Context) error {
	initCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := InitializeParams{
		ProcessID:    os.Getpid(),
		RootURI:      c.rootURI,
		Capabilities: map[string]any{},
	}
	if _, err := c.call(initCtx, "initialize", params); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	_ = c.notify("initialized", map[string]any{})
	return nil
}

// Shutdown sends shutdown/exit and terminates the subprocess.
func (c *Client) Shutdown(ctx context.Context) error {
	_, _ = c.call(ctx, "shutdown", nil)
	_ = c.notify("exit", nil)

	if c.done != nil {
		close(c.done)
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
	c.wg.Wait()

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}

// Stderr returns captured stderr output.
func (c *Client) Stderr() string {
	if c.stderr == nil {
		return ""
	}
	return c.stderr.String()
}

// DocumentSymbol returns the hierarchical symbol tree for a document.
func (c *Client) DocumentSymbol(ctx context.Context, uri string) ([]DocumentSymbol, error) {
	key := cacheKey{URI: uri, Version: 0, Method: "textDocument/documentSymbol"}
	params := DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	raw, err := c.callWithCache(ctx, key, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}
	var result []DocumentSymbol
	if err := json.Unmarshal(raw, &result); err != nil {
		// Try SymbolInformation fallback.
		var infos []SymbolInformation
		if err2 := json.Unmarshal(raw, &infos); err2 != nil {
			return nil, err
		}
		for _, info := range infos {
			result = append(result, DocumentSymbol{
				Name:  info.Name,
				Kind:  info.Kind,
				Range: info.Location.Range,
			})
		}
	}
	return result, nil
}

// Definition returns the definition location(s) for the position.
func (c *Client) Definition(ctx context.Context, uri string, line, character int) ([]Location, error) {
	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}
	raw, err := c.call(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}
	var result []Location
	if err := json.Unmarshal(raw, &result); err != nil {
		var loc Location
		if err2 := json.Unmarshal(raw, &loc); err2 != nil {
			return nil, err
		}
		result = []Location{loc}
	}
	return result, nil
}

// References returns reference locations for the position.
func (c *Client) References(ctx context.Context, uri string, line, character int) ([]Location, error) {
	params := ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
		Context:      ReferenceContext{IncludeDeclaration: true},
	}
	raw, err := c.call(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}
	var result []Location
	if err := json.Unmarshal(raw, &result); err != nil {
		var loc Location
		if err2 := json.Unmarshal(raw, &loc); err2 != nil {
			return nil, err
		}
		result = []Location{loc}
	}
	return result, nil
}

// PrepareCallHierarchy returns call-hierarchy items for the position.
func (c *Client) PrepareCallHierarchy(ctx context.Context, uri string, line, character int) ([]CallHierarchyItem, error) {
	params := CallHierarchyPrepareParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}
	raw, err := c.call(ctx, "textDocument/prepareCallHierarchy", params)
	if err != nil {
		return nil, err
	}
	var result []CallHierarchyItem
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// IncomingCalls returns callers for a call-hierarchy item.
func (c *Client) IncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	params := CallHierarchyIncomingCallsParams{Item: item}
	raw, err := c.call(ctx, "callHierarchy/incomingCalls", params)
	if err != nil {
		return nil, err
	}
	var result []CallHierarchyIncomingCall
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// OutgoingCalls returns callees for a call-hierarchy item.
func (c *Client) OutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	params := CallHierarchyOutgoingCallsParams{Item: item}
	raw, err := c.call(ctx, "callHierarchy/outgoingCalls", params)
	if err != nil {
		return nil, err
	}
	var result []CallHierarchyOutgoingCall
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DidOpen notifies the server that a document is open.
func (c *Client) DidOpen(ctx context.Context, uri, languageID string, version int, text string) error {
	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    version,
			Text:       text,
		},
	}
	return c.notify("textDocument/didOpen", params)
}

// internal JSON-RPC machinery.

func (c *Client) callWithCache(ctx context.Context, key cacheKey, method string, params any) (json.RawMessage, error) {
	if data, ok := c.cache.Get(key); ok {
		return data, nil
	}
	result, err := c.call(ctx, method, params)
	if err != nil {
		return nil, err
	}
	c.cache.Set(key, result)
	return result, nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan *jsonRPCResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  mustMarshal(params),
	}
	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "[LSP] call method=%s id=%d\n", method, id)
	if err := c.write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "[LSP] call id=%d context done\n", id)
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		fmt.Fprintf(os.Stderr, "[LSP] call id=%d got response\n", id)
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params any) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustMarshal(params),
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.write(data)
}

func (c *Client) write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("client not started")
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	_, err := c.stdin.Write(data)
	return err
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	reader := bufio.NewReader(c.stdout)
	fmt.Fprintf(os.Stderr, "[LSP] readLoop started\n")
	for {
		select {
		case <-c.done:
			fmt.Fprintf(os.Stderr, "[LSP] readLoop done signal\n")
			return
		default:
		}

		data, err := readMessage(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[LSP] readLoop error: %v\n", err)
			return
		}
		fmt.Fprintf(os.Stderr, "[LSP] readLoop got message: %s\n", string(data))

		var resp jsonRPCResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "[LSP] readLoop unmarshal error: %v\n", err)
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			fmt.Fprintf(os.Stderr, "[LSP] readLoop delivering response id=%d\n", resp.ID)
			ch <- &resp
			delete(c.pending, resp.ID)
		} else {
			fmt.Fprintf(os.Stderr, "[LSP] readLoop no pending for id=%d\n", resp.ID)
		}
		c.mu.Unlock()
	}
}

// limitedBuffer captures output up to a limit.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len()+len(p) > b.max {
		if b.buf.Len() < b.max {
			b.buf.Write(p[:b.max-b.buf.Len()])
			b.buf.WriteString("\n...truncated...")
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// responseCache is keyed by (URI, version, method).
type responseCache struct {
	mu    sync.RWMutex
	items map[cacheKey]json.RawMessage
}

type cacheKey struct {
	URI     string
	Version int
	Method  string
}

func newResponseCache() *responseCache {
	return &responseCache{items: make(map[cacheKey]json.RawMessage)}
}

func (rc *responseCache) Get(key cacheKey) (json.RawMessage, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	v, ok := rc.items[key]
	return v, ok
}

func (rc *responseCache) Set(key cacheKey, value json.RawMessage) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.items[key] = value
}

// LSP message framing.

func readMessage(r *bufio.Reader) ([]byte, error) {
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			if n, err := strconv.Atoi(val); err == nil {
				length = n
			}
		}
	}
	if length == 0 {
		return nil, fmt.Errorf("no Content-Length")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// JSON-RPC types.

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LSP protocol types.

type InitializeParams struct {
	ProcessID             int             `json:"processId"`
	RootURI               string          `json:"rootUri,omitempty"`
	Capabilities          map[string]any  `json:"capabilities"`
	InitializationOptions any             `json:"initializationOptions,omitempty"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type CallHierarchyPrepareParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DocumentSymbol represents a hierarchical symbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Deprecated     bool             `json:"deprecated,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation is the flat fallback.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Deprecated    bool     `json:"deprecated,omitempty"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// CallHierarchyItem represents an item in the call hierarchy.
type CallHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	Tags           []int    `json:"tags,omitempty"`
	Detail         string   `json:"detail,omitempty"`
	URI            string   `json:"uri"`
	Range          Range    `json:"range"`
	SelectionRange Range    `json:"selectionRange"`
	Data           any      `json:"data,omitempty"`
}

// CallHierarchyIncomingCall represents an incoming call.
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyOutgoingCall represents an outgoing call.
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
