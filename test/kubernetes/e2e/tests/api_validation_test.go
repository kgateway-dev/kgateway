package tests

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
)

func TestAPIValidation(t *testing.T) {
	ctx := t.Context()
	ti := e2e.CreateTestInstallation(t, &install.Context{
		ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
	})

	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{
			name: "Backend: enforce ExactlyOneOf for backend type",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: backend-oneof
spec:
  type: AWS
  aws:
    accountId: "000000000000"
    lambda:
      functionName: hello-function
      invocationMode: Async
  static:
    hosts:
    - host: example.com
      port: 80
`,
			wantError: "exactly one of the fields in [ai aws static dynamicForwardProxy] must be set",
		},
		{
			name: "BackendConfigPolicy: enforce AtMostOneOf for HTTP protocol options",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: backend-config-both-http-options
spec:
  targetRefs:
  - group: ""
    kind: Service
    name: test-service
  http1ProtocolOptions:
    enableTrailers: true
    headerFormat: ProperCaseHeaderKeyFormat
  http2ProtocolOptions:
    maxConcurrentStreams: 100
    overrideStreamErrorOnInvalidHttpMessage: true
`,
			wantError: "at most one of the fields in [http1ProtocolOptions http2ProtocolOptions] may be set",
		},
	}

	t.Cleanup(func() {
		ctx := context.Background()
		ti.UninstallKgatewayCRDs(ctx)
	})
	ti.InstallKgatewayCRDsFromLocalChart(ctx)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			t.Cleanup(func() {
				ti.Actions.Kubectl().DeleteFile(ctx, tc.input) //nolint:errcheck
			})

			out := new(bytes.Buffer)

			err := ti.Actions.Kubectl().WithReceiver(out).Apply(ctx, []byte(tc.input))
			if tc.wantError != "" {
				r.Error(err)
				r.Contains(out.String(), tc.wantError)
			}
		})
	}
}
