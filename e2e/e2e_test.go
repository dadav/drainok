//go:build e2e

// Package e2e drives drainok against a real (kind) cluster. It is compiled
// only under the "e2e" build tag, so `go test ./...` never touches it.
//
// The test deploys one large pause pod per worker node, sized so that no other
// node has room for it. drainok must then report every node as not drainable
// with a "fit" blocker.
//
// Run it with:
//
//	just e2e                 # defaults to context kind-drainok
//	just e2e kind-kind       # against another cluster
//
// or directly:
//
//	DRAINOK_E2E_CONTEXT=kind-drainok go test -tags e2e ./e2e
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	resourcehelpers "k8s.io/component-helpers/resource"

	"github.com/dadav/drainok/internal/analyzer"
	"github.com/dadav/drainok/internal/checks"
	"github.com/dadav/drainok/internal/kube"
)

const (
	e2eNamespace = "drainok-e2e"
	pauseImage   = "registry.k8s.io/pause:3.10"
	// headroomMilli is left free per node so the fat pod is admitted by the
	// kubelet on its own node while still not fitting anywhere else. Must stay
	// below the smallest evictable control-plane pod request (coredns: 100m),
	// otherwise the control-plane node becomes drainable and the test fails.
	headroomMilli = 50
)

// TestNoNodeDrainable is the end-to-end scenario: fat pods make every node's
// eviction impossible to reschedule, so drainok reports no node as drainable.
func TestNoNodeDrainable(t *testing.T) {
	contextName := os.Getenv("DRAINOK_E2E_CONTEXT")
	if contextName == "" {
		t.Skip("DRAINOK_E2E_CONTEXT not set; skipping cluster e2e test")
	}

	client, err := kube.NewClient("", contextName)
	if err != nil {
		t.Fatalf("build kube client for context %q: %v", contextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	createNamespace(t, ctx, client)
	t.Cleanup(func() {
		// Fresh context: the test's ctx may already be cancelled/expired.
		delCtx, delCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer delCancel()
		if err := client.CoreV1().Namespaces().Delete(delCtx, e2eNamespace, metav1.DeleteOptions{}); err != nil {
			t.Logf("cleanup: delete namespace %q: %v", e2eNamespace, err)
		}
	})

	workers := workerNodes(t, ctx, client)
	if len(workers) < 2 {
		t.Fatalf("need at least 2 worker nodes, found %d", len(workers))
	}

	for _, node := range workers {
		free := freeCPUMilli(t, ctx, client, &node)
		requestMilli := free - headroomMilli
		if requestMilli <= 0 {
			t.Fatalf("node %q has no free CPU to size a fat pod (free=%dm)", node.Name, free)
		}
		createFatPod(t, ctx, client, node.Name, requestMilli)
		t.Logf("scheduled fat pod on %q requesting %dm CPU (node free=%dm)", node.Name, requestMilli, free)
	}

	waitPodsRunning(t, ctx, client)

	snap, err := kube.FetchSnapshot(ctx, client)
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}
	results, err := analyzer.Analyze(snap, analyzer.Options{IncludeControlPlane: true})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("analyzer returned no node results")
	}
	for _, result := range results {
		if result.Skipped {
			t.Errorf("node %q was skipped (%s); expected it to be evaluated", result.Node, result.SkipReason)
			continue
		}
		if result.Drainable {
			t.Errorf("node %q reported drainable; expected not drainable", result.Node)
			continue
		}
		if !hasFitBlocker(result.Blockers) {
			t.Errorf("node %q not drainable but has no 'fit' blocker; blockers=%v", result.Node, result.Blockers)
		}
	}
}

func createNamespace(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: e2eNamespace}}
	// Fail on an existing namespace: a leftover from a previous run would carry
	// stale pods and make the result meaningless.
	if _, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace %q (leftover from a previous run?): %v", e2eNamespace, err)
	}
}

// workerNodes returns the schedulable, non-control-plane nodes.
func workerNodes(t *testing.T, ctx context.Context, client kubernetes.Interface) []corev1.Node {
	t.Helper()
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var workers []corev1.Node
	for _, node := range list.Items {
		if isControlPlane(&node) {
			continue
		}
		workers = append(workers, node)
	}
	return workers
}

func isControlPlane(node *corev1.Node) bool {
	if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
		return true
	}
	_, ok := node.Labels["node-role.kubernetes.io/master"]
	return ok
}

// freeCPUMilli is the node's allocatable CPU minus the CPU already requested by
// its non-terminal pods, in milli-cores.
func freeCPUMilli(t *testing.T, ctx context.Context, client kubernetes.Interface, node *corev1.Node) int64 {
	t.Helper()
	allocatable := node.Status.Allocatable.Cpu().MilliValue()

	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + node.Name,
	})
	if err != nil {
		t.Fatalf("list pods on node %q: %v", node.Name, err)
	}
	var used int64
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		// Same request math as the kubelet and drainok's RescheduleCheck:
		// includes init containers, sidecars and pod overhead.
		requests := resourcehelpers.PodRequests(pod, resourcehelpers.PodResourcesOptions{})
		used += requests.Cpu().MilliValue()
	}
	return allocatable - used
}

func createFatPod(t *testing.T, ctx context.Context, client kubernetes.Interface, nodeName string, cpuMilli int64) {
	t.Helper()
	cpu := resource.NewMilliQuantity(cpuMilli, resource.DecimalSI)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fat-" + nodeName,
			Namespace: e2eNamespace,
			Labels:    map[string]string{"app": "drainok-e2e-fat"},
		},
		Spec: corev1.PodSpec{
			// Pin directly to the node: bypasses the scheduler so placement is
			// deterministic regardless of what else the cluster is running.
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: pauseImage,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: *cpu},
				},
			}},
		},
	}
	if _, err := client.CoreV1().Pods(e2eNamespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create fat pod on %q: %v", nodeName, err)
	}
}

// waitPodsRunning blocks until every pod in the e2e namespace is Running.
func waitPodsRunning(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for {
		pods, err := client.CoreV1().Pods(e2eNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("list e2e pods: %v", err)
		}
		allRunning := len(pods.Items) > 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodFailed {
				t.Fatalf("pod %q failed (reason=%s, message=%s); fat pod likely oversized for its node",
					pod.Name, pod.Status.Reason, pod.Status.Message)
			}
			if pod.Status.Phase != corev1.PodRunning {
				allRunning = false
			}
		}
		if allRunning {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("e2e pods did not all reach Running within timeout")
		}
		time.Sleep(2 * time.Second)
	}
}

func hasFitBlocker(blockers []checks.Blocker) bool {
	for _, b := range blockers {
		if b.Check == "fit" {
			return true
		}
	}
	return false
}
