package install

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/solo-io/gloo/pkg/cliutil"
)

// Deprecated: Prefer kubectl.Cli
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

// Deprecated: Prefer kubectl.Cli
func Kubectl(stdin io.Reader, args ...string) error {
	_, err := KubectlOut(stdin, args...)
	return err
}

// Deprecated: Prefer kubectl.Cli
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
