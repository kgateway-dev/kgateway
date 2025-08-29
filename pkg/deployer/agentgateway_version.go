package deployer

import (
	"fmt"
	"log/slog"
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

	// check if any replace directives exist for agentgateway
	// if multiple exist, the last one takes precedence
	var lastReplace *modfile.ReplacedGoPackage
	for i := range packages.Replace {
		if packages.Replace[i].Old.Path == agentgatewayModule {
			lastReplace = &packages.Replace[i]
		}
	}

	if lastReplace != nil {
		if lastReplace.New.Version != "" {
			return normalizeVersion(lastReplace.New.Version), nil
		}
		return "", fmt.Errorf("agentgateway is replaced with a local path or commit, cannot determine version")
	}

	// no replace directive found, check require directives
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
		slog.Warn("failed to get agentgateway version from go.mod, using default", "error", err, "default", AgentgatewayDefaultTag)
		return AgentgatewayDefaultTag
	}
	return version
}
