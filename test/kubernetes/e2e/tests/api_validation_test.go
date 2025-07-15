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
		{
			name: "BackendConfigPolicy: valid target references",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: backend-config-valid-targets
spec:
  targetRefs:
  - group: ""
    kind: Service
    name: test-service
  - group: gateway.kgateway.dev
    kind: Backend
    name: test-backend
  targetSelectors:
  - group: ""
    kind: Service
    matchLabels:
      app: myapp
  http1ProtocolOptions:
    enableTrailers: true
`,
		},
		{
			name: "BackendConfigPolicy: invalid target reference",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: backend-config-invalid-target
spec:
  targetRefs:
  - group: apps
    kind: Deployment
    name: test-deployment
`,
			wantError: "TargetRefs must reference either a Kubernetes Service or a Backend API",
		},
		{
			name: "BackendConfigPolicy: invalid target selector",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: backend-config-invalid-selector
spec:
  targetSelectors:
  - group: apps
    kind: Deployment
    matchLabels:
      app: myapp
`,
			wantError: "TargetSelectors must reference either a Kubernetes Service or a Backend API",
		},
		{
			name: "TrafficPolicy: valid target references",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: traffic-policy-valid-targets
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: test-gateway
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route
  - group: gateway.networking.x-k8s.io
    kind: XListenerSet
    name: test-listener
  targetSelectors:
  - group: gateway.networking.k8s.io
    kind: Gateway
    matchLabels:
      app: myapp
`,
		},
		{
			name: "TrafficPolicy: invalid target reference",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: traffic-policy-invalid-target
spec:
  targetRefs:
  - group: apps
    kind: Deployment
    name: test-deployment
`,
			wantError: "targetRefs may only reference Gateway, HTTPRoute, or XListenerSet resources",
		},
		{
			name: "TrafficPolicy: policy with autoHostRewrite can only target HTTPRoute",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: traffic-policy-ahr-invalid-target
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: test-gateway
  autoHostRewrite: true
`,
			wantError: "autoHostRewrite can only be used when targeting HTTPRoute resources",
		},
		{
			name: "HTTPListenerPolicy: valid target references",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: http-listener-policy-valid-targets
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: test-gateway
  targetSelectors:
  - group: gateway.networking.k8s.io
    kind: Gateway
    matchLabels:
      app: myapp
`,
		},
		{
			name: "HTTPListenerPolicy: invalid target reference - HTTPRoute not allowed",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: http-listener-policy-invalid-target-httproute
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route
`,
			wantError: "targetRefs may only reference Gateway resources",
		},
		{
			name: "HTTPListenerPolicy: invalid target reference - wrong resource type",
			input: `---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: http-listener-policy-invalid-target
spec:
  targetRefs:
  - group: gateway.networking.x-k8s.io
    kind: XListenerSet
    name: test-listener
`,
			wantError: "targetRefs may only reference Gateway resources",
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
