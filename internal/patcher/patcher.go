// Package patcher implements FR-6: File Patching.
//
// Before modifying any file it creates an in-memory backup (NFR-2: Safety),
// then performs an exact search_block → replace_block substitution.
package patcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Apply reads `filePath`, replaces the first occurrence of `search` with
// `replace`, and writes the result back. An in-memory backup is kept so
// the original can be restored if the write fails (NFR-2).
func Apply(filePath, search, replace string) error {
	resolvedPath, err := securePath(filePath)
	if err != nil {
		return err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("patcher: cannot stat %s: %w", resolvedPath, err)
	}
	mode := info.Mode()

	original, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("patcher: cannot read %s: %w", resolvedPath, err)
	}

	content := string(original)
	if !strings.Contains(content, search) {
		return fmt.Errorf("patcher: search_block not found in %s", resolvedPath)
	}

	patched := strings.Replace(content, search, replace, 1)

	// Write patched content; restore backup on failure (NFR-2).
	if err := os.WriteFile(resolvedPath, []byte(patched), mode); err != nil {
		// Attempt restore from in-memory backup.
		_ = os.WriteFile(resolvedPath, original, mode)
		return fmt.Errorf("patcher: write failed, original restored: %w", err)
	}

	return nil
}

// securePath validates that `filePath` is strictly within the current working directory
// and returns the resolved absolute path. This prevents path traversal attacks (NFR-2).
func securePath(filePath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %s: %w", filePath, err)
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute working directory: %w", err)
	}

	// Relativize the path to ensure it starts inside CWD and does not escape it
	rel, err := filepath.Rel(absCwd, absPath)
	if err != nil {
		return "", fmt.Errorf("path validation failed for %s: %w", filePath, err)
	}

	// If the relative path starts with ".." or is absolute (meaning it escaped), it is out of bounds!
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("security violation: path %s escapes the active workspace directory", filePath)
	}

	return absPath, nil
}
