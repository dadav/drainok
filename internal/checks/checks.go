package checks

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/kube"
)

// Blocker describes one reason a node cannot be drained. Check is the blocker
// kind shown to the user ("fit", "constraints", "pdb", ...), Pod is
// "namespace/name" when the blocker is tied to a specific pod.
type Blocker struct {
	Check  string `json:"check"`
	Pod    string `json:"pod,omitempty"`
	Reason string `json:"reason"`
}

// Check evaluates one drainability condition for a single node. Checks are
// pure functions over the snapshot: no API calls, no side effects.
type Check interface {
	Name() string
	Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker
}

// All returns every available check. Order is the evaluation and report order.
func All() []Check {
	return []Check{
		ClusterHealthCheck{},
		RescheduleCheck{},
		PDBCheck{},
		NakedPodsCheck{},
		LocalStorageCheck{},
		SafeToEvictCheck{},
	}
}

// Names returns the names of all available checks, sorted.
func Names() []string {
	var names []string
	for _, c := range All() {
		names = append(names, c.Name())
	}
	sort.Strings(names)
	return names
}

// Filter removes ignored checks from the list. Unknown names are an error so
// typos do not silently disable nothing.
func Filter(all []Check, ignore []string) ([]Check, error) {
	byName := map[string]bool{}
	for _, c := range all {
		byName[c.Name()] = true
	}
	ignored := map[string]bool{}
	for _, name := range ignore {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !byName[name] {
			return nil, fmt.Errorf("unknown check %q (available: %s)", name, strings.Join(Names(), ", "))
		}
		ignored[name] = true
	}
	var result []Check
	for _, c := range all {
		if !ignored[c.Name()] {
			result = append(result, c)
		}
	}
	return result, nil
}

func podKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}
