package testutils

import (
	"bytes"
	"context"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	errors "github.com/rotisserie/eris"
)

func Glooctl(argStr string) error {
	return NewCli().Execute(context.Background(), argStr)
}

func GlooctlOut(argStr string) (string, error) {
	return NewCli().ExecuteOut(context.Background(), argStr)
}

func ExecuteCommandWithArgs(command *cobra.Command, args ...string) error {
	command.SetArgs(args)
	return command.Execute()
}

// ExecuteCommandWithArgsOut executes the provided cobra.Command with the defined arguments
// Any output to Stdout or Stderr will be returned in a string, and if an error was encountered
// an error will be returned optionally
//
// NOTE:
//
//	cobra.Command's support configuring an alternative to using stdout and stderr
//	However, glooctl does not rely on this functionality and uses os.Stdout directly
//	We opt to bake this complexity directly into this tool, instead of forcing developers to
//	be aware of it. As a result, we do the following:
//		1. Capture the stdout and stderr Files
//		2. Update them to point to a writer of our choosing
//		3. Execute the command
//		4. Undo the change to stdout and stderr
//		5. Return the output string
//
// Update May 7th: @sam-heilbron tried to call this function within a struct following
// our cmdutils.Cmd interface. However, even with no functional changes, it was triggering
// a data-race when updating os.Stdout
func ExecuteCommandWithArgsOut(command *cobra.Command, args ...string) (string, error) {
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

	err = ExecuteCommandWithArgs(command, args...)

	// back to normal state
	w.Close()
	os.Stdout = stdOut // restoring the real stdout
	os.Stderr = stdErr
	out := <-outC

	return strings.TrimSuffix(out, "\n"), err
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
