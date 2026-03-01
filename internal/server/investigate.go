package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/skyhook-io/radar/internal/ai/investigate"
	"github.com/skyhook-io/radar/internal/ai/llm"
	"github.com/skyhook-io/radar/internal/settings"
)

// AI provider management — package-level with mutex protection.
var (
	aiMu       sync.RWMutex
	aiProvider llm.Provider
	aiConfig   llm.Config
)

// SetAIConfig updates the AI provider configuration and recreates the provider.
func SetAIConfig(cfg llm.Config) error {
	aiMu.Lock()
	defer aiMu.Unlock()

	if !cfg.IsConfigured() {
		aiProvider = nil
		aiConfig = cfg
		return nil
	}

	provider, err := llm.NewProvider(cfg)
	if err != nil {
		return err
	}

	aiProvider = provider
	aiConfig = cfg
	return nil
}

// GetAIProvider returns the current AI provider (nil if not configured).
func GetAIProvider() llm.Provider {
	aiMu.RLock()
	defer aiMu.RUnlock()
	return aiProvider
}

// GetAIConfig returns the current AI config.
func GetAIConfig() llm.Config {
	aiMu.RLock()
	defer aiMu.RUnlock()
	return aiConfig
}

// AIConfigResponse is returned by GET /api/ai/config.
type AIConfigResponse struct {
	Provider   string `json:"provider"`
	BaseURL    string `json:"baseUrl"`
	Model      string `json:"model"`
	Configured bool   `json:"configured"`
}

// handleAIConfig returns the current AI configuration status.
// GET /api/ai/config
func (s *Server) handleAIConfig(w http.ResponseWriter, r *http.Request) {
	cfg := GetAIConfig()
	s.writeJSON(w, AIConfigResponse{
		Provider:   cfg.Provider,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		Configured: cfg.IsConfigured(),
	})
}

// AIConfigUpdateRequest is the body for PUT /api/ai/config.
type AIConfigUpdateRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Model    string `json:"model,omitempty"`
}

// handleUpdateAIConfig updates the AI configuration.
// PUT /api/ai/config
func (s *Server) handleUpdateAIConfig(w http.ResponseWriter, r *http.Request) {
	var req AIConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg := llm.Config{
		Provider: req.Provider,
		APIKey:   req.APIKey,
		BaseURL:  req.BaseURL,
		Model:    req.Model,
	}

	// If no API key provided, keep the existing one (UI may omit it for security)
	if req.APIKey == "" {
		existing := GetAIConfig()
		if existing.Provider == req.Provider {
			cfg.APIKey = existing.APIKey
		}
	}

	if err := SetAIConfig(cfg); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Persist to settings file
	if _, err := settings.Update(func(s *settings.Settings) {
		s.AIProvider = cfg.Provider
		s.AIBaseURL = cfg.BaseURL
		s.AIAPIKey = cfg.APIKey
		s.AIModel = cfg.Model
	}); err != nil {
		log.Printf("[ai] Failed to save settings: %v", err)
	}

	s.writeJSON(w, AIConfigResponse{
		Provider:   cfg.Provider,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		Configured: cfg.IsConfigured(),
	})
}

// aiStreamWriter emits AI SDK UI Message Stream Protocol events as SSE.
type aiStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (sw *aiStreamWriter) emit(event map[string]any) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ai] Failed to marshal SSE event: %v", err)
		return
	}
	fmt.Fprintf(sw.w, "data: %s\n\n", data)
	sw.flusher.Flush()
}

func (sw *aiStreamWriter) done() {
	fmt.Fprintf(sw.w, "data: [DONE]\n\n")
	sw.flusher.Flush()
}

// investigateRequestBody supports both direct params and AI SDK useChat transport.
type investigateRequestBody struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Question  string `json:"question,omitempty"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages,omitempty"`
}

