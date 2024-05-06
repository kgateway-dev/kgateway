package runtime

import (
	"os/exec"
	"strings"
)

func GlooDirectory() string {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(data))
}
