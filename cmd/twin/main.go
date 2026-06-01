package main

import (
	"fmt"
	"os"
	"strings"

	"twin/config"
	"twin/internal/executor"
	"twin/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: twin <command> [args...]  (or 'twin --config' to configure)")
		os.Exit(1)
	}

	// ── Intercept CLI flags ──────────────────────────────────────────────────
	arg1 := strings.ToLower(strings.TrimSpace(os.Args[1]))
	if arg1 == "--config" || arg1 == "setup" || arg1 == "--setup" {
		if err := ui.RunSetup(); err != nil {
			fmt.Fprintf(os.Stderr, "twin: setup failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// ── FR-1: run the child process ───────────────────────────────────────────
	result, err := executor.Run(cmd, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "twin: internal error: %v\n", err)
		os.Exit(1)
	}

	if result.ExitCode == 0 {
		os.Exit(0)
	}

	// Load configuration inside the error pathway to keep zero-overhead for successful runs
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "twin: configuration error: %v\n", err)
		fmt.Print("Would you like to run the configuration setup now? [y/N]: ")
		var resp string
		_, _ = fmt.Scanln(&resp)
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp == "y" || resp == "yes" {
			if err := ui.RunSetup(); err != nil {
				fmt.Fprintf(os.Stderr, "twin: setup error: %v\n", err)
			}
			os.Exit(0)
		}
		os.Exit(result.ExitCode)
	}

	// Run the self-healing TUI
	p := tea.NewProgram(ui.NewModel(cfg, cmd, args, result))
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "twin: TUI execution error: %v\n", err)
		os.Exit(result.ExitCode)
	}

	m := finalModel.(ui.Model)
	if m.Success {
		os.Exit(0)
	} else {
		os.Exit(m.ExitCode)
	}
}
