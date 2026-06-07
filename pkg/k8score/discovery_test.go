package k8score

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAddAPIResourceRegistersGroupQualifiedCRD(t *testing.T) {
	d := &ResourceDiscovery{
		resourceMap: make(map[string]APIResource),
		gvrMap:      make(map[string]schema.GroupVersionResource),
	}

	d.AddAPIResource(APIResource{
		Group:      "networking.istio.io",
		Version:    "v1",
		Kind:       "VirtualService",
		Name:       "virtualservices",
		Namespaced: true,
		IsCRD:      true,
		Verbs:      []string{"get", "list", "watch"},
	})

	gvr, ok := d.GetGVRWithGroup("VirtualService", "networking.istio.io")
	if !ok {
		t.Fatal("expected group-qualified VirtualService lookup to resolve")
	}
	if gvr != (schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"}) {
		t.Fatalf("GVR = %v, want networking.istio.io/v1 virtualservices", gvr)
	}
	if !d.SupportsWatchGVR(gvr) {
		t.Fatal("expected exact fallback GVR to support watch")
	}
	if got := d.GetKindForGVR(gvr); got != "VirtualService" {
		t.Fatalf("kind = %q, want VirtualService", got)
	}
}

func TestSupportsWatchGVRUsesExactGroupVersionResource(t *testing.T) {
	d := &ResourceDiscovery{
		resourceMap: make(map[string]APIResource),
		gvrMap:      make(map[string]schema.GroupVersionResource),
	}

	d.AddAPIResource(APIResource{
		Group:      "gateway.networking.k8s.io",
		Version:    "v1",
		Kind:       "Gateway",
		Name:       "gateways",
		Namespaced: true,
		IsCRD:      true,
		Verbs:      []string{"get", "list", "watch"},
	})
	d.AddAPIResource(APIResource{
		Group:      "networking.istio.io",
		Version:    "v1",
		Kind:       "Gateway",
		Name:       "gateways",
		Namespaced: true,
		IsCRD:      true,
		Verbs:      []string{"get", "list"},
	})

	gatewayAPI := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	istio := schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "gateways"}
	if !d.SupportsWatchGVR(gatewayAPI) {
		t.Fatal("Gateway API GVR should support watch")
	}
	if d.SupportsWatchGVR(istio) {
		t.Fatal("Istio Gateway GVR should not inherit watch support from Gateway API Gateway")
	}
	if got := d.GetKindForGVR(istio); got != "Gateway" {
		t.Fatalf("kind = %q, want Gateway", got)
	}
}

func TestAddAPIResourceUpdatesExistingGVR(t *testing.T) {
	d := &ResourceDiscovery{
		resourceMap: make(map[string]APIResource),
		gvrMap:      make(map[string]schema.GroupVersionResource),
	}

	resource := APIResource{
		Group:      "keda.sh",
		Version:    "v1alpha1",
		Kind:       "ScaledObject",
		Name:       "scaledobjects",
		Namespaced: true,
		IsCRD:      true,
		Verbs:      []string{"get", "list"},
	}
	d.AddAPIResource(resource)
	resource.Verbs = []string{"get", "list", "watch"}
	d.AddAPIResource(resource)

	resources, err := d.GetAPIResources()
	if err != nil {
		t.Fatalf("GetAPIResources failed: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(resources))
	}
	gvr := schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"}
	if !d.SupportsWatchGVR(gvr) {
		t.Fatal("updated GVR should support watch")
	}
}

func TestSupportsWatchGVRCoreResourceUsesExactEmptyGroup(t *testing.T) {
	d := &ResourceDiscovery{
		resourceMap: make(map[string]APIResource),
		gvrMap:      make(map[string]schema.GroupVersionResource),
	}

	d.AddAPIResource(APIResource{
		Group:      "",
		Version:    "v1",
		Kind:       "Service",
		Name:       "services",
		Namespaced: true,
		IsCRD:      false,
		Verbs:      []string{"get", "list", "watch"},
	})
	d.AddAPIResource(APIResource{
		Group:      "serving.knative.dev",
		Version:    "v1",
		Kind:       "Service",
		Name:       "services",
		Namespaced: true,
		IsCRD:      true,
		Verbs:      []string{"get", "list"},
	})

	core := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	knative := schema.GroupVersionResource{Group: "serving.knative.dev", Version: "v1", Resource: "services"}
	if !d.SupportsWatchGVR(core) {
		t.Fatal("core Service GVR should support watch")
	}
	if d.SupportsWatchGVR(knative) {
		t.Fatal("Knative Service GVR should not inherit watch support from core Service")
	}
}

// Kueue's aggregated visibility API serves a get-only "clusterqueues"
// resource that shadows the real kueue.x-k8s.io plural. Bare-name lookup
// must resolve to the list+watch-capable resource regardless of which one
// discovery enumerates first.
func TestIndexPrefersListWatchCapableResourceAcrossGroups(t *testing.T) {
	d := &ResourceDiscovery{
		resourceMap: map[string]APIResource{},
		gvrMap:      map[string]schema.GroupVersionResource{},
	}

	visibility := APIResource{
		Group: "visibility.kueue.x-k8s.io", Version: "v1beta2",
		Name: "clusterqueues", Kind: "ClusterQueue",
		IsCRD: true, Verbs: []string{"get"},
	}
	real := APIResource{
		Group: "kueue.x-k8s.io", Version: "v1beta2",
		Name: "clusterqueues", Kind: "ClusterQueue",
		IsCRD: true, Verbs: []string{"get", "list", "watch"},
	}

	// Enumeration order 1: shadow first, real second — real must win.
	d.indexResourceLocked(visibility)
	d.indexResourceLocked(real)
	if got := d.gvrMap["clusterqueues"].Group; got != "kueue.x-k8s.io" {
		t.Fatalf("shadow-first: bare plural resolved to %q, want kueue.x-k8s.io", got)
	}

	// Enumeration order 2: real first, shadow second — real must stay.
	d2 := &ResourceDiscovery{
		resourceMap: map[string]APIResource{},
		gvrMap:      map[string]schema.GroupVersionResource{},
	}
	d2.indexResourceLocked(real)
	d2.indexResourceLocked(visibility)
	if got := d2.gvrMap["clusterqueues"].Group; got != "kueue.x-k8s.io" {
		t.Fatalf("real-first: bare plural resolved to %q, want kueue.x-k8s.io", got)
	}
}
