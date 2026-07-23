package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSafeToEvictFalseBlocks(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("pinned", "worker-1", 100, 128, func(p *corev1.Pod) {
			p.Annotations = map[string]string{SafeToEvictAnnotation: "false"}
		})},
	)
	blockers := SafeToEvictCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "safe-to-evict" {
		t.Fatalf("expected one safe-to-evict blocker, got %v", blockers)
	}
}

func TestSafeToEvictTrueOrUnsetDoesNotBlock(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{
			testPod("plain", "worker-1", 100, 128),
			testPod("allowed", "worker-1", 100, 128, func(p *corev1.Pod) {
				p.Annotations = map[string]string{SafeToEvictAnnotation: "true"}
			}),
		},
	)
	if blockers := (SafeToEvictCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}
