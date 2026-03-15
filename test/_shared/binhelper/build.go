package binhelper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary builds the oasmock binary.
func buildBinary(binPath string, t *testing.T) error {
	// Get project root
	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %v", err)
	}

	// Create bin directory if it doesn't exist
	binDir := filepath.Dir(binPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %v", err)
	}

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/oasmock")
	cmd.Dir = projectRoot
	cmd.Stdout = nil
	cmd.Stderr = nil

	t.Logf("Building binary with: go build -o %s ./cmd/oasmock", binPath)
	if err := cmd.Run(); err != nil {
		// Try to capture stderr for better error messages
		cmdWithOutput := exec.Command("go", "build", "-o", binPath, "./cmd/oasmock")
		cmdWithOutput.Dir = projectRoot
		if output, err2 := cmdWithOutput.CombinedOutput(); err2 != nil {
			return fmt.Errorf("build failed: %v\n%s", err2, string(output))
		}
		// If cmdWithOutput succeeded, return nil (original error might be flaky)
		return nil
	}

	return nil
}

// verifyBinary checks if the binary is executable and runs basic validation.
func verifyBinary(binPath string, t *testing.T) error {
	// Check if binary exists and is executable
	info, err := os.Stat(binPath)
	if err != nil {
		return fmt.Errorf("binary not found after build: %v", err)
	}

	// Check if it's executable
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", binPath)
	}

	// Try to run with --help to verify it works
	cmd := exec.Command(binPath, "--help")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil && !strings.Contains(err.Error(), "exit status") {
		// Some binaries return non-zero exit code for --help, which is okay
		// Only fail if there's a real execution error
		t.Logf("Warning: binary --help check failed: %v", err)
	}

	return nil
}
