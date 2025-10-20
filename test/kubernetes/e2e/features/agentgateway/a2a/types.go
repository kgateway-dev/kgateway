//go:build e2e

package a2a

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

// A2AMessageResponse models the A2A JSON-RPC response
type A2AMessageResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Result  *struct {
		Kind      string `json:"kind"`
		MessageID string `json:"messageId"`
		Parts     []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"parts"`
		Role string `json:"role"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// A2AAgentCard models the agent card discovery response
type A2AAgentCard struct {
	Name                              string   `json:"name"`
	Version                           string   `json:"version"`
	Description                       string   `json:"description"`
	ProtocolVersion                   string   `json:"protocolVersion"`
	PreferredTransport                string   `json:"preferredTransport"`
	URL                               string   `json:"url"`
	DefaultInputModes                 []string `json:"defaultInputModes"`
	DefaultOutputModes                []string `json:"defaultOutputModes"`
	SupportsAuthenticatedExtendedCard bool     `json:"supportsAuthenticatedExtendedCard"`
	Capabilities                      struct {
		Streaming bool `json:"streaming"`
	} `json:"capabilities"`
	Skills []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Examples    []string `json:"examples"`
		Tags        []string `json:"tags"`
	} `json:"skills"`
}

const (
	// a2aProto is the protocol version for A2A
	a2aProto         = "0.3.0"
	curlPodName      = "curl"
	curlPodNamespace = "curl"
)

var (
	_ e2e.NewSuiteFunc = NewTestingSuite

	gatewayName      = "gw"
	gatewayNamespace = "default"

	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest, defaults.CurlPodManifest},
	}
)
