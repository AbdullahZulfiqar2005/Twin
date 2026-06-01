// Package ui implements FR-5: Interactive Resolution via a state-of-the-art Charm CLI TUI.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/AbdullahZulfiqar2005/twin/config"
	twinctx "github.com/AbdullahZulfiqar2005/twin/internal/context"
	"github.com/AbdullahZulfiqar2005/twin/internal/executor"
	"github.com/AbdullahZulfiqar2005/twin/internal/interceptor"
	"github.com/AbdullahZulfiqar2005/twin/internal/llm"
	"github.com/AbdullahZulfiqar2005/twin/internal/patcher"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionState int

const (
	stateAnalyzing sessionState = iota
	stateConfirming
	stateApplying
	stateSuccess
	stateFailure
)

type fileBackup struct {
	content []byte
	mode    os.FileMode
}

// Model represents the Bubble Tea application state machine.
type Model struct {
	mu sync.Mutex

	// Configuration & CLI context
	cfg           *config.Config
	cmd           string
	args          []string
	failedCmd     string
	globalBackups map[string]fileBackup

	// Self-healing progress
	attempt     int
	maxAttempts int

	// Runtime state
	state         sessionState
	currentResult *executor.Result
	fix           *llm.Fix
	err           error
	width         int

	// Exposed properties for main.go to read upon termination
	Success  bool
	ExitCode int

	// Components
	spinner spinner.Model
}

// Curated UI color palette & Lip Gloss styles
var (
	purpleColor = lipgloss.Color("#8B5CF6") // Violet-500
	greenColor  = lipgloss.Color("#10B981") // Emerald-500
	redColor    = lipgloss.Color("#EF4444") // Red-500
	grayColor   = lipgloss.Color("#9CA3AF") // Gray-400
	slateColor  = lipgloss.Color("#4B5563") // Slate-600

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(purpleColor).
			Padding(0, 1).
			MarginBottom(1)

	explanationStyle = lipgloss.NewStyle().
				Italic(true).
				Foreground(lipgloss.Color("#E5E7EB")).
				MarginBottom(1)

	originalTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(redColor)
	proposedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(greenColor)

	boxStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(slateColor).
			MarginBottom(1)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(redColor).
			MarginBottom(1)

	successBannerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(greenColor).
				Padding(0, 2).
				MarginBottom(1)

	failureBannerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(redColor).
				Padding(0, 2).
				MarginBottom(1)

	footerStyle = lipgloss.NewStyle().
			Foreground(slateColor).
			MarginTop(1)
)

// NewModel instantiates the self-healing TUI.
func NewModel(cfg *config.Config, cmd string, args []string, initialResult *executor.Result) Model {
	failedCmd := cmd
	for _, arg := range args {
		failedCmd += " " + arg
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(purpleColor)

	return Model{
		cfg:           cfg,
		cmd:           cmd,
		args:          args,
		failedCmd:     failedCmd,
		globalBackups: make(map[string]fileBackup),
		attempt:       1,
		maxAttempts:   3,
		state:         stateAnalyzing,
		currentResult: initialResult,
		spinner:       s,
	}
}

// Init triggers initial asynchronously loading of the solution.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.askLLM())
}

// Update handles reactive state-transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.rollbackAll()
			m.Success = false
			m.ExitCode = 130 // SIGINT exit code
			m.state = stateFailure
			return m, tea.Quit
		}

		if m.state == stateConfirming {
			switch msg.String() {
			case "y", "enter":
				m.state = stateApplying
				return m, tea.Batch(m.spinner.Tick, m.applyAndReRun())
			case "n", "q":
				m.rollbackAll()
				m.Success = false
				m.ExitCode = m.currentResult.ExitCode
				m.state = stateFailure
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case llmSuccessMsg:
		m.fix = msg.fix
		m.state = stateConfirming
		return m, nil

	case llmErrorMsg:
		m.err = msg.err
		m.state = stateFailure
		m.rollbackAll()
		m.ExitCode = m.currentResult.ExitCode
		return m, tea.Quit

	case runSuccessMsg:
		if msg.result.ExitCode == 0 {
			m.Success = true
			m.state = stateSuccess
			return m, tea.Quit
		}

		// Failed again. Attempt next turn.
		m.attempt++
		if m.attempt > m.maxAttempts {
			m.rollbackAll()
			m.ExitCode = msg.result.ExitCode
			m.state = stateFailure
			return m, tea.Quit
		}

		m.currentResult = msg.result
		m.state = stateAnalyzing
		return m, tea.Batch(m.spinner.Tick, m.askLLM())

	case runErrorMsg:
		m.err = msg.err
		m.state = stateFailure
		m.rollbackAll()
		m.ExitCode = 1
		return m, tea.Quit
	}

	return m, nil
}

