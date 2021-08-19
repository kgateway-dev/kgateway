package argsutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

const NameError = "name must be specified in flag (--name) or via first arg"

func MetadataArgsParse(opts *options.Options, args []string) error {
	// even if we are in interactive mode, we first want to check the flags and args for metadata and use those values if given
	if opts.Metadata.GetName() == "" && len(args) > 0 {
		// name is a special parameter that can be provided in the command list
		opts.Metadata.Name = args[0]
	}

	// if interactive mode, get any missing fields interactively
	if opts.Top.Interactive {
		return surveyutils.EnsureMetadataSurvey(opts.Top.Ctx, &opts.Metadata)
	}

	// if not interactive mode, ensure that the required fields were provided
	if opts.Metadata.GetName() == "" {
		return errors.Errorf(NameError)
	}
	// don't need to check namespace as is passed by a flag with a default value
	return nil
}
