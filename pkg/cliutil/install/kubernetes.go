package install

import (
	"bytes"
	"io"
	"os"
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
	kubectl.Stdout = os.Stdout
	kubectl.Stderr = os.Stderr
	return kubectl.Run()
}
