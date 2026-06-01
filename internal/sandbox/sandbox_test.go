package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"twin/internal/interceptor"
)

func TestGetDockerImage(t *testing.T) {
	cases := []struct {
		kind interceptor.FailureKind
		want string
	}{
		{interceptor.KindGCC, "gcc:latest"},
		{interceptor.KindGo, "golang:latest"},
		{interceptor.KindPython, "python:3-slim"},
		{interceptor.KindRust, "rust:latest"},
		{interceptor.KindUnknown, "ubuntu:latest"},
	}

	for _, tc := range cases {
		got := getDockerImage(tc.kind)
		if got != tc.want {
			t.Errorf("getDockerImage(%s) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestCopyDirAndFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Setup simple src tree
	file1 := filepath.Join(src, "main.go")
	content1 := "package main\n"
	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	subDir := filepath.Join(src, "pkg")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	file2 := filepath.Join(subDir, "utils.go")
	content2 := "package pkg\n"
	if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create a folder to skip
	skipDir := filepath.Join(src, ".git")
	if err := os.Mkdir(skipDir, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify copied files
	copied1 := filepath.Join(dst, "main.go")
	if _, err := os.Stat(copied1); err != nil {
		t.Errorf("main.go not copied: %v", err)
	}

	copied2 := filepath.Join(dst, "pkg", "utils.go")
	if _, err := os.Stat(copied2); err != nil {
		t.Errorf("pkg/utils.go not copied: %v", err)
	}

	// Verify skipped files
	skipped := filepath.Join(dst, ".git")
	if _, err := os.Stat(skipped); !os.IsNotExist(err) {
		t.Errorf("expected .git to be skipped, but it exists in dst")
	}
}
