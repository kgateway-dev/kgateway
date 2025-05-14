package agentgateway

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	a2aPort = 9090
)

var (
	// Test A2A Agent
	a2aAgentManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "a2a.yaml")

	// kgateway managed deployment for the agentgateway
	deployAgentGatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agentgateway-deploy.yaml")
)
