package binhelper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// buildBinaryWithLock builds the binary with file-based locking.
func buildBinaryWithLock(binPath, lockPath string, timeoutSeconds int, t *testing.T) error {
	// Try to acquire lock with timeout
	lockAcquired, err := tryAcquireLock(lockPath, timeoutSeconds, t)
	if err != nil {
		return fmt.Errorf("lock acquisition failed: %v", err)
	}

	if !lockAcquired {
		// Someone else is building, wait for them to finish
		return waitForBinary(binPath, timeoutSeconds, t)
	}

	// We have the lock, build the binary
	defer releaseLock(lockPath)

	return buildBinary(binPath, t)
}

// tryAcquireLock attempts to acquire the build lock.
func tryAcquireLock(lockPath string, timeoutSeconds int, t *testing.T) (bool, error) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)

	for time.Now().Before(deadline) {
		// Try to create lock file exclusively
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			// Successfully created lock file
			lockInfo := LockInfo{
				BuildPID:  os.Getpid(),
				Timestamp: time.Now(),
				TestBuilt: true,
			}

			data, err := json.Marshal(lockInfo)
			if err != nil {
				file.Close()
				os.Remove(lockPath)
				return false, fmt.Errorf("failed to marshal lock info: %v", err)
			}

			if _, err := file.Write(data); err != nil {
				file.Close()
				os.Remove(lockPath)
				return false, fmt.Errorf("failed to write lock file: %v", err)
			}

			file.Close()
			ourBuildLock = true
			buildLockPath = lockPath

			t.Logf("Acquired build lock at %s", lockPath)
			return true, nil
		}

		// Check if lock file exists
		if os.IsExist(err) {
			// Lock exists, check if it's stale
			if isLockStale(lockPath) {
				// Stale lock, remove it and retry
				t.Logf("Removing stale lock file %s", lockPath)
				if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
					t.Logf("Warning: failed to remove stale lock: %v", err)
				}
				continue
			}

			// Valid lock exists, wait a bit
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Other error
		return false, fmt.Errorf("failed to create lock file: %v", err)
	}

	return false, fmt.Errorf("timeout after %d seconds waiting for build lock", timeoutSeconds)
}

// isLockStale checks if a lock file is stale (owning process no longer exists).
func isLockStale(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		// Can't read, assume stale
		return true
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		// Invalid format, assume stale
		return true
	}

	// Check if process exists
	if !processExists(lockInfo.BuildPID) {
		return true
	}

	// Check if lock is older than 5 minutes (extra safety)
	if time.Since(lockInfo.Timestamp) > 5*time.Minute {
		return true
	}

	return false
}

// waitForBinary waits for the binary to be built by another process.
func waitForBinary(binPath string, timeoutSeconds int, t *testing.T) error {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	t.Logf("Waiting for binary to be built by another process...")

	for time.Now().Before(deadline) {
		if _, err := os.Stat(binPath); err == nil {
			// Binary exists now
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout after %d seconds waiting for binary to be built", timeoutSeconds)
}

// releaseLock releases the build lock.
func releaseLock(lockPath string) {
	if ourBuildLock && buildLockPath == lockPath {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove lock file %s: %v\n", lockPath, err)
		}
		ourBuildLock = false
		buildLockPath = ""
	}
}

// getLockInfo reads the current lock information.
func getLockInfo(lockPath string) (*LockInfo, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		return nil, err
	}

	return &lockInfo, nil
}

// cleanupLock removes lock file if we own it.
func cleanupLock() {
	if ourBuildLock && buildLockPath != "" {
		if err := os.Remove(buildLockPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove lock file %s: %v\n", buildLockPath, err)
		}
	}
}

// cleanupAllLocks removes all lock files and markers (for cleanup on exit).
func cleanupAllLocks(binDir string) {
	lockPath := filepath.Join(binDir, lockFileName)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		// Silent cleanup
	}

	// Also cleanup any stale caller markers
	callersDir := filepath.Join(binDir, callersDirName)
	cleanupStaleMarkers(callersDir)
}
