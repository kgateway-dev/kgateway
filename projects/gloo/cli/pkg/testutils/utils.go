package testutils

import (
	"os/exec"
	"strings"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func Glooctl(args string) error {
	app := cmd.GlooCli("test")
	app.SetArgs(strings.Split(args, " "))
	return app.Execute()
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
