package inferenceextension

import (
	"net/http"
	"path/filepath"
	"time"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// testNS is the namespace used for e2e tests. Test data manifests are hardcoded for this namespace.
	testNS = "inf-ext-e2e"
	// vllmDeployName is the name of the vLLM deployment name
	vllmDeployName = "vllm-llama3-8b-instruct"
	// podRunTimeout is time required for a pod to reach a "Running" status
	podRunTimeout = 3 * time.Minute
	// gtwProgramTimeout is time required for the gateway to reach "Programmed" status
	gtwProgramTimeout = 60 * time.Second
	// Manifests used to run test resources
	vllmManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "vllm.yaml")
	modelManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "models.yaml")
	poolManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "pool.yaml")
	eppManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "epp.yaml")
	gtwManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	routeManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route.yaml")
	clientManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "client.yaml")

	// The Gateway resources created by kgateway
	gtwObjectMeta = metav1.ObjectMeta{
		Name:      "inference-gateway",
		Namespace: testNS,
	}
	gtwDeployment = &appsv1.Deployment{ObjectMeta: gtwObjectMeta}
	gtwService    = &corev1.Service{ObjectMeta: gtwObjectMeta}

	// The expected response when curl'ing the vLLM backend
	expectedVllmResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}
)
