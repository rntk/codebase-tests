package lsp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestGoplsDirect(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	cmd := exec.Command("gopls")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				t.Logf("stderr: %s", buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	cmd.Start()
	defer cmd.Process.Kill()

	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":%d,"rootUri":"file:///app/testdata/sample-go","capabilities":{}}}`, os.Getpid())
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	t.Log("sending init")
	stdin.Write([]byte(msg))

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read line: %v", err)
		}
		t.Logf("line: %q", line)
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	length := 5510
	buf := make([]byte, length)
	_, err := io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	t.Logf("body: %s", buf)
}
