package binhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// Default environment variable names
	envSkipBuild   = "OASMOCK_TEST_SKIP_BUILD"
	envLockTimeout = "OASMOCK_TEST_LOCK_TIMEOUT"
	envKeepBinary  = "OASMOCK_TEST_KEEP_BINARY"

	// Default values
	defaultLockTimeout = 30 // seconds
	defaultSkipBuild   = false
	defaultKeepBinary  = false

	// File names
	lockFileName    = ".buildlock"
	callersDirName  = ".callers"
	testBuiltMarker = ".testbuilt"
)

var (
	// Global synchronization
	buildOnce sync.Once
	initOnce  sync.Once

	// State tracking
	buildLockPath   string
	callerMarkers   = make(map[string]string) // markerID -> markerPath
	callerMarkersMu sync.RWMutex
	ourBuildLock    = false

	// Cached project root
	cachedProjectRoot     string
	cachedProjectRootErr  error
	cachedProjectRootOnce sync.Once
)

// LockInfo represents the build lock file structure
type LockInfo struct {
	BuildPID  int       `json:"build_pid"`
	Timestamp time.Time `json:"timestamp"`
	TestBuilt bool      `json:"test_built"`
}

// GetBuilded returns the absolute path to the built binary.
// It builds the binary if necessary, with proper locking and caller tracking.
func GetBuilded(t *testing.T) string {
	return GetBuildedWithConfig(t, envSkipBuild)
}

// GetBuildedWithConfig returns the absolute path to the built binary
// with configurable environment variable names.
func GetBuildedWithConfig(t *testing.T, skipBuildEnvVar string) string {
	// Initialize once per process
	initOnce.Do(func() {
		initSignalHandler()
	})

	// Get configuration from environment
	skipBuild := getBoolEnv(skipBuildEnvVar, defaultSkipBuild)
	lockTimeout := getIntEnv(envLockTimeout, defaultLockTimeout)
	keepBinary := getBoolEnv(envKeepBinary, defaultKeepBinary)

	// Calculate absolute paths
	projectRoot, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to determine project root: %v", err)
	}

	binDir := filepath.Join(projectRoot, "bin")
	binPath := filepath.Join(binDir, "oasmock")
	lockPath := filepath.Join(binDir, lockFileName)
	testBuiltPath := filepath.Join(binDir, testBuiltMarker)

	// Register caller for cleanup coordination
	callerID := registerCaller(t, binDir)

	// Ensure cleanup of caller marker
	t.Cleanup(func() {
		cleanupCaller(callerID, binDir, binPath, testBuiltPath, keepBinary)
	})

	// Check if binary already exists
	if _, err := os.Stat(binPath); err == nil {
		// Binary exists, check if it was test-built
		if _, err := os.Stat(testBuiltPath); err == nil {
			// Test-built binary, keep it for now
			t.Logf("Using test-built binary at %s", binPath)
			return binPath
		}
		// User's binary (not test-built)
		t.Logf("Using existing binary at %s", binPath)
		return binPath
	}

	// Binary doesn't exist
	if skipBuild {
		t.Fatalf("binary required but not found at %s (OASMOCK_TEST_SKIP_BUILD=%s)",
			binPath, os.Getenv(skipBuildEnvVar))
	}

	// Build binary with synchronization
	var buildErr error
	buildOnce.Do(func() {
		buildErr = buildBinaryWithLock(binPath, lockPath, lockTimeout, t)
		if buildErr == nil {
			// Mark as test-built
			if err := os.WriteFile(testBuiltPath, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
				t.Logf("Warning: failed to create test-built marker: %v", err)
			}
		}
	})

	if buildErr != nil {
		t.Fatalf("failed to build binary: %v", buildErr)
	}

	t.Logf("Binary built at %s", binPath)
	return binPath
}

// getProjectRoot returns the absolute path to the project root.
func getProjectRoot() (string, error) {
	cachedProjectRootOnce.Do(func() {
		// Start from the directory of this file
		dir, err := os.Getwd()
		if err != nil {
			cachedProjectRootErr = err
			return
		}

		// Navigate up to project root (from test/_shared/binhelper)
		for i := 0; i < 10; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				cachedProjectRoot = dir
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break // reached root
			}
			dir = parent
		}

		cachedProjectRootErr = fmt.Errorf("project root (go.mod) not found")
	})
	return cachedProjectRoot, cachedProjectRootErr
}

// getBoolEnv reads a boolean environment variable.
func getBoolEnv(name string, defaultValue bool) bool {
	val := os.Getenv(name)
	if val == "" {
		return defaultValue
	}

	lower := strings.ToLower(val)
	return lower == "true" || lower == "1" || lower == "yes" || lower == "on"
}

// getIntEnv reads an integer environment variable.
func getIntEnv(name string, defaultValue int) int {
	val := os.Getenv(name)
	if val == "" {
		return defaultValue
	}

	if i, err := strconv.Atoi(val); err == nil && i > 0 {
		return i
	}

	return defaultValue
}
