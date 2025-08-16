package deployer

import (
	"fmt"
	"strings"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/modfile"
)

const agentgatewayModule = "github.com/agentgateway/agentgateway"

// getAgentgatewayVersionFromGoMod extracts the agentgateway version from go.mod if available.
func getAgentgatewayVersionFromGoMod() (string, error) {
	packages, err := modfile.Parse()
	if err != nil {
		return "", fmt.Errorf("failed to parse go.mod: %w", err)
	}

	// first prefer replaced with a specific version
	for _, replace := range packages.Replace {
		if replace.Old.Path == agentgatewayModule {
			if replace.New.Version != "" {
				return normalizeVersion(replace.New.Version), nil
			}
			return "", fmt.Errorf("agentgateway is replaced with a local path or commit, cannot determine version")
		}
	}

	for _, require := range packages.Require {
		if require.Path == agentgatewayModule {
			return normalizeVersion(require.Version), nil
		}
	}

	return "", fmt.Errorf("agentgateway dependency not found in go.mod")
}

// normalizeVersion removes the 'v' prefix from version strings to match Docker tag format
func normalizeVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

// GetAgentgatewayVersion returns the agentgateway version from go.mod,
func GetAgentgatewayVersion() string {
	version, err := getAgentgatewayVersionFromGoMod()
	if err != nil {
		return AgentgatewayDefaultTag
	}
	return version
}
