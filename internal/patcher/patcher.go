// Package patcher implements FR-6: File Patching.
//
// Before modifying any file it creates an in-memory backup (NFR-2: Safety),
// then performs an exact search_block → replace_block substitution.
package patcher

import (
	"fmt"
	"os"
	"strings"
)

// Apply reads `filePath`, replaces the first occurrence of `search` with
// `replace`, and writes the result back. An in-memory backup is kept so
// the original can be restored if the write fails (NFR-2).
func Apply(filePath, search, replace string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("patcher: cannot stat %s: %w", filePath, err)
	}
	mode := info.Mode()

	original, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("patcher: cannot read %s: %w", filePath, err)
	}

	content := string(original)
	if !strings.Contains(content, search) {
		return fmt.Errorf("patcher: search_block not found in %s", filePath)
	}

	patched := strings.Replace(content, search, replace, 1)

	// Write patched content; restore backup on failure (NFR-2).
	if err := os.WriteFile(filePath, []byte(patched), mode); err != nil {
		// Attempt restore from in-memory backup.
		_ = os.WriteFile(filePath, original, mode)
		return fmt.Errorf("patcher: write failed, original restored: %w", err)
	}

	return nil
}
