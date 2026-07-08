package downward

import (
	"testing"

	"istio.io/api/label"
	corev1 "k8s.io/api/core/v1"
)

// The control plane derives endpoint localities from the same topology label keys
// (pkg/krtcollections.LocalityFromLabels), and zone-aware routing only works when
// both sides agree on them. The production constants are local strings so the
// envoyinit binary does not link k8s.io/api and istio.io/api; this guard pins them
// to the canonical definitions, which the test binary is free to import.
func TestTopologyLabelKeysMatchCanonical(t *testing.T) {
	if labelTopologyRegion != corev1.LabelTopologyRegion {
		t.Errorf("labelTopologyRegion = %q, canonical corev1.LabelTopologyRegion = %q", labelTopologyRegion, corev1.LabelTopologyRegion)
	}
	if labelTopologyZone != corev1.LabelTopologyZone {
		t.Errorf("labelTopologyZone = %q, canonical corev1.LabelTopologyZone = %q", labelTopologyZone, corev1.LabelTopologyZone)
	}
	if labelTopologySubzone != label.TopologySubzone.Name {
		t.Errorf("labelTopologySubzone = %q, canonical label.TopologySubzone.Name = %q", labelTopologySubzone, label.TopologySubzone.Name)
	}
}
