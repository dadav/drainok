package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ClusterSnapshot is a one-shot, in-memory copy of everything the checks
// need. Checks operate only on this struct so they stay deterministic and
// unit-testable without a live cluster.
type ClusterSnapshot struct {
	Nodes      []corev1.Node
	PodsByNode map[string][]corev1.Pod
	PDBs       []policyv1.PodDisruptionBudget
	// PVs is keyed by PersistentVolume name.
	PVs map[string]corev1.PersistentVolume
	// PVCs is keyed by "namespace/name".
	PVCs map[string]corev1.PersistentVolumeClaim
}

// FetchSnapshot lists nodes, pods, PDBs, PVCs and PVs across the cluster.
func FetchSnapshot(ctx context.Context, client kubernetes.Interface) (*ClusterSnapshot, error) {
	snap := &ClusterSnapshot{
		PodsByNode: map[string][]corev1.Pod{},
		PVs:        map[string]corev1.PersistentVolume{},
		PVCs:       map[string]corev1.PersistentVolumeClaim{},
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	snap.Nodes = nodes.Items

	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		snap.PodsByNode[pod.Spec.NodeName] = append(snap.PodsByNode[pod.Spec.NodeName], pod)
	}

	pdbs, err := client.PolicyV1().PodDisruptionBudgets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list poddisruptionbudgets: %w", err)
	}
	snap.PDBs = pdbs.Items

	pvcs, err := client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
	}
	for _, pvc := range pvcs.Items {
		snap.PVCs[pvc.Namespace+"/"+pvc.Name] = pvc
	}

	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumes: %w", err)
	}
	for _, pv := range pvs.Items {
		snap.PVs[pv.Name] = pv
	}

	return snap, nil
}

// NodeByName returns the node with the given name, or nil.
func (s *ClusterSnapshot) NodeByName(name string) *corev1.Node {
	for i := range s.Nodes {
		if s.Nodes[i].Name == name {
			return &s.Nodes[i]
		}
	}
	return nil
}

// EvictablePods returns the pods on a node that a drain would actually evict:
// DaemonSet pods, mirror (static) pods and terminal pods are excluded.
func (s *ClusterSnapshot) EvictablePods(nodeName string) []corev1.Pod {
	var result []corev1.Pod
	for _, pod := range s.PodsByNode[nodeName] {
		if IsTerminalPod(&pod) || IsDaemonSetPod(&pod) || IsMirrorPod(&pod) {
			continue
		}
		result = append(result, pod)
	}
	return result
}

// IsDaemonSetPod reports whether the pod is controlled by a DaemonSet.
func IsDaemonSetPod(pod *corev1.Pod) bool {
	ref := metav1.GetControllerOf(pod)
	return ref != nil && ref.Kind == "DaemonSet"
}

// IsMirrorPod reports whether the pod is a static pod's mirror.
func IsMirrorPod(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}

// IsTerminalPod reports whether the pod has finished running.
func IsTerminalPod(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

// IsNodeReady reports whether the node has a Ready=True condition.
func IsNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
