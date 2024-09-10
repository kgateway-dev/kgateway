package proxy_syncer

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type SecretInputs struct {
	Secrets v1.SecretList
}
