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
	"github.com/skyhook-io/radar/internal/topology"
)

// buildTools returns the set of tools available during an AI investigation.
// These call Radar's internal functions directly (not via HTTP).
func buildTools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        "get_resource",
			Description: "Get detailed information about a Kubernetes resource including its spec, status, and conditions.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "resource kind (e.g. pod, deployment, service)"},
					"namespace": map[string]any{"type": "string", "description": "resource namespace"},
					"name":      map[string]any{"type": "string", "description": "resource name"},
				},
				"required": []string{"kind", "namespace", "name"},
			}),
			Execute: executeGetResource,
		},
		{
			Name:        "get_events",
			Description: "Get Kubernetes warning events, optionally filtered to a specific resource. Events show scheduling failures, image pull errors, OOM kills, etc.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "filter to a specific namespace"},
					"kind":      map[string]any{"type": "string", "description": "filter to events involving this resource kind (e.g. Pod, Deployment)"},
					"name":      map[string]any{"type": "string", "description": "filter to events involving this resource name"},
				},
			}),
			Execute: executeGetEvents,
		},
		{
			Name:        "get_pod_logs",
			Description: "Get filtered log lines from a pod, prioritizing errors and warnings. Returns diagnostically relevant lines.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "pod namespace"},
					"name":      map[string]any{"type": "string", "description": "pod name"},
					"container": map[string]any{"type": "string", "description": "container name (defaults to first container)"},
				},
				"required": []string{"namespace", "name"},
			}),
			Execute: executeGetPodLogs,
		},
		{
			Name:        "get_changes",
			Description: "Get recent resource changes (creates, updates, deletes) from the cluster timeline. Use to investigate what changed before an incident.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "filter to a specific namespace"},
					"kind":      map[string]any{"type": "string", "description": "filter to a resource kind (e.g. Deployment, Pod)"},
					"name":      map[string]any{"type": "string", "description": "filter to a specific resource name"},
					"since":     map[string]any{"type": "string", "description": "duration to look back, e.g. 1h, 30m (default 1h)"},
				},
			}),
			Execute: executeGetChanges,
		},
		{
			Name:        "get_related_resources",
			Description: "Get resources related to a specific resource — parents, children, services, config, scalers, etc.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "resource kind"},
					"namespace": map[string]any{"type": "string", "description": "resource namespace"},
					"name":      map[string]any{"type": "string", "description": "resource name"},
				},
				"required": []string{"kind", "namespace", "name"},
			}),
			Execute: executeGetRelatedResources,
		},
		{
			Name:        "list_resources",
			Description: "List Kubernetes resources of a given kind with minified summaries. Use to see what pods/deployments/etc exist in a namespace.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "description": "resource kind (e.g. pods, deployments, services)"},
					"namespace": map[string]any{"type": "string", "description": "filter to a specific namespace"},
				},
				"required": []string{"kind"},
			}),
			Execute: executeListResources,
		},
	}
}

// Tool execution functions

func executeGetResource(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	cache := k8s.GetResourceCache()
	if cache == nil {
		return "", fmt.Errorf("not connected to cluster")
	}

	kind := strings.ToLower(input.Kind)
	obj, err := k8s.FetchResource(cache, kind, input.Namespace, input.Name)
	if err == k8s.ErrUnknownKind {
		u, dynErr := cache.GetDynamicWithGroup(ctx, kind, input.Namespace, input.Name, "")
		if dynErr != nil {
			return "", fmt.Errorf("resource not found: %w", dynErr)
		}
		return toJSON(aicontext.MinifyUnstructured(u, aicontext.LevelDetail))
	}
	if err != nil {
		return "", fmt.Errorf("resource not found: %w", err)
	}

	k8s.SetTypeMeta(obj)
	minified, err := aicontext.Minify(obj, aicontext.LevelDetail)
	if err != nil {
		return "", fmt.Errorf("failed to minify: %w", err)
	}
	return toJSON(minified)
}

func executeGetEvents(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	cache := k8s.GetResourceCache()
	if cache == nil {
		return "", fmt.Errorf("not connected to cluster")
	}

	eventLister := cache.Events()
	if eventLister == nil {
		return "[]", nil
	}

	var events []*corev1.Event
	var err error
	if input.Namespace != "" {
		events, err = eventLister.Events(input.Namespace).List(labels.Everything())
	} else {
		events, err = eventLister.List(labels.Everything())
	}
	if err != nil {
		return "", fmt.Errorf("failed to list events: %w", err)
	}

	// Filter to warning events involving the specified resource
	var warnings []corev1.Event
	for _, e := range events {
		if e.Type != "Warning" {
			continue
		}
		if input.Kind != "" && !strings.EqualFold(e.InvolvedObject.Kind, input.Kind) {
			continue
		}
		if input.Name != "" && e.InvolvedObject.Name != input.Name {
			continue
		}
		warnings = append(warnings, *e)
	}

	if len(warnings) == 0 {
		return "[]", nil
	}

	deduplicated := aicontext.DeduplicateEvents(warnings)
	if len(deduplicated) > 15 {
		deduplicated = deduplicated[:15]
	}
	return toJSON(deduplicated)
}

