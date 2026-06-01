# twin 🚀

A lightweight, state-of-the-art agentic CLI wrapper written in Go. It executes your commands normally with zero overhead, and if something fails, it engages a self-healing LLM agent loop that parses errors, extracts source code context, runs trial builds inside isolated Docker containers, audits command safety, and guides you through visual resolution in a premium terminal TUI.

```bash
twin gcc main.c          # Works exactly like `gcc main.c`
twin go build ./...      # On failure ➜ LLM explains, tests in container, proposes diff
```

---

## Key Features

*   **Zero-Overhead Child Execution**: Transparently pipes standard streams (`stdin`/`stdout`/`stderr`) directly, introducing less than **5ms** overhead for successful commands.
*   **Dynamic Error Interception & Classification**: Monitors command exits and applies heuristic regular expressions to identify compiler kinds (GCC/Clang, Go build, Python tracebacks, Rust/Cargo).
*   **Windowed Code Context Extractor**: Parses target source files and line numbers directly from the compiler's diagnostic output, pulling precise windowed excerpts with line annotations for the LLM.
*   **Containerized Sandbox Trial Runner**: Automatically verifies proposed patches or commands inside a tailored Docker container (e.g., `gcc:latest`, `golang:latest`, `rust:latest`) in the background, displaying real-time compiler verification and live sandbox build logs in the TUI.
*   **Command Safety Risk Audit**: Statically audits all proposed setup commands for dangerous system operations (such as administrative escalation, filesystem deletions, raw socket opening, and systemctl modifications), highlighting warnings in warning boxes and disabling fat-finger `enter` key activations for maximum security.
*   **Transactional Safe Rollbacks**: Maintains transactional in-memory backups of all modified files. If the self-healing loop is aborted or fails, all changes are rolled back to their pristine states.
*   **Global Restrictive Config Security**: Automatically saves API key credentials using secure global user config directories under `0700` and `0600` owner-restricted permissions.

---

## Status

| Reference | Description | Status |
| :--- | :--- | :--- |
| **FR-1** | Process execution + stream piping | ✅ Done |
| **FR-2** | stderr interception + exit code monitor | ✅ Done |
| **FR-3** | Context extraction (file path / line parsing) | ✅ Done |
| **FR-4** | LLM communication (OpenAI, Gemini Structured Outputs, Ollama) | ✅ Done |
| **FR-5** | Interactive, highly styled resolution TUI | ✅ Done |
| **FR-6** | File patching with transactional in-memory safety backups | ✅ Done |
| **SEC-1** | Owner-restricted configuration permissions (`0700`/`0600`) | ✅ Done |
| **SEC-2** | Command risk analyzer & accidental command activation protection | ✅ Done |
| **SND-1** | Isolated Docker Sandbox compiler trial validation | ✅ Done |

---

## Directory Structure

```
twin/
├── cmd/twin/
│   └── main.go          # Entry point
├── config/              # Env-based & global JSON config loader
├── internal/
│   ├── executor/        # FR-1 — Child process spawning & piping
│   ├── interceptor/     # FR-2 — Exit code monitor & parser
│   ├── context/         # FR-3 — Stderr parser & annotated code reader
│   ├── llm/             # FR-4 — Structured API integrations (Gemini, OpenAI, Ollama)
│   ├── ui/              # FR-5 — Bubble Tea interactive TUI & setup wizard
│   ├── patcher/         # FR-6 — Search-and-replace code patching
│   └── sandbox/         # SND-1 — Cloned Docker sandbox trial compiler
└── main.cpp             # Deliberate C++ test case with compilation errors
```

---

## Setup & Configuration

Configure `twin` globally using the persistent configuration wizard:

```bash
twin --config
```

The wizard will guide you through picking your provider (Gemini, OpenAI, or a local Offline Ollama instance running developer models like `qwen2.5-coder`) and saving the credentials.

### Environment Fallbacks

You can also control behavior via system environment variables:

| Variable | Description | Allowed Values |
| :--- | :--- | :--- |
| `TWIN_PROVIDER` | Targeted AI service API | `gemini` \| `openai` \| `ollama` |
| `TWIN_API_KEY` | Developer key credentials | `your-api-key` |
| `TWIN_OLLAMA_MODEL` | Ollama model override | e.g. `qwen2.5-coder` |
| `TWIN_OLLAMA_HOST` | Ollama service endpoint | e.g. `http://localhost:11434` |

---

## Development & Build

Ensure you have Go (1.24+) installed on your machine.

### Build Binary
```bash
go build -o twin ./cmd/twin
```

### Run Tests
```bash
go test ./...
```
