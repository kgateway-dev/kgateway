//go:build e2e

package backends

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	manifests = []string{
		filepath.Join(fsutils.MustGetThisDir(), "testdata/base.yaml"),
		// backend in separate manifest to allow creation independently of routing config
		filepath.Join(fsutils.MustGetThisDir(), "testdata/backend.yaml"),
		defaults.CurlPodManifest,
	}

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjMeta}

	nginxMeta = metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: "default",
	}
)

type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestConfigureBackingDestinationsWithUpstream() {
	backendMeta := metav1.ObjectMeta{
		Name:      "nginx-static",
		Namespace: "default",
	}
	backend := &kgateway.Backend{
		ObjectMeta: backendMeta,
	}

	testutils.Cleanup(s.T(), func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment, backend)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}

	// assert the expected resources are created and running before attempting to send traffic
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment, backend)
	// TODO: make this a specific assertion to remove the need for c/p the label selector
	// e.g. EventuallyCurlPodRunning(...) etc.
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=curl",
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(defaults.NginxResponse),
		})

	s.assertStatus(backend, metav1.Condition{
		Type:    "Accepted",
		Status:  metav1.ConditionTrue,
		Reason:  "Accepted",
		Message: "Backend accepted",
	})
}

// TestBackendWithRuntimeError tests if backend condition is updated with error
func (s *testingSuite) TestBackendWithRuntimeError() {
	errorManifest := filepath.Join(fsutils.MustGetThisDir(), "testdata/backend-error.yaml")

	testutils.Cleanup(s.T(), func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, errorManifest)
		s.Require().NoError(err)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, errorManifest)
	s.Require().NoError(err)

	backendWithError := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-aws-backend",
			Namespace: "default",
		},
	}

	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, backendWithError)

	s.assertStatus(backendWithError, metav1.Condition{
		Type:    "Accepted",
		Status:  metav1.ConditionFalse,
		Reason:  "Invalid",
		Message: `Backend error: "Secret default/lambda-secret not found"`,
	})

	updateErrorManifest := filepath.Join(fsutils.MustGetThisDir(), "testdata/backend-update-error.yaml")

	testutils.Cleanup(s.T(), func() {
		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, updateErrorManifest)
		s.Require().NoError(err)
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, updateErrorManifest)
	s.Require().NoError(err)

	s.assertStatus(backendWithError, metav1.Condition{
		Type:   "Accepted",
		Status: metav1.ConditionFalse,
		Reason: "Invalid",
		Message: `Backend error: "failed to create aws request signing config: failed to derive static secret: access_key is not a valid string
secret_key is not a valid string"`,
	})
}

func (s *testingSuite) assertStatus(backend *kgateway.Backend, expected metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.testInstallation.AssertionsT(s.T())
	p.Gomega.Eventually(func(g gomega.Gomega) {
		be := &kgateway.Backend{}
		objKey := client.ObjectKeyFromObject(backend)
		err := s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get Backend %s", objKey)

		actual := be.Status.Conditions
		g.Expect(actual).To(gomega.HaveLen(1), "condition should have length of 1")
		cond := meta.FindStatusCondition(actual, expected.Type)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(expected.Status))
		g.Expect(cond.Reason).To(gomega.Equal(expected.Reason))
		g.Expect(cond.Message).To(gomega.Equal(expected.Message))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
