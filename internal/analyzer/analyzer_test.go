package analyzer

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dadav/drainok/internal/kube"
)

func testCluster() *kube.ClusterSnapshot {
	makeNode := func(name string, labels map[string]string) corev1.Node {
		if labels == nil {
			labels = map[string]string{}
		}
		labels["kubernetes.io/hostname"] = name
		return corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		}
	}
	nakedPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "loner", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "worker-2"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	return &kube.ClusterSnapshot{
		Nodes: []corev1.Node{
			makeNode("control-plane", map[string]string{"node-role.kubernetes.io/control-plane": ""}),
			makeNode("worker-1", nil),
			makeNode("worker-2", nil),
		},
		PodsByNode: map[string][]corev1.Pod{"worker-2": {nakedPod}},
		PVs:        map[string]corev1.PersistentVolume{},
		PVCs:       map[string]corev1.PersistentVolumeClaim{},
	}
}

func TestAnalyzeSkipsControlPlaneAndFindsBlockers(t *testing.T) {
	results, err := Analyze(testCluster(), Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byNode := map[string]NodeResult{}
	for _, result := range results {
		byNode[result.Node] = result
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !byNode["control-plane"].Skipped {
		t.Error("control-plane should be skipped by default")
	}
	if !byNode["worker-1"].Drainable {
		t.Errorf("worker-1 should be drainable, blockers: %v", byNode["worker-1"].Blockers)
	}
	if byNode["worker-2"].Drainable {
		t.Error("worker-2 hosts a naked pod and should not be drainable")
	}
}

func TestAnalyzeIgnoreChecksTurnsNodeDrainable(t *testing.T) {
	results, err := Analyze(testCluster(), Options{
		NodeNames:    []string{"worker-2"},
		IgnoreChecks: []string{"naked-pods"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Drainable {
		t.Fatalf("worker-2 should be drainable with naked-pods ignored, got %+v", results)
	}
}

func TestAnalyzeUnknownNodeFails(t *testing.T) {
	if _, err := Analyze(testCluster(), Options{NodeNames: []string{"nope"}}); err == nil {
		t.Fatal("expected an error for an unknown node")
	}
}

func TestAnalyzeUnknownCheckFails(t *testing.T) {
	if _, err := Analyze(testCluster(), Options{IgnoreChecks: []string{"nope"}}); err == nil {
		t.Fatal("expected an error for an unknown check")
	}
}

func TestAnalyzeIncludeControlPlane(t *testing.T) {
	results, err := Analyze(testCluster(), Options{IncludeControlPlane: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, result := range results {
		if result.Skipped {
			t.Errorf("no node should be skipped with --include-control-plane, got %+v", result)
		}
	}
}
