package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnvFallback(t *testing.T) {
	// Clean env and ensure config file doesn't exist by overriding UserConfigDir
	tempDir := t.TempDir()
	t.Setenv("TWIN_PROVIDER", "gemini")
	t.Setenv("TWIN_API_KEY", "test-api-key-123")

	// Mock XDG config path to a temp dir so it doesn't read the real user config
	originalUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalUserConfigDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed on env fallback: %v", err)
	}

	if cfg.Provider != ProviderGemini {
		t.Errorf("Provider = %q, want %q", cfg.Provider, ProviderGemini)
	}
	if cfg.APIKey != "test-api-key-123" {
		t.Errorf("APIKey = %q, want 'test-api-key-123'", cfg.APIKey)
	}
}

func TestSaveAndLoad_JSONFile(t *testing.T) {
	tempDir := t.TempDir()

	// Direct testing of saving/loading in a target file path
	cfg := &Config{
		Provider:    ProviderOllama,
		OllamaModel: "codellama:7b",
		OllamaHost:  "http://127.0.0.1:11434",
	}

	// We override GetConfigPath target by using XDG_CONFIG_HOME
	originalUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalUserConfigDir)

	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file was created in XDG_CONFIG_HOME/twin/config.json
	path := filepath.Join(tempDir, "twin", "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected config file to exist at %s, but it was not found", path)
	}

	// Load and verify
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed after Save: %v", err)
	}

	if loaded.Provider != ProviderOllama {
		t.Errorf("loaded.Provider = %q, want %q", loaded.Provider, ProviderOllama)
	}
	if loaded.OllamaModel != "codellama:7b" {
		t.Errorf("loaded.OllamaModel = %q, want 'codellama:7b'", loaded.OllamaModel)
	}
	if loaded.OllamaHost != "http://127.0.0.1:11434" {
		t.Errorf("loaded.OllamaHost = %q, want 'http://127.0.0.1:11434'", loaded.OllamaHost)
	}
}
