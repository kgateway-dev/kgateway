package runner

import (
	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	DebugPort   int    `envconfig:"DEBUG_PORT" default:"9091"`
	ServerPort  int    `envconfig:"SERVER_PORT" default:"8083"`
	ServiceName string `envconfig:"SERVICE_NAME" default:"AccessLog"`
}

func NewSettings() Settings {
	var s Settings

	err := envconfig.Process("", &s)
	if err != nil {
		panic(err)
	}

	return s
}
