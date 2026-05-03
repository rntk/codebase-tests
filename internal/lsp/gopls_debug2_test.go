package lsp

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func TestGoplsRaw(t *testing.T) {
	cmd := exec.Command("gopls")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	cmd.Start()
	defer cmd.Process.Kill()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				t.Logf("stdout raw: %q", buf[:n])
			}
			if err != nil {
				t.Logf("stdout err: %v", err)
				return
			}
		}
	}()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":123,"rootUri":"file:///app/testdata/sample-go","capabilities":{}}}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	t.Log("sending")
	stdin.Write([]byte(msg))

	time.Sleep(2 * time.Second)
	t.Log("done")
}

func TestGoplsWithClientStdin(t *testing.T) {
	cmd := exec.Command("gopls")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	cmd.Start()
	defer cmd.Process.Kill()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				t.Logf("stdout raw: %q", buf[:n])
			}
			if err != nil {
				t.Logf("stdout err: %v", err)
				return
			}
		}
	}()

	// Use our client's exact write path
	client := newClientForTesting("file:///app/testdata/sample-go", 10*time.Second)
	client.stdin = stdin

	body := mustMarshal(InitializeParams{
		ProcessID:    123,
		RootURI:      "file:///app/testdata/sample-go",
		Capabilities: map[string]any{},
	})
	req := jsonRPCRequest{JSONRPC: "2.0", ID: 0, Method: "initialize", Params: body}
	data, _ := json.Marshal(req)

	t.Log("writing")
	client.write(data)

	time.Sleep(2 * time.Second)
	t.Log("done")
}
