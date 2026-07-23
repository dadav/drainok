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
	}, nil)
	blockers := ClusterHealthCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "cluster-health" {
		t.Fatalf("expected one cluster-health blocker, got %v", blockers)
	}
}
