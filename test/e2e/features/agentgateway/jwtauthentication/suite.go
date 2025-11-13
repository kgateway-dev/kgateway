//go:build e2e

package jwtauthentication

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

//
// Use `go run test/e2e/testutils/jwt/jwt-generator.go`
// to generate jwks and a jwt signed by the key in it
//

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	// test namespace for proxy resources
	namespace = "default"
	// test service name
	serviceName = "backend-0"
	jwt1        = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjkxMjY5MjUwMjQ1MTc1Mjc2OTIiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL3NvbG8uaW8iLCJzdWIiOiJ0ZXN0QHNvbG8saW8iLCJleHAiOjIwNzA2NTIxOTQsIm5iZiI6MTc2MzA2ODE5NCwiaWF0IjoxNzYzMDY4MTk0fQ.xQ-EvQs6PI6sIIcY8SLcPkjO4jrdcwZGt7oDeM0fTL2pwIO0oW42ZqM9K-wtZTHySJUhVa-QZIhBmHiJEDL9dMKp7I6mK60KadLTWo9rhtCfu9HIXfy3AYQzvEa8S3-hM0YmQKvAWAenCdytscl4y0tAmBc0gAfqYWP_elaXBsS9ORkIhsMkA9cS0rgJRFMhaMiq9n8t9HfZ4Z5dBHSAl__bjX9JiVeTndFiAJhAm65Q_-zvkBse142kIKCF93vpjQFFWzqc_GDjBfuRNFqPRgCSUfQXpVdq5h2U0vdR3aeWBi4l9r4do5Zd7q_eLwdgPzz0sgFa8-ZUW0x1Y52iYw"
	jwt2        = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjQ1MTg5NDI1Nzc5OTY4MTIzNSIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL3NvbG8uaW8iLCJzdWIiOiJ0ZXN0QHNvbG8saW8iLCJleHAiOjIwNzA2NTI3MzIsIm5iZiI6MTc2MzA2ODczMiwiaWF0IjoxNzYzMDY4NzMyfQ.TWpoBaqy6avY0MO4yjVoC3KCN7qEQfkD962UgUqCiaCiw_AkGo3whrUMZORjYTXx1-OfBuQukL1q-6xt0ye04jyFW5ryHPdrExlypgwJOGZOoxo24plh6MtrI_150eaoCj7xWV0ycusYG13Kcb7lQFfizweokqgGhD1O65RW0O_NDUdbhhBVvT4AdwdboKIGVYgxRgB17tqb2So1ehAL1viRIm4-5eRQSLS8ghs8zYpglEzf7YJJ3_Zi2R0Vig_bn5I4qq6n3XPUbD-NMbq05V5NADS_DZ6OculUINR0I-ikKe1WbZFMlmT7lpOOHoansa8lyy4BGR_gFQEC0Gywwg"
	jwt3        = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjE4NjY4MTc3NTQzNzA0MDEwMDYiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL3NvbG8uaW8iLCJzdWIiOiJ0ZXN0QHNvbG8saW8iLCJleHAiOjIwNzA2NTMwMDMsIm5iZiI6MTc2MzA2OTAwMywiaWF0IjoxNzYzMDY5MDAzfQ.nX_eY5y5hxRy_tKaFzUF7EALzpwTzNCgQK2CxXt5qRDYdxcVoXzVPfd-pO9a8iU1Wo-Ioq6cVlidsdVWKxsmKxiQVbzyD17ML8vQlNwVzxp7lqACir1fRUF_gtI63EflroYhyZRjsG1edzUhTSXsTGyGhlCTnGd7hphlhAK3P9BI0dyqAS9gXg1Y6dx-vRG5siJvn9UmZ4GLoJbFwmOyCyM97Z7GcvmeVeO6U4Cf6RM--pJtQx-6dnOMEFcTPFRzWfF3_3oZtRySiOYAtRhBFLPe2YRlRMxywehzYslCPGTppw0ErJmWk5XQo4ZQjwI9fQ9a0CYYCYb2qcE4LuRzXg"
)

