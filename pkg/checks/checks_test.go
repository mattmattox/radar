package checks

import "testing"

func eff(kind, group, ns, name, checkID, category, rawSev, msg string) EffectiveFinding {
	return EffectiveFinding{
		Source:            SourceRadarBuiltin,
		Resource:          ResourceRef{ClusterID: "cl_1", Group: group, Kind: kind, Namespace: ns, Name: name},
		CheckID:           checkID,
		Category:          category,
		OriginalSeverity:  rawSev,
		EffectiveSeverity: MapSeverity(rawSev),
		Message:           msg,
		State:             DefaultEffectiveState(),
	}
}

func fixture() []EffectiveFinding {
	return []EffectiveFinding{
		eff("Deployment", "apps", "prod", "api", "no-limits", CategoryReliability, rawWarning, "no resource limits"),
		eff("Deployment", "apps", "prod", "web", "no-limits", CategoryReliability, rawWarning, "no resource limits"),
		eff("Pod", "", "kube-system", "coredns", "run-as-root", CategorySecurity, rawDanger, "runs as root"),
		eff("Pod", "", "prod", "api-xyz", "run-as-root", CategorySecurity, rawDanger, "runs as root"),
	}
}

var catalog = map[string]CheckMeta{
	"no-limits":   {ID: "no-limits", Title: "Set resource limits", Description: "Containers should set CPU/memory limits", Remediation: "Add resources.limits"},
	"run-as-root": {ID: "run-as-root", Title: "Avoid running as root", Description: "Containers should not run as root", Remediation: "Set runAsNonRoot"},
}

func TestMapSeverity(t *testing.T) {
	if MapSeverity(rawDanger) != SeverityHigh {
		t.Errorf("danger should map to high")
	}
	if MapSeverity(rawWarning) != SeverityMedium {
		t.Errorf("warning should map to medium")
	}
	if MapSeverity("nonsense") != SeverityMedium {
		t.Errorf("unknown should fall back to medium")
	}
}

func TestBuildChecks_GroupByCheck(t *testing.T) {
	out := BuildChecks(fixture(), catalog, "cl_1", "prod")
	if len(out) != 2 {
		t.Fatalf("expected 2 checks (one per checkID), got %d", len(out))
	}
	byID := map[string]Check{}
	for _, c := range out {
		byID[c.CheckID] = c
	}

	nl := byID["no-limits"]
	if nl.AffectedResources != 2 || nl.AffectedFindings != 2 {
		t.Errorf("no-limits: affectedResources=%d affectedFindings=%d, want 2/2", nl.AffectedResources, nl.AffectedFindings)
	}
	if nl.Title != "Set resource limits" {
		t.Errorf("title should come from catalog, got %q", nl.Title)
	}
	if nl.EffectiveSeverity != SeverityMedium {
		t.Errorf("no-limits effective severity = %q, want medium", nl.EffectiveSeverity)
	}

	rr := byID["run-as-root"]
	if rr.EffectiveSeverity != SeverityHigh {
		t.Errorf("run-as-root effective severity = %q, want high", rr.EffectiveSeverity)
	}
	if rr.ID != "cl_1|radar_builtin|run-as-root" {
		t.Errorf("check id = %q", rr.ID)
	}
}

func TestBuildChecks_WorstFirstOrder(t *testing.T) {
	out := BuildChecks(fixture(), catalog, "cl_1", "prod")
	// Security/high (run-as-root) should outrank Reliability/medium (no-limits).
	if out[0].CheckID != "run-as-root" {
		t.Errorf("highest-priority check should be run-as-root, got %q", out[0].CheckID)
	}
}

func TestBuildChecks_PriorityFactorsExplainable(t *testing.T) {
	out := BuildChecks(fixture(), catalog, "cl_1", "prod")
	var rr Check
	for _, c := range out {
		if c.CheckID == "run-as-root" {
			rr = c
		}
	}
	keys := map[string]bool{}
	for _, f := range rr.PriorityFactors {
		keys[f.Key] = true
	}
	for _, want := range []string{"severity", "category", "blast_radius", "environment"} {
		if !keys[want] {
			t.Errorf("expected priority factor %q on run-as-root, factors=%+v", want, rr.PriorityFactors)
		}
	}
}

func TestBuildChecks_DedupsResourcesWithinCheck(t *testing.T) {
	// Two findings on the SAME resource for one check → 1 affected resource, 2
	// findings (e.g. a workload failing the check on two containers).
	out := BuildChecks([]EffectiveFinding{
		eff("Deployment", "apps", "prod", "api", "no-limits", CategoryReliability, rawWarning, "container a"),
		eff("Deployment", "apps", "prod", "api", "no-limits", CategoryReliability, rawWarning, "container b"),
	}, catalog, "cl_1", "")
	if len(out) != 1 {
		t.Fatalf("expected 1 check, got %d", len(out))
	}
	if out[0].AffectedResources != 1 {
		t.Errorf("affectedResources = %d, want 1 (same resource)", out[0].AffectedResources)
	}
	if out[0].AffectedFindings != 2 {
		t.Errorf("affectedFindings = %d, want 2", out[0].AffectedFindings)
	}
}
