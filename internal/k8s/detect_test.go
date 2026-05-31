package k8s

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// TestDetectProblems_PopulatesGroup pins that every built-in Problem
// emitted by DetectProblems carries the correct canonical API group.
//
// The summary_context issue index keys per-resource counts as
// "group|kind|ns|name" — a Problem with an empty Group collides with
// no real bucket, silently zeroing issueCount for that workload row.
// Pre-fix, all the built-in append-Problem sites omitted the field, so
// every broken Deployment/StatefulSet/DaemonSet/HPA/CronJob/Job
// reported issueCount: 0 in the AI list envelope — a regression
// against the pre-group-aware behavior.
//
// Construct one broken object per built-in kind, drive DetectProblems
// against a fake client, and assert each emitted Problem's Group
// matches the canonical group for its kind.
func TestDetectProblems_PopulatesGroup(t *testing.T) {
	defer ResetTestState()

	oneReplica := int32(1)
	minReplicas := int32(1)
	now := time.Now()
	// Job needs to be older than 1h to surface a "stuck" problem.
	jobStart := metav1.NewTime(now.Add(-2 * time.Hour))

	client := fake.NewClientset(
		// Deployment with unavailable replicas — triggers the
		// "X/Y available" Problem branch.
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec:       appsv1.DeploymentSpec{Replicas: &oneReplica},
			Status: appsv1.DeploymentStatus{
				Replicas:            1,
				UnavailableReplicas: 1,
			},
		},
		// StatefulSet with readyReplicas < replicas.
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &oneReplica},
			Status: appsv1.StatefulSetStatus{
				Replicas:      1,
				ReadyReplicas: 0,
			},
		},
		// DaemonSet with numberUnavailable > 0.
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "logger", Namespace: "prod"},
			Status: appsv1.DaemonSetStatus{
				NumberUnavailable: 2,
			},
		},
		// HPA at its replica ceiling — DetectHPAProblems flags
		// "maxed" when current and desired both hit MaxReplicas.
		// The wrapper sets Group="autoscaling".
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &minReplicas,
				MaxReplicas: 10,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 10,
				DesiredReplicas: 10,
			},
		},
		// Job stuck Active>0 for >1h with no completions.
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "prod", CreationTimestamp: jobStart},
			Status: batchv1.JobStatus{
				Active:    1,
				Succeeded: 0,
				Failed:    0,
			},
		},
	)

	if err := InitTestResourceCache(client); err != nil {
		t.Fatalf("InitTestResourceCache: %v", err)
	}
	cache := GetResourceCache()
	if cache == nil {
		t.Fatal("cache nil after init")
	}

	// Allow informers a brief moment to populate. The fake clientset
	// pre-seeds the store, but the lister types reconstruct via
	// informer events on a separate goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hasAllProblemTypes(DetectProblems(cache, "prod")) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	problems := DetectProblems(cache, "prod")

	wantGroup := map[string]string{
		"Deployment":              "apps",
		"StatefulSet":             "apps",
		"DaemonSet":               "apps",
		"HorizontalPodAutoscaler": "autoscaling",
		"Job":                     "batch",
	}

	got := make(map[string]string, len(problems))
	for _, p := range problems {
		// One Problem per kind is enough for the Group assertion;
		// duplicates (e.g. Deployment Available + ProgressDeadline)
		// must agree on Group so the last-write-wins shape is fine.
		got[p.Kind] = p.Group
	}

	for kind, want := range wantGroup {
		gotGroup, ok := got[kind]
		if !ok {
			t.Errorf("no Problem emitted for %s — fixture wiring broken; got %d problems: %+v", kind, len(problems), problems)
			continue
		}
		if gotGroup != want {
			t.Errorf("%s.Group = %q, want %q (summary_context index keys by group — empty Group zeros issueCount)", kind, gotGroup, want)
		}
	}
}

func hasAllProblemTypes(problems []Detection) bool {
	seen := map[string]bool{}
	for _, p := range problems {
		seen[p.Kind] = true
	}
	return seen["Deployment"] && seen["StatefulSet"] && seen["DaemonSet"] && seen["HorizontalPodAutoscaler"] && seen["Job"]
}

