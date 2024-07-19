package validation_always_accept

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/skv2/codegen/util"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

const (
	invalidVsName = "i-am-invalid"
	validVsName   = "i-am-valid"
)

var (
	invalidVS = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-vs.yaml")
	validVS   = filepath.Join(util.MustGetThisDir(), "testdata", "valid-vs.yaml")
	upstream  = filepath.Join(util.MustGetThisDir(), "testdata", "upstream.yaml")
	switchVS  = filepath.Join(util.MustGetThisDir(), "testdata", "switch-valid-invalid.yaml")

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the upstream manifest files
	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	expectedUpstreamResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("Welcome to nginx!"),
	}
)
