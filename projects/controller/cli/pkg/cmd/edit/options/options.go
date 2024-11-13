package options

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

type EditOptions struct {
	*options.Options
	ResourceVersion string
}
