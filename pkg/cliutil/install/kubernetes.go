package install

import (
	"bytes"
	"io"
	"os/exec"

	"github.com/solo-io/gloo/pkg/cliutil"
)

func KubectlApply(manifest []byte, extraArgs ...string) error {
	return Kubectl(bytes.NewBuffer(manifest), append([]string{"apply", "-f", "-"}, extraArgs...)...)
}

func KubectlDelete(manifest []byte, extraArgs ...string) error {
	return Kubectl(bytes.NewBuffer(manifest), append([]string{"delete", "-f", "-"}, extraArgs...)...)
}

type KubeCli interface {
	Kubectl(stdin io.Reader, args ...string) error
}

type CmdKubectl struct{}

func (k *CmdKubectl) Kubectl(stdin io.Reader, args ...string) error {
	return Kubectl(stdin, args...)
}

var verbose bool

func SetVerbose() {

}

func Kubectl(stdin io.Reader, args ...string) error {
	kubectl := exec.Command("kubectl", args...)
	if stdin != nil {
		kubectl.Stdin = stdin
	}
	cliutil.Initialize()
	kubectl.Stdout = cliutil.GetLogger()
	kubectl.Stderr = cliutil.GetLogger()
	return kubectl.Run()
}
