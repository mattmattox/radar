package server

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/skyhook-io/radar/internal/k8s"
	"github.com/skyhook-io/radar/pkg/packages"
	"github.com/skyhook-io/radar/pkg/subject"
)

// Applications is the workload-centric twin of /api/packages. Where packages
// answers "what software is installed" (chart/GitOps-declaration centric, the
// Add-ons surface), Applications answers "what are MY services and what version
// runs where" — the unit is a logical app: the set of workloads sharing a
// pkg/subject Tier-2 app-overlay key, anchored on the container image:tag (the
// real running version, not a chart version that's empty for git-based apps).
//
// Why a separate path (not the packages feed): collectWorkloadInputs only
// admits Helm-labeled workloads and carries no image, so the packages feed
// structurally cannot represent a non-Helm app's services or their versions.
// See ADDONS-AND-APPLICATIONS-PLAN.md §8.3 (the row-identity change) — this is
// that change done on the right entities.

// applicationsResponse is the GET /api/applications body.
type applicationsResponse struct {
	Applications []appRow `json:"applications"`
}

// appRow is one logical app in this cluster: workloads collapsed by app-overlay
// key. Raw-always: a workload with no app signal (no overlay) is its own row
// keyed by "<ns>/<kind>/<name>" — nothing is hidden.
type appRow struct {
	Key        string         `json:"key"`                  // overlay key, or "<ns>/<kind>/<name>" raw
	Name       string         `json:"name"`                 // display name
	Namespace  string         `json:"namespace,omitempty"`  // overlay namespace (grouping scope)
	Tier       int            `json:"tier,omitempty"`       // pkg/subject overlay tier (0 = raw, no overlay)
	Confidence string         `json:"confidence,omitempty"` // high | medium | low
	Health     string         `json:"health"`               // worst-of across workloads
	Versions   []string       `json:"versions,omitempty"`   // distinct image tags (the running version)
	Workloads  []appWorkload  `json:"workloads"`
	Events     []appEvent     `json:"events,omitempty"`     // recent Warning events across the app's workloads/pods
}

// appEvent is a recent k8s Warning event correlated to an app's workloads/pods
// (the "why is it broken" feed — BackOff, FailedScheduling, FailedMount, …).
type appEvent struct {
	Type     string `json:"type"`
	Reason   string `json:"reason"`
	Message  string `json:"message,omitempty"`
	Count    int    `json:"count"`
	Object   string `json:"object"` // "<Kind>/<name>"
	LastSeen string `json:"lastSeen,omitempty"`
}

// appWorkload is one running controller (Deployment/StatefulSet/DaemonSet)
// belonging to an app, with its primary container image as the version anchor.
type appWorkload struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Image     string `json:"image,omitempty"`   // full primary-container image ref
	Version   string `json:"version,omitempty"` // image tag (digest-only → empty)
	Health    string `json:"health"`
	Ready     int    `json:"ready"`             // ready/available replicas
	Desired   int    `json:"desired"`           // desired replicas
	Restarts  int    `json:"restarts"`          // total container restarts across the workload's pods
	Reason    string `json:"reason,omitempty"`  // last-terminated reason of the worst pod (CrashLoopBackOff/OOMKilled/…)
}

