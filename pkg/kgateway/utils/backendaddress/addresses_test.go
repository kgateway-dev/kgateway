package backendaddress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestServiceAddresses(t *testing.T) {
	svc := func(clusterIP string, clusterIPs ...string) *corev1.Service {
		return &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: clusterIP, ClusterIPs: clusterIPs}}
	}
	assert.Equal(t, []string{"10.0.0.1", "2001:db8::1"}, ServiceAddresses(svc("10.0.0.1", "10.0.0.1", "2001:db8::1")),
		"clusterIPs win and keep their order")
	assert.Equal(t, []string{"10.0.0.1"}, ServiceAddresses(svc("10.0.0.1")),
		"clusterIP is the fallback when clusterIPs is unset")
	assert.Nil(t, ServiceAddresses(svc("None", "None")), "a headless Service has no addresses")
	assert.Equal(t, []string{"10.0.0.2"}, ServiceAddresses(svc("None", "", "10.0.0.2")),
		"empty and None entries are skipped, not returned")
	assert.Nil(t, ServiceAddresses(svc("")), "no clusterIP at all yields no addresses")
}

func TestServiceEntryAddresses(t *testing.T) {
	se := &networkingclient.ServiceEntry{}
	se.Spec.Addresses = []string{"240.240.0.1"}
	se.Status.Addresses = []*networkingv1beta1.ServiceEntryAddress{{Value: "240.240.0.2"}}
	assert.Equal(t, []string{"240.240.0.1", "240.240.0.2"}, ServiceEntryAddresses(se),
		"spec addresses come first, then auto-allocated status addresses")
	assert.Empty(t, ServiceEntryAddresses(&networkingclient.ServiceEntry{}))
}