// handleInvestigate starts an AI investigation and streams progress via SSE.
// Emits the AI SDK UI Message Stream Protocol for compatibility with useChat.
// POST /api/ai/investigate
func (s *Server) handleInvestigate(w http.ResponseWriter, r *http.Request) {
	if !s.requireConnected(w) {
		return
	}

	provider := GetAIProvider()
	if provider == nil {
		s.writeError(w, http.StatusBadRequest, "AI provider not configured. Set up an AI provider in Settings > AI.")
		return
	}

	var raw investigateRequestBody
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := investigate.InvestigateParams{
		Kind:      raw.Kind,
		Namespace: raw.Namespace,
		Name:      raw.Name,
		Question:  raw.Question,
	}

	// Extract question from latest user message if using useChat format
	if params.Question == "" && len(raw.Messages) > 1 {
		for i := len(raw.Messages) - 1; i >= 0; i-- {
			if raw.Messages[i].Role == "user" {
				params.Question = raw.Messages[i].Content
				break
			}
		}
	}

	if params.Kind == "" || params.Name == "" {
		s.writeError(w, http.StatusBadRequest, "kind and name are required")
		return
	}

	// Set up SSE streaming with AI SDK UI Message Stream Protocol
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx := r.Context()
	sw := &aiStreamWriter{w: w, flusher: flusher}

	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	sw.emit(map[string]any{"type": "start", "messageId": messageID})

	engine := investigate.NewEngine(provider)

	textPartOpen := false
	textCounter := 0
	currentTextID := ""
	stepOpen := false

	engine.Investigate(ctx, params, func(event investigate.Event) {
		switch event.Type {
		case "status":
			sw.emit(map[string]any{
				"type": "data-status",
				"data": map[string]any{"content": event.Content},
			})

		case "step_start":
			// Close any open text part from the previous step
			if textPartOpen {
				sw.emit(map[string]any{"type": "text-end", "id": currentTextID})
				textPartOpen = false
			}
			if stepOpen {
				sw.emit(map[string]any{"type": "finish-step"})
			}
			sw.emit(map[string]any{"type": "start-step"})
			stepOpen = true

		case "tool_call":
			// Close any open text part before tool events
			if textPartOpen {
				sw.emit(map[string]any{"type": "text-end", "id": currentTextID})
				textPartOpen = false
			}
			sw.emit(map[string]any{
				"type":       "tool-input-start",
				"toolCallId": event.ToolCallID,
				"toolName":   event.Tool,
				"dynamic":    true,
			})
			var input any
			if json.Unmarshal([]byte(event.Args), &input) != nil {
				input = event.Args
			}
			sw.emit(map[string]any{
				"type":       "tool-input-available",
				"toolCallId": event.ToolCallID,
				"toolName":   event.Tool,
				"input":      input,
				"dynamic":    true,
			})

		case "tool_result":
			var output any
			if json.Unmarshal([]byte(event.Content), &output) != nil {
				output = event.Content
			}
			sw.emit(map[string]any{
				"type":       "tool-output-available",
				"toolCallId": event.ToolCallID,
				"output":     output,
				"dynamic":    true,
			})

		case "analysis":
			// Open a new text part if one isn't already open
			if !textPartOpen {
				textCounter++
				currentTextID = fmt.Sprintf("text-%d", textCounter)
				sw.emit(map[string]any{"type": "text-start", "id": currentTextID})
				textPartOpen = true
			}
			sw.emit(map[string]any{
				"type":  "text-delta",
				"id":    currentTextID,
				"delta": event.Content,
			})

		case "error":
			sw.emit(map[string]any{
				"type":      "error",
				"errorText": event.Content,
			})

		case "done":
			if textPartOpen {
				sw.emit(map[string]any{"type": "text-end", "id": currentTextID})
			}
			if stepOpen {
				sw.emit(map[string]any{"type": "finish-step"})
			}
			sw.emit(map[string]any{"type": "finish"})
			sw.done()
		}
	})
}

// LoadAIConfigFromSettings loads AI config from the persisted settings file.
// Called at startup to restore previously saved configuration.
func LoadAIConfigFromSettings(cliCfg llm.Config) {
	s := settings.Load()

	// CLI flags take precedence over saved settings
	cfg := llm.Config{
		Provider: cliCfg.Provider,
		APIKey:   cliCfg.APIKey,
		BaseURL:  cliCfg.BaseURL,
		Model:    cliCfg.Model,
	}

	// Fill in from settings where CLI didn't specify
	if cfg.Provider == "" {
		cfg.Provider = s.AIProvider
	}
	if cfg.APIKey == "" {
		cfg.APIKey = s.AIAPIKey
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = s.AIBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = s.AIModel
	}

	if cfg.IsConfigured() {
		if err := SetAIConfig(cfg); err != nil {
			log.Printf("[ai] Failed to initialize AI provider from settings: %v", err)
		} else {
			log.Printf("[ai] AI provider initialized: %s (model: %s)", cfg.Provider, cfg.Model)
		}
	}
}
