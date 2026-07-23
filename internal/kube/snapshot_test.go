package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFetchSnapshot(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "worker-1"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
		},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "web-pdb", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}},
	)

	snap, err := FetchSnapshot(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snap.Nodes) != 1 || snap.Nodes[0].Name != "worker-1" {
		t.Errorf("unexpected nodes: %v", snap.Nodes)
	}
	if len(snap.PodsByNode["worker-1"]) != 1 {
		t.Errorf("expected 1 pod on worker-1, got %d", len(snap.PodsByNode["worker-1"]))
	}
	if len(snap.PDBs) != 1 {
		t.Errorf("expected 1 pdb, got %d", len(snap.PDBs))
	}
	if _, ok := snap.PVCs["default/data"]; !ok {
		t.Error("expected pvc default/data in snapshot")
	}
	if _, ok := snap.PVs["pv-1"]; !ok {
		t.Error("expected pv pv-1 in snapshot")
	}
}

func TestEvictablePodsFiltering(t *testing.T) {
	controller := true
	snap := &ClusterSnapshot{
		PodsByNode: map[string][]corev1.Pod{
			"worker-1": {
				{
					ObjectMeta: metav1.ObjectMeta{Name: "normal", Namespace: "default"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ds-pod", Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds", Controller: &controller}},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-pod", Namespace: "kube-system",
						Annotations: map[string]string{corev1.MirrorPodAnnotationKey: "mirror"},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "default"},
					Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
				},
			},
		},
	}
	evictable := snap.EvictablePods("worker-1")
	if len(evictable) != 1 || evictable[0].Name != "normal" {
		t.Fatalf("expected only the normal pod to be evictable, got %v", evictable)
	}
}
