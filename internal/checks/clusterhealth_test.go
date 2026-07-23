package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestClusterHealthPassesWithAnotherReadyNode(t *testing.T) {
	snap := testSnapshot([]corev1.Node{
		testNode("worker-1", 2000, 4096),
		testNode("worker-2", 2000, 4096),
	}, nil)
	blockers := ClusterHealthCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestClusterHealthFailsWhenOtherNodesUnavailable(t *testing.T) {
	snap := testSnapshot([]corev1.Node{
		testNode("worker-1", 2000, 4096),
		testNode("worker-2", 2000, 4096, func(n *corev1.Node) { n.Spec.Unschedulable = true }),
		testNode("worker-3", 2000, 4096, func(n *corev1.Node) {
			n.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
		}),
	}, []corev1.Pod{testPod("web", "worker-1", 100, 128)})
	blockers := ClusterHealthCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "cluster-health" {
		t.Fatalf("expected one cluster-health blocker, got %v", blockers)
	}
}

func TestClusterHealthPassesForNodeWithNothingToEvict(t *testing.T) {
	// The only node in the cluster, carrying just a DaemonSet pod: a drain
	// evicts nothing, so there is nowhere for pods to go and that is fine.
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("agent", "worker-1", 100, 128, asDaemonSetPod)},
	)
	if blockers := (ClusterHealthCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}
