package listenerset

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest                           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	validListenerSetManifest                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "valid-listenerset.yaml")
	validListenerSetManifest2               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "valid-listenerset-2.yaml")
	invalidListenerSetNotAllowedManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-listenerset-not-allowed.yaml")
	invalidListenerSetNonExistingGWManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-listenerset-non-existing-gw.yaml")
	policyManifest                          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "policies.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}

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

	// TestValidListenerSet
	validListenerSet = types.NamespacedName{
		Name:      "valid-ls",
		Namespace: "allowed-ns",
	}

	// TestInvalidListenerSetNotAllowed
	invalidListenerSetNotAllowed = types.NamespacedName{
		Name:      "invalid-ls-not-allowed",
		Namespace: "curl",
	}

	// TestInvalidListenerSetNonExistingGW
	invalidListenerSetNonExistingGW = types.NamespacedName{
		Name:      "invalid-ls-non-existing-gw",
		Namespace: "default",
	}

	expectOK = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}

	expectOKWithCustomHeader = func(key, value string) *testmatchers.HttpResponse {
		return &testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gstruct.Ignore(),
			Headers: map[string]interface{}{
				key: value,
			},
		}
	}

	expectNotFound = &testmatchers.HttpResponse{
		StatusCode: http.StatusNotFound,
		Body:       gstruct.Ignore(),
	}

	curlExitErrorCode = 28

	setup = base.SimpleTestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
		Resources: []client.Object{e2edefaults.CurlPod, exampleSvc, nginxPod, proxyDeployment, proxyService},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestValidListenerSet": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{validListenerSetManifest},
			},
		},
		"TestInvalidListenerSetNotAllowed": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{invalidListenerSetNotAllowedManifest},
			},
		},
		"TestInvalidListenerSetNonExistingGW": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{invalidListenerSetNonExistingGWManifest},
			},
		},
		"TestPolicies": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{validListenerSetManifest, validListenerSetManifest2, policyManifest},
			},
		},
	}
)
