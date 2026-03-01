package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const (
	openaiDefaultModel = "gpt-5-mini"
	maxToolIterations  = 10
	llmCallTimeout     = 60 * time.Second
)

type openaiProvider struct {
	client openai.Client
	model  string
}

func newOpenAIProvider(cfg Config) (*openaiProvider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	client := openai.NewClient(opts...)

	model := cfg.Model
	if model == "" {
		model = openaiDefaultModel
	}

	return &openaiProvider{client: client, model: model}, nil
}

func (p *openaiProvider) Investigate(ctx context.Context, req InvestigateRequest, onEvent func(StreamEvent)) (*InvestigateResult, error) {
	tools := make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
	toolMap := make(map[string]Tool, len(req.Tools))
	for i, t := range req.Tools {
		var params openai.FunctionParameters
		if t.Parameters != nil {
			if err := json.Unmarshal(t.Parameters, &params); err != nil {
				return nil, fmt.Errorf("invalid tool parameters for %s: %w", t.Name, err)
			}
		}
		tools[i] = openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  params,
		})
		toolMap[t.Name] = t
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(req.SystemPrompt),
		openai.UserMessage(req.UserPrompt),
	}

	var allToolCalls []ToolCallRecord
	var finalText strings.Builder

	for iteration := range maxToolIterations {
		onEvent(StreamEvent{Type: "step_start"})

		callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
		stream := p.client.Chat.Completions.NewStreaming(callCtx, openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    openai.ChatModel(p.model),
			Tools:    tools,
		})

		acc := openai.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// Emit text deltas as they arrive for real-time streaming
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				delta := chunk.Choices[0].Delta.Content
				onEvent(StreamEvent{Type: "text", Content: delta})
				finalText.WriteString(delta)
			}
		}
		cancel()

		if err := stream.Err(); err != nil {
			onEvent(StreamEvent{Type: "error", Content: err.Error()})
			return nil, fmt.Errorf("openai streaming failed (iteration %d): %w", iteration, err)
		}

		if len(acc.Choices) == 0 {
			onEvent(StreamEvent{Type: "error", Content: "no choices returned"})
			return nil, fmt.Errorf("openai returned empty choices")
		}

		msg := acc.Choices[0].Message

		// No tool calls → done
		if len(msg.ToolCalls) == 0 {
			onEvent(StreamEvent{Type: "done"})
			break
		}

		// Append the assistant message (with tool calls) to conversation
		messages = append(messages, msg.ToParam())

		// Execute each tool call
		for _, tc := range msg.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := tc.Function.Arguments

			log.Printf("\033[1;36m[ai]\033[0m tool_call: %s %s", toolName, toolArgs)
			onEvent(StreamEvent{Type: "tool_call", Tool: toolName, Args: toolArgs, ToolCallID: tc.ID})

			tool, ok := toolMap[toolName]
			if !ok {
				errMsg := fmt.Sprintf("unknown tool: %s", toolName)
				log.Printf("\033[1;36m[ai]\033[0m tool_error: %s", errMsg)
				onEvent(StreamEvent{Type: "tool_result", Tool: toolName, Content: errMsg, ToolCallID: tc.ID})
				messages = append(messages, openai.ToolMessage(errMsg, tc.ID))
				allToolCalls = append(allToolCalls, ToolCallRecord{Tool: toolName, Args: toolArgs, Result: errMsg})
				continue
			}

			result, execErr := tool.Execute(ctx, json.RawMessage(toolArgs))
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}

			log.Printf("\033[1;36m[ai]\033[0m tool_result: %s (%d chars)", toolName, len(result))
			onEvent(StreamEvent{Type: "tool_result", Tool: toolName, Content: truncateResult(result, 200), ToolCallID: tc.ID})

			messages = append(messages, openai.ToolMessage(result, tc.ID))
			allToolCalls = append(allToolCalls, ToolCallRecord{Tool: toolName, Args: toolArgs, Result: result})
		}

		// Check if this was the last iteration
		if iteration == maxToolIterations-1 {
			onEvent(StreamEvent{Type: "error", Content: "max tool iterations reached"})
		}
	}

	return &InvestigateResult{
		Analysis:  finalText.String(),
		ToolCalls: allToolCalls,
	}, nil
}

// truncateResult returns the first n characters of s for display purposes.
func truncateResult(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
