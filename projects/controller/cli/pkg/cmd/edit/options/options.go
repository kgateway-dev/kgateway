package options

import (
	"github.com/solo-io/gloo/projects/controller/cli/pkg/cmd/options"
)

type EditOptions struct {
	*options.Options
	ResourceVersion string
}
