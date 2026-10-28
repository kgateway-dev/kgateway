package utils

import (
	"fmt"
	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"
)

// listenerSets builds n ListenerSets with one HTTPS listener each, all referencing the same
// certificate Secret, with fields set the way the API server defaults them.
func listenerSets(n int) []*gwv1.ListenerSet {
	out := make([]*gwv1.ListenerSet, 0, n)
	for i := range n {
		out = append(out, &gwv1.ListenerSet{
			Spec: gwv1.ListenerSetSpec{
				Listeners: []gwv1.ListenerEntry{{
					Name:     "https",
					Hostname: new(gwv1.Hostname(fmt.Sprintf("host-%d.example.com", i))),
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					TLS: &gwv1.ListenerTLSConfig{
						Mode: new(gwv1.TLSModeTerminate),
						CertificateRefs: []gwv1.SecretObjectReference{{
							Group: new(gwv1.Group("")),
							Kind:  new(gwv1.Kind("Secret")),
							Name:  "shared-tls",
						}},
					},
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{From: new(gwv1.NamespacesFromSame)},
						Kinds:      []gwv1.RouteGroupKind{{Group: new(gwv1.Group(gwv1.GroupName)), Kind: "HTTPRoute"}},
					},
				}},
			},
		})
	}
	return out
}

func TestToListenerAliasesEntry(t *testing.T) {
	g := NewWithT(t)
	entry := listenerSets(1)[0].Spec.Listeners[0]
	entry.Filters = &gwv1.ListenerFilters{}

	l := ToListener(entry)

	g.Expect(l.Name).To(Equal(entry.Name))
	g.Expect(l.Port).To(Equal(entry.Port))
	g.Expect(l.Protocol).To(Equal(entry.Protocol))
	g.Expect(l.Hostname).To(BeIdenticalTo(entry.Hostname), "Hostname should alias the entry")
	g.Expect(l.TLS).To(BeIdenticalTo(entry.TLS), "TLS should alias the entry")
	g.Expect(l.AllowedRoutes).To(BeIdenticalTo(entry.AllowedRoutes), "AllowedRoutes should alias the entry")
	g.Expect(l.Filters).To(BeIdenticalTo(entry.Filters), "Filters should alias the entry")
}

// BenchmarkToListenerSlice converts the listeners of many ListenerSets, as each Gateway
// translation does once per ListenerSet and route parentRef.
func BenchmarkToListenerSlice(b *testing.B) {
	sets := listenerSets(2500)
	b.ReportAllocs()
	for b.Loop() {
		for _, ls := range sets {
			if listeners := ToListenerSlice(ls.Spec.Listeners); len(listeners) != 1 {
				b.Fatalf("got %d listeners, want 1", len(listeners))
			}
		}
	}
}
