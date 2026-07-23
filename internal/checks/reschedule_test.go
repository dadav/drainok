package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dadav/drainok/internal/kube"
)

func TestRescheduleFitsSmallPod(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{testPod("web", "worker-1", 500, 512)},
	)
	if blockers := (RescheduleCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestRescheduleReportsFitBlockerForOversizedPod(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 4000, 8192), testNode("worker-2", 1000, 1024)},
		[]corev1.Pod{testPod("big", "worker-1", 3000, 4096)},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "fit" {
		t.Fatalf("expected one fit blocker, got %v", blockers)
	}
}

func TestRescheduleAccountsForExistingPodsOnTarget(t *testing.T) {
	// worker-2 has 2000m allocatable but 1500m already requested, so the
	// displaced 1000m pod must not fit.
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{
			testPod("moving", "worker-1", 1000, 512),
			testPod("resident", "worker-2", 1500, 512),
		},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "fit" {
		t.Fatalf("expected one fit blocker, got %v", blockers)
	}
}

func TestRescheduleBinPacksAcrossMultipleTargets(t *testing.T) {
	// Two 800m pods cannot share one 1000m node but can spread over two.
	snap := testSnapshot(
		[]corev1.Node{
			testNode("worker-1", 2000, 4096),
			testNode("worker-2", 1000, 4096),
			testNode("worker-3", 1000, 4096),
		},
		[]corev1.Pod{
			testPod("app-a", "worker-1", 800, 256),
			testPod("app-b", "worker-1", 800, 256),
		},
	)
	if blockers := (RescheduleCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestRescheduleReportsConstraintsBlockerForNodeSelector(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{
			testNode("worker-1", 2000, 4096, func(n *corev1.Node) { n.Labels["disk"] = "ssd" }),
			testNode("worker-2", 2000, 4096),
		},
		[]corev1.Pod{testPod("picky", "worker-1", 100, 128, func(p *corev1.Pod) {
			p.Spec.NodeSelector = map[string]string{"disk": "ssd"}
		})},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "constraints" {
		t.Fatalf("expected one constraints blocker, got %v", blockers)
	}
}

func TestRescheduleReportsConstraintsBlockerForUntoleratedTaint(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{
			testNode("worker-1", 2000, 4096),
			testNode("worker-2", 2000, 4096, func(n *corev1.Node) {
				n.Spec.Taints = []corev1.Taint{{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule}}
			}),
		},
		[]corev1.Pod{testPod("web", "worker-1", 100, 128)},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "constraints" {
		t.Fatalf("expected one constraints blocker, got %v", blockers)
	}
}

func TestRescheduleReportsConstraintsBlockerForAntiAffinity(t *testing.T) {
	asWebApp := func(p *corev1.Pod) { p.Labels["app"] = "web" }
	withAntiAffinity := func(p *corev1.Pod) {
		p.Spec.Affinity = &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
					TopologyKey:   "kubernetes.io/hostname",
					LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				}},
			},
		}
	}
	// A replica of "web" already runs on worker-2, so the displaced pod with
	// hostname anti-affinity against app=web cannot land there.
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{
			testPod("web-1", "worker-1", 100, 128, asWebApp, withAntiAffinity),
			testPod("web-2", "worker-2", 100, 128, asWebApp),
		},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "constraints" {
		t.Fatalf("expected one constraints blocker, got %v", blockers)
	}
}

func TestRescheduleHonoursExistingPodAntiAffinity(t *testing.T) {
	// The displaced pod has no anti-affinity of its own, but the pod already
	// on worker-2 refuses to share a host with app=web. The scheduler enforces
	// required anti-affinity in both directions.
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{
			testPod("web-1", "worker-1", 100, 128, func(p *corev1.Pod) { p.Labels["app"] = "web" }),
			testPod("picky", "worker-2", 100, 128, func(p *corev1.Pod) {
				p.Spec.Affinity = &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
							TopologyKey:   "kubernetes.io/hostname",
							LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
						}},
					},
				}
			}),
		},
	)
	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "constraints" {
		t.Fatalf("expected one constraints blocker, got %v", blockers)
	}
}

func TestReschedulePlacesPodOnNodeMatchingItsVolume(t *testing.T) {
	// The pod's PV is restricted to zone-a, and worker-3 is in that zone.
	snap := testSnapshot(
		[]corev1.Node{
			testNode("worker-1", 2000, 4096, inZone("zone-a")),
			testNode("worker-2", 2000, 4096, inZone("zone-b")),
			testNode("worker-3", 2000, 4096, inZone("zone-a")),
		},
		[]corev1.Pod{testPod("db", "worker-1", 100, 128, withVolume(pvcVolume("data", "db-data")))},
	)
	bindClaim(snap, "db-data", zonalPV("zonal-pv-1", "zone-a"))

	if blockers := (RescheduleCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}

func TestRescheduleBlocksWhenNoNodeMatchesVolumeTopology(t *testing.T) {
	// Plenty of free capacity on worker-2, but it is in the wrong zone, so
	// the pod's volume cannot follow it there.
	snap := testSnapshot(
		[]corev1.Node{
			testNode("worker-1", 2000, 4096, inZone("zone-a")),
			testNode("worker-2", 2000, 4096, inZone("zone-b")),
		},
		[]corev1.Pod{testPod("db", "worker-1", 100, 128, withVolume(pvcVolume("data", "db-data")))},
	)
	bindClaim(snap, "db-data", zonalPV("zonal-pv-1", "zone-a"))

	blockers := RescheduleCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "constraints" {
		t.Fatalf("expected one constraints blocker, got %v", blockers)
	}
}

func inZone(zone string) func(*corev1.Node) {
	return func(n *corev1.Node) { n.Labels["topology.kubernetes.io/zone"] = zone }
}

func pvcVolume(name, claimName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
		},
	}
}

// bindClaim registers a bound PVC/PV pair in the default namespace.
func bindClaim(snap *kube.ClusterSnapshot, claimName string, pv corev1.PersistentVolume) {
	snap.PVCs["default/"+claimName] = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pv.Name},
	}
	snap.PVs[pv.Name] = pv
}

func zonalPV(name, zone string) corev1.PersistentVolume {
	return corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "topology.kubernetes.io/zone",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{zone},
						}},
					}},
				},
			},
		},
	}
}
