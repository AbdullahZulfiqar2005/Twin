package patcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_PreservesPermissionsAndReplaces(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")

	originalContent := "line 1\nline 2: target_to_replace\nline 3"
	expectedContent := "line 1\nline 2: substituted_value\nline 3"

	// Use custom permissions (e.g., 0600 - read/write owner only)
	originalMode := os.FileMode(0600)
	if err := os.WriteFile(filePath, []byte(originalContent), originalMode); err != nil {
		t.Fatalf("setup: failed to write test file: %v", err)
	}

	// Apply patch
	err := Apply(filePath, "target_to_replace", "substituted_value")
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify content was replaced
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read patched file: %v", err)
	}
	gotContent := string(contentBytes)
	if gotContent != expectedContent {
		t.Errorf("content = %q, want %q", gotContent, expectedContent)
	}

	// Verify permissions were preserved
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat patched file: %v", err)
	}
	if info.Mode().Perm() != originalMode {
		t.Errorf("permissions = %v, want %v", info.Mode().Perm(), originalMode)
	}
}

func TestApply_SearchBlockNotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")

	content := "hello world"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	err := Apply(filePath, "non_existent_string", "replacement")
	if err == nil {
		t.Fatal("expected error when search block is not found, got nil")
	}

	if !strings.Contains(err.Error(), "search_block not found") {
		t.Errorf("expected 'search_block not found' error message, got: %v", err)
	}
}
