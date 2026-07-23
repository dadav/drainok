package checks

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"

	"github.com/dadav/drainok/internal/kube"
)

// boundPV resolves a PersistentVolumeClaim reference to the PersistentVolume
// it is bound to. Returns nil if the claim or the volume is unknown, or if the
// claim is not bound yet.
func boundPV(snap *kube.ClusterSnapshot, namespace, claimName string) *corev1.PersistentVolume {
	pvc, ok := snap.PVCs[namespace+"/"+claimName]
	if !ok || pvc.Spec.VolumeName == "" {
		return nil
	}
	pv, ok := snap.PVs[pvc.Spec.VolumeName]
	if !ok {
		return nil
	}
	return &pv
}

// pvAllowsNode reports whether the volume can be mounted on the node. A PV
// without required node affinity is assumed to be reachable from anywhere
// (network storage). An unparseable affinity is treated as "not allowed", so
// the caller reports a blocker rather than a false "drainable".
func pvAllowsNode(pv *corev1.PersistentVolume, node *corev1.Node) bool {
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return true
	}
	selector, err := nodeaffinity.NewNodeSelector(pv.Spec.NodeAffinity.Required)
	if err != nil {
		return false
	}
	return selector.Match(node)
}
