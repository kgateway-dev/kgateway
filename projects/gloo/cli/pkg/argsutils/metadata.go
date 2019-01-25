package argsutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

func MetadataArgsParse(opts *options.Options, args []string) error {
	if opts.Top.Interactive {
		return surveyutils.MetadataSurvey(&opts.Metadata)
	}
	switch {
	case opts.Metadata.Name != "":
	case opts.Metadata.Name == "" && len(args) > 0:
		opts.Metadata.Name = args[0]
	default:
		return errors.Errorf("name must be specified in flag (--name) or via first arg")
	}
	return nil
}