func TestDetectProblems_OperationalSignals(t *testing.T) {
	defer ResetTestState()

	now := time.Now()
	old := metav1.NewTime(now.Add(-10 * time.Minute))
	jobFailedAt := metav1.NewTime(now.Add(-2 * time.Minute))

	client := fake.NewClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashy", Namespace: "prod", CreationTimestamp: old},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready", Namespace: "prod", Labels: map[string]string{"app": "not-ready"}, CreationTimestamp: old},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod", Labels: map[string]string{"app": "api"}, CreationTimestamp: old},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  "app",
				Ports: []corev1.ContainerPort{{Name: "admin", ContainerPort: 9090}},
			}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "prod", CreationTimestamp: old},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "missing"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready", Namespace: "prod", CreationTimestamp: old},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "not-ready"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod", CreationTimestamp: old},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports: []corev1.ServicePort{{
					Port:       80,
					TargetPort: intstr.FromString("http"),
				}},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "prod", CreationTimestamp: old},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimLost},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "prod", CreationTimestamp: old},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{{
					Type:               batchv1.JobFailed,
					Status:             corev1.ConditionTrue,
					Reason:             "BackoffLimitExceeded",
					Message:            "Job has reached the specified backoff limit",
					LastTransitionTime: jobFailedAt,
				}},
			},
		},
	)

	if err := InitTestResourceCache(client); err != nil {
		t.Fatalf("InitTestResourceCache: %v", err)
	}
	cache := GetResourceCache()
	if cache == nil {
		t.Fatal("cache nil after init")
	}

	deadline := time.Now().Add(2 * time.Second)
	var problems []Detection
	for time.Now().Before(deadline) {
		problems = DetectProblems(cache, "prod")
		if hasProblem(problems, "Pod", "crashy", "CrashLoopBackOff") &&
			hasProblem(problems, "Service", "empty", "Selector matches no pods") &&
			hasProblem(problems, "Service", "not-ready", "0/1 selected pods ready") &&
			hasProblem(problems, "Service", "api", "Unresolved named targetPort: http") &&
			hasProblem(problems, "PersistentVolumeClaim", "data", "Lost") &&
			hasProblem(problems, "Job", "migrate", "BackoffLimitExceeded") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	assertProblem(t, problems, "Pod", "crashy", "CrashLoopBackOff", "critical")
	// "Selector matches no pods" is warning, not critical — could be a
	// deliberately scaled-to-zero workload. The "0/N selected pods ready"
	// case below stays critical (workload exists, routing is actually
	// broken).
	assertProblem(t, problems, "Service", "empty", "Selector matches no pods", "warning")
	assertProblem(t, problems, "Service", "not-ready", "0/1 selected pods ready", "critical")
	assertProblem(t, problems, "Service", "api", "Unresolved named targetPort: http", "high")
	assertProblem(t, problems, "PersistentVolumeClaim", "data", "Lost", "critical")
	assertProblem(t, problems, "Job", "migrate", "BackoffLimitExceeded", "critical")
}

func hasProblem(problems []Detection, kind, name, reason string) bool {
	for _, p := range problems {
		if p.Kind == kind && p.Name == name && p.Reason == reason {
			return true
		}
	}
	return false
}

func assertProblem(t *testing.T, problems []Detection, kind, name, reason, severity string) {
	t.Helper()
	for _, p := range problems {
		if p.Kind != kind || p.Name != name || p.Reason != reason {
			continue
		}
		if p.Severity != severity {
			t.Fatalf("%s/%s severity = %q, want %q; problem=%+v", kind, name, p.Severity, severity, p)
		}
		return
	}
	t.Fatalf("missing problem kind=%s name=%s reason=%q; got %+v", kind, name, reason, problems)
}

// TestDetectProblems_SharedRWOVolume pins the multi-replica ReadWriteOnce
// conflict detector: a Deployment wanting >1 replica that mounts an RWO PVC is
// flagged (only one node can attach it), while a single-replica RWO mount and a
// multi-replica ReadWriteMany mount are not.
func TestDetectProblems_SharedRWOVolume(t *testing.T) {
	defer ResetTestState()

	two := int32(2)
	one := int32(1)
	three := int32(3)

	mkDeploy := func(name string, replicas *int32, claim string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: replicas,
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:         "app",
						VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
					}},
					Volumes: []corev1.Volume{{
						Name:         "data",
						VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}},
					}},
				}},
			},
		}
	}
	mkPVC := func(name string, mode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "prod"},
			Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{mode}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, AccessModes: []corev1.PersistentVolumeAccessMode{mode}},
		}
	}

	client := fake.NewClientset(
		mkDeploy("conflict", &two, "rwo-pvc"), // 2 replicas + RWO → flagged
		mkDeploy("single", &one, "rwo-pvc"),   // 1 replica + RWO → fine
		mkDeploy("rwx", &three, "rwx-pvc"),    // 3 replicas + RWX → fine
		mkPVC("rwo-pvc", corev1.ReadWriteOnce),
		mkPVC("rwx-pvc", corev1.ReadWriteMany),
	)
	if err := InitTestResourceCache(client); err != nil {
		t.Fatalf("InitTestResourceCache: %v", err)
	}
	cache := GetResourceCache()

	const reason = "ReadWriteOnce volume shared across replicas"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hasProblem(DetectProblems(cache, "prod"), "Deployment", "conflict", reason) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	problems := DetectProblems(cache, "prod")

	assertProblem(t, problems, "Deployment", "conflict", reason, "high")
	if hasProblem(problems, "Deployment", "single", reason) {
		t.Errorf("single-replica RWO mount should not be flagged: %+v", problems)
	}
	if hasProblem(problems, "Deployment", "rwx", reason) {
		t.Errorf("multi-replica RWX mount should not be flagged: %+v", problems)
	}
}
