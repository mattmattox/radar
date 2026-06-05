package server

import (
	"testing"

	"github.com/skyhook-io/radar/pkg/packages"
	"github.com/skyhook-io/radar/pkg/subject"
	"github.com/skyhook-io/radar/pkg/topology"
)

// rawInput builds a workload with no label overlay and its own structural root
// (a singleton, raw-always).
func rawInput(kind, ns, name, version, health string) appWorkloadInput {
	return appWorkloadInput{
		wl:       appWorkload{Kind: kind, Namespace: ns, Name: name, Version: version, Health: health, WorkloadClass: classifyWorkload(kind, nil)},
		rootKey:  ns + "/" + kind + "/" + name,
		rootKind: kind,
	}
}

// overlayInput builds a workload carrying a Tier-2 label overlay (Argo/Flux/Helm
// /part-of), keyed by its own structural root.
func overlayInput(kind, ns, name, version, health string, tier subject.Tier, key string, conf subject.Confidence) appWorkloadInput {
	in := rawInput(kind, ns, name, version, health)
	in.overlay = &subject.AppOverlay{Winner: subject.Signal{Tier: tier, Key: key, Confidence: conf}}
	return in
}

func rowByName(rows []appRow, name string) *appRow {
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i]
		}
	}
	return nil
}

// A label overlay shared by two workloads collapses them into one app; an
// unrelated raw workload stays its own app (raw-always).
func TestGroupApplications_OverlayConsolidationAndRawAlways(t *testing.T) {
	rows := groupApplications([]appWorkloadInput{
		overlayInput("Deployment", "prod", "api", "1.2.0", "healthy", subject.TierPartOf, "prod/app/checkout", subject.ConfidenceMedium),
		overlayInput("Deployment", "prod", "worker", "1.2.0", "healthy", subject.TierPartOf, "prod/app/checkout", subject.ConfidenceMedium),
		rawInput("StatefulSet", "prod", "lonely-db", "15", "healthy"),
	})

	if len(rows) != 2 {
		t.Fatalf("want 2 apps (checkout + lonely-db), got %d: %+v", len(rows), rows)
	}
	checkout := rowByName(rows, "checkout")
	if checkout == nil {
		t.Fatalf("checkout app missing: %+v", rows)
	}
	if len(checkout.Workloads) != 2 {
		t.Errorf("checkout should hold api+worker (2 workloads), got %d", len(checkout.Workloads))
	}
	if checkout.Tier != int(subject.TierPartOf) || checkout.Confidence != string(subject.ConfidenceMedium) {
		t.Errorf("checkout tier/confidence = %d/%s, want %d/%s", checkout.Tier, checkout.Confidence, subject.TierPartOf, subject.ConfidenceMedium)
	}
	lonely := rowByName(rows, "lonely-db")
	if lonely == nil || lonely.Tier != 0 || len(lonely.Workloads) != 1 {
		t.Errorf("lonely-db should be a raw single-workload app at tier 0, got %+v", lonely)
	}
}

// ArgoCD tracking-id mode ("<ns>/Application/<name>") and instance-label mode
// ("/Application/<name>", empty ns) name the same app — they must collapse into
// one row. This is the declaration/workload-collapse fix.
func TestGroupApplications_ArgoTrackingModesCollapse(t *testing.T) {
	rows := groupApplications([]appWorkloadInput{
		overlayInput("Deployment", "prod", "api", "2.0.0", "healthy", subject.TierArgoTrackingID, "argocd/Application/storefront", subject.ConfidenceHigh),
		overlayInput("Deployment", "prod", "cache", "7.2", "healthy", subject.TierArgoInstance, "/Application/storefront", subject.ConfidenceHigh),
	})

	if len(rows) != 1 {
		t.Fatalf("Argo tracking-id + instance modes must collapse to 1 app, got %d: %+v", len(rows), rows)
	}
	if len(rows[0].Workloads) != 2 {
		t.Errorf("collapsed Argo app should hold both workloads, got %d", len(rows[0].Workloads))
	}
	// Tracking-id (tier 3) outranks instance (tier 4) for identity.
	if rows[0].Tier != int(subject.TierArgoTrackingID) {
		t.Errorf("identity tier = %d, want tracking-id %d", rows[0].Tier, subject.TierArgoTrackingID)
	}
}

