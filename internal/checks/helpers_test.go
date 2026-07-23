package checks

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dadav/drainok/internal/kube"
)

// testNode builds a Ready, schedulable node with the given allocatable
// capacity and a kubernetes.io/hostname label.
func testNode(name string, cpuMilli, memMi int64, mods ...func(*corev1.Node)) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"kubernetes.io/hostname": name},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(cpuMilli, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(memMi*1024*1024, resource.BinarySI),
				corev1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	for _, mod := range mods {
		mod(&node)
	}
	return node
}

// testPod builds a ReplicaSet-owned running pod with the given requests.
func testPod(name, nodeName string, cpuMilli, memMi int64, mods ...func(*corev1.Pod)) corev1.Pod {
	controller := true
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{"app": name},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: name + "-rs", Controller: &controller},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name: "main",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    *resource.NewMilliQuantity(cpuMilli, resource.DecimalSI),
						corev1.ResourceMemory: *resource.NewQuantity(memMi*1024*1024, resource.BinarySI),
					},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	for _, mod := range mods {
		mod(&pod)
	}
	return pod
}

func withoutOwner(pod *corev1.Pod) {
	pod.OwnerReferences = nil
}

// testSnapshot indexes the given pods by node into a ClusterSnapshot.
func testSnapshot(nodes []corev1.Node, pods []corev1.Pod) *kube.ClusterSnapshot {
	snap := &kube.ClusterSnapshot{
		Nodes:      nodes,
		PodsByNode: map[string][]corev1.Pod{},
		PVs:        map[string]corev1.PersistentVolume{},
		PVCs:       map[string]corev1.PersistentVolumeClaim{},
	}
	for _, pod := range pods {
		snap.PodsByNode[pod.Spec.NodeName] = append(snap.PodsByNode[pod.Spec.NodeName], pod)
	}
	return snap
}
