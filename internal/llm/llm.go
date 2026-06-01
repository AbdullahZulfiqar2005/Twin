// Package llm implements FR-4: LLM Communication.
//
// It sends a context payload to the configured LLM API (Gemini or OpenAI)
// and enforces a strict JSON schema response matching the Fix struct.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AbdullahZulfiqar2005/twin/config"
)

// Patch represents a single text substitution in a file.
type Patch struct {
	FilePath     string `json:"file_path"`
	SearchBlock  string `json:"search_block"`  // exact text to find in file
	ReplaceBlock string `json:"replace_block"` // replacement text
}

// Fix is the structured response the LLM must return.
type Fix struct {
	Explanation string  `json:"explanation"`
	IsCommand   bool    `json:"is_command"`
	Command     string  `json:"command"`
	Patches     []Patch `json:"patches"` // list of patches to apply
}

const promptTemplate = `You are a helpful software engineering assistant called "twin".
You are run as a CLI wrapper. The following command failed:

=== FAILED COMMAND ===
%s
======================

With the following error output (stderr):

=== STDERR ===
%s
==============

Here are the relevant source code snippets from the files where the error occurred (lines are annotated with "lineNumber | "):

=== SOURCE SNIPPETS ===
%s
=======================

Analyze the errors and propose a list of precise patches to fix them. You can propose multiple patches in the patches array to fix multiple errors at once.
If the fix requires patching source files:
- Set is_command to false.
- Populate the patches array. Each patch object must contain:
  * file_path: The relative or absolute path of the file to patch (e.g., "src/main.rs" or "main.c"). It MUST match one of the files listed in the snippets.
  * search_block: The exact block of code that needs to be replaced in the file. (Do NOT include line numbers or " | " annotation prefixes. The block MUST match the original source code character-for-character, including indentation and spacing).
  * replace_block: The replacement text to insert.
- Provide a brief, single-sentence explanation of what was wrong in explanation.
- Leave command empty.

If the fix requires running a shell command (e.g. installing a dependency, initializing a tool):
- Set is_command to true.
- Provide the shell command in command.
- Provide a brief explanation in explanation.
- Leave the patches array empty.`

// Ask sends the payload to the LLM and returns a structured Fix.
func Ask(cfg *config.Config, failedCmd, stderr string, fileSnips map[string]string) (*Fix, error) {
	// Assemble source snippets content for the prompt
	var snipBuilder strings.Builder
	for fileKey, content := range fileSnips {
		snipBuilder.WriteString(fmt.Sprintf("--- File: %s ---\n%s\n", fileKey, content))
	}

	prompt := fmt.Sprintf(promptTemplate, failedCmd, stderr, snipBuilder.String())

	// Use a 30-second context timeout for API requests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch cfg.Provider {
	case config.ProviderGemini:
		return callGemini(ctx, cfg.APIKey, prompt)
	case config.ProviderOpenAI:
		return callOpenAI(ctx, cfg.APIKey, prompt)
	case config.ProviderOllama:
		return callOllama(ctx, prompt, cfg.OllamaModel, cfg.OllamaHost)
	default:
		return nil, fmt.Errorf("llm: unsupported provider %q", cfg.Provider)
	}
}

// ── Gemini API Implementation ──────────────────────────────────────────────────

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiSchema struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description,omitempty"`
	Properties  map[string]geminiSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Items       *geminiSchema           `json:"items,omitempty"`
}

