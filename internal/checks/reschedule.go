package checks

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	resourcehelpers "k8s.io/component-helpers/resource"
	schedulingcorev1 "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/klog/v2"

	"github.com/dadav/drainok/internal/kube"
)

// RescheduleCheck simulates evicting every pod of the node and placing it on
// another node, using first-fit-decreasing bin-packing over CPU/memory
// requests. A pod blocks the drain if no other Ready, schedulable node
// matches its scheduling constraints ("constraints" blocker) or if the
// matching nodes lack free capacity ("fit" blocker).
//
// Known limitation: required pod anti-affinity is only evaluated against pods
// on the candidate node itself, i.e. hostname-level topology. Zone-scoped
// anti-affinity conflicts with pods on other nodes in the same zone are not
// detected.
type RescheduleCheck struct{}

func (RescheduleCheck) Name() string { return "reschedule" }

// targetNode tracks the remaining capacity of a candidate node during the
// simulation. pods holds real plus simulated pods for anti-affinity checks.
type targetNode struct {
	node     *corev1.Node
	freeCPU  int64 // milliCPU
	freeMem  int64 // bytes
	freePods int64
	pods     []corev1.Pod
}

func (RescheduleCheck) Run(snap *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	targets := buildTargets(snap, node.Name)
	evictable := snap.EvictablePods(node.Name)

	// First-fit-decreasing: place the biggest pods first so the simulation
	// is deterministic and close to a best-case packing.
	sort.SliceStable(evictable, func(i, j int) bool {
		ri := resourcehelpers.PodRequests(&evictable[i], resourcehelpers.PodResourcesOptions{})
		rj := resourcehelpers.PodRequests(&evictable[j], resourcehelpers.PodResourcesOptions{})
		if ri.Cpu().MilliValue() != rj.Cpu().MilliValue() {
			return ri.Cpu().MilliValue() > rj.Cpu().MilliValue()
		}
		return ri.Memory().Value() > rj.Memory().Value()
	})

	var blockers []Blocker
	for i := range evictable {
		pod := &evictable[i]
		requests := resourcehelpers.PodRequests(pod, resourcehelpers.PodResourcesOptions{})
		cpu := requests.Cpu().MilliValue()
		mem := requests.Memory().Value()

		matchingNodes := 0
		placed := false
		for t := range targets {
			target := &targets[t]
			if !podMatchesNode(snap, pod, target) {
				continue
			}
			matchingNodes++
			if target.freeCPU >= cpu && target.freeMem >= mem && target.freePods >= 1 {
				target.freeCPU -= cpu
				target.freeMem -= mem
				target.freePods--
				target.pods = append(target.pods, *pod)
				placed = true
				break
			}
		}
		if placed {
			continue
		}
		if matchingNodes == 0 {
			blockers = append(blockers, Blocker{
				Check:  "constraints",
				Pod:    podKey(pod),
				Reason: fmt.Sprintf("pod %s matches no other node (nodeSelector/affinity/taints/anti-affinity/volume topology)", podKey(pod)),
			})
		} else {
			blockers = append(blockers, Blocker{
				Check:  "fit",
				Pod:    podKey(pod),
				Reason: fmt.Sprintf("pod %s does not fit on any other node (requests %s cpu, %s memory)", podKey(pod), requests.Cpu(), requests.Memory()),
			})
		}
	}
	return blockers
}

// buildTargets returns every other Ready, schedulable node with its free
// capacity: allocatable minus the requests of all non-terminal pods on it.
func buildTargets(snap *kube.ClusterSnapshot, drainedNode string) []targetNode {
	var targets []targetNode
	for i := range snap.Nodes {
		node := &snap.Nodes[i]
		if node.Name == drainedNode || node.Spec.Unschedulable || !kube.IsNodeReady(node) {
			continue
		}
		target := targetNode{
			node:     node,
			freeCPU:  node.Status.Allocatable.Cpu().MilliValue(),
			freeMem:  node.Status.Allocatable.Memory().Value(),
			freePods: node.Status.Allocatable.Pods().Value(),
		}
		for _, pod := range snap.PodsByNode[node.Name] {
			if kube.IsTerminalPod(&pod) {
				continue
			}
			requests := resourcehelpers.PodRequests(&pod, resourcehelpers.PodResourcesOptions{})
			target.freeCPU -= requests.Cpu().MilliValue()
			target.freeMem -= requests.Memory().Value()
			target.freePods--
			target.pods = append(target.pods, pod)
		}
		targets = append(targets, target)
	}
	return targets
}

// podMatchesNode mirrors the scheduler's hard predicates: nodeSelector and
// required node affinity, NoSchedule/NoExecute taints, required pod
// anti-affinity against pods already on the target, and the node affinity of
// the PersistentVolumes the pod is bound to.
func podMatchesNode(snap *kube.ClusterSnapshot, pod *corev1.Pod, target *targetNode) bool {
	match, err := nodeaffinity.GetRequiredNodeAffinity(pod).Match(target.node)
	if err != nil || !match {
		return false
	}
	_, untolerated := schedulingcorev1.FindMatchingUntoleratedTaint(
		klog.Background(),
		target.node.Spec.Taints,
		pod.Spec.Tolerations,
		func(t *corev1.Taint) bool {
			return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
		},
		false,
	)
	if untolerated {
		return false
	}
	if !podVolumesFitNode(snap, pod, target.node) {
		return false
	}
	return antiAffinityAllows(pod, target)
}

// podVolumesFitNode reports whether every PersistentVolume bound to the pod
// can be mounted on the target node. A volume pinned by node affinity (local
// or zonal storage) cannot follow the pod to a node it does not select.
func podVolumesFitNode(snap *kube.ClusterSnapshot, pod *corev1.Pod, node *corev1.Node) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		pv := boundPV(snap, pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
		if pv == nil {
			continue
		}
		if !pvAllowsNode(pv, node) {
			return false
		}
	}
	return true
}

// antiAffinityAllows checks required pod anti-affinity in both directions: the
// incoming pod's own terms against the pods already on the target, and those
// pods' terms against the incoming pod. The scheduler enforces both; only the
// preferred terms of existing pods are ignored.
func antiAffinityAllows(pod *corev1.Pod, target *targetNode) bool {
	for i := range target.pods {
		existing := &target.pods[i]
		if !antiAffinityTermsAllow(pod, existing, target.node) {
			return false
		}
		if !antiAffinityTermsAllow(existing, pod, target.node) {
			return false
		}
	}
	return true
}

// antiAffinityTermsAllow reports whether source's required anti-affinity terms
// tolerate other running alongside it on node.
func antiAffinityTermsAllow(source, other *corev1.Pod, node *corev1.Node) bool {
	if source.Spec.Affinity == nil || source.Spec.Affinity.PodAntiAffinity == nil {
		return true
	}
	for _, term := range source.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
		if _, hasTopologyLabel := node.Labels[term.TopologyKey]; !hasTopologyLabel {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
		if err != nil {
			return false
		}
		if !termCoversNamespace(source, other.Namespace, &term) {
			continue
		}
		if selector.Matches(labels.Set(other.Labels)) {
			return false
		}
	}
	return true
}

// termCoversNamespace reports whether the term applies to pods in namespace.
// NamespaceSelector is treated as matching all namespaces, erring on the side
// of "not drainable".
func termCoversNamespace(pod *corev1.Pod, namespace string, term *corev1.PodAffinityTerm) bool {
	if term.NamespaceSelector != nil {
		return true
	}
	if len(term.Namespaces) == 0 {
		return namespace == pod.Namespace
	}
	for _, ns := range term.Namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}
