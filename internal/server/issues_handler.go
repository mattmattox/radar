package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/skyhook-io/radar/internal/filter"
	"github.com/skyhook-io/radar/internal/issues"
)

// handleIssues serves GET /api/issues — the unified cluster-health
// endpoint. Composes problems + audit findings (opt-in) + warning
// events + generic CRD condition fallback into one normalized list.
//
// Query params:
//
//	namespace= / namespaces=  one or comma-separated
//	severity=  critical,warning  (default: all)
//	source=    problem,audit,event,condition. Default omits audit;
//	           opt audit in by passing 'audit' in source= OR by
//	           setting include_audit=true.
//	kind=      Pod,Deployment,...  (default: all)
//	since=     duration like 15m, 1h (default: no time restriction; only affects events)
//	limit=     default 200, max 1000
//	include_audit=true  shorthand to opt audit findings in
func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	if !s.requireConnected(w) {
		return
	}
	provider := issues.NewCacheProvider()
	if provider == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Resource cache not available")
		return
	}

	q := r.URL.Query()

	// Auth-filter the requested namespaces. nil = "all namespaces" (user
	// is unrestricted); non-nil empty = "user has no access to anything
	// they asked for" → return empty rather than leak cluster-wide rows.
	namespaces := s.parseNamespacesForUser(r)
	if noNamespaceAccess(namespaces) {
		s.writeJSON(w, map[string]any{"issues": []any{}, "total": 0})
		return
	}

	severities, err := parseSeverities(q.Get("severity"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sources, err := parseSources(q.Get("source"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	since, err := parseDuration(q.Get("since"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	filters := issues.Filters{
		Namespaces:   namespaces,
		Severities:   severities,
		Sources:      sources,
		Kinds:        splitCSV(q.Get("kind")),
		Since:        since,
		Limit:        parseLimit(q.Get("limit")),
		IncludeAudit: q.Get("include_audit") == "true" || hasSourceAudit(q.Get("source")),
	}
	if expr := q.Get("filter"); expr != "" {
		f, err := filter.CachedIssueFilter(expr)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "filter: "+err.Error())
			return
		}
		filters.Filter = f
	}

	out, stats := issues.ComposeWithStats(provider, filters)
	resp := map[string]any{
		"issues": out,
		"total":  len(out),
	}
	if stats.FilterErrors > 0 {
		resp["filter_errors"] = stats.FilterErrors
		resp["filter_error_sample"] = stats.FilterErrorSample
	}
	s.writeJSON(w, resp)
}

func parseSeverities(v string) ([]issues.Severity, error) {
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]issues.Severity, 0, len(parts))
	for _, p := range parts {
		s := strings.ToLower(strings.TrimSpace(p))
		switch s {
		case "":
			continue
		case "critical":
			out = append(out, issues.SeverityCritical)
		case "warning":
			out = append(out, issues.SeverityWarning)
		default:
			return nil, fmt.Errorf("unknown severity %q (want: critical, warning)", p)
		}
	}
	return out, nil
}

func parseSources(v string) ([]issues.Source, error) {
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]issues.Source, 0, len(parts))
	for _, p := range parts {
		s := strings.ToLower(strings.TrimSpace(p))
		switch s {
		case "":
			continue
		case "problem":
			out = append(out, issues.SourceProblem)
		case "audit":
			out = append(out, issues.SourceAudit)
		case "event":
			out = append(out, issues.SourceEvent)
		case "condition":
			out = append(out, issues.SourceCondition)
		default:
			return nil, fmt.Errorf("unknown source %q (want: problem, audit, event, condition)", p)
		}
	}
	return out, nil
}

// hasSourceAudit lets `?source=audit` implicitly opt audit in without
// the caller also passing `?include_audit=true` — the param-source
// list is more discoverable.
func hasSourceAudit(v string) bool {
	for _, p := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(p), "audit") {
			return true
		}
	}
	return false
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseDuration(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid since=%q: %w", v, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("since must be non-negative, got %s", d)
	}
	return d, nil
}
