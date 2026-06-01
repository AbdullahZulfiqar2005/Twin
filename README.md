# twin

A lightweight agentic CLI wrapper written in Go. It runs your commands normally,
and when something fails it uses an LLM to explain the error and propose a fix.

```
twin gcc main.c          # works exactly like `gcc main.c`
twin go build ./...      # on failure → LLM explains + patches the file
```

## Status

| FR | Description | Status |
|----|-------------|--------|
| FR-1 | Process execution + stream piping | ✅ Done |
| FR-2 | stderr interception + exit code monitor | 🔲 Stub |
| FR-3 | Context extraction (file path / line parsing) | 🔲 Stub |
| FR-4 | LLM communication (OpenAI / Gemini JSON mode) | 🔲 Stub |
| FR-5 | Interactive [Y/n] prompt | 🔲 Stub |
| FR-6 | File patching with in-memory backup | 🔲 Stub |

## Build

```bash
go build -o twin ./cmd/twin
```

Single binary, Linux only (NFR-3).

## Config

| Env var | Default | Values |
|---------|---------|--------|
| `TWIN_PROVIDER` | `openai` | `openai` \| `gemini` |
| `TWIN_API_KEY` | — | your API key |

## Directory structure

```
twin/
├── cmd/twin/
│   └── main.go          # entry point
├── internal/
│   ├── executor/        # FR-1  — spawn child, pipe stdio, capture stderr
│   ├── interceptor/     # FR-2  — detect failures
│   ├── context/         # FR-3  — parse stderr, read source files
│   ├── llm/             # FR-4  — call LLM API
│   ├── ui/              # FR-5  — Y/n confirmation prompt
│   └── patcher/         # FR-6  — apply file patch
└── config/              # env-based config loader
```
