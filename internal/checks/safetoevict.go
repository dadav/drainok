package checks

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/kube"
)

// SafeToEvictAnnotation is the cluster-autoscaler convention for pods that
// must not be evicted.
const SafeToEvictAnnotation = "cluster-autoscaler.kubernetes.io/safe-to-evict"

// SafeToEvictCheck flags pods explicitly annotated as not safe to evict.
type SafeToEvictCheck struct{}

func (SafeToEvictCheck) Name() string { return "safe-to-evict" }

func (SafeToEvictCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	var blockers []Blocker
	for _, pod := range snap.EvictablePods(node.Name) {
		if pod.Annotations[SafeToEvictAnnotation] == "false" {
			blockers = append(blockers, Blocker{
				Check:  "safe-to-evict",
				Pod:    podKey(&pod),
				Reason: fmt.Sprintf("pod %s is annotated %s=false", podKey(&pod), SafeToEvictAnnotation),
			})
		}
	}
	return blockers
}
