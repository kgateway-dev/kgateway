//go:build e2e

package tls

import (
	"context"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

// TODO: Add negative test case to verify that invalid/malformed TLS certificates
// prevent the control plane from starting or cause traffic to fail. This validates
// proper error handling and ensures the system fails closed rather than falling back
// to insecure communication.

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{
			testdefaults.HttpbinManifest,
			routeManifest,
		},
	}
	testCases := map[string]*base.TestCase{
		"TestTLSControlPlaneBasicFunctionality": {},
		"TestTLSCertificateRotation":            {},
	}
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestTLSControlPlaneBasicFunctionality validates that the control plane with TLS enabled
// can successfully configure a basic Gateway and route traffic.
func (s *testingSuite) TestTLSControlPlaneBasicFunctionality() {
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("test.example.com"),
		},
		curl.WithHostHeader("test.example.com"),
		curl.WithPath("/headers"),
		curl.WithPort(8080),
	)
}

// TestTLSCertificateRotation validates that the control plane handles cert
// rotation correctly by updating the TLS secret and verifying continued operation.
func (s *testingSuite) TestTLSCertificateRotation() {
	// validate initial traffic works with the original certificate
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("test.example.com"),
		},
		curl.WithHostHeader("test.example.com"),
		curl.WithPath("/headers"),
		curl.WithPort(8080),
	)

	// generate a new certificate for rotation and update Secret
	rotatedSecretYAML, err := SecretManifest(s.TestInstallation.Metadata.InstallNamespace, DefaultExpiration)
	s.Require().NoError(err, "failed to generate rotated certificate")

	err = s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(rotatedSecretYAML))
	s.Require().NoError(err, "failed to apply rotated TLS secret")

	// wait for certificate rotation to propagate. arbitrary sleep used here to allow
	// time for kubelet and the control plane's certificate watcher to detect the change
	// and propagate the change throughout.
	time.Sleep(10 * time.Second)

	// verify traffic still works after rotation
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("test.example.com"),
		},
		curl.WithHostHeader("test.example.com"),
		curl.WithPath("/headers"),
		curl.WithPort(8080),
	)
}
