package testutils

import (
	"context"
	"os/exec"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil/glooctl"

	"github.com/solo-io/go-utils/threadsafe"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	errors "github.com/rotisserie/eris"
)

func Glooctl(argStr string) error {
	args := strings.Split(argStr, " ")
	return glooctl.NewCli().RunCommand(context.Background(), args...)
}

func GlooctlOut(argStr string) (string, error) {
	args := strings.Split(argStr, " ")

	var outLocation threadsafe.Buffer
	cmd := glooctl.NewCli().Command(context.Background(), args...).WithStdout(&outLocation)

	if runErr := cmd.Run(); runErr != nil {
		return "", runErr.Cause()
	}
	return outLocation.String(), nil
}

func Make(dir, args string) error {
	makeCmd := exec.Command("make", strings.Split(args, " ")...)
	makeCmd.Dir = dir
	out, err := makeCmd.CombinedOutput()
	if err != nil {
		return errors.Errorf("make failed with err: %s", out)
	}
	return nil
}

func GetTestSettings() *v1.Settings {
	return &v1.Settings{
		Metadata: &core.Metadata{
			Name:      "default",
			Namespace: defaults.GlooSystem,
		},
		Gloo: &v1.GlooOptions{
			XdsBindAddr: "test:80",
		},
		ConfigSource:    &v1.Settings_DirectoryConfigSource{},
		DevMode:         true,
		SecretSource:    &v1.Settings_KubernetesSecretSource{},
		WatchNamespaces: []string{"default"},
	}
}