// An in-cluster GitOps manager (an ArgoCD Application node managing workloads
// via EdgeManages) collapses its workloads even when they carry no label, and
// its kind synthesizes provenance (Argo/Flux tier) for the surface.
func TestGroupApplications_StructuralManagerRoot(t *testing.T) {
	// Two unlabeled Deployments whose structural root is the same Argo App node.
	a := rawInput("Deployment", "prod", "api", "3.1.0", "healthy")
	a.rootKey, a.rootKind = "argocd/Application/billing", "Application"
	b := rawInput("Deployment", "prod", "worker", "3.1.0", "degraded")
	b.rootKey, b.rootKind = "argocd/Application/billing", "Application"

	rows := groupApplications([]appWorkloadInput{a, b})
	if len(rows) != 1 {
		t.Fatalf("workloads under one Argo App must be one app, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Name != "billing" || len(r.Workloads) != 2 {
		t.Errorf("billing app malformed: name=%q workloads=%d", r.Name, len(r.Workloads))
	}
	if r.Tier != int(subject.TierArgoTrackingID) || r.Confidence != string(subject.ConfidenceHigh) {
		t.Errorf("structural Argo root should synthesize Argo tier/high, got %d/%s", r.Tier, r.Confidence)
	}
	if r.Health != "degraded" {
		t.Errorf("app health is worst-of workloads, want degraded got %q", r.Health)
	}
}

// Over-merge guardrail: two distinct apps that share a satellite Service must
// NOT fuse. Satellites are attached, never used to partition.
func TestGroupApplications_SharedSatelliteDoesNotMerge(t *testing.T) {
	a := overlayInput("Deployment", "prod", "api", "1.0", "healthy", subject.TierPartOf, "prod/app/alpha", subject.ConfidenceMedium)
	a.rels = &appRelationships{Services: []string{"shared-gateway"}}
	b := overlayInput("Deployment", "prod", "web", "1.0", "healthy", subject.TierPartOf, "prod/app/beta", subject.ConfidenceMedium)
	b.rels = &appRelationships{Services: []string{"shared-gateway"}}

	rows := groupApplications([]appWorkloadInput{a, b})
	if len(rows) != 2 {
		t.Fatalf("apps sharing only a Service must stay separate, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Relationships == nil || len(r.Relationships.Services) != 1 || r.Relationships.Services[0] != "shared-gateway" {
			t.Errorf("each app should still list the shared service, got %+v", r.Relationships)
		}
	}
}

// structuralRoot must stop AT the in-cluster GitOps manager (Flux
// Kustomization) and NOT climb the EdgeManages edge to the GitRepository source
// that feeds it. The topology builder models GitRepository → Kustomization as
// EdgeManages too, so without the stop-at-manager rule a Flux mono-repo (one
// GitRepository sourcing N Kustomizations) resolves every workload to the same
// GitRepository root and union-find merges all installations into one app.
func TestStructuralRoot_StopsAtManagerNotSource(t *testing.T) {
	node := func(id, kind, ns, name string) topology.Node {
		return topology.Node{ID: id, Kind: topology.NodeKind(kind), Name: name, Data: map[string]any{"namespace": ns}}
	}
	manages := func(src, dst string) topology.Edge {
		return topology.Edge{ID: src + "->" + dst, Source: src, Target: dst, Type: topology.EdgeManages}
	}
	topo := &topology.Topology{
		Nodes: []topology.Node{
			node("gitrepo", "GitRepository", "flux-system", "monorepo"),
			node("ks-apps", "Kustomization", "flux-system", "apps"),
			node("ks-infra", "Kustomization", "flux-system", "infrastructure"),
			node("dep-api", "Deployment", "prod", "api"),
			node("dep-grafana", "Deployment", "monitoring", "grafana"),
		},
		Edges: []topology.Edge{
			manages("gitrepo", "ks-apps"),       // source ref — must NOT be climbed through
			manages("gitrepo", "ks-infra"),      // source ref — must NOT be climbed through
			manages("ks-apps", "dep-api"),       // manager → workload
			manages("ks-infra", "dep-grafana"),  // manager → workload
		},
	}
	g := &appGraph{byID: map[string]topology.Node{}, byKNN: map[string]string{}, topo: topo, idx: topology.IndexByResource(topo)}
	for _, n := range topo.Nodes {
		g.byID[n.ID] = n
		ns, _ := n.Data["namespace"].(string)
		g.byKNN[knnKey(string(n.Kind), ns, n.Name)] = n.ID
	}

	apiRoot, _ := g.rootOf("Deployment", "prod", "api")
	grafanaRoot, _ := g.rootOf("Deployment", "monitoring", "grafana")

	if apiRoot != "flux-system/Kustomization/apps" {
		t.Errorf("api root = %q, want the apps Kustomization (not the GitRepository)", apiRoot)
	}
	if grafanaRoot != "flux-system/Kustomization/infrastructure" {
		t.Errorf("grafana root = %q, want the infrastructure Kustomization (not the GitRepository)", grafanaRoot)
	}
	if apiRoot == grafanaRoot {
		t.Fatalf("two Kustomizations under one GitRepository share root %q — the mono-repo over-merge", apiRoot)
	}
}

// Add-ons are classified with evidence, never dropped (raw-always). A user
// workload named "grafana" still appears — tagged, explained, foldable.
func TestClassifyAddon_ClassifiesNotHides(t *testing.T) {
	addon, why := packages.ClassifyAddon("", "grafana", "", "grafana-0")
	if !addon || why == "" {
		t.Fatalf("grafana should classify as addon with evidence, got addon=%v why=%q", addon, why)
	}

	rows := groupApplications([]appWorkloadInput{
		func() appWorkloadInput {
			in := rawInput("Deployment", "monitoring", "grafana", "10.0", "healthy")
			in.addon, in.addonWhy = packages.ClassifyAddon("", "grafana", "", "grafana")
			return in
		}(),
		rawInput("Deployment", "prod", "my-service", "1.0", "healthy"),
	})
	if len(rows) != 2 {
		t.Fatalf("add-on must remain a row (not dropped), got %d apps", len(rows))
	}
	g := rowByName(rows, "grafana")
	if g == nil || g.Category != "addon" || g.AddonReason == "" {
		t.Errorf("grafana row should be Category=addon with a reason, got %+v", g)
	}
	svc := rowByName(rows, "my-service")
	if svc == nil || svc.Category != "app" {
		t.Errorf("my-service should be Category=app, got %+v", svc)
	}
}

func TestClassifyAddon_MixedEvidenceDoesNotForceAddon(t *testing.T) {
	addon := rawInput("Deployment", "prod", "grafana-sidecar", "10.0", "healthy")
	addon.addon, addon.addonWhy = packages.ClassifyAddon("", "grafana", "", "grafana-sidecar")
	app := rawInput("Deployment", "prod", "api", "1.0", "healthy")
	addon.overlay = &subject.AppOverlay{Winner: subject.Signal{Tier: subject.TierPartOf, Key: "prod/app/checkout", Confidence: subject.ConfidenceMedium}}
	app.overlay = &subject.AppOverlay{Winner: subject.Signal{Tier: subject.TierPartOf, Key: "prod/app/checkout", Confidence: subject.ConfidenceMedium}}

	rows := groupApplications([]appWorkloadInput{addon, app})
	if len(rows) != 1 {
		t.Fatalf("shared overlay should produce one app, got %d: %+v", len(rows), rows)
	}
	if rows[0].Category != "mixed" {
		t.Fatalf("mixed add-on evidence should classify as mixed, got %q", rows[0].Category)
	}
	if rows[0].AddonReason == "" {
		t.Fatalf("mixed classification should preserve add-on evidence")
	}
}

func TestWorkloadClass_FacetIsDerivedFromRuntimeShape(t *testing.T) {
	service := rawInput("Deployment", "prod", "api", "1.0", "healthy")
	service.wl.WorkloadClass = classifyWorkload("Deployment", &appRelationships{Services: []string{"api"}})
	service.rels = &appRelationships{Services: []string{"api"}}
	worker := rawInput("Deployment", "prod", "worker", "1.0", "healthy")
	job := rawInput("CronJob", "prod", "nightly", "", "healthy")

	rows := groupApplications([]appWorkloadInput{service, worker, job})
	if got := rowByName(rows, "api"); got == nil || got.WorkloadClass != "service" {
		t.Fatalf("service row class = %+v, want service", got)
	}
	if got := rowByName(rows, "worker"); got == nil || got.WorkloadClass != "worker" {
		t.Fatalf("worker row class = %+v, want worker", got)
	}
	if got := rowByName(rows, "nightly"); got == nil || got.WorkloadClass != "job" {
		t.Fatalf("cronjob row class = %+v, want job", got)
	}
}
