// Package config loads and saves twin's runtime configuration from XDG config directories or environment variables.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderGemini Provider = "gemini"
	ProviderOllama Provider = "ollama"
)

// Config holds all runtime settings for twin.
type Config struct {
	Provider    Provider `json:"provider"`
	APIKey      string   `json:"api_key,omitempty"`
	OllamaModel string   `json:"ollama_model,omitempty"`
	OllamaHost  string   `json:"ollama_host,omitempty"`
}

// GetConfigPath returns the XDG compliant path to the twin configuration file.
func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "twin", "config.json"), nil
}

// Load reads configuration. It checks the global config file first, then falls back to env vars.
func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err == nil {
		if _, err := os.Stat(path); err == nil {
			content, err := os.ReadFile(path)
			if err == nil {
				var cfg Config
				if err := json.Unmarshal(content, &cfg); err == nil {
					// Validate loaded config
					if cfg.Provider == "" {
						cfg.Provider = ProviderOpenAI
					}
					return &cfg, nil
				}
			}
		}
	}

	// Fallback to environment variables
	provider := Provider(os.Getenv("TWIN_PROVIDER"))
	if provider == "" {
		provider = ProviderOpenAI
	}

	if provider != ProviderOpenAI && provider != ProviderGemini && provider != ProviderOllama {
		return nil, fmt.Errorf("config: unknown provider %q (use 'openai', 'gemini', or 'ollama')", provider)
	}

	key := os.Getenv("TWIN_API_KEY")
	ollamaModel := os.Getenv("TWIN_OLLAMA_MODEL")
	ollamaHost := os.Getenv("TWIN_OLLAMA_HOST")

	// Set defaults for Ollama if chosen
	if provider == ProviderOllama {
		if ollamaModel == "" {
			ollamaModel = "qwen2.5-coder" // sensible developer-focused default
		}
		if ollamaHost == "" {
			ollamaHost = "http://localhost:11434"
		}
	} else if key == "" {
		return nil, fmt.Errorf("config: TWIN_API_KEY is not set and no config file was found. Please run 'twin --config'")
	}

	return &Config{
		Provider:    provider,
		APIKey:      key,
		OllamaModel: ollamaModel,
		OllamaHost:  ollamaHost,
	}, nil
}

// Save persists the config struct to XDG config directory as formatted JSON.
func Save(cfg *Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Create directories if they do not exist (0700 restricts access to the owner only)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: failed to create directories: %w", err)
	}

	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: failed to marshal config: %w", err)
	}

	// Write file with 0600 permissions so it is only readable/writable by the owner
	if err := os.WriteFile(path, content, 0600); err != nil {
		return fmt.Errorf("config: failed to write file: %w", err)
	}

	return nil
}
