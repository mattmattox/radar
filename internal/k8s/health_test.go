package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestClassifyPodHealth(t *testing.T) {
	now := time.Now()
	oldTime := metav1.NewTime(now.Add(-10 * time.Minute))
	recentTime := metav1.NewTime(now.Add(-1 * time.Minute))

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "healthy running pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true, RestartCount: 0},
					},
				},
			},
			want: "healthy",
		},
		{
			name: "succeeded pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
			want: "healthy",
		},
		{
			name: "failed pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodFailed},
			},
			want: "error",
		},
		{
			name: "CrashLoopBackOff",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
					},
				},
			},
			want: "error",
		},
		{
			name: "OOMKilled",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}},
					},
				},
			},
			want: "error",
		},
		{
			name: "LastTerminationState OOMKilled",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Ready:                true,
							LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
						},
					},
				},
			},
			want: "error",
		},
		{
			name: "init container error",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
					},
				},
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: oldTime},
			},
			want: "error",
		},
		{
			name: "pending over 5 minutes",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: oldTime},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			want: "warning",
		},
		{
			name: "recently pending is healthy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: recentTime},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			want: "healthy",
		},
		{
			name: "recovered: high restart count but now ready and stable is healthy",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true, RestartCount: 10, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-2 * time.Hour))}}},
					},
				},
			},
			want: "healthy",
		},
		{
			name: "actively thrashing: high restarts, not ready, churning now is warning",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Ready:        false,
							RestartCount: 1659,
							State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-30 * time.Minute))}},
							LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
								Reason: "Completed", ExitCode: 0, FinishedAt: metav1.NewTime(now.Add(-30 * time.Second)),
							}},
						},
					},
				},
			},
			want: "warning",
		},
		{
			name: "stale restarts: not ready but last restart was days ago is healthy",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Ready:        false,
							RestartCount: 200,
							State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-72 * time.Hour))}},
							LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
								Reason: "Completed", ExitCode: 0, FinishedAt: metav1.NewTime(now.Add(-72 * time.Hour)),
							}},
						},
					},
				},
			},
			want: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPodHealth(tt.pod, now)
			if got != tt.want {
				t.Errorf("ClassifyPodHealth() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestClassifyPodHealth_StableCrashLoopAcrossPhases is the crashloop-monotonicity
// pin. A crashlooping container's instantaneous State flaps
// Waiting → Running → Terminated → Waiting poll-to-poll, but its stable
// history fields (RestartCount + LastTerminationState) don't. ClassifyPodHealth
// and PodProblemReason must read the stable fields, so {severity, reason} stay
// fixed across the oscillation — otherwise the category-hashed issue_id churns.

func TestClassifyPodHealth_StableCrashLoopAcrossPhases(t *testing.T) {
	now := time.Now()

	// The same crashlooping pod, observed at three successive polls. Only the
	// instantaneous container State differs; RestartCount + LastTerminationState
	// (the stable crash history) are identical across all three.
	crashHistory := corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1},
	}
	mkPod := func(state corev1.ContainerState) *corev1.Pod {
		return &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					RestartCount:         7,
					State:                state,
					LastTerminationState: crashHistory,
				}},
			},
		}
	}

	phases := []struct {
		name  string
		state corev1.ContainerState
	}{
		{"waiting backoff", corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
		{"running (just restarted)", corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now)}}},
		{"waiting backoff again", corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
	}

	const wantHealth = "error"
	const wantReason = "CrashLoopBackOff"
	for _, ph := range phases {
		t.Run(ph.name, func(t *testing.T) {
			pod := mkPod(ph.state)
			if got := ClassifyPodHealth(pod, now); got != wantHealth {
				t.Errorf("ClassifyPodHealth() = %q, want stable %q (phase=%s)", got, wantHealth, ph.name)
			}
			if got := PodProblemReason(pod); got != wantReason {
				t.Errorf("PodProblemReason() = %q, want stable %q (phase=%s)", got, wantReason, ph.name)
			}
		})
	}
}

// TestStableCrashLoop_PreservesSpecificReasons confirms the crashloop
// normalization does NOT clobber more-specific, stable signals. OOMKilled has
// its own category; an active ImagePullBackOff is a distinct startup symptom.

func TestStableCrashLoop_PreservesSpecificReasons(t *testing.T) {
	now := time.Now()

	// A container OOMKilled then backing off must NOT be folded to
	// CrashLoopBackOff — it routes to the OOM category. (isStableCrashLoop
	// excludes OOMKilled, so the override never fires and the OOM signal
	// surfaces from the last-termination walk.)
	oom := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{
			RestartCount:         4,
			State:                corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
		}},
	}}
	if got := PodProblemReason(oom); got != "OOMKilled" {
		t.Errorf("OOMKilled reason = %q, want OOMKilled (must not fold to crashloop)", got)
	}
	if got := ClassifyPodHealth(oom, now); got != "error" {
		t.Errorf("OOMKilled health = %q, want error", got)
	}

	// An active ImagePullBackOff with restart history keeps the image-pull
	// reason — it's a more-specific, stable signal than the generic crashloop.
	imgPull := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{
			RestartCount:         2,
			State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
		}},
	}}
	if got := PodProblemReason(imgPull); got != "ImagePullBackOff" {
		t.Errorf("reason = %q, want ImagePullBackOff (specific reason must win)", got)
	}
}

