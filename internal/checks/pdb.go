package checks

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/dadav/drainok/internal/kube"
)

// PDBCheck flags pods covered by a PodDisruptionBudget that currently allows
// zero disruptions: evicting them would be denied and the drain would hang.
type PDBCheck struct{}

func (PDBCheck) Name() string { return "pdb" }

func (PDBCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	var blockers []Blocker
	for _, pod := range snap.EvictablePods(node.Name) {
		for i := range snap.PDBs {
			pdb := &snap.PDBs[i]
			if pdb.Namespace != pod.Namespace {
				continue
			}
			selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
			if err != nil {
				continue
			}
			if !selector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			if pdb.Status.DisruptionsAllowed == 0 {
				blockers = append(blockers, Blocker{
					Check:  "pdb",
					Pod:    podKey(&pod),
					Reason: fmt.Sprintf("pod %s is protected by PodDisruptionBudget %q which allows 0 disruptions", podKey(&pod), pdb.Name),
				})
			}
		}
	}
	return blockers
}
