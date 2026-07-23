package checks

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/dadav/drainok/internal/kube"
)

const pdbCheckName = "pdb"

// PDBCheck flags pods whose eviction the API would refuse: pods covered by a
// PodDisruptionBudget that currently allows zero disruptions, and pods covered
// by more than one PDB (the eviction API rejects those outright, see
// https://kubernetes.io/docs/tasks/run-application/configure-pdb).
type PDBCheck struct{}

func (PDBCheck) Name() string { return pdbCheckName }

// pdbMatch is one PodDisruptionBudget covering a pod. invalidSelector marks a
// PDB whose selector could not be parsed; it counts as covering the pod
// because a false blocker is safer than a false "drainable".
type pdbMatch struct {
	name               string
	disruptionsAllowed int32
	invalidSelector    bool
}

func (PDBCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	var blockers []Blocker
	for _, pod := range snap.EvictablePods(node.Name) {
		matches := coveringPDBs(snap, &pod)
		for _, match := range matches {
			switch {
			case match.invalidSelector:
				blockers = append(blockers, Blocker{
					Check:  pdbCheckName,
					Pod:    podKey(&pod),
					Reason: fmt.Sprintf("PodDisruptionBudget %q has an unparseable selector, so its effect on pod %s cannot be determined", match.name, podKey(&pod)),
				})
			case match.disruptionsAllowed == 0:
				blockers = append(blockers, Blocker{
					Check:  pdbCheckName,
					Pod:    podKey(&pod),
					Reason: fmt.Sprintf("pod %s is protected by PodDisruptionBudget %q which allows 0 disruptions", podKey(&pod), match.name),
				})
			}
		}
		if len(matches) > 1 {
			blockers = append(blockers, Blocker{
				Check:  pdbCheckName,
				Pod:    podKey(&pod),
				Reason: fmt.Sprintf("pod %s is covered by multiple PodDisruptionBudgets (%s); the eviction API rejects such pods", podKey(&pod), strings.Join(matchNames(matches), ", ")),
			})
		}
	}
	return blockers
}

// coveringPDBs returns every PodDisruptionBudget in the pod's namespace whose
// selector matches the pod.
func coveringPDBs(snap *kube.ClusterSnapshot, pod *corev1.Pod) []pdbMatch {
	var matches []pdbMatch
	for i := range snap.PDBs {
		pdb := &snap.PDBs[i]
		if pdb.Namespace != pod.Namespace {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			matches = append(matches, pdbMatch{name: pdb.Name, invalidSelector: true})
			continue
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		matches = append(matches, pdbMatch{name: pdb.Name, disruptionsAllowed: pdb.Status.DisruptionsAllowed})
	}
	return matches
}

func matchNames(matches []pdbMatch) []string {
	var names []string
	for _, match := range matches {
		names = append(names, match.name)
	}
	return names
}