// TestClassifyPodHealth_RecoveredAfterCrashIsHealthy pins the recovery guard: a
// container that crashed earlier (RestartCount>0 + a crash in
// LastTerminationState — both persist for the life of the container) but has
// since been Running continuously past the kubelet's max CrashLoopBackOff
// backoff (5m) has recovered. Its stale history fields must NOT keep it flagged
// as a crashloop error — otherwise every pod that restarted once at startup
// reads red forever.

func TestClassifyPodHealth_RecoveredAfterCrashIsHealthy(t *testing.T) {
	now := time.Now()
	crash := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}

	recovered := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{
			Ready:                true,
			RestartCount:         2,
			State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-30 * time.Minute))}},
			LastTerminationState: crash,
		}},
	}}
	if got := ClassifyPodHealth(recovered, now); got != "healthy" {
		t.Errorf("recovered-after-crash pod (Running 30m) = %q, want healthy", got)
	}

	// Control: identical crash history but Running only 30s — still inside the
	// loop's backoff window, so it must stay error (the flap-fix is preserved).
	looping := recovered.DeepCopy()
	looping.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.NewTime(now.Add(-30 * time.Second))
	if got := ClassifyPodHealth(looping, now); got != "error" {
		t.Errorf("just-restarted crashloop (Running 30s) = %q, want error", got)
	}

	// An init container that failed once then completed (current state
	// Terminated exit 0) keeps RestartCount>0 + a crash LastTerminationState for
	// the pod's life. With a healthy Running main container the pod is healthy —
	// the clean-completion recovery guard must not let the stale init history
	// paint it red (the common init-waits-on-dependency-then-succeeds case).
	completedInit := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		InitContainerStatuses: []corev1.ContainerStatus{{
			RestartCount:         1,
			State:                corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}},
			LastTerminationState: crash,
		}},
		ContainerStatuses: []corev1.ContainerStatus{{
			Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-10 * time.Minute))}},
		}},
	}}
	if got := ClassifyPodHealth(completedInit, now); got != "healthy" {
		t.Errorf("retried-then-completed init + healthy main = %q, want healthy", got)
	}
}

func TestClassifyNodeHealth(t *testing.T) {
	tests := []struct {
		name              string
		node              *corev1.Node
		wantReady         bool
		wantUnschedulable bool
		wantPressures     int
	}{
		{
			name: "ready node",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
					NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.28.3"},
				},
			},
			wantReady:         true,
			wantUnschedulable: false,
			wantPressures:     0,
		},
		{
			name: "not ready node",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Message: "kubelet stopped"},
					},
				},
			},
			wantReady:         false,
			wantUnschedulable: false,
			wantPressures:     0,
		},
		{
			name: "cordoned and ready",
			node: &corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			wantReady:         true,
			wantUnschedulable: true,
			wantPressures:     0,
		},
		{
			name: "cordoned and not ready",
			node: &corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
			wantReady:         false,
			wantUnschedulable: true,
			wantPressures:     0,
		},
		{
			name: "memory pressure",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
					},
				},
			},
			wantReady:         true,
			wantUnschedulable: false,
			wantPressures:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyNodeHealth(tt.node)
			if got.Ready != tt.wantReady {
				t.Errorf("Ready = %v, want %v", got.Ready, tt.wantReady)
			}
			if got.Unschedulable != tt.wantUnschedulable {
				t.Errorf("Unschedulable = %v, want %v", got.Unschedulable, tt.wantUnschedulable)
			}
			if len(got.Pressures) != tt.wantPressures {
				t.Errorf("Pressures = %v, want %d pressures", got.Pressures, tt.wantPressures)
			}
		})
	}
}

func TestPodProblemReason(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "waiting reason",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
					},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "terminated reason",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}},
					},
				},
			},
			want: "OOMKilled",
		},
		{
			name: "falls back to phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			},
			want: "Pending",
		},
		{
			// Init-container failure: main ContainerStatuses haven't been
			// populated yet (init is blocking) so without the init-status
			// check the reason would fall through to "Pending", masking
			// the real CrashLoopBackOff signal that the agent needs to
			// triage. Pins the init-reason fix.
			name: "init waiting reason wins over phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
					},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "init terminated reason wins over phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "ImagePullBackOff"}}},
					},
				},
			},
			want: "ImagePullBackOff",
		},
		{
			// Init reason wins when both present — init failures are the
			// actual blocker; main containers haven't even started yet.
			name: "init reason wins when both present",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PostStartHookError"}}},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
					},
				},
			},
			want: "PostStartHookError",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PodProblemReason(tt.pod)
			if got != tt.want {
				t.Errorf("PodProblemReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
