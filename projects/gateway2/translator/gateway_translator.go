package translator

import (
	"context"
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// K8sGwTranslator This translator Translates K8s Gateway resources into Gloo Edege Proxies.
type K8sGwTranslator interface {
	// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the Gloo Edge Proxy resources that should configure a target Gloo Edge Proxy.
	// A null return value indicates the K8s Gateway resource failed to translate into a Gloo Edge Proxy. The error will be reported on the provided reporter.
	TranslateProxy(
		ctx context.Context,
		gateway *v1beta1.Gateway,
		queries controller.GatewayQueries,
		reporter reports.Reporter,
	) *v1.Proxy
}

func NewTranslator() K8sGwTranslator {
	return &translator{}
}

type translator struct{}

func (t *translator) TranslateProxy(
	ctx context.Context,
	gateway *v1beta1.Gateway,
	queries controller.GatewayQueries,
	reporter reports.Reporter,
) *v1.Proxy {
	if !validateGateway(gateway, queries, reporter) {
		return nil
	}

	routes, err := queries.GetRoutesForGw(ctx, gateway)
	if err != nil {
		// TODO(ilackarms): fill in the specific error / validation
		reporter.Gateway(gateway).Err(err.Error())
		return nil
	}

	listeners := listener.TranslateListeners(
		gateway,
		routes,
		reporter,
	)

	return &v1.Proxy{
		Metadata:  proxyMetadata(gateway),
		Listeners: listeners,
	}
}

func proxyMetadata(gateway *v1beta1.Gateway) *core.Metadata {
	// TODO(ilackarms) what should the proxy ID be
	// ROLE ON ENVOY MUST MATCH <proxy_namespace>~<proxy_name>
	// equal to role: {{.Values.settings.writeNamespace | default .Release.Namespace }}~{{ $name | kebabcase }}
	return &core.Metadata{
		Name:      "",
		Namespace: "",
	}
}