// View outputs the styled string depending on active TUI state.
func (m Model) View() string {
	var b strings.Builder

	// Title header
	b.WriteString(titleStyle.Render("twin 🚀 agentic shell recovery") + "\n")

	// Adjust width logic
	w := m.width
	if w == 0 {
		w = 80
	}

	switch m.state {
	case stateAnalyzing:
		kind := "unknown"
		if event := interceptor.Intercept(m.currentResult); event != nil {
			kind = string(event.Kind)
		}
		b.WriteString(fmt.Sprintf(" %s %s failure (Exit %d) — Attempt %d of %d\n\n",
			m.spinner.View(),
			strings.ToUpper(kind),
			m.currentResult.ExitCode,
			m.attempt,
			m.maxAttempts))
		b.WriteString(lipgloss.NewStyle().Foreground(purpleColor).Render(" Calling LLM API for resolution plan...") + "\n")

	case stateConfirming:
		if m.fix == nil {
			break
		}
		b.WriteString(fmt.Sprintf(" twin found a proposed fix (Attempt %d of %d):\n", m.attempt, m.maxAttempts))
		b.WriteString(explanationStyle.Render("💡 "+m.fix.Explanation) + "\n\n")

		if m.fix.IsCommand {
			cmdBox := boxStyle.BorderForeground(purpleColor).Render(
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("$ ") +
					lipgloss.NewStyle().Foreground(greenColor).Render(m.fix.Command),
			)
			b.WriteString(cmdBox + "\n")
		} else {
			for i, p := range m.fix.Patches {
				b.WriteString(fmt.Sprintf(" Patch %d of %d: %s\n", i+1, len(m.fix.Patches), lipgloss.NewStyle().Foreground(purpleColor).Render(p.FilePath)))
				b.WriteString(renderDiff(p, w) + "\n")
			}
		}

		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render(" [y/enter] Apply Fix   [n/q] Decline & Exit") + "\n")

	case stateApplying:
		b.WriteString(fmt.Sprintf(" %s Applying patches and verifying command...\n\n", m.spinner.View()))
		b.WriteString(lipgloss.NewStyle().Foreground(purpleColor).Render(fmt.Sprintf("$ %s", m.failedCmd)) + "\n")

	case stateSuccess:
		b.WriteString(successBannerStyle.Render("✨ SELF-HEALING SUCCESSFUL! Command recovered successfully.") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(grayColor).Render("All changes have been successfully committed to disk.") + "\n")

	case stateFailure:
		if m.err != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("❌ Error: %v", m.err)) + "\n\n")
		} else {
			b.WriteString(failureBannerStyle.Render(fmt.Sprintf("❌ SELF-HEALING FAILED after %d attempts.", m.maxAttempts)) + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(grayColor).Render("All changes have been rolled back to original pristine state.") + "\n")
	}

	b.WriteString(footerStyle.Render("Press Ctrl+C to force exit at any time") + "\n")

	return b.String()
}

// ── Background Commands ───────────────────────────────────────────────────────

type llmSuccessMsg struct{ fix *llm.Fix }
type llmErrorMsg struct{ err error }
type runSuccessMsg struct{ result *executor.Result }
type runErrorMsg struct{ err error }

func (m *Model) askLLM() tea.Cmd {
	return func() tea.Msg {
		event := interceptor.Intercept(m.currentResult)
		if event == nil {
			return llmErrorMsg{err: fmt.Errorf("failure could not be intercepted")}
		}

		payload, err := twinctx.Extract(event)
		if err != nil {
			return llmErrorMsg{err: fmt.Errorf("context extraction failed: %w", err)}
		}

		fix, err := llm.Ask(m.cfg, m.failedCmd, payload.Stderr, payload.Snippets)
		if err != nil {
			return llmErrorMsg{err: err}
		}
		return llmSuccessMsg{fix: fix}
	}
}

func (m *Model) applyAndReRun() tea.Cmd {
	return func() tea.Msg {
		if m.fix.IsCommand {
			shellCmd := exec.Command("/bin/sh", "-c", m.fix.Command)
			if err := shellCmd.Run(); err != nil {
				return runErrorMsg{err: fmt.Errorf("command execution failed: %w", err)}
			}
		} else {
			// safety backups
			for _, p := range m.fix.Patches {
				if err := m.backupFile(p.FilePath); err != nil {
					return runErrorMsg{err: fmt.Errorf("safety backup failed for %s: %w", p.FilePath, err)}
				}
			}
			// apply patches
			for _, p := range m.fix.Patches {
				if err := patcher.Apply(p.FilePath, p.SearchBlock, p.ReplaceBlock); err != nil {
					return runErrorMsg{err: fmt.Errorf("failed to apply patch to %s: %w", p.FilePath, err)}
				}
			}
		}

		// Re-run original failed command
		newResult, err := executor.Run(m.cmd, m.args)
		if err != nil {
			return runErrorMsg{err: fmt.Errorf("failed to re-execute command: %w", err)}
		}

		return runSuccessMsg{result: newResult}
	}
}

// ── Transactional Backup Registry (Thread-Safe) ──────────────────────────────

func (m *Model) backupFile(filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, already := m.globalBackups[filePath]; already {
		return nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	m.globalBackups[filePath] = fileBackup{
		content: content,
		mode:    info.Mode(),
	}
	return nil
}

func (m *Model) rollbackAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.globalBackups) == 0 {
		return
	}
	for path, original := range m.globalBackups {
		_ = os.WriteFile(path, original.content, original.mode)
	}
}

// ── Diff Rendering helper ────────────────────────────────────────────────────

func renderDiff(p llm.Patch, terminalWidth int) string {
	origTitle := originalTitleStyle.Render("❌ BEFORE (Original)")
	propTitle := proposedTitleStyle.Render("✅ AFTER (Proposed)")

	origContent := strings.TrimSpace(p.SearchBlock)
	propContent := strings.TrimSpace(p.ReplaceBlock)

	if terminalWidth > 85 {
		boxWidth := (terminalWidth - 8) / 2
		if boxWidth > 60 {
			boxWidth = 60
		}

		origBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(redColor).
			Padding(0, 1).
			Width(boxWidth).
			Render(origTitle + "\n" + origContent)

		propBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(greenColor).
			Padding(0, 1).
			Width(boxWidth).
			Render(propTitle + "\n" + propContent)

		return lipgloss.JoinHorizontal(lipgloss.Top, origBox, "  ", propBox)
	}

	origBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(redColor).
		Padding(0, 1).
		Width(terminalWidth - 4).
		Render(origTitle + "\n" + origContent)

	propBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(greenColor).
		Padding(0, 1).
		Width(terminalWidth - 4).
		Render(propTitle + "\n" + propContent)

	return origBox + "\n" + propBox
}
