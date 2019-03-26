package install

import (
	"bytes"
	"github.com/solo-io/gloo/pkg/cliutil"
	"io"
	"os/exec"
)

func KubectlApply(manifest []byte) error {
	return Kubectl(bytes.NewBuffer(manifest), "apply", "-f", "-")
}

type KubeCli interface {
	Kubectl(stdin io.Reader, args ...string) error
}

type CmdKubectl struct{}

func (k *CmdKubectl) Kubectl(stdin io.Reader, args ...string) error {
	return Kubectl(stdin, args...)
}

func Kubectl(stdin io.Reader, args ...string) error {
	kubectl := exec.Command("kubectl", args...)
	if stdin != nil {
		kubectl.Stdin = stdin
	}
	cliutil.Initialize()
	kubectl.Stdout = cliutil.Logger
	kubectl.Stderr = cliutil.Logger
	return kubectl.Run()
}
