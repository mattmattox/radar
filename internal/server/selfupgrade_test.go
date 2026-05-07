package server

import "testing"

// TestSelfUpgradePatchOptions is a tripwire: it pins the PatchOptions used
// by handleSelfUpgrade. Field manager "helm" + Force=true is what prevents
// the apiserver from recording "radar" as the owner of .image (derived from
// User-Agent when FieldManager is empty), which would break every later
// `helm upgrade` with a server-side-apply conflict. See selfupgrade.go for
// the full rationale.
func TestSelfUpgradePatchOptions(t *testing.T) {
	opts := selfUpgradePatchOptions()
	if opts.FieldManager != "helm" {
		t.Errorf("FieldManager = %q, want %q", opts.FieldManager, "helm")
	}
	if opts.Force == nil {
		t.Fatal("Force is nil, want *true")
	}
	if !*opts.Force {
		t.Errorf("Force = false, want true")
	}
}
