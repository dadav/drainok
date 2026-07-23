package checks

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func withVolume(vol corev1.Volume) func(*corev1.Pod) {
	return func(p *corev1.Pod) { p.Spec.Volumes = append(p.Spec.Volumes, vol) }
}

func pinnedPV(name, nodeName string) corev1.PersistentVolume {
	return corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "kubernetes.io/hostname",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{nodeName},
						}},
					}},
				},
			},
		},
	}
}

func TestEmptyDirBlocks(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("cache", "worker-1", 100, 128, withVolume(corev1.Volume{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		}))},
	)
	blockers := LocalStorageCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "local-storage" {
		t.Fatalf("expected one local-storage blocker, got %v", blockers)
	}
}

func TestHostPathBlocks(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096)},
		[]corev1.Pod{testPod("agent", "worker-1", 100, 128, withVolume(corev1.Volume{
			Name:         "host",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}},
		}))},
	)
	blockers := LocalStorageCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "local-storage" {
		t.Fatalf("expected one local-storage blocker, got %v", blockers)
	}
}

func TestPinnedLocalPVBlocks(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{testPod("db", "worker-1", 100, 128, withVolume(corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "db-data"},
			},
		}))},
	)
	snap.PVCs["default/db-data"] = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "db-data", Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "local-pv-1"},
	}
	snap.PVs["local-pv-1"] = pinnedPV("local-pv-1", "worker-1")

	blockers := LocalStorageCheck{}.Run(snap, &snap.Nodes[0])
	if len(blockers) != 1 || blockers[0].Check != "local-storage" {
		t.Fatalf("expected one local-storage blocker, got %v", blockers)
	}
}

func TestNetworkPVDoesNotBlock(t *testing.T) {
	snap := testSnapshot(
		[]corev1.Node{testNode("worker-1", 2000, 4096), testNode("worker-2", 2000, 4096)},
		[]corev1.Pod{testPod("db", "worker-1", 100, 128, withVolume(corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "db-data"},
			},
		}))},
	)
	snap.PVCs["default/db-data"] = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "db-data", Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "net-pv-1"},
	}
	// PV without node affinity, e.g. network storage.
	snap.PVs["net-pv-1"] = corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "net-pv-1"}}

	if blockers := (LocalStorageCheck{}).Run(snap, &snap.Nodes[0]); len(blockers) != 0 {
		t.Fatalf("expected no blockers, got %v", blockers)
	}
}
