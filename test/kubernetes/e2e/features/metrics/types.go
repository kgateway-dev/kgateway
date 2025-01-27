package metrics

import (
	"path/filepath"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// manifests
	exampleServiceManifest            = filepath.Join(util.MustGetThisDir(), "testdata", "service.yaml")
	gatewayAndRouteToServiceManifest  = filepath.Join(util.MustGetThisDir(), "testdata", "gateway-and-route-to-service.yaml")
	gatewayAndRouteToUpstreamManifest = filepath.Join(util.MustGetThisDir(), "testdata", "gateway-and-route-to-upstream.yaml")

	// objects
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": "nginx",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("http-web-svc"),
				},
			},
		},
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}

	kubeUpstream = &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      "example-upstream",
			Namespace: "default",
		},
		UpstreamType: &gloov1.Upstream_Kube{
			Kube: &kubernetes.UpstreamSpec{
				ServiceName:      "example-svc",
				ServiceNamespace: "default",
				ServicePort:      8080,
			},
		},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestKubeServiceSuccessStats": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{testdefaults.CurlPodManifest, exampleServiceManifest, gatewayAndRouteToServiceManifest},
				Resources: []client.Object{testdefaults.CurlPod, exampleSvc, nginxPod, proxyDeployment, proxyService},
			},
		},
		"TestKubeUpstreamSuccessStats": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{testdefaults.CurlPodManifest, exampleServiceManifest, gatewayAndRouteToUpstreamManifest},
				Resources: []client.Object{testdefaults.CurlPod, exampleSvc, nginxPod, proxyDeployment, proxyService},
			},
		},
	}
)
