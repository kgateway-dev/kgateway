package install

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/solo-io/gloo/pkg/cliutil"
)

func KubectlApply(manifest []byte, extraArgs ...string) error {
	return Kubectl(bytes.NewBuffer(manifest), append([]string{"apply", "-f", "-"}, extraArgs...)...)
}

func KubectlApplyOut(manifest []byte, extraArgs ...string) ([]byte, error) {
	return KubectlOut(bytes.NewBuffer(manifest), append([]string{"apply", "-f", "-"}, extraArgs...)...)
}

func KubectlDelete(manifest []byte, extraArgs ...string) error {
	return Kubectl(bytes.NewBuffer(manifest), append([]string{"delete", "-f", "-"}, extraArgs...)...)
}

type KubeCli interface {
	Kubectl(stdin io.Reader, args ...string) error
	KubectlOut(stdin io.Reader, args ...string) ([]byte, error)
}

type CmdKubectl struct{}

var _ KubeCli = &CmdKubectl{}

func (k *CmdKubectl) Kubectl(stdin io.Reader, args ...string) error {
	return Kubectl(stdin, args...)
}

func (k *CmdKubectl) KubectlOut(stdin io.Reader, args ...string) ([]byte, error) {
	return KubectlOut(stdin, args...)
}

var verbose bool

func SetVerbose(b bool) {
	verbose = b
}

func Kubectl(stdin io.Reader, args ...string) error {
	kubectl := exec.Command("kubectl", args...)
	if stdin != nil {
		kubectl.Stdin = stdin
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "running kubectl command: %v\n", kubectl.Args)
		kubectl.Stdout = os.Stdout
		kubectl.Stderr = os.Stderr
	} else {
		// use logfile
		cliutil.Initialize()
		kubectl.Stdout = cliutil.GetLogger()
		kubectl.Stderr = cliutil.GetLogger()
	}
	return kubectl.Run()
}

func KubectlOut(stdin io.Reader, args ...string) ([]byte, error) {
	kubectl := exec.Command("kubectl", args...)

	if stdin != nil {
		kubectl.Stdin = stdin
	}

	var stdout, stderr io.Writer
	if verbose {
		fmt.Fprintf(os.Stderr, "running kubectl command: %v\n", kubectl.Args)
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		// use logfile
		cliutil.Initialize()
		stdout = cliutil.GetLogger()
		stderr = cliutil.GetLogger()
	}

	buf := &bytes.Buffer{}

	kubectl.Stdout = io.MultiWriter(stdout, buf)
	kubectl.Stderr = io.MultiWriter(stderr, buf)

	err := kubectl.Run()

	return buf.Bytes(), err
}
