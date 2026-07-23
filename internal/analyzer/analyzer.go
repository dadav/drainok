package analyzer

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/checks"
	"github.com/dadav/drainok/internal/kube"
)

// Options controls which nodes and checks the analysis covers.
type Options struct {
	// NodeNames restricts the analysis to these nodes. Empty means all nodes.
	NodeNames []string
	// IgnoreChecks lists check names to skip.
	IgnoreChecks []string
	// IncludeControlPlane evaluates control-plane nodes instead of skipping them.
	IncludeControlPlane bool
}

// NodeResult is the drainability verdict for one node.
type NodeResult struct {
	Node       string           `json:"node"`
	Drainable  bool             `json:"drainable"`
	Skipped    bool             `json:"skipped,omitempty"`
	SkipReason string           `json:"skipReason,omitempty"`
	Blockers   []checks.Blocker `json:"blockers,omitempty"`
}

// Analyze runs all enabled checks against every selected node.
func Analyze(snap *kube.ClusterSnapshot, opts Options) ([]NodeResult, error) {
	enabled, err := checks.Filter(checks.All(), opts.IgnoreChecks)
	if err != nil {
		return nil, err
	}

	nodes, err := selectNodes(snap, opts.NodeNames)
	if err != nil {
		return nil, err
	}

	var results []NodeResult
	for _, node := range nodes {
		if isControlPlane(node) && !opts.IncludeControlPlane {
			results = append(results, NodeResult{
				Node:       node.Name,
				Skipped:    true,
				SkipReason: "control-plane node (use --include-control-plane to evaluate)",
			})
			continue
		}
		var blockers []checks.Blocker
		for _, check := range enabled {
			blockers = append(blockers, check.Run(snap, node)...)
		}
		results = append(results, NodeResult{
			Node:      node.Name,
			Drainable: len(blockers) == 0,
			Blockers:  blockers,
		})
	}
	return results, nil
}

func selectNodes(snap *kube.ClusterSnapshot, names []string) ([]*corev1.Node, error) {
	var nodes []*corev1.Node
	if len(names) == 0 {
		for i := range snap.Nodes {
			nodes = append(nodes, &snap.Nodes[i])
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
		return nodes, nil
	}
	for _, name := range names {
		node := snap.NodeByName(name)
		if node == nil {
			return nil, fmt.Errorf("node %q not found in the cluster", name)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func isControlPlane(node *corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}
