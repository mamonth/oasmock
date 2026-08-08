package binhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var callerCounter atomic.Int64

// registerCaller creates a caller marker and returns its ID.
func registerCaller(t *testing.T, binDir string) string {
	callersDir := filepath.Join(binDir, callersDirName)

	// Create callers directory if it doesn't exist
	if err := os.MkdirAll(callersDir, 0755); err != nil {
		func() { defer func() { recover() }(); t.Logf("Warning: failed to create callers directory: %v", err) }()
		// Continue anyway, cleanup will handle missing directory
		return ""
	}

	// Generate unique caller ID: {package}_{pid}_{timestamp}
	pid := os.Getpid()
	timestamp := time.Now().Format("20060102T150405")
	callerID := fmt.Sprintf("test_%d_%s_%d", pid, timestamp, callerCounter.Add(1))

	// Create marker file
	markerPath := filepath.Join(callersDir, callerID)
	if err := os.WriteFile(markerPath, []byte(timestamp), 0644); err != nil {
		func() { defer func() { recover() }(); t.Logf("Warning: failed to create caller marker: %v", err) }()
		return ""
	}

	// Store for cleanup
	callerMarkersMu.Lock()
	callerMarkers[callerID] = markerPath
	callerMarkersMu.Unlock()

	func() { defer func() { recover() }(); t.Logf("Registered caller %s", callerID) }()
	return callerID
}

// cleanupCaller removes the caller marker.
// The binary is a process-level resource — it is not deleted per-test
// to avoid breaking subsequent tests that reuse the same binary.
func cleanupCaller(callerID, binDir string) {
	// Remove our caller marker if it exists
	callerMarkersMu.Lock()
	markerPath, ok := callerMarkers[callerID]
	if ok {
		delete(callerMarkers, callerID)
	}
	callerMarkersMu.Unlock()
	if ok {
		if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
			// Non-fatal warning
			fmt.Printf("Warning: failed to remove caller marker %s: %v\n", markerPath, err)
		}
	}

	callersDir := filepath.Join(binDir, callersDirName)
	// Remove empty callers directory if no callers remain
	if dirExists(callersDir) {
		entries, err := os.ReadDir(callersDir)
		if err == nil && len(entries) == 0 {
			if err := os.Remove(callersDir); err != nil && !os.IsNotExist(err) {
				// Silent cleanup
			}
		}
	}
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// processExists checks if a process with given PID exists.
func processExists(pid int) bool {
	// Try to send signal 0 to check process existence
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 doesn't actually send a signal, just checks permissions
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// cleanupStaleMarkers removes caller markers older than 1 hour or from dead processes.
func cleanupStaleMarkers(callersDir string) {
	if !dirExists(callersDir) {
		return
	}

	entries, err := os.ReadDir(callersDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-1 * time.Hour)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		markerPath := filepath.Join(callersDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if marker is older than 1 hour
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
				// Silent cleanup failure
			}
			continue
		}

		// Check if marker belongs to a dead process
		if isMarkerFromDeadProcess(entry.Name()) {
			if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
				// Silent cleanup failure
			}
		}
	}
}

// isMarkerFromDeadProcess checks if a marker filename corresponds to a dead process.
func isMarkerFromDeadProcess(filename string) bool {
	// Format: test_<pid>_<timestamp>
	if !strings.HasPrefix(filename, "test_") {
		return false
	}
	parts := strings.Split(filename, "_")
	if len(parts) < 3 {
		return false
	}
	pidStr := parts[1]
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}
	return !processExists(pid)
}
