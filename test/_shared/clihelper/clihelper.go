package clihelper

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/mamonth/oasmock/test/_shared/binhelper"
)

// getBinaryPath returns the absolute path to the oasmock binary, building it if necessary.
func getBinaryPath(t *testing.T) string {
	return binhelper.GetBuilded(t)
}

// findFreePort finds and returns a free TCP port.
func findFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// FindFreePort finds and returns a free TCP port.
// Deprecated: Use Cmd(t).SetPort(0).Run() instead.
func FindFreePort(t *testing.T) int {
	return findFreePort(t)
}

// ServerBuilder provides a fluent interface for configuring and starting an oasmock server.
// No validation is performed - invalid configurations will be caught by the CLI.
type ServerBuilder struct {
	t            *testing.T
	port         int      // 0 = auto-find free port
	schemas      []string // --from values
	prefixes     []string // --prefix values (optional, must match schemas length if provided)
	verbose      bool     // --verbose
	delay        int      // --delay (milliseconds)
	noCORS       bool     // --nocors
	historySize  int      // --history-size
	noControlAPI bool     // --no-control-api
	extraArgs    []string // Additional raw CLI arguments
	env          []string // Environment variables
}

// Cmd creates a new ServerBuilder for configuring an oasmock server.
func Cmd(t *testing.T) *ServerBuilder {
	return &ServerBuilder{t: t}
}

// SetPort sets the port to listen on. Use 0 to auto-find a free port.
func (b *ServerBuilder) SetPort(port int) *ServerBuilder {
	b.port = port
	return b
}

// SetSchema adds a schema file with an optional prefix.
// Can be called multiple times to add multiple schemas.
func (b *ServerBuilder) SetSchema(schema string, prefix string) *ServerBuilder {
	b.schemas = append(b.schemas, schema)
	b.prefixes = append(b.prefixes, prefix)
	return b
}

// SetVerbose enables or disables verbose logging.
func (b *ServerBuilder) SetVerbose(verbose bool) *ServerBuilder {
	b.verbose = verbose
	return b
}

// SetDelay sets the delay between request and response in milliseconds.
func (b *ServerBuilder) SetDelay(milliseconds int) *ServerBuilder {
	b.delay = milliseconds
	return b
}

// DisableCORS enables or disables CORS headers.
func (b *ServerBuilder) DisableCORS(disable bool) *ServerBuilder {
	b.noCORS = disable
	return b
}

// SetHistorySize sets the maximum number of requests to keep in history.
func (b *ServerBuilder) SetHistorySize(size int) *ServerBuilder {
	b.historySize = size
	return b
}

// DisableControlAPI enables or disables the management control API.
func (b *ServerBuilder) DisableControlAPI(disable bool) *ServerBuilder {
	b.noControlAPI = disable
	return b
}

// AddArg adds a raw CLI argument for advanced configuration.
func (b *ServerBuilder) AddArg(arg string) *ServerBuilder {
	b.extraArgs = append(b.extraArgs, arg)
	return b
}

// SetEnv sets environment variables for the server process.
func (b *ServerBuilder) SetEnv(env []string) *ServerBuilder {
	b.env = env
	return b
}

// Run starts the configured oasmock server and returns the command,
// error channel, and actual port used (useful when port=0).
func (b *ServerBuilder) Run() (*exec.Cmd, <-chan error, int) {
	binaryPath := getBinaryPath(b.t)

	// Determine actual port
	actualPort := b.port
	if actualPort <= 0 {
		actualPort = findFreePort(b.t)
	}

	// Build arguments
	args := []string{"mock"}
	for i, schema := range b.schemas {
		args = append(args, "--from", schema)
		if i < len(b.prefixes) && b.prefixes[i] != "" {
			args = append(args, "--prefix", b.prefixes[i])
		}
	}
	args = append(args, "--port", fmt.Sprintf("%d", actualPort))
	if b.verbose {
		args = append(args, "--verbose")
	}
	if b.delay > 0 {
		args = append(args, "--delay", fmt.Sprintf("%d", b.delay))
	}
	if b.noCORS {
		args = append(args, "--nocors")
	}
	if b.historySize > 0 {
		args = append(args, "--history-size", fmt.Sprintf("%d", b.historySize))
	}
	if b.noControlAPI {
		args = append(args, "--no-control-api")
	}
	args = append(args, b.extraArgs...)

	cmd := exec.Command(binaryPath, args...)
	if len(b.env) > 0 {
		cmd.Env = append(os.Environ(), b.env...)
	}

	// Capture stderr for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		b.t.Fatalf("failed to get stderr pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		b.t.Fatalf("failed to start oasmock: %v", err)
	}

	// Create error channel
	errCh := make(chan error, 1)
	go func() {
		// Wait for command to exit
		err := cmd.Wait()
		errCh <- err
		close(errCh)
	}()

	// Read stderr output for debugging
	go func() {
		// Capture t early to avoid race with test completion
		t := b.t
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			// Use recover to avoid panic if test has already completed
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Test likely completed, ignore logging
					}
				}()
				t.Logf("oasmock stderr: %s", scanner.Text())
			}()
		}
		if err := scanner.Err(); err != nil {
			func() {
				defer func() { recover() }()
				t.Logf("stderr scanner error: %v", err)
			}()
		}
	}()

	return cmd, errCh, actualPort
}

// StartServer starts the oasmock CLI server as a subprocess.
// Returns the command, an error channel that will receive the exit error, and the actual port used.
// Deprecated: Use Cmd(t).SetSchema(schemaFile, "").SetPort(port).AddArg(extraArgs...).Run() instead.
func StartServer(t *testing.T, port int, schemaFile string, extraArgs ...string) (*exec.Cmd, <-chan error, int) {
	builder := Cmd(t).SetSchema(schemaFile, "").SetPort(port)
	for _, arg := range extraArgs {
		builder = builder.AddArg(arg)
	}
	return builder.Run()
}

// WaitForServer waits for the server to be ready by attempting to connect.
func WaitForServer(t *testing.T, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// StopServer stops the CLI server process.
func StopServer(t *testing.T, cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// Try graceful shutdown with SIGTERM
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Logf("Failed to send SIGTERM: %v", err)
		// Fall back to kill
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}

	// Wait a bit for process to exit
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(2 * time.Second):
		t.Logf("Timeout waiting for process to exit")
	}
}

// RunCmd executes a CLI command with optional environment variables.
// Returns the combined output (stdout+stderr) and a cleanup function to kill the process.
func RunCmd(t *testing.T, args []string, env []string) (string, func()) {
	binaryPath := getBinaryPath(t)

	cmd := exec.Command(binaryPath, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

	outputPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	// Read output asynchronously
	outputChan := make(chan string, 1)
	go func() {
		var output []byte
		buf := make([]byte, 1024)
		for {
			n, err := outputPipe.Read(buf)
			if n > 0 {
				output = append(output, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		// Also read stderr
		stderrBuf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(stderrBuf)
			if n > 0 {
				output = append(output, stderrBuf[:n]...)
			}
			if err != nil {
				break
			}
		}
		outputChan <- string(output)
	}()

	cleanup := func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}

	// Wait a bit for initial output
	select {
	case output := <-outputChan:
		return output, cleanup
	case <-time.After(2 * time.Second):
		cleanup()
		return "", func() {}
	}
}
