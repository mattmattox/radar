package investigate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	aicontext "github.com/skyhook-io/radar/internal/ai/context"
	"github.com/skyhook-io/radar/internal/ai/llm"
	"github.com/skyhook-io/radar/internal/k8s"
	"github.com/skyhook-io/radar/internal/timeline"
)

// Engine orchestrates AI-powered investigations.
type Engine struct {
	provider llm.Provider
}

// NewEngine creates an investigation engine with the given LLM provider.
func NewEngine(provider llm.Provider) *Engine {
	return &Engine{provider: provider}
}

// InvestigateParams defines what to investigate.
type InvestigateParams struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Question  string `json:"question,omitempty"`
}

// Event represents a streamed investigation progress event.
type Event struct {
	Type       string `json:"type"`                  // "status", "tool_call", "tool_result", "analysis", "error", "done"
	Content    string `json:"content"`               // human-readable content
	Tool       string `json:"tool,omitempty"`        // tool name for tool_call events
	Args       string `json:"args,omitempty"`        // tool args for tool_call events
	ToolCallID string `json:"toolCallId,omitempty"`  // unique ID for correlating tool calls with results
}

// Investigate runs an AI investigation on the specified resource.
// Progress is streamed via the onEvent callback.
func (e *Engine) Investigate(ctx context.Context, params InvestigateParams, onEvent func(Event)) error {
	onEvent(Event{Type: "status", Content: "Assembling resource context..."})

	// Build initial context from Radar's data
	initialContext, err := assembleInitialContext(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to assemble context: %w", err)
	}

	userPrompt := buildUserPrompt(params.Kind, params.Namespace, params.Name, initialContext, params.Question)

	// Build tools for the investigation
	tools := buildTools()

	// Bridge investigation events to the caller (engine sends its own "done")
	llmOnEvent := func(ev llm.StreamEvent) {
		switch ev.Type {
		case "step_start":
			onEvent(Event{Type: "step_start"})
		case "tool_call":
			onEvent(Event{
				Type:       "tool_call",
				Content:    fmt.Sprintf("Calling %s", ev.Tool),
				Tool:       ev.Tool,
				Args:       ev.Args,
				ToolCallID: ev.ToolCallID,
			})
		case "tool_result":
			onEvent(Event{
				Type:       "tool_result",
				Content:    ev.Content,
				Tool:       ev.Tool,
				ToolCallID: ev.ToolCallID,
			})
		case "text":
			onEvent(Event{Type: "analysis", Content: ev.Content})
		case "thinking":
			onEvent(Event{Type: "status", Content: ev.Content})
		case "error":
			onEvent(Event{Type: "error", Content: ev.Content})
		case "done":
			// Handled by engine after Investigate returns
		}
	}

	req := llm.InvestigateRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Tools:        tools,
	}

	_, err = e.provider.Investigate(ctx, req, llmOnEvent)
	if err != nil {
		onEvent(Event{Type: "error", Content: err.Error()})
		return fmt.Errorf("investigation failed: %w", err)
	}

	onEvent(Event{Type: "done", Content: ""})
	return nil
}

// assembleInitialContext gathers resource data to provide as the starting context.
func assembleInitialContext(ctx context.Context, params InvestigateParams) (string, error) {
	cache := k8s.GetResourceCache()
	if cache == nil {
		return "", fmt.Errorf("not connected to cluster")
	}

	kind := strings.ToLower(params.Kind)
	sections := aicontext.ContextSections{
		ResourceKind:      params.Kind,
		ResourceNamespace: params.Namespace,
		ResourceName:      params.Name,
	}

	// 1. Minified resource
	obj, err := k8s.FetchResource(cache, kind, params.Namespace, params.Name)
	if err == nil {
		k8s.SetTypeMeta(obj)
		if minified, minErr := aicontext.Minify(obj, aicontext.LevelDetail); minErr == nil {
			data, _ := json.MarshalIndent(minified, "", "  ")
			sections.MinifiedResource = string(data)
		}
	} else if err == k8s.ErrUnknownKind {
		u, dynErr := cache.GetDynamicWithGroup(ctx, kind, params.Namespace, params.Name, "")
		if dynErr == nil {
			data, _ := json.MarshalIndent(aicontext.MinifyUnstructured(u, aicontext.LevelDetail), "", "  ")
			sections.MinifiedResource = string(data)
		}
	}

	// 2. Events for this resource
	if eventLister := cache.Events(); eventLister != nil {
		var events []*corev1.Event
		if params.Namespace != "" {
			events, _ = eventLister.Events(params.Namespace).List(labels.Everything())
		} else {
			events, _ = eventLister.List(labels.Everything())
		}
		var matched []corev1.Event
		for _, e := range events {
			if e.Type != "Warning" {
				continue
			}
			if strings.EqualFold(e.InvolvedObject.Kind, params.Kind) && e.InvolvedObject.Name == params.Name {
				matched = append(matched, *e)
			}
		}
		if len(matched) > 0 {
			deduplicated := aicontext.DeduplicateEvents(matched)
			if len(deduplicated) > 10 {
				deduplicated = deduplicated[:10]
			}
			data, _ := json.MarshalIndent(deduplicated, "", "  ")
			sections.Events = string(data)
		}
	}

	// 3. Logs (if pod)
	if isPodKind(kind) {
		if client := k8s.GetClient(); client != nil {
			tailLines := int64(100)
			opts := &corev1.PodLogOptions{TailLines: &tailLines}
			stream, logErr := client.CoreV1().Pods(params.Namespace).GetLogs(params.Name, opts).Stream(ctx)
			if logErr == nil {
				defer stream.Close()
				data, readErr := io.ReadAll(stream)
				if readErr == nil {
					filtered := aicontext.FilterLogs(string(data))
					jsonData, _ := json.MarshalIndent(filtered, "", "  ")
					sections.Logs = string(jsonData)
				}
			}
		}
	}

	// 4. Recent changes
	if store := timeline.GetStore(); store != nil {
		queryOpts := timeline.QueryOptions{
			Since:        time.Now().Add(-1 * time.Hour),
			FilterPreset: "workloads",
			Limit:        10,
		}
		if params.Namespace != "" {
			queryOpts.Namespaces = []string{params.Namespace}
		}
		changes, queryErr := store.Query(ctx, queryOpts)
		if queryErr == nil && len(changes) > 0 {
			type change struct {
				Kind       string `json:"kind"`
				Name       string `json:"name"`
				ChangeType string `json:"changeType"`
				Summary    string `json:"summary"`
				Timestamp  string `json:"timestamp"`
			}
			var changeSummaries []change
			for _, c := range changes {
				summary := ""
				if c.Diff != nil && c.Diff.Summary != "" {
					summary = c.Diff.Summary
				} else if c.Message != "" {
					summary = k8s.Truncate(c.Message, 100)
				}
				changeSummaries = append(changeSummaries, change{
					Kind:       c.Kind,
					Name:       c.Name,
					ChangeType: string(c.EventType),
					Summary:    summary,
					Timestamp:  c.Timestamp.Format(time.RFC3339),
				})
			}
			data, _ := json.MarshalIndent(changeSummaries, "", "  ")
			sections.Metrics = string(data) // Reuse metrics slot for changes in initial context
		}
	}

	assembled := aicontext.AssembleContext(sections, aicontext.BudgetCloud)
	log.Printf("[ai] Assembled initial context: %d chars", len(assembled))
	return assembled, nil
}

func isPodKind(kind string) bool {
	return kind == "pod" || kind == "pods"
}
