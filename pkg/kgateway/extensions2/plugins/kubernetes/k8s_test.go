package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// serviceWithClusterIPs builds a Service with a fixed spec and the given VIPs.
// Generation is left at 0 throughout: core Services never bump it, which is
// precisely why spec changes need a compared IR field to be noticed.
func serviceWithClusterIPs(resourceVersion string, clusterIPs ...string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "svc",
			Namespace:       "ns",
			UID:             "svc-uid",
			ResourceVersion: resourceVersion,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	if len(clusterIPs) > 0 {
		svc.Spec.ClusterIP = clusterIPs[0]
		svc.Spec.ClusterIPs = clusterIPs
	}
	return svc
}

// A Service converted single-stack -> dual-stack gains a clusterIP without
// bumping generation, and the base cluster it translates to is byte-identical
// because base translation emits EDS and never reads the VIPs. Only the
// waypoint overlay does, by inlining them into a STATIC cluster. If the pair
// compares equal, KRT keeps the old row and those clients keep the stale
// single-stack endpoint indefinitely.
func TestServiceBackendIR_ReactsToAdditionalClusterIP(t *testing.T) {
	singleStack := BuildServiceBackendObjectIR(serviceWithClusterIPs("1", "10.0.0.1"), 80, "HTTP")
	dualStack := BuildServiceBackendObjectIR(serviceWithClusterIPs("2", "10.0.0.1", "2001:2::f0f0:1"), 80, "HTTP")

	assert.NotNil(t, singleStack.ObjIr, "Service backend must set ObjIr")

	assert.False(t, singleStack.EqualsIgnoringResourceVersion(dualStack),
		"a Service gaining a dual-stack clusterIP must not compare equal")
	assert.False(t, dualStack.EqualsIgnoringResourceVersion(singleStack),
		"equality must be symmetric for the clusterIPs change")
}

// The negative control for the optimization EqualsIgnoringResourceVersion
// exists for: a write that touches only resourceVersion must still compare
// equal, or every Service status write fans out to every client again.
func TestServiceBackendIR_StableWhenOnlyResourceVersionMoves(t *testing.T) {
	before := BuildServiceBackendObjectIR(serviceWithClusterIPs("1", "10.0.0.1"), 80, "HTTP")
	after := BuildServiceBackendObjectIR(serviceWithClusterIPs("2", "10.0.0.1"), 80, "HTTP")

	assert.True(t, before.EqualsIgnoringResourceVersion(after),
		"a resourceVersion-only write must not reach clients")
	assert.False(t, before.Equals(after),
		"plain Equals still compares resourceVersion for generation-less kinds")
}

// A headless Service resolves to no addresses; two of them must compare equal
// rather than tripping on a nil-vs-empty slice difference.
func TestServiceBackendIR_StableWithoutClusterIPs(t *testing.T) {
	a := BuildServiceBackendObjectIR(serviceWithClusterIPs("1"), 80, "HTTP")
	b := BuildServiceBackendObjectIR(serviceWithClusterIPs("2"), 80, "HTTP")

	assert.True(t, a.EqualsIgnoringResourceVersion(b),
		"two address-less Services must compare equal")
}
