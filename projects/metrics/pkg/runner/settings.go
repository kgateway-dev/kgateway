package runner

import (
	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	ServerPort  int    `envconfig:"SERVER_PORT" default:"9966"`
	ServiceName string `envconfig:"SERVICE_NAME" default:"metrics"`
}

func NewSettings() Settings {
	var s Settings

	err := envconfig.Process("", &s)
	if err != nil {
		panic(err)
	}

	return s
}
