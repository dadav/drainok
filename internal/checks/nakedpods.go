package checks

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dadav/drainok/internal/kube"
)

// NakedPodsCheck flags pods without a controller: a drain deletes them and
// nothing recreates them elsewhere.
type NakedPodsCheck struct{}

func (NakedPodsCheck) Name() string { return "naked-pods" }

func (NakedPodsCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	var blockers []Blocker
	for _, pod := range snap.EvictablePods(node.Name) {
		if metav1.GetControllerOf(&pod) == nil {
			blockers = append(blockers, Blocker{
				Check:  "naked-pods",
				Pod:    podKey(&pod),
				Reason: fmt.Sprintf("pod %s has no controller and would be deleted, not rescheduled", podKey(&pod)),
			})
		}
	}
	return blockers
}
