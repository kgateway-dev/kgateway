package main

import (
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
)

func main() {
	app := cmd.GlooCli()
	if err := app.Execute(); err != nil {
		//fmt.Println(err)
		os.Exit(1)
	}
}
