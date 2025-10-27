//go:build e2e

package inference

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/onsi/gomega"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// gtwName is the name of the gateway created by test manifests.
	gtwName = "inference-gateway"
	// vllmDeployName is the name of the vLLM deployment name
	vllmDeployName = "vllm-llama3-8b-instruct"
	// targetModelName is the model name value defined in an LLM request body.
	targetModelName = "food-review-1"
	// testRouteName is the test data HTTPRoute name used in tests
	testRouteName = "llm-route"
	// podRunTimeout is time required for a pod to reach a "Running" status
	podRunTimeout = 3 * time.Minute
	// gtwProgramTimeout is time required for the gateway to reach "Programmed" status
	gtwProgramTimeout = 60 * time.Second
	// vllmManifest is the manifest for the vLLM simulator model server
	//go:embed testdata/vllm.yaml
	vllmManifest []byte
	// poolManifest is the manifest for the InferencePool resource
	//go:embed testdata/pool.yaml
	poolManifest []byte
	// eppManifest is the manifest for the Endpoint Picker (EPP)
	//go:embed testdata/epp.yaml
	eppManifest []byte
	// gtwManifest is the manifest for the primary Gateway resource
	//go:embed testdata/gateway.yaml
	gtwManifest []byte
	// routeManifest is the manifest for the HTTPRoute resource
	//go:embed testdata/route.yaml
	routeManifest []byte
	// clientManifest is the manifest for the curl client
	//go:embed testdata/client.yaml
	clientManifest []byte
	// manifestLabelKey is the label selector key for manifests
	manifestLabelKey = "app"
	// clientPodName is the name of the client pod
	clientPodName = "curl"
)

func expectedVllmRespWithPort(port string) *testmatchers.HttpResponse {
	return &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring(`"model":"` + targetModelName + `"`),
		Headers: map[string]interface{}{
			"x-inference-port": port,
		},
	}
}
