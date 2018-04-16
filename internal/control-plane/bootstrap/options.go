package bootstrap

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
)

type Options struct {
	bootstrap.Options
	EnvoyOptions EnvoyOptions
}

type EnvoyOptions struct {
	BindAddress string
}
