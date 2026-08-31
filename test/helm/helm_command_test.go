package helm

import (
	"os"
	"os/exec"
	"strings"
)

func helmCommand(args ...string) *exec.Cmd {
	helm := os.Getenv("HELM")
	if helm == "" {
		helm = "go tool helm"
	}

	cmdParts := strings.Fields(helm)
	if len(cmdParts) == 0 {
		cmdParts = []string{"go", "tool", "helm"}
	}
	return exec.Command(cmdParts[0], append(cmdParts[1:], args...)...) //nolint:gosec // G204: helm binary is sourced from the HELM env var (set by the developer/test harness), not untrusted input
}