type geminiGenerationConfig struct {
	ResponseMimeType string        `json:"responseMimeType"`
	ResponseSchema   *geminiSchema `json:"responseSchema,omitempty"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func callGemini(ctx context.Context, apiKey, prompt string) (*Fix, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=" + apiKey

	// Define our strict JSON response schema
	schema := &geminiSchema{
		Type: "OBJECT",
		Properties: map[string]geminiSchema{
			"explanation": {
				Type:        "STRING",
				Description: "Short explanation of the error and how to fix it.",
			},
			"is_command": {
				Type:        "BOOLEAN",
				Description: "True if the fix requires running a shell command instead of patching files.",
			},
			"command": {
				Type:        "STRING",
				Description: "The exact shell command to run if is_command is true.",
			},
			"patches": {
				Type:        "ARRAY",
				Description: "List of file patches to apply if is_command is false.",
				Items: &geminiSchema{
					Type: "OBJECT",
					Properties: map[string]geminiSchema{
						"file_path": {
							Type:        "STRING",
							Description: "Path to the file to be patched (relative to current directory).",
						},
						"search_block": {
							Type:        "STRING",
							Description: "Exact block of code to search for in the source file. Must match character-for-character including indentation.",
						},
						"replace_block": {
							Type:        "STRING",
							Description: "Replacement block of code to insert instead of search_block.",
						},
					},
					Required: []string{"file_path", "search_block", "replace_block"},
				},
			},
		},
		Required: []string{"explanation", "is_command", "command", "patches"},
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   schema,
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to marshal gemini request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: gemini api request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to read gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: gemini api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("llm: gemini returned no content candidates")
	}

	rawJSON := geminiResp.Candidates[0].Content.Parts[0].Text

	var fix Fix
	if err := json.Unmarshal([]byte(rawJSON), &fix); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal structured fix JSON: %w\nRaw JSON: %s", err, rawJSON)
	}

	return &fix, nil
}

// ── OpenAI API Implementation ──────────────────────────────────────────────────

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormatSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]interface{} `json:"properties"`
	Required             []string               `json:"required"`
	AdditionalProperties bool                   `json:"additionalProperties"`
}

type openAIJSONSchema struct {
	Name   string                     `json:"name"`
	Strict bool                       `json:"strict"`
	Schema openAIResponseFormatSchema `json:"schema"`
}

type openAIResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIMessage      `json:"messages"`
	ResponseFormat openAIResponseFormat `json:"response_format"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func callOpenAI(ctx context.Context, apiKey, prompt string) (*Fix, error) {
	url := "https://api.openai.com/v1/chat/completions"

	reqBody := openAIRequest{
		Model: "gpt-4o-mini",
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
		ResponseFormat: openAIResponseFormat{
			Type: "json_schema",
			JSONSchema: &openAIJSONSchema{
				Name:   "fix_response",
				Strict: true,
				Schema: openAIResponseFormatSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"explanation": map[string]string{"type": "string"},
						"is_command":  map[string]string{"type": "boolean"},
						"command":     map[string]string{"type": "string"},
						"patches": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"file_path":     map[string]string{"type": "string"},
									"search_block":  map[string]string{"type": "string"},
									"replace_block": map[string]string{"type": "string"},
								},
								"required":             []string{"file_path", "search_block", "replace_block"},
								"additionalProperties": false,
							},
						},
					},
					Required:             []string{"explanation", "is_command", "command", "patches"},
					AdditionalProperties: false,
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: openai api request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to read openai response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: openai api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal openai response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("llm: openai returned no response choices")
	}

	rawJSON := openAIResp.Choices[0].Message.Content

	var fix Fix
	if err := json.Unmarshal([]byte(rawJSON), &fix); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal structured fix JSON: %w\nRaw JSON: %s", err, rawJSON)
	}

	return &fix, nil
}

// ── Ollama API Implementation ──────────────────────────────────────────────────

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Format string `json:"format"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func callOllama(ctx context.Context, prompt, model, host string) (*Fix, error) {
	if host == "" {
		host = "http://localhost:11434"
	}
	url := fmt.Sprintf("%s/api/generate", strings.TrimSuffix(host, "/"))

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Format: "json",
		Stream: false,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: ollama api request failed (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: failed to read ollama response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: ollama api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(bodyBytes, &ollamaResp); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal ollama response: %w", err)
	}

	var fix Fix
	if err := json.Unmarshal([]byte(ollamaResp.Response), &fix); err != nil {
		return nil, fmt.Errorf("llm: failed to unmarshal structured fix JSON from Ollama: %w\nRaw response: %s", err, ollamaResp.Response)
	}

	return &fix, nil
}
