package serviceentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// serviceEntryWithStatusAddrs builds a ServiceEntry with a fixed spec and the
// given auto-allocated VIPs in Status.Addresses.
func serviceEntryWithStatusAddrs(generation int64, statusAddrs ...string) *networkingclient.ServiceEntry {
	se := &networkingclient.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "autogen.server.server",
			Namespace:  "istio-system",
			Generation: generation,
			UID:        "uid-1",
		},
		Spec: networking.ServiceEntry{
			Hosts:      []string{"server.server.mesh.internal"},
			Location:   networking.ServiceEntry_MESH_INTERNAL,
			Resolution: networking.ServiceEntry_STATIC,
			Ports: []*networking.ServicePort{
				{Name: "http", Number: 80, Protocol: "HTTP", TargetPort: 9898},
			},
		},
	}
	for _, addr := range statusAddrs {
		se.Status.Addresses = append(se.Status.Addresses, &networking.ServiceEntryAddress{
			Value: addr,
			Host:  "server.server.mesh.internal",
		})
	}
	return se
}

func backendFor(se *networkingclient.ServiceEntry) interface {
	Equals(any) bool
} {
	be := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil)
	return be.ObjIr
}

// A status-only VIP write must flip backend equality, otherwise the empty
// backend is never reprogrammed with the allocated address.
func TestServiceEntryBackendIR_ReactsToStatusVIP(t *testing.T) {
	beforeVIP := backendFor(serviceEntryWithStatusAddrs(1))
	afterVIP := backendFor(serviceEntryWithStatusAddrs(1, "240.240.0.1"))

	assert.NotNil(t, beforeVIP, "ServiceEntry backend must set ObjIr")
	assert.NotNil(t, afterVIP, "ServiceEntry backend must set ObjIr")

	assert.False(t, beforeVIP.Equals(afterVIP),
		"a ServiceEntry gaining an auto-allocated VIP in its status must NOT be considered equal")
	assert.False(t, afterVIP.Equals(beforeVIP),
		"equality must be symmetric for the status VIP change")
}

// Identical resolved addresses must compare equal so no-op status writes don't
// churn Envoy config.
func TestServiceEntryBackendIR_StableWhenStatusUnchanged(t *testing.T) {
	a := backendFor(serviceEntryWithStatusAddrs(1, "240.240.0.1"))
	b := backendFor(serviceEntryWithStatusAddrs(1, "240.240.0.1"))

	assert.True(t, a.Equals(b), "identical resolved addresses must compare equal")
}

// Adding a second (e.g. IPv6) address must also be detected as a change.
func TestServiceEntryBackendIR_ReactsToAdditionalVIP(t *testing.T) {
	single := backendFor(serviceEntryWithStatusAddrs(1, "240.240.0.1"))
	dual := backendFor(serviceEntryWithStatusAddrs(1, "240.240.0.1", "2001:2::f0f0:1"))

	assert.False(t, single.Equals(dual),
		"gaining an additional auto-allocated address must be detected as a change")
}
