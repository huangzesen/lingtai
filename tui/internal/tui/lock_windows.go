//go:build windows

package tui

// tryLock on Windows always returns true (no flock support).
func tryLock(path string) bool {
	return true
}
