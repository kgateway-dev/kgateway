package listener

import (
	"context"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8scert "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// secretQueries resolves every Secret reference to one Secret.
type secretQueries struct {
	query.GatewayQueries
	secret ir.Secret
}

func (q secretQueries) GetSecretForRef(krt.HandlerContext, context.Context, schema.GroupKind, string, gwv1.SecretObjectReference) (*ir.Secret, error) {
	secret := q.secret
	return &secret, nil
}

// TestTranslateListenersLeavesSourceObjectsUnmodified guards the listener IR sharing its
// Hostname, TLS and AllowedRoutes with the Gateway and ListenerSet in the informer cache.
func TestTranslateListenersLeavesSourceObjectsUnmodified(t *testing.T) {
	g := NewWithT(t)
	certPEM, keyPEM, err := k8scert.GenerateSelfSignedCertKey("example.com", nil, nil)
	g.Expect(err).NotTo(HaveOccurred())
	queries := secretQueries{secret: ir.Secret{
		ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: "default", Name: "tls"},
		Data:         map[string][]byte{corev1.TLSCertKey: certPEM, corev1.TLSPrivateKeyKey: keyPEM},
	}}

	// Optional fields are left unset where translation applies a default.
	tls := &gwv1.ListenerTLSConfig{
		Mode:            new(gwv1.TLSModeTerminate),
		CertificateRefs: []gwv1.SecretObjectReference{{Name: "tls"}},
		Options:         map[gwv1.AnnotationKey]gwv1.AnnotationValue{annotations.AlpnProtocols: "h2,http/1.1"},
	}
	allowedRoutes := &gwv1.AllowedRoutes{
		Namespaces: &gwv1.RouteNamespaces{From: new(gwv1.NamespacesFromAll)},
		Kinds:      []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{{
				Name:          "https",
				Hostname:      new(gwv1.Hostname("gw.example.com")),
				Port:          443,
				Protocol:      gwv1.HTTPSProtocolType,
				TLS:           tls.DeepCopy(),
				AllowedRoutes: allowedRoutes.DeepCopy(),
			}},
		},
	}
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ls"},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gwv1.ListenerEntry{
				{
					Name:          "https",
					Hostname:      new(gwv1.Hostname("ls.example.com")),
					Port:          443,
					Protocol:      gwv1.HTTPSProtocolType,
					TLS:           tls.DeepCopy(),
					AllowedRoutes: allowedRoutes.DeepCopy(),
				},
				{
					Name:          "http",
					Port:          80,
					Protocol:      gwv1.HTTPProtocolType,
					AllowedRoutes: allowedRoutes.DeepCopy(),
				},
			},
		},
	}
	gwIR := gwToIr(gw, ls, nil)
	wantGw, wantLs := gw.DeepCopy(), ls.DeepCopy()

	rm := reports.NewReportMap()
	translated := TranslateListeners(krt.TestingDummyContext{}, context.Background(), queries, gwIR,
		query.NewRoutesForGwResult(), reports.NewReporter(&rm), ListenerTranslatorConfig{})
	g.Expect(translated).To(HaveLen(2), "the HTTPS listeners should share port 443 and the HTTP listener get port 80")

	g.Expect(gw).To(Equal(wantGw), "translation should not modify the Gateway")
	g.Expect(ls).To(Equal(wantLs), "translation should not modify the ListenerSet")
}

// TestHTTPSFilterChainsRecordTheirListener checks that each HTTPS filter chain records the
// listener it serves, so status reports on that listener, and that the plain HTTP chain,
// which merges every listener on its port, records none.
func TestHTTPSFilterChainsRecordTheirListener(t *testing.T) {
	g := NewWithT(t)
	certPEM, keyPEM, err := k8scert.GenerateSelfSignedCertKey("example.com", nil, nil)
	g.Expect(err).NotTo(HaveOccurred())
	queries := secretQueries{secret: ir.Secret{
		ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: "default", Name: "tls"},
		Data:         map[string][]byte{corev1.TLSCertKey: certPEM, corev1.TLSPrivateKeyKey: keyPEM},
	}}
	tls := &gwv1.ListenerTLSConfig{
		Mode:            new(gwv1.TLSModeTerminate),
		CertificateRefs: []gwv1.SecretObjectReference{{Name: "tls"}},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gw"},
		Spec: gwv1.GatewaySpec{Listeners: []gwv1.Listener{
			{Name: "https", Hostname: new(gwv1.Hostname("gw.example.com")), Port: 443, Protocol: gwv1.HTTPSProtocolType, TLS: tls},
			{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType},
		}},
	}
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ls"},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gwv1.ListenerEntry{
				{Name: "https", Hostname: new(gwv1.Hostname("ls.example.com")), Port: 443, Protocol: gwv1.HTTPSProtocolType, TLS: tls},
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType},
			},
		},
	}

	rm := reports.NewReportMap()
	translated := TranslateListeners(krt.TestingDummyContext{}, context.Background(), queries, gwToIr(gw, ls, nil),
		query.NewRoutesForGwResult(), reports.NewReporter(&rm), ListenerTranslatorConfig{})

	type owner struct {
		parent client.Object
		name   gwv1.SectionName
	}
	owners := map[string]*owner{}
	for _, l := range translated {
		for _, fc := range l.HttpFilterChain {
			if fc.Listener == nil {
				owners[fc.FilterChainName] = nil
				continue
			}
			owners[fc.FilterChainName] = &owner{parent: fc.Listener.Parent, name: fc.Listener.Name}
		}
	}
	g.Expect(owners).To(Equal(map[string]*owner{
		"https":            {parent: gw, name: "https"},
		"default/ls/https": {parent: ls, name: "https"},
		"listener~80":      nil,
	}))
}
