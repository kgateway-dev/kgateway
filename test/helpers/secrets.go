package helpers

import (
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func NewTestSecrets() map[string]*dependencies.Secret {
	return map[string]*dependencies.Secret{
		"user": {
			Data: map[string]string{
				"username": "me@example.com",
				"password": "foobar",
			},
		},
	}
}
