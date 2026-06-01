package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/AbdullahZulfiqar2005/twin/config"
)

// RunSetup runs an interactive command-line wizard to configure twin.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n┌────────────────────────────────────────────────────────┐")
	fmt.Println("│  twin 🚀 global configuration wizard                  │")
	fmt.Println("└────────────────────────────────────────────────────────┘")
	fmt.Println("This wizard will help you configure twin for persistent use.")
	fmt.Println("Your config will be saved globally under XDG specifications.")

	// 1. Choose Provider
	fmt.Println("\nSelect AI Provider:")
	fmt.Println("  [1] Gemini (Google Cloud)")
	fmt.Println("  [2] OpenAI (ChatGPT)")
	fmt.Println("  [3] Ollama (Local Offline Models)")
	fmt.Print("\nEnter choice [1-3] (default: 1): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var provider config.Provider
	switch choice {
	case "2":
		provider = config.ProviderOpenAI
	case "3":
		provider = config.ProviderOllama
	default:
		provider = config.ProviderGemini
	}

	cfg := &config.Config{
		Provider: provider,
	}

	// 2. Provider-specific inputs
	switch provider {
	case config.ProviderGemini, config.ProviderOpenAI:
		fmt.Printf("\nEnter your %s API Key: ", provider)
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("API key cannot be empty")
		}
		cfg.APIKey = key

	case config.ProviderOllama:
		fmt.Print("\nEnter Ollama Host (default: http://localhost:11434): ")
		host, _ := reader.ReadString('\n')
		host = strings.TrimSpace(host)
		if host == "" {
			host = "http://localhost:11434"
		}
		cfg.OllamaHost = host

		fmt.Print("Enter Ollama Model (default: qwen2.5-coder): ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)
		if model == "" {
			model = "qwen2.5-coder"
		}
		cfg.OllamaModel = model
	}

	// 3. Save Config
	err := config.Save(cfg)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	path, _ := config.GetConfigPath()
	fmt.Println("\n\033[1;32m✨ Configuration completed successfully!\033[0m")
	fmt.Printf("Settings saved to: %s\n", path)
	fmt.Println("You can now run 'twin <command>' from anywhere on your system.")
	return nil
}
