package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestNakedPodBlocks(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("loner", "worker-1", 100, 128, withoutOwner)},
	)
	blockers := NakedPodsCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "naked-pods" {
		t.Fatalf("expected one naked-pods blocker, got %v", blockers)
	}
}

func TestControlledPodDoesNotBlock(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	if blockers := (NakedPodsCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}
