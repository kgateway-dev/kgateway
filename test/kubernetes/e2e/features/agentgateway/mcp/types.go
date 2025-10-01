package mcp

import (
	"path/filepath"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

type ToolsListResponse struct {
	JSONRPC string `json:"jsonrpc"`
	Result  *struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		} `json:"tools"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ResourcesListResponse struct {
	JSONRPC string `json:"jsonrpc"`
	Result  *struct {
		Resources []struct {
			URI  string `json:"uri"`
			Name string `json:"name,omitempty"`
		} `json:"resources"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// InitializeResponse models the MCP initialize payload.
type InitializeResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  *struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Instructions string `json:"instructions,omitempty"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// mcpProto is the protocol version for the MCP server
// This will be set dynamically from the initialize response

var (
	mcpProto   = "2025-03-26" // Default fallback, will be updated dynamically
	httpOKCode = 200
	warmupTime = 75 * time.Millisecond
)

var (
	_ e2e.NewSuiteFunc = NewTestingSuite
	// Gateway defaults used by this feature suite
	gatewayName      = "gw"
	gatewayNamespace = "default"

	// manifests
	setupManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	staticSetupManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "static.yaml")
	dynamicSetupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "dynamic.yaml")

	// Base test setup - common resources + curl pod
	setup = base.TestCase{
		Manifests: []string{setupManifest, defaults.CurlPodManifest},
	}

	// Dynamic test setup (only dynamic-specific resources)
	dynamicSetup = base.TestCase{
		Manifests: []string{dynamicSetupManifest},
	}

	// Static test setup (resources needed for non-dynamic tests)
	staticSetup = base.TestCase{
		Manifests: []string{staticSetupManifest},
	}
)