// handleListApplications serves GET /api/applications.
//
//	?namespaces=a,b,c | ?namespace=a — limit to workloads in the namespace set.
func (s *Server) handleListApplications(w http.ResponseWriter, r *http.Request) {
	if !s.requireConnected(w) {
		return
	}
	namespaces := s.parseNamespacesForUser(r)
	resp, err := ListApplications(r.Context(), namespaces)
	if err != nil {
		if err == errResourceCacheUnavailable {
			s.writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, resp)
}

// ListApplications enumerates the cluster's app workloads, resolves each to its
// pkg/subject app-overlay, and groups them into logical apps. Add-on workloads
// (cert-manager et al.) are excluded — they belong to the Add-ons surface.
func ListApplications(ctx context.Context, namespaces []string) (*applicationsResponse, error) {
	cache := k8s.GetResourceCache()
	if cache == nil {
		return nil, errResourceCacheUnavailable
	}
	wls := collectAppWorkloads(cache, namespaces)
	return &applicationsResponse{Applications: groupApplications(wls)}, nil
}

// appWorkloadInput is the pre-grouping shape: a workload plus its resolved
// overlay (nil when no app signal at/above tier 7).
type appWorkloadInput struct {
	wl      appWorkload
	overlay *subject.AppOverlay
	events  []appEvent
}

// collectAppWorkloads walks Deployments/StatefulSets/DaemonSets, captures the
// primary container image, resolves the app-overlay, and drops add-on workloads.
func collectAppWorkloads(cache *k8s.ResourceCache, namespaces []string) []appWorkloadInput {
	var out []appWorkloadInput

	add := func(kind, ns, name string, lbls, anns map[string]string, image string, health packages.Health, ready, desired int, selector *metav1.LabelSelector) {
		// Cluster plumbing (kube-proxy, kindnet, CNI, local-path) is never a
		// user service — exclude system namespaces outright.
		if systemNamespaces[ns] {
			return
		}
		// Add-ons live on their own surface — keep 3rd-party platform machinery
		// out of "your services". (Interim chart/name match; consolidate with
		// the SPA's KNOWN_ADDON list + OSS catalog later — plan §12 Q3.)
		if isAddonWorkload(lbls, name) {
			return
		}
		// One pod fetch powers both restarts (crashloop signal) and the Warning
		// event feed (image-pull / scheduling / mount failures restarts miss).
		var pods []*corev1.Pod
		if selector != nil {
			pods = cache.GetPodsForWorkload(ns, selector)
		}
		restarts, reason := podsRestarts(pods)
		meta := metav1.ObjectMeta{Namespace: ns, Name: name, Labels: lbls, Annotations: anns}
		ov := subject.ResolveOverlay(&meta, false)
		out = append(out, appWorkloadInput{
			wl: appWorkload{
				Kind:      kind,
				Namespace: ns,
				Name:      name,
				Image:     image,
				Version:   imageTag(image),
				Health:    string(health),
				Ready:     ready,
				Desired:   desired,
				Restarts:  restarts,
				Reason:    reason,
			},
			overlay: ov,
			events:  podsEvents(cache, ns, name, pods),
		})
	}

	forEachNamespace := func(fn func(ns string)) {
		if namespaces == nil {
			fn("")
			return
		}
		for _, ns := range namespaces {
			fn(ns)
		}
	}

	if depLister := cache.Deployments(); depLister != nil {
		forEachNamespace(func(ns string) {
			var items []*appsv1.Deployment
			if ns == "" {
				items, _ = depLister.List(labels.Everything())
			} else {
				items, _ = depLister.Deployments(ns).List(labels.Everything())
			}
			for _, d := range items {
				add("Deployment", d.Namespace, d.Name, d.Labels, d.Annotations,
					primaryImage(d.Spec.Template.Spec.Containers),
					deploymentHealth(int(d.Status.Replicas), int(d.Status.AvailableReplicas)),
					int(d.Status.AvailableReplicas), int(d.Status.Replicas), d.Spec.Selector)
			}
		})
	}
	if dsLister := cache.DaemonSets(); dsLister != nil {
		forEachNamespace(func(ns string) {
			var items []*appsv1.DaemonSet
			if ns == "" {
				items, _ = dsLister.List(labels.Everything())
			} else {
				items, _ = dsLister.DaemonSets(ns).List(labels.Everything())
			}
			for _, d := range items {
				add("DaemonSet", d.Namespace, d.Name, d.Labels, d.Annotations,
					primaryImage(d.Spec.Template.Spec.Containers),
					daemonsetHealth(int(d.Status.DesiredNumberScheduled), int(d.Status.NumberReady)),
					int(d.Status.NumberReady), int(d.Status.DesiredNumberScheduled), d.Spec.Selector)
			}
		})
	}
	if ssLister := cache.StatefulSets(); ssLister != nil {
		forEachNamespace(func(ns string) {
			var items []*appsv1.StatefulSet
			if ns == "" {
				items, _ = ssLister.List(labels.Everything())
			} else {
				items, _ = ssLister.StatefulSets(ns).List(labels.Everything())
			}
			for _, d := range items {
				add("StatefulSet", d.Namespace, d.Name, d.Labels, d.Annotations,
					primaryImage(d.Spec.Template.Spec.Containers),
					statefulsetHealth(int(d.Status.Replicas), int(d.Status.ReadyReplicas)),
					int(d.Status.ReadyReplicas), int(d.Status.Replicas), d.Spec.Selector)
			}
		})
	}
	return out
}

// groupApplications collapses workloads by app-overlay key. Workloads with no
// overlay (raw-always) become their own single-workload row keyed on identity.
func groupApplications(inputs []appWorkloadInput) []appRow {
	rows := map[string]*appRow{}
	order := []string{}
	for _, in := range inputs {
		var key, name, ns string
		var tier int
		var conf string
		if in.overlay != nil {
			win := in.overlay.Winner
			key = win.Key
			name = appNameFromKey(win.Key)
			ns = namespaceFromKey(win.Key)
			tier = int(win.Tier)
			conf = string(win.Confidence)
		} else {
			// Raw-always: identifiable on its own, never hidden.
			key = in.wl.Namespace + "/" + in.wl.Kind + "/" + in.wl.Name
			name = in.wl.Name
			ns = in.wl.Namespace
		}
		r, ok := rows[key]
		if !ok {
			r = &appRow{Key: key, Name: name, Namespace: ns, Tier: tier, Confidence: conf}
			rows[key] = r
			order = append(order, key)
		}
		r.Workloads = append(r.Workloads, in.wl)
		r.Events = append(r.Events, in.events...)
		r.Health = string(worstAppHealth(packages.Health(r.Health), packages.Health(in.wl.Health)))
		if v := in.wl.Version; v != "" && !contains(r.Versions, v) {
			r.Versions = append(r.Versions, v)
		}
	}
	out := make([]appRow, 0, len(order))
	for _, k := range order {
		r := rows[k]
		sort.Strings(r.Versions)
		// Newest events first; cap the feed.
		sort.SliceStable(r.Events, func(i, j int) bool { return r.Events[i].LastSeen > r.Events[j].LastSeen })
		if len(r.Events) > 12 {
			r.Events = r.Events[:12]
		}
		out = append(out, *r)
	}
	// Deterministic: by name then key.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// --- small helpers --------------------------------------------------------

// primaryImage returns the first container's image (the conventional "the app"
// container — mirrors pkg/ai/context/summary.go's first-container choice).
func primaryImage(containers []corev1.Container) string {
	if len(containers) > 0 {
		return containers[0].Image
	}
	return ""
}

// podsRestarts sums container restarts across a workload's pods and returns the
// last-terminated reason of the worst (most-restarting) pod — the crash signal
// (CrashLoopBackOff / OOMKilled / Error).
func podsRestarts(pods []*corev1.Pod) (int, string) {
	total := 0
	var worst int32 = -1
	reason := ""
	for _, p := range pods {
		rc, r := k8s.PodRestartContext(p)
		total += int(rc)
		if rc > worst {
			worst = rc
			reason = r
		}
	}
	return total, reason
}

// podsEvents collects recent Warning events involving the workload or its pods,
// deduped by (object, reason) with summed counts — the "why is it broken" feed
// (FailedScheduling, ImagePullBackOff, FailedMount, …) that restarts alone miss.
func podsEvents(cache *k8s.ResourceCache, ns, workloadName string, pods []*corev1.Pod) []appEvent {
	lister := cache.Events()
	if lister == nil {
		return nil
	}
	names := map[string]bool{workloadName: true}
	for _, p := range pods {
		names[p.Name] = true
	}
	evs, err := lister.Events(ns).List(labels.Everything())
	if err != nil {
		return nil
	}
	byKey := map[string]*appEvent{}
	order := []string{}
	for _, e := range evs {
		if e.Type != "Warning" || !names[e.InvolvedObject.Name] {
			continue
		}
		key := e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name + "/" + e.Reason
		c := int(e.Count)
		if c < 1 {
			c = 1
		}
		if a, ok := byKey[key]; ok {
			a.Count += c
			if ts := e.LastTimestamp.Format(time.RFC3339); ts > a.LastSeen {
				a.LastSeen = ts
				a.Message = e.Message
			}
			continue
		}
		ae := &appEvent{Type: e.Type, Reason: e.Reason, Message: e.Message, Count: c, Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, LastSeen: e.LastTimestamp.Format(time.RFC3339)}
		byKey[key] = ae
		order = append(order, key)
	}
	out := make([]appEvent, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// imageTag extracts the tag from an image ref. Digest-pinned refs (@sha256:…)
// and untagged refs (implicit :latest) return "" — no false version.
func imageTag(image string) string {
	if image == "" {
		return ""
	}
	if at := strings.Index(image, "@"); at >= 0 {
		image = image[:at]
	}
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon > slash {
		return image[colon+1:]
	}
	return ""
}

func appNameFromKey(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 && i < len(key)-1 {
		return key[i+1:]
	}
	return key
}

func namespaceFromKey(key string) string {
	if i := strings.Index(key, "/"); i > 0 {
		return key[:i]
	}
	return ""
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// worstAppHealth merges two health values (local copy of pkg/packages's
// unexported worseHealth): unhealthy > degraded > unknown > healthy; "" defers
// to the other side.
func worstAppHealth(a, b packages.Health) packages.Health {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if appHealthRank(a) >= appHealthRank(b) {
		return a
	}
	return b
}

func appHealthRank(h packages.Health) int {
	switch h {
	case packages.HealthUnhealthy:
		return 4
	case packages.HealthDegraded:
		return 3
	case packages.HealthUnknown:
		return 2
	case packages.HealthHealthy:
		return 1
	}
	return 2
}

// systemNamespaces hold cluster plumbing, never user services.
var systemNamespaces = map[string]bool{
	"kube-system":        true,
	"kube-public":        true,
	"kube-node-lease":    true,
	"local-path-storage": true,
}

// knownAddonNames — interim add-on allowlist (mirrors radar-hub-web
// packagesModel.KNOWN_ADDON_CHARTS; consolidate into the OSS catalog later).
var knownAddonNames = map[string]bool{
	"cert-manager": true, "argo-cd": true, "argocd": true, "argo-rollouts": true,
	"argo-workflows": true, "argo-events": true, "flux": true, "flux2": true,
	"karpenter": true, "external-secrets": true, "velero": true, "kyverno": true,
	"kube-prometheus-stack": true, "prometheus": true, "prometheus-operator": true,
	"grafana": true, "loki": true, "tempo": true, "mimir": true, "istio": true,
	"istiod": true, "istio-base": true, "traefik": true, "cloudnative-pg": true,
	"cnpg": true, "opentelemetry-operator": true, "opentelemetry-collector": true,
	"keda": true, "cluster-api": true, "trivy-operator": true, "cilium": true,
	"ingress-nginx": true, "nginx-ingress": true, "external-dns": true,
	"metrics-server": true, "cluster-autoscaler": true, "sealed-secrets": true,
	"vault": true, "fluent-bit": true, "fluentd": true, "vector": true,
	"opencost": true, "kubecost": true, "reloader": true, "descheduler": true,
	"aws-load-balancer-controller": true, "gatekeeper": true, "kube-state-metrics": true,
	"coredns": true, "calico": true, "longhorn": true, "crossplane": true, "metallb": true,
}

// isAddonWorkload returns true when a workload belongs to 3rd-party platform
// machinery (so it stays on Add-ons, not Applications). Matches the helm chart
// name, app.kubernetes.io/name, part-of, or the workload name against the
// allowlist — exact or hyphen-prefixed ("kube-prometheus-stack-…").
func isAddonWorkload(lbls map[string]string, name string) bool {
	candidates := []string{
		chartBaseName(lbls["helm.sh/chart"]),
		lbls["app.kubernetes.io/name"],
		lbls["app.kubernetes.io/part-of"],
		name,
	}
	for _, c := range candidates {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if knownAddonNames[c] {
			return true
		}
		for known := range knownAddonNames {
			if strings.HasPrefix(c, known+"-") {
				return true
			}
		}
	}
	return false
}

// chartBaseName strips a trailing -<version> from a helm.sh/chart value
// ("cert-manager-v1.14.0" → "cert-manager").
func chartBaseName(chart string) string {
	for i := len(chart) - 1; i >= 1; i-- {
		if chart[i-1] != '-' {
			continue
		}
		c := chart[i]
		if (c >= '0' && c <= '9') || c == 'v' {
			return chart[:i-1]
		}
	}
	return chart
}
