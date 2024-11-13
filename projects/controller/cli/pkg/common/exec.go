package common

import (
	"io"
	"os/exec"
)

type Runner interface {
	Run(cmd string, args ...string) error
}

func NewShellRunner(in io.Reader, out io.Writer) Runner {
	return &runner{
		in:  in,
		out: out,
	}
}

type runner struct {
	in  io.Reader
	out io.Writer
}

func (r *runner) Run(cmd string, args ...string) error {
	execCmd := exec.Command(cmd, append([]string{"-eux", "-c"}, args...)...)
	execCmd.Stdin = r.in
	execCmd.Stdout = r.out
	execCmd.Stderr = r.out
	return execCmd.Run()
}
