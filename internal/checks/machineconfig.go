package checks

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/dadav/drainok/internal/kube"
)

const machineConfigCheckName = "machine-config"

// OpenShift Machine Config Daemon annotations. On vanilla Kubernetes these are
// absent and the check is a silent no-op.
const (
	mcoCurrentConfigAnnotation = "machineconfiguration.openshift.io/currentConfig"
	mcoDesiredConfigAnnotation = "machineconfiguration.openshift.io/desiredConfig"
	mcoStateAnnotation         = "machineconfiguration.openshift.io/state"
	mcoReasonAnnotation        = "machineconfiguration.openshift.io/reason"

	mcoStateDone     = "Done"
	mcoStateWorking  = "Working"
	mcoStateDegraded = "Degraded"
)

// MachineConfigCheck flags OpenShift nodes the Machine Config Operator is
// mid-way through updating or has given up on. Draining such a node collides
// with the MCO, which cordons, drains and reboots the node itself once its
// desiredConfig differs from its currentConfig; a node in a stuck state
// (Degraded, Unreconcilable) is one the daemon has stopped acting on. Any
// state other than Done or Working blocks, so unknown future states err
// toward "not drainable".
type MachineConfigCheck struct{}

func (MachineConfigCheck) Name() string { return machineConfigCheckName }

func (MachineConfigCheck) Run(_ *kube.ClusterSnapshot, node *corev1.Node) []Blocker {
	state := node.Annotations[mcoStateAnnotation]
	current := node.Annotations[mcoCurrentConfigAnnotation]
	desired := node.Annotations[mcoDesiredConfigAnnotation]

	// No MCO annotations at all: not an OpenShift node, nothing to report.
	if state == "" && current == "" && desired == "" {
		return nil
	}

	// Degraded, Unreconcilable, or any state this check does not know: the
	// Machine Config Daemon has flagged the node and stopped acting on it.
	if state != "" && state != mcoStateDone && state != mcoStateWorking {
		reason := node.Annotations[mcoReasonAnnotation]
		if reason == "" {
			reason = "no reason reported"
		}
		return []Blocker{{
			Check:  machineConfigCheckName,
			Reason: fmt.Sprintf("node machine-config state is %s (%s)", state, reason),
		}}
	}

	if state == mcoStateWorking || (current != "" && desired != "" && current != desired) {
		return []Blocker{{
			Check:  machineConfigCheckName,
			Reason: "the Machine Config Operator is applying a machine config update to this node",
		}}
	}

	return nil
}
