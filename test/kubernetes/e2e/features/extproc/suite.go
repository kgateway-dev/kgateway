package extproc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/transforms"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for external processing functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestExtProcWithRoute() {
	manifests := []string{
		testdefaults.CurlPodManifest,
		setupManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,              // curl
		extProcService, extProcDeployment, // ext-proc service
		backendService, backendDeployment, // backend service
		gatewayService, gatewayDeployment, // gateway service
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, extProcDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=ext-proc-grpc",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, backendDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0",
	})

	// Should have a successful response to the backend route
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})

	// Should have a successful response to the extproc route
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})
}

// The instructions format that the example extproc service understands.
// See the `basic-sink` example in https://github.com/solo-io/ext-proc-examples
type instructions struct {
	// Header key/value pairs to add to the request or response.
	AddHeaders map[string]string `json:"addHeaders"`
	// Header keys to remove from the request or response.
	RemoveHeaders []string `json:"removeHeaders"`
}

func getInstructionsJson(instr instructions) string {
	bytes, _ := json.Marshal(instr)
	return string(bytes)
}
