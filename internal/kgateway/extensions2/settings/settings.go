package settings

import (
	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	EnableIstioIntegration bool   `split_words:"true"`
	EnableAutoMtls         bool   `split_words:"true"`
	StsClusterName         string `split_words:"true"`
	StsUri                 string `split_words:"true"`
}

// BuildSettings returns a zero-valued Settings obj if error is encountered when parsing env
func BuildSettings() (*Settings, error) {
	settings := &Settings{}
	if err := envconfig.Process("KGW", settings); err != nil {
		return settings, err
	}
	return settings, nil
}
