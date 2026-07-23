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

func TestPDBBlocksOnOverlappingBudgets(t *testing.T) {
	// Both PDBs allow a disruption, but the eviction API rejects a pod that
	// two PDBs cover, so the drain would fail anyway.
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	snap.PDBs = []policyv1.PodDisruptionBudget{
		testPDB("web-pdb", map[string]string{"app": "web"}, 1),
		testPDB("catch-all-pdb", nil, 1),
	}

	blockers := PDBCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "pdb" || blockers[0].Pod != "default/web" {
		t.Fatalf("expected one pdb blocker for overlapping budgets, got %v", blockers)
	}
}

func TestPDBBlocksOnUnparseableSelector(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	broken := testPDB("broken-pdb", nil, 1)
	broken.Spec.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      "app",
			Operator: "NotAValidOperator",
			Values:   []string{"web"},
		}},
	}
	snap.PDBs = []policyv1.PodDisruptionBudget{broken}

	blockers := PDBCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "pdb" || blockers[0].Pod != "default/web" {
		t.Fatalf("expected one pdb blocker for the invalid selector, got %v", blockers)
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
