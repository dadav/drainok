package checks

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/kube"
)

// ClusterHealthCheck verifies that at least one other node is Ready and
// schedulable, so evicted pods have somewhere to go at all.
type ClusterHealthCheck struct{}

func (ClusterHealthCheck) Name() string { return "cluster-health" }

func (ClusterHealthCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	for i := range snap.Nodes {
		other := &snap.Nodes[i]
		if other.Name == node.Name {
			continue
		}
		if kube.IsNodeReady(other) && !other.Spec.Unschedulable {
			return nil
		}
	}
	return []Blocker{{
		Check:  "cluster-health",
		Reason: "no other Ready, schedulable node in the cluster",
	}}
}
