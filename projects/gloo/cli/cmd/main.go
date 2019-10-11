package main

import (
	"context"
	"os"
	"strings"

	"github.com/solo-io/gloo/pkg/utils/setuputils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

const (
	args = "args"
)

func main() {
	setuputils.StartReportingUsage(context.Background(), &cliUsageReader{}, "glooctl")

	app := cmd.GlooCli()
	if err := app.Execute(); err != nil {
		//fmt.Println(err)
		os.Exit(1)
	}
}

type cliUsageReader struct {
}

func (c *cliUsageReader) GetPayload() (map[string]string, error) {
	return map[string]string{
		args: strings.Join(os.Args, "|"),
	}, nil
}
