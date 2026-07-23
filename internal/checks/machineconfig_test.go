package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func withMCOAnnotations(annotations map[string]string) func(*corev1.Node) {
	return func(n *corev1.Node) {
		if n.Annotations == nil {
			n.Annotations = map[string]string{}
		}
		for k, v := range annotations {
			n.Annotations[k] = v
		}
	}
}

func TestMachineConfigDegradedBlocks(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation:  mcoStateDegraded,
		mcoReasonAnnotation: "boom",
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	blockers := MachineConfigCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != machineConfigCheckName {
		t.Fatalf("expected one machine-config blocker, got %v", blockers)
	}
}

func TestMachineConfigUnreconcilableBlocks(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation: "Unreconcilable",
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	blockers := MachineConfigCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != machineConfigCheckName {
		t.Fatalf("expected one machine-config blocker, got %v", blockers)
	}
}

func TestMachineConfigUnknownStateBlocks(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation:         "SomeFutureState",
		mcoCurrentConfigAnnotation: "rendered-worker-aaa",
		mcoDesiredConfigAnnotation: "rendered-worker-aaa",
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	if blockers := (MachineConfigCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 1 {
		t.Fatalf("expected one machine-config blocker, got %v", blockers)
	}
}

func TestMachineConfigWorkingBlocks(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation: mcoStateWorking,
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	if blockers := (MachineConfigCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 1 {
		t.Fatalf("expected one machine-config blocker, got %v", blockers)
	}
}

func TestMachineConfigMismatchBlocks(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation:         "Done",
		mcoCurrentConfigAnnotation: "rendered-worker-aaa",
		mcoDesiredConfigAnnotation: "rendered-worker-bbb",
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	if blockers := (MachineConfigCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 1 {
		t.Fatalf("expected one machine-config blocker, got %v", blockers)
	}
}

func TestMachineConfigDoneDoesNotBlock(t *testing.T) {
	node := testNode("worker-1", 2000, 4096, withMCOAnnotations(map[string]string{
		mcoStateAnnotation:         "Done",
		mcoCurrentConfigAnnotation: "rendered-worker-aaa",
		mcoDesiredConfigAnnotation: "rendered-worker-aaa",
	}))
	snap := testSnapshot([]corev1.Node{node}, nil)
	if blockers := (MachineConfigCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestMachineConfigVanillaNodeIsNoOp(t *testing.T) {
	node := testNode("worker-1", 2000, 4096)
	snap := testSnapshot([]corev1.Node{node}, nil)
	if blockers := (MachineConfigCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers on a non-OpenShift node, got %v", blockers)
	}
}