func executeGetPodLogs(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Container string `json:"container"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	client := k8s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected to cluster")
	}

	tailLines := int64(200)
	opts := &corev1.PodLogOptions{TailLines: &tailLines}
	if input.Container != "" {
		opts.Container = input.Container
	}

	stream, err := client.CoreV1().Pods(input.Namespace).GetLogs(input.Name, opts).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	filtered := aicontext.FilterLogs(string(data))
	return toJSON(filtered)
}

func executeGetChanges(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Since     string `json:"since"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	store := timeline.GetStore()
	if store == nil {
		return "[]", nil
	}

	since := 1 * time.Hour
	if input.Since != "" {
		parsed, err := time.ParseDuration(input.Since)
		if err != nil {
			return "", fmt.Errorf("invalid duration %q: %w", input.Since, err)
		}
		since = parsed
	}

	queryOpts := timeline.QueryOptions{
		Since:        time.Now().Add(-since),
		FilterPreset: "default",
		Limit:        20,
	}
	if input.Namespace != "" {
		queryOpts.Namespaces = []string{input.Namespace}
	}
	if input.Kind != "" {
		queryOpts.Kinds = []string{input.Kind}
	}
	if input.Name != "" {
		queryOpts.Limit = 200 // fetch more to compensate for name filter
	}

	events, err := store.Query(ctx, queryOpts)
	if err != nil {
		return "", fmt.Errorf("failed to query timeline: %w", err)
	}

	// Post-filter by name
	if input.Name != "" {
		filtered := events[:0]
		for _, e := range events {
			if e.Name == input.Name {
				filtered = append(filtered, e)
			}
		}
		events = filtered
		if len(events) > 20 {
			events = events[:20]
		}
	}

	type change struct {
		Kind       string `json:"kind"`
		Namespace  string `json:"namespace"`
		Name       string `json:"name"`
		ChangeType string `json:"changeType"`
		Summary    string `json:"summary"`
		Timestamp  string `json:"timestamp"`
	}

	changes := make([]change, 0, len(events))
	for _, e := range events {
		summary := ""
		if e.Diff != nil && e.Diff.Summary != "" {
			summary = e.Diff.Summary
		} else if e.Message != "" {
			summary = k8s.Truncate(e.Message, 100)
		}
		changes = append(changes, change{
			Kind:       e.Kind,
			Namespace:  e.Namespace,
			Name:       e.Name,
			ChangeType: string(e.EventType),
			Summary:    summary,
			Timestamp:  e.Timestamp.Format(time.RFC3339),
		})
	}

	return toJSON(changes)
}

func executeGetRelatedResources(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	opts := topology.DefaultBuildOptions()
	if input.Namespace != "" {
		opts.Namespaces = []string{input.Namespace}
	}

	builder := topology.NewBuilder()
	topo, err := builder.Build(opts)
	if err != nil {
		return "", fmt.Errorf("failed to build topology: %w", err)
	}

	rels := topology.GetRelationships(input.Kind, input.Namespace, input.Name, topo)
	if rels == nil {
		return `{"message": "no relationships found"}`, nil
	}

	return toJSON(rels)
}

func executeListResources(ctx context.Context, params json.RawMessage) (string, error) {
	var input struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	cache := k8s.GetResourceCache()
	if cache == nil {
		return "", fmt.Errorf("not connected to cluster")
	}

	kind := strings.ToLower(input.Kind)
	var namespaces []string
	if input.Namespace != "" {
		namespaces = []string{input.Namespace}
	}

	objs, err := k8s.FetchResourceList(cache, kind, namespaces)
	if err == k8s.ErrUnknownKind {
		// Try dynamic cache for CRDs
		var allItems []any
		items, dynErr := cache.ListDynamicWithGroup(ctx, kind, input.Namespace, "")
		if dynErr != nil {
			return "", fmt.Errorf("failed to list %s: %w", kind, dynErr)
		}
		for _, item := range items {
			allItems = append(allItems, aicontext.MinifyUnstructured(item, aicontext.LevelSummary))
		}
		return toJSON(allItems)
	}
	if err != nil {
		return "", fmt.Errorf("failed to list %s: %w", kind, err)
	}

	results, err := aicontext.MinifyList(objs, aicontext.LevelSummary)
	if err != nil {
		return "", fmt.Errorf("failed to minify: %w", err)
	}

	return toJSON(results)
}

// Helpers

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		log.Fatalf("[ai] Failed to marshal JSON schema: %v", err)
	}
	return data
}

func toJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(data), nil
}
