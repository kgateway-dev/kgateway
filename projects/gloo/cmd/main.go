package main

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"github.com/solo-io/solo-kit/pkg/utils/stats"
)

func main() {
	stats.StartStatsServer()
	if err := setup.Main(); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}
