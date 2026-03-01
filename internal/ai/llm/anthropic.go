package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const anthropicDefaultModel = "claude-sonnet-4-6"

type anthropicProvider struct {
	client anthropic.Client
	model  string
}

func newAnthropicProvider(cfg Config) (*anthropicProvider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	client := anthropic.NewClient(opts...)

	model := cfg.Model
	if model == "" {
		model = anthropicDefaultModel
	}

	return &anthropicProvider{client: client, model: model}, nil
}

func (p *anthropicProvider) Investigate(ctx context.Context, req InvestigateRequest, onEvent func(StreamEvent)) (*InvestigateResult, error) {
	tools := make([]anthropic.ToolUnionParam, len(req.Tools))
	toolMap := make(map[string]Tool, len(req.Tools))
	for i, t := range req.Tools {
		var schema map[string]interface{}
		if t.Parameters != nil {
			if err := json.Unmarshal(t.Parameters, &schema); err != nil {
				return nil, fmt.Errorf("invalid tool parameters for %s: %w", t.Name, err)
			}
		}
		inputSchema := anthropic.ToolInputSchemaParam{
			Properties: schema["properties"],
		}
		if reqList, ok := schema["required"]; ok {
			if sl, ok := reqList.([]interface{}); ok {
				strs := make([]string, len(sl))
				for i, v := range sl {
					strs[i] = fmt.Sprintf("%v", v)
				}
				inputSchema.Required = strs
			}
		}

		tools[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: inputSchema,
			},
		}
		toolMap[t.Name] = t
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
	}

	var allToolCalls []ToolCallRecord
	var finalText strings.Builder

	for iteration := range maxToolIterations {
		onEvent(StreamEvent{Type: "step_start"})

		callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
		stream := p.client.Messages.NewStreaming(callCtx, anthropic.MessageNewParams{
			Model:     anthropic.Model(p.model),
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: req.SystemPrompt},
			},
			Messages: messages,
			Tools:    tools,
		})

		// Accumulate the full message while streaming text deltas
		message := anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				log.Printf("[ai] accumulate error: %v", err)
			}

			// Stream text deltas in real time
			switch variant := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := variant.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					onEvent(StreamEvent{Type: "text", Content: delta.Text})
					finalText.WriteString(delta.Text)
				}
			}
		}
		cancel()

		if err := stream.Err(); err != nil {
			onEvent(StreamEvent{Type: "error", Content: err.Error()})
			return nil, fmt.Errorf("anthropic streaming failed (iteration %d): %w", iteration, err)
		}

		// Process accumulated content blocks for tool calls
		hasToolUse := false
		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range message.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				hasToolUse = true
				argsJSON, _ := json.Marshal(variant.Input)
				toolArgs := string(argsJSON)

				log.Printf("\033[1;36m[ai]\033[0m tool_call: %s %s", variant.Name, toolArgs)
				onEvent(StreamEvent{Type: "tool_call", Tool: variant.Name, Args: toolArgs, ToolCallID: variant.ID})

				tool, ok := toolMap[variant.Name]
				if !ok {
					errMsg := fmt.Sprintf("unknown tool: %s", variant.Name)
					log.Printf("\033[1;36m[ai]\033[0m tool_error: %s", errMsg)
					onEvent(StreamEvent{Type: "tool_result", Tool: variant.Name, Content: errMsg, ToolCallID: variant.ID})
					toolResults = append(toolResults, anthropic.NewToolResultBlock(variant.ID, errMsg, true))
					allToolCalls = append(allToolCalls, ToolCallRecord{Tool: variant.Name, Args: toolArgs, Result: errMsg})
					continue
				}

				result, execErr := tool.Execute(ctx, json.RawMessage(toolArgs))
				if execErr != nil {
					result = fmt.Sprintf("error: %v", execErr)
				}

				log.Printf("\033[1;36m[ai]\033[0m tool_result: %s (%d chars)", variant.Name, len(result))
				onEvent(StreamEvent{Type: "tool_result", Tool: variant.Name, Content: truncateResult(result, 200), ToolCallID: variant.ID})

				toolResults = append(toolResults, anthropic.NewToolResultBlock(variant.ID, result, false))
				allToolCalls = append(allToolCalls, ToolCallRecord{Tool: variant.Name, Args: toolArgs, Result: result})
			}
		}

		// If no tool use, we're done
		if !hasToolUse {
			onEvent(StreamEvent{Type: "done"})
			break
		}

		// Append assistant message and tool results to continue conversation
		messages = append(messages, message.ToParam())
		messages = append(messages, anthropic.NewUserMessage(toolResults...))

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
