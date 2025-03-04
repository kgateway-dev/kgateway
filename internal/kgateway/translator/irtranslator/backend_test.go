package irtranslator_test

import (
	"context"
	"testing"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func testBInitBackend(ctx context.Context, in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
}

func TestBackendTranslatorTranslatesAppProtocol(t *testing.T) {
	var bt irtranslator.BackendTranslator
	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	backend := ir.BackendObjectIR{
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
			InitBackend: testBInitBackend,
		},
	}

	c, err := bt.TranslateBackend(kctx, ucc, backend)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	opts := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	if opts == nil {
		t.Errorf("Expected HttpProtocolOptions, got nil")
	}

	p, err := opts.UnmarshalNew()
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	httpOpts, ok := p.(*envoy_upstreams_v3.HttpProtocolOptions)
	if !ok {
		t.Errorf("Expected HttpProtocolOptions, got %T", p)
	}
	if httpOpts.GetExplicitHttpConfig().GetHttp2ProtocolOptions() == nil {
		t.Errorf("Expected Http2ProtocolOptions, got nil")
	}
}
