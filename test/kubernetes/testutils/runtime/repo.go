package runtime

import (
	"os/exec"
	"strings"
)

// GlooDirectory returns the root directory of the Gloo project
// This is determined by running `git rev-parse --show-toplevel` and will vary between different clones of the repository (solo-projects vs. solo-io/gloo)
func GlooDirectory() string {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(data))
}
