package inferenceextension

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// runOpenAITests drives the common curl-based subtests for OpenAI APIs.
// - ns: the namespace where the gateway lives
// - svcMeta: ObjectMeta of the gateway Service
// - headerToModel: map[x-model-name header]modelName; if key=="" skip setting that header.
func (s *testingSuite) runOpenAITests(ns string, svcMeta metav1.ObjectMeta, headerToModel map[string]string) {
	tests := buildOpenAITests()
	for i, tc := range tests {
		name := fmt.Sprintf("OpenAITestCase%d", i)
		s.T().Run(name, func(t *testing.T) {
			for header, model := range headerToModel {
				// build the prompt/messages fragment
				var fieldJSON string
				if tc.api == "/v1/completions" {
					fieldJSON = fmt.Sprintf(`"prompt":"%s"`, tc.prompt)
				} else {
					fieldJSON = fmt.Sprintf(`"messages":%s`, tc.prompt)
				}
				// inject into body
				body := fmt.Sprintf(
					`{"model":"%s",%s,"max_tokens":100,"temperature":0}`,
					model,
					fieldJSON,
				)
				// prepare curl opts
				opts := defaults.CurlPodExecOpt
				opts.Namespace = ns

				// build the options slice
				curlOpts := []curl.Option{
					curl.WithHost(kubeutils.ServiceFQDN(svcMeta)),
					curl.WithHeader("Content-Type", "application/json"),
					curl.WithPath(tc.api),
					curl.WithBody(body),
				}
				if header != "" {
					curlOpts = append(curlOpts, curl.WithHeader("x-model-name", header))
				}

				// perform the assertion
				s.testInstallation.Assertions.AssertEventualCurlResponse(
					s.ctx, opts, curlOpts, expectedVllmResp,
				)
			}
		})
	}
}
