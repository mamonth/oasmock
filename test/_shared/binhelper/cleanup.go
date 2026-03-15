package binhelper

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// initSignalHandler sets up signal handling for clean shutdown.
func initSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, cleaning up...\n", sig)
		cleanupOnSignal()
		os.Exit(1)
	}()
}

// cleanupOnSignal performs cleanup when a termination signal is received.
func cleanupOnSignal() {
	// Get project root to find bin directory
	projectRoot, err := getProjectRoot()
	if err != nil {
		fmt.Printf("Warning: failed to get project root during cleanup: %v\n", err)
		return
	}

	binDir := filepath.Join(projectRoot, "bin")

	// Release any lock we hold
	cleanupLock()

	// Clean up any caller markers from this process
	cleanupProcessMarkers(binDir, os.Getpid())

	// Remove lock file if we own it (already done in cleanupLock)
	// Also clean up any stale locks
	cleanupAllLocks(binDir)
}

// cleanupProcessMarkers removes caller markers belonging to the current process.
func cleanupProcessMarkers(binDir string, pid int) {
	callerMarkersMu.Lock()
	defer callerMarkersMu.Unlock()

	callersDir := filepath.Join(binDir, callersDirName)
	if !dirExists(callersDir) {
		return
	}

	entries, err := os.ReadDir(callersDir)
	if err != nil {
		return
	}

	pidStr := fmt.Sprintf("_%d_", pid)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if marker belongs to this process
		if strings.Contains(entry.Name(), pidStr) {
			markerPath := filepath.Join(callersDir, entry.Name())
			if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
				fmt.Printf("Warning: failed to remove caller marker %s: %v\n", markerPath, err)
			}
			delete(callerMarkers, entry.Name())
		}
	}

	// Remove empty callers directory
	if dirExists(callersDir) {
		entries, err := os.ReadDir(callersDir)
		if err == nil && len(entries) == 0 {
			os.Remove(callersDir)
		}
	}
}

// cleanupAllResources cleans up all binhelper resources (for testing).
func cleanupAllResources(t *testing.T) {
	projectRoot, err := getProjectRoot()
	if err != nil {
		t.Logf("Warning: failed to get project root during cleanup: %v", err)
		return
	}

	binDir := filepath.Join(projectRoot, "bin")

	// Clean up lock
	cleanupLock()

	// Clean up all caller markers
	callersDir := filepath.Join(binDir, callersDirName)
	if dirExists(callersDir) {
		entries, err := os.ReadDir(callersDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					markerPath := filepath.Join(callersDir, entry.Name())
					os.Remove(markerPath)
				}
			}
		}
		// Remove directory
		os.Remove(callersDir)
	}

	// Remove test-built marker
	testBuiltPath := filepath.Join(binDir, testBuiltMarker)
	os.Remove(testBuiltPath)

	// Remove lock file
	lockPath := filepath.Join(binDir, lockFileName)
	os.Remove(lockPath)
}
