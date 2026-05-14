package k8s

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/skyhook-io/radar/pkg/k8score"
)

// dynamicCapabilityKinds lists ResourcePermissions fields that are surfaced
// via the dynamic-cache path rather than a typed informer in pkg/k8score.
// Each entry must have a matching candidate in supportedCRDFallbacks.
//
// These kinds are also "optional" from a UI perspective: a false bool means
// "CRD not installed", not "RBAC denied something Radar expected". When
// changing this list, update OPTIONAL_RESOURCE_KINDS in
// packages/k8s-ui/src/types/core.ts so the frontend keeps filtering them
// out of the limited-access banner.
//
// Keep this list small and explicit — if you find yourself adding many
// entries, the typed/dynamic boundary in this codebase has shifted and the
// design needs revisiting, not the allowlist.
var dynamicCapabilityKinds = map[string]bool{
	"gateways":               true,
	"httproutes":             true,
	"verticalpodautoscalers": true,
}

// TestCapabilitiesAlignment_AllFieldsProbed asserts every ResourcePermissions
// field has a matching probe entry. Catches the "added a struct field, forgot
// the probe" failure mode.
func TestCapabilitiesAlignment_AllFieldsProbed(t *testing.T) {
	perms := &ResourcePermissions{}
	probes := resourceProbeTargets(perms)

	// Build a set of field *bool addresses the probes write into.
	probeFields := make(map[uintptr]string, len(probes))
	for _, p := range probes {
		probeFields[reflect.ValueOf(p.field).Pointer()] = p.key
	}

	permsVal := reflect.ValueOf(perms).Elem()
	permsType := permsVal.Type()
	for i := 0; i < permsType.NumField(); i++ {
		field := permsType.Field(i)
		addr := permsVal.Field(i).Addr().Pointer()
		if _, ok := probeFields[addr]; !ok {
			t.Errorf("ResourcePermissions.%s (json:%q) has no probe entry in resourceProbeTargets — "+
				"adding a field requires a matching probe so the bool gets populated.",
				field.Name, field.Tag.Get("json"))
		}
	}
}

// TestCapabilitiesAlignment_TypedVsDynamic asserts that every probe key
// either has a typed informer in pkg/k8score OR is in the explicit
// dynamicCapabilityKinds allowlist with a matching supportedCRDFallbacks
// entry. Catches "added a probe but no way to actually surface data".
func TestCapabilitiesAlignment_TypedVsDynamic(t *testing.T) {
	probes := resourceProbeTargets(&ResourcePermissions{})

	typedKeys := make(map[string]bool, len(probes))
	for _, k := range k8score.InformerResourceKeys() {
		typedKeys[k] = true
	}

	dynamicByGVR := make(map[schema.GroupVersionResource]bool, len(supportedCRDFallbacks))
	for _, c := range supportedCRDFallbacks {
		for _, v := range c.Versions {
			dynamicByGVR[schema.GroupVersionResource{Group: c.Group, Version: v, Resource: c.Resource}] = true
		}
	}

	for _, p := range probes {
		if dynamicCapabilityKinds[p.key] {
			if !p.requiresDiscovery {
				t.Errorf("probe %q is in dynamicCapabilityKinds but missing requiresDiscovery: true — "+
					"dynamic CRDs must gate on IsNotFound or capabilities will lie when the CRD isn't installed.", p.key)
			}
			if !dynamicByGVR[p.gvr] {
				t.Errorf("probe %q (dynamic) has GVR %v with no matching supportedCRDFallbacks entry — "+
					"the dynamic cache won't serve this kind even if discovery sees it.", p.key, p.gvr)
			}
			continue
		}
		if !typedKeys[p.key] {
			t.Errorf("probe %q has no typed informer in pkg/k8score and isn't in dynamicCapabilityKinds — "+
				"either add a typed informer in pkg/k8score.buildInformerSetups or add %q to dynamicCapabilityKinds (and supportedCRDFallbacks).",
				p.key, p.key)
		}
		if p.requiresDiscovery {
			t.Errorf("probe %q is a typed informer but has requiresDiscovery: true — "+
				"only dynamic CRDs need the IsNotFound gate.", p.key)
		}
	}
}

