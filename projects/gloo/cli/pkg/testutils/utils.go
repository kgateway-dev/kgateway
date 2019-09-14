package testutils

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/spf13/cobra"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func Glooctl(args string) error {
	app := cmd.GlooCli("test")
	return ExecuteCli(app, args)
}

func ExecuteCli(command *cobra.Command, args string) error {
	command.SetArgs(strings.Split(args, " "))
	return command.Execute()
}

func GlooctlOut(args string) (string, error) {
	app := cmd.GlooCli("test")
	return ExecuteCliOut(app, args)
}

func ExecuteCliOut(command *cobra.Command, args string) (string, error) {
	stdOut := os.Stdout
	stdErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	os.Stderr = w

	outC := make(chan string)

	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	command.SetArgs(strings.Split(args, " "))
	err = command.Execute()

	// back to normal state
	w.Close()
	os.Stdout = stdOut // restoring the real stdout
	os.Stderr = stdErr
	out := <-outC

	return strings.TrimSuffix(out, "\n"), err
}

func Make(dir, args string) error {
	make := exec.Command("make", strings.Split(args, " ")...)
	make.Dir = dir
	out, err := make.CombinedOutput()
	if err != nil {
		return errors.Errorf("make failed with err: %s", out)
	}
	return nil
}

func GetTestSettings() *v1.Settings {
	return &v1.Settings{
		Metadata: core.Metadata{
			Name:      "default",
			Namespace: defaults.GlooSystem,
		},
		BindAddr:        "test:80",
		ConfigSource:    &v1.Settings_DirectoryConfigSource{},
		DevMode:         true,
		SecretSource:    &v1.Settings_KubernetesSecretSource{},
		WatchNamespaces: []string{"default"},
		Extensions: &v1.Extensions{
			Configs: map[string]*types.Struct{
				"someotherextension": {},
			},
		},
	}
}