var (
	// metadata for gateway - matches the name "super-gateway" from common.yaml
	gatewayObjectMeta = metav1.ObjectMeta{Name: "super-gateway", Namespace: namespace}
	gateway           = &gwv1.Gateway{
		ObjectMeta: gatewayObjectMeta,
	}

	// metadata for proxy resources
	proxyObjectMeta = metav1.ObjectMeta{Name: "super-gateway", Namespace: namespace}

	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: proxyObjectMeta,
	}
	proxyService = &corev1.Service{
		ObjectMeta: proxyObjectMeta,
	}
	proxyServiceAccount = &corev1.ServiceAccount{
		ObjectMeta: proxyObjectMeta,
	}

	// metadata for backend service
	serviceMeta = metav1.ObjectMeta{
		Namespace: namespace,
		Name:      serviceName,
	}

	simpleSvc = &corev1.Service{
		ObjectMeta: serviceMeta,
	}

	simpleDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      serviceName,
		},
	}

	insecureRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "route-example-insecure",
		},
	}
	secureGwRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "route-secure-gw",
		},
	}
	secureRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "route-secure",
		},
	}
	secureGwPolicy = &v1alpha1.AgentgatewayPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "gw-policy",
		},
	}
	secureRoutePolicy = &v1alpha1.AgentgatewayPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "route-policy",
		},
	}
)

type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// manifests shared by all tests
	commonManifests []string
	// resources from manifests shared by all tests
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

var (
	insecureRouteManifest     = getTestFile("insecure-route.yaml")
	secureGWPolicyManifest    = getTestFile("secured-gateway-policy.yaml")
	secureRoutePolicyManifest = getTestFile("secured-route.yaml")
)

func (s *testingSuite) SetupSuite() {
	s.commonManifests = []string{
		testdefaults.CurlPodManifest,
		getTestFile("common.yaml"),
		getTestFile("service.yaml"),
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		testdefaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// resources from gateway manifest
		gateway,
		// deployer-generated resources
		proxyDeployment, proxyService, proxyServiceAccount,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	// make sure pods are gone
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) TestRoutePolicy() {
	s.setupTest([]string{insecureRouteManifest, secureRoutePolicyManifest}, []client.Object{insecureRoute, secureRoute, secureRoutePolicy})

	// verify unprotected route works
	s.assertResponseWithoutAuth("insecureroute.com", http.StatusOK)
	// verify a provider with a single key in jwks works
	s.assertResponse("secureroute.com", jwt1, http.StatusOK)
	// verify a provider with multiple keys in jwks works
	s.assertResponse("secureroute.com", jwt2, http.StatusOK)
	s.assertResponse("secureroute.com", jwt3, http.StatusOK)
	// verify invalid/missing tokens are caught
	s.assertResponse("secureroute.com", "nosuchkey", http.StatusUnauthorized)
	s.assertResponseWithoutAuth("secureroute.com", http.StatusUnauthorized)
}

func (s *testingSuite) TestGatewayPolicy() {
	s.setupTest([]string{secureGWPolicyManifest}, []client.Object{secureGwRoute, secureGwPolicy})

	// verify a provider with a single key in jwks works
	s.assertResponse("securegateways.com", jwt1, http.StatusOK)
	// verify a provider with multiple keys in jwks works
	s.assertResponse("securegateways.com", jwt2, http.StatusOK)
	s.assertResponse("securegateways.com", jwt3, http.StatusOK)
	s.assertResponse("securegateways.com", "nosuchkey", http.StatusUnauthorized)
	// verify invalid/missing tokens are caught
	s.assertResponseWithoutAuth("securegateways.com", http.StatusUnauthorized)
}

func (s *testingSuite) setupTest(manifests []string, resources []client.Object) {
	testutils.Cleanup(s.T(), func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, resources...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, resources...)
}

func (s *testingSuite) assertResponse(hostHeader, authHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader(hostHeader),
			curl.WithHeader("Authorization", "Bearer "+authHeader),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func (s *testingSuite) assertResponseWithoutAuth(hostHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader(hostHeader),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