// TestCapabilitiesAlignment_JSONTagMatchesProbeKey asserts the JSON tag,
// lowercased, equals the probe key. This is the actual contract — a future
// field that breaks it (e.g. tag "snake_case") will fail this test.
//
// Without this, a typed field could get a JSON name that frontend consumers
// expect to look up (via key lowercasing) while the probe writes to a
// different map slot — silent miss.
func TestCapabilitiesAlignment_JSONTagMatchesProbeKey(t *testing.T) {
	perms := &ResourcePermissions{}
	probes := resourceProbeTargets(perms)

	keyByField := make(map[uintptr]string, len(probes))
	for _, p := range probes {
		keyByField[reflect.ValueOf(p.field).Pointer()] = p.key
	}

	permsVal := reflect.ValueOf(perms).Elem()
	permsType := permsVal.Type()
	for i := 0; i < permsType.NumField(); i++ {
		field := permsType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			t.Errorf("ResourcePermissions.%s has no json tag", field.Name)
			continue
		}
		expectedKey := strings.ToLower(jsonTag)
		actualKey, ok := keyByField[permsVal.Field(i).Addr().Pointer()]
		if !ok {
			continue // Reported by TestCapabilitiesAlignment_AllFieldsProbed.
		}
		if actualKey != expectedKey {
			t.Errorf("ResourcePermissions.%s json:%q lowercases to %q but probe key is %q — "+
				"these must match so reflection lookups (informer enabled map, dynamic cache, etc.) work.",
				field.Name, jsonTag, expectedKey, actualKey)
		}
	}
}

// TestCapabilitiesAlignment_FullyAllowedProbeSetsEveryField is the end-to-end
// smoke: run a fully-allowed probe and assert every ResourcePermissions
// field comes back true. If any field is unreachable from the probe pass,
// this fails — the catch-all behind invariants A/B/C.
func TestCapabilitiesAlignment_FullyAllowedProbeSetsEveryField(t *testing.T) {
	dyn := fakeDyn(t, func(_ schema.GroupVersionResource, _ string) bool { return true })

	result, hadErrors := probeResourceAccess(context.Background(), dyn, "", false)
	if hadErrors {
		t.Fatalf("hadErrors should be false on a fully-allowed run")
	}

	permsVal := reflect.ValueOf(result.Perms).Elem()
	permsType := permsVal.Type()
	for i := 0; i < permsType.NumField(); i++ {
		field := permsType.Field(i)
		if !permsVal.Field(i).Bool() {
			t.Errorf("ResourcePermissions.%s should be true after a fully-allowed probe pass — "+
				"the probe doesn't reach this field. Check resourceProbeTargets and the field address mapping.",
				field.Name)
		}
	}
}

// TestApplyDiscoveryGate_DeniesOnNotFoundForDynamic asserts the discovery
// gate denies a dynamic probe when the API returns NotFound (CRD not
// installed), and clears the transient so it doesn't shorten the cache TTL.
// Typed probes (requiresDiscovery=false) are unaffected.
func TestApplyDiscoveryGate_DeniesOnNotFoundForDynamic(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gateways"}, "")

	// Dynamic + NotFound → denied, transient cleared.
	allowed, transient := applyDiscoveryGate(true, notFound, true)
	if allowed {
		t.Errorf("dynamic probe with NotFound should be denied, got allowed=true")
	}
	if transient != nil {
		t.Errorf("dynamic probe with NotFound should clear transient (it's expected, not an API hiccup), got %v", transient)
	}

	// Typed + NotFound → optimistic-allow preserved (typed GVRs always exist;
	// if we ever see NotFound from one, it's a transient API hiccup).
	allowed, transient = applyDiscoveryGate(true, notFound, false)
	if !allowed {
		t.Errorf("typed probe with NotFound should preserve optimistic-allow, got allowed=false")
	}
	if transient != notFound {
		t.Errorf("typed probe should preserve the transient error for TTL shortening")
	}

	// Dynamic + non-NotFound transient (a server error or arbitrary
	// network failure) → optimistic-allow preserved. We only gate on the
	// specific NotFound case because that's "CRD not installed"; everything
	// else is a real hiccup the reflector will retry.
	other := &apierrors.StatusError{ErrStatus: metav1.Status{Code: 503, Reason: metav1.StatusReasonServiceUnavailable}}
	allowed, transient = applyDiscoveryGate(true, other, true)
	if !allowed {
		t.Errorf("dynamic probe with non-NotFound transient should preserve optimistic-allow, got allowed=false")
	}
	if !errors.Is(transient, other) && transient != error(other) {
		t.Errorf("dynamic probe with non-NotFound transient should preserve the error, got %v", transient)
	}
}
