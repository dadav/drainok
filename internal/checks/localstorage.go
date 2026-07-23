package checks

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/kube"
)

const localStorageCheckName = "local-storage"

// LocalStorageCheck flags pods whose data would be lost or which could not be
// rescheduled because their storage is tied to the node: emptyDir, hostPath,
// or PVCs bound to PersistentVolumes pinned to this node via node affinity.
type LocalStorageCheck struct{}

func (LocalStorageCheck) Name() string { return localStorageCheckName }

func (LocalStorageCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	var blockers []Blocker
	for _, pod := range snap.EvictablePods(node.Name) {
		for _, vol := range pod.Spec.Volumes {
			switch {
			case vol.EmptyDir != nil:
				blockers = append(blockers, Blocker{
					Check:  localStorageCheckName,
					Pod:    podKey(&pod),
					Reason: fmt.Sprintf("pod %s uses emptyDir volume %q whose data is lost on drain", podKey(&pod), vol.Name),
				})
			case vol.HostPath != nil:
				blockers = append(blockers, Blocker{
					Check:  localStorageCheckName,
					Pod:    podKey(&pod),
					Reason: fmt.Sprintf("pod %s uses hostPath volume %q tied to this node", podKey(&pod), vol.Name),
				})
			case vol.PersistentVolumeClaim != nil:
				pvName := boundPVPinnedToNode(snap, &pod, vol.PersistentVolumeClaim.ClaimName, node)
				if pvName != "" {
					blockers = append(blockers, Blocker{
						Check:  localStorageCheckName,
						Pod:    podKey(&pod),
						Reason: fmt.Sprintf("pod %s uses PersistentVolume %q which is pinned to this node", podKey(&pod), pvName),
					})
				}
			}
		}
	}
	return blockers
}

// boundPVPinnedToNode returns the PV name if the claim is bound to a
// PersistentVolume whose node affinity matches this node but no other Ready,
// schedulable node in the cluster. Returns "" otherwise. Nodes that are
// NotReady or cordoned do not count as alternative homes for the volume,
// mirroring the target selection in buildTargets.
func boundPVPinnedToNode(snap *kube.ClusterSnapshot, pod *corev1.Pod, claimName string, node *corev1.Node) string {
	pv := boundPV(snap, pod.Namespace, claimName)
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return ""
	}
	for i := range snap.Nodes {
		other := &snap.Nodes[i]
		if other.Name == node.Name || other.Spec.Unschedulable || !kube.IsNodeReady(other) {
			continue
		}
		if pvAllowsNode(pv, other) {
			return ""
		}
	}
	return pv.Name
}
