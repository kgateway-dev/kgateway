package validation

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
)

const (
	SecretName       = "tls-secret"
	UnusedSecretName = "tls-secret-unused"

	ExampleVsName       = "example-vs"
	ExampleUpstreamName = "nginx-upstream"

	ValidVsName   = "i-am-valid"
	InvalidVsName = "i-am-invalid"
)

var (
	// setup configs
	ExampleVS       = filepath.Join(util.MustGetThisDir(), "testdata", "example-vs.yaml")
	ExampleUpstream = filepath.Join(util.MustGetThisDir(), "testdata", "example-upstream.yaml")

	// Switch VirtualService configs (allow warnings)
	InvalidVS = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-vs.yaml")
	ValidVS   = filepath.Join(util.MustGetThisDir(), "testdata", "valid-vs.yaml")
	SwitchVS  = filepath.Join(util.MustGetThisDir(), "testdata", "switch-valid-invalid.yaml")

	// Secret Configs (allow warnings, strict tests)
	SecretVS                       = filepath.Join(util.MustGetThisDir(), "testdata", "secret-deletion", "vs-with-secret.yaml")
	ValidationWebhookConfigFailure = filepath.Join(util.MustGetThisDir(), "testdata", "secret-deletion", "validationwebhookconfig-failure.yaml")
	UnusedSecret                   = filepath.Join(util.MustGetThisDir(), "testdata", "secret-deletion", "unused-secret.yaml")
	Secret                         = filepath.Join(util.MustGetThisDir(), "testdata", "secret-deletion", "secret.yaml")

	// Invalid resources (allow warnings, strict, allow all)
	InvalidUpstreamNoPort         = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "invalid-upstream-no-port.yaml")
	InvalidGateway                = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "gateway.yaml")
	InvalidVirtualServiceMatcher  = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "vs-method-matcher.yaml")
	InvalidVirtualServiceTypo     = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "vs-typo.yaml")
	InvalidVirtualMissingUpstream = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "vs-no-upstream.yaml")
	InvalidRLC                    = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-resources", "rlc.yaml")

	// transformation validation (allow warnings, server_enabled)
	VSTransformationExtractors    = filepath.Join(util.MustGetThisDir(), "testdata", "transformation", "vs-transform-extractors.yaml")
	VSTransformationHeaderText    = filepath.Join(util.MustGetThisDir(), "testdata", "transformation", "vs-transform-header-text.yaml")
	VSTransformationSingleReplace = filepath.Join(util.MustGetThisDir(), "testdata", "transformation", "vs-transform-single-replace.yaml")

	// CurlPodExecOpt is the Pod that will be used to execute curl requests, and is defined in the upstream manifest files
	CurlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	ExpectedUpstreamResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("Welcome to nginx!"),
	}
)
