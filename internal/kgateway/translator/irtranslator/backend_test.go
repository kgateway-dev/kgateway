package irtranslator_test

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
)

func testInitBackend(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
	return nil
}

func TestBackendTranslatorTranslatesAppProtocol(t *testing.T) {
	g := NewWithT(t)
	var bt irtranslator.BackendTranslator
	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		AppProtocol: ir.HTTP2AppProtocol,
	}
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: testInitBackend,
		},
	}

	c, err := bt.TranslateBackend(kctx, ucc, backend)
	g.Expect(err).NotTo(HaveOccurred())
	opts := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	g.Expect(opts).NotTo(BeNil())

	p, err := opts.UnmarshalNew()
	g.Expect(err).NotTo(HaveOccurred())

	httpOpts, ok := p.(*envoy_upstreams_v3.HttpProtocolOptions)
	g.Expect(ok).To(BeTrue())
	g.Expect(httpOpts.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).NotTo(BeNil())
}
