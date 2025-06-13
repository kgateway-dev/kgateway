package inferenceextension

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// clientName is the name of the curl client pod used for testing.
	clientName = "curl"
	// gtwName is the name of the Gateway used for testing.
	gtwName = "inference-gateway"
	// routeName is the HTTPRoute name used for testing.
	routeName = "llm-route"
	// secondRouteName is the name of the second HTTPRoute used for testing.
	secondRouteName = routeName + "-2"
	// vllmName is the name of the vLLM deployment name used for testing.
	vllmName = "vllm-llama3-8b-instruct"
	// secondVllmName is the name of the second test data vLLM deployment name
	secondVllmName = vllmName + "-2"
	// eppName is the name of the EPP deployment name used for testing.
	eppName = vllmName + "-epp"
	// secondEPPName is the name of the second test data EPP deployment name
	secondEPPName = eppName + "-2"
	// baseModelName is the model name value defined in request bodies.
	baseModelName = "food-review"
	// secondBaseModelName is the second model name value defined in request bodies.
	secondBaseModelName = baseModelName + "-2"
	// targetModelName is the expected value in response bodies when requesting the baseModelName
	// and the vLLM model server is configured for the `food-review-1` LoRA adapter.
	targetModelName = baseModelName + "-1"
	// podRunTimeout is time required for a pod to reach a "Running" status
	podRunTimeout = 3 * time.Minute
	// gtwProgramTimeout is time required for the gateway to reach "Programmed" status
	gtwProgramTimeout = 1 * time.Minute

	// basicManifest is the manifest for the TestSingleHTTPRouteSingleInferencePool test
	//go:embed testdata/single_route_single_pool.yaml
	basicManifest []byte
	// basicTestNs is namespace used for the TestSingleHTTPRouteSingleInferencePool test
	basicTestNs = "inf-ext-basic"

	// singleRouteMultiPoolManifest is the manifest for the TestSingleHTTPRouteMultiInferencePool test
	//go:embed testdata/single_route_multi_pool.yaml
	singleRouteMultiPoolManifest []byte
	// singleRouteMultiPoolNs is namespace used for the TestSingleHTTPRouteMultiInferencePool test
	singleRouteMultiPoolNs = "inf-ext-single-route-multi-pool"

	// multiRouteSinglePoolManifest is the manifest for the TestMultiHTTPRouteSingleInferencePool test
	//go:embed testdata/multi_route_single_pool.yaml
	multiRouteSinglePoolManifest []byte
	// multiRouteSinglePoolNs is namespace used for the TestMultiHTTPRouteSingleInferencePool test
	multiRouteSinglePoolNs = "inf-ext-multi-route-single-pool"

	// headerToModel maps an HTTP header value to a base model name used in a curl request
	headerToModel = map[string]string{
		"facebook/llama3-8b-instruct":   baseModelName,
		"facebook/llama3-8b-instruct-2": secondBaseModelName,
	}
	// The expected response when curl'ing the vLLM backend
	expectedVllmResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		// Use a Gomega matcher so that we assert that the response body CONTAINS the substring `"model":"<modelName>"`
		Body: gomega.ContainSubstring(`"model":"` + targetModelName + `"`),
	}
)

// objectMeta returns a metav1.ObjectMeta with the given ns and name.
func objectMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: ns,
		Name:      name,
	}
}
