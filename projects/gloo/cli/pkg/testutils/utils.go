package testutils

import (
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func Glooctl(args string) error {
	app := cmd.GlooCli("test")
	app.SetArgs(strings.Split(args, " "))
	return app.Execute()
}
