package topology

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func meta(labels, annos map[string]string) metav1.Object {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annos}}
}

// TestSynthesizeManagedBy_LabelsAndAnnotations covers precedence rules that
// mirror packages/k8s-ui/src/utils/gitops-owner.ts. The test order is the
// detection order — a stable contract because T14 will retire the frontend
// copy and assert equivalence.
func TestSynthesizeManagedBy_LabelsAndAnnotations(t *testing.T) {
	cases := []struct {
		name     string
		labels   map[string]string
		annos    map[string]string
		wantKind string
		wantNS   string
		wantName string
		wantGrp  string
	}{
		{
			name:     "Flux HelmRelease labels",
			labels:   map[string]string{fluxHelmNameLabel: "podinfo", fluxHelmNSLabel: "flux-system"},
			wantKind: "HelmRelease", wantNS: "flux-system", wantName: "podinfo", wantGrp: fluxHelmGroup,
		},
		{
			name: "Flux HelmRelease wins over Flux Kustomization",
			labels: map[string]string{
				fluxHelmNameLabel:      "podinfo",
				fluxHelmNSLabel:        "flux-system",
				fluxKustomizeNameLabel: "infra",
				fluxKustomizeNSLabel:   "flux-system",
			},
			wantKind: "HelmRelease", wantName: "podinfo", wantGrp: fluxHelmGroup, wantNS: "flux-system",
		},
		{
			name:     "Flux Kustomization labels",
			labels:   map[string]string{fluxKustomizeNameLabel: "infra", fluxKustomizeNSLabel: "flux-system"},
			wantKind: "Kustomization", wantNS: "flux-system", wantName: "infra", wantGrp: fluxKustomizeGroup,
		},
		{
			name: "Flux labels beat Argo tracking-id",
			labels: map[string]string{
				fluxHelmNameLabel: "podinfo",
				fluxHelmNSLabel:   "flux-system",
			},
			annos:    map[string]string{argoTrackingIDAnnotation: "argocd_other-app:apps/Deployment:default/web"},
			wantKind: "HelmRelease", wantName: "podinfo", wantGrp: fluxHelmGroup, wantNS: "flux-system",
		},
		{
			name:     "Argo tracking-id namespaced form",
			annos:    map[string]string{argoTrackingIDAnnotation: "argocd_my-app:apps/Deployment:default/web"},
			wantKind: "Application", wantNS: "argocd", wantName: "my-app", wantGrp: argoApplicationGroup,
		},
		{
			name:     "Argo tracking-id legacy single-name form yields empty namespace",
			annos:    map[string]string{argoTrackingIDAnnotation: "my-app:apps/Deployment:default/web"},
			wantKind: "Application", wantNS: "", wantName: "my-app", wantGrp: argoApplicationGroup,
		},
		{
			name:     "Argo instance label fallback",
			labels:   map[string]string{argoInstanceLabel: "guestbook"},
			wantKind: "Application", wantNS: "", wantName: "guestbook", wantGrp: argoApplicationGroup,
		},
		{
			name:     "Argo tracking-id beats instance label",
			labels:   map[string]string{argoInstanceLabel: "wrong"},
			annos:    map[string]string{argoTrackingIDAnnotation: "argocd_right:apps/Deployment:default/web"},
			wantKind: "Application", wantNS: "argocd", wantName: "right", wantGrp: argoApplicationGroup,
		},
		{
			name:     "Helm release annotation",
			annos:    map[string]string{helmReleaseNameAnno: "cert-manager", helmReleaseNSAnno: "cert-manager"},
			wantKind: "HelmRelease", wantNS: "cert-manager", wantName: "cert-manager", wantGrp: "",
		},
		{
			// app.kubernetes.io/instance is stamped by every Helm chart and was
			// the legacy false-positive trigger for "Managed by …" chips on
			// plain Helm installs. Frontend dropped it; we don't fall back on it.
			name:   "standard k8s instance label is NOT treated as Argo",
			labels: map[string]string{"app.kubernetes.io/instance": "guestbook-healthy"},
		},
		{
			name: "no signals returns no manager",
		},
		{
			name:   "Flux HelmRelease requires both name and namespace labels",
			labels: map[string]string{fluxHelmNameLabel: "podinfo"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SynthesizeManagedBy(meta(c.labels, c.annos), "Deployment", "default", "web", nil, nil)
			if c.wantKind == "" {
				if got != nil {
					t.Fatalf("want nil ManagedBy, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want 1 manager ref, got %d (%+v)", len(got), got)
			}
			r := got[0]
			if r.Kind != c.wantKind || r.Namespace != c.wantNS || r.Name != c.wantName || r.Group != c.wantGrp {
				t.Errorf("want {Kind:%q Group:%q NS:%q Name:%q}, got %+v",
					c.wantKind, c.wantGrp, c.wantNS, c.wantName, r)
			}
		})
	}
}

// TestSynthesizeManagedBy_TopologyOwnerChain verifies the case-6 fallback:
// when no GitOps/Helm signals are on the resource itself, we walk the
// EdgeManages chain in topology to the topmost ancestor. Pod -> ReplicaSet ->
// Deployment yields the Deployment.
func TestSynthesizeManagedBy_TopologyOwnerChain(t *testing.T) {
	topo := &Topology{
		Nodes: []Node{
			{ID: "deployment/demo/web", Kind: KindDeployment, Name: "web"},
			{ID: "replicaset/demo/web-abc", Kind: KindReplicaSet, Name: "web-abc"},
			{ID: "pod/demo/web-abc-xyz", Kind: KindPod, Name: "web-abc-xyz"},
		},
		Edges: []Edge{
			{ID: "d-rs", Source: "deployment/demo/web", Target: "replicaset/demo/web-abc", Type: EdgeManages},
			{ID: "rs-p", Source: "replicaset/demo/web-abc", Target: "pod/demo/web-abc-xyz", Type: EdgeManages},
		},
	}

	got := SynthesizeManagedBy(nil, "Pod", "demo", "web-abc-xyz", topo, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 manager ref via topology walk, got %d (%+v)", len(got), got)
	}
	if got[0].Kind != "Deployment" || got[0].Name != "web" || got[0].Namespace != "demo" {
		t.Errorf("want topmost owner Deployment/demo/web, got %+v", got[0])
	}
}

// TestSynthesizeManagedBy_LabelsBeatTopologyWalk verifies precedence: a Pod
// carrying an Argo tracking-id annotation surfaces the Application instead
// of the topmost K8s owner from the topology.
func TestSynthesizeManagedBy_LabelsBeatTopologyWalk(t *testing.T) {
	topo := &Topology{
		Nodes: []Node{
			{ID: "deployment/demo/web", Kind: KindDeployment, Name: "web"},
			{ID: "replicaset/demo/web-abc", Kind: KindReplicaSet, Name: "web-abc"},
			{ID: "pod/demo/web-abc-xyz", Kind: KindPod, Name: "web-abc-xyz"},
		},
		Edges: []Edge{
			{ID: "d-rs", Source: "deployment/demo/web", Target: "replicaset/demo/web-abc", Type: EdgeManages},
			{ID: "rs-p", Source: "replicaset/demo/web-abc", Target: "pod/demo/web-abc-xyz", Type: EdgeManages},
		},
	}
	pod := meta(nil, map[string]string{argoTrackingIDAnnotation: "argocd_guestbook:apps/Deployment:demo/web"})

	got := SynthesizeManagedBy(pod, "Pod", "demo", "web-abc-xyz", topo, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 manager ref, got %d (%+v)", len(got), got)
	}
	if got[0].Kind != "Application" || got[0].Name != "guestbook" || got[0].Namespace != "argocd" {
		t.Errorf("want Argo Application/argocd/guestbook, got %+v", got[0])
	}
}

// TestSynthesizeManagedBy_CycleSafe pins behavior on a self-referential
// EdgeManages chain. Without the visited-set guard, walkTopmostOwner would
// loop forever. Such a cycle is impossible from real K8s ownerReferences
// (single controller, parent set before child) but a corrupted topology
// should degrade gracefully rather than hang.
func TestSynthesizeManagedBy_CycleSafe(t *testing.T) {
	topo := &Topology{
		Nodes: []Node{
			{ID: "deployment/demo/a", Kind: KindDeployment, Name: "a"},
			{ID: "deployment/demo/b", Kind: KindDeployment, Name: "b"},
		},
		Edges: []Edge{
			{ID: "a-b", Source: "deployment/demo/a", Target: "deployment/demo/b", Type: EdgeManages},
			{ID: "b-a", Source: "deployment/demo/b", Target: "deployment/demo/a", Type: EdgeManages},
		},
	}
	got := SynthesizeManagedBy(nil, "Deployment", "demo", "a", topo, nil)
	// Either ref is acceptable — the test only asserts termination + a non-nil ref.
	if len(got) != 1 {
		t.Fatalf("want 1 manager ref under cycle, got %d (%+v)", len(got), got)
	}
}

func TestParseArgoTrackingID(t *testing.T) {
	cases := []struct {
		in       string
		ns, name string
		ok       bool
	}{
		{"argocd_my-app:apps/Deployment:default/web", "argocd", "my-app", true},
		{"my-app:apps/Deployment:default/web", "", "my-app", true},
		{"just-garbage-no-colon", "", "", false},
		{"my-ns_:apps/Deployment:default/web", "", "", false},
		{":apps/Deployment:default/web", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			ns, name, ok := parseArgoTrackingID(c.in)
			if ok != c.ok || ns != c.ns || name != c.name {
				t.Errorf("parseArgoTrackingID(%q) = (%q,%q,%v); want (%q,%q,%v)",
					c.in, ns, name, ok, c.ns, c.name, c.ok)
			}
		})
	}
}
