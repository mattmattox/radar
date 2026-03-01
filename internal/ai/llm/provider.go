package llm

import (
	"context"
	"encoding/json"
)

// Provider is the interface for LLM backends.
type Provider interface {
	// Investigate runs a multi-turn investigation with tool calling.
	// The onEvent callback streams progress events to the caller.
	Investigate(ctx context.Context, req InvestigateRequest, onEvent func(StreamEvent)) (*InvestigateResult, error)
}

// InvestigateRequest contains the prompts and tools for an investigation.
type InvestigateRequest struct {
	SystemPrompt string
	UserPrompt   string
	Tools        []Tool
}

// Tool defines a callable tool that the LLM can invoke during investigation.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
	Execute     func(ctx context.Context, params json.RawMessage) (string, error)
}

// StreamEvent is sent via the onEvent callback to report investigation progress.
type StreamEvent struct {
	Type       string `json:"type"`                  // "step_start", "thinking", "tool_call", "tool_result", "text", "error", "done"
	Content    string `json:"content"`               // text content or error message
	Tool       string `json:"tool,omitempty"`        // tool name for tool_call/tool_result events
	Args       string `json:"args,omitempty"`        // tool arguments for tool_call events
	ToolCallID string `json:"toolCallId,omitempty"`  // unique ID for correlating tool calls with results
}

// InvestigateResult is the final output of an investigation.
type InvestigateResult struct {
	Analysis  string           `json:"analysis"`
	ToolCalls []ToolCallRecord `json:"toolCalls"`
}

// ToolCallRecord logs a single tool invocation during investigation.
type ToolCallRecord struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Result string `json:"result"`
}
