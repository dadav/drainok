package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testPDB(name string, matchLabels map[string]string, disruptionsAllowed int32) policyv1.PodDisruptionBudget {
	return policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: matchLabels},
		},
		Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: disruptionsAllowed},
	}
}

func TestPDBBlocksAtZeroDisruptions(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	snap.PDBs = []policyv1.PodDisruptionBudget{testPDB("web-pdb", map[string]string{"app": "web"}, 0)}

	blockers := PDBCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "pdb" || blockers[0].Pod != "default/web" {
		t.Fatalf("expected one pdb blocker for default/web, got %v", blockers)
	}
}

func TestPDBAllowsWithRemainingDisruptions(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	snap.PDBs = []policyv1.PodDisruptionBudget{testPDB("web-pdb", map[string]string{"app": "web"}, 1)}

	if blockers := (PDBCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestPDBIgnoresNonMatchingSelector(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	snap.PDBs = []policyv1.PodDisruptionBudget{testPDB("other-pdb", map[string]string{"app": "other"}, 0)}

	if blockers := (PDBCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}
