package glooctl

import (
	"path/filepath"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/skv2/codegen/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	backendManifestFile      = filepath.Join(util.MustGetThisDir(), "testdata", "backend.yaml")
	edgeGatewaysManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "edge-gateway-gateways.yaml")
	edgeRoutesManifestFile   = filepath.Join(util.MustGetThisDir(), "testdata", "edge-gateway-routes.yaml")
	kubeGatewaysManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "kube-gateway-routes.yaml")

	// resources created by backend manifest
	nginxSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-svc",
			Namespace: "default",
		},
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}
	nginxUpstream = &gloov1.Upstream{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-upstream",
			Namespace: "default",
		},
	}

	// resources created by edge gateways manifest (these should be applied in Gloo Gateway's
	// install namespace)
	getEdgeGateway1 = func(installNamespace string) *gatewayv1.Gateway {
		return &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway1",
				Namespace: installNamespace,
			},
		}
	}
	getEdgeGateway2 = func(installNamespace string) *gatewayv1.Gateway {
		return &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway2",
				Namespace: installNamespace,
			},
		}
	}

	// resources created by the edge routes manifest
	edgeVs1 = &gatewayv1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs1",
			Namespace: "default",
		},
	}
	edgeVs2 = &gatewayv1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs1",
			Namespace: "default",
		},
	}

	// resources created by the kube gateways/routes manifest
	kubeGateway1 = &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw1",
			Namespace: "default",
		},
	}
	kubeRoute1 = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route1",
			Namespace: "default",
		},
	}
	kubeGateway2 = &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw2",
			Namespace: "default",
		},
	}
	kubeRoute2 = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route2",
			Namespace: "default",
		},
	}

	// all proxies should be created in Gloo Gateway's write namespace, which
	// defaults to the install namespace

	// expected edge proxies
	edgeProxy1Name = "proxy1"
	edgeProxy2Name = "proxy2"
	// this is the proxy associated with the default Gateways
	edgeDefaultProxyName = "gateway-proxy"

	// expected kube proxies
	kubeProxy1Name = "default-gw1"
	kubeProxy2Name = "default-gw2"
)
